package tui

import (
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/BoxMiao007/clash-speedtest-plus/speedtester"
	tea "github.com/charmbracelet/bubbletea"
)

// TestTagRoundCmdWrapsMessagesWithOwner 壳给轮命令打属主标记的粘合层：
// 普通消息包上属主下标，退出信号原样放行，批次逐个子命令打标。
func TestTagRoundCmdWrapsMessagesWithOwner(t *testing.T) {
	if cmd := tagRoundCmd(nil, 3); cmd != nil {
		t.Fatal("空命令不应包装")
	}
	tagged := tagRoundCmd(func() tea.Msg { return timerTickMsg{} }, 3)
	owned, ok := tagged().(roundOwnedMsg)
	if !ok || owned.round != 3 {
		t.Fatalf("命令消息应带属主标记: %#v", tagged())
	}
	if _, ok := owned.msg.(timerTickMsg); !ok {
		t.Fatalf("内层消息应原样保留: %#v", owned.msg)
	}
	quit := tagRoundCmd(tea.Quit, 1)
	if _, ok := quit().(tea.QuitMsg); !ok {
		t.Fatalf("退出信号应原样放行: %#v", quit())
	}
	batched := tagRoundCmd(func() tea.Msg {
		return tea.BatchMsg{func() tea.Msg { return timerTickMsg{} }, tea.Quit}
	}, 2)
	batch, ok := batched().(tea.BatchMsg)
	if !ok || len(batch) != 2 {
		t.Fatalf("批次应展开为带标记的子命令: %#v", batched())
	}
	if owned, ok := batch[0]().(roundOwnedMsg); !ok || owned.round != 2 {
		t.Fatalf("批次内子命令应带属主标记: %#v", batch[0]())
	}
	if _, ok := batch[1]().(tea.QuitMsg); !ok {
		t.Fatalf("批次内的退出信号应放行: %#v", batch[1]())
	}
}

// advanceToNextRound 驱动壳完成一次轮推进：消费激活轮的推进标记、执行
// 轮准备命令、接住准备结果。返回更新后的壳。
func advanceToNextRound(t *testing.T, q QueueModel) QueueModel {
	t.Helper()
	updated, cmd := q.Update(doneMsg{})
	q = updated.(QueueModel)
	if cmd == nil {
		t.Fatal("doneMsg 后壳应安排准备下一轮")
	}
	updated, _ = q.Update(cmd())
	return updated.(QueueModel)
}

// Ctrl+↑/↓ 在各轮视图间切换，边界轮无操作；视图跟着激活轮走。
func TestQueueSwitchRoundsWithCtrlArrows(t *testing.T) {
	round1 := newQueueTestRound(t, true)
	round1.SetRoundLabel("第 1/2 轮 甲")
	var round2 tuiModel
	prepare := func(index int) (QueueRound, bool, error) {
		round2 = newQueueTestRound(t, false)
		round2.SetRoundLabel("第 2/2 轮 乙")
		return round2, false, nil
	}
	q := NewQueueModel(round1, 0, prepare, 2, &atomic.Bool{})
	q = advanceToNextRound(t, q)
	if q.active != 1 {
		t.Fatalf("推进后应在第二轮: active=%d", q.active)
	}

	// Ctrl+↑ 切回第一轮；再到边界无操作。
	updated, _ := q.Update(tea.KeyMsg{Type: tea.KeyCtrlUp})
	q = updated.(QueueModel)
	if q.active != 0 {
		t.Fatalf("Ctrl+↑ 应切回第一轮: active=%d", q.active)
	}
	updated, _ = q.Update(tea.KeyMsg{Type: tea.KeyCtrlUp})
	q = updated.(QueueModel)
	if q.active != 0 {
		t.Fatalf("第一轮再按 Ctrl+↑ 应无操作: active=%d", q.active)
	}
	if view := q.View(); !strings.Contains(view, "第 1/2 轮 甲") {
		t.Fatalf("应显示第一轮视图: %q", view)
	}

	// Ctrl+↓ 切回第二轮；到边界无操作。
	updated, _ = q.Update(tea.KeyMsg{Type: tea.KeyCtrlDown})
	q = updated.(QueueModel)
	updated, _ = q.Update(tea.KeyMsg{Type: tea.KeyCtrlDown})
	q = updated.(QueueModel)
	if q.active != 1 {
		t.Fatalf("Ctrl+↓ 应停在第二轮: active=%d", q.active)
	}
	if view := q.View(); !strings.Contains(view, "第 2/2 轮 乙") {
		t.Fatalf("应显示第二轮视图: %q", view)
	}
}

// 单轮队列没有切换目标：键位无效，帮助行也不标注。
func TestQueueSingleRoundSwitchKeysInert(t *testing.T) {
	q := NewQueueModel(newQueueTestRound(t, false), 0, func(int) (QueueRound, bool, error) {
		return nil, false, nil
	}, 1, &atomic.Bool{})
	updated, _ := q.Update(tea.KeyMsg{Type: tea.KeyCtrlUp})
	q = updated.(QueueModel)
	updated, _ = q.Update(tea.KeyMsg{Type: tea.KeyCtrlDown})
	q = updated.(QueueModel)
	if q.active != 0 || len(q.rounds) != 1 {
		t.Fatalf("单轮队列切换键位应无效: active=%d rounds=%d", q.active, len(q.rounds))
	}
	if m := q.rounds[0].(tuiModel); m.help.keyMap.SwitchRound.Enabled() {
		t.Fatal("单轮帮助行不应标注切换键位")
	}
}

// 切换视图时重放当前窗口尺寸：留在旧视图的轮错过窗口调整，切回去布局不塌。
func TestQueueSwitchReplaysWindowSize(t *testing.T) {
	round1 := newQueueTestRound(t, true)
	prepare := func(index int) (QueueRound, bool, error) {
		return newQueueTestRound(t, false), false, nil
	}
	q := NewQueueModel(round1, 0, prepare, 2, &atomic.Bool{})

	updated, _ := q.Update(tea.WindowSizeMsg{Width: 90, Height: 30})
	q = updated.(QueueModel)
	q = advanceToNextRound(t, q)
	// 在第二轮调整窗口：第一轮错过这次尺寸。
	updated, _ = q.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	q = updated.(QueueModel)
	// 切回第一轮：重放当前尺寸。
	updated, _ = q.Update(tea.KeyMsg{Type: tea.KeyCtrlUp})
	q = updated.(QueueModel)

	got := q.rounds[0].(tuiModel)
	if got.windowWidth != 80 || got.windowHeight != 24 {
		t.Fatalf("切回的视图应拿到重放尺寸: %dx%d", got.windowWidth, got.windowHeight)
	}
}

// 在测轮切走后测试继续：结果持续流入属主轮（非激活视图），重新武装的
// 等待命令仍带属主标记；切回显示实时进度。已完成轮的数据不变。
func TestQueueBackgroundRoundKeepsReceivingResults(t *testing.T) {
	round1 := newQueueTestRound(t, true)
	round1.results = append(round1.results, &speedtester.Result{
		ProxyName:   "香港 01",
		ProxyType:   "SS",
		ProxyConfig: map[string]any{},
	})
	var round2Channel chan *speedtester.Result
	prepare := func(index int) (QueueRound, bool, error) {
		view := newQueueTestRound(t, false)
		round2Channel = view.resultChannel
		return view, false, nil
	}
	q := NewQueueModel(round1, 0, prepare, 2, &atomic.Bool{})
	q = advanceToNextRound(t, q)
	// 切回第一轮，第二轮转入后台。
	updated, _ := q.Update(tea.KeyMsg{Type: tea.KeyCtrlUp})
	q = updated.(QueueModel)
	if q.active != 0 {
		t.Fatalf("应停留在第一轮: active=%d", q.active)
	}

	// 后台在测轮的结果流入属主轮，第一轮不受影响。
	second := &speedtester.Result{ProxyName: "日本 01", ProxyType: "SS", ProxyConfig: map[string]any{}}
	updated, rearmed := q.Update(roundOwnedMsg{round: 1, msg: resultMsg{result: second}})
	q = updated.(QueueModel)
	if got := q.rounds[1].(tuiModel); len(got.results) != 1 || got.results[0] != second {
		t.Fatalf("后台轮应收到自己的结果: %#v", got.results)
	}
	if got := q.rounds[0].(tuiModel); len(got.results) != 1 {
		t.Fatalf("已完成轮不应收到别轮结果: %#v", got.results)
	}

	// 消化结果后重新武装的等待命令仍带属主标记，后台继续收第三发。
	third := &speedtester.Result{ProxyName: "日本 02", ProxyType: "SS", ProxyConfig: map[string]any{}}
	round2Channel <- third
	batch, ok := rearmed().(tea.BatchMsg)
	if !ok {
		t.Fatalf("重新武装的命令应展开为批次: %T", rearmed())
	}
	msgs := make(chan tea.Msg, len(batch))
	for _, c := range batch {
		go func(c tea.Cmd) { msgs <- c() }(c)
	}
	var routed *roundOwnedMsg
	for i := 0; i < len(batch); i++ {
		select {
		case msg := <-msgs:
			if owned, ok := msg.(roundOwnedMsg); ok {
				if _, isResult := owned.msg.(resultMsg); isResult {
					routed = &owned
				}
			}
		case <-time.After(time.Second):
			t.Fatal("等待后台轮结果超时")
		}
	}
	if routed == nil || routed.round != 1 {
		t.Fatalf("重新武装的命令应把结果送回属主轮: %#v", routed)
	}
	updated, _ = q.Update(*routed)
	q = updated.(QueueModel)
	if got := q.rounds[1].(tuiModel); len(got.results) != 2 {
		t.Fatalf("后台轮应继续收到结果: %#v", got.results)
	}

	// 刷新表格后切回第二轮：看到实时进度。
	updated, _ = q.Update(roundOwnedMsg{round: 1, msg: flushResultsMsg{}})
	q = updated.(QueueModel)
	updated, _ = q.Update(tea.KeyMsg{Type: tea.KeyCtrlDown})
	q = updated.(QueueModel)
	if q.active != 1 {
		t.Fatalf("Ctrl+↓ 应回到第二轮: active=%d", q.active)
	}
	if view := q.View(); !strings.Contains(view, "日本 02") {
		t.Fatalf("切回应看到实时进度: %q", view)
	}
}

// 后台在测轮测完自动推进，不抢用户正看的视图；推进由轮完成事件驱动，
// 与用户正看着哪一轮无关。
func TestQueueBackgroundAdvanceKeepsCurrentView(t *testing.T) {
	prepared := []int{}
	round1 := newQueueTestRound(t, true)
	prepare := func(index int) (QueueRound, bool, error) {
		prepared = append(prepared, index)
		return newQueueTestRound(t, index < 2), false, nil
	}
	q := NewQueueModel(round1, 0, prepare, 3, &atomic.Bool{})
	q = advanceToNextRound(t, q)
	if len(prepared) != 1 || prepared[0] != 1 {
		t.Fatalf("应准备源 1: %v", prepared)
	}
	// 切回第一轮（已完成）。
	updated, _ := q.Update(tea.KeyMsg{Type: tea.KeyCtrlUp})
	q = updated.(QueueModel)

	// 后台在测的第二轮测完：推进第三轮，当前视图不被动。
	updated, prepareCmd := q.Update(roundOwnedMsg{round: 1, msg: doneMsg{}})
	q = updated.(QueueModel)
	if prepareCmd == nil {
		t.Fatal("后台轮测完应推进下一轮")
	}
	updated, _ = q.Update(prepareCmd())
	q = updated.(QueueModel)
	if len(prepared) != 2 || prepared[1] != 2 {
		t.Fatalf("应准备源 2: %v", prepared)
	}
	if len(q.rounds) != 3 || q.active != 0 {
		t.Fatalf("后台推进不应抢当前视图: active=%d rounds=%d", q.active, len(q.rounds))
	}

	// Ctrl+↓ 两次切到第三轮：新一轮视图就绪可看。
	updated, _ = q.Update(tea.KeyMsg{Type: tea.KeyCtrlDown})
	q = updated.(QueueModel)
	updated, _ = q.Update(tea.KeyMsg{Type: tea.KeyCtrlDown})
	q = updated.(QueueModel)
	if q.active != 2 {
		t.Fatalf("应能切到第三轮: active=%d", q.active)
	}
}

// 已完成轮（含切回去看的旧轮）按 q：退出整个队列而不是开新轮（「退出」
// 词条、ADR-0002）；剩余轮作废，在途的轮准备不再激活新视图。
func TestQueueQuitKeyOnFinishedRoundExitsQueue(t *testing.T) {
	prepared := []int{}
	round1 := newQueueTestRound(t, true)
	round1.SetImageExport(inRepoTempDir(t), false)
	prepare := func(index int) (QueueRound, bool, error) {
		prepared = append(prepared, index)
		return newQueueTestRound(t, false), false, nil
	}
	voided := &atomic.Bool{}
	q := NewQueueModel(round1, 0, prepare, 3, voided)
	q = advanceToNextRound(t, q)
	// 切回已完成的第一轮。
	updated, _ := q.Update(tea.KeyMsg{Type: tea.KeyCtrlUp})
	q = updated.(QueueModel)

	// 按 q：先走收尾保存，标记退出全部。
	updated, saveCmd := q.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	q = updated.(QueueModel)
	if saveCmd == nil {
		t.Fatal("已完成轮按 q 应走收尾保存")
	}
	if result := q.Result(); !result.ExitAll {
		t.Fatalf("已完成中途轮按 q 应标记退出全部: %+v", result)
	}
	saved, ok := runQueueCmd(t, saveCmd).(imageSavedMsg)
	if !ok {
		t.Fatalf("收尾保存应返回 imageSavedMsg: %T", saved)
	}
	updated, quitCmd := q.Update(saved)
	q = updated.(QueueModel)
	if _, ok := quitCmd().(tea.QuitMsg); !ok {
		t.Fatalf("保存完应退出整个队列: %T", quitCmd())
	}
	// 剩余轮作废：不开新的轮，在途准备的结果也不入列。
	if len(prepared) != 1 {
		t.Fatalf("退出后不应再准备新的轮: %v", prepared)
	}
	if !voided.Load() {
		t.Fatal("退出全部应作废剩余轮")
	}
	updated, _ = q.Update(nextRoundMsg{view: newQueueTestRound(t, false)})
	q = updated.(QueueModel)
	if len(q.rounds) != 2 {
		t.Fatalf("作废后的准备结果不应入列: rounds=%d", len(q.rounds))
	}
}

// 切走后再按 Esc：语义不变——中断本轮、作废整个队列（含后台在测轮）、
// 回选源，不再准备新的轮。
func TestQueueEscapeAfterSwitchStillVoidsQueue(t *testing.T) {
	voided := &atomic.Bool{}
	prepared := []int{}
	round1 := newQueueTestRound(t, true)
	round1.SetEscapeToParent(true)
	prepare := func(index int) (QueueRound, bool, error) {
		prepared = append(prepared, index)
		return newQueueTestRound(t, false), false, nil
	}
	q := NewQueueModel(round1, 0, prepare, 2, voided)
	q = advanceToNextRound(t, q)
	updated, _ := q.Update(tea.KeyMsg{Type: tea.KeyCtrlUp})
	q = updated.(QueueModel)

	updated, quitCmd := q.Update(tea.KeyMsg{Type: tea.KeyEsc})
	q = updated.(QueueModel)
	if _, ok := quitCmd().(tea.QuitMsg); !ok {
		t.Fatalf("Esc 应结束队列程序: %T", quitCmd())
	}
	result := q.Result()
	if !result.Escaped {
		t.Fatal("Esc 后应报告回选源")
	}
	if !voided.Load() {
		t.Fatal("Esc 应作废整个队列（含后台在测轮）")
	}
	if len(prepared) != 1 {
		t.Fatalf("Esc 后不应再准备新的轮: %v", prepared)
	}
}

// 切走后再按 Ctrl+C：语义不变——退出全部（含后台在测轮），不回选源；
// 等落盘的收尾照旧走完。
func TestQueueCtrlCAfterSwitchExitsAll(t *testing.T) {
	round1 := newQueueTestRound(t, true)
	prepare := func(index int) (QueueRound, bool, error) {
		return newQueueTestRound(t, false), false, nil
	}
	q := NewQueueModel(round1, 0, prepare, 2, &atomic.Bool{})
	q = advanceToNextRound(t, q)
	updated, _ := q.Update(tea.KeyMsg{Type: tea.KeyCtrlUp})
	q = updated.(QueueModel)

	updated, saveCmd := q.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	q = updated.(QueueModel)
	if saveCmd == nil {
		t.Fatal("Ctrl+C 应先走收尾保存")
	}
	if result := q.Result(); !result.ExitAll || result.Escaped {
		t.Fatalf("Ctrl+C 应标记退出全部且不回选源: %+v", result)
	}
	saved, ok := runQueueCmd(t, saveCmd).(imageSavedMsg)
	if !ok {
		t.Fatalf("收尾保存应返回 imageSavedMsg: %T", saved)
	}
	updated, quitCmd := q.Update(roundOwnedMsg{round: 0, msg: saved})
	q = updated.(QueueModel)
	if _, ok := quitCmd().(tea.QuitMsg); !ok {
		t.Fatalf("保存完应退出全部: %T", quitCmd())
	}
}

// 帮助行的轮间切换键位只在第二个轮视图就位后出现（单轮队列与首测轮
// 都没有切换目标），出现后各轮视图都标注。
func TestQueueHelpShowsRoundSwitchKeysOnlyWithMultipleRounds(t *testing.T) {
	round1 := newQueueTestRound(t, true)
	var round2 tuiModel
	prepare := func(index int) (QueueRound, bool, error) {
		round2 = newQueueTestRound(t, false)
		return round2, false, nil
	}
	q := NewQueueModel(round1, 0, prepare, 2, &atomic.Bool{})
	if m := q.rounds[0].(tuiModel); m.help.keyMap.SwitchRound.Enabled() {
		t.Fatal("还没有第二个视图时帮助行不应标注切换键位")
	}

	q = advanceToNextRound(t, q)
	if len(q.rounds) != 2 {
		t.Fatalf("应有两个轮视图: %d", len(q.rounds))
	}
	for i, r := range q.rounds {
		view := r.(tuiModel).help.view()
		if !strings.Contains(view, "轮间切换") {
			t.Fatalf("第 %d 轮帮助行应标注切换键位: %q", i+1, view)
		}
	}
	_ = round2
}
