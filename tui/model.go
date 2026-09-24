package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/faceair/clash-speedtest/output"
	"github.com/faceair/clash-speedtest/speedtester"
)

// Messages for TUI updates
type resultMsg struct {
	result *speedtester.Result
}

type progressMsg struct {
	current int
	total   int
	name    string
}

type doneMsg struct{}

type timerTickMsg struct{}

type flushResultsMsg struct{}

// nodeProgressMsg 携带测速引擎的单个进度事件快照。
type nodeProgressMsg struct {
	progress speedtester.Progress
}

// earlyStopMsg 表示过筛数量达到限额，不再派发新节点。
type earlyStopMsg struct{}

// imageSavedMsg 是结果表图写完后的提示。
type imageSavedMsg struct {
	text  string
	quit  bool
	error bool
}

// uploadStatusMsg 是后台 gist/仓库上传结束后的提示。
type uploadStatusMsg struct {
	text string
}

const statusHold = 3 * time.Second

// PauseController 由测速引擎实现，TUI 通过它在暂停/继续时控制节点派发。
type PauseController interface {
	Pause()
	Resume()
}

// inFlightNode 保存一个在测节点的最新进度快照，随 200ms 进度事件刷新。
type inFlightNode struct {
	name      string
	proxyType string
	latest    speedtester.Progress
}

// tuiModel represents the Bubble Tea model for the TUI
type tuiModel struct {
	mode          speedtester.SpeedMode
	totalProxies  int
	currentProxy  int
	results       []*speedtester.Result
	sequence      map[*speedtester.Result]int
	nextSequence  int
	baseHeaders   []string
	testing       bool
	quitting      bool
	progress      progress.Model
	table         table.Model
	help          helpState
	resultChannel chan *speedtester.Result
	sortColumn    int
	sortAscending bool
	detailVisible bool
	detailResult  *speedtester.Result
	// detailInFlight 非空表示详情面板当前展示的是在测节点占位详情。
	detailInFlight *inFlightNode
	selectedIndex  int
	windowWidth    int
	windowHeight   int
	startTime      time.Time
	resultsDirty   bool
	flushScheduled bool
	detailHeight   int
	perf           *perfTracker

	// 测试循环状态：暂停/提前结束由外部信号驱动。
	pauseCtl      PauseController
	progressCh    <-chan speedtester.Progress
	earlyStopCh   <-chan struct{}
	paused        bool
	earlyStopped  bool
	inFlight      map[string]*inFlightNode
	inFlightOrder []string
	finishedNames map[string]struct{}
	// pauseStartedAt 记录本次暂停起点；pausedElapsed 累计历史暂停时长，用于冻结已用时。
	pauseStartedAt time.Time
	pausedElapsed  time.Duration

	// 结果表图：-no-image 只关自动导出，s 仍可手动保存。
	autoImage     bool
	imageDir      string
	imageSource   string
	savingImage   bool
	statusText    string
	statusUntil   time.Time
	autoImageDone bool
	// quittingAfterSave 表示整轮已结束，第一次退出正在等本地产物。
	quittingAfterSave bool
	forceQuit         bool
	saveFailed        bool
	configSaver       func([]*speedtester.Result) (string, error)
	configSaved       bool
	uploadFunc        func() string
	uploadCancel      func()
	// scrollbarDrag 为真表示正在拖动滚动条滑块。scrollbarGrab 是按下时相对滑块顶部的偏移。
	scrollbarDrag bool
	scrollbarGrab int
}

const (
	tableHeaderPadding  = 1
	tableHeaderLines    = 2
	detailPanelMinWidth = 60
	defaultDetailWidth  = 80
	resultFlushInterval = 100 * time.Millisecond
)

var selectedRowStyle = lipgloss.NewStyle().
	Background(lipgloss.Color("62")).
	Foreground(lipgloss.Color("230")).
	Bold(true)

// NewTUIModel creates a new TUI model
func NewTUIModel(mode speedtester.SpeedMode, totalProxies int, resultChannel chan *speedtester.Result) tuiModel {
	return newTUIModel(mode, totalProxies, resultChannel, nil, nil, nil)
}

// NewTUIModelWithEngine 接入测速引擎的暂停控制、进度事件与提前结束信号，
// 供交互模式使用；非交互模式仍用 NewTUIModel。
func NewTUIModelWithEngine(
	mode speedtester.SpeedMode,
	totalProxies int,
	resultChannel chan *speedtester.Result,
	pauseCtl PauseController,
	progressCh <-chan speedtester.Progress,
	earlyStopCh <-chan struct{},
) tuiModel {
	return newTUIModel(mode, totalProxies, resultChannel, pauseCtl, progressCh, earlyStopCh)
}

// newTUIModel 支持注入暂停控制、进度事件与提前结束信号，便于测试与 main 接线。
func newTUIModel(
	mode speedtester.SpeedMode,
	totalProxies int,
	resultChannel chan *speedtester.Result,
	pauseCtl PauseController,
	progressCh <-chan speedtester.Progress,
	earlyStopCh <-chan struct{},
) tuiModel {
	// Initialize progress bar
	p := progress.New(
		progress.WithDefaultGradient(),
		progress.WithWidth(40),
	)

	// Initialize table with headers
	headers := output.GetHeaders(mode)
	sortColumn, sortAscending := defaultSortState(mode)
	columns := buildColumns(addSortIndicators(headers, sortColumn, sortAscending), 0, mode)

	t := table.New(
		table.WithColumns(columns),
		table.WithRows([]table.Row{}),
		table.WithFocused(true),
		table.WithHeight(10),
	)

	// Style the table
	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(false)
	s.Selected = selectedRowStyle
	t.SetStyles(s)

	return tuiModel{
		mode:           mode,
		totalProxies:   totalProxies,
		currentProxy:   0,
		results:        make([]*speedtester.Result, 0),
		sequence:       make(map[*speedtester.Result]int),
		nextSequence:   0,
		baseHeaders:    headers,
		testing:        true,
		quitting:       false,
		progress:       p,
		table:          t,
		help:           newHelpState(t.KeyMap),
		resultChannel:  resultChannel,
		sortColumn:     sortColumn,
		sortAscending:  sortAscending,
		detailVisible:  false,
		detailResult:   nil,
		selectedIndex:  -1,
		windowWidth:    0,
		windowHeight:   0,
		startTime:      time.Now(),
		resultsDirty:   false,
		flushScheduled: false,
		detailHeight:   0,
		perf:           newPerfTracker(),

		pauseCtl:      pauseCtl,
		progressCh:    progressCh,
		earlyStopCh:   earlyStopCh,
		inFlight:      make(map[string]*inFlightNode),
		finishedNames: make(map[string]struct{}),
		autoImage:     true,
		imageDir:      ".",
	}
}

// SetImageExport 配置结果表图目录。auto 为 false 时只关自动导出。
func (m *tuiModel) SetImageExport(dir string, auto bool) {
	if dir != "" {
		m.imageDir = dir
	}
	m.autoImage = auto
}

// SetImageSource 记录配置文件名或订阅地址，画在结果图顶部。
func (m *tuiModel) SetImageSource(source string) {
	m.imageSource = source
}

// SetConfigSaver 在整轮结束时写本地 yaml。gist/仓库上传由调用方自行后台处理。
func (m *tuiModel) SetConfigSaver(save func([]*speedtester.Result) (string, error)) {
	m.configSaver = save
}

// SetUploader 在本地 yaml 写完后后台上传。返回的文字会盖住状态行。
func (m *tuiModel) SetUploader(upload func() string, cancel func()) {
	m.uploadFunc = upload
	m.uploadCancel = cancel
}

// Init initializes the TUI model
func (m tuiModel) Init() tea.Cmd {
	cmds := []tea.Cmd{
		timerTickCmd(),
		m.waitForResult(),
		m.waitForProgress(),
		m.waitForEarlyStop(),
	}
	return tea.Batch(cmds...)
}

// waitForResult waits for results from the channel
func (m tuiModel) waitForResult() tea.Cmd {
	return func() tea.Msg {
		result, ok := <-m.resultChannel
		if !ok {
			return doneMsg{}
		}
		return resultMsg{result: result}
	}
}

func scheduleFlushCmd() tea.Cmd {
	return tea.Tick(resultFlushInterval, func(time.Time) tea.Msg {
		return flushResultsMsg{}
	})
}

func (m *tuiModel) flushResultsIfDirty() {
	if !m.resultsDirty {
		return
	}
	m.sortResults()
	m.updateTableRows()
	m.resultsDirty = false
}

// Update handles messages and updates the model
func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.quittingAfterSave && msg.String() != "q" && msg.String() != "ctrl+c" {
			return m, nil
		}
		switch msg.String() {
		case "esc":
			if m.detailVisible {
				m.detailVisible = false
				m.help.setDetailVisible(false)
				m.detailHeight = 0
				m.updateTableLayout()
				return m, nil
			}
		case "q", "ctrl+c":
			if m.quittingAfterSave {
				m.forceQuit = true
				m.quitting = true
				if m.uploadCancel != nil {
					m.uploadCancel()
				}
				if err := output.RemovePartialImages(m.imageDir); err != nil {
					m.saveFailed = true
					m.statusText = "删除半截图失败: " + err.Error()
				}
				return m, tea.Quit
			}
			if m.roundFinished() {
				// 整轮结束通常已自动保存过产物，直接退出，避免退出时重复出图。
				if m.autoImageDone && !m.savingImage && !m.saveFailed {
					m.quitting = true
					if m.uploadCancel != nil {
						m.uploadCancel()
					}
					return m, tea.Quit
				}
				m.quittingAfterSave = true
				m.help.setSaving(true)
				m.setStatus("正在保存")
				if m.savingImage {
					return m, nil
				}
				m.savingImage = true
				return m, m.saveImageCmd(true, true)
			}
			// 本轮尚未结束：丢掉在测节点，不写 yaml 和自动图，并删掉半截图。
			if stopper, ok := m.pauseCtl.(interface{ Stop() }); ok {
				stopper.Stop()
			}
			if m.uploadCancel != nil {
				m.uploadCancel()
			}
			if err := output.RemovePartialImages(m.imageDir); err != nil {
				m.saveFailed = true
				m.statusText = "删除半截图失败: " + err.Error()
			}
			m.quitting = true
			return m, tea.Quit
		case "s":
			if m.savingImage {
				m.setStatus("正在保存")
				return m, nil
			}
			m.savingImage = true
			m.setStatus("正在保存")
			return m, m.saveImageCmd(false, false)
		case " ":
			// 空格切换暂停/继续；详情开着时仍是暂停，不关详情。
			// 已完成或已提前结束时空格无效。
			if !m.testing || m.earlyStopped || m.pauseCtl == nil {
				return m, nil
			}
			if m.paused {
				m.pausedElapsed += time.Since(m.pauseStartedAt)
				m.paused = false
				m.pauseCtl.Resume()
			} else {
				m.pauseStartedAt = time.Now()
				m.paused = true
				m.pauseCtl.Pause()
			}
			m.help.setPaused(m.paused)
			m.help.setEarlyStopped(m.earlyStopped)
			return m, nil
		}

		m.table, cmd = m.table.Update(msg)
		m.syncSelectionFromCursor()
		return m, cmd

	case tea.MouseMsg:
		if m.quittingAfterSave {
			return m, nil
		}
		if msg.Button == tea.MouseButtonWheelUp {
			m.table.MoveUp(1)
			m.syncSelectionFromCursor()
			return m, nil
		}
		if msg.Button == tea.MouseButtonWheelDown {
			m.table.MoveDown(1)
			m.syncSelectionFromCursor()
			return m, nil
		}
		if m.scrollbarDrag && msg.Action == tea.MouseActionMotion && msg.Button == tea.MouseButtonLeft {
			if mark, ok := m.scrollbarMarkAt(msg.X, msg.Y); ok {
				m.jumpScrollbar(mark)
			}
			return m, nil
		}
		if msg.Action == tea.MouseActionRelease {
			m.scrollbarDrag = false
		}
		if mark, onBar := m.scrollbarMarkAt(msg.X, msg.Y); onBar && msg.Button == tea.MouseButtonLeft {
			top, thumb, ok := m.scrollbarRange()
			onThumb := ok && mark >= top && mark < top+thumb
			if msg.Action == tea.MouseActionPress && onThumb {
				m.scrollbarDrag = true
				m.scrollbarGrab = mark - top
				return m, nil
			}
			if msg.Action == tea.MouseActionPress || msg.Action == tea.MouseActionRelease {
				m.jumpScrollbar(mark)
			}
			return m, nil
		}
		if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionRelease {
			if m.isHeaderClick(msg.Y) {
				if columnIndex := m.columnAtX(msg.X); columnIndex >= 0 {
					if columnIndex == m.sortColumn {
						m.sortAscending = !m.sortAscending
					} else {
						m.sortColumn = columnIndex
						m.sortAscending = defaultSortAscending(columnIndex)
					}
					m.sortResults()
					m.updateTableHeaders()
					m.updateTableRows()
					m.resultsDirty = false
				}
				return m, nil
			}
			if rowIndex, ok := m.rowAtY(msg.Y); ok {
				if rowIndex < 0 {
					// 负偏移索引在测行：打开测试中占位详情，再点同一行则关掉。
					m.toggleInFlightDetail(m.inFlightOrder[-rowIndex-1])
				} else {
					m.toggleDetail(m.results[rowIndex])
					m.setSelection(rowIndex)
				}
				return m, nil
			}
		}

	case tea.WindowSizeMsg:
		m.windowWidth = msg.Width
		m.windowHeight = msg.Height
		m.refreshDetailHeight()
		m.updateTableLayout()
		return m, nil

	case resultMsg:
		m.currentProxy++
		m.results = append(m.results, msg.result)
		m.recordSequence(msg.result)
		m.finishInFlight(msg.result.ProxyName)
		m.resultsDirty = true
		m.progress.SetPercent(float64(m.currentProxy) / float64(m.totalProxies))
		cmds := []tea.Cmd{m.waitForResult()}
		if !m.flushScheduled {
			m.flushScheduled = true
			cmds = append(cmds, scheduleFlushCmd())
		}
		if cmd := m.startFinalSave(false); cmd != nil {
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)

	case doneMsg:
		m.testing = false
		m.flushScheduled = false
		m.flushResultsIfDirty()
		// 全部测完后空格无效果，帮助条隐藏空格项。
		m.help.setEarlyStopped(true)
		m.help.setPaused(false)
		m.progress.SetPercent(1.0)
		var cmds []tea.Cmd
		if cmd := m.startFinalSave(false); cmd != nil {
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)

	case nodeProgressMsg:
		m.applyProgress(msg.progress)
		return m, m.waitForProgress()

	case earlyStopMsg:
		m.earlyStopped = true
		m.help.setEarlyStopped(true)
		m.help.setPaused(false)
		m.statusText = ""
		m.statusUntil = time.Time{}
		return m, nil

	case progressMsg:
		m.currentProxy = msg.current
		m.totalProxies = msg.total
		m.progress.SetPercent(float64(m.currentProxy) / float64(m.totalProxies))

		// Update progress bar
		progressModel, progressCmd := m.progress.Update(msg)
		m.progress = progressModel.(progress.Model)
		cmd = progressCmd

		return m, cmd

	case imageSavedMsg:
		m.savingImage = false
		if strings.Contains(msg.text, "已保存配置") {
			m.configSaved = true
		}
		if msg.error {
			m.saveFailed = true
		}
		m.setStatus(msg.text)
		if msg.quit || m.quittingAfterSave {
			m.quitting = true
			return m, tea.Quit
		}
		if m.configSaved && m.uploadFunc != nil {
			upload := m.uploadFunc
			m.uploadFunc = nil
			return m, func() tea.Msg {
				return uploadStatusMsg{text: upload()}
			}
		}
		return m, nil

	case uploadStatusMsg:
		if msg.text != "" {
			m.setStatus(msg.text)
		}
		return m, nil

	case timerTickMsg:
		if !m.statusUntil.IsZero() && time.Now().After(m.statusUntil) {
			m.statusText = ""
			m.statusUntil = time.Time{}
		}
		return m, timerTickCmd()

	case flushResultsMsg:
		m.flushScheduled = false
		m.flushResultsIfDirty()
		return m, nil
	}

	// Default: update progress bar for other messages
	progressModel, progressCmd := m.progress.Update(msg)
	m.progress = progressModel.(progress.Model)
	cmd = progressCmd

	return m, cmd
}

// waitForProgress 等待测速引擎的进度事件（节点开始、阶段变化、瞬时速度）。
// progressCh 为 nil 时返回 nil，避免空转。
func (m tuiModel) waitForProgress() tea.Cmd {
	if m.progressCh == nil {
		return nil
	}
	return func() tea.Msg {
		p, ok := <-m.progressCh
		if !ok {
			return nil
		}
		return nodeProgressMsg{progress: p}
	}
}

// waitForEarlyStop 等待提前结束信号（channel 关闭）。
func (m tuiModel) waitForEarlyStop() tea.Cmd {
	if m.earlyStopCh == nil {
		return nil
	}
	return func() tea.Msg {
		<-m.earlyStopCh
		return earlyStopMsg{}
	}
}

// applyProgress 更新在测节点快照；收到同一节点的后续事件时覆盖快照并重绘，
// 保证延迟与瞬时速度随事件刷新。
func (m *tuiModel) applyProgress(p speedtester.Progress) {
	if _, done := m.finishedNames[p.Name]; done {
		return
	}
	if node, ok := m.inFlight[p.Name]; ok {
		node.latest = p
		m.updateTableRows()
		return
	}
	node := &inFlightNode{name: p.Name, proxyType: p.Type, latest: p}
	m.inFlight[p.Name] = node
	m.inFlightOrder = append(m.inFlightOrder, p.Name)
	m.updateTableRows()
}

// finishInFlight 在节点结果上表后移除在测状态；若占位详情开着则切换为完成详情。
func (m *tuiModel) finishInFlight(name string) {
	if m.finishedNames == nil {
		m.finishedNames = make(map[string]struct{})
	}
	m.finishedNames[name] = struct{}{}
	node, ok := m.inFlight[name]
	if !ok {
		return
	}
	selectedName := ""
	if cursor := m.table.Cursor(); cursor >= len(m.results) && cursor-len(m.results) < len(m.inFlightOrder) {
		selectedName = m.inFlightOrder[cursor-len(m.results)]
	}
	delete(m.inFlight, name)
	for i, n := range m.inFlightOrder {
		if n == name {
			m.inFlightOrder = append(m.inFlightOrder[:i], m.inFlightOrder[i+1:]...)
			break
		}
	}
	if m.detailInFlight == node || selectedName == name {
		m.detailInFlight = nil
		m.detailResult = m.findResultByName(name)
		m.selectResult(m.detailResult)
		if m.detailVisible {
			m.refreshDetailHeight()
		}
	}
}

func (m *tuiModel) selectResult(result *speedtester.Result) {
	if result == nil {
		return
	}
	for i, item := range m.results {
		if item == result {
			m.selectedIndex = i
			return
		}
	}
}

func (m *tuiModel) findResultByName(name string) *speedtester.Result {
	for _, result := range m.results {
		if result.ProxyName == name {
			return result
		}
	}
	return nil
}

func (m *tuiModel) setStatus(text string) {
	m.statusText = text
	m.statusUntil = time.Now().Add(statusHold)
}

// startFinalSave 等拓尾收完后再写 yaml 和自动结果图，避免图里重复在测行。
func (m *tuiModel) startFinalSave(quit bool) tea.Cmd {
	if m.testing && !m.earlyStopped {
		return nil
	}
	if m.inFlightCount() > 0 {
		return nil
	}
	if m.autoImageDone || m.savingImage {
		return nil
	}
	if !m.autoImage && m.configSaver == nil {
		return nil
	}
	m.autoImageDone = true
	m.savingImage = true
	m.setStatus("正在保存")
	return m.saveImageCmd(true, quit)
}

// saveImageCmd 用按下或触发那一瞬的快照编码结果表图。
func (m tuiModel) saveImageCmd(finished bool, quit bool) tea.Cmd {
	spec := m.imageSpec(finished)
	dir := m.imageDir
	saver := m.configSaver
	results := append([]*speedtester.Result(nil), m.results...)
	writeConfig := finished && saver != nil && !m.configSaved
	writeImage := m.autoImage || !finished
	if quit {
		writeImage = m.autoImage || m.savingImage
	}
	return func() tea.Msg {
		var parts []string
		failed := false
		if writeConfig {
			path, err := saver(results)
			if err != nil {
				failed = true
				parts = append(parts, "保存配置失败: "+err.Error())
			} else if path != "" {
				parts = append(parts, "已保存配置 "+path)
			}
		}
		if writeImage {
			path, warning, err := output.WriteResultImage(dir, spec)
			if err != nil {
				failed = true
				parts = append(parts, "保存结果图失败: "+err.Error())
			} else {
				parts = append(parts, output.JoinStatus("已保存 "+path, warning))
			}
		}
		text := "已保存"
		if len(parts) > 0 {
			text = strings.Join(parts, "；")
		}
		return imageSavedMsg{text: text, quit: quit, error: failed}
	}
}

func (m tuiModel) roundFinished() bool {
	return !m.testing || m.earlyStopped
}

// ExitStatus 供进程离开时决定退出码和 stderr 提示。
func (m tuiModel) ExitStatus() (string, bool) {
	return m.statusText, m.saveFailed || m.forceQuit
}

// Model 是交互模式结束后可读取的结果。
type Model interface {
	ExitStatus() (string, bool)
}

func (m tuiModel) imageSpec(finished bool) output.ImageSpec {
	status := m.stateLabel()
	if finished {
		if m.earlyStopped {
			status = "已提前结束"
		} else {
			status = "已完成"
		}
	}
	rows := make([]output.ImageRow, 0, m.inFlightCount()+len(m.results))
	// 结果图与终端一致：完成行在上，在测行在下。
	rows = append(rows, output.BuildImageRows(m.results, m.mode)...)
	for _, name := range m.inFlightOrder {
		node := m.inFlight[name]
		if node == nil {
			continue
		}
		rows = append(rows, output.ImageRow{
			Cells:    plainInFlightCells(m.mode, node),
			InFlight: true,
		})
	}
	return output.ImageSpec{
		Mode:    m.mode,
		Source:  m.imageSource,
		Summary: output.SummaryLine(time.Now(), m.mode, status, m.currentProxy, m.totalProxies),
		Headers: m.baseHeaders,
		Rows:    rows,
		Now:     time.Now(),
	}
}

func plainInFlightCells(mode speedtester.SpeedMode, node *inFlightNode) []string {
	p := node.latest
	latency := "测试中"
	if p.Latency > 0 {
		latency = p.Latency.Round(time.Millisecond).String()
	}
	if mode.IsFast() {
		return []string{"…", node.name, node.proxyType, latency}
	}
	jitter, loss, download, upload := "", "", "", ""
	if p.Latency > 0 {
		jitter = p.Jitter.Round(time.Millisecond).String()
		loss = formatLoss(p.PacketLoss)
	}
	if p.Phase >= speedtester.PhaseDownload {
		download = speedtester.FormatSpeed(p.DownloadSpeed)
	}
	if p.Phase >= speedtester.PhaseUpload {
		upload = speedtester.FormatSpeed(p.UploadSpeed)
	}
	row := []string{"…", node.name, node.proxyType, latency, jitter, loss, download}
	if mode.UploadEnabled() {
		row = append(row, upload)
	}
	return row
}

func formatLoss(value float64) string {
	return fmt.Sprintf("%.1f%%", value)
}

// grayInFlightLines 把已渲染的在测行整行变灰。选中行保持表格高亮。
// 染色放在表格截断之后，转义码不会把类型、延迟裁掉。
func grayInFlightLines(view string, finished int, cursor int) string {
	lines := strings.Split(view, "\n")
	body := 0
	for i, line := range lines {
		if strings.TrimSpace(line) == "" || strings.Contains(line, "─") {
			continue
		}
		if body == 0 {
			body++
			continue
		}
		index := body - 1
		body++
		if index < finished || index == cursor {
			continue
		}
		lines[i] = "\x1b[2m" + line + "\x1b[22m"
	}
	return strings.Join(lines, "\n")
}

// View renders the TUI
func (m tuiModel) View() string {
	if m.quitting {
		return ""
	}

	// Layout: progress bar at top, table below
	tableView := grayInFlightLines(m.table.View(), len(m.results), m.table.Cursor())
	detailView := m.detailPanelView()

	sections := []string{
		m.progressLine(),
		"",
		m.tableWithScrollbar(tableView),
	}
	if detailView != "" {
		sections = append(sections, "", detailView)
	}
	helpView := m.help.view()
	if helpView != "" {
		sections = append(sections, "", helpView)
	}
	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}
