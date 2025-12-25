package main

import (
	"fmt"
	"log"
	"os"
)

func testGlados() {
	githubToken := os.Getenv("GITHUB_TOKEN")
	if githubToken == "" {
		githubToken = "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"
	}

	collector := NewCollector(githubToken)

	// 测试解析 glados clash 链接
	testLink := "https://update.glados-config.com/clash/481162/4e8de2a/195828/glados.yaml"
	
	fmt.Printf("测试解析 glados 链接: %s\n", testLink)
	
	nodes, err := collector.ParseGladosLink(testLink)
	if err != nil {
		log.Fatalf("解析失败: %v", err)
	}

	fmt.Printf("\n成功解析出 %d 个节点:\n\n", len(nodes))
	
	// 显示前10个节点
	maxShow := 10
	if len(nodes) < maxShow {
		maxShow = len(nodes)
	}
	
	for i := 0; i < maxShow; i++ {
		fmt.Printf("%d. %s\n", i+1, nodes[i])
	}
	
	if len(nodes) > maxShow {
		fmt.Printf("\n... 还有 %d 个节点未显示\n", len(nodes)-maxShow)
	}
}

