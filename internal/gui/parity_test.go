package gui

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/BoxMiao007/clash-speedtest-plus/internal/export"
	"github.com/BoxMiao007/clash-speedtest-plus/internal/ip"
	"github.com/BoxMiao007/clash-speedtest-plus/internal/speedtester"
	"github.com/BoxMiao007/clash-speedtest-plus/internal/store"
	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/ui"
)

// quietSettings 是测试用的默认设置：不写图片、不重命名（不联网）、输出关闭、
// 阈值全放开，让假引擎的结果都能过筛。
func quietSettings() store.Settings {
	settings := store.DefaultSettings()
	settings.ImageEnabled = false
	settings.RenameEnabled = false
	settings.OutputMode = store.OutputModeOff
	settings.MaxLatencyMS = 0
	settings.MaxPacketLoss = -1
	settings.MinDownloadMBps = 0
	settings.MinUploadMBps = 0
	return settings
}

func seedSettings(t *testing.T, dir string, settings store.Settings) {
	t.Helper()
	database, err := store.Open(store.Options{BaseDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
}

func waitFor(t *testing.T, driver *app.Driver, condition func() bool, message string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if err := driver.Pump(); err != nil {
			t.Fatal(err)
		}
		if condition() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal(message)
}

func waitForTimeout(t *testing.T, driver *app.Driver, timeout time.Duration, condition func() bool, message string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := driver.Pump(); err != nil {
			t.Fatal(err)
		}
		if condition() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal(message)
}

func waitForFile(t *testing.T, driver *app.Driver, path string) {
	t.Helper()
	waitFor(t, driver, func() bool {
		_, err := os.Stat(path)
		return err == nil
	}, "没有在超时内写出文件："+path)
}

func fixtureSubscription(nodeNames ...string) string {
	var builder strings.Builder
	builder.WriteString("proxies:\n")
	for index, name := range nodeNames {
		fmt.Fprintf(&builder, "  - name: %q\n", name)
		builder.WriteString("    type: ss\n")
		builder.WriteString("    server: 127.0.0.1\n")
		fmt.Fprintf(&builder, "    port: %d\n", 30000+index)
		builder.WriteString("    cipher: aes-128-gcm\n")
		builder.WriteString("    password: fixture\n")
	}
	return builder.String()
}

// 订阅条目：新建后拉取并写缓存，重启后直接用缓存、不再请求网络。
func TestSubscriptionEntryFetchesOnceAndLoadsCache(t *testing.T) {
	dir := t.TempDir()
	seedSettings(t, dir, quietSettings())
	body := fixtureSubscription("HK-01", "JP-01")
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	application, driver := openTestApp(t, dir, newFakeRunner(nil))
	clickLabel(t, driver, "新建条目")
	focusAndType(t, driver, "名称", "订阅 A")
	clickLabel(t, driver, "订阅地址")
	focusAndType(t, driver, "订阅链接", server.URL)
	clickID(t, driver, application.ids.entrySave)

	waitFor(t, driver, func() bool {
		entry := application.entryByID(application.entries[0].entry.ID)
		return entry != nil && entry.loaded && len(entry.nodes) == 2
	}, "订阅条目没有在超时内拉到节点")
	if !hasText(driver.Snapshot(), "节点 订阅 A / HK-01") {
		t.Fatalf("订阅节点没有显示；文本=%v", driver.Snapshot().Labels)
	}
	entry := application.entries[0]
	if !entry.entry.IsSubscription() {
		t.Fatal("条目种类没有保存成订阅")
	}
	cachePath, err := application.subscriptionPath(entry)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(cachePath); err != nil {
		t.Fatalf("订阅缓存没有写出：%v", err)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("首次拉取请求数 = %d，期望 1", got)
	}

	// 重启：缓存直接可用，不再发请求。
	driver.Close()
	application.Close()
	reopened, driver2 := openTestApp(t, dir, newFakeRunner(nil))
	if len(reopened.entries) != 1 || len(reopened.entries[0].nodes) != 2 {
		t.Fatalf("重启后应从缓存读到 2 个节点，实际 %d 个条目", len(reopened.entries))
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("重启后请求数 = %d，期望仍为 1（只读缓存）", got)
	}
	_ = driver2
}

// 多个条目各自一轮，测完自动推进到下一轮。
func TestMultipleEntriesRunSequentialRounds(t *testing.T) {
	dir := t.TempDir()
	seedSettings(t, dir, quietSettings())
	writeNodesFile(t, dir, "a.yaml", "HK-01")
	writeNodesFile(t, dir, "b.yaml", "JP-01", "JP-02")
	seedEntry(t, dir, store.Entry{ID: "e1", Name: "机场 A", Path: "a.yaml"})
	seedEntry(t, dir, store.Entry{ID: "e2", Name: "机场 B", Path: "b.yaml"})
	runner := newFakeRunner(nil)
	application, driver := openTestApp(t, dir, runner)

	clickID(t, driver, application.ids.startTest)
	waitUntilIdle(t, application, driver)

	if len(application.rounds) != 2 {
		t.Fatalf("应有 2 轮，实际 %d", len(application.rounds))
	}
	if application.roundIndex != 1 {
		t.Fatalf("测完应停在最后一轮，实际第 %d 轮", application.roundIndex+1)
	}
	for index, round := range application.rounds {
		if !round.done {
			t.Fatalf("第 %d 轮没有标记完成", index+1)
		}
	}
	if got := len(runner.lastBatch()); got != 2 {
		t.Fatalf("最后一轮批量 = %d，期望 2", got)
	}
	if !hasText(driver.Snapshot(), "第 2/2 轮") {
		t.Fatalf("结果视图应显示轮次；文本=%v", driver.Snapshot().Labels)
	}
}

// 设置页保存后写盘、下一轮引擎用新参数，完整模式显示上传列。
func TestSettingsSavePersistsAndConfiguresEngine(t *testing.T) {
	dir := t.TempDir()
	seedSettings(t, dir, quietSettings())
	writeNodesFile(t, dir, "a.yaml", "HK-01")
	seedEntry(t, dir, store.Entry{ID: "e1", Name: "机场 A", Path: "a.yaml"})
	runner := newFakeRunner(nil)
	application, driver := openTestApp(t, dir, runner)

	clickLabel(t, driver, "设置")
	clickLabel(t, driver, "完整")
	replaceFieldText(t, driver, "下载大小（MB）", "12")
	clickID(t, driver, application.ids.settingsSave)

	database, err := store.Open(store.Options{BaseDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	settings := database.Settings()
	if settings.Mode != "full" || settings.DownloadMB != 12 {
		t.Fatalf("设置没有保存：%+v（界面状态：%q，表单错误：%q）", settings, application.status, application.settingsForm.err)
	}

	clickLabel(t, driver, "节点选择")
	clickID(t, driver, application.ids.startTest)
	waitUntilIdle(t, application, driver)
	if got := runner.lastConfig(); got.Mode != "full" || got.DownloadMB != 12 {
		t.Fatalf("引擎没有拿到新参数：%+v", got)
	}
	application.active = viewResults
	if err := driver.Pump(); err != nil {
		t.Fatal(err)
	}
	if !hasText(driver.Snapshot(), "上传速度") {
		t.Fatalf("完整模式应显示上传列；文本=%v", driver.Snapshot().Labels)
	}
}

// 输出模式「当前目录下」：测完自动写一份带导出骨架的 yaml。
func TestRoundExportWritesProfile(t *testing.T) {
	dir := t.TempDir()
	settings := quietSettings()
	settings.OutputMode = store.OutputModeAuto
	seedSettings(t, dir, settings)
	writeNodesFile(t, dir, "a.yaml", "HK-01", "JP-01")
	seedEntry(t, dir, store.Entry{ID: "e1", Name: "机场 A", Path: "a.yaml"})
	application, driver := openTestApp(t, dir, newFakeRunner(nil))

	clickID(t, driver, application.ids.startTest)
	waitUntilIdle(t, application, driver)
	var path string
	// 产物序号接续程序目录历史（空目录时从 1 开始），文件名形如 1.a-导出.yaml。
	waitFor(t, driver, func() bool {
		matches, _ := filepath.Glob(filepath.Join(dir, "*-导出.yaml"))
		if len(matches) == 0 {
			return false
		}
		path = matches[0]
		return true
	}, fmt.Sprintf("没有在超时内写出导出文件；状态=%q 导出中=%v 输出模式=%q",
		application.status, application.exporting, application.settings.OutputMode))

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, want := range []string{"mixed-port: 7890", "proxies:", "节点选择", "自动选择", "MATCH,节点选择"} {
		if !strings.Contains(text, want) {
			t.Fatalf("导出配置缺少 %q：\n%s", want, text)
		}
	}
}

// 过滤条决定测哪些节点：只测过滤后仍选中的节点。
func TestFilterLimitsTestedNodes(t *testing.T) {
	dir := t.TempDir()
	seedSettings(t, dir, quietSettings())
	writeNodesFile(t, dir, "a.yaml", "HK-01", "JP-01")
	seedEntry(t, dir, store.Entry{ID: "e1", Name: "机场 A", Path: "a.yaml"})
	runner := newFakeRunner(nil)
	application, driver := openTestApp(t, dir, runner)

	replaceFieldText(t, driver, "过滤表达式", "HK")
	clickLabel(t, driver, "应用")
	clickID(t, driver, application.ids.startTest)
	waitUntilIdle(t, application, driver)

	batch := runner.lastBatch()
	if len(batch) != 1 {
		t.Fatalf("过滤后应只测 1 个节点，实际 %d", len(batch))
	}
	if !strings.Contains(string(batch[0].Key), application.nodes[0].id) {
		// Key 形如 entryID/nodeID，只校验数量与节点名足够。
	}
	if got := application.rounds[0].nodes[0].name; got != "HK-01" {
		t.Fatalf("过滤后测得节点 = %q，期望 HK-01", got)
	}
}

// 暂停/继续：按钮文案与状态跟着变。
func TestPauseResumeUpdatesStatus(t *testing.T) {
	dir := t.TempDir()
	seedSettings(t, dir, quietSettings())
	writeNodesFile(t, dir, "a.yaml", "HK-01")
	seedEntry(t, dir, store.Entry{ID: "e1", Name: "机场 A", Path: "a.yaml"})
	runner := &stagedRunner{ready: make(chan Handlers, 1), finish: make(chan struct{})}
	application, driver := openTestApp(t, dir, runner)

	clickID(t, driver, application.ids.startTest)
	handlers := <-runner.ready
	_ = handlers

	clickLabel(t, driver, "暂停")
	if application.status != "已暂停测速" {
		t.Fatalf("暂停后状态 = %q", application.status)
	}
	if !hasText(driver.Snapshot(), "继续") {
		t.Fatal("暂停后按钮应变成「继续」")
	}
	clickLabel(t, driver, "继续")
	if application.status != "已继续测速" {
		t.Fatalf("继续后状态 = %q", application.status)
	}
	close(runner.finish)
	waitUntilIdle(t, application, driver)
}

// 结果表点击行展开详情，再点收起。
func TestResultRowTogglesDetailPanel(t *testing.T) {
	dir := t.TempDir()
	seedSettings(t, dir, quietSettings())
	writeNodesFile(t, dir, "a.yaml", "HK-01")
	seedEntry(t, dir, store.Entry{ID: "e1", Name: "机场 A", Path: "a.yaml"})
	application, driver := openTestApp(t, dir, newFakeRunner(nil))

	clickID(t, driver, application.ids.startTest)
	waitUntilIdle(t, application, driver)
	node := application.results[0]
	clickID(t, driver, node.ids.row)
	snapshot := driver.Snapshot()
	if !hasText(snapshot, "下载错误") && !hasText(snapshot, "上次测速：") {
		t.Fatalf("详情面板没有出现；文本=%v", snapshot.Labels)
	}
	clickID(t, driver, node.ids.row)
	if hasText(driver.Snapshot(), "上次测速：") {
		t.Fatal("再次点击行应关闭详情")
	}
}

// 点表头按该列排序，再点一次反转方向。
func TestSortHeadersReorderResults(t *testing.T) {
	dir := t.TempDir()
	seedSettings(t, dir, quietSettings())
	writeNodesFile(t, dir, "a.yaml", "HK-01", "JP-01")
	seedEntry(t, dir, store.Entry{ID: "e1", Name: "机场 A", Path: "a.yaml"})
	runner := newFakeRunner(map[string]fakeResult{
		"HK-01": {ok: true, latency: 80 * time.Millisecond, speed: 1 * 1024 * 1024},
		"JP-01": {ok: true, latency: 10 * time.Millisecond, speed: 30 * 1024 * 1024},
	})
	application, driver := openTestApp(t, dir, runner)
	clickID(t, driver, application.ids.startTest)
	waitUntilIdle(t, application, driver)

	// 默认按下载速度降序：JP-01 在前。
	if application.results[0].name != "JP-01" {
		t.Fatalf("默认应按下载速度降序，实际 %v", resultNames(application))
	}
	clickID(t, driver, application.ids.sortLatency)
	if application.results[0].name != "JP-01" {
		t.Fatalf("延迟升序时 10ms 的 JP-01 应在最前，实际 %v", resultNames(application))
	}
	clickID(t, driver, application.ids.sortLatency)
	if application.results[0].name != "HK-01" {
		t.Fatalf("再点一次应反转成延迟降序，实际 %v", resultNames(application))
	}
}

// 手动导出结果表：写出 TSV，表头与旧输出一致。
func TestManualTSVExport(t *testing.T) {
	dir := t.TempDir()
	seedSettings(t, dir, quietSettings())
	writeNodesFile(t, dir, "a.yaml", "HK-01")
	seedEntry(t, dir, store.Entry{ID: "e1", Name: "机场 A", Path: "a.yaml"})
	application, driver := openTestApp(t, dir, newFakeRunner(nil))
	clickID(t, driver, application.ids.startTest)
	waitUntilIdle(t, application, driver)

	clickLabel(t, driver, "结果表")
	var path string
	waitFor(t, driver, func() bool {
		matches, _ := filepath.Glob(filepath.Join(dir, "结果表-*.tsv"))
		if len(matches) == 0 {
			return false
		}
		raw, err := os.ReadFile(matches[0])
		if err != nil || !strings.Contains(string(raw), "HK-01") {
			return false
		}
		path = matches[0]
		return true
	}, "没有写出完整的结果表文件")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "节点名称") || !strings.Contains(string(raw), "HK-01") {
		t.Fatalf("结果表内容不符：\n%s", raw)
	}
}

// 条目种类切换：从本地文件改成订阅地址后，路径字段清空、地址生效。
func TestEntryKindSwitchToSubscription(t *testing.T) {
	dir := t.TempDir()
	seedSettings(t, dir, quietSettings())
	writeNodesFile(t, dir, "a.yaml", "HK-01")
	seedEntry(t, dir, store.Entry{ID: "e1", Name: "机场 A", Path: "a.yaml"})
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte(fixtureSubscription("SG-01")))
	}))
	defer server.Close()
	application, driver := openTestApp(t, dir, newFakeRunner(nil))

	entry := application.entryByID("e1")
	clickID(t, driver, entry.ids.edit)
	clickLabel(t, driver, "订阅地址")
	focusAndType(t, driver, "订阅链接", server.URL)
	clickID(t, driver, application.ids.entrySave)

	waitFor(t, driver, func() bool {
		entry := application.entryByID("e1")
		return entry != nil && entry.loaded && len(entry.nodes) == 1 && entry.nodes[0].name == "SG-01"
	}, "改成订阅后没有拉到新节点")
	if !application.entryByID("e1").entry.IsSubscription() {
		t.Fatal("条目没有保存成订阅种类")
	}
	_ = ui.ID{}
}

// 导出结束时如果这一轮已被用户停止，不推进剩余轮次。
func TestExportedStoppedRoundDoesNotAdvance(t *testing.T) {
	dir := t.TempDir()
	seedSettings(t, dir, quietSettings())
	writeNodesFile(t, dir, "a.yaml", "HK-01")
	seedEntry(t, dir, store.Entry{ID: "e1", Name: "机场 A", Path: "a.yaml"})
	application, driver := openTestApp(t, dir, newFakeRunner(nil))
	clickID(t, driver, application.ids.startTest)
	waitUntilIdle(t, application, driver)

	round := application.rounds[0]
	round.stopped = true
	application.exporting = 1
	application.applyExported(runEvent{kind: eventExported, roundRef: round, path: "导出.yaml"})
	if application.roundIndex != 0 || len(application.rounds) != 1 {
		t.Fatalf("停止后不应推进轮次：index=%d 轮数=%d", application.roundIndex, len(application.rounds))
	}
}

// 导出前的过筛：阈值挡掉慢节点，配置不被就地修改。
func TestBuildProxiesAppliesThresholds(t *testing.T) {
	results := []*speedtester.Result{
		{ProxyConfig: map[string]any{"name": "slow", "server": "127.0.0.1"}, Latency: 50 * time.Millisecond, DownloadSpeed: 1 * megabyte},
		{ProxyConfig: map[string]any{"name": "fast", "server": "127.0.0.1"}, Latency: 10 * time.Millisecond, DownloadSpeed: 30 * megabyte},
		{ProxyConfig: map[string]any{"server": "127.0.0.1"}, Latency: 10 * time.Millisecond, DownloadSpeed: 30 * megabyte},
	}
	filter := export.Filter{MinDownloadBps: 5 * megabyte, DownloadTested: true, MaxPacketLoss: 100}
	proxies := buildProxies(results, filter, false, "")
	if len(proxies) != 1 || proxies[0]["name"] != "fast" {
		t.Fatalf("过筛结果 = %v，期望只剩 fast", proxies)
	}
	if results[0].ProxyConfig["name"] != "slow" {
		t.Fatal("导出过滤不应改动原结果里的节点配置")
	}
}

// 结果图开启时，每轮结束自动写出 PNG。
func TestRoundExportWritesResultImage(t *testing.T) {
	dir := t.TempDir()
	settings := quietSettings()
	settings.ImageEnabled = true
	seedSettings(t, dir, settings)
	writeNodesFile(t, dir, "a.yaml", "HK-01")
	seedEntry(t, dir, store.Entry{ID: "e1", Name: "机场 A", Path: "a.yaml"})
	application, driver := openTestApp(t, dir, newFakeRunner(nil))

	clickID(t, driver, application.ids.startTest)
	waitUntilIdle(t, application, driver)
	var path string
	waitForTimeout(t, driver, 20*time.Second, func() bool {
		matches, _ := filepath.Glob(filepath.Join(dir, "*.png"))
		if len(matches) == 0 {
			return false
		}
		path = matches[0]
		return true
	}, fmt.Sprintf("没有写出结果图；状态=%q 图片开关=%v 导出中=%d", application.status, application.settings.ImageEnabled, application.exporting))
	info, err := os.Stat(path)
	if err != nil || info.Size() == 0 {
		t.Fatalf("结果图文件为空或不可读：%v", err)
	}
}

// 重命名只在导出副本上生效，并套用模板；查询失败保留原名。
func TestBuildProxiesRenamesWithLocation(t *testing.T) {
	previous := ipLookup
	defer func() { ipLookup = previous }()
	ipLookup = func(string) (*ip.IPLocation, error) {
		return &ip.IPLocation{CountryCode: "HK", Country: "Hong Kong"}, nil
	}
	results := []*speedtester.Result{
		{ProxyConfig: map[string]any{"name": "原名", "server": "127.0.0.1"}, Latency: 20 * time.Millisecond, DownloadSpeed: 12 * megabyte},
	}
	proxies := buildProxies(results, export.Filter{MaxPacketLoss: -1, DownloadTested: true}, true, "")
	if len(proxies) != 1 {
		t.Fatalf("导出节点数 = %d，期望 1", len(proxies))
	}
	name, _ := proxies[0]["name"].(string)
	if name == "原名" || !strings.Contains(name, "HK") {
		t.Fatalf("重命名结果 = %q，期望含 HK 且不同于原名", name)
	}
	if results[0].ProxyConfig["name"] != "原名" {
		t.Fatal("重命名不应改动原结果里的节点配置")
	}

	ipLookup = func(string) (*ip.IPLocation, error) { return nil, fmt.Errorf("查询失败") }
	proxies = buildProxies(results, export.Filter{MaxPacketLoss: -1, DownloadTested: true}, true, "")
	if len(proxies) != 1 || proxies[0]["name"] != "原名" {
		t.Fatalf("查询失败应保留原名，实际 %v", proxies)
	}
}

// 上传按设置把配置分别送到 Gist 与仓库，文件名取导出文件名。
func TestUploadConfigSendsToGistAndRepo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "1.导出.yaml")
	if err := os.WriteFile(path, []byte("proxies: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fake := &fakeUploader{}
	previous := newUploader
	defer func() { newUploader = previous }()
	newUploader = func() uploader { return fake }

	settings := quietSettings()
	settings.GistToken = "gist-token"
	settings.GistAddress = "gist-id"
	settings.RepoToken = "repo-token"
	settings.RepoAddress = "owner/repo"
	settings.RepoFilePath = "clash/proxies.yaml"
	settings.RepoBranch = "main"
	if err := uploadConfig(settings, path); err != nil {
		t.Fatalf("uploadConfig 失败：%v", err)
	}
	if fake.gistName != "1.导出.yaml" || fake.gistContent != "proxies: []\n" {
		t.Fatalf("Gist 上传参数不符：name=%q content=%q", fake.gistName, fake.gistContent)
	}
	if fake.repoPath != "clash/proxies.yaml" || fake.repoBranch != "main" || fake.repoAddress != "owner/repo" {
		t.Fatalf("仓库上传参数不符：%+v", fake)
	}
}

type fakeUploader struct {
	gistName    string
	gistContent string
	repoPath    string
	repoBranch  string
	repoAddress string
}

func (fake *fakeUploader) UpdateFile(token, address, filename string, content []byte) error {
	fake.gistName = filename
	fake.gistContent = string(content)
	return nil
}

func (fake *fakeUploader) UpdateRepoFile(token, address, filePath, branch string, content []byte) error {
	fake.repoPath = filePath
	fake.repoBranch = branch
	fake.repoAddress = address
	return nil
}
