package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
)

type helpState struct {
	model  help.Model
	keyMap helpKeyMap
}

type helpKeyMap struct {
	Quit        key.Binding
	CloseDetail key.Binding
	TogglePause key.Binding
	SaveImage   key.Binding
	SwitchRound key.Binding
	Table       table.KeyMap
}

func newHelpState(tableKeys table.KeyMap) helpState {
	state := helpState{
		model: help.New(),
		keyMap: helpKeyMap{
			Table: tableKeys,
			Quit: key.NewBinding(
				key.WithKeys("q", "ctrl+c"),
				key.WithHelp("q/ctrl+c", "退出"),
			),
			CloseDetail: key.NewBinding(
				key.WithKeys("esc"),
				key.WithHelp("esc", "关闭详情"),
			),
			TogglePause: key.NewBinding(
				key.WithKeys(" "),
				key.WithHelp("空格", "暂停"),
			),
			SaveImage: key.NewBinding(
				key.WithKeys("s"),
				key.WithHelp("s", "保存结果图"),
			),
			SwitchRound: key.NewBinding(
				key.WithKeys("ctrl+up", "ctrl+down"),
				key.WithHelp("ctrl+↑/↓", "轮间切换"),
			),
		},
	}
	state.setDetailVisible(false)
	state.setPaused(false)
	state.setEarlyStopped(false)
	state.setRoundSwitch(false)
	return state
}

func (h *helpState) setWidth(width int) {
	h.model.Width = width
}

func (h *helpState) setDetailVisible(visible bool) {
	h.keyMap.CloseDetail.SetEnabled(visible)
}

// setPaused 切换空格键提示文案；暂停中显示「继续」。
func (h *helpState) setPaused(paused bool) {
	if paused {
		h.keyMap.TogglePause.SetHelp("空格", "继续")
	} else {
		h.keyMap.TogglePause.SetHelp("空格", "暂停")
	}
}

// setEarlyStopped 提前结束后空格无效果，帮助条隐藏空格项。
func (h *helpState) setEarlyStopped(stopped bool) {
	h.keyMap.TogglePause.SetEnabled(!stopped)
}

// setRoundSwitch 多轮队列的帮助行亮出 Ctrl+↑/↓：每个轮视图就位即启用，
// 首个视图也不例外；单轮队列键位无效，不标注。
func (h *helpState) setRoundSwitch(enabled bool) {
	h.keyMap.SwitchRound.SetEnabled(enabled)
}

// setSaving 等待落盘时帮助条只留强制退出。
func (h *helpState) setSaving(saving bool) {
	h.keyMap.TogglePause.SetEnabled(!saving)
	h.keyMap.SaveImage.SetEnabled(!saving)
	h.keyMap.CloseDetail.SetEnabled(!saving)
	h.keyMap.Table.LineUp.SetEnabled(!saving)
	h.keyMap.Table.LineDown.SetEnabled(!saving)
	h.keyMap.Table.PageUp.SetEnabled(!saving)
	h.keyMap.Table.PageDown.SetEnabled(!saving)
	h.keyMap.Table.HalfPageUp.SetEnabled(!saving)
	h.keyMap.Table.HalfPageDown.SetEnabled(!saving)
	h.keyMap.Table.GotoTop.SetEnabled(!saving)
	h.keyMap.Table.GotoBottom.SetEnabled(!saving)
	if saving {
		h.keyMap.Quit.SetHelp("再按 q 或 Ctrl+C", "强制退出")
	} else {
		h.keyMap.Quit.SetHelp("q/ctrl+c", "退出")
	}
}

func (h helpState) view() string {
	return h.model.View(h.keyMap)
}

// versionBadgeStyle 是右下角版本角标的灰显样式，与帮助条键位拉开一档。
var versionBadgeStyle = lipgloss.NewStyle().Faint(true)

// fullHelpWidth 量出帮助条完整（不截断）键位文案的显示宽度。bubbles 在
// 窄屏会自行截断，直接量渲染结果会把「截断后的残行」当成放得下；这里借
// bubbles 以不限宽再渲染一次。View 走值接收者，改的是副本宽度，不动真实模型。
func (h helpState) fullHelpWidth() int {
	width := h.model.Width
	h.model.Width = 0 // bubbles 以 0 宽为不限宽
	full := h.view()
	h.model.Width = width
	return lipgloss.Width(full)
}

// helpWithVersion 把版本角标挂到帮助条右端（见 CONTEXT.md「版本角标」词条）。
// 判满规则与 picker/view.go helpLine 同一口径：完整帮助文案优先——bubbles
// 截断剩下的空间不算数，完整文案加两格间隔放不下角标就藏。m.version 已是
// 整理好的角标文案（v 前缀在调用方处理）。帮助条是一行，挂到行尾右对齐。
func (m tuiModel) helpWithVersion(helpView string) string {
	badge := m.version
	if badge == "" || m.windowWidth <= 0 {
		return helpView
	}
	badgeW := lipgloss.Width(badge)
	if m.help.fullHelpWidth()+2+badgeW > m.windowWidth {
		return helpView
	}
	gap := m.windowWidth - lipgloss.Width(helpView) - badgeW
	return helpView + strings.Repeat(" ", gap) + versionBadgeStyle.Render(badge)
}

func (h helpState) height() int {
	view := h.view()
	if view == "" {
		return 0
	}
	return lipgloss.Height(view)
}

func (km helpKeyMap) ShortHelp() []key.Binding {
	if !km.Quit.Enabled() {
		return nil
	}
	if km.Quit.Help().Key == "再按 q 或 Ctrl+C" {
		return []key.Binding{km.Quit}
	}
	return []key.Binding{
		km.Table.LineUp,
		km.Table.LineDown,
		km.SwitchRound,
		km.TogglePause,
		km.SaveImage,
		km.Quit,
		km.CloseDetail,
	}
}

func (km helpKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{km.Table.LineUp, km.Table.LineDown, km.Table.GotoTop, km.Table.GotoBottom},
		{km.Table.PageUp, km.Table.PageDown, km.Table.HalfPageUp, km.Table.HalfPageDown},
		{km.TogglePause, km.SwitchRound, km.SaveImage, km.CloseDetail, km.Quit},
	}
}
