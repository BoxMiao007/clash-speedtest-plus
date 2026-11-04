package gui

import "github.com/vivaui/viva/ui"

// onShortcut 处理 Ctrl/Command 修饰的可打印和弦。输入框聚焦时，标准编辑
// 和弦先被平台文本系统消费（Ctrl+A 在输入框里是全选文本），因此这些动作
// 只在列表等位置生效。Alt/Meta 组合留给系统，不抢。
func (a *App) onShortcut(event ui.KeyEvent) ui.EventResult {
	// 模态对话框打开时，全局快捷键不生效，避免在录入/确认中被后台动作打断。
	if a.dialog != dialogNone {
		return ui.EventResult{}
	}
	if event.Release || event.Modifiers&ui.ModifierControl == 0 {
		return ui.EventResult{}
	}
	if event.Modifiers&(ui.ModifierAlt|ui.ModifierMeta) != 0 {
		return ui.EventResult{}
	}
	switch event.Text {
	case "a":
		if event.Modifiers&ui.ModifierShift != 0 {
			a.setAllNodesSelected(false)
		} else {
			a.setAllNodesSelected(true)
		}
		return ui.EventResult{Handled: true, Repaint: true}
	case "r":
		a.refetchAllSubscriptions()
		return ui.EventResult{Handled: true, Repaint: true}
	}
	return ui.EventResult{}
}

// onKey 先处理应用级命名键（F1 帮助、Ctrl+Enter 开测、Esc 关详情），其余
// 交给聚焦中的分隔条：方向键微调「节点来源」宽度。
func (a *App) onKey(event ui.KeyEvent) ui.EventResult {
	if event.Release {
		return ui.EventResult{}
	}
	// 模态对话框独占键盘：F1/Ctrl+Enter/Esc 等应用级动作不得穿透到背景。
	// 对话框自身的关闭由 flex.Dialog 的 DismissID 处理，不经过这里。
	if a.dialog != dialogNone {
		return ui.EventResult{}
	}
	// 键盘翻页：测速列表支持 PageUp/PageDown，纯键盘用户也能访问视口外的节点。
	if a.active == viewTest && !a.menu.open && !a.presetOpen.Open &&
		(event.Key == ui.KeyPageUp || event.Key == ui.KeyPageDown) {
		delta := a.nodeViewport.Height * 0.9
		if event.Key == ui.KeyPageUp {
			delta = -delta
		}
		return scrollList(&a.nodeOffset, a.nodeContent, a.nodeViewport.Height, delta)
	}
	if event.Key == ui.KeyF1 {
		a.openAbout()
		return ui.EventResult{Handled: true, Repaint: true}
	}
	if event.Key == ui.KeyEnter && event.Modifiers&ui.ModifierControl != 0 {
		if !a.running() && a.selectedCount > 0 {
			a.startSelectedTest()
		}
		return ui.EventResult{Handled: true, Repaint: true}
	}
	if event.Key == ui.KeyEscape && a.dialog == dialogNone && a.detailNode != nil {
		a.closeDetail()
		return ui.EventResult{Handled: true, Repaint: true}
	}
	if a.entriesCollapsed {
		return ui.EventResult{}
	}
	return a.entriesSplit.Key(event)
}
