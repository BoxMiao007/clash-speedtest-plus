package picker

import (
	"reflect"
	"testing"
)

// 地址切分的老行为：URL 里的逗号必须留下（查询串常带逗号），
// 换行拆段、空段丢弃。多源各自一轮的拆分以此为前提。
func TestSplitSubscriptionTextKeepsCommaInsideURL(t *testing.T) {
	got := SplitSubscriptionText("https://example.com/sub?token=secret&flag=meta,ss, https://example.com/b")

	want := []string{
		"https://example.com/sub?token=secret&flag=meta,ss",
		"https://example.com/b",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestSplitSubscriptionTextSplitsLinesAndDropsBlanks(t *testing.T) {
	got := SplitSubscriptionText(" https://example.com/a \n\nhttps://example.com/b\r\n")

	want := []string{
		"https://example.com/a",
		"https://example.com/b",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

// 命令行 -c 传多个源时，按逗号拆开：每个源各自一轮，http 前缀标记为订阅源。
// 订阅源的展示名去查询参数，token 不跟着进图顶来源行。
func TestSplitConfigArg(t *testing.T) {
	got := SplitConfigArg(" /opt/a.yaml , https://example.com/sub?token=x ,,b.yml,http://s2.com/c ")
	want := []SourceSpec{
		{Value: "/opt/a.yaml", DisplayName: "a.yaml"},
		{Value: "https://example.com/sub?token=x", DisplayName: "https://example.com/sub", FromSubscription: true},
		{Value: "b.yml", DisplayName: "b.yml"},
		{Value: "http://s2.com/c", DisplayName: "http://s2.com/c", FromSubscription: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SplitConfigArg = %#v, want %#v", got, want)
	}
}

func TestSplitConfigArgDropsEmptyParts(t *testing.T) {
	if got := SplitConfigArg(" , ,, "); got != nil {
		t.Fatalf("全空应返回 nil，got %#v", got)
	}
	if got := SplitConfigArg(""); got != nil {
		t.Fatalf("空串应返回 nil，got %#v", got)
	}
}

// 源列表：先是勾选的配置文件，再是拉取写出的订阅临时文件；
// 订阅源的展示名用原地址，不能把临时文件名带进任何产物。
func TestSourceListMarksSubscriptionSources(t *testing.T) {
	model := New(sessionFixture())
	model.checked[0] = true
	model.fetchedSources = []SourceSpec{
		{Value: "/exec/clash-speedtest-sub-1.yaml", DisplayName: "https://example.com/sub?token=x", FromSubscription: true},
	}
	got := model.SourceList()
	want := []SourceSpec{
		{Value: "/opt/a.yaml", DisplayName: "a.yaml"},
		{Value: "/exec/clash-speedtest-sub-1.yaml", DisplayName: "https://example.com/sub?token=x", FromSubscription: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SourceList = %#v, want %#v", got, want)
	}
}
