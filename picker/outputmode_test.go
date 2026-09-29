package picker

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// outputModel 返回焦点停在输出路径行的模型。
func outputModel(t *testing.T) Model {
	t.Helper()
	model := New(sessionFixture())
	model.focus = focusOptions
	model.optionIndex = optionIndexFor(OptionOutputPath)
	return model
}

// 左右键循环输出模式，默认停在关闭。三态之间可自由循环——自定义态没打字时
// 同样可切；有内容才进入编辑锁定（见 CONTEXT.md「输出模式」词条）。
func TestOutputModeCyclesWithArrowKeys(t *testing.T) {
	model := outputModel(t)
	if model.options.OutputMode != OutputModeClosed {
		t.Fatalf("默认应停在关闭态: %v", model.options.OutputMode)
	}
	steps := []struct {
		key  tea.KeyType
		want OutputMode
	}{
		{tea.KeyRight, OutputModeCustom},
		{tea.KeyRight, OutputModeDefaultPath}, // 自定义没打字，继续可切
		{tea.KeyLeft, OutputModeCustom},
		{tea.KeyLeft, OutputModeClosed},
		{tea.KeyLeft, OutputModeDefaultPath},
	}
	for i, step := range steps {
		updated, _ := model.Update(tea.KeyMsg{Type: step.key})
		model = updated.(Model)
		if model.options.OutputMode != step.want {
			t.Fatalf("第 %d 次按键 %v 后应处于态 %v，得到 %v", i+1, step.key, step.want, model.options.OutputMode)
		}
	}
}

// 空格循环三态，与测速模式行的既有习惯一致；没打字的自定义态不拦空格。
func TestOutputModeCyclesWithSpace(t *testing.T) {
	model := outputModel(t)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeySpace})
	if got := updated.(Model).options.OutputMode; got != OutputModeCustom {
		t.Fatalf("空格应切到自定义态: %v", got)
	}
	updated, _ = updated.(Model).Update(tea.KeyMsg{Type: tea.KeySpace})
	if got := updated.(Model).options.OutputMode; got != OutputModeDefaultPath {
		t.Fatalf("自定义没打字，空格应继续切到默认当前路径: %v", got)
	}
}

// 点击 < > 也能循环三态：关闭态点 > 进自定义，自定义没打字继续可点，
// 点 < 反向回关闭，一路循环。
func TestOutputModeClickCycles(t *testing.T) {
	model := outputModel(t)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model = updated.(Model)
	steps := []struct {
		right bool
		want  OutputMode
	}{
		{true, OutputModeCustom},
		{true, OutputModeDefaultPath},
		{false, OutputModeCustom},
		{false, OutputModeClosed},
	}
	for i, step := range steps {
		// 每次切态后值的宽度都变，左右箭头位置重算。
		_, y := renderedOptionPosition(t, model, "输出路径")
		line := strings.Split(stripANSI(model.View()), "\n")[y]
		x := strings.LastIndex(line, "<")
		end := strings.LastIndex(line, ">")
		if x < 0 || end < 0 {
			t.Fatalf("第 %d 步前应显示 < > 提示可循环:\n%s", i+1, line)
		}
		clickX := lipgloss.Width(line[:x])
		if step.right {
			clickX = lipgloss.Width(line[:end])
		}
		updated, _ = model.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: clickX, Y: y})
		model = updated.(Model)
		if model.options.OutputMode != step.want {
			t.Fatalf("第 %d 步点击后应处于态 %v，得到 %v", i+1, step.want, model.options.OutputMode)
		}
	}
}

// 关闭态与默认当前路径态打字自动跳自定义并进入编辑：从空词干开始输入，
// 不把切换前残留的词干接进新输入。
func TestOutputModeTypingJumpsToCustom(t *testing.T) {
	cases := []struct {
		name string
		mode OutputMode
	}{
		{"关闭态打字", OutputModeClosed},
		{"默认当前路径态打字", OutputModeDefaultPath},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			model := outputModel(t)
			model.options.OutputMode = tc.mode
			model.options.OutputPath = "stale"
			updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
			got := updated.(Model)
			if got.options.OutputMode != OutputModeCustom {
				t.Fatalf("打字后应跳到自定义态: %v", got.options.OutputMode)
			}
			if got.options.OutputPath != "r" {
				t.Fatalf("打字后应从空词干开始输入: %q", got.options.OutputPath)
			}
		})
	}
}

// 自定义态打字沿用追加编辑。
func TestCustomModeTypingAppends(t *testing.T) {
	model := outputModel(t)
	model.options.OutputMode = OutputModeCustom
	model.options.OutputPath = "re"
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("sult")})
	if got := updated.(Model).options.OutputPath; got != "result" {
		t.Fatalf("自定义态打字应追加: %q", got)
	}
}

// 自定义态一旦有内容就进入编辑锁定：左右键、空格、点击都不切换（四路统一
// 锁，见 CONTEXT.md「输出模式」词条），点击只选中该行。
func TestCustomModeWithContentDoesNotCycle(t *testing.T) {
	model := outputModel(t)
	model.options.OutputMode = OutputModeCustom
	model.options.OutputPath = "re"
	assertStays := func(t *testing.T, got Model, what string) {
		t.Helper()
		if got.options.OutputMode != OutputModeCustom {
			t.Fatalf("%s 不应切走: %v", what, got.options.OutputMode)
		}
		if got.options.OutputPath != "re" {
			t.Fatalf("%s 不应改词干: %q", what, got.options.OutputPath)
		}
	}
	for _, key := range []tea.KeyType{tea.KeyLeft, tea.KeyRight, tea.KeySpace} {
		updated, _ := model.Update(tea.KeyMsg{Type: key})
		model = updated.(Model)
		assertStays(t, model, "自定义态有内容按 "+key.String())
	}

	// 点击行内（值区域）只选中，不循环三态。
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model = updated.(Model)
	x, y := renderedOptionPosition(t, model, "输出路径")
	updated, _ = model.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: x + 3, Y: y})
	model = updated.(Model)
	assertStays(t, model, "自定义态有内容点击")
	if model.focus != focusOptions || model.optionIndex != optionIndexFor(OptionOutputPath) {
		t.Fatalf("自定义态有内容点击应选中该行: focus=%v index=%v", model.focus, model.optionIndex)
	}
}

// 自定义态逐字退格清空后解锁：左右键恢复循环三态。
func TestCustomModeBackspaceToEmptyUnlocks(t *testing.T) {
	model := outputModel(t)
	model.options.OutputMode = OutputModeCustom
	model.options.OutputPath = "re"
	for i, want := range []string{"r", ""} {
		updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyBackspace})
		model = updated.(Model)
		if got := model.options.OutputPath; got != want {
			t.Fatalf("第 %d 次退格后词干应为 %q: %q", i+1, want, got)
		}
	}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if got := updated.(Model).options.OutputMode; got != OutputModeClosed {
		t.Fatalf("清空后左右键应恢复循环: %v", got)
	}
}

// 自定义态留空回车视同关闭、正常开始测速（见 CONTEXT.md「输出模式」词条）。
func TestCustomOutputEmptyEnterTreatedAsClosed(t *testing.T) {
	for _, value := range []string{"", "  "} {
		model := outputModel(t)
		model.options.OutputMode = OutputModeCustom
		model.options.OutputPath = value
		model.checked[0] = true
		updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
		got := updated.(Model)
		if !got.started {
			t.Fatalf("留空 %q 视同关闭，应正常开始测速: status=%q", value, got.status)
		}
		if got.Options().OutputMode != OutputModeClosed || got.Options().OutputPath != "" {
			t.Fatalf("留空 %q 回车应视同关闭: mode=%v path=%q", value, got.Options().OutputMode, got.Options().OutputPath)
		}
	}
}

// 默认当前路径态不填词干，回车直接开始测速：输出模式原样带回，
// 输出视为开启（重命名与 Gist/仓库上传跟着可用）。
func TestDefaultPathModeStartsWithoutStem(t *testing.T) {
	model := outputModel(t)
	model.options.OutputMode = OutputModeDefaultPath
	model.checked[0] = true
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(Model)
	if !got.started {
		t.Fatalf("默认当前路径态应正常开始测速: status=%q", got.status)
	}
	if got.Options().OutputMode != OutputModeDefaultPath {
		t.Fatalf("输出模式应原样带回: %v", got.Options().OutputMode)
	}
	if !got.optionState().OutputOpen() {
		t.Fatal("默认当前路径态输出应视为开启")
	}
}

// 切换测速模式时输出模式三态与已填词干保留（见 CONTEXT.md「输出模式」词条）。
func TestSpeedModeChangeKeepsOutputMode(t *testing.T) {
	for _, mode := range []OutputMode{OutputModeClosed, OutputModeCustom, OutputModeDefaultPath} {
		model := outputModel(t)
		model.options.OutputMode = mode
		model.options.OutputPath = "result"
		model.optionIndex = optionIndexFor(OptionSpeedMode)
		updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRight})
		got := updated.(Model)
		if got.options.OutputMode != mode || got.options.OutputPath != "result" {
			t.Fatalf("切测速模式后输出设置应保留: mode=%v path=%q", got.options.OutputMode, got.options.OutputPath)
		}
	}
}

// 三态的行内显示：关闭与默认当前路径带 < > 提示可循环，没打字的自定义态
// 同样带 < >（可切）；已填词干显示纯词干（编辑锁定，无符号）。
func TestViewShowsOutputModeStates(t *testing.T) {
	cases := []struct {
		name string
		mode OutputMode
		path string
		want string
	}{
		{"关闭", OutputModeClosed, "", "< 关闭 >"},
		{"默认当前路径", OutputModeDefaultPath, "", "< 默认当前路径 >"},
		{"自定义未打字", OutputModeCustom, "", "< 自定义 >"},
		{"自定义已填词干", OutputModeCustom, "result", "result"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			model := New(sessionFixture())
			model.options.OutputMode = tc.mode
			model.options.OutputPath = tc.path
			tail := stripANSI(outputOptionLineTail(t, model))
			if !strings.Contains(tail, tc.want) {
				t.Fatalf("输出路径行应显示 %q:\n%s", tc.want, tail)
			}
		})
	}
}
