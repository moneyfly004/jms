# GlaDOS 关键词采集功能添加说明

## 修改日期
2026-03-11

## 修改内容

已成功将 `update.glados-config.com` 关键词添加到采集程序中，并设置为最优先采集。

### 1. 添加正则表达式模式 (第 54-55 行)

```go
// 匹配 update.glados-config.com 链接的正则表达式（优先）
gladosLinkPattern = regexp.MustCompile(`https?://update\.glados-config\.com[^\s"']*`)
```

### 2. 添加链接提取函数 (第 766-786 行)

```go
// extractGladosLinks 提取 update.glados-config.com 链接
func (c *Collector) extractGladosLinks(content string) []string {
	var links []string
	seenLinks := make(map[string]bool)

	matches := gladosLinkPattern.FindAllString(content, -1)
	for _, match := range matches {
		link := strings.TrimSpace(match)
		link = strings.TrimRight(link, ".,;!?)")
		link = strings.TrimRight(link, "\"')")

		if link != "" &&
			strings.Contains(link, "update.glados-config.com") &&
			!seenLinks[link] {
			seenLinks[link] = true
			links = append(links, link)
		}
	}

	return links
}
```

### 3. 添加搜索函数 (第 743-783 行)

```go
// SearchGladosLinks 搜索 update.glados-config.com 链接
func (c *Collector) SearchGladosLinks(keywords []string) ([]string, error) {
	var allLinks []string
	seenLinks := make(map[string]bool)

	for _, keyword := range keywords {
		log.Printf("正在搜索 GlaDOS 关键词: %s", keyword)

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

			links := c.extractGladosLinks(fileContent)
			for _, link := range links {
				if !seenLinks[link] {
					seenLinks[link] = true
					allLinks = append(allLinks, link)
					log.Printf("发现新 GlaDOS 链接: %s", link)
				}
			}
		}

		time.Sleep(2 * time.Second)
	}

	return allLinks, nil
}
```

### 4. 添加节点采集函数 (第 2095-2102 行)

```go
// CollectGladosNodes 采集 update.glados-config.com 链接中的节点
func (c *Collector) CollectGladosNodes() error {
	keywords := []string{
		"update.glados-config.com",
		"glados-config.com",
	}
	return c.collectNodesGeneric(c.SearchGladosLinks, "GlaDOS", keywords)
}
```

### 5. 在 main 函数中添加采集步骤 (第 2691-2697 行)

将 GlaDOS 采集设置为**第二步（最优先）**，仅次于 JMS 节点采集：

```go
// 第二步（最优先）：采集 GlaDOS 链接中的节点（追加到 nodes.txt）
log.Println("========== 开始采集 GlaDOS 链接节点（最优先） ==========")
if err := collector.CollectGladosNodes(); err != nil {
	log.Printf("GlaDOS 节点采集失败: %v", err)
} else {
	log.Println("========== GlaDOS 节点采集完成 ==========")
}
```

## 采集优先级

修改后的采集顺序：

1. JMS 节点
2. **GlaDOS 链接节点（最优先）** ⭐ 新增
3. m7r52rosihxm 链接节点（优先）
4. 建森电器链接节点（优先）
5. ninjasub 链接节点（优先）
6. nginx24zfd 链接节点（优先）
7. iplcme 链接节点（优先）
8. smallstrawberry 链接节点
9. ssidwork 链接节点
10. fcsubcn 链接节点
11. nn8qozmu 链接节点
12. 订阅 token 链接节点（优先）

## 搜索关键词

程序会使用以下关键词在 GitHub 上搜索 GlaDOS 订阅链接：

- `update.glados-config.com`
- `glados-config.com`

## 功能说明

采集流程与其他关键词（如 m7r52rosihxm、建森电器等）完全一致：

1. **GitHub 搜索**: 使用 GitHub Code Search API 搜索包含关键词 `update.glados-config.com` 的代码文件
2. **获取文件内容**: 从搜索结果中获取每个文件的内容
3. **链接提取**: 使用正则表达式从文件内容中提取所有 `update.glados-config.com` 开头的链接
4. **节点采集**: 访问提取到的链接并获取其中的代理节点配置
5. **去重处理**: 自动去除重复的链接和节点
6. **优先级最高**: GlaDOS 节点会在其他大部分节点之前采集（第二步）

## 测试建议

运行程序后，查看日志输出中是否有：

```
========== 开始采集 GlaDOS 链接节点（最优先） ==========
正在搜索 GlaDOS 关键词: update.glados-config.com
找到 X 个结果
发现新 GlaDOS 链接: https://update.glados-config.com/...
========== GlaDOS 节点采集完成 ==========
```

## 注意事项

1. 需要设置 `GITHUB_TOKEN` 环境变量以提高 API 速率限制
2. GlaDOS 订阅通常每个链接只包含 1 个节点
3. 采集过程会自动处理 Clash YAML 格式的配置文件
