package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

var portAllocationMutex sync.Mutex
var (
	singBoxPathOnce   sync.Once
	cachedSingBoxPath string
	cachedSingBoxErr  error
	testConfigOnce    sync.Once
	cachedTestURLs    []string
	cachedTimeout     time.Duration
	cachedCount       int
	globalTransport   = &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 20,
		IdleConnTimeout:     30 * time.Second,
		TLSHandshakeTimeout: 4 * time.Second,
		ResponseHeaderTimeout: 5 * time.Second,
	}
)

func getSingBoxPath() (string, error) {
	singBoxPathOnce.Do(func() {
		if path := os.Getenv("SINGBOX_PATH"); path != "" {
			cachedSingBoxPath = path
			return
		}
		localPaths := []string{
			"./sing-box-1.10.0-alpha.29-darwin-amd64/sing-box",
			"./sing-box-1.10.0-alpha.29-linux-amd64/sing-box",
			"./sing-box-1.10.0-alpha.29-darwin-arm64/sing-box",
			"./sing-box",
			"sing-box",
		}
		for _, path := range localPaths {
			if _, err := os.Stat(path); err == nil {
				cachedSingBoxPath = path
				return
			}
		}
		if path, err := exec.LookPath("sing-box"); err == nil {
			cachedSingBoxPath = path
			return
		}
		cachedSingBoxErr = errors.New("sing-box 未找到，请确保 sing-box 内核文件在项目目录中")
	})
	return cachedSingBoxPath, cachedSingBoxErr
}

func getTestConfig() ([]string, time.Duration, int) {
	testConfigOnce.Do(func() {
		cachedTestURLs = []string{
			"http://www.apple.com",
			"http://www.microsoft.com",
			"http://www.amazon.com",
			"http://cp.cloudflare.com",
			"http://www.google.com/generate_204",
		}
		if customURL := os.Getenv("TEST_URL"); customURL != "" {
			cachedTestURLs = []string{customURL}
		}
		cachedTimeout = 15 * time.Second
		if timeoutStr := os.Getenv("TEST_TIMEOUT"); timeoutStr != "" {
			if seconds, err := time.ParseDuration(timeoutStr + "s"); err == nil && seconds > 0 {
				cachedTimeout = seconds
			}
		}
		cachedCount = 1
		if testCountEnv := os.Getenv("TEST_COUNT"); testCountEnv != "" {
			if n, err := strconv.Atoi(testCountEnv); err == nil && n > 0 && n <= 3 {
				cachedCount = n
			}
		}
	})
	return cachedTestURLs, cachedTimeout, cachedCount
}

func getFreePort() (int, error) {
	portAllocationMutex.Lock()
	defer portAllocationMutex.Unlock()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("无法分配端口: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	time.Sleep(10 * time.Millisecond)
	return port, nil
}

func (c *Collector) TestNodeWithSingBox(nodeLink string) *ValidNode {
	result := &ValidNode{Link: nodeLink}
	node, err := ParseNodeLink(nodeLink)
	if err != nil {
		result.Error = fmt.Errorf("解析失败: %v", err)
		return result
	}
	result.Type = node.Type

	if os.Getenv("USE_SINGBOX") == "false" {
		return c.TestNode(nodeLink)
	}

	singBoxPath, err := getSingBoxPath()
	if err != nil {
		result.Error = err
		return result
	}

	port, err := getFreePort()
	if err != nil {
		return c.TestNode(nodeLink)
	}

	configFile, err := c.createSingBoxConfig(nodeLink, port)
	if err != nil {
		return c.TestNode(nodeLink)
	}
	defer os.Remove(configFile)

	latency, err := c.testLatencyWithSingBox(singBoxPath, configFile, port)
	if err != nil {
		nodeLinkShort := nodeLink
		if len(nodeLinkShort) > 50 {
			nodeLinkShort = nodeLinkShort[:50]
		}
		result.Error = fmt.Errorf("代理测试失败: %v", err)
		return result
	}

	result.Latency = latency

	if os.Getenv("TEST_SPEED") == "true" {
		speed, err := c.testSpeedWithSingBox(singBoxPath, configFile)
		if err == nil {
			nodeLinkShort := nodeLink
			if len(nodeLinkShort) > 50 {
				nodeLinkShort = nodeLinkShort[:50]
			}
			fmt.Printf("节点 %s 速度: %.2f MB/s\n", nodeLinkShort, speed)
		}
	}

	return result
}

func (c *Collector) createSingBoxConfig(nodeLink string, listenPort int) (string, error) {
	node, err := ParseNodeLink(nodeLink)
	if err != nil {
		return "", err
	}
	tmpDir := os.TempDir()
	configFile := filepath.Join(tmpDir, fmt.Sprintf("singbox_%d.json", time.Now().UnixNano()))

	var outbound map[string]interface{}

	switch node.Type {
	case "ss":
		outbound = map[string]interface{}{
			"type":         "shadowsocks",
			"server":      node.Server,
			"server_port": node.Port,
			"method":      node.Cipher,
			"password":    node.Password,
		}
	case "ssr":
		outbound = map[string]interface{}{
			"type":         "shadowsocks",
			"server":      node.Server,
			"server_port": node.Port,
			"method":      node.Cipher,
			"password":    node.Password,
		}
	case "vmess":
		outbound = map[string]interface{}{
			"type":         "vmess",
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
		if node.Network != "" && node.Network != "tcp" {
			transportConfig := map[string]interface{}{}
			switch node.Network {
			case "ws":
				transportConfig["type"] = "ws"
				if node.WSPath != "" {
					transportConfig["path"] = node.WSPath
				}
				if node.WSHost != "" {
					transportConfig["headers"] = map[string]interface{}{"Host": node.WSHost}
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
			tlsConfig := map[string]interface{}{"enabled": true}
			if node.SNI != "" {
				tlsConfig["server_name"] = node.SNI
			}
			if node.ALPN != "" {
				tlsConfig["alpn"] = []string{node.ALPN}
			}
			if node.Insecure {
				tlsConfig["insecure"] = true
			}
			outbound["tls"] = tlsConfig
		}
	case "vless":
		outbound = map[string]interface{}{
			"type":         "vless",
			"server":      node.Server,
			"server_port": node.Port,
			"uuid":        node.UUID,
		}
		if node.Flow != "" {
			outbound["flow"] = node.Flow
		}
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
					transportConfig["headers"] = map[string]interface{}{"Host": node.WSHost}
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
		if node.TLS {
			tlsConfig := map[string]interface{}{"enabled": true}
			if node.SNI != "" {
				tlsConfig["server_name"] = node.SNI
			}
			if node.ALPN != "" {
				tlsConfig["alpn"] = []string{node.ALPN}
			}
			if node.Fingerprint != "" {
				tlsConfig["utls"] = map[string]interface{}{
					"enabled":     true,
					"fingerprint": node.Fingerprint,
				}
			}
			if node.Security == "reality" {
				realityConfig := map[string]interface{}{"enabled": true}
				if node.RealityPublicKey != "" {
					realityConfig["public_key"] = node.RealityPublicKey
				}
				if node.RealityShortID != "" {
					realityConfig["short_id"] = node.RealityShortID
				}
				tlsConfig["reality"] = realityConfig
			}
			if node.Insecure {
				tlsConfig["insecure"] = true
			}
			outbound["tls"] = tlsConfig
		}
	case "trojan":
		outbound = map[string]interface{}{
			"type":         "trojan",
			"server":      node.Server,
			"server_port": node.Port,
			"password":    node.Password,
		}
		if node.TLS {
			tlsConfig := map[string]interface{}{"enabled": true}
			if node.SNI != "" {
				tlsConfig["server_name"] = node.SNI
			}
			if node.ALPN != "" {
				tlsConfig["alpn"] = []string{node.ALPN}
			}
			if node.Insecure {
				tlsConfig["insecure"] = true
			}
			outbound["tls"] = tlsConfig
		}
	case "hysteria":
		outbound = map[string]interface{}{
			"type":         "hysteria",
			"server":      node.Server,
			"server_port": node.Port,
		}
		if node.Auth != "" {
			outbound["auth"] = node.Auth
		}
		if node.ObfsPassword != "" {
			outbound["obfs"] = node.ObfsPassword
		}
	case "hysteria2":
		outbound = map[string]interface{}{
			"type":         "hysteria2",
			"server":      node.Server,
			"server_port": node.Port,
			"password":    node.Password,
		}
		tlsConfig := map[string]interface{}{"enabled": true}
		if node.SNI != "" {
			tlsConfig["server_name"] = node.SNI
		}
		if node.ALPN != "" {
			tlsConfig["alpn"] = []string{node.ALPN}
		}
		if !node.TLS {
			tlsConfig["insecure"] = true
		}
		outbound["tls"] = tlsConfig
		if node.ObfsPassword != "" {
			outbound["obfs"] = map[string]interface{}{
				"type":     "salamander",
				"password": node.ObfsPassword,
			}
		}
	case "wireguard":
		outbound = map[string]interface{}{
			"type":         "wireguard",
			"server":      node.Server,
			"server_port": node.Port,
			"private_key": node.PrivateKey,
		}
		if node.PublicKey != "" {
			outbound["peer_public_key"] = node.PublicKey
		}
		if node.Reserved != "" {
			outbound["reserved"] = node.Reserved
		}
	case "tuic":
		outbound = map[string]interface{}{
			"type":         "tuic",
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

	outbound["tag"] = "proxy"

	config := map[string]interface{}{
		"log":       map[string]interface{}{"level": "error"},
		"inbounds": []map[string]interface{}{
			{
				"type":        "mixed",
				"tag":         "mixed-in",
				"listen":      "127.0.0.1",
				"listen_port": listenPort,
			},
		},
		"outbounds": []map[string]interface{}{outbound},
		"route": map[string]interface{}{
			"final": "proxy",
		},
	}

	configJSON, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(configFile, configJSON, 0644); err != nil {
		return "", err
	}

	return configFile, nil
}

func (c *Collector) testLatencyWithSingBox(singBoxPath, configFile string, port int) (time.Duration, error) {
	cmdCheck := exec.Command(singBoxPath, "check", "-c", configFile)
	if output, err := cmdCheck.CombinedOutput(); err != nil {
		return 0, fmt.Errorf("sing-box check 失败: %v, 输出: %s", err, string(output))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmdRun := exec.CommandContext(ctx, singBoxPath, "run", "-c", configFile)
	cmdRun.Stdout = nil
	cmdRun.Stderr = nil

	if err := cmdRun.Start(); err != nil {
		return 0, fmt.Errorf("启动 sing-box 失败: %v", err)
	}

	defer func() {
		if cmdRun.Process != nil {
			_ = cmdRun.Process.Kill()
			_, _ = cmdRun.Process.Wait()
		}
	}()

	maxWait := 5 * time.Second
	waitInterval := 100 * time.Millisecond
	waited := time.Duration(0)
	portReady := false

	for waited < maxWait {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 50*time.Millisecond)
		if err == nil {
			conn.Close()
			portReady = true
			break
		}
		time.Sleep(waitInterval)
		waited += waitInterval
	}

	if !portReady {
		return 0, fmt.Errorf("sing-box 启动超时，端口 %d 未就绪", port)
	}

	urls, timeout, count := getTestConfig()

	proxyURLParsed, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))
	if err != nil {
		return 0, fmt.Errorf("解析代理 URL 失败: %v", err)
	}

	transport := &http.Transport{
		Proxy:                 http.ProxyURL(proxyURLParsed),
		ProxyConnectHeader:    make(http.Header),
		ResponseHeaderTimeout: 5 * time.Second,
		IdleConnTimeout:       30 * time.Second,
		DisableKeepAlives:     false,
		TLSHandshakeTimeout:   4 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}

	minSuccessCount := int(float64(count) * 0.5)
	if minSuccessCount < 1 {
		minSuccessCount = 1
	}

	var lastErr error

	for _, testURL := range urls {
		var latencies []time.Duration
		successCount := 0

		for i := 0; i < count; i++ {
			start := time.Now()
			req, err := http.NewRequestWithContext(ctx, "GET", testURL, nil)
			if err != nil {
				lastErr = fmt.Errorf("创建请求失败: %v", err)
				continue
			}

			req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
			req.Header.Set("Accept", "*/*")
			req.Header.Set("Accept-Language", "en-US,en;q=0.9")
			req.Header.Set("Connection", "keep-alive")

			resp, err := client.Do(req)
			if err != nil {
				lastErr = fmt.Errorf("代理连接失败: %v", err)
				continue
			}

			latency := time.Since(start)
			statusCode := resp.StatusCode

			body := make([]byte, 1024)
			n, readErr := resp.Body.Read(body)
			_ = resp.Body.Close()

			if readErr != nil && !errors.Is(readErr, io.EOF) && n == 0 {
				lastErr = fmt.Errorf("读取响应体失败: %v", readErr)
				continue
			}

			if statusCode >= 200 && statusCode < 400 {
				latencies = append(latencies, latency)
				successCount++
			} else {
				lastErr = fmt.Errorf("HTTP 状态码: %d", statusCode)
			}
		}

		if successCount >= minSuccessCount && len(latencies) > 0 {
			var sum time.Duration
			for _, lat := range latencies {
				sum += lat
			}
			return sum / time.Duration(len(latencies)), nil
		}
	}

	if lastErr != nil {
		return 0, fmt.Errorf("测试失败，最后错误: %v", lastErr)
	}
	return 0, fmt.Errorf("所有测试均未达到最低成功率要求（%d/%d）", minSuccessCount, count)
}

func (c *Collector) testSpeedWithSingBox(singBoxPath, configFile string) (float64, error) {
	return 0, nil
}
