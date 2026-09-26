package output

import (
	"bytes"
	"embed"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/faceair/clash-speedtest/speedtester"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

//go:embed flags/*.png
var flagFiles embed.FS

const (
	resultImageFontSize  = 15
	resultImageRowPadX   = 28
	resultImageRowPadY   = 6
	resultImageLineGap   = 0
	resultImageBarWidth  = 120
	resultImageNameExtra = 80
)

// ImageRow 是结果表图的一行。InFlight 为真时整行灰色，不走阈值配色。
type ImageRow struct {
	Cells    []string
	InFlight bool
	Result   *speedtester.Result
}

// ImageSpec 描述一张结果表图。
type ImageSpec struct {
	Mode     speedtester.SpeedMode
	Source   string
	Summary  string
	Headers  []string
	Rows     []ImageRow
	FontPath string
	Now      time.Time
}

type imageFont struct {
	face         font.Face
	emoji        font.Face
	fallback     bool
	emojiMissing bool
}

func (f imageFont) close() {
	if f.face != nil {
		_ = f.face.Close()
	}
	if f.emoji != nil {
		_ = f.emoji.Close()
	}
}

// LoadImageFont 优先加载系统 CJK 字体，并尽量附上 emoji 备用字体。
// 找不到 CJK 时用基本拉丁字体并标记 fallback。
func LoadImageFont(path string) (imageFont, error) {
	if path == "" {
		path = findCJKFont()
	}
	if path == "" {
		face, err := openEmbeddedFace()
		if err != nil {
			return imageFont{}, err
		}
		return imageFont{face: face, fallback: true, emojiMissing: findEmojiFont() == ""}, nil
	}
	face, err := openFontFace(path)
	if err != nil {
		return imageFont{}, err
	}
	emojiPath := findEmojiFont()
	if emojiPath == "" {
		return imageFont{face: face, emojiMissing: true}, nil
	}
	emoji, err := openFontFace(emojiPath)
	if err != nil {
		return imageFont{face: face, emojiMissing: true}, nil
	}
	return imageFont{face: face, emoji: emoji}, nil
}

func openFontFace(path string) (font.Face, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read font %s: %w", path, err)
	}
	parsed, err := opentype.Parse(data)
	if err != nil {
		collection, collErr := opentype.ParseCollection(data)
		if collErr != nil {
			return nil, fmt.Errorf("parse font %s: %w", path, err)
		}
		parsed, err = collection.Font(0)
		if err != nil {
			return nil, fmt.Errorf("font face %s: %w", path, err)
		}
	}
	face, err := opentype.NewFace(parsed, &opentype.FaceOptions{
		Size:    resultImageFontSize,
		DPI:     96,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return nil, fmt.Errorf("new face %s: %w", path, err)
	}
	return face, nil
}

func findEmojiFont() string {
	home, _ := os.UserHomeDir()
	windir := os.Getenv("WINDIR")
	if windir == "" {
		windir = os.Getenv("SystemRoot")
	}
	if windir == "" {
		windir = `C:\Windows`
	}
	candidates := []string{
		filepath.Join(windir, "Fonts", "seguiemj.ttf"),
		"/mnt/c/Windows/Fonts/seguiemj.ttf",
		"/System/Library/Fonts/Apple Color Emoji.ttc",
		filepath.Join(home, ".local/share/fonts/NotoEmoji.ttf"),
		"/usr/share/fonts/noto/NotoEmoji-Regular.ttf",
		"/usr/share/fonts/truetype/noto/NotoEmoji-Regular.ttf",
		"/usr/share/fonts/noto/NotoColorEmoji.ttf",
		"/usr/share/fonts/truetype/noto/NotoColorEmoji.ttf",
		"/usr/share/fonts/noto-emoji/NotoColorEmoji.ttf",
		"/usr/share/fonts/google-noto-emoji/NotoColorEmoji.ttf",
	}
	return firstExistingFont(candidates)
}

func findCJKFont() string {
	windir := os.Getenv("WINDIR")
	if windir == "" {
		windir = os.Getenv("SystemRoot")
	}
	if windir == "" {
		windir = `C:\Windows`
	}
	return firstExistingFont([]string{
		"/usr/share/fonts/noto-cjk/NotoSansCJK-Regular.ttc",
		"/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc",
		"/usr/share/fonts/truetype/noto/NotoSansCJK-Regular.ttc",
		"/usr/share/fonts/truetype/wqy/wqy-microhei.ttc",
		"/usr/share/fonts/truetype/wqy/wqy-zenhei.ttc",
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
		"/mnt/c/Windows/Fonts/msyh.ttc",
		"/mnt/c/Windows/Fonts/simhei.ttf",
		"/mnt/c/Windows/Fonts/simsun.ttc",
		filepath.Join(windir, "Fonts", "msyh.ttc"),
		filepath.Join(windir, "Fonts", "msyh.ttf"),
		filepath.Join(windir, "Fonts", "simhei.ttf"),
		filepath.Join(windir, "Fonts", "simsun.ttc"),
		filepath.Join(windir, "Fonts", "arial.ttf"),
	})
}

func openEmbeddedFace() (font.Face, error) {
	parsed, err := opentype.Parse(goregular.TTF)
	if err != nil {
		return nil, err
	}
	return opentype.NewFace(parsed, &opentype.FaceOptions{
		Size:    resultImageFontSize,
		DPI:     96,
		Hinting: font.HintingFull,
	})
}

func firstExistingFont(candidates []string) string {
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// RenderResultImage 把结果表画成浅色 PNG。
// 节点名列按最长名字撑宽；节点名超过 64 个字符才开始截断并加省略号。
func RenderResultImage(spec ImageSpec, fontFace imageFont) ([]byte, string, error) {
	if fontFace.face == nil {
		return nil, "", fmt.Errorf("font face is nil")
	}
	headers := spec.Headers
	if len(headers) == 0 {
		headers = GetHeaders(spec.Mode)
	}
	rows := stripIndexDots(spec.Rows)
	colWidths := measureColumns(fontFace.face, headers, rows)
	if len(colWidths) > 1 {
		colWidths[1] += resultImageNameExtra
	}

	lineHeight := fontFace.face.Metrics().Height.Ceil()
	if lineHeight <= 0 {
		lineHeight = resultImageFontSize + 4
	}
	rowHeight := lineHeight + resultImageRowPadY*2
	width := sumWidths(colWidths)
	if width < 200 {
		width = 200
	}
	// 来源可能是一长串文件名或链接，超宽时按图宽折行，不遮出边界。
	sourceLines := wrapTextByWidth(fontFace.face, sourceLabel(spec.Source), width)
	titleRows := len(sourceLines)
	if spec.Summary != "" {
		titleRows++
	}
	height := (titleRows+1+len(rows))*rowHeight + resultImageLineGap
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	fill(img, color.NRGBA{255, 255, 255, 255})

	y := 0
	for _, line := range sourceLines {
		drawRow(img, fontFace, y, rowHeight, colWidths, []string{line}, color.NRGBA{255, 255, 255, 255}, color.NRGBA{15, 23, 42, 255}, false)
		y += rowHeight
	}
	if spec.Summary != "" {
		drawRow(img, fontFace, y, rowHeight, colWidths, []string{spec.Summary}, color.NRGBA{255, 255, 255, 255}, color.NRGBA{71, 85, 105, 255}, false)
		y += rowHeight
	}
	drawHeaderRow(img, fontFace, y, rowHeight, colWidths, headers)
	y += rowHeight
	scales := metricScales(rows, spec.Mode)
	for _, row := range rows {
		drawDataRow(img, fontFace, y, rowHeight, colWidths, spec.Mode, row, scales)
		y += rowHeight
	}
	drawGrid(img, titleRows, rowHeight, colWidths)

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, "", err
	}
	var warnings []string
	if fontFace.fallback {
		warnings = append(warnings, "缺少中文字体，节点名可能显示为方框")
	}
	if fontFace.emojiMissing {
		warnings = append(warnings, "缺少 emoji 字体，表情符号已降级")
	}
	return buf.Bytes(), strings.Join(warnings, "；"), nil
}

func measureColumns(face font.Face, headers []string, rows []ImageRow) []int {
	widths := make([]int, len(headers))
	for i, header := range headers {
		widths[i] = textWidth(face, header) + resultImageRowPadX
	}
	for _, row := range rows {
		for i, cell := range row.Cells {
			if i >= len(widths) {
				continue
			}
			text := cell
			if i == 1 {
				// 节点名：超过 64 个字符才开始截断，短名完整参与列宽。
				text = truncateName64(cell)
			} else if isTransferColumn(i) && !metricCell(cell) {
				// 下载/上传列里的错误文本不撑列宽，绘制时再截断。
				// 类型、延迟、抖动、丢包率必须按原文撑开，否则类型名会被截断。
				continue
			}
			if w := textWidth(face, text) + resultImageRowPadX; w > widths[i] {
				widths[i] = w
			}
		}
	}
	return widths
}

// truncateName64 节点名超过 64 个字符才截断并加省略号；短名保持完整。
func truncateName64(name string) string {
	runes := []rune(name)
	if len(runes) <= 64 {
		return name
	}
	return string(runes[:64]) + "…"
}

// isTransferColumn 只指下载速度、上传速度。类型列不在这里。
func isTransferColumn(index int) bool {
	return index >= 6
}

// metricCell 判断格子是不是测速数据。错误信息不是，不能撑开速度列。
func metricCell(text string) bool {
	switch text {
	case "", "N/A", "测试中", "…":
		return true
	}
	return strings.HasSuffix(text, "ms") || strings.HasSuffix(text, "%") ||
		strings.HasSuffix(text, "B/s") || strings.HasSuffix(text, "KB/s") ||
		strings.HasSuffix(text, "MB/s") || strings.HasSuffix(text, "GB/s") ||
		strings.HasSuffix(text, "TB/s")
}

func isSpeedHeader(header string) bool {
	return strings.Contains(header, "速度")
}

func truncateToWidth(face font.Face, text string, maxWidth int) string {
	if textWidth(face, text) <= maxWidth {
		return text
	}
	runes := []rune(text)
	for len(runes) > 1 {
		runes = runes[:len(runes)-1]
		candidate := string(runes) + "…"
		if textWidth(face, candidate) <= maxWidth {
			return candidate
		}
	}
	return "…"
}

func textWidth(face font.Face, text string) int {
	return font.MeasureString(face, text).Ceil()
}

func faceForRune(primary, emoji font.Face, r rune) font.Face {
	if primary == nil {
		return emoji
	}
	if glyphAdvance, ok := primary.GlyphAdvance(r); ok && glyphAdvance > 0 {
		return primary
	}
	if emoji == nil {
		return primary
	}
	if glyphAdvance, ok := emoji.GlyphAdvance(r); ok && glyphAdvance > 0 {
		return emoji
	}
	return primary
}

func drawFallbackString(drawer *font.Drawer, primary, emoji font.Face, text string) {
	for _, r := range text {
		drawer.Face = faceForRune(primary, emoji, r)
		drawer.DrawString(string(r))
	}
}

func sumWidths(widths []int) int {
	total := 0
	for _, w := range widths {
		total += w
	}
	return total
}

func fill(img *image.NRGBA, c color.NRGBA) {
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := 0; x < img.Bounds().Dx(); x++ {
			img.SetNRGBA(x, y, c)
		}
	}
}

func drawRow(img *image.NRGBA, faces imageFont, y, height int, colWidths []int, cells []string, bg, fg color.NRGBA, bold bool) {
	fillRect(img, 0, y, img.Bounds().Dx(), height, bg)
	x := 0
	for i, width := range colWidths {
		text := ""
		if i < len(cells) {
			text = cells[i]
		}
		if i == 0 && len(cells) == 1 {
			drawText(img, faces, resultImageRowPadX/2, y, height, text, fg)
		} else {
			drawCentered(img, faces, x, y, width, height, text, fg)
		}
		_ = bold
		x += width
	}
}

func drawHeaderRow(img *image.NRGBA, faces imageFont, y, height int, colWidths []int, headers []string) {
	fillRect(img, 0, y, img.Bounds().Dx(), height, color.NRGBA{226, 232, 240, 255})
	x := 0
	for i, width := range colWidths {
		text := ""
		if i < len(headers) {
			text = headers[i]
		}
		drawCellText(img, faces, x, y, width, height, text, color.NRGBA{0, 0, 0, 255}, false)
		x += width
	}
}

func drawDataRow(img *image.NRGBA, faces imageFont, y, height int, colWidths []int, mode speedtester.SpeedMode, row ImageRow, scales map[int]metricScale) {
	fillRect(img, 0, y, img.Bounds().Dx(), height, color.NRGBA{255, 255, 255, 255})
	x := 0
	for i, width := range colWidths {
		text := ""
		if i < len(row.Cells) {
			text = row.Cells[i]
		}
		fg := color.NRGBA{0, 0, 0, 255}
		left := i == 1
		if row.InFlight {
			drawCellText(img, faces, x, y, width, height, text, fg, left)
			x += width
			continue
		}
		switch kindOfColumn(i, mode) {
		case colMetric:
			if !plainMetricCell(row.Result, i, text) {
				bg, _ := metricColors(row.Result, i, mode, scales, text)
				fillRect(img, x, y, width, height, bg)
			}
			drawCellText(img, faces, x, y, width, height, text, fg, left)
		default:
			drawCellText(img, faces, x, y, width, height, text, fg, left)
		}
		x += width
	}
}

type columnKind int

const (
	colPlain columnKind = iota
	colMetric
)

func kindOfColumn(index int, mode speedtester.SpeedMode) columnKind {
	if index == 3 {
		return colMetric
	}
	if mode.IsFast() {
		return colPlain
	}
	if index == 4 || index == 5 || index == 6 || (mode.UploadEnabled() && index == 7) {
		return colMetric
	}
	return colPlain
}

// plainMetricCell 表示 N/A 和 100% 丢包不铺底色。
func plainMetricCell(result *speedtester.Result, index int, text string) bool {
	if text == "" || text == "N/A" || text == "测试中" {
		return true
	}
	return index == 5 && result != nil && result.PacketLoss >= 100
}

func metricColors(result *speedtester.Result, index int, mode speedtester.SpeedMode, scales map[int]metricScale, text string) (color.NRGBA, color.NRGBA) {
	speedColumn := index == 6 || index == 7
	if result == nil || text == "" || text == "N/A" || text == "测试中" {
		if speedColumn {
			return redScale(0), scoreText(0)
		}
		return greenScale(0), scoreText(0)
	}
	score := metricScore(result, index, mode, scales)
	if speedColumn {
		return redScale(score), scoreText(score)
	}
	return greenScale(score), scoreText(score)
}

// metricScale 是一个指标列在本批结果里的取值范围，色阶的深浅两端点。
type metricScale struct {
	low, high float64
}

// metricScales 收集每个指标列在本批结果里的最小、最大值，每列独立：
// 下载列不看上传列的值，延迟列只看延迟。速度、延迟、抖动为 0 表示
// 没测出来，不参加统计；丢包率 0% 是有效值，参加。
func metricScales(rows []ImageRow, mode speedtester.SpeedMode) map[int]metricScale {
	type bound struct {
		low, high float64
		count     int
	}
	accs := map[int]*bound{
		3: {}, 4: {}, 5: {}, 6: {},
	}
	if mode.UploadEnabled() {
		accs[7] = &bound{}
	}
	for _, row := range rows {
		if row.Result == nil {
			continue
		}
		r := row.Result
		values := map[int]float64{
			3: r.Latency.Seconds() * 1000,
			4: r.Jitter.Seconds() * 1000,
			5: r.PacketLoss,
			6: r.DownloadSpeed,
		}
		if mode.UploadEnabled() {
			values[7] = r.UploadSpeed
		}
		for index, value := range values {
			b := accs[index]
			if b == nil {
				continue
			}
			if index != 5 && value <= 0 {
				continue
			}
			if b.count == 0 {
				b.low, b.high = value, value
			} else {
				b.low = min(b.low, value)
				b.high = max(b.high, value)
			}
			b.count++
		}
	}
	scales := make(map[int]metricScale, len(accs))
	for index, b := range accs {
		if b.count > 0 {
			scales[index] = metricScale{low: b.low, high: b.high}
		}
	}
	return scales
}

// metricScore 给出单元格颜色的深浅：0 最浅、1 最深。
// 每列按本批的 min..max 归一化，全部同值时填最深。
func metricScore(result *speedtester.Result, index int, mode speedtester.SpeedMode, scales map[int]metricScale) float64 {
	if result == nil {
		return 0
	}
	switch index {
	case 3:
		return lowerIsBetter(result.Latency.Seconds()*1000, scales[index], true)
	case 4:
		return lowerIsBetter(result.Jitter.Seconds()*1000, scales[index], true)
	case 5:
		return lowerIsBetter(result.PacketLoss, scales[index], false)
	case 6:
		return higherIsBetter(result.DownloadSpeed, scales[index])
	case 7:
		if mode.UploadEnabled() {
			return higherIsBetter(result.UploadSpeed, scales[index])
		}
	}
	return 0
}

// lowerIsBetter 越小越好的指标：本列最小值得最深色。
// zeroMissing 为真时 0 表示没测出，按最浅处理（丢包率 0% 是有效值，照常归一化）。
func lowerIsBetter(value float64, scale metricScale, zeroMissing bool) float64 {
	if zeroMissing && value <= 0 {
		return 0
	}
	if scale.high <= scale.low {
		return 1 // 本批没有可比数据或全部同值：填最深。
	}
	return 1 - clamp01((value-scale.low)/(scale.high-scale.low))
}

// higherIsBetter 越大越好的指标：本列最大值得最深色，0 表示没测出。
func higherIsBetter(value float64, scale metricScale) float64 {
	if value <= 0 {
		return 0
	}
	if scale.high <= scale.low {
		return 1
	}
	return clamp01((value - scale.low) / (scale.high - scale.low))
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func greenScale(score float64) color.NRGBA {
	return lerpColor(score, color.NRGBA{0xc0, 0xeb, 0xf2, 0xff}, color.NRGBA{0x64, 0xc4, 0xcc, 0xff})
}

func redScale(score float64) color.NRGBA {
	return lerpColor(score, color.NRGBA{0xfa, 0xdf, 0xe4, 0xff}, color.NRGBA{0xfa, 0x0b, 0x55, 0xff})
}

func lerpColor(score float64, light, dark color.NRGBA) color.NRGBA {
	score = clamp01(score)
	mix := func(a, b uint8) uint8 {
		return uint8(float64(a) + score*float64(int(b)-int(a)))
	}
	return color.NRGBA{mix(light.R, dark.R), mix(light.G, dark.G), mix(light.B, dark.B), 0xff}
}

func scoreText(score float64) color.NRGBA {
	return color.NRGBA{0, 0, 0, 255}
}

func drawGrid(img *image.NRGBA, titleRows, rowHeight int, colWidths []int) {
	line := color.NRGBA{203, 213, 225, 255}
	width := img.Bounds().Dx()
	height := img.Bounds().Dy()
	top := titleRows * rowHeight
	for y := top; y < height; y += rowHeight {
		for x := 0; x < width; x++ {
			img.SetNRGBA(x, y, line)
		}
	}
	for y := height - 1; y >= top && y < height; y++ {
		break
	}
	for x := 0; x < width; x++ {
		img.SetNRGBA(x, height-1, line)
	}
	x := 0
	for _, w := range colWidths {
		for y := top; y < height; y++ {
			img.SetNRGBA(x, y, line)
		}
		x += w
	}
	for y := top; y < height; y++ {
		img.SetNRGBA(width-1, y, line)
	}
}

func drawCellText(img *image.NRGBA, faces imageFont, x, y, width, height int, text string, fg color.NRGBA, left bool) {
	if left {
		drawNameCell(img, faces, x, y, width, height, text, fg)
		return
	}
	drawCentered(img, faces, x, y, width, height, text, fg)
}

func drawNameCell(img *image.NRGBA, faces imageFont, x, y, width, height int, text string, fg color.NRGBA) {
	left := x + 8
	code, rest, strip := leadingFlag(text)
	if flag := flagImage(code); flag != nil {
		h := flag.Bounds().Dy()
		w := flag.Bounds().Dx()
		top := y + (height-h)/2
		dst := image.Rect(left, top, left+w, top+h)
		draw.Draw(img, dst, flag, flag.Bounds().Min, draw.Over)
		left += w + 6
		if strip {
			text = strings.TrimSpace(rest)
		}
	}
	available := x + width - 8 - left
	drawText(img, faces, left, y, height, truncateToWidth(faces.face, text, available), fg)
}

// leadingFlag 优先识别名字开头的国旗符号；没有时再认 JP、HK 这类国家码。
func leadingFlag(text string) (code, rest string, strip bool) {
	if code, rest, ok := splitLeadingFlag(text); ok {
		return code, rest, true
	}
	upper := strings.ToUpper(text)
	for _, token := range []string{"JP", "HK", "TW", "CN", "US", "SG", "KR", "GB", "UK", "DE", "FR"} {
		if strings.HasPrefix(upper, token) {
			code = strings.ToLower(token)
			if code == "uk" {
				code = "gb"
			}
			return code, text, false
		}
	}
	return "", text, false
}

func splitLeadingFlag(text string) (code, rest string, ok bool) {
	runes := []rune(text)
	if len(runes) < 2 || !isRegionalIndicator(runes[0]) || !isRegionalIndicator(runes[1]) {
		return "", text, false
	}
	code = string([]byte{byte(runes[0] - 0x1F1E6 + 'a'), byte(runes[1] - 0x1F1E6 + 'a')})
	return code, string(runes[2:]), true
}

func isRegionalIndicator(r rune) bool {
	return r >= 0x1F1E6 && r <= 0x1F1FF
}

func flagImage(code string) image.Image {
	data, err := flagFiles.ReadFile("flags/" + code + ".png")
	if err != nil {
		return nil
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	return img
}

func drawCentered(img *image.NRGBA, faces imageFont, x, y, width, height int, text string, fg color.NRGBA) {
	text = truncateToWidth(faces.face, text, width-8)
	tw := textWidth(faces.face, text)
	left := x + (width-tw)/2
	if left < x+4 {
		left = x + 4
	}
	metrics := faces.face.Metrics()
	baseline := y + (height+metrics.Ascent.Ceil()-metrics.Descent.Ceil())/2
	drawTextAt(img, faces, left, baseline, text, fg)
}

func drawText(img *image.NRGBA, faces imageFont, x, y, height int, text string, fg color.NRGBA) {
	metrics := faces.face.Metrics()
	baseline := y + (height+metrics.Ascent.Ceil()-metrics.Descent.Ceil())/2
	drawTextAt(img, faces, x, baseline, text, fg)
}

func drawTextAt(img *image.NRGBA, faces imageFont, x, baseline int, text string, fg color.NRGBA) {
	drawer := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(fg),
		Face: faces.face,
		Dot:  fixed.P(x, baseline),
	}
	drawFallbackString(drawer, faces.face, faces.emoji, text)
}

func stripIndexDots(rows []ImageRow) []ImageRow {
	out := make([]ImageRow, len(rows))
	for i, row := range rows {
		out[i] = row
		if len(row.Cells) == 0 {
			continue
		}
		cells := append([]string(nil), row.Cells...)
		cells[0] = strings.TrimSuffix(cells[0], ".")
		out[i].Cells = cells
	}
	return out
}

func fillRect(img *image.NRGBA, x, y, w, h int, c color.NRGBA) {
	bounds := img.Bounds()
	for dy := 0; dy < h; dy++ {
		py := y + dy
		if py < bounds.Min.Y || py >= bounds.Max.Y {
			continue
		}
		for dx := 0; dx < w; dx++ {
			px := x + dx
			if px < bounds.Min.X || px >= bounds.Max.X {
				continue
			}
			img.SetNRGBA(px, py, c)
		}
	}
}

// RemovePartialImages 删除导出目录里未完成的结果图临时文件。
func RemovePartialImages(dir string) error {
	clean, err := safeImageDir(dir)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(clean)
	if err != nil {
		return err
	}
	var first error
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, ".clash-speedtest-") || !strings.HasSuffix(name, ".png.part") {
			continue
		}
		if err := os.Remove(filepath.Join(clean, name)); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func safeImageDir(dir string) (string, error) {
	if dir == "" {
		dir = "."
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	clean := filepath.Clean(dir)
	if !filepath.IsAbs(clean) {
		clean = filepath.Join(cwd, clean)
	}
	rel, err := filepath.Rel(cwd, clean)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("image directory escapes working directory")
	}
	return clean, nil
}

// ImageFileName 生成本地时间戳文件名；同一秒冲突则加 -1、-2。
func ImageFileName(dir string, now time.Time) (string, error) {
	base := now.Format("20060102-150405")
	name := fmt.Sprintf("clash-speedtest-%s.png", base)
	path := filepath.Join(dir, name)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path, nil
	}
	for i := 1; i < 1000; i++ {
		candidate := filepath.Join(dir, fmt.Sprintf("clash-speedtest-%s-%d.png", base, i))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("too many result images in the same second")
}

// WriteResultImage 编码并写入 PNG。返回路径与缺字体警告（可为空）。
// 先写临时文件，编码或写入失败时删除半截，成功后再改成正式文件名。
func WriteResultImage(dir string, spec ImageSpec) (string, string, error) {
	face, err := LoadImageFont(spec.FontPath)
	if err != nil {
		return "", "", err
	}
	defer face.close()
	data, warning, err := RenderResultImage(spec, face)
	if err != nil {
		return "", "", err
	}
	if spec.Now.IsZero() {
		spec.Now = time.Now()
	}
	dir, err = safeImageDir(dir)
	if err != nil {
		return "", warning, err
	}
	path, err := ImageFileName(dir, spec.Now)
	if err != nil {
		return "", warning, err
	}
	tmp, err := os.CreateTemp(dir, ".clash-speedtest-*.png.part")
	if err != nil {
		return "", warning, err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return "", warning, err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", warning, err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return "", warning, err
	}
	return path, warning, nil
}

// ImageSpeedFilter 是 -image-speed-only 筛完后的图行和计数。
// 无效是已完成且两侧速度都是 0 的行；测试中是在测且两侧速度都是 0 的行。
type ImageSpeedFilter struct {
	Rows    []ImageRow
	Invalid int
	Testing int
}

// FilterImageRowsBySpeed 在 enabled 时只留下载或上传速度大于 0 的行，并重编序号。
// 关闭时原样返回，不计数。
func FilterImageRowsBySpeed(rows []ImageRow, enabled bool) ImageSpeedFilter {
	if !enabled {
		return ImageSpeedFilter{Rows: rows}
	}
	kept := make([]ImageRow, 0, len(rows))
	var invalid, testing int
	for _, row := range rows {
		if imageRowHasSpeed(row) {
			kept = append(kept, row)
			continue
		}
		if row.InFlight {
			testing++
			continue
		}
		invalid++
	}
	for i := range kept {
		if len(kept[i].Cells) == 0 {
			continue
		}
		cells := append([]string(nil), kept[i].Cells...)
		cells[0] = fmt.Sprintf("%d.", i+1)
		kept[i].Cells = cells
	}
	return ImageSpeedFilter{Rows: kept, Invalid: invalid, Testing: testing}
}

func imageRowHasSpeed(row ImageRow) bool {
	if row.Result != nil && (row.Result.DownloadSpeed > 0 || row.Result.UploadSpeed > 0) {
		return true
	}
	if row.InFlight {
		return inFlightCellHasSpeed(row.Cells)
	}
	return false
}

func inFlightCellHasSpeed(cells []string) bool {
	for _, index := range []int{6, 7} {
		if index >= len(cells) {
			continue
		}
		text := strings.TrimSpace(cells[index])
		if text != "" && text != "N/A" && text != "测试中" && text != "…" {
			return true
		}
	}
	return false
}

// AppendImageSpeedCounts 在摘要分数后补括号。某一项为 0 就省略，各项都是 0 不加括号。
// untested 是还没轮到测的节点数，通常等于总数减已完成减在测。
func AppendImageSpeedCounts(summary string, valid, invalid, testing, untested int) string {
	var parts []string
	if valid > 0 {
		parts = append(parts, fmt.Sprintf("有效 %d", valid))
	}
	if invalid > 0 {
		parts = append(parts, fmt.Sprintf("无效 %d", invalid))
	}
	if testing > 0 {
		parts = append(parts, fmt.Sprintf("测试中 %d", testing))
	}
	if untested > 0 {
		parts = append(parts, fmt.Sprintf("未测试 %d", untested))
	}
	if len(parts) == 0 {
		return summary
	}
	return summary + "（" + strings.Join(parts, "，") + "）"
}

// BuildImageRows 把完成结果转成图行。节点名用订阅原名。
func BuildImageRows(results []*speedtester.Result, mode speedtester.SpeedMode) []ImageRow {
	rows := make([]ImageRow, 0, len(results))
	for i, result := range results {
		if result == nil {
			continue
		}
		rows = append(rows, ImageRow{
			Cells:  FormatRow(result, mode, i),
			Result: result,
		})
	}
	return rows
}

// SourceLabel 把配置路径或订阅地址收成顶部标题，不带查询参数。
func SourceLabel(raw string) string {
	return sourceLabel(raw)
}

// wrapTextByWidth 把文本按显示宽度折成多行，超宽逐字符断开。
// 结果图顶部的文件名或链接可能很长，折起来画才不会遮出边界。
func wrapTextByWidth(face font.Face, text string, maxWidth int) []string {
	if text == "" {
		return nil
	}
	if maxWidth <= 0 || textWidth(face, text) <= maxWidth {
		return []string{text}
	}
	var lines []string
	var current strings.Builder
	currentWidth := 0
	for _, r := range text {
		w := textWidth(face, string(r))
		if currentWidth > 0 && currentWidth+w > maxWidth {
			lines = append(lines, current.String())
			current.Reset()
			currentWidth = 0
		}
		current.WriteRune(r)
		currentWidth += w
	}
	if current.Len() > 0 {
		lines = append(lines, current.String())
	}
	return lines
}

func sourceLabel(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parts := strings.Split(raw, ",")
	labels := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if i := strings.IndexAny(part, "?#"); i >= 0 {
			part = part[:i]
		}
		part = strings.TrimRight(part, "/")
		if strings.Contains(part, "://") {
			labels = append(labels, part)
			continue
		}
		labels = append(labels, filepath.Base(part))
	}
	return strings.Join(labels, "，")
}

// SummaryLine 是结果表图顶部一行：时间、模式、状态、已完成/总数。
func SummaryLine(now time.Time, mode speedtester.SpeedMode, status string, done, total int) string {
	modeLabel := "下载"
	switch {
	case mode.IsFast():
		modeLabel = "快速"
	case mode.UploadEnabled():
		modeLabel = "完整"
	}
	if status == "" {
		status = "已完成"
	}
	return fmt.Sprintf("%s  %s  %s  %d/%d", now.Format("2006-01-02 15:04:05"), modeLabel, status, done, total)
}

func basicFallbackFace() font.Face {
	// 无系统字体时仍能出图：用固定宽度的点阵近似不可用，调用方会收到警告。
	// 这里返回一个零尺寸 face 的替代：opentype 解析失败时由调用方处理。
	return emptyFace{}
}

type emptyFace struct{}

func (emptyFace) Close() error { return nil }
func (emptyFace) Glyph(dot fixed.Point26_6, r rune) (dr image.Rectangle, mask image.Image, maskp image.Point, advance fixed.Int26_6, ok bool) {
	return image.Rectangle{}, image.NewAlpha(image.Rect(0, 0, 8, 16)), image.Point{}, fixed.I(8), true
}
func (emptyFace) GlyphBounds(r rune) (bounds fixed.Rectangle26_6, advance fixed.Int26_6, ok bool) {
	return fixed.R(0, -12, 8, 4), fixed.I(8), true
}
func (emptyFace) GlyphAdvance(r rune) (advance fixed.Int26_6, ok bool) {
	if r == '\n' {
		return 0, true
	}
	return fixed.I(8), true
}
func (emptyFace) Kern(r0, r1 rune) fixed.Int26_6 { return 0 }
func (emptyFace) Metrics() font.Metrics {
	return font.Metrics{Height: fixed.I(20), Ascent: fixed.I(16), Descent: fixed.I(4)}
}

// JoinStatus 把保存成功与缺字体警告拼成一句。
func JoinStatus(saved string, warning string) string {
	saved = strings.TrimSpace(saved)
	warning = strings.TrimSpace(warning)
	if warning == "" {
		return saved
	}
	if saved == "" {
		return warning
	}
	return saved + "；" + warning
}
