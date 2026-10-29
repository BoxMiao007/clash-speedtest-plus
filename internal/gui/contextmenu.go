package gui

import (
	"strings"

	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/components"
	"github.com/vivaui/viva/flex"
	"github.com/vivaui/viva/ui"
)

type menuEntry struct {
	item      components.ContextMenuItem
	separator bool
}

func (a *App) openNodeMenu(canvas *ui.Canvas, node *nodeState, x, y float32) {
	targets := []*nodeState{node}
	multi := false
	if node.selected && a.selectedCount > 1 {
		targets = a.selectedNodes()
		multi = true
	}
	allSelected := true
	for _, target := range targets {
		if !target.selected {
			allSelected = false
			break
		}
	}

	entries := make([]menuEntry, 0, 6)
	if multi {
		entries = append(entries, menuEntry{item: components.ContextMenuItem{
			ID: a.ids.menuCopyMulti, Label: "复制所选节点信息",
		}})
		entries = append(entries, menuEntry{item: components.ContextMenuItem{
			ID: a.ids.menuTestMulti, Label: "立即测试所选节点", Disabled: a.running(),
		}})
		entries = append(entries, menuEntry{separator: true})
		selectID, selectLabel := a.ids.menuSelect, "选中所选节点"
		if allSelected {
			selectID, selectLabel = a.ids.menuUnselMulti, "取消选中所选节点"
		}
		entries = append(entries, menuEntry{item: components.ContextMenuItem{ID: selectID, Label: selectLabel}})
		entries = append(entries, menuEntry{item: components.ContextMenuItem{ID: a.ids.menuExclMulti, Label: "排除所选节点"}})
	} else {
		entries = append(entries, menuEntry{item: components.ContextMenuItem{
			ID: a.ids.menuCopy, Label: "复制此节点信息",
		}})
		entries = append(entries, menuEntry{item: components.ContextMenuItem{
			ID: a.ids.menuTest, Label: "立即测试此节点", Disabled: a.running(),
		}})
		entries = append(entries, menuEntry{separator: true})
		selectID, selectLabel := a.ids.menuSelect, "选中此节点"
		if allSelected {
			selectID, selectLabel = a.ids.menuUnselect, "取消选中此节点"
		}
		entries = append(entries, menuEntry{item: components.ContextMenuItem{ID: selectID, Label: selectLabel}})
		entries = append(entries, menuEntry{item: components.ContextMenuItem{ID: a.ids.menuExclude, Label: "排除此节点"}})
	}

	clip := ""
	if multi {
		var builder strings.Builder
		for index, target := range targets {
			if index > 0 {
				builder.WriteString("\n\n")
			}
			builder.WriteString(target.clipText)
		}
		clip = builder.String()
	} else {
		clip = node.clipText
	}

	width, height := float32(160), float32(8)
	for _, entry := range entries {
		if entry.separator {
			height += 9
			continue
		}
		width = maxFloat(width, components.ContextMenuItemWidth(canvas, a.theme, entry.item))
		height += components.ContextMenuItemHeight(a.theme, entry.item, 0)
	}
	x = minFloat(maxFloat(x, 8), maxFloat(a.window.Width-width-8, 8))
	y = minFloat(maxFloat(y, 8), maxFloat(a.window.Height-height-8, 8))
	a.menu = menuState{
		open:   true,
		x:      x,
		y:      y,
		nodes:  targets,
		multi:  multi,
		clip:   clip,
		items:  entries,
		width:  width,
		height: height,
	}
}

func (a *App) paintMenu(f *flex.Frame) {
	if !a.menu.open {
		return
	}
	canvas := f.Canvas()
	bounds := ui.Rect{X: a.menu.x, Y: a.menu.y, Width: a.menu.width + 8, Height: a.menu.height}
	menu := components.BeginContextMenu(canvas, a.theme, bounds, components.ContextMenuOptions{
		Layer:     1,
		Scope:     a.ids.menuRoot,
		DismissID: a.ids.menuDismiss,
		Viewport:  ui.Rect{Width: a.window.Width, Height: a.window.Height},
	})
	for _, entry := range a.menu.items {
		if entry.separator {
			menu.AddSeparator()
			continue
		}
		menu.AddItem(entry.item)
		if canvas.Clicked(entry.item.ID) {
			a.activateMenu(entry.item.ID)
		}
	}
	menu.End()
	if canvas.Clicked(a.ids.menuDismiss) {
		a.menu.open = false
	}
}

func (a *App) activateMenu(id ui.ID) {
	nodes := a.menu.nodes
	a.menu.open = false
	switch id {
	case a.ids.menuCopy, a.ids.menuCopyMulti:
		if err := app.SetClipboardText(a.menu.clip); err != nil {
			a.status = "复制失败：" + err.Error()
			return
		}
		if a.menu.multi {
			a.status = "已复制所选节点信息"
		} else {
			a.status = "已复制此节点信息"
		}
	case a.ids.menuTest, a.ids.menuTestMulti:
		a.startNodesTest(nodes)
	case a.ids.menuSelect, a.ids.menuUnselect:
		selected := id == a.ids.menuSelect
		for _, node := range nodes {
			node.selected = selected
		}
		a.rebuild()
	case a.ids.menuExclude, a.ids.menuExclMulti:
		for _, node := range nodes {
			node.selected = false
		}
		a.rebuild()
		if a.menu.multi {
			a.status = "已排除所选节点"
		} else {
			a.status = "已排除此节点"
		}
	}
}

func minFloat(left, right float32) float32 {
	if left < right {
		return left
	}
	return right
}

func maxFloat(left, right float32) float32 {
	if left > right {
		return left
	}
	return right
}
