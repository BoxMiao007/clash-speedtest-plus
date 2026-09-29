package picker

import (
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// sanitizeInput 剥掉终端粘贴可能带进来的控制字符（如 \x00）。
// 它们在地址里不可见、TrimSpace 也洗不掉，最后会让 url.Parse 报错。
// 空格不属于控制字符，天然保留。
func sanitizeInput(text string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, text)
}

// Options 是选源界面上可填写的测速选项。没填的项沿用命令行默认值。
type Options struct {
	Filter         string
	Block          string
	Mode           string
	DownloadSize   string
	UploadSize     string
	Concurrent     string
	Parallel       string
	Timeout        string
	EarlyStop      string
	MaxLatency     string
	MaxPacketLoss  string
	MinDownload    string
	MinUpload      string
	ImageSpeedOnly bool
	NoImage        bool
	OutputPath     string
	// OutputMode 是输出路径行的三态（见 CONTEXT.md「输出模式」）。
	// OutputPath 只在自定义态被消费；其他态保留已填值，切回自定义还能看到。
	OutputMode     OutputMode
	Rename         bool
	RenameTemplate string
	GistToken      string
	GistAddress    string
	RepoToken      string
	RepoAddress    string
	RepoFilePath   string
	RepoBranch     string
	ServerURL      string
	UserAgent      string
}

// Session 是选源界面打开时已经准备好的配置列表。
type Session struct {
	Configs []ConfigEntry
	// ExecDir 是程序文件所在目录。选源结果里的临时文件写到这里。
	ExecDir string
	// Fetch 把订阅地址变成源（临时文件 + 原地址）。测试里注入假的，不访问网络。
	Fetch func(urls []string) (sources []SourceSpec, used []string, err error)
}

// Model 是选源界面。checked 只记录合格配置是否打勾。
type Model struct {
	configs []ConfigEntry
	cursor  int
	checked []bool
	status  string
	started bool
	options Options

	fetching    bool
	quitting    bool
	focus       int
	address     string
	optionIndex int

	// 各区的滚动位置。
	configScroll int
	optionScroll int

	// 终端尺寸，未知（0）时不滚动、不限宽。
	width  int
	height int

	// fetch 结果：订阅内容写出的临时文件（带原地址），代替地址参与本次测速。
	fetchedSources []SourceSpec
	usedFlagged    []string
	session        Session
}

const (
	focusConfigs = iota
	focusAddress
	focusOptions
)

// 帮助行热区。
const (
	helpHitNone = iota
	helpHitEnter
	helpHitQuit
)

// fetchDoneMsg 是订阅地址拉取结束。err 非空时留在选源界面。
type fetchDoneMsg struct {
	err     error
	sources []SourceSpec
	used    []string
}

// New 用一份会话创建选源界面。
// 有合格配置时光标停在第一条；一个都没有时直接停在地址框。
func New(session Session) Model {
	checked := make([]bool, len(session.Configs))
	cursor := 0
	focus := focusAddress
	for i, config := range session.Configs {
		if config.Selectable {
			cursor = i
			focus = focusConfigs
			break
		}
	}
	return Model{
		configs: session.Configs,
		cursor:  cursor,
		checked: checked,
		focus:   focus,
		session: session,
		options: defaultOptions(),
	}
}

func (m *Model) ensureFetch() {
	if m.session.Fetch == nil {
		// 每次回车取用当前「拉取订阅 UA」，改完重试即生效。
		m.session.Fetch = defaultFetch(m.session.ExecDir, m.fetchUA())
	}
}

// fetchUA 给出拉订阅用的 User-Agent：界面里填了就用填的，空则用默认。
func (m Model) fetchUA() string {
	if ua := strings.TrimSpace(m.options.UserAgent); ua != "" {
		return ua
	}
	return defaultSubscriptionUA
}

// Started 表示是否已确认过测速源（回车成功过）。
func (m Model) Started() bool { return m.started }

// Options 返回选好的测速选项，由调用方传给测速。
func (m Model) Options() Options { return m.options }

// UsedFlagged 返回实际用了补过 flag=meta 的地址。
func (m Model) UsedFlagged() []string { return m.usedFlagged }

func (m Model) optionState() OptionState {
	return OptionState{Mode: m.options.Mode, OutputMode: m.options.OutputMode, OutputPath: m.options.OutputPath}
}

func isQuit(msg tea.KeyMsg) bool {
	// 选源界面只认 Ctrl+C：q 和 Esc 都不再是退出，免得误触。
	return msg.Type == tea.KeyCtrlC
}

// Update 处理勾选、地址输入、选项编辑和回车开始。
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if isQuit(msg) {
			m.quitting = true
			return m, tea.Quit
		}
		if m.fetching {
			// 获取中其余按键忽略。
			return m, nil
		}
		wasFetching := m.fetching
		m.handleKey(msg)
		m.ensureVisible()
		if m.fetching && !wasFetching {
			return m, m.fetchCmd()
		}
		if m.started {
			// 回车确认即结束，把整理好的参数交给调用方。
			return m, tea.Quit
		}
	case fetchDoneMsg:
		m.fetching = false
		if msg.err != nil {
			m.status = msg.err.Error()
			break
		}
		if len(msg.sources) > 0 {
			m.fetchedSources = msg.sources
			m.usedFlagged = msg.used
			if len(msg.used) > 0 {
				m.status = "已用补过参数的地址 " + strings.Join(msg.used, "，")
			}
		}
		m.started = true
		return m, tea.Quit
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.ensureVisible()
	case tea.MouseMsg:
		if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
			m.wheelScroll(msg.Button == tea.MouseButtonWheelDown, msg.Y)
			break
		}
		if msg.Action != tea.MouseActionPress {
			break
		}
		if m.fetching {
			// 获取中界面锁定：滚轮可以滚窗口，点击一概忽略。
			break
		}
		x := m.contentX(msg.X)
		switch m.helpHit(x, msg.Y) {
		case helpHitEnter:
			m.pressEnter()
			if m.started {
				return m, tea.Quit
			}
			if m.fetching {
				return m, m.fetchCmd()
			}
		case helpHitQuit:
			m.quitting = true
			return m, tea.Quit
		}
		// 右键不参与点击；选项栏的加减按点击位置分，不分鼠标键。
		if msg.Button == tea.MouseButtonLeft {
			m.handleClick(x, msg.Y)
		}
	}
	return m, nil
}

// helpHit 报告内容坐标 (x, y) 落在帮助行的哪个热区上。
func (m Model) helpHit(x, y int) int {
	lo := m.computeLayout()
	if y != lo.helpY {
		return helpHitNone
	}
	enter, quit := m.helpZones()
	if x >= enter[0] && x < enter[1] {
		return helpHitEnter
	}
	if x >= quit[0] && x < quit[1] {
		return helpHitQuit
	}
	return helpHitNone
}

// contentX 把屏幕 X 换算成内容坐标：终端比内容宽时整块居中，左边有偏移。
func (m Model) contentX(x int) int {
	if m.width > maxContentWidth {
		return x - (m.width-maxContentWidth)/2
	}
	return x
}

func (m *Model) handleKey(msg tea.KeyMsg) {
	switch msg.Type {
	case tea.KeyDown:
		m.move(1)
	case tea.KeyUp:
		m.move(-1)
	case tea.KeyTab:
		m.move(-1)
	case tea.KeyLeft, tea.KeyRight:
		if m.focus == focusConfigs {
			// 文件栏里左右都切到选项栏，从测速模式行开始。
			m.focus = focusOptions
			m.optionIndex = 0
			return
		}
		delta := -1
		if msg.Type == tea.KeyRight {
			delta = 1
		}
		m.adjustOption(delta)
	case tea.KeyRunes:
		// bubbletea 在所有平台上把空格报成 KeyRunes（见 key_windows.go），
		// 必须在这里归一，否则勾选和开关在真实终端里按不动。
		if string(msg.Runes) == " " {
			m.pressSpace()
			return
		}
		// 粘贴可能带进看不见的控制字符（如 \x00），拦在输入这一层。
		text := sanitizeInput(string(msg.Runes))
		if text == "" {
			return
		}
		switch m.focus {
		case focusAddress:
			m.address += text
		case focusOptions:
			m.typeOption(text)
		}
	case tea.KeyBackspace:
		switch {
		case m.focus == focusAddress && m.address != "":
			m.address = m.address[:len(m.address)-1]
		case m.focus == focusOptions:
			m.backspaceOption()
		}
	case tea.KeySpace:
		m.pressSpace()
	case tea.KeyEnter:
		m.pressEnter()
	}
}

func (m *Model) pressSpace() {
	switch m.focus {
	case focusConfigs:
		m.toggle(m.cursor)
	case focusAddress:
		m.address += " "
	case focusOptions:
		m.pressOption()
	}
}

func (m *Model) pressEnter() {
	if !m.hasSource() {
		m.status = "先勾选配置或填入订阅地址"
		return
	}
	if _, err := m.validateEnabledRows(); err != nil {
		m.status = err.Error()
		return
	}
	m.finalizeOutputPath()
	if strings.TrimSpace(m.address) != "" {
		m.fetching = true
		m.status = "正在获取"
		return
	}
	m.started = true
}

// finalizeOutputPath 在回车开始测速、校验通过后按输出模式落定（见
// CONTEXT.md「输出模式」词条）：自定义态沿用「后缀补全」，词干不带后缀
// 时补 .yaml（result → result.yaml），自带 .yml 原样，纯空白视同关闭；
// 关闭态与默认当前路径态不消费自定义词干——自动命名在测速端按源算，
// 已填值原样保留，Esc 返回后切回自定义态还能看到。此后（含 Esc 返回
// 选源界面）行内显示落定后的值。
func (m *Model) finalizeOutputPath() {
	if m.options.OutputMode != OutputModeCustom {
		return
	}
	// 与命令行 -o 同一份校验口径，落定的都是去空白后的值。
	value, err := ValidateOutputPath(m.options.OutputPath)
	if err != nil {
		// 校验通过才会走到这里，不该再见到目录意图；原样保留等用户改。
		return
	}
	if value == "" {
		// 自定义态留空回车视同关闭：落定空值，行内显示切回「关闭」。
		m.options.OutputMode = OutputModeClosed
		m.options.OutputPath = ""
		return
	}
	m.options.OutputPath = CompleteYAMLSuffix(value)
}

// adjustOption 用左右键调当前选项的值：模式循环、输出模式三态循环、
// 开关切换、数字按步长增减。文本类选项靠打字，左右键对它们无操作。
func (m *Model) adjustOption(delta int) {
	if m.focus != focusOptions || m.optionIndex < 0 || m.optionIndex >= len(optionOrder) {
		return
	}
	row := optionOrder[m.optionIndex]
	if !m.optionState().Enabled(row.option) {
		return
	}
	switch row.kind {
	case kindMode:
		m.options.changeMode(delta)
	case kindOutput:
		if m.options.OutputMode == OutputModeCustom {
			// 自定义态是编辑态：左右键留给路径输入，不循环三态——与点击
			// 的豁免一致（arrowSymbolX 在自定义态不算热区）。
			return
		}
		m.options.changeOutputMode(delta)
	case kindBool:
		m.options.toggle(row.option)
	case kindText:
		m.options.adjust(row.option, delta)
	}
}

// fetchCmd 由调用方在 pressEnter 后取用，发起后台拉取。
func (m Model) fetchCmd() tea.Cmd {
	m2 := m
	m2.ensureFetch()
	session := m2.session
	urls := SplitSubscriptionText(m.address)
	return func() tea.Msg {
		sources, used, err := session.Fetch(urls)
		if err != nil {
			return fetchDoneMsg{err: err}
		}
		return fetchDoneMsg{sources: sources, used: used}
	}
}

func (m *Model) handleClick(x, y int) {
	lo := m.computeLayout()
	switch {
	case y >= lo.paneY && y < lo.paneY+lo.paneH:
		if x < lo.filesX+lo.filesW {
			m.clickConfig(lo, y)
			return
		}
		m.clickOption(lo, y, x)
	case y == lo.addressY:
		m.focus = focusAddress
	}
	m.ensureVisible()
}

func (m *Model) clickConfig(lo layout, y int) {
	if len(m.configs) == 0 {
		return
	}
	offsets, _ := m.entryOffsets(lo)
	abs := y - lo.paneY - 1 + offsets[m.configScroll] // 第 0 行是节标题
	if abs < 0 {
		return
	}
	end := m.visibleEntriesEnd(lo)
	// 条目折行后占多行，点中任何一行都算命中这个条目；
	// 装不下留白的行不算命中。
	index := -1
	for i := m.configScroll; i < end; i++ {
		if offsets[i] <= abs && abs < offsets[i]+m.entryHeight(lo, i) {
			index = i
		}
	}
	if index < 0 {
		return
	}
	m.focus = focusConfigs
	m.cursor = index
	m.toggle(index)
}

// clickOption 点击选项行：聚焦该行。只有精准点中「<」「>」符号才增减，
// 开关只在值文字处切换，热区左右各一格；其余位置只选中，灰行无操作。
func (m *Model) clickOption(lo layout, y, x int) {
	index := y - lo.paneY - 1 + m.optionScroll // 第 0 行是节标题
	if index < 0 || index >= len(optionOrder) {
		return
	}
	row := optionOrder[index]
	if !m.optionState().Enabled(row.option) {
		return
	}
	m.focus = focusOptions
	m.optionIndex = index
	if row.kind == kindBool {
		start := lo.optionsX + 2 + optionLabelWidth() + 2
		end := start + lipgloss.Width(m.optionValue(row, true))
		// 值被窄终端截掉时不能留下不可见的热区，容差也不能越过栏边界。
		if lo.optionsW > 0 && (end > lo.optionsX+lo.optionsW || x >= lo.optionsX+lo.optionsW) {
			return
		}
		if x >= start-1 && x <= end {
			m.adjustOption(1)
		}
		return
	}
	left, right, ok := m.arrowSymbolX(lo, index)
	if !ok {
		return // 没有符号的行（纯文本）只选中
	}
	// 符号各带一格容差，其余位置不响应。
	if x >= left-1 && x <= left+1 {
		m.adjustOption(-1)
		return
	}
	if x >= right-1 && x <= right+1 {
		m.adjustOption(1)
	}
}

// arrowSymbolX 算出该选项行「<」「>」符号的内容列位置。
// 行布局与 optionLine 渲染共用：2 格缩进 + 标签列 + 2 格间隔，随后是值。
func (m Model) arrowSymbolX(lo layout, index int) (left, right int, ok bool) {
	row := optionOrder[index]
	if row.kind == kindOutput && m.options.OutputMode == OutputModeCustom {
		// 自定义态是编辑态：就算词干里碰巧带 < 也不是循环符号。
		return 0, 0, false
	}
	focused := m.focus == focusOptions && m.optionIndex == index
	value := m.optionValue(row, focused)
	if !strings.Contains(value, "<") {
		return 0, 0, false
	}
	x0 := lo.optionsX + 2 + optionLabelWidth() + 2
	right = x0 + lipgloss.Width(value) - 1
	// 值被窄终端截断时尾部符号不在画面上，不给点。
	if lo.optionsW > 0 && right >= lo.optionsX+lo.optionsW {
		return 0, 0, false
	}
	return x0, right, true
}

func (m *Model) typeOption(text string) {
	if !m.editableRow() {
		return
	}
	option := optionOrder[m.optionIndex].option
	if filter := acceptsInput(option); filter != nil {
		text = strings.Map(func(r rune) rune {
			if filter(r) {
				return r
			}
			return -1
		}, text)
		if text == "" {
			return
		}
	}
	if option == OptionOutputPath && m.options.OutputMode != OutputModeCustom {
		// 关闭/默认当前路径态打字：自动跳自定义并进入编辑，从空词干开始，
		// 不把没显示出来的残留值接进新输入（见 CONTEXT.md「输出模式」词条）。
		m.options.OutputMode = OutputModeCustom
		m.options.OutputPath = ""
	}
	m.options.setText(option, m.options.value(option)+text)
}

func (m *Model) backspaceOption() {
	if !m.editableRow() {
		return
	}
	option := optionOrder[m.optionIndex].option
	if option == OptionOutputPath && m.options.OutputMode != OutputModeCustom {
		// 关闭/默认当前路径态行内没有可删的内容，退格不动（打字才跳自定义）。
		return
	}
	value := m.options.value(option)
	if value == "" {
		return
	}
	m.options.setText(option, value[:len(value)-1])
}

func (m *Model) pressOption() {
	if m.focus != focusOptions || m.optionIndex < 0 || m.optionIndex >= len(optionOrder) {
		return
	}
	row := optionOrder[m.optionIndex]
	if !m.optionState().Enabled(row.option) {
		return
	}
	switch row.kind {
	case kindBool:
		m.options.toggle(row.option)
	case kindMode:
		m.options.changeMode(1)
	case kindOutput:
		m.options.changeOutputMode(1)
	}
}

// editableRow 报告当前行是否接受打字输入。
func (m *Model) editableRow() bool {
	if m.focus != focusOptions || m.optionIndex < 0 || m.optionIndex >= len(optionOrder) {
		return false
	}
	row := optionOrder[m.optionIndex]
	if !m.optionState().Enabled(row.option) {
		return false
	}
	return row.kind == kindText || row.kind == kindBool || row.kind == kindOutput
}

// move 沿环形次序「文件 → 地址 → 选项 → 文件」移动焦点或栏内光标。
// ↑↓ 走的是同一个环的正反两个方向，↑ 从文件栏头部绕到选项栏尾部。
// Tab 等同 ↑。
func (m *Model) move(delta int) {
	switch m.focus {
	case focusConfigs:
		if len(m.configs) == 0 {
			m.focus = focusAddress
			return
		}
		next := m.cursor + delta
		if next < 0 {
			// 文件栏头部再往上，绕环落到选项栏末项。
			m.focus = focusOptions
			m.optionIndex = len(optionOrder) - 1
			return
		}
		if next >= len(m.configs) {
			m.focus = focusAddress
			return
		}
		m.cursor = next
	case focusAddress:
		if delta > 0 {
			m.focus = focusOptions
			m.optionIndex = 0
			return
		}
		if len(m.configs) > 0 {
			m.focus = focusConfigs
			m.cursor = len(m.configs) - 1
			return
		}
		m.focus = focusOptions
		m.optionIndex = len(optionOrder) - 1
	case focusOptions:
		next := m.optionIndex + delta
		if next < 0 {
			m.focus = focusAddress
			return
		}
		if next >= len(optionOrder) {
			if len(m.configs) > 0 {
				m.focus = focusConfigs
				m.cursor = 0
				return
			}
			m.focus = focusAddress
			return
		}
		m.optionIndex = next
	}
}

// SourceList 返回参与测速的源，每个源各自一轮：
// 先是勾选的配置文件，再是拉取写出的订阅临时文件（FromSubscription 标记）。
func (m Model) SourceList() []SourceSpec {
	list := make([]SourceSpec, 0, len(m.configs)+len(m.fetchedSources))
	for i, config := range m.configs {
		if config.Selectable && m.checked[i] {
			list = append(list, SourceSpec{Value: config.Path, DisplayName: config.Name})
		}
	}
	list = append(list, m.fetchedSources...)
	return list
}

func (m Model) hasSource() bool {
	if strings.TrimSpace(m.address) != "" {
		return true
	}
	for i, config := range m.configs {
		if config.Selectable && m.checked[i] {
			return true
		}
	}
	return false
}

func (m *Model) toggle(index int) {
	if index < 0 || index >= len(m.configs) || !m.configs[index].Selectable {
		return
	}
	m.checked[index] = !m.checked[index]
}

// Init 满足 bubbletea 的界面接口。选源界面打开时没有后台任务。
func (m Model) Init() tea.Cmd { return nil }

// ensureVisible 让焦点行留在各自栏的窗口里。文件栏条目可能折行占多行：
// 滚动以条目为单位，光标条目要么完整可见要么整个滚出（比窗口还高的条目
// 让它的顶部贴窗顶）。
func (m *Model) ensureVisible() {
	lo := m.computeLayout()
	switch m.focus {
	case focusConfigs:
		if len(m.configs) == 0 || lo.paneH <= 1 {
			m.configScroll = 0
			return
		}
		offsets, _ := m.entryOffsets(lo)
		viewport := lo.paneH - 1
		s := min(max(m.configScroll, 0), m.maxConfigScroll(lo))
		if m.cursor < s {
			s = m.cursor
		}
		for s < m.cursor && offsets[m.cursor]+m.entryHeight(lo, m.cursor) > offsets[s]+viewport {
			s++
		}
		m.configScroll = s
	case focusOptions:
		m.optionScroll = clampScroll(m.optionScroll, m.optionIndex, len(optionOrder), lo.paneH-1)
	}
}

// visibleEntriesEnd 是窗口实际装下的最后一个条目下标（开区间），装法与
// filePaneRows 一致：按条目整块放，放不下的留白，首条目超高是例外。
func (m Model) visibleEntriesEnd(lo layout) int {
	rows := 1 // 节标题
	end := m.configScroll
	for i := m.configScroll; i < len(m.configs) && rows < lo.paneH; i++ {
		h := m.entryHeight(lo, i)
		if rows+h > lo.paneH && rows > 1 {
			break
		}
		rows += h
		end = i + 1
	}
	return end
}

// maxConfigScroll 是能作为窗口首条目的最大下标：从它起到末尾的内容仍能把
// 窗口放满，避免末尾滚出大段空白。
func (m Model) maxConfigScroll(lo layout) int {
	offsets, total := m.entryOffsets(lo)
	viewport := lo.paneH - 1
	if len(m.configs) == 0 || viewport <= 0 || total <= viewport {
		return 0
	}
	maxS := 0
	for i := range m.configs {
		if total-offsets[i] >= viewport {
			maxS = i
		}
	}
	return maxS
}

func clampScroll(scroll, focus, count, visible int) int {
	if visible <= 0 || count <= visible {
		return 0
	}
	if focus < scroll {
		return focus
	}
	if focus >= scroll+visible {
		return focus - visible + 1
	}
	return min(max(scroll, 0), max(count-visible, 0))
}

// wheelScroll 滚焦点所在的栏：文件或选项，窗口动、光标不动。
// 文件栏按条目滚动（条目折行时占多行），选项栏按行滚动。
func (m *Model) wheelScroll(down bool, y int) {
	lo := m.computeLayout()
	step := 3
	if y >= lo.paneY && y < lo.paneY+lo.paneH {
		if m.focus == focusConfigs {
			maxS := m.maxConfigScroll(lo)
			if down {
				m.configScroll = min(m.configScroll+step, maxS)
			} else {
				m.configScroll = max(m.configScroll-step, 0)
			}
			return
		}
		m.optionScroll = stepScroll(m.optionScroll, down, step, len(optionOrder), lo.paneH-1)
	}
}

func stepScroll(scroll int, down bool, step, count, visible int) int {
	if visible <= 0 || count <= visible {
		return 0
	}
	if down {
		return min(scroll+step, count-visible)
	}
	return max(scroll-step, 0)
}
