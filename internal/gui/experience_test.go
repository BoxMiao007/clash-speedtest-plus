package gui

import (
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/BoxMiao007/clash-speedtest-plus/internal/store"
	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/ui"
)

// TestSelectAllNoneCoversAllGroups 验证工具条的全选/全不选作用于所有条目的
// 所有节点，而不是只看当前展开的分组。
func TestSelectAllNoneCoversAllGroups(t *testing.T) {
	dir := t.TempDir()
	writeNodesFile(t, dir, "a.yaml", "HK-01", "HK-02")
	writeNodesFile(t, dir, "b.yaml", "JP-01", "JP-02")
	seedEntry(t, dir, store.Entry{ID: "e1", Name: "机场 A", Path: "a.yaml"})
	seedEntry(t, dir, store.Entry{ID: "e2", Name: "机场 B", Path: "b.yaml"})
	application, driver := openTestApp(t, dir, newFakeRunner(nil))

	if application.selectedCount != 4 || application.totalCount != 4 {
		t.Fatalf("初始应全选 4 个节点，实际 %d/%d", application.selectedCount, application.totalCount)
	}
	clickID(t, driver, application.nodes[0].ids.check)
	clickID(t, driver, application.nodes[2].ids.check)
	if application.selectedCount != 2 {
		t.Fatalf("取消两个勾选后应剩 2 个，实际 %d", application.selectedCount)
	}

	clickID(t, driver, application.ids.selectAll)
	if application.selectedCount != 4 {
		t.Fatalf("全选后应选满 4 个，实际 %d", application.selectedCount)
	}
	for _, node := range application.nodes {
		if !node.selected {
			t.Fatalf("节点 %q 未被全选", node.name)
		}
	}

	clickID(t, driver, application.ids.selectNone)
	if application.selectedCount != 0 {
		t.Fatalf("全不选后应剩 0 个，实际 %d", application.selectedCount)
	}
}

// TestClearResultsWipesStoreAndRows 验证「清空结果」同时清内存与磁盘，但
// 保留条目与节点。
func TestClearResultsWipesStoreAndRows(t *testing.T) {
	dir := t.TempDir()
	writeNodesFile(t, dir, "a.yaml", "HK-01")
	seedEntry(t, dir, store.Entry{ID: "e1", Name: "机场 A", Path: "a.yaml"})
	runner := newFakeRunner(map[string]fakeResult{
		"HK-01": {ok: true, latency: 20 * time.Millisecond, speed: 8 * 1024 * 1024},
	})
	application, driver := openTestApp(t, dir, runner)
	// 关闭结果图自动导出，避免清空时还有后台导出在跑。
	application.settings.ImageEnabled = false
	clickID(t, driver, application.ids.startTest)
	waitUntilIdle(t, application, driver)
	if len(application.results) != 1 {
		t.Fatalf("应有 1 条结果，实际 %d", len(application.results))
	}
	node := application.nodes[0]

	clickID(t, driver, application.ids.clearResults)
	if application.dialog != dialogConfirm {
		t.Fatalf("清空结果应先弹确认框，实际 %v", application.dialog)
	}
	clickID(t, driver, application.ids.confirmOK)
	if application.dialog != dialogNone {
		t.Fatalf("确认后对话框应关闭，实际 %v", application.dialog)
	}
	if node.hasResult {
		t.Fatal("清空后节点不应保留结果")
	}
	if len(application.results) != 0 || len(application.rounds) != 0 {
		t.Fatalf("清空后结果与轮次都应为空，实际 %d/%d", len(application.results), len(application.rounds))
	}
	database, err := store.Open(store.Options{BaseDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := database.Result("e1", node.id); ok {
		t.Fatal("磁盘上的结果也应被清空")
	}
	if len(database.Entries()) != 1 {
		t.Fatalf("条目应保留，实际 %d", len(database.Entries()))
	}
}

// TestOutputDirectoryBrowseFillsStem 验证设置页的「浏览…」把选中目录写进
// 输出路径，并沿用原来的文件名。
func TestOutputDirectoryBrowseFillsStem(t *testing.T) {
	dir := t.TempDir()
	application, driver := openTestApp(t, dir, newFakeRunner(nil))
	clickLabel(t, driver, "设置")
	// 设置内容很长，先滚到输出路径那一行。
	viewport := application.settingsScrollBox.Rect()
	if err := driver.Scroll(ui.ScrollEvent{
		X: viewport.X + viewport.Width/2, Y: viewport.Y + viewport.Height/2,
		DeltaY: 4000, Precise: true,
	}); err != nil {
		t.Fatal(err)
	}
	clickLabel(t, driver, "输出路径")
	if err := driver.Type("D:/old/导出.yaml"); err != nil {
		t.Fatal(err)
	}
	chosen := filepath.Join(dir, "导出目录")
	if err := os.MkdirAll(chosen, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := driver.QueueOpenDirectory(chosen, nil); err != nil {
		t.Fatal(err)
	}
	clickID(t, driver, application.ids.setOutputBrowse)
	want := filepath.Join(chosen, "导出.yaml")
	if got := application.settingsForm.outputPath.Text(); got != want {
		t.Fatalf("浏览后输出路径应为 %q，实际 %q", want, got)
	}
	if application.settingsForm.err != "" {
		t.Fatalf("浏览不应报错：%s", application.settingsForm.err)
	}
}

// TestDroppedConfigBecomesEntry 验证把 YAML 拖进窗口会新增本地条目并读取节点。
func TestDroppedConfigBecomesEntry(t *testing.T) {
	dir := t.TempDir()
	path := writeNodesFile(t, dir, "拖入.yaml", "HK-01", "HK-02")
	application, driver := openTestApp(t, dir, newFakeRunner(nil))
	if len(application.entries) != 0 {
		t.Fatalf("初始不应有条目，实际 %d", len(application.entries))
	}
	accepted, err := driver.Drop(ui.FileDropEvent{
		Phase: ui.FileDropDrop,
		X:     200,
		Y:     200,
		Paths: []string{path, filepath.Join(dir, "说明.txt")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !accepted {
		t.Fatal("拖放应被接受")
	}
	if len(application.entries) != 1 {
		t.Fatalf("应新增 1 个条目，实际 %d", len(application.entries))
	}
	entry := application.entries[0]
	if entry.entry.Name != "拖入" || !entry.selected {
		t.Fatalf("拖入条目名称/选中不对：%q %v", entry.entry.Name, entry.selected)
	}
	if len(entry.nodes) != 2 {
		t.Fatalf("拖入条目应读取 2 个节点，实际 %d", len(entry.nodes))
	}
	database, err := store.Open(store.Options{BaseDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(database.Entries()) != 1 {
		t.Fatalf("拖入的条目应写盘，实际 %d", len(database.Entries()))
	}
}

// TestAboutDialogShowsShortcutHelp 验证 F1 打开关于对话框，内容含版本、
// 构建提交与快捷键；页脚「关于」按钮同样能打开。
func TestAboutDialogShowsShortcutHelp(t *testing.T) {
	dir := t.TempDir()
	application, driver := openTestApp(t, dir, newFakeRunner(nil))
	application.commit = "f3ca060"
	if err := driver.Key(ui.KeyF1, 0); err != nil {
		t.Fatal(err)
	}
	if application.dialog != dialogAbout {
		t.Fatalf("F1 应打开关于对话框，实际 %v", application.dialog)
	}
	for _, want := range []string{"Ctrl+A", "Ctrl+R", "Ctrl+Enter", "f3ca060"} {
		if !strings.Contains(application.aboutText, want) {
			t.Fatalf("关于内容缺少 %q：%s", want, application.aboutText)
		}
	}
	clickLabel(t, driver, "关闭")
	if application.dialog != dialogNone {
		t.Fatal("关闭后对话框应消失")
	}
	clickID(t, driver, application.ids.about)
	if application.dialog != dialogAbout {
		t.Fatal("页脚「关于」按钮也应打开对话框")
	}
}

// TestWindowGeometryIsRemembered 验证退出时保存窗口位置与尺寸，下次启动
// 按同一几何打开。
func TestWindowGeometryIsRemembered(t *testing.T) {
	dir := t.TempDir()
	application, driver := openTestApp(t, dir, newFakeRunner(nil))
	if err := driver.Resize(1280, 860); err != nil {
		t.Fatal(err)
	}
	if err := driver.MoveWindow(140, 90); err != nil {
		t.Fatal(err)
	}
	application.Close()

	database, err := store.Open(store.Options{BaseDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	saved := database.Settings()
	if !saved.WindowSet || saved.WindowWidth != 1280 || saved.WindowHeight != 860 {
		t.Fatalf("窗口尺寸未保存：%+v", saved)
	}
	if saved.WindowX != 140 || saved.WindowY != 90 {
		t.Fatalf("窗口位置未保存：%+v", saved)
	}

	reopened, err := New(Options{BaseDir: dir, Version: "test", Runner: newFakeRunner(nil)})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	config := reopened.Config()
	if config.Width != 1280 || config.Height != 860 {
		t.Fatalf("重启后窗口尺寸应为 1280×860，实际 %d×%d", config.Width, config.Height)
	}
	if config.Origin == nil || config.Origin.X != 140 || config.Origin.Y != 90 {
		t.Fatalf("重启后窗口位置应为 (140,90)，实际 %+v", config.Origin)
	}
}

// TestSavedSettingsKeepWindowGeometry 验证保存设置不会把窗口几何清掉。
func TestSavedSettingsKeepWindowGeometry(t *testing.T) {
	dir := t.TempDir()
	application, driver := openTestApp(t, dir, newFakeRunner(nil))
	if err := driver.Resize(1200, 800); err != nil {
		t.Fatal(err)
	}
	if err := driver.MoveWindow(60, 40); err != nil {
		t.Fatal(err)
	}
	clickLabel(t, driver, "设置")
	clickID(t, driver, application.ids.settingsSave)
	database, err := store.Open(store.Options{BaseDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	saved := database.Settings()
	if !saved.WindowSet || saved.WindowX != 60 || saved.WindowY != 40 {
		t.Fatalf("保存设置后窗口几何应保留：%+v", saved)
	}
}

// TestPreviewDecodesResultImage 验证结果图预览能解码 PNG 并在对话框里绘制。
func TestPreviewDecodesResultImage(t *testing.T) {
	dir := t.TempDir()
	application, driver := openTestApp(t, dir, newFakeRunner(nil))
	path := filepath.Join(dir, "预览.png")
	picture := image.NewRGBA(image.Rect(0, 0, 8, 4))
	picture.Set(0, 0, color.RGBA{R: 255, A: 255})
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(file, picture); err != nil {
		file.Close()
		t.Fatal(err)
	}
	file.Close()

	application.startImagePreview(path)
	if application.dialog != dialogPreview {
		t.Fatalf("应打开预览对话框，实际 %v", application.dialog)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if err := driver.Pump(); err != nil {
			t.Fatal(err)
		}
		if !application.preview.loading {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if application.preview.err != "" {
		t.Fatalf("预览失败：%s", application.preview.err)
	}
	if application.preview.bitmap.Width != 8 || application.preview.bitmap.Height != 4 {
		t.Fatalf("解码尺寸不对：%d×%d", application.preview.bitmap.Width, application.preview.bitmap.Height)
	}
	// 左上角是纯红，其余透明：预乘 BGRA 下应为 0,0,255,255。
	pixels := application.preview.bitmap.Pix
	if pixels[0] != 0 || pixels[1] != 0 || pixels[2] != 255 || pixels[3] != 255 {
		t.Fatalf("预乘 BGRA 转换不对：%v", pixels[:4])
	}
	// scrim、右上角关闭按钮与 footer 按钮共享同一 ID，按标签点 footer 的
	// 「关闭」，避免点到整屏 scrim 的中心（落在表面内，不会触发关闭）。
	clickLabel(t, driver, "关闭")
	if application.dialog != dialogNone || application.preview.open {
		t.Fatalf("关闭后预览状态应复位：dialog=%v open=%v", application.dialog, application.preview.open)
	}
}

// TestContextMenuOffersEntryDirectory 验证右键菜单提供打开配置目录；订阅
// 条目则提供打开缓存目录。
func TestContextMenuOffersEntryDirectory(t *testing.T) {
	dir := t.TempDir()
	writeNodesFile(t, dir, "a.yaml", "HK-01")
	seedEntry(t, dir, store.Entry{ID: "e1", Name: "机场 A", Path: "a.yaml"})
	// 订阅条目先放一份缓存：启动只读缓存不联网。
	subscriptionDir := filepath.Join(dir, "data", "subscriptions")
	if err := os.MkdirAll(subscriptionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeNodesFile(t, subscriptionDir, "e2.yaml", "JP-01")
	seedEntry(t, dir, store.Entry{ID: "e2", Name: "机场 B", Kind: store.EntryKindSubscription, URL: "https://example.com/sub"})
	application, driver := openTestApp(t, dir, newFakeRunner(nil))

	var opened string
	original := openDirectory
	openDirectory = func(path string) error {
		opened = path
		return nil
	}
	t.Cleanup(func() { openDirectory = original })

	rightClickID(t, driver, application.nodes[0].ids.row)
	if !menuHasLabel(application.menu, "打开配置文件所在目录") {
		t.Fatalf("本地条目菜单缺少打开目录项：%+v", application.menu.items)
	}
	clickLabel(t, driver, "打开配置文件所在目录")
	if opened != filepath.Clean(dir) {
		t.Fatalf("应打开配置文件所在目录 %q，实际 %q", dir, opened)
	}
	if !strings.Contains(application.status, "已打开目录") {
		t.Fatalf("打开目录后应有状态提示，实际 %q", application.status)
	}

	subscription := application.entryByID("e2")
	if subscription == nil || len(subscription.nodes) != 1 {
		t.Fatalf("订阅缓存应有 1 个节点，实际 %+v", subscription)
	}
	rightClickID(t, driver, subscription.nodes[0].ids.row)
	if !menuHasLabel(application.menu, "打开订阅缓存目录") {
		t.Fatalf("订阅条目菜单文案不对：%+v", application.menu.items)
	}
}

// TestWindowSettingsRoundTrip 验证设置文件的窗口字段能原样写读，重启按
// 保存的几何打开（含副屏负坐标）。
func TestWindowSettingsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	database, err := store.Open(store.Options{BaseDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	settings := store.DefaultSettings()
	settings.WindowSet = true
	settings.WindowWidth = 1440
	settings.WindowHeight = 900
	settings.WindowX = -200
	settings.WindowY = 50
	if err := database.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "data", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "window_set") {
		t.Fatalf("设置文件应写入窗口字段：%s", data)
	}
	var envelope struct {
		Settings store.Settings `json:"settings"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Settings.WindowWidth != 1440 || envelope.Settings.WindowX != -200 {
		t.Fatalf("窗口字段读取不对：%+v", envelope.Settings)
	}

	application, _ := openTestApp(t, dir, newFakeRunner(nil))
	if application.Config().Width != 1440 {
		t.Fatalf("重启后应使用保存的宽度，实际 %d", application.Config().Width)
	}
	if origin := application.Config().Origin; origin == nil || origin.X != -200 || origin.Y != 50 {
		t.Fatalf("重启后应使用保存的位置，实际 %+v", origin)
	}
}

// 点击一个命中的中心；with 为真时先按住修饰键（Shift 或 Ctrl）再点击。
func clickHitWith(t *testing.T, driver *app.Driver, id ui.ID, key ui.Key, with bool) {
	t.Helper()
	var rect = hitRect(t, driver, id)
	if with {
		if err := driver.KeyDown(key, ""); err != nil {
			t.Fatal(err)
		}
	}
	if err := driver.Click(rect.X+rect.Width/2, rect.Y+rect.Height/2); err != nil {
		t.Fatal(err)
	}
	if with {
		if err := driver.KeyUp(key, ""); err != nil {
			t.Fatal(err)
		}
	}
}

// Shift 点击选中锚点到目标之间的全部可见节点；Ctrl 点击切换单个节点。
func TestShiftAndCtrlClickMultiSelect(t *testing.T) {
	dir := t.TempDir()
	writeNodesFile(t, dir, "a.yaml", "A-01", "A-02", "A-03", "A-04", "A-05")
	seedEntry(t, dir, store.Entry{ID: "e1", Name: "机场 A", Path: "a.yaml"})
	application, driver := openTestApp(t, dir, newFakeRunner(nil))

	// 先全不选，效果才看得清。
	clickID(t, driver, application.ids.selectNone)
	if application.selectedCount != 0 {
		t.Fatalf("全不选后应为 0，实际 %d", application.selectedCount)
	}

	// 普通点击 A-01：选中并记为范围锚点。
	clickHitWith(t, driver, application.nodes[0].ids.row, 0, false)
	if application.selectedCount != 1 {
		t.Fatalf("普通点击应只选 A-01，实际 %d", application.selectedCount)
	}

	// Shift 点击 A-04：A-01..A-04 全部选中，A-05 不动。
	clickHitWith(t, driver, application.nodes[3].ids.row, ui.KeyShiftLeft, true)
	if application.selectedCount != 4 {
		t.Fatalf("Shift 范围应选中 4 个，实际 %d", application.selectedCount)
	}
	for index := 0; index < 4; index++ {
		if !application.nodes[index].selected {
			t.Fatalf("A-0%d 应被范围选中", index+1)
		}
	}
	if application.nodes[4].selected {
		t.Fatal("范围外的 A-05 不应被选中")
	}

	// Ctrl 点击 A-05：单独加入，其它保持。
	clickHitWith(t, driver, application.nodes[4].ids.row, ui.KeyControlLeft, true)
	if application.selectedCount != 5 {
		t.Fatalf("Ctrl 点击应加入 A-05，实际 %d", application.selectedCount)
	}

	// Ctrl 点击 A-05：从选择中移除。
	clickHitWith(t, driver, application.nodes[4].ids.row, ui.KeyControlLeft, true)
	if application.selectedCount != 4 || application.nodes[4].selected {
		t.Fatal("Ctrl 再次点击应移除 A-05")
	}

	// 锚点未变：Shift 点击 A-02 会把范围缩到 A-01..A-02，但已选中的
	// A-03、A-04 保留（范围选择只增不减）。
	clickHitWith(t, driver, application.nodes[1].ids.row, ui.KeyShiftLeft, true)
	if !application.nodes[2].selected || !application.nodes[3].selected {
		t.Fatal("Shift 范围选择不应取消范围外的勾选")
	}
}

// 条目卡片上的「打开目录」打开配置文件所在目录。
func TestEntryCardOpensDirectory(t *testing.T) {
	dir := t.TempDir()
	writeNodesFile(t, dir, "a.yaml", "HK-01")
	seedEntry(t, dir, store.Entry{ID: "e1", Name: "机场 A", Path: "a.yaml"})
	application, driver := openTestApp(t, dir, newFakeRunner(nil))

	var opened string
	original := openDirectory
	openDirectory = func(path string) error {
		opened = path
		return nil
	}
	t.Cleanup(func() { openDirectory = original })

	entry := application.entryByID("e1")
	clickID(t, driver, entry.ids.openDir)
	if opened != filepath.Clean(dir) {
		t.Fatalf("应打开配置所在目录 %q，实际 %q", dir, opened)
	}
	if !strings.Contains(application.status, "已打开目录") {
		t.Fatalf("应有状态提示，实际 %q", application.status)
	}
}

// 点击详情弹窗以外的任意位置关闭弹窗。
func TestDetailPopupDismissesOnOutsideClick(t *testing.T) {
	dir := t.TempDir()
	writeNodesFile(t, dir, "a.yaml", "HK-01")
	seedEntry(t, dir, store.Entry{ID: "e1", Name: "机场 A", Path: "a.yaml"})
	application, driver := openTestApp(t, dir, newFakeRunner(nil))

	clickID(t, driver, application.nodes[0].ids.knob)
	if application.detailNode == nil {
		t.Fatal("点三点图标应打开详情")
	}
	// 点窗口左上角（页面标题区，弹窗之外）。
	if err := driver.Click(300, 16); err != nil {
		t.Fatal(err)
	}
	if application.detailNode != nil || application.activeNode != nil {
		t.Fatal("点击弹窗之外应关闭详情")
	}
}
