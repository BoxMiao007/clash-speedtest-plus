package picker

import (
	"fmt"
	"regexp"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// Options 是选源界面上可填写的测速选项。没填的项沿用命令行默认值。
type Options struct {
	Filter string
}

// Session 是选源界面打开时已经准备好的配置列表。
type Session struct {
	Configs []ConfigEntry
}

// Model 是选源界面。checked 只记录合格配置是否打勾。
type Model struct {
	configs []ConfigEntry
	cursor  int
	checked []bool
	status   string
	started  bool
	options  Options
	fetching bool
	quitting bool
}

// New 用一份会话创建选源界面。有合格配置时光标停在第一条。
func New(session Session) Model {
	checked := make([]bool, len(session.Configs))
	cursor := 0
	for i, config := range session.Configs {
		if config.Selectable {
			cursor = i
			break
		}
	}
	return Model{configs: session.Configs, cursor: cursor, checked: checked}
}

// Update 处理勾选。空格和点击只切换当前合格配置。
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.fetching && (msg.Type == tea.KeyEsc || msg.Type == tea.KeyCtrlC || string(msg.Runes) == "q") {
			m.quitting = true
			return m, tea.Quit
		}
		switch msg.Type {
		case tea.KeyDown:
			if m.cursor < len(m.configs)-1 {
				m.cursor++
			}
		case tea.KeySpace:
			m.toggle(m.cursor)
		case tea.KeyEnter:
			if !m.hasSource() {
				m.status = "先勾选配置或填入订阅地址"
				break
			}
			if _, err := regexp.Compile(m.options.Filter); err != nil {
				m.status = "过滤正则不合法"
				break
			}
			m.started = true
		}
	case tea.MouseMsg:
		if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress {
			if msg.Y >= 0 && msg.Y < len(m.configs) {
				m.cursor = msg.Y
				m.toggle(m.cursor)
			}
		}
	}
	return m, nil
}

func (m Model) hasSource() bool {
	for i, config := range m.configs {
		if config.Selectable && m.checked[i] {
			return true
		}
	}
	return false
}

func (m *Model) toggle(index int) {
	if index < 0 || index >= len(m.configs) || !m.configs[index].Selectable {
		return
	}
	m.checked[index] = !m.checked[index]
}

// Init 满足 bubbletea 的界面接口。选源界面打开时没有后台任务。
func (m Model) Init() tea.Cmd { return nil }

// View 画出配置列表。打勾的合格配置带 ✓，不可选的带原因。
func (m Model) View() string {
	var b strings.Builder
	if len(m.configs) == 0 {
		b.WriteString("没有配置\n")
	}
	for i, config := range m.configs {
		mark := " "
		if m.checked[i] {
			mark = "✓"
		}
		line := fmt.Sprintf("%s %s", mark, config.Name)
		if !config.Selectable && config.Reason != "" {
			line += "  " + config.Reason
		}
		b.WriteString(line + "\n")
	}
	if m.status != "" {
		b.WriteString(m.status + "\n")
	}
	return b.String()
}
