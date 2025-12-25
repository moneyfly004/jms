package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// TestNodeWithSingBox 使用 sing-box 测试节点（真实链接测速）
func (c *Collector) TestNodeWithSingBox(nodeLink string) *ValidNode {
	result := &ValidNode{
		Link: nodeLink,
	}

	// 解析节点
	node, err := ParseNodeLink(nodeLink)
	if err != nil {
		result.Error = fmt.Errorf("解析失败: %v", err)
		return result
	}

	result.Type = node.Type

	// 检查是否启用 sing-box 测试
	useSingBox := os.Getenv("USE_SINGBOX") == "true"
	if !useSingBox {
		// 如果未启用，回退到 TCP 测试
		return c.TestNode(nodeLink)
	}

	// 检查 sing-box 是否可用
	singBoxPath := os.Getenv("SINGBOX_PATH")
	if singBoxPath == "" {
		singBoxPath = "sing-box" // 默认路径
	}

	// 检查 sing-box 是否存在
	if _, err := exec.LookPath(singBoxPath); err != nil {
		log.Printf("⚠️ sing-box 未找到，回退到 TCP 测试")
		return c.TestNode(nodeLink)
	}

	// 创建临时配置文件
	configFile, err := c.createSingBoxConfig(nodeLink)
	if err != nil {
		log.Printf("创建 sing-box 配置失败: %v，回退到 TCP 测试", err)
		return c.TestNode(nodeLink)
	}
	defer os.Remove(configFile)

	// 使用 sing-box 测试延迟
	latency, err := c.testLatencyWithSingBox(singBoxPath, configFile)
	if err != nil {
		result.Error = fmt.Errorf("sing-box 测试失败: %v", err)
		return result
	}

	result.Latency = latency

	// 可选：进行速度测试
	if os.Getenv("TEST_SPEED") == "true" {
		speed, err := c.testSpeedWithSingBox(singBoxPath, configFile)
		if err == nil {
			log.Printf("节点 %s 速度: %.2f MB/s", nodeLink[:50], speed)
		}
	}

	return result
}

// createSingBoxConfig 创建 sing-box 配置文件
func (c *Collector) createSingBoxConfig(nodeLink string) (string, error) {
	// 解析节点链接
	node, err := ParseNodeLink(nodeLink)
	if err != nil {
		return "", err
	}

	// 创建临时目录
	tmpDir := os.TempDir()
	configFile := filepath.Join(tmpDir, fmt.Sprintf("singbox_%d.json", time.Now().UnixNano()))

	// 根据节点类型创建配置
	var outbound map[string]interface{}

	switch node.Type {
	case "ss":
		outbound = map[string]interface{}{
			"type":        "shadowsocks",
			"server":      node.Server,
			"server_port": node.Port,
			"method":      node.Cipher,
			"password":    node.Password,
		}
	case "ssr":
		// SSR 在 sing-box 中需要转换为 shadowsocks
		// 注意：sing-box 不完全支持 SSR，这里使用基本配置
		outbound = map[string]interface{}{
			"type":        "shadowsocks",
			"server":      node.Server,
			"server_port": node.Port,
			"method":      node.Cipher,
			"password":    node.Password,
		}
		log.Printf("⚠️ SSR 节点 %s 转换为 Shadowsocks 配置（sing-box 不完全支持 SSR）", node.Name)
	case "vmess":
		outbound = map[string]interface{}{
			"type":        "vmess",
			"server":      node.Server,
			"server_port": node.Port,
			"uuid":        node.UUID,
			"security":    func() string {
				if node.Security != "" {
					return node.Security
				}
				return "auto"
			}(),
			"alter_id": func() int {
				if node.AlterID > 0 {
					return node.AlterID
				}
				return 0
			}(),
		}
		if node.Network != "" && node.Network != "tcp" {
			outbound["network"] = node.Network
		}
		if node.TLS {
			tlsConfig := map[string]interface{}{
				"enabled": true,
			}
			if node.SNI != "" {
				tlsConfig["server_name"] = node.SNI
			}
			if node.ALPN != "" {
				tlsConfig["alpn"] = []string{node.ALPN}
			}
			outbound["tls"] = tlsConfig
		}
	case "vless":
		outbound = map[string]interface{}{
			"type":        "vless",
			"server":      node.Server,
			"server_port": node.Port,
			"uuid":        node.UUID,
		}
		if node.Flow != "" {
			outbound["flow"] = node.Flow
		}
		if node.Network != "" && node.Network != "tcp" {
			outbound["network"] = node.Network
		}
		if node.TLS {
			tlsConfig := map[string]interface{}{
				"enabled": true,
			}
			if node.SNI != "" {
				tlsConfig["server_name"] = node.SNI
			}
			if node.ALPN != "" {
				tlsConfig["alpn"] = []string{node.ALPN}
			}
			outbound["tls"] = tlsConfig
		}
	case "trojan":
		outbound = map[string]interface{}{
			"type":        "trojan",
			"server":      node.Server,
			"server_port": node.Port,
			"password":    node.Password,
		}
		if node.TLS {
			tlsConfig := map[string]interface{}{
				"enabled": true,
			}
			if node.SNI != "" {
				tlsConfig["server_name"] = node.SNI
			}
			if node.ALPN != "" {
				tlsConfig["alpn"] = []string{node.ALPN}
			}
			outbound["tls"] = tlsConfig
		}
	case "hysteria":
		outbound = map[string]interface{}{
			"type":        "hysteria",
			"server":      node.Server,
			"server_port": node.Port,
		}
		if node.Auth != "" {
			outbound["auth"] = node.Auth
		}
		if node.ObfsPassword != "" {
			outbound["obfs"] = node.ObfsPassword
		}
	case "wireguard":
		outbound = map[string]interface{}{
			"type":        "wireguard",
			"server":      node.Server,
			"server_port": node.Port,
			"private_key": node.PrivateKey,
		}
		if node.PublicKey != "" {
			outbound["peer_public_key"] = node.PublicKey
		}
		if node.Reserved != "" {
			// Reserved 通常是 base64 编码的 3 个字节
			outbound["reserved"] = node.Reserved
		}
	case "tuic":
		outbound = map[string]interface{}{
			"type":        "tuic",
			"server":      node.Server,
			"server_port": node.Port,
			"uuid":        node.UUID,
			"password":    node.Password,
		}
		if node.Token != "" {
			outbound["token"] = node.Token
		}
	default:
		return "", fmt.Errorf("不支持的节点类型: %s", node.Type)
	}

	// 创建完整配置
	config := map[string]interface{}{
		"log": map[string]interface{}{
			"level": "error",
		},
		"inbounds": []map[string]interface{}{
			{
				"type":        "mixed",
				"listen":      "127.0.0.1",
				"listen_port": 0, // 自动分配端口
			},
		},
		"outbounds": []map[string]interface{}{
			outbound,
		},
		"route": map[string]interface{}{
			"rules": []map[string]interface{}{
				{
					"outbound": "proxy",
					"default":  true,
				},
			},
		},
	}

	// 写入配置文件
	configJSON, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(configFile, configJSON, 0644); err != nil {
		return "", err
	}

	return configFile, nil
}

// testLatencyWithSingBox 使用 sing-box 测试延迟（真实链接测速）
func (c *Collector) testLatencyWithSingBox(singBoxPath, configFile string) (time.Duration, error) {
	// 读取配置文件获取服务器信息
	configData, err := os.ReadFile(configFile)
	if err != nil {
		return 0, err
	}

	var config map[string]interface{}
	if err := json.Unmarshal(configData, &config); err != nil {
		return 0, err
	}

	// 获取 outbound 配置
	outbounds, ok := config["outbounds"].([]interface{})
	if !ok || len(outbounds) == 0 {
		return 0, fmt.Errorf("无效的配置")
	}

	// 使用 sing-box check 验证配置
	cmd := exec.Command(singBoxPath, "check", "-c", configFile)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("sing-box check 失败: %v, 输出: %s", err, string(output))
	}

	// 修改配置，添加一个测试用的 inbound（HTTP 代理）
	// 获取一个随机端口
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("无法分配端口: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	// 更新配置中的 inbound 端口
	inbounds := config["inbounds"].([]interface{})
	if len(inbounds) > 0 {
		inbound := inbounds[0].(map[string]interface{})
		inbound["listen_port"] = port
	}

	// 保存更新后的配置
	updatedConfigJSON, err := json.Marshal(config)
	if err != nil {
		return 0, err
	}
	if err := os.WriteFile(configFile, updatedConfigJSON, 0644); err != nil {
		return 0, err
	}

	// 启动 sing-box 进行真实连接测试
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd = exec.CommandContext(ctx, singBoxPath, "run", "-c", configFile)
	cmd.Stdout = os.Stderr // 重定向输出到 stderr
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("启动 sing-box 失败: %v", err)
	}
	defer cmd.Process.Kill()

	// 等待 sing-box 启动
	time.Sleep(1 * time.Second)

	// 通过代理测试真实连接
	testURL := os.Getenv("TEST_URL")
	if testURL == "" {
		testURL = "http://www.google.com/generate_204"
	}

	start := time.Now()
	proxyURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	// 创建 HTTP 客户端，使用代理
	proxyFunc := func(_ *http.Request) (*url.URL, error) {
		return url.Parse(proxyURL)
	}
	transport := &http.Transport{
		Proxy: proxyFunc,
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   5 * time.Second,
	}

	req, err := http.NewRequest("GET", testURL, nil)
	if err != nil {
		return 0, fmt.Errorf("创建请求失败: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("代理连接失败: %v", err)
	}
	defer resp.Body.Close()

	latency := time.Since(start)

	// 如果状态码是 200 或 204，说明连接成功
	if resp.StatusCode == 200 || resp.StatusCode == 204 {
		return latency, nil
	}

	return latency, fmt.Errorf("HTTP 状态码: %d", resp.StatusCode)
}

// testSpeedWithSingBox 使用 sing-box 测试速度
func (c *Collector) testSpeedWithSingBox(singBoxPath, configFile string) (float64, error) {
	// 这里可以实现速度测试逻辑
	// 例如下载一个测试文件并计算速度
	// 由于实现较复杂，这里返回 0
	return 0, nil
}
