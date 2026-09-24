package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/faceair/clash-speedtest/speedtester"
)

func testingModel(t *testing.T, finished int) tuiModel {
	t.Helper()
	resultChannel := make(chan *speedtester.Result, 1)
	model := NewTUIModel(speedtester.SpeedModeDownload, finished+1, resultChannel)
	model.windowWidth = 120
	model.windowHeight = 24
	for i := 0; i < finished; i++ {
		model.results = append(model.results, &speedtester.Result{
			ProxyName: "Proxy",
			ProxyType: "SS",
			Latency:   time.Duration(100+i) * time.Millisecond,
		})
	}
	model.inFlight["inflight"] = &inFlightNode{name: "inflight", proxyType: "SS"}
	model.inFlightOrder = []string{"inflight"}
	model.updateTableRows()
	model.updateTableLayout()
	return model
}

func clickRow(model tuiModel, visualRow int) tuiModel {
	rowY := model.tableHeaderY() + tableHeaderLines + visualRow
	updated, _ := model.Update(tea.MouseMsg{
		X:      1,
		Y:      rowY,
		Action: tea.MouseActionRelease,
		Button: tea.MouseButtonLeft,
	})
	return updated.(tuiModel)
}

// 详情打开后，方向键和滚轮应能滚动表格，且详情仍停在点开的那一行。
func TestDetailOpenKeepsScrollAndPinnedDetail(t *testing.T) {
	model := testingModel(t, 8)
	opened := clickRow(model, 0)
	if !opened.detailVisible || opened.detailResult != opened.results[0] {
		t.Fatalf("点击应打开第一行详情: visible=%v", opened.detailVisible)
	}

	down, _ := opened.Update(tea.KeyMsg{Type: tea.KeyDown})
	scrolled := down.(tuiModel)
	if scrolled.table.Cursor() != 1 {
		t.Fatalf("详情打开后方向键下没有滚到下一行: %d", scrolled.table.Cursor())
	}
	if scrolled.detailResult != opened.results[1] {
		t.Fatal("方向键滚动后详情应跟着当前行")
	}

	wheel, _ := scrolled.Update(tea.MouseMsg{Button: tea.MouseButtonWheelDown})
	wheeled := wheel.(tuiModel)
	if wheeled.table.Cursor() != 2 {
		t.Fatalf("详情打开后滚轮没有继续滚动: %d", wheeled.table.Cursor())
	}

	refreshed, _ := wheeled.Update(nodeProgressMsg{progress: speedtester.Progress{Name: "inflight", Type: "SS"}})
	if refreshed.(tuiModel).table.Cursor() != 2 {
		t.Fatalf("进度刷新把光标拉回去了: %d", refreshed.(tuiModel).table.Cursor())
	}
}

// 进度刷新不应把已经滚到下面的视口拉回顶部。
func TestProgressRefreshKeepsViewport(t *testing.T) {
	model := testingModel(t, 30)
	model.table.GotoBottom()
	before := model.table.View()
	if !strings.Contains(before, "inflight") {
		t.Fatal("滚到底应能看到在测行")
	}

	updated, _ := model.Update(nodeProgressMsg{progress: speedtester.Progress{Name: "inflight", Type: "SS"}})
	after := updated.(tuiModel).table.View()
	if !strings.Contains(after, "inflight") {
		t.Fatal("进度刷新把视口拉回顶部，在测行看不见了")
	}
}
