package output

import (
	"bytes"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/speedtester"
)

func TestRenderResultImagePNG(t *testing.T) {
	face, err := LoadImageFont("")
	if err != nil {
		t.Fatalf("load font: %v", err)
	}
	defer face.close()

	now := time.Date(2026, 7, 22, 12, 30, 0, 0, time.Local)
	result := &speedtester.Result{
		ProxyName:     "香港 01",
		ProxyType:     "SS",
		Latency:       120 * time.Millisecond,
		Jitter:        20 * time.Millisecond,
		PacketLoss:    0,
		DownloadSpeed: 12 * 1024 * 1024,
	}
	spec := ImageSpec{
		Mode:    speedtester.SpeedModeDownload,
		Summary: SummaryLine(now, speedtester.SpeedModeDownload, "已完成", 1, 2),
		Headers: GetHeaders(speedtester.SpeedModeDownload),
		Rows:    BuildImageRows([]*speedtester.Result{result}, speedtester.SpeedModeDownload),
		Now:     now,
	}
	data, warning, err := RenderResultImage(spec, face)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected png bytes")
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode png: %v", err)
	}
	if img.Bounds().Dx() < 200 || img.Bounds().Dy() < 40 {
		t.Fatalf("image too small: %v", img.Bounds())
	}
	if face.fallback && warning == "" {
		t.Fatal("expected missing-font warning when falling back")
	}
}

func TestImageFileNameConflict(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 7, 22, 12, 30, 1, 0, time.Local)
	first, err := ImageFileName(dir, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(first, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := ImageFileName(dir, now)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(second) == filepath.Base(first) {
		t.Fatalf("expected conflict suffix, got %s", second)
	}
	if filepath.Ext(second) != ".png" {
		t.Fatalf("expected png, got %s", second)
	}
}

func TestJoinStatus(t *testing.T) {
	got := JoinStatus("已保存 clash-speedtest.png", "缺少中文字体，节点名可能显示为方框")
	if got != "已保存 clash-speedtest.png；缺少中文字体，节点名可能显示为方框" {
		t.Fatalf("got %q", got)
	}
}
