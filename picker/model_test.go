package picker

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/faceair/clash-speedtest/speedtester"
)

func TestSelectionReturnsCheckedConfigsAndAddresses(t *testing.T) {
	model := New(sessionFixture())
	model.checked[0] = true
	model.fetchedFiles = []string{"/opt/sub-1.yaml", "/opt/sub-2.yaml"}
	got := model.Selection()
	want := []string{"/opt/a.yaml", "/opt/sub-1.yaml", "/opt/sub-2.yaml"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestEnterFetchesSubscriptionsThenStarts(t *testing.T) {
	var requested []string
	session := sessionFixture()
	session.Fetch = func(urls []string) ([]string, []string, error) {
		requested = append(requested, urls...)
		return []string{"/opt/sub-1.yaml"}, []string{"https://example.com/a&flag=meta"}, nil
	}
	model := New(session)
	for model.focus != focusAddress {
		updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
		model = updated.(Model)
	}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("https://example.com/a")})
	model = updated.(Model)
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if model.started || !model.fetching {
		t.Fatalf("应先获取: started=%v fetching=%v", model.started, model.fetching)
	}
	done, _ := model.Update(cmd())
	model = done.(Model)
	if !model.started || model.fetching {
		t.Fatalf("获取成功应开始: started=%v fetching=%v", model.started, model.fetching)
	}
	if !reflect.DeepEqual(model.Selection(), []string{"/opt/sub-1.yaml"}) {
		t.Fatalf("Selection = %#v", model.Selection())
	}
	if !strings.Contains(model.status, "flag=meta") {
		t.Fatalf("应提示实际用过的地址: %q", model.status)
	}
	if !reflect.DeepEqual(requested, []string{"https://example.com/a"}) {
		t.Fatalf("requested = %#v", requested)
	}
}

func TestEmptySelectableListStartsOnAddress(t *testing.T) {
	session := Session{Configs: []ConfigEntry{
		{Name: "bad.yaml", Path: "/opt/bad.yaml", Reason: "没有节点"},
	}}
	model := New(session)
	if model.focus != focusAddress {
		t.Fatalf("没有合格配置应停在地址框: %v", model.focus)
	}
}

func TestQuitKeysLeaveWhenIdle(t *testing.T) {
	for _, key := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune{'q'}},
		{Type: tea.KeyEsc},
		{Type: tea.KeyCtrlC},
	} {
		model := New(sessionFixture())
		updated, cmd := model.Update(key)
		got := updated.(Model)
		if !got.quitting || cmd == nil {
			t.Fatalf("key %v did not leave: quitting=%v cmd=%v", key, got.quitting, cmd)
		}
	}
}

func TestQTypesIntoAddressWhenEditing(t *testing.T) {
	model := New(Session{})
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	got := updated.(Model)
	if got.quitting || cmd != nil {
		t.Fatalf("地址里 q 不该退出: quitting=%v", got.quitting)
	}
	if got.address != "q" {
		t.Fatalf("address = %q", got.address)
	}
}

func TestEnterWithBadTimeoutStaysOnThatRow(t *testing.T) {
	model := New(Session{})
	model.address = "https://example.com/a"
	model.options.Timeout = "abc"
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(Model)
	if got.started || got.fetching {
		t.Fatalf("超时填错不该开始: started=%v fetching=%v", got.started, got.fetching)
	}
	if !strings.Contains(got.status, "单请求超时") {
		t.Fatalf("status = %q", got.status)
	}
}

func TestViewShowsCheckAndUnselectableReason(t *testing.T) {
	model := New(sessionFixture())
	model.checked[0] = true
	view := model.View()
	if !strings.Contains(view, "a.yaml") || !strings.Contains(view, "✓") {
		t.Fatalf("合格配置未显示勾选:\n%s", view)
	}
	if !strings.Contains(view, "bad.yaml") || !strings.Contains(view, "不是合法的 yaml") {
		t.Fatalf("不可选配置未显示原因:\n%s", view)
	}
}

func TestSpaceTogglesConfigAsRunes(t *testing.T) {
	// bubbletea 在真实终端里把空格报成 KeyRunes{' '}，不是 KeySpace。
	model := New(sessionFixture())
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	if !updated.(Model).checked[0] {
		t.Fatal("空格应以 KeyRunes 形式勾选当前配置")
	}
}

func TestSpaceAndClickToggleOnlySelectableConfigs(t *testing.T) {
	model := New(sessionFixture())
	model.cursor = 0

	toggled, _ := model.Update(tea.KeyMsg{Type: tea.KeySpace})
	if !toggled.(Model).checked[0] {
		t.Fatal("space did not check the first config")
	}

	clicked, _ := toggled.Update(tea.MouseMsg{
		Action: tea.MouseActionPress, Button: tea.MouseButtonLeft,
		Y: toggled.(Model).computeLayout().paneY + 1,
	})
	if clicked.(Model).checked[0] {
		t.Fatal("click did not uncheck the first config")
	}

	skipped, _ := clicked.Update(tea.KeyMsg{Type: tea.KeyDown})
	skipped, _ = skipped.Update(tea.KeyMsg{Type: tea.KeySpace})
	if skipped.(Model).checked[1] {
		t.Fatal("unselectable config was checked")
	}
}

func TestFastModeDisablesDownloadSizeOnScreen(t *testing.T) {
	model := New(sessionFixture())
	if model.options.Mode != "download" {
		t.Fatalf("默认模式 = %q", model.options.Mode)
	}
	for model.focus != focusOptions {
		updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
		model = updated.(Model)
	}
	// 停在模式行，空格循环：下载→完整→快速。
	for i := 0; i < 2; i++ {
		updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
		model = updated.(Model)
	}
	view := model.View()
	if !strings.Contains(view, "快速") || !strings.Contains(view, "下载大小") {
		t.Fatalf("画面缺少模式或下载大小:\n%s", view)
	}
	if !optionLineDisabled(view, "下载大小") {
		t.Fatalf("快速模式下下载大小应不可用:\n%s", view)
	}
	before := model.options.DownloadSize
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("9")})
	if updated.(Model).options.DownloadSize != before {
		t.Fatal("不可用的下载大小被改了")
	}
}

func TestDownFromModeEditsDownloadSizeOnly(t *testing.T) {
	model := New(sessionFixture())
	for model.focus != focusOptions {
		updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
		model = updated.(Model)
	}
	// 测速模式后依次是过滤正则、屏蔽关键字、下载大小。
	for i := 0; i < 3; i++ {
		updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
		model = updated.(Model)
	}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyLeft})
	model = updated.(Model)
	if model.options.Mode != "download" {
		t.Fatalf("左右键不该改模式: %q", model.options.Mode)
	}
	if model.optionIndex != 2 {
		t.Fatalf("左键应移到上一项: %d", model.optionIndex)
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRight})
	model = updated.(Model)
	if model.optionIndex != 3 {
		t.Fatalf("右键应移回下载大小: %d", model.optionIndex)
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("8")})
	model = updated.(Model)
	if model.options.DownloadSize != "508" {
		t.Fatalf("下载大小 = %q", model.options.DownloadSize)
	}
}

func TestFastModeGreysSpeedOptionsAndEmptyOutputGreysRename(t *testing.T) {
	model := New(sessionFixture())
	model.options.Mode = "fast"
	view := model.View()
	for _, label := range []string{"下载大小", "最低下载速度", "结果图只留有速度", "上传大小", "最低上传速度"} {
		if !optionLineDisabled(view, label) {
			t.Fatalf("%s 应不可用:\n%s", label, view)
		}
	}
	if !optionLineDisabled(view, "重命名") {
		t.Fatal("没有输出路径时重命名应不可用")
	}

	model.options.Mode = "download"
	model.options.OutputPath = "out.yaml"
	view = model.View()
	if optionLineDisabled(view, "下载大小") || optionLineDisabled(view, "重命名") {
		t.Fatalf("下载模式且有输出时不应不可用:\n%s", view)
	}
	if !optionLineDisabled(view, "上传大小") {
		t.Fatal("下载模式上传大小应不可用")
	}
}

func optionLineDisabled(view, label string) bool {
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, label) {
			return strings.Contains(line, "不可用")
		}
	}
	return false
}

func TestTypedSubscriptionStartsWithoutCheckedConfig(t *testing.T) {
	model := New(sessionFixture())
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	updated, _ = updated.Update(tea.KeyMsg{Type: tea.KeyDown})
	updated, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("https://example.com/a")})
	session := sessionFixture()
	session.Fetch = func(urls []string) ([]string, []string, error) {
		return nil, nil, fmt.Errorf("%s: 订阅内容不是 Clash/Mihomo 配置", strings.Join(urls, "，"))
	}
	withFetch := updated.(Model)
	withFetch.session = session
	updated, cmd := withFetch.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(Model)
	if got.started || !got.fetching || !strings.Contains(got.status, "正在获取") {
		t.Fatalf("填了地址应先获取，started=%v fetching=%v status=%q", got.started, got.fetching, got.status)
	}

	failed, _ := got.Update(cmd())
	got = failed.(Model)
	if got.started || got.fetching || !strings.Contains(got.status, "https://example.com/a") {
		t.Fatalf("获取失败应留下并说明: started=%v fetching=%v status=%q", got.started, got.fetching, got.status)
	}
}

func TestEnterWithoutSourceStaysAndExplains(t *testing.T) {
	model := New(sessionFixture())
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(Model)
	if got.started {
		t.Fatal("started without a source")
	}
	if got.status == "" {
		t.Fatal("missing status")
	}
}

func TestEnterWithBadFilterStaysOnThatField(t *testing.T) {
	model := New(sessionFixture())
	model.checked[0] = true
	model.options.Filter = "("
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(Model)
	if got.started {
		t.Fatal("started with an invalid filter")
	}
	if !strings.Contains(got.status, "过滤") {
		t.Fatalf("status = %q", got.status)
	}
}

func TestFetchingQuitCancelsAndLeaves(t *testing.T) {
	model := New(sessionFixture())
	model.fetching = true
	keys := []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune{'q'}},
		{Type: tea.KeyEsc},
		{Type: tea.KeyCtrlC},
	}
	for _, key := range keys {
		updated, cmd := model.Update(key)
		got := updated.(Model)
		if !got.quitting || cmd == nil {
			t.Fatalf("key %v did not leave: quitting=%v cmd=%v", key, got.quitting, cmd)
		}
	}
}

func TestViewTruncatesEveryLineToWidth(t *testing.T) {
	model := New(Session{})
	model.address = "https://example.com/very-long-subscription-address-with-lots-of-characters"
	model.options.ServerURL = "https://example.com/very-long-server-url-with-many-many-characters"
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 40, Height: 40})
	model = updated.(Model)
	for i, line := range strings.Split(model.View(), "\n") {
		if w := lipgloss.Width(line); w > 40 {
			t.Fatalf("第 %d 行超宽 %d:\n%s", i, w, line)
		}
	}
}

func TestFocusedRowStaysVisibleOnSmallTerminal(t *testing.T) {
	model := New(sessionFixture())
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
	model = updated.(Model)
	// 沿环形导航走到最后一个选项，视野应一路跟上。
	for steps := 0; steps < 60 && (model.focus != focusOptions || model.optionIndex != len(optionOrder)-1); steps++ {
		updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
		model = updated.(Model)
	}
	if model.focus != focusOptions || model.optionIndex != len(optionOrder)-1 {
		t.Fatalf("导航没走到最后一个选项: focus=%v index=%d", model.focus, model.optionIndex)
	}
	view := model.View()
	if !strings.Contains(view, "拉取订阅 UA") {
		t.Fatalf("光标行应滚动到视野内:\n%s", view)
	}
	if strings.Contains(view, "▎ 选项") {
		t.Fatalf("滚出视野的节标题不应还画着:\n%s", view)
	}
}

func TestWheelScrollsFocusPaneWithoutMovingCursor(t *testing.T) {
	model := New(sessionFixture())
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
	model = updated.(Model)
	for model.focus != focusOptions {
		updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
		model = updated.(Model)
	}
	lo := model.computeLayout()
	scrolled, _ := model.Update(tea.MouseMsg{
		Button: tea.MouseButtonWheelDown,
		Y:      lo.paneY + 2, // 选项栏的行
	})
	model = scrolled.(Model)
	view := model.View()
	if strings.Contains(view, "测速模式") {
		t.Fatalf("滚轮后选项栏首行应滚出视野:\n%s", view)
	}
	if model.optionIndex != 0 {
		t.Fatalf("滚轮不该动光标: index=%d", model.optionIndex)
	}
}

func TestClickFocusesOptionRowAtLayoutPosition(t *testing.T) {
	model := New(sessionFixture())
	lo := model.computeLayout()
	clicked, _ := model.Update(tea.MouseMsg{
		Action: tea.MouseActionPress, Button: tea.MouseButtonLeft,
		X: lo.optionsX + 2, Y: lo.paneY + 2, // 节标题后第二行 = 选项栏第 1 项
	})
	got := clicked.(Model)
	if got.focus != focusOptions || got.optionIndex != 1 {
		t.Fatalf("点击选项行应定位到该行: focus=%v index=%d", got.focus, got.optionIndex)
	}
}

func TestNumericOptionRejectsNonDigitsWhileTyping(t *testing.T) {
	model := New(Session{})
	model.focus = focusOptions
	model.optionIndex = optionIndexFor(OptionDownloadSize)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("9a9")})
	if got := updated.(Model).options.DownloadSize; got != "5099" {
		t.Fatalf("数字项应拦掉字母: %q", got)
	}

	model = updated.(Model)
	model.optionIndex = optionIndexFor(OptionTimeout)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("1s5x")})
	if got := updated.(Model).options.Timeout; got != "5s1s5" {
		t.Fatalf("时长项应只留数字和单位字母: %q", got)
	}

	model = updated.(Model)
	model.optionIndex = optionIndexFor(OptionFilter)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("HK|港")})
	if got := updated.(Model).options.Filter; got != ".+HK|港" {
		t.Fatalf("正则项不该拦字符: %q", got)
	}
}

func optionIndexFor(option Option) int {
	for i, row := range optionOrder {
		if row.option == option {
			return i
		}
	}
	panic("未知的选项")
}

func TestEnterTriggersOnStart(t *testing.T) {
	var gotRequest StartRequest
	session := sessionFixture()
	session.OnStart = func(req StartRequest) tea.Cmd {
		gotRequest = req
		return func() tea.Msg { return TestingStartedMsg{Total: 3} }
	}
	model := New(session)
	model.checked[0] = true
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(Model)
	if !got.started || !got.loading {
		t.Fatalf("回车应进入加载: started=%v loading=%v", got.started, got.loading)
	}
	if cmd == nil {
		t.Fatal("应返回 OnStart 的 Cmd")
	}
	started, _ := got.Update(cmd())
	got = started.(Model)
	if !got.testing || got.loading {
		t.Fatalf("OnStart 完成应进入测试: testing=%v loading=%v", got.testing, got.loading)
	}
	if got.total != 3 {
		t.Fatalf("total = %d", got.total)
	}
	if len(gotRequest.Selection) == 0 || gotRequest.Selection[0] != "/opt/a.yaml" {
		t.Fatalf("OnStart 应收到勾选的配置: %#v", gotRequest.Selection)
	}
	if gotRequest.Options.Mode != "download" {
		t.Fatalf("OnStart 应收到选项: %q", gotRequest.Options.Mode)
	}
}

type picker_StartRequest = StartRequest

func TestLoadFailureReturnsToEditable(t *testing.T) {
	session := sessionFixture()
	session.OnStart = func(StartRequest) tea.Cmd {
		return func() tea.Msg { return LoadFailedMsg{Err: fmt.Errorf("加载节点失败: 坏文件")} }
	}
	model := New(session)
	model.checked[0] = true
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	failed, _ := updated.(Model).Update(cmd())
	got := failed.(Model)
	if got.started || got.loading || got.testing {
		t.Fatalf("加载失败应回到选源: started=%v loading=%v testing=%v", got.started, got.loading, got.testing)
	}
	if !strings.Contains(got.status, "坏文件") {
		t.Fatalf("status = %q", got.status)
	}
	// 回到选源后还能重新勾选。
	toggled, _ := got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	if toggled.(Model).checked[0] || toggled.(Model).checked[1] {
		t.Fatal("加载失败后应仍可编辑勾选")
	}
}

func TestResultsFlowIntoRecordTable(t *testing.T) {
	model := New(sessionFixture())
	model.checked[0] = true
	model.testing = true
	model.total = 2
	r1 := &speedtester.Result{ProxyName: "JP|01", Latency: 100 * time.Millisecond, DownloadSpeed: 1024 * 1024}
	r2 := &speedtester.Result{ProxyName: "US|02", Latency: 200 * time.Millisecond, DownloadError: "超时"}
	updated, _ := model.Update(TestResultMsg{Result: r1})
	model = updated.(Model)
	updated, _ = model.Update(TestResultMsg{Result: r2})
	model = updated.(Model)
	view := model.View()
	if !strings.Contains(view, "测试记录") || !strings.Contains(view, "已测 2/2") {
		t.Fatalf("记录表缺少进度:\n%s", view)
	}
	if !strings.Contains(view, "JP|01") || !strings.Contains(view, "100ms") || !strings.Contains(view, "1.00MB/s") {
		t.Fatalf("记录行未画出:\n%s", view)
	}
	if !strings.Contains(view, "超时") {
		t.Fatalf("失败结果未画出:\n%s", view)
	}

	done, _ := model.Update(TestDoneMsg{})
	model = done.(Model)
	if !model.TestDone() || model.testing {
		t.Fatalf("测试应完成: done=%v testing=%v", model.TestDone(), model.testing)
	}
	if !strings.Contains(model.View(), "完成 2/2") {
		t.Fatalf("完成后应显示汇总:\n%s", model.View())
	}
	// Results 交给调用方导出。
	if len(model.Results()) != 2 || model.Results()[0] != r1 {
		t.Fatalf("Results = %#v", model.Results())
	}
}

func TestTestingPhaseIgnoresEditingButScrollsAndQuits(t *testing.T) {
	model := New(sessionFixture())
	model.checked[0] = true
	model.testing = true
	model.total = 5
	for i := 0; i < 8; i++ {
		model.records = append(model.records, &speedtester.Result{ProxyName: fmt.Sprintf("节点%02d", i)})
	}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	got := updated.(Model)
	if got.cursor != 0 || got.optionIndex != 0 || got.recordScroll == 0 {
		t.Fatalf("测试中 ↑↓ 应滚记录表而不是编辑: cursor=%d index=%d recordScroll=%d", got.cursor, got.optionIndex, got.recordScroll)
	}
	edited, _ := got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if edited.(Model).address != "" {
		t.Fatal("测试中不应接受输入")
	}
	quitted, cmd := edited.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if !quitted.(Model).quitting || cmd == nil {
		t.Fatal("测试中 q 应退出")
	}
}

func sessionFixture() Session {
	return Session{
		Configs: []ConfigEntry{
			{Name: "a.yaml", Path: "/opt/a.yaml", Selectable: true},
			{Name: "bad.yaml", Path: "/opt/bad.yaml", Reason: "不是合法的 yaml"},
		},
	}
}
