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
	RetryDelay       = 5 * time.Second
	NodesOutputFile  = "nodes.txt"
)

var (
	// 通用的 URL 提取正则
	urlRegex = regexp.MustCompile(`https?://[^\s"'<>]+`)
)

// GitHubSearchResult GitHub 搜索结果
type GitHubSearchResult struct {
	TotalCount int `json:"total_count"`
	Items      []struct {
		HTMLURL string `json:"html_url"`
		APIURL  string `json:"url"`
	} `json:"items"`
}

type GitHubFileContent struct {
	Content  string `json:"content"`
	Encoding string `json:"encoding"`
}

// NodeResult 节点结果
type ValidNode struct {
	Link    string
	Type    string
	Latency time.Duration
	Error   error
}

// 模拟外部 Node 解析结构体 (保持与你的外部依赖兼容)
type ParsedNode struct {
	Type   string
	Server string
	Port   int
}

// Collector 采集器
type Collector struct {
	githubToken string
	httpClient  *http.Client
}

func NewCollector(githubToken string) *Collector {
	return &Collector{
		githubToken: githubToken,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 20,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

// ==================== 阶段 1: 通用链接搜索 ====================

// SearchLinksByKeyword 通用关键词搜索，替代了之前所有冗余的特定网站搜索函数
func (c *Collector) SearchLinksByKeyword(keyword string) ([]string, error) {
	var allLinks []string
	seenLinks := make(map[string]bool)

	log.Printf("🔍 正在 GitHub 搜索特征关键词: %s", keyword)

	// 如果特征是 token，只搜索最近 3 个月更新的代码，保证节点时效性
	searchQuery := url.QueryEscape(keyword)
	if strings.Contains(keyword, "token=") {
		threeMonthsAgo := time.Now().AddDate(0, -3, 0).Format("2006-01-02")
		searchQuery += fmt.Sprintf("+pushed:>%s", threeMonthsAgo)
	}

	searchURL := fmt.Sprintf("%s/search/code?q=%s&per_page=100", GitHubAPIBaseURL, searchQuery)

	var results GitHubSearchResult
	if err := c.makeRequest(searchURL, &results); err != nil {
		return nil, fmt.Errorf("搜索失败: %v", err)
	}

	log.Printf("✅ 关键词 [%s] 找到 %d 个仓库文件结果", keyword, results.TotalCount)

	for _, item := range results.Items {
		fileContent, err := c.getFileContent(item.APIURL)
		if err != nil {
			continue
		}

		// 使用正则提取文件中所有的 URL
		matches := urlRegex.FindAllString(fileContent, -1)
		for _, match := range matches {
			link := strings.TrimSpace(match)
			link = strings.TrimRight(link, ".,;!?)")

			// 关键：URL 必须包含我们的关键词，才认为是有效订阅链接
			if strings.Contains(link, keyword) && !seenLinks[link] {
				seenLinks[link] = true
				allLinks = append(allLinks, link)
			}
		}
	}

	return allLinks, nil
}

// ==================== 阶段 2: 订阅内容抓取与解析 ====================

// FetchSubscription 获取订阅内容
func (c *Collector) FetchSubscription(link string) (string, error) {
	req, err := http.NewRequest("GET", link, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// ParseNodes 解析节点（支持 Base64、Clash YAML、sing-box JSON、纯文本）
func (c *Collector) ParseNodes(content string) []string {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}

	// 1. 尝试解析 Clash YAML
	if nodes := c.parseClashYAML(content); len(nodes) > 0 {
		return nodes
	}

	// 2. 尝试解析 sing-box JSON
	if nodes := c.parseSingBoxJSON(content); len(nodes) > 0 {
		return nodes
	}

	// 3. 尝试 base64 解码
	decodedContent := content
	if decodedBytes, err := base64.StdEncoding.DecodeString(content); err == nil {
		decodedContent = string(decodedBytes)
	} else if decodedBytes, err := base64.URLEncoding.DecodeString(content); err == nil {
		decodedContent = string(decodedBytes)
	}

	// 4. 从解码后的内容提取节点链接
	return c.extractNodeLinks(decodedContent)
}

// extractNodeLinks 从文本中提取标准节点协议链接
func (c *Collector) extractNodeLinks(content string) []string {
	var nodes []string
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if isNodeLink(line) {
			nodes = append(nodes, line)
		}
	}
	return nodes
}

func isNodeLink(line string) bool {
	prefixes := []string{"ss://", "vmess://", "vless://", "trojan://", "ssr://", "hysteria://", "hy2://", "wireguard://", "wg://", "tuic://", "http://", "socks5://"}
	for _, p := range prefixes {
		if strings.HasPrefix(line, p) {
			return true
		}
	}
	return false
}

// ==================== 阶段 3: 并发测速机制 ====================

// TestNode 测试单个节点联通性 (TCP Ping)
func (c *Collector) TestNode(nodeLink string) *ValidNode {
	result := &ValidNode{Link: nodeLink, Type: "unknown"}

	// 此处调用你外部的 ParseNodeLink 函数
	// node, err := ParseNodeLink(nodeLink)
	// 为保持示例独立运行，这里做简单 Mock:
	server, port, nType := mockParseNodeLink(nodeLink)
	result.Type = nType

	if server == "" || port == 0 {
		result.Error = fmt.Errorf("解析失败或缺少地址")
		return result
	}

	timeout := 3 * time.Second
	if timeoutEnv := os.Getenv("TEST_TIMEOUT"); timeoutEnv != "" {
		if n, err := strconv.Atoi(timeoutEnv); err == nil && n > 0 {
			timeout = time.Duration(n) * time.Second
		}
	}

	start := time.Now()
	address := fmt.Sprintf("%s:%d", server, port)
	tcpConn, err := net.DialTimeout("tcp", address, timeout)
	if err == nil {
		tcpConn.Close()
		result.Latency = time.Since(start)
	} else {
		result.Error = fmt.Errorf("连接超时或失败")
	}

	return result
}

// ==================== 核心执行逻辑 ====================

func main() {
	// 加载配置
	timeout := 60 * time.Minute
	if timeoutEnv := os.Getenv("COLLECT_TIMEOUT"); timeoutEnv != "" {
		if d, err := time.ParseDuration(timeoutEnv); err == nil && d > 0 {
			timeout = d
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	githubToken := os.Getenv("GITHUB_TOKEN")
	collector := NewCollector(githubToken)

	// 1. 读取关键词
	keywords, err := loadKeywords("keywords.txt")
	if err != nil {
		log.Fatalf("❌ 无法读取 keywords.txt 文件: %v", err)
	}
	log.Printf("🚀 成功加载 %d 个订阅特征关键词，开始全网搜索...", len(keywords))

	// 2. 收集所有订阅链接 (去重)
	uniqueSubLinks := make(map[string]bool)
	var subLinks []string

	for _, kw := range keywords {
		links, err := collector.SearchLinksByKeyword(kw)
		if err != nil {
			log.Printf("⚠️ 搜索 [%s] 失败: %v", kw, err)
			continue
		}
		for _, link := range links {
			if !uniqueSubLinks[link] {
				uniqueSubLinks[link] = true
				subLinks = append(subLinks, link)
			}
		}
		time.Sleep(2 * time.Second) // 避免触发 GitHub 速率限制
	}
	log.Printf("✅ 爬取阶段完成，共获得 %d 个独立订阅链接。", len(subLinks))

	// 3. 抓取订阅配置，提取原始节点 (去重)
	uniqueNodes := make(map[string]bool)
	var rawNodes []string
	var nodeMu sync.Mutex
	var fetchWg sync.WaitGroup

	// 限制并发下载订阅
	fetchSemaphore := make(chan struct{}, 15)

	log.Println("🌐 开始下载订阅链接内容提取节点...")
	for _, link := range subLinks {
		fetchWg.Add(1)
		go func(l string) {
			defer fetchWg.Done()
			fetchSemaphore <- struct{}{}
			defer func() { <-fetchSemaphore }()

			content, err := collector.FetchSubscription(l)
			if err != nil {
				return
			}
			nodes := collector.ParseNodes(content)

			nodeMu.Lock()
			for _, n := range nodes {
				if !uniqueNodes[n] {
					uniqueNodes[n] = true
					rawNodes = append(rawNodes, n)
				}
			}
			nodeMu.Unlock()
		}(link)
	}
	fetchWg.Wait()
	log.Printf("✅ 节点提取完成，共获得 %d 个独立节点(去重后)。", len(rawNodes))

	// 4. 并发测速验证节点 (Worker Pool 模式)
	log.Println("⚡ 开始对节点进行可用性测速...")
	validNodesChan := make(chan *ValidNode, len(rawNodes))
	var testWg sync.WaitGroup

	maxConcurrency := 50 // 提高测速并发量
	if envC := os.Getenv("MAX_CONCURRENCY"); envC != "" {
		if c, err := strconv.Atoi(envC); err == nil && c > 0 {
			maxConcurrency = c
		}
	}
	testSemaphore := make(chan struct{}, maxConcurrency)

	for _, nodeStr := range rawNodes {
		testWg.Add(1)
		go func(n string) {
			defer testWg.Done()
			testSemaphore <- struct{}{}
			defer func() { <-testSemaphore }()

			// 检查上下文是否已超时
			select {
			case <-ctx.Done():
				return
			default:
			}

			// 决定使用什么测速方式
			var res *ValidNode
			if os.Getenv("USE_SINGBOX") == "true" {
				// 此处需替换为你实现的 collector.TestNodeWithSingBox(n)
				res = collector.TestNode(n)
			} else {
				res = collector.TestNode(n)
			}

			if res.Error == nil {
				validNodesChan <- res
			}
		}(nodeStr)
	}

	// 等待测速完成并关闭通道
	go func() {
		testWg.Wait()
		close(validNodesChan)
	}()

	// 5. 收集有效节点并统计
	var finalValidNodes []string
	typeStats := make(map[string]int)

	for n := range validNodesChan {
		finalValidNodes = append(finalValidNodes, n.Link)
		typeStats[n.Type]++
	}

	log.Printf("🎉 测速结束！最终得到 %d 个可用节点。", len(finalValidNodes))
	for t, count := range typeStats {
		log.Printf("   - [%s] : %d 个", t, count)
	}

	// 6. 保存为 Base64 文件 (提供给 V2rayN/Clash 等客户端)
	if len(finalValidNodes) > 0 {
		plainContent := strings.Join(finalValidNodes, "\n")
		encodedContent := base64.StdEncoding.EncodeToString([]byte(plainContent))
		err = os.WriteFile(NodesOutputFile, []byte(encodedContent), 0644)
		if err != nil {
			log.Fatalf("❌ 保存节点文件失败: %v", err)
		}
		log.Printf("💾 有效节点已保存并 Base64 编码至 %s", NodesOutputFile)
	} else {
		log.Println("⚠️ 没有找到任何可用节点，跳过保存。")
	}
}

// ==================== 辅助工具函数 ====================

func loadKeywords(filename string) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var keywords []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			keywords = append(keywords, line)
		}
	}
	return keywords, scanner.Err()
}

// GitHub API 请求封装
func (c *Collector) makeRequest(url string, result interface{}) error {
	req, _ := http.NewRequest("GET", url, nil)
	if c.githubToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.githubToken)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	var resp *http.Response
	var err error
	for i := 0; i < MaxRetries; i++ {
		resp, err = c.httpClient.Do(req)
		if err == nil && resp.StatusCode == 200 {
			break
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(RetryDelay)
	}
	if err != nil || resp.StatusCode != 200 {
		return fmt.Errorf("请求失败")
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(result)
}

// 获取 GitHub 文件具体内容并解 Base64
func (c *Collector) getFileContent(apiURL string) (string, error) {
	var fileContent GitHubFileContent
	if err := c.makeRequest(apiURL, &fileContent); err != nil {
		return "", err
	}
	if fileContent.Encoding == "base64" {
		decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(fileContent.Content, "\n", ""))
		if err != nil {
			return "", err
		}
		return string(decoded), nil
	}
	return fileContent.Content, nil
}

// 以下是你原始文件中的解析函数占位实现 (精简代码展示，原文件中的 convertClashSS 等函数需保留)
func (c *Collector) parseClashYAML(content string) []string {
	var config struct {
		Proxies []map[string]interface{} `yaml:"proxies"`
	}
	if err := yaml.Unmarshal([]byte(content), &config); err != nil {
		return nil
	}
	// ... 此处保留你原有的 convertClashProxyToLink 逻辑 ...
	return nil
}

func (c *Collector) parseSingBoxJSON(content string) []string {
	// ... 此处保留你原有的 convertSingBoxOutboundToLink 逻辑 ...
	return nil
}

// mockParseNodeLink 用于代替原代码中未提供的解析逻辑。实际使用时请替换回你的 ParseNodeLink
func mockParseNodeLink(link string) (string, int, string) {
	u, err := url.Parse(link)
	if err != nil {
		return "", 0, "unknown"
	}
	port, _ := strconv.Atoi(u.Port())
	return u.Hostname(), port, u.Scheme
}
