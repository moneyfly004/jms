package main

import (
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
	"sync/atomic"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	GitHubAPIBaseURL = "https://api.github.com"
	MaxRetries       = 3
	RetryDelay       = 5 * time.Second
)

var (
	// 匹配目标链接的正则表达式
	linkPattern = regexp.MustCompile(`https?://(?:jmssub\.net|jjsubmarines\.com)/members/getsub\.php\?[^\s"']+`)
	// 匹配 m7r52rosihxm 链接的正则表达式
	m7r52rosihxmLinkPattern = regexp.MustCompile(`https?://[^\s"']*m7r52rosihxm\.com[^\s"']*`)
	// 匹配建森电器链接的正则表达式
	jiansendianqiLinkPattern = regexp.MustCompile(`https?://[^\s"']*建森电器\.com[^\s"']*`)
	// 匹配 ninjasub 链接的正则表达式
	ninjasubLinkPattern = regexp.MustCompile(`https?://[^\s"']*ninjasub\.com/link[^\s"']*`)
	// 匹配 xueshan 链接的正则表达式
	xueshanLinkPattern = regexp.MustCompile(`https?://[^\s"']*xueshan\.shop/s/[^\s"']*`)
	// 匹配 nginx24zfd 链接的正则表达式
	nginx24zfdLinkPattern = regexp.MustCompile(`https?://[^\s"']*nginx24zfd\.xyz/link/[^\s"']*`)
	// 匹配 iplcme 链接的正则表达式
	iplcmeLinkPattern = regexp.MustCompile(`https?://[^\s"']*iplcme\.com[^\s"']*`)
	// 匹配 smallstrawberry 链接的正则表达式
	smallstrawberryLinkPattern = regexp.MustCompile(`https?://[^\s"']*smallstrawberry\.com[^\s"']*`)
	// 匹配 ssidwork 链接的正则表达式
	ssidworkLinkPattern = regexp.MustCompile(`https?://[^\s"']*ssidwork\.com[^\s"']*`)
	// 匹配 fcsubcn 链接的正则表达式
	fcsubcnLinkPattern = regexp.MustCompile(`https?://[^\s"']*fcsubcn\.cc[^\s"']*`)
	// 匹配 nn8qozmu 链接的正则表达式
	nn8qozmuLinkPattern = regexp.MustCompile(`https?://[^\s"']*nn8qozmu\.top[^\s"']*`)
	// 匹配包含 /api/v1/client/subscribe?token= 的链接的正则表达式
	subscribeTokenPattern = regexp.MustCompile(`https?://[^\s"']*/api/v1/client/subscribe\?token=[^\s"']*`)
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
				IdleConnTimeout:     90 * time.Second,
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

// SearchM7r52rosihxmLinks 搜索 m7r52rosihxm 链接（类似 SearchGhelperLinks，但只提取 m7r52rosihxm 链接）
func (c *Collector) SearchM7r52rosihxmLinks(keywords []string) ([]string, error) {
	var allLinks []string
	seenLinks := make(map[string]bool)

	for _, keyword := range keywords {
		log.Printf("正在搜索 m7r52rosihxm 关键词: %s", keyword)

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

			// 只提取 m7r52rosihxm 链接
			links := c.extractM7r52rosihxmLinks(fileContent)
			for _, link := range links {
				if !seenLinks[link] {
					seenLinks[link] = true
					allLinks = append(allLinks, link)
					log.Printf("发现新 m7r52rosihxm 链接: %s", link)
				}
			}
		}

		// 避免速率限制
		time.Sleep(2 * time.Second)
	}

	return allLinks, nil
}

// extractM7r52rosihxmLinks 提取 m7r52rosihxm 链接
func (c *Collector) extractM7r52rosihxmLinks(content string) []string {
	var links []string
	seenLinks := make(map[string]bool)

	// 提取 m7r52rosihxm 链接（特征：m7r52rosihxm.com）
	m7r52rosihxmMatches := m7r52rosihxmLinkPattern.FindAllString(content, -1)
	for _, match := range m7r52rosihxmMatches {
		link := strings.TrimSpace(match)
		link = strings.TrimRight(link, ".,;!?)")
		link = strings.TrimRight(link, "\"')")

		// 只保留包含 m7r52rosihxm.com 的链接
		if link != "" &&
			strings.Contains(link, "m7r52rosihxm.com") &&
			!seenLinks[link] {
			seenLinks[link] = true
			links = append(links, link)
		}
	}

	return links
}

// SearchJiansendianqiLinks 搜索建森电器链接（类似 SearchGhelperLinks，但只提取建森电器链接）
func (c *Collector) SearchJiansendianqiLinks(keywords []string) ([]string, error) {
	var allLinks []string
	seenLinks := make(map[string]bool)

	for _, keyword := range keywords {
		log.Printf("正在搜索建森电器关键词: %s", keyword)

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

			// 只提取建森电器链接
			links := c.extractJiansendianqiLinks(fileContent)
			for _, link := range links {
				if !seenLinks[link] {
					seenLinks[link] = true
					allLinks = append(allLinks, link)
					log.Printf("发现新建森电器链接: %s", link)
				}
			}
		}

		// 避免速率限制
		time.Sleep(2 * time.Second)
	}

	return allLinks, nil
}

// extractJiansendianqiLinks 提取建森电器链接
func (c *Collector) extractJiansendianqiLinks(content string) []string {
	var links []string
	seenLinks := make(map[string]bool)

	// 提取建森电器链接（特征：建森电器.com）
	jiansendianqiMatches := jiansendianqiLinkPattern.FindAllString(content, -1)
	for _, match := range jiansendianqiMatches {
		link := strings.TrimSpace(match)
		link = strings.TrimRight(link, ".,;!?)")
		link = strings.TrimRight(link, "\"')")

		// 只保留包含建森电器.com 的链接
		if link != "" &&
			strings.Contains(link, "建森电器.com") &&
			!seenLinks[link] {
			seenLinks[link] = true
			links = append(links, link)
		}
	}

	return links
}

// SearchNinjasubLinks 搜索 ninjasub 链接（类似 SearchGhelperLinks，但只提取 ninjasub 链接）
func (c *Collector) SearchNinjasubLinks(keywords []string) ([]string, error) {
	var allLinks []string
	seenLinks := make(map[string]bool)

	for _, keyword := range keywords {
		log.Printf("正在搜索 ninjasub 关键词: %s", keyword)

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

			// 只提取 ninjasub 链接
			links := c.extractNinjasubLinks(fileContent)
			for _, link := range links {
				if !seenLinks[link] {
					seenLinks[link] = true
					allLinks = append(allLinks, link)
					log.Printf("发现新 ninjasub 链接: %s", link)
				}
			}
		}

		// 避免速率限制
		time.Sleep(2 * time.Second)
	}

	return allLinks, nil
}

// extractNinjasubLinks 提取 ninjasub 链接
func (c *Collector) extractNinjasubLinks(content string) []string {
	var links []string
	seenLinks := make(map[string]bool)

	// 提取 ninjasub 链接（特征：ninjasub.com/link）
	ninjasubMatches := ninjasubLinkPattern.FindAllString(content, -1)
	for _, match := range ninjasubMatches {
		link := strings.TrimSpace(match)
		link = strings.TrimRight(link, ".,;!?)")
		link = strings.TrimRight(link, "\"')")

		// 只保留包含 ninjasub.com/link 的链接
		if link != "" &&
			strings.Contains(link, "ninjasub.com/link") &&
			!seenLinks[link] {
			seenLinks[link] = true
			links = append(links, link)
		}
	}

	return links
}

// SearchXueshanLinks 搜索 xueshan 链接（类似 SearchGhelperLinks，但只提取 xueshan 链接）
func (c *Collector) SearchXueshanLinks(keywords []string) ([]string, error) {
	var allLinks []string
	seenLinks := make(map[string]bool)

	for _, keyword := range keywords {
		log.Printf("正在搜索 xueshan 关键词: %s", keyword)

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

			// 只提取 xueshan 链接
			links := c.extractXueshanLinks(fileContent)
			for _, link := range links {
				if !seenLinks[link] {
					seenLinks[link] = true
					allLinks = append(allLinks, link)
					log.Printf("发现新 xueshan 链接: %s", link)
				}
			}
		}

		// 避免速率限制
		time.Sleep(2 * time.Second)
	}

	return allLinks, nil
}

// extractXueshanLinks 提取 xueshan 链接
func (c *Collector) extractXueshanLinks(content string) []string {
	var links []string
	seenLinks := make(map[string]bool)

	// 提取 xueshan 链接（特征：xueshan.shop/s/）
	xueshanMatches := xueshanLinkPattern.FindAllString(content, -1)
	for _, match := range xueshanMatches {
		link := strings.TrimSpace(match)
		link = strings.TrimRight(link, ".,;!?)")
		link = strings.TrimRight(link, "\"')")

		// 只保留包含 xueshan.shop/s/ 的链接
		if link != "" &&
			strings.Contains(link, "xueshan.shop/s/") &&
			!seenLinks[link] {
			seenLinks[link] = true
			links = append(links, link)
		}
	}

	return links
}

// SearchNginx24zfdLinks 搜索 nginx24zfd 链接（类似 SearchGhelperLinks，但只提取 nginx24zfd 链接）
func (c *Collector) SearchNginx24zfdLinks(keywords []string) ([]string, error) {
	var allLinks []string
	seenLinks := make(map[string]bool)

	for _, keyword := range keywords {
		log.Printf("正在搜索 nginx24zfd 关键词: %s", keyword)

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

			// 只提取 nginx24zfd 链接
			links := c.extractNginx24zfdLinks(fileContent)
			for _, link := range links {
				if !seenLinks[link] {
					seenLinks[link] = true
					allLinks = append(allLinks, link)
					log.Printf("发现新 nginx24zfd 链接: %s", link)
				}
			}
		}

		// 避免速率限制
		time.Sleep(2 * time.Second)
	}

	return allLinks, nil
}

// extractNginx24zfdLinks 提取 nginx24zfd 链接
func (c *Collector) extractNginx24zfdLinks(content string) []string {
	var links []string
	seenLinks := make(map[string]bool)

	// 提取 nginx24zfd 链接（特征：nginx24zfd.xyz/link/）
	nginx24zfdMatches := nginx24zfdLinkPattern.FindAllString(content, -1)
	for _, match := range nginx24zfdMatches {
		link := strings.TrimSpace(match)
		link = strings.TrimRight(link, ".,;!?)")
		link = strings.TrimRight(link, "\"')")

		// 只保留包含 nginx24zfd.xyz/link/ 的链接
		if link != "" &&
			strings.Contains(link, "nginx24zfd.xyz/link/") &&
			!seenLinks[link] {
			seenLinks[link] = true
			links = append(links, link)
		}
	}

	return links
}

// SearchIplcmeLinks 搜索 iplcme 链接（类似 SearchGhelperLinks，但只提取 iplcme 链接）
func (c *Collector) SearchIplcmeLinks(keywords []string) ([]string, error) {
	var allLinks []string
	seenLinks := make(map[string]bool)

	for _, keyword := range keywords {
		log.Printf("正在搜索 iplcme 关键词: %s", keyword)

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

			// 只提取 iplcme 链接
			links := c.extractIplcmeLinks(fileContent)
			for _, link := range links {
				if !seenLinks[link] {
					seenLinks[link] = true
					allLinks = append(allLinks, link)
					log.Printf("发现新 iplcme 链接: %s", link)
				}
			}
		}

		// 避免速率限制
		time.Sleep(2 * time.Second)
	}

	return allLinks, nil
}

// extractIplcmeLinks 提取 iplcme 链接
func (c *Collector) extractIplcmeLinks(content string) []string {
	var links []string
	seenLinks := make(map[string]bool)

	// 提取 iplcme 链接（特征：iplcme.com）
	iplcmeMatches := iplcmeLinkPattern.FindAllString(content, -1)
	for _, match := range iplcmeMatches {
		link := strings.TrimSpace(match)
		link = strings.TrimRight(link, ".,;!?)")
		link = strings.TrimRight(link, "\"')")

		// 只保留包含 iplcme.com 的链接
		if link != "" &&
			strings.Contains(link, "iplcme.com") &&
			!seenLinks[link] {
			seenLinks[link] = true
			links = append(links, link)
		}
	}

	return links
}

// SearchSmallstrawberryLinks 搜索 smallstrawberry 链接
func (c *Collector) SearchSmallstrawberryLinks(keywords []string) ([]string, error) {
	var allLinks []string
	seenLinks := make(map[string]bool)

	for _, keyword := range keywords {
		log.Printf("正在搜索 smallstrawberry 关键词: %s", keyword)

		searchURL := fmt.Sprintf("%s/search/code?q=%s&per_page=100", GitHubAPIBaseURL, url.QueryEscape(keyword))

		var results GitHubSearchResult
		if err := c.makeRequest(searchURL, &results); err != nil {
			log.Printf("搜索关键词 %s 失败: %v", keyword, err)
			continue
		}

		log.Printf("找到 %d 个结果", results.TotalCount)

		for _, item := range results.Items {
			fileContent, err := c.getFileContent(item.APIURL)
			if err != nil {
				log.Printf("获取文件内容失败 %s: %v", item.HTMLURL, err)
				continue
			}

			links := c.extractSmallstrawberryLinks(fileContent)
			for _, link := range links {
				if !seenLinks[link] {
					seenLinks[link] = true
					allLinks = append(allLinks, link)
					log.Printf("发现新 smallstrawberry 链接: %s", link)
				}
			}
		}

		time.Sleep(2 * time.Second)
	}

	return allLinks, nil
}

// extractSmallstrawberryLinks 提取 smallstrawberry 链接
func (c *Collector) extractSmallstrawberryLinks(content string) []string {
	var links []string
	seenLinks := make(map[string]bool)

	matches := smallstrawberryLinkPattern.FindAllString(content, -1)
	for _, match := range matches {
		link := strings.TrimSpace(match)
		link = strings.TrimRight(link, ".,;!?)")
		link = strings.TrimRight(link, "\"')")

		if link != "" &&
			strings.Contains(link, "smallstrawberry.com") &&
			!seenLinks[link] {
			seenLinks[link] = true
			links = append(links, link)
		}
	}

	return links
}

// SearchSsidworkLinks 搜索 ssidwork 链接
func (c *Collector) SearchSsidworkLinks(keywords []string) ([]string, error) {
	var allLinks []string
	seenLinks := make(map[string]bool)

	for _, keyword := range keywords {
		log.Printf("正在搜索 ssidwork 关键词: %s", keyword)

		searchURL := fmt.Sprintf("%s/search/code?q=%s&per_page=100", GitHubAPIBaseURL, url.QueryEscape(keyword))

		var results GitHubSearchResult
		if err := c.makeRequest(searchURL, &results); err != nil {
			log.Printf("搜索关键词 %s 失败: %v", keyword, err)
			continue
		}

		log.Printf("找到 %d 个结果", results.TotalCount)

		for _, item := range results.Items {
			fileContent, err := c.getFileContent(item.APIURL)
			if err != nil {
				log.Printf("获取文件内容失败 %s: %v", item.HTMLURL, err)
				continue
			}

			links := c.extractSsidworkLinks(fileContent)
			for _, link := range links {
				if !seenLinks[link] {
					seenLinks[link] = true
					allLinks = append(allLinks, link)
					log.Printf("发现新 ssidwork 链接: %s", link)
				}
			}
		}

		time.Sleep(2 * time.Second)
	}

	return allLinks, nil
}

// extractSsidworkLinks 提取 ssidwork 链接
func (c *Collector) extractSsidworkLinks(content string) []string {
	var links []string
	seenLinks := make(map[string]bool)

	matches := ssidworkLinkPattern.FindAllString(content, -1)
	for _, match := range matches {
		link := strings.TrimSpace(match)
		link = strings.TrimRight(link, ".,;!?)")
		link = strings.TrimRight(link, "\"')")

		if link != "" &&
			strings.Contains(link, "ssidwork.com") &&
			!seenLinks[link] {
			seenLinks[link] = true
			links = append(links, link)
		}
	}

	return links
}

// SearchFcsubcnLinks 搜索 fcsubcn 链接
func (c *Collector) SearchFcsubcnLinks(keywords []string) ([]string, error) {
	var allLinks []string
	seenLinks := make(map[string]bool)

	for _, keyword := range keywords {
		log.Printf("正在搜索 fcsubcn 关键词: %s", keyword)

		searchURL := fmt.Sprintf("%s/search/code?q=%s&per_page=100", GitHubAPIBaseURL, url.QueryEscape(keyword))

		var results GitHubSearchResult
		if err := c.makeRequest(searchURL, &results); err != nil {
			log.Printf("搜索关键词 %s 失败: %v", keyword, err)
			continue
		}

		log.Printf("找到 %d 个结果", results.TotalCount)

		for _, item := range results.Items {
			fileContent, err := c.getFileContent(item.APIURL)
			if err != nil {
				log.Printf("获取文件内容失败 %s: %v", item.HTMLURL, err)
				continue
			}

			links := c.extractFcsubcnLinks(fileContent)
			for _, link := range links {
				if !seenLinks[link] {
					seenLinks[link] = true
					allLinks = append(allLinks, link)
					log.Printf("发现新 fcsubcn 链接: %s", link)
				}
			}
		}

		time.Sleep(2 * time.Second)
	}

	return allLinks, nil
}

// extractFcsubcnLinks 提取 fcsubcn 链接
func (c *Collector) extractFcsubcnLinks(content string) []string {
	var links []string
	seenLinks := make(map[string]bool)

	matches := fcsubcnLinkPattern.FindAllString(content, -1)
	for _, match := range matches {
		link := strings.TrimSpace(match)
		link = strings.TrimRight(link, ".,;!?)")
		link = strings.TrimRight(link, "\"')")

		if link != "" &&
			strings.Contains(link, "fcsubcn.cc") &&
			!seenLinks[link] {
			seenLinks[link] = true
			links = append(links, link)
		}
	}

	return links
}

// SearchNn8qozmuLinks 搜索 nn8qozmu 链接
func (c *Collector) SearchNn8qozmuLinks(keywords []string) ([]string, error) {
	var allLinks []string
	seenLinks := make(map[string]bool)

	for _, keyword := range keywords {
		log.Printf("正在搜索 nn8qozmu 关键词: %s", keyword)

		searchURL := fmt.Sprintf("%s/search/code?q=%s&per_page=100", GitHubAPIBaseURL, url.QueryEscape(keyword))

		var results GitHubSearchResult
		if err := c.makeRequest(searchURL, &results); err != nil {
			log.Printf("搜索关键词 %s 失败: %v", keyword, err)
			continue
		}

		log.Printf("找到 %d 个结果", results.TotalCount)

		for _, item := range results.Items {
			fileContent, err := c.getFileContent(item.APIURL)
			if err != nil {
				log.Printf("获取文件内容失败 %s: %v", item.HTMLURL, err)
				continue
			}

			links := c.extractNn8qozmuLinks(fileContent)
			for _, link := range links {
				if !seenLinks[link] {
					seenLinks[link] = true
					allLinks = append(allLinks, link)
					log.Printf("发现新 nn8qozmu 链接: %s", link)
				}
			}
		}

		time.Sleep(2 * time.Second)
	}

	return allLinks, nil
}

// extractNn8qozmuLinks 提取 nn8qozmu 链接
func (c *Collector) extractNn8qozmuLinks(content string) []string {
	var links []string
	seenLinks := make(map[string]bool)

	matches := nn8qozmuLinkPattern.FindAllString(content, -1)
	for _, match := range matches {
		link := strings.TrimSpace(match)
		link = strings.TrimRight(link, ".,;!?)")
		link = strings.TrimRight(link, "\"')")

		if link != "" &&
			strings.Contains(link, "nn8qozmu.top") &&
			!seenLinks[link] {
			seenLinks[link] = true
			links = append(links, link)
		}
	}

	return links
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
	// 1. 先尝试解析 Clash YAML
	if nodes := c.parseClashYAML(content); len(nodes) > 0 {
		return nodes
	}

	// 2. 尝试解析 sing-box JSON
	if nodes := c.parseSingBoxJSON(content); len(nodes) > 0 {
		return nodes
	}

	// 3. 从文本中提取节点链接（支持所有协议）
	return c.extractNodeLinks(content)
}

// convertClashProxyToLink 将 Clash 代理配置转换为节点链接
func convertClashProxyToLink(proxy map[string]interface{}) string {
	proxyType, _ := proxy["type"].(string)
	if proxyType == "" {
		return ""
	}

	server, _ := proxy["server"].(string)
	if server == "" {
		return ""
	}

	var port interface{}
	if p, ok := proxy["port"]; ok {
		port = p
	} else {
		port = 443
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

// convertSingBoxOutboundToLink 将 sing-box outbound 配置转换为节点链接
func convertSingBoxOutboundToLink(outbound map[string]interface{}) string {
	outboundType, _ := outbound["type"].(string)
	if outboundType == "" {
		return ""
	}

	server, _ := outbound["server"].(string)
	if server == "" {
		return ""
	}

	var port interface{}
	if p, ok := outbound["server_port"]; ok {
		port = p
	} else {
		port = 443
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

// Clash 格式转换函数
func convertClashSS(proxy map[string]interface{}, server, port, name string) string {
	cipher, _ := proxy["cipher"].(string)
	password, _ := proxy["password"].(string)
	if cipher == "" || password == "" {
		return ""
	}
	// ss://base64(method:password@server:port)#name
	auth := fmt.Sprintf("%s:%s", cipher, password)
	authBase64 := base64.StdEncoding.EncodeToString([]byte(auth))
	return fmt.Sprintf("ss://%s@%s:%s#%s", authBase64, server, port, url.QueryEscape(name))
}

func convertClashVMess(proxy map[string]interface{}, server, port, name string) string {
	uuid, _ := proxy["uuid"].(string)
	if uuid == "" {
		return ""
	}
	// 构建 VMess JSON
	vmessData := map[string]interface{}{
		"v":    "2",
		"ps":   name,
		"add":  server,
		"port": port,
		"id":   uuid,
		"aid":  0,
		"net":  "tcp",
		"type": "none",
		"tls":  "",
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
	u := url.URL{
		Scheme:   "vless",
		User:     url.User(uuid),
		Host:     fmt.Sprintf("%s:%s", server, port),
		Fragment: name,
	}
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
	u := url.URL{
		Scheme:   "trojan",
		User:     url.User(password),
		Host:     fmt.Sprintf("%s:%s", server, port),
		Fragment: name,
	}
	q := url.Values{}
	if sni, ok := proxy["sni"].(string); ok && sni != "" {
		q.Set("sni", sni)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func convertClashSSR(proxy map[string]interface{}, server, port, name string) string {
	// SSR 格式较复杂，这里简化处理
	cipher, _ := proxy["cipher"].(string)
	password, _ := proxy["password"].(string)
	protocol, _ := proxy["protocol"].(string)
	obfs, _ := proxy["obfs"].(string)
	if cipher == "" || password == "" {
		return ""
	}
	// ssr://base64(server:port:protocol:cipher:obfs:base64(password))
	passwordBase64 := base64.StdEncoding.EncodeToString([]byte(password))
	ssrStr := fmt.Sprintf("%s:%s:%s:%s:%s:%s", server, port, protocol, cipher, obfs, passwordBase64)
	return "ssr://" + base64.URLEncoding.EncodeToString([]byte(ssrStr))
}

func convertClashHysteria(proxy map[string]interface{}, server, port, name string) string {
	u := url.URL{
		Scheme:   "hysteria",
		Host:     fmt.Sprintf("%s:%s", server, port),
		Fragment: name,
	}
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
	u := url.URL{
		Scheme:   "wireguard",
		User:     url.User(privateKey),
		Host:     fmt.Sprintf("%s:%s", server, port),
		Fragment: name,
	}
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
	u := url.URL{
		Scheme:   "tuic",
		User:     url.User(uuid),
		Host:     fmt.Sprintf("%s:%s", server, port),
		Fragment: name,
	}
	q := url.Values{}
	if token, ok := proxy["token"].(string); ok && token != "" {
		q.Set("token", token)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func convertClashHTTP(proxy map[string]interface{}, server, port, name string) string {
	u := url.URL{
		Scheme:   "http",
		Host:     fmt.Sprintf("%s:%s", server, port),
		Fragment: name,
	}
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
	u := url.URL{
		Scheme:   "socks5",
		Host:     fmt.Sprintf("%s:%s", server, port),
		Fragment: name,
	}
	if username, ok := proxy["username"].(string); ok && username != "" {
		if password, ok := proxy["password"].(string); ok && password != "" {
			u.User = url.UserPassword(username, password)
		} else {
			u.User = url.User(username)
		}
	}
	return u.String()
}

// sing-box 格式转换函数
func convertSingBoxSS(outbound map[string]interface{}, server, port, tag string) string {
	method, _ := outbound["method"].(string)
	password, _ := outbound["password"].(string)
	if method == "" || password == "" {
		return ""
	}
	auth := fmt.Sprintf("%s:%s", method, password)
	authBase64 := base64.StdEncoding.EncodeToString([]byte(auth))
	return fmt.Sprintf("ss://%s@%s:%s#%s", authBase64, server, port, url.QueryEscape(tag))
}

func convertSingBoxVMess(outbound map[string]interface{}, server, port, tag string) string {
	uuid, _ := outbound["uuid"].(string)
	if uuid == "" {
		return ""
	}
	vmessData := map[string]interface{}{
		"v":    "2",
		"ps":   tag,
		"add":  server,
		"port": port,
		"id":   uuid,
		"aid":  0,
		"net":  "tcp",
		"type": "none",
		"tls":  "",
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
	u := url.URL{
		Scheme:   "vless",
		User:     url.User(uuid),
		Host:     fmt.Sprintf("%s:%s", server, port),
		Fragment: tag,
	}
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
	u := url.URL{
		Scheme:   "trojan",
		User:     url.User(password),
		Host:     fmt.Sprintf("%s:%s", server, port),
		Fragment: tag,
	}
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
	u := url.URL{
		Scheme:   "hysteria",
		Host:     fmt.Sprintf("%s:%s", server, port),
		Fragment: tag,
	}
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
	u := url.URL{
		Scheme:   "wireguard",
		User:     url.User(privateKey),
		Host:     fmt.Sprintf("%s:%s", server, port),
		Fragment: tag,
	}
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
	u := url.URL{
		Scheme:   "tuic",
		User:     url.User(uuid),
		Host:     fmt.Sprintf("%s:%s", server, port),
		Fragment: tag,
	}
	q := url.Values{}
	if token, ok := outbound["token"].(string); ok && token != "" {
		q.Set("token", token)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func convertSingBoxHTTP(outbound map[string]interface{}, server, port, tag string) string {
	u := url.URL{
		Scheme:   "http",
		Host:     fmt.Sprintf("%s:%s", server, port),
		Fragment: tag,
	}
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
	u := url.URL{
		Scheme:   "socks5",
		Host:     fmt.Sprintf("%s:%s", server, port),
		Fragment: tag,
	}
	if username, ok := outbound["username"].(string); ok && username != "" {
		if password, ok := outbound["password"].(string); ok && password != "" {
			u.User = url.UserPassword(username, password)
		} else {
			u.User = url.User(username)
		}
	}
	return u.String()
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

// ParseNodes 解析节点（支持多种格式：base64、Clash YAML、sing-box JSON、纯文本）
func (c *Collector) ParseNodes(content string) ([]string, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, fmt.Errorf("内容为空")
	}

	// 1. 尝试解析 Clash YAML 格式
	if nodes := c.parseClashYAML(content); len(nodes) > 0 {
		return nodes, nil
	}

	// 2. 尝试解析 sing-box JSON 格式
	if nodes := c.parseSingBoxJSON(content); len(nodes) > 0 {
		return nodes, nil
	}

	// 3. 尝试 base64 解码
	var decoded string
	var err error

	decodedBytes, err := base64.StdEncoding.DecodeString(content)
	if err != nil {
		decodedBytes, err = base64.URLEncoding.DecodeString(content)
		if err != nil {
			decoded, err = safeBase64Decode(content)
			if err != nil {
				// 4. 如果都失败，可能是纯文本格式，直接使用原始内容
				decoded = content
			}
		} else {
			decoded = string(decodedBytes)
		}
	} else {
		decoded = string(decodedBytes)
	}

	// 5. 从解码后的内容提取节点链接
	return c.extractNodeLinks(decoded), nil
}

// parseClashYAML 解析 Clash YAML 格式
func (c *Collector) parseClashYAML(content string) []string {
	var config struct {
		Proxies []map[string]interface{} `yaml:"proxies"`
	}

	// 尝试解析 YAML
	if err := yaml.Unmarshal([]byte(content), &config); err != nil {
		return nil
	}

	var nodes []string
	seenNodes := make(map[string]bool)

	for _, proxy := range config.Proxies {
		if nodeLink := convertClashProxyToLink(proxy); nodeLink != "" {
			if !seenNodes[nodeLink] {
				seenNodes[nodeLink] = true
				nodes = append(nodes, nodeLink)
			}
		}
	}

	return nodes
}

// parseSingBoxJSON 解析 sing-box JSON 格式
func (c *Collector) parseSingBoxJSON(content string) []string {
	var config struct {
		Outbounds []map[string]interface{} `json:"outbounds"`
	}

	// 尝试解析 JSON
	if err := json.Unmarshal([]byte(content), &config); err != nil {
		return nil
	}

	var nodes []string
	seenNodes := make(map[string]bool)

	for _, outbound := range config.Outbounds {
		if nodeLink := convertSingBoxOutboundToLink(outbound); nodeLink != "" {
			if !seenNodes[nodeLink] {
				seenNodes[nodeLink] = true
				nodes = append(nodes, nodeLink)
			}
		}
	}

	return nodes
}

// extractNodeLinks 从文本内容中提取所有类型的节点链接
func (c *Collector) extractNodeLinks(content string) []string {
	var nodes []string
	seenNodes := make(map[string]bool)

	// 支持多种分隔符
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// 检查是否是节点链接（支持所有协议）
		if isNodeLink(line) {
			if !seenNodes[line] {
				seenNodes[line] = true
				nodes = append(nodes, line)
			}
		}
	}

	return nodes
}

// isNodeLink 检查是否是节点链接
func isNodeLink(line string) bool {
	return strings.HasPrefix(line, "ss://") ||
		strings.HasPrefix(line, "vmess://") ||
		strings.HasPrefix(line, "vless://") ||
		strings.HasPrefix(line, "trojan://") ||
		strings.HasPrefix(line, "ssr://") ||
		strings.HasPrefix(line, "hysteria://") ||
		strings.HasPrefix(line, "hy2://") ||
		strings.HasPrefix(line, "wireguard://") ||
		strings.HasPrefix(line, "wg://") ||
		strings.HasPrefix(line, "tuic://") ||
		strings.HasPrefix(line, "http://") ||
		strings.HasPrefix(line, "https://") ||
		strings.HasPrefix(line, "socks://") ||
		strings.HasPrefix(line, "socks4://") ||
		strings.HasPrefix(line, "socks5://") ||
		strings.HasPrefix(line, "anytls://") ||
		strings.HasPrefix(line, "gost://") ||
		strings.HasPrefix(line, "gost+")
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

	// TCP 连接测试（减少超时时间，提高速度）
	timeout := 3 * time.Second // 减少到 3 秒
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

	// 采集节点（不去重，让所有节点都进行测试，最后统一去重）
	var allValidNodes []*ValidNode
	var allParsedNodes []string // 保存所有解析出的节点（用于统计）
	var wg sync.WaitGroup
	resultsChan := make(chan *NodeResult, len(links))
	var mu sync.Mutex

	// 并发采集，增加并发数以提高速度
	maxConcurrency := 20 // 默认增加到 20，提高速度
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

			// 测试节点（不去重，让所有节点都进行测试，最后统一去重）
			for _, nodeLink := range nodes {
				// 记录所有解析出的节点（用于统计）
				mu.Lock()
				allParsedNodes = append(allParsedNodes, nodeLink)
				mu.Unlock()

				// 默认使用 sing-box 进行真实链接测速
				var validNode *ValidNode
				useSingBox := os.Getenv("USE_SINGBOX")
				if useSingBox == "false" {
					// 只有明确禁用时才使用 TCP 测试
					validNode = c.TestNode(nodeLink)
				} else {
					// 默认使用 sing-box 进行真实链接测速
					validNode = c.TestNodeWithSingBox(nodeLink)
				}
				// 如果测试通过，添加到结果中（不去重，最后统一去重）
				if validNode.Error == nil {
					result.ValidNodes = append(result.ValidNodes, validNode)
					mu.Lock()
					allValidNodes = append(allValidNodes, validNode)
					mu.Unlock()
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
	typeStats := make(map[string]int)       // 所有解析出的节点类型统计
	validTypeStats := make(map[string]int)  // 测试通过的节点类型统计
	failedTypeStats := make(map[string]int) // 测试失败的节点类型统计
	totalNodes := 0
	totalValidNodes := 0

	for result := range resultsChan {
		if result.Error != nil {
			log.Printf("链接 %s 处理失败: %v", result.Link, result.Error)
		} else {
			totalNodes += len(result.Nodes)
			validCount := 0
			for _, validNode := range result.ValidNodes {
				if validNode.Error == nil {
					validCount++
				}
			}
			totalValidNodes += validCount

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
				result.Link, len(result.Nodes), validCount)
		}
	}

	log.Printf("采集完成，共解析 %d 个节点（去重后 %d 个），%d 个可用节点", totalNodes, len(allParsedNodes), len(allValidNodes))
	if len(typeStats) > 0 {
		log.Printf("解析出的节点类型统计:")
		for nodeType, count := range typeStats {
			validCount := validTypeStats[nodeType]
			failedCount := failedTypeStats[nodeType]
			log.Printf("  %s: 共 %d 个 (可用: %d, 失败: %d)", nodeType, count, validCount, failedCount)
		}
	}

	// 保存结果
	// 默认只保存测试通过的节点
	// 如果需要保存所有节点（包括测试失败的），设置 SAVE_ALL_NODES=true
	saveAllNodes := os.Getenv("SAVE_ALL_NODES") == "true"
	if saveAllNodes {
		log.Printf("保存所有解析出的节点（包括测试失败的）")
		return c.SaveAllNodes(allParsedNodes)
	} else {
		// 只保存测试通过的节点（Error == nil）
		log.Printf("只保存测试通过的节点（共 %d 个）", len(allValidNodes))
		return c.SaveResults(allValidNodes)
	}
}

// collectNodesGeneric 通用的节点采集函数，提取所有重复的采集逻辑
// searchFunc: 搜索链接的函数
// name: 采集器名称（用于日志）
// keywords: 搜索关键词列表
func (c *Collector) collectNodesGeneric(searchFunc func([]string) ([]string, error), name string, keywords []string) error {
	// 搜索链接
	links, err := searchFunc(keywords)
	if err != nil {
		return fmt.Errorf("搜索 %s 链接失败: %v", name, err)
	}

	log.Printf("共找到 %d 个 %s 链接，开始采集节点", len(links), name)

	// 采集节点（不去重，让所有节点都进行测试，最后统一去重）
	var allValidNodes []*ValidNode
	var allParsedNodes []string
	var wg sync.WaitGroup
	resultsChan := make(chan *NodeResult, len(links))
	var mu sync.Mutex

	// 并发采集
	maxConcurrency := 10
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
				log.Printf("%s 链接 %s 解析失败: %v，尝试从原始内容提取", name, l, err)
				nodes = c.extractNodesFromRawContent(content)
				if len(nodes) == 0 {
					result.Error = err
					resultsChan <- result
					return
				}
				log.Printf("从原始内容提取到 %d 个节点", len(nodes))
			}

			result.Nodes = nodes
			log.Printf("%s 链接 %s 解析出 %d 个节点", name, l, len(nodes))

			// 测试节点（不去重，让所有节点都进行测试）
			for _, nodeLink := range nodes {
				// 记录所有解析出的节点（用于统计）
				mu.Lock()
				allParsedNodes = append(allParsedNodes, nodeLink)
				mu.Unlock()

				// 默认使用 sing-box 进行真实链接测速
				var validNode *ValidNode
				useSingBox := os.Getenv("USE_SINGBOX")
				if useSingBox == "false" {
					validNode = c.TestNode(nodeLink)
				} else {
					validNode = c.TestNodeWithSingBox(nodeLink)
				}
				// 如果测试通过，添加到结果中（不去重，最后统一去重）
				if validNode.Error == nil {
					result.ValidNodes = append(result.ValidNodes, validNode)
					mu.Lock()
					allValidNodes = append(allValidNodes, validNode)
					mu.Unlock()
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
	typeStats := make(map[string]int)
	validTypeStats := make(map[string]int)
	failedTypeStats := make(map[string]int)
	totalNodes := 0
	totalValidNodes := 0

	for result := range resultsChan {
		if result.Error != nil {
			log.Printf("%s 链接 %s 处理失败: %v", name, result.Link, result.Error)
		} else {
			totalNodes += len(result.Nodes)
			// 只统计真正可用的节点（Error == nil）
			for _, validNode := range result.ValidNodes {
				if validNode.Error == nil {
					totalValidNodes++
				}
			}

			// 统计所有解析出的节点类型
			for _, nodeLink := range result.Nodes {
				if node, err := ParseNodeLink(nodeLink); err == nil {
					typeStats[node.Type]++
				}
			}

			// 统计测试结果（只统计真正可用的节点）
			validCount := 0
			for _, validNode := range result.ValidNodes {
				if validNode.Type != "" {
					if validNode.Error == nil {
						validTypeStats[validNode.Type]++
						validCount++
					} else {
						failedTypeStats[validNode.Type]++
					}
				}
			}

			log.Printf("%s 链接 %s: 共 %d 个节点，%d 个可用",
				name, result.Link, len(result.Nodes), validCount)
		}
	}

	log.Printf("%s 节点采集完成，共解析 %d 个节点（去重后 %d 个），%d 个可用节点", name, totalNodes, len(allParsedNodes), len(allValidNodes))
	if len(typeStats) > 0 {
		log.Printf("解析出的节点类型统计:")
		for nodeType, count := range typeStats {
			validCount := validTypeStats[nodeType]
			failedCount := failedTypeStats[nodeType]
			log.Printf("  %s: 共 %d 个 (可用: %d, 失败: %d)", nodeType, count, validCount, failedCount)
		}
	}

	// 保存结果到 nodes.txt（追加模式，保留现有节点）
	return c.saveNodesToFile(allValidNodes, name)
}

// saveNodesToFile 保存节点到 nodes.txt 文件（追加模式）
func (c *Collector) saveNodesToFile(allValidNodes []*ValidNode, name string) error {
	// 读取现有的 nodes.txt 内容（如果存在）
	existingContent := ""
	if content, err := os.ReadFile("nodes.txt"); err == nil {
		existingContent = string(content)
	}

	// 收集所有节点链接（去重）
	var allNodes []string
	seenNodes := make(map[string]bool)

	// 先添加现有节点
	if existingContent != "" {
		// 尝试解码 Base64（如果文件是 Base64 编码的）
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(existingContent))
		if err == nil {
			// 成功解码，说明文件是 Base64 编码的
			existingContent = string(decoded)
		}
		// 如果解码失败，说明文件是原始格式，直接使用

		lines := strings.Split(strings.TrimSpace(existingContent), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" && !seenNodes[line] {
				seenNodes[line] = true
				allNodes = append(allNodes, line)
			}
		}
	}

	// 添加新的节点（只添加测试成功的节点）
	for _, node := range allValidNodes {
		if node.Error == nil && !seenNodes[node.Link] {
			seenNodes[node.Link] = true
			allNodes = append(allNodes, node.Link)
		}
	}

	// 将所有节点链接合并为字符串（每行一个）
	plainContent := strings.Join(allNodes, "\n")

	// 进行 Base64 编码，以便 v2rayN 等客户端订阅使用
	encodedContent := base64.StdEncoding.EncodeToString([]byte(plainContent))

	// 写入文件（Base64 编码后的内容）
	outputFile := "nodes.txt"
	if err := os.WriteFile(outputFile, []byte(encodedContent), 0644); err != nil {
		return fmt.Errorf("写入文件失败: %v", err)
	}

	log.Printf("%s 节点已保存到 %s，共 %d 个节点（包含现有节点，已 Base64 编码）", name, outputFile, len(allNodes))

	// 推送到 Gist（如果配置了）
	gistID := os.Getenv("GIST_ID")
	gistToken := os.Getenv("GIST_TOKEN")
	if gistToken == "" {
		gistToken = c.githubToken
	}

	if gistID != "" || gistToken != "" {
		log.Printf("准备推送到 Gist (ID: %s)...", gistID)
		// 只推送测试成功的节点（去重后的）
		var validNodesForGist []*ValidNode
		seenGistNodes := make(map[string]bool)
		for _, node := range allValidNodes {
			if node.Error == nil {
				nodeLink := strings.TrimSpace(node.Link)
				if nodeLink != "" && !seenGistNodes[nodeLink] {
					seenGistNodes[nodeLink] = true
					validNodesForGist = append(validNodesForGist, node)
				}
			}
		}
		if err := c.PushToGist(outputFile, validNodesForGist); err != nil {
			log.Printf("❌ 推送到 Gist 失败: %v", err)
		} else {
			log.Printf("✅ Gist 推送成功，本地文件已保存到 %s", outputFile)
		}
	}

	return nil
}

// CollectM7r52rosihxmNodes 采集 m7r52rosihxm 链接中的节点，保存到 nodes.txt
func (c *Collector) CollectM7r52rosihxmNodes() error {
	keywords := []string{
		"m7r52rosihxm.com",
	}
	return c.collectNodesGeneric(c.SearchM7r52rosihxmLinks, "m7r52rosihxm", keywords)
}

// CollectJiansendianqiNodes 采集建森电器链接中的节点，保存到 nodes.txt
func (c *Collector) CollectJiansendianqiNodes() error {
	keywords := []string{
		"建森电器.com",
		"建森电器",
	}
	return c.collectNodesGeneric(c.SearchJiansendianqiLinks, "建森电器", keywords)
}

// CollectNinjasubNodes 采集 ninjasub 链接中的节点，保存到 nodes.txt
func (c *Collector) CollectNinjasubNodes() error {
	keywords := []string{
		"ninjasub.com/link",
		"ninjasub.com/link/",
	}
	return c.collectNodesGeneric(c.SearchNinjasubLinks, "ninjasub", keywords)
}

// CollectXueshanNodes 采集 xueshan 链接中的节点，保存到 nodes.txt
func (c *Collector) CollectXueshanNodes() error {
	keywords := []string{
		"xueshan.shop/s",
		"xueshan.shop/s/",
	}
	return c.collectNodesGeneric(c.SearchXueshanLinks, "xueshan", keywords)
}

// CollectNginx24zfdNodes 采集 nginx24zfd 链接中的节点，保存到 nodes.txt
func (c *Collector) CollectNginx24zfdNodes() error {
	keywords := []string{
		"nginx24zfd.xyz/link",
		"nginx24zfd.xyz/link/",
	}
	return c.collectNodesGeneric(c.SearchNginx24zfdLinks, "nginx24zfd", keywords)
}

// CollectIplcmeNodes 采集 iplcme 链接中的节点，保存到 nodes.txt
func (c *Collector) CollectIplcmeNodes() error {
	keywords := []string{
		"iplcme.com",
	}
	return c.collectNodesGeneric(c.SearchIplcmeLinks, "iplcme", keywords)
}

// CollectSmallstrawberryNodes 采集 smallstrawberry 链接中的节点，保存到 nodes.txt
func (c *Collector) CollectSmallstrawberryNodes() error {
	keywords := []string{
		"smallstrawberry.com",
	}
	return c.collectNodesGeneric(c.SearchSmallstrawberryLinks, "smallstrawberry", keywords)
}

// CollectSsidworkNodes 采集 ssidwork 链接中的节点，保存到 nodes.txt
func (c *Collector) CollectSsidworkNodes() error {
	keywords := []string{
		"ssidwork.com",
	}
	return c.collectNodesGeneric(c.SearchSsidworkLinks, "ssidwork", keywords)
}

// CollectFcsubcnNodes 采集 fcsubcn 链接中的节点，保存到 nodes.txt
func (c *Collector) CollectFcsubcnNodes() error {
	keywords := []string{
		"fcsubcn.cc",
	}
	return c.collectNodesGeneric(c.SearchFcsubcnLinks, "fcsubcn", keywords)
}

// CollectNn8qozmuNodes 采集 nn8qozmu 链接中的节点，保存到 nodes.txt
func (c *Collector) CollectNn8qozmuNodes() error {
	keywords := []string{
		"nn8qozmu.top",
	}
	return c.collectNodesGeneric(c.SearchNn8qozmuLinks, "nn8qozmu", keywords)
}

// SearchSubscribeTokenLinks 搜索包含 /api/v1/client/subscribe?token= 的链接
// 只返回三个月内更新的仓库中的链接
func (c *Collector) SearchSubscribeTokenLinks(keywords []string) ([]string, error) {
	var allLinks []string
	seenLinks := make(map[string]bool)

	// 计算三个月前的日期，用于 GitHub 搜索过滤
	threeMonthsAgo := time.Now().AddDate(0, -3, 0).Format("2006-01-02")

	for _, keyword := range keywords {
		log.Printf("正在搜索订阅 token 关键词: %s（仅三个月内更新的仓库）", keyword)

		// GitHub Code Search API 支持 pushed:>日期 过滤仓库最近推送时间
		searchURL := fmt.Sprintf("%s/search/code?q=%s+pushed:>%s&per_page=100",
			GitHubAPIBaseURL, url.QueryEscape(keyword), threeMonthsAgo)

		var results GitHubSearchResult
		if err := c.makeRequest(searchURL, &results); err != nil {
			log.Printf("搜索关键词 %s 失败: %v", keyword, err)
			continue
		}

		log.Printf("找到 %d 个结果（三个月内更新）", results.TotalCount)

		// 处理每个结果
		for _, item := range results.Items {
			// 获取文件内容
			fileContent, err := c.getFileContent(item.APIURL)
			if err != nil {
				log.Printf("获取文件内容失败 %s: %v", item.HTMLURL, err)
				continue
			}

			// 提取包含 /api/v1/client/subscribe?token= 的链接
			links := c.extractSubscribeTokenLinks(fileContent)
			for _, link := range links {
				if !seenLinks[link] {
					seenLinks[link] = true
					allLinks = append(allLinks, link)
					log.Printf("发现新订阅 token 链接: %s", link)
				}
			}
		}

		// 避免速率限制
		time.Sleep(2 * time.Second)
	}

	return allLinks, nil
}

// extractSubscribeTokenLinks 提取包含 /api/v1/client/subscribe?token= 的链接
func (c *Collector) extractSubscribeTokenLinks(content string) []string {
	var links []string
	seenLinks := make(map[string]bool)

	// 提取包含 /api/v1/client/subscribe?token= 的链接
	matches := subscribeTokenPattern.FindAllString(content, -1)
	for _, match := range matches {
		link := strings.TrimSpace(match)
		link = strings.TrimRight(link, ".,;!?)")
		link = strings.TrimRight(link, "\"')")

		// 确保链接包含 /api/v1/client/subscribe?token=
		if link != "" &&
			strings.Contains(link, "/api/v1/client/subscribe?token=") &&
			!seenLinks[link] {
			seenLinks[link] = true
			links = append(links, link)
		}
	}

	return links
}

// CollectSubscribeTokenNodes 采集包含 /api/v1/client/subscribe?token= 的链接中的节点
// 只收集有节点数据的链接，最多收集 200 个
func (c *Collector) CollectSubscribeTokenNodes() error {
	keywords := []string{
		"/api/v1/client/subscribe?token=",
		"api/v1/client/subscribe",
		"client/subscribe?token",
	}

	// 搜索链接
	links, err := c.SearchSubscribeTokenLinks(keywords)
	if err != nil {
		return fmt.Errorf("搜索订阅 token 链接失败: %v", err)
	}

	log.Printf("共找到 %d 个订阅 token 链接，开始采集节点（目标：200 个有节点数据的链接）", len(links))

	// 采集节点，只统计有节点数据的链接
	var allValidNodes []*ValidNode
	var allParsedNodes []string
	var validLinks []string // 记录有节点数据的链接
	var wg sync.WaitGroup
	resultsChan := make(chan *NodeResult, len(links))
	var mu sync.Mutex
	var validLinksCount int32 // 使用原子操作来统计有节点数据的链接数量

	// 并发采集
	maxConcurrency := 10
	if maxConcurrencyEnv := os.Getenv("MAX_CONCURRENCY"); maxConcurrencyEnv != "" {
		if n, err := strconv.Atoi(maxConcurrencyEnv); err == nil && n > 0 {
			maxConcurrency = n
		}
	}
	semaphore := make(chan struct{}, maxConcurrency)

	for _, link := range links {
		// 如果已经收集到 200 个有节点数据的链接，停止
		if atomic.LoadInt32(&validLinksCount) >= 200 {
			log.Printf("已收集到 200 个有节点数据的链接，停止采集")
			break
		}

		wg.Add(1)
		go func(l string) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// 再次检查是否已经达到 200 个
			if atomic.LoadInt32(&validLinksCount) >= 200 {
				return
			}

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
				log.Printf("订阅 token 链接 %s 解析失败: %v，尝试从原始内容提取", l, err)
				nodes = c.extractNodesFromRawContent(content)
				if len(nodes) == 0 {
					result.Error = err
					resultsChan <- result
					return
				}
				log.Printf("从原始内容提取到 %d 个节点", len(nodes))
			}

			// 如果没有节点数据，不计算在内
			if len(nodes) == 0 {
				log.Printf("订阅 token 链接 %s 没有节点数据，跳过", l)
				result.Error = fmt.Errorf("没有节点数据")
				resultsChan <- result
				return
			}

			result.Nodes = nodes
			log.Printf("订阅 token 链接 %s 解析出 %d 个节点", l, len(nodes))

			// 记录有节点数据的链接（使用原子操作）
			mu.Lock()
			validLinks = append(validLinks, l)
			currentCount := int32(len(validLinks))
			mu.Unlock()
			atomic.StoreInt32(&validLinksCount, currentCount)

			// 测试节点（不去重，让所有节点都进行测试）
			for _, nodeLink := range nodes {
				// 记录所有解析出的节点（用于统计）
				mu.Lock()
				allParsedNodes = append(allParsedNodes, nodeLink)
				mu.Unlock()

				// 默认使用 sing-box 进行真实链接测速
				var validNode *ValidNode
				useSingBox := os.Getenv("USE_SINGBOX")
				if useSingBox == "false" {
					validNode = c.TestNode(nodeLink)
				} else {
					validNode = c.TestNodeWithSingBox(nodeLink)
				}
				// 如果测试通过，添加到结果中（不去重，最后统一去重）
				if validNode.Error == nil {
					result.ValidNodes = append(result.ValidNodes, validNode)
					mu.Lock()
					allValidNodes = append(allValidNodes, validNode)
					mu.Unlock()
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
	typeStats := make(map[string]int)
	validTypeStats := make(map[string]int)
	failedTypeStats := make(map[string]int)
	totalNodes := 0
	totalValidNodes := 0

	for result := range resultsChan {
		if result.Error != nil {
			if result.Error.Error() != "没有节点数据" {
				log.Printf("订阅 token 链接 %s 处理失败: %v", result.Link, result.Error)
			}
		} else {
			totalNodes += len(result.Nodes)
			validCount := 0
			for _, validNode := range result.ValidNodes {
				if validNode.Error == nil {
					validCount++
				}
			}
			totalValidNodes += validCount

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

			log.Printf("订阅 token 链接 %s: 共 %d 个节点，%d 个可用",
				result.Link, len(result.Nodes), validCount)
		}
	}

	log.Printf("订阅 token 节点采集完成，共找到 %d 个有节点数据的链接，解析 %d 个节点（去重后 %d 个），%d 个可用节点",
		len(validLinks), totalNodes, len(allParsedNodes), len(allValidNodes))
	if len(typeStats) > 0 {
		log.Printf("解析出的节点类型统计:")
		for nodeType, count := range typeStats {
			validCount := validTypeStats[nodeType]
			failedCount := failedTypeStats[nodeType]
			log.Printf("  %s: 总数 %d, 可用 %d, 不可用 %d", nodeType, count, validCount, failedCount)
		}
	}

	// 保存节点到文件
	if err := c.saveNodesToFile(allValidNodes, "subscribe-token"); err != nil {
		return fmt.Errorf("保存节点失败: %v", err)
	}

	return nil
}

// SaveResults 保存结果
func (c *Collector) SaveResults(nodes []*ValidNode) error {
	// 创建输出文件
	outputFile := "nodes.txt"

	// 收集所有节点链接
	var nodeLinks []string
	for _, node := range nodes {
		nodeLinks = append(nodeLinks, node.Link)
	}

	// 将所有节点链接合并为字符串（每行一个）
	plainContent := strings.Join(nodeLinks, "\n")

	// 进行 Base64 编码，以便 v2rayN 等客户端订阅使用
	encodedContent := base64.StdEncoding.EncodeToString([]byte(plainContent))

	// 写入文件（Base64 编码后的内容）
	if err := os.WriteFile(outputFile, []byte(encodedContent), 0644); err != nil {
		return fmt.Errorf("写入文件失败: %v", err)
	}

	log.Printf("结果已保存到 %s，共 %d 个节点（已 Base64 编码）", outputFile, len(nodes))

	// 优先推送到 Gist（适用于私有仓库）
	gistID := os.Getenv("GIST_ID")
	gistToken := os.Getenv("GIST_TOKEN")
	if gistID != "" || gistToken != "" {
		log.Printf("准备推送到 Gist (ID: %s)...", gistID)
		if err := c.PushToGist(outputFile, nodes); err != nil {
			log.Printf("❌ 推送到 Gist 失败: %v", err)
			// Gist 推送失败，继续推送到仓库（如果有配置）
		} else {
			log.Printf("✅ Gist 推送成功，本地文件已保存到 %s", outputFile)
			// 注意：即使 Gist 推送成功，本地文件也已经保存，工作流会提交它
			return nil // Gist 推送成功，不再推送到 GitHub API
		}
	}

	// 如果配置了 GitHub，推送到 GitHub
	if repo := os.Getenv("GITHUB_REPO"); repo != "" {
		return c.PushToGitHub(outputFile, nodes)
	}

	return nil
}

// SaveAllNodes 保存所有解析出的节点（包括测试失败的）
func (c *Collector) SaveAllNodes(nodeLinks []string) error {
	// 创建输出文件
	outputFile := "nodes.txt"

	// 将所有节点链接合并为字符串（每行一个）
	plainContent := strings.Join(nodeLinks, "\n")

	// 进行 Base64 编码，以便 v2rayN 等客户端订阅使用
	encodedContent := base64.StdEncoding.EncodeToString([]byte(plainContent))

	// 写入文件（Base64 编码后的内容）
	if err := os.WriteFile(outputFile, []byte(encodedContent), 0644); err != nil {
		return fmt.Errorf("写入文件失败: %v", err)
	}

	log.Printf("结果已保存到 %s，共 %d 个节点（包括测试失败的，已 Base64 编码）", outputFile, len(nodeLinks))

	// 转换为 ValidNode 格式
	var nodes []*ValidNode
	for _, link := range nodeLinks {
		nodes = append(nodes, &ValidNode{Link: link})
	}

	// 优先推送到 Gist（适用于私有仓库）
	gistID := os.Getenv("GIST_ID")
	gistToken := os.Getenv("GIST_TOKEN")
	if gistID != "" || gistToken != "" {
		log.Printf("准备推送到 Gist (ID: %s)...", gistID)
		if err := c.PushToGist(outputFile, nodes); err != nil {
			log.Printf("❌ 推送到 Gist 失败: %v", err)
			// Gist 推送失败，继续推送到仓库（如果有配置）
		} else {
			log.Printf("✅ Gist 推送成功，本地文件已保存到 %s", outputFile)
			// 注意：即使 Gist 推送成功，本地文件也已经保存，工作流会提交它
			return nil // Gist 推送成功，不再推送到 GitHub API
		}
	}

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

// testSubscribe 测试订阅地址
func testSubscribe(subscribeURL string) {
	log.Printf("\n========== 开始测试订阅地址: %s ==========", subscribeURL)

	collector := NewCollector("")

	// 获取订阅内容
	content, err := collector.FetchSubscription(subscribeURL)
	if err != nil {
		log.Printf("❌ 获取订阅内容失败: %v", err)
		return
	}

	log.Printf("✅ 成功获取订阅内容，长度: %d 字节", len(content))

	// 解析节点
	nodes, err := collector.ParseNodes(content)
	if err != nil {
		log.Printf("⚠️ 解析节点失败: %v，尝试从原始内容提取", err)
		nodes = collector.extractNodesFromRawContent(content)
		if len(nodes) == 0 {
			log.Printf("❌ 无法解析任何节点")
			return
		}
	}

	log.Printf("📊 解析出 %d 个节点", len(nodes))

	// 测试节点
	var wg sync.WaitGroup
	var mu sync.Mutex
	var validNodes []*ValidNode
	var failedNodes []*ValidNode

	maxConcurrency := 10 // 限制并发数，提高速度
	semaphore := make(chan struct{}, maxConcurrency)

	for i, nodeLink := range nodes {
		wg.Add(1)
		go func(index int, link string) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// 使用 sing-box 测试（如果可用）
			var validNode *ValidNode
			useSingBox := os.Getenv("USE_SINGBOX")
			if useSingBox == "false" {
				validNode = collector.TestNode(link)
			} else {
				validNode = collector.TestNodeWithSingBox(link)
			}

			mu.Lock()
			if validNode.Error == nil {
				validNodes = append(validNodes, validNode)
				log.Printf("✅ [%d/%d] 节点可用 - 延迟: %v, 类型: %s",
					index+1, len(nodes), validNode.Latency, validNode.Type)
			} else {
				failedNodes = append(failedNodes, validNode)
				log.Printf("❌ [%d/%d] 节点不可用 - 类型: %s, 错误: %v",
					index+1, len(nodes), validNode.Type, validNode.Error)
			}
			mu.Unlock()
		}(i, nodeLink)
	}

	wg.Wait()

	// 输出统计结果
	log.Printf("\n========== 测试结果统计 ==========")
	log.Printf("订阅地址: %s", subscribeURL)
	log.Printf("总节点数: %d", len(nodes))
	log.Printf("✅ 可用节点: %d", len(validNodes))
	log.Printf("❌ 不可用节点: %d", len(failedNodes))

	if len(validNodes) > 0 {
		log.Printf("\n可用节点列表:")
		for i, node := range validNodes {
			log.Printf("  %d. 类型: %s, 延迟: %v", i+1, node.Type, node.Latency)
		}
	}

	log.Printf("\n")
}

// testSubscribes 测试订阅地址的主函数（独立运行）
func testSubscribes() {
	// 从命令行参数读取订阅地址（从第二个参数开始）
	if len(os.Args) < 3 {
		fmt.Println("用法: go run . test-subscribe <订阅地址1> [订阅地址2] ...")
		fmt.Println("示例: go run . test-subscribe https://example.com/subscribe?token=xxx")
		return
	}

	subscribeURLs := os.Args[2:] // 从第二个参数开始的所有参数都是订阅地址

	for i, url := range subscribeURLs {
		if i > 0 {
			time.Sleep(2 * time.Second) // 间隔一下，避免请求过快
		}
		testSubscribe(url)
	}

	fmt.Println("\n========== 所有测试完成 ==========")
}

func main() {
	// 检查是否是测试订阅地址模式
	if len(os.Args) > 1 && os.Args[1] == "test-subscribe" {
		testSubscribes()
		return
	}

	// 设置 30 分钟超时，增加采集时间
	timeout := 30 * time.Minute
	if timeoutEnv := os.Getenv("COLLECT_TIMEOUT"); timeoutEnv != "" {
		if d, err := time.ParseDuration(timeoutEnv); err == nil && d > 0 {
			timeout = d
		}
	}

	// 创建带超时的上下文
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// 启动超时监控 goroutine
	done := make(chan bool, 1)
	go func() {
		<-ctx.Done()
		if ctx.Err() == context.DeadlineExceeded {
			log.Printf("⏰ 采集超时（%v），停止采集并发布节点", timeout)
			done <- true
		}
	}()

	// 从环境变量获取 GitHub token（可选）
	githubToken := os.Getenv("GITHUB_TOKEN")
	if githubToken == "" {
		log.Println("警告: 未设置 GITHUB_TOKEN，将使用未认证的 API（速率限制较低）")
	}

	collector := NewCollector(githubToken)

	// 在超时前执行采集任务
	go func() {
		defer func() {
			done <- true
		}()

		// 第一步：采集 JMS 节点（生成 nodes.txt）
		log.Println("========== 开始采集 JMS 节点 ==========")
		if err := collector.Collect(); err != nil {
			log.Printf("JMS 节点采集失败: %v", err)
		} else {
			log.Println("========== JMS 节点采集完成 ==========")
		}

		// 第二步（优先）：采集 m7r52rosihxm 链接中的节点（追加到 nodes.txt）
		log.Println("========== 开始采集 m7r52rosihxm 链接节点（优先） ==========")
		if err := collector.CollectM7r52rosihxmNodes(); err != nil {
			log.Printf("m7r52rosihxm 节点采集失败: %v", err)
		} else {
			log.Println("========== m7r52rosihxm 节点采集完成 ==========")
		}

		// 第三步（优先）：采集建森电器链接中的节点（追加到 nodes.txt）
		log.Println("========== 开始采集建森电器链接节点（优先） ==========")
		if err := collector.CollectJiansendianqiNodes(); err != nil {
			log.Printf("建森电器节点采集失败: %v", err)
		} else {
			log.Println("========== 建森电器节点采集完成 ==========")
		}

		// 第四步（优先）：采集 ninjasub 链接中的节点（追加到 nodes.txt）
		log.Println("========== 开始采集 ninjasub 链接节点（优先） ==========")
		if err := collector.CollectNinjasubNodes(); err != nil {
			log.Printf("ninjasub 节点采集失败: %v", err)
		} else {
			log.Println("========== ninjasub 节点采集完成 ==========")
		}

		// 第五步（优先）：采集 xueshan 链接中的节点（追加到 nodes.txt）
		log.Println("========== 开始采集 xueshan 链接节点（优先） ==========")
		if err := collector.CollectXueshanNodes(); err != nil {
			log.Printf("xueshan 节点采集失败: %v", err)
		} else {
			log.Println("========== xueshan 节点采集完成 ==========")
		}

		// 第六步（优先）：采集 nginx24zfd 链接中的节点（追加到 nodes.txt）
		log.Println("========== 开始采集 nginx24zfd 链接节点（优先） ==========")
		if err := collector.CollectNginx24zfdNodes(); err != nil {
			log.Printf("nginx24zfd 节点采集失败: %v", err)
		} else {
			log.Println("========== nginx24zfd 节点采集完成 ==========")
		}

		// 第七步（优先）：采集 iplcme 链接中的节点（追加到 nodes.txt）
		log.Println("========== 开始采集 iplcme 链接节点（优先） ==========")
		if err := collector.CollectIplcmeNodes(); err != nil {
			log.Printf("iplcme 节点采集失败: %v", err)
		} else {
			log.Println("========== iplcme 节点采集完成 ==========")
		}

		// 第八步：采集 smallstrawberry 链接中的节点（追加到 nodes.txt）
		log.Println("========== 开始采集 smallstrawberry 链接节点 ==========")
		if err := collector.CollectSmallstrawberryNodes(); err != nil {
			log.Printf("smallstrawberry 节点采集失败: %v", err)
		} else {
			log.Println("========== smallstrawberry 节点采集完成 ==========")
		}

		// 第九步：采集 ssidwork 链接中的节点（追加到 nodes.txt）
		log.Println("========== 开始采集 ssidwork 链接节点 ==========")
		if err := collector.CollectSsidworkNodes(); err != nil {
			log.Printf("ssidwork 节点采集失败: %v", err)
		} else {
			log.Println("========== ssidwork 节点采集完成 ==========")
		}

		// 第十步：采集 fcsubcn 链接中的节点（追加到 nodes.txt）
		log.Println("========== 开始采集 fcsubcn 链接节点 ==========")
		if err := collector.CollectFcsubcnNodes(); err != nil {
			log.Printf("fcsubcn 节点采集失败: %v", err)
		} else {
			log.Println("========== fcsubcn 节点采集完成 ==========")
		}

		// 第十一步：采集 nn8qozmu 链接中的节点（追加到 nodes.txt）
		log.Println("========== 开始采集 nn8qozmu 链接节点 ==========")
		if err := collector.CollectNn8qozmuNodes(); err != nil {
			log.Printf("nn8qozmu 节点采集失败: %v", err)
		} else {
			log.Println("========== nn8qozmu 节点采集完成 ==========")
		}

		// 第十二步（优先）：采集包含 /api/v1/client/subscribe?token= 的链接中的节点（追加到 nodes.txt）
		log.Println("========== 开始采集订阅 token 链接节点（优先） ==========")
		if err := collector.CollectSubscribeTokenNodes(); err != nil {
			log.Printf("订阅 token 节点采集失败: %v", err)
		} else {
			log.Println("========== 订阅 token 节点采集完成 ==========")
		}

		log.Println("========== 所有采集任务完成 ==========")
	}()

	// 等待采集完成或超时
	select {
	case <-done:
		log.Println("========== 采集任务结束 ==========")
	case <-ctx.Done():
		if ctx.Err() == context.DeadlineExceeded {
			log.Printf("⏰ 采集超时（%v），停止采集", timeout)
		}
	}
}
