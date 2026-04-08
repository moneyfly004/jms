package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// PushToGistFinal 统一在采集结束时推送所有结果到 Gist
func (c *Collector) PushToGistFinal() error {
	gistID := os.Getenv("GIST_ID")
	gistToken := os.Getenv("GIST_TOKEN")
	if gistToken == "" {
		gistToken = c.githubToken
	}

	if gistToken == "" {
		c.logger.Warn("未配置 GIST_TOKEN 或 GITHUB_TOKEN，跳过 Gist 推送")
		return nil
	}

	content, err := os.ReadFile("nodes.txt")
	if err != nil {
		c.logger.Error(fmt.Sprintf("读取 nodes.txt 失败: %v", err))
		return err
	}

	contentStr := strings.TrimSpace(string(content))
	var nodeCount int
	decoded, err := base64.StdEncoding.DecodeString(contentStr)
	if err == nil {
		lines := strings.Split(strings.TrimSpace(string(decoded)), "\n")
		nodeCount = len(lines)
		if len(lines) == 1 && lines[0] == "" {
			nodeCount = 0
		}
	} else {
		nodeCount = len(validNodes)
	}

	c.logger.Info(fmt.Sprintf("准备推送 %d 个节点到 Gist", nodeCount))

	files := map[string]interface{}{
		"nodes.txt": map[string]string{
			"content": contentStr,
		},
	}

	payload := map[string]interface{}{
		"description": fmt.Sprintf("JMS 节点列表 - %d 个节点", nodeCount),
		"public":      true,
		"files":       files,
	}

	var apiURL string
	method := "POST"
	if gistID != "" {
		apiURL = fmt.Sprintf("https://api.github.com/gists/%s", gistID)
		method = "PATCH"
	} else {
		apiURL = "https://api.github.com/gists"
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("JSON 编码失败: %v", err)
	}

	req, err := http.NewRequest(method, apiURL, strings.NewReader(string(jsonData)))
	if err != nil {
		return fmt.Errorf("创建请求失败: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+gistToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		c.logger.Error(fmt.Sprintf("Gist API 响应错误: HTTP %d", resp.StatusCode))
		return fmt.Errorf("推送失败 HTTP %d: %s", resp.StatusCode, string(body))
	}

	var gistResponse struct {
		ID    string `json:"id"`
		URL   string `json:"html_url"`
		Files map[string]struct {
			RawURL string `json:"raw_url"`
		} `json:"files"`
	}

	if err := json.Unmarshal(body, &gistResponse); err == nil {
		if gistResponse.ID != "" && gistID == "" {
			c.logger.Success(fmt.Sprintf("Gist 已创建，ID: %s", gistResponse.ID))
		} else if gistID != "" {
			c.logger.Success(fmt.Sprintf("Gist 已更新，ID: %s", gistID))
		}

		if gistResponse.Files != nil && gistResponse.Files["nodes.txt"].RawURL != "" {
			c.logger.Info(fmt.Sprintf("🔗 订阅地址: %s", gistResponse.Files["nodes.txt"].RawURL))
		}
		if gistResponse.URL != "" {
			c.logger.Info(fmt.Sprintf("🌐 Gist 页面: %s", gistResponse.URL))
		}
	}

	c.logger.Success("已成功推送到 GitHub Gist")
	return nil
}
