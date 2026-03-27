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
}

func NewCollector(githubToken string) *Collector {
	return &Collector{
		githubToken: githubToken,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

// --- 核心采集与测试逻辑 ---

// SearchKeywordLinks 基于关键词在 GitHub 搜索相关代码文件，并提取包含该关键词的订阅链接
func (c *Collector) SearchKeywordLinks(keyword string) ([]string, error) {
	var allLinks []string
	seenLinks := make(map[string]bool)

	log.Printf("正在 GitHub 搜索关键词: %s", keyword)
	searchURL := fmt.Sprintf("%s/search/code?q=%s&per_page=100", GitHubAPIBaseURL, url.QueryEscape(keyword))

	var results GitHubSearchResult
	if err := c.makeRequest(searchURL, &results); err != nil {
		return nil, fmt.Errorf("搜索失败: %v", err)
	}

	log.Printf("找到 %d 个代码文件结果", results.TotalCount)
	linkPattern := regexp.MustCompile(`https?://[^\s"'<>]+`)

	for _, item := range results.Items {
		fileContent, err := c.getFileContent(item.APIURL)
		if err != nil {
			continue // 忽略获取失败的文件
		}

		matches := linkPattern.FindAllString(fileContent, -1)
		for _, match := range matches {
			link := strings.TrimSpace(match)
			link = strings.TrimRight(link, ".,;!?)")
			link = strings.TrimRight(link, "\"')")

			// 精准过滤：只提取确实包含了搜索关键词的链接
			if link != "" && strings.Contains(link, keyword) && !seenLinks[link] {
				seenLinks[link] = true
				allLinks = append(allLinks, link)
				log.Printf("发现匹配链接: %s", link)
			}
		}
	}

	time.Sleep(2 * time.Second) // 避免触发 GitHub API 速率限制
	return allLinks, nil
}

// CollectNodesForKeyword 采集、解析、测速指定关键词下的所有节点
func (c *Collector) CollectNodesForKeyword(keyword string) error {
	links, err := c.SearchKeywordLinks(keyword)
	if err != nil {
		return err
	}

	if len(links) == 0 {
		log.Printf("关键词 [%s] 未提取到任何有效链接，跳过", keyword)
		return nil
	}

	log.Printf("关键词 [%s] 共提取到 %d 个链接，开始并发解析与测速", keyword, len(links))

	var allValidNodes []*ValidNode
	var wg sync.WaitGroup
	resultsChan := make(chan *NodeResult, len(links))
	var mu sync.Mutex

	// 并发控制：避免同时发起过多请求导致内存或文件描述符耗尽
	maxConcurrency := 15
	if maxConcurrencyEnv := os.Getenv("MAX_CONCURRENCY"); maxConcurrencyEnv != "" {
		if n, err := strconv.Atoi(maxConcurrencyEnv); err == nil && n > 0 {
			maxConcurrency = n
		}
	}
	semaphore := make(chan struct{}, maxConcurrency)

	for _, link := range links {
		wg.Add(1)
		go func(l string) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			result := &NodeResult{Link: l}
			content, err := c.FetchSubscription(l)
			if err != nil {
				result.Error = err
				resultsChan <- result
				return
			}

			// 解析节点内容
			nodes, err := c.ParseNodes(content)
			if err != nil {
				nodes = c.extractNodesFromRawContent(content)
			}

			if len(nodes) == 0 {
				result.Error = fmt.Errorf("没有解析出节点数据")
				resultsChan <- result
				return
			}
			result.Nodes = nodes

			// 测试解析出的每个节点
			for _, nodeLink := range nodes {
				var validNode *ValidNode
				if os.Getenv("USE_SINGBOX") == "false" {
					validNode = c.TestNode(nodeLink)
				} else {
					validNode = c.TestNodeWithSingBox(nodeLink)
				}

				if validNode.Error == nil {
					result.ValidNodes = append(result.ValidNodes, validNode)
					mu.Lock()
					allValidNodes = append(allValidNodes, validNode)
					mu.Unlock()
				}
			}
			resultsChan <- result
		}(link)
	}

	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	totalNodes := 0
	for result := range resultsChan {
		if result.Error == nil {
			totalNodes += len(result.Nodes)
		}
	}

	log.Printf("关键词 [%s] 测速完成，共解析 %d 个节点，其中可用节点: %d 个", keyword, totalNodes, len(allValidNodes))

	if len(allValidNodes) > 0 {
		return c.saveNodesToFile(allValidNodes, keyword)
	}
	return nil
}

// saveNodesToFile 边采边存，读取现有文件并追加去重后的新节点
func (c *Collector) saveNodesToFile(allValidNodes []*ValidNode, keyword string) error {
	existingContent := ""
	if content, err := os.ReadFile("nodes.txt"); err == nil {
		existingContent = string(content)
	}

	var allNodes []string
	seenNodes := make(map[string]bool)

	// 处理之前已有的节点数据（兼容 Base64 和纯文本）
	if existingContent != "" {
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(existingContent))
		if err == nil {
			existingContent = string(decoded)
		}
		for _, line := range strings.Split(strings.TrimSpace(existingContent), "\n") {
			line = strings.TrimSpace(line)
			if line != "" && !seenNodes[line] {
				seenNodes[line] = true
				allNodes = append(allNodes, line)
			}
		}
	}

	// 注入本次测试成功的可用节点
	for _, node := range allValidNodes {
		if node.Error == nil && !seenNodes[node.Link] {
			seenNodes[node.Link] = true
			allNodes = append(allNodes, node.Link)
		}
	}

	// 重新编码并保存
	encodedContent := base64.StdEncoding.EncodeToString([]byte(strings.Join(allNodes, "\n")))
	outputFile := "nodes.txt"
	if err := os.WriteFile(outputFile, []byte(encodedContent), 0644); err != nil {
		return fmt.Errorf("写入文件失败: %v", err)
	}

	log.Printf("关键词 [%s] 的节点已保存/追加到 %s，当前文件总去重节点数: %d", keyword, outputFile, len(allNodes))

	// 可选：向 Gist 推送最新数据
	gistID, gistToken := os.Getenv("GIST_ID"), os.Getenv("GIST_TOKEN")
	if gistToken == "" {
		gistToken = c.githubToken
	}
	if gistID != "" || gistToken != "" {
		_ = c.PushToGist(outputFile, allValidNodes)
	}

	return nil
}

// --- 基础工具类与解析逻辑保持不变，确保协议解析的完整性 ---

func (c *Collector) getFileContent(apiURL string) (string, error) {
	var fileContent GitHubFileContent
	if err := c.makeRequest(apiURL, &fileContent); err != nil {
		return "", err
	}
	if fileContent.Encoding == "base64" {
		decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(fileContent.Content, "\n", ""))
		if err != nil {
			return "", fmt.Errorf("base64 解码失败: %v", err)
		}
		return string(decoded), nil
	}
	return fileContent.Content, nil
}

func (c *Collector) extractNodesFromRawContent(content string) []string {
	if nodes := c.parseClashYAML(content); len(nodes) > 0 {
		return nodes
	}
	if nodes := c.parseSingBoxJSON(content); len(nodes) > 0 {
		return nodes
	}
	return c.extractNodeLinks(content)
}

func convertClashProxyToLink(proxy map[string]interface{}) string {
	proxyType, _ := proxy["type"].(string)
	server, _ := proxy["server"].(string)
	if proxyType == "" || server == "" {
		return ""
	}

	var port interface{} = 443
	if p, ok := proxy["port"]; ok {
		port = p
	}

	portStr := ""
	switch v := port.(type) {
	case int:
		portStr = strconv.Itoa(v)
	case float64:
		portStr = strconv.Itoa(int(v))
	case string:
		portStr = v
	}

	name, _ := proxy["name"].(string)

	switch proxyType {
	case "ss":
		return convertClashSS(proxy, server, portStr, name)
	case "vmess":
		return convertClashVMess(proxy, server, portStr, name)
	case "vless":
		return convertClashVLESS(proxy, server, portStr, name)
	case "trojan":
		return convertClashTrojan(proxy, server, portStr, name)
	case "ssr":
		return convertClashSSR(proxy, server, portStr, name)
	case "hysteria", "hysteria2":
		return convertClashHysteria(proxy, server, portStr, name)
	case "wireguard":
		return convertClashWireGuard(proxy, server, portStr, name)
	case "tuic":
		return convertClashTUIC(proxy, server, portStr, name)
	case "http":
		return convertClashHTTP(proxy, server, portStr, name)
	case "socks5":
		return convertClashSOCKS(proxy, server, portStr, name)
	}
	return ""
}

func convertSingBoxOutboundToLink(outbound map[string]interface{}) string {
	outboundType, _ := outbound["type"].(string)
	server, _ := outbound["server"].(string)
	if outboundType == "" || server == "" {
		return ""
	}

	var port interface{} = 443
	if p, ok := outbound["server_port"]; ok {
		port = p
	}

	portStr := ""
	switch v := port.(type) {
	case int:
		portStr = strconv.Itoa(v)
	case float64:
		portStr = strconv.Itoa(int(v))
	case string:
		portStr = v
	}

	tag, _ := outbound["tag"].(string)

	switch outboundType {
	case "shadowsocks":
		return convertSingBoxSS(outbound, server, portStr, tag)
	case "vmess":
		return convertSingBoxVMess(outbound, server, portStr, tag)
	case "vless":
		return convertSingBoxVLESS(outbound, server, portStr, tag)
	case "trojan":
		return convertSingBoxTrojan(outbound, server, portStr, tag)
	case "hysteria", "hysteria2":
		return convertSingBoxHysteria(outbound, server, portStr, tag)
	case "wireguard":
		return convertSingBoxWireGuard(outbound, server, portStr, tag)
	case "tuic":
		return convertSingBoxTUIC(outbound, server, portStr, tag)
	case "http":
		return convertSingBoxHTTP(outbound, server, portStr, tag)
	case "socks":
		return convertSingBoxSOCKS(outbound, server, portStr, tag)
	}
	return ""
}

// Clash 格式转换
func convertClashSS(proxy map[string]interface{}, server, port, name string) string {
	cipher, _ := proxy["cipher"].(string)
	password, _ := proxy["password"].(string)
	if cipher == "" || password == "" {
		return ""
	}
	authBase64 := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", cipher, password)))
	return fmt.Sprintf("ss://%s@%s:%s#%s", authBase64, server, port, url.QueryEscape(name))
}
func convertClashVMess(proxy map[string]interface{}, server, port, name string) string {
	uuid, _ := proxy["uuid"].(string)
	if uuid == "" {
		return ""
	}
	vmessData := map[string]interface{}{
		"v": "2", "ps": name, "add": server, "port": port, "id": uuid, "aid": 0, "net": "tcp", "type": "none", "tls": "",
	}
	if tls, ok := proxy["tls"].(bool); ok && tls {
		vmessData["tls"] = "tls"
		if sni, ok := proxy["sni"].(string); ok && sni != "" {
			vmessData["sni"] = sni
		}
	}
	if network, ok := proxy["network"].(string); ok && network != "" {
		vmessData["net"] = network
	}
	jsonData, _ := json.Marshal(vmessData)
	return "vmess://" + base64.StdEncoding.EncodeToString(jsonData)
}
func convertClashVLESS(proxy map[string]interface{}, server, port, name string) string {
	uuid, _ := proxy["uuid"].(string)
	if uuid == "" {
		return ""
	}
	u := url.URL{Scheme: "vless", User: url.User(uuid), Host: fmt.Sprintf("%s:%s", server, port), Fragment: name}
	q := url.Values{}
	if tls, ok := proxy["tls"].(bool); ok && tls {
		q.Set("security", "tls")
		if sni, ok := proxy["sni"].(string); ok && sni != "" {
			q.Set("sni", sni)
		}
	}
	if network, ok := proxy["network"].(string); ok && network != "" {
		q.Set("type", network)
	}
	u.RawQuery = q.Encode()
	return u.String()
}
func convertClashTrojan(proxy map[string]interface{}, server, port, name string) string {
	password, _ := proxy["password"].(string)
	if password == "" {
		return ""
	}
	u := url.URL{Scheme: "trojan", User: url.User(password), Host: fmt.Sprintf("%s:%s", server, port), Fragment: name}
	q := url.Values{}
	if sni, ok := proxy["sni"].(string); ok && sni != "" {
		q.Set("sni", sni)
	}
	u.RawQuery = q.Encode()
	return u.String()
}
func convertClashSSR(proxy map[string]interface{}, server, port, name string) string {
	cipher, _ := proxy["cipher"].(string)
	password, _ := proxy["password"].(string)
	protocol, _ := proxy["protocol"].(string)
	obfs, _ := proxy["obfs"].(string)
	if cipher == "" || password == "" {
		return ""
	}
	passwordBase64 := base64.StdEncoding.EncodeToString([]byte(password))
	ssrStr := fmt.Sprintf("%s:%s:%s:%s:%s:%s", server, port, protocol, cipher, obfs, passwordBase64)
	return "ssr://" + base64.URLEncoding.EncodeToString([]byte(ssrStr))
}
func convertClashHysteria(proxy map[string]interface{}, server, port, name string) string {
	u := url.URL{Scheme: "hysteria", Host: fmt.Sprintf("%s:%s", server, port), Fragment: name}
	q := url.Values{}
	if auth, ok := proxy["auth"].(string); ok && auth != "" {
		q.Set("auth", auth)
	}
	if obfs, ok := proxy["obfs"].(string); ok && obfs != "" {
		q.Set("obfs", obfs)
	}
	u.RawQuery = q.Encode()
	return u.String()
}
func convertClashWireGuard(proxy map[string]interface{}, server, port, name string) string {
	privateKey, _ := proxy["private-key"].(string)
	if privateKey == "" {
		return ""
	}
	u := url.URL{Scheme: "wireguard", User: url.User(privateKey), Host: fmt.Sprintf("%s:%s", server, port), Fragment: name}
	q := url.Values{}
	if publicKey, ok := proxy["public-key"].(string); ok && publicKey != "" {
		q.Set("publickey", publicKey)
	}
	u.RawQuery = q.Encode()
	return u.String()
}
func convertClashTUIC(proxy map[string]interface{}, server, port, name string) string {
	uuid, _ := proxy["uuid"].(string)
	if uuid == "" {
		return ""
	}
	u := url.URL{Scheme: "tuic", User: url.User(uuid), Host: fmt.Sprintf("%s:%s", server, port), Fragment: name}
	q := url.Values{}
	if token, ok := proxy["token"].(string); ok && token != "" {
		q.Set("token", token)
	}
	u.RawQuery = q.Encode()
	return u.String()
}
func convertClashHTTP(proxy map[string]interface{}, server, port, name string) string {
	u := url.URL{Scheme: "http", Host: fmt.Sprintf("%s:%s", server, port), Fragment: name}
	if username, ok := proxy["username"].(string); ok && username != "" {
		if password, ok := proxy["password"].(string); ok && password != "" {
			u.User = url.UserPassword(username, password)
		} else {
			u.User = url.User(username)
		}
	}
	return u.String()
}
func convertClashSOCKS(proxy map[string]interface{}, server, port, name string) string {
	u := url.URL{Scheme: "socks5", Host: fmt.Sprintf("%s:%s", server, port), Fragment: name}
	if username, ok := proxy["username"].(string); ok && username != "" {
		if password, ok := proxy["password"].(string); ok && password != "" {
			u.User = url.UserPassword(username, password)
		} else {
			u.User = url.User(username)
		}
	}
	return u.String()
}

// sing-box 格式转换
func convertSingBoxSS(outbound map[string]interface{}, server, port, tag string) string {
	method, _ := outbound["method"].(string)
	password, _ := outbound["password"].(string)
	if method == "" || password == "" {
		return ""
	}
	authBase64 := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", method, password)))
	return fmt.Sprintf("ss://%s@%s:%s#%s", authBase64, server, port, url.QueryEscape(tag))
}
func convertSingBoxVMess(outbound map[string]interface{}, server, port, tag string) string {
	uuid, _ := outbound["uuid"].(string)
	if uuid == "" {
		return ""
	}
	vmessData := map[string]interface{}{
		"v": "2", "ps": tag, "add": server, "port": port, "id": uuid, "aid": 0, "net": "tcp", "type": "none", "tls": "",
	}
	if tls, ok := outbound["tls"].(map[string]interface{}); ok && tls != nil {
		vmessData["tls"] = "tls"
		if sni, ok := tls["server_name"].(string); ok && sni != "" {
			vmessData["sni"] = sni
		}
	}
	if transport, ok := outbound["transport"].(map[string]interface{}); ok {
		if t, ok := transport["type"].(string); ok && t != "" {
			vmessData["net"] = t
		}
	}
	jsonData, _ := json.Marshal(vmessData)
	return "vmess://" + base64.StdEncoding.EncodeToString(jsonData)
}
func convertSingBoxVLESS(outbound map[string]interface{}, server, port, tag string) string {
	uuid, _ := outbound["uuid"].(string)
	if uuid == "" {
		return ""
	}
	u := url.URL{Scheme: "vless", User: url.User(uuid), Host: fmt.Sprintf("%s:%s", server, port), Fragment: tag}
	q := url.Values{}
	if tls, ok := outbound["tls"].(map[string]interface{}); ok && tls != nil {
		q.Set("security", "tls")
		if sni, ok := tls["server_name"].(string); ok && sni != "" {
			q.Set("sni", sni)
		}
	}
	if transport, ok := outbound["transport"].(map[string]interface{}); ok {
		if t, ok := transport["type"].(string); ok && t != "" {
			q.Set("type", t)
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}
func convertSingBoxTrojan(outbound map[string]interface{}, server, port, tag string) string {
	password, _ := outbound["password"].(string)
	if password == "" {
		return ""
	}
	u := url.URL{Scheme: "trojan", User: url.User(password), Host: fmt.Sprintf("%s:%s", server, port), Fragment: tag}
	q := url.Values{}
	if tls, ok := outbound["tls"].(map[string]interface{}); ok && tls != nil {
		if sni, ok := tls["server_name"].(string); ok && sni != "" {
			q.Set("sni", sni)
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}
func convertSingBoxHysteria(outbound map[string]interface{}, server, port, tag string) string {
	u := url.URL{Scheme: "hysteria", Host: fmt.Sprintf("%s:%s", server, port), Fragment: tag}
	q := url.Values{}
	if auth, ok := outbound["auth_str"].(string); ok && auth != "" {
		q.Set("auth", auth)
	}
	u.RawQuery = q.Encode()
	return u.String()
}
func convertSingBoxWireGuard(outbound map[string]interface{}, server, port, tag string) string {
	privateKey, _ := outbound["private_key"].(string)
	if privateKey == "" {
		return ""
	}
	u := url.URL{Scheme: "wireguard", User: url.User(privateKey), Host: fmt.Sprintf("%s:%s", server, port), Fragment: tag}
	q := url.Values{}
	if peers, ok := outbound["peers"].([]interface{}); ok && len(peers) > 0 {
		if peer, ok := peers[0].(map[string]interface{}); ok {
			if publicKey, ok := peer["public_key"].(string); ok && publicKey != "" {
				q.Set("publickey", publicKey)
			}
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}
func convertSingBoxTUIC(outbound map[string]interface{}, server, port, tag string) string {
	uuid, _ := outbound["uuid"].(string)
	if uuid == "" {
		return ""
	}
	u := url.URL{Scheme: "tuic", User: url.User(uuid), Host: fmt.Sprintf("%s:%s", server, port), Fragment: tag}
	q := url.Values{}
	if token, ok := outbound["token"].(string); ok && token != "" {
		q.Set("token", token)
	}
	u.RawQuery = q.Encode()
	return u.String()
}
func convertSingBoxHTTP(outbound map[string]interface{}, server, port, tag string) string {
	u := url.URL{Scheme: "http", Host: fmt.Sprintf("%s:%s", server, port), Fragment: tag}
	if username, ok := outbound["username"].(string); ok && username != "" {
		if password, ok := outbound["password"].(string); ok && password != "" {
			u.User = url.UserPassword(username, password)
		} else {
			u.User = url.User(username)
		}
	}
	return u.String()
}
func convertSingBoxSOCKS(outbound map[string]interface{}, server, port, tag string) string {
	u := url.URL{Scheme: "socks5", Host: fmt.Sprintf("%s:%s", server, port), Fragment: tag}
	if username, ok := outbound["username"].(string); ok && username != "" {
		if password, ok := outbound["password"].(string); ok && password != "" {
			u.User = url.UserPassword(username, password)
		} else {
			u.User = url.User(username)
		}
	}
	return u.String()
}

func (c *Collector) makeRequest(url string, result interface{}) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	if c.githubToken != "" {
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
			time.Sleep(RetryDelay)
		}
	}

	if err != nil {
		return fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d 错误", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(result)
}

func (c *Collector) FetchSubscription(link string) (string, error) {
	req, err := http.NewRequest("GET", link, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
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

func (c *Collector) ParseNodes(content string) ([]string, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, fmt.Errorf("内容为空")
	}
	if nodes := c.parseClashYAML(content); len(nodes) > 0 {
		return nodes, nil
	}
	if nodes := c.parseSingBoxJSON(content); len(nodes) > 0 {
		return nodes, nil
	}

	var decoded string
	decodedBytes, err := base64.StdEncoding.DecodeString(content)
	if err != nil {
		decodedBytes, err = base64.URLEncoding.DecodeString(content)
		if err != nil {
			decoded, err = safeBase64Decode(content)
			if err != nil {
				decoded = content
			}
		} else {
			decoded = string(decodedBytes)
		}
	} else {
		decoded = string(decodedBytes)
	}

	return c.extractNodeLinks(decoded), nil
}

func (c *Collector) parseClashYAML(content string) []string {
	var config struct {
		Proxies []map[string]interface{} `yaml:"proxies"`
	}
	if err := yaml.Unmarshal([]byte(content), &config); err != nil {
		return nil
	}

	var nodes []string
	seenNodes := make(map[string]bool)
	for _, proxy := range config.Proxies {
		if nodeLink := convertClashProxyToLink(proxy); nodeLink != "" && !seenNodes[nodeLink] {
			seenNodes[nodeLink] = true
			nodes = append(nodes, nodeLink)
		}
	}
	return nodes
}

func (c *Collector) parseSingBoxJSON(content string) []string {
	var config struct {
		Outbounds []map[string]interface{} `json:"outbounds"`
	}
	if err := json.Unmarshal([]byte(content), &config); err != nil {
		return nil
	}

	var nodes []string
	seenNodes := make(map[string]bool)
	for _, outbound := range config.Outbounds {
		if nodeLink := convertSingBoxOutboundToLink(outbound); nodeLink != "" && !seenNodes[nodeLink] {
			seenNodes[nodeLink] = true
			nodes = append(nodes, nodeLink)
		}
	}
	return nodes
}

func (c *Collector) extractNodeLinks(content string) []string {
	var nodes []string
	seenNodes := make(map[string]bool)
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")

	prefixes := []string{
		"ss://", "vmess://", "vless://", "trojan://", "ssr://", "hysteria://", "hy2://",
		"wireguard://", "wg://", "tuic://", "http://", "https://", "socks://", "socks4://",
		"socks5://", "anytls://", "gost://", "gost+",
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		for _, p := range prefixes {
			if strings.HasPrefix(line, p) {
				if !seenNodes[line] {
					seenNodes[line] = true
					nodes = append(nodes, line)
				}
				break
			}
		}
	}
	return nodes
}

// TestNode 测试节点的连通性和延迟
func (c *Collector) TestNode(nodeLink string) *ValidNode {
	result := &ValidNode{Link: nodeLink}
	node, err := ParseNodeLink(nodeLink)
	if err != nil {
		result.Error = fmt.Errorf("解析失败: %v", err)
		return result
	}
	result.Type = node.Type

	timeout := 3 * time.Second
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
		result.Error = fmt.Errorf("连接失败")
	}
	return result
}

// loadKeywords 从文件加载用户定义关键词
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
		if results[i].Error == nil {
			validCount++
		}
	}
	fmt.Printf("\n总计: %d 个节点, %d 个可用, %d 个不可用\n", len(nodes), validCount, len(nodes)-validCount)
}

// ======= 主程序入口 =======

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
	collector := NewCollector(os.Getenv("GITHUB_TOKEN"))

	go func() {
		defer func() { done <- true }()

		// 严格限定只从 keywords.txt 读取采集配置
		keywords, err := loadKeywords("keywords.txt")
		if err != nil || len(keywords) == 0 {
			log.Fatalf("❌ 无法加载 keywords.txt 或文件为空 (%v)，程序终止", err)
			return
		}

		log.Printf("✅ 成功加载 %d 个自定义关键词，开始执行自动化采集...", len(keywords))

		for _, keyword := range keywords {
			err := collector.CollectNodesForKeyword(keyword)
			if err != nil {
				log.Printf("⚠️ 处理关键词 [%s] 时出现错误: %v", keyword, err)
			}
			time.Sleep(2 * time.Second) // 避免不同关键词之间的并发过快被 GitHub 封禁
		}

		log.Println("========== 所有关键词采集并测速打包任务完成 ==========")
	}()

	select {
	case <-done:
		log.Println("========== 工作流结束 ==========")
	case <-ctx.Done():
		if ctx.Err() == context.DeadlineExceeded {
			log.Printf("⏰ 采集超时（%v），强制停止", timeout)
		}
	}
}
