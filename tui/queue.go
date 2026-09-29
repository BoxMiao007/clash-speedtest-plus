package tui

import (
	"sync/atomic"

	tea "github.com/charmbracelet/bubbletea"
)

// 队列壳：整个多轮队列由一个 tea.Program 承载（ADR-0015）。每轮一个视图、
// 全部保留在内存里；本轮测完、产物保存完，壳准备好下一轮的视图并切换过去，
// 不再「每轮一个程序」。轮间切换（Ctrl+↑/↓，后续票）的接缝已就位：切换视图
// 就是改 active 并重放窗口尺寸，非激活轮的模型照常保留数据。
//
// 消息都交给当前激活轮处理。本票里非激活轮只可能是已完成的轮，不再有属于
// 它的测试消息；轮间切换引入后台在测轮后，消息按属主分流在壳里扩展。

// nextRoundMsg 是准备完一轮的结果：view 就绪、skipped 表示该源没有节点跳过、
// err 表示致命错误（界面退出后再报错终止，终端能正常还原）。
type nextRoundMsg struct {
	view    QueueRound
	skipped bool
	err     error
}

// QueueRound 是队列壳眼里的一轮视图：交互模型加三个意图查询。
type QueueRound interface {
	tea.Model
	Model
	// AdvanceRequested 报告本轮已完成且产物已保存、应推进下一轮。
	AdvanceRequested() bool
	// ExitAll 报告用户要求退出整个队列：未完成轮的 q/Ctrl+C，或已完成轮的 Ctrl+C。
	ExitAll() bool
	// Quitting 报告本轮视图正在退出。裸 QuitMsg 不经过 Update，
	// 壳的收尾（状态捕获）只能靠轮询这个意图。
	Quitting() bool
}

// QueueResult 是队列程序结束后的结局，main 据此决定回选源、退出码与 stderr 收尾。
type QueueResult struct {
	// Escaped 为真表示按了 Esc：中断本轮＋作废剩余＋回选源界面。
	Escaped bool
	// ExitAll 为真表示离开方式是退出全部（q/Ctrl+C），不再回选源。
	ExitAll bool
	// Failed 为真表示任一轮保存失败或被强制退出，退出码非 0。
	Failed bool
	// SetupErr 是某轮准备时的致命错误（测速器组装、节点加载失败）。
	SetupErr error
	// Statuses 收集各轮离开时的状态行文本，程序结束后统一打到 stderr。
	Statuses []string
}

// QueueModel 承载整个队列的壳。
type QueueModel struct {
	rounds     []QueueRound // 已就绪的轮视图，按激活顺序追加，保留不丢弃
	active     int          // rounds 里当前显示的轮
	nextSource int          // 下一个待准备的源下标
	total      int          // 源总数
	prepare    func(index int) (QueueRound, bool, error)
	voided     *atomic.Bool // Esc/退出全部后置位，让在途的轮准备尽快放弃
	escaped    bool
	exitAll    bool
	failed     bool
	setupErr   error
	statuses   []string
	// lastCaptured 是最近抓过收尾状态的轮下标：同一轮只抓一次，
	// 避免 Quitting 轮询与 QuitMsg 兜底重复入列。
	lastCaptured int
	width        int
	height       int
	preparing    bool
}

// NewQueueModel 组建队列壳。first 是已就绪的第一轮视图（源下标 firstIndex，
// 之前的源都因 0 节点被跳过）；prepare 在轮推进时准备下一个源，返回
// skipped=true 表示该源解析不出节点、跳过继续；total 为源总数；voided 由
// 调用方持有，Esc/退出全部时置位，让准备中的轮放弃刚启动的测试。
func NewQueueModel(
	first QueueRound,
	firstIndex int,
	prepare func(index int) (QueueRound, bool, error),
	total int,
	voided *atomic.Bool,
) QueueModel {
	return QueueModel{
		rounds:       []QueueRound{first},
		active:       0,
		nextSource:   firstIndex + 1,
		total:        total,
		prepare:      prepare,
		voided:       voided,
		lastCaptured: -1,
	}
}

// Init 启动第一轮视图的等待命令。后续轮的 Init 由壳在推进时手动调用
// （bubbletea 只对初始模型调一次 Init）。
func (q QueueModel) Init() tea.Cmd {
	return q.rounds[0].Init()
}

// Update 分发消息：除准备结果与兜底退出外都交给当前激活轮，并在它消化
// 消息后检查推进/离开意图。
func (q QueueModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case nextRoundMsg:
		return q.handleNextRound(msg)
	case tea.QuitMsg:
		// 兜底：正常路径下裸 QuitMsg 不经过 Update，这里很少走到。
		return q.finish()
	case tea.WindowSizeMsg:
		q.width = msg.Width
		q.height = msg.Height
		return q.delegate(msg)
	default:
		return q.delegate(msg)
	}
}

// View 渲染当前激活轮；退出中由轮视图自己返回空串清屏。
func (q QueueModel) View() string {
	if q.active >= len(q.rounds) {
		return ""
	}
	return q.rounds[q.active].View()
}

// delegate 把消息交给当前激活轮视图，并统一收口它的意图。
func (q QueueModel) delegate(msg tea.Msg) (tea.Model, tea.Cmd) {
	updated, cmd := q.rounds[q.active].Update(msg)
	q.rounds[q.active] = updated.(QueueRound)
	return q.afterRoundUpdate(cmd)
}

// afterRoundUpdate 在轮视图消化消息后检查意图：Esc 回选源、退出全部、
// 本轮完成推进下一轮、保存失败记入退出码。
func (q QueueModel) afterRoundUpdate(cmd tea.Cmd) (tea.Model, tea.Cmd) {
	r := q.rounds[q.active]
	if r.EscapedToParent() {
		// Esc：中断本轮＋作废剩余＋回上一级（选源界面）。
		q.voided.Store(true)
		q.escaped = true
		q.captureStatus(r)
		return q, tea.Quit
	}
	if r.ExitAll() {
		// Ctrl+C（或未完成轮的 q）：退出全部。立即作废在途的轮准备；
		// 实际退出看 Quitting，等待落盘的退出要在保存完成后才走。
		q.exitAll = true
		q.voided.Store(true)
	}
	if _, failed := r.ExitStatus(); failed {
		q.failed = true
	}
	if r.Quitting() {
		// 轮视图要退出程序：最后一轮收尾完成、强制退出、或中止本轮。
		// 用户显式退出优先于推进：即便本轮还留着推进标记也不开下一轮。
		q.captureStatus(r)
		if cmd == nil {
			cmd = tea.Quit
		}
		return q, cmd
	}
	if !q.preparing && r.AdvanceRequested() {
		// 本轮完成、产物已保存：自动连续推进，准备下一个源。
		q.captureStatus(r)
		if next := q.scheduleNext(); next != nil {
			return q, tea.Batch(cmd, next)
		}
		// 推进时已没有更多源：队列走完（正常到不了这里，防御一下）。
		return q, tea.Quit
	}
	return q, cmd
}

// handleNextRound 接住准备结果：入列并切换到新一轮，跳过则继续准备，
// 出错或队列已作废则结束程序。
func (q QueueModel) handleNextRound(msg nextRoundMsg) (tea.Model, tea.Cmd) {
	q.preparing = false
	if q.voided.Load() {
		return q, tea.Quit
	}
	if msg.err != nil {
		q.setupErr = msg.err
		return q, tea.Quit
	}
	if msg.skipped {
		if next := q.scheduleNext(); next != nil {
			return q, next
		}
		return q.finish()
	}
	q.rounds = append(q.rounds, msg.view)
	q.active = len(q.rounds) - 1
	var cmds []tea.Cmd
	// 新视图错过了程序的 WindowSizeMsg：重放当前尺寸，布局才不会塌。
	if q.width > 0 || q.height > 0 {
		updated, sizeCmd := msg.view.Update(tea.WindowSizeMsg{Width: q.width, Height: q.height})
		q.rounds[q.active] = updated.(QueueRound)
		if sizeCmd != nil {
			cmds = append(cmds, sizeCmd)
		}
	}
	// 后续轮的等待命令（结果、进度、提前结束）在这里启动。
	if initCmd := msg.view.Init(); initCmd != nil {
		cmds = append(cmds, initCmd)
	}
	return q, tea.Batch(cmds...)
}

// scheduleNext 安排准备下一个源；没有更多源时返回 nil（队列走完，调用方收尾）。
func (q *QueueModel) scheduleNext() tea.Cmd {
	if q.nextSource >= q.total {
		return nil
	}
	q.preparing = true
	index := q.nextSource
	q.nextSource++
	prepare := q.prepare
	return func() tea.Msg {
		view, skipped, err := prepare(index)
		return nextRoundMsg{view: view, skipped: skipped, err: err}
	}
}

// finish 队列走到尽头（最后一个源也处理完）或收到兜底退出：抓当前轮状态
// 后结束程序。
func (q QueueModel) finish() (tea.Model, tea.Cmd) {
	if q.active < len(q.rounds) {
		q.captureStatus(q.rounds[q.active])
	}
	return q, tea.Quit
}

// captureStatus 抓一轮离开时的状态行文本（同一轮只抓一次），程序结束后统一
// 打到 stderr：状态行 3 秒就清，离开 TUI 后再补一行已保存路径方便复制。
func (q *QueueModel) captureStatus(r QueueRound) {
	if q.lastCaptured == q.active {
		return
	}
	q.lastCaptured = q.active
	if status, _ := r.ExitStatus(); status != "" {
		q.statuses = append(q.statuses, status)
	}
}

// Result 供 main 在程序结束后读取队列结局。
func (q QueueModel) Result() QueueResult {
	return QueueResult{
		Escaped:  q.escaped,
		ExitAll:  q.exitAll,
		Failed:   q.failed,
		SetupErr: q.setupErr,
		Statuses: q.statuses,
	}
}
