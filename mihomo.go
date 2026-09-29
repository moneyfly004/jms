package main

import (
	"bytes"
	"context"
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
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// MihomoVersion 是下载/使用 mihomo（原 Clash.Meta）内核的默认版本。
// 升级内核时同时修改这里以及 .github/workflows/collect-nodes.yml。
const MihomoVersion = "v1.19.31"

// mihomoProxyName 是测试配置中唯一出站的名称。
const mihomoProxyName = "PROXY"

var (
	mihomoPathOnce   sync.Once
	cachedMihomoPath string
	cachedMihomoErr  error

	testConfigOnce sync.Once
	cachedTestURLs []string
	cachedTimeout  time.Duration
	cachedCount    int

	portAllocationMutex sync.Mutex
)

// mihomoBinaryName 返回当前平台下 mihomo 可执行文件的名字。
func mihomoBinaryName() string {
	if runtime.GOOS == "windows" {
		return "mihomo.exe"
	}
	return "mihomo"
}

// getMihomoPath 定位 mihomo 内核，优先级：
// MIHOMO_PATH 环境变量 > 项目内约定目录 > PATH。
func getMihomoPath() (string, error) {
	mihomoPathOnce.Do(func() {
		exe := mihomoBinaryName()

		var candidates []string
		if p := strings.TrimSpace(os.Getenv("MIHOMO_PATH")); p != "" {
			candidates = append(candidates, p)
		}

		dirs := []string{
			filepath.Join(".", "mihomo"),
			filepath.Join(".", "bin"),
			filepath.Join(".", "core"),
			fmt.Sprintf("./mihomo-%s-%s", runtime.GOOS, runtime.GOARCH),
			fmt.Sprintf("./mihomo-%s-amd64", runtime.GOOS),
			fmt.Sprintf("./mihomo-%s-arm64", runtime.GOOS),
			".",
		}
		for _, d := range dirs {
			candidates = append(candidates, filepath.Join(d, exe))
		}

		for _, p := range candidates {
			if info, err := os.Stat(p); err == nil && !info.IsDir() {
				cachedMihomoPath = p
				return
			}
		}

		if p, err := exec.LookPath("mihomo"); err == nil {
			cachedMihomoPath = p
			return
		}
		if p, err := exec.LookPath(exe); err == nil {
			cachedMihomoPath = p
			return
		}

		cachedMihomoErr = fmt.Errorf("未找到 mihomo 内核，请将 %s 放入 ./mihomo/ 目录或设置 MIHOMO_PATH", exe)
	})
	return cachedMihomoPath, cachedMihomoErr
}

// getTestConfig 返回测速目标地址、超时时间与重试次数。
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

// getFreePort 申请一个本地空闲端口给 mihomo 的 mixed-port 使用。
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

// TestNodeWithMihomo 使用 mihomo 内核真实代理一次请求来验证节点可用性。
func (c *Collector) TestNodeWithMihomo(nodeLink string) *ValidNode {
	result := &ValidNode{Link: nodeLink}

	node, err := ParseNodeLink(nodeLink)
	if err != nil {
		result.Error = fmt.Errorf("解析失败: %v", err)
		return result
	}
	result.Type = node.Type

	mihomoPath, err := getMihomoPath()
	if err != nil {
		result.Error = err
		return result
	}

	port, err := getFreePort()
	if err != nil {
		result.Error = fmt.Errorf("分配端口失败: %v", err)
		return result
	}

	workDir, configFile, err := c.createMihomoConfig(node, port)
	if err != nil {
		result.Error = fmt.Errorf("创建配置失败: %v", err)
		return result
	}
	defer os.RemoveAll(workDir)

	latency, err := c.testLatencyWithMihomo(mihomoPath, workDir, configFile, port)
	if err != nil {
		log.Printf("⚠️ 节点 %s mihomo 测试失败: %v", shortenLink(nodeLink), err)
		result.Error = fmt.Errorf("代理测试失败: %v", err)
		return result
	}

	result.Latency = latency
	return result
}

// createMihomoConfig 为单个节点生成一份最小可用的 mihomo 配置。
// 返回 (工作目录, 配置文件路径, 错误)。
func (c *Collector) createMihomoConfig(node *ProxyNode, listenPort int) (string, string, error) {
	proxy, err := buildMihomoProxy(node)
	if err != nil {
		return "", "", err
	}

	config := map[string]interface{}{
		"mixed-port":     listenPort,
		"bind-address":   "127.0.0.1",
		"allow-lan":      false,
		"mode":           "rule",
		"log-level":      "silent",
		"ipv6":           false,
		"unified-delay":  true,
		"tcp-concurrent": false,
		// 关闭不需要的子系统，减少内核启动耗时与外部依赖
		"find-process-mode": "off",
		"geodata-mode":      false,
		"geo-auto-update":   false,
		"sniffer":           map[string]interface{}{"enable": false},
		"dns": map[string]interface{}{
			"enable": false,
			"ipv6":   false,
		},
		"profile": map[string]interface{}{
			"store-selected": false,
			"store-fake-ip":  false,
		},
		"proxies": []interface{}{proxy},
		"rules":   []string{fmt.Sprintf("MATCH,%s", mihomoProxyName)},
	}

	data, err := yaml.Marshal(config)
	if err != nil {
		return "", "", fmt.Errorf("序列化 mihomo 配置失败: %v", err)
	}

	workDir, err := os.MkdirTemp("", "mihomo-node-*")
	if err != nil {
		return "", "", fmt.Errorf("创建临时目录失败: %v", err)
	}

	configFile := filepath.Join(workDir, "config.yaml")
	if err := os.WriteFile(configFile, data, 0644); err != nil {
		_ = os.RemoveAll(workDir)
		return "", "", fmt.Errorf("写入 mihomo 配置失败: %v", err)
	}

	return workDir, configFile, nil
}

// buildMihomoProxy 把解析后的节点转换成 mihomo(Clash.Meta) 的 proxy 配置项。
func buildMihomoProxy(node *ProxyNode) (map[string]interface{}, error) {
	if node.Server == "" {
		return nil, errors.New("缺少服务器地址")
	}
	if node.Port <= 0 || node.Port > 65535 {
		return nil, fmt.Errorf("无效端口: %d", node.Port)
	}

	proxy := map[string]interface{}{
		"name":   mihomoProxyName,
		"server": node.Server,
		"port":   node.Port,
	}

	switch node.Type {
	case "ss":
		if node.Cipher == "" || node.Password == "" {
			return nil, errors.New("ss 节点缺少加密方式或密码")
		}
		proxy["type"] = "ss"
		proxy["cipher"] = node.Cipher
		proxy["password"] = node.Password
		proxy["udp"] = true
		if plugin, opts := parseShadowsocksPlugin(node); plugin != "" {
			proxy["plugin"] = plugin
			if len(opts) > 0 {
				proxy["plugin-opts"] = opts
			}
		}

	case "ssr":
		if node.Cipher == "" || node.Password == "" {
			return nil, errors.New("ssr 节点缺少加密方式或密码")
		}
		proxy["type"] = "ssr"
		proxy["cipher"] = node.Cipher
		proxy["password"] = node.Password
		proxy["protocol"] = defaultString(node.Protocol, "origin")
		proxy["obfs"] = defaultString(node.Obfs, "plain")
		if node.ProtocolParam != "" {
			proxy["protocol-param"] = node.ProtocolParam
		}
		if node.ObfsParam != "" {
			proxy["obfs-param"] = node.ObfsParam
		}
		proxy["udp"] = true

	case "vmess":
		if node.UUID == "" {
			return nil, errors.New("vmess 节点缺少 UUID")
		}
		proxy["type"] = "vmess"
		proxy["uuid"] = node.UUID
		proxy["alterId"] = node.AlterID
		proxy["cipher"] = normalizeVmessCipher(node.Security)
		proxy["udp"] = true
		applyMihomoTransport(proxy, node)
		if node.TLS {
			proxy["tls"] = true
			if node.SNI != "" {
				proxy["servername"] = node.SNI
			}
			if node.Insecure {
				proxy["skip-cert-verify"] = true
			}
			if fp := normalizeFingerprint(node.Fingerprint); fp != "" {
				proxy["client-fingerprint"] = fp
			}
		}

	case "vless":
		if node.UUID == "" {
			return nil, errors.New("vless 节点缺少 UUID")
		}
		proxy["type"] = "vless"
		proxy["uuid"] = node.UUID
		proxy["udp"] = true
		if node.Flow != "" {
			proxy["flow"] = node.Flow
		}
		applyMihomoTransport(proxy, node)
		if node.TLS {
			proxy["tls"] = true
			if node.SNI != "" {
				proxy["servername"] = node.SNI
			}
			if alpn := alpnList(node.ALPN); len(alpn) > 0 {
				proxy["alpn"] = alpn
			}
			if node.Insecure {
				proxy["skip-cert-verify"] = true
			}
			if fp := normalizeFingerprint(node.Fingerprint); fp != "" {
				proxy["client-fingerprint"] = fp
			}
		}
		reality := map[string]interface{}{}
		if node.RealityPublicKey != "" {
			reality["public-key"] = node.RealityPublicKey
		}
		if node.RealityShortID != "" {
			reality["short-id"] = node.RealityShortID
		}
		if len(reality) > 0 {
			proxy["reality-opts"] = reality
			proxy["tls"] = true
			if _, ok := proxy["client-fingerprint"]; !ok {
				proxy["client-fingerprint"] = "chrome"
			}
		}

	case "trojan":
		if node.Password == "" {
			return nil, errors.New("trojan 节点缺少密码")
		}
		proxy["type"] = "trojan"
		proxy["password"] = node.Password
		proxy["udp"] = true
		if node.SNI != "" {
			proxy["sni"] = node.SNI
		}
		if alpn := alpnList(node.ALPN); len(alpn) > 0 {
			proxy["alpn"] = alpn
		}
		if node.Insecure {
			proxy["skip-cert-verify"] = true
		}
		if fp := normalizeFingerprint(node.Fingerprint); fp != "" {
			proxy["client-fingerprint"] = fp
		}
		applyMihomoTransport(proxy, node)

	case "hysteria":
		proxy["type"] = "hysteria"
		proxy["up"] = defaultString(node.Up, "100 Mbps")
		proxy["down"] = defaultString(node.Down, "100 Mbps")
		auth := defaultString(node.Auth, node.Password)
		if auth != "" {
			proxy["auth-str"] = auth
		}
		if node.ObfsPassword != "" {
			proxy["obfs"] = node.ObfsPassword
		}
		if node.SNI != "" {
			proxy["sni"] = node.SNI
		}
		if alpn := alpnList(node.ALPN); len(alpn) > 0 {
			proxy["alpn"] = alpn
		}
		if node.Insecure {
			proxy["skip-cert-verify"] = true
		}
		if node.Protocol != "" {
			proxy["protocol"] = node.Protocol
		}

	case "hysteria2":
		password := defaultString(node.Password, node.Auth)
		if password == "" {
			return nil, errors.New("hysteria2 节点缺少密码")
		}
		proxy["type"] = "hysteria2"
		proxy["password"] = password
		proxy["up"] = defaultString(node.Up, "100 Mbps")
		proxy["down"] = defaultString(node.Down, "100 Mbps")
		if node.SNI != "" {
			proxy["sni"] = node.SNI
		}
		if alpn := alpnList(node.ALPN); len(alpn) > 0 {
			proxy["alpn"] = alpn
		}
		if node.Insecure {
			proxy["skip-cert-verify"] = true
		}
		if node.ObfsPassword != "" {
			proxy["obfs"] = defaultString(node.Obfs, "salamander")
			proxy["obfs-password"] = node.ObfsPassword
		}

	case "tuic":
		if node.UUID == "" && node.Token == "" {
			return nil, errors.New("tuic 节点缺少 uuid/token")
		}
		proxy["type"] = "tuic"
		if node.UUID != "" {
			proxy["uuid"] = node.UUID
		}
		if node.Password != "" {
			proxy["password"] = node.Password
		}
		if node.Token != "" {
			proxy["token"] = node.Token
		}
		if node.SNI != "" {
			proxy["sni"] = node.SNI
		}
		if alpn := alpnList(node.ALPN); len(alpn) > 0 {
			proxy["alpn"] = alpn
		}
		if node.Insecure {
			proxy["skip-cert-verify"] = true
		}
		proxy["udp-relay-mode"] = "native"
		proxy["congestion-controller"] = "bbr"

	case "wireguard":
		if node.PrivateKey == "" || node.PublicKey == "" {
			return nil, errors.New("wireguard 节点缺少密钥")
		}
		proxy["type"] = "wireguard"
		proxy["private-key"] = node.PrivateKey
		proxy["public-key"] = node.PublicKey
		proxy["ip"] = defaultString(node.Address, "172.16.0.2/32")
		proxy["udp"] = true
		if node.PreSharedKey != "" {
			proxy["pre-shared-key"] = node.PreSharedKey
		}
		if reserved := reservedList(node.Reserved); len(reserved) > 0 {
			proxy["reserved"] = reserved
		}

	case "http":
		proxy["type"] = "http"
		if node.Username != "" || node.Password != "" {
			proxy["username"] = node.Username
			proxy["password"] = node.Password
		}
		if node.TLS {
			proxy["tls"] = true
			if node.SNI != "" {
				proxy["sni"] = node.SNI
			}
			if node.Insecure {
				proxy["skip-cert-verify"] = true
			}
		}

	case "socks5":
		proxy["type"] = "socks5"
		proxy["udp"] = true
		if node.Username != "" || node.Password != "" {
			proxy["username"] = node.Username
			proxy["password"] = node.Password
		}
		if node.TLS {
			proxy["tls"] = true
			if node.Insecure {
				proxy["skip-cert-verify"] = true
			}
		}

	case "anytls":
		password := defaultString(node.Password, node.UUID)
		if password == "" {
			return nil, errors.New("anytls 节点缺少密码")
		}
		proxy["type"] = "anytls"
		proxy["password"] = password
		proxy["udp"] = true
		if node.SNI != "" {
			proxy["sni"] = node.SNI
		}
		if alpn := alpnList(node.ALPN); len(alpn) > 0 {
			proxy["alpn"] = alpn
		}
		if node.Insecure {
			proxy["skip-cert-verify"] = true
		}
		if fp := normalizeFingerprint(node.Fingerprint); fp != "" {
			proxy["client-fingerprint"] = fp
		}

	default:
		// mihomo 不支持 socks4 / gost 等协议
		return nil, fmt.Errorf("mihomo 不支持的节点类型: %s", node.Type)
	}

	return proxy, nil
}

// applyMihomoTransport 填充 ws / grpc / h2 / http 传输层参数。
func applyMihomoTransport(proxy map[string]interface{}, node *ProxyNode) {
	network := strings.ToLower(node.Network)
	switch network {
	case "", "tcp", "raw":
		return
	case "ws", "websocket":
		proxy["network"] = "ws"
		wsOpts := map[string]interface{}{}
		if node.WSPath != "" {
			wsOpts["path"] = node.WSPath
		}
		if node.WSHost != "" {
			wsOpts["headers"] = map[string]interface{}{"Host": node.WSHost}
		}
		if len(wsOpts) > 0 {
			proxy["ws-opts"] = wsOpts
		}
	case "grpc", "gun":
		proxy["network"] = "grpc"
		grpcOpts := map[string]interface{}{}
		if node.ServiceName != "" {
			grpcOpts["grpc-service-name"] = node.ServiceName
		}
		if node.WSPath != "" {
			grpcOpts["grpc-service-name"] = strings.TrimPrefix(node.WSPath, "/")
		}
		if len(grpcOpts) > 0 {
			proxy["grpc-opts"] = grpcOpts
		}
	case "h2":
		proxy["network"] = "h2"
		h2Opts := map[string]interface{}{}
		if node.WSHost != "" {
			h2Opts["host"] = []string{node.WSHost}
		}
		if node.WSPath != "" {
			h2Opts["path"] = node.WSPath
		}
		if len(h2Opts) > 0 {
			proxy["h2-opts"] = h2Opts
		}
	case "http":
		proxy["network"] = "http"
		httpOpts := map[string]interface{}{}
		if node.WSPath != "" {
			httpOpts["path"] = []string{node.WSPath}
		}
		if node.WSHost != "" {
			httpOpts["headers"] = map[string]interface{}{"Host": []string{node.WSHost}}
		}
		if len(httpOpts) > 0 {
			proxy["http-opts"] = httpOpts
		}
	default:
		// quic / kcp / splithttp 等 mihomo 不支持或参数不全，退化为直连并让测速判定失败
		log.Printf("⚠️ 节点 %s 的传输协议 %q 不被 mihomo 支持，按 TCP 处理", node.Name, node.Network)
	}
}

// parseShadowsocksPlugin 解析 ss 的 plugin / plugin-opts。
func parseShadowsocksPlugin(node *ProxyNode) (string, map[string]interface{}) {
	plugin := strings.TrimSpace(node.Plugin)
	if plugin == "" {
		return "", nil
	}

	parts := strings.Split(plugin, ";")
	name := strings.TrimSpace(parts[0])
	rawOpts := map[string]string{}
	for _, kv := range parts[1:] {
		k, v, found := strings.Cut(kv, "=")
		if found {
			rawOpts[strings.TrimSpace(k)] = strings.TrimSpace(v)
		} else if strings.TrimSpace(kv) != "" {
			rawOpts[strings.TrimSpace(kv)] = "true"
		}
	}
	// parseShadowsocks 会把 PluginOpts 也填成完整的 plugin 串，这里合并一次。
	if node.PluginOpts != "" && node.PluginOpts != plugin {
		for _, kv := range strings.Split(node.PluginOpts, ";") {
			k, v, found := strings.Cut(kv, "=")
			if found {
				rawOpts[strings.TrimSpace(k)] = strings.TrimSpace(v)
			}
		}
	}

	opts := map[string]interface{}{}
	switch name {
	case "obfs-local", "simple-obfs", "obfs", "obfs4":
		// Clash 中 simple-obfs 的插件名固定为 obfs
		if mode := firstNonEmpty(rawOpts["obfs"], rawOpts["mode"]); mode != "" {
			opts["mode"] = mode
		}
		if host := firstNonEmpty(rawOpts["obfs-host"], rawOpts["host"]); host != "" {
			opts["host"] = host
		}
		return "obfs", opts
	case "v2ray-plugin":
		if mode := rawOpts["mode"]; mode != "" {
			opts["mode"] = mode
		}
		if host := rawOpts["host"]; host != "" {
			opts["host"] = host
		}
		if path := rawOpts["path"]; path != "" {
			opts["path"] = path
		}
		if tls := rawOpts["tls"]; tls == "true" || tls == "1" {
			opts["tls"] = true
		}
		if rawOpts["mux"] == "true" || rawOpts["mux"] == "1" {
			opts["mux"] = true
		}
		return "v2ray-plugin", opts
	case "shadow-tls":
		if host := rawOpts["host"]; host != "" {
			opts["host"] = host
		}
		if password := rawOpts["password"]; password != "" {
			opts["password"] = password
		}
		if version, err := strconv.Atoi(rawOpts["version"]); err == nil {
			opts["version"] = version
		}
		return "shadow-tls", opts
	case "restls":
		if host := rawOpts["host"]; host != "" {
			opts["host"] = host
		}
		if password := rawOpts["password"]; password != "" {
			opts["password"] = password
		}
		if script := rawOpts["restls-script"]; script != "" {
			opts["restls-script"] = script
		}
		return "restls", opts
	default:
		log.Printf("⚠️ 节点 %s 使用了 mihomo 不支持的 ss 插件 %q，忽略插件", node.Name, name)
		return "", nil
	}
}

// testLatencyWithMihomo 启动 mihomo 并通过其 mixed-port 发起真实请求。
func (c *Collector) testLatencyWithMihomo(mihomoPath, workDir, configFile string, port int) (time.Duration, error) {
	// 先用 -t 做一次配置校验，快速剔除无法生成的节点
	checkCmd := exec.Command(mihomoPath, "-t", "-d", workDir, "-f", configFile)
	if output, err := checkCmd.CombinedOutput(); err != nil {
		return 0, fmt.Errorf("mihomo 配置校验失败: %v, 输出: %s", err, truncateOutput(output))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var stderrBuf bytes.Buffer
	cmdRun := exec.CommandContext(ctx, mihomoPath, "-d", workDir, "-f", configFile)
	cmdRun.Stdout = nil
	cmdRun.Stderr = &stderrBuf

	if err := cmdRun.Start(); err != nil {
		return 0, fmt.Errorf("启动 mihomo 失败: %v", err)
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
		if cmdRun.ProcessState != nil && cmdRun.ProcessState.Exited() {
			return 0, fmt.Errorf("mihomo 启动后立即退出: %s", strings.TrimSpace(stderrBuf.String()))
		}
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
		return 0, fmt.Errorf("mihomo 启动超时，端口 %d 未就绪: %s", port, strings.TrimSpace(stderrBuf.String()))
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

			req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
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

func normalizeVmessCipher(security string) string {
	switch strings.ToLower(strings.TrimSpace(security)) {
	case "auto", "aes-128-gcm", "chacha20-poly1305", "none", "zero":
		return strings.ToLower(strings.TrimSpace(security))
	default:
		return "auto"
	}
}

func normalizeFingerprint(fp string) string {
	switch strings.ToLower(strings.TrimSpace(fp)) {
	case "chrome", "firefox", "safari", "ios", "android", "edge", "360", "qq", "random":
		return strings.ToLower(strings.TrimSpace(fp))
	default:
		return ""
	}
}

func alpnList(alpn string) []string {
	if strings.TrimSpace(alpn) == "" {
		return nil
	}
	var result []string
	for _, item := range strings.Split(alpn, ",") {
		if item = strings.TrimSpace(item); item != "" {
			result = append(result, item)
		}
	}
	return result
}

func reservedList(reserved string) []int {
	if strings.TrimSpace(reserved) == "" {
		return nil
	}
	var result []int
	for _, item := range strings.Split(reserved, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(item))
		if err != nil || n < 0 || n > 255 {
			return nil
		}
		result = append(result, n)
	}
	return result
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func shortenLink(link string) string {
	if len(link) > 50 {
		return link[:50]
	}
	return link
}

func truncateOutput(output []byte) string {
	text := strings.TrimSpace(string(output))
	if len(text) > 300 {
		return text[:300]
	}
	return text
}
