package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/faceair/clash-speedtest/speedtester"
)

// headerClick 在画面表头行的指定列上模拟一次点击。
func headerClick(model tuiModel, x int) tuiModel {
	y := model.tableHeaderY()
	updated, _ := model.Update(tea.MouseMsg{
		X: x, Y: y,
		Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft,
	})
	return updated.(tuiModel)
}

// dataClick 点击画面正文区的第 visualRow 行（0 起）。
func dataClick(model tuiModel, visualRow int) tuiModel {
	y := model.tableHeaderY() + tableHeaderLines + visualRow
	updated, _ := model.Update(tea.MouseMsg{
		X: 1, Y: y,
		Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft,
	})
	return updated.(tuiModel)
}

// 跟随最新条目时视口贴底，表头虽然一直可见，但点完排序画面还停在底部，
// 用户看不见排序结果，就像点击没反应。点表头必须取消跟随并回到顶部。
func TestHeaderClickSortsWhileFollowing(t *testing.T) {
	model := testingModel(t, 40)
	model.windowHeight = 16
	model.updateTableLayout()

	got := headerClick(model, 40) // 节点名称列
	if got.sortColumn == 0 && model.sortColumn == 0 {
		t.Fatalf("点表头应切换排序列: %d", got.sortColumn)
	}
	if got.followTail {
		t.Fatal("点表头排序应取消跟随，否则看不到排序结果")
	}
	if start := got.viewportStart(); start != 0 {
		t.Fatalf("点表头后应回顶部显示排序结果: start=%d", start)
	}
	// 排序确实改变了行序。
	if got.View() == model.View() {
		t.Fatal("排序后画面应有变化")
	}
}

// 取消跟随后点表头：排序照常生效。
func TestHeaderClickSortsAfterManualScroll(t *testing.T) {
	model := testingModel(t, 40)
	model.windowHeight = 16
	model.updateTableLayout()
	model.followTail = false

	before := model.sortColumn
	got := headerClick(model, 40)
	if got.sortColumn == before && got.sortAscending == model.sortAscending {
		t.Fatalf("点表头应切换排序: column %d->%d", before, got.sortColumn)
	}
}

// 跟随时点击画面上的行：必须打开画面上那一行的详情，不是数组里别的那条。
func TestDetailOpensClickedRowWhileFollowing(t *testing.T) {
	model := testingModel(t, 40)
	model.windowHeight = 16
	model.updateTableLayout()
	// 跟随时视口贴底：画面第 0 行对应绝对行 viewportStart()。
	want := model.viewportStart()
	if want <= 0 {
		t.Fatalf("贴底视口起点应为正: %d", want)
	}

	got := dataClick(model, 0)
	if !got.detailVisible {
		t.Fatal("点击应打开详情")
	}
	if got.selectedIndex != want {
		t.Fatalf("详情应是画面那一行: selectedIndex=%d want=%d", got.selectedIndex, want)
	}
}

// 详情面板开着时点表头排序：选中节点不该变成数组里另一条——
// 排序重排了 m.results，按名字把选中行跟过去。
func TestHeaderClickWhileDetailOpenKeepsSelection(t *testing.T) {
	model := testingModel(t, 40)
	model.windowHeight = 24
	model.updateTableLayout()
	opened := dataClick(model, 0)
	if !opened.detailVisible {
		t.Fatal("点击应打开详情")
	}
	selectedName := opened.results[opened.selectedIndex].ProxyName

	got := headerClick(opened, 40) // 切换排序，results 数组重排
	if !got.detailVisible {
		t.Fatal("排序后详情应保持打开")
	}
	if got.selectedIndex < 0 || got.selectedIndex >= len(got.results) {
		t.Fatalf("排序后选中越界: %d", got.selectedIndex)
	}
	if got.results[got.selectedIndex].ProxyName != selectedName {
		t.Fatalf("排序后详情内容突变: 选中节点从 %q 变成 %q",
			selectedName, got.results[got.selectedIndex].ProxyName)
	}
}

// 排序之后点击行：详情跟随画面序号，不受排序影响。
func TestDetailAfterSortOpensClickedRow(t *testing.T) {
	model := testingModel(t, 40)
	model.windowHeight = 24
	model.updateTableLayout()
	model.followTail = false
	sorted := headerClick(model, 40)

	got := dataClick(sorted, 0)
	if !got.detailVisible {
		t.Fatal("点击应打开详情")
	}
	if got.selectedIndex != 0 {
		t.Fatalf("排序后点画面第一行应是第一条: %d", got.selectedIndex)
	}
}

// 测试中点在测行：打开的是在测详情，不是已完成行。
func TestClickInFlightRowOpensInFlightDetail(t *testing.T) {
	model := testingModel(t, 3)
	model.windowHeight = 24
	model.updateTableLayout()

	// 在测行垫底：完整数据只有 4 行，全可见，在测行是画面第 3 行。
	got := dataClick(model, len(model.results))
	if !got.detailVisible {
		t.Fatal("点击在测行应打开详情")
	}
	if !strings.Contains(got.View(), "测试中") {
		t.Fatal("在测详情应标明仍在测试中")
	}
}

var _ = speedtester.SpeedModeFull

// 点表头后列宽口径不能膨胀：任何一行的显示宽度超过窗口宽时，
// 视觉行坐标会整体错一倍，所有点击定位全乱。逐列点一遍锁住。
func TestHeaderClickKeepsRowsWithinWindow(t *testing.T) {
	model := testingModel(t, 40)
	model.windowHeight = 16
	model.updateTableLayout()

	clicked := model
	for x := 4; x < model.windowWidth; x += 16 {
		clicked = headerClick(clicked, x)
	}
	for i, line := range strings.Split(clicked.View(), "\n") {
		if w := lipgloss.Width(line); w > clicked.windowWidth {
			t.Fatalf("第 %d 行超宽 %d > %d", i, w, clicked.windowWidth)
		}
	}
}
