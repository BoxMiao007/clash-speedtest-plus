package tui

import (
	"sync/atomic"

	tea "github.com/charmbracelet/bubbletea"
)

// 队列壳：整个多轮队列由一个 tea.Program 承载（ADR-0015）。每轮一个视图、
// 全部保留在内存里；本轮测完、产物保存完，壳准备好下一轮的视图并切换过去，
// 不再「每轮一个程序」。Ctrl+↑/↓ 在各轮视图间切换（「轮间切换」）：在测轮
// 切走后测试继续，结果等消息按属主轮分流，已完成轮显示最终表。
//
// 消息分流：轮视图返回的命令产生的消息由壳统一包上属主下标（tagRoundCmd），
// Update 收到后交回属主轮处理，不再一律交给当前激活轮——否则在测轮切走后，
// 它的结果会误落进正在显示的轮。按键、窗口尺寸等界面事件仍归当前激活轮。

// nextRoundMsg 是准备完一轮的结果：view 就绪、skipped 表示该源没有节点跳过、
// err 表示致命错误（界面退出后再报错终止，终端能正常还原）。
type nextRoundMsg struct {
	view    QueueRound
	skipped bool
	err     error
}

// roundOwnedMsg 包着一条属于某个轮的消息。壳把轮视图返回的命令包上属主
// 下标，命令产出的消息据此送回属主轮；内层消息原样保留。
type roundOwnedMsg struct {
	round int
	msg   tea.Msg
}

// tagRoundCmd 给轮视图返回的命令包上属主标记：命令产出的消息变成
// roundOwnedMsg。退出信号必须原样放行（包上会拦住程序退出），嵌套批次
// 逐个子命令打标后交回运行时展开。
func tagRoundCmd(cmd tea.Cmd, index int) tea.Cmd {
	if cmd == nil {
		return nil
	}
	return func() tea.Msg {
		msg := cmd()
		switch msg := msg.(type) {
		case nil:
			return nil
		case tea.QuitMsg:
			return msg
		case tea.BatchMsg:
			tagged := make(tea.BatchMsg, 0, len(msg))
			for _, sub := range msg {
				if sub == nil {
					continue
				}
				tagged = append(tagged, tagRoundCmd(sub, index))
			}
			return tagged
		default:
			return roundOwnedMsg{round: index, msg: msg}
		}
	}
}

// QueueRound 是队列壳眼里的一轮视图：交互模型加三个意图查询。
type QueueRound interface {
	tea.Model
	Model
	// AdvanceRequested 报告本轮已完成且产物已保存、应推进下一轮。
	AdvanceRequested() bool
	// ExitAll 报告用户要求退出整个队列：未完成轮或已完成中途轮的 q/Ctrl+C
	//（剩余轮没测完）；最后一轮测完后的正常离开不算。
	ExitAll() bool
	// Quitting 报告本轮视图正在退出。裸 QuitMsg 不经过 Update，
	// 壳的收尾（状态捕获）只能靠轮询这个意图。
	Quitting() bool
}

// enableRoundSwitchHint 给就位的轮视图亮出轮间切换键位：只有壳的真实视图
// （tuiModel）承接得到，其他实现原样返回。
func enableRoundSwitchHint(view QueueRound) QueueRound {
	if m, ok := view.(tuiModel); ok {
		m.SetRoundSwitchHint(true)
		return m
	}
	return view
}

// QueueResult 是队列程序结束后的结局，main 据此决定回选源、退出码与 stderr 收尾。
type QueueResult struct {
	// Escaped 为真表示按了 Esc：中断本轮＋作废剩余＋回选源界面。
	Escaped bool
	// ExitAll 为真表示用户在队列未走完时要求退出全部（q/Ctrl+C），不再回选源。
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
	// captured 记录哪些轮的状态行已经抓过：一轮只抓一次，避免收尾路径
	// 与推进路径重复入列。
	captured  map[int]bool
	width     int
	height    int
	preparing bool
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
	// 多轮队列从第一个轮视图就位起就亮出轮间切换提示：壳启动时已知总轮
	// 数（总源数），不必等第二个视图；单轮队列键位无效，维持不标注。
	if total > 1 {
		first = enableRoundSwitchHint(first)
	}
	return QueueModel{
		rounds:     []QueueRound{first},
		active:     0,
		nextSource: firstIndex + 1,
		total:      total,
		prepare:    prepare,
		voided:     voided,
		captured:   make(map[int]bool),
	}
}

// Init 启动第一轮视图的等待命令。后续轮的 Init 由壳在推进时手动调用
// （bubbletea 只对初始模型调一次 Init）。切回已就绪的轮不重调 Init：
// 等待命令链常驻程序，重调会成倍叠加。
func (q QueueModel) Init() tea.Cmd {
	return tagRoundCmd(q.rounds[0].Init(), 0)
}

// Update 分流消息：属主消息交回属主轮；按键里的 Ctrl+↑/↓ 切换视图；
// 其余交给当前激活轮，并在它消化消息后检查推进/离开意图。
func (q QueueModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case roundOwnedMsg:
		return q.routeOwned(msg)
	case nextRoundMsg:
		return q.handleNextRound(msg)
	case tea.QuitMsg:
		// 兜底：正常路径下裸 QuitMsg 不经过 Update，这里很少走到。
		return q.finish()
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+up":
			return q.switchRound(-1)
		case "ctrl+down":
			return q.switchRound(1)
		}
		return q.delegate(msg)
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

// delegate 把消息交给当前激活轮视图；它返回的命令包上激活轮下标，
// 并在它消化消息后统一收口意图。
func (q QueueModel) delegate(msg tea.Msg) (tea.Model, tea.Cmd) {
	updated, cmd := q.rounds[q.active].Update(msg)
	q.rounds[q.active] = updated.(QueueRound)
	return q.afterRoundUpdate(q.active, tagRoundCmd(cmd, q.active))
}

// routeOwned 把属主消息交回它的轮视图（哪怕不是当前显示的轮）：在测轮
// 切走后，结果、进度、保存完成等消息仍持续流入属主轮，测试不中断。
func (q QueueModel) routeOwned(msg roundOwnedMsg) (tea.Model, tea.Cmd) {
	if msg.round < 0 || msg.round >= len(q.rounds) {
		return q, nil
	}
	updated, cmd := q.rounds[msg.round].Update(msg.msg)
	q.rounds[msg.round] = updated.(QueueRound)
	return q.afterRoundUpdate(msg.round, tagRoundCmd(cmd, msg.round))
}

// afterRoundUpdate 在某个轮视图消化消息后检查它的意图：Esc 回选源、
// 退出全部、本轮完成推进下一轮、保存失败记入退出码。idx 是刚处理完消息
// 的轮；推进、退出等意图跟着属主轮走，与当前显示的是哪一轮无关。
func (q QueueModel) afterRoundUpdate(idx int, cmd tea.Cmd) (tea.Model, tea.Cmd) {
	r := q.rounds[idx]
	if r.EscapedToParent() {
		// Esc：中断本轮＋作废剩余（整个队列）＋回上一级（选源界面）。
		q.voided.Store(true)
		q.escaped = true
		q.captureAllStatuses()
		return q, tea.Quit
	}
	if r.ExitAll() {
		// 退出全部（未完成轮或已完成中途轮的 q/Ctrl+C）：立即作废在途的轮
		// 准备；实际退出看 Quitting，等待落盘的退出要在保存完成后才走。
		q.exitAll = true
		q.voided.Store(true)
	}
	if _, failed := r.ExitStatus(); failed {
		q.failed = true
	}
	if r.Quitting() {
		// 轮视图要退出程序：最后一轮收尾完成、强制退出、或中止本轮。
		// 用户显式退出优先于推进：即便本轮还留着推进标记也不开下一轮。
		q.captureAllStatuses()
		if cmd == nil {
			cmd = tea.Quit
		}
		return q, cmd
	}
	if r.AdvanceRequested() {
		// 推进标记先消费掉：重访旧轮残留的标记不能再次触发推进。
		q.consumeAdvance(idx)
		if !q.preparing && idx == len(q.rounds)-1 {
			// 本轮完成、产物已保存：自动连续推进，准备下一个源。
			// 只有最新（刚完成的）轮能推进；在旧轮上的重复请求忽略。
			q.captureStatus(idx)
			if next := q.scheduleNext(); next != nil {
				return q, tea.Batch(cmd, next)
			}
			// 推进时已没有更多源：队列走完（正常到不了这里，防御一下）。
			return q, tea.Quit
		}
	}
	return q, cmd
}

// consumeAdvance 清掉一轮的推进标记。标记由壳消费一次；不然轮上残留的
// 标记会让壳每条消息都再开一轮。
func (q *QueueModel) consumeAdvance(idx int) {
	m, ok := q.rounds[idx].(tuiModel)
	if !ok || !m.advanceRequested {
		return
	}
	m.advanceRequested = false
	q.rounds[idx] = m
}

// switchRound 切换显示的轮视图：Ctrl+↑/↓ 逐轮移动，边界轮无操作，
// 单轮队列没有切换目标。切过去重放当前窗口尺寸，布局才不会塌。
func (q QueueModel) switchRound(delta int) (tea.Model, tea.Cmd) {
	if len(q.rounds) <= 1 {
		return q, nil
	}
	target := q.active + delta
	if target < 0 || target >= len(q.rounds) {
		return q, nil
	}
	q.active = target
	if q.width > 0 || q.height > 0 {
		updated, _ := q.rounds[q.active].Update(tea.WindowSizeMsg{Width: q.width, Height: q.height})
		q.rounds[q.active] = updated.(QueueRound)
	}
	return q, nil
}

// handleNextRound 接住准备结果：入列，跳过则继续准备，出错或队列已作废
// 则结束程序。用户正看着刚完成的最新轮就切到新一轮；切走在别的轮上时
// 不抢视图，新一轮在后台继续测。
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
	followed := q.active == len(q.rounds)-1
	q.rounds = append(q.rounds, msg.view)
	newIdx := len(q.rounds) - 1
	if followed {
		q.active = newIdx
	}
	var cmds []tea.Cmd
	// 新视图错过了程序的 WindowSizeMsg：重放当前尺寸，布局才不会塌。
	if q.width > 0 || q.height > 0 {
		updated, sizeCmd := msg.view.Update(tea.WindowSizeMsg{Width: q.width, Height: q.height})
		q.rounds[newIdx] = updated.(QueueRound)
		if sizeCmd != nil {
			cmds = append(cmds, tagRoundCmd(sizeCmd, newIdx))
		}
	}
	// 新就位的轮视图也要亮出轮间切换键位：多轮队列每个视图就位即启用
	// （首轮在壳启动时已启用），单轮队列不标注。
	if q.total > 1 {
		q.rounds[newIdx] = enableRoundSwitchHint(q.rounds[newIdx])
	}
	// 后续轮的等待命令（结果、进度、提前结束）在这里启动。
	if initCmd := msg.view.Init(); initCmd != nil {
		cmds = append(cmds, tagRoundCmd(initCmd, newIdx))
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

// finish 队列走到尽头（最后一个源也处理完）或收到兜底退出：抓全部轮的
// 状态后结束程序。
func (q QueueModel) finish() (tea.Model, tea.Cmd) {
	q.captureAllStatuses()
	return q, tea.Quit
}

// captureStatus 抓一轮离开时的状态行文本（同一轮只抓一次），程序结束后
// 统一打到 stderr：状态行 3 秒就清，离开 TUI 后再补一行已保存路径方便复制。
func (q *QueueModel) captureStatus(idx int) {
	if idx < 0 || idx >= len(q.rounds) || q.captured[idx] {
		return
	}
	q.captured[idx] = true
	if status, _ := q.rounds[idx].ExitStatus(); status != "" {
		q.statuses = append(q.statuses, status)
	}
}

// captureAllStatuses 程序离开前抓齐所有轮的状态：切走后完成或收尾的轮
// 没有别的收口时机，落下任何一轮都会丢 stderr 补打的已保存路径。
func (q *QueueModel) captureAllStatuses() {
	for i := range q.rounds {
		q.captureStatus(i)
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
