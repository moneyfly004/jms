package main

import (
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

// SubscribeServer 订阅服务器
type SubscribeServer struct {
	port     string
	nodesFile string
}

// NewSubscribeServer 创建订阅服务器
func NewSubscribeServer(port string) *SubscribeServer {
	return &SubscribeServer{
		port:      port,
		nodesFile: "nodes.txt",
	}
}

// handleSubscribe 处理订阅请求
func (s *SubscribeServer) handleSubscribe(w http.ResponseWriter, r *http.Request) {
	// 读取节点文件
	content, err := os.ReadFile(s.nodesFile)
	if err != nil {
		http.Error(w, fmt.Sprintf("读取节点文件失败: %v", err), http.StatusInternalServerError)
		return
	}

	// 按行分割节点
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	var validNodes []string
	
	// 过滤空行
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && (strings.HasPrefix(line, "ss://") ||
			strings.HasPrefix(line, "vmess://") ||
			strings.HasPrefix(line, "vless://") ||
			strings.HasPrefix(line, "trojan://")) {
			validNodes = append(validNodes, line)
		}
	}

	// 合并为字符串
	nodesContent := strings.Join(validNodes, "\n")
	
	// Base64 编码
	encoded := base64.StdEncoding.EncodeToString([]byte(nodesContent))

	// 设置响应头
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=subscription.txt")
	
	// 返回 base64 编码的内容
	io.WriteString(w, encoded)
	
	log.Printf("订阅请求来自 %s，返回 %d 个节点", r.RemoteAddr, len(validNodes))
}

// Start 启动订阅服务器
func (s *SubscribeServer) Start() error {
	http.HandleFunc("/subscribe", s.handleSubscribe)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		html := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>JMS 节点订阅</title>
</head>
<body>
    <h1>JMS 节点订阅服务</h1>
    <p>订阅链接：<code>%s/subscribe</code></p>
    <p>当前节点数量：请查看订阅内容</p>
    <p><a href="/subscribe">直接下载订阅文件</a></p>
</body>
</html>
		`, "http://"+r.Host)
		io.WriteString(w, html)
	})

	addr := ":" + s.port
	log.Printf("订阅服务器启动在端口 %s", s.port)
	log.Printf("订阅链接: http://localhost%s/subscribe", addr)
	return http.ListenAndServe(addr, nil)
}

// StartSubscribeServer 启动订阅服务器（如果启用）
func StartSubscribeServer() {
	port := os.Getenv("SUBSCRIBE_PORT")
	if port == "" {
		return // 未启用订阅服务器
	}

	server := NewSubscribeServer(port)
	go func() {
		if err := server.Start(); err != nil {
			log.Printf("订阅服务器启动失败: %v", err)
		}
	}()
}

