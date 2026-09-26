package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/faceair/clash-speedtest/speedtester"
)

func TestTableScrollWithKeyboard(t *testing.T) {
	resultChannel := make(chan *speedtester.Result, 1)
	model := NewTUIModel(speedtester.SpeedModeDownload, 5, resultChannel)

	model.results = []*speedtester.Result{
		{ProxyName: "Proxy 1", ProxyType: "SS"},
		{ProxyName: "Proxy 2", ProxyType: "SS"},
		{ProxyName: "Proxy 3", ProxyType: "SS"},
		{ProxyName: "Proxy 4", ProxyType: "SS"},
		{ProxyName: "Proxy 5", ProxyType: "SS"},
	}
	model.updateTableRows()
	model.table.SetHeight(3)

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	updatedModel := updated.(tuiModel)
	if updatedModel.table.Cursor() != 1 {
		t.Fatalf("expected cursor to move down to 1, got %d", updatedModel.table.Cursor())
	}
	if updatedModel.selectedIndex != 1 {
		t.Fatalf("expected selectedIndex to follow cursor, got %d", updatedModel.selectedIndex)
	}
}

func TestTableScrollWithMouseWheel(t *testing.T) {
	resultChannel := make(chan *speedtester.Result, 1)
	model := NewTUIModel(speedtester.SpeedModeDownload, 5, resultChannel)

	model.results = []*speedtester.Result{
		{ProxyName: "Proxy 1", ProxyType: "SS"},
		{ProxyName: "Proxy 2", ProxyType: "SS"},
		{ProxyName: "Proxy 3", ProxyType: "SS"},
		{ProxyName: "Proxy 4", ProxyType: "SS"},
		{ProxyName: "Proxy 5", ProxyType: "SS"},
	}
	model.updateTableRows()
	model.table.SetHeight(3)

	updated, _ := model.Update(tea.MouseMsg{Button: tea.MouseButtonWheelDown})
	updatedModel := updated.(tuiModel)
	if updatedModel.table.Cursor() != 1 {
		t.Fatalf("expected cursor to move down to 1, got %d", updatedModel.table.Cursor())
	}
	if updatedModel.selectedIndex != 1 {
		t.Fatalf("expected selectedIndex to follow cursor, got %d", updatedModel.selectedIndex)
	}
}

// 跟随最新条目：默认视口贴着尾部，看到的是最新测完的行。
func TestFollowTailScrollsToLatestRow(t *testing.T) {
	model := testingModel(t, 40)
	model.windowHeight = 16
	model.updateTableLayout()
	if start := model.viewportStart(); start != model.tableRowCount()-model.table.Height() {
		t.Fatalf("跟随最新条目时视口应贴底: start=%d want=%d",
			start, model.tableRowCount()-model.table.Height())
	}
	if got := latencyOnLine(firstDataLine(model)); got == 100 {
		t.Fatalf("跟随时第一可见行不应是最初那条: %d", got)
	}
}

// 向上滚取消跟随，一路向下滚回底部自动恢复。
func TestFollowTailCancelsUpwardAndResumesAtBottom(t *testing.T) {
	model := testingModel(t, 40)
	model.windowHeight = 16
	model.updateTableLayout()

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyUp})
	got := updated.(tuiModel)
	if got.followTail {
		t.Fatal("向上滚应取消跟随")
	}
	if start := got.viewportStart(); start >= got.tableRowCount()-got.table.Height() {
		t.Fatalf("取消后视口应离开尾部: %d", start)
	}

	for i := 0; i < 60 && !got.followTail; i++ {
		updated, _ = got.Update(tea.KeyMsg{Type: tea.KeyDown})
		got = updated.(tuiModel)
	}
	if !got.followTail {
		t.Fatal("滚回底部应恢复跟随")
	}
}

// 滚轮向上同样取消跟随。
func TestWheelUpCancelsFollowTail(t *testing.T) {
	model := testingModel(t, 40)
	model.windowHeight = 16
	model.updateTableLayout()
	updated, _ := model.Update(tea.MouseMsg{Button: tea.MouseButtonWheelUp})
	if updated.(tuiModel).followTail {
		t.Fatal("滚轮向上应取消跟随")
	}
}
