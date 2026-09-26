package picker

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// 屏幕纵向预算：标题区 2 行、链接区 3 行、页脚 2 行，其余给上半分栏。
const (
	headerLines = 2 // 标题 + 分隔线
	footerLines = 2 // 状态 + 帮助

	// 终端超过这个宽度时内容不再拉宽，整块居中，长行不再稀疏。
	maxContentWidth = 100
)

// layout 是一次渲染的分区位置。View 按它画，鼠标和滚轮按它定位，
// 改布局只动这里。
type layout struct {
	width    int
	paneY    int // 上半分栏起始行
	paneH    int // 分栏高度
	filesX   int // 文件栏起始列
	filesW   int
	optionsX int // 选项栏起始列（分隔线后一格）
	optionsW int
	addressY int // 链接行
	helpY    int // 帮助行，含「回车」「Ctrl+C」两段可点热区
}

// computeLayout 算出各区的位置。尺寸未知时各栏按内容展开、不滚动。
func (m Model) computeLayout() layout {
	width := m.contentWidth()
	lo := layout{width: width, paneY: headerLines}

	longest := 0
	for _, config := range m.configs {
		if w := lipgloss.Width(config.Name); w > longest {
			longest = w
		}
	}
	if len(m.configs) == 0 {
		longest = lipgloss.Width("（程序目录里没有可勾选的 .yaml）")
	}
	// 文件名之外再给灰行原因留出宽度，但最多占一半屏宽。
	filesW := max(longest+20, 24)
	if width > 0 {
		filesW = min(filesW, width/2)
	}
	lo.filesX = 0
	lo.filesW = filesW
	lo.optionsX = filesW + 1
	lo.optionsW = width - lo.optionsX
	if width <= 0 {
		// 宽度未知时渲染不限宽；filesW 只留给鼠标分界用。
		lo.optionsW = 0
	}

	need := max(len(m.configs), len(optionOrder)) + 1
	if m.height <= 0 {
		lo.paneH = need
		lo.addressY = headerLines + need + 1
		lo.helpY = lo.addressY + 3
		return lo
	}
	// 标题 2 行、链接区 3 行、页脚 2 行之外的屏幕都给分栏。
	lo.paneH = min(need, max(m.height-headerLines-footerLines-3, 3))
	lo.addressY = headerLines + lo.paneH + 1
	lo.helpY = lo.addressY + 3
	return lo
}

func (m Model) contentWidth() int {
	if m.width <= 0 {
		return 0
	}
	return min(m.width, maxContentWidth)
}

// View 画出标题、分栏、链接和页脚。超宽终端上整块居中；行宽始终受控。
func (m Model) View() string {
	lo := m.computeLayout()
	lines := make([]string, 0, m.height+2)

	lines = append(lines, m.headerLine(lo.width), ruleLine(lo.width, ""))
	for row := 0; row < lo.paneH; row++ {
		lines = append(lines, m.paneLine(lo, row))
	}
	lines = append(lines, ruleLine(lo.width, ""), m.addressLine(lo), ruleLine(lo.width, ""))
	lines = append(lines, m.statusLine(lo.width), m.helpLine(lo.width))

	block := strings.Join(lines, "\n")
	if lo.width > 0 && m.width > lo.width {
		return lipgloss.PlaceHorizontal(m.width, lipgloss.Center, block)
	}
	return block
}

// paneLine 拼出分栏的一行：左文件、分隔线、右选项。
func (m Model) paneLine(lo layout, row int) string {
	left := m.fileLine(lo, m.configScroll+row)
	if lo.width <= 0 {
		return left + " " + sectionMarkStyle.Render("│") + " " + m.optionLine(lo, m.optionScroll+row)
	}
	leftW := min(lo.filesW, lo.width)
	rightW := max(lo.width-leftW-1, 1)
	return padPlain(truncatePlain(left, leftW), leftW) +
		greyStyle.Render("│") +
		padPlain(truncatePlain(m.optionLine(lo, m.optionScroll+row), rightW), rightW)
}

// fileLine 画文件栏的一行：row 越界时是空白。
func (m Model) fileLine(lo layout, row int) string {
	width := lo.filesW
	if lo.width <= 0 {
		width = 0 // 宽度未知时不截断
	}
	if row == 0 {
		return joinParts(width,
			part{"▎", sectionMarkStyle},
			part{" 文件", sectionStyle},
		)
	}
	index := row - 1
	if len(m.configs) == 0 {
		if index == 0 {
			return joinParts(width, part{"（程序目录里没有可勾选的 .yaml）", greyStyle})
		}
		return ""
	}
	if index >= len(m.configs) {
		return ""
	}
	config := m.configs[index]
	mark := "○"
	markStyle := dimStyle
	nameStyle := plainStyle
	text := config.Name
	if config.Selectable && m.checked[index] {
		mark = "✓"
		markStyle = okStyle
	}
	if !config.Selectable {
		mark = "×"
		markStyle = greyStyle
		nameStyle = greyStyle
		if config.Reason != "" {
			text += "  " + config.Reason
		}
	}
	if m.focus == focusConfigs && m.cursor == index {
		return selectedLine(mark+" "+text, width)
	}
	return joinParts(width,
		part{mark, markStyle},
		part{" " + text, nameStyle},
	)
}

// optionLine 画选项栏的一行。row 0 是节标题，之后每个选项一行。
func (m Model) optionLine(lo layout, row int) string {
	width := lo.optionsW
	if lo.width <= 0 {
		width = 0 // 宽度未知时不截断
	}
	if row == 0 {
		return joinParts(width,
			part{"▎", sectionMarkStyle},
			part{" 选项", sectionStyle},
		)
	}
	index := row - 1
	if index >= len(optionOrder) {
		return ""
	}
	optionRow := optionOrder[index]
	enabled := m.optionState().Enabled(optionRow.option)
	focused := m.focus == focusOptions && m.optionIndex == index

	value := m.optionValue(optionRow, focused)
	valueStyle := plainStyle
	switch {
	case optionRow.kind == kindBool && value == "开":
		valueStyle = okStyle
	case optionRow.kind == kindBool:
		valueStyle = dimStyle
	case optionRow.kind == kindText && (strings.HasPrefix(value, "默认") || value == "不输出"):
		valueStyle = dimStyle
	}

	prefix := "  " + padDisplay(optionRow.label, optionLabelWidth())
	plain := prefix + "  " + value
	if focused {
		shown := plain
		if !enabled {
			shown += "  不可用"
		}
		return selectedLine(shown, width)
	}
	if !enabled {
		// 「不可用」用斜体，和灰掉的原因区分开；先截断再拼接保证不超宽。
		suffix := "  不可用"
		base := width
		if width > 0 {
			base = max(width-lipgloss.Width(suffix), 0)
		}
		return greyStyle.Render(padPlain(truncatePlain(plain, base), base)) + disabledStyle.Render(suffix)
	}
	return joinParts(width,
		part{prefix, labelStyle},
		part{"  ", plainStyle},
		part{value, valueStyle},
	)
}

func (m Model) addressLine(lo layout) string {
	width := lo.width
	value := m.address
	valueStyle := plainStyle
	if value == "" {
		value = "（多条地址用「逗号加空格」或换行分开）"
		valueStyle = dimStyle
	} else if m.focus == focusAddress {
		value += "▌"
	}
	plain := "  " + padDisplay("订阅地址", optionLabelWidth()) + "  " + value
	if m.focus == focusAddress {
		return selectedLine(plain, width)
	}
	return joinParts(width,
		part{"  " + padDisplay("订阅地址", optionLabelWidth()), labelStyle},
		part{"  ", plainStyle},
		part{value, valueStyle},
	)
}

func (m Model) headerLine(width int) string {
	left := joinParts(0,
		part{"clash-speedtest", titleStyle},
		part{" · 选源界面", titleDimStyle},
	)
	if len(m.configs) == 0 || width <= 0 {
		return padPlain(left, width)
	}
	checked := 0
	for i, config := range m.configs {
		if config.Selectable && m.checked[i] {
			checked++
		}
	}
	info := titleDimStyle.Render(fmt.Sprintf("已勾 %d/%d", checked, len(m.configs)))
	if lipgloss.Width(left)+lipgloss.Width(info) > width {
		// 放不下统计时只留标题。
		return padPlain(truncatePlain(left, width), width)
	}
	gap := width - lipgloss.Width(left) - lipgloss.Width(info)
	return left + strings.Repeat(" ", gap) + info
}

// ruleLine 画分隔线，marker 是行尾的滚动提示（▲ 还有上文、▼ 还有下文）。
func ruleLine(width int, marker string) string {
	if width <= 0 {
		if marker == "" {
			marker = "─"
		}
		return ruleStyle.Render(strings.Repeat("─", 2) + marker)
	}
	if marker == "" {
		return ruleStyle.Render(strings.Repeat("─", width))
	}
	return ruleStyle.Render(strings.Repeat("─", width-lipgloss.Width(marker))) + ruleStyle.Render(marker)
}

func (m Model) statusLine(width int) string {
	switch {
	case m.fetching:
		return joinParts(width,
			part{"● ", warnStyle},
			part{m.status + " · 按 Ctrl+C 取消", warnStyle},
		)
	case m.status != "":
		style := warnStyle
		if strings.HasPrefix(m.status, "已用") {
			style = okStyle
		}
		return joinParts(width, part{"● ", style}, part{m.status, style})
	default:
		return joinParts(width, part{"●", greyStyle})
	}
}

// helpSpan 是帮助行的一段。hit 标记段是哪个热区：渲染高亮和点击定位共用。
type helpSpan struct {
	key   string
	label string
	hit   int // helpHitNone / helpHitEnter / helpHitQuit
}

var helpSpans = []helpSpan{
	{key: "空格", label: " 勾选/开关"},
	{key: "←→", label: " 切区/调参数"},
	{key: "↑↓/Tab", label: " 移动"},
	{key: "Enter", label: " 开始测速", hit: helpHitEnter},
	{key: "Ctrl+C", label: " 退出", hit: helpHitQuit},
}

func (m Model) helpLine(width int) string {
	parts := make([]part, 0, len(helpSpans)*3)
	for i, span := range helpSpans {
		if i > 0 {
			parts = append(parts, part{"   ", labelStyle})
		}
		style := keyStyle
		if span.hit != helpHitNone {
			style = helpActiveStyle
		}
		parts = append(parts, part{span.key, style}, part{span.label, labelStyle})
	}
	return joinParts(width, parts...)
}

// helpZones 算出可点段在帮助行内的 X 范围（内容坐标）。
// 帮助行怎么画就从这里怎么量，两处不会漂移。
func helpZones() (enter, quit [2]int) {
	x := 0
	for i, span := range helpSpans {
		if i > 0 {
			x += 3
		}
		w := lipgloss.Width(span.key) + lipgloss.Width(span.label)
		if span.hit != helpHitNone {
			if span.hit == helpHitEnter {
				enter = [2]int{x, x + w}
			} else {
				quit = [2]int{x, x + w}
			}
		}
		x += w
	}
	return
}

func selectedLine(plain string, width int) string {
	return selectedStyle.Render(padPlain(truncatePlain(plain, width), width))
}

// optionValue 给出该行显示的值。可调行带「< >」符号提示点击方向，
// 下载/上传大小显示 MB 单位；文本行留空显示「默认 (…)」，输出路径例外。
func (m Model) optionValue(row optionRow, focused bool) string {
	if row.kind == kindBool {
		return m.options.value(row.option)
	}
	if row.kind == kindMode {
		return "< " + modeLabel(m.options.Mode) + " >"
	}
	value := m.options.value(row.option)
	if strings.TrimSpace(value) == "" {
		if row.option == OptionOutputPath {
			return "不输出"
		}
		if hint := defaultHint(row.option); hint != "" {
			return hint
		}
		return "默认"
	}
	if optionAdjustable(row.option) {
		shown := value
		if row.option == OptionDownloadSize || row.option == OptionUploadSize {
			shown += "MB"
		}
		if focused {
			shown = value + "▌"
			if row.option == OptionDownloadSize || row.option == OptionUploadSize {
				shown += "MB"
			}
		}
		return "< " + shown + " >"
	}
	if focused {
		return value + "▌"
	}
	return value
}

// optionLabelWidth 是最长的选项标签宽度，链接行与选项行的值按它对齐。
func optionLabelWidth() int {
	width := 0
	for _, row := range optionOrder {
		if w := lipgloss.Width(row.label); w > width {
			width = w
		}
	}
	return width
}

func modeLabel(mode string) string {
	switch mode {
	case "fast":
		return "快速"
	case "full":
		return "完整"
	default:
		return "下载"
	}
}

type part struct {
	text  string
	style lipgloss.Style
}

// joinParts 先按显示宽度截断纯文本，再逐段上色，最后补齐行宽。
// 先截断后上色是为了不把 ANSI 序列拦腰砍断。
func joinParts(width int, parts ...part) string {
	total := 0
	rendered := make([]string, 0, len(parts))
	for _, p := range parts {
		w := lipgloss.Width(p.text)
		if width > 0 && total+w > width {
			remaining := width - total
			if remaining > 0 {
				if cut := truncatePlain(p.text, remaining); cut != "" {
					rendered = append(rendered, p.style.Render(cut))
				}
			}
			total = width
			break
		}
		rendered = append(rendered, p.style.Render(p.text))
		total += w
	}
	line := strings.Join(rendered, "")
	if width > 0 && total < width {
		line += strings.Repeat(" ", width-total)
	}
	return line
}

func padPlain(line string, width int) string {
	if width <= 0 {
		return line
	}
	gap := width - lipgloss.Width(line)
	if gap <= 0 {
		return line
	}
	return line + strings.Repeat(" ", gap)
}

// padDisplay 把文本按显示宽度补齐，中文按两个单元格计。
func padDisplay(text string, width int) string {
	return text + strings.Repeat(" ", max(width-lipgloss.Width(text), 0))
}

// truncatePlain 按显示宽度截断，超长时以「…」收尾。
func truncatePlain(text string, width int) string {
	if width <= 0 || lipgloss.Width(text) <= width {
		return text
	}
	var b strings.Builder
	current := 0
	for _, r := range text {
		rw := runewidth.RuneWidth(r)
		if current+rw >= width {
			break
		}
		b.WriteRune(r)
		current += rw
	}
	return b.String() + "…"
}
