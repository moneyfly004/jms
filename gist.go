package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

// PushToGist 推送节点到 GitHub Gist
func (c *Collector) PushToGist(filePath string, nodes []*ValidNode) error {
	gistID := os.Getenv("GIST_ID")
	gistToken := os.Getenv("GIST_TOKEN")
	
	if gistToken == "" {
		// 如果没有设置 GIST_TOKEN，使用 GITHUB_TOKEN
		gistToken = c.githubToken
	}
	
	if gistToken == "" {
		return fmt.Errorf("需要 GIST_TOKEN 或 GITHUB_TOKEN 才能推送到 Gist")
	}

	// 读取文件内容
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("读取文件失败: %v", err)
	}
	
	// 验证文件内容
	contentStr := string(content)
	fileNodeCount := len(strings.Split(strings.TrimSpace(contentStr), "\n"))
	if strings.TrimSpace(contentStr) == "" {
		fileNodeCount = 0
	}
	
	log.Printf("📄 读取文件 %s，包含 %d 行节点", filePath, fileNodeCount)
	log.Printf("📊 准备推送 %d 个节点到 Gist", len(nodes))
	
	// 验证节点数量是否一致
	if fileNodeCount != len(nodes) {
		log.Printf("⚠️ 警告：文件节点数 (%d) 与节点数组数 (%d) 不一致，使用文件内容", fileNodeCount, len(nodes))
	}

	// 准备 Gist 内容
	files := map[string]interface{}{
		"nodes.txt": map[string]string{
			"content": contentStr,
		},
	}

	payload := map[string]interface{}{
		"description": fmt.Sprintf("JMS 节点列表 - %d 个节点", len(nodes)),
		"public":      true,
		"files":       files,
	}

	var apiURL string
	if gistID == "" {
		// 创建新的 Gist
		apiURL = "https://api.github.com/gists"
	} else {
		// 更新现有的 Gist
		apiURL = fmt.Sprintf("https://api.github.com/gists/%s", gistID)
		payload["files"] = files
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("JSON 编码失败: %v", err)
	}

	// 创建请求
	method := "POST"
	if gistID != "" {
		method = "PATCH"
	}

	req, err := http.NewRequest(method, apiURL, strings.NewReader(string(jsonData)))
	if err != nil {
		return fmt.Errorf("创建请求失败: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+gistToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	// 发送请求
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		log.Printf("❌ Gist API 响应错误: HTTP %d", resp.StatusCode)
		log.Printf("响应内容: %s", string(body))
		return fmt.Errorf("推送失败 HTTP %d: %s", resp.StatusCode, string(body))
	}

	// 解析响应获取 Gist ID
	var gistResponse struct {
		ID  string `json:"id"`
		URL string `json:"html_url"`
		Files map[string]struct {
			RawURL string `json:"raw_url"`
		} `json:"files"`
	}

	if err := json.Unmarshal(body, &gistResponse); err == nil {
		if gistResponse.ID != "" && gistID == "" {
			log.Printf("✅ Gist 已创建，ID: %s", gistResponse.ID)
			log.Printf("📝 请设置环境变量 GIST_ID=%s 以便后续更新", gistResponse.ID)
		} else if gistID != "" {
			log.Printf("✅ Gist 已更新，ID: %s", gistID)
		}
		
		if gistResponse.Files != nil && gistResponse.Files["nodes.txt"].RawURL != "" {
			log.Printf("🔗 订阅地址: %s", gistResponse.Files["nodes.txt"].RawURL)
		}
		
		if gistResponse.URL != "" {
			log.Printf("🌐 Gist 页面: %s", gistResponse.URL)
		}
		
		// 验证推送的节点数量
		if len(nodes) > 0 {
			log.Printf("📊 已推送 %d 个节点到 Gist", len(nodes))
		}
	} else {
		log.Printf("⚠️ 无法解析 Gist 响应，但推送可能已成功")
	}

	log.Printf("✅ 已成功推送到 GitHub Gist")
	return nil
}

