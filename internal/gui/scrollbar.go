package gui

import (
	"github.com/vivaui/viva/components"
	"github.com/vivaui/viva/ui"
)

// barState 是一条滚动条的拖动状态。偏移量不放在这里：节点与结果列表的
// 偏移在 state 里，节点来源与设置由 flex.ScrollState 托管，滚动条只是它的
// 一种输入方式。
type barState struct {
	dragging bool
	grab     float32
}

// paintScrollBars 在整页绘制完成后补画可见滚动条。flex 的 ScrollArea 只做
// 裁剪与偏移，轨道、滑块与拖动归应用（Viva 的组件分工即如此）。只画当前
// 视图里可能溢出的区域；对话框或右键菜单打开时不画，避免盖住浮层。
func (a *App) paintScrollBars(canvas *ui.Canvas) {
	if a.dialog != dialogNone || a.menu.open {
		return
	}
	if !a.entriesCollapsed && a.entryScrollValid {
		a.paintScrollAreaBar(canvas, a.ids.entryBar, "节点来源滚动条", a.entryScrollBox.Rect(), a.ids.entryScroll, &a.entryBar)
	}
	switch a.active {
	case viewResults:
		if a.resultListValid {
			next, changed := a.paintScrollBar(canvas, a.ids.resultBar, "结果列表滚动条", a.resultViewport, a.resultOffset, a.resultContent, &a.resultBar)
			if changed {
				a.resultOffset = next
				a.clampResultOffset()
			}
		}
	case viewSettings:
		if a.settingsScrollValid {
			a.paintScrollAreaBar(canvas, a.ids.settingsBar, "设置滚动条", a.settingsScrollBox.Rect(), a.ids.settingsScroll, &a.settingsBar)
		}
	default:
		if a.nodeListValid {
			next, changed := a.paintScrollBar(canvas, a.ids.nodeBar, "节点列表滚动条", a.nodeViewport, a.nodeOffset, a.nodeContent, &a.nodeBar)
			if changed {
				a.nodeOffset = next
				a.clampNodeOffset()
			}
		}
	}
}

// paintScrollAreaBar 给 flex.ScrollArea 画滚动条：内容与视口高度来自它托管
// 的 ScrollState，拖动后直接 ScrollTo，布局下一帧按新偏移排版。
func (a *App) paintScrollAreaBar(canvas *ui.Canvas, barID ui.ID, label string, rect ui.Rect, areaID ui.ID, bar *barState) {
	if rect.Width <= 0 || rect.Height <= 0 {
		return
	}
	state := a.view.ScrollState(areaID)
	// rect 含 ScrollArea 的上下内边距；把内边距同时加进内容高度，最大偏移
	// 才和 ScrollState.Max 一致，滑块比例不会偏。
	padding := rect.Height - state.Viewport
	if padding < 0 {
		padding = 0
	}
	next, changed := a.paintScrollBar(canvas, barID, label, rect, state.Offset, state.Content+padding, bar)
	if changed {
		state.ScrollTo(next)
	}
}

// paintScrollBar 画一条纵向滚动条并处理拖动，返回拖动后的偏移与是否变化。
// 轨道比滑块宽，按下轨道会把滑块中心移到指针处（与常见桌面滚动条一致）。
func (a *App) paintScrollBar(canvas *ui.Canvas, id ui.ID, label string, viewport ui.Rect, offset, content float32, bar *barState) (float32, bool) {
	// 滑块用前景色的淡化版本：主题 Border 在白底上几乎看不见，而这里需要
	// 「有内容被遮住」一眼可见；悬停时组件自己会换成更实的颜色。
	barTheme := a.theme
	barTheme.Border = a.theme.MutedForeground
	barTheme.Border.Alpha *= 0.45
	area := components.BeginScrollArea(canvas, barTheme, viewport, ui.Size{Width: viewport.Width, Height: content}).
		Offset(0, offset).
		VerticalScrollbar(id, label, "")
	geometry := area.EndContent()
	if geometry.MaxOffsetY <= 0 {
		bar.dragging = false
		return offset, false
	}
	event, ok := canvas.PointerOn(id)
	if !ok {
		return offset, false
	}
	switch event.Kind {
	case ui.PointerDown:
		if event.Button != ui.PointerButtonPrimary && event.Button != ui.PointerButtonNone {
			return offset, false
		}
		thumb := geometry.VerticalThumb
		bar.grab = thumb.Height / 2
		if event.Y >= thumb.Y && event.Y < thumb.Y+thumb.Height {
			bar.grab = event.Y - thumb.Y
		}
		bar.dragging = true
	case ui.PointerMove:
		if !bar.dragging {
			return offset, false
		}
	default:
		if !bar.dragging {
			return offset, false
		}
		bar.dragging = false
		canvas.Consume()
		return offset, false
	}
	next := geometry.VerticalOffsetForThumb(event.Y - bar.grab)
	canvas.Consume()
	return next, next != offset
}
