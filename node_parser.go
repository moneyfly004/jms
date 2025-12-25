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

	// TLS 配置
	if tls, ok := data["tls"].(string); ok && tls == "tls" {
		node.TLS = true
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

	// TLS 配置
	security := query.Get("security")
	if security == "tls" || security == "xtls" || security == "reality" {
		node.TLS = true
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
					authPart := parts[0]  // method:password
					addrPart := parts[1]  // server:port
					
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
							
							return &ProxyNode{
								Name:     name,
								Type:     "ss",
								Server:   server,
								Port:     port,
								Cipher:   method,
								Password: password,
							}, nil
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

func getPort(parsed *url.URL) int {
	portStr := parsed.Port()
	if portStr == "" {
		// 根据协议推断默认端口
		switch parsed.Scheme {
		case "vmess", "vless", "trojan":
			return 443
		case "ss", "ssr":
			return 8388
		default:
			return 443
		}
	}
	port, _ := strconv.Atoi(portStr)
	return port
}

