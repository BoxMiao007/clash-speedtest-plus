package gui

import (
	"github.com/vivaui/viva/flex"
	"github.com/vivaui/viva/ui"
)

const (
	colStatus  float32 = 64
	colEntry   float32 = 110
	colType    float32 = 80
	colLatency float32 = 68
	colJitter  float32 = 60
	colLoss    float32 = 60
	colSpeed   float32 = 96
	colUpload  float32 = 96
	colTested  float32 = 118
)

func (a *App) paintResults(f *flex.Frame) {
	f.Row().Height(52).PadX(20).Items(flex.Center).Gap(8).Children(func() {
		f.Text(a.resultText).FontSize(12).Muted().Ellipsis().Grow(1)
		if len(a.rounds) > 1 {
			if f.Button(a.ids.roundPrev, "上一轮").XSmall().Outline().Disabled(a.roundIndex == 0).Clicked() {
				a.switchRound(-1)
			}
			if f.Button(a.ids.roundNext, "下一轮").XSmall().Outline().Disabled(a.roundIndex >= len(a.rounds)-1).Clicked() {
				a.switchRound(1)
			}
		}
		if a.running() {
			if f.Button(a.ids.pauseTest, a.pauseLabel).Small().Outline().Clicked() {
				f.Canvas().AfterPaint(func() { a.togglePause() })
			}
			if f.Button(a.ids.stopTest, "停止测速").Small().Destructive().Clicked() {
				a.stopTest()
			}
		}
		if f.Button(a.ids.exportYAML, "导出配置").Small().Outline().Disabled(a.exporting).Clicked() {
			f.Canvas().AfterPaint(func() { a.startManualExport(manualYAML) })
		}
		if f.Button(a.ids.exportImage, "结果图").Small().Outline().Disabled(a.exporting).Clicked() {
			f.Canvas().AfterPaint(func() { a.startManualExport(manualImage) })
		}
		if f.Button(a.ids.exportTSV, "结果表").Small().Outline().Disabled(a.exporting).Clicked() {
			f.Canvas().AfterPaint(func() { a.startManualExport(manualTSV) })
		}
		if f.Button(a.ids.backToSelect, "返回选择").Small().Outline().Clicked() {
			a.active = viewNodes
		}
	})
	f.Row().Height(30).FillWidth().PadX(20).Items(flex.Center).Gap(8).Children(func() {
		a.resultHeaderCell(f, "状态", colStatus)
		f.Text("节点").FontSize(11).Muted().Grow(1)
		a.resultHeaderCell(f, "条目", colEntry)
		a.resultHeaderCell(f, "类型", colType)
		a.sortHeaderCell(f, a.headerLatency, a.ids.sortLatency, sortByLatency, colLatency)
		a.sortHeaderCell(f, a.headerJitter, a.ids.sortJitter, sortByJitter, colJitter)
		a.sortHeaderCell(f, a.headerLoss, a.ids.sortLoss, sortByLoss, colLoss)
		a.sortHeaderCell(f, a.headerSpeed, a.ids.sortSpeed, sortBySpeed, colSpeed)
		if a.showUpload {
			a.sortHeaderCell(f, a.headerUpload, a.ids.sortUpload, sortByUpload, colUpload)
		}
		a.sortHeaderCell(f, a.headerTested, a.ids.sortTested, sortByTested, colTested)
	})
	f.Column().Height(1).FillWidth().PadX(20).Children(func() {
		f.Column().FillWidth().Height(1).Background(a.theme.Border)
	})
	if len(a.resultItems) == 0 {
		f.Column().Grow(1).FillWidth().PadY(32).Items(flex.Center).Gap(6).Children(func() {
			f.Text("还没有测速结果。").FontSize(13).Muted()
			f.Text("回到「节点选择」，勾选节点后点「下载速度」开始。").FontSize(12).Muted()
		})
		return
	}
	viewport := a.resultViewport.Height
	if viewport <= 0 {
		viewport = a.window.Height - 226
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
	a.resultListBox = list
	a.resultListValid = true
	list.Children(func() {
		for _, item := range a.resultItems {
			top := item.y - a.resultOffset
			if top+item.height < 0 || top > viewport {
				continue
			}
			f.Column().Absolute().Top(top).Left(0).Right(0).Height(item.height).PadX(20).Children(func() {
				a.paintResultRow(f, item.node)
			})
		}
	})
	if a.detailNode != nil {
		a.paintDetailPanel(f)
	}
}

func (a *App) resultHeaderCell(f *flex.Frame, label string, width float32) {
	f.Text(label).FontSize(11).Muted().Width(width).TextEnd()
}

func (a *App) sortHeaderCell(f *flex.Frame, label string, id ui.ID, key sortKey, width float32) {
	cell := f.Text(label).FontSize(11).Muted().Width(width).TextEnd().Hit(id, "按"+label+"排序")
	if cell.Clicked() {
		a.setSort(key)
	}
}

// setSort 切换排序列：点同一列反转方向，换列用该列的默认方向。
func (s *state) setSort(key sortKey) {
	if s.sortKey == key {
		s.sortAsc = !s.sortAsc
	} else {
		s.sortKey = key
		s.sortAsc = key == sortByLatency || key == sortByName
	}
	s.refreshSortHeaders()
	s.rebuildResults()
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
}

// refreshModeFlags 按当前模式决定结果表要不要上传列。
func (s *state) refreshModeFlags() {
	s.showUpload = s.settings.Mode == "full"
	s.pauseLabel = "暂停"
	if s.paused {
		s.pauseLabel = "继续"
	}
}

func (a *App) paintResultRow(f *flex.Frame, node *nodeState) {
	row := f.Row().Height(36).FillWidth().PadX(8).Gap(8).Items(flex.Center).
		Radius(a.theme.Metrics.RadiusItem()).
		HoverBackground(a.theme.Muted).
		Hit(node.ids.row, node.rowLabel)
	if node.phase == phaseTesting {
		row = row.Background(a.theme.Muted)
	}
	if a.detailKey == node.key {
		row = row.Background(a.theme.Muted)
	}
	row.Children(func() {
		statusColor := a.theme.MutedForeground
		switch {
		case node.phase == phaseTesting:
			statusColor = a.theme.Primary
		case node.hasResult && !node.result.OK:
			statusColor = a.theme.Danger
		}
		f.Text(node.statusText).FontSize(11).Color(statusColor).Width(colStatus).Ellipsis()
		f.Column().Gap(0).Grow(1).Children(func() {
			f.Text(node.name).Ellipsis()
			f.Text(node.serverText).FontSize(10).Muted().Ellipsis()
		})
		f.Text(node.entry.entry.Name).FontSize(11).Muted().Width(colEntry).Ellipsis()
		f.Text(node.typ).FontSize(11).Muted().Width(colType).Ellipsis()
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
	if row.Clicked() {
		a.toggleDetail(node)
	}
	a.handleNodePointer(f.Canvas(), node.ids.row, node)
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
