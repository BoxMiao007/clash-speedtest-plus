package picker

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/faceair/clash-speedtest/speedtester"
)

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

// StartRequest 是选源界面确认开始时交给调用方的全部内容。
type StartRequest struct {
	Selection   []string
	Options     Options
	UsedFlagged []string
	SourceLabel string
}

// Session 是选源界面打开时已经准备好的配置列表。
type Session struct {
	Configs []ConfigEntry
	// ExecDir 是程序文件所在目录。选源结果里的临时文件写到这里。
	ExecDir string
	// Fetch 把订阅地址变成 Clash/Mihomo yaml。测试里注入假的，不访问网络。
	Fetch func(urls []string) (files []string, used []string, err error)
	// OnStart 在回车确认后调用。返回的 Cmd 继续驱动界面：开始测速或报加载错误。
	OnStart func(StartRequest) tea.Cmd
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

	// 测试执行状态。loading 在加载节点，testing 在测，testDone 停在记录表上等退出。
	loading  bool
	testing  bool
	testDone bool
	total    int
	records  []*speedtester.Result

	// 各区的滚动位置。
	configScroll int
	optionScroll int
	recordScroll int

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

// fetchDoneMsg 是订阅地址拉取结束。err 非空时留在选源界面。
type fetchDoneMsg struct {
	err   error
	files []string
	used  []string
}

// LoadFailedMsg 是回车后加载节点失败，选源界面回到可编辑状态。
type LoadFailedMsg struct{ Err error }

// TestingStartedMsg 是节点加载完成，记录表开始接收结果。
type TestingStartedMsg struct{ Total int }

// TestResultMsg 是一个节点测完。
type TestResultMsg struct{ Result *speedtester.Result }

// TestDoneMsg 是整轮测速结束（含提前结束）。
type TestDoneMsg struct{}

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
		options: Options{
			Filter: ".+", Mode: "download", DownloadSize: "50", UploadSize: "20",
			Concurrent: "4", Parallel: "1", Timeout: "5s", MaxLatency: "1s",
			MaxPacketLoss: "100", MinDownload: "5", MinUpload: "2", Rename: true,
		},
	}
}

func (m *Model) ensureFetch() {
	if m.session.Fetch == nil {
		m.session.Fetch = defaultFetch(m.session.ExecDir)
	}
}

// Started 表示是否已确认过测速源（回车成功过）。
func (m Model) Started() bool { return m.started }

// TestDone 表示整轮测速是否已经完成。
func (m Model) TestDone() bool { return m.testDone }

// Total 返回本轮要测的节点总数，画结果图摘要用。
func (m Model) Total() int { return m.total }

// Results 返回记录表里已完成的结果，由调用方导出产物。
func (m Model) Results() []*speedtester.Result { return m.records }

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

func isQuit(msg tea.KeyMsg, m Model) bool {
	if msg.Type == tea.KeyEsc || msg.Type == tea.KeyCtrlC {
		return true
	}
	// q 在输入文字的地方就是字符，其余地方当退出。
	return string(msg.Runes) == "q" && !m.editingText()
}

func (m Model) editingText() bool {
	if m.focus == focusAddress {
		return true
	}
	if m.focus != focusOptions || m.optionIndex < 0 || m.optionIndex >= len(optionOrder) {
		return false
	}
	row := optionOrder[m.optionIndex]
	return row.kind == kindText && m.optionState().Enabled(row.option)
}

// Update 处理勾选、地址输入、选项编辑、开始测速和测试中的记录表。
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if isQuit(msg, m) {
			m.quitting = true
			return m, tea.Quit
		}
		if m.fetching || m.loading {
			// 获取/加载中其余按键忽略。
			return m, nil
		}
		if m.testing || m.testDone {
			// 记录表阶段只能滚动查看。
			if msg.Type == tea.KeyUp || msg.Type == tea.KeyDown {
				m.scrollRecords(msg.Type == tea.KeyDown)
			}
			return m, nil
		}
		wasFetching := m.fetching
		cmd := m.handleKey(msg)
		m.ensureVisible()
		if m.fetching && !wasFetching {
			return m, m.fetchCmd()
		}
		if cmd != nil {
			return m, cmd
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
		return m, m.beginStart()
	case LoadFailedMsg:
		m.loading = false
		m.started = false
		m.status = msg.Err.Error()
	case TestingStartedMsg:
		m.loading = false
		m.testing = true
		m.total = msg.Total
	case TestResultMsg:
		m.records = append(m.records, msg.Result)
		m.recordScroll = max(len(m.records)-m.recordRows(), 0)
	case TestDoneMsg:
		m.testing = false
		m.testDone = true
		m.status = "测试完成"
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.ensureVisible()
	case tea.MouseMsg:
		if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
			m.wheelScroll(msg.Button == tea.MouseButtonWheelDown, msg.Y)
			break
		}
		if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress {
			m.handleClick(msg.X, msg.Y)
		}
	}
	return m, nil
}

// beginStart 确认开始：先交出选好的源，再交给 OnStart 起测速。
func (m *Model) beginStart() tea.Cmd {
	m.started = true
	if m.session.OnStart == nil {
		return nil
	}
	m.loading = true
	m.status = "正在加载节点"
	return m.session.OnStart(StartRequest{
		Selection:   m.Selection(),
		Options:     m.options,
		UsedFlagged: m.usedFlagged,
		SourceLabel: m.SourceLabel(),
	})
}

func (m *Model) handleKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.Type {
	case tea.KeyDown:
		m.move(1)
	case tea.KeyUp:
		m.move(-1)
	case tea.KeyLeft, tea.KeyTab:
		m.move(-1)
	case tea.KeyRight:
		m.move(1)
	case tea.KeyRunes:
		// bubbletea 在所有平台上把空格报成 KeyRunes（见 key_windows.go），
		// 必须在这里归一，否则勾选和开关在真实终端里按不动。
		if string(msg.Runes) == " " {
			m.pressSpace()
			return nil
		}
		switch m.focus {
		case focusAddress:
			m.address += string(msg.Runes)
		case focusOptions:
			m.typeOption(string(msg.Runes))
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
		return m.pressEnter()
	}
	return nil
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

func (m *Model) pressEnter() tea.Cmd {
	if !m.hasSource() {
		m.status = "先勾选配置或填入订阅地址"
		return nil
	}
	if _, err := m.validateEnabledRows(); err != nil {
		m.status = err.Error()
		return nil
	}
	if strings.TrimSpace(m.address) != "" {
		m.fetching = true
		m.status = "正在获取"
		return nil
	}
	return m.beginStart()
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
		m.clickOption(lo, y)
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

func (m *Model) clickOption(lo layout, y int) {
	index := y - lo.paneY - 1 + m.optionScroll // 第 0 行是节标题
	if index < 0 || index >= len(optionOrder) {
		return
	}
	if !m.optionState().Enabled(optionOrder[index].option) {
		return
	}
	m.focus = focusOptions
	m.optionIndex = index
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
		m.cycleMode()
	}
}

// cycleMode 让测速模式在快速、下载、完整之间循环。
func (m *Model) cycleMode() {
	modes := []string{"fast", "download", "full"}
	next := 0
	for i, mode := range modes {
		if m.options.Mode == mode {
			next = (i + 1) % len(modes)
		}
	}
	m.options.Mode = modes[next]
}

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

// move 沿「文件 → 地址 → 选项」的环形次序移动焦点或栏内光标。
func (m *Model) move(delta int) {
	switch m.focus {
	case focusConfigs:
		if len(m.configs) == 0 {
			m.focus = focusAddress
			return
		}
		next := m.cursor + delta
		if next < 0 || next >= len(m.configs) {
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
		if next < 0 || next >= len(optionOrder) {
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

// wheelScroll 按点击位置滚对应的区：上半区滚文件或选项栏，下半区滚记录表。
func (m *Model) wheelScroll(down bool, y int) {
	lo := m.computeLayout()
	step := 3
	switch {
	case y >= lo.paneY && y < lo.paneY+lo.paneH:
		if lo.width > 0 {
			// X 在鼠标事件里拿得到，这里按当前焦点栏滚最直观：滚焦点所在栏。
			if m.focus == focusConfigs {
				m.configScroll = stepScroll(m.configScroll, down, step, len(m.configs), lo.paneH-1)
				return
			}
			m.optionScroll = stepScroll(m.optionScroll, down, step, len(optionOrder), lo.paneH-1)
			return
		}
		m.optionScroll = stepScroll(m.optionScroll, down, step, len(optionOrder), lo.paneH-1)
	case y >= lo.recordsY:
		m.recordScroll = stepScroll(m.recordScroll, down, step, len(m.records), m.recordRows())
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

func (m Model) recordRows() int {
	lo := m.computeLayout()
	return max(lo.recordsH-2, 0)
}

func (m *Model) scrollRecords(down bool) {
	m.recordScroll = stepScroll(m.recordScroll, down, 1, len(m.records), m.recordRows())
}
