package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	GitHubAPIBaseURL = "https://api.github.com"
	MaxRetries       = 5
	RetryDelay       = 3 * time.Second
	SearchInterval   = 15 * time.Second // 避免429限流
)

var linkPattern = regexp.MustCompile(`https?://[^\s"'<>]+`)

type GitHubSearchResult struct {
	TotalCount int `json:"total_count"`
	Items      []struct {
		HTMLURL string `json:"html_url"`
		APIURL  string `json:"url"`
		Path    string `json:"path"`
		Repo    struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
	} `json:"items"`
}

type GitHubFileContent struct {
	Content  string `json:"content"`
	Encoding string `json:"encoding"`
}

type ValidNode struct {
	Link    string
	Type    string
	Latency time.Duration
	Error   error
}

type Collector struct {
	githubToken string
	httpClient  *http.Client
	seenLinks   sync.Map
	seenNodes   sync.Map
}

func NewCollector(githubToken string) *Collector {
	return &Collector{
		githubToken: githubToken,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        200,
				MaxIdleConnsPerHost: 20,
				IdleConnTimeout:     60 * time.Second,
				DisableKeepAlives:   false,
			},
		},
	}
}

func (c *Collector) PreFilterLinks(links []string) []string {
	type filterResult struct {
		link  string
		valid bool
	}
	resultCh := make(chan filterResult, len(links))
	sem := make(chan struct{}, 30)
	var wg sync.WaitGroup

	client := &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}

	for _, link := range links {
		wg.Add(1)
		go func(l string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			u, err := url.Parse(l)
			if err != nil || u.Host == "" {
				resultCh <- filterResult{l, false}
				return
			}

			req, err := http.NewRequest("HEAD", l, nil)
			if err != nil {
				resultCh <- filterResult{l, false}
				return
			}
			req.Header.Set("User-Agent", "Mozilla/5.0")

			resp, err := client.Do(req)
			if err != nil {
				req2, _ := http.NewRequest("GET", l, nil)
				if req2 == nil {
					resultCh <- filterResult{l, false}
					return
				}
				req2.Header.Set("User-Agent", "Mozilla/5.0")
				resp, err = client.Do(req2)
				if err != nil {
					resultCh <- filterResult{l, false}
					return
				}
			}
			resp.Body.Close()

			valid := resp.StatusCode < 400
			resultCh <- filterResult{l, valid}
		}(link)
	}

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	var validLinks []string
	for r := range resultCh {
		if r.valid {
			validLinks = append(validLinks, r.link)
		}
	}

	filtered := len(links) - len(validLinks)
	if filtered > 0 {
		log.Printf("🔗 预过滤: %d/%d 链接可访问，剔除 %d 个死链", len(validLinks), len(links), filtered)
	}
	return validLinks
}

func (c *Collector) SearchKeywordLinks(keyword string) ([]string, error) {
	log.Printf("正在 GitHub 搜索关键词: %s", keyword)
	searchURL := fmt.Sprintf("%s/search/code?q=%s&per_page=100", GitHubAPIBaseURL, url.QueryEscape(keyword))
	var results GitHubSearchResult
	if err := c.makeRequest(searchURL, &results); err != nil {
		return nil, fmt.Errorf("搜索失败: %v", err)
	}

	log.Printf("找到 %d 个代码文件结果", results.TotalCount)

	var allLinks []string
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 20)

	for _, item := range results.Items {
		wg.Add(1)
		go func(apiURL string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			fileContent, err := c.getFileContent(apiURL)
			if err != nil {
				return
			}

			matches := linkPattern.FindAllString(fileContent, -1)
			for _, match := range matches {
				link := cleanLink(match)
				if link != "" && strings.Contains(link, keyword) {
					if _, loaded := c.seenLinks.LoadOrStore(link, true); !loaded {
						mu.Lock()
						allLinks = append(allLinks, link)
						mu.Unlock()
						log.Printf("发现匹配链接: %s", link)
					}
				}
			}
		}(item.APIURL)
	}
	wg.Wait()
	return allLinks, nil
}

func (c *Collector) ProcessKeywordLinks(keyword string, links []string) bool {
	links = c.PreFilterLinks(links)
	if len(links) == 0 {
		log.Printf("关键词 [%s] 所有链接均不可访问，跳过", keyword)
		return false
	}
	log.Printf("关键词 [%s] 共 %d 个有效链接，开始解析与测速...", keyword, len(links))

	nodeCh := make(chan string, 1000)
	validNodeCh := make(chan *ValidNode, 1000)

	var fetchWg sync.WaitGroup
	fetchSem := make(chan struct{}, 20)
	for _, link := range links {
		fetchWg.Add(1)
		go func(l string) {
			defer fetchWg.Done()
			fetchSem <- struct{}{}
			defer func() { <-fetchSem }()
			content, err := c.FetchSubscription(l)
			if err != nil {
				return
			}
			nodes := c.ParseNodes(content)
			for _, n := range nodes {
				if _, loaded := c.seenNodes.LoadOrStore(n, true); !loaded {
					nodeCh <- n
				}
			}
		}(link)
	}

	go func() {
		fetchWg.Wait()
		close(nodeCh)
	}()

	maxConcurrency := getEnvInt("MAX_CONCURRENCY", 30)
	var testWg sync.WaitGroup
	for i := 0; i < maxConcurrency; i++ {
		testWg.Add(1)
		go func() {
			defer testWg.Done()
			for nodeLink := range nodeCh {
				// 强制仅使用 Sing-box 测速
				validNode := c.TestNodeWithSingBox(nodeLink)
				if validNode != nil && validNode.Error == nil {
					validNodeCh <- validNode
				}
			}
		}()
	}

	go func() {
		testWg.Wait()
		close(validNodeCh)
	}()

	var validNodes []*ValidNode
	for vn := range validNodeCh {
		validNodes = append(validNodes, vn)
	}

	log.Printf("关键词 [%s] 测速完成，可用节点: %d 个", keyword, len(validNodes))

	if len(validNodes) > 0 {
		if err := appendValidLinksToFile(links); err != nil {
			log.Printf("⚠️ 保存有效链接失败: %v", err)
		}
		c.saveNodesToFile(validNodes, keyword)
		return true
	}
	return false
}

func (c *Collector) saveNodesToFile(validNodes []*ValidNode, keyword string) error {
	var allNodes []string
	if content, err := os.ReadFile("nodes.txt"); err == nil {
		decoded, err := safeDecodeBase64(strings.TrimSpace(string(content)))
		if err == nil {
			for _, line := range strings.Split(decoded, "\n") {
				line = strings.TrimSpace(line)
				if line != "" {
					allNodes = append(allNodes, line)
					c.seenNodes.Store(line, true)
				}
			}
		}
	}

	for _, n := range validNodes {
		allNodes = append(allNodes, n.Link)
	}

	encodedContent := base64.StdEncoding.EncodeToString([]byte(strings.Join(allNodes, "\n")))
	outputFile := "nodes.txt"
	if err := os.WriteFile(outputFile, []byte(encodedContent), 0644); err != nil {
		return fmt.Errorf("写入文件失败: %v", err)
	}

	log.Printf("关键词 [%s] 的节点已保存，当前文件总去重节点数: %d", keyword, len(allNodes))

	gistID, gistToken := os.Getenv("GIST_ID"), os.Getenv("GIST_TOKEN")
	if gistToken == "" {
		gistToken = c.githubToken
	}
	if gistID != "" || gistToken != "" {
		_ = c.PushToGist(outputFile, validNodes)
	}

	return nil
}

func (c *Collector) ParseNodes(content string) []string {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}
	if nodes := c.parseClashYAML(content); len(nodes) > 0 {
		return nodes
	}
	if nodes := c.parseSingBoxJSON(content); len(nodes) > 0 {
		return nodes
	}
	decoded, err := safeDecodeBase64(content)
	if err != nil {
		decoded = content
	}
	return c.extractNodeLinks(decoded)
}

func (c *Collector) parseClashYAML(content string) []string {
	var config struct {
		Proxies []map[string]interface{} `yaml:"proxies"`
	}
	if yaml.Unmarshal([]byte(content), &config) != nil {
		return nil
	}
	var nodes []string
	for _, proxy := range config.Proxies {
		if link := convertProxyToLink(proxy, true); link != "" {
			nodes = append(nodes, link)
		}
	}
	return nodes
}

func (c *Collector) parseSingBoxJSON(content string) []string {
	var config struct {
		Outbounds []map[string]interface{} `json:"outbounds"`
	}
	if json.Unmarshal([]byte(content), &config) != nil {
		return nil
	}
	var nodes []string
	for _, ob := range config.Outbounds {
		if link := convertProxyToLink(ob, false); link != "" {
			nodes = append(nodes, link)
		}
	}
	return nodes
}

func convertProxyToLink(proxy map[string]interface{}, isClash bool) string {
	pType := getStr(proxy, "type")
	server := getStr(proxy, "server")
	if pType == "" || server == "" {
		return ""
	}
	port := "443"
	if isClash {
		port = getStrPort(proxy["port"])
	} else {
		port = getStrPort(proxy["server_port"])
	}

	name := getStr(proxy, "name")
	if !isClash {
		name = getStr(proxy, "tag")
	}

	switch pType {
	case "ss", "shadowsocks":
		method := getStr(proxy, "cipher")
		if !isClash {
			method = getStr(proxy, "method")
		}
		pass := getStr(proxy, "password")
		if method != "" && pass != "" {
			auth := base64.StdEncoding.EncodeToString([]byte(method + ":" + pass))
			return fmt.Sprintf("ss://%s@%s:%s#%s", auth, server, port, url.QueryEscape(name))
		}
	case "vmess":
		uuid := getStr(proxy, "uuid")
		if uuid == "" {
			return ""
		}
		vmessData := map[string]interface{}{
			"v": "2", "ps": name, "add": server, "port": port, "id": uuid, "aid": 0, "net": "tcp", "type": "none", "tls": "",
		}
		if isClash {
			if tls, _ := proxy["tls"].(bool); tls {
				vmessData["tls"] = "tls"
				if sni := getStr(proxy, "sni"); sni != "" {
					vmessData["sni"] = sni
				}
			}
			if netw := getStr(proxy, "network"); netw != "" {
				vmessData["net"] = netw
			}
		} else {
			if tls, ok := proxy["tls"].(map[string]interface{}); ok {
				vmessData["tls"] = "tls"
				if sni := getStr(tls, "server_name"); sni != "" {
					vmessData["sni"] = sni
				}
			}
		}
		jsonData, _ := json.Marshal(vmessData)
		return "vmess://" + base64.StdEncoding.EncodeToString(jsonData)
	case "vless", "trojan", "tuic", "hysteria", "hysteria2":
		u := url.URL{Scheme: strings.TrimRight(pType, "2"), Host: fmt.Sprintf("%s:%s", server, port), Fragment: name}
		if pType == "vless" || pType == "tuic" {
			u.User = url.User(getStr(proxy, "uuid"))
		} else if pType == "trojan" {
			u.User = url.User(getStr(proxy, "password"))
		}
		q := url.Values{}
		if sni := getStr(proxy, "sni"); sni != "" {
			q.Set("sni", sni)
		}
		u.RawQuery = q.Encode()
		return u.String()
	}
	return ""
}

func (c *Collector) extractNodeLinks(content string) []string {
	var nodes []string
	prefixes := []string{"ss://", "vmess://", "vless://", "trojan://", "ssr://", "hysteria://", "hy2://", "tuic://", "wg://"}
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		for _, p := range prefixes {
			if strings.HasPrefix(line, p) {
				nodes = append(nodes, line)
				break
			}
		}
	}
	return nodes
}

func (c *Collector) makeRequest(apiURL string, result interface{}) error {
	for i := 0; i < MaxRetries; i++ {
		req, err := http.NewRequest("GET", apiURL, nil)
		if err != nil {
			return err
		}
		if c.githubToken != "" {
			req.Header.Set("Authorization", "Bearer "+c.githubToken)
		}
		req.Header.Set("Accept", "application/vnd.github.v3+json")
		resp, err := c.httpClient.Do(req)
		if err != nil {
			if i == MaxRetries-1 {
				log.Printf("API 请求网络错误: %v (URL: %s)", err, apiURL)
			}
			time.Sleep(RetryDelay * time.Duration(i+1))
			continue
		}

		if resp.StatusCode == 200 {
			defer resp.Body.Close()
			return json.NewDecoder(resp.Body).Decode(result)
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode == 403 || resp.StatusCode == 429 {
			waitDur := c.parseRateLimitWait(resp)
			log.Printf("⏳ API 限流 (HTTP %d)，等待 %v 后重试 (%d/%d) (URL: %s)",
				resp.StatusCode, waitDur.Round(time.Second), i+1, MaxRetries, apiURL)
			time.Sleep(waitDur)
			continue
		}

		if i == MaxRetries-1 {
			log.Printf("API 请求失败 HTTP %d: %s (URL: %s)", resp.StatusCode, string(body[:min(len(body), 200)]), apiURL)
		}
		time.Sleep(RetryDelay * time.Duration(i+1))
	}
	return fmt.Errorf("请求重试 %d 次后失败", MaxRetries)
}

func (c *Collector) parseRateLimitWait(resp *http.Response) time.Duration {
	if ra := resp.Header.Get("Retry-After"); ra != "" {
		if secs, err := strconv.Atoi(ra); err == nil && secs > 0 {
			return time.Duration(secs) * time.Second
		}
	}
	if reset := resp.Header.Get("X-RateLimit-Reset"); reset != "" {
		if ts, err := strconv.ParseInt(reset, 10, 64); err == nil {
			waitDur := time.Until(time.Unix(ts, 0)) + 2*time.Second
			if waitDur > 0 && waitDur < 120*time.Second {
				return waitDur
			}
		}
	}
	return 60 * time.Second
}

func (c *Collector) getFileContent(apiURL string) (string, error) {
	var fileContent GitHubFileContent
	if err := c.makeRequest(apiURL, &fileContent); err != nil {
		return "", err
	}
	if fileContent.Encoding == "base64" {
		return safeDecodeBase64(strings.ReplaceAll(fileContent.Content, "\n", ""))
	}
	return fileContent.Content, nil
}

func (c *Collector) FetchSubscription(link string) (string, error) {
	req, err := http.NewRequest("GET", link, nil)
	if err != nil {
		return "", fmt.Errorf("无效URL: %v", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("fetch error: HTTP %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	return string(body), nil
}

func cleanLink(link string) string {
	link = strings.TrimSpace(link)
	link = strings.TrimRight(link, ".,;!?)>'\"")
	return link
}

func getStr(m map[string]interface{}, key string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return ""
}

func getStrPort(p interface{}) string {
	switch v := p.(type) {
	case int:
		return strconv.Itoa(v)
	case float64:
		return strconv.Itoa(int(v))
	case string:
		return v
	default:
		return "443"
	}
}

func getEnvInt(key string, defaultVal int) int {
	if s := os.Getenv(key); s != "" {
		if v, err := strconv.Atoi(s); err == nil {
			return v
		}
	}
	return defaultVal
}

func safeDecodeBase64(s string) (string, error) {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "-", "+")
	s = strings.ReplaceAll(s, "_", "/")
	if m := len(s) % 4; m != 0 {
		s += strings.Repeat("=", 4-m)
	}
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		b, err = base64.URLEncoding.DecodeString(s)
	}
	return string(b), err
}

func loadKeywords(filename string) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var keywords []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if line := strings.TrimSpace(scanner.Text()); line != "" && !strings.HasPrefix(line, "#") {
			keywords = append(keywords, line)
		}
	}
	return keywords, scanner.Err()
}

func appendValidLinksToFile(newLinks []string) error {
	file, err := os.OpenFile("links.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()
	writer := bufio.NewWriter(file)
	for _, link := range newLinks {
		fmt.Fprintln(writer, link)
	}
	return writer.Flush()
}

func testNodesFromFile() {
	if len(os.Args) < 3 {
		fmt.Println("用法: go run . test-nodes <节点文件>")
		return
	}
	file, err := os.Open(os.Args[2])
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	var nodes []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			nodes = append(nodes, line)
		}
	}

	collector := NewCollector("")
	var wg sync.WaitGroup
	var mu sync.Mutex
	results := make(map[int]*ValidNode)

	for i, nodeLink := range nodes {
		wg.Add(1)
		go func(index int, link string) {
			defer wg.Done()
			validNode := collector.TestNodeWithSingBox(link)
			mu.Lock()
			results[index] = validNode
			mu.Unlock()
		}(i, nodeLink)
	}
	wg.Wait()

	validCount := 0
	for i := 0; i < len(nodes); i++ {
		if results[i] != nil && results[i].Error == nil {
			validCount++
		}
	}
	fmt.Printf("\n总计: %d 个节点, %d 个可用, %d 个不可用\n", len(nodes), validCount, len(nodes)-validCount)
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "test-nodes" {
		testNodesFromFile()
		return
	}

	// 修复未使用变量报错：直接使用 context 控制超时
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
	defer cancel()

	done := make(chan bool, 1)
	collector := NewCollector(os.Getenv("GITHUB_TOKEN"))

	go func() {
		defer func() { done <- true }()

		keywords, err := loadKeywords("keywords.txt")
		if err != nil || len(keywords) == 0 {
			log.Fatalf("❌ 无法加载 keywords.txt 或文件为空 (%v)，程序终止", err)
		}

		_ = os.Remove("nodes.txt")
		_ = os.Remove("links.txt")
		log.Println("🧹 已清理本地残留文件，确保推送的都是本次采集结果")
		log.Printf("✅ 成功加载 %d 个关键词", len(keywords))
		log.Println("开始执行自动化采集...")

		var bgWg sync.WaitGroup
		var failedMu sync.Mutex
		var failedKeywords []string

		for i, keyword := range keywords {
			links, err := collector.SearchKeywordLinks(keyword)
			if err != nil || len(links) == 0 {
				failedMu.Lock()
				failedKeywords = append(failedKeywords, keyword)
				failedMu.Unlock()
				log.Printf("⚠️ 关键词 [%s] 无搜索结果，标记为失效", keyword)
				if i < len(keywords)-1 {
					time.Sleep(SearchInterval)
				}
				continue
			}

			bgWg.Add(1)
			go func(kw string, kLinks []string) {
				defer bgWg.Done()
				hasNodes := collector.ProcessKeywordLinks(kw, kLinks)
				if !hasNodes {
					failedMu.Lock()
					failedKeywords = append(failedKeywords, kw)
					failedMu.Unlock()
					log.Printf("⚠️ 关键词 [%s] 无可用节点，标记为失效", kw)
				}
			}(keyword, links)

			if i < len(keywords)-1 {
				time.Sleep(SearchInterval)
			}
		}
		bgWg.Wait()

		if len(failedKeywords) > 0 {
			log.Printf("⚠️ 以下 %d 个关键词本次未搜索到结果: %v", len(failedKeywords), failedKeywords)
		}

		log.Println("========== 所有关键词采集并测速打包任务完成 ==========")
	}()

	select {
	case <-done:
		log.Println("========== 工作流正常结束 ==========")
	case <-ctx.Done():
		log.Printf("⏰ 采集超时（1小时），强制停止")
	}
}
