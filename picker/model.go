package picker

import (
	"fmt"
	"regexp"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// Options 是选源界面上可填写的测速选项。没填的项沿用命令行默认值。
type Options struct {
	Filter         string
	Mode           string
	DownloadSize   string
	UploadSize     string
	MinDownload    string
	MinUpload      string
	ImageSpeedOnly bool
	OutputPath     string
	Rename         bool
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
	status      string
	started     bool
	options     Options
	fetching    bool
	quitting    bool
	focus       int
	address     string
	addressLine int
	optionIndex int
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
	return Model{
		configs:     session.Configs,
		cursor:      cursor,
		checked:     checked,
		addressLine: len(session.Configs) + 1,
		options: Options{
			Filter: ".+", Mode: "download", DownloadSize: "50", UploadSize: "20",
			MinDownload: "5", MinUpload: "2", Rename: true,
		},
	}
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
			m.move(1)
		case tea.KeyUp:
			m.move(-1)
		case tea.KeyLeft, tea.KeyRight:
			if m.focus == focusOptions {
				m.changeMode(msg.Type == tea.KeyRight)
			}
		case tea.KeyRunes:
			if m.focus == focusAddress {
				m.address += string(msg.Runes)
			} else if m.focus == focusOptions {
				m.typeOption(string(msg.Runes))
			}
		case tea.KeyBackspace:
			if m.focus == focusAddress && m.address != "" {
				m.address = m.address[:len(m.address)-1]
			}
		case tea.KeySpace:
			if m.focus == focusConfigs {
				m.toggle(m.cursor)
			} else if m.focus == focusAddress {
				m.address += " "
			}
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
				m.focus = focusConfigs
				m.cursor = msg.Y
				m.toggle(m.cursor)
			} else if msg.Y == m.addressLine {
				m.focus = focusAddress
			}
		}
	}
	return m, nil
}

const (
	focusConfigs = iota
	focusAddress
	focusOptions
)

func (m *Model) changeMode(next bool) {
	modes := []string{"fast", "download", "full"}
	index := 1
	for i, mode := range modes {
		if m.options.Mode == mode {
			index = i
		}
	}
	if next && index < len(modes)-1 {
		index++
	}
	if !next && index > 0 {
		index--
	}
	m.options.Mode = modes[index]
}

func (m *Model) typeOption(text string) {
	if m.optionIndex != 0 || !m.optionState().Enabled(OptionDownloadSize) {
		return
	}
	m.options.DownloadSize += text
}

func (m Model) optionState() OptionState {
	return OptionState{Mode: m.options.Mode, OutputPath: m.options.OutputPath}
}

func (m *Model) move(delta int) {
	if m.focus == focusConfigs && delta > 0 && (len(m.configs) == 0 || m.cursor >= len(m.configs)-1) {
		m.focus = focusAddress
		return
	}
	if m.focus == focusAddress && delta > 0 {
		m.focus = focusOptions
		return
	}
	if m.focus == focusAddress && delta < 0 && len(m.configs) > 0 {
		m.focus = focusConfigs
		m.cursor = len(m.configs) - 1
		return
	}
	if m.focus == focusOptions && delta < 0 {
		m.focus = focusAddress
		return
	}
	if m.focus != focusConfigs || len(m.configs) == 0 {
		return
	}
	next := m.cursor + delta
	if next < 0 || next >= len(m.configs) {
		return
	}
	m.cursor = next
}

func (m Model) hasSource() bool {
	if strings.TrimSpace(m.address) != "" {
		return true
	}
	for i, config := range m.configs {
		if config.Selectable && m.checked[i] {
			return true
		}
	}
	return false
}

func optionLine(state OptionState, option Option, label, value string) string {
	line := label + "  " + value
	if !state.Enabled(option) {
		line += "  不可用"
	}
	return line + "\n"
}

func boolText(on bool) string {
	if on {
		return "开"
	}
	return "关"
}

func modeLabel(mode string) string {
	switch mode {
	case "fast":
		return "快速"
	case "full":
		return "完整"
	default:
		return "下载"
	}
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
	b.WriteString("\n订阅地址  " + m.address + "\n")
	b.WriteString("测速模式  " + modeLabel(m.options.Mode) + "\n")
	state := m.optionState()
	b.WriteString(optionLine(state, OptionDownloadSize, "下载大小", m.options.DownloadSize))
	b.WriteString(optionLine(state, OptionMinDownload, "最低下载速度", m.options.MinDownload))
	b.WriteString(optionLine(state, OptionImageSpeedOnly, "结果图只留有速度", boolText(m.options.ImageSpeedOnly)))
	b.WriteString(optionLine(state, OptionUploadSize, "上传大小", m.options.UploadSize))
	b.WriteString(optionLine(state, OptionMinUpload, "最低上传速度", m.options.MinUpload))
	b.WriteString(optionLine(state, OptionRename, "重命名", boolText(m.options.Rename)))
	if m.status != "" {
		b.WriteString(m.status + "\n")
	}
	return b.String()
}
