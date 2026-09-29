package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// requirements: 需要本地 ./mihomo/ 目录下存在 mihomo 内核（scripts/download-mihomo.*）。
// 内核不存在时自动跳过，因此在没有内核的 CI 环境里也不会失败。

func mihomoTestBinary(t *testing.T) string {
	t.Helper()
	path, err := getMihomoPath()
	if err != nil {
		t.Skipf("跳过：未找到 mihomo 内核 (%v)", err)
	}
	return path
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("<读取 %s 失败: %v>", path, err)
	}
	return string(data)
}

func ssLink(method, password, host string, port int, query string) string {
	auth := base64.RawURLEncoding.EncodeToString([]byte(method + ":" + password))
	link := fmt.Sprintf("ss://%s@%s:%d", auth, host, port)
	if query != "" {
		link += "?" + query
	}
	return link + "#ss-test"
}

func vmessLink(fields map[string]interface{}) string {
	data, _ := json.Marshal(fields)
	return "vmess://" + base64.StdEncoding.EncodeToString(data)
}

func ssrLink(host string, port int, protocol, method, obfs, password string) string {
	main := fmt.Sprintf("%s:%d:%s:%s:%s:%s", host, port, protocol, method, obfs,
		base64.RawURLEncoding.EncodeToString([]byte(password)))
	main += "/?remarks=" + base64.RawURLEncoding.EncodeToString([]byte("ssr-test"))
	return "ssr://" + base64.RawURLEncoding.EncodeToString([]byte(main))
}

func testKey(seed byte) string {
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = seed + byte(i)
	}
	return base64.StdEncoding.EncodeToString(raw)
}

func sampleNodeLinks() []struct {
	name string
	link string
} {
	uuid := "b831381d-6324-4d53-ad4f-8cda48b30811"
	// 长度为 43 的 base64url REALITY 公钥
	realityPubKey := "jNXHt1yRo0vDuchQlIP6Z0ZvjT3KtzVI-T4E7RoLJS0"

	return []struct {
		name string
		link string
	}{
		{"ss-plain", ssLink("aes-256-gcm", "passw0rd", "1.2.3.4", 8388, "")},
		{"ss-obfs", ssLink("aes-256-gcm", "passw0rd", "1.2.3.4", 8388,
			"plugin="+url.QueryEscape("obfs-local;obfs=http;obfs-host=www.bing.com"))},
		{"ss-v2ray-plugin", ssLink("chacha20-ietf-poly1305", "passw0rd", "1.2.3.4", 8388,
			"plugin="+url.QueryEscape("v2ray-plugin;mode=websocket;host=example.com;path=/ws;tls"))},
		{"ssr", ssrLink("1.2.3.4", 8388, "auth_aes128_md5", "aes-256-cfb", "tls1.2_ticket_auth", "passw0rd")},
		{"vmess-tcp-tls", vmessLink(map[string]interface{}{
			"v": "2", "ps": "vmess-tcp", "add": "1.2.3.4", "port": "443", "id": uuid,
			"aid": "0", "net": "tcp", "type": "none", "tls": "tls", "sni": "example.com",
		})},
		{"vmess-ws-tls", vmessLink(map[string]interface{}{
			"v": "2", "ps": "vmess-ws", "add": "1.2.3.4", "port": "443", "id": uuid,
			"aid": "0", "net": "ws", "type": "none", "host": "example.com", "path": "/ws",
			"tls": "tls", "sni": "example.com",
		})},
		{"vmess-grpc", vmessLink(map[string]interface{}{
			"v": "2", "ps": "vmess-grpc", "add": "1.2.3.4", "port": "443", "id": uuid,
			"aid": "0", "net": "grpc", "type": "none", "path": "grpcsvc", "tls": "tls",
		})},
		{"vless-ws-tls", "vless://" + uuid + "@1.2.3.4:443?encryption=none&security=tls&sni=example.com" +
			"&type=ws&host=example.com&path=%2Fws#vless-ws"},
		{"vless-reality-grpc", "vless://" + uuid + "@1.2.3.4:443?encryption=none&security=reality" +
			"&sni=www.microsoft.com&fp=chrome&pbk=" + realityPubKey + "&sid=0123" +
			"&type=grpc&serviceName=grpcsvc&flow=xtls-rprx-vision#vless-reality"},
		{"trojan-tcp", "trojan://passw0rd@1.2.3.4:443?sni=example.com#trojan-tcp"},
		{"trojan-ws", "trojan://passw0rd@1.2.3.4:443?sni=example.com&type=ws&host=example.com&path=%2Fws#trojan-ws"},
		{"hysteria", "hysteria://1.2.3.4:443?auth=passw0rd&peer=example.com&insecure=1" +
			"&upmbps=100&downmbps=100&obfs=xplus&protocol=udp#hysteria"},
		{"hysteria2", "hysteria2://passw0rd@1.2.3.4:443?sni=example.com&insecure=1" +
			"&obfs=salamander&obfs-password=obfspass#hysteria2"},
		{"tuic", "tuic://" + uuid + ":passw0rd@1.2.3.4:443?sni=example.com&alpn=h3&congestion_control=bbr#tuic"},
		{"wireguard", "wireguard://" + testKey(1) + "@1.2.3.4:51820?publickey=" + testKey(100) +
			"&address=10.0.0.2/32&reserved=1,2,3#wireguard"},
		{"http", "http://user:passw0rd@1.2.3.4:8080#http"},
		{"https", "https://user:passw0rd@1.2.3.4:8443#https"},
		{"socks5", "socks5://user:passw0rd@1.2.3.4:1080#socks5"},
		{"anytls", "anytls://passw0rd@1.2.3.4:8443?sni=example.com&insecure=1#anytls"},
	}
}

func TestGeneratedConfigsPassMihomoKernelCheck(t *testing.T) {
	binary := mihomoTestBinary(t)
	collector := NewCollector("")

	for _, sample := range sampleNodeLinks() {
		sample := sample
		t.Run(sample.name, func(t *testing.T) {
			node, err := ParseNodeLink(sample.link)
			if err != nil {
				t.Fatalf("解析节点失败: %v", err)
			}

			port, err := getFreePort()
			if err != nil {
				t.Fatalf("分配端口失败: %v", err)
			}

			workDir, configFile, err := collector.createMihomoConfig(node, port)
			if err != nil {
				t.Fatalf("生成 mihomo 配置失败: %v", err)
			}
			defer os.RemoveAll(workDir)

			output, err := exec.Command(binary, "-t", "-d", workDir, "-f", configFile).CombinedOutput()
			if err != nil {
				t.Fatalf("mihomo 配置校验失败: %v\n%s\n--- 生成的配置 ---\n%s",
					err, strings.TrimSpace(string(output)), mustReadFile(t, configFile))
			}
		})
	}
}

func TestUnsupportedProtocolsAreRejected(t *testing.T) {
	cases := map[string]string{
		"socks4": "socks4://1.2.3.4:1080#socks4",
		"gost":   "gost://1.2.3.4:8080#gost",
	}

	for name, link := range cases {
		name, link := name, link
		t.Run(name, func(t *testing.T) {
			node, err := ParseNodeLink(link)
			if err != nil {
				t.Skipf("解析器不支持该协议，直接跳过: %v", err)
			}
			if _, err := buildMihomoProxy(node); err == nil {
				t.Fatalf("期望 %s 被拒绝，但成功生成了配置", name)
			}
		})
	}
}

// TestEndToEndLatencyThroughMihomo 用完全本地的假上游代理跑通
// “生成配置 -> 启动 mihomo -> 通过 mixed-port 发起请求 -> 得到延迟” 全流程。
func TestEndToEndLatencyThroughMihomo(t *testing.T) {
	binary := mihomoTestBinary(t)

	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	upstream := httptest.NewServer(okHandler)
	defer upstream.Close()

	target := httptest.NewServer(okHandler)
	defer target.Close()

	t.Setenv("TEST_URL", target.URL)
	t.Setenv("TEST_TIMEOUT", "10")

	upstreamURL, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatalf("解析假上游地址失败: %v", err)
	}

	link := fmt.Sprintf("http://%s#e2e", upstreamURL.Host)
	node, err := ParseNodeLink(link)
	if err != nil {
		t.Fatalf("解析节点失败: %v", err)
	}

	port, err := getFreePort()
	if err != nil {
		t.Fatalf("分配端口失败: %v", err)
	}

	collector := NewCollector("")
	workDir, configFile, err := collector.createMihomoConfig(node, port)
	if err != nil {
		t.Fatalf("生成配置失败: %v", err)
	}
	defer os.RemoveAll(workDir)

	latency, err := collector.testLatencyWithMihomo(binary, workDir, configFile, port)
	if err != nil {
		t.Fatalf("端到端测速失败: %v\n--- 生成的配置 ---\n%s", err, mustReadFile(t, configFile))
	}
	if latency <= 0 {
		t.Fatalf("延迟异常: %v", latency)
	}
	t.Logf("端到端延迟: %v", latency)
}
