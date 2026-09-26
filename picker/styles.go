package picker

import "github.com/charmbracelet/lipgloss"

// 配色与测速结果界面同源：选中行 62 底 230 字，分隔与灰行用 240，
// 其余只做点缀，避免整屏花哨。
var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("62"))
	titleDimStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))

	ruleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	sectionMarkStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("62"))
	sectionStyle     = lipgloss.NewStyle().Bold(true)

	// selectedStyle 与测速表格的选中行一致。
	selectedStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("62")).
			Foreground(lipgloss.Color("230"))

	okStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("114"))
	dimStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	greyStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	warnStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))

	keyStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("230"))
	labelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	plainStyle = lipgloss.NewStyle()
)
