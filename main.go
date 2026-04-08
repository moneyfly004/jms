package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
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
	MaxRetries       = 3
	RetryDelay       = 2 * time.Second
)

// 全局预编译正则，提升性能
var linkPattern = regexp.MustCompile(`https?://[^\s"'<>]+`)

// GitHubSearchResult GitHub 搜索结果
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
	seenLinks   sync.Map // 并发安全的全局链接去重
	seenNodes   sync.Map // 并发安全的全局节点去重
}

func NewCollector(githubToken string) *Collector {
	return &Collector{
		githubToken: githubToken,
		httpClient: &http.Client{
			Timeout: 30 * time.Second, // 稍微缩短以防卡死
			Transport: &http.Transport{
				MaxIdleConns:        200,
				MaxIdleConnsPerHost: 20,
				IdleConnTimeout:     60 * time.Second,
				DisableKeepAlives:   false,
			},
		},
	}
}

// --- 核心采集与测试逻辑 (Pipeline并流架构) ---

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
	sem := make(chan struct{}, 10) // 限制并发请求GitHub API

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
	time.Sleep(2 * time.Second)
	return allLinks, nil
}

func (c *Collector) CollectNodesForKeyword(keyword string) (bool, error) {
	links, err := c.SearchKeywordLinks(keyword)
	if err != nil || len(links) == 0 {
		return false, err
	}

	log.Printf("关键词 [%s] 共提取到 %d 个链接，开始解析与测速...", keyword, len(links))

	// 使用 Pipeline 模式: fetch worker -> parse worker -> test worker
	nodeCh := make(chan string, 1000)
	validNodeCh := make(chan *ValidNode, 1000)

	// 1. 获取并解析订阅内容池
	var fetchWg sync.WaitGroup
	for _, link := range links {
		fetchWg.Add(1)
		go func(l string) {
			defer fetchWg.Done()
			content, err := c.FetchSubscription(l)
			if err != nil {
				return
			}
			nodes := c.ParseNodes(content)
			for _, n := range nodes {
				// 节点级别初步去重
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

	// 2. 节点并发测速池
	maxConcurrency := getEnvInt("MAX_CONCURRENCY", 50)
	var testWg sync.WaitGroup
	for i := 0; i < maxConcurrency; i++ {
		testWg.Add(1)
		go func() {
			defer testWg.Done()
			for nodeLink := range nodeCh {
				validNode := c.TestNode(nodeLink)
				if validNode.Error == nil {
					validNodeCh <- validNode
				}
			}
		}()
	}

	go func() {
		testWg.Wait()
		close(validNodeCh)
	}()

	// 3. 收集结果
	var validNodes []*ValidNode
	for vn := range validNodeCh {
		validNodes = append(validNodes, vn)
	}

	log.Printf("关键词 [%s] 测速完成，可用节点: %d 个", keyword, len(validNodes))

	// 写入文件
	if err := appendValidLinksToFile(links); err != nil {
		log.Printf("⚠️ 保存有效链接失败: %v", err)
	}
	if len(validNodes) > 0 {
		err := c.saveNodesToFile(validNodes, keyword)
		return true, err
	}
	return false, nil
}

// 统一写入节点逻辑
func (c *Collector) saveNodesToFile(validNodes []*ValidNode, keyword string) error {
	var allNodes []string
	
	// 读取已有文件并解码
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

	// 追加新节点
	for _, n := range validNodes {
		allNodes = append(allNodes, n.Link)
	}

	encodedContent := base64.StdEncoding.EncodeToString([]byte(strings.Join(allNodes, "\n")))
	if err := os.WriteFile("nodes.txt", []byte(encodedContent), 0644); err != nil {
		return fmt.Errorf("写入文件失败: %v", err)
	}

	log.Printf("关键词 [%s] 的节点已保存，当前文件总去重节点数: %d", keyword, len(allNodes))
	return nil
}

// --- 协议解析与格式化工具 (优化精简版) ---

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
		decoded = content // 尝试作为纯文本解析
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

// 统一的代理转换工厂，大幅消除重复代码
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
		if !isClash { method = getStr(proxy, "method") }
		pass := getStr(proxy, "password")
		if method != "" && pass != "" {
			auth := base64.StdEncoding.EncodeToString([]byte(method + ":" + pass))
			return fmt.Sprintf("ss://%s@%s:%s#%s", auth, server, port, url.QueryEscape(name))
		}
	case "vmess":
		uuid := getStr(proxy, "uuid")
		if uuid == "" { return "" }
		vmessData := map[string]interface{}{
			"v": "2", "ps": name, "add": server, "port": port, "id": uuid, "aid": 0, "net": "tcp", "type": "none", "tls": "",
		}
		// 简单处理TLS和Network
		if isClash {
			if tls, _ := proxy["tls"].(bool); tls {
				vmessData["tls"] = "tls"
				if sni := getStr(proxy, "sni"); sni != "" { vmessData["sni"] = sni }
			}
			if netw := getStr(proxy, "network"); netw != "" { vmessData["net"] = netw }
		} else {
			if tls, ok := proxy["tls"].(map[string]interface{}); ok {
				vmessData["tls"] = "tls"
				if sni := getStr(tls, "server_name"); sni != "" { vmessData["sni"] = sni }
			}
		}
		jsonData, _ := json.Marshal(vmessData)
		return "vmess://" + base64.StdEncoding.EncodeToString(jsonData)
	case "vless", "trojan", "tuic", "hysteria", "hysteria2":
		// 使用 URL Scheme 构建通用格式
		u := url.URL{Scheme: strings.TrimRight(pType, "2"), Host: fmt.Sprintf("%s:%s", server, port), Fragment: name}
		if pType == "vless" || pType == "tuic" {
			u.User = url.User(getStr(proxy, "uuid"))
		} else if pType == "trojan" {
			u.User = url.User(getStr(proxy, "password"))
		}
		q := url.Values{}
		if sni := getStr(proxy, "sni"); sni != "" { q.Set("sni", sni) }
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

// --- 核心：精准的节点解析与 TCP 测速 ---

func (c *Collector) TestNode(nodeLink string) *ValidNode {
	result := &ValidNode{Link: nodeLink}
	
	// 提取真实的 IP/域名 和 端口
	host, port, err := extractHostPort(nodeLink)
	if err != nil {
		result.Error = fmt.Errorf("解析URL失败: %v", err)
		return result
	}

	timeout := time.Duration(getEnvInt("TEST_TIMEOUT", 3)) * time.Second
	address := net.JoinHostPort(host, port)

	// TCP 测速
	start := time.Now()
	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		result.Error = err
		return result
	}
	conn.Close()
	result.Latency = time.Since(start)
	
	// 简单的类型提取
	result.Type = strings.SplitN(nodeLink, "://", 2)[0]
	return result
}

// extractHostPort 准确解析各种协议的连接地址 (替代原缺失的 ParseNodeLink)
func extractHostPort(link string) (string, string, error) {
	if strings.HasPrefix(link, "vmess://") {
		decoded, err := safeDecodeBase64(strings.TrimPrefix(link, "vmess://"))
		if err != nil { return "", "", err }
		var v map[string]interface{}
		if json.Unmarshal([]byte(decoded), &v) != nil { return "", "", fmt.Errorf("vmess json invalid") }
		return getStr(v, "add"), getStrPort(v["port"]), nil
	}

	if strings.HasPrefix(link, "ssr://") {
		decoded, err := safeDecodeBase64(strings.TrimPrefix(link, "ssr://"))
		if err != nil { return "", "", err }
		parts := strings.Split(decoded, ":")
		if len(parts) >= 2 { return parts[0], parts[1], nil }
		return "", "", fmt.Errorf("ssr format invalid")
	}

	// 其他标准 URI (ss, vless, trojan, hysteria 等)
	u, err := url.Parse(link)
	if err != nil {
		// 针对老旧不规范 ss:// 链接的容错
		if strings.HasPrefix(link, "ss://") && !strings.Contains(link, "@") {
			decoded, _ := safeDecodeBase64(strings.TrimPrefix(link, "ss://"))
			parts := strings.Split(decoded, "@")
			if len(parts) == 2 {
				u, err = url.Parse("ss://placeholder@" + parts[1])
			}
		}
		if err != nil { return "", "", err }
	}
	
	host, port, err := net.SplitHostPort(u.Host)
	if err != nil { return u.Host, "443", nil } // 默认443
	return host, port, nil
}

// --- 基础网络请求与通用工具类 ---

func (c *Collector) makeRequest(url string, result interface{}) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil { return err }
	if c.githubToken != "" { req.Header.Set("Authorization", "Bearer "+c.githubToken) }
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	for i := 0; i < MaxRetries; i++ {
		resp, err := c.httpClient.Do(req)
		if err == nil && resp.StatusCode == 200 {
			defer resp.Body.Close()
			return json.NewDecoder(resp.Body).Decode(result)
		}
		if resp != nil { resp.Body.Close() }
		time.Sleep(RetryDelay)
	}
	return fmt.Errorf("请求重试失败")
}

func (c *Collector) getFileContent(apiURL string) (string, error) {
	var fileContent GitHubFileContent
	if err := c.makeRequest(apiURL, &fileContent); err != nil { return "", err }
	if fileContent.Encoding == "base64" {
		return safeDecodeBase64(strings.ReplaceAll(fileContent.Content, "\n", ""))
	}
	return fileContent.Content, nil
}

func (c *Collector) FetchSubscription(link string) (string, error) {
	req, _ := http.NewRequest("GET", link, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := c.httpClient.Do(req)
	if err != nil || resp.StatusCode != 200 {
		if resp != nil { resp.Body.Close() }
		return "", fmt.Errorf("fetch error")
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return string(body), nil
}

// --- Helper Functions 助手函数 ---

func cleanLink(link string) string {
	link = strings.TrimSpace(link)
	link = strings.TrimRight(link, ".,;!?)>\"'")
	return link
}

func getStr(m map[string]interface{}, key string) string {
	if val, ok := m[key].(string); ok { return val }
	return ""
}

func getStrPort(p interface{}) string {
	switch v := p.(type) {
	case int: return strconv.Itoa(v)
	case float64: return strconv.Itoa(int(v))
	case string: return v
	default: return "443"
	}
}

func getEnvInt(key string, defaultVal int) int {
	if s := os.Getenv(key); s != "" {
		if v, err := strconv.Atoi(s); err == nil { return v }
	}
	return defaultVal
}

// safeDecodeBase64 安全的 Base64 解码，自动处理 padding 和标准/URL编码
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
	if err != nil { return nil, err }
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
	if err != nil { return err }
	defer file.Close()

	writer := bufio.NewWriter(file)
	for _, link := range newLinks {
		fmt.Fprintln(writer, link)
	}
	return writer.Flush()
}

// ======= 主程序入口 =======

func main() {
	timeout := 60 * time.Minute
	if timeoutEnv := os.Getenv("COLLECT_TIMEOUT"); timeoutEnv != "" {
		if d, err := time.ParseDuration(timeoutEnv); err == nil && d > 0 {
			timeout = d
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	done := make(chan bool, 1)
	collector := NewCollector(os.Getenv("GITHUB_TOKEN"))

	go func() {
		defer func() { done <- true }()

		keywords, err := loadKeywords("keywords.txt")
		if err != nil || len(keywords) == 0 {
			log.Fatalf("❌ 无法加载 keywords.txt 或文件为空 (%v)，程序终止", err)
			return
		}

		log.Printf("✅ 成功加载 %d 个自定义关键词，开始执行自动化采集...", len(keywords))

		for _, keyword := range keywords {
			collector.CollectNodesForKeyword(keyword)
			time.Sleep(1 * time.Second) // 降低触发风控概率
		}
		log.Println("========== 所有关键词采集并测速打包任务完成 ==========")
	}()

	select {
	case <-done:
		log.Println("========== 工作流正常结束 ==========")
	case <-ctx.Done():
		log.Printf("⏰ 采集超时（%v），强制停止", timeout)
	}
}
