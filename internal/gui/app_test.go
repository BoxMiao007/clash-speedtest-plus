package gui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/BoxMiao007/clash-speedtest-plus/internal/filter"
	"github.com/BoxMiao007/clash-speedtest-plus/internal/speedtester"
	"github.com/BoxMiao007/clash-speedtest-plus/internal/store"
	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/components"
	"github.com/vivaui/viva/ui"
)

// fakeRunner 不联网：按脚本立即产出一条节点的开始、进度和结果事件，然后返回。
// GUI 的后台回调仍走真实的待处理队列与 Invalidate，因此测试覆盖的是实际同步路径。
type fakeRunner struct {
	mu      sync.Mutex
	results map[string]fakeResult
	batch   []RunNode
	stopped bool
}

type fakeResult struct {
	ok      bool
	latency time.Duration
	speed   float64
}

func newFakeRunner(results map[string]fakeResult) *fakeRunner {
	return &fakeRunner{results: results}
}

func (runner *fakeRunner) Run(ctx context.Context, batch []RunNode, handlers Handlers) error {
	runner.mu.Lock()
	runner.batch = append([]RunNode(nil), batch...)
	runner.mu.Unlock()
	for _, node := range batch {
		if err := ctx.Err(); err != nil {
			return err
		}
		name := ""
		if node.Proxy != nil {
			name = node.Proxy.Name()
		}
		result, ok := runner.results[name]
		if !ok {
			result = fakeResult{ok: true, latency: 30 * time.Millisecond, speed: 5 * 1024 * 1024}
		}
		if handlers.Start != nil {
			handlers.Start(node.Key, "")
		}
		if handlers.Progress != nil && result.ok {
			handlers.Progress(speedtester.Progress{
				Name:         node.Key,
				Phase:        speedtester.PhaseDownload,
				Latency:      result.latency,
				InstantSpeed: result.speed,
			})
		}
		if handlers.Result != nil {
			value := &speedtester.Result{
				ProxyName:     node.Key,
				Latency:       result.latency,
				DownloadSpeed: result.speed,
			}
			if !result.ok {
				value.DownloadError = "connection refused"
			}
			handlers.Result(value)
		}
	}
	return nil
}

func (runner *fakeRunner) Stop() {
	runner.mu.Lock()
	runner.stopped = true
	runner.mu.Unlock()
}

func (runner *fakeRunner) lastBatch() []RunNode {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return append([]RunNode(nil), runner.batch...)
}

func writeNodesFile(t *testing.T, dir, name string, nodeNames ...string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	var builder strings.Builder
	builder.WriteString("proxies:\n")
	for index, nodeName := range nodeNames {
		fmt.Fprintf(&builder, "  - name: %q\n", nodeName)
		builder.WriteString("    type: ss\n")
		builder.WriteString("    server: 127.0.0.1\n")
		fmt.Fprintf(&builder, "    port: %d\n", 20000+index)
		builder.WriteString("    cipher: aes-128-gcm\n")
		builder.WriteString("    password: fixture\n")
	}
	if err := os.WriteFile(path, []byte(builder.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func seedEntry(t *testing.T, dir string, entry store.Entry) {
	t.Helper()
	seedEntryImpl(t, dir, entry)
}

func seedEntryImpl(tb testing.TB, dir string, entry store.Entry) {
	tb.Helper()
	database, err := store.Open(store.Options{BaseDir: dir})
	if err != nil {
		tb.Fatal(err)
	}
	if err := database.PutEntry(entry); err != nil {
		tb.Fatal(err)
	}
}

func openTestApp(t *testing.T, dir string, runner Runner) (*App, *app.Driver) {
	t.Helper()
	application, err := New(Options{
		BaseDir: dir,
		Version: "test",
		Runner:  runner,
	})
	if err != nil {
		t.Fatal(err)
	}
	driver, err := app.NewDriver(application.Config(), nil)
	if err != nil {
		application.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		driver.Close()
		application.Close()
	})
	return application, driver
}

func clickLabel(t *testing.T, driver *app.Driver, label string) {
	t.Helper()
	for _, hit := range driver.Snapshot().Hits {
		if hit.Label == label {
			if err := driver.Click(hit.Bounds.X+hit.Bounds.Width/2, hit.Bounds.Y+hit.Bounds.Height/2); err != nil {
				t.Fatal(err)
			}
			return
		}
	}
	t.Fatalf("找不到标签为 %q 的控件；当前命中：%v", label, hitLabels(driver.Snapshot()))
}

func clickID(t *testing.T, driver *app.Driver, id ui.ID) {
	t.Helper()
	for _, hit := range driver.Snapshot().Hits {
		if hit.ID == id {
			if err := driver.Click(hit.Bounds.X+hit.Bounds.Width/2, hit.Bounds.Y+hit.Bounds.Height/2); err != nil {
				t.Fatal(err)
			}
			return
		}
	}
	t.Fatalf("找不到 ID %v；当前命中：%v", id, hitLabels(driver.Snapshot()))
}

func rightClickID(t *testing.T, driver *app.Driver, id ui.ID) {
	t.Helper()
	for _, hit := range driver.Snapshot().Hits {
		if hit.ID == id {
			if err := driver.ClickButton(hit.Bounds.X+hit.Bounds.Width/2, hit.Bounds.Y+hit.Bounds.Height/2, ui.PointerButtonSecondary); err != nil {
				t.Fatal(err)
			}
			return
		}
	}
	t.Fatalf("找不到 ID %v；当前命中：%v", id, hitLabels(driver.Snapshot()))
}

func hitLabels(snapshot app.Snapshot) []string {
	labels := make([]string, 0, len(snapshot.Hits))
	for _, hit := range snapshot.Hits {
		labels = append(labels, hit.Label)
	}
	return labels
}

func hasHitID(snapshot app.Snapshot, id ui.ID) bool {
	for _, hit := range snapshot.Hits {
		if hit.ID == id {
			return true
		}
	}
	return false
}

func hitDisabled(snapshot app.Snapshot, id ui.ID) bool {
	for _, hit := range snapshot.Hits {
		if hit.ID == id {
			return hit.Disabled
		}
	}
	return false
}

func hasText(snapshot app.Snapshot, want string) bool {
	for _, label := range snapshot.Labels {
		if strings.Contains(label.Value, want) {
			return true
		}
	}
	for _, hit := range snapshot.Hits {
		if strings.Contains(hit.Label, want) {
			return true
		}
	}
	return false
}

func hitValue(t *testing.T, snapshot app.Snapshot, label string) string {
	t.Helper()
	for _, hit := range snapshot.Hits {
		if hit.Label == label {
			return hit.Value
		}
	}
	t.Fatalf("找不到标签为 %q 的控件", label)
	return ""
}

func waitUntilIdle(t *testing.T, application *App, driver *app.Driver) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if err := driver.Pump(); err != nil {
			t.Fatal(err)
		}
		if !application.running() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("测速未在超时内结束")
}

func focusAndType(t *testing.T, driver *app.Driver, label, text string) {
	t.Helper()
	clickLabel(t, driver, label)
	if err := driver.Type(text); err != nil {
		t.Fatal(err)
	}
}

func replaceFieldText(t *testing.T, driver *app.Driver, label, text string) {
	t.Helper()
	clickLabel(t, driver, label)
	if err := driver.SelectText(0, 4096); err != nil {
		t.Fatal(err)
	}
	if err := driver.Type(text); err != nil {
		t.Fatal(err)
	}
}

func TestCreateEntryThroughDialogPersistsSoftPath(t *testing.T) {
	dir := t.TempDir()
	configPath := writeNodesFile(t, dir, "a.yaml", "HK-01", "HK-02")
	application, driver := openTestApp(t, dir, newFakeRunner(nil))

	clickLabel(t, driver, "新建条目")
	if !hasText(driver.Snapshot(), "新建条目") {
		t.Fatal("新建条目对话框没有打开")
	}
	focusAndType(t, driver, "名称", "机场 A")
	if err := driver.QueueOpenFiles([]string{configPath}, nil); err != nil {
		t.Fatal(err)
	}
	clickLabel(t, driver, "选择文件…")
	clickID(t, driver, application.ids.entrySave)

	snapshot := driver.Snapshot()
	if !hasText(snapshot, "机场 A") {
		t.Fatal("条目没有出现在列表里")
	}
	if !hasText(snapshot, "节点 机场 A / HK-01") {
		t.Fatalf("自动读取没有展示节点；界面文本 = %v", snapshot.Labels)
	}
	entriesPath := filepath.Join(dir, "data", "entries.json")
	raw, err := os.ReadFile(entriesPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"path": "a.yaml"`) {
		t.Fatalf("程序目录内的绝对路径应保存为软路径，实际：%s", raw)
	}
}

func TestRenameChangePathAndDeleteEntry(t *testing.T) {
	dir := t.TempDir()
	writeNodesFile(t, dir, "a.yaml", "HK-01")
	bPath := writeNodesFile(t, dir, "b.yaml", "JP-01")
	seedEntry(t, dir, store.Entry{ID: "e1", Name: "机场 A", Path: "a.yaml"})
	application, driver := openTestApp(t, dir, newFakeRunner(nil))

	entry := application.entryByID("e1")
	clickID(t, driver, entry.ids.edit)
	replaceFieldText(t, driver, "名称", "主力机场")
	clickID(t, driver, application.ids.entrySave)
	if !hasText(driver.Snapshot(), "主力机场") {
		t.Fatal("重命名没有生效")
	}

	entry = application.entryByID("e1")
	clickID(t, driver, entry.ids.edit)
	if err := driver.QueueOpenFiles([]string{bPath}, nil); err != nil {
		t.Fatal(err)
	}
	clickLabel(t, driver, "选择文件…")
	clickID(t, driver, application.ids.entrySave)
	if !hasText(driver.Snapshot(), "软路径 · b.yaml") {
		t.Fatalf("修改路径没有生效；文本=%v", driver.Snapshot().Labels)
	}
	if !hasText(driver.Snapshot(), "节点 主力机场 / JP-01") {
		t.Fatal("修改路径后没有读取新文件的节点")
	}

	driver.Close()
	application, driver = openTestApp(t, dir, newFakeRunner(nil))
	entry = application.entryByID("e1")
	clickID(t, driver, entry.ids.remove)
	clickID(t, driver, application.ids.confirmOK)
	if hasText(driver.Snapshot(), "主力机场") {
		t.Fatal("删除后条目仍然显示")
	}
	raw, err := os.ReadFile(filepath.Join(dir, "data", "entries.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "e1") {
		t.Fatalf("条目没有从 entries.json 删除：%s", raw)
	}
}

func TestGroupsStaySeparateAcrossEntries(t *testing.T) {
	dir := t.TempDir()
	writeNodesFile(t, dir, "a.yaml", "HK-01")
	writeNodesFile(t, dir, "b.yaml", "HK-01", "JP-01")
	seedEntry(t, dir, store.Entry{ID: "e1", Name: "机场 A", Path: "a.yaml"})
	seedEntry(t, dir, store.Entry{ID: "e2", Name: "机场 B", Path: "b.yaml"})
	application, driver := openTestApp(t, dir, newFakeRunner(nil))

	if len(application.groups) != 2 {
		t.Fatalf("应有 2 个条目分组，实际 %d", len(application.groups))
	}
	snapshot := driver.Snapshot()
	if !hasText(snapshot, "节点 机场 A / HK-01") || !hasText(snapshot, "节点 机场 B / HK-01") {
		t.Fatal("跨条目的同名节点应各自成行")
	}

	clickID(t, driver, application.entryByID("e2").ids.check)
	if len(application.groups) != 1 {
		t.Fatalf("取消勾选后应只剩 1 个分组，实际 %d", len(application.groups))
	}
	clickID(t, driver, application.ids.readNodes)
	if len(application.groups) != 1 {
		t.Fatal("手动读取只应读取已勾选条目")
	}

	// 重新勾选条目应触发自动读取，分组回来。
	clickID(t, driver, application.entryByID("e2").ids.check)
	if len(application.groups) != 2 {
		t.Fatalf("勾选条目应自动读取并恢复分组，实际 %d", len(application.groups))
	}

	// 组内全不选 / 全选。
	entry := application.entryByID("e1")
	clickID(t, driver, entry.ids.selectNone)
	for _, node := range entry.nodes {
		if node.selected {
			t.Fatalf("全不选后 %s 仍被勾选", node.name)
		}
	}
	clickID(t, driver, entry.ids.selectAll)
	for _, node := range entry.nodes {
		if !node.selected {
			t.Fatalf("全选后 %s 没有被勾选", node.name)
		}
	}
}

func TestRefetchKeepsResultAndDropsRemovedNode(t *testing.T) {
	dir := t.TempDir()
	path := writeNodesFile(t, dir, "a.yaml", "HK-01", "HK-02")
	seedEntry(t, dir, store.Entry{ID: "e1", Name: "机场 A", Path: "a.yaml"})
	runner := newFakeRunner(map[string]fakeResult{
		"HK-01": {ok: true, latency: 25 * time.Millisecond, speed: 10 * 1024 * 1024},
		"HK-02": {ok: true, latency: 40 * time.Millisecond, speed: 2 * 1024 * 1024},
	})
	application, driver := openTestApp(t, dir, runner)

	clickID(t, driver, application.ids.startTest)
	waitUntilIdle(t, application, driver)
	clickID(t, driver, application.ids.backToSelect)

	infoBefore := application.byKey["e1/"+nodeIdentityOf(t, application, "HK-01")].infoText
	if !strings.Contains(infoBefore, "上次") {
		t.Fatalf("测试后应显示上次测速时间，实际 %q", infoBefore)
	}

	writeNodesFile(t, dir, "a.yaml", "HK-01", "HK-03")
	entry := application.entryByID("e1")
	clickID(t, driver, entry.ids.refetch)

	if application.byKey["e1/"+nodeIdentityOf(t, application, "HK-01")] == nil {
		t.Fatal("身份匹配的 HK-01 应保留")
	}
	if application.byKey["e1/"+nodeIdentityOf(t, application, "HK-03")] == nil {
		t.Fatal("新节点 HK-03 应加入")
	}
	if len(application.entryByID("e1").nodes) != 2 {
		t.Fatalf("消失的 HK-02 应移除，实际节点数 %d", len(application.entryByID("e1").nodes))
	}
	infoAfter := application.byKey["e1/"+nodeIdentityOf(t, application, "HK-01")].infoText
	if infoAfter != infoBefore {
		t.Fatalf("重新获取后结果被覆盖：before=%q after=%q", infoBefore, infoAfter)
	}
	newNode := application.byKey["e1/"+nodeIdentityOf(t, application, "HK-03")]
	if newNode.hasResult || !strings.Contains(newNode.infoText, "未测试") {
		t.Fatalf("新节点应显示未测试，实际 %q", newNode.infoText)
	}
	_ = path
}

func nodeIdentityOf(t *testing.T, application *App, name string) string {
	t.Helper()
	for _, node := range application.nodes {
		if node.name == name {
			return node.id
		}
	}
	t.Fatalf("找不到节点 %q", name)
	return ""
}

func TestNodeIdentityIncludesNestedParams(t *testing.T) {
	dir := t.TempDir()
	write := func(name, path string) string {
		file := filepath.Join(dir, name)
		body := fmt.Sprintf("proxies:\n  - name: \"VM-01\"\n    type: vmess\n    server: 127.0.0.1\n    port: 1001\n    uuid: 00000000-0000-0000-0000-000000000000\n    alterId: 0\n    cipher: auto\n    network: ws\n    ws-opts:\n      path: %q\n", path)
		if err := os.WriteFile(file, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return file
	}
	load := func(file string) *speedtester.CProxy {
		nodes, err := speedtester.LoadNodesFile(file)
		if err != nil || len(nodes) != 1 {
			t.Fatalf("加载 %s 失败：%v", file, err)
		}
		return nodes[0]
	}

	first := load(write("a.yaml", "/a"))
	second := load(write("b.yaml", "/b"))
	same := load(write("c.yaml", "/a"))
	if nodeIdentity(first) == nodeIdentity(second) {
		t.Fatal("仅 ws-opts.path 不同的节点身份不应相同")
	}
	if nodeIdentity(first) != nodeIdentity(same) {
		t.Fatal("配置相同的节点身份应稳定相同")
	}
}

func TestFilterSimpleAdvancedAndInvalidRegex(t *testing.T) {
	dir := t.TempDir()
	writeNodesFile(t, dir, "a.yaml", "香港 IPLC 01", "香港 试用 02", "日本 03")
	seedEntry(t, dir, store.Entry{ID: "e1", Name: "机场 A", Path: "a.yaml"})
	application, driver := openTestApp(t, dir, newFakeRunner(nil))

	focusAndType(t, driver, "过滤表达式", "香港 !试用")
	if err := driver.Key(ui.KeyEnter, 0); err != nil {
		t.Fatal(err)
	}
	snapshot := driver.Snapshot()
	if !hasText(snapshot, "香港 IPLC 01") || hasText(snapshot, "香港 试用 02") || hasText(snapshot, "日本 03") {
		t.Fatalf("简化过滤结果不对：%v", snapshot.Labels)
	}

	clickID(t, driver, application.ids.combScope.At(1))
	replaceFieldText(t, driver, "过滤表达式", "香港 日本")
	if err := driver.Key(ui.KeyEnter, 0); err != nil {
		t.Fatal(err)
	}
	if !hasText(driver.Snapshot(), "日本 03") {
		t.Fatal("「或」组合应让日本节点可见")
	}

	clickID(t, driver, application.ids.modeScope.At(1))
	replaceFieldText(t, driver, "过滤表达式", "^香港 IPLC")
	if err := driver.Key(ui.KeyEnter, 0); err != nil {
		t.Fatal(err)
	}
	snapshot = driver.Snapshot()
	if !hasText(snapshot, "香港 IPLC 01") || hasText(snapshot, "日本 03") {
		t.Fatal("正则过滤结果不对")
	}

	replaceFieldText(t, driver, "过滤表达式", "[")
	if err := driver.Key(ui.KeyEnter, 0); err != nil {
		t.Fatal(err)
	}
	if !hasText(driver.Snapshot(), "正则表达式无效") {
		t.Fatal("非法正则应显示错误")
	}
	if !hasText(driver.Snapshot(), "香港 IPLC 01") {
		t.Fatal("非法正则应保留上一个有效过滤结果")
	}

	replaceFieldText(t, driver, "过滤表达式", "^ZZZ")
	if err := driver.Key(ui.KeyEnter, 0); err != nil {
		t.Fatal(err)
	}
	if !hasText(driver.Snapshot(), "没有节点匹配当前过滤条件") {
		t.Fatal("过滤无匹配时应显示空态")
	}
}

func TestPresetsSaveApplyRenameOverwriteDelete(t *testing.T) {
	dir := t.TempDir()
	writeNodesFile(t, dir, "a.yaml", "香港 01", "日本 02")
	seedEntry(t, dir, store.Entry{ID: "e1", Name: "机场 A", Path: "a.yaml"})
	application, driver := openTestApp(t, dir, newFakeRunner(nil))

	focusAndType(t, driver, "过滤表达式", "香港")
	if err := driver.Key(ui.KeyEnter, 0); err != nil {
		t.Fatal(err)
	}
	clickID(t, driver, application.ids.combScope.At(1))
	clickID(t, driver, application.ids.savePreset)
	focusAndType(t, driver, "预设名称", "港")
	clickID(t, driver, application.ids.presetSave)

	raw, err := os.ReadFile(filepath.Join(dir, "data", "presets.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"name": "港"`) {
		t.Fatalf("预设没有落盘：%s", raw)
	}

	replaceFieldText(t, driver, "过滤表达式", "日本")
	clickLabel(t, driver, "预设")
	clickLabel(t, driver, "港")
	if got := hitValue(t, driver.Snapshot(), "过滤表达式"); got != "香港" {
		t.Fatalf("应用预设没有恢复表达式，实际 %q", got)
	}

	clickID(t, driver, application.ids.renamePreset)
	replaceFieldText(t, driver, "预设名称", "香港预设")
	clickID(t, driver, application.ids.presetSave)
	if len(application.presets) != 1 || application.presets[0].Name != "香港预设" {
		t.Fatalf("重命名失败：%+v", application.presets)
	}

	replaceFieldText(t, driver, "过滤表达式", "新词")
	clickID(t, driver, application.ids.savePreset)
	clickID(t, driver, application.ids.presetSave)
	if application.presets[0].Expression != "新词" {
		t.Fatalf("覆盖预设失败：%+v", application.presets[0])
	}

	clickID(t, driver, application.ids.deletePreset)
	clickID(t, driver, application.ids.confirmOK)
	if len(application.presets) != 0 {
		t.Fatalf("删除预设失败：%+v", application.presets)
	}
}

func TestPresetRestoresModeAndSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	writeNodesFile(t, dir, "a.yaml", "HK-01")
	seedEntry(t, dir, store.Entry{ID: "e1", Name: "机场 A", Path: "a.yaml"})
	application, driver := openTestApp(t, dir, newFakeRunner(nil))

	clickID(t, driver, application.ids.combScope.At(1))
	clickID(t, driver, application.ids.modeScope.At(1))
	focusAndType(t, driver, "过滤表达式", "HK")
	if err := driver.Key(ui.KeyEnter, 0); err != nil {
		t.Fatal(err)
	}
	clickID(t, driver, application.ids.savePreset)
	focusAndType(t, driver, "预设名称", "高级")
	clickID(t, driver, application.ids.presetSave)
	if len(application.presets) != 1 {
		t.Fatalf("预设没有保存：%+v", application.presets)
	}
	driver.Close()

	application, driver = openTestApp(t, dir, newFakeRunner(nil))
	if len(application.presets) != 1 || application.presets[0].Name != "高级" {
		t.Fatalf("重启后预设丢失：%+v", application.presets)
	}
	clickID(t, driver, application.ids.modeScope.At(0))
	if application.filterMode != filter.ModeSimple {
		t.Fatal("切换简化模式失败")
	}
	clickLabel(t, driver, "预设")
	clickLabel(t, driver, "高级")
	if application.filterMode != filter.ModeAdvanced || application.filterCombine != filter.CombineOr {
		t.Fatalf("应用预设未恢复模式/组合：mode=%v combine=%v", application.filterMode, application.filterCombine)
	}
	if got := hitValue(t, driver.Snapshot(), "过滤表达式"); got != "HK" {
		t.Fatalf("应用预设未恢复表达式，实际 %q", got)
	}
}

func TestStartTestSwitchesViewSortsBySpeedAndKeepsRowIDs(t *testing.T) {
	dir := t.TempDir()
	writeNodesFile(t, dir, "a.yaml", "HK-01", "HK-02")
	seedEntry(t, dir, store.Entry{ID: "e1", Name: "机场 A", Path: "a.yaml"})
	runner := newFakeRunner(map[string]fakeResult{
		"HK-01": {ok: true, latency: 20 * time.Millisecond, speed: 1 * 1024 * 1024},
		"HK-02": {ok: true, latency: 30 * time.Millisecond, speed: 20 * 1024 * 1024},
	})
	application, driver := openTestApp(t, dir, runner)

	rowsBefore := make(map[string]ui.ID)
	for _, node := range application.nodes {
		rowsBefore[node.id] = node.ids.row
	}
	clickID(t, driver, application.ids.startTest)
	waitUntilIdle(t, application, driver)
	if application.active != viewResults {
		t.Fatal("开始测速应自动切到下载速度视图")
	}
	if len(application.results) != 2 || application.results[0].name != "HK-02" {
		t.Fatalf("结果应按下载速度降序，实际 %v", resultNames(application))
	}
	for _, node := range application.nodes {
		if node.ids.row != rowsBefore[node.id] {
			t.Fatalf("重排后行身份变了：%s", node.name)
		}
	}
	snapshot := driver.Snapshot()
	for _, node := range application.nodes {
		if !hasHitID(snapshot, node.ids.row) {
			t.Fatalf("结果视图缺少节点 %s 的稳定行命中", node.name)
		}
	}
	if !hasText(snapshot, "20.00MB/s") {
		t.Fatalf("应显示格式化的下载速度；文本=%v", snapshot.Labels)
	}
}

func resultNames(application *App) []string {
	names := make([]string, 0, len(application.results))
	for _, node := range application.results {
		names = append(names, node.name)
	}
	return names
}

func TestExcludeFailedOnlySendsHealthyNodes(t *testing.T) {
	dir := t.TempDir()
	writeNodesFile(t, dir, "a.yaml", "HK-01", "HK-02")
	seedEntry(t, dir, store.Entry{ID: "e1", Name: "机场 A", Path: "a.yaml"})
	runner := newFakeRunner(map[string]fakeResult{
		"HK-01": {ok: true, latency: 20 * time.Millisecond, speed: 8 * 1024 * 1024},
		"HK-02": {ok: false},
	})
	application, driver := openTestApp(t, dir, runner)

	clickID(t, driver, application.ids.startTest)
	waitUntilIdle(t, application, driver)
	clickID(t, driver, application.ids.backToSelect)
	clickID(t, driver, application.ids.excludeFailed)

	excluded := 0
	for _, node := range application.nodes {
		if node.name == "HK-02" {
			if node.selected {
				t.Fatal("上次失败节点应被取消勾选")
			}
			excluded++
		}
	}
	if excluded != 1 {
		t.Fatal("没有找到失败节点")
	}

	clickID(t, driver, application.ids.startTest)
	waitUntilIdle(t, application, driver)
	batch := runner.lastBatch()
	if len(batch) != 1 || batch[0].Proxy.Name() != "HK-01" {
		t.Fatalf("排除后只应测试 HK-01，实际 %d 个", len(batch))
	}
}

type stagedRunner struct {
	ready  chan Handlers
	finish chan struct{}
}

func (runner *stagedRunner) Run(ctx context.Context, batch []RunNode, handlers Handlers) error {
	runner.ready <- handlers
	select {
	case <-runner.finish:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (runner *stagedRunner) Stop() {}

// 结果陆续产生时下载速度视图应实时重排，且同一节点的行身份不变。
func TestLiveSortDuringRunKeepsRowIdentity(t *testing.T) {
	dir := t.TempDir()
	writeNodesFile(t, dir, "a.yaml", "HK-01", "HK-02")
	seedEntry(t, dir, store.Entry{ID: "e1", Name: "机场 A", Path: "a.yaml"})
	runner := &stagedRunner{ready: make(chan Handlers, 1), finish: make(chan struct{})}
	application, driver := openTestApp(t, dir, runner)

	rows := make(map[string]ui.ID)
	for _, node := range application.nodes {
		rows[node.name] = node.ids.row
	}
	clickID(t, driver, application.ids.startTest)
	handlers := <-runner.ready
	first, second := application.nodes[0], application.nodes[1]

	handlers.Start(first.key, "")
	handlers.Result(&speedtester.Result{ProxyName: first.key, Latency: 20 * time.Millisecond, DownloadSpeed: 1 * 1024 * 1024})
	if err := driver.Pump(); err != nil {
		t.Fatal(err)
	}
	if len(application.results) != 2 || application.results[0].name != first.name {
		t.Fatalf("第一条结果后应立即排到等待行之前，实际 %v", resultNames(application))
	}
	before := application.results[0].ids.row

	handlers.Result(&speedtester.Result{ProxyName: second.key, Latency: 20 * time.Millisecond, DownloadSpeed: 30 * 1024 * 1024})
	if err := driver.Pump(); err != nil {
		t.Fatal(err)
	}
	if len(application.results) != 2 || application.results[0].name != second.name {
		t.Fatalf("较快的后到结果应排到第一，实际 %v", resultNames(application))
	}
	if application.results[1].ids.row != before {
		t.Fatal("重排后行身份发生变化")
	}
	for name, id := range rows {
		if application.byKey["e1/"+nodeIdentityOf(t, application, name)].ids.row != id {
			t.Fatalf("节点 %s 的行身份发生变化", name)
		}
	}

	close(runner.finish)
	waitUntilIdle(t, application, driver)
}

func TestNodeListVirtualizesAndScrolls(t *testing.T) {
	dir := t.TempDir()
	names := make([]string, 0, 60)
	for index := 0; index < 60; index++ {
		names = append(names, fmt.Sprintf("节点 %03d", index))
	}
	writeNodesFile(t, dir, "a.yaml", names...)
	seedEntry(t, dir, store.Entry{ID: "e1", Name: "机场 A", Path: "a.yaml"})
	application, driver := openTestApp(t, dir, newFakeRunner(nil))

	if hasText(driver.Snapshot(), "节点 059") {
		t.Fatal("列表应只绘制视口内的行")
	}
	viewport := application.nodeViewport
	if viewport.Height <= 0 {
		t.Fatal("没有记录节点列表视口")
	}
	if err := driver.Scroll(ui.ScrollEvent{X: viewport.X + viewport.Width/2, Y: viewport.Y + viewport.Height/2, DeltaY: 5000, Precise: true}); err != nil {
		t.Fatal(err)
	}
	snapshot := driver.Snapshot()
	if hasText(snapshot, "节点 000") {
		t.Fatal("滚动后顶部行应移出视口")
	}
	if !hasText(snapshot, "节点 059") {
		t.Fatalf("滚动后应看到末尾节点；文本=%v", snapshot.Labels)
	}
}

func TestResultsSurviveRestart(t *testing.T) {
	dir := t.TempDir()
	writeNodesFile(t, dir, "a.yaml", "HK-01")
	seedEntry(t, dir, store.Entry{ID: "e1", Name: "机场 A", Path: "a.yaml"})
	runner := newFakeRunner(map[string]fakeResult{
		"HK-01": {ok: true, latency: 20 * time.Millisecond, speed: 8 * 1024 * 1024},
	})
	application, driver := openTestApp(t, dir, runner)
	clickID(t, driver, application.ids.startTest)
	waitUntilIdle(t, application, driver)
	driver.Close()

	application, driver = openTestApp(t, dir, newFakeRunner(nil))
	node := application.nodes[0]
	if !node.hasResult || !strings.Contains(node.infoText, "上次") {
		t.Fatalf("重启后应恢复上次测速结果，实际 %q", node.infoText)
	}
	if node.speedText != "8.00MB/s" {
		t.Fatalf("重启后应恢复速度文案，实际 %q", node.speedText)
	}
	if len(application.results) != 1 {
		t.Fatalf("重启后下载速度视图应展示历史结果，实际 %d", len(application.results))
	}
}

func TestContextMenuWordingSingleAndMulti(t *testing.T) {
	dir := t.TempDir()
	writeNodesFile(t, dir, "a.yaml", "HK-01", "HK-02")
	seedEntry(t, dir, store.Entry{ID: "e1", Name: "机场 A", Path: "a.yaml"})
	application, driver := openTestApp(t, dir, newFakeRunner(nil))

	first := application.nodes[0]
	for _, node := range application.nodes {
		if node != first && node.selected {
			clickID(t, driver, node.ids.check)
		}
	}
	rightClickID(t, driver, first.ids.row)
	if application.menu.multi || !menuHasLabel(application.menu, "复制此节点信息") {
		t.Fatalf("单选右键菜单文案不对：%+v", application.menu.items)
	}
	assertFlatMenu(t, application.menu)
	clickLabel(t, driver, "复制此节点信息")
	if !strings.Contains(driver.Clipboard(), first.name) {
		t.Fatalf("剪贴板没有节点信息：%q", driver.Clipboard())
	}

	for _, node := range application.nodes {
		if !node.selected {
			clickID(t, driver, node.ids.check)
		}
	}
	rightClickID(t, driver, first.ids.row)
	if !application.menu.multi || !menuHasLabel(application.menu, "复制所选节点信息") {
		t.Fatalf("多选右键菜单文案不对：%+v", application.menu.items)
	}
	if menuHasLabel(application.menu, "复制此节点信息") {
		t.Fatalf("多选菜单不应出现单选措辞：%+v", application.menu.items)
	}
	assertFlatMenu(t, application.menu)
	clickLabel(t, driver, "复制所选节点信息")
	clipboard := driver.Clipboard()
	if !strings.Contains(clipboard, "HK-01") || !strings.Contains(clipboard, "HK-02") {
		t.Fatalf("多选复制内容不全：%q", clipboard)
	}
}

func menuHasLabel(menu menuState, label string) bool {
	for _, entry := range menu.items {
		if !entry.separator && entry.item.Label == label {
			return true
		}
	}
	return false
}

func assertFlatMenu(t *testing.T, menu menuState) {
	t.Helper()
	for _, entry := range menu.items {
		if entry.separator {
			continue
		}
		if entry.item.Kind == components.ContextMenuSubmenu {
			t.Fatalf("右键菜单必须单层平铺，出现子菜单项：%q", entry.item.Label)
		}
	}
}

func TestRunningDisablesMutatingControls(t *testing.T) {
	dir := t.TempDir()
	writeNodesFile(t, dir, "a.yaml", "HK-01")
	seedEntry(t, dir, store.Entry{ID: "e1", Name: "机场 A", Path: "a.yaml"})
	runner := &stagedRunner{ready: make(chan Handlers, 1), finish: make(chan struct{})}
	application, driver := openTestApp(t, dir, runner)

	clickID(t, driver, application.ids.startTest)
	<-runner.ready
	entry := application.entryByID("e1")
	snapshot := driver.Snapshot()
	if !hitDisabled(snapshot, application.ids.newEntry) {
		t.Fatal("测速中新建条目应禁用")
	}
	if !hitDisabled(snapshot, entry.ids.read) || !hitDisabled(snapshot, entry.ids.remove) {
		t.Fatal("测速中读取/删除条目应禁用")
	}
	close(runner.finish)
	waitUntilIdle(t, application, driver)
}

func TestDataDirUnavailableShowsAlertAndDoesNotFallBack(t *testing.T) {
	dir := t.TempDir()
	configPath := writeNodesFile(t, dir, "a.yaml", "HK-01")
	if err := os.WriteFile(filepath.Join(dir, "data"), []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	application, driver := openTestApp(t, dir, newFakeRunner(nil))
	if !hasText(driver.Snapshot(), "数据目录不可用") {
		t.Fatal("数据目录不可用时应显示明确错误")
	}
	clickLabel(t, driver, "确定")
	if application.dialog != dialogNone {
		t.Fatalf("确认后错误对话框应关闭；hits=%v", hitLabels(driver.Snapshot()))
	}
	if !hasText(driver.Snapshot(), "数据目录不可用 ·") {
		t.Fatal("数据目录不可用提示应固定在状态栏")
	}

	// 持久化不可用时仍可操作，但成功文案必须明确「未保存」。
	clickLabel(t, driver, "新建条目")
	focusAndType(t, driver, "名称", "机场 A")
	if err := driver.QueueOpenFiles([]string{configPath}, nil); err != nil {
		t.Fatal(err)
	}
	clickLabel(t, driver, "选择文件…")
	clickID(t, driver, application.ids.entrySave)
	if !hasText(driver.Snapshot(), "未保存") {
		t.Fatalf("数据目录不可用时保存条目应提示未保存；文本=%v", driver.Snapshot().Labels)
	}
	if _, err := os.Stat(filepath.Join(dir, "data", "entries.json")); err == nil {
		t.Fatal("不可写数据目录下不应产生数据文件")
	}
}

func TestIdleAfterRun(t *testing.T) {
	dir := t.TempDir()
	writeNodesFile(t, dir, "a.yaml", "HK-01")
	seedEntry(t, dir, store.Entry{ID: "e1", Name: "机场 A", Path: "a.yaml"})
	application, driver := openTestApp(t, dir, newFakeRunner(nil))
	clickID(t, driver, application.ids.startTest)
	waitUntilIdle(t, application, driver)
	if driver.Transitioning() {
		t.Fatal("测速结束后不应有动画帧待绘制")
	}
	if err := driver.Advance(500 * time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if driver.Transitioning() {
		t.Fatal("空闲时不应有帧调度")
	}
	if application.running() {
		t.Fatal("运行标记应已清零")
	}
}

func BenchmarkPaintNodeList(b *testing.B) {
	benchmarkPaintNodes(b, 300)
}

func BenchmarkPaintSmallList(b *testing.B) {
	benchmarkPaintNodes(b, 20)
}

func BenchmarkPaintEmpty(b *testing.B) {
	dir := b.TempDir()
	application, err := New(Options{BaseDir: dir, Version: "bench", Runner: newFakeRunner(nil)})
	if err != nil {
		b.Fatal(err)
	}
	defer application.Close()
	var canvas ui.Canvas
	size := ui.Size{Width: 1180, Height: 800}
	canvas.SetTextEngine(app.PlatformTextEngine())
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		canvas.Reset()
		application.paint(&canvas, size)
	}
}

func benchmarkPaintNodes(b *testing.B, count int) {
	dir := b.TempDir()
	names := make([]string, 0, count)
	for index := 0; index < count; index++ {
		names = append(names, fmt.Sprintf("节点 %03d", index))
	}
	path := filepath.Join(dir, "a.yaml")
	var builder strings.Builder
	builder.WriteString("proxies:\n")
	for index, name := range names {
		fmt.Fprintf(&builder, "  - name: %q\n    type: ss\n    server: 127.0.0.1\n    port: %d\n    cipher: aes-128-gcm\n    password: fixture\n", name, 30000+index)
	}
	if err := os.WriteFile(path, []byte(builder.String()), 0o644); err != nil {
		b.Fatal(err)
	}
	seedEntryImpl(b, dir, store.Entry{ID: "e1", Name: "机场 A", Path: "a.yaml"})
	application, err := New(Options{BaseDir: dir, Version: "bench", Runner: newFakeRunner(nil)})
	if err != nil {
		b.Fatal(err)
	}
	defer application.Close()

	var canvas ui.Canvas
	size := ui.Size{Width: 1180, Height: 800}
	canvas.SetTextEngine(app.PlatformTextEngine())
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		canvas.Reset()
		application.paint(&canvas, size)
	}
}
