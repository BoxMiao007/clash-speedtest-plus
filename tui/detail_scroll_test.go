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

func TestClickSecondRowSelectsSecond(t *testing.T) {
	model := testingModel(t, 4)
	clicked := clickRow(model, 1)
	if clicked.table.Cursor() != 1 {
		t.Fatalf("点击第二行实际选中了 %d", clicked.table.Cursor())
	}
	if clicked.detailResult != clicked.results[1] {
		t.Fatal("点击第二行打开的不是第二行详情")
	}
}

func TestScrollbarAppearsWhenRowsOverflow(t *testing.T) {
	model := testingModel(t, 40)
	model.windowHeight = 16
	model.updateTableLayout()
	view := model.View()
	if !strings.Contains(view, "█") {
		t.Fatalf("行数超出窗口时应显示滚动条:\n%s", view)
	}
	lines := strings.Split(view, "\n")
	start := model.tableHeaderY() + dataRowOffset(model.table.View())
	found := false
	for i := start; i < start+model.table.Height() && i < len(lines); i++ {
		plain := stripANSI(lines[i])
		last := []rune(strings.TrimRight(plain, " "))
		if len(last) == 0 {
			continue
		}
		mark := last[len(last)-1]
		if mark != '█' && mark != '░' {
			t.Fatalf("滚动条不在窗口最右: %q", plain)
		}
		found = true
	}
	if !found {
		t.Fatal("数据行上看不到滚动条")
	}
	if model.table.Width() >= model.windowWidth {
		t.Fatalf("滚动条应让出最右一列: table=%d window=%d", model.table.Width(), model.windowWidth)
	}
}

func TestScrollbarClickAndDrag(t *testing.T) {
	model := testingModel(t, 40)
	model.windowHeight = 16
	model.updateTableLayout()
	startY := model.tableHeaderY() + dataRowOffset(model.table.View())
	bottom := startY + model.table.Height() - 1

	clicked, _ := model.Update(tea.MouseMsg{
		X: model.windowWidth - 1, Y: bottom,
		Button: tea.MouseButtonLeft, Action: tea.MouseActionPress,
	})
	jumped := clicked.(tuiModel)
	if jumped.table.Cursor() < 20 {
		t.Fatalf("点击轨道底部应跳到后面的行: %d", jumped.table.Cursor())
	}

	top, _, ok := jumped.scrollbarRange()
	if !ok {
		t.Fatal("行数超出时应有滑块")
	}
	pressed, _ := jumped.Update(tea.MouseMsg{
		X: model.windowWidth - 1, Y: startY + top,
		Button: tea.MouseButtonLeft, Action: tea.MouseActionPress,
	})
	dragging := pressed.(tuiModel)
	if !dragging.scrollbarDrag {
		t.Fatal("按住滑块应进入拖动")
	}
	moved, _ := dragging.Update(tea.MouseMsg{
		X: model.windowWidth - 1, Y: startY,
		Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion,
	})
	if moved.(tuiModel).table.Cursor() != 0 {
		t.Fatalf("拖到顶部应选中第一行: %d", moved.(tuiModel).table.Cursor())
	}
	// Windows 拖出轨道时按钮仍是左键，不能丢。
	off, _ := moved.(tuiModel).Update(tea.MouseMsg{
		X: 0, Y: startY,
		Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion,
	})
	if !off.(tuiModel).scrollbarDrag {
		t.Fatal("拖出轨道不应结束拖动")
	}
	released, _ := off.(tuiModel).Update(tea.MouseMsg{
		X: 0, Y: startY,
		Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease,
	})
	if released.(tuiModel).scrollbarDrag {
		t.Fatal("松手应结束拖动")
	}
	if released.(tuiModel).detailVisible {
		t.Fatal("拖动滚动条松手不应打开详情")
	}
}

func TestThumbSitsOnCurrentRow(t *testing.T) {
	model := testingModel(t, 30)
	model.windowHeight = 20
	model.updateTableLayout()
	model.table.SetCursor(model.tableRowCount() - 1)
	top, thumb, ok := model.scrollbarRange()
	if !ok {
		t.Fatal("应有滚动条")
	}
	if top+thumb != model.table.Height() {
		t.Fatalf("选中最后一行时滑块应贴底: top=%d thumb=%d height=%d", top, thumb, model.table.Height())
	}
	model.table.SetCursor(0)
	top, _, ok = model.scrollbarRange()
	if !ok || top != 0 {
		t.Fatalf("选中第一行时滑块应贴顶: %d", top)
	}
}

// 选中第 9 行后刷新，不能跳回第 1 行。
func TestRefreshKeepsLaterSelection(t *testing.T) {
	model := testingModel(t, 10)
	opened := clickRow(model, 0)
	moved := opened
	for i := 0; i < 8; i++ {
		next, _ := moved.Update(tea.KeyMsg{Type: tea.KeyDown})
		moved = next.(tuiModel)
	}
	if moved.table.Cursor() != 8 {
		t.Fatalf("应停在第 9 行: %d", moved.table.Cursor())
	}
	refreshed, _ := moved.Update(nodeProgressMsg{progress: speedtester.Progress{Name: "inflight", Type: "SS"}})
	if refreshed.(tuiModel).table.Cursor() != 8 {
		t.Fatalf("刷新跳回了上一条: %d", refreshed.(tuiModel).table.Cursor())
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
