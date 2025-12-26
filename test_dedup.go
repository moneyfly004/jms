package main

import (
	"log"
)

func testDedup() {
	node1 := "vmess://ew0KICAidiI6ICIyIiwNCiAgInBzIjogIlx1NjVCMFx1NTJBMFx1NTc2MS1cdTRGMThcdTUzMTYtR2VtaW5pLUdQVCIsDQogICJhZGQiOiAicGxhbmIubW9qY24uY29tIiwNCiAgInBvcnQiOiAiMTY2MTgiLA0KICAiaWQiOiAiMmU1N2E1MjgtOWQ5NS00NzFjLWE0NDAtZGRjYzM3NmE2MzI5IiwNCiAgImFpZCI6ICIwIiwNCiAgInNjeSI6ICJhdXRvIiwNCiAgIm5ldCI6ICJ3cyIsDQogICJ0eXBlIjogIm5vbmUiLA0KICAiaG9zdCI6ICIxM2ZkNTcxNWIyZjRkOGEyMjhkNjRiNTNjMGNjYTRlMy5tb2Jnc2xiLnRiY2FjaGUuY29tIiwNCiAgInBhdGgiOiAiLyIsDQogICJ0bHMiOiAiIiwNCiAgInNuaSI6ICIiLA0KICAiYWxwbiI6ICIiLA0KICAiZnAiOiAiIg0KfQ=="
	node2 := "vmess://ew0KICAidiI6ICIyIiwNCiAgInBzIjogIlx1NjVCMFx1NTJBMFx1NTc2MS1cdTRGMThcdTUzMTYtR2VtaW5pLUdQVCIsDQogICJhZGQiOiAicGxhbmIubW9qY24uY29tIiwNCiAgInBvcnQiOiAiMTY2MTgiLA0KICAiaWQiOiAiZDY3MGIyY2EtNGRjNC00OWFiLTkzY2UtNGM5NWJkY2VkZDE1IiwNCiAgImFpZCI6ICIwIiwNCiAgInNjeSI6ICJhdXRvIiwNCiAgIm5ldCI6ICJ3cyIsDQogICJ0eXBlIjogIm5vbmUiLA0KICAiaG9zdCI6ICIzNDBmMjVmNWI0NWQzYWFmY2U5YmEzOWRlM2ZlODcxZS5tb2Jnc2xiLnRiY2FjaGUuY29tIiwNCiAgInBhdGgiOiAiLyIsDQogICJ0bHMiOiAiIiwNCiAgInNuaSI6ICIiLA0KICAiYWxwbiI6ICIiLA0KICAiZnAiOiAiIg0KfQ=="

	log.Println("========== 测试去重逻辑 ==========")
	log.Printf("节点1链接: %s", node1)
	log.Printf("节点2链接: %s", node2)
	log.Printf("节点1 == 节点2: %v", node1 == node2)

	// 解析节点查看 UUID
	node1Parsed, err1 := ParseNodeLink(node1)
	if err1 != nil {
		log.Fatalf("解析节点1失败: %v", err1)
	}

	node2Parsed, err2 := ParseNodeLink(node2)
	if err2 != nil {
		log.Fatalf("解析节点2失败: %v", err2)
	}

	log.Printf("\n节点1信息:")
	log.Printf("  UUID: %s", node1Parsed.UUID)
	log.Printf("  名称: %s", node1Parsed.Name)
	log.Printf("  服务器: %s:%d", node1Parsed.Server, node1Parsed.Port)

	log.Printf("\n节点2信息:")
	log.Printf("  UUID: %s", node2Parsed.UUID)
	log.Printf("  名称: %s", node2Parsed.Name)
	log.Printf("  服务器: %s:%d", node2Parsed.Server, node2Parsed.Port)

	log.Printf("\nUUID 相同: %v", node1Parsed.UUID == node2Parsed.UUID)
	log.Printf("名称相同: %v", node1Parsed.Name == node2Parsed.Name)

	// 模拟去重逻辑
	seenNodes := make(map[string]bool)
	var allNodes []string

	// 添加节点1
	if !seenNodes[node1] {
		seenNodes[node1] = true
		allNodes = append(allNodes, node1)
		log.Printf("\n✅ 节点1已添加（去重后）")
	}

	// 尝试添加节点2
	if !seenNodes[node2] {
		seenNodes[node2] = true
		allNodes = append(allNodes, node2)
		log.Printf("✅ 节点2已添加（去重后）")
	} else {
		log.Printf("❌ 节点2被判定为重复，未添加")
	}

	log.Printf("\n最终节点数: %d", len(allNodes))
	log.Printf("结论: 去重逻辑基于完整节点链接，UUID不同的节点不会被判定为重复 ✅")
}

