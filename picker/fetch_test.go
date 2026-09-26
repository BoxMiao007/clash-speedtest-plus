package picker

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

const yamlBody = "proxies:\n  - {name: n, type: ss, server: 127.0.0.1, port: 1, cipher: aes-128-gcm, password: x}\n"

// 多数机场订阅按 User-Agent 分流，拉取必须把 UA 发出去。
func TestFetchSendsUserAgent(t *testing.T) {
	var gotUA string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		_, _ = w.Write([]byte(yamlBody))
	}))
	defer server.Close()

	files, used, err := fetchSubscriptions(t.TempDir(), "my-ua/1.0", []string{server.URL})
	if err != nil {
		t.Fatalf("拉取失败: %s", err)
	}
	if gotUA != "my-ua/1.0" {
		t.Fatalf("服务器收到的 UA = %q", gotUA)
	}
	if len(files) != 1 || len(used) != 0 {
		t.Fatalf("files=%v used=%v", files, used)
	}
}

// UA 留空时用默认的 clash 标识，机场才返回 Clash/Mihomo 格式。
func TestFetchFallsBackToDefaultUA(t *testing.T) {
	var gotUA string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		_, _ = w.Write([]byte(yamlBody))
	}))
	defer server.Close()

	fetch := fetchWithUA("  ")
	if _, err := fetch(server.URL); err != nil {
		t.Fatalf("拉取失败: %s", err)
	}
	if gotUA != defaultSubscriptionUA {
		t.Fatalf("默认 UA = %q", gotUA)
	}
}

// 内容解不出节点时，错误里要提示可以换「拉取订阅 UA」再试。
func TestFetchNotClashErrorMentionsUAHint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("这不是配置"))
	}))
	defer server.Close()

	_, _, err := fetchSubscriptions(t.TempDir(), "", []string{server.URL})
	if err == nil {
		t.Fatal("应报错")
	}
	if !strings.Contains(err.Error(), "拉取订阅 UA") {
		t.Fatalf("错误应提示 UA 可调: %v", err)
	}
}

// 界面默认值照顾双击直用：并行 6，结果图只留有速度。
func TestNewPicksInterfaceDefaults(t *testing.T) {
	model := New(Session{})
	if model.options.Parallel != "6" {
		t.Fatalf("节点并行默认 = %q", model.options.Parallel)
	}
	if !model.options.ImageSpeedOnly {
		t.Fatal("结果图只留有速度应默认开")
	}
}

// 粘贴带进来的控制字符（如 \x00）必须被剥掉：界面输入一层，
// 地址切分一层——它在地址里不可见，最后会让 url.Parse 报错。
func TestControlCharactersNeverEnterAddress(t *testing.T) {
	model := New(Session{})
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'\x00', 'h', '\x01'}})
	if got := updated.(Model).address; got != "h" {
		t.Fatalf("地址里不该有控制字符: %q", got)
	}
}

func TestSplitSubscriptionTextTrimsInvisibleControlCharacters(t *testing.T) {
	got := SplitSubscriptionText("\x00https://a.com/x, \x01https://b.com\n")
	want := []string{"https://a.com/x", "https://b.com"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("切分 = %#v, want %#v", got, want)
	}
}

// 地址框里已经混进控制字符（比如改代码前粘贴进来的），回车拉取也该被救回。
func TestDirtyAddressIsCleanedBeforeFetch(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(yamlBody))
	}))
	defer server.Close()

	model := New(Session{})
	model.address = "\x00" + server.URL // 模拟粘贴带进来的 NUL
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(Model)
	if !got.fetching || cmd == nil {
		t.Fatalf("应进入获取: fetching=%v", got.fetching)
	}
	done, _ := got.Update(cmd())
	got = done.(Model)
	if !got.Started() {
		t.Fatalf("脏地址应被洗净后拉取成功: status=%q", got.status)
	}
	if gotPath != "/" && gotPath == "" {
		t.Fatalf("服务器路径异常: %q", gotPath)
	}
}
