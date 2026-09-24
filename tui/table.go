package tui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
	"github.com/faceair/clash-speedtest/output"
	"github.com/faceair/clash-speedtest/speedtester"
)

// updateTableRows updates the table rows with current results
func (m *tuiModel) updateTableRows() {
	start := time.Now()
	defer m.perf.record(perfEventRows, len(m.results), start)
	inFlightCount := m.inFlightCount()
	rows := make([]table.Row, 0, inFlightCount+len(m.results))
	// 已完成行在上，在测行排在下面。
	columns := len(m.table.Columns())
	for i, result := range m.results {
		rows = append(rows, fitRowWidth(output.FormatRow(result, m.mode, i), columns))
	}
	for _, name := range m.inFlightOrder {
		rows = append(rows, fitRowWidth(m.inFlightRow(m.inFlight[name]), columns))
	}
	m.table.SetRows(rows)
	m.syncSelection()
}

// inFlightRow 渲染一个在测行：延迟阶段显示「测试中」，下载/上传进行中显示阶段瞬时速度，
// 已结束的阶段列定格。单元格保持纯文本，选中样式交给表格本身。
func (m *tuiModel) inFlightRow(node *inFlightNode) table.Row {
	p := node.latest
	idStr := "…"
	name := node.name
	pxyType := node.proxyType

	latencyStr := "测试中"
	if p.Latency > 0 {
		latencyStr = fmt.Sprintf("%dms", p.Latency.Milliseconds())
	}
	if m.mode.IsFast() {
		return table.Row{idStr, name, pxyType, latencyStr}
	}

	jitterStr := ""
	lossStr := ""
	downloadStr := ""
	uploadStr := ""
	if p.Latency > 0 {
		jitterStr = fmt.Sprintf("%dms", p.Jitter.Milliseconds())
		lossStr = fmt.Sprintf("%.1f%%", p.PacketLoss)
	}
	if p.Phase >= speedtester.PhaseDownload {
		downloadStr = speedtester.FormatSpeed(p.DownloadSpeed)
	}
	if p.Phase >= speedtester.PhaseUpload {
		uploadStr = speedtester.FormatSpeed(p.UploadSpeed)
	}
	row := table.Row{idStr, name, pxyType, latencyStr, jitterStr, lossStr, downloadStr}
	if m.mode.UploadEnabled() {
		row = append(row, uploadStr)
	}
	return row
}

// fitRowWidth 保证行单元格数不超过表头列数，避免 bubbles table 按下标渲染时越界。
func fitRowWidth(row table.Row, columns int) table.Row {
	if columns <= 0 || len(row) == columns {
		return row
	}
	fitted := make(table.Row, columns)
	for i := 0; i < columns && i < len(row); i++ {
		fitted[i] = row[i]
	}
	return fitted
}

func (m *tuiModel) updateTableHeaders() {
	if len(m.baseHeaders) == 0 {
		return
	}
	columns := buildColumns(addSortIndicators(m.baseHeaders, m.sortColumn, m.sortAscending), m.windowWidth, m.mode)
	m.table.SetColumns(columns)
}

func buildColumns(headers []string, width int, mode speedtester.SpeedMode) []table.Column {
	columns := make([]table.Column, len(headers))
	widths := calculateColumnWidths(width, mode)
	for i, h := range headers {
		columnWidth := 10
		if i < len(widths) {
			columnWidth = widths[i]
		}
		columns[i] = table.Column{Title: h, Width: columnWidth}
	}
	return columns
}

func calculateColumnWidths(width int, mode speedtester.SpeedMode) []int {
	columnPadding := 2
	columnCount := 7
	if mode.IsFast() {
		columnCount = 4
	} else if mode.UploadEnabled() {
		columnCount = 8
	}
	windowWidth := width
	availableWidth := width
	if width > 0 {
		availableWidth = max(width-columnCount*columnPadding, 0)
	}

	if mode.IsFast() {
		indexWidth := 6
		typeWidth := 12
		latencyWidth := 10
		if windowWidth <= 0 {
			return []int{indexWidth, 30, typeWidth, latencyWidth}
		}
		minIndexWidth := 4
		minNameWidth := 4
		minTypeWidth := 6
		minLatencyWidth := 6
		fixedWidth := indexWidth + typeWidth + latencyWidth
		nameWidth := max(minNameWidth, availableWidth-fixedWidth)
		widths := []int{indexWidth, nameWidth, typeWidth, latencyWidth}
		minWidths := []int{minIndexWidth, minNameWidth, minTypeWidth, minLatencyWidth}
		shrinkOrder := []int{1, 3, 2, 0}
		return shrinkWidthsToFit(windowWidth, columnPadding, widths, minWidths, shrinkOrder)
	}

	indexWidth := 6
	typeWidth := 12
	latencyWidth := 10
	jitterWidth := 10
	lossWidth := 10
	downloadWidth := 16
	uploadWidth := 16
	if windowWidth <= 0 {
		if mode.UploadEnabled() {
			return []int{indexWidth, 30, typeWidth, latencyWidth, jitterWidth, lossWidth, downloadWidth, uploadWidth}
		}
		return []int{indexWidth, 30, typeWidth, latencyWidth, jitterWidth, lossWidth, downloadWidth}
	}
	minIndexWidth := 4
	minNameWidth := 4
	minTypeWidth := 6
	minLatencyWidth := 6
	minJitterWidth := 6
	minLossWidth := 6
	minDownloadWidth := 6
	minUploadWidth := 6
	if mode.UploadEnabled() {
		fixedWidth := indexWidth + typeWidth + latencyWidth + jitterWidth + lossWidth + downloadWidth + uploadWidth
		nameWidth := max(minNameWidth, availableWidth-fixedWidth)
		widths := []int{indexWidth, nameWidth, typeWidth, latencyWidth, jitterWidth, lossWidth, downloadWidth, uploadWidth}
		minWidths := []int{minIndexWidth, minNameWidth, minTypeWidth, minLatencyWidth, minJitterWidth, minLossWidth, minDownloadWidth, minUploadWidth}
		shrinkOrder := []int{1, 6, 7, 4, 5, 3, 2, 0}
		return shrinkWidthsToFit(windowWidth, columnPadding, widths, minWidths, shrinkOrder)
	}
	fixedWidth := indexWidth + typeWidth + latencyWidth + jitterWidth + lossWidth + downloadWidth
	nameWidth := max(minNameWidth, availableWidth-fixedWidth)
	widths := []int{indexWidth, nameWidth, typeWidth, latencyWidth, jitterWidth, lossWidth, downloadWidth}
	minWidths := []int{minIndexWidth, minNameWidth, minTypeWidth, minLatencyWidth, minJitterWidth, minLossWidth, minDownloadWidth}
	shrinkOrder := []int{1, 6, 4, 5, 3, 2, 0}
	return shrinkWidthsToFit(windowWidth, columnPadding, widths, minWidths, shrinkOrder)
}

func shrinkWidthsToFit(windowWidth int, columnPadding int, widths []int, minWidths []int, shrinkOrder []int) []int {
	if windowWidth <= 0 {
		return widths
	}
	padding := columnPadding * len(widths)
	maxTotal := max(windowWidth-padding, 0)
	total := 0
	for _, value := range widths {
		total += value
	}
	overflow := total - maxTotal
	for overflow > 0 {
		shrunk := false
		for _, idx := range shrinkOrder {
			if idx < 0 || idx >= len(widths) || idx >= len(minWidths) {
				continue
			}
			if widths[idx] > minWidths[idx] {
				widths[idx]--
				overflow--
				shrunk = true
				if overflow == 0 {
					break
				}
			}
		}
		if !shrunk {
			break
		}
	}
	return widths
}

func addSortIndicators(headers []string, sortColumn int, sortAscending bool) []string {
	withIndicators := make([]string, len(headers))
	for i, header := range headers {
		withIndicators[i] = header + " ⇅"
	}
	if sortColumn >= 0 && sortColumn < len(withIndicators) {
		direction := "↓"
		if sortAscending {
			direction = "↑"
		}
		withIndicators[sortColumn] = headers[sortColumn] + " " + direction
	}
	return withIndicators
}

func (m tuiModel) columnAtX(x int) int {
	if x < 0 {
		return -1
	}
	columns := m.table.Columns()
	currentX := 0
	for i, col := range columns {
		width := col.Width + tableHeaderPadding*2
		if x >= currentX && x < currentX+width {
			return i
		}
		currentX += width
	}
	return -1
}

func (m tuiModel) rowAtY(y int) (int, bool) {
	startY := m.tableHeaderY() + tableHeaderLines
	if y < startY {
		return 0, false
	}
	rowIndex := y - startY
	if rowIndex < 0 || rowIndex >= m.table.Height() {
		return 0, false
	}
	start := tableStartIndex(m.table.Cursor(), m.table.Height())
	absoluteIndex := start + rowIndex
	if absoluteIndex < 0 || absoluteIndex >= m.tableRowCount() {
		return 0, false
	}
	// 完成行在上。超出完成区的是在测行，用负偏移标记。
	if absoluteIndex >= len(m.results) {
		return len(m.results) - absoluteIndex - 1, true
	}
	return absoluteIndex, true
}

// tableRowCount 返回表格总行数：在测行 + 完成行。
func (m tuiModel) tableRowCount() int {
	return m.inFlightCount() + len(m.results)
}

func (m tuiModel) tableHeaderY() int {
	return lipgloss.Height(lipgloss.JoinVertical(
		lipgloss.Left,
		m.progressLine(),
		"",
	))
}

func (m tuiModel) isHeaderClick(y int) bool {
	startY := m.tableHeaderY()
	endY := startY + tableHeaderLines
	return y >= startY && y < endY
}

func (m *tuiModel) setSelection(index int) {
	if index < 0 || index >= len(m.results) {
		m.selectedIndex = -1
		return
	}
	if m.detailResult == nil || m.detailResult != m.results[index] {
		m.detailResult = m.results[index]
	}
	m.selectedIndex = index
	// 完成行排在在测行前面。
	m.table.SetCursor(index)
	m.table.Focus()
}

func (m *tuiModel) syncSelection() {
	if m.detailVisible {
		// 详情打开时保留用户滚动到的行，避免刷新把光标拉回详情行。
		return
	}
	if m.selectedIndex < 0 {
		return
	}
	if m.detailResult != nil {
		for i, result := range m.results {
			if result == m.detailResult {
				m.selectedIndex = i
				break
			}
		}
	}
	if m.selectedIndex < 0 || m.selectedIndex >= len(m.results) {
		m.selectedIndex = -1
		m.table.Blur()
		return
	}
	m.table.SetCursor(m.selectedIndex)
	m.table.Focus()
}

func (m *tuiModel) syncSelectionFromCursor() {
	cursor := m.table.Cursor()
	if cursor < 0 || cursor >= m.tableRowCount() {
		return
	}
	// 不在这里重绘。测试过程中每次按键都 SetRows 会重算视口，方向键和滚轮看起来像失灵。
	if cursor >= len(m.results) {
		// 点击/滚动到表尾在测行：仅高亮，不更新完成区选中索引。
		m.highlightInFlight(m.inFlightOrder[cursor-len(m.results)])
		return
	}
	resultIndex := cursor
	m.detailInFlight = nil
	m.selectedIndex = resultIndex
	if m.detailVisible {
		if m.detailResult != m.results[resultIndex] {
			previousHeight := m.detailHeight
			m.detailResult = m.results[resultIndex]
			m.refreshDetailHeight()
			if m.detailHeight != previousHeight {
				// Keep layout in sync when detail content height changes on scroll.
				m.updateTableLayout()
			}
		}
	}
	m.table.Focus()
}

// highlightInFlight 把在测行映射为临时选中索引；详情面板仍显示该节点已有的进度。
// 结果上表后 finishInFlight 会把占位详情切换为完成详情。
func (m *tuiModel) highlightInFlight(name string) {
	node, ok := m.inFlight[name]
	if !ok {
		return
	}
	// 负值索引表示选中在测行，避免与完成区索引冲突。
	m.selectedIndex = -(m.inFlightCount() + 1)
	m.table.Focus()
	if m.detailVisible {
		previousHeight := m.detailHeight
		m.detailInFlight = node
		m.detailResult = nil
		m.refreshDetailHeight()
		if m.detailHeight != previousHeight {
			m.updateTableLayout()
		}
	}
}

func tableStartIndex(cursor int, height int) int {
	if cursor < 0 {
		return 0
	}
	return clampInt(cursor-height, 0, cursor)
}

func clampInt(value int, low int, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

// colorizeRow applies color thresholds to a row
func (m *tuiModel) colorizeRow(row []string, result *speedtester.Result) table.Row {
	// Color thresholds matching ANSI colors in main.go
	// Latency: <800ms green, <1500ms yellow, >=1500ms red
	latencyStr := row[3]
	if result.Latency > 0 {
		if result.Latency < 800*time.Millisecond {
			latencyStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Render(latencyStr) // green
		} else if result.Latency < 1500*time.Millisecond {
			latencyStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFF00")).Render(latencyStr) // yellow
		} else {
			latencyStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Render(latencyStr) // red
		}
	} else {
		latencyStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Render(latencyStr) // red
	}

	if m.mode.IsFast() {
		return table.Row{row[0], row[1], row[2], latencyStr}
	}

	// Jitter: <800ms green, <1500ms yellow, >=1500ms red
	jitterStr := row[4]
	if result.Jitter > 0 {
		if result.Jitter < 800*time.Millisecond {
			jitterStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Render(jitterStr) // green
		} else if result.Jitter < 1500*time.Millisecond {
			jitterStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFF00")).Render(jitterStr) // yellow
		} else {
			jitterStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Render(jitterStr) // red
		}
	} else {
		jitterStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Render(jitterStr) // red
	}

	// Packet loss: <10% green, <20% yellow, >=20% red
	packetLossStr := row[5]
	if result.PacketLoss < 10 {
		packetLossStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Render(packetLossStr) // green
	} else if result.PacketLoss < 20 {
		packetLossStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFF00")).Render(packetLossStr) // yellow
	} else {
		packetLossStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Render(packetLossStr) // red
	}

	// Download speed: >=10MB/s green, >=5MB/s yellow, <5MB/s red
	downloadSpeed := result.DownloadSpeed / (1024 * 1024)
	downloadSpeedStr := row[6]
	if downloadSpeed >= 10 {
		downloadSpeedStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Render(downloadSpeedStr) // green
	} else if downloadSpeed >= 5 {
		downloadSpeedStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFF00")).Render(downloadSpeedStr) // yellow
	} else {
		downloadSpeedStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Render(downloadSpeedStr) // red
	}

	if !m.mode.UploadEnabled() {
		return table.Row{
			row[0],
			row[1],
			row[2],
			latencyStr,
			jitterStr,
			packetLossStr,
			downloadSpeedStr,
		}
	}

	// Upload speed: >=5MB/s green, >=2MB/s yellow, <2MB/s red
	uploadSpeed := result.UploadSpeed / (1024 * 1024)
	uploadSpeedStr := row[7]
	if uploadSpeed >= 5 {
		uploadSpeedStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Render(uploadSpeedStr) // green
	} else if uploadSpeed >= 2 {
		uploadSpeedStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFF00")).Render(uploadSpeedStr) // yellow
	} else {
		uploadSpeedStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Render(uploadSpeedStr) // red
	}

	return table.Row{
		row[0],
		row[1],
		row[2],
		latencyStr,
		jitterStr,
		packetLossStr,
		downloadSpeedStr,
		uploadSpeedStr,
	}
}
