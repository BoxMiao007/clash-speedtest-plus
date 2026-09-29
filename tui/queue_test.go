package tui

import (
	"sync/atomic"
	"testing"

	"github.com/BoxMiao007/clash-speedtest-plus/speedtester"
	tea "github.com/charmbracelet/bubbletea"
)

// newQueueTestRound 造一个无产物的轮视图：关自动图、不设 configSaver，
// 测完走「标记推进」路径，不碰真实文件系统（图片目录用临时目录）。
func newQueueTestRound(t *testing.T, autoAdvance bool) tuiModel {
	model := NewTUIModel(speedtester.SpeedModeDownload, 1, make(chan *speedtester.Result, 4))
	model.SetImageExport(t.TempDir(), false)
	model.SetAutoAdvance(autoAdvance)
	return model
}

// runQueueCmd 执行壳返回的命令并解开属主包裹：轮视图命令产出的消息带
// roundOwnedMsg 标记，壳侧命令（轮准备）原样返回。模拟运行时投递用。
func runQueueCmd(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatal("命令不应为空")
	}
	msg := cmd()
	if owned, ok := msg.(roundOwnedMsg); ok {
		return owned.msg
	}
	return msg
}

// 队列壳核心行为（ADR-0015 seam）：一个程序承载整个队列，轮推进时开新一轮
// 视图，已完成轮的数据保留不丢，最后一轮照旧停在界面等退出键。
func TestQueueAdvancesAndKeepsRoundResults(t *testing.T) {
	round1 := newQueueTestRound(t, true)
	prepared := []int{}
	prepare := func(index int) (QueueRound, bool, error) {
		prepared = append(prepared, index)
		view := newQueueTestRound(t, false)
		view.SetImageSource("源C")
		return view, false, nil
	}
	q := NewQueueModel(round1, 0, prepare, 2, &atomic.Bool{})

	first := &speedtester.Result{ProxyName: "香港 01", ProxyType: "SS", ProxyConfig: map[string]any{}}
	updated, _ := q.Update(resultMsg{result: first})
	q = updated.(QueueModel)
	if got := q.rounds[0].(tuiModel); len(got.results) != 1 || got.results[0] != first {
		t.Fatalf("第一轮应收到结果: %#v", got.results)
	}

	// 第一轮测完（无产物）：标记推进，壳安排准备第二轮。
	updated, cmd := q.Update(doneMsg{})
	q = updated.(QueueModel)
	if cmd == nil {
		t.Fatal("doneMsg 后壳应安排准备下一轮")
	}
	msg := cmd()
	if _, ok := msg.(nextRoundMsg); !ok {
		t.Fatalf("准备命令应返回 nextRoundMsg: %T", msg)
	}
	updated, _ = q.Update(msg)
	q = updated.(QueueModel)
	if len(q.rounds) != 2 || q.active != 1 {
		t.Fatalf("应切换到第二轮视图: rounds=%d active=%d", len(q.rounds), q.active)
	}
	if len(prepared) != 1 || prepared[0] != 1 {
		t.Fatalf("应准备源下标 1: %v", prepared)
	}

	// 第二轮收到自己的结果，第一轮的数据不丢。
	second := &speedtester.Result{ProxyName: "日本 01", ProxyType: "SS", ProxyConfig: map[string]any{}}
	updated, _ = q.Update(resultMsg{result: second})
	q = updated.(QueueModel)
	if got := q.rounds[1].(tuiModel); len(got.results) != 1 || got.results[0] != second {
		t.Fatalf("第二轮应收到自己的结果: %#v", got.results)
	}
	if got := q.rounds[0].(tuiModel); len(got.results) != 1 || got.results[0] != first {
		t.Fatalf("已完成轮的数据应保留: %#v", got.results)
	}

	// 第二轮是最后一轮：测完不推进，停在界面等退出键。
	updated, advCmd := q.Update(doneMsg{})
	q = updated.(QueueModel)
	if advCmd != nil {
		t.Fatal("最后一轮测完不应安排推进")
	}
	if q.active != 1 || len(q.rounds) != 2 {
		t.Fatalf("最后一轮应停留在本轮视图: active=%d rounds=%d", q.active, len(q.rounds))
	}

	// q 离开最后一轮：等收尾保存完成后退出，不回选源。
	updated, saveCmd := q.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	q = updated.(QueueModel)
	if saveCmd == nil {
		t.Fatal("最后一轮按 q 应触发收尾保存")
	}
	saved, ok := runQueueCmd(t, saveCmd).(imageSavedMsg)
	if !ok {
		t.Fatalf("收尾保存应返回 imageSavedMsg: %T", saved)
	}
	updated, quitCmd := q.Update(saved)
	q = updated.(QueueModel)
	if _, ok := quitCmd().(tea.QuitMsg); !ok {
		t.Fatalf("保存完应退出: %T", quitCmd)
	}
	result := q.Result()
	if result.Escaped || result.ExitAll {
		t.Fatalf("最后一轮正常离开不回选源也不算退出全部: %+v", result)
	}
	if len(result.Statuses) == 0 {
		t.Fatal("离开时的收尾状态应被捕获，供 stderr 补打")
	}
}

// 推进到新一轮时重放当前窗口尺寸：新视图没赶上程序的 WindowSizeMsg，布局不能塌。
func TestQueueReplaysWindowSizeOnAdvance(t *testing.T) {
	round1 := newQueueTestRound(t, true)
	prepare := func(index int) (QueueRound, bool, error) {
		return newQueueTestRound(t, false), false, nil
	}
	q := NewQueueModel(round1, 0, prepare, 2, &atomic.Bool{})

	updated, _ := q.Update(tea.WindowSizeMsg{Width: 90, Height: 30})
	q = updated.(QueueModel)
	updated, cmd := q.Update(doneMsg{})
	q = updated.(QueueModel)
	updated, _ = q.Update(cmd())
	q = updated.(QueueModel)

	got := q.rounds[q.active].(tuiModel)
	if got.windowWidth != 90 || got.windowHeight != 30 {
		t.Fatalf("新视图应拿到重放的窗口尺寸: %dx%d", got.windowWidth, got.windowHeight)
	}
}

// 某源解析不出节点：壳跳过该轮继续准备下一轮，不废掉队列。
func TestQueueSkipsEmptySourceAndActivatesNext(t *testing.T) {
	round1 := newQueueTestRound(t, true)
	prepared := []int{}
	prepare := func(index int) (QueueRound, bool, error) {
		prepared = append(prepared, index)
		if index == 1 {
			return nil, true, nil // 第二个源 0 节点
		}
		return newQueueTestRound(t, false), false, nil
	}
	q := NewQueueModel(round1, 0, prepare, 3, &atomic.Bool{})

	updated, cmd := q.Update(doneMsg{})
	q = updated.(QueueModel)
	updated, next := q.Update(cmd())
	q = updated.(QueueModel)
	if next == nil {
		t.Fatal("跳过空源后应继续准备下一轮")
	}
	updated, _ = q.Update(next())
	q = updated.(QueueModel)

	if len(prepared) != 2 || prepared[0] != 1 || prepared[1] != 2 {
		t.Fatalf("应依次准备源 1 和 2: %v", prepared)
	}
	if q.active != 1 || len(q.rounds) != 2 {
		t.Fatalf("应激活第三个源的视图: active=%d rounds=%d", q.active, len(q.rounds))
	}
}

// 最后一个源解析不出节点：跳过后队列走完，正常结束（不回选源）。
func TestQueueLastSourceSkippedFinishesQueue(t *testing.T) {
	round1 := newQueueTestRound(t, true)
	prepare := func(index int) (QueueRound, bool, error) {
		return nil, true, nil
	}
	q := NewQueueModel(round1, 0, prepare, 2, &atomic.Bool{})

	updated, cmd := q.Update(doneMsg{})
	q = updated.(QueueModel)
	updated, quitCmd := q.Update(cmd())
	q = updated.(QueueModel)
	if _, ok := quitCmd().(tea.QuitMsg); !ok {
		t.Fatalf("队列走完应正常结束: %T", quitCmd)
	}
	if q.Result().Escaped {
		t.Fatal("队列正常走完不应报告回选源")
	}
}

// Esc：中断本轮＋作废剩余＋回选源；在途的轮准备被 voided 拦下。
func TestQueueEscapeAbortsAndVoidsRemaining(t *testing.T) {
	round1 := newQueueTestRound(t, false)
	round1.SetEscapeToParent(true)
	voided := &atomic.Bool{}
	prepareCalled := false
	prepare := func(index int) (QueueRound, bool, error) {
		prepareCalled = true
		return newQueueTestRound(t, false), false, nil
	}
	q := NewQueueModel(round1, 0, prepare, 2, voided)

	updated, quitCmd := q.Update(tea.KeyMsg{Type: tea.KeyEsc})
	q = updated.(QueueModel)
	if _, ok := quitCmd().(tea.QuitMsg); !ok {
		t.Fatalf("Esc 应结束队列程序: %T", quitCmd)
	}
	result := q.Result()
	if !result.Escaped {
		t.Fatal("Esc 后应报告回选源")
	}
	if !voided.Load() {
		t.Fatal("Esc 后应作废在途的轮准备")
	}
	if prepareCalled {
		t.Fatal("Esc 后不应再准备新的轮")
	}

	// 已在途的轮准备此刻完成：壳丢弃结果直接结束，不激活新视图。
	updated, _ = q.Update(nextRoundMsg{view: newQueueTestRound(t, false)})
	q = updated.(QueueModel)
	if len(q.rounds) != 1 {
		t.Fatalf("作废后的准备结果不应入列: rounds=%d", len(q.rounds))
	}
}

// Ctrl+C 在未完成轮：退出全部（中断本轮＋作废剩余），不回选源。
func TestQueueCtrlCExitsAllMidRound(t *testing.T) {
	round1 := newQueueTestRound(t, false)
	prepareCalled := false
	prepare := func(index int) (QueueRound, bool, error) {
		prepareCalled = true
		return newQueueTestRound(t, false), false, nil
	}
	q := NewQueueModel(round1, 0, prepare, 2, &atomic.Bool{})

	updated, quitCmd := q.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	q = updated.(QueueModel)
	if _, ok := quitCmd().(tea.QuitMsg); !ok {
		t.Fatalf("Ctrl+C 应结束程序: %T", quitCmd)
	}
	result := q.Result()
	if !result.ExitAll {
		t.Fatal("Ctrl+C 应标记退出全部")
	}
	if result.Escaped {
		t.Fatal("Ctrl+C 不回选源")
	}
	if prepareCalled {
		t.Fatal("退出全部后不应准备新的轮")
	}
}

// 已完成轮在推进准备期间按 Ctrl+C：保存完收尾后退出全部，
// 在途的准备被 voided 拦下，不再开下一轮。
func TestQueueCtrlCOnFinishedRoundExitsAll(t *testing.T) {
	round1 := newQueueTestRound(t, true)
	prepare := func(index int) (QueueRound, bool, error) {
		return newQueueTestRound(t, false), false, nil
	}
	voided := &atomic.Bool{}
	q := NewQueueModel(round1, 0, prepare, 2, voided)

	// 第一轮测完（无产物）：壳安排准备第二轮，命令还在途。
	updated, prepareCmd := q.Update(doneMsg{})
	q = updated.(QueueModel)
	if prepareCmd == nil {
		t.Fatal("应安排准备下一轮")
	}

	// 准备在途时按 Ctrl+C：先走收尾保存，再退出。
	updated, saveCmd := q.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	q = updated.(QueueModel)
	if saveCmd == nil {
		t.Fatal("收尾保存应先于退出")
	}
	saved, ok := runQueueCmd(t, saveCmd).(imageSavedMsg)
	if !ok {
		t.Fatalf("收尾保存应返回 imageSavedMsg: %T", saved)
	}
	updated, quitCmd := q.Update(saved)
	q = updated.(QueueModel)
	if _, ok := quitCmd().(tea.QuitMsg); !ok {
		t.Fatalf("保存完应退出全部: %T", quitCmd)
	}
	if !q.Result().ExitAll {
		t.Fatal("Ctrl+C 应标记退出全部")
	}

	// 在途准备此刻完成：voided 已置位，壳丢弃结果，不开下一轮。
	updated, _ = q.Update(prepareCmd())
	q = updated.(QueueModel)
	if len(q.rounds) != 1 {
		t.Fatalf("退出全部后不应激活新视图: rounds=%d", len(q.rounds))
	}
	if !voided.Load() {
		t.Fatal("退出全部应作废在途的轮准备")
	}
}

// 未完成轮按 q：结束整个进程（ADR-0002），不再只关本轮界面留剩余轮。
func TestQuitKeyMidRoundExitsAll(t *testing.T) {
	model := newQueueTestRound(t, false)
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	m := updated.(tuiModel)
	if !m.ExitAll() || !m.quitting {
		t.Fatalf("未完成轮按 q 应退出全部: exitAll=%v quitting=%v", m.ExitAll(), m.quitting)
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("应返回退出命令: %T", cmd)
	}
}

// 中途轮产物已存时按 q：标记推进下一轮，而不是退出整个队列。
func TestQuitOnFinishedMidQueueRoundRequestsAdvance(t *testing.T) {
	dir := inRepoTempDir(t)
	model := newQueueTestRound(t, true)
	model.SetImageExport(dir, true)
	saveCount := 0
	model.SetConfigSaver(func([]*speedtester.Result) (string, error) {
		saveCount++
		return "clash-speedtest-plus.yaml", nil
	})
	model.results = append(model.results, &speedtester.Result{
		ProxyName: "香港 01",
		ProxyType: "SS",
	})

	// 测完自动保存。
	updated, cmd := model.Update(doneMsg{})
	m := updated.(tuiModel)
	msg := cmd()
	updated, _ = m.Update(msg)
	m = updated.(tuiModel)
	if saveCount != 1 {
		t.Fatalf("自动保存应写一次配置: %d", saveCount)
	}

	// 再按 q：产物已存，直接标记推进，不退出。
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	m = updated.(tuiModel)
	if m.quitting {
		t.Fatal("中途轮按 q 不应退出，由队列壳推进下一轮")
	}
	if !m.AdvanceRequested() {
		t.Fatal("中途轮按 q 应标记推进下一轮")
	}
	if saveCount != 1 {
		t.Fatalf("按 q 不应再次保存: %d", saveCount)
	}
}
