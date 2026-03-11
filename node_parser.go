package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// ProxyNode 代理节点结构
type ProxyNode struct {
	Name     string
	Type     string
	Server   string
	Port     int
	UUID     string
	Password string
	Cipher   string
	Network  string
	TLS      bool
	UDP      bool
	// SSR 相关
	Protocol      string // SSR 协议
	ProtocolParam string // SSR 协议参数
	Obfs          string // SSR 混淆
	ObfsParam     string // SSR 混淆参数
	// Hysteria 相关
	Auth         string // Hysteria 认证
	ObfsPassword string // Hysteria 混淆密码
	// WireGuard 相关
	PrivateKey string // WireGuard 私钥
	PublicKey  string // WireGuard 公钥
	Reserved   string // WireGuard Reserved
	// TUIC 相关
	Token string // TUIC Token
	// 通用字段
	SNI      string // Server Name Indication
	ALPN     string // Application-Layer Protocol Negotiation
	Flow     string // VLESS Flow
	Security string // VMess Security / VLESS Security (tls/reality)
	AlterID  int    // VMess AlterID
	// Reality 相关
	RealityPublicKey string // Reality public key (pbk)
	RealityShortID   string // Reality short ID (sid)
	Fingerprint      string // TLS fingerprint (fp)
	// gRPC 相关
	ServiceName string // gRPC service name
	// WebSocket 相关
	WSHost string // WebSocket host
	WSPath string // WebSocket path
	// SS 插件相关
	Plugin     string // SS 插件名称 (v2ray-plugin, obfs-local, simple-obfs 等)
	PluginOpts string // SS 插件选项
	// AnyTLS 相关
	AnyTLSVersion string // AnyTLS 版本
	AnyTLSPadding string // AnyTLS 填充方案
	// GOST 相关
	GOSTProtocol string // GOST 协议类型
	GOSTPath     string // GOST 路径
}

// ParseNodeLink 解析节点链接
func ParseNodeLink(link string) (*ProxyNode, error) {
	link = strings.TrimSpace(link)

	if strings.HasPrefix(link, "vmess://") {
		return parseVMess(link)
	} else if strings.HasPrefix(link, "ss://") {
		return parseShadowsocks(link)
	} else if strings.HasPrefix(link, "vless://") {
		return parseVLESS(link)
	} else if strings.HasPrefix(link, "trojan://") {
		return parseTrojan(link)
	} else if strings.HasPrefix(link, "ssr://") {
		return parseSSR(link)
	} else if strings.HasPrefix(link, "hysteria://") {
		return parseHysteria(link)
	} else if strings.HasPrefix(link, "hysteria2://") || strings.HasPrefix(link, "hy2://") {
		return parseHysteria2(link)
	} else if strings.HasPrefix(link, "wireguard://") || strings.HasPrefix(link, "wg://") {
		return parseWireGuard(link)
	} else if strings.HasPrefix(link, "tuic://") {
		return parseTUIC(link)
	} else if strings.HasPrefix(link, "http://") || strings.HasPrefix(link, "https://") {
		return parseHTTP(link)
	} else if strings.HasPrefix(link, "socks://") || strings.HasPrefix(link, "socks4://") || strings.HasPrefix(link, "socks5://") {
		return parseSOCKS(link)
	} else if strings.HasPrefix(link, "anytls://") {
		return parseAnyTLS(link)
	} else if strings.HasPrefix(link, "gost://") || strings.HasPrefix(link, "gost+") {
		return parseGOST(link)
	}

	return nil, fmt.Errorf("不支持的协议")
}

// parseVMess 解析 VMess 链接
func parseVMess(link string) (*ProxyNode, error) {
	encoded := strings.TrimPrefix(link, "vmess://")

	// 尝试 Base64 解码
	decoded, err := safeBase64Decode(encoded)
	if err != nil {
		return nil, fmt.Errorf("Base64 解码失败: %v", err)
	}

	// 解析 JSON
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(decoded), &data); err != nil {
		return nil, fmt.Errorf("JSON 解析失败: %v", err)
	}

	// 提取基本信息
	server, _ := data["add"].(string)

	// 处理端口（可能是数字或字符串）
	var port int
	if portFloat, ok := data["port"].(float64); ok {
		port = int(portFloat)
	} else if portStr, ok := data["port"].(string); ok {
		if parsedPort, err := strconv.Atoi(portStr); err == nil {
			port = parsedPort
		}
	}

	// 验证端口
	if port <= 0 || port > 65535 {
		return nil, fmt.Errorf("无效的端口: %v", data["port"])
	}

	uuid, _ := data["id"].(string)
	if uuid == "" {
		return nil, fmt.Errorf("缺少 UUID (id)")
	}

	// 验证服务器地址
	if server == "" {
		return nil, fmt.Errorf("缺少服务器地址 (add)")
	}

	network, _ := data["net"].(string)
	if network == "" {
		network = "tcp"
	}

	// 构建节点
	node := &ProxyNode{
		Name:    getString(data, "ps", fmt.Sprintf("VMess-%s:%d", server, port)),
		Type:    "vmess",
		Server:  server,
		Port:    port,
		UUID:    uuid,
		Network: network,
		UDP:     true,
	}

	// Security 配置
	if security, ok := data["scy"].(string); ok && security != "" {
		node.Security = security
	} else if security, ok := data["security"].(string); ok && security != "" {
		node.Security = security
	}

	// AlterID 配置
	if alterID, ok := data["aid"].(float64); ok {
		node.AlterID = int(alterID)
	} else if alterID, ok := data["alterId"].(float64); ok {
		node.AlterID = int(alterID)
	}

	// TLS 配置
	if tls, ok := data["tls"].(string); ok && tls == "tls" {
		node.TLS = true
	} else if tls, ok := data["tls"].(bool); ok && tls {
		node.TLS = true
	}

	// SNI 配置
	if sni, ok := data["sni"].(string); ok && sni != "" {
		node.SNI = sni
	} else if sni, ok := data["host"].(string); ok && sni != "" {
		node.SNI = sni
	}

	// ALPN 配置
	if alpn, ok := data["alpn"].(string); ok && alpn != "" {
		node.ALPN = alpn
	} else if alpnArray, ok := data["alpn"].([]interface{}); ok && len(alpnArray) > 0 {
		if alpnStr, ok := alpnArray[0].(string); ok {
			node.ALPN = alpnStr
		}
	}

	// WebSocket 配置（从 JSON 数据中提取）
	if network == "ws" || network == "websocket" {
		if path, ok := data["path"].(string); ok && path != "" {
			node.WSPath = path
		}
		if host, ok := data["host"].(string); ok && host != "" {
			// 如果 host 不是 SNI，则作为 WebSocket Host
			if node.SNI == "" {
				node.SNI = host
			}
			node.WSHost = host
		}
	}

	return node, nil
}

// parseVLESS 解析 VLESS 链接
func parseVLESS(link string) (*ProxyNode, error) {
	parsed, err := url.Parse(link)
	if err != nil {
		return nil, err
	}

	uuid := parsed.User.Username()
	if uuid == "" {
		return nil, fmt.Errorf("缺少 UUID")
	}

	query := parsed.Query()
	network := query.Get("type")
	if network == "" {
		network = "tcp"
	}

	node := &ProxyNode{
		Name:    getFragment(parsed, fmt.Sprintf("VLESS-%s:%s", parsed.Hostname(), parsed.Port())),
		Type:    "vless",
		Server:  parsed.Hostname(),
		Port:    getPort(parsed),
		UUID:    uuid,
		Network: network,
		UDP:     true,
	}

	// Security 配置
	security := query.Get("security")
	node.Security = security
	if security == "tls" || security == "xtls" || security == "reality" {
		node.TLS = true
	}

	// Flow 配置
	flow := query.Get("flow")
	if flow != "" {
		node.Flow = flow
	}

	// SNI 配置
	sni := query.Get("sni")
	if sni == "" {
		sni = query.Get("host")
	}
	if sni != "" {
		node.SNI = sni
	}

	// ALPN 配置
	alpn := query.Get("alpn")
	if alpn != "" {
		node.ALPN = alpn
	}

	// Reality 配置
	if security == "reality" {
		pbk := query.Get("pbk")
		if pbk != "" {
			node.RealityPublicKey = pbk
		}
		sid := query.Get("sid")
		if sid != "" {
			node.RealityShortID = sid
		}
		fp := query.Get("fp")
		if fp != "" {
			node.Fingerprint = fp
		}
	}

	// gRPC 配置
	if network == "grpc" {
		serviceName := query.Get("serviceName")
		if serviceName != "" {
			node.ServiceName = serviceName
		}
	}

	// WebSocket 配置
	if network == "ws" {
		wsHost := query.Get("host")
		if wsHost != "" {
			node.WSHost = wsHost
		}
		wsPath := query.Get("path")
		if wsPath != "" {
			node.WSPath = wsPath
		}
	}

	return node, nil
}

// parseTrojan 解析 Trojan 链接
func parseTrojan(link string) (*ProxyNode, error) {
	parsed, err := url.Parse(link)
	if err != nil {
		return nil, err
	}

	password := parsed.User.Username()
	if password == "" {
		return nil, fmt.Errorf("缺少密码")
	}

	node := &ProxyNode{
		Name:     getFragment(parsed, fmt.Sprintf("Trojan-%s:%s", parsed.Hostname(), parsed.Port())),
		Type:     "trojan",
		Server:   parsed.Hostname(),
		Port:     getPort(parsed),
		Password: password,
		TLS:      true,
		UDP:      true,
	}

	// SNI 配置
	query := parsed.Query()
	sni := query.Get("sni")
	if sni == "" {
		sni = query.Get("peer")
	}
	if sni != "" {
		node.SNI = sni
	}

	// ALPN 配置
	alpn := query.Get("alpn")
	if alpn != "" {
		node.ALPN = alpn
	}

	return node, nil
}

// parseShadowsocks 解析 Shadowsocks 链接
func parseShadowsocks(link string) (*ProxyNode, error) {
	// 处理 URL 编码
	decodedLink, err := url.QueryUnescape(link)
	if err == nil {
		link = decodedLink
	}

	// 特殊格式：ss://base64(method:password@server:port)#remark
	// 例如：ss://YWVzLTI1Ni1nY206YzVUYzNGN1c0NFdjQmRFREA5Ni40NS4xODguMzM6MTUxMzA#JMS-1268850@...
	// 先尝试这种格式
	if strings.HasPrefix(link, "ss://") {
		// 移除 ss:// 前缀
		rest := strings.TrimPrefix(link, "ss://")

		// 查找 # 分隔符（如果有备注）
		var base64Part string
		if idx := strings.Index(rest, "#"); idx != -1 {
			base64Part = rest[:idx]
		} else {
			base64Part = rest
		}

		// 尝试解码 base64
		decoded, err := safeBase64Decode(base64Part)
		if err == nil {
			// 解码后格式应该是：method:password@server:port
			if strings.Contains(decoded, "@") {
				// 分割为认证部分和地址部分
				parts := strings.SplitN(decoded, "@", 2)
				if len(parts) == 2 {
					authPart := parts[0] // method:password
					addrPart := parts[1] // server:port

					// 解析认证信息
					if strings.Contains(authPart, ":") {
						authParts := strings.SplitN(authPart, ":", 2)
						method := authParts[0]
						password := authParts[1]

						// 解析地址
						if strings.Contains(addrPart, ":") {
							addrParts := strings.SplitN(addrPart, ":", 2)
							server := addrParts[0]
							portStr := addrParts[1]

							port, err := strconv.Atoi(portStr)
							if err != nil {
								return nil, fmt.Errorf("无效的端口: %s", portStr)
							}

							// 获取备注（如果有）
							name := getFragmentFromLink(link)
							if name == "" {
								name = fmt.Sprintf("SS-%s:%d", server, port)
							}

							node := &ProxyNode{
								Name:     name,
								Type:     "ss",
								Server:   server,
								Port:     port,
								Cipher:   method,
								Password: password,
							}

							// 尝试从原始链接解析插件信息（如果有查询参数）
							if parsed, err := url.Parse(link); err == nil {
								query := parsed.Query()
								if plugin := query.Get("plugin"); plugin != "" {
									node.Plugin = plugin
									var pluginOpts []string
									if path := query.Get("path"); path != "" {
										pluginOpts = append(pluginOpts, "path="+path)
									}
									if host := query.Get("host"); host != "" {
										pluginOpts = append(pluginOpts, "host="+host)
									}
									if tls := query.Get("tls"); tls != "" {
										pluginOpts = append(pluginOpts, "tls")
									}
									if obfs := query.Get("obfs"); obfs != "" {
										pluginOpts = append(pluginOpts, "obfs="+obfs)
									}
									if obfsHost := query.Get("obfs-host"); obfsHost != "" {
										pluginOpts = append(pluginOpts, "obfs-host="+obfsHost)
									}
									if len(pluginOpts) > 0 {
										node.PluginOpts = strings.Join(pluginOpts, ";")
									} else if strings.Contains(plugin, ";") {
										node.PluginOpts = plugin
									}
								}
							}

							return node, nil
						}
					}
				}
			}
		}
	}

	// 如果特殊格式解析失败，尝试标准格式
	parsed, err := url.Parse(link)
	if err != nil {
		return nil, err
	}

	// 解析认证信息
	var method, password string
	if parsed.User != nil {
		authInfo := parsed.User.String()
		// URL 解码
		authInfo, _ = url.QueryUnescape(authInfo)

		if strings.Contains(authInfo, ":") {
			parts := strings.SplitN(authInfo, ":", 2)
			method = parts[0]
			password = parts[1]
		} else {
			// 可能是 Base64 编码的 method:password
			decoded, err := safeBase64Decode(authInfo)
			if err == nil && strings.Contains(decoded, ":") {
				parts := strings.SplitN(decoded, ":", 2)
				method = parts[0]
				password = parts[1]
			} else {
				// 如果解码失败，尝试直接使用（可能是纯文本密码）
				method = "aes-256-gcm" // 默认加密方法
				password = authInfo
			}
		}
	}

	// 如果还是没有认证信息，尝试从 host 部分解析
	if method == "" || password == "" {
		// 某些 SS 链接格式可能是 ss://base64( method:password@host:port )
		if strings.Contains(link, "@") {
			parts := strings.SplitN(link, "@", 2)
			if len(parts) == 2 {
				authPart := strings.TrimPrefix(parts[0], "ss://")
				decoded, err := safeBase64Decode(authPart)
				if err == nil && strings.Contains(decoded, ":") {
					authParts := strings.SplitN(decoded, ":", 2)
					method = authParts[0]
					password = authParts[1]
				}
			}
		}
	}

	if method == "" || password == "" {
		return nil, fmt.Errorf("缺少认证信息")
	}

	node := &ProxyNode{
		Name:     getFragment(parsed, fmt.Sprintf("SS-%s:%s", parsed.Hostname(), parsed.Port())),
		Type:     "ss",
		Server:   parsed.Hostname(),
		Port:     getPort(parsed),
		Cipher:   method,
		Password: password,
	}

	// 解析 SS 插件参数
	query := parsed.Query()
	if plugin := query.Get("plugin"); plugin != "" {
		// 插件格式可能是: plugin=v2ray-plugin;path=/path;host=example.com;tls
		// 或者: plugin=obfs-local;obfs=http;obfs-host=example.com
		node.Plugin = plugin

		// 解析插件选项
		var pluginOpts []string
		if path := query.Get("path"); path != "" {
			pluginOpts = append(pluginOpts, "path="+path)
		}
		if host := query.Get("host"); host != "" {
			pluginOpts = append(pluginOpts, "host="+host)
		}
		if tls := query.Get("tls"); tls != "" {
			pluginOpts = append(pluginOpts, "tls")
		}
		if obfs := query.Get("obfs"); obfs != "" {
			pluginOpts = append(pluginOpts, "obfs="+obfs)
		}
		if obfsHost := query.Get("obfs-host"); obfsHost != "" {
			pluginOpts = append(pluginOpts, "obfs-host="+obfsHost)
		}
		if mux := query.Get("mux"); mux != "" {
			pluginOpts = append(pluginOpts, "mux="+mux)
		}
		if mode := query.Get("mode"); mode != "" {
			pluginOpts = append(pluginOpts, "mode="+mode)
		}

		// 如果插件参数中包含分号，说明是完整格式
		if strings.Contains(plugin, ";") {
			node.PluginOpts = plugin
		} else if len(pluginOpts) > 0 {
			node.PluginOpts = strings.Join(pluginOpts, ";")
		}
	}

	return node, nil
}

// getFragmentFromLink 从链接中提取备注（# 后面的内容）
func getFragmentFromLink(link string) string {
	if idx := strings.Index(link, "#"); idx != -1 {
		fragment := link[idx+1:]
		decoded, err := url.QueryUnescape(fragment)
		if err == nil {
			return decoded
		}
		return fragment
	}
	return ""
}

// 辅助函数
func safeBase64Decode(s string) (string, error) {
	// 清理文本
	clean := strings.ReplaceAll(s, " ", "")
	clean = strings.ReplaceAll(clean, "\n", "")
	clean = strings.ReplaceAll(clean, "\r", "")
	clean = strings.ReplaceAll(clean, "-", "+")
	clean = strings.ReplaceAll(clean, "_", "/")

	// 补全 padding
	if len(clean)%4 != 0 {
		clean += strings.Repeat("=", 4-len(clean)%4)
	}

	decoded, err := base64.StdEncoding.DecodeString(clean)
	if err != nil {
		return "", err
	}

	return string(decoded), nil
}

func getString(m map[string]interface{}, key, defaultValue string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return defaultValue
}

func getFragment(parsed *url.URL, defaultValue string) string {
	if parsed.Fragment != "" {
		decoded, err := url.QueryUnescape(parsed.Fragment)
		if err == nil {
			if decoded != "" && decoded != parsed.Fragment {
				return decoded
			}
			return decoded
		}
		return parsed.Fragment
	}
	return defaultValue
}

// parseSSR 解析 SSR 链接
func parseSSR(link string) (*ProxyNode, error) {
	// SSR 格式: ssr://base64(server:port:protocol:method:obfs:base64(password)/?obfsparam=base64(obfsparam)&protoparam=base64(protoparam)&remarks=base64(remarks))
	encoded := strings.TrimPrefix(link, "ssr://")

	// URL 解码
	decoded, err := url.QueryUnescape(encoded)
	if err == nil {
		encoded = decoded
	}

	// Base64 解码
	decodedBytes, err := base64.URLEncoding.DecodeString(encoded)
	if err != nil {
		decodedBytes, err = base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, fmt.Errorf("SSR Base64 解码失败: %v", err)
		}
	}

	decodedStr := string(decodedBytes)

	// 解析 SSR 格式
	parts := strings.Split(decodedStr, "/")
	if len(parts) < 1 {
		return nil, fmt.Errorf("无效的 SSR 格式")
	}

	mainPart := parts[0]
	params := ""
	if len(parts) > 1 {
		params = strings.Join(parts[1:], "/")
	}

	// 解析主部分: server:port:protocol:method:obfs:password
	mainParts := strings.Split(mainPart, ":")
	if len(mainParts) < 6 {
		return nil, fmt.Errorf("SSR 格式不完整")
	}

	server := mainParts[0]
	port, err := strconv.Atoi(mainParts[1])
	if err != nil {
		return nil, fmt.Errorf("无效的端口: %s", mainParts[1])
	}

	protocol := mainParts[2]
	method := mainParts[3]
	obfs := mainParts[4]
	passwordBase64 := strings.Join(mainParts[5:], ":")

	// 解码密码
	passwordBytes, err := base64.URLEncoding.DecodeString(passwordBase64)
	if err != nil {
		passwordBytes, err = base64.StdEncoding.DecodeString(passwordBase64)
		if err != nil {
			return nil, fmt.Errorf("密码解码失败: %v", err)
		}
	}
	password := string(passwordBytes)

	// 解析参数
	var obfsParam, protocolParam, remarks string
	if params != "" {
		parsedParams, _ := url.Parse("?" + params)
		query := parsedParams.Query()

		if obfsParamBase64 := query.Get("obfsparam"); obfsParamBase64 != "" {
			decoded, _ := base64.URLEncoding.DecodeString(obfsParamBase64)
			if len(decoded) > 0 {
				obfsParam = string(decoded)
			}
		}

		if protocolParamBase64 := query.Get("protoparam"); protocolParamBase64 != "" {
			decoded, _ := base64.URLEncoding.DecodeString(protocolParamBase64)
			if len(decoded) > 0 {
				protocolParam = string(decoded)
			}
		}

		if remarksBase64 := query.Get("remarks"); remarksBase64 != "" {
			decoded, _ := base64.URLEncoding.DecodeString(remarksBase64)
			if len(decoded) > 0 {
				remarks = string(decoded)
			}
		}
	}

	if remarks == "" {
		remarks = fmt.Sprintf("SSR-%s:%d", server, port)
	}

	return &ProxyNode{
		Name:          remarks,
		Type:          "ssr",
		Server:        server,
		Port:          port,
		Password:      password,
		Cipher:        method,
		Protocol:      protocol,
		ProtocolParam: protocolParam,
		Obfs:          obfs,
		ObfsParam:     obfsParam,
	}, nil
}

// parseHysteria 解析 Hysteria 链接
func parseHysteria(link string) (*ProxyNode, error) {
	parsed, err := url.Parse(link)
	if err != nil {
		return nil, err
	}

	query := parsed.Query()

	port := getPort(parsed)
	auth := query.Get("auth")
	obfsPassword := query.Get("obfs")

	name := getFragment(parsed, fmt.Sprintf("Hysteria-%s:%d", parsed.Hostname(), port))

	return &ProxyNode{
		Name:         name,
		Type:         "hysteria",
		Server:       parsed.Hostname(),
		Port:         port,
		Auth:         auth,
		ObfsPassword: obfsPassword,
		UDP:          true,
	}, nil
}

// parseHysteria2 解析 Hysteria2 链接
func parseHysteria2(link string) (*ProxyNode, error) {
	parsed, err := url.Parse(link)
	if err != nil {
		return nil, err
	}

	// Hysteria2 格式: hysteria2://password@host:port?params
	password := parsed.User.Username()
	if password == "" {
		return nil, fmt.Errorf("缺少密码")
	}

	query := parsed.Query()
	port := getPort(parsed)
	name := getFragment(parsed, fmt.Sprintf("Hysteria2-%s:%d", parsed.Hostname(), port))

	node := &ProxyNode{
		Name:     name,
		Type:     "hysteria2",
		Server:   parsed.Hostname(),
		Port:     port,
		Password: password,
		UDP:      true,
	}

	// SNI
	if sni := query.Get("sni"); sni != "" {
		node.SNI = sni
	}

	// ALPN
	if alpn := query.Get("alpn"); alpn != "" {
		node.ALPN = alpn
	}

	// Obfs (混淆)
	if obfs := query.Get("obfs"); obfs != "" {
		node.ObfsPassword = obfs
	}

	// insecure/allowInsecure
	insecure := query.Get("insecure") == "1" || query.Get("allowInsecure") == "1"
	if !insecure {
		node.TLS = true
	}

	return node, nil
}

// parseWireGuard 解析 WireGuard 链接
func parseWireGuard(link string) (*ProxyNode, error) {
	parsed, err := url.Parse(link)
	if err != nil {
		return nil, err
	}

	privateKey := parsed.User.Username()
	if privateKey == "" {
		return nil, fmt.Errorf("缺少私钥")
	}

	query := parsed.Query()
	publicKey := query.Get("publickey")
	reserved := query.Get("reserved")

	port := getPort(parsed)
	name := getFragment(parsed, fmt.Sprintf("WireGuard-%s:%d", parsed.Hostname(), port))

	return &ProxyNode{
		Name:       name,
		Type:       "wireguard",
		Server:     parsed.Hostname(),
		Port:       port,
		PrivateKey: privateKey,
		PublicKey:  publicKey,
		Reserved:   reserved,
		UDP:        true,
	}, nil
}

// parseTUIC 解析 TUIC 链接
func parseTUIC(link string) (*ProxyNode, error) {
	parsed, err := url.Parse(link)
	if err != nil {
		return nil, err
	}

	uuid := parsed.User.Username()
	if uuid == "" {
		return nil, fmt.Errorf("缺少 UUID")
	}

	query := parsed.Query()
	token := query.Get("token")
	password := query.Get("password")
	if password == "" {
		password = uuid
	}

	port := getPort(parsed)
	name := getFragment(parsed, fmt.Sprintf("TUIC-%s:%d", parsed.Hostname(), port))

	return &ProxyNode{
		Name:     name,
		Type:     "tuic",
		Server:   parsed.Hostname(),
		Port:     port,
		UUID:     uuid,
		Password: password,
		Token:    token,
		UDP:      true,
	}, nil
}

// parseHTTP 解析 HTTP 代理节点
func parseHTTP(link string) (*ProxyNode, error) {
	parsed, err := url.Parse(link)
	if err != nil {
		return nil, err
	}

	port := getPort(parsed)
	name := getFragment(parsed, fmt.Sprintf("HTTP-%s:%d", parsed.Hostname(), port))

	node := &ProxyNode{
		Name:   name,
		Type:   "http",
		Server: parsed.Hostname(),
		Port:   port,
	}

	// 解析认证信息
	if parsed.User != nil {
		node.Password = parsed.User.Username()
		if pwd, ok := parsed.User.Password(); ok {
			node.Password = pwd
		}
	}

	return node, nil
}

// parseSOCKS 解析 SOCKS 代理节点
func parseSOCKS(link string) (*ProxyNode, error) {
	parsed, err := url.Parse(link)
	if err != nil {
		return nil, err
	}

	port := getPort(parsed)
	scheme := parsed.Scheme
	socksType := "socks5"
	if scheme == "socks4" {
		socksType = "socks4"
	} else if scheme == "socks" {
		socksType = "socks5" // 默认 SOCKS5
	}

	name := getFragment(parsed, fmt.Sprintf("SOCKS-%s:%d", parsed.Hostname(), port))

	node := &ProxyNode{
		Name:   name,
		Type:   socksType,
		Server: parsed.Hostname(),
		Port:   port,
	}

	// 解析认证信息
	if parsed.User != nil {
		node.Password = parsed.User.Username()
		if pwd, ok := parsed.User.Password(); ok {
			node.Password = pwd
		}
	}

	return node, nil
}

// parseAnyTLS 解析 AnyTLS 节点
func parseAnyTLS(link string) (*ProxyNode, error) {
	// AnyTLS 格式类似 vless/vmess，但使用 anytls 作为传输方式
	// 示例: anytls://uuid@server:port?version=1&padding=random#name
	parsed, err := url.Parse(link)
	if err != nil {
		return nil, err
	}

	uuid := parsed.User.Username()
	if uuid == "" {
		return nil, fmt.Errorf("缺少 UUID")
	}

	query := parsed.Query()
	port := getPort(parsed)
	name := getFragment(parsed, fmt.Sprintf("AnyTLS-%s:%d", parsed.Hostname(), port))

	node := &ProxyNode{
		Name:   name,
		Type:   "anytls",
		Server: parsed.Hostname(),
		Port:   port,
		UUID:   uuid,
		TLS:    true,
		UDP:    true,
	}

	// 解析 AnyTLS 特定参数
	if version := query.Get("version"); version != "" {
		node.AnyTLSVersion = version
	}
	if padding := query.Get("padding"); padding != "" {
		node.AnyTLSPadding = padding
	}

	// 解析通用参数
	if sni := query.Get("sni"); sni != "" {
		node.SNI = sni
	}
	if alpn := query.Get("alpn"); alpn != "" {
		node.ALPN = alpn
	}
	if security := query.Get("security"); security != "" {
		node.Security = security
	}

	return node, nil
}

// parseGOST 解析 GOST 节点
func parseGOST(link string) (*ProxyNode, error) {
	// GOST 格式: gost://protocol+transport://user:pass@host:port?param=value
	// 或者: gost+protocol+transport://user:pass@host:port
	parsed, err := url.Parse(link)
	if err != nil {
		return nil, err
	}

	port := getPort(parsed)
	scheme := parsed.Scheme

	// 解析协议类型
	protocol := "auto"
	if strings.HasPrefix(scheme, "gost+") {
		// gost+http+tls:// 格式
		parts := strings.Split(strings.TrimPrefix(scheme, "gost+"), "+")
		if len(parts) > 0 {
			protocol = parts[0]
		}
	} else if scheme == "gost" {
		// gost:// 格式，从查询参数获取协议
		query := parsed.Query()
		if p := query.Get("protocol"); p != "" {
			protocol = p
		}
	}

	name := getFragment(parsed, fmt.Sprintf("GOST-%s:%d", parsed.Hostname(), port))

	node := &ProxyNode{
		Name:         name,
		Type:         "gost",
		Server:       parsed.Hostname(),
		Port:         port,
		GOSTProtocol: protocol,
	}

	// 解析认证信息
	if parsed.User != nil {
		node.Password = parsed.User.Username()
		if pwd, ok := parsed.User.Password(); ok {
			node.Password = pwd
		}
	}

	// 解析查询参数
	query := parsed.Query()
	if path := query.Get("path"); path != "" {
		node.GOSTPath = path
	}
	if sni := query.Get("sni"); sni != "" {
		node.SNI = sni
	}
	if tls := query.Get("tls"); tls == "true" || tls == "1" {
		node.TLS = true
	}

	return node, nil
}

func getPort(parsed *url.URL) int {
	portStr := parsed.Port()
	if portStr == "" {
		// 根据协议推断默认端口
		switch parsed.Scheme {
		case "vmess", "vless", "trojan", "https":
			return 443
		case "ss", "ssr":
			return 8388
		case "http":
			return 80
		case "socks", "socks4", "socks5":
			return 1080
		default:
			return 443
		}
	}
	port, _ := strconv.Atoi(portStr)
	return port
}
