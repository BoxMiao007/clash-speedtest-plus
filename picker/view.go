package picker

import (
	"fmt"
	"strconv"
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

	// 名称列固定宽度上限：到 40 个显示格（中文按 2 格）自动折行。
	// 序号列与节点数列不占这 40 格。
	nameColumnMaxWidth = 40
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
		if w := lipgloss.Width(m.entryText(config)); w > longest {
			longest = w
		}
	}
	if len(m.configs) == 0 {
		longest = lipgloss.Width("（程序目录里没有可勾选的 .yaml）")
	}
	// 名称列最多 40 显示格，超出的部分靠折行而不是撑栏；名称之外依次是
	// 勾选符、序号列、节点数列，整栏宽度最多占一半屏宽。
	nameW := min(longest, nameColumnMaxWidth)
	filesW := 2 + m.indexWidth() + nameW
	if countW := m.maxCountWidth(); countW > 0 {
		filesW += 2 + countW
	}
	filesW = max(filesW, 24)
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

	// 分栏高度按折行后的条目总高预算。
	entryRows := 0
	for i := range m.configs {
		entryRows += m.entryHeight(lo, i)
	}
	need := max(entryRows, len(optionOrder)) + 1
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
	leftRows := m.filePaneRows(lo)
	for row := 0; row < lo.paneH; row++ {
		left := ""
		if row < len(leftRows) {
			left = leftRows[row]
		}
		lines = append(lines, m.paneLine(lo, left, row))
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
func (m Model) paneLine(lo layout, left string, row int) string {
	if lo.width <= 0 {
		return left + " " + sectionMarkStyle.Render("│") + " " + m.optionLine(lo, m.optionScroll+row)
	}
	leftW := min(lo.filesW, lo.width)
	rightW := max(lo.width-leftW-1, 1)
	return padPlain(truncatePlain(left, leftW), leftW) +
		greyStyle.Render("│") +
		padPlain(truncatePlain(m.optionLine(lo, m.optionScroll+row), rightW), rightW)
}

// filePaneRows 渲染文件栏可见的一窗：row 0 是节标题，之后从 configScroll 条目起
// 按条目整块放入（名称折行的条目占多行），放不下的条目整块留白；
// 首条目比窗口还高是唯一的例外，只截断它自己。宽度未知时各栏按内容展开。
func (m Model) filePaneRows(lo layout) []string {
	width := lo.filesW
	rows := make([]string, 0, lo.paneH)
	rows = append(rows, joinParts(width, lipgloss.Style{},
		part{"▎", sectionMarkStyle},
		part{" 文件", sectionStyle},
	))
	if len(m.configs) == 0 {
		if lo.paneH > 1 {
			rows = append(rows, joinParts(width, lipgloss.Style{}, part{"（程序目录里没有可勾选的 .yaml）", greyStyle}))
		}
		return padRows(rows, lo.paneH)
	}
	end := m.visibleEntriesEnd(lo)
	for i := m.configScroll; i < end && len(rows) < lo.paneH; i++ {
		rows = append(rows, m.entryRows(lo, i)...)
	}
	return padRows(rows, lo.paneH)
}

func padRows(rows []string, height int) []string {
	for len(rows) < height {
		rows = append(rows, "")
	}
	return rows
}

// entryRows 渲染一个条目的全部行：首行是勾选符+序号+名称首段+右对齐节点数，
// 折行的续行缩进对齐名称起点。选中时整条高亮。
func (m Model) entryRows(lo layout, index int) []string {
	width := lo.filesW
	if lo.width <= 0 {
		width = 0 // 宽度未知时不截断
	}
	config := m.configs[index]
	mark := "○"
	markStyle := dimStyle
	nameStyle := plainStyle
	text := m.entryText(config)
	// 节点数右对齐到文件栏右缘；远程 providers 的数量不可知：
	// 本地 37 个加远程写作 37+，纯 providers 只写 +。
	count := nodeCountText(config)
	if config.Selectable && m.checked[index] {
		mark = "✓"
		markStyle = okStyle
	}
	if !config.Selectable {
		mark = "×"
		markStyle = greyStyle
		nameStyle = greyStyle
	}
	// 续行缩进对齐名称起点：勾选符 + 空格 + 序号列。
	indent := strings.Repeat(" ", 2+m.indexWidth())
	chunks := wrapDisplay(text, m.nameColWidth(lo))
	selected := m.focus == focusConfigs && m.cursor == index
	rows := make([]string, 0, len(chunks))
	left := mark + " " + m.indexPrefix(index) + chunks[0]
	if selected {
		rows = append(rows, selectedLine(withTrailingCount(left, count, width), width))
		for _, chunk := range chunks[1:] {
			rows = append(rows, selectedLine(indent+chunk, width))
		}
		return rows
	}
	if count == "" {
		rows = append(rows, joinParts(width, lipgloss.Style{},
			part{mark, markStyle},
			part{strings.TrimPrefix(left, mark), nameStyle},
		))
	} else {
		// 节点数颜色压暗；名称太长时截短，数字始终完整露出。
		line := withTrailingCount(left, count, width)
		idx := strings.LastIndex(line, count)
		head, tail := line[:idx], line[idx:]
		rows = append(rows, joinParts(width, lipgloss.Style{},
			part{mark, markStyle},
			part{strings.TrimPrefix(head, mark), nameStyle},
			part{tail, dimStyle},
		))
	}
	for _, chunk := range chunks[1:] {
		rows = append(rows, joinParts(width, lipgloss.Style{}, part{indent + chunk, nameStyle}))
	}
	return rows
}

// entryText 是条目参与折行的完整文本：灰行把原因文案接在名称后面。
func (m Model) entryText(config ConfigEntry) string {
	if !config.Selectable && config.Reason != "" {
		return config.Name + "  " + config.Reason
	}
	return config.Name
}

// nodeCountText 是可勾选条目的节点数文案：本地个数，加远程未补 +。
func nodeCountText(config ConfigEntry) string {
	if !config.Selectable {
		return ""
	}
	if config.Nodes == 0 && config.More {
		return "+"
	}
	count := strconv.Itoa(config.Nodes)
	if config.More {
		count += "+"
	}
	return count
}

// maxCountWidth 是可勾选条目里节点数文案（如 37+）的最大显示宽度，没有则为 0。
func (m Model) maxCountWidth() int {
	w := 0
	for _, config := range m.configs {
		if cw := lipgloss.Width(nodeCountText(config)); cw > w {
			w = cw
		}
	}
	return w
}

// nameColWidth 是名称列的实际折行宽度：栏宽扣掉勾选符、序号列、节点数列，
// 窄终端优先压名称列。
func (m Model) nameColWidth(lo layout) int {
	w := lo.filesW
	if w <= 0 {
		return nameColumnMaxWidth
	}
	w -= 2 + m.indexWidth()
	if countW := m.maxCountWidth(); countW > 0 {
		w -= 2 + countW
	}
	return max(w, 4)
}

// entryHeight 是条目折行后占的行数。
func (m Model) entryHeight(lo layout, index int) int {
	return len(wrapDisplay(m.entryText(m.configs[index]), m.nameColWidth(lo)))
}

// entryOffsets 给出每个条目在文件栏内容里的起始行，及内容总行数。
func (m Model) entryOffsets(lo layout) ([]int, int) {
	offsets := make([]int, len(m.configs))
	row := 0
	for i := range m.configs {
		offsets[i] = row
		row += m.entryHeight(lo, i)
	}
	return offsets, row
}

// wrapDisplay 按显示宽度把文本折行：中文按 2 格计，到格即折，不保留整词。
func wrapDisplay(text string, width int) []string {
	if width <= 0 {
		width = nameColumnMaxWidth
	}
	lines := []string{""}
	cur := 0
	for _, r := range text {
		rw := runewidth.RuneWidth(r)
		if cur+rw > width {
			lines = append(lines, "")
			cur = 0
		}
		lines[len(lines)-1] += string(r)
		cur += rw
	}
	return lines
}

// withTrailingCount 把节点数右对齐拼到行尾。行宽未知时不补齐，直接跟在名称后。
func withTrailingCount(left, count string, width int) string {
	if width <= 0 {
		return left + "  " + count
	}
	gap := width - lipgloss.Width(left) - lipgloss.Width(count)
	if gap < 2 {
		// 名称太长就把名称截短，让节点数始终完整露出。
		left = truncatePlain(left, width-lipgloss.Width(count)-2)
		gap = width - lipgloss.Width(left) - lipgloss.Width(count)
	}
	return left + strings.Repeat(" ", gap) + count
}

// indexPrefix 给出条目的序号前缀：位数按列表最大数右对齐补齐，
// 所有行名称起点一致。
func (m Model) indexPrefix(index int) string {
	w := m.indexWidth()
	if w <= 0 {
		return ""
	}
	return fmt.Sprintf("%*d. ", w-2, index+1)
}

// indexWidth 是序号列（含「.」和尾随空格）的宽度；没有条目时为 0。
func (m Model) indexWidth() int {
	if len(m.configs) == 0 {
		return 0
	}
	return len(fmt.Sprintf("%d", len(m.configs))) + 2
}

// optionLine 画选项栏的一行。row 0 是节标题，之后每个选项一行。
func (m Model) optionLine(lo layout, row int) string {
	width := lo.optionsW
	if lo.width <= 0 {
		width = 0 // 宽度未知时不截断
	}
	if row == 0 {
		return joinParts(width, lipgloss.Style{},
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
	case optionRow.kind == kindText && strings.HasPrefix(value, "默认"):
		valueStyle = dimStyle
	case optionRow.kind == kindOutput && m.outputModePlaceholder():
		valueStyle = dimStyle
	}

	prefix := "  " + padDisplay(optionRow.label, optionLabelWidth())
	plain := prefix + "  " + value
	if focused {
		if hint := m.outputSuffixHint(optionRow); hint != "" {
			// 输出路径行聚焦时，缺的后缀段灰显在光标前（见 CONTEXT.md
			// 「后缀补全」词条）。此时值非空，行尾必是 optionValue 补的光标。
			// 行尾补白传选中样式，整行高亮与 selectedLine 一致。
			return joinParts(width, selectedStyle,
				part{strings.TrimSuffix(plain, "▌"), selectedStyle},
				part{hint, greyStyle},
				part{"▌", selectedStyle},
			)
		}
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
	return joinParts(width, lipgloss.Style{},
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
	return joinParts(width, lipgloss.Style{},
		part{"  " + padDisplay("订阅地址", optionLabelWidth()), labelStyle},
		part{"  ", plainStyle},
		part{value, valueStyle},
	)
}

func (m Model) headerLine(width int) string {
	left := joinParts(0, lipgloss.Style{},
		part{"clash-speedtest-plus", titleStyle},
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
		return joinParts(width, lipgloss.Style{},
			part{"● ", warnStyle},
			part{m.status + " · 按 Ctrl+C 取消", warnStyle},
		)
	case m.status != "":
		style := warnStyle
		if strings.HasPrefix(m.status, "已用") {
			style = okStyle
		}
		return joinParts(width, lipgloss.Style{}, part{"● ", style}, part{m.status, style})
	default:
		return joinParts(width, lipgloss.Style{}, part{"●", greyStyle})
	}
}

// helpSpan 是帮助行的一段。hit 标记段是哪个热区：渲染高亮和点击定位共用。
type helpSpan struct {
	key   string
	label string
	hit   int // helpHitNone / helpHitEnter / helpHitQuit
}

func (m Model) helpSpans() []helpSpan {
	spans := []helpSpan{
		{key: "空格", label: " 勾选/开关"},
		{key: "←→", label: " 切区/调参数"},
		{key: "↑↓/Tab", label: " 移动"},
		{key: "Enter", label: " 开始测速", hit: helpHitEnter},
		{key: "Ctrl+C", label: " 退出", hit: helpHitQuit},
	}
	// 多源会连测多轮，回车前就把轮数亮出来，免得以为漏了源。
	if n := m.expectedRounds(); n > 1 {
		for i, span := range spans {
			if span.hit == helpHitEnter {
				spans[i].label = fmt.Sprintf(" 开始测速(共 %d 轮)", n)
			}
		}
	}
	return spans
}

// expectedRounds 估算回车后要连测的轮数：勾选的配置数加地址框里拆出的订阅数。
func (m Model) expectedRounds() int {
	n := len(SplitSubscriptionText(m.address))
	for i, config := range m.configs {
		if config.Selectable && m.checked[i] {
			n++
		}
	}
	return n
}

func (m Model) helpLine(width int) string {
	spans := m.helpSpans()
	parts := make([]part, 0, len(spans)*3)
	for i, span := range spans {
		if i > 0 {
			parts = append(parts, part{"   ", labelStyle})
		}
		style := keyStyle
		if span.hit != helpHitNone {
			style = helpActiveStyle
		}
		parts = append(parts, part{span.key, style}, part{span.label, labelStyle})
	}
	return joinParts(width, lipgloss.Style{}, parts...)
}

// helpZones 算出可点段在帮助行内的 X 范围（内容坐标）。
// 帮助行怎么画就从这里怎么量，两处不会漂移。
func (m Model) helpZones() (enter, quit [2]int) {
	x := 0
	for i, span := range m.helpSpans() {
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

// outputSuffixHint 是聚焦输出路径行时该灰显的缺省后缀段；自定义态没打字
// 或后缀已写全时为空，关闭/默认当前路径态没有编辑中的词干也不显示。只有
// 输出路径一行有灰显提示，其余文本行原样。
func (m Model) outputSuffixHint(row optionRow) string {
	if row.option != OptionOutputPath {
		return ""
	}
	if m.options.OutputMode != OutputModeCustom {
		return ""
	}
	value := m.options.OutputPath
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return MissingYAMLSuffix(value)
}

// outputModeValue 给出输出模式行的显示值（见 CONTEXT.md「输出模式」词条）：
// 关闭与默认当前路径带 < > 提示左右键和点击可循环；自定义态显示词干，
// 没打字时提示态名并可接着输入。
func (m Model) outputModeValue(focused bool) string {
	switch m.options.OutputMode {
	case OutputModeCustom:
		value := m.options.OutputPath
		if strings.TrimSpace(value) == "" {
			// 没打字的自定义态与关闭/默认态一样可循环，带 < > 提示；
			// 聚焦时光标留在值内示意等输入。判空口径与 options.go 的
			// outputModeLocked 同源：有内容即编辑锁定、退回纯词干。
			if focused {
				return "< 自定义▌ >"
			}
			return "< 自定义 >"
		}
		if focused {
			return value + "▌"
		}
		return value
	case OutputModeDefaultPath:
		return "< 默认当前路径 >"
	default:
		return "< 关闭 >"
	}
}

// outputModePlaceholder 报告输出模式行的值是不是占位提示：关闭态、
// 自定义态还没打字时灰显；默认当前路径与已填词干按普通值渲染。
func (m Model) outputModePlaceholder() bool {
	switch m.options.OutputMode {
	case OutputModeClosed:
		return true
	case OutputModeCustom:
		return strings.TrimSpace(m.options.OutputPath) == ""
	default:
		return false
	}
}

// optionValue 给出该行显示的值。可调行带「< >」符号提示点击方向，
// 下载/上传大小显示 MB 单位；文本行留空显示「默认 (…)」，输出模式行
// 按三态显示（见 outputModeValue）。
func (m Model) optionValue(row optionRow, focused bool) string {
	if row.kind == kindBool {
		return m.options.value(row.option)
	}
	if row.kind == kindMode {
		return "< " + modeLabel(m.options.Mode) + " >"
	}
	if row.kind == kindOutput {
		return m.outputModeValue(focused)
	}
	value := m.options.value(row.option)
	if strings.TrimSpace(value) == "" {
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
// 先截断后上色是为了不把 ANSI 序列拦腰砍断。pad 是行尾补白的样式：
// 零值原样补空格（绝大多数调用）；整行高亮的行传选中样式，让补白
// 也带上高亮、与 selectedLine 一致。
func joinParts(width int, pad lipgloss.Style, parts ...part) string {
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
		line += pad.Render(strings.Repeat(" ", width-total))
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
