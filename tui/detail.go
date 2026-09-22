package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/faceair/clash-speedtest/speedtester"
)

func (m *tuiModel) toggleDetail(result *speedtester.Result) {
	if result == nil {
		return
	}
	if m.detailVisible && m.detailResult == result {
		m.detailVisible = false
		m.help.setDetailVisible(false)
		m.detailHeight = 0
		m.updateTableLayout()
		return
	}
	m.detailResult = result
	m.detailInFlight = nil
	m.detailVisible = true
	m.help.setDetailVisible(true)
	m.refreshDetailHeight()
	m.updateTableLayout()
}

// toggleInFlightDetail 打开或关闭在测节点占位详情；节点测完后 finishInFlight
// 会自动切成与完成行相同的详情。
func (m *tuiModel) toggleInFlightDetail(name string) {
	node, ok := m.inFlight[name]
	if !ok {
		return
	}
	if m.detailVisible && m.detailInFlight == node {
		m.detailVisible = false
		m.help.setDetailVisible(false)
		m.detailHeight = 0
		m.updateTableLayout()
		return
	}
	m.detailInFlight = node
	m.detailResult = nil
	m.detailVisible = true
	m.help.setDetailVisible(true)
	m.refreshDetailHeight()
	m.updateTableLayout()
}

func (m tuiModel) detailPanelView() string {
	if !m.detailVisible {
		return ""
	}
	if m.detailInFlight != nil {
		return m.inFlightDetailPanel()
	}
	if m.detailResult == nil {
		return ""
	}
	panelWidth := m.detailPanelWidth()
	contentWidth := max(10, panelWidth-2)
	content := buildDetailContent(m.detailResult, contentWidth, m.mode)
	return lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Padding(0, 1).Width(panelWidth).Render(content)
}

// inFlightDetailPanel 渲染在测节点占位详情：展示已有的延迟/瞬时速度并标明仍在测试中。
// 该节点测完后 detailInFlight 被清空，面板自动切为完成详情。
func (m tuiModel) inFlightDetailPanel() string {
	p := m.detailInFlight.latest
	panelWidth := m.detailPanelWidth()
	contentWidth := max(10, panelWidth-2)
	lines := []string{
		fmt.Sprintf("节点: %s", m.detailInFlight.name),
		fmt.Sprintf("类型: %s", m.detailInFlight.proxyType),
		"",
		fmt.Sprintf("状态: 测试中"),
	}
	if p.Latency > 0 {
		lines = append(lines,
			fmt.Sprintf("延迟: %dms", p.Latency.Milliseconds()),
			fmt.Sprintf("抖动: %dms", p.Jitter.Milliseconds()),
			fmt.Sprintf("丢包率: %.1f%%", p.PacketLoss),
		)
	}
	if p.Phase >= speedtester.PhaseDownload {
		lines = append(lines, "", fmt.Sprintf("下载: %s", speedtester.FormatSpeed(p.DownloadSpeed)))
	}
	if p.Phase >= speedtester.PhaseUpload {
		lines = append(lines, "", fmt.Sprintf("上传: %s", speedtester.FormatSpeed(p.UploadSpeed)))
	}
	lines = append(lines, "", wrapText("详情随测速进度刷新，测完后展示完整结果。", contentWidth)[0])
	return lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Padding(0, 1).Width(panelWidth).Render(strings.Join(lines, "\n"))
}

func (m tuiModel) detailPanelWidth() int {
	if m.windowWidth == 0 {
		return defaultDetailWidth
	}
	width := max(detailPanelMinWidth, m.windowWidth-8)
	maxWidth := m.windowWidth - 2
	if maxWidth < 20 {
		maxWidth = m.windowWidth
	}
	if width > maxWidth {
		width = maxWidth
	}
	if width < 20 {
		width = 20
	}
	return width
}

func (m tuiModel) detailPanelHeight() int {
	if !m.detailVisible {
		return 0
	}
	if m.detailInFlight != nil {
		if m.detailHeight > 0 {
			return m.detailHeight
		}
		height := lipgloss.Height(m.inFlightDetailPanel())
		if height > 0 {
			m.detailHeight = height
		}
		return height
	}
	if m.detailResult == nil {
		return 0
	}
	if m.detailHeight > 0 {
		return m.detailHeight
	}
	return m.calculateDetailHeight()
}

func (m *tuiModel) refreshDetailHeight() {
	m.detailHeight = m.calculateDetailHeight()
}

func (m tuiModel) calculateDetailHeight() int {
	if !m.detailVisible {
		return 0
	}
	if m.detailInFlight != nil {
		return lipgloss.Height(m.inFlightDetailPanel())
	}
	if m.detailResult == nil {
		return 0
	}
	panelWidth := m.detailPanelWidth()
	contentWidth := max(10, panelWidth-2)
	content := buildDetailContent(m.detailResult, contentWidth, m.mode)
	return lipgloss.Height(lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Padding(0, 1).Width(panelWidth).Render(content))
}

func buildDetailContent(result *speedtester.Result, width int, mode speedtester.SpeedMode) string {
	lines := []string{
		fmt.Sprintf("Node: %s", result.ProxyName),
		fmt.Sprintf("Type: %s", result.ProxyType),
		"",
		fmt.Sprintf("Latency: %s", result.FormatLatency()),
	}
	if !mode.IsFast() {
		lines = append(lines,
			fmt.Sprintf("Jitter: %s", result.FormatJitter()),
			fmt.Sprintf("Packet Loss: %s", result.FormatPacketLoss()),
			"",
			fmt.Sprintf("Download: %s", result.FormatDownloadSpeedValue()),
		)
		lines = appendWrappedValue(lines, "Download Error:", result.FormatDownloadError(), width)
		if mode.UploadEnabled() {
			lines = append(lines, "", fmt.Sprintf("Upload: %s", result.FormatUploadSpeedValue()))
			lines = appendWrappedValue(lines, "Upload Error:", result.FormatUploadError(), width)
		}
	}
	lines = append(lines, "", "Press ESC to close details.")
	return strings.Join(lines, "\n")
}

func appendWrappedValue(lines []string, label, value string, width int) []string {
	if value == "" {
		value = "N/A"
	}
	prefix := label + " "
	wrapWidth := max(width-lipgloss.Width(prefix), 10)
	wrapped := wrapText(value, wrapWidth)
	for i, line := range wrapped {
		if i == 0 {
			lines = append(lines, prefix+line)
			continue
		}
		lines = append(lines, strings.Repeat(" ", lipgloss.Width(prefix))+line)
	}
	return lines
}

func wrapText(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}
	var lines []string
	for _, rawLine := range strings.Split(text, "\n") {
		words := strings.Fields(rawLine)
		if len(words) == 0 {
			lines = append(lines, "")
			continue
		}
		current := words[0]
		for _, word := range words[1:] {
			if lipgloss.Width(current)+1+lipgloss.Width(word) > width {
				lines = append(lines, current)
				current = word
				continue
			}
			current += " " + word
		}
		lines = append(lines, current)
	}
	return lines
}
