package gui

import (
	"fmt"
	"strings"
	"testing"
	"time"

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
	if hasHitID(snapshot, application.ids.entryCollapse) {
		t.Fatal("折叠后不应再显示折叠按钮")
	}
	if !hasText(snapshot, "»") {
		t.Fatalf("折叠后应显示展开按钮；文本=%v", snapshot.Labels)
	}
	if !hasHitID(snapshot, application.ids.entryExpand) {
		t.Fatal("折叠后应保留展开按钮的命中")
	}

	clickID(t, driver, application.ids.entryExpand)
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

func countHitID(snapshot app.Snapshot, id ui.ID) int {
	count := 0
	for _, hit := range snapshot.Hits {
		if hit.ID == id {
			count++
		}
	}
	return count
}

// 组标题的折叠按钮只隐藏节点行，不改变勾选与测速范围。
func TestGroupHeaderCollapsesRows(t *testing.T) {
	dir := t.TempDir()
	writeNodesFile(t, dir, "a.yaml", "A-01", "A-02", "A-03")
	seedEntry(t, dir, store.Entry{ID: "e1", Name: "机场 A", Path: "a.yaml"})
	application, driver := openTestApp(t, dir, newFakeRunner(nil))
	entry := application.entries[0]
	node := application.nodes[0]

	if !hasHitID(driver.Snapshot(), node.ids.row) {
		t.Fatal("折叠前应有节点行")
	}
	clickID(t, driver, entry.ids.groupToggle)
	if !entry.collapsed {
		t.Fatal("点组标题箭头后应进入折叠状态")
	}
	snapshot := driver.Snapshot()
	if hasHitID(snapshot, node.ids.row) {
		t.Fatal("折叠后不应再绘制节点行")
	}
	if !hasText(snapshot, "已折叠") {
		t.Fatalf("折叠后组标题应标明已折叠；文本=%v", snapshot.Labels)
	}
	if !hasHitID(snapshot, entry.ids.groupToggle) {
		t.Fatal("折叠后仍应能从组标题展开")
	}
	if application.selectedCount != 3 {
		t.Fatalf("折叠不应改变勾选，实际已选 %d", application.selectedCount)
	}
	clickID(t, driver, entry.ids.groupToggle)
	if entry.collapsed {
		t.Fatal("再点一次应展开")
	}
	if !hasHitID(driver.Snapshot(), node.ids.row) {
		t.Fatal("展开后节点行应回来")
	}
}

// 组标题滚出视口后固定在列表顶部，滚到下一个条目时自动换成下一组的标题。
func TestStickyGroupHeaderTracksScroll(t *testing.T) {
	dir := t.TempDir()
	namesA := make([]string, 0, 30)
	namesB := make([]string, 0, 30)
	for index := 0; index < 30; index++ {
		namesA = append(namesA, fmt.Sprintf("A-%02d", index))
		namesB = append(namesB, fmt.Sprintf("B-%02d", index))
	}
	writeNodesFile(t, dir, "a.yaml", namesA...)
	writeNodesFile(t, dir, "b.yaml", namesB...)
	seedEntry(t, dir, store.Entry{ID: "e1", Name: "机场 A", Path: "a.yaml"})
	seedEntry(t, dir, store.Entry{ID: "e2", Name: "机场 B", Path: "b.yaml"})
	application, driver := openTestApp(t, dir, newFakeRunner(nil))
	viewport := application.nodeViewport
	if viewport.Height <= 0 {
		t.Fatal("没有记录列表视口")
	}
	if err := driver.Scroll(ui.ScrollEvent{X: viewport.X + viewport.Width/2, Y: viewport.Y + viewport.Height/2, DeltaY: 200, Precise: true}); err != nil {
		t.Fatal(err)
	}
	snapshot := driver.Snapshot()
	viewport = application.nodeViewport
	sticky := hitRect(t, driver, application.entries[0].ids.selectAll)
	// 按钮在 32px 标题内垂直居中，命中顶部比列表顶多几个像素。
	if delta := sticky.Y - viewport.Y; delta < -1.5 || delta > 6 {
		t.Fatalf("第一组标题应固定在列表顶部：hit.Y=%v viewport.Y=%v", sticky.Y, viewport.Y)
	}
	if got := countHitID(snapshot, application.entries[0].ids.selectAll); got != 1 {
		t.Fatalf("固定标题只应出现一次，实际 %d 次", got)
	}

	if err := driver.Scroll(ui.ScrollEvent{X: viewport.X + viewport.Width/2, Y: viewport.Y + viewport.Height/2, DeltaY: 5000, Precise: true}); err != nil {
		t.Fatal(err)
	}
	snapshot = driver.Snapshot()
	viewport = application.nodeViewport
	second := hitRect(t, driver, application.entries[1].ids.selectAll)
	if delta := second.Y - viewport.Y; delta < -1.5 || delta > 6 {
		t.Fatalf("滚到第二组后应固定第二组标题：hit.Y=%v viewport.Y=%v", second.Y, viewport.Y)
	}
	if hasHitID(snapshot, application.entries[0].ids.selectAll) {
		t.Fatal("第一组标题已滚出视口，不应再出现")
	}

	clickID(t, driver, application.entries[1].ids.selectNone)
	selected := 0
	for _, node := range application.entries[1].nodes {
		if node.selected {
			selected++
		}
	}
	if selected != 0 {
		t.Fatalf("固定标题上的「全不选」应生效，实际还有 %d 个选中", selected)
	}
}

// 条目卡片整卡可点：点空白处切换勾选，节点列表随之出现/消失。
func TestEntryCardClickTogglesSelection(t *testing.T) {
	dir := t.TempDir()
	writeNodesFile(t, dir, "a.yaml", "A-01", "A-02")
	seedEntry(t, dir, store.Entry{ID: "e1", Name: "机场 A", Path: "a.yaml"})
	application, driver := openTestApp(t, dir, newFakeRunner(nil))
	entry := application.entries[0]
	node := application.nodes[0]
	if !entry.selected {
		t.Fatal("条目默认应选中")
	}
	clickID(t, driver, entry.ids.card)
	if entry.selected {
		t.Fatal("点卡片应取消条目勾选")
	}
	if hasHitID(driver.Snapshot(), node.ids.row) {
		t.Fatal("条目取消勾选后节点行应消失")
	}
	clickID(t, driver, entry.ids.card)
	if !entry.selected {
		t.Fatal("再点卡片应重新勾选")
	}
	if !hasHitID(driver.Snapshot(), node.ids.row) {
		t.Fatal("重新勾选后节点行应回来")
	}
}

// 右键「立即测试所选节点」也必须遵守当前过滤：隐藏的节点不能被测速。
func TestContextMenuTestRespectsFilter(t *testing.T) {
	dir := t.TempDir()
	writeNodesFile(t, dir, "a.yaml", "A-HK", "A-JP")
	seedEntry(t, dir, store.Entry{ID: "e1", Name: "机场 A", Path: "a.yaml"})
	runner := newFakeRunner(nil)
	application, driver := openTestApp(t, dir, runner)
	application.filterField.SetText("HK")
	application.compileFilter()
	all := append([]*nodeState(nil), application.nodes...)
	application.startNodesTest(all)
	waitUntilIdle(t, application, driver)
	runner.mu.Lock()
	batch := append([]RunNode(nil), runner.batch...)
	runner.mu.Unlock()
	if len(batch) != 1 || batch[0].Proxy == nil || batch[0].Proxy.Name() != "A-HK" {
		t.Fatalf("过滤后只应测 A-HK，实际 %v", batchNames(batch))
	}
}

func batchNames(batch []RunNode) []string {
	names := make([]string, 0, len(batch))
	for _, node := range batch {
		if node.Proxy != nil {
			names = append(names, node.Proxy.Name())
		}
	}
	return names
}

// 详情面板要跟着结果刷新，而不是停在打开时的快照。
func TestDetailPanelRefreshesAfterRun(t *testing.T) {
	dir := t.TempDir()
	writeNodesFile(t, dir, "a.yaml", "A-01")
	seedEntry(t, dir, store.Entry{ID: "e1", Name: "机场 A", Path: "a.yaml"})
	runner := newFakeRunner(map[string]fakeResult{"A-01": {ok: true, latency: 42 * time.Millisecond, speed: 8 * 1024 * 1024}})
	application, driver := openTestApp(t, dir, runner)
	clickID(t, driver, application.ids.startTest)
	waitUntilIdle(t, application, driver)
	node := application.nodes[0]
	application.toggleDetail(node)
	if strings.Contains(application.detailText, "尚未测试") || !strings.Contains(application.detailText, "42ms") {
		t.Fatalf("详情应显示已完成的结果：%s", application.detailText)
	}
	// 结果再次更新（例如后面又测了一次）时，打开着的详情也要跟着刷新。
	node.result.LatencyMS = 99
	application.refreshNodeDisplay(node)
	if !strings.Contains(application.detailText, "99ms") {
		t.Fatalf("详情没有跟随结果刷新：%s", application.detailText)
	}
}

// 节点被过滤掉后，详情与固定行要一并清掉，不能留下幽灵详情。
func TestHiddenNodesClearDetailAndPin(t *testing.T) {
	dir := t.TempDir()
	writeNodesFile(t, dir, "a.yaml", "A-HK", "A-JP")
	seedEntry(t, dir, store.Entry{ID: "e1", Name: "机场 A", Path: "a.yaml"})
	application, _ := openTestApp(t, dir, newFakeRunner(nil))
	node := application.nodes[0]
	application.toggleDetail(node)
	application.pinNode = node
	application.filterField.SetText("JP")
	application.compileFilter()
	if application.detailNode != nil || application.pinNode != nil {
		t.Fatal("节点被过滤掉后详情与固定行都应清空")
	}
}

// 行的命中区在滚动条轨道左侧结束，右缘空白不会吃掉行的左键/右键。
func TestRowHitsStopBeforeScrollBar(t *testing.T) {
	dir := t.TempDir()
	names := make([]string, 0, 30)
	for index := 0; index < 30; index++ {
		names = append(names, fmt.Sprintf("A-%02d", index))
	}
	writeNodesFile(t, dir, "a.yaml", names...)
	seedEntry(t, dir, store.Entry{ID: "e1", Name: "机场 A", Path: "a.yaml"})
	application, driver := openTestApp(t, dir, newFakeRunner(nil))
	node := application.nodes[0]
	row := hitRect(t, driver, node.ids.row)
	bar := hitRect(t, driver, application.ids.nodeBar)
	if row.X+row.Width > bar.X+0.5 {
		t.Fatalf("行命中右缘 %v 不应盖住滚动条轨道（左缘 %v）", row.X+row.Width, bar.X)
	}
}
