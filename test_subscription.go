package main

import (
	"log"
	"os"
	"strings"
	"time"
)

func testSubscription() {
	subscriptionURL := "https://msub.xn--m7r52rosihxm.com/api/v1/client/subscribe?token=3bce9b9dae74d7f31709ffbdfdba98ba"
	
	if len(os.Args) > 1 {
		subscriptionURL = os.Args[1]
	}

	log.Printf("========== 开始测试订阅地址 ==========")
	log.Printf("订阅地址: %s", subscriptionURL)

	// 创建 Collector
	githubToken := os.Getenv("GITHUB_TOKEN")
	if githubToken == "" {
		githubToken = "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"
	}
	collector := NewCollector(githubToken)

	// 设置使用 sing-box
	os.Setenv("USE_SINGBOX", "true")
	os.Setenv("TEST_TIMEOUT", "20") // 20 秒超时

	// 获取订阅内容
	log.Printf("正在获取订阅内容...")
	content, err := collector.FetchSubscription(subscriptionURL)
	if err != nil {
		log.Fatalf("获取订阅内容失败: %v", err)
	}

	log.Printf("订阅内容长度: %d 字节", len(content))

	// 解析节点
	log.Printf("正在解析节点...")
	nodes, err := collector.ParseNodes(content)
	if err != nil {
		log.Printf("解析失败: %v，尝试从原始内容提取", err)
		nodes = collector.extractNodesFromRawContent(content)
		if len(nodes) == 0 {
			log.Fatalf("无法解析节点: %v", err)
		}
		log.Printf("从原始内容提取到 %d 个节点", len(nodes))
	} else {
		log.Printf("解析出 %d 个节点", len(nodes))
	}

	if len(nodes) == 0 {
		log.Fatalf("未找到任何节点")
	}

	log.Printf("\n========== 开始测试节点 ==========")

	successCount := 0
	failCount := 0
	timeoutCount := 0

	startTime := time.Now()

	for i, nodeLink := range nodes {
		log.Printf("\n========== 测试节点 %d/%d ==========", i+1, len(nodes))
		nodeLinkShort := nodeLink
		if len(nodeLinkShort) > 80 {
			nodeLinkShort = nodeLinkShort[:80] + "..."
		}
		log.Printf("节点链接: %s", nodeLinkShort)

		testStart := time.Now()
		result := collector.TestNodeWithSingBox(nodeLink)
		testDuration := time.Since(testStart)

		if result.Error != nil {
			failCount++
			errMsg := result.Error.Error()
			if strings.Contains(errMsg, "timeout") || strings.Contains(errMsg, "超时") || testDuration > 20*time.Second {
				timeoutCount++
				log.Printf("❌ 测试超时 (耗时: %v): %v", testDuration, errMsg)
			} else {
				log.Printf("❌ 测试失败 (耗时: %v): %v", testDuration, errMsg)
			}
		} else {
			successCount++
			log.Printf("✅ 测试成功 (耗时: %v, 延迟: %v)", testDuration, result.Latency)
		}
	}

	totalDuration := time.Since(startTime)
	log.Printf("\n========== 测试完成 ==========")
	log.Printf("总节点数: %d", len(nodes))
	log.Printf("成功: %d", successCount)
	log.Printf("失败: %d (其中超时: %d)", failCount, timeoutCount)
	log.Printf("总耗时: %v", totalDuration)
	if len(nodes) > 0 {
		log.Printf("平均每个节点: %v", totalDuration/time.Duration(len(nodes)))
	}
}

