package main

import (
	"fmt"
	"os"
)

func testSingBoxNodes() {
	// 测试节点列表
	testNodes := []string{
		"ss://YWVzLTI1Ni1nY206dXFlUmZNOXNnbTdvc05USw%3D%3D@104.160.42.197:22008#JMS-671458%40c67s1.portablesubmarines.com%3A22008",
		"ss://YWVzLTI1Ni1nY206dXFlUmZNOXNnbTdvc05USw%3D%3D@104.160.46.246:22008#JMS-671458%40c67s2.portablesubmarines.com%3A22008",
		"vmess://ew0KICAidiI6ICIyIiwNCiAgInBzIjogIkpNUy02NzE0NThAYzY3czMucG9ydGFibGVzdWJtYXJpbmVzLmNvbToyMjAwOCIsDQogICJhZGQiOiAiMTk4LjE4MS40My4xMjMiLA0KICAicG9ydCI6ICIyMjAwOCIsDQogICJpZCI6ICI2MmQ2YjEyNC02NDI3LTQ0ZDUtODQwYS1hM2I3YTQ1OWVlYjAiLA0KICAiYWlkIjogIjAiLA0KICAic2N5IjogImF1dG8iLA0KICAibmV0IjogInRjcCIsDQogICJ0eXBlIjogIm5vbmUiLA0KICAiaG9zdCI6ICIiLA0KICAicGF0aCI6ICIiLA0KICAidGxzIjogIiIsDQogICJzbmkiOiAiIiwNCiAgImFscG4iOiAiIiwNCiAgImZwIjogIiINCn0=",
		"ss://YWVzLTI1Ni1nY206YzVUYzNGN1c0NFdjQmRFRA%3D%3D@96.45.188.33:15130#JMS-1268850%40c83s1.portablesubmarines.com%3A15130",
		"ss://YWVzLTI1Ni1nY206YzVUYzNGN1c0NFdjQmRFRA%3D%3D@104.160.44.223:15130#JMS-1268850%40c83s2.portablesubmarines.com%3A15130",
		"ss://YWVzLTI1Ni1nY206NTdkdG96YmRRUlpUOU1hTA%3D%3D@c60s1.portablesubmarines.com:26351#JMS-388138%40c60s1.portablesubmarines.com%3A26351",
		"vmess://ew0KICAidiI6ICIyIiwNCiAgInBzIjogIkpNUy0xMjY4ODUwQGM4M3MzLnBvcnRhYmxlc3VibWFyaW5lcy5jb206MTUxMzAiLA0KICAiYWRkIjogIjE5OC4zNS40Ny4yNyIsDQogICJwb3J0IjogIjE1MTMwIiwNCiAgImlkIjogImRkZjg4ZjQxLWMyM2UtNDZhMC04MGZhLTA2MmJiOTBiYzg0OCIsDQogICJhaWQiOiAiMCIsDQogICJzY3kiOiAiYXV0byIsDQogICJuZXQiOiAidGNwIiwNCiAgInR5cGUiOiAibm9uZSIsDQogICJob3N0IjogIiIsDQogICJwYXRoIjogIiIsDQogICJ0bHMiOiAiIiwNCiAgInNuaSI6ICIiLA0KICAiYWxwbiI6ICIiLA0KICAiZnAiOiAiIg0KfQ==",
		"vmess://ew0KICAidiI6ICIyIiwNCiAgInBzIjogIkpNUy02NzE0NThAYzY3czQucG9ydGFibGVzdWJtYXJpbmVzLmNvbToyMjAwOCIsDQogICJhZGQiOiAiMTc4LjE1Ny41Ny40IiwNCiAgInBvcnQiOiAiMjIwMDgiLA0KICAiaWQiOiAiNjJkNmIxMjQtNjQyNy00NGQ1LTg0MGEtYTNiN2E0NTllZWIwIiwNCiAgImFpZCI6ICIwIiwNCiAgInNjeSI6ICJhdXRvIiwNCiAgIm5ldCI6ICJ0Y3AiLA0KICAidHlwZSI6ICJub25lIiwNCiAgImhvc3QiOiAiIiwNCiAgInBhdGgiOiAiIiwNCiAgInRscyI6ICIiLA0KICAic25pIjogIiIsDQogICJhbHBuIjogIiIsDQogICJmcCI6ICIiDQp9",
		"ss://YWVzLTI1Ni1nY206NTdkdG96YmRRUlpUOU1hTA%3D%3D@c60s2.portablesubmarines.com:26351#JMS-388138%40c60s2.portablesubmarines.com%3A26351",
		"vmess://ew0KICAidiI6ICIyIiwNCiAgInBzIjogIkpNUy0xMjY4ODUwQGM4M3M0LnBvcnRhYmxlc3VibWFyaW5lcy5jb206MTUxMzAiLA0KICAiYWRkIjogIjIxMi41MC4yNDQuMjEzIiwNCiAgInBvcnQiOiAiMTUxMzAiLA0KICAiaWQiOiAiZGRmODhmNDEtYzIzZS00NmEwLTgwZmEtMDYyYmI5MGJjODQ4IiwNCiAgImFpZCI6ICIwIiwNCiAgInNjeSI6ICJhdXRvIiwNCiAgIm5ldCI6ICJ0Y3AiLA0KICAidHlwZSI6ICJub25lIiwNCiAgImhvc3QiOiAiIiwNCiAgInBhdGgiOiAiIiwNCiAgInRscyI6ICIiLA0KICAic25pIjogIiIsDQogICJhbHBuIjogIiIsDQogICJmcCI6ICIiDQp9",
	}

	// 设置环境变量启用 sing-box
	os.Setenv("USE_SINGBOX", "true")
	os.Setenv("TEST_TIMEOUT", "15")
	os.Setenv("TEST_URL", "http://www.google.com/generate_204")

	// 创建采集器
	collector := NewCollector("")

	fmt.Println("🧪 开始使用 sing-box 测试节点...\n")
	fmt.Printf("共 %d 个节点需要测试\n\n", len(testNodes))

	successCount := 0
	failCount := 0

	for i, nodeLink := range testNodes {
		fmt.Printf("[%d/%d] 测试节点: %s\n", i+1, len(testNodes), nodeLink[:min(60, len(nodeLink))])
		
		// 解析节点
		node, err := ParseNodeLink(nodeLink)
		if err != nil {
			fmt.Printf("  ❌ 解析失败: %v\n\n", err)
			failCount++
			continue
		}
		fmt.Printf("  📋 类型: %s, 服务器: %s:%d\n", node.Type, node.Server, node.Port)

		// 使用 sing-box 测试
		result := collector.TestNodeWithSingBox(nodeLink)
		
		if result.Error == nil {
			fmt.Printf("  ✅ 测试成功！延迟: %v\n\n", result.Latency)
			successCount++
		} else {
			fmt.Printf("  ❌ 测试失败: %v\n\n", result.Error)
			failCount++
		}
	}

	fmt.Println("=" + string(make([]byte, 50)) + "=")
	fmt.Printf("📊 测试结果统计:\n")
	fmt.Printf("  ✅ 成功: %d 个\n", successCount)
	fmt.Printf("  ❌ 失败: %d 个\n", failCount)
	fmt.Printf("  📈 成功率: %.1f%%\n", float64(successCount)/float64(len(testNodes))*100)
}

