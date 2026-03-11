package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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

	// 检查是否禁用 sing-box 测试（默认启用）
	useSingBox := os.Getenv("USE_SINGBOX")
	if useSingBox == "false" {
		// 如果明确禁用，回退到 TCP 测试
		return c.TestNode(nodeLink)
	}

	// 查找 sing-box 可执行文件
	singBoxPath := os.Getenv("SINGBOX_PATH")
	if singBoxPath == "" {
		// 优先使用本地目录中的 sing-box
		localPaths := []string{
			"./sing-box-1.10.0-alpha.29-darwin-amd64/sing-box", // macOS arm64
			"./sing-box-1.10.0-alpha.29-linux-amd64/sing-box",  // Linux amd64
			"./sing-box-1.10.0-alpha.29-darwin-arm64/sing-box", // macOS arm64
			"./sing-box", // 当前目录
			"sing-box",   // PATH 中
		}

		for _, path := range localPaths {
			if _, err := os.Stat(path); err == nil {
				singBoxPath = path
				break
			}
		}

		// 如果本地文件都不存在，尝试从 PATH 查找
		if singBoxPath == "" {
			if path, err := exec.LookPath("sing-box"); err == nil {
				singBoxPath = path
			}
		}
	}

	// 检查 sing-box 是否存在
	if singBoxPath == "" {
		return &ValidNode{
			Link:  nodeLink,
			Type:  node.Type,
			Error: fmt.Errorf("sing-box 未找到，请确保 sing-box 内核文件在项目目录中"),
		}
	}

	// 检查文件是否可执行
	if _, err := os.Stat(singBoxPath); os.IsNotExist(err) {
		return &ValidNode{
			Link:  nodeLink,
			Type:  node.Type,
			Error: fmt.Errorf("sing-box 文件不存在: %s", singBoxPath),
		}
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
		// 记录详细错误信息用于调试
		nodeLinkShort := nodeLink
		if len(nodeLinkShort) > 50 {
			nodeLinkShort = nodeLinkShort[:50]
		}
		log.Printf("⚠️ 节点 %s sing-box 测试失败: %v", nodeLinkShort, err)
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
			"security": func() string {
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
		// VMess 传输方式配置
		if node.Network != "" && node.Network != "tcp" {
			transportConfig := map[string]interface{}{}
			switch node.Network {
			case "ws":
				transportConfig["type"] = "ws"
				if node.WSPath != "" {
					transportConfig["path"] = node.WSPath
				}
				if node.WSHost != "" {
					transportConfig["headers"] = map[string]interface{}{
						"Host": node.WSHost,
					}
				}
			case "http":
				transportConfig["type"] = "http"
				if node.WSPath != "" {
					transportConfig["path"] = node.WSPath
				}
				if node.WSHost != "" {
					transportConfig["host"] = []string{node.WSHost}
				}
			case "grpc":
				transportConfig["type"] = "grpc"
				if node.ServiceName != "" {
					transportConfig["service_name"] = node.ServiceName
				}
			default:
				transportConfig["type"] = node.Network
			}
			if len(transportConfig) > 0 {
				outbound["transport"] = transportConfig
			}
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

		// 传输方式配置
		if node.Network != "" && node.Network != "tcp" {
			transportConfig := map[string]interface{}{}

			switch node.Network {
			case "grpc":
				transportConfig["type"] = "grpc"
				if node.ServiceName != "" {
					transportConfig["service_name"] = node.ServiceName
				}
			case "ws":
				transportConfig["type"] = "ws"
				if node.WSPath != "" {
					transportConfig["path"] = node.WSPath
				}
				if node.WSHost != "" {
					transportConfig["headers"] = map[string]interface{}{
						"Host": node.WSHost,
					}
				}
			case "http":
				transportConfig["type"] = "http"
				if node.WSPath != "" {
					transportConfig["path"] = node.WSPath
				}
				if node.WSHost != "" {
					transportConfig["host"] = []string{node.WSHost}
				}
			default:
				transportConfig["type"] = node.Network
			}

			if len(transportConfig) > 0 {
				outbound["transport"] = transportConfig
			}
		}

		// TLS/Reality 配置
		if node.TLS {
			if node.Security == "reality" {
				// Reality 配置
				realityConfig := map[string]interface{}{
					"enabled": true,
				}
				if node.RealityPublicKey != "" {
					realityConfig["public_key"] = node.RealityPublicKey
				}
				if node.RealityShortID != "" {
					realityConfig["short_id"] = node.RealityShortID
				}
				if node.SNI != "" {
					realityConfig["server_name"] = node.SNI
				}
				if node.Fingerprint != "" {
					realityConfig["fingerprint"] = node.Fingerprint
				}
				outbound["reality"] = realityConfig
			} else {
				// 标准 TLS 配置
				tlsConfig := map[string]interface{}{
					"enabled": true,
				}
				if node.SNI != "" {
					tlsConfig["server_name"] = node.SNI
				}
				if node.ALPN != "" {
					tlsConfig["alpn"] = []string{node.ALPN}
				}
				if node.Fingerprint != "" {
					tlsConfig["fingerprint"] = node.Fingerprint
				}
				outbound["tls"] = tlsConfig
			}
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

	// 为 outbound 添加 tag
	outbound["tag"] = "proxy"

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
				},
			},
			"final": "proxy",
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

	// 启动 sing-box 进行真实连接测试（减少超时时间，提高速度）
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cmd = exec.CommandContext(ctx, singBoxPath, "run", "-c", configFile)
	// 不重定向输出，避免干扰
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("启动 sing-box 失败: %v", err)
	}

	// 确保进程被清理
	defer func() {
		if cmd.Process != nil {
			cmd.Process.Kill()
			cmd.Process.Wait()
		}
	}()

	// 等待 sing-box 启动（减少等待时间，提高速度）
	maxWait := 3 * time.Second             // 减少到 3 秒
	waitInterval := 100 * time.Millisecond // 减少间隔时间
	waited := time.Duration(0)
	for waited < maxWait {
		// 检查端口是否已监听
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 50*time.Millisecond)
		if err == nil {
			conn.Close()
			break
		}
		time.Sleep(waitInterval)
		waited += waitInterval
	}

	if waited >= maxWait {
		return 0, fmt.Errorf("sing-box 启动超时，端口 %d 未就绪", port)
	}

	// 通过代理测试真实连接
	// 使用多个可靠的测试 URL，按优先级排序
	testURLs := []string{
		"http://cp.cloudflare.com",           // Cloudflare 连通性检测，返回简单文本
		"http://www.cloudflare.com",          // Cloudflare，稳定
		"http://www.google.com/generate_204", // Google 204 响应
		"http://1.1.1.1",                     // Cloudflare DNS，简单
		"http://www.baidu.com",               // 百度，国内可访问
	}

	// 从环境变量读取测试 URL（如果设置了）
	if customURL := os.Getenv("TEST_URL"); customURL != "" {
		testURLs = []string{customURL}
	}

	proxyURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	// 从环境变量读取超时时间
	timeoutStr := os.Getenv("TEST_TIMEOUT")
	timeout := 8 * time.Second // 默认 8 秒，提高速度
	if timeoutStr != "" {
		if seconds, err := time.ParseDuration(timeoutStr + "s"); err == nil && seconds > 0 {
			timeout = seconds
		}
	}

	// 创建 HTTP 客户端，使用代理
	proxyURLParsed, err := url.Parse(proxyURL)
	if err != nil {
		return 0, fmt.Errorf("解析代理 URL 失败: %v", err)
	}

	transport := &http.Transport{
		Proxy: http.ProxyURL(proxyURLParsed),
		// 移除 DialContext，因为使用代理时应该通过代理连接，而不是直接连接
		// 设置代理连接超时
		ProxyConnectHeader:    make(http.Header),
		ResponseHeaderTimeout: 5 * time.Second,  // 响应头超时，减少到 5 秒
		IdleConnTimeout:       30 * time.Second, // 空闲连接超时
		DisableKeepAlives:     false,            // 启用 Keep-Alive
		TLSHandshakeTimeout:   4 * time.Second,  // TLS 握手超时，减少到 4 秒
		ExpectContinueTimeout: 1 * time.Second,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   timeout,
		// 禁用自动重定向，手动处理以确保通过代理
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	// 减少测试次数以提高速度
	testCount := 1 // 默认只测试 1 次，提高速度
	if testCountEnv := os.Getenv("TEST_COUNT"); testCountEnv != "" {
		if n, err := strconv.Atoi(testCountEnv); err == nil && n > 0 && n <= 3 {
			testCount = n
		}
	}

	// 计算最小成功次数（至少 50% 成功率，提高速度）
	minSuccessCount := int(float64(testCount) * 0.5)
	if minSuccessCount < 1 {
		minSuccessCount = 1
	}

	var successfulTests []time.Duration
	var lastErr error
	urlSuccessCount := 0 // 记录成功测试的 URL 数量

	// 尝试多个测试 URL，每个 URL 测试多次
	// 优化：只测试第一个 URL，如果成功就返回，提高速度
	for _, testURL := range testURLs {
		var urlLatencies []time.Duration
		successCount := 0
		urlErrors := []error{}

		// 对每个 URL 进行多次测试
		for i := 0; i < testCount; i++ {
			start := time.Now()
			req, err := http.NewRequestWithContext(ctx, "GET", testURL, nil)
			if err != nil {
				urlErrors = append(urlErrors, fmt.Errorf("创建请求失败: %v", err))
				lastErr = err
				continue
			}

			// 设置 User-Agent 和 Accept，避免某些服务器拒绝请求
			req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
			req.Header.Set("Accept", "*/*")
			req.Header.Set("Accept-Language", "en-US,en;q=0.9")
			req.Header.Set("Connection", "keep-alive")

			resp, err := client.Do(req)
			if err != nil {
				// 记录错误但继续尝试
				errMsg := fmt.Errorf("代理连接失败: %v", err)
				urlErrors = append(urlErrors, errMsg)
				lastErr = errMsg
				continue
			}

			latency := time.Since(start)
			statusCode := resp.StatusCode

			// 读取响应体（至少读取一部分）以确保连接完全建立
			body := make([]byte, 1024)
			n, readErr := resp.Body.Read(body)
			resp.Body.Close()

			// 检查读取错误和状态码
			// 如果读取失败且没有读取到任何数据（且不是正常的 EOF），认为连接失败
			if readErr != nil && !errors.Is(readErr, io.EOF) && n == 0 {
				errMsg := fmt.Errorf("读取响应体失败: %v", readErr)
				urlErrors = append(urlErrors, errMsg)
				lastErr = errMsg
				continue
			}

			// 2xx/3xx 认为连接成功
			// 注意：204 No Content 没有响应体（n==0），但仍然是成功的连接
			if statusCode >= 200 && statusCode < 400 {
				urlLatencies = append(urlLatencies, latency)
				successCount++
			} else {
				// 对于其他错误，记录但继续
				errMsg := fmt.Errorf("HTTP 状态码: %d 或响应体为空", statusCode)
				urlErrors = append(urlErrors, errMsg)
				lastErr = errMsg
			}
		}

		// 只有成功次数达到最小要求（至少 50%）才认为这个 URL 测试通过
		if successCount >= minSuccessCount && len(urlLatencies) > 0 {
			var sum time.Duration
			for _, lat := range urlLatencies {
				sum += lat
			}
			avgLatency := sum / time.Duration(len(urlLatencies))
			successfulTests = append(successfulTests, avgLatency)
			urlSuccessCount++

			// 任意一个 URL 测试通过即可返回，不必全部测试
			return avgLatency, nil
		}
	}

	// 必须至少有一个 URL 的所有测试都达到最小成功率要求，才认为节点可用
	if urlSuccessCount == 0 {
		if lastErr != nil {
			return 0, fmt.Errorf("所有测试 URL 都失败，最后错误: %v", lastErr)
		}
		return 0, fmt.Errorf("所有测试 URL 都失败，未达到最小成功率要求（%d/%d）", minSuccessCount, testCount)
	}

	// 计算所有成功测试的平均延迟
	if len(successfulTests) > 0 {
		var sum time.Duration
		for _, lat := range successfulTests {
			sum += lat
		}
		avgLatency := sum / time.Duration(len(successfulTests))
		return avgLatency, nil
	}

	// 所有测试都失败
	if lastErr != nil {
		return 0, fmt.Errorf("所有测试都失败: %v", lastErr)
	}
	return 0, fmt.Errorf("所有测试都失败，未达到最小成功率要求（%d/%d）", minSuccessCount, testCount)
}

// testSpeedWithSingBox 使用 sing-box 测试速度
func (c *Collector) testSpeedWithSingBox(singBoxPath, configFile string) (float64, error) {
	// 这里可以实现速度测试逻辑
	// 例如下载一个测试文件并计算速度
	// 由于实现较复杂，这里返回 0
	return 0, nil
}
