package tui

import (
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

// PauseController 由测速引擎实现，TUI 通过它在暂停/继续时控制节点派发。
type PauseController interface {
	Pause()
	Resume()
}

// inFlightNode 保存一个在测节点的最新进度快照，随 200ms 进度事件刷新。
type inFlightNode struct {
	name     string
	proxyType string
	latest   speedtester.Progress
}

// tuiModel represents the Bubble Tea model for the TUI
type tuiModel struct {
	mode           speedtester.SpeedMode
	totalProxies   int
	currentProxy   int
	results        []*speedtester.Result
	sequence       map[*speedtester.Result]int
	nextSequence   int
	baseHeaders    []string
	testing        bool
	quitting       bool
	progress       progress.Model
	table          table.Model
	help           helpState
	resultChannel  chan *speedtester.Result
	sortColumn     int
	sortAscending  bool
	detailVisible  bool
	detailResult   *speedtester.Result
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
	// pauseStartedAt 记录本次暂停起点；pausedElapsed 累计历史暂停时长，用于冻结已用时。
	pauseStartedAt time.Time
	pausedElapsed  time.Duration
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
	}
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
			m.quitting = true
			return m, tea.Quit
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
					m.toggleInFlightDetail(m.inFlightOrder[m.tableRowCount()+rowIndex])
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
		progressCmd := m.progress.SetPercent(float64(m.currentProxy) / float64(m.totalProxies))
		cmds := []tea.Cmd{progressCmd, m.waitForResult()}
		if !m.flushScheduled {
			m.flushScheduled = true
			cmds = append(cmds, scheduleFlushCmd())
		}
		return m, tea.Batch(cmds...)

	case doneMsg:
		m.testing = false
		m.flushScheduled = false
		m.flushResultsIfDirty()
		// 全部测完后空格无效果，帮助条隐藏空格项。
		m.help.setEarlyStopped(true)
		m.help.setPaused(false)
		progressCmd := m.progress.SetPercent(1.0)
		return m, progressCmd

	case nodeProgressMsg:
		m.applyProgress(msg.progress)
		return m, m.waitForProgress()

	case earlyStopMsg:
		m.earlyStopped = true
		m.help.setEarlyStopped(true)
		m.help.setPaused(false)
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

	case timerTickMsg:
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
	node, ok := m.inFlight[name]
	if !ok {
		return
	}
	delete(m.inFlight, name)
	for i, n := range m.inFlightOrder {
		if n == name {
			m.inFlightOrder = append(m.inFlightOrder[:i], m.inFlightOrder[i+1:]...)
			break
		}
	}
	if m.detailInFlight == node {
		m.detailInFlight = nil
		if m.detailVisible {
			m.detailResult = m.findResultByName(name)
			m.refreshDetailHeight()
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

// View renders the TUI
func (m tuiModel) View() string {
	if m.quitting {
		return ""
	}

	// Layout: progress bar at top, table below
	tableView := m.table.View()
	detailView := m.detailPanelView()

	sections := []string{
		m.progressLine(),
		"",
		tableView,
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
