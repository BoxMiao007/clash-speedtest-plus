package gui

import (
	"github.com/vivaui/viva/flex"
	"github.com/vivaui/viva/ui"
)

// 节点来源面板的宽度约束：默认 320，可拖动到 220；折叠后只留一条窄栏。
const (
	entriesPanelWidth float32 = 320
	entriesPanelMin   float32 = 220
	entriesRailWidth  float32 = 56
	contentPanelMin   float32 = 420
)

// 合并视图（测速）的列宽。节点名列吃剩余宽度；状态与数值列定宽右对齐。
const (
	colCheck   float32 = 40
	colStatus  float32 = 52
	colLatency float32 = 62
	colJitter  float32 = 54
	colLoss    float32 = 48
	colSpeed   float32 = 88
	colUpload  float32 = 84
	colTested  float32 = 104
)

// paintTest 画合并后的「测速」视图：上半部是选择与开测工具条，下半部是
// 同一个列表——勾选节点、看结果、排序、展开详情都在这一张表里完成。
func (a *App) paintTest(f *flex.Frame) {
	a.paintTestToolbar(f)
	a.paintTestHeader(f)
	if len(a.nodeItems) == 0 {
		a.paintNodeEmpty(f)
		return
	}
	viewport := a.nodeViewport.Height
	if viewport <= 0 {
		viewport = a.window.Height - 236
		if viewport < 200 {
			viewport = 200
		}
	}
	if a.detailNode != nil {
		viewport -= detailPanelHeight
		if viewport < 120 {
			viewport = 120
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
	if a.detailNode != nil {
		a.paintDetailPanel(f)
	}
}

// showResultBar 决定是否显示结果条：跑过测速、有结果或在导出时才有意义，
// 第一次打开时把纵向空间留给节点列表。
func (a *App) showResultBar() bool {
	return a.running() || len(a.rounds) > 0 || len(a.results) > 0 || a.exporting > 0
}

func (a *App) paintTestToolbar(f *flex.Frame) {
	f.Row().Height(52).FillWidth().PadX(20).Items(flex.Center).Gap(8).Children(func() {
		f.Text(a.countText).FontSize(12).Muted().Ellipsis()
		f.Spacer()
		if f.Button(a.ids.excludeFailed, "排除上次失败").Small().Outline().Disabled(a.running()).Clicked() {
			a.excludeFailed()
		}
		if f.Button(a.ids.readNodes, "读取节点").Small().Outline().Disabled(a.running()).Clicked() {
			f.Canvas().AfterPaint(func() { a.readSelectedEntries() })
		}
		if a.running() {
			if f.Button(a.ids.pauseTest, a.pauseLabel).Small().Outline().Clicked() {
				f.Canvas().AfterPaint(func() { a.togglePause() })
			}
			if f.Button(a.ids.stopTest, "停止测速").Small().Destructive().Clicked() {
				a.stopTest()
			}
			return
		}
		if f.Button(a.ids.startTest, "下载速度").Small().Disabled(a.selectedCount == 0).Clicked() {
			a.startSelectedTest()
		}
	})
	if !a.showResultBar() {
		return
	}
	f.Row().Height(40).FillWidth().PadX(20).Items(flex.Center).Gap(8).Children(func() {
		f.Text(a.resultText).FontSize(11).Muted().Ellipsis().Grow(1)
		if len(a.rounds) > 1 {
			if f.Button(a.ids.roundPrev, "上一轮").XSmall().Outline().Disabled(a.roundIndex == 0).Clicked() {
				a.switchRound(-1)
			}
			if f.Button(a.ids.roundNext, "下一轮").XSmall().Outline().Disabled(a.roundIndex >= len(a.rounds)-1).Clicked() {
				a.switchRound(1)
			}
		}
		disabled := a.exporting > 0 || len(a.results) == 0
		if f.Button(a.ids.exportYAML, "导出配置").XSmall().Outline().Disabled(disabled).Clicked() {
			f.Canvas().AfterPaint(func() { a.startManualExport(manualYAML) })
		}
		if f.Button(a.ids.exportImage, "结果图").XSmall().Outline().Disabled(disabled).Clicked() {
			f.Canvas().AfterPaint(func() { a.startManualExport(manualImage) })
		}
		if f.Button(a.ids.exportTSV, "结果表").XSmall().Outline().Disabled(disabled).Clicked() {
			f.Canvas().AfterPaint(func() { a.startManualExport(manualTSV) })
		}
	})
}

func (a *App) paintTestHeader(f *flex.Frame) {
	f.Row().Height(30).FillWidth().PadX(20).Items(flex.Center).Children(func() {
		f.Row().FillWidth().PadX(8).Gap(8).Items(flex.Center).Children(func() {
			f.Spacer().Width(colCheck).Grow(0)
			f.Text("节点").FontSize(11).Muted().Grow(1)
			f.Text("状态").FontSize(11).Muted().Width(colStatus)
			a.sortHeaderCell(f, a.headerLatency, a.sortHitLatency, a.ids.sortLatency, sortByLatency, colLatency)
			a.sortHeaderCell(f, a.headerJitter, a.sortHitJitter, a.ids.sortJitter, sortByJitter, colJitter)
			a.sortHeaderCell(f, a.headerLoss, a.sortHitLoss, a.ids.sortLoss, sortByLoss, colLoss)
			a.sortHeaderCell(f, a.headerSpeed, a.sortHitSpeed, a.ids.sortSpeed, sortBySpeed, colSpeed)
			if a.showUpload {
				a.sortHeaderCell(f, a.headerUpload, a.sortHitUpload, a.ids.sortUpload, sortByUpload, colUpload)
			}
			a.sortHeaderCell(f, a.headerTested, a.sortHitTested, a.ids.sortTested, sortByTested, colTested)
		})
	})
	f.Column().Height(1).FillWidth().PadX(20).Children(func() {
		f.Column().FillWidth().Height(1).Background(a.theme.Border)
	})
}

func (a *App) sortHeaderCell(f *flex.Frame, label, hitLabel string, id ui.ID, key sortKey, width float32) {
	// 右侧留 3px 内边距：平台字体与参考字宽对箭头字形有 1–2px 偏差，直接
	// 右对齐会让文字顶出列边界；命中区仍是整列。hitLabel 预计算，绘制不拼串。
	cell := f.Text(label).FontSize(11).Muted().Width(width).
		PadSides(0, 3, 0, 0).TextEnd().Hit(id, hitLabel)
	if cell.Clicked() {
		a.setSort(key)
	}
}

// setSort 切换排序列：点同一列反转方向，换列用该列的默认方向。合并视图
// 里排序只作用于每个条目分组内部。
func (s *state) setSort(key sortKey) {
	if s.sortKey == key {
		s.sortAsc = !s.sortAsc
	} else {
		s.sortKey = key
		s.sortAsc = key == sortByLatency || key == sortByName
	}
	s.refreshSortHeaders()
	s.applySort()
}

// refreshSortHeaders 预计算表头文案里的升降序箭头，Paint 只读字段。
func (s *state) refreshSortHeaders() {
	mark := func(key sortKey, base string) string {
		if s.sortKey != key {
			return base
		}
		if s.sortAsc {
			return base + " ▲"
		}
		return base + " ▼"
	}
	s.headerLatency = mark(sortByLatency, "延迟")
	s.headerJitter = mark(sortByJitter, "抖动")
	s.headerLoss = mark(sortByLoss, "丢包")
	s.headerSpeed = mark(sortBySpeed, "下载速度")
	s.headerUpload = mark(sortByUpload, "上传速度")
	s.headerTested = mark(sortByTested, "上次测速")
	// 无障碍名称同样预计算：绘制路径里不做字符串拼接，保持 0 分配。
	s.sortHitLatency = "按" + s.headerLatency + "排序"
	s.sortHitJitter = "按" + s.headerJitter + "排序"
	s.sortHitLoss = "按" + s.headerLoss + "排序"
	s.sortHitSpeed = "按" + s.headerSpeed + "排序"
	s.sortHitUpload = "按" + s.headerUpload + "排序"
	s.sortHitTested = "按" + s.headerTested + "排序"
}

// refreshModeFlags 按当前模式决定列表要不要上传列。
func (s *state) refreshModeFlags() {
	s.showUpload = s.settings.Mode == "full"
	s.pauseLabel = "暂停"
	if s.paused {
		s.pauseLabel = "继续"
	}
}

func (a *App) paintNodeRow(f *flex.Frame, node *nodeState) {
	row := f.Row().Height(40).FillWidth().PadX(8).Gap(8).Items(flex.Center).
		Radius(a.theme.Metrics.RadiusItem()).
		HoverBackground(a.theme.Muted).
		Hit(node.ids.row, node.rowLabel)
	if node.selected || node.phase == phaseTesting || a.detailKey == node.key {
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
			f.Text(node.subtitle).FontSize(10).Muted().Ellipsis()
		})
		statusColor := a.theme.MutedForeground
		switch {
		case node.phase == phaseTesting:
			statusColor = a.theme.Primary
		case node.hasResult && !node.result.OK:
			statusColor = a.theme.Danger
		}
		f.Text(node.statusText).FontSize(11).Color(statusColor).Width(colStatus).Ellipsis()
		f.Text(node.latencyText).FontSize(11).Muted().Width(colLatency).TextEnd()
		f.Text(node.jitterText).FontSize(11).Muted().Width(colJitter).TextEnd()
		f.Text(node.lossText).FontSize(11).Muted().Width(colLoss).TextEnd()
		speedColor := a.theme.Foreground
		if node.hasResult && !node.result.OK {
			speedColor = a.theme.Danger
		}
		f.Text(node.speedText).FontSize(11).Color(speedColor).Width(colSpeed).TextEnd()
		if a.showUpload {
			f.Text(node.uploadText).FontSize(11).Muted().Width(colUpload).TextEnd()
		}
		f.Text(node.lastTestText).FontSize(11).Muted().Width(colTested).Ellipsis()
	})
	// 复选框负责勾选，行本身打开详情；右键菜单同时提供两种操作。
	if row.Clicked() {
		a.toggleDetail(node)
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

// toggleDetail 展开/收起某个节点的详情面板；文案在切换时算好，Paint 只读。
func (a *App) toggleDetail(node *nodeState) {
	if a.detailKey == node.key {
		a.detailKey = ""
		a.detailNode = nil
		return
	}
	a.detailKey = node.key
	a.detailNode = node
	a.detailText = detailText(node)
}

// detailPanelHeight 是详情面板的固定高度（含标题行）。
const detailPanelHeight float32 = 148

func (a *App) paintDetailPanel(f *flex.Frame) {
	f.Column().Height(detailPanelHeight).FillWidth().PadX(20).PadY(8).Children(func() {
		panel := f.Column().FillWidth().Height(detailPanelHeight-16).Pad(10).Gap(4).
			Background(a.theme.Card).Border(1, a.theme.Border).Radius(a.theme.Metrics.RadiusSurface)
		panel.Children(func() {
			f.Row().FillWidth().Items(flex.Center).Gap(8).Children(func() {
				f.Text(a.detailNode.name).Bold().Ellipsis().Grow(1)
				if f.Button(a.ids.closeDetail, "关闭详情").XSmall().Ghost().Clicked() {
					a.detailKey = ""
					a.detailNode = nil
				}
			})
			f.Paragraph(a.detailText, 6).FontSize(11).Muted()
		})
	})
}

// detailText 汇总节点的全部测速信息，供详情面板显示。
func detailText(node *nodeState) string {
	lines := []string{
		"类型：" + node.typ + " · 服务器：" + node.serverText + " · 条目：" + node.entry.entry.Name,
	}
	if !node.hasResult {
		lines = append(lines, "状态：尚未测试")
		return joinLines(lines)
	}
	lines = append(lines,
		"延迟："+node.latencyText+" · 抖动："+node.jitterText+" · 丢包："+node.lossText,
		"下载速度："+node.speedText+" · 上传速度："+node.uploadText,
		"上次测速："+node.result.TestedAt.Local().Format("2006-01-02 15:04:05"),
	)
	if node.result.DownloadError != "" {
		lines = append(lines, "下载错误："+node.result.DownloadError)
	}
	if node.result.UploadError != "" {
		lines = append(lines, "上传错误："+node.result.UploadError)
	}
	return joinLines(lines)
}

func (a *App) paintEntriesPanel(f *flex.Frame) {
	f.Column().FillWidth().FillHeight().Children(func() {
		f.Row().Height(54).PadX(16).Items(flex.Center).Gap(8).Children(func() {
			f.Text("节点来源").Bold()
			f.Spacer()
			if f.Button(a.ids.entryCollapse, "折叠").XSmall().Ghost().Clicked() {
				a.entriesCollapsed = true
			}
			if f.Button(a.ids.newEntry, "新建条目").Small().Outline().Disabled(a.running()).Clicked() {
				a.openEntryDialog(nil)
			}
		})
		scroll := f.ScrollArea(a.ids.entryScroll).Grow(1).FillWidth().PadSides(12, 12, 12, 12).Gap(10)
		a.entryScrollBox, a.entryScrollValid = scroll, true
		scroll.Children(func() {
			if len(a.entries) == 0 {
				f.Column().Pad(10).Gap(6).Children(func() {
					f.Text("还没有节点来源条目。").FontSize(13).Bold()
					f.Text("点上方「新建条目」，选择本地配置文件或填订阅地址。").FontSize(12).Muted()
				})
				return
			}
			for _, entry := range a.entries {
				a.paintEntryCard(f, entry)
			}
		})
	})
}

// paintEntriesRail 是折叠后的窄栏：只留展开按钮，宽度固定，不占内容。
func (a *App) paintEntriesRail(f *flex.Frame) {
	f.Column().Width(entriesRailWidth).FillHeight().PadY(10).PadX(8).Gap(8).Children(func() {
		if f.Button(a.ids.entryExpand, "展开").XSmall().Outline().Clicked() {
			a.entriesCollapsed = false
		}
		f.Text("来源").FontSize(10).Muted()
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
		if entry.absText != "" {
			f.Text(entry.absText).FontSize(11).Muted().Ellipsis()
		}
		statusColor := a.theme.MutedForeground
		if entry.loadErr != "" || entry.missing {
			statusColor = a.theme.Danger
		}
		f.Text(entry.statusText).FontSize(11).Color(statusColor).Ellipsis()
		f.Row().Gap(6).Children(func() {
			if f.Button(entry.ids.read, entry.readLabel).XSmall().Outline().Disabled(a.running() || entry.fetching).Clicked() {
				f.Canvas().AfterPaint(func() { a.readEntry(entry) })
			}
			if f.Button(entry.ids.refetch, entry.fetchLabel).XSmall().Outline().Disabled(a.running() || entry.fetching).Clicked() {
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
		f.Text("还没有节点来源条目。点左上角「新建条目」，添加本地配置文件或订阅地址。").FontSize(12).Muted()
	})
}
