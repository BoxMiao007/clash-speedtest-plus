package speedtester_test

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/BoxMiao007/clash-speedtest-plus/internal/speedtester"
)

// fixtureYAML 是一份最小的 Clash 配置，节点全部是假地址/假凭据。
const fixtureYAML = `proxies:
  - name: "香港-01"
    type: ss
    server: 127.0.0.1
    port: 8388
    cipher: aes-128-gcm
    password: "fixture-password"
`

func TestFetchSubscriptionBodyAcceptsPlainYAML(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); got != "test-agent/1.0" {
			t.Errorf("User-Agent = %q，期望 test-agent/1.0", got)
		}
		_, _ = w.Write([]byte(fixtureYAML))
	}))
	defer server.Close()

	body, used, err := speedtester.FetchSubscriptionBody(server.URL, "test-agent/1.0")
	if err != nil {
		t.Fatalf("FetchSubscriptionBody 失败：%v", err)
	}
	if used != server.URL {
		t.Errorf("used = %q，期望原地址 %q", used, server.URL)
	}
	nodes, err := speedtester.ParseSubscriptionNodes(body, "")
	if err != nil {
		t.Fatalf("ParseSubscriptionNodes 失败：%v", err)
	}
	if len(nodes) != 1 || nodes[0].Name() != "香港-01" {
		t.Fatalf("解析出 %d 个节点（%v），期望 1 个「香港-01」", len(nodes), nodes)
	}
}

func TestFetchSubscriptionBodyRetriesWithFlagMeta(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 地址没带 flag 参数时给一段非 Clash 内容，带上 flag=meta 才给配置。
		if r.URL.Query().Get("flag") != "meta" {
			_, _ = w.Write([]byte("not a clash config"))
			return
		}
		_, _ = w.Write([]byte(fixtureYAML))
	}))
	defer server.Close()

	body, used, err := speedtester.FetchSubscriptionBody(server.URL, "")
	if err != nil {
		t.Fatalf("FetchSubscriptionBody 失败：%v", err)
	}
	if !strings.Contains(used, "flag=meta") {
		t.Errorf("used = %q，期望带 flag=meta", used)
	}
	if !strings.Contains(string(body), "proxies:") {
		t.Errorf("正文 = %q，期望规范化后的 Clash 配置", body)
	}
}

func TestFetchSubscriptionBodyAcceptsBase64(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte(fixtureYAML))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(encoded))
	}))
	defer server.Close()

	body, used, err := speedtester.FetchSubscriptionBody(server.URL, "")
	if err != nil {
		t.Fatalf("FetchSubscriptionBody 失败：%v", err)
	}
	if used != server.URL {
		t.Errorf("base64 正文不应重试，used = %q", used)
	}
	if !strings.Contains(string(body), "proxies:") {
		t.Errorf("正文 = %q，期望解码后的 Clash 配置", body)
	}
}

func TestFetchSubscriptionBodyRejectsNonClash(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("still not a clash config"))
	}))
	defer server.Close()

	if _, _, err := speedtester.FetchSubscriptionBody(server.URL, ""); err == nil {
		t.Fatal("非 Clash 内容期望返回错误，实际为 nil")
	}
}
