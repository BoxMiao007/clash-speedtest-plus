package gui

import (
	"unicode/utf8"

	"github.com/vivaui/viva/flex"
	"github.com/vivaui/viva/ui"
)

// 节点来源面板的宽度约束：默认 320，可拖动到 220；折叠后只留一条窄栏。
const (
	entriesPanelWidth float32 = 320
	entriesPanelMin   float32 = 260
	entriesRailWidth  float32 = 44
	contentPanelMin   float32 = 420
)

// 合并视图（测速）的列宽。节点名列吃剩余宽度；状态与数值列定宽右对齐。
const (
	// groupBarHeight 是固定在列表顶部的分组标题高度（32px 标题 + 1px 分隔线）。
	groupBarHeight float32 = 33

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
	// nodeViewport 是上一帧列表（Stack）的实际高度；只有首帧没有记录时
	// 才用窗口高度估算。
	viewport := a.nodeViewport.Height
	if viewport <= 0 {
		viewport = a.window.Height - 236
		if viewport < 200 {
			viewport = 200
		}
	}
	list := f.Stack().Grow(1).FillWidth().Clip()
	a.nodeListBox = list
	a.nodeListValid = true
	// 当前停在视口顶部的条目分组：组标题滚出视口后固定在列表顶部，滚到
	// 下一个条目时自动换成下一组的标题（ADR-0023）。
	var sticky *groupState
	for _, item := range a.nodeItems {
		if item.kind != itemHeader {
			continue
		}
		if item.y-a.nodeOffset <= 0 {
			sticky = item.group
			continue
		}
		break
	}
	list.Children(func() {
		for _, item := range a.nodeItems {
			top := item.y - a.nodeOffset
			// 用窗口高度而不是上一帧的列表视口做下限：窗口刚拉高时这一帧
			// 也能画满新高度（超出列表的部分由 Stack 的 Clip 裁掉）。
			if top+item.height < 0 || top > a.window.Height {
				continue
			}
			if item.kind == itemHeader && item.group == sticky {
				// 这一组的固定标题由下面的浮层画，避免同一身份出现两次。
				continue
			}
			if sticky != nil {
				// 固定标题盖住的行要单独裁剪：只画 33px 以下的部分，命中区也
				// 随裁剪收窄，避免点到被盖住的节点；内容本身不移动，不产生跳动。
				bottom := top + item.height
				if bottom <= groupBarHeight {
					continue
				}
				if top < groupBarHeight {
					clip := f.Column().Absolute().Top(groupBarHeight).Left(0).Right(0).
						Height(bottom - groupBarHeight).Clip()
					clip.Children(func() {
						a.paintListRow(f, top-groupBarHeight, item)
					})
					continue
				}
			}
			a.paintListRow(f, top, item)
		}
		if sticky != nil {
			// 被盖住的行已经按 33px 裁掉，固定标题空白处不会命中下面任何行。
			bar := f.Column().Absolute().Top(0).Left(0).Right(0).Height(groupBarHeight).Background(a.theme.Background)
			bar.Children(func() {
				f.Column().Height(32).FillWidth().PadX(20).Children(func() {
					a.paintGroupHeader(f, sticky)
				})
				f.Column().Height(1).FillWidth().Background(a.theme.Border)
			})
		}
	})
}

// paintListRow 在当前位置声明一行（分组标题或节点行）。top 是相对列表的
// 坐标，固定标题存在时可能为负（被裁剪的滚动内容）。
func (a *App) paintListRow(f *flex.Frame, top float32, item listItem) {
	row := f.Column().Absolute().Top(top).Left(0).Right(0).Height(item.height).PadSides(0, 30, 0, 20)
	if item.kind == itemHeader {
		row.Children(func() {
			a.paintGroupHeader(f, item.group)
		})
		return
	}
	row.Children(func() {
		a.paintNodeRow(f, item.node)
	})
}

// showResultBar 决定是否显示结果条：跑过测速、有结果或在导出时才有意义，
// 第一次打开时把纵向空间留给节点列表。
func (a *App) showResultBar() bool {
	return a.running() || len(a.rounds) > 0 || len(a.results) > 0 || a.exporting > 0
}

func (a *App) paintTestToolbar(f *flex.Frame) {
	f.Row().Height(52).FillWidth().PadX(20).Items(flex.Center).Gap(8).Children(func() {
		f.Text(a.countText).FontSize(12).Muted().Ellipsis()
		if f.Button(a.ids.selectAll, "全选").XSmall().Ghost().Disabled(a.running() || a.selectedCount == a.totalCount).Clicked() {
			a.setAllNodesSelected(true)
		}
		if f.Button(a.ids.selectNone, "全不选").XSmall().Ghost().Disabled(a.running() || a.selectedCount == 0).Clicked() {
			a.setAllNodesSelected(false)
		}
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
		if a.roundIndex >= 0 && a.roundIndex < len(a.rounds) && a.rounds[a.roundIndex].err != "" {
			if f.Button(a.ids.retryExport, "重试导出").XSmall().Outline().Destructive().Disabled(a.exporting > 0).Clicked() {
				f.Canvas().AfterPaint(a.retryExportRound)
			}
		}
		if a.lastImage != "" {
			if f.Button(a.ids.previewImage, "预览结果图").XSmall().Outline().Disabled(a.exporting > 0).Clicked() {
				f.Canvas().AfterPaint(func() { a.startImagePreview(a.lastImage) })
			}
		}
		if len(a.results) > 0 {
			if f.Button(a.ids.clearResults, "清空结果").XSmall().Ghost().Destructive().Disabled(a.running() || a.exporting > 0).Clicked() {
				f.Canvas().AfterPaint(func() { a.askClearResults() })
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
	f.Row().Height(30).FillWidth().PadSides(0, 30, 0, 20).Items(flex.Center).Children(func() {
		f.Row().FillWidth().PadX(8).Gap(8).Items(flex.Center).Children(func() {
			f.Spacer().Width(colCheck).Grow(0)
			f.Text("节点").FontSize(11).Muted().Grow(1).Basis(0)
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
	// 用户明确点了表头：解除手动单测的固定，按新列重排。
	s.pinNode, s.pinIndex = nil, 0
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
		Hit(node.ids.row, node.rowLabel)
	// 手动单测的节点用主题色浅浅标出，并在结果到达后保持原位（不参与重排）。
	pinned := node == a.pinNode
	active := node == a.activeNode || node.marked
	// 悬停只在当前底色上加深一点：不要用固定的灰底覆盖选中态，否则鼠标
	// 一指过去，蓝色选中效果就像消失了一样。
	hover := a.theme.Muted
	switch {
	case pinned:
		tint := a.theme.Primary
		tint.Alpha *= 0.20
		row = row.Background(tint)
		hover = a.theme.Primary
		hover.Alpha *= 0.28
	case active:
		// 单击或右键的那一行：主题色淡底 + 主题色节点名，位置一眼可见。
		tint := a.theme.Primary
		tint.Alpha *= 0.14
		row = row.Background(tint)
		hover = a.theme.Primary
		hover.Alpha *= 0.22
	case node.selected || node.phase == phaseTesting:
		row = row.Background(a.theme.Muted)
		hover = a.theme.MutedForeground
		hover.Alpha *= 0.10
	}
	row = row.HoverBackground(hover)
	canvas := f.Canvas()
	row.Children(func() {
		check := f.Checkbox(node.ids.check, node.selected).
			Label(node.checkLabel)
		if check.Clicked() {
			a.toggleNode(node)
		}
		f.Column().Gap(0).Grow(1).Basis(0).Children(func() {
			nameColor := a.theme.Foreground
			if pinned || active {
				nameColor = a.theme.Primary
			}
			f.Text(node.name).Color(nameColor).Ellipsis()
			f.Row().Gap(4).Items(flex.Center).Children(func() {
				f.Text(node.serverText).FontSize(10).Muted().Ellipsis().MaxWidth(180)
				if node.typ != "" {
					a.paintPill(f, node.typ, 9)
				}
				f.Spacer()
			})
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
		// 行尾竖向三点：打开详情的唯一入口。在 pointer 事件 pass 里处理，
		// 才能拿到图标位置作为弹窗锚点；Consume 后不再走 click 事件 pass。
		knobColor := a.theme.MutedForeground
		if pinned || active {
			knobColor = a.theme.Primary
		}
		f.Text("⋮").FontSize(16).Color(knobColor).Width(20).Height(24).
			TextCenter().Radius(4).HoverBackground(a.theme.Muted).
			Hit(node.ids.knob, node.knobLabel)
		if event, ok := canvas.PointerOn(node.ids.knob); ok &&
			event.Kind == ui.PointerUp && event.Button == ui.PointerButtonPrimary && event.ClickCount == 1 {
			// pointer 事件 pass 结束后不运行 AfterPaint 队列，这里直接改
			// 状态，Consume 会触发重绘把弹窗画出来。
			a.toggleDetailAt(node.key, event.X, event.Y)
			canvas.Consume()
		}
	})
	// 行点击在 pointer 事件 pass 里处理（click pass 拿不到修饰键）：
	// 普通与 Ctrl 点击切换该行，Shift 点击选中范围；双击的第二下忽略。
	if event, ok := canvas.PointerOn(node.ids.row); ok &&
		event.Kind == ui.PointerUp && event.Button == ui.PointerButtonPrimary && event.ClickCount == 1 {
		a.handleRowClick(node, event.Modifiers)
		canvas.Consume()
	}
	a.handleNodePointer(f.Canvas(), node.ids.row, node)
	a.handleNodePointer(f.Canvas(), node.ids.check, node)
	// 详情弹窗的锚点跟随目标行：行在本帧绘制过就刷新「还看得见」标记。
	if a.detailNode == node {
		a.detailSeen = true
	}
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

// toggleDetailAt 打开/关闭某个节点的详情弹窗。弹窗从行尾三点图标的位置
// 展开（anchor 是图标点击位置），不再使用底部面板；点同一个图标关闭。
func (a *App) toggleDetailAt(key string, x, y float32) {
	if a.detailKey == key {
		a.closeDetail()
		return
	}
	node := a.byKey[key]
	if node == nil {
		return
	}
	a.detailKey = key
	a.detailNode = node
	a.activeNode = node
	a.detailText = detailText(node)
	// 锚点取图标点击位置；弹窗右缘对齐它向左展开，放不下时 Below 会
	// 自动改到上方。
	a.detailAnchor = ui.Rect{X: x - 12, Y: y - 12, Width: 24, Height: 24}
}

// closeDetail 关闭详情弹窗并清掉行选中态。
func (a *App) closeDetail() {
	a.detailKey = ""
	a.detailNode = nil
	a.activeNode = nil
	a.detailText = ""
}

// detailPopupWidth 是详情弹窗宽度；高度按内容行数在绘制时计算。
const detailPopupWidth float32 = 420

// paintDetailPopup 在三点图标旁绘制详情弹窗。全窗口先声明一个 dismiss 命
// 中，目标行滚出视口后自动关闭；弹窗本体由 paintDetailOverlay 在 paint 阶
// 段直接画（见下），这里只处理 flex 阶段能做的 dismiss 命中。
func (a *App) paintDetailPopup(f *flex.Frame) {
	if a.detailNode == nil {
		return
	}
	if !a.detailSeen {
		// 目标行不在本帧视口内（滚动走了、条目折叠了）：关闭详情。
		a.closeDetail()
		return
	}
	canvas := f.Canvas()
	canvas.HitWithLabel(a.ids.detailDismiss, ui.Rect{Width: a.window.Width, Height: a.window.Height}, "关闭节点详情")
	if canvas.Clicked(a.ids.detailDismiss) {
		a.closeDetail()
	}
}

// paintDetailOverlay 用 canvas 直接绘制详情弹窗。必须在 view.Draw 之后、
// PushLayer(1) 内画：渲染器按 layer 合成、同层先全部矩形再全部文字，弹窗
// 不推高层的话列表行文字会画在弹窗背景之上。弹窗内的命中（面板本身）也
// 在同一层声明，优先于 layer 0 的 dismiss。
func (a *App) paintDetailOverlay(canvas *ui.Canvas) {
	if a.detailNode == nil || !a.detailSeen {
		return
	}
	width := detailPopupWidth
	contentWidth := width - 24
	lines := canvas.WrapLines(nil, a.detailText, 11, ui.WeightRegular, contentWidth, 14)
	lineHeight := float32(18)
	titleHeight := float32(24)
	height := 24 + titleHeight + float32(len(lines))*lineHeight
	// 右缘对齐图标向左展开；下方放不下改到上方，整体钳制在窗口内。
	x := a.detailAnchor.X + a.detailAnchor.Width - width
	y := a.detailAnchor.Y + a.detailAnchor.Height + 6
	if y+height > a.window.Height-8 {
		y = a.detailAnchor.Y - 6 - height
	}
	x = min(max(x, 8), max(a.window.Width-width-8, 8))
	y = max(min(y, max(a.window.Height-height-8, 8)), 8)
	rect := ui.Rect{X: x, Y: y, Width: width, Height: height}
	canvas.PushLayer(1)
	// 弹窗面板的命中在 layer 1，盖过 layer 0 的 dismiss：点弹窗内部不关闭。
	canvas.HitWithLabel(a.ids.detailClose, rect, "节点详情")
	canvas.Elevate(rect, a.theme.Metrics.RadiusSurface, ui.ShadowMD)
	canvas.RoundedBorder(rect, a.theme.Metrics.RadiusSurface, 1, a.theme.Border, a.theme.Background)
	canvas.Label(a.detailNode.name, ui.Rect{
		X: rect.X + 12, Y: rect.Y + 12, Width: contentWidth, Height: titleHeight,
	}, ui.TextStyle{Size: 13, Weight: ui.WeightMedium, Color: a.theme.Foreground})
	textY := rect.Y + 12 + titleHeight
	for _, line := range lines {
		canvas.Label(line, ui.Rect{
			X: rect.X + 12, Y: textY, Width: contentWidth, Height: lineHeight,
		}, ui.TextStyle{Size: 11, Color: a.theme.MutedForeground})
		textY += lineHeight
	}
	canvas.PopLayer()
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
			f.Text("节点来源").Bold().MinWidth(78)
			f.Spacer()
			if f.Button(a.ids.entryCollapse, "折叠").XSmall().Ghost().Clicked() {
				a.entriesCollapsed = true
			}
			if f.Button(a.ids.newEntry, "新建条目").Small().Outline().Disabled(a.running()).Clicked() {
				a.openEntryDialog(nil)
			}
		})
		if a.hasSubscription {
			f.Row().Height(28).PadSides(0, 16, 0, 12).Items(flex.Center).Gap(8).Children(func() {
				f.Text("订阅").FontSize(11).Muted()
				if f.Button(a.ids.refetchAll, "全部重新获取").XSmall().Outline().Disabled(a.running()).Clicked() {
					f.Canvas().AfterPaint(func() { a.refetchAllSubscriptions() })
				}
			})
		}
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

// paintEntriesRail 是折叠后的窄栏：顶部一个「»」展开按钮，下面是竖排的
// 「节点来源」四个字。竖排用逐字 Text 拼出来，不做文字旋转。
func (a *App) paintEntriesRail(f *flex.Frame) {
	f.Column().Width(entriesRailWidth).FillHeight().PadY(12).Items(flex.Center).Gap(12).Children(func() {
		expand := f.Text("»").FontSize(18).Muted().Width(32).PadY(2).
			TextCenter().Radius(4).HoverBackground(a.theme.Muted).
			Hit(a.ids.entryExpand, "展开节点来源")
		if expand.Clicked() {
			a.entriesCollapsed = false
		}
		f.Column().Items(flex.Center).Gap(2).Children(func() {
			f.Text("节").FontSize(11).Muted()
			f.Text("点").FontSize(11).Muted()
			f.Text("来").FontSize(11).Muted()
			f.Text("源").FontSize(11).Muted()
		})
	})
}

// paintPill 画小徽标（路径形态、协议类型）：固定高度、文字居中、底色与
// 正文有明显区分；字号比正文小一号。纯展示，不注册命中。
//
// 中文字符在 Viva 的参考字宽（布局检查用）里按约 1.35em 估，而平台排版引擎
// 按 1em；自动宽度的中文徽标会被判定为截断。取两者较大值再加内边距，两种
// 度量都放得下，左右留白也对称。
func (a *App) paintPill(f *flex.Frame, label string, size float32) {
	width := f.Canvas().TextWidth(label, size) + 12
	if runes := utf8.RuneCountInString(label); runes != len(label) {
		if minimum := float32(runes)*size*1.45 + 12; minimum > width {
			width = minimum
		}
	}
	background := a.theme.MutedForeground
	background.Alpha *= 0.16
	foreground := a.theme.Foreground
	foreground.Alpha *= 0.85
	pill := f.Row().Width(width).Height(size + 10).Shrink(0).Background(background).Radius(4).Items(flex.Center)
	pill.Children(func() {
		f.Text(label).FontSize(size).Color(foreground).TextCenter().Grow(1)
	})
}

func (a *App) paintEntryCard(f *flex.Frame, entry *entryState) {
	// 整张卡片可点（点空白处切换勾选），视觉保持默认白底：选中与否由
	// 复选框表达，不再给整卡上主题色。
	card := f.Column().FillWidth().Pad(10).Gap(5).
		Background(a.theme.Card).Border(1, a.theme.Border).
		Radius(a.theme.Metrics.RadiusSurface).
		HoverBackground(a.theme.Muted).
		Hit(entry.ids.card, entry.cardLabel)
	if card.Clicked() && !a.running() && !entry.fetching {
		f.Canvas().AfterPaint(func() { a.toggleEntry(entry) })
	}
	card.Children(func() {
		f.Row().Items(flex.Center).Gap(8).Children(func() {
			check := f.Checkbox(entry.ids.check, entry.selected).
				Label(entry.checkLabel).
				Disabled(a.running())
			if check.Clicked() {
				f.Canvas().AfterPaint(func() { a.toggleEntry(entry) })
			}
			f.Text(entry.entry.Name).Bold().Ellipsis().Grow(1)
		})
		// 路径只显示一次；软/硬路径与订阅用徽标区分，不再「软路径 · xxx」+ xxx 两行。
		f.Row().Items(flex.Center).Gap(6).Children(func() {
			if entry.pathBadge != "" {
				a.paintPill(f, entry.pathBadge, 10)
			}
			f.Text(entry.pathText).FontSize(11).Muted().Ellipsis().Grow(1)
		})
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
		})
		f.Row().Gap(6).Children(func() {
			if f.Button(entry.ids.edit, "编辑").XSmall().Ghost().Disabled(a.running()).Clicked() {
				a.openEntryDialog(entry)
			}
			if f.Button(entry.ids.openDir, "打开目录").XSmall().Ghost().Clicked() {
				f.Canvas().AfterPaint(func() { a.openEntryDirectory(entry) })
			}
			if f.Button(entry.ids.remove, "删除").XSmall().Ghost().Destructive().Disabled(a.running()).Clicked() {
				a.askDeleteEntry(entry)
			}
		})
	})
}

func (a *App) paintGroupHeader(f *flex.Frame, group *groupState) {
	f.Row().Height(32).FillWidth().Items(flex.Center).Gap(8).Children(func() {
		toggle := f.Text(group.arrow).FontSize(12).Muted().Width(18).TextCenter().
			Radius(4).HoverBackground(a.theme.Muted).
			Hit(group.entry.ids.groupToggle, group.toggleLabel)
		if toggle.Clicked() {
			// 重建 nodeItems 不能发生在遍历它的绘制回合里。
			f.Canvas().AfterPaint(func() { a.toggleGroup(group) })
		}
		f.Text(group.entry.entry.Name).Bold()
		f.Text(group.countText).FontSize(11).Muted()
		if group.entry.loadErr != "" {
			f.Text("读取失败").FontSize(10).Color(a.theme.Danger).Ellipsis()
		}
		if group.collapsed {
			f.Text("已折叠").FontSize(11).Muted()
		}
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
					if f.Button(entry.ids.emptyRead, "读取").Small().Outline().Clicked() {
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
