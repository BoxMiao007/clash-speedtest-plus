package gui

import (
	"errors"

	"github.com/BoxMiao007/clash-speedtest-plus/internal/filter"
	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/flex"
	"github.com/vivaui/viva/ui"
)

var (
	viewLabels      = []string{"测速", "设置"}
	modeLabels      = []string{"简化", "正则"}
	combineLabels   = []string{"全部命中", "任一命中"}
	entryKindLabels = []string{"本地文件", "订阅地址"}
)

// Options 是 GUI 的启动参数。生产代码只传程序目录、版本与构建提交；测试注入
// 临时目录与假引擎。ThemeMode 只在测试里固定配色（"dark"/"light"/"system"），
// 正常启动从 settings.json 读。
type Options struct {
	BaseDir   string
	DataDir   string
	Version   string
	Commit    string
	ThemeMode string
	Runner    Runner
}

// App 是 Viva 应用外壳：拥有身份树、视图缓存和唯一的窗口配置。
type App struct {
	*state
	version     string
	view        flex.View
	invalidator app.Invalidator
	config      app.Config
	paintRoot   func(*flex.Frame)
	// previewArea 是预览对话框内容区上一帧的几何：位图在 view.Draw 之后
	// 画上去，避免遮住对话框自己的标题与按钮。
	previewArea flex.Box
	// brand 是标题栏品牌图标位图，brandBox 是它在上一帧布局里的占位矩形。
	brand    ui.Bitmap
	brandBox flex.Box
	closed   bool
	// stopWatch 解除系统外观订阅；themeStale 表示系统刚换了深浅色，
	// 下一次绘制开始时重算一次 token（绘制路径本身不做分配）。
	stopWatch  func()
	themeStale bool
}

// New 装配应用：打开数据目录、读取条目、预计算首帧要显示的所有文案。
func New(opts Options) (*App, error) {
	if opts.BaseDir == "" {
		return nil, errors.New("程序目录不能为空")
	}
	model, err := newState(opts)
	if err != nil {
		return nil, err
	}
	application := &App{
		state:   model,
		version: versionBadge(opts.Version),
	}
	model.commit = opts.Commit
	// 图标只在启动时解码一次：任务栏图标与标题栏品牌块共用同一份设计。
	application.brand = brandBitmap()
	// 页面与输入框集合都只装配一次：每帧新建捕获变量的闭包或切片会把分配
	// 带进绘制路径。
	application.paintRoot = func(f *flex.Frame) { application.paintPage(f) }
	inputs := []*flex.Field{&model.filterField, &model.entryForm.name, &model.entryForm.url, &model.presetForm.name}
	inputs = append(inputs, model.settingsForm.fields()...)
	width, height := 1180, 800
	var origin *ui.Point
	if model.settings.WindowSet {
		width, height = model.settings.WindowWidth, model.settings.WindowHeight
		origin = &ui.Point{X: float32(model.settings.WindowX), Y: float32(model.settings.WindowY)}
	}
	application.config = app.Config{
		Title:       "订阅测速",
		Width:       width,
		Height:      height,
		MinWidth:    1000,
		MinHeight:   640,
		Origin:      origin,
		Tree:        model.tree,
		Icon:        windowIcon(),
		Paint:       application.paint,
		OnWake:      model.applyPending,
		Invalidator: &application.invalidator,
		OnScroll:    application.onScroll,
		OnKey:       application.onKey,
		OnShortcut:  application.onShortcut,
		OnFileDrop:  application.onFileDrop,
		// 记下窗口的当前位置与尺寸，退出前写回设置；拖动过程中不落盘。
		OnFrameChange: application.onFrameChange,
		TextInput:     flex.Fields(inputs...),
		OnClosed:      model.close,
		ClearColor:    appColor(model.theme.Background),
	}
	model.invalidator = &application.invalidator
	application.applyNativeAppearance()
	application.stopWatch = app.WatchSystemAppearance(application.onSystemAppearance)
	return application, nil
}

// onSystemAppearance 由事件循环线程调用：只记标记并请求重绘。
func (a *App) onSystemAppearance() {
	a.themeStale = true
	if a.state.invalidator != nil {
		a.state.invalidator.Invalidate()
	}
}

// applyNativeAppearance 让系统自己画的部件（macOS 标题栏、文件对话框）跟界面一致。
func (a *App) applyNativeAppearance() {
	app.SetNativeAppearance(a.nativeAppearance())
}

func (a *App) nativeAppearance() app.Appearance {
	if a.themeMode == themeSystem {
		return app.AppearanceUnknown
	}
	if a.themeMode == themeLight {
		return app.AppearanceLight
	}
	return app.AppearanceDark
}

// setThemeMode 切换配色：重算 token、同步原生外观、落盘并重绘。
func (a *App) setThemeMode(mode themeMode) {
	if mode == a.themeMode {
		return
	}
	a.applyThemeMode(mode)
	a.applyNativeAppearance()
	a.config.ClearColor = appColor(a.theme.Background)
	a.persistSettings()
	if a.state.invalidator != nil {
		a.state.invalidator.Invalidate()
	}
}

// refreshTheme 在系统外观变化后重算一次 token；非跟随系统时什么都不做。
func (a *App) refreshTheme() {
	if a.themeMode != themeSystem {
		return
	}
	a.theme, a.palette = resolveTheme(themeSystem)
	a.config.ClearColor = appColor(a.theme.Background)
}

// Config 返回窗口配置，测试把它交给 app.Driver。
func (a *App) Config() app.Config { return a.config }

// Close 停止后台测速并保存设置；重复调用无副作用。
func (a *App) Close() {
	if a.closed {
		return
	}
	a.closed = true
	if a.stopWatch != nil {
		a.stopWatch()
		a.stopWatch = nil
	}
	a.state.close()
}

func (a *App) onScroll(event ui.ScrollEvent) ui.EventResult {
	// 模态对话框打开时不把滚轮传给背景列表/表单。
	if a.dialog != dialogNone {
		return ui.EventResult{}
	}
	if a.presetOpen.Open {
		return a.presetOpen.Wheel(event)
	}
	if a.menu.open {
		return ui.EventResult{}
	}
	if a.active != viewTest {
		// 设置视图的滚动交给 ScrollArea 自己处理。
		return a.view.Wheel(event)
	}
	if pointIn(a.nodeViewport, event.X, event.Y) {
		return scrollList(&a.nodeOffset, a.nodeContent, a.nodeViewport.Height, event.DeltaY)
	}
	return a.view.Wheel(event)
}

func pointIn(rect ui.Rect, x, y float32) bool {
	return x >= rect.X && x < rect.X+rect.Width && y >= rect.Y && y < rect.Y+rect.Height
}

func scrollList(offset *float32, content, viewport, delta float32) ui.EventResult {
	maximum := content - viewport
	if maximum < 0 {
		maximum = 0
	}
	before := *offset
	next := before + delta
	if next < 0 {
		next = 0
	}
	if next > maximum {
		next = maximum
	}
	*offset = next
	return ui.EventResult{Handled: true, Repaint: next != before}
}

func (a *App) paint(canvas *ui.Canvas, size ui.Size) {
	if a.themeStale {
		a.themeStale = false
		a.refreshTheme()
	}
	a.window = size
	a.nodeListValid = false
	a.entryScrollValid = false
	a.settingsScrollValid = false
	// 详情弹窗的「目标行还看得见」标记由行绘制刷新；本帧没画到目标行就
	// 会在 paintDetailPopup 里关闭弹窗。
	a.detailSeen = false
	bounds := ui.Rect{X: 0, Y: 0, Width: size.Width, Height: size.Height}
	canvas.Fill(bounds, a.theme.Background)
	a.view.Draw(canvas, a.theme, bounds, a.paintRoot)
	// 标题栏品牌图标：flex 里只有占位，位图按本帧矩形画（与结果图预览同理）。
	a.paintBrand(canvas)
	// 结果图预览的位图最后画：它只覆盖对话框内容区（上一帧记录的几何），
	// 标题与按钮已经在 flex 里画好了。
	a.paintPreviewImage(canvas)
	// 详情弹窗在 flex 绘制之后用 canvas 直接画（layer 1），盖过列表文字。
	a.paintDetailOverlay(canvas)
	// 拖入文件时整窗覆盖一层提示，说明松开后会发生什么。
	if a.dropActive {
		a.paintDropHint(canvas)
	}
	if a.nodeListValid {
		a.nodeViewport = a.nodeListBox.Rect()
		a.clampNodeOffset()
	}
	// 滚动条画在内容之上：flex 只负责裁剪与偏移，轨道、滑块与拖动都在应用侧。
	a.paintScrollBars(canvas)
}

func (a *App) paintPage(f *flex.Frame) {
	f.Column().Fill().Children(func() {
		a.paintHeader(f)
		a.paintFilterBar(f)
		f.Row().Grow(1).FillWidth().Children(func() {
			if a.entriesCollapsed {
				a.paintEntriesRail(f)
				f.Column().Width(1).FillHeight().Background(a.theme.Border)
			} else {
				// 第一栏宽度由分隔条维护：拖动或方向键改 SplitterState.Pos，
				// 下一帧按新宽度排版。
				f.Column().Width(a.entriesSplit.Pos).FillHeight().Children(func() {
					a.paintEntriesPanel(f)
				})
				f.Splitter(a.ids.entrySplitter, &a.entriesSplit).
					Mins(entriesPanelMin, contentPanelMin).
					Label("调整节点来源面板宽度")
			}
			a.paintContent(f)
		})
		a.paintFooter(f)
		a.paintDialogs(f)
		// 详情弹窗在右键菜单之下：两者同时打开时菜单优先。
		a.paintDetailPopup(f)
		a.paintMenu(f)
	})
}

func (a *App) paintHeader(f *flex.Frame) {
	f.Row().Height(headerH).FillWidth().PadSides(0, pagePadX, 0, pagePadX).Items(flex.Center).Gap(gapM).
		Background(a.theme.Card).Children(func() {
		// 品牌块：与应用任务栏图标同一份设计，flex 只占 24×24 的位置，
		// 位图在 view.Draw 之后画进这个矩形（见 paintBrand）。
		a.brandBox = f.Column().Size(24, 24)
		f.Text("订阅测速").FontSize(fontTitle).Bold()
		if a.version != "" {
			pillBox(f, a.version, a.theme.Muted, a.theme.MutedForeground)
		}
		f.Spacer()
		// 主题放在标题栏：换深浅色是一眼的事，不用进设置页。
		if index, ok := f.Choice(a.ids.themeScope, themeModeLabels, a.themeMode.index()).ChosenIndex(); ok {
			a.setThemeMode(themeModeAt(index))
		}
		f.Column().Width(1).Height(20).Background(a.theme.Border)
		if index, ok := f.Choice(a.ids.viewScope, viewLabels, a.ids.viewIndex(a.active)).ChosenIndex(); ok {
			a.active = viewKind(index)
		}
	})
	f.Column().Height(1).FillWidth().Background(a.theme.Border)
}

func (a *App) paintFilterBar(f *flex.Frame) {
	f.Row().Height(filterBarH).FillWidth().PadX(pagePadX).Items(flex.Center).Gap(gapS).Children(func() {
		f.Text("过滤").FontSize(fontBody).Muted()
		modeIndex := 0
		if a.filterMode == filter.ModeAdvanced {
			modeIndex = 1
		}
		if index, ok := f.Choice(a.ids.modeScope, modeLabels, modeIndex).ChosenIndex(); ok {
			if index == 0 {
				a.setFilterMode(filter.ModeSimple)
			} else {
				a.setFilterMode(filter.ModeAdvanced)
			}
		}
		expression := f.Field(&a.filterField, a.ids.filterField, "过滤表达式", a.filterPlaceholder()).Grow(1)
		if expression.Submitted() {
			a.compileFilter()
			a.persistSettings()
		}
		if a.filterMode == filter.ModeSimple {
			combineIndex := 0
			if a.filterCombine == filter.CombineOr {
				combineIndex = 1
			}
			if index, ok := f.Choice(a.ids.combScope, combineLabels, combineIndex).ChosenIndex(); ok {
				if index == 0 {
					a.setFilterCombine(filter.CombineAnd)
				} else {
					a.setFilterCombine(filter.CombineOr)
				}
			}
			if a.filterCombine == filter.CombineAnd {
				f.Text("每个关键词都要出现").FontSize(fontCaption).Muted().Ellipsis()
			} else {
				f.Text("任一关键词出现即可").FontSize(fontCaption).Muted().Ellipsis()
			}
		} else {
			f.Text("完整正则，区分大小写").FontSize(fontCaption).Muted()
		}
		if f.Button(a.ids.filterApply, "应用").XSmall().Outline().Clicked() {
			a.compileFilter()
			a.persistSettings()
		}
		selected := a.presetSel + 1
		preset := f.Select(a.ids.preset, &a.presetOpen, a.presetLabels, selected).Width(150).Placeholder("预设")
		if index, ok := preset.Picked(); ok {
			if index == 0 {
				a.presetSel = -1
				a.status = "已取消预设选择"
			} else {
				a.applyPreset(index - 1)
			}
		}
		if f.Button(a.ids.savePreset, "保存预设").XSmall().Outline().Clicked() {
			a.openPresetSaveDialog()
		}
		if a.presetSel >= 0 && a.presetSel < len(a.presets) {
			if f.Button(a.ids.renamePreset, "重命名预设").XSmall().Outline().Clicked() {
				a.openPresetRenameDialog()
			}
			if f.Button(a.ids.deletePreset, "删除预设").XSmall().Outline().Destructive().Clicked() {
				a.askDeletePreset()
			}
		}
	})
	if a.filterErr != "" {
		// 错误用带底色的条，位置和语法提示一致，视线不用重新找。
		f.Row().Height(26).FillWidth().PadSides(0, pagePadX, 0, pagePadX).Items(flex.Center).Children(func() {
			f.Row().FillHeight().PadX(gapS).Radius(a.theme.Metrics.RadiusControl).
				Background(alpha(a.theme.Danger, 0.12)).Items(flex.Center).Children(func() {
				f.Text(a.filterErr).FontSize(fontCaption).Color(a.theme.Danger).Ellipsis()
			})
		})
	} else if a.filterMode == filter.ModeSimple {
		// 占位符在输入后消失；语法提示要常驻，随时能查 ! 与通配符。
		f.Row().Height(22).FillWidth().PadX(pagePadX).Items(flex.Center).Children(func() {
			f.Text("语法：空格分隔；! 排除；* 任意长度；? 一个字符；例：香港 *IPLC !试用").FontSize(fontCaption).Muted().Ellipsis()
		})
	}
	f.Column().Height(1).FillWidth().Background(a.theme.Border)
}

func (a *App) filterPlaceholder() string {
	if a.filterMode == filter.ModeAdvanced {
		return "正则表达式，例如 ^HK-.*$"
	}
	return "关键词，空格分隔；! 排除；支持 * 和 ?"
}

func (a *App) paintFooter(f *flex.Frame) {
	f.Column().Height(1).FillWidth().Background(a.theme.Border)
	f.Row().Height(footerH).FillWidth().PadX(pagePadX).Items(flex.Center).Gap(gapS).
		Background(a.theme.Card).Children(func() {
		// 状态灯把「就绪 / 测速中 / 出错」压成一眼能分辨的颜色。
		statusDot(f, a.statusColor())
		f.Text(a.status).FontSize(fontCaption).Color(a.theme.MutedForeground).Ellipsis().Grow(1)
		dataColor := a.theme.MutedForeground
		if a.storeErr != "" {
			dataColor = a.theme.Danger
		}
		f.Text(a.dataText).FontSize(fontCaption).Color(dataColor).Ellipsis().MaxWidthPercent(45)
		if f.Button(a.ids.about, "关于").XSmall().Ghost().Clicked() {
			a.openAbout()
		}
	})
}

// statusColor 是页脚状态灯的颜色：只读数据目录或出错时红，测速/导出进行中时
// 用强调色，其余中性。
func (a *App) statusColor() ui.Color {
	if a.storeErr != "" {
		return a.theme.Danger
	}
	if a.running() {
		return a.palette.Accent
	}
	return a.theme.MutedForeground
}

func (a *App) paintContent(f *flex.Frame) {
	f.Column().Grow(1).FillHeight().Children(func() {
		if a.active == viewSettings {
			a.paintSettings(f)
			return
		}
		a.paintTest(f)
	})
}

func appOpenFiles() ([]string, error) {
	return app.OpenFiles(app.OpenFileOptions{
		Title:   "选择 Clash/Mihomo 配置文件",
		Filters: []app.FileFilter{{Name: "YAML 配置", Pattern: "*.yaml"}, {Name: "所有文件", Pattern: "*"}},
	})
}

func appOpenDirectory() (string, error) {
	return app.OpenDirectory(app.OpenDirectoryOptions{Title: "选择输出目录"})
}

func isCanceled(err error) bool { return errors.Is(err, app.ErrCanceled) }

func appColor(color ui.Color) app.Color {
	return app.Color{
		Red:   float64(color.Red),
		Green: float64(color.Green),
		Blue:  float64(color.Blue),
		Alpha: float64(color.Alpha),
	}
}

// 对话框里的字段与按钮共享这些辅助判断，避免在绘制路径里拼字符串。
func (a *App) entryDialogTitle() string {
	if a.entryForm.editing {
		return "编辑条目"
	}
	return "新建条目"
}

func (a *App) presetDialogTitle() string {
	if a.presetForm.rename {
		return "重命名预设"
	}
	return "保存预设"
}
