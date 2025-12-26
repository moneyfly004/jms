package main

import (
	"log"
	"os"
	"strings"
	"time"
)

func testSingBoxNodes() {
	// 从文件读取节点
	nodesFile := "test_nodes.txt"
	if len(os.Args) > 1 {
		nodesFile = os.Args[1]
	}

	content, err := os.ReadFile(nodesFile)
	if err != nil {
		log.Fatalf("读取文件失败: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	var nodes []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && (strings.HasPrefix(line, "vmess://") || strings.HasPrefix(line, "vless://") || strings.HasPrefix(line, "ss://") || strings.HasPrefix(line, "trojan://")) {
			nodes = append(nodes, line)
		}
	}

	log.Printf("找到 %d 个节点，开始测试...", len(nodes))

	// 创建 Collector
	githubToken := os.Getenv("GITHUB_TOKEN")
	if githubToken == "" {
		githubToken = "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"
	}
	collector := NewCollector(githubToken)

	// 设置使用 sing-box
	os.Setenv("USE_SINGBOX", "true")
	os.Setenv("TEST_TIMEOUT", "20") // 20 秒超时

	successCount := 0
	failCount := 0
	timeoutCount := 0

	startTime := time.Now()

	for i, nodeLink := range nodes {
		log.Printf("\n========== 测试节点 %d/%d ==========", i+1, len(nodes))
		log.Printf("节点链接: %s", nodeLink[:testMin(80, len(nodeLink))])

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
	log.Printf("平均每个节点: %v", totalDuration/time.Duration(len(nodes)))
}

func testMin(a, b int) int {
	if a < b {
		return a
	}
	return b
}

