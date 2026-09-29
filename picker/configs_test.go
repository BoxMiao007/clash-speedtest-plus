package picker

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListConfigsSortsSelectableAndKeepsBrokenVisible(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("b.yaml", "proxies:\n  - {name: a, type: ss}\n")
	write("A.yaml", "proxy-providers:\n  airport:\n    type: http\n    url: https://example.com/p\n")
	write("empty.yaml", "proxies: []\n")
	write("bad.yaml", ":\n")
	write("notes.yml", "proxies:\n  - {name: a, type: ss}\n")
	write("readme.txt", "proxies: []\n")

	got, err := ListConfigs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("len = %d, entries = %#v", len(got), got)
	}
	wantNames := []string{"A.yaml", "b.yaml", "bad.yaml", "empty.yaml"}
	wantSelectable := []bool{true, true, false, false}
	for i, entry := range got {
		if entry.Name != wantNames[i] || entry.Selectable != wantSelectable[i] {
			t.Fatalf("entry %d = %+v", i, entry)
		}
	}
}

func TestListConfigsCountsNodesAndProviders(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("local.yaml", "proxies:\n  - {name: a, type: ss}\n  - {name: b, type: ss}\n")
	write("mixed.yaml", "proxies:\n  - {name: a, type: ss}\nproxy-providers:\n  airport:\n    type: http\n    url: https://example.com/p\n")
	write("remote.yaml", "proxy-providers:\n  airport:\n    type: http\n    url: https://example.com/p\n")

	got, err := ListConfigs(dir)
	if err != nil {
		t.Fatal(err)
	}
	entries := map[string]ConfigEntry{}
	for _, entry := range got {
		entries[entry.Name] = entry
	}
	checks := []struct {
		name  string
		nodes int
		more  bool
	}{
		{"local.yaml", 2, false},
		{"mixed.yaml", 1, true},
		{"remote.yaml", 0, true},
	}
	for _, want := range checks {
		entry := entries[want.name]
		if !entry.Selectable || entry.Nodes != want.nodes || entry.More != want.more {
			t.Fatalf("%s = %+v", want.name, entry)
		}
	}
}

// 补全规则产出的文件名要能被「合格配置」识别回圈：用户输入 result.txt
// 之类词干，落定后写出的 result.txt.yaml（双层后缀）仍能列出并勾选。
func TestListConfigsRecognizesCompletedNames(t *testing.T) {
	dir := t.TempDir()
	body := "proxies:\n  - {name: a, type: ss}\n"
	for _, stem := range []string{"result.txt", "result.", "机场A"} {
		name := CompleteYAMLSuffix(stem)
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := ListConfigs(dir)
	if err != nil {
		t.Fatal(err)
	}
	entries := map[string]ConfigEntry{}
	for _, entry := range got {
		entries[entry.Name] = entry
	}
	for _, name := range []string{"result.txt.yaml", "result.yaml", "机场A.yaml"} {
		entry, ok := entries[name]
		if !ok {
			t.Fatalf("补全后的文件名 %s 应被列出: %#v", name, got)
		}
		if !entry.Selectable || entry.Nodes != 1 {
			t.Fatalf("%s 应可选且有节点: %+v", name, entry)
		}
	}
}
