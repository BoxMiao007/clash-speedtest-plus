package output

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// 产物序号：启动时扫图目录里已有的带号结果图，新图接着最大号往下编；
// 旧版无号图不算，非 png 不算。
func TestNextImageSequence(t *testing.T) {
	dir := t.TempDir()
	if got := NextImageSequence(dir); got != 1 {
		t.Fatalf("空目录应从 1 起，got %d", got)
	}

	// 无号旧图不计数。
	writeFile(t, filepath.Join(dir, "clash-speedtest-plus-20260927-100000.png"))
	writeFile(t, filepath.Join(dir, "机场A-20260927-100001.png"))
	if got := NextImageSequence(dir); got != 1 {
		t.Fatalf("无号图不该计数，got %d", got)
	}

	// 带号图取最大号 + 1。
	writeFile(t, filepath.Join(dir, "1.机场A-20260927-100002.png"))
	if got := NextImageSequence(dir); got != 2 {
		t.Fatalf("有 1 号图应从 2 起，got %d", got)
	}
	writeFile(t, filepath.Join(dir, "10.clash-speedtest-plus-20260927-100003.png"))
	writeFile(t, filepath.Join(dir, "3.机场B-20260927-100004.png"))
	if got := NextImageSequence(dir); got != 11 {
		t.Fatalf("应取最大号 10 接着编 11，got %d", got)
	}

	// 前导零按数字识别（007 = 7）；仍小于最大号 10，号不变。
	writeFile(t, filepath.Join(dir, "007.机场C-20260927-100005.png"))
	if got := NextImageSequence(dir); got != 11 {
		t.Fatalf("前导零 007 应按 7 识别且不推高最大号，got %d", got)
	}
	writeFile(t, filepath.Join(dir, "99.说明.txt"))
	if got := NextImageSequence(dir); got != 11 {
		t.Fatalf("非 png 不该计数，got %d", got)
	}
}

func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// 序号加在文件名最前，跟随基名/默认前缀都在其后；同秒冲突序号仍在最后。
func TestImageFileNameWithSequence(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 9, 27, 15, 30, 0, 0, time.Local)
	first, err := ImageFileName(dir, now, "机场A", 3)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(first) != "3.机场A-20260927-153000.png" {
		t.Fatalf("带序号的图名 = %s", first)
	}
	if err := os.WriteFile(first, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := ImageFileName(dir, now, "机场A", 3)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(second) != "3.机场A-20260927-153000-1.png" {
		t.Fatalf("同秒冲突序号应在最后: %s", second)
	}

	// 订阅轮：默认前缀 + 序号。
	sub, err := ImageFileName(dir, now, "", 5)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(sub) != "5.clash-speedtest-plus-20260927-153000.png" {
		t.Fatalf("默认名 + 序号 = %s", sub)
	}

	// 序号 0 只出现在旧测试里：生产路径的起始号恒 ≥ 1，每张图都带号。
	plain, err := ImageFileName(dir, now, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(filepath.Base(plain), ".clash-speedtest-plus") {
		t.Fatalf("seq=0 不该有序号前缀: %s", plain)
	}
	if filepath.Base(plain) != "clash-speedtest-plus-20260927-153000.png" {
		t.Fatalf("seq=0 旧行为 = %s", plain)
	}
}

// yaml 同轮同号：序号加在最前，基名/时间戳逻辑不变。
func TestFollowedConfigExportPathWithSequence(t *testing.T) {
	now := time.Date(2026, 9, 27, 15, 30, 1, 0, time.Local)
	cases := []struct {
		path, base string
		seq        int
		want       string
	}{
		{"result.yaml", "机场A", 3, "3.机场A-result.yaml"},
		{"/exec/result.yaml", "机场A", 3, "/exec/3.机场A-result.yaml"},
		// 订阅轮没有基名可跟；有了序号已能区分轮次，不再叠时间戳。
		{"result.yaml", "", 3, "3.result.yaml"},
		{"result.yaml", "", 0, "result-20260927-153001.yaml"},
		{"", "机场A", 3, ""},
	}
	for _, c := range cases {
		if got := FollowedConfigExportPath(c.path, c.base, now, c.seq); got != c.want {
			t.Fatalf("FollowedConfigExportPath(%q, %q, %d) = %q, want %q", c.path, c.base, c.seq, got, c.want)
		}
	}
}
