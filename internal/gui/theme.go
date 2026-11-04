package gui

import (
	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/flex"
	"github.com/vivaui/viva/ui"
)

// themeMode 是用户可选的配色：深色、浅色，或跟随系统。它是纯展示状态，
// 存在 data/settings.json 的 theme 字段里。
type themeMode uint8

const (
	themeDark themeMode = iota
	themeLight
	themeSystem
)

var (
	themeModeKeys   = []string{"dark", "light", "system"}
	themeModeLabels = []string{"深色", "浅色", "跟随系统"}
)

// parseThemeMode 把设置里的字符串读成模式；空值或未知值按深色处理（首次启动、
// 旧版本数据文件都走这里）。
func parseThemeMode(value string) themeMode {
	for index, key := range themeModeKeys {
		if value == key {
			return themeMode(index)
		}
	}
	return themeDark
}

func (m themeMode) key() string { return themeModeKeys[m.index()] }

func (m themeMode) index() int {
	switch m {
	case themeLight:
		return 1
	case themeSystem:
		return 2
	default:
		return 0
	}
}

// resolveTheme 把模式折算成实际配色；跟随系统时读一次系统外观。只在模式或
// 系统外观变化时调用，不进绘制路径。
func resolveTheme(mode themeMode) (ui.Theme, palette) {
	if mode == themeSystem {
		if app.SystemAppearance() == app.AppearanceLight {
			return lightTheme(), lightPalette()
		}
		return darkTheme(), darkPalette()
	}
	if mode == themeLight {
		return lightTheme(), lightPalette()
	}
	return darkTheme(), darkPalette()
}

// themeModeAt 把分段控件的选中下标还原成模式。
func themeModeAt(index int) themeMode {
	switch index {
	case 1:
		return themeLight
	case 2:
		return themeSystem
	default:
		return themeDark
	}
}

// pillBox 画一个胶囊徽标：显式宽高的圆角底 + 居中文字。只给文字加底色时
// 宽度会随字宽漂移，徽标之间就对不齐。用于版本号、协议、路径形态。
func pillBox(f *flex.Frame, label string, background, foreground ui.Color) flex.Box {
	width := f.Canvas().TextWidth(label, fontCaption) + 2*gapS
	return f.Column().Height(18).Width(width).Radius(9).Background(background).Center().Children(func() {
		f.Text(label).FontSize(fontCaption).Color(foreground)
	})
}

// noticeChip 是一行带淡色衬底的提示条：校验错误、只读警告都用它，位置与
// 内边距一致，扫一眼就能发现。
func noticeChip(f *flex.Frame, message string, tone, background ui.Color) flex.Box {
	return f.Row().FillWidth().PadY(gapXS).PadX(gapS).Radius(f.Theme().Metrics.RadiusControl).
		Background(background).Items(flex.Center).Children(func() {
		f.Text(message).FontSize(fontBody).Color(tone).Ellipsis()
	})
}

// statusDot 是页脚的状态指示灯：颜色区分就绪 / 进行中 / 出错。
func statusDot(f *flex.Frame, color ui.Color) flex.Box {
	return f.Column().Size(8, 8).Radius(4).Background(color)
}

// applyThemeMode 记录模式并重算 token。它不落盘也不重绘：启动时由 newState
// 调用，运行中由 App.setThemeMode 调用。
func (s *state) applyThemeMode(mode themeMode) {
	s.themeMode = mode
	s.theme, s.palette = resolveTheme(mode)
}

// palette 是 ui.Theme 之外的语义色：强调色、行状态底色、轨道与抬升面。
// 组件只认 ui.Theme，应用自己的绘制用 palette，避免各处临时算颜色。
type palette struct {
	Accent     ui.Color // 强调色：进度、焦点、选中描边
	AccentSoft ui.Color // 强调色的低饱和衬底
	Success    ui.Color
	Warning    ui.Color
	Info       ui.Color
	Track      ui.Color // 进度条与滚动条轨道
	RowHover   ui.Color // 行 hover 底
	RowActive  ui.Color // activeNode：点开的行
	RowPin     ui.Color // 手动单测固定行
	RowMarked  ui.Color // 勾选行底
	Grid       ui.Color // 表格内的分隔竖线
	Sunken     ui.Color // 表头、输入区等凹陷面
	Raised     ui.Color // 卡片、弹窗等抬升面
	Ghost      ui.Color // 幽灵按钮 hover 底
}

// 布局节奏：所有视图共用同一组间距与字号，改一处就能整体调密或调松。
const (
	pagePadX   float32 = 20
	headerH    float32 = 56
	filterBarH float32 = 44
	footerH    float32 = 28
	rowH       float32 = 40
	groupBarH  float32 = 32

	gapXS float32 = 4
	gapS  float32 = 8
	gapM  float32 = 12
	gapL  float32 = 16

	fontCaption  float32 = 11
	fontBody     float32 = 12
	fontSubtitle float32 = 13
	fontTitle    float32 = 19
)

// mix 在两色之间线性插值：amount 为 0 返回 base，为 1 返回 top。
// 需要在既有底色上叠一层色调时用它，比写死颜色更耐改。
func mix(base, top ui.Color, amount float32) ui.Color {
	if amount <= 0 {
		return base
	}
	if amount >= 1 {
		return top
	}
	return ui.Color{
		Red:   base.Red + (top.Red-base.Red)*amount,
		Green: base.Green + (top.Green-base.Green)*amount,
		Blue:  base.Blue + (top.Blue-base.Blue)*amount,
		Alpha: base.Alpha + (top.Alpha-base.Alpha)*amount,
	}
}

// alpha 保留通道，只换不透明度。
func alpha(color ui.Color, value float32) ui.Color {
	color.Alpha = value
	return color
}

// appMetrics 是控制件几何：比 Viva 默认略紧，桌面端一屏能多放两行。
func appMetrics() ui.Metrics {
	metrics := ui.DefaultMetrics()
	metrics.ControlHeight = 30
	metrics.RadiusControl = 7
	metrics.RadiusSurface = 10
	metrics.FontSizeS = 11
	metrics.FontSizeM = 13
	metrics.FontSizeL = 20
	metrics.PadControlX = 12
	metrics.PadSurface = 14
	metrics.GapItem = 6
	return metrics
}

// darkTheme 是默认配色：深蓝灰底 + 蓝调强调色，长时间盯着测速表不刺眼。
func darkTheme() ui.Theme {
	return ui.Theme{
		Background:          ui.RGB24(0x0B0E14),
		Foreground:          ui.RGB24(0xE8ECF4),
		Card:                ui.RGB24(0x12161F),
		CardForeground:      ui.RGB24(0xE8ECF4),
		CardElevated:        ui.RGB24(0x171C26),
		Popover:             ui.RGB24(0x171C26),
		PopoverForeground:   ui.RGB24(0xE8ECF4),
		Muted:               ui.RGB24(0x1B212C),
		MutedForeground:     ui.RGB24(0x97A1B2),
		Border:              ui.RGB24(0x262E3B),
		Hover:               ui.RGB24(0x1A2029),
		Input:               ui.RGB24(0x1E2632),
		Primary:             ui.RGB24(0x3B82F6),
		PrimaryForeground:   ui.RGB24(0xF8FAFC),
		Secondary:           ui.RGB24(0x1E2531),
		SecondaryForeground: ui.RGB24(0xE8ECF4),
		Accent:              ui.RGB24(0x202836),
		AccentForeground:    ui.RGB24(0xE8ECF4),
		Ring:                ui.RGB24(0x60A5FA),
		Success:             ui.RGB24(0x34D399),
		Danger:              ui.RGB24(0xF87171),
		DangerForeground:    ui.RGB24(0xFFFFFF),
		Chart: [5]ui.Color{
			ui.RGB24(0x60A5FA), ui.RGB24(0x3B82F6), ui.RGB24(0x818CF8),
			ui.RGB24(0x34D399), ui.RGB24(0xFBBF24),
		},
		Metrics:    appMetrics(),
		Transition: ui.DefaultMotion(),
	}
}

func darkPalette() palette {
	return palette{
		Accent:     ui.RGB24(0x4C8DFF),
		AccentSoft: ui.RGBA(76, 141, 255, 0.14),
		Success:    ui.RGB24(0x34D399),
		Warning:    ui.RGB24(0xFBBF24),
		Info:       ui.RGB24(0x60A5FA),
		Track:      ui.RGB24(0x1E2531),
		RowHover:   ui.RGB24(0x171D28),
		RowActive:  ui.RGBA(76, 141, 255, 0.14),
		RowPin:     ui.RGBA(76, 141, 255, 0.22),
		RowMarked:  ui.RGB24(0x151B26),
		Grid:       ui.RGB24(0x1E2531),
		Sunken:     ui.RGB24(0x0F131A),
		Raised:     ui.RGB24(0x171C26),
		Ghost:      ui.RGBA(255, 255, 255, 0.06),
	}
}

// lightTheme 是白天配色：灰白底、白卡片，边界靠浅灰描边而不是阴影。
func lightTheme() ui.Theme {
	return ui.Theme{
		Background:          ui.RGB24(0xF5F7FA),
		Foreground:          ui.RGB24(0x101827),
		Card:                ui.RGB24(0xFFFFFF),
		CardForeground:      ui.RGB24(0x101827),
		CardElevated:        ui.RGB24(0xFFFFFF),
		Popover:             ui.RGB24(0xFFFFFF),
		PopoverForeground:   ui.RGB24(0x101827),
		Muted:               ui.RGB24(0xEEF1F5),
		MutedForeground:     ui.RGB24(0x64748B),
		Border:              ui.RGB24(0xE2E7EE),
		Hover:               ui.RGB24(0xF1F4F8),
		Input:               ui.RGB24(0xDDE3EB),
		Primary:             ui.RGB24(0x2563EB),
		PrimaryForeground:   ui.RGB24(0xFFFFFF),
		Secondary:           ui.RGB24(0xEDF1F6),
		SecondaryForeground: ui.RGB24(0x1E293B),
		Accent:              ui.RGB24(0xEAEFF7),
		AccentForeground:    ui.RGB24(0x1E293B),
		Ring:                ui.RGB24(0x2563EB),
		Success:             ui.RGB24(0x15803D),
		Danger:              ui.RGB24(0xDC2626),
		DangerForeground:    ui.RGB24(0xFFFFFF),
		Chart: [5]ui.Color{
			ui.RGB24(0x2563EB), ui.RGB24(0x3B82F6), ui.RGB24(0x6366F1),
			ui.RGB24(0x16A34A), ui.RGB24(0xD97706),
		},
		Metrics:    appMetrics(),
		Transition: ui.DefaultMotion(),
	}
}

func lightPalette() palette {
	return palette{
		Accent:     ui.RGB24(0x2563EB),
		AccentSoft: ui.RGBA(37, 99, 235, 0.10),
		Success:    ui.RGB24(0x16A34A),
		Warning:    ui.RGB24(0xD97706),
		Info:       ui.RGB24(0x2563EB),
		Track:      ui.RGB24(0xE6EBF2),
		RowHover:   ui.RGB24(0xF1F5F9),
		RowActive:  ui.RGBA(37, 99, 235, 0.10),
		RowPin:     ui.RGBA(37, 99, 235, 0.16),
		RowMarked:  ui.RGB24(0xEDF2FB),
		Grid:       ui.RGB24(0xEBEFF5),
		Sunken:     ui.RGB24(0xF1F4F8),
		Raised:     ui.RGB24(0xFFFFFF),
		Ghost:      ui.RGBA(15, 23, 42, 0.05),
	}
}
