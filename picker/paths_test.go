package picker

import (
	"path/filepath"
	"testing"
)

func TestResolveOutputPathAnchorsRelativeBesideExecutable(t *testing.T) {
	got := ResolveOutputPath("/opt/clash-speedtest", "result.yaml")
	want := filepath.Join("/opt/clash-speedtest", "result.yaml")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestResolveOutputPathKeepsAbsoluteAndEmpty(t *testing.T) {
	absolute := filepath.Join(string(filepath.Separator), "tmp", "out.yaml")
	if got := ResolveOutputPath("/opt/clash-speedtest", absolute); got != absolute {
		t.Fatalf("absolute got %q", got)
	}
	if got := ResolveOutputPath("/opt/clash-speedtest", ""); got != "" {
		t.Fatalf("empty got %q", got)
	}
	parent := ResolveOutputPath("/opt/clash-speedtest", filepath.Join("..", "out.yaml"))
	if parent != filepath.Join("/opt", "out.yaml") {
		t.Fatalf("parent got %q", parent)
	}
}
