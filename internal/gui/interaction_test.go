package gui

import (
	"fmt"
	"testing"

	"github.com/BoxMiao007/clash-speedtest-plus/internal/filter"
	"github.com/BoxMiao007/clash-speedtest-plus/internal/store"
	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/ui"
)

// hitRect 找指定身份的命中矩形；找不到直接失败，方便断言控件确实画出来了。
func hitRect(t *testing.T, driver *app.Driver, id ui.ID) ui.Rect {
	t.Helper()
	for _, hit := range driver.Snapshot().Hits {
		if hit.ID == id {
			return hit.Bounds
		}
	}
	t.Fatalf("找不到 ID %v；当前命中：%v", id, hitLabels(driver.Snapshot()))
	return ui.Rect{}
}

// drag 用按下、移动、抬起模拟一次拖动，覆盖 Viva 的指针捕获路径。
func drag(t *testing.T, driver *app.Driver, startX, startY, endX, endY float32) {
	t.Helper()
	if err := driver.Press(startX, startY, ui.PointerButtonPrimary); err != nil {
		t.Fatal(err)
	}
	if err := driver.Move((startX+endX)/2, (startY+endY)/2); err != nil {
		t.Fatal(err)
	}
	if err := driver.Move(endX, endY); err != nil {
		t.Fatal(err)
	}
	if err := driver.Release(endX, endY, ui.PointerButtonPrimary); err != nil {
		t.Fatal(err)
	}
}

func TestEntriesPanelCollapsesAndResizesByDrag(t *testing.T) {
	dir := t.TempDir()
	writeNodesFile(t, dir, "a.yaml", "HK-01")
	seedEntry(t, dir, store.Entry{ID: "e1", Name: "机场 A", Path: "a.yaml"})
	application, driver := openTestApp(t, dir, newFakeRunner(nil))

	if application.entriesSplit.Pos != entriesPanelWidth {
		t.Fatalf("初始宽度 = %v，期望 %v", application.entriesSplit.Pos, entriesPanelWidth)
	}
	before := application.entryScrollBox.Rect()

	clickLabel(t, driver, "折叠")
	if !application.entriesCollapsed {
		t.Fatal("点「折叠」后应进入折叠状态")
	}
	snapshot := driver.Snapshot()
	if hasText(snapshot, "节点来源") {
		t.Fatal("折叠后不应再显示面板标题")
	}
	if !hasText(snapshot, "展开") {
		t.Fatalf("折叠后应显示展开入口；文本=%v", snapshot.Labels)
	}

	clickLabel(t, driver, "展开")
	if application.entriesCollapsed {
		t.Fatal("点「展开」后应恢复面板")
	}
	if !hasText(driver.Snapshot(), "节点来源") {
		t.Fatal("展开后应重新显示面板标题")
	}

	splitter := hitRect(t, driver, application.ids.entrySplitter)
	startX := splitter.X + splitter.Width/2
	startY := splitter.Y + 100
	drag(t, driver, startX, startY, startX+120, startY)
	got := application.entriesSplit.Pos
	if got < entriesPanelWidth+100 || got > entriesPanelWidth+140 {
		t.Fatalf("拖动后宽度 = %v，期望约 %v", got, entriesPanelWidth+120)
	}
	after := application.entryScrollBox.Rect()
	if after.Width <= before.Width {
		t.Fatalf("拖动后节点来源区域应更宽：before=%v after=%v", before, after)
	}
}

func TestScrollbarsFollowOverflow(t *testing.T) {
	dir := t.TempDir()
	names := make([]string, 0, 60)
	for index := 0; index < 60; index++ {
		names = append(names, fmt.Sprintf("节点 %03d", index))
	}
	writeNodesFile(t, dir, "a.yaml", names...)
	for index := 0; index < 8; index++ {
		seedEntry(t, dir, store.Entry{ID: fmt.Sprintf("e%d", index), Name: fmt.Sprintf("机场 %d", index), Path: "a.yaml"})
	}
	application, driver := openTestApp(t, dir, newFakeRunner(nil))

	if !hasHitID(driver.Snapshot(), application.ids.entryBar) {
		t.Fatal("节点来源内容溢出时应显示滚动条")
	}
	if !hasHitID(driver.Snapshot(), application.ids.nodeBar) {
		t.Fatal("节点列表内容溢出时应显示滚动条")
	}

	clickLabel(t, driver, "设置")
	if !hasHitID(driver.Snapshot(), application.ids.settingsBar) {
		t.Fatal("设置内容溢出时应显示滚动条")
	}

	clickLabel(t, driver, "测速")
	clickID(t, driver, application.ids.startTest)
	waitUntilIdle(t, application, driver)
	if application.active != viewTest {
		t.Fatal("开测后应留在合并的测速视图")
	}
	if !hasHitID(driver.Snapshot(), application.ids.nodeBar) {
		t.Fatal("同一张表在测完后仍应保留滚动条")
	}
	if !hasText(driver.Snapshot(), "已完成") {
		t.Fatalf("合并视图应显示结果进度；文本=%v", driver.Snapshot().Labels)
	}
}

func TestScrollbarsHideWhenContentFits(t *testing.T) {
	dir := t.TempDir()
	writeNodesFile(t, dir, "a.yaml", "HK-01", "HK-02")
	seedEntry(t, dir, store.Entry{ID: "e1", Name: "机场 A", Path: "a.yaml"})
	application, driver := openTestApp(t, dir, newFakeRunner(nil))

	snapshot := driver.Snapshot()
	if hasHitID(snapshot, application.ids.nodeBar) {
		t.Fatal("节点没有溢出时不应画滚动条")
	}
	if hasHitID(snapshot, application.ids.entryBar) {
		t.Fatal("条目没有溢出时不应画滚动条")
	}
}

func TestScrollBarDragScrollsNodeList(t *testing.T) {
	dir := t.TempDir()
	names := make([]string, 0, 60)
	for index := 0; index < 60; index++ {
		names = append(names, fmt.Sprintf("节点 %03d", index))
	}
	writeNodesFile(t, dir, "a.yaml", names...)
	seedEntry(t, dir, store.Entry{ID: "e1", Name: "机场 A", Path: "a.yaml"})
	application, driver := openTestApp(t, dir, newFakeRunner(nil))

	bar := hitRect(t, driver, application.ids.nodeBar)
	if bar.Width <= 0 || bar.Height <= 0 {
		t.Fatalf("滚动条矩形无效：%v", bar)
	}
	if application.nodeOffset != 0 {
		t.Fatalf("初始偏移应为 0，实际 %v", application.nodeOffset)
	}
	centerX, centerY := bar.X+bar.Width/2, bar.Y+bar.Height/2
	if err := driver.Press(centerX, centerY, ui.PointerButtonPrimary); err != nil {
		t.Fatal(err)
	}
	if application.nodeOffset <= 0 {
		t.Fatal("按下轨道应把列表滚到指针位置")
	}
	first := application.nodeOffset
	if err := driver.Move(centerX, bar.Y+bar.Height+40); err != nil {
		t.Fatal(err)
	}
	maximum := application.nodeContent - application.nodeViewport.Height
	if maximum < 0 {
		maximum = 0
	}
	if application.nodeOffset != maximum {
		t.Fatalf("拖到轨道底部应滚到最大偏移 %v，实际 %v", maximum, application.nodeOffset)
	}
	if application.nodeOffset <= first {
		t.Fatalf("向下拖动滑块应继续滚动：first=%v now=%v", first, application.nodeOffset)
	}
	if err := driver.Release(centerX, bar.Y+bar.Height+40, ui.PointerButtonPrimary); err != nil {
		t.Fatal(err)
	}
	if !hasText(driver.Snapshot(), "节点 059") {
		t.Fatalf("拖到底部应看到末尾节点；文本=%v", driver.Snapshot().Labels)
	}
}

func TestScrollAreaBarDragScrollsSettings(t *testing.T) {
	dir := t.TempDir()
	seedEntry(t, dir, store.Entry{ID: "e1", Name: "机场 A"})
	application, driver := openTestApp(t, dir, newFakeRunner(nil))
	clickLabel(t, driver, "设置")

	bar := hitRect(t, driver, application.ids.settingsBar)
	if bar.Width <= 0 || bar.Height <= 0 {
		t.Fatalf("设置滚动条矩形无效：%v", bar)
	}
	centerX, centerY := bar.X+bar.Width/2, bar.Y+bar.Height/2
	if err := driver.Press(centerX, centerY, ui.PointerButtonPrimary); err != nil {
		t.Fatal(err)
	}
	state := application.view.ScrollState(application.ids.settingsScroll)
	if state.Offset <= 0 {
		t.Fatalf("按下设置滚动条后偏移应大于 0，实际 %v", state.Offset)
	}
	if err := driver.Release(centerX, centerY, ui.PointerButtonPrimary); err != nil {
		t.Fatal(err)
	}
}

func TestFilterCombineLabelsExplainMeaning(t *testing.T) {
	dir := t.TempDir()
	writeNodesFile(t, dir, "a.yaml", "香港 01")
	seedEntry(t, dir, store.Entry{ID: "e1", Name: "机场 A", Path: "a.yaml"})
	application, driver := openTestApp(t, dir, newFakeRunner(nil))

	snapshot := driver.Snapshot()
	if !hasText(snapshot, "全部命中") || !hasText(snapshot, "每个关键词都要出现") {
		t.Fatalf("「全部命中」应带说明文字；文本=%v", snapshot.Labels)
	}
	clickID(t, driver, application.ids.combScope.At(1))
	if application.filterCombine != filter.CombineOr {
		t.Fatalf("点「任一命中」后组合方式 = %v，期望 or", application.filterCombine)
	}
	if !hasText(driver.Snapshot(), "任一关键词出现即可") {
		t.Fatal("「任一命中」应换成对应说明")
	}
}

func TestOutputSwitchesExplainEachProduct(t *testing.T) {
	dir := t.TempDir()
	seedEntry(t, dir, store.Entry{ID: "e1", Name: "机场 A"})
	application, driver := openTestApp(t, dir, newFakeRunner(nil))
	clickLabel(t, driver, "设置")

	// 设置内容很长，先滚到底再找输出开关。
	viewport := application.settingsScrollBox.Rect()
	if err := driver.Scroll(ui.ScrollEvent{
		X: viewport.X + viewport.Width/2, Y: viewport.Y + viewport.Height/2,
		DeltaY: 4000, Precise: true,
	}); err != nil {
		t.Fatal(err)
	}
	snapshot := driver.Snapshot()
	for _, want := range []string{
		"每轮自动保存结果图（PNG）",
		"结果图只保留有速度的节点",
		"导出配置时按 IP 归属地重命名节点",
		"测完在输出目录生成结果图",
	} {
		if !hasText(snapshot, want) {
			t.Fatalf("输出开关缺少说明 %q；文本=%v", want, snapshot.Labels)
		}
	}

	if !application.settingsForm.imageEnabled {
		t.Fatal("默认应开启结果图")
	}
	clickLabel(t, driver, "每轮自动保存结果图（PNG）")
	if application.settingsForm.imageEnabled {
		t.Fatal("点开关后应关闭结果图")
	}
}
