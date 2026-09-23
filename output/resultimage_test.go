package output

import (
	"bytes"
	"image"
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

func TestErrorTextDoesNotWidenResultImage(t *testing.T) {
	face, err := LoadImageFont("")
	if err != nil {
		t.Fatalf("load font: %v", err)
	}
	defer face.close()

	ok := &speedtester.Result{
		ProxyName:     "香港 01",
		ProxyType:     "SS",
		Latency:       120 * time.Millisecond,
		Jitter:        20 * time.Millisecond,
		PacketLoss:    0,
		DownloadSpeed: 12 * 1024 * 1024,
		UploadSpeed:   4 * 1024 * 1024,
	}
	// 类型与正常节点保持一致，让测试只聚焦「错误文本」这一个变量。
	failed := &speedtester.Result{
		ProxyName:     "失败节点",
		ProxyType:     "SS",
		Latency:       200 * time.Millisecond,
		Jitter:        30 * time.Millisecond,
		PacketLoss:    1,
		DownloadError: "download request to https://example.com/very/long/path failed: context deadline exceeded while reading response body",
		UploadError:   "upload request to https://example.com/very/long/path failed: connection reset by peer",
	}
	spec := func(results []*speedtester.Result) ImageSpec {
		return ImageSpec{
			Mode:    speedtester.SpeedModeFull,
			Headers: GetHeaders(speedtester.SpeedModeFull),
			Rows:    BuildImageRows(results, speedtester.SpeedModeFull),
		}
	}
	normal := renderBounds(t, face, spec([]*speedtester.Result{ok}))
	withError := renderBounds(t, face, spec([]*speedtester.Result{ok, failed}))
	if withError.Dx() != normal.Dx() || withError.Dy()-normal.Dy() != rowHeightOf(face) {
		t.Fatalf("错误信息不应改变格子宽度或行高: normal=%v withError=%v row=%d", normal, withError, rowHeightOf(face))
	}
}

func TestNodeNameHeaderIsCentered(t *testing.T) {
	face, err := LoadImageFont("")
	if err != nil {
		t.Fatalf("load font: %v", err)
	}
	defer face.close()

	result := &speedtester.Result{
		ProxyName: "香港 01",
		ProxyType: "SS",
		Latency:   120 * time.Millisecond,
	}
	spec := ImageSpec{
		Mode:    speedtester.SpeedModeFast,
		Headers: GetHeaders(speedtester.SpeedModeFast),
		Rows:    BuildImageRows([]*speedtester.Result{result}, speedtester.SpeedModeFast),
	}
	img := renderImage(t, face, spec)
	nameX, nameWidth := nameColumnBox(face, spec.Headers, spec.Rows)
	headerY := rowHeightOf(face) / 2
	bodyY := rowHeightOf(face) + rowHeightOf(face)/2
	headerInk := inkSpan(img, nameX, nameX+nameWidth, headerY)
	bodyInk := inkSpan(img, nameX, nameX+nameWidth, bodyY)
	if !centeredIn(headerInk, nameX, nameWidth) {
		t.Fatalf("节点名称表头未居中: span=%v column=[%d,%d)", headerInk, nameX, nameX+nameWidth)
	}
	if centeredIn(bodyInk, nameX, nameWidth) {
		t.Fatalf("节点名称内容不应居中: span=%v column=[%d,%d)", bodyInk, nameX, nameX+nameWidth)
	}
}

func renderBounds(t *testing.T, face imageFont, spec ImageSpec) image.Rectangle {
	t.Helper()
	return renderImage(t, face, spec).Bounds()
}

func renderImage(t *testing.T, face imageFont, spec ImageSpec) image.Image {
	t.Helper()
	data, _, err := RenderResultImage(spec, face)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	return img
}

func rowHeightOf(face imageFont) int {
	lineHeight := face.face.Metrics().Height.Ceil()
	if lineHeight <= 0 {
		lineHeight = resultImageFontSize + 4
	}
	return lineHeight + resultImageRowPadY*2
}

func nameColumnBox(face imageFont, headers []string, rows []ImageRow) (int, int) {
	widths := measureColumns(face.face, headers, rows)
	if len(widths) > 1 {
		widths[1] += resultImageNameExtra
	}
	x := 0
	for i := 0; i < 1 && i < len(widths); i++ {
		x += widths[i]
	}
	return x, widths[1]
}

type pixelSpan struct {
	left, right int
}

func inkSpan(img image.Image, x0, x1, y int) pixelSpan {
	span := pixelSpan{left: -1, right: -1}
	for x := x0; x < x1; x++ {
		r, g, b, _ := img.At(x, y).RGBA()
		if r>>8 < 80 && g>>8 < 80 && b>>8 < 80 {
			if span.left < 0 {
				span.left = x
			}
			span.right = x
		}
	}
	return span
}

func centeredIn(span pixelSpan, x, width int) bool {
	if span.left < 0 || span.right < span.left {
		return false
	}
	mid := (span.left + span.right) / 2
	center := x + width/2
	delta := mid - center
	if delta < 0 {
		delta = -delta
	}
	return delta <= 8
}

func TestJoinStatus(t *testing.T) {
	got := JoinStatus("已保存 clash-speedtest.png", "缺少中文字体，节点名可能显示为方框")
	if got != "已保存 clash-speedtest.png；缺少中文字体，节点名可能显示为方框" {
		t.Fatalf("got %q", got)
	}
}
