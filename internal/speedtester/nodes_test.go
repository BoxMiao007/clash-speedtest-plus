package speedtester

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/metacubex/mihomo/component/profile/cachefile"
	"github.com/metacubex/mihomo/constant"
)

func TestLoadNodesFilePreservesOrderAndDuplicateServerPort(t *testing.T) {
	nodes, err := LoadNodesFile(filepath.Join("testdata", "nodes.yaml"))
	if err != nil {
		t.Fatalf("LoadNodesFile returned error: %v", err)
	}
	if len(nodes) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(nodes))
	}

	wantNames := []string{"香港-01", "日本-01", "美国-01"}
	wantTypes := []string{"Shadowsocks", "Vmess", "Trojan"}
	wantServers := []string{"127.0.0.1", "example.com", "127.0.0.1"}
	for i, node := range nodes {
		if node.Name() != wantNames[i] {
			t.Fatalf("node %d: expected name %q, got %q", i, wantNames[i], node.Name())
		}
		if node.Type().String() != wantTypes[i] {
			t.Fatalf("node %d: expected type %q, got %q", i, wantTypes[i], node.Type().String())
		}
		server, _ := node.Config["server"].(string)
		if server != wantServers[i] {
			t.Fatalf("node %d: expected server %q, got %q", i, wantServers[i], server)
		}
	}

	// 香港-01 与 美国-01 的 server:port 相同，LoadNodesFile 不做去重，两个都要保留。
	firstKey, firstOK := buildProxyServerPortKey(nodes[0])
	lastKey, lastOK := buildProxyServerPortKey(nodes[2])
	if !firstOK || !lastOK {
		t.Fatalf("expected both nodes to have server:port keys")
	}
	if firstKey != lastKey {
		t.Fatalf("expected fixture nodes to share server:port %q, got %q and %q", firstKey, firstKey, lastKey)
	}
}

func TestLoadNodesFileEmpty(t *testing.T) {
	for _, name := range []string{"empty.yaml", "zero_bytes.yaml"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join("testdata", name)
			if name == "zero_bytes.yaml" {
				path = filepath.Join(t.TempDir(), "zero_bytes.yaml")
				if err := os.WriteFile(path, nil, 0o600); err != nil {
					t.Fatalf("write empty file: %v", err)
				}
			}
			nodes, err := LoadNodesFile(path)
			if err != nil {
				t.Fatalf("LoadNodesFile returned error: %v", err)
			}
			if nodes == nil {
				t.Fatalf("expected non-nil empty slice")
			}
			if len(nodes) != 0 {
				t.Fatalf("expected 0 nodes, got %d", len(nodes))
			}
		})
	}
}

func TestLoadNodesFileBrokenYAML(t *testing.T) {
	_, err := LoadNodesFile(filepath.Join("testdata", "broken.yaml"))
	if err == nil {
		t.Fatalf("expected parse error for broken yaml")
	}
	if !strings.Contains(err.Error(), "解析节点文件失败") {
		t.Fatalf("expected wrapped parse error, got %v", err)
	}
}

func TestLoadNodesFileMissingFile(t *testing.T) {
	_, err := LoadNodesFile(filepath.Join("testdata", "does-not-exist.yaml"))
	if err == nil {
		t.Fatalf("expected error for missing file")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected os.ErrNotExist, got %v", err)
	}
}

func TestLoadNodesFileDuplicateName(t *testing.T) {
	_, err := LoadNodesFile(filepath.Join("testdata", "duplicate_names.yaml"))
	if err == nil {
		t.Fatalf("expected duplicate name error")
	}
	if !strings.Contains(err.Error(), "香港-01") {
		t.Fatalf("expected error to mention duplicate node name, got %v", err)
	}
}

func TestLoadNodesFileProxyProviders(t *testing.T) {
	const providerBody = `
proxies:
  - name: "provider-b"
    type: trojan
    server: 127.0.0.1
    port: 8443
    password: "fixture-password"
  - name: "provider-a"
    type: ss
    server: 127.0.0.1
    port: 8388
    cipher: aes-128-gcm
    password: "fixture-password"
`
	var (
		mu         sync.Mutex
		userAgents []string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		userAgents = append(userAgents, r.Header.Get("User-Agent"))
		mu.Unlock()
		_, _ = w.Write([]byte(providerBody))
	}))
	defer server.Close()

	// provider 缓存写到用户目录；测试指到临时目录，避免污染真实环境。
	// cache.db 由 mihomo 单例持有，必须在 TempDir 清理前关闭。
	oldHome := constant.Path.HomeDir()
	constant.SetHomeDir(t.TempDir())
	t.Cleanup(func() {
		if cache := cachefile.Cache(); cache.DB != nil {
			_ = cache.Close()
		}
		constant.SetHomeDir(oldHome)
	})

	writeConfig := func(t *testing.T, providerURL string) string {
		t.Helper()
		configBody := `
proxies:
  - name: "direct-01"
    type: ss
    server: 127.0.0.1
    port: 8389
    cipher: aes-128-gcm
    password: "fixture-password"
proxy-providers:
  example:
    type: http
    url: "` + providerURL + `/provider.yaml"
    interval: 3600
`
		path := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(path, []byte(configBody), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
		return path
	}

	t.Run("provider nodes follow direct nodes and sort by name", func(t *testing.T) {
		path := writeConfig(t, server.URL)
		nodes, err := LoadNodesFile(path)
		if err != nil {
			t.Fatalf("LoadNodesFile returned error: %v", err)
		}
		wantNames := []string{"direct-01", "provider-a", "provider-b"}
		if len(nodes) != len(wantNames) {
			t.Fatalf("expected %d nodes, got %d", len(wantNames), len(nodes))
		}
		for i, want := range wantNames {
			if nodes[i].Name() != want {
				t.Fatalf("node %d: expected name %q, got %q", i, want, nodes[i].Name())
			}
		}
		if nodes[2].Config == nil {
			t.Fatalf("expected provider node config to be populated")
		}

		mu.Lock()
		defer mu.Unlock()
		wantUA := defaultFetchConfigUA()
		found := false
		for _, ua := range userAgents {
			if ua == wantUA {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected provider fetch with UA %q, got %v", wantUA, userAgents)
		}
	})

	t.Run("failed provider is skipped", func(t *testing.T) {
		broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("proxies: []\n"))
		}))
		providerURL := broken.URL
		broken.Close()

		path := writeConfig(t, providerURL)
		nodes, err := LoadNodesFile(path)
		if err != nil {
			t.Fatalf("provider failure must not fail the whole file: %v", err)
		}
		if len(nodes) != 1 || nodes[0].Name() != "direct-01" {
			t.Fatalf("expected only direct node, got %d nodes", len(nodes))
		}
	})
}

// LoadProxies 与 LoadNodesFile 共用解析 helper；这个回归测试确认旧的
// 去重与过滤流程仍然生效。
func TestLoadProxiesUsesSharedParsing(t *testing.T) {
	st, err := New(&Config{
		ConfigPaths: filepath.Join("testdata", "nodes.yaml"),
		ServerURL:   "http://127.0.0.1:1",
		Parallel:    1,
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	proxies, err := st.LoadProxies()
	if err != nil {
		t.Fatalf("LoadProxies returned error: %v", err)
	}
	// 三个节点里有两个 server:port 相同，旧行为是按 server:port 去重后剩两个。
	if len(proxies) != 2 {
		t.Fatalf("expected 2 proxies after server:port dedupe, got %d", len(proxies))
	}
	if _, ok := proxies["日本-01"]; !ok {
		t.Fatalf("expected 日本-01 to remain")
	}
	duplicates := 0
	for _, name := range []string{"香港-01", "美国-01"} {
		if _, ok := proxies[name]; ok {
			duplicates++
		}
	}
	if duplicates != 1 {
		t.Fatalf("expected exactly one of the duplicate server:port nodes, got %d", duplicates)
	}
}
