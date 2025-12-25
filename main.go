package main

import (
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
)

const (
	GitHubAPIBaseURL = "https://api.github.com"
	MaxRetries       = 3
	RetryDelay       = 5 * time.Second
)

var (
	// 匹配目标链接的正则表达式
	linkPattern = regexp.MustCompile(`https?://(?:jmssub\.net|jjsubmarines\.com)/members/getsub\.php\?[^\s"']+`)
)

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

// GitHubFileContent GitHub 文件内容
type GitHubFileContent struct {
	Content  string `json:"content"`
	Encoding string `json:"encoding"`
}

// NodeResult 节点结果
type NodeResult struct {
	Link       string
	Nodes      []string
	ValidNodes []*ValidNode
	Error      error
}

// ValidNode 可用节点
type ValidNode struct {
	Link    string
	Type    string
	Latency time.Duration
	Error   error
}

// Collector 采集器
type Collector struct {
	githubToken string
	httpClient  *http.Client
	seenLinks   map[string]bool
	mu          sync.Mutex
}

func NewCollector(githubToken string) *Collector {
	return &Collector{
		githubToken: githubToken,
		httpClient: &http.Client{
			Timeout: 60 * time.Second, // 增加超时时间
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:    90 * time.Second,
			},
		},
		seenLinks: make(map[string]bool),
	}
}

// SearchGitHub 搜索 GitHub
func (c *Collector) SearchGitHub(keywords []string) ([]string, error) {
	var allLinks []string
	seenLinks := make(map[string]bool)

	for _, keyword := range keywords {
		log.Printf("正在搜索关键词: %s", keyword)

		// GitHub API 搜索代码
		searchURL := fmt.Sprintf("%s/search/code?q=%s&per_page=100", GitHubAPIBaseURL, url.QueryEscape(keyword))

		var results GitHubSearchResult
		if err := c.makeRequest(searchURL, &results); err != nil {
			log.Printf("搜索关键词 %s 失败: %v", keyword, err)
			continue
		}

		log.Printf("找到 %d 个结果", results.TotalCount)

		// 处理每个结果
		for _, item := range results.Items {
			// 获取文件内容
			fileContent, err := c.getFileContent(item.APIURL)
			if err != nil {
				log.Printf("获取文件内容失败 %s: %v", item.HTMLURL, err)
				continue
			}

			// 提取链接
			links := c.extractLinks(fileContent)
			for _, link := range links {
				if !seenLinks[link] {
					seenLinks[link] = true
					allLinks = append(allLinks, link)
					log.Printf("发现新链接: %s", link)
				}
			}
		}

		// 避免速率限制
		time.Sleep(2 * time.Second)
	}

	return allLinks, nil
}

// getFileContent 获取文件内容
func (c *Collector) getFileContent(apiURL string) (string, error) {
	var fileContent GitHubFileContent
	if err := c.makeRequest(apiURL, &fileContent); err != nil {
		return "", err
	}

	// 如果是 base64 编码，解码
	if fileContent.Encoding == "base64" {
		decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(fileContent.Content, "\n", ""))
		if err != nil {
			return "", fmt.Errorf("base64 解码失败: %v", err)
		}
		return string(decoded), nil
	}

	return fileContent.Content, nil
}

// extractLinks 提取链接
func (c *Collector) extractLinks(content string) []string {
	matches := linkPattern.FindAllString(content, -1)
	var links []string
	for _, match := range matches {
		// 清理链接
		link := strings.TrimSpace(match)
		link = strings.TrimRight(link, ".,;!?)")
		if link != "" {
			links = append(links, link)
		}
	}
	return links
}

// extractNodesFromRawContent 从原始内容中提取节点链接（不进行 base64 解码）
func (c *Collector) extractNodesFromRawContent(content string) []string {
	var nodes []string
	seenNodes := make(map[string]bool)

	// 支持多种分隔符
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// 直接检查是否是节点链接
		if strings.HasPrefix(line, "ss://") ||
			strings.HasPrefix(line, "vmess://") ||
			strings.HasPrefix(line, "vless://") ||
			strings.HasPrefix(line, "trojan://") ||
			strings.HasPrefix(line, "ssr://") {
			if !seenNodes[line] {
				seenNodes[line] = true
				nodes = append(nodes, line)
			}
		}
	}

	return nodes
}

// makeRequest 发送 HTTP 请求
func (c *Collector) makeRequest(url string, result interface{}) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	// 添加认证头（如果提供了 token）
	if c.githubToken != "" {
		// GitHub 推荐使用 Bearer 前缀，但 token 前缀也支持
		req.Header.Set("Authorization", "Bearer "+c.githubToken)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	var resp *http.Response
	for i := 0; i < MaxRetries; i++ {
		resp, err = c.httpClient.Do(req)
		if err == nil && resp.StatusCode == 200 {
			break
		}

		if resp != nil {
			resp.Body.Close()
		}

		if i < MaxRetries-1 {
			log.Printf("请求失败，%v 后重试... (尝试 %d/%d)", RetryDelay, i+1, MaxRetries)
			time.Sleep(RetryDelay)
		}
	}

	if err != nil {
		return fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode == 401 {
			return fmt.Errorf("HTTP 401: 需要 GitHub Token 进行认证。请设置 GITHUB_TOKEN 环境变量")
		}
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	return json.NewDecoder(resp.Body).Decode(result)
}

// FetchSubscription 获取订阅内容
func (c *Collector) FetchSubscription(link string) (string, error) {
	resp, err := c.httpClient.Get(link)
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

// ParseNodes 解析节点
func (c *Collector) ParseNodes(content string) ([]string, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, fmt.Errorf("内容为空")
	}

	var decoded string
	var err error

	// 尝试多种解码方式
	// 1. 先尝试 base64 解码
	decodedBytes, err := base64.StdEncoding.DecodeString(content)
	if err != nil {
		// 2. 如果 base64 失败，尝试 URL-safe base64
		decodedBytes, err = base64.URLEncoding.DecodeString(content)
		if err != nil {
			// 3. 如果还是失败，尝试使用 safeBase64Decode（处理 padding 问题）
			decoded, err = safeBase64Decode(content)
			if err != nil {
				// 4. 如果都失败，可能是纯文本格式，直接使用原始内容
				decoded = content
			} else {
				// safeBase64Decode 已经返回 string
			}
		} else {
			decoded = string(decodedBytes)
		}
	} else {
		decoded = string(decodedBytes)
	}

	// 按行分割（支持 \n 和 \r\n）
	lines := strings.Split(strings.ReplaceAll(decoded, "\r\n", "\n"), "\n")
	var nodes []string
	seenNodes := make(map[string]bool) // 去重

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// 检查是否是节点链接
		if strings.HasPrefix(line, "ss://") ||
			strings.HasPrefix(line, "vmess://") ||
			strings.HasPrefix(line, "vless://") ||
			strings.HasPrefix(line, "trojan://") ||
			strings.HasPrefix(line, "ssr://") {
			// 去重
			if !seenNodes[line] {
				seenNodes[line] = true
				nodes = append(nodes, line)
			}
		}
	}

	if len(nodes) == 0 {
		return nil, fmt.Errorf("未找到有效节点")
	}

	return nodes, nil
}

// TestNode 测试节点
func (c *Collector) TestNode(nodeLink string) *ValidNode {
	result := &ValidNode{
		Link: nodeLink,
	}

	// 解析节点
	node, err := ParseNodeLink(nodeLink)
	if err != nil {
		result.Error = fmt.Errorf("解析失败: %v", err)
		return result
	}

	result.Type = node.Type

	// TCP 连接测试
	timeout := 5 * time.Second
	if timeoutEnv := os.Getenv("TEST_TIMEOUT"); timeoutEnv != "" {
		if n, err := strconv.Atoi(timeoutEnv); err == nil && n > 0 {
			timeout = time.Duration(n) * time.Second
		}
	}

	start := time.Now()
	address := fmt.Sprintf("%s:%d", node.Server, node.Port)
	tcpConn, err := net.DialTimeout("tcp", address, timeout)
	if err == nil {
		tcpConn.Close()
		result.Latency = time.Since(start)
	} else {
		result.Error = fmt.Errorf("连接失败: %v", err)
	}

	return result
}

// Collect 执行采集
func (c *Collector) Collect() error {
	// 搜索 GitHub（增加更多关键词以获取更多节点）
	keywords := []string{
		"jmssub.net",
		"jjsubmarines.com",
		"getsub.php",
		"jmssub.net/members",
		"jjsubmarines.com/members",
	}
	links, err := c.SearchGitHub(keywords)
	if err != nil {
		return fmt.Errorf("搜索失败: %v", err)
	}

	log.Printf("共找到 %d 个链接", len(links))

	// 采集节点
	var allValidNodes []*ValidNode
	var wg sync.WaitGroup
	resultsChan := make(chan *NodeResult, len(links))

	// 并发采集
	maxConcurrency := 10
	if maxConcurrencyEnv := os.Getenv("MAX_CONCURRENCY"); maxConcurrencyEnv != "" {
		if n, err := strconv.Atoi(maxConcurrencyEnv); err == nil && n > 0 {
			maxConcurrency = n
		}
	}
	semaphore := make(chan struct{}, maxConcurrency) // 限制并发数

	for _, link := range links {
		wg.Add(1)
		go func(l string) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			result := &NodeResult{Link: l}

			// 获取订阅内容
			content, err := c.FetchSubscription(l)
			if err != nil {
				result.Error = err
				resultsChan <- result
				return
			}

			// 解析节点（即使失败也继续，尝试从原始内容提取）
			nodes, err := c.ParseNodes(content)
			if err != nil {
				// 如果 base64 解码失败，尝试直接从原始内容提取节点链接
				log.Printf("链接 %s 解析失败: %v，尝试从原始内容提取", l, err)
				nodes = c.extractNodesFromRawContent(content)
				if len(nodes) == 0 {
					result.Error = err
					resultsChan <- result
					return
				}
				log.Printf("从原始内容提取到 %d 个节点", len(nodes))
			}

			result.Nodes = nodes
			log.Printf("链接 %s 解析出 %d 个节点", l, len(nodes))

			// 测试节点
			for _, nodeLink := range nodes {
				validNode := c.TestNode(nodeLink)
				if validNode.Error == nil {
					result.ValidNodes = append(result.ValidNodes, validNode)
					allValidNodes = append(allValidNodes, validNode)
				} else {
					// 记录测试失败的节点类型（用于统计）
					if validNode.Type != "" {
						result.ValidNodes = append(result.ValidNodes, validNode)
					}
				}
			}

			resultsChan <- result
		}(link)
	}

	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// 收集结果并统计
	typeStats := make(map[string]int)        // 所有解析出的节点类型统计
	validTypeStats := make(map[string]int)    // 测试通过的节点类型统计
	failedTypeStats := make(map[string]int)   // 测试失败的节点类型统计
	totalNodes := 0
	totalValidNodes := 0
	
	for result := range resultsChan {
		if result.Error != nil {
			log.Printf("链接 %s 处理失败: %v", result.Link, result.Error)
		} else {
			totalNodes += len(result.Nodes)
			totalValidNodes += len(result.ValidNodes)
			
			// 统计所有解析出的节点类型
			for _, nodeLink := range result.Nodes {
				if node, err := ParseNodeLink(nodeLink); err == nil {
					typeStats[node.Type]++
				}
			}
			
			// 统计测试结果
			for _, validNode := range result.ValidNodes {
				if validNode.Type != "" {
					if validNode.Error == nil {
						validTypeStats[validNode.Type]++
					} else {
						failedTypeStats[validNode.Type]++
					}
				}
			}
			
			log.Printf("链接 %s: 共 %d 个节点，%d 个可用",
				result.Link, len(result.Nodes), len(result.ValidNodes))
		}
	}

	log.Printf("采集完成，共解析 %d 个节点，%d 个可用节点", totalNodes, len(allValidNodes))
	if len(typeStats) > 0 {
		log.Printf("解析出的节点类型统计:")
		for nodeType, count := range typeStats {
			validCount := validTypeStats[nodeType]
			failedCount := failedTypeStats[nodeType]
			log.Printf("  %s: 共 %d 个 (可用: %d, 失败: %d)", nodeType, count, validCount, failedCount)
		}
	}

	// 保存结果
	return c.SaveResults(allValidNodes)
}

// SaveResults 保存结果
func (c *Collector) SaveResults(nodes []*ValidNode) error {
	// 创建输出文件
	outputFile := "nodes.txt"
	file, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("创建文件失败: %v", err)
	}
	defer file.Close()

	// 写入节点
	for _, node := range nodes {
		file.WriteString(node.Link + "\n")
	}

	log.Printf("结果已保存到 %s，共 %d 个节点", outputFile, len(nodes))

	// 如果配置了 GitHub，推送到 GitHub
	if repo := os.Getenv("GITHUB_REPO"); repo != "" {
		return c.PushToGitHub(outputFile, nodes)
	}

	return nil
}

// PushToGitHub 推送到 GitHub
func (c *Collector) PushToGitHub(filePath string, nodes []*ValidNode) error {
	if c.githubToken == "" {
		return fmt.Errorf("需要 GITHUB_TOKEN 才能推送到 GitHub")
	}

	repo := os.Getenv("GITHUB_REPO")
	if repo == "" {
		return fmt.Errorf("需要设置 GITHUB_REPO 环境变量（格式: owner/repo）")
	}

	// 读取文件内容
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("读取文件失败: %v", err)
	}

	// Base64 编码内容
	encodedContent := base64.StdEncoding.EncodeToString(content)

	// 构建 API URL
	apiURL := fmt.Sprintf("%s/repos/%s/contents/nodes.txt", GitHubAPIBaseURL, repo)

	// 检查文件是否存在
	var existingFile struct {
		SHA string `json:"sha"`
	}
	req, _ := http.NewRequest("GET", apiURL, nil)
	req.Header.Set("Authorization", "Bearer "+c.githubToken)
	resp, err := c.httpClient.Do(req)
	if err == nil && resp.StatusCode == 200 {
		json.NewDecoder(resp.Body).Decode(&existingFile)
		resp.Body.Close()
	}

	// 创建或更新文件
	payload := map[string]interface{}{
		"message": fmt.Sprintf("更新节点列表 (%d 个节点)", len(nodes)),
		"content": encodedContent,
		"branch":  "main",
	}

	if existingFile.SHA != "" {
		payload["sha"] = existingFile.SHA
	}

	jsonData, _ := json.Marshal(payload)
	req, _ = http.NewRequest("PUT", apiURL, strings.NewReader(string(jsonData)))
	req.Header.Set("Authorization", "Bearer "+c.githubToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err = c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("推送失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("推送失败 HTTP %d: %s", resp.StatusCode, string(body))
	}

	log.Printf("已成功推送到 GitHub: %s/nodes.txt", repo)
	return nil
}

func main() {
	// 从环境变量获取 GitHub token（可选）
	githubToken := os.Getenv("GITHUB_TOKEN")
	if githubToken == "" {
		log.Println("警告: 未设置 GITHUB_TOKEN，将使用未认证的 API（速率限制较低）")
	}

	collector := NewCollector(githubToken)
	if err := collector.Collect(); err != nil {
		log.Fatalf("采集失败: %v", err)
	}
}
