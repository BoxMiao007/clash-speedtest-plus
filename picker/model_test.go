package picker

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
	done, quitCmd := model.Update(cmd())
	model = done.(Model)
	if !model.started || model.fetching {
		t.Fatalf("获取成功应开始: started=%v fetching=%v", model.started, model.fetching)
	}
	if quitCmd == nil {
		t.Fatal("回车确认后应返回 tea.Quit 结束选源界面")
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
	} {
		model := New(sessionFixture())
		updated, cmd := model.Update(key)
		got := updated.(Model)
		if got.quitting || cmd != nil {
			t.Fatalf("key %v 不该退出: quitting=%v cmd=%v", key, got.quitting, cmd)
		}
	}
	model := New(sessionFixture())
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if got := updated.(Model); !got.quitting || cmd == nil {
		t.Fatalf("Ctrl+C 应退出: quitting=%v cmd=%v", got.quitting, cmd)
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

func TestArrowKeysAdjustOptionValues(t *testing.T) {
	model := New(sessionFixture())
	for model.focus != focusOptions {
		updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
		model = updated.(Model)
	}
	// 停在模式行：左右键循环切换模式。
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if got := updated.(Model).options.Mode; got != "fast" {
		t.Fatalf("左键应切到快速: %q", got)
	}
	updated, _ = updated.(Model).Update(tea.KeyMsg{Type: tea.KeyRight})
	if got := updated.(Model).options.Mode; got != "download" {
		t.Fatalf("右键应切回下载: %q", got)
	}
	model = updated.(Model)

	// 下到下载大小：按步长 5MB 增减，打到边界不再动。
	for i := 0; i < 3; i++ {
		updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
		model = updated.(Model)
	}
	if model.currentOption().option != OptionDownloadSize {
		t.Fatalf("导航应停在下载大小: %d", model.optionIndex)
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRight})
	if got := updated.(Model).options.DownloadSize; got != "55" {
		t.Fatalf("右键应 +5MB: %q", got)
	}
	updated, _ = updated.(Model).Update(tea.KeyMsg{Type: tea.KeyLeft})
	if got := updated.(Model).options.DownloadSize; got != "50" {
		t.Fatalf("左键应 -5MB: %q", got)
	}

	// 值被手改坏时左右键不动，回车校验会指出这一行。
	broken := updated.(Model)
	broken.options.DownloadSize = "abc"
	if got, _ := broken.Update(tea.KeyMsg{Type: tea.KeyRight}); got.(Model).options.DownloadSize != "abc" {
		t.Fatal("值不合法时左右键不该改值")
	}

	// 文本项（过滤正则）左右键无操作。
	for i := 0; i < 2; i++ {
		updated, _ = updated.(Model).Update(tea.KeyMsg{Type: tea.KeyUp})
		model = updated.(Model)
	}
	if model.currentOption().option != OptionFilter {
		t.Fatalf("导航应停在过滤正则: %d", model.optionIndex)
	}
	if got, _ := model.Update(tea.KeyMsg{Type: tea.KeyRight}); got.(Model).options.Filter != ".+" {
		t.Fatalf("文本项左右键不该改值: %q", got.(Model).options.Filter)
	}

	// Tab 沿环往回走：选项栏头再 Tab 到地址行。
	for model.optionIndex != 0 {
		updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyUp})
		model = updated.(Model)
	}
	if got, _ := model.Update(tea.KeyMsg{Type: tea.KeyTab}); got.(Model).focus != focusAddress {
		t.Fatalf("Tab 应从选项栏头回到地址行: %v", got.(Model).focus)
	}
}

func (m Model) currentOption() optionRow {
	return optionOrder[m.optionIndex]
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
	// 获取中 q 和 Esc 不再是退出键，只有 Ctrl+C 取消。
	model := New(sessionFixture())
	model.fetching = true
	for _, key := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune{'q'}},
		{Type: tea.KeyEsc},
	} {
		updated, cmd := model.Update(key)
		got := updated.(Model)
		if got.quitting || cmd != nil || !got.fetching {
			t.Fatalf("key %v 不该取消获取: quitting=%v cmd=%v", key, got.quitting, cmd)
		}
	}
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	got := updated.(Model)
	if !got.quitting || cmd == nil {
		t.Fatalf("Ctrl+C 应取消获取: quitting=%v cmd=%v", got.quitting, cmd)
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

func TestEnterWithCheckedConfigQuits(t *testing.T) {
	model := New(sessionFixture())
	// 空格勾上第一个配置（真实终端里空格以 KeyRunes 形式到达）。
	toggled, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	model = toggled.(Model)
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(Model)
	if !got.Started() {
		t.Fatal("勾选后回车应确认开始")
	}
	if cmd == nil {
		t.Fatal("回车确认后应返回 tea.Quit 结束选源界面")
	}
}

func TestUpArrowWrapsTheFullRing(t *testing.T) {
	model := New(sessionFixture()) // 光标停在 configs[0]
	// 文件头 ↑ 绕环到选项栏末项。
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyUp})
	got := updated.(Model)
	if got.focus != focusOptions || got.optionIndex != len(optionOrder)-1 {
		t.Fatalf("文件头 ↑ 应绕到选项栏末项: focus=%v index=%d", got.focus, got.optionIndex)
	}
	// 一直 ↑ 到选项头，再 ↑ 一次应到地址。
	for got.optionIndex > 0 {
		updated, _ = got.Update(tea.KeyMsg{Type: tea.KeyUp})
		got = updated.(Model)
	}
	updated, _ = got.Update(tea.KeyMsg{Type: tea.KeyUp})
	got = updated.(Model)
	if got.focus != focusAddress {
		t.Fatalf("选项头 ↑ 应到地址: %v", got.focus)
	}
	updated, _ = got.Update(tea.KeyMsg{Type: tea.KeyUp})
	got = updated.(Model)
	if got.focus != focusConfigs || got.cursor != 1 {
		t.Fatalf("地址 ↑ 应到文件末项: focus=%v cursor=%d", got.focus, got.cursor)
	}
	updated, _ = got.Update(tea.KeyMsg{Type: tea.KeyUp})
	got = updated.(Model)
	if got.focus != focusConfigs || got.cursor != 0 {
		t.Fatalf("文件内 ↑: focus=%v cursor=%d", got.focus, got.cursor)
	}
	updated, _ = got.Update(tea.KeyMsg{Type: tea.KeyUp})
	got = updated.(Model)
	if got.focus != focusOptions || got.optionIndex != len(optionOrder)-1 {
		t.Fatalf("文件头再 ↑ 应回选项栏末项: focus=%v index=%d", got.focus, got.optionIndex)
	}
}

func TestArrowKeysFromFilesJumpToOptionsFirstRow(t *testing.T) {
	for _, key := range []tea.KeyMsg{
		{Type: tea.KeyLeft},
		{Type: tea.KeyRight},
	} {
		model := New(sessionFixture()) // 焦点在文件栏
		updated, _ := model.Update(key)
		got := updated.(Model)
		if got.focus != focusOptions || got.optionIndex != 0 {
			t.Fatalf("文件栏 %v 应切到选项栏模式行: focus=%v index=%d", key.Type, got.focus, got.optionIndex)
		}
	}
	// 地址行 ←→ 无操作。
	model := New(sessionFixture())
	model.focus = focusAddress
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRight})
	if got := updated.(Model); got.focus != focusAddress || got.address != "" {
		t.Fatalf("地址行 → 应无操作: focus=%v address=%q", got.focus, got.address)
	}
}

func TestClickHelpZonesRunActions(t *testing.T) {
	model := New(sessionFixture())
	enter, quit := helpZones()
	lo := model.computeLayout()

	// 没勾选时点「回车」：留下提示，不开始。
	updated, _ := model.Update(tea.MouseMsg{
		Action: tea.MouseActionPress, Button: tea.MouseButtonLeft,
		X: enter[0] + 1, Y: lo.helpY,
	})
	got := updated.(Model)
	if got.started || got.fetching {
		t.Fatalf("无源点回车不该开始: started=%v fetching=%v", got.started, got.fetching)
	}
	if !strings.Contains(got.status, "先勾选") {
		t.Fatalf("status = %q", got.status)
	}

	// 勾上第一个配置再点「回车」：开始并退出。
	toggled, _ := got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	model = toggled.(Model)
	updated, cmd := model.Update(tea.MouseMsg{
		Action: tea.MouseActionPress, Button: tea.MouseButtonLeft,
		X: enter[0] + 1, Y: lo.helpY,
	})
	got = updated.(Model)
	if !got.started || cmd == nil {
		t.Fatalf("点回车热区应开始并退出: started=%v cmd=%v", got.started, cmd)
	}

	// 点「Ctrl+C」热区：退出。
	model = New(sessionFixture())
	updated, cmd = model.Update(tea.MouseMsg{
		Action: tea.MouseActionPress, Button: tea.MouseButtonLeft,
		X: quit[0] + 1, Y: lo.helpY,
	})
	got = updated.(Model)
	if !got.quitting || cmd == nil {
		t.Fatalf("点 Ctrl+C 热区应退出: quitting=%v cmd=%v", got.quitting, cmd)
	}

	// 热区外的帮助行位置不触发。
	model = New(sessionFixture())
	updated, cmd = model.Update(tea.MouseMsg{
		Action: tea.MouseActionPress, Button: tea.MouseButtonLeft,
		X: 0, Y: lo.helpY,
	})
	if got := updated.(Model); got.quitting || got.started || cmd != nil {
		t.Fatalf("点帮助行空白处不该有反应: quitting=%v started=%v", got.quitting, got.started)
	}
}

func TestClickOptionSymbolAdjustsValueOnly(t *testing.T) {
	model := New(sessionFixture())
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	model = updated.(Model)
	lo := model.computeLayout()
	y := lo.paneY + 1 + optionIndexFor(OptionDownloadSize) // 下载大小行

	// 点值本身：只选中，不减不加。
	clicked, _ := model.Update(tea.MouseMsg{
		Action: tea.MouseActionPress, Button: tea.MouseButtonLeft,
		X: lo.optionsX + 2 + optionLabelWidth() + 2 + 2, Y: y, // 落在「50」的数字上
	})
	got := clicked.(Model)
	if got.options.DownloadSize != "50" || got.focus != focusOptions || got.optionIndex != optionIndexFor(OptionDownloadSize) {
		t.Fatalf("点值应只选中: size=%q focus=%v", got.options.DownloadSize, got.focus)
	}

	// 点行左半但不在符号上：同样只选中。
	clicked, _ = model.Update(tea.MouseMsg{
		Action: tea.MouseActionPress, Button: tea.MouseButtonLeft,
		X: lo.optionsX + 1, Y: y,
	})
	if got := clicked.(Model); got.options.DownloadSize != "50" {
		t.Fatalf("点标签不该减值: %q", got.options.DownloadSize)
	}

	// 点「<」符号：减。
	model = New(sessionFixture())
	updated, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	model = updated.(Model)
	left, right, ok := model.arrowSymbolX(lo, optionIndexFor(OptionDownloadSize))
	if !ok || left <= 0 || right <= left {
		t.Fatalf("符号位置应有效: left=%d right=%d ok=%v", left, right, ok)
	}
	clicked, _ = model.Update(tea.MouseMsg{
		Action: tea.MouseActionPress, Button: tea.MouseButtonLeft,
		X: left, Y: y,
	})
	if got := clicked.(Model); got.options.DownloadSize != "45" {
		t.Fatalf("点 < 应减 5MB: %q", got.options.DownloadSize)
	}

	// 点「>」符号：加。
	model = clicked.(Model)
	clicked, _ = model.Update(tea.MouseMsg{
		Action: tea.MouseActionPress, Button: tea.MouseButtonLeft,
		X: right, Y: y,
	})
	if got := clicked.(Model); got.options.DownloadSize != "50" {
		t.Fatalf("点 > 应加回 50MB: %q", got.options.DownloadSize)
	}

	// 符号两侧一格内仍算命中（容差），两格外不响应。
	model = clicked.(Model)
	clicked, _ = model.Update(tea.MouseMsg{
		Action: tea.MouseActionPress, Button: tea.MouseButtonLeft,
		X: left + 1, Y: y,
	})
	if got := clicked.(Model); got.options.DownloadSize != "45" {
		t.Fatalf("符号相邻一格应命中: %q", got.options.DownloadSize)
	}
	model = clicked.(Model)
	clicked, _ = model.Update(tea.MouseMsg{
		Action: tea.MouseActionPress, Button: tea.MouseButtonLeft,
		X: left + 2, Y: y, // 已落在值的空格/数字区
	})
	if got := clicked.(Model); got.options.DownloadSize != "45" {
		t.Fatalf("容差之外不该减: %q", got.options.DownloadSize)
	}

	// 模式行同样只认符号。
	model = New(sessionFixture())
	updated, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	model = updated.(Model)
	modeLeft, _, ok := model.arrowSymbolX(lo, optionIndexFor(OptionSpeedMode))
	if !ok {
		t.Fatal("模式行应有符号")
	}
	clicked, _ = model.Update(tea.MouseMsg{
		Action: tea.MouseActionPress, Button: tea.MouseButtonLeft,
		X: modeLeft, Y: lo.paneY + 1 + optionIndexFor(OptionSpeedMode),
	})
	if got := clicked.(Model); got.options.Mode != "fast" {
		t.Fatalf("点模式行 < 应切到快速: %q", got.options.Mode)
	}

	// 开关行点哪儿都切换（无符号）。
	model = New(sessionFixture())
	updated, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	model = updated.(Model)
	boolIndex := optionIndexFor(OptionNoImage)
	if got, _ := model.Update(tea.MouseMsg{
		Action: tea.MouseActionPress, Button: tea.MouseButtonLeft,
		X: lo.optionsX + 4, Y: lo.paneY + 1 + boolIndex,
	}); !got.(Model).options.NoImage {
		t.Fatal("开关行点击应切换")
	}
}

func TestFetchingLocksMouseClicks(t *testing.T) {
	model := New(sessionFixture())
	model.fetching = true
	updated, _ := model.Update(tea.MouseMsg{
		Action: tea.MouseActionPress, Button: tea.MouseButtonLeft,
		X: 1, Y: model.computeLayout().paneY + 1,
	})
	got := updated.(Model)
	if got.checked[0] || got.focus != focusConfigs {
		t.Fatalf("获取中点击不该改勾选: checked=%v focus=%v", got.checked, got.focus)
	}
}

func TestContentXShiftsForWideTerminal(t *testing.T) {
	model := New(sessionFixture())
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	model = updated.(Model)
	if got := model.contentX(30); got != 30-(120-maxContentWidth)/2 {
		t.Fatalf("宽终端点击应减去居中偏移: %d", got)
	}
	narrow := New(sessionFixture())
	if got := narrow.contentX(10); got != 10 {
		t.Fatalf("窄于 100 列时无需换算: %d", got)
	}
}

func TestHelpZonesAlignWithRenderedLine(t *testing.T) {
	// 布局的 helpY 必须真的指向渲染出的帮助行，否则热区点击会落空。
	// 坐标对坐标的测试自洽不了，只能拿 View 的实际行来对。
	for _, size := range []tea.WindowSizeMsg{
		{Width: 80, Height: 24},
		{Width: 0, Height: 0}, // 无尺寸（测试/极小终端）同样要对齐
	} {
		model := New(sessionFixture())
		updated, _ := model.Update(size)
		model = updated.(Model)
		lo := model.computeLayout()
		lines := strings.Split(model.View(), "\n")
		if lo.helpY < 0 || lo.helpY >= len(lines) {
			t.Fatalf("helpY %d 超出渲染范围（%d 行）", lo.helpY, len(lines))
		}
		if !strings.Contains(lines[lo.helpY], "Ctrl+C") || !strings.Contains(lines[lo.helpY], "开始测速") {
			t.Fatalf("helpY=%d 指向的不是帮助行:\n%s", lo.helpY, lines[lo.helpY])
		}
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

func sessionFixture() Session {
	return Session{
		Configs: []ConfigEntry{
			{Name: "a.yaml", Path: "/opt/a.yaml", Selectable: true},
			{Name: "bad.yaml", Path: "/opt/bad.yaml", Reason: "不是合法的 yaml"},
		},
	}
}
