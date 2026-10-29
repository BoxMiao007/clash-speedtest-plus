package gui

import (
	"github.com/vivaui/viva/flex"
)

const (
	colStatus  float32 = 64
	colEntry   float32 = 120
	colType    float32 = 84
	colLatency float32 = 62
	colLoss    float32 = 62
	colSpeed   float32 = 100
	colTested  float32 = 124
)

func (a *App) paintResults(f *flex.Frame) {
	f.Row().Height(52).PadX(20).Items(flex.Center).Gap(8).Children(func() {
		f.Text(a.resultText).FontSize(12).Muted()
		f.Spacer()
		if a.running() {
			if f.Button(a.ids.stopTest, "停止测速").Small().Destructive().Clicked() {
				a.stopTest()
			}
		}
		if f.Button(a.ids.backToSelect, "返回选择").Small().Outline().Clicked() {
			a.active = viewNodes
		}
	})
	f.Row().Height(30).FillWidth().PadX(20).Items(flex.Center).Gap(8).Children(func() {
		a.resultHeaderCell(f, "状态", colStatus, false)
		f.Text("节点").FontSize(11).Muted().Grow(1)
		a.resultHeaderCell(f, "条目", colEntry, false)
		a.resultHeaderCell(f, "类型", colType, false)
		a.resultHeaderCell(f, "延迟", colLatency, true)
		a.resultHeaderCell(f, "丢包", colLoss, true)
		a.resultHeaderCell(f, "下载速度", colSpeed, true)
		a.resultHeaderCell(f, "上次测速", colTested, true)
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
}

func (a *App) resultHeaderCell(f *flex.Frame, label string, width float32, end bool) {
	cell := f.Text(label).FontSize(11).Muted().Width(width)
	if end {
		cell.TextEnd()
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
		f.Text(node.lossText).FontSize(11).Muted().Width(colLoss).TextEnd()
		speedColor := a.theme.Foreground
		if node.hasResult && !node.result.OK {
			speedColor = a.theme.Danger
		}
		f.Text(node.speedText).FontSize(11).Color(speedColor).Width(colSpeed).TextEnd()
		f.Text(node.lastTestText).FontSize(11).Muted().Width(colTested).Ellipsis()
	})
	a.handleNodePointer(f.Canvas(), node.ids.row, node)
}
