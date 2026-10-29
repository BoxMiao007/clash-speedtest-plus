package gui

import (
	"github.com/vivaui/viva/flex"
	"github.com/vivaui/viva/ui"
)

func (a *App) paintEntriesPanel(f *flex.Frame) {
	f.Column().Width(320).FillHeight().Children(func() {
		f.Row().Height(54).PadX(16).Items(flex.Center).Gap(8).Children(func() {
			f.Text("测试文件").Bold()
			f.Spacer()
			if f.Button(a.ids.newEntry, "新建条目").Small().Outline().Disabled(a.running()).Clicked() {
				a.openEntryDialog(nil)
			}
		})
		f.ScrollArea(a.ids.entryScroll).Grow(1).FillWidth().PadSides(12, 12, 12, 12).Gap(10).Children(func() {
			if len(a.entries) == 0 {
				f.Column().Pad(10).Gap(6).Children(func() {
					f.Text("还没有测试文件条目。").FontSize(13).Bold()
					f.Text("点上方「新建条目」，选择一份 Clash/Mihomo 配置文件。").FontSize(12).Muted()
				})
				return
			}
			for _, entry := range a.entries {
				a.paintEntryCard(f, entry)
			}
		})
	})
}

func (a *App) paintEntryCard(f *flex.Frame, entry *entryState) {
	f.Column().FillWidth().Pad(10).Gap(5).Background(a.theme.Card).Border(1, a.theme.Border).Radius(a.theme.Metrics.RadiusSurface).Children(func() {
		f.Row().Items(flex.Center).Gap(8).Children(func() {
			check := f.Checkbox(entry.ids.check, entry.selected).
				Label(entry.checkLabel).
				Disabled(a.running())
			if check.Clicked() {
				f.Canvas().AfterPaint(func() { a.toggleEntry(entry) })
			}
			f.Text(entry.entry.Name).Bold().Ellipsis().Grow(1)
		})
		f.Text(entry.pathText).FontSize(11).Muted().Ellipsis()
		f.Text(entry.absText).FontSize(11).Muted().Ellipsis()
		statusColor := a.theme.MutedForeground
		if entry.loadErr != "" || entry.missing {
			statusColor = a.theme.Danger
		}
		f.Text(entry.statusText).FontSize(11).Color(statusColor).Ellipsis()
		f.Row().Gap(6).Children(func() {
			if f.Button(entry.ids.read, "读取").XSmall().Outline().Disabled(a.running()).Clicked() {
				f.Canvas().AfterPaint(func() { a.readEntry(entry) })
			}
			if f.Button(entry.ids.refetch, "重新获取").XSmall().Outline().Disabled(a.running()).Clicked() {
				f.Canvas().AfterPaint(func() { a.refetchEntry(entry) })
			}
			if f.Button(entry.ids.edit, "编辑").XSmall().Ghost().Disabled(a.running()).Clicked() {
				a.openEntryDialog(entry)
			}
			if f.Button(entry.ids.remove, "删除").XSmall().Ghost().Destructive().Disabled(a.running()).Clicked() {
				a.askDeleteEntry(entry)
			}
		})
	})
}

func (a *App) paintNodes(f *flex.Frame) {
	f.Row().Height(52).PadX(20).Items(flex.Center).Gap(8).Children(func() {
		f.Text(a.countText).FontSize(12).Muted()
		f.Spacer()
		if f.Button(a.ids.excludeFailed, "排除上次失败").Small().Outline().Disabled(a.running()).Clicked() {
			a.excludeFailed()
		}
		if f.Button(a.ids.readNodes, "读取节点").Small().Outline().Disabled(a.running()).Clicked() {
			f.Canvas().AfterPaint(func() { a.readSelectedEntries() })
		}
		if f.Button(a.ids.startTest, "下载速度").Small().Disabled(a.running() || a.selectedCount == 0).Clicked() {
			a.startSelectedTest()
		}
	})
	if len(a.nodeItems) == 0 {
		a.paintNodeEmpty(f)
		return
	}
	viewport := a.nodeViewport.Height
	if viewport <= 0 {
		viewport = a.window.Height - 195
		if viewport < 200 {
			viewport = 200
		}
	}
	list := f.Stack().Grow(1).FillWidth().Clip()
	a.nodeListBox = list
	a.nodeListValid = true
	list.Children(func() {
		for _, item := range a.nodeItems {
			top := item.y - a.nodeOffset
			if top+item.height < 0 || top > viewport {
				continue
			}
			row := f.Column().Absolute().Top(top).Left(0).Right(0).Height(item.height).PadX(20)
			if item.kind == itemHeader {
				row.Children(func() {
					a.paintGroupHeader(f, item.group)
				})
				continue
			}
			row.Children(func() {
				a.paintNodeRow(f, item.node)
			})
		}
	})
}

func (a *App) paintNodeEmpty(f *flex.Frame) {
	if len(a.entries) == 0 {
		a.paintNoEntries(f)
		return
	}
	f.Column().Grow(1).FillWidth().PadSides(12, 20, 16, 20).PadY(24).Gap(6).Children(func() {
		if a.totalCount == 0 {
			f.Text("当前条目还没有读取到节点。").FontSize(13).Muted()
			for _, entry := range a.entries {
				if !entry.selected {
					continue
				}
				f.Row().Height(36).FillWidth().Gap(8).Items(flex.Center).Children(func() {
					f.Text(entry.entry.Name).Bold()
					f.Text(entry.statusText).FontSize(11).Muted().Ellipsis().Grow(1)
					if f.Button(entry.ids.read, "读取").Small().Outline().Clicked() {
						f.Canvas().AfterPaint(func() { a.readEntry(entry) })
					}
				})
			}
			return
		}
		f.Text("没有节点匹配当前过滤条件。").FontSize(13).Muted()
		f.Text("清空过滤表达式或换一个关键词再试。").FontSize(12).Muted()
	})
}

func (a *App) paintNoEntries(f *flex.Frame) {
	f.Column().Grow(1).FillWidth().PadY(32).Items(flex.Center).Gap(8).Children(func() {
		f.Text("欢迎使用订阅测速").FontSize(18).Bold()
		f.Text("还没有测试文件条目。点左上角「新建条目」，添加一份 Clash/Mihomo 配置文件。").FontSize(12).Muted()
	})
}

func (a *App) paintGroupHeader(f *flex.Frame, group *groupState) {
	f.Row().Height(32).FillWidth().Items(flex.Center).Gap(8).Children(func() {
		f.Text(group.entry.entry.Name).Bold()
		f.Text(group.countText).FontSize(11).Muted()
		f.Spacer()
		if f.Button(group.entry.ids.selectAll, "全选").XSmall().Ghost().Clicked() {
			a.setGroupSelected(group, true)
		}
		if f.Button(group.entry.ids.selectNone, "全不选").XSmall().Ghost().Clicked() {
			a.setGroupSelected(group, false)
		}
	})
}

func (a *App) paintNodeRow(f *flex.Frame, node *nodeState) {
	row := f.Row().Height(38).FillWidth().PadX(8).Gap(10).Items(flex.Center).
		Radius(a.theme.Metrics.RadiusItem()).
		HoverBackground(a.theme.Muted).
		Hit(node.ids.row, node.rowLabel)
	if node.selected {
		row = row.Background(a.theme.Muted)
	}
	row.Children(func() {
		check := f.Checkbox(node.ids.check, node.selected).
			Label(node.checkLabel)
		if check.Clicked() {
			a.toggleNode(node)
		}
		f.Column().Gap(0).Grow(1).Children(func() {
			f.Text(node.name).Ellipsis()
			f.Text(node.subtitle).FontSize(11).Muted().Ellipsis()
		})
		infoColor := a.theme.MutedForeground
		if node.hasResult && !node.result.OK {
			infoColor = a.theme.Danger
		}
		f.Text(node.infoText).FontSize(11).Color(infoColor).Ellipsis().MaxWidth(250)
	})
	if row.Clicked() {
		a.toggleNode(node)
	}
	a.handleNodePointer(f.Canvas(), node.ids.row, node)
	a.handleNodePointer(f.Canvas(), node.ids.check, node)
}

// handleNodePointer 在行或复选框上处理右键；左键仍由 Clicked 处理。
func (a *App) handleNodePointer(canvas *ui.Canvas, id ui.ID, node *nodeState) {
	event, ok := canvas.PointerOn(id)
	if !ok {
		return
	}
	if event.Kind == ui.PointerDown && event.Button == ui.PointerButtonSecondary {
		a.openNodeMenu(canvas, node, event.X, event.Y)
		canvas.Consume()
	}
}
