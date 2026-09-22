package output

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
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

const (
	resultImageMaxWidth = 4000
	resultImageFontSize = 18
	resultImageRowPadX  = 16
	resultImageRowPadY  = 10
	resultImageLineGap  = 8
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
	candidates := []string{
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
	rows := spec.Rows
	colWidths := measureColumns(fontFace.face, headers, rows)
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
	headerRows := 2 // 摘要 + 表头
	height := (headerRows+len(rows))*rowHeight + resultImageLineGap
	width := sumWidths(colWidths) + resultImageRowPadX*2
	if width < 200 {
		width = 200
	}
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	fill(img, color.NRGBA{255, 255, 255, 255})

	y := 0
	drawRow(img, fontFace, y, rowHeight, colWidths, []string{spec.Summary}, color.NRGBA{245, 247, 250, 255}, color.NRGBA{30, 41, 59, 255}, false)
	y += rowHeight
	drawRow(img, fontFace, y, rowHeight, colWidths, headers, color.NRGBA{226, 232, 240, 255}, color.NRGBA{15, 23, 42, 255}, true)
	y += rowHeight
	for i, row := range rows {
		bg := color.NRGBA{255, 255, 255, 255}
		if i%2 == 1 {
			bg = color.NRGBA{248, 250, 252, 255}
		}
		fg := color.NRGBA{15, 23, 42, 255}
		if row.InFlight {
			fg = color.NRGBA{148, 163, 184, 255}
		}
		drawRow(img, fontFace, y, rowHeight, colWidths, row.Cells, bg, fg, false)
		if !row.InFlight && row.Result != nil {
			paintThresholds(img, fontFace, y, rowHeight, colWidths, spec.Mode, row)
		}
		y += rowHeight
	}

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
	return widths
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
	for x := 0; x < img.Bounds().Dx(); x++ {
		for dy := 0; dy < height && y+dy < img.Bounds().Dy(); dy++ {
			img.SetNRGBA(x, y+dy, bg)
		}
	}
	drawer := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(fg),
		Face: faces.face,
	}
	x := resultImageRowPadX / 2
	baseline := y + height - resultImageRowPadY
	for i, width := range colWidths {
		text := ""
		if i < len(cells) {
			text = cells[i]
		}
		if bold {
			_ = bold
		}
		drawer.Dot = fixed.P(x, baseline)
		drawFallbackString(drawer, faces.face, faces.emoji, text)
		x += width
	}
}

func paintThresholds(img *image.NRGBA, faces imageFont, y, height int, colWidths []int, mode speedtester.SpeedMode, row ImageRow) {
	result := row.Result
	if result == nil || len(row.Cells) < 4 {
		return
	}
	// 列顺序与 GetHeaders 一致：序号、名称、类型、延迟、抖动、丢包、下载、上传。
	paintCell(img, faces, y, height, colWidths, 3, row.Cells[3], latencyColor(result.Latency))
	if mode.IsFast() || len(row.Cells) < 7 {
		return
	}
	paintCell(img, faces, y, height, colWidths, 4, row.Cells[4], latencyColor(result.Jitter))
	paintCell(img, faces, y, height, colWidths, 5, row.Cells[5], lossColor(result.PacketLoss))
	paintCell(img, faces, y, height, colWidths, 6, row.Cells[6], speedColor(result.DownloadSpeed, 10, 5))
	if mode.UploadEnabled() && len(row.Cells) > 7 {
		paintCell(img, faces, y, height, colWidths, 7, row.Cells[7], speedColor(result.UploadSpeed, 5, 2))
	}
}

func paintCell(img *image.NRGBA, faces imageFont, y, height int, colWidths []int, index int, text string, fg color.NRGBA) {
	x := resultImageRowPadX / 2
	for i := 0; i < index && i < len(colWidths); i++ {
		x += colWidths[i]
	}
	drawer := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(fg),
		Face: faces.face,
		Dot:  fixed.P(x, y+height-resultImageRowPadY),
	}
	drawFallbackString(drawer, faces.face, faces.emoji, text)
}

func latencyColor(value time.Duration) color.NRGBA {
	if value > 0 && value < 800*time.Millisecond {
		return color.NRGBA{22, 163, 74, 255}
	}
	if value > 0 && value < 1500*time.Millisecond {
		return color.NRGBA{202, 138, 4, 255}
	}
	return color.NRGBA{220, 38, 38, 255}
}

func lossColor(value float64) color.NRGBA {
	if value < 10 {
		return color.NRGBA{22, 163, 74, 255}
	}
	if value < 20 {
		return color.NRGBA{202, 138, 4, 255}
	}
	return color.NRGBA{220, 38, 38, 255}
}

func speedColor(bytesPerSecond float64, greenMB, yellowMB float64) color.NRGBA {
	mb := bytesPerSecond / (1024 * 1024)
	if mb >= greenMB {
		return color.NRGBA{22, 163, 74, 255}
	}
	if mb >= yellowMB {
		return color.NRGBA{202, 138, 4, 255}
	}
	return color.NRGBA{220, 38, 38, 255}
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
