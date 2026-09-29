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

// 左右键在关闭 → 自定义 → 默认当前路径之间循环，默认停在关闭
// （见 CONTEXT.md「输出模式」词条）。
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
		{tea.KeyRight, OutputModeDefaultPath},
		{tea.KeyRight, OutputModeClosed},
		{tea.KeyLeft, OutputModeDefaultPath},
		{tea.KeyLeft, OutputModeCustom},
		{tea.KeyLeft, OutputModeClosed},
	}
	for i, step := range steps {
		updated, _ := model.Update(tea.KeyMsg{Type: step.key})
		model = updated.(Model)
		if model.options.OutputMode != step.want {
			t.Fatalf("第 %d 次按键 %v 后应处于态 %v，得到 %v", i+1, step.key, step.want, model.options.OutputMode)
		}
	}
}

// 空格循环三态，与测速模式行的既有习惯一致。
func TestOutputModeCyclesWithSpace(t *testing.T) {
	model := outputModel(t)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeySpace})
	if got := updated.(Model).options.OutputMode; got != OutputModeCustom {
		t.Fatalf("空格应切到自定义态: %v", got)
	}
}

// 点击 < > 也能循环三态：关闭态点 > 进自定义，点 < 反向到默认当前路径。
func TestOutputModeClickCycles(t *testing.T) {
	model := outputModel(t)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model = updated.(Model)
	// x 是值「< 关闭 >」的起点，即左箭头；右箭头从画面行尾量出。
	x, y := renderedOptionPosition(t, model, "输出路径")
	line := strings.Split(stripANSI(model.View()), "\n")[y]
	end := strings.LastIndex(line, ">")
	if end < 0 {
		t.Fatalf("关闭态应显示右箭头:\n%s", line)
	}
	rightX := lipgloss.Width(line[:end])

	updated, _ = model.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: rightX, Y: y})
	if got := updated.(Model).options.OutputMode; got != OutputModeCustom {
		t.Fatalf("点 > 应切到自定义态: %v", got)
	}

	// 右键循环回关闭态，再点左箭头反向到默认当前路径。
	model = updated.(Model)
	for model.options.OutputMode != OutputModeClosed {
		updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRight})
		model = updated.(Model)
	}
	updated, _ = model.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: x, Y: y})
	if got := updated.(Model).options.OutputMode; got != OutputModeDefaultPath {
		t.Fatalf("点 < 应反向切到默认当前路径态: %v", got)
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

// 三态的行内显示：关闭与默认当前路径带 < > 提示可循环，自定义态显示
// 词干（没打字时显示态名占位）。
func TestViewShowsOutputModeStates(t *testing.T) {
	cases := []struct {
		name string
		mode OutputMode
		path string
		want string
	}{
		{"关闭", OutputModeClosed, "", "< 关闭 >"},
		{"默认当前路径", OutputModeDefaultPath, "", "< 默认当前路径 >"},
		{"自定义未打字", OutputModeCustom, "", "自定义"},
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
