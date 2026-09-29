package picker

import (
	"path/filepath"
	"testing"
)

func TestResolveOutputPathAnchorsRelativeBesideExecutable(t *testing.T) {
	got := ResolveOutputPath("/opt/clash-speedtest-plus", "result.yaml")
	want := filepath.Join("/opt/clash-speedtest-plus", "result.yaml")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestResolveOutputPathKeepsAbsoluteAndEmpty(t *testing.T) {
	absolute := filepath.Join(string(filepath.Separator), "tmp", "out.yaml")
	if got := ResolveOutputPath("/opt/clash-speedtest-plus", absolute); got != absolute {
		t.Fatalf("absolute got %q", got)
	}
	if got := ResolveOutputPath("/opt/clash-speedtest-plus", ""); got != "" {
		t.Fatalf("empty got %q", got)
	}
	parent := ResolveOutputPath("/opt/clash-speedtest-plus", filepath.Join("..", "out.yaml"))
	if parent != filepath.Join("/opt", "out.yaml") {
		t.Fatalf("parent got %q", parent)
	}
}

func TestCompleteYAMLSuffix(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"result", "result.yaml"},
		{"result.", "result.yaml"},
		{"result.txt", "result.txt.yaml"},
		{"result.yml", "result.yml"},
		{"result.YAML", "result.YAML"},
		{".yaml", ".yaml"},
		{".yml", ".yml"},
		{".", "."},
		{"./v1.2/out", "./v1.2/out.yaml"},
		{"dir/result", "dir/result.yaml"},
		{"/tmp/result", "/tmp/result.yaml"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := CompleteYAMLSuffix(tc.in); got != tc.want {
			t.Fatalf("CompleteYAMLSuffix(%q) = %q，期望 %q", tc.in, got, tc.want)
		}
		// 补全恒等于原路径接缺的那段后缀。
		if got := tc.in + MissingYAMLSuffix(tc.in); got != tc.want {
			t.Fatalf("%q + MissingYAMLSuffix = %q，期望 %q", tc.in, got, tc.want)
		}
	}
}

func TestMissingYAMLSuffix(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"result", ".yaml"},
		{"result.", "yaml"},
		{"result.txt", ".yaml"},
		{"result.yaml", ""},
		{"result.yml", ""},
		{"result.YAML", ""},
		{".yaml", ""},
		{".yml", ""},
		{".", ""},
		{"./v1.2/out", ".yaml"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := MissingYAMLSuffix(tc.in); got != tc.want {
			t.Fatalf("MissingYAMLSuffix(%q) = %q，期望 %q", tc.in, got, tc.want)
		}
	}
}
