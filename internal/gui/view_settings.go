package gui

import (
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"

	"github.com/BoxMiao007/clash-speedtest-plus/internal/store"
	"github.com/vivaui/viva/flex"
	"github.com/vivaui/viva/ui"
)

var (
	modeChoiceLabels   = []string{"快速", "下载", "完整"}
	outputChoiceLabels = []string{"关闭", "当前目录下", "自定义"}
	modeValues         = []string{"fast", "download", "full"}
	outputValues       = []string{store.OutputModeOff, store.OutputModeAuto, store.OutputModeCustom}
)

// settingsForm 是设置视图里的编辑缓冲区：字段全部是文本，保存时统一校验
// 再折算成 store.Settings。图案与重命名是布尔开关；上传口令按密码框显示。
type settingsForm struct {
	modeIdx   int
	outputIdx int

	downloadMB flex.Field
	uploadMB   flex.Field
	timeoutMS  flex.Field
	concurrent flex.Field
	parallel   flex.Field
	serverURL  flex.Field
	userAgent  flex.Field
	earlyStop  flex.Field

	maxLatency    flex.Field
	maxPacketLoss flex.Field
	minDownload   flex.Field
	minUpload     flex.Field

	outputPath     flex.Field
	imageEnabled   bool
	imageSpeedOnly bool
	renameEnabled  bool
	renameTemplate flex.Field

	gistToken    flex.Field
	gistAddress  flex.Field
	repoToken    flex.Field
	repoAddress  flex.Field
	repoFilePath flex.Field
	repoBranch   flex.Field

	err string
}

// load 用一份设置覆盖编辑缓冲区。只在启动、保存、恢复默认时调用，
// 避免每帧 SetText 把光标移走。
func (form *settingsForm) load(settings store.Settings) {
	form.modeIdx = indexOf(modeValues, settings.Mode)
	form.outputIdx = indexOf(outputValues, settings.OutputMode)
	form.downloadMB.SetText(strconv.Itoa(settings.DownloadMB))
	form.uploadMB.SetText(strconv.Itoa(settings.UploadMB))
	form.timeoutMS.SetText(strconv.Itoa(settings.TimeoutMS))
	form.concurrent.SetText(strconv.Itoa(settings.Concurrent))
	form.parallel.SetText(strconv.Itoa(settings.Parallel))
	form.serverURL.SetText(settings.ServerURL)
	form.userAgent.SetText(settings.UserAgent)
	form.earlyStop.SetText(strconv.Itoa(settings.EarlyStop))
	form.maxLatency.SetText(strconv.Itoa(settings.MaxLatencyMS))
	form.maxPacketLoss.SetText(strconv.FormatFloat(settings.MaxPacketLoss, 'f', -1, 64))
	form.minDownload.SetText(strconv.FormatFloat(settings.MinDownloadMBps, 'f', -1, 64))
	form.minUpload.SetText(strconv.FormatFloat(settings.MinUploadMBps, 'f', -1, 64))
	form.outputPath.SetText(settings.OutputPath)
	form.imageEnabled = settings.ImageEnabled
	form.imageSpeedOnly = settings.ImageSpeedOnly
	form.renameEnabled = settings.RenameEnabled
	form.renameTemplate.SetText(settings.RenameTemplate)
	form.gistToken.Secure = true
	form.repoToken.Secure = true
	form.gistToken.SetText(settings.GistToken)
	form.gistAddress.SetText(settings.GistAddress)
	form.repoToken.SetText(settings.RepoToken)
	form.repoAddress.SetText(settings.RepoAddress)
	form.repoFilePath.SetText(settings.RepoFilePath)
	form.repoBranch.SetText(settings.RepoBranch)
	form.err = ""
}

func indexOf(values []string, value string) int {
	for index, candidate := range values {
		if candidate == value {
			return index
		}
	}
	return 0
}

// fields 返回注册给 Config.TextInput 的输入框集合。
func (form *settingsForm) fields() []*flex.Field {
	return []*flex.Field{
		&form.downloadMB, &form.uploadMB, &form.timeoutMS, &form.concurrent,
		&form.parallel, &form.serverURL, &form.userAgent, &form.earlyStop,
		&form.maxLatency, &form.maxPacketLoss, &form.minDownload, &form.minUpload,
		&form.outputPath, &form.renameTemplate,
		&form.gistToken, &form.gistAddress, &form.repoToken, &form.repoAddress,
		&form.repoFilePath, &form.repoBranch,
	}
}

// collectSettings 校验并折算成设置；返回用户可读的第一条错误。
func (s *state) collectSettings() (store.Settings, string) {
	form := &s.settingsForm
	downloadMB, msg := parseIntField(&form.downloadMB, "下载大小（MB）", 1, 102400)
	if msg != "" {
		return store.Settings{}, msg
	}
	uploadMB, msg := parseIntField(&form.uploadMB, "上传大小（MB）", 1, 102400)
	if msg != "" {
		return store.Settings{}, msg
	}
	timeoutMS, msg := parseIntField(&form.timeoutMS, "超时（毫秒）", 1, 3600000)
	if msg != "" {
		return store.Settings{}, msg
	}
	concurrent, msg := parseIntField(&form.concurrent, "单节点并发", 1, 1024)
	if msg != "" {
		return store.Settings{}, msg
	}
	parallel, msg := parseIntField(&form.parallel, "节点并行", 1, 1024)
	if msg != "" {
		return store.Settings{}, msg
	}
	earlyStop, msg := parseIntField(&form.earlyStop, "提前结束", 0, 100000)
	if msg != "" {
		return store.Settings{}, msg
	}
	maxLatency, msg := parseIntField(&form.maxLatency, "最高延迟（毫秒）", 0, 3600000)
	if msg != "" {
		return store.Settings{}, msg
	}
	maxPacketLoss, msg := parseFloatFieldAllowNegative(&form.maxPacketLoss, "最高丢包（%）")
	if msg != "" {
		return store.Settings{}, msg
	}
	if maxPacketLoss < -1 || maxPacketLoss > 100 {
		return store.Settings{}, "最高丢包必须在 -1 到 100 之间（-1 表示不限制）"
	}
	minDownload, msg := parseFloatField(&form.minDownload, "最低下载（MB/s）", 1000000)
	if msg != "" {
		return store.Settings{}, msg
	}
	minUpload, msg := parseFloatField(&form.minUpload, "最低上传（MB/s）", 1000000)
	if msg != "" {
		return store.Settings{}, msg
	}
	outputMode := outputValues[form.outputIdx]
	outputPath := strings.TrimSpace(form.outputPath.Text())
	if outputMode == store.OutputModeCustom && outputPath == "" {
		return store.Settings{}, "输出模式为「自定义」时必须填输出路径"
	}
	if outputMode == store.OutputModeCustom {
		// 只判空不够：目录不存在或填成目录时，错误会延后到后台导出才暴露。
		abs, _ := s.resolvePath(outputPath)
		if abs == "" {
			return store.Settings{}, "输出路径无效"
		}
		dir := filepath.Dir(abs)
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			return store.Settings{}, "输出目录不存在：" + dir
		}
	}
	serverURL := strings.TrimSpace(form.serverURL.Text())
	if serverURL == "" {
		return store.Settings{}, "测速服务器地址不能为空"
	}
	if parsed, err := url.Parse(serverURL); err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return store.Settings{}, "测速服务器地址必须是 http(s) URL"
	}
	renameTemplate := strings.TrimSpace(form.renameTemplate.Text())
	if renameTemplate != "" {
		if _, err := template.New("rename").Parse(renameTemplate); err != nil {
			return store.Settings{}, "重命名模板无效：" + err.Error()
		}
	}
	gistToken := strings.TrimSpace(form.gistToken.Text())
	gistAddress := strings.TrimSpace(form.gistAddress.Text())
	if (gistToken == "") != (gistAddress == "") {
		return store.Settings{}, "Gist 上传需要同时填写 token 与地址"
	}
	repoToken := strings.TrimSpace(form.repoToken.Text())
	repoAddress := strings.TrimSpace(form.repoAddress.Text())
	if (repoToken == "") != (repoAddress == "") {
		return store.Settings{}, "仓库上传需要同时填写 token 与地址"
	}

	settings := store.Settings{
		// 配色不在表单里，但保存设置不能把用户选的深色/浅色清掉。
		Theme:            s.themeMode.key(),
		FilterMode:       string(s.filterMode),
		FilterExpression: s.filterField.Text(),
		FilterCombine:    string(s.filterCombine),

		Mode:       modeValues[form.modeIdx],
		DownloadMB: downloadMB,
		UploadMB:   uploadMB,
		TimeoutMS:  timeoutMS,
		Concurrent: concurrent,
		Parallel:   parallel,
		ServerURL:  serverURL,
		UserAgent:  strings.TrimSpace(form.userAgent.Text()),
		EarlyStop:  earlyStop,

		MaxLatencyMS:    maxLatency,
		MaxPacketLoss:   maxPacketLoss,
		MinDownloadMBps: minDownload,
		MinUploadMBps:   minUpload,

		OutputMode:     outputMode,
		OutputPath:     outputPath,
		ImageEnabled:   form.imageEnabled,
		ImageSpeedOnly: form.imageSpeedOnly,
		RenameEnabled:  form.renameEnabled,
		RenameTemplate: renameTemplate,

		GistToken:    gistToken,
		GistAddress:  gistAddress,
		RepoToken:    repoToken,
		RepoAddress:  repoAddress,
		RepoFilePath: strings.TrimSpace(form.repoFilePath.Text()),
		RepoBranch:   strings.TrimSpace(form.repoBranch.Text()),
	}
	// 窗口几何不在表单里，但必须跟着设置一起写回：保存设置不能把上次退出
	// 时记下的窗口尺寸/位置清掉。
	settings.WindowWidth = s.settings.WindowWidth
	settings.WindowHeight = s.settings.WindowHeight
	settings.WindowX = s.settings.WindowX
	settings.WindowY = s.settings.WindowY
	settings.WindowSet = s.settings.WindowSet
	return settings.Normalize(), ""
}

func parseIntField(field *flex.Field, label string, minimum, maximum int) (int, string) {
	text := strings.TrimSpace(field.Text())
	value, err := strconv.Atoi(text)
	if err != nil {
		return 0, label + "必须是整数"
	}
	if value < minimum {
		return 0, fmt.Sprintf("%s不能小于 %d", label, minimum)
	}
	if value > maximum {
		return 0, fmt.Sprintf("%s不能大于 %d", label, maximum)
	}
	return value, ""
}

func parseFloatField(field *flex.Field, label string, maximum float64) (float64, string) {
	text := strings.TrimSpace(field.Text())
	value, err := strconv.ParseFloat(text, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, label + "必须是有效数字"
	}
	if value < 0 {
		return 0, label + "不能为负数"
	}
	if value > maximum {
		return 0, label + "不能大于 " + strconv.FormatFloat(maximum, 'f', -1, 64)
	}
	return value, ""
}

// parseFloatFieldAllowNegative 用于丢包阈值：-1 是「不限制」的既有口径。
func parseFloatFieldAllowNegative(field *flex.Field, label string) (float64, string) {
	value, err := strconv.ParseFloat(strings.TrimSpace(field.Text()), 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, label + "必须是有效数字"
	}
	return value, ""
}

// resetSettingsForm 把编辑缓冲区恢复成默认参数；点保存后才生效。
func (s *state) resetSettingsForm() {
	s.settingsForm.load(store.DefaultSettings())
	s.settingsForm.err = ""
	s.status = "已填入默认参数，点「保存设置」生效"
}

// chooseOutputDirectory 用系统目录选择器填「输出路径」：保留原文件名（没有
// 就用「导出.yaml」），只替换目录部分，省去手输路径。
func (a *App) chooseOutputDirectory() {
	dir, err := appOpenDirectory()
	if err != nil {
		if isCanceled(err) {
			return
		}
		a.settingsForm.err = "选择目录失败：" + err.Error()
		return
	}
	if dir == "" {
		return
	}
	name := "导出.yaml"
	current := strings.TrimSpace(a.settingsForm.outputPath.Text())
	if current != "" {
		if base := filepath.Base(filepath.FromSlash(current)); base != "." && base != string(filepath.Separator) {
			name = base
		}
	}
	a.settingsForm.outputPath.SetText(filepath.Join(dir, name))
	a.settingsForm.err = ""
}

// saveSettingsForm 校验并保存设置；保存后下一轮测速与导出用新参数。
func (s *state) saveSettingsForm() {
	// 先记下当前窗口几何，再收集表单：窗口状态不归表单管，但不能丢。
	s.captureWindowFrame()
	settings, message := s.collectSettings()
	if message != "" {
		s.settingsForm.err = message
		s.status = "设置未保存：" + message
		return
	}
	s.settings = settings
	s.settingsForm.err = ""
	if s.savable() {
		if err := s.store.SetSettings(settings); err != nil {
			s.settingsForm.err = "保存设置失败：" + err.Error()
			s.status = s.settingsForm.err
			return
		}
	}
	s.resetSort()
	s.refreshSortHeaders()
	s.refreshModeFlags()
	s.compileFilter()
	s.rebuild()
	s.noteSaved("设置已保存")
}

// paintSettings 画设置视图：顶部工具条 + 可滚动分区表单。
func (a *App) paintSettings(f *flex.Frame) {
	f.Row().Height(52).PadX(pagePadX).Items(flex.Center).Gap(gapM).Children(func() {
		f.Text("测速与输出设置").FontSize(fontSubtitle).Bold()
		f.Text("保存后下一轮测速生效").FontSize(fontCaption).Muted()
		f.Spacer()
		if f.Button(a.ids.settingsReset, "恢复默认").Small().Outline().Clicked() {
			a.resetSettingsForm()
		}
		if f.Button(a.ids.settingsSave, "保存设置").Small().Clicked() {
			f.Canvas().AfterPaint(func() { a.saveSettingsForm() })
		}
	})
	if a.settingsForm.err != "" {
		// 校验失败时在工具条下面挂一条红底提示，滚动到哪儿都看得见。
		f.Row().Height(26).FillWidth().PadX(pagePadX).Items(flex.Center).Children(func() {
			f.Row().FillHeight().PadX(gapS).Radius(a.theme.Metrics.RadiusControl).
				Background(alpha(a.theme.Danger, 0.12)).Items(flex.Center).Children(func() {
				f.Text(a.settingsForm.err).FontSize(fontCaption).Color(a.theme.Danger).Ellipsis()
			})
		})
	}
	scroll := f.ScrollArea(a.ids.settingsScroll).Grow(1).FillWidth().PadSides(20, 20, 12, 12).Gap(2)
	a.settingsScrollBox, a.settingsScrollValid = scroll, true
	scroll.Children(func() {
		a.settingsSection(f, "界面")
		f.Row().Height(44).FillWidth().Items(flex.Center).Gap(10).Children(func() {
			f.Text("配色").FontSize(fontBody).Muted().Width(150)
			if index, ok := f.Choice(a.ids.settingsTheme, themeModeLabels, a.themeMode.index()).ChosenIndex(); ok {
				a.setThemeMode(themeModeAt(index))
			}
			f.Text("深色适合长时间看测速表；跟随系统随后续系统深浅色切换").FontSize(10).Muted().Ellipsis().Grow(1)
		})

		a.settingsSection(f, "测速模式")
		f.Row().Height(44).FillWidth().Items(flex.Center).Gap(10).Children(func() {
			f.Text("模式").FontSize(fontBody).Muted().Width(150)
			if index, ok := f.Choice(a.ids.settingsMode, modeChoiceLabels, a.settingsForm.modeIdx).ChosenIndex(); ok {
				a.settingsForm.modeIdx = index
			}
			f.Text("快速只测延迟；下载测下载速度；完整额外测上传").FontSize(10).Muted()
		})

		a.settingsSection(f, "测速参数")
		a.settingsFieldRow(f, "下载大小（MB）", "每个节点下载多少数据", &a.settingsForm.downloadMB, a.ids.setDownload, "50")
		a.settingsFieldRow(f, "上传大小（MB）", "仅完整模式使用", &a.settingsForm.uploadMB, a.ids.setUpload, "20")
		a.settingsFieldRow(f, "超时（毫秒）", "单个请求的超时", &a.settingsForm.timeoutMS, a.ids.setTimeout, "5000")
		a.settingsFieldRow(f, "单节点并发", "同一节点的下载连接数", &a.settingsForm.concurrent, a.ids.setConcurrent, "4")
		a.settingsFieldRow(f, "节点并行", "同时测试的节点数", &a.settingsForm.parallel, a.ids.setParallel, "6")
		a.settingsFieldRow(f, "测速服务器", "下载测速用的地址", &a.settingsForm.serverURL, a.ids.setServer, "https://…")
		a.settingsFieldRow(f, "拉取 UA", "拉取订阅与配置时使用的 User-Agent；空 = 内核默认", &a.settingsForm.userAgent, a.ids.setUA, "留空使用内核默认")
		a.settingsFieldRow(f, "提前结束", "过筛结果达到该数量就停止；0 = 关闭", &a.settingsForm.earlyStop, a.ids.setEarlyStop, "0")

		a.settingsSection(f, "结果阈值（导出与提前结束共用）")
		a.settingsFieldRow(f, "最高延迟（毫秒）", "0 = 不限制", &a.settingsForm.maxLatency, a.ids.setMaxLatency, "0")
		a.settingsFieldRow(f, "最高丢包（%）", "-1 = 不限制，0 = 只放行零丢包", &a.settingsForm.maxPacketLoss, a.ids.setMaxLoss, "100")
		a.settingsFieldRow(f, "最低下载（MB/s）", "仅下载/完整模式生效；0 = 不限制", &a.settingsForm.minDownload, a.ids.setMinDownload, "0")
		a.settingsFieldRow(f, "最低上传（MB/s）", "仅完整模式生效；0 = 不限制", &a.settingsForm.minUpload, a.ids.setMinUpload, "0")

		a.settingsSection(f, "输出")
		f.Row().Height(44).FillWidth().Items(flex.Center).Gap(10).Children(func() {
			f.Text("输出模式").FontSize(fontBody).Muted().Width(150)
			if index, ok := f.Choice(a.ids.settingsOutput, outputChoiceLabels, a.settingsForm.outputIdx).ChosenIndex(); ok {
				a.settingsForm.outputIdx = index
			}
			f.Text("每轮测完是否自动写配置文件：关闭 = 不写；其余见下方开关").FontSize(10).Muted().Ellipsis().Grow(1)
		})
		f.Row().Height(46).FillWidth().Gap(gapM).Items(flex.Center).Children(func() {
			f.Column().Width(150).Gap(1).Children(func() {
				f.Text("输出路径").FontSize(fontBody)
				f.Text("自定义模式的词干；自动补 .yaml").FontSize(10).Muted().Ellipsis()
			})
			f.Field(&a.settingsForm.outputPath, a.ids.setOutputPath, "输出路径", "例如 D:/订阅/导出.yaml").Grow(1)
			if f.Button(a.ids.setOutputBrowse, "浏览…").Small().Outline().Clicked() {
				f.Canvas().AfterPaint(func() { a.chooseOutputDirectory() })
			}
		}) // 三个开关各自控制一种产物，逐行给标签与说明，避免只看名字猜作用。
		if a.settingsToggleRow(f, a.ids.setImage, a.settingsForm.imageEnabled,
			"每轮自动保存结果图（PNG）", "测完在输出目录生成结果图；关闭后仍可在「测速」页点「结果图」手动保存") {
			a.settingsForm.imageEnabled = !a.settingsForm.imageEnabled
		}
		if a.settingsToggleRow(f, a.ids.setSpeedOnly, a.settingsForm.imageSpeedOnly,
			"结果图只保留有速度的节点", "只影响结果图的内容；结果表与导出的配置文件不受影响") {
			a.settingsForm.imageSpeedOnly = !a.settingsForm.imageSpeedOnly
		}
		if a.settingsToggleRow(f, a.ids.setRename, a.settingsForm.renameEnabled,
			"导出配置时按 IP 归属地重命名节点", "只改导出文件里的节点名；关闭则保留原始名称，节点多时命名会逐条查询 IP") {
			a.settingsForm.renameEnabled = !a.settingsForm.renameEnabled
		}
		a.settingsFieldRow(f, "重命名模板", "Go 模板；空 = 内置「🇭🇰 HK 001 | ⬇️ 12.3MB/s」格式", &a.settingsForm.renameTemplate, a.ids.setRenameTmpl, "{{.Flag}} {{.CountryCode}} {{.Index}} | {{.Direction}} {{.Speed}}{{.SpeedUnit}}")

		a.settingsSection(f, "上传")
		a.settingsFieldRow(f, "Gist token", "与地址同时填写才上传；保存在 data/settings.json", &a.settingsForm.gistToken, a.ids.setGistToken, "ghp_…")
		a.settingsFieldRow(f, "Gist 地址", "Gist 地址或 ID；文件名用导出文件名", &a.settingsForm.gistAddress, a.ids.setGistAddress, "https://gist.github.com/…")
		a.settingsFieldRow(f, "仓库 token", "与仓库地址同时填写才上传", &a.settingsForm.repoToken, a.ids.setRepoToken, "ghp_…")
		a.settingsFieldRow(f, "仓库地址", "owner/repo 或仓库地址", &a.settingsForm.repoAddress, a.ids.setRepoAddress, "owner/repo")
		a.settingsFieldRow(f, "仓库文件路径", "留空用导出文件名", &a.settingsForm.repoFilePath, a.ids.setRepoFile, "clash/proxies.yaml")
		a.settingsFieldRow(f, "仓库分支", "留空用默认分支", &a.settingsForm.repoBranch, a.ids.setRepoBranch, "main")
		f.Row().Height(28).FillWidth().Children(func() {
			f.Text("提示：口令以明文保存在程序目录 data/settings.json；只在本机使用，别把数据目录发给别人。").FontSize(10).Muted().Ellipsis()
		})
	})
}

// settingsSection 是分区标题：上留白 + 小号大写感的分组名 + 分隔线，滚动时
// 让每组有明确的边界。
func (a *App) settingsSection(f *flex.Frame, title string) {
	f.Row().Height(38).FillWidth().Items(flex.End).Children(func() {
		f.Text(title).FontSize(fontBody).Bold().Color(a.palette.Accent)
	})
	f.Divider().FillWidth()
}

func (a *App) settingsFieldRow(f *flex.Frame, label, hint string, field *flex.Field, id ui.ID, placeholder string) {
	f.Row().Height(46).FillWidth().Gap(gapM).Items(flex.Center).Children(func() {
		f.Column().Width(150).Gap(1).Children(func() {
			f.Text(label).FontSize(fontBody)
			if hint != "" {
				f.Text(hint).FontSize(10).Muted().Ellipsis()
			}
		})
		f.Field(field, id, label, placeholder).Grow(1)
	})
}

// settingsToggleRow 画一行开关：勾选框 + 标签 + 一句说明，返回本帧是否被点击。
// 说明始终可见，用户不用记住三个开关分别对应哪种产物；勾选框的命中区向右
// 扩到标签末尾，点标签也能切换。
func (a *App) settingsToggleRow(f *flex.Frame, id ui.ID, on bool, label, hint string) bool {
	clicked := false
	f.Row().Height(42).FillWidth().Gap(10).Items(flex.Center).Children(func() {
		labelWidth := f.Canvas().TextWidth(label, fontBody)
		box := f.Checkbox(id, on).Label(label).HitOutset(0, labelWidth+8, 0, 0)
		if box.Clicked() {
			clicked = true
		}
		f.Text(label).FontSize(fontBody)
		f.Text(hint).FontSize(10).Muted().Ellipsis().Grow(1)
	})
	return clicked
}
