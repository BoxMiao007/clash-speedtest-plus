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
	// Fetch 把订阅地址变成 Clash/Mihomo yaml。测试里注入假的，不访问网络。
	Fetch func(urls []string) (files []string, used []string, err error)
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

	// fetch 结果：订阅内容写出的临时文件，代替地址参与本次测速。
	fetchedFiles []string
	usedFlagged  []string
	session      Session
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
	err   error
	files []string
	used  []string
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
		// 界面默认值照顾双击直用的场景：并行 6 加速整轮测速，
		// 结果图只留有速度的行，减少空行。命令行参数默认值不受影响。
		options: Options{
			Filter: ".+", Mode: "download", DownloadSize: "50", UploadSize: "20",
			Concurrent: "4", Parallel: "6", Timeout: "5s", MaxLatency: "1s",
			MaxPacketLoss: "100", MinDownload: "5", MinUpload: "2", Rename: true,
			ImageSpeedOnly: true,
		},
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

// SourceLabel 返回画在结果图顶部的来源：勾选的配置文件名加用户填的订阅地址，
// 不含拉取写出的临时文件名。
func (m Model) SourceLabel() string {
	var parts []string
	for i, config := range m.configs {
		if config.Selectable && m.checked[i] {
			parts = append(parts, config.Name)
		}
	}
	parts = append(parts, SplitSubscriptionText(m.address)...)
	return strings.Join(parts, ",")
}

func (m Model) optionState() OptionState {
	return OptionState{Mode: m.options.Mode, OutputPath: m.options.OutputPath}
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
		if len(msg.files) > 0 {
			m.fetchedFiles = msg.files
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
	enter, quit := helpZones()
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
	if strings.TrimSpace(m.address) != "" {
		m.fetching = true
		m.status = "正在获取"
		return
	}
	m.started = true
}

// adjustOption 用左右键调当前选项的值：模式循环、开关切换、数字按步长增减。
// 文本类选项靠打字，左右键对它们无操作。
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
		m.options.Mode = cycleMode(m.options.Mode, delta)
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
		files, used, err := session.Fetch(urls)
		if err != nil {
			return fetchDoneMsg{err: err}
		}
		return fetchDoneMsg{files: files, used: used}
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
	index := y - lo.paneY - 1 + m.configScroll // 第 0 行是节标题
	if index < 0 || index >= len(m.configs) {
		return
	}
	m.focus = focusConfigs
	m.cursor = index
	m.toggle(index)
}

// clickOption 点击选项行：聚焦该行。只有精准点中「<」「>」符号才增减，
// 点值本身、标签或空白都只选中；开关行点哪儿都切换；灰行无操作。
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
		m.adjustOption(1)
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
	m.options.setText(option, m.options.value(option)+text)
}

func (m *Model) backspaceOption() {
	if !m.editableRow() {
		return
	}
	option := optionOrder[m.optionIndex].option
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
		m.options.Mode = cycleMode(m.options.Mode, 1)
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
	return row.kind == kindText || row.kind == kindBool
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

// Selection 返回这次要测的源：先是勾选的配置路径，再是拉取成功后写出的临时文件。
func (m Model) Selection() []string {
	var sources []string
	for i, config := range m.configs {
		if config.Selectable && m.checked[i] {
			sources = append(sources, config.Path)
		}
	}
	sources = append(sources, m.fetchedFiles...)
	return sources
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

// ensureVisible 让焦点行留在各自栏的窗口里。
func (m *Model) ensureVisible() {
	lo := m.computeLayout()
	switch m.focus {
	case focusConfigs:
		m.configScroll = clampScroll(m.configScroll, m.cursor, len(m.configs), lo.paneH-1)
	case focusOptions:
		m.optionScroll = clampScroll(m.optionScroll, m.optionIndex, len(optionOrder), lo.paneH-1)
	}
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
func (m *Model) wheelScroll(down bool, y int) {
	lo := m.computeLayout()
	step := 3
	if y >= lo.paneY && y < lo.paneY+lo.paneH {
		if m.focus == focusConfigs {
			m.configScroll = stepScroll(m.configScroll, down, step, len(m.configs), lo.paneH-1)
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
