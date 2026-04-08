package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

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
	Insecure bool
	UDP      bool
	Protocol      string
	ProtocolParam string
	Obfs          string
	ObfsParam     string
	Auth         string
	ObfsPassword string
	PrivateKey   string
	PublicKey    string
	Reserved     string
	Token        string
	SNI          string
	ALPN         string
	Flow         string
	Security     string
	AlterID      int
	RealityPublicKey string
	RealityShortID   string
	Fingerprint      string
	ServiceName string
	WSHost      string
	WSPath      string
	Plugin      string
	PluginOpts  string
	AnyTLSVersion string
	AnyTLSPadding string
	GOSTProtocol string
	GOSTPath     string
}

func ParseNodeLink(link string) (*ProxyNode, error) {
	link = strings.TrimSpace(link)
	if strings.HasPrefix(link, "gost+") {
		return parseGOST(link)
	}

	scheme, _, found := strings.Cut(link, "://")
	if !found {
		return nil, fmt.Errorf("不支持的协议或格式错误")
	}

	switch strings.ToLower(scheme) {
	case "vmess":
		return parseVMess(link)
	case "ss":
		return parseShadowsocks(link)
	case "vless":
		return parseVLESS(link)
	case "trojan":
		return parseTrojan(link)
	case "ssr":
		return parseSSR(link)
	case "hysteria":
		return parseHysteria(link)
	case "hysteria2", "hy2":
		return parseHysteria2(link)
	case "wireguard", "wg":
		return parseWireGuard(link)
	case "tuic":
		return parseTUIC(link)
	case "http", "https":
		return parseHTTP(link)
	case "socks", "socks4", "socks5":
		return parseSOCKS(link)
	case "anytls":
		return parseAnyTLS(link)
	case "gost":
		return parseGOST(link)
	default:
		return nil, fmt.Errorf("不支持的协议: %s", scheme)
	}
}

func parseVMess(link string) (*ProxyNode, error) {
	encoded := strings.TrimPrefix(link, "vmess://")
	decoded, err := safeBase64Decode(encoded)
	if err != nil {
		return nil, fmt.Errorf("Base64 解码失败: %v", err)
	}

	var data map[string]interface{}
	if err := json.Unmarshal([]byte(decoded), &data); err != nil {
		return nil, fmt.Errorf("JSON 解析失败: %v", err)
	}

	server := getString(data, "add", "")
	if server == "" {
		return nil, fmt.Errorf("缺少服务器地址 (add)")
	}

	uuid := getString(data, "id", "")
	if uuid == "" {
		return nil, fmt.Errorf("缺少 UUID (id)")
	}

	var port int
	switch p := data["port"].(type) {
	case float64:
		port = int(p)
	case string:
		port, _ = strconv.Atoi(p)
	}
	if port <= 0 || port > 65535 {
		return nil, fmt.Errorf("无效的端口: %v", data["port"])
	}

	network := getString(data, "net", "tcp")
	node := &ProxyNode{
		Name:    getString(data, "ps", fmt.Sprintf("VMess-%s:%d", server, port)),
		Type:    "vmess",
		Server:  server,
		Port:    port,
		UUID:    uuid,
		Network: network,
		UDP:     true,
	}

	if scy := getString(data, "scy", ""); scy != "" {
		node.Security = scy
	} else if security := getString(data, "security", ""); security != "" {
		node.Security = security
	}

	if aid, ok := data["aid"].(float64); ok {
		node.AlterID = int(aid)
	} else if alterId, ok := data["alterId"].(float64); ok {
		node.AlterID = int(alterId)
	}

	switch t := data["tls"].(type) {
	case string:
		node.TLS = (t == "tls")
	case bool:
		node.TLS = t
	}

	if sni := getString(data, "sni", ""); sni != "" {
		node.SNI = sni
	} else if host := getString(data, "host", ""); host != "" {
		node.SNI = host
	}

	if alpn, ok := data["alpn"].(string); ok && alpn != "" {
		node.ALPN = alpn
	} else if alpnArr, ok := data["alpn"].([]interface{}); ok && len(alpnArr) > 0 {
		if alpnStr, ok := alpnArr[0].(string); ok {
			node.ALPN = alpnStr
		}
	}

	if network == "ws" || network == "websocket" {
		if path := getString(data, "path", ""); path != "" {
			node.WSPath = path
		}
		if host := getString(data, "host", ""); host != "" {
			if node.SNI == "" {
				node.SNI = host
			}
			node.WSHost = host
		}
	}

	return node, nil
}

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
	network := queryGetFirst(query, "type")
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

	parseCommonURLParams(node, query)

	security := query.Get("security")
	node.Security = security
	if security == "tls" || security == "xtls" || security == "reality" {
		node.TLS = true
	}

	if flow := query.Get("flow"); flow != "" {
		node.Flow = flow
	}

	if security == "reality" {
		node.RealityPublicKey = query.Get("pbk")
		node.RealityShortID = query.Get("sid")
		node.Fingerprint = query.Get("fp")
	}

	if network == "grpc" {
		node.ServiceName = query.Get("serviceName")
	} else if network == "ws" {
		node.WSHost = query.Get("host")
		node.WSPath = query.Get("path")
	}

	return node, nil
}

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

	parseCommonURLParams(node, parsed.Query())
	return node, nil
}

func parseShadowsocks(link string) (*ProxyNode, error) {
	if decodedLink, err := url.QueryUnescape(link); err == nil {
		link = decodedLink
	}
	var method, password string
	remark := getFragmentFromLink(link)

	rest := strings.TrimPrefix(link, "ss://")
	base64Part, hashPart, _ := strings.Cut(rest, "#")

	if !strings.Contains(base64Part, "@") {
		if decoded, err := safeBase64Decode(base64Part); err == nil && strings.Contains(decoded, "@") {
			link = "ss://" + decoded
			if hashPart != "" {
				link += "#" + hashPart
			}
		}
	}

	parsed, err := url.Parse(link)
	if err != nil {
		return nil, err
	}

	if parsed.User != nil {
		authInfo, _ := url.QueryUnescape(parsed.User.String())
		method, password, _ = strings.Cut(authInfo, ":")

		if password == "" {
			if decoded, err := safeBase64Decode(authInfo); err == nil && strings.Contains(decoded, ":") {
				method, password, _ = strings.Cut(decoded, ":")
			} else {
				method = "aes-256-gcm"
				password = authInfo
			}
		}
	}

	if method == "" || password == "" {
		if authPart, _, ok := strings.Cut(strings.TrimPrefix(link, "ss://"), "@"); ok {
			if decoded, err := safeBase64Decode(authPart); err == nil {
				method, password, _ = strings.Cut(decoded, ":")
			}
		}
	}

	if method == "" || password == "" {
		return nil, fmt.Errorf("缺少认证信息")
	}

	server := parsed.Hostname()
	port := getPort(parsed)

	if remark == "" {
		remark = fmt.Sprintf("SS-%s:%d", server, port)
	}

	node := &ProxyNode{
		Name:     remark,
		Type:     "ss",
		Server:   server,
		Port:     port,
		Cipher:   method,
		Password: password,
	}

	query := parsed.Query()
	if plugin := query.Get("plugin"); plugin != "" {
		node.Plugin = plugin
		if strings.Contains(plugin, ";") {
			node.PluginOpts = plugin
		} else {
			var opts []string
			for _, k := range []string{"path", "host", "obfs", "obfs-host", "mux", "mode"} {
				if v := query.Get(k); v != "" {
					opts = append(opts, k+"="+v)
				}
			}
			if query.Get("tls") != "" {
				opts = append(opts, "tls")
			}
			node.PluginOpts = strings.Join(opts, ";")
		}
	}

	return node, nil
}

func parseSSR(link string) (*ProxyNode, error) {
	encoded := strings.TrimPrefix(link, "ssr://")
	if decoded, err := url.QueryUnescape(encoded); err == nil {
		encoded = decoded
	}

	decodedBytes, err := base64.URLEncoding.DecodeString(encoded)
	if err != nil {
		decodedBytes, err = base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, fmt.Errorf("SSR Base64 解码失败: %v", err)
		}
	}

	decodedStr := string(decodedBytes)
	mainPart, paramsPart, _ := strings.Cut(decodedStr, "/?")

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

	pwDecoded, err := base64.URLEncoding.DecodeString(passwordBase64)
	if err != nil {
		pwDecoded, _ = base64.StdEncoding.DecodeString(passwordBase64)
	}
	password := string(pwDecoded)

	node := &ProxyNode{
		Type:     "ssr",
		Server:   server,
		Port:     port,
		Password: password,
		Cipher:   method,
		Protocol: protocol,
		Obfs:     obfs,
	}

	var remarks string
	if paramsPart != "" {
		if parsedParams, err := url.Parse("?" + paramsPart); err == nil {
			query := parsedParams.Query()
			if v := query.Get("obfsparam"); v != "" {
				if dec, _ := base64.URLEncoding.DecodeString(v); len(dec) > 0 {
					node.ObfsParam = string(dec)
				}
			}
			if v := query.Get("protoparam"); v != "" {
				if dec, _ := base64.URLEncoding.DecodeString(v); len(dec) > 0 {
					node.ProtocolParam = string(dec)
				}
			}
			if v := query.Get("remarks"); v != "" {
				if dec, _ := base64.URLEncoding.DecodeString(v); len(dec) > 0 {
					remarks = string(dec)
				}
			}
		}
	}

	if remarks == "" {
		remarks = fmt.Sprintf("SSR-%s:%d", server, port)
	}
	node.Name = remarks

	return node, nil
}

func parseHysteria(link string) (*ProxyNode, error) {
	parsed, err := url.Parse(link)
	if err != nil {
		return nil, err
	}
	port := getPort(parsed)
	return &ProxyNode{
		Name:         getFragment(parsed, fmt.Sprintf("Hysteria-%s:%d", parsed.Hostname(), port)),
		Type:         "hysteria",
		Server:       parsed.Hostname(),
		Port:         port,
		Auth:         parsed.Query().Get("auth"),
		ObfsPassword: parsed.Query().Get("obfs"),
		UDP:          true,
	}, nil
}

func parseHysteria2(link string) (*ProxyNode, error) {
	parsed, err := url.Parse(link)
	if err != nil {
		return nil, err
	}
	password := parsed.User.Username()
	if password == "" {
		return nil, fmt.Errorf("缺少密码")
	}

	port := getPort(parsed)
	node := &ProxyNode{
		Name:     getFragment(parsed, fmt.Sprintf("Hysteria2-%s:%d", parsed.Hostname(), port)),
		Type:     "hysteria2",
		Server:   parsed.Hostname(),
		Port:     port,
		Password: password,
		UDP:      true,
	}

	query := parsed.Query()
	parseCommonURLParams(node, query)

	if obfs := query.Get("obfs"); obfs != "" {
		node.ObfsPassword = obfs
	}

	node.TLS = !node.Insecure
	return node, nil
}

func parseWireGuard(link string) (*ProxyNode, error) {
	parsed, err := url.Parse(link)
	if err != nil {
		return nil, err
	}
	privateKey := parsed.User.Username()
	if privateKey == "" {
		return nil, fmt.Errorf("缺少私钥")
	}

	port := getPort(parsed)
	return &ProxyNode{
		Name:       getFragment(parsed, fmt.Sprintf("WireGuard-%s:%d", parsed.Hostname(), port)),
		Type:       "wireguard",
		Server:     parsed.Hostname(),
		Port:       port,
		PrivateKey: privateKey,
		PublicKey:  parsed.Query().Get("publickey"),
		Reserved:   parsed.Query().Get("reserved"),
		UDP:        true,
	}, nil
}

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
	password := query.Get("password")
	if password == "" {
		password = uuid
	}

	port := getPort(parsed)
	return &ProxyNode{
		Name:     getFragment(parsed, fmt.Sprintf("TUIC-%s:%d", parsed.Hostname(), port)),
		Type:     "tuic",
		Server:   parsed.Hostname(),
		Port:     port,
		UUID:     uuid,
		Password: password,
		Token:    query.Get("token"),
		UDP:      true,
	}, nil
}

func parseHTTP(link string) (*ProxyNode, error) {
	parsed, err := url.Parse(link)
	if err != nil {
		return nil, err
	}
	port := getPort(parsed)
	node := &ProxyNode{
		Name:   getFragment(parsed, fmt.Sprintf("HTTP-%s:%d", parsed.Hostname(), port)),
		Type:   "http",
		Server: parsed.Hostname(),
		Port:   port,
	}

	if parsed.User != nil {
		node.Password = parsed.User.Username()
		if pwd, ok := parsed.User.Password(); ok {
			node.Password = pwd
		}
	}

	return node, nil
}

func parseSOCKS(link string) (*ProxyNode, error) {
	parsed, err := url.Parse(link)
	if err != nil {
		return nil, err
	}
	port := getPort(parsed)
	socksType := "socks5"
	if parsed.Scheme == "socks4" {
		socksType = "socks4"
	}

	node := &ProxyNode{
		Name:   getFragment(parsed, fmt.Sprintf("SOCKS-%s:%d", parsed.Hostname(), port)),
		Type:   socksType,
		Server: parsed.Hostname(),
		Port:   port,
	}

	if parsed.User != nil {
		node.Password = parsed.User.Username()
		if pwd, ok := parsed.User.Password(); ok {
			node.Password = pwd
		}
	}

	return node, nil
}

func parseAnyTLS(link string) (*ProxyNode, error) {
	parsed, err := url.Parse(link)
	if err != nil {
		return nil, err
	}
	uuid := parsed.User.Username()
	if uuid == "" {
		return nil, fmt.Errorf("缺少 UUID")
	}

	port := getPort(parsed)
	node := &ProxyNode{
		Name:   getFragment(parsed, fmt.Sprintf("AnyTLS-%s:%d", parsed.Hostname(), port)),
		Type:   "anytls",
		Server: parsed.Hostname(),
		Port:   port,
		UUID:   uuid,
		TLS:    true,
		UDP:    true,
	}

	query := parsed.Query()
	parseCommonURLParams(node, query)

	if version := query.Get("version"); version != "" {
		node.AnyTLSVersion = version
	}
	if padding := query.Get("padding"); padding != "" {
		node.AnyTLSPadding = padding
	}
	if security := query.Get("security"); security != "" {
		node.Security = security
	}

	return node, nil
}

func parseGOST(link string) (*ProxyNode, error) {
	parsed, err := url.Parse(link)
	if err != nil {
		return nil, err
	}
	port := getPort(parsed)
	protocol := "auto"

	if strings.HasPrefix(parsed.Scheme, "gost+") {
		parts := strings.Split(strings.TrimPrefix(parsed.Scheme, "gost+"), "+")
		if len(parts) > 0 {
			protocol = parts[0]
		}
	} else if parsed.Scheme == "gost" {
		if p := parsed.Query().Get("protocol"); p != "" {
			protocol = p
		}
	}

	node := &ProxyNode{
		Name:         getFragment(parsed, fmt.Sprintf("GOST-%s:%d", parsed.Hostname(), port)),
		Type:         "gost",
		Server:       parsed.Hostname(),
		Port:         port,
		GOSTProtocol: protocol,
	}

	if parsed.User != nil {
		node.Password = parsed.User.Username()
		if pwd, ok := parsed.User.Password(); ok {
			node.Password = pwd
		}
	}

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

func parseCommonURLParams(node *ProxyNode, query url.Values) {
	if sni := queryGetFirst(query, "sni", "peer", "host"); sni != "" && node.SNI == "" {
		node.SNI = sni
	}
	if alpn := query.Get("alpn"); alpn != "" {
		node.ALPN = alpn
	}
	if isQueryTrue(query, "insecure", "allowInsecure", "allow_insecure") {
		node.Insecure = true
	}
}

func queryGetFirst(query url.Values, keys ...string) string {
	for _, k := range keys {
		if v := query.Get(k); v != "" {
			return v
		}
	}
	return ""
}

func isQueryTrue(query url.Values, keys ...string) bool {
	for _, k := range keys {
		if v := query.Get(k); v == "1" || v == "true" {
			return true
		}
	}
	return false
}

func getPort(parsed *url.URL) int {
	if portStr := parsed.Port(); portStr != "" {
		if port, err := strconv.Atoi(portStr); err == nil {
			return port
		}
	}
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

func safeBase64Decode(s string) (string, error) {
	clean := strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\n', '\r':
			return -1
		case '-':
			return '+'
		case '_':
			return '/'
		default:
			return r
		}
	}, s)

	if pad := len(clean) % 4; pad != 0 {
		clean += strings.Repeat("=", 4-pad)
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
		if decoded, err := url.QueryUnescape(parsed.Fragment); err == nil && decoded != "" {
			return decoded
		}
		return parsed.Fragment
	}
	return defaultValue
}

func getFragmentFromLink(link string) string {
	if _, frag, ok := strings.Cut(link, "#"); ok {
		if decoded, err := url.QueryUnescape(frag); err == nil {
			return decoded
		}
		return frag
	}
	return ""
}
