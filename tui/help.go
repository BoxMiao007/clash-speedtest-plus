package tui

import (
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
		},
	}
	state.setDetailVisible(false)
	state.setPaused(false)
	state.setEarlyStopped(false)
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

func (h helpState) view() string {
	return h.model.View(h.keyMap)
}

func (h helpState) height() int {
	view := h.view()
	if view == "" {
		return 0
	}
	return lipgloss.Height(view)
}

func (km helpKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{
		km.Table.LineUp,
		km.Table.LineDown,
		km.TogglePause,
		km.Quit,
		km.CloseDetail,
	}
}

func (km helpKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{km.Table.LineUp, km.Table.LineDown, km.Table.GotoTop, km.Table.GotoBottom},
		{km.Table.PageUp, km.Table.PageDown, km.Table.HalfPageUp, km.Table.HalfPageDown},
		{km.TogglePause, km.CloseDetail, km.Quit},
	}
}
