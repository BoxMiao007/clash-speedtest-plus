package picker

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestSelectionReturnsCheckedConfigsAndAddresses(t *testing.T) {
	model := New(sessionFixture())
	model.checked[0] = true
	model.fetchedSources = []SourceSpec{
		{Value: "/opt/sub-1.yaml", DisplayName: "https://example.com/1", FromSubscription: true},
		{Value: "/opt/sub-2.yaml", DisplayName: "https://example.com/2", FromSubscription: true},
	}
	got := model.SourceList()
	wantValues := []string{"/opt/a.yaml", "/opt/sub-1.yaml", "/opt/sub-2.yaml"}
	if len(got) != len(wantValues) {
		t.Fatalf("源数量 = %d, want %d", len(got), len(wantValues))
	}
	for i, src := range got {
		if src.Value != wantValues[i] {
			t.Fatalf("源 %d Value = %q, want %q", i, src.Value, wantValues[i])
		}
	}
}

func TestEnterFetchesSubscriptionsThenStarts(t *testing.T) {
	var requested []string
	session := sessionFixture()
	session.Fetch = func(urls []string) ([]SourceSpec, []string, error) {
		requested = append(requested, urls...)
		return []SourceSpec{{Value: "/opt/sub-1.yaml", DisplayName: "https://example.com/a", FromSubscription: true}}, []string{"https://example.com/a&flag=meta"}, nil
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
	gotSources := model.SourceList()
	if len(gotSources) != 1 || gotSources[0].Value != "/opt/sub-1.yaml" {
		t.Fatalf("SourceList = %#v", gotSources)
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

// focusOutputPath 把焦点挪到输出路径行并打字，模拟用户上下键选中后输入。
func focusOutputPath(t *testing.T, model Model, text string) Model {
	t.Helper()
	model.focus = focusOptions
	model.optionIndex = optionIndexFor(OptionOutputPath)
	if text == "" {
		return model
	}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text)})
	return updated.(Model)
}

func TestEnterFinalizesOutputPathSuffix(t *testing.T) {
	cases := []struct{ name, input, want string }{
		{"词干补 .yaml", "result", "result.yaml"},
		{"自带 .yml 原样", "result.yml", "result.yml"},
		{"留空仍是不输出", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			model := focusOutputPath(t, New(sessionFixture()), tc.input)
			model.checked[0] = true
			updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
			got := updated.(Model)
			if !got.started {
				t.Fatal("回车确认后应开始测速")
			}
			if cmd == nil {
				t.Fatal("回车确认后应返回 tea.Quit 结束选源界面")
			}
			if value := got.Options().OutputPath; value != tc.want {
				t.Fatalf("OutputPath = %q, want %q", value, tc.want)
			}
			if tc.want != "" && !strings.Contains(stripANSI(got.View()), tc.want) {
				t.Fatalf("落定后行内应显示补全后的值 %q:\n%s", tc.want, stripANSI(got.View()))
			}
		})
	}
}

func TestEnterWithTrailingSeparatorStaysAndExplains(t *testing.T) {
	for _, input := range []string{"abc/", `abc\`} {
		t.Run(input, func(t *testing.T) {
			model := focusOutputPath(t, New(sessionFixture()), input)
			model.checked[0] = true
			if view := stripANSI(model.View()); !strings.Contains(view, input) {
				t.Fatalf("打字过程中行内应显示原样输入:\n%s", view)
			}
			updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
			got := updated.(Model)
			if got.started || got.fetching {
				t.Fatalf("目录意图不该开始测速: started=%v fetching=%v", got.started, got.fetching)
			}
			if !strings.Contains(got.status, "输出路径") || !strings.Contains(got.status, "不合法") {
				t.Fatalf("status = %q", got.status)
			}
			// 目录意图的原因要在状态行露出，不能被「不合法」包装吞掉。
			if !strings.Contains(got.status, "目录意图") {
				t.Fatalf("状态行应带上目录意图的原因: %q", got.status)
			}
			if value := got.Options().OutputPath; value != input {
				t.Fatalf("报错不应改写已填的值: %q", value)
			}
		})
	}
}

func TestEnterFetchesAfterFinalizingOutputPath(t *testing.T) {
	model := focusOutputPath(t, New(sessionFixture()), "result")
	model.address = "https://example.com/a"
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(Model)
	if got.started || !got.fetching {
		t.Fatalf("填了地址应先获取: started=%v fetching=%v", got.started, got.fetching)
	}
	if value := got.Options().OutputPath; value != "result.yaml" {
		t.Fatalf("获取前就应落定输出路径: %q", value)
	}
}

// Esc 返回帧：测速中按 Esc 返回选源界面时，main 把落定后的同一个 model
// 原样再喂给 runPickerModel。再渲染的帧要仍显示补全后的值，再次回车
// 也不得把后缀叠成 result.yaml.yaml。
func TestOutputPathFinalizedFrameSurvivesEscReturn(t *testing.T) {
	model := focusOutputPath(t, New(sessionFixture()), "result")
	model.checked[0] = true
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	returned := updated.(Model)
	if view := stripANSI(returned.View()); !strings.Contains(view, "result.yaml") {
		t.Fatalf("Esc 返回后的帧应显示补全后的输出路径:\n%s", view)
	}
	reentered, _ := returned.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := reentered.(Model)
	if value := got.Options().OutputPath; value != "result.yaml" {
		t.Fatalf("再次回车不应叠加后缀: %q", value)
	}
}

func TestModeChangeKeepsOutputPathUncompleted(t *testing.T) {
	model := focusOutputPath(t, New(sessionFixture()), "result")
	// 回到模式行切换测速模式：该行不重置，也不在此刻补全。
	model.optionIndex = optionIndexFor(OptionSpeedMode)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRight})
	got := updated.(Model)
	if got.Options().Mode != "full" {
		t.Fatalf("应切到 full 模式: %q", got.Options().Mode)
	}
	if value := got.Options().OutputPath; value != "result" {
		t.Fatalf("切模式不应重置或补全输出路径: %q", value)
	}
	if mode := got.Options().OutputMode; mode != OutputModeCustom {
		t.Fatalf("切模式不应重置输出模式: %v", mode)
	}
}

// outputOptionLineTail 取画面里输出路径行标签之后的部分，避免断言被
// 同一物理行左栏的文件名干扰。
func outputOptionLineTail(t *testing.T, model Model) string {
	t.Helper()
	for _, line := range strings.Split(model.View(), "\n") {
		if idx := strings.Index(line, "输出路径"); idx >= 0 {
			return line[idx:]
		}
	}
	t.Fatal("画面里没有输出路径行")
	return ""
}

func TestViewGreysMissingSuffixOnFocusedOutputPath(t *testing.T) {
	// 测试环境不是 TTY，lipgloss 会剥掉转义序列；强制彩色输出以便断言灰显。
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })
	cases := []struct{ name, input, hint string }{
		{"词干缺 .yaml", "result", ".yaml"},
		{"半截扩展名缺 yaml", "result.", "yaml"},
		{"目录段加词干", "dir/result", ".yaml"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			model := focusOutputPath(t, New(sessionFixture()), tc.input)
			tail := outputOptionLineTail(t, model)
			if !strings.Contains(tail, greyStyle.Render(tc.hint)) {
				t.Fatalf("聚焦输出路径应灰显缺省后缀 %q:\n%s", tc.hint, tail)
			}
			// 纯文本上灰显段落在光标前。
			if plain := stripANSI(tail); !strings.Contains(plain, tc.input+tc.hint+"▌") {
				t.Fatalf("灰显段应在光标前:\n%s", plain)
			}
			// 纯渲染不改值。
			if value := model.Options().OutputPath; value != tc.input {
				t.Fatalf("灰显不应进入实际值: %q", value)
			}
		})
	}
}

func TestViewHidesSuffixHintWhenCompleteOrEmptyStem(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })
	for _, input := range []string{"result.yml", "result.yaml", "result.YAML", ".yaml", ".yml", "."} {
		t.Run(input, func(t *testing.T) {
			model := focusOutputPath(t, New(sessionFixture()), input)
			tail := outputOptionLineTail(t, model)
			for _, hint := range []string{"yaml", ".yaml"} {
				if strings.Contains(tail, greyStyle.Render(hint)) {
					t.Fatalf("后缀已写全或词干为空时不应灰显 %q:\n%s", hint, tail)
				}
			}
			if plain := stripANSI(tail); !strings.Contains(plain, input+"▌") {
				t.Fatalf("行内应显示原样输入加光标:\n%s", plain)
			}
			if value := model.Options().OutputPath; value != input {
				t.Fatalf("无灰显时也不应改值: %q", value)
			}
		})
	}
}

func TestViewShowsOutputPathAsIsWhenUnfocused(t *testing.T) {
	model := New(sessionFixture())
	model.options.OutputMode = OutputModeCustom
	model.options.OutputPath = "result"
	plain := stripANSI(outputOptionLineTail(t, model))
	if !strings.Contains(plain, "result") {
		t.Fatalf("未聚焦的输出路径行应显示原样输入:\n%s", plain)
	}
	if strings.Contains(plain, "yaml") {
		t.Fatalf("未聚焦时不应有灰显补全:\n%s", plain)
	}
}

func TestFocusedOutputPathWithHintStaysWithinWidth(t *testing.T) {
	model := New(sessionFixture())
	model.focus = focusOptions
	model.optionIndex = optionIndexFor(OptionOutputPath)
	model.options.OutputMode = OutputModeCustom
	model.options.OutputPath = strings.Repeat("r", 100)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 40, Height: 40})
	model = updated.(Model)
	for i, line := range strings.Split(model.View(), "\n") {
		if w := lipgloss.Width(line); w > 40 {
			t.Fatalf("第 %d 行超宽 %d:\n%s", i, w, line)
		}
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

func TestViewShowsNodeCountRightAligned(t *testing.T) {
	model := New(Session{Configs: []ConfigEntry{
		{Name: "a.yaml", Path: "/opt/a.yaml", Selectable: true, Nodes: 37},
		{Name: "b.yaml", Path: "/opt/b.yaml", Selectable: true, Nodes: 2, More: true},
		{Name: "c.yaml", Path: "/opt/c.yaml", Selectable: true, More: true},
		{Name: "bad.yaml", Path: "/opt/bad.yaml", Reason: "不是合法的 yaml"},
	}})
	model.width, model.height = 80, 20
	var lines []string
	for _, line := range strings.Split(model.View(), "\n") {
		if strings.Contains(line, ".yaml") {
			// 分栏右半边是选项栏，行尾在分隔线 │ 之前。
			lines = append(lines, stripANSI(strings.Split(line, "│")[0]))
		}
	}
	if len(lines) != 4 {
		t.Fatalf("lines = %d:\n%s", len(lines), model.View())
	}
	assertCount := func(i int, name, count string) {
		t.Helper()
		if !strings.Contains(lines[i], name) || !strings.HasSuffix(strings.TrimRight(lines[i], " "), count) {
			t.Fatalf("%s 行未以 %s 结尾: %q", name, count, lines[i])
		}
		if strings.Contains(lines[i], "0+") {
			t.Fatalf("纯 providers 不应显示 0+: %q", lines[i])
		}
	}
	assertCount(0, "a.yaml", "37")
	assertCount(1, "b.yaml", "2+")
	// 纯 providers 显示 +，不是 0+。
	assertCount(2, "c.yaml", "+")
	if strings.Contains(lines[3], "+") || strings.Contains(lines[3], " 0") {
		t.Fatalf("灰行不应显示节点数: %q", lines[3])
	}
}

// filePaneLines 取出渲染结果中文件栏的可见行（剥颜色、截去选项栏）。
func filePaneLines(t *testing.T, m Model) []string {
	t.Helper()
	// 保留调用方给定的窗口，让宽屏上限与窄屏压缩可分别验证。
	if m.width == 0 {
		updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
		m = updated.(Model)
	}
	lo := m.computeLayout()
	raw := strings.Split(stripANSI(m.View()), "\n")
	lines := make([]string, 0, lo.paneH)
	for _, line := range raw[lo.paneY : lo.paneY+lo.paneH] {
		lines = append(lines, strings.Split(line, "│")[0])
	}
	return lines
}

func TestViewWrapsLongNameAtNameColumn(t *testing.T) {
	model := New(Session{Configs: []ConfigEntry{
		{Name: strings.Repeat("a", 48) + ".yaml", Path: "/x", Selectable: true, Nodes: 3},
		{Name: "b.yaml", Path: "/y", Selectable: true, Nodes: 1},
	}})
	// 窗口要宽过「勾选符 + 序号 + 40 格名称 + 节点数」，否则窄终端会把名称列压到上限以下。
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	model = updated.(Model)
	lines := filePaneLines(t, model)
	// lines[0] 是节标题「▎ 文件」，之后是条目行。
	if len(lines) < 4 || strings.Contains(lines[1], "…") {
		t.Fatalf("长名称应折成两行且不截断: %q", lines)
	}
	// 名称列 40 显示格：首段 40 个 a，续行接剩下的 8 个 a 与 .yaml。
	if !strings.Contains(lines[1], strings.Repeat("a", 40)) {
		t.Fatalf("首段应填满 40 格: %q", lines[1])
	}
	// 续行与首行的名称起点同列。超宽终端的居中留白两行一样，不用单独扣。
	nameByte := strings.Index(lines[1], strings.Repeat("a", 40))
	// strings.Index 返回字节位置；勾选符是多字节字符，须按显示宽度比较缩进。
	nameColumn := lipgloss.Width(lines[1][:nameByte])
	want := strings.Repeat(" ", nameColumn) + strings.Repeat("a", 8) + ".yaml"
	got := strings.TrimRight(lines[2], " ")
	if got != want {
		t.Fatalf("续行应对齐名称起点 %d:\n实际 %q\n期望 %q", nameColumn, got, want)
	}
	// 节点数只在首行末尾。
	if !strings.HasSuffix(strings.TrimRight(lines[1], " "), "3") {
		t.Fatalf("节点数应在首行末尾: %q", lines[1])
	}
}

func TestNameColumnDisplayWidthBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, first, rest string
	}{
		{strings.Repeat("a", 34) + ".yaml", strings.Repeat("a", 34) + ".yaml", ""},
		{strings.Repeat("a", 35) + ".yaml", strings.Repeat("a", 35) + ".yaml", ""},
		{strings.Repeat("a", 36) + ".yaml", strings.Repeat("a", 36) + ".yam", "l"},
		{strings.Repeat("中", 17) + "a.yaml", strings.Repeat("中", 17) + "a.yaml", ""},
		{strings.Repeat("中", 20) + ".yaml", strings.Repeat("中", 20), ".yaml"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model := New(Session{Configs: []ConfigEntry{
				{Name: tc.name, Path: "/first", Selectable: true, Nodes: 37},
				{Name: "next.yaml", Path: "/next", Selectable: true, Nodes: 1},
			}})
			updated, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
			lines := filePaneLines(t, updated.(Model))
			if !strings.HasPrefix(lines[1], "○ 1. "+tc.first+"  37") {
				t.Fatalf("首行内容或节点数位置错误: %q", lines[1])
			}
			next := 2
			if tc.rest != "" {
				if got := strings.TrimRight(lines[2], " "); got != "     "+tc.rest {
					t.Fatalf("续行不符: %q", got)
				}
				next++
			}
			if !strings.Contains(lines[next], "2. next.yaml") {
				t.Fatalf("下一个条目的起始行不符: %q", lines[next])
			}
		})
	}
}

func TestViewHighlightsAllRowsOfSelectedWrappedEntry(t *testing.T) {
	// 测试环境不是 TTY，lipgloss 会剥掉转义序列；强制彩色输出以便断言高亮。
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })
	model := New(Session{Configs: []ConfigEntry{
		{Name: strings.Repeat("a", 40) + ".yaml", Path: "/x", Selectable: true},
		{Name: "b.yaml", Path: "/y", Selectable: true},
	}})
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	model = updated.(Model)
	lo := model.computeLayout()
	rawLines := strings.Split(model.View(), "\n")
	var raw []string
	for _, line := range rawLines[lo.paneY : lo.paneY+lo.paneH] {
		if strings.Contains(stripANSI(line), "aaaa") {
			raw = append(raw, line)
		}
	}
	if len(raw) != 2 {
		t.Fatalf("期望两行折行: %d", len(raw))
	}
	// 选中条目整条高亮：两行都整行套了选中样式（行首就是转义序列）。
	for i, line := range raw {
		if !strings.HasPrefix(line, "\x1b[") {
			t.Fatalf("第 %d 行未被选中高亮: %q", i, line)
		}
	}
}

func TestClickWrappedEntryHitsOnContinuationRow(t *testing.T) {
	model := New(Session{Configs: []ConfigEntry{
		{Name: strings.Repeat("a", 40) + ".yaml", Path: "/x", Selectable: true},
		{Name: "b.yaml", Path: "/y", Selectable: true},
	}})
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	model = updated.(Model)
	lo := model.computeLayout()
	clicked, _ := model.Update(tea.MouseMsg{
		Action: tea.MouseActionPress, Button: tea.MouseButtonLeft,
		X: 2, Y: lo.paneY + 2, // 第一个条目的折行续行
	})
	got := clicked.(Model)
	if got.cursor != 0 || !got.checked[0] || got.checked[1] {
		t.Fatalf("点折行续行应命中条目本身: cursor=%d checked=%v", got.cursor, got.checked)
	}
}

func TestScrollMovesByWholeEntries(t *testing.T) {
	configs := make([]ConfigEntry, 6)
	for i := range configs {
		// 每条名称都折成两行。
		configs[i] = ConfigEntry{
			Name: strings.Repeat(string(rune('a'+i)), 40) + ".yaml",
			Path: "/x", Selectable: true,
		}
	}
	model := New(Session{Configs: configs})
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 10})
	model = updated.(Model) // paneH=3，窗口只能放 2 行
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	got := updated.(Model)
	if got.configScroll != 1 {
		t.Fatalf("光标移到第二条目时应整个滚动一条目: scroll=%d", got.configScroll)
	}
	lines := strings.Split(stripANSI(got.View()), "\n")
	if !strings.Contains(lines[got.computeLayout().paneY+1], "2. bbb") {
		t.Fatalf("窗口首条目应是第二个:\n%s", strings.Join(lines, "\n"))
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

var ansiPattern = regexp.MustCompile("\x1b\\[[0-9;]*m")

// stripANSI 去掉颜色码，方便断言纯文本的对齐。
func stripANSI(s string) string { return ansiPattern.ReplaceAllString(s, "") }

func TestViewNumbersRowsWithRightAlignedIndex(t *testing.T) {
	configs := make([]ConfigEntry, 0, 12)
	for i := 1; i <= 11; i++ {
		name := fmt.Sprintf("f%02d.yaml", i)
		configs = append(configs, ConfigEntry{Name: name, Path: "/opt/" + name, Selectable: true})
	}
	configs = append(configs, ConfigEntry{Name: "bad.yaml", Path: "/opt/bad.yaml", Reason: "不是合法的 yaml"})
	model := New(Session{Configs: configs})
	model.checked[0] = true

	lines := strings.Split(stripANSI(model.View()), "\n")
	row := func(name string) string {
		for _, line := range lines {
			if strings.Contains(line, name) {
				return line
			}
		}
		t.Fatalf("渲染里找不到 %s:\n%s", name, strings.Join(lines, "\n"))
		return ""
	}
	first, last, grey := row("f01.yaml"), row("f11.yaml"), row("bad.yaml")
	if !strings.Contains(first, "✓  1. f01.yaml") {
		t.Fatalf("勾选行序号补齐不对: %q", first)
	}
	if !strings.Contains(last, "○ 11. f11.yaml") {
		t.Fatalf("未勾选行序号不对: %q", last)
	}
	if !strings.Contains(grey, "× 12. bad.yaml") {
		t.Fatalf("灰行也应带序号: %q", grey)
	}
	if strings.Index(first, "f01.yaml") != strings.Index(last, "f11.yaml") {
		t.Fatalf("名称列起点没对齐:\n%q\n%q", first, last)
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
	model.options.OutputMode = OutputModeCustom
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
	session.Fetch = func(urls []string) ([]SourceSpec, []string, error) {
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

// 帮助行：单源照旧，多源时回车段亮出轮数，热区跟着文案走。
func TestHelpLineShowsRoundCountForMultipleSources(t *testing.T) {
	single := New(sessionFixture())
	if got := single.helpLine(200); strings.Contains(got, "轮") {
		t.Fatalf("单源帮助行不该提轮数: %q", got)
	}

	multi := New(sessionFixture())
	multi.checked[0] = true
	var updated tea.Model
	for multi.focus != focusAddress {
		updated, _ = multi.Update(tea.KeyMsg{Type: tea.KeyDown})
		multi = updated.(Model)
	}
	updated, _ = multi.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("https://a.com/x, https://b.com/y")})
	multi = updated.(Model)
	if got := multi.helpLine(200); !strings.Contains(got, "共 3 轮") {
		t.Fatalf("1 文件 + 2 链接应提示 3 轮: %q", got)
	}
	enter, _ := multi.helpZones()
	// 注入假拉取，点击「开始」段先进入获取，获取完才开始。
	session := sessionFixture()
	session.Fetch = func(urls []string) ([]SourceSpec, []string, error) {
		sources := make([]SourceSpec, 0, len(urls))
		for _, u := range urls {
			sources = append(sources, SourceSpec{Value: "/opt/sub-" + u, DisplayName: u, FromSubscription: true})
		}
		return sources, nil, nil
	}
	multi.session = session
	updated, cmd := multi.Update(tea.MouseMsg{
		Action: tea.MouseActionPress, Button: tea.MouseButtonLeft,
		X: enter[0] + 1, Y: multi.computeLayout().helpY,
	})
	got := updated.(Model)
	if !got.fetching || cmd == nil {
		t.Fatal("点回车热区应进入获取")
	}
	done, _ := got.Update(cmd())
	if !done.(Model).started {
		t.Fatal("获取完成应开始测速")
	}
}

func TestClickHelpZonesRunActions(t *testing.T) {
	model := New(sessionFixture())
	enter, quit := model.helpZones()
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

	// 开关行点标签只选中，不翻转。
	model = New(sessionFixture())
	updated, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	model = updated.(Model)
	boolIndex := optionIndexFor(OptionNoImage)
	if got, _ := model.Update(tea.MouseMsg{
		Action: tea.MouseActionPress, Button: tea.MouseButtonLeft,
		X: lo.optionsX + 4, Y: lo.paneY + 1 + boolIndex,
	}); got.(Model).options.NoImage {
		t.Fatal("点开关标签不应翻转")
	}
}

// 从实际画面量取值的位置，避免测试和命中实现共享坐标公式。
func renderedOptionPosition(t *testing.T, model Model, label string) (int, int) {
	t.Helper()
	for y, line := range strings.Split(stripANSI(model.View()), "\n") {
		if start := strings.Index(line, label); start >= 0 {
			end := start + len(label)
			value := strings.TrimLeft(line[end:], " ")
			if value == "" {
				t.Fatalf("选项 %s 的值未显示: %q", label, line)
			}
			return lipgloss.Width(line[:len(line)-len(value)]), y
		}
	}
	t.Fatalf("未找到选项 %s", label)
	return 0, 0
}

func TestClickBooleanOnlyAtRenderedValue(t *testing.T) {
	for _, option := range []Option{OptionImageSpeedOnly, OptionNoImage, OptionRename} {
		label := optionOrder[optionIndexFor(option)].label
		for _, width := range []int{80, 120} {
			for _, on := range []bool{false, true} {
				for _, offset := range []int{-3, -2, -1, 0, 1, 2, 3, 5} {
					t.Run(fmt.Sprintf("%s/宽%d/开%t/偏移%d", label, width, on, offset), func(t *testing.T) {
						model := New(sessionFixture())
						model.options.OutputMode = OutputModeCustom
						model.options.OutputPath = "out.yaml"
						model.options.ImageSpeedOnly, model.options.NoImage, model.options.Rename = on, on, on
						updated, _ := model.Update(tea.WindowSizeMsg{Width: width, Height: 30})
						model = updated.(Model)
						x, y := renderedOptionPosition(t, model, label)
						want := model.Options()
						// 「开/关」占两格；连同左右容差，命中偏移为 -1、0、1、2。
						if offset >= -1 && offset <= 2 {
							switch option {
							case OptionImageSpeedOnly:
								want.ImageSpeedOnly = !on
							case OptionNoImage:
								want.NoImage = !on
							case OptionRename:
								want.Rename = !on
							}
						}
						updated, _ = model.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: x + offset, Y: y})
						got := updated.(Model)
						if got.Options() != want {
							t.Fatalf("点击开关的结果不符: 实际 %+v，期望 %+v", got.Options(), want)
						}
						if got.focus != focusOptions || got.optionIndex != optionIndexFor(option) {
							t.Fatal("点击应选中该开关行")
						}
					})
				}
			}
		}
	}
}

func TestBooleanKeyboardAndDisabledClicks(t *testing.T) {
	for _, option := range []Option{OptionImageSpeedOnly, OptionNoImage, OptionRename} {
		label := optionOrder[optionIndexFor(option)].label
		t.Run(label, func(t *testing.T) {
			model := New(sessionFixture())
			model.options.OutputMode = OutputModeCustom
			model.options.OutputPath = "out.yaml"
			updated, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
			model = updated.(Model)
			x, y := renderedOptionPosition(t, model, label)
			updated, _ = model.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: x - 4, Y: y})
			model = updated.(Model)
			for _, key := range []tea.KeyMsg{{Type: tea.KeySpace}, {Type: tea.KeyRunes, Runes: []rune(" ")}, {Type: tea.KeyLeft}, {Type: tea.KeyRight}} {
				before := model.Options()
				updated, _ = model.Update(key)
				model = updated.(Model)
				if model.Options() == before {
					t.Fatalf("按键 %s 应翻转开关", key.String())
				}
			}
			if option == OptionNoImage {
				return // 此开关没有不可用状态。
			}
			model.options.Mode = "fast"
			model.options.OutputPath = ""
			before := model.Options()
			x, y = renderedOptionPosition(t, model, label)
			updated, _ = model.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: x, Y: y})
			if updated.(Model).Options() != before {
				t.Fatal("不可用的开关不应响应点击")
			}
		})
	}
}

func TestBooleanClickAfterScrollingAndClipping(t *testing.T) {
	model := New(sessionFixture())
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 12})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRight})
	model = updated.(Model)
	for range optionIndexFor(OptionNoImage) {
		updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
		model = updated.(Model)
	}
	x, y := renderedOptionPosition(t, model, "关闭自动结果图")
	updated, _ = model.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: x, Y: y})
	model = updated.(Model)
	if !model.Options().NoImage {
		t.Fatal("滚动后点击可见开关应翻转")
	}
	updated, _ = model.Update(tea.WindowSizeMsg{Width: 42, Height: 12})
	model = updated.(Model)
	line := strings.Split(stripANSI(model.View()), "\n")[y]
	if strings.Contains(line, "开") {
		t.Fatalf("此窄屏用例应截掉开关值: %q", line)
	}
	before := model.Options()
	for x := strings.Index(line, "│") + 1; x < 55; x++ {
		updated, _ = model.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: x, Y: y})
		if updated.(Model).Options() != before {
			t.Fatalf("被截掉的开关不应留有点击热区: x=%d", x)
		}
	}
}

func TestModeChangeResetsAdjustableOptions(t *testing.T) {
	for _, mode := range []struct{ current, left, right string }{
		{"download", "fast", "full"},
		{"full", "download", "fast"},
		{"fast", "full", "download"},
	} {
		for _, input := range []string{"左键", "右键", "空格键", "空格字符", "点击左箭头", "点击右箭头"} {
			for _, output := range []string{"", "out.yaml"} {
				t.Run(mode.current+"/"+input+"/输出"+output, func(t *testing.T) {
					model := New(sessionFixture())
					updated, _ := model.Update(tea.KeyMsg{Type: tea.KeySpace})
					model = updated.(Model)
					model.address = "https://example.com/subscription"
					model.options = Options{
						Mode: mode.current, Filter: "香港", Block: "到期",
						DownloadSize: "80", UploadSize: "35", Concurrent: "8", Parallel: "9",
						Timeout: "10s", EarlyStop: "30", MaxLatency: "500ms", MaxPacketLoss: "20",
						MinDownload: "10", MinUpload: "6", ImageSpeedOnly: false, NoImage: true, Rename: false,
						OutputPath: output, RenameTemplate: "自定义名称",
						GistToken: "测试占位非凭据", GistAddress: "https://example.com/gist",
						RepoToken: "测试占位非凭据", RepoAddress: "https://example.com/repo",
						RepoFilePath: "configs/result.yaml", RepoBranch: "测试分支",
						ServerURL: "https://example.com/speed", UserAgent: "测试UA",
					}
					updated, _ = model.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
					model = updated.(Model)
					x, y := renderedOptionPosition(t, model, "测速模式")
					before := model.Options()
					updated, _ = model.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: x - 4, Y: y})
					model = updated.(Model)
					if model.Options() != before {
						t.Fatal("只选中模式行不应重置选项")
					}
					wantMode := mode.right
					var msg tea.Msg
					switch input {
					case "左键":
						wantMode = mode.left
						msg = tea.KeyMsg{Type: tea.KeyLeft}
					case "右键":
						msg = tea.KeyMsg{Type: tea.KeyRight}
					case "空格键":
						msg = tea.KeyMsg{Type: tea.KeySpace}
					case "空格字符":
						msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")}
					case "点击左箭头":
						wantMode = mode.left
						msg = tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: x, Y: y}
					case "点击右箭头":
						line := strings.Split(stripANSI(model.View()), "\n")[y]
						end := strings.LastIndex(line, ">")
						if end < 0 {
							t.Fatal("模式行未显示右箭头")
						}
						msg = tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: lipgloss.Width(line[:end]), Y: y}
					}
					want := Options{
						Mode: wantMode, Filter: "香港", Block: "到期",
						DownloadSize: "50", UploadSize: "20", Concurrent: "4", Parallel: "6",
						Timeout: "5s", EarlyStop: "", MaxLatency: "1s", MaxPacketLoss: "100",
						MinDownload: "5", MinUpload: "2", ImageSpeedOnly: true, NoImage: false, Rename: true,
						OutputPath: output, RenameTemplate: "自定义名称",
						GistToken: "测试占位非凭据", GistAddress: "https://example.com/gist",
						RepoToken: "测试占位非凭据", RepoAddress: "https://example.com/repo",
						RepoFilePath: "configs/result.yaml", RepoBranch: "测试分支",
						ServerURL: "https://example.com/speed", UserAgent: "测试UA",
					}
					sources := model.SourceList()
					updated, _ = model.Update(msg)
					got := updated.(Model)
					if got.Options() != want {
						t.Fatalf("切模式后选项不符:\n实际 %+v\n期望 %+v", got.Options(), want)
					}
					if !reflect.DeepEqual(got.SourceList(), sources) || got.address != model.address {
						t.Fatal("切模式不应改变勾选配置或订阅地址")
					}
					if got.focus != model.focus || got.optionIndex != model.optionIndex {
						t.Fatal("切模式不应移动焦点")
					}
				})
			}
		}
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

func TestViewShowsVersionBadgeAtHelpLineRight(t *testing.T) {
	model := New(Session{
		Configs: []ConfigEntry{{Name: "a.yaml", Path: "/opt/a.yaml", Selectable: true}},
		Version: "v2.5.0",
	})
	model.width, model.height = 100, 30
	lines := strings.Split(stripANSI(model.View()), "\n")
	last := lines[len(lines)-1]
	if want := "v2.5.0"; !strings.HasSuffix(strings.TrimRight(last, " "), want) {
		t.Fatalf("帮助行右端应是 %s: %q", want, last)
	}
	if w := lipgloss.Width(last); w != 100 {
		t.Fatalf("角标应贴内容右缘，行宽 = %d: %q", w, last)
	}
}

func TestViewHidesVersionBadgeWhenTooNarrow(t *testing.T) {
	model := New(Session{
		Configs: []ConfigEntry{{Name: "a.yaml", Path: "/opt/a.yaml", Selectable: true}},
		Version: "v2.5.0",
	})
	model.width, model.height = 40, 30
	if view := stripANSI(model.View()); strings.Contains(view, "v2.5.0") {
		t.Fatalf("窄终端应藏角标: %q", view)
	}
}

func TestVersionBadgeKeepsHelpHotZones(t *testing.T) {
	configs := []ConfigEntry{{Name: "a.yaml", Path: "/opt/a.yaml", Selectable: true}}
	model := New(Session{Configs: configs, Version: "v2.5.0"})
	model.width, model.height = 100, 30
	enter, quit := model.helpZones()
	badgeStart := 100 - lipgloss.Width("v2.5.0")
	if gap := badgeStart - quit[1]; gap < 2 {
		t.Fatalf("角标与热区之间应留至少两格，实际 %d：quit=%v 角标起点=%d", gap, quit, badgeStart)
	}
	plainEnter, plainQuit := New(Session{Configs: configs}).helpZones()
	if enter != plainEnter || quit != plainQuit {
		t.Fatalf("角标不应挪动热区：带角标 enter=%v quit=%v，不带 enter=%v quit=%v", enter, quit, plainEnter, plainQuit)
	}
}
