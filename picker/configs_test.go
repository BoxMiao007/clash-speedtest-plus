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
