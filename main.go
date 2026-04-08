package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"io"
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
	MaxRetries       = 5
	RetryDelay       = 3 * time.Second
	SearchInterval   = 3 * time.Second
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

// 轻量级并发安全日志器
type Logger struct {
	mu        sync.Mutex
	startTime time.Time
	total     int
	current   int
}

func NewLogger(total int) *Logger {
	return &Logger{startTime: time.Now(), total: total}
}

func (l *Logger) Info(msg string)   { l.print("ℹ️", msg, "\033[34m", "\033[0m") }
func (l *Logger) Success(msg string){ l.print("✅", msg, "\033[32m", "\033[0m") }
func (l *Logger) Warn(msg string)   { l.print("⚠️", msg, "\033[33m", "\033[0m") }
func (l *Logger) Error(msg string)  { l.print("❌", msg, "\033[31m", "\033[0m") }
func (l *Logger) Progress(msg string) {
	l.mu.Lock()
	l.current++
	fmt.Printf("\r\033[36m⚡ 进度 [%d/%d] %s...\033[0m", l.current, l.total, msg)
	l.mu.Unlock()
}
func (l *Logger) DoneProgress() {
	l.mu.Lock()
	fmt.Println()
	l.mu.Unlock()
}
func (l *Logger) PrintSummary(links, nodes int) {
	l.mu.Lock()
	elapsed := time.Since(l.startTime).Round(time.Second)
	fmt.Println("\n" + strings.Repeat("─", 45))
	fmt.Printf("📊 采集统计 | 耗时: %v\n", elapsed)
	fmt.Printf("🔗 有效订阅: %d 个\n", links)
	fmt.Printf("🌐 可用节点: %d 个\n", nodes)
	fmt.Println(strings.Repeat("─", 45))
	l.mu.Unlock()
}
func (l *Logger) print(icon, msg, color, reset string) {
	l.mu.Lock()
	fmt.Printf("%s%s [%s] %s%s\n", color, icon, time.Now().Format("15:04:05"), msg, reset)
	l.mu.Unlock()
}

// 全局内存存储（替代高频文件读写）
var (
	globalMu    sync.Mutex
	globalLinks = make(map[string]struct{})
	globalNodes = make(map[string]struct{})
	validLinks  []string
	validNodes  []*ValidNode
)

type Collector struct {
	githubToken string
	httpClient  *http.Client
	seenLinks   sync.Map
	seenNodes   sync.Map
	logger      *Logger
}

func NewCollector(githubToken string, log *Logger) *Collector {
	return &Collector{
		githubToken: githubToken,
		logger:      log,
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
		c.logger.Info(fmt.Sprintf("预过滤: %d/%d 链接可访问，剔除 %d 个死链", len(validLinks), len(links), filtered))
	}
	return validLinks
}

func (c *Collector) SearchKeywordLinks(keyword string) ([]string, error) {
	c.logger.Info(fmt.Sprintf("正在 GitHub 搜索关键词: %s", keyword))
	searchURL := fmt.Sprintf("%s/search/code?q=%s&per_page=100", GitHubAPIBaseURL, url.QueryEscape(keyword))
	var results GitHubSearchResult
	if err := c.makeRequest(searchURL, &results); err != nil {
		return nil, fmt.Errorf("搜索失败: %v", err)
	}

	c.logger.Info(fmt.Sprintf("找到 %d 个代码文件结果", results.TotalCount))

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
					}
				}
			}
		}(item.APIURL)
	}
	wg.Wait()
	return allLinks, nil
}

func (c *Collector) CollectNodesForKeyword(keyword string) (bool, error) {
	links, err := c.SearchKeywordLinks(keyword)
	if err != nil || len(links) == 0 {
		return false, err
	}
	return c.ProcessKeywordLinks(keyword, links), nil
}

func (c *Collector) ProcessKeywordLinks(keyword string, links []string) bool {
	links = c.PreFilterLinks(links)
	if len(links) == 0 {
		c.logger.Warn(fmt.Sprintf("关键词 [%s] 所有链接均不可访问，跳过", keyword))
		return false
	}
	c.logger.Info(fmt.Sprintf("关键词 [%s] 共 %d 个有效链接，开始解析与测速...", keyword, len(links)))

	nodeCh := make(chan string, 2000)
	validNodeCh := make(chan *ValidNode, 2000)

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

	maxConcurrency := getEnvInt("MAX_CONCURRENCY", 40)
	var testWg sync.WaitGroup
	for i := 0; i < maxConcurrency; i++ {
		testWg.Add(1)
		go func() {
			defer testWg.Done()
			for nodeLink := range nodeCh {
				c.logger.Progress(nodeLink)
				var validNode *ValidNode
				if os.Getenv("USE_SINGBOX") == "false" {
					validNode = c.TestNode(nodeLink)
				} else {
					validNode = c.TestNodeWithSingBox(nodeLink)
				}

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

	var foundNodes []*ValidNode
	for vn := range validNodeCh {
		foundNodes = append(foundNodes, vn)
	}
	c.logger.DoneProgress()

	if len(foundNodes) > 0 {
		globalMu.Lock()
		for _, l := range links {
			if _, ok := globalLinks[l]; !ok {
				globalLinks[l] = struct{}{}
				validLinks = append(validLinks, l)
			}
		}
		for _, n := range foundNodes {
			if _, ok := globalNodes[n.Link]; !ok {
				globalNodes[n.Link] = struct{}{}
				validNodes = append(validNodes, n)
			}
		}
		globalMu.Unlock()
		c.logger.Success(fmt.Sprintf("关键词 [%s] 产出 %d 个可用节点", keyword, len(foundNodes)))
		return true
	}
	c.logger.Warn(fmt.Sprintf("关键词 [%s] 无可用节点", keyword))
	return false
}

// finalizeResults 统一写入文件并推送 Gist
func finalizeResults(log *Logger, token string) {
	if len(validLinks) == 0 && len(validNodes) == 0 {
		log.Warn("本次采集未获得有效数据")
		return
	}

	if err := os.WriteFile("links.txt", []byte(strings.Join(validLinks, "\n")), 0644); err != nil {
		log.Error(fmt.Sprintf("写入 links.txt 失败: %v", err))
	}

	var nodeLines []string
	for _, n := range validNodes {
		nodeLines = append(nodeLines, n.Link)
	}
	encoded := base64.StdEncoding.EncodeToString([]byte(strings.Join(nodeLines, "\n")))
	if err := os.WriteFile("nodes.txt", []byte(encoded), 0644); err != nil {
		log.Error(fmt.Sprintf("写入 nodes.txt 失败: %v", err))
	}

	log.Success(fmt.Sprintf("已写入文件: links.txt(%d) nodes.txt(%d)", len(validLinks), len(validNodes)))

	collector := NewCollector(token, log)
	_ = collector.PushToGistFinal()
}

// 核心网络与工具函数
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
				c.logger.Error(fmt.Sprintf("API 请求网络错误: %v (URL: %s)", err, apiURL))
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
			c.logger.Warn(fmt.Sprintf("⏳ API 限流 (HTTP %d)，等待 %v 后重试 (%d/%d)", resp.StatusCode, waitDur.Round(time.Second), i+1, MaxRetries))
			time.Sleep(waitDur)
			continue
		}

		if i == MaxRetries-1 {
			c.logger.Error(fmt.Sprintf("API 请求失败 HTTP %d: %s", resp.StatusCode, string(body[:min(len(body), 200)])))
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

func removeKeywords(filename string, toRemove []string) {
	removeSet := make(map[string]bool)
	for _, k := range toRemove {
		removeSet[k] = true
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		return
	}

	var kept []string
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			kept = append(kept, line)
			continue
		}
		if !removeSet[trimmed] {
			kept = append(kept, line)
		}
	}

	_ = os.WriteFile(filename, []byte(strings.Join(kept, "\n")), 0644)
}

func testNodesFromFile() {
	if len(os.Args) < 3 {
		fmt.Println("用法: go run . test-nodes <节点文件>")
		return
	}
	file, err := os.Open(os.Args[2])
	if err != nil {
		fmt.Println("打开文件失败:", err)
		return
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

	collector := NewCollector("", NewLogger(len(nodes)))
	var wg sync.WaitGroup
	var mu sync.Mutex
	results := make(map[int]*ValidNode)

	for i, nodeLink := range nodes {
		wg.Add(1)
		go func(index int, link string) {
			defer wg.Done()
			collector.logger.Progress(link)
			validNode := collector.TestNodeWithSingBox(link)
			mu.Lock()
			results[index] = validNode
			mu.Unlock()
		}(i, nodeLink)
	}
	wg.Wait()
	collector.logger.DoneProgress()

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

	timeout := 60 * time.Minute
	if timeoutEnv := os.Getenv("COLLECT_TIMEOUT"); timeoutEnv != "" {
		if d, err := time.ParseDuration(timeoutEnv); err == nil && d > 0 {
			timeout = d
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	done := make(chan bool, 1)
	keywords, err := loadKeywords("keywords.txt")
	if err != nil || len(keywords) == 0 {
		fmt.Println("\033[31m[❌] 无法加载 keywords.txt 或文件为空，程序终止\033[0m")
		os.Exit(1)
	}

	log := NewLogger(len(keywords) * 50)
	collector := NewCollector(os.Getenv("GITHUB_TOKEN"), log)

	_ = os.Remove("nodes.txt")
	_ = os.Remove("links.txt")
	log.Success(fmt.Sprintf("成功加载 %d 个关键词，已清理残留文件", len(keywords)))

	var bgWg sync.WaitGroup
	var failedMu sync.Mutex
	var failedKeywords []string

	for i, keyword := range keywords {
		links, err := collector.SearchKeywordLinks(keyword)
		if err != nil || len(links) == 0 {
			failedMu.Lock()
			failedKeywords = append(failedKeywords, keyword)
			failedMu.Unlock()
			log.Warn(fmt.Sprintf("[%s] 无搜索结果", keyword))
			if i < len(keywords)-1 {
				time.Sleep(SearchInterval)
			}
			continue
		}

		bgWg.Add(1)
		go func(kw string, kLinks []string) {
			defer bgWg.Done()
			if !collector.ProcessKeywordLinks(kw, kLinks) {
				failedMu.Lock()
				failedKeywords = append(failedKeywords, kw)
				failedMu.Unlock()
			}
		}(keyword, links)

		if i < len(keywords)-1 {
			time.Sleep(SearchInterval)
		}
	}
	bgWg.Wait()

	if len(failedKeywords) > 0 {
		removeKeywords("keywords.txt", failedKeywords)
		log.Warn(fmt.Sprintf("已移除 %d 个失效关键词", len(failedKeywords)))
	}

	finalizeResults(log, os.Getenv("GITHUB_TOKEN"))
	log.PrintSummary(len(validLinks), len(validNodes))
}
