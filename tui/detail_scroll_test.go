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
	if jumped.table.Cursor() != 0 {
		t.Fatalf("点击滚动条不应改选中行: %d", jumped.table.Cursor())
	}
	if jumped.scrollOffset < 20 {
		t.Fatalf("点击轨道底部应滚到后面的行: %d", jumped.scrollOffset)
	}
	if strings.Contains(firstDataLine(jumped), "100ms") {
		t.Fatalf("滚动后第一行不应还是最初那条:\n%s", firstDataLine(jumped))
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
		t.Fatalf("拖动滚动条不应改选中行: %d", moved.(tuiModel).table.Cursor())
	}
	if moved.(tuiModel).scrollOffset != 0 {
		t.Fatalf("拖到顶部视口应回到第一行: %d", moved.(tuiModel).scrollOffset)
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

// 滚出第一屏后再点击，详情和选中行必须是画面上那一行，不能是它上面一条。
func TestClickAfterScrollSelectsVisibleRow(t *testing.T) {
	model := testingModel(t, 40)
	model.windowHeight = 16
	model.updateTableLayout()
	cur := model
	for i := 0; i < 15; i++ {
		next, _ := cur.Update(tea.KeyMsg{Type: tea.KeyDown})
		cur = next.(tuiModel)
	}
	lines := strings.Split(cur.View(), "\n")
	start := cur.tableHeaderY() + dataRowOffset(cur.table.View())
	if start >= len(lines) {
		t.Fatalf("找不到第一条数据行: start=%d lines=%d", start, len(lines))
	}
	shown := latencyOnLine(stripANSI(lines[start]))
	if shown <= 0 {
		t.Fatalf("第一条数据行没有延迟: %q", stripANSI(lines[start]))
	}
	clicked := clickRow(cur, 0)
	if clicked.detailResult == nil {
		t.Fatal("点击应打开详情")
	}
	if clicked.detailResult.Latency.Milliseconds() != int64(shown) {
		t.Fatalf("画面上是 %dms，点开的是 %dms", shown, clicked.detailResult.Latency.Milliseconds())
	}
	if clicked.table.Cursor() != shown-100 {
		t.Fatalf("选中行应是 %d，实际 %d", shown-100, clicked.table.Cursor())
	}
}

func firstDataLine(model tuiModel) string {
	lines := strings.Split(model.View(), "\n")
	start := model.tableHeaderY() + dataRowOffset(model.table.View())
	if start < 0 || start >= len(lines) {
		return ""
	}
	return stripANSI(lines[start])
}

func latencyOnLine(line string) int {
	fields := strings.Fields(line)
	for _, field := range fields {
		if !strings.HasSuffix(field, "ms") {
			continue
		}
		n := 0
		for _, r := range strings.TrimSuffix(field, "ms") {
			if r < '0' || r > '9' {
				n = 0
				break
			}
			n = n*10 + int(r-'0')
		}
		if n > 0 {
			return n
		}
	}
	return 0
}

// 详情已经打开时再点另一行，选中和详情都要换成画面上那一行。
func TestClickWhileDetailOpenSelectsVisibleRow(t *testing.T) {
	model := testingModel(t, 40)
	model.windowHeight = 16
	model.updateTableLayout()
	opened := clickRow(model, 0)
	cur := opened
	for i := 0; i < 12; i++ {
		next, _ := cur.Update(tea.KeyMsg{Type: tea.KeyDown})
		cur = next.(tuiModel)
	}
	if !cur.detailVisible {
		t.Fatal("方向键不应关掉详情")
	}
	shown := latencyOnLine(firstDataLine(cur))
	clicked := clickRow(cur, 0)
	if clicked.detailResult == nil || clicked.detailResult.Latency.Milliseconds() != int64(shown) {
		got := time.Duration(0)
		if clicked.detailResult != nil {
			got = clicked.detailResult.Latency
		}
		t.Fatalf("详情开着时点第一行：画面 %dms，详情 %s", shown, got)
	}
}

// 内部偏移已经不是 0 时，最后一行、在测行、表头和空白都不能点错。
func TestClickEdgesAfterScroll(t *testing.T) {
	model := testingModel(t, 40)
	model.windowHeight = 16
	model.updateTableLayout()
	cur := model
	for i := 0; i < 20; i++ {
		next, _ := cur.Update(tea.KeyMsg{Type: tea.KeyDown})
		cur = next.(tuiModel)
	}
	if tableYOffset(cur.table) == 0 {
		t.Fatal("这组按键应让表格内部出现纵向偏移，否则测不到点上一行")
	}

	lastVisual := cur.table.Height() - 1
	lastLine := dataLine(cur, lastVisual)
	lastShown := latencyOnLine(lastLine)
	clickedLast := clickRow(cur, lastVisual)
	if lastShown > 0 {
		if clickedLast.detailResult == nil || clickedLast.detailResult.Latency.Milliseconds() != int64(lastShown) {
			t.Fatalf("最后一行画面 %dms，点开 %v", lastShown, clickedLast.detailResult)
		}
	} else if !strings.Contains(lastLine, "inflight") || clickedLast.detailInFlight == nil || clickedLast.detailInFlight.name != "inflight" {
		t.Fatalf("最后一行应打开在测详情: %q inFlight=%v", lastLine, clickedLast.detailInFlight)
	}

	// 表头仍然只排序，不打开详情。
	header, _ := cur.Update(tea.MouseMsg{
		X: 1, Y: cur.tableHeaderY(),
		Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft,
	})
	if header.(tuiModel).detailVisible {
		t.Fatal("点表头不应打开详情")
	}

	// 数据区下面的空白不选中、不打开详情。
	blankY := cur.tableHeaderY() + tableHeaderLines + cur.table.Height()
	blank, _ := cur.Update(tea.MouseMsg{
		X: 1, Y: blankY,
		Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft,
	})
	blanked := blank.(tuiModel)
	if blanked.detailVisible || blanked.table.Cursor() != cur.table.Cursor() {
		t.Fatalf("点空白不应改选中或打开详情: cursor %d->%d detail=%v", cur.table.Cursor(), blanked.table.Cursor(), blanked.detailVisible)
	}
}

// 拖到后面再点画面上的行，应选中那一行，而不是拖之前的选中行。
func TestClickAfterScrollbarDragSelectsVisibleRow(t *testing.T) {
	model := testingModel(t, 40)
	model.windowHeight = 16
	model.updateTableLayout()
	startY := model.tableHeaderY() + dataRowOffset(model.table.View())
	bottom := startY + model.table.Height() - 1
	clicked, _ := model.Update(tea.MouseMsg{
		X: model.windowWidth - 1, Y: bottom,
		Button: tea.MouseButtonLeft, Action: tea.MouseActionPress,
	})
	scrolled := clicked.(tuiModel)
	if scrolled.table.Cursor() != 0 || scrolled.followSelection {
		t.Fatalf("拖滚动条后选中应停在原处: cursor=%d follow=%v", scrolled.table.Cursor(), scrolled.followSelection)
	}
	shown := latencyOnLine(firstDataLine(scrolled))
	if shown == 100 {
		t.Fatalf("视口没有滚开: %s", firstDataLine(scrolled))
	}
	picked := clickRow(scrolled, 0)
	if picked.detailResult == nil || picked.detailResult.Latency.Milliseconds() != int64(shown) {
		t.Fatalf("滚动后点击：画面 %dms，详情 %v", shown, picked.detailResult)
	}
	if !picked.followSelection {
		t.Fatal("点某一行后应恢复选中跟随")
	}
}

// 按键把选中行移走之后，再拖滚动条，选中行保持不动，画面跟着滑块走。
func TestDragScrollbarKeepsMovedSelection(t *testing.T) {
	model := testingModel(t, 40)
	model.windowHeight = 16
	model.updateTableLayout()
	cur := model
	for i := 0; i < 3; i++ {
		next, _ := cur.Update(tea.KeyMsg{Type: tea.KeyDown})
		cur = next.(tuiModel)
	}
	if cur.table.Cursor() != 3 {
		t.Fatalf("方向键应停在第 4 行: %d", cur.table.Cursor())
	}
	startY := cur.tableHeaderY() + dataRowOffset(cur.table.View())
	bottom := startY + cur.table.Height() - 1
	dragged, _ := cur.Update(tea.MouseMsg{
		X: cur.windowWidth - 1, Y: bottom,
		Button: tea.MouseButtonLeft, Action: tea.MouseActionPress,
	})
	after := dragged.(tuiModel)
	if after.table.Cursor() != 3 {
		t.Fatalf("拖滚动条把选中行从 3 改成了 %d", after.table.Cursor())
	}
	if after.detailVisible {
		t.Fatal("拖滚动条不应打开详情")
	}
	if latencyOnLine(firstDataLine(after)) == 100 {
		t.Fatal("拖到底后画面仍停在第一行")
	}
	// 松手后按方向键，选中继续从刚才那一行走，而不是从视口顶走。
	released, _ := after.Update(tea.MouseMsg{
		X: cur.windowWidth - 1, Y: bottom,
		Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease,
	})
	down, _ := released.(tuiModel).Update(tea.KeyMsg{Type: tea.KeyDown})
	if down.(tuiModel).table.Cursor() != 4 {
		t.Fatalf("松手后方向键应从原选中行继续: %d", down.(tuiModel).table.Cursor())
	}
}

// 进度刷新把在测行变灰时，不能把已经滚到下面的完成行也涂灰。
func TestGrayOnlyInFlightWhenScrolled(t *testing.T) {
	model := testingModel(t, 40)
	model.windowHeight = 16
	model.updateTableLayout()
	startY := model.tableHeaderY() + dataRowOffset(model.table.View())
	bottom := startY + model.table.Height() - 1
	clicked, _ := model.Update(tea.MouseMsg{
		X: model.windowWidth - 1, Y: bottom,
		Button: tea.MouseButtonLeft, Action: tea.MouseActionPress,
	})
	scrolled := clicked.(tuiModel)
	view := scrolled.View()
	lines := strings.Split(view, "\n")
	dataStart := scrolled.tableHeaderY() + dataRowOffset(scrolled.table.View())
	for i := 0; i < scrolled.table.Height() && dataStart+i < len(lines); i++ {
		line := lines[dataStart+i]
		plain := stripANSI(line)
		if strings.TrimSpace(plain) == "" {
			continue
		}
		if strings.Contains(plain, "inflight") {
			if !strings.Contains(line, "\x1b[2m") {
				t.Fatal("在测行应是灰色")
			}
			continue
		}
		if strings.Contains(line, "\x1b[2m") {
			t.Fatalf("完成行被涂灰了: %s", plain)
		}
	}
}

func dataLine(model tuiModel, visualRow int) string {
	lines := strings.Split(model.View(), "\n")
	start := model.tableHeaderY() + dataRowOffset(model.table.View()) + visualRow
	if start < 0 || start >= len(lines) {
		return ""
	}
	return stripANSI(lines[start])
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
