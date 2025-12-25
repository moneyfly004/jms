package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
)

// CollectGladosNodes 采集 glados 链接中的节点，限制最多200个可用节点
func (c *Collector) CollectGladosNodes() error {
	// 搜索包含 glados 链接的关键词（和 JMS 搜索方式一样）
	keywords := []string{
		"update.glados-config.com",
	}

	// 使用 SearchGitHub 方式搜索，只提取 glados 链接
	gladosLinks, err := c.SearchGladosLinks(keywords)
	if err != nil {
		return fmt.Errorf("搜索 glados 链接失败: %v", err)
	}

	log.Printf("共找到 %d 个 glados 链接，开始采集节点（目标：200个可用节点）", len(gladosLinks))

	// 采集节点
	var allValidNodes []*ValidNode
	var allParsedNodes []string
	seenNodeLinks := make(map[string]bool)
	var wg sync.WaitGroup
	resultsChan := make(chan *NodeResult, len(gladosLinks))
	maxValidNodes := 200 // 限制最多200个可用节点
	var mu sync.Mutex    // 保护 allValidNodes 的并发访问

	// 并发采集
	maxConcurrency := 10
	if maxConcurrencyEnv := os.Getenv("MAX_CONCURRENCY"); maxConcurrencyEnv != "" {
		if n, err := strconv.Atoi(maxConcurrencyEnv); err == nil && n > 0 {
			maxConcurrency = n
		}
	}
	semaphore := make(chan struct{}, maxConcurrency)

	for _, link := range gladosLinks {
		// 检查是否已达到目标节点数
		mu.Lock()
		currentCount := len(allValidNodes)
		mu.Unlock()

		if currentCount >= maxValidNodes {
			log.Printf("已达到目标节点数（%d个），停止采集新的 glados 链接", maxValidNodes)
			break
		}

		wg.Add(1)
		go func(l string) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			result := &NodeResult{Link: l}

			// 再次检查是否已达到目标（避免不必要的请求）
			mu.Lock()
			if len(allValidNodes) >= maxValidNodes {
				mu.Unlock()
				return
			}
			mu.Unlock()

			// 解析 glados 链接
			nodes, err := c.ParseGladosLink(l)
			if err != nil {
				log.Printf("glados 链接 %s 解析失败: %v", l, err)
				result.Error = err
				resultsChan <- result
				return
			}

			result.Nodes = nodes
			log.Printf("glados 链接 %s 解析出 %d 个节点", l, len(nodes))

			// 测试节点
			for _, nodeLink := range nodes {
				// 检查是否已达到目标节点数
				mu.Lock()
				if len(allValidNodes) >= maxValidNodes {
					mu.Unlock()
					break
				}
				mu.Unlock()

				// 全局去重
				mu.Lock()
				if seenNodeLinks[nodeLink] {
					mu.Unlock()
					continue
				}
				seenNodeLinks[nodeLink] = true
				mu.Unlock()

				allParsedNodes = append(allParsedNodes, nodeLink)

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

				if validNode.Error == nil {
					mu.Lock()
					if len(allValidNodes) < maxValidNodes {
						result.ValidNodes = append(result.ValidNodes, validNode)
						allValidNodes = append(allValidNodes, validNode)
						currentCount := len(allValidNodes)
						mu.Unlock()
						log.Printf("✅ 可用节点: %d/%d", currentCount, maxValidNodes)
					} else {
						mu.Unlock()
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
	totalNodes := 0
	totalValidNodes := 0

	for result := range resultsChan {
		if result.Error != nil {
			log.Printf("glados 链接 %s 处理失败: %v", result.Link, result.Error)
		} else {
			totalNodes += len(result.Nodes)
			totalValidNodes += len(result.ValidNodes)

			// 统计节点类型
			for _, nodeLink := range result.Nodes {
				if node, err := ParseNodeLink(nodeLink); err == nil {
					typeStats[node.Type]++
				}
			}

			// 统计测试结果
			for _, validNode := range result.ValidNodes {
				if validNode.Type != "" && validNode.Error == nil {
					validTypeStats[validNode.Type]++
				}
			}

			log.Printf("glados 链接 %s: 共 %d 个节点，%d 个可用",
				result.Link, len(result.Nodes), len(result.ValidNodes))
		}
	}

	// 限制最终节点数量为200个
	finalValidNodes := allValidNodes
	if len(finalValidNodes) > maxValidNodes {
		finalValidNodes = finalValidNodes[:maxValidNodes]
	}

	// 保存结果到 sub.txt（追加模式，因为订阅链接也会写入这个文件）
	if err := c.SaveGladosResults(finalValidNodes); err != nil {
		return fmt.Errorf("保存 glados 节点失败: %v", err)
	}

	// 统计信息
	log.Printf("glados 节点采集完成，共解析 %d 个节点（去重后 %d 个），%d 个可用节点",
		totalNodes, len(allParsedNodes), len(finalValidNodes))

	if len(typeStats) > 0 {
		log.Printf("解析出的节点类型统计:")
		for nodeType, count := range typeStats {
			validCount := validTypeStats[nodeType]
			failCount := count - validCount
			log.Printf("  %s: 共 %d 个 (可用: %d, 失败: %d)", nodeType, count, validCount, failCount)
		}
	}

	return nil
}

// SaveGladosResults 保存 glados 节点结果到 sub.txt（追加模式）
func (c *Collector) SaveGladosResults(nodes []*ValidNode) error {
	// 读取现有的 sub.txt 内容（如果存在）
	existingContent := ""
	if content, err := os.ReadFile("sub.txt"); err == nil {
		existingContent = string(content)
	}

	// 收集新的节点链接
	var newNodes []string
	seenNodes := make(map[string]bool)

	// 先添加现有节点（去重）
	if existingContent != "" {
		lines := strings.Split(strings.TrimSpace(existingContent), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" && !seenNodes[line] {
				seenNodes[line] = true
				newNodes = append(newNodes, line)
			}
		}
	}

	// 添加新的 glados 节点（只添加测试成功且没有超时的节点）
	for _, node := range nodes {
		// 检查节点是否有错误，包括超时错误
		if node.Error != nil {
			// 检查是否是超时错误
			errorStr := node.Error.Error()
			if strings.Contains(errorStr, "timeout") ||
				strings.Contains(errorStr, "deadline exceeded") ||
				strings.Contains(errorStr, "超时") {
				log.Printf("⚠️ 跳过超时节点: %s", node.Link)
				continue
			}
			// 其他错误也跳过
			continue
		}
		
		// 只保存没有错误的节点
		if !seenNodes[node.Link] {
			seenNodes[node.Link] = true
			newNodes = append(newNodes, node.Link)
		}
	}

	// 写入文件
	content := strings.Join(newNodes, "\n")
	if err := os.WriteFile("sub.txt", []byte(content), 0644); err != nil {
		return fmt.Errorf("写入文件失败: %v", err)
	}

	log.Printf("glados 节点已保存到 sub.txt，共 %d 个节点（包含现有节点）", len(newNodes))

	// 推送到 Gist
	subGistID := os.Getenv("SUB_GIST_ID")
	gistToken := os.Getenv("GIST_TOKEN")
	if gistToken == "" {
		gistToken = c.githubToken
	}

	if subGistID != "" && gistToken != "" {
		if err := c.PushToSubGist("sub.txt", nodes, subGistID, gistToken); err != nil {
			log.Printf("推送到 Gist 失败: %v", err)
		}
	}

	return nil
}

