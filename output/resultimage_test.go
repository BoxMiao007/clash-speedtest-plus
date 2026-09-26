package output

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/speedtester"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
)

func TestResultImageFontSizeIsOneStepSmaller(t *testing.T) {
	if resultImageFontSize != 15 {
		t.Fatalf("结果图字号应为 15，实际 %v", resultImageFontSize)
	}
	smaller, err := openEmbeddedFace()
	if err != nil {
		t.Fatalf("load font: %v", err)
	}
	defer smaller.Close()
	parsed, err := opentype.Parse(goregular.TTF)
	if err != nil {
		t.Fatalf("parse font: %v", err)
	}
	previous, err := opentype.NewFace(parsed, &opentype.FaceOptions{
		Size:    16,
		DPI:     96,
		Hinting: font.HintingFull,
	})
	if err != nil {
		t.Fatalf("16 号字: %v", err)
	}
	defer previous.Close()
	if smaller.Metrics().Height >= previous.Metrics().Height {
		t.Fatalf("15 号字行高应小于 16 号: %v >= %v", smaller.Metrics().Height, previous.Metrics().Height)
	}
}

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

func TestTypeColumnFitsFullProxyType(t *testing.T) {
	face, err := LoadImageFont("")
	if err != nil {
		t.Fatalf("load font: %v", err)
	}
	defer face.close()

	proxyType := "Shadowsocks"
	headers := GetHeaders(speedtester.SpeedModeDownload)
	rows := BuildImageRows([]*speedtester.Result{{
		ProxyName: "节点",
		ProxyType: proxyType,
		Latency:   120 * time.Millisecond,
	}}, speedtester.SpeedModeDownload)
	widths := measureColumns(face.face, headers, rows)
	if len(widths) < 3 {
		t.Fatalf("列数不足: %v", widths)
	}
	if widths[2] < textWidth(face.face, proxyType)+8 {
		t.Fatalf("类型列被截短了: 列宽 %d，类型 %q 宽 %d", widths[2], proxyType, textWidth(face.face, proxyType))
	}
	for _, header := range []string{"延迟", "抖动", "丢包率"} {
		found := false
		for i, name := range headers {
			if name != header {
				continue
			}
			found = true
			if widths[i] < textWidth(face.face, header) {
				t.Fatalf("%s 列比表头还窄: %d < %d", header, widths[i], textWidth(face.face, header))
			}
		}
		if !found {
			t.Fatalf("缺少表头 %s", header)
		}
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
		DownloadError: "timeout",
		UploadError:   "reset",
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

func TestTruncateName64(t *testing.T) {
	exactly := strings.Repeat("名", 64)
	if got := truncateName64(exactly); got != exactly {
		t.Fatalf("64 字符名字不应截断: got %d runes", len([]rune(got)))
	}
	long := strings.Repeat("名", 65)
	got := truncateName64(long)
	if len([]rune(got)) != 65 {
		t.Fatalf("65 字符应截断为 64 字符 + 省略号: got %d runes", len([]rune(got)))
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("截断后应带省略号: %q", got)
	}
}

func TestNodeNameColumnWidthRespects64Chars(t *testing.T) {
	face, err := LoadImageFont("")
	if err != nil {
		t.Fatalf("load font: %v", err)
	}
	defer face.close()

	name40 := strings.Repeat("名", 40)
	name65 := strings.Repeat("名", 65)
	headers := GetHeaders(speedtester.SpeedModeFast)
	rows := BuildImageRows([]*speedtester.Result{
		{ProxyName: name40, ProxyType: "SS", Latency: 100 * time.Millisecond},
		{ProxyName: name65, ProxyType: "SS", Latency: 100 * time.Millisecond},
	}, speedtester.SpeedModeFast)

	widths := measureColumns(face.face, headers, rows)
	if len(widths) < 2 {
		t.Fatalf("列宽数量不足: %v", widths)
	}
	// 与 RenderResultImage 一致，节点名列额外加 resultImageNameExtra。
	nameWidth := widths[1] + resultImageNameExtra
	if nameWidth < textWidth(face.face, name40) {
		t.Fatalf("40 字符名字不应被截断: 列宽 %d < 名字宽 %d", nameWidth, textWidth(face.face, name40))
	}
	capped := textWidth(face.face, strings.Repeat("名", 64)+"…")
	if nameWidth > capped+resultImageRowPadX+resultImageNameExtra {
		t.Fatalf("列宽应按 64 字符封顶: 列宽 %d > 上限 %d", nameWidth, capped+resultImageRowPadX+resultImageNameExtra)
	}
}

func TestJoinStatus(t *testing.T) {
	got := JoinStatus("已保存 clash-speedtest.png", "缺少中文字体，节点名可能显示为方框")
	if got != "已保存 clash-speedtest.png；缺少中文字体，节点名可能显示为方框" {
		t.Fatalf("got %q", got)
	}
}

// 顶部来源超宽时必须折成多行，每行都不超过图宽。
func TestWrapTextByWidthFoldsLongSource(t *testing.T) {
	face, err := LoadImageFont("")
	if err != nil {
		t.Skipf("环境没有可用字体: %s", err)
	}
	defer face.close()

	short := "a.yaml"
	if got := wrapTextByWidth(face.face, short, 200); len(got) != 1 || got[0] != short {
		t.Fatalf("短文本不该折行: %#v", got)
	}

	long := strings.Repeat("很长的订阅地址", 40)
	got := wrapTextByWidth(face.face, long, 200)
	if len(got) < 2 {
		t.Fatalf("超宽文本应折成多行: %d 行", len(got))
	}
	joined := strings.Join(got, "")
	if joined != long {
		t.Fatalf("折行不应丢字符: %q...(%d/%d)", joined[:16], len(joined), len(long))
	}
	for i, line := range got {
		if w := textWidth(face.face, line); w > 200 {
			t.Fatalf("第 %d 行超宽 %d: %q", i, w, line)
		}
	}
}

// 带超长来源的整图也要能渲染：多行来源抬高的高度被记进图高。
func TestRenderResultImageWithVeryLongSource(t *testing.T) {
	face, err := LoadImageFont("")
	if err != nil {
		t.Skipf("环境没有可用字体: %s", err)
	}
	defer face.close()

	spec := ImageSpec{
		Mode:    speedtester.SpeedModeDownload,
		Source:  strings.Repeat("https://example.com/very-long-subscription-path/", 10),
		Summary: "2026-09-26 21:04:21  下载  测试中 52/230",
		Headers: []string{"节点", "延迟", "下载"},
		Rows: []ImageRow{{Cells: []string{"节点 1", "100ms", "1.00MB/s"},
			Result: &speedtester.Result{ProxyName: "节点 1", Latency: 100 * time.Millisecond, DownloadSpeed: 1024 * 1024}}},
	}
	data, _, err := RenderResultImage(spec, face)
	if err != nil {
		t.Fatalf("渲染失败: %s", err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("解码失败: %s", err)
	}
	// 折行后的来源行数远超单行，图高必须明显大于最小行数布局。
	singleTitleSpec := spec
	singleTitleSpec.Source = "a.yaml"
	single, _, err := RenderResultImage(singleTitleSpec, face)
	if err != nil {
		t.Fatalf("渲染失败: %s", err)
	}
	singleImg, err := png.Decode(bytes.NewReader(single))
	if err != nil {
		t.Fatalf("解码失败: %s", err)
	}
	if img.Bounds().Dy() <= singleImg.Bounds().Dy() {
		t.Fatalf("超长来源应折行抬高图高: %d <= %d", img.Bounds().Dy(), singleImg.Bounds().Dy())
	}
}

// 每个指标列独立收集本批的 min..max：上传列的色阶不受下载列更大的值影响。
func TestMetricScalesIndependentPerColumn(t *testing.T) {
	rows := []ImageRow{
		{Result: &speedtester.Result{ProxyName: "a", Latency: 400 * time.Millisecond, PacketLoss: 10, DownloadSpeed: 10 * 1024 * 1024, UploadSpeed: 2 * 1024 * 1024}},
		{Result: &speedtester.Result{ProxyName: "b", Latency: 100 * time.Millisecond, PacketLoss: 0, DownloadSpeed: 1 * 1024 * 1024, UploadSpeed: 0.5 * 1024 * 1024}},
		{Result: &speedtester.Result{ProxyName: "failed", Latency: 0, DownloadSpeed: 0, UploadError: "超时"}},
	}
	scales := metricScales(rows, speedtester.SpeedModeFull)

	if s := scales[6]; s.low != 1*1024*1024 || s.high != 10*1024*1024 {
		t.Fatalf("下载列色阶 = %v..%v", s.low, s.high)
	}
	// 上传列的 high 只看上传自己：0.5..2MB/s，绝不吸收下载列的 10。
	if s := scales[7]; s.low != 0.5*1024*1024 || s.high != 2*1024*1024 {
		t.Fatalf("上传列色阶被下载列污染: %v..%v", s.low, s.high)
	}
	// 延迟 0 表示没测出，不参加统计。
	if s := scales[3]; s.low != 100 || s.high != 400 {
		t.Fatalf("延迟列色阶 = %v..%v", s.low, s.high)
	}
	// 丢包率 0% 是有效值，参加统计。
	if s := scales[5]; s.low != 0 || s.high != 10 {
		t.Fatalf("丢包列色阶 = %v..%v", s.low, s.high)
	}
}

// 色阶两端：本列最小值最浅、最大值最深，方向按指标好坏。
func TestMetricScoreNormalizesPerColumn(t *testing.T) {
	rows := []ImageRow{
		{Result: &speedtester.Result{ProxyName: "slow", Latency: 400 * time.Millisecond, DownloadSpeed: 1 * 1024 * 1024, UploadSpeed: 0.5 * 1024 * 1024}},
		{Result: &speedtester.Result{ProxyName: "fast", Latency: 100 * time.Millisecond, DownloadSpeed: 10 * 1024 * 1024, UploadSpeed: 2 * 1024 * 1024}},
	}
	scales := metricScales(rows, speedtester.SpeedModeFull)

	// 下载列：1MB/s 最浅，10MB/s 最深。
	if got := metricScore(rows[0].Result, 6, speedtester.SpeedModeFull, scales); got != 0 {
		t.Fatalf("下载最小值应最浅: %v", got)
	}
	if got := metricScore(rows[1].Result, 6, speedtester.SpeedModeFull, scales); got != 1 {
		t.Fatalf("下载最大值应最深: %v", got)
	}
	// 上传列独立：2MB/s 就是最深——不能被下载列的 10MB/s 稀释。
	if got := metricScore(rows[1].Result, 7, speedtester.SpeedModeFull, scales); got != 1 {
		t.Fatalf("上传最大值应最深（不被下载列稀释）: %v", got)
	}
	if got := metricScore(rows[0].Result, 7, speedtester.SpeedModeFull, scales); got != 0 {
		t.Fatalf("上传最小值应最浅: %v", got)
	}
	// 延迟越小越好：100ms 最深，400ms 最浅。
	if got := metricScore(rows[1].Result, 3, speedtester.SpeedModeFull, scales); got != 1 {
		t.Fatalf("最低延迟应最深: %v", got)
	}
	if got := metricScore(rows[0].Result, 3, speedtester.SpeedModeFull, scales); got != 0 {
		t.Fatalf("最高延迟应最浅: %v", got)
	}
	// 失败行（没测出）按最浅处理。
	failed := &speedtester.Result{ProxyName: "f", Latency: 0, DownloadSpeed: 0}
	if got := metricScore(failed, 6, speedtester.SpeedModeFull, scales); got != 0 {
		t.Fatalf("失败行应最浅: %v", got)
	}
}

// 本批全部同值时分不出深浅，填最深。
func TestMetricScoreAllSameValueFillsDeepest(t *testing.T) {
	rows := []ImageRow{
		{Result: &speedtester.Result{ProxyName: "a", Latency: 100 * time.Millisecond, DownloadSpeed: 5 * 1024 * 1024}},
		{Result: &speedtester.Result{ProxyName: "b", Latency: 100 * time.Millisecond, DownloadSpeed: 5 * 1024 * 1024}},
	}
	scales := metricScales(rows, speedtester.SpeedModeDownload)
	if got := metricScore(rows[0].Result, 6, speedtester.SpeedModeDownload, scales); got != 1 {
		t.Fatalf("全同速应填最深: %v", got)
	}
	if got := metricScore(rows[0].Result, 3, speedtester.SpeedModeDownload, scales); got != 1 {
		t.Fatalf("全同延迟应填最深: %v", got)
	}
}
