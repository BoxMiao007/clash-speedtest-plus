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
	resultImageMaxWidth  = 4000
	resultImageFontSize  = 16
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
	candidates := []string{
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
// 节点名列按最长名字撑宽；总宽超过约 4000 像素时截断名字并加省略号。
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
	nameIndex := 1
	total := sumWidths(colWidths) + resultImageRowPadX*2
	if total > resultImageMaxWidth && nameIndex < len(colWidths) {
		overflow := total - resultImageMaxWidth
		minName := textWidth(fontFace.face, "…") + resultImageRowPadX
		if colWidths[nameIndex]-overflow < minName {
			overflow = colWidths[nameIndex] - minName
		}
		if overflow > 0 {
			colWidths[nameIndex] -= overflow
			rows = truncateNameColumn(fontFace.face, rows, colWidths[nameIndex]-resultImageRowPadX)
		}
	}

	lineHeight := fontFace.face.Metrics().Height.Ceil()
	if lineHeight <= 0 {
		lineHeight = resultImageFontSize + 4
	}
	rowHeight := lineHeight + resultImageRowPadY*2
	source := sourceLabel(spec.Source)
	titleRows := 0
	if source != "" {
		titleRows++
	}
	if spec.Summary != "" {
		titleRows++
	}
	height := (titleRows+1+len(rows))*rowHeight + resultImageLineGap
	width := sumWidths(colWidths)
	if width < 200 {
		width = 200
	}
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	fill(img, color.NRGBA{255, 255, 255, 255})

	y := 0
	if source != "" {
		drawRow(img, fontFace, y, rowHeight, colWidths, []string{source}, color.NRGBA{255, 255, 255, 255}, color.NRGBA{15, 23, 42, 255}, false)
		y += rowHeight
	}
	if spec.Summary != "" {
		drawRow(img, fontFace, y, rowHeight, colWidths, []string{spec.Summary}, color.NRGBA{255, 255, 255, 255}, color.NRGBA{71, 85, 105, 255}, false)
		y += rowHeight
	}
	drawHeaderRow(img, fontFace, y, rowHeight, colWidths, headers)
	y += rowHeight
	maxSpeed := maxSpeedInRows(rows, spec.Mode)
	for _, row := range rows {
		drawDataRow(img, fontFace, y, rowHeight, colWidths, spec.Mode, row, maxSpeed)
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
			w := textWidth(face, cell) + resultImageRowPadX
			if w > widths[i] {
				widths[i] = w
			}
		}
	}
	for i, header := range headers {
		if !isSpeedHeader(header) {
			continue
		}
		need := resultImageBarWidth + resultImageRowPadX
		for _, row := range rows {
			if i >= len(row.Cells) {
				continue
			}
			if w := textWidth(face, row.Cells[i]) + resultImageRowPadX*2; w > need {
				need = w
			}
		}
		if widths[i] < need {
			widths[i] = need
		}
	}
	return widths
}

func isSpeedHeader(header string) bool {
	return strings.Contains(header, "速度")
}

func truncateNameColumn(face font.Face, rows []ImageRow, maxWidth int) []ImageRow {
	out := make([]ImageRow, len(rows))
	for i, row := range rows {
		out[i] = row
		if len(row.Cells) < 2 {
			continue
		}
		cells := append([]string(nil), row.Cells...)
		cells[1] = truncateToWidth(face, cells[1], maxWidth)
		out[i].Cells = cells
	}
	return out
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
		drawCellText(img, faces, x, y, width, height, text, color.NRGBA{0, 0, 0, 255}, i == 1)
		x += width
	}
}

func drawDataRow(img *image.NRGBA, faces imageFont, y, height int, colWidths []int, mode speedtester.SpeedMode, row ImageRow, maxSpeed float64) {
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
			bg, _ := metricColors(row.Result, i, mode, maxSpeed, text)
			fillRect(img, x, y, width, height, bg)
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

func metricColors(result *speedtester.Result, index int, mode speedtester.SpeedMode, maxSpeed float64, text string) (color.NRGBA, color.NRGBA) {
	if result == nil || text == "" || text == "N/A" || text == "测试中" {
		return color.NRGBA{254, 226, 226, 255}, color.NRGBA{153, 27, 27, 255}
	}
	score := metricScore(result, index, mode, maxSpeed)
	if index == 6 || index == 7 {
		return redScale(score), scoreText(score)
	}
	return greenScale(score), scoreText(score)
}

func metricScore(result *speedtester.Result, index int, mode speedtester.SpeedMode, maxSpeed float64) float64 {
	switch index {
	case 3:
		return durationScore(result.Latency)
	case 4:
		return durationScore(result.Jitter)
	case 5:
		return 1 - clamp01(result.PacketLoss/30)
	default:
		return speedRatio(result, index, mode, maxSpeed)
	}
}

func durationScore(value time.Duration) float64 {
	if value <= 0 {
		return 0
	}
	ms := value.Seconds() * 1000
	return 1 - clamp01(ms/800)
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
	return paletteColor(score, []color.NRGBA{
		{0x97, 0xfb, 0xe6, 0xff},
		{0x74, 0xf2, 0xe3, 0xff},
		{0x21, 0xde, 0xde, 0xff},
	})
}

func redScale(score float64) color.NRGBA {
	return paletteColor(score, []color.NRGBA{
		{0xfa, 0xdf, 0xe4, 0xff},
		{0xfd, 0x47, 0x7d, 0xff},
		{0xfa, 0x0b, 0x55, 0xff},
	})
}

func paletteColor(score float64, palette []color.NRGBA) color.NRGBA {
	score = clamp01(score)
	index := int(score*float64(len(palette)-1) + 0.5)
	if index < 0 {
		index = 0
	}
	if index >= len(palette) {
		index = len(palette) - 1
	}
	return palette[index]
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
		drawNameCell(img, faces, x, y, height, text, fg)
		return
	}
	drawCentered(img, faces, x, y, width, height, text, fg)
}

func drawNameCell(img *image.NRGBA, faces imageFont, x, y, height int, text string, fg color.NRGBA) {
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
	drawText(img, faces, left, y, height, text, fg)
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

func speedRatio(result *speedtester.Result, index int, mode speedtester.SpeedMode, maxSpeed float64) float64 {
	speed := cellSpeed(result, index, mode)
	if speed <= 0 || maxSpeed <= 0 {
		return 0
	}
	return speed / maxSpeed
}

func maxSpeedInRows(rows []ImageRow, mode speedtester.SpeedMode) float64 {
	var maxSpeed float64
	indexes := []int{6}
	if mode.UploadEnabled() {
		indexes = append(indexes, 7)
	}
	for _, row := range rows {
		for _, index := range indexes {
			if speed := cellSpeed(row.Result, index, mode); speed > maxSpeed {
				maxSpeed = speed
			}
		}
	}
	return maxSpeed
}

func cellSpeed(result *speedtester.Result, index int, mode speedtester.SpeedMode) float64 {
	if result == nil {
		return 0
	}
	if mode.UploadEnabled() && index == 7 {
		return result.UploadSpeed
	}
	return result.DownloadSpeed
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
