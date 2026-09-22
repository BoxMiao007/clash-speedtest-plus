package tui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m *tuiModel) updateTableLayout() {
	if m.windowWidth == 0 || m.windowHeight == 0 {
		return
	}
	start := time.Now()
	defer m.perf.record(perfEventLayout, len(m.results), start)
	columns := buildColumns(addSortIndicators(m.baseHeaders, m.sortColumn, m.sortAscending), m.windowWidth, m.mode)
	m.table.SetColumns(columns)
	m.table.SetWidth(m.windowWidth)
	m.help.setWidth(m.windowWidth)
	reserved := 2
	if m.detailVisible && m.detailResult != nil {
		detailHeight := m.detailPanelHeight()
		if detailHeight > 0 {
			reserved += detailHeight + 1
		}
	}
	helpHeight := m.help.height()
	if helpHeight > 0 {
		reserved += helpHeight + 1
	}
	tableHeight := max(6, m.windowHeight-reserved)
	m.table.SetHeight(tableHeight)
}

func (m tuiModel) progressLine() string {
	// 暂停时已用时与剩余冻结：总耗时扣除历史及当前暂停时长。
	elapsed := m.testingElapsed()
	state := m.stateLabel()
	info := state + " " + fmt.Sprintf("%d/%d", m.currentProxy, m.totalProxies)
	if inFlight := m.inFlightCount(); inFlight > 0 {
		info += fmt.Sprintf("，%d 在测", inFlight)
	}
	metrics := fmt.Sprintf("已用时 %s", formatDuration(elapsed))
	// 只在还打算继续派发新节点时显示剩余；暂停时随已用时一起冻结。
	if m.testing && !m.earlyStopped {
		eta := formatETA(elapsed, m.currentProxy, m.totalProxies)
		metrics += " | 剩余 " + eta
	}
	barWidth := 40
	if m.windowWidth > 0 {
		available := min(max(m.windowWidth-lipgloss.Width(info)-lipgloss.Width(metrics)-lipgloss.Width(" | ")-1, 10), 40)
		barWidth = available
	}
	progressModel := m.progress
	progressModel.Width = barWidth
	bar := progressModel.View()
	return fmt.Sprintf("%s %s | %s", info, bar, metrics)
}

// testingElapsed 返回扣除暂停时段后的已测时长；暂停中冻结在当前值。
func (m tuiModel) testingElapsed() time.Duration {
	elapsed := time.Since(m.startTime) - m.pausedElapsed
	if m.paused {
		elapsed -= time.Since(m.pauseStartedAt)
	}
	if elapsed < 0 {
		elapsed = 0
	}
	return elapsed
}

// stateLabel 返回进度行状态文案；全部测完优先于暂停（暂停中拓尾收完也算已完成），
// 提前结束到达后即使拓尾未收完也保持「已提前结束」。
func (m tuiModel) stateLabel() string {
	switch {
	case m.earlyStopped:
		return "已提前结束"
	case !m.testing:
		return "已完成"
	case m.paused:
		return "已暂停"
	default:
		return "测试中"
	}
}

// inFlightCount 返回当前在测节点数。
func (m tuiModel) inFlightCount() int {
	return len(m.inFlightOrder)
}

func formatDuration(value time.Duration) string {
	if value < 0 {
		value = 0
	}
	seconds := int(value.Seconds())
	hours := seconds / 3600
	minutes := (seconds % 3600) / 60
	remainingSeconds := seconds % 60
	if hours > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, remainingSeconds)
	}
	return fmt.Sprintf("%02d:%02d", minutes, remainingSeconds)
}

func formatETA(elapsed time.Duration, current, total int) string {
	if current <= 0 || total <= 0 {
		return "N/A"
	}
	progress := float64(current) / float64(total)
	if progress <= 0 {
		return "N/A"
	}
	estimatedTotal := time.Duration(float64(elapsed) / progress)
	remaining := max(estimatedTotal-elapsed, 0)
	return formatDuration(remaining)
}

func timerTickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return timerTickMsg{}
	})
}
