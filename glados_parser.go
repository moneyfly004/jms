package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// ClashConfig Clash 配置结构
type ClashConfig struct {
	Proxies []ClashProxy `yaml:"proxies"`
}

// ClashProxy Clash 代理节点结构
type ClashProxy struct {
	Name       string                 `yaml:"name"`
	Type       string                 `yaml:"type"`
	Server     string                 `yaml:"server"`
	Port       interface{}            `yaml:"port"` // 可能是 int 或 string
	UUID       string                 `yaml:"uuid"`
	Password   string                 `yaml:"password"`
	Cipher     string                 `yaml:"cipher"`
	Network    string                 `yaml:"network"`
	TLS        bool                   `yaml:"tls"`
	SNI        string                 `yaml:"sni"`
	ALPN       []string               `yaml:"alpn"`
	UDP        bool                   `yaml:"udp"`
	Flow       string                 `yaml:"flow"`
	Plugin     string                 `yaml:"plugin"`     // SS 插件名称（如 obfs）
	PluginOpts map[string]interface{} `yaml:"plugin-opts"` // SS 插件选项
	WSOpts     map[string]interface{} `yaml:"ws-opts"`
	GrpcOpts   map[string]interface{} `yaml:"grpc-opts"`
	Reality    map[string]interface{} `yaml:"reality-opts"`
	Other      map[string]interface{} `yaml:",inline"`
}

// ParseGladosLink 解析 glados 链接并提取节点
func (c *Collector) ParseGladosLink(link string) ([]string, error) {
	// 判断链接类型
	if strings.Contains(link, "/singbox/") {
		return c.parseSingBoxLink(link)
	} else if strings.Contains(link, "/clash/") {
		// 将 clash 链接转换为 v2ray 链接，更容易解析
		v2rayLink := c.convertClashToV2rayLink(link)
		if v2rayLink != "" {
			log.Printf("将 clash 链接转换为 v2ray 链接: %s -> %s", link, v2rayLink)
			return c.parseV2rayLink(v2rayLink)
		}
		// 如果转换失败，回退到原来的 clash 解析方式
		return c.parseClashLink(link)
	} else if strings.Contains(link, "/subscribe/") {
		return c.parseSubscribeLink(link)
	}
	return nil, fmt.Errorf("不支持的 glados 链接格式: %s", link)
}

// parseSingBoxLink 解析 sing-box 格式的链接
func (c *Collector) parseSingBoxLink(link string) ([]string, error) {
	// 获取内容
	content, err := c.FetchSubscription(link)
	if err != nil {
		return nil, fmt.Errorf("获取内容失败: %v", err)
	}

	// sing-box 配置通常是 JSON 格式
	var config map[string]interface{}
	if err := json.Unmarshal([]byte(content), &config); err != nil {
		// 如果不是 JSON，尝试作为节点链接列表解析
		nodes := c.parseNodeLinksFromContent(content)
		return nodes, nil
	}

	// 解析 sing-box JSON 配置
	var nodes []string
	if outbounds, ok := config["outbounds"].([]interface{}); ok {
		for _, outbound := range outbounds {
			if outboundMap, ok := outbound.(map[string]interface{}); ok {
				nodeLink := c.convertSingBoxOutboundToLink(outboundMap)
				if nodeLink != "" {
					nodes = append(nodes, nodeLink)
				}
			}
		}
	}

	return nodes, nil
}

// parseClashLink 解析 Clash YAML 格式的链接
func (c *Collector) parseClashLink(link string) ([]string, error) {
	// 获取内容
	content, err := c.FetchSubscription(link)
	if err != nil {
		return nil, fmt.Errorf("获取内容失败: %v", err)
	}

	// 解析 YAML
	var config ClashConfig
	if err := yaml.Unmarshal([]byte(content), &config); err != nil {
		return nil, fmt.Errorf("YAML 解析失败: %v", err)
	}

	// 转换 Clash 节点为标准节点链接
	var nodes []string
	for _, proxy := range config.Proxies {
		nodeLink := c.convertClashProxyToLink(proxy)
		if nodeLink != "" {
			nodes = append(nodes, nodeLink)
		}
	}

	log.Printf("从 Clash 配置中提取到 %d 个节点", len(nodes))
	return nodes, nil
}

// convertClashProxyToLink 将 Clash 代理节点转换为标准节点链接
func (c *Collector) convertClashProxyToLink(proxy ClashProxy) string {
	// 处理端口（可能是 int 或 string）
	port := 0
	switch v := proxy.Port.(type) {
	case int:
		port = v
	case string:
		if p, err := strconv.Atoi(v); err == nil {
			port = p
		}
	case float64:
		port = int(v)
	}

	if port == 0 || proxy.Server == "" {
		return ""
	}

	// 根据类型转换
	switch strings.ToLower(proxy.Type) {
	case "ss", "shadowsocks":
		return c.convertClashSSToLink(proxy, port)
	case "vmess":
		return c.convertClashVMessToLink(proxy, port)
	case "vless":
		return c.convertClashVLESSToLink(proxy, port)
	case "trojan":
		return c.convertClashTrojanToLink(proxy, port)
	default:
		log.Printf("⚠️ 不支持的 Clash 代理类型: %s", proxy.Type)
		return ""
	}
}

// convertClashSSToLink 转换 Clash SS 节点
func (c *Collector) convertClashSSToLink(proxy ClashProxy, port int) string {
	if proxy.Cipher == "" || proxy.Password == "" {
		return ""
	}

	// 构建 SS 链接: ss://base64(method:password)@server:port?plugin=...#name
	auth := fmt.Sprintf("%s:%s", proxy.Cipher, proxy.Password)
	authBase64 := base64.StdEncoding.EncodeToString([]byte(auth))
	
	link := fmt.Sprintf("ss://%s@%s:%d", authBase64, proxy.Server, port)
	
	// 处理插件（如 obfs）
	if proxy.Plugin != "" && proxy.PluginOpts != nil {
		var pluginParts []string
		
		// obfs 插件处理
		if proxy.Plugin == "obfs" {
			pluginParts = append(pluginParts, "obfs-local")
			
			// 获取 mode (tls/http)
			if mode, ok := proxy.PluginOpts["mode"].(string); ok && mode != "" {
				pluginParts = append(pluginParts, "obfs="+mode)
			}
			
			// 获取 host
			if host, ok := proxy.PluginOpts["host"].(string); ok && host != "" {
				pluginParts = append(pluginParts, "obfs-host="+host)
			}
		} else {
			// 其他插件
			pluginParts = append(pluginParts, proxy.Plugin)
			// 可以在这里添加其他插件的处理逻辑
		}
		
		if len(pluginParts) > 0 {
			pluginStr := strings.Join(pluginParts, ";")
			link += "?plugin=" + url.QueryEscape(pluginStr)
		}
	}
	
	if proxy.Name != "" {
		link += "#" + url.QueryEscape(proxy.Name)
	}
	
	return link
}

// convertClashVMessToLink 转换 Clash VMess 节点
func (c *Collector) convertClashVMessToLink(proxy ClashProxy, port int) string {
	if proxy.UUID == "" {
		return ""
	}

	// 构建 VMess JSON
	vmessData := map[string]interface{}{
		"v":    "2",
		"ps":   proxy.Name,
		"add":  proxy.Server,
		"port": port,
		"id":   proxy.UUID,
		"aid":  0,
		"net":  func() string {
			if proxy.Network != "" {
				return proxy.Network
			}
			return "tcp"
		}(),
		"type": "none",
		"tls":  func() string {
			if proxy.TLS {
				return "tls"
			}
			return ""
		}(),
	}

	// 添加 SNI
	if proxy.SNI != "" {
		vmessData["sni"] = proxy.SNI
	}

	// 添加 WebSocket 配置
	if proxy.Network == "ws" && proxy.WSOpts != nil {
		if path, ok := proxy.WSOpts["path"].(string); ok && path != "" {
			vmessData["path"] = path
		}
		if headers, ok := proxy.WSOpts["headers"].(map[string]interface{}); ok {
			if host, ok := headers["Host"].(string); ok && host != "" {
				vmessData["host"] = host
			}
		}
	}

	// 转换为 JSON 并 Base64 编码
	jsonData, err := json.Marshal(vmessData)
	if err != nil {
		return ""
	}

	return "vmess://" + base64.StdEncoding.EncodeToString(jsonData)
}

// convertClashVLESSToLink 转换 Clash VLESS 节点
func (c *Collector) convertClashVLESSToLink(proxy ClashProxy, port int) string {
	if proxy.UUID == "" {
		return ""
	}

	link := fmt.Sprintf("vless://%s@%s:%d", proxy.UUID, proxy.Server, port)
	
	query := url.Values{}
	if proxy.Network != "" && proxy.Network != "tcp" {
		query.Set("type", proxy.Network)
	}
	if proxy.TLS {
		query.Set("security", "tls")
	}
	if proxy.SNI != "" {
		query.Set("sni", proxy.SNI)
	}
	if proxy.Flow != "" {
		query.Set("flow", proxy.Flow)
	}

	// WebSocket 配置
	if proxy.Network == "ws" && proxy.WSOpts != nil {
		if path, ok := proxy.WSOpts["path"].(string); ok && path != "" {
			query.Set("path", path)
		}
		if headers, ok := proxy.WSOpts["headers"].(map[string]interface{}); ok {
			if host, ok := headers["Host"].(string); ok && host != "" {
				query.Set("host", host)
			}
		}
	}

	// gRPC 配置
	if proxy.Network == "grpc" && proxy.GrpcOpts != nil {
		if serviceName, ok := proxy.GrpcOpts["grpc-service-name"].(string); ok && serviceName != "" {
			query.Set("serviceName", serviceName)
		}
	}

	// Reality 配置
	if proxy.Reality != nil {
		query.Set("security", "reality")
		if pbk, ok := proxy.Reality["public-key"].(string); ok && pbk != "" {
			query.Set("pbk", pbk)
		}
		if sid, ok := proxy.Reality["short-id"].(string); ok && sid != "" {
			query.Set("sid", sid)
		}
		if fp, ok := proxy.Reality["fingerprint"].(string); ok && fp != "" {
			query.Set("fp", fp)
		}
		if sni, ok := proxy.Reality["server-name"].(string); ok && sni != "" {
			query.Set("sni", sni)
		}
	}

	if len(query) > 0 {
		link += "?" + query.Encode()
	}

	if proxy.Name != "" {
		link += "#" + url.QueryEscape(proxy.Name)
	}

	return link
}

// convertClashTrojanToLink 转换 Clash Trojan 节点
func (c *Collector) convertClashTrojanToLink(proxy ClashProxy, port int) string {
	if proxy.Password == "" {
		return ""
	}

	link := fmt.Sprintf("trojan://%s@%s:%d", proxy.Password, proxy.Server, port)
	
	query := url.Values{}
	if proxy.SNI != "" {
		query.Set("sni", proxy.SNI)
	}
	if proxy.Network != "" && proxy.Network != "tcp" {
		query.Set("type", proxy.Network)
	}

	// WebSocket 配置
	if proxy.Network == "ws" && proxy.WSOpts != nil {
		if path, ok := proxy.WSOpts["path"].(string); ok && path != "" {
			query.Set("path", path)
		}
		if headers, ok := proxy.WSOpts["headers"].(map[string]interface{}); ok {
			if host, ok := headers["Host"].(string); ok && host != "" {
				query.Set("host", host)
			}
		}
	}

	if len(query) > 0 {
		link += "?" + query.Encode()
	}

	if proxy.Name != "" {
		link += "#" + url.QueryEscape(proxy.Name)
	}

	return link
}

// convertSingBoxOutboundToLink 将 sing-box outbound 转换为节点链接
func (c *Collector) convertSingBoxOutboundToLink(outbound map[string]interface{}) string {
	outboundType, ok := outbound["type"].(string)
	if !ok {
		return ""
	}

	server, _ := outbound["server"].(string)
	serverPort, _ := outbound["server_port"].(float64)
	port := int(serverPort)

	if server == "" || port == 0 {
		return ""
	}

	switch outboundType {
	case "shadowsocks":
		return c.convertSingBoxSSToLink(outbound, server, port)
	case "vmess":
		return c.convertSingBoxVMessToLink(outbound, server, port)
	case "vless":
		return c.convertSingBoxVLESSToLink(outbound, server, port)
	case "trojan":
		return c.convertSingBoxTrojanToLink(outbound, server, port)
	}

	return ""
}

// convertSingBoxSSToLink 转换 sing-box SS 节点
func (c *Collector) convertSingBoxSSToLink(outbound map[string]interface{}, server string, port int) string {
	method, _ := outbound["method"].(string)
	password, _ := outbound["password"].(string)

	if method == "" || password == "" {
		return ""
	}

	auth := fmt.Sprintf("%s:%s", method, password)
	authBase64 := base64.StdEncoding.EncodeToString([]byte(auth))
	return fmt.Sprintf("ss://%s@%s:%d", authBase64, server, port)
}

// convertSingBoxVMessToLink 转换 sing-box VMess 节点
func (c *Collector) convertSingBoxVMessToLink(outbound map[string]interface{}, server string, port int) string {
	uuid, _ := outbound["uuid"].(string)
	if uuid == "" {
		return ""
	}

	vmessData := map[string]interface{}{
		"v":    "2",
		"add":  server,
		"port": port,
		"id":   uuid,
		"aid":  0,
		"net":  "tcp",
		"type": "none",
		"tls":  "",
	}

	if security, ok := outbound["security"].(string); ok && security != "" {
		vmessData["scy"] = security
	}

	if network, ok := outbound["network"].(string); ok && network != "" {
		vmessData["net"] = network
	}

	if tls, ok := outbound["tls"].(map[string]interface{}); ok {
		if enabled, ok := tls["enabled"].(bool); ok && enabled {
			vmessData["tls"] = "tls"
			if sni, ok := tls["server_name"].(string); ok && sni != "" {
				vmessData["sni"] = sni
			}
		}
	}

	jsonData, _ := json.Marshal(vmessData)
	return "vmess://" + base64.StdEncoding.EncodeToString(jsonData)
}

// convertSingBoxVLESSToLink 转换 sing-box VLESS 节点
func (c *Collector) convertSingBoxVLESSToLink(outbound map[string]interface{}, server string, port int) string {
	uuid, _ := outbound["uuid"].(string)
	if uuid == "" {
		return ""
	}

	link := fmt.Sprintf("vless://%s@%s:%d", uuid, server, port)
	query := url.Values{}

	if flow, ok := outbound["flow"].(string); ok && flow != "" {
		query.Set("flow", flow)
	}

	if transport, ok := outbound["transport"].(map[string]interface{}); ok {
		if transportType, ok := transport["type"].(string); ok && transportType != "tcp" {
			query.Set("type", transportType)
			
			if transportType == "ws" {
				if path, ok := transport["path"].(string); ok && path != "" {
					query.Set("path", path)
				}
				if headers, ok := transport["headers"].(map[string]interface{}); ok {
					if host, ok := headers["Host"].(string); ok && host != "" {
						query.Set("host", host)
					}
				}
			} else if transportType == "grpc" {
				if serviceName, ok := transport["service_name"].(string); ok && serviceName != "" {
					query.Set("serviceName", serviceName)
				}
			}
		}
	}

	if reality, ok := outbound["reality"].(map[string]interface{}); ok {
		query.Set("security", "reality")
		if pbk, ok := reality["public_key"].(string); ok && pbk != "" {
			query.Set("pbk", pbk)
		}
		if sid, ok := reality["short_id"].(string); ok && sid != "" {
			query.Set("sid", sid)
		}
		if fp, ok := reality["fingerprint"].(string); ok && fp != "" {
			query.Set("fp", fp)
		}
		if sni, ok := reality["server_name"].(string); ok && sni != "" {
			query.Set("sni", sni)
		}
	} else if tls, ok := outbound["tls"].(map[string]interface{}); ok {
		if enabled, ok := tls["enabled"].(bool); ok && enabled {
			query.Set("security", "tls")
			if sni, ok := tls["server_name"].(string); ok && sni != "" {
				query.Set("sni", sni)
			}
		}
	}

	if len(query) > 0 {
		link += "?" + query.Encode()
	}

	return link
}

// convertSingBoxTrojanToLink 转换 sing-box Trojan 节点
func (c *Collector) convertSingBoxTrojanToLink(outbound map[string]interface{}, server string, port int) string {
	password, _ := outbound["password"].(string)
	if password == "" {
		return ""
	}

	link := fmt.Sprintf("trojan://%s@%s:%d", password, server, port)
	query := url.Values{}

	if tls, ok := outbound["tls"].(map[string]interface{}); ok {
		if sni, ok := tls["server_name"].(string); ok && sni != "" {
			query.Set("sni", sni)
		}
	}

	if len(query) > 0 {
		link += "?" + query.Encode()
	}

	return link
}

// parseNodeLinksFromContent 从内容中解析节点链接
func (c *Collector) parseNodeLinksFromContent(content string) []string {
	lines := strings.Split(content, "\n")
	var nodes []string
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		
		// 检查是否是节点链接
		if strings.HasPrefix(line, "ss://") ||
			strings.HasPrefix(line, "vmess://") ||
			strings.HasPrefix(line, "vless://") ||
			strings.HasPrefix(line, "trojan://") {
			nodes = append(nodes, line)
		}
	}
	
	return nodes
}

// convertClashToV2rayLink 将 clash 链接转换为 v2ray 链接
// 例如: https://update.glados-config.com/clash/477901/f18d16d/131560/glados.yaml
// 转换为: https://update.glados-config.com/v2ray/477901/f18d16d/131560
func (c *Collector) convertClashToV2rayLink(clashLink string) string {
	// 匹配 clash 链接格式: /clash/{id1}/{id2}/{id3}/glados.yaml
	clashPattern := regexp.MustCompile(`(/clash/([^/]+)/([^/]+)/([^/]+)/glados\.yaml)`)
	matches := clashPattern.FindStringSubmatch(clashLink)
	if len(matches) >= 5 {
		// 提取 ID 部分
		id1 := matches[2]
		id2 := matches[3]
		id3 := matches[4]
		// 构建 v2ray 链接
		v2rayPath := fmt.Sprintf("/v2ray/%s/%s/%s", id1, id2, id3)
		// 替换 clash 路径为 v2ray 路径
		v2rayLink := strings.Replace(clashLink, matches[1], v2rayPath, 1)
		return v2rayLink
	}
	return ""
}

// parseV2rayLink 解析 v2ray 格式的链接（返回 base64 编码的节点列表）
func (c *Collector) parseV2rayLink(link string) ([]string, error) {
	// 获取内容
	content, err := c.FetchSubscription(link)
	if err != nil {
		return nil, fmt.Errorf("获取内容失败: %v", err)
	}

	// v2ray 格式返回的是 base64 编码的节点列表
	// 使用 ParseNodes 函数解析 base64 内容
	nodes, err := c.ParseNodes(content)
	if err != nil {
		// 如果 base64 解码失败，尝试从原始内容提取节点链接
		log.Printf("v2ray 链接 base64 解码失败: %v，尝试从原始内容提取", err)
		nodes = c.parseNodeLinksFromContent(content)
	}

	log.Printf("从 v2ray 链接中提取到 %d 个节点", len(nodes))
	return nodes, nil
}

// parseSubscribeLink 解析 subscribe 格式的链接（纯文本节点列表）
func (c *Collector) parseSubscribeLink(link string) ([]string, error) {
	// 获取内容
	content, err := c.FetchSubscription(link)
	if err != nil {
		return nil, fmt.Errorf("获取内容失败: %v", err)
	}

	// subscribe 格式通常是纯文本，每行一个节点链接
	// 也可能包含 base64 编码的内容
	nodes := c.parseNodeLinksFromContent(content)
	
	// 如果没有找到节点链接，尝试 base64 解码
	if len(nodes) == 0 {
		decodedNodes, err := c.ParseNodes(content)
		if err == nil && len(decodedNodes) > 0 {
			nodes = decodedNodes
		}
	}

	log.Printf("从 subscribe 链接中提取到 %d 个节点", len(nodes))
	return nodes, nil
}

