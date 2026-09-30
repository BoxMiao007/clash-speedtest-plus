package picker

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Option 是选源界面上的一项。变灰的项不检查、也不传给测速。
type Option int

const (
	OptionSpeedMode Option = iota
	OptionFilter
	OptionBlock
	OptionDownloadSize
	OptionUploadSize
	OptionConcurrent
	OptionParallel
	OptionTimeout
	OptionEarlyStop
	OptionMaxLatency
	OptionMaxPacketLoss
	OptionMinDownload
	OptionMinUpload
	OptionImageSpeedOnly
	OptionNoImage
	OptionOutputPath
	OptionRename
	OptionRenameTemplate
	OptionGistToken
	OptionGistAddress
	OptionRepoToken
	OptionRepoAddress
	OptionRepoFilePath
	OptionRepoBranch
	OptionServerURL
	OptionUA
)

type optionKind int

const (
	kindMode optionKind = iota
	kindBool
	kindText
	// kindOutput 是输出路径行专属：三态循环（左右键或点击）加打字编辑，
	// 关闭/当前目录下态显示 < > 符号，自定义态没打字时同样显示（可切），
	// 有内容后退回纯词干（编辑锁定）。
	kindOutput
)

// OutputMode 是输出路径行的三态（见 GLOSSARY.md「输出模式」词条）。
type OutputMode int

const (
	// OutputModeClosed 关闭：不写输出配置。
	OutputModeClosed OutputMode = iota
	// OutputModeCustom 自定义：打字填路径，后缀补全与目录意图校验沿用，留空视同关闭。
	OutputModeCustom
	// OutputModeDefaultPath 当前目录下：不填词干，按「基名-导出.yaml」
	// 自动命名，订阅轮没有基名用「导出-时间戳」兜底。
	OutputModeDefaultPath
)

// cycleOutputMode 在关闭 → 自定义 → 当前目录下之间按 delta 方向循环。
func cycleOutputMode(mode OutputMode, delta int) OutputMode {
	modes := []OutputMode{OutputModeClosed, OutputModeCustom, OutputModeDefaultPath}
	for i, m := range modes {
		if m == mode {
			return modes[(i+delta+len(modes))%len(modes)]
		}
	}
	return OutputModeClosed
}

// changeOutputMode 循环输出模式三态，左右键、空格与点击 < > 共用。
func (o *Options) changeOutputMode(delta int) {
	o.OutputMode = cycleOutputMode(o.OutputMode, delta)
}

// outputModeLocked 报告自定义态是否进入编辑锁定：有内容（TrimSpace 判空，
// 纯空白不算）时左右键、空格与点击都不切换，退格清空后恢复可切
// （见 GLOSSARY.md「输出模式」词条）。锁定判断在调用侧，本函数不做切换。
func (o *Options) outputModeLocked() bool {
	return o.OutputMode == OutputModeCustom && strings.TrimSpace(o.OutputPath) != ""
}

// optionRow 是选项在界面上的呈现：顺序即排列，kind 决定怎么编辑。
type optionRow struct {
	option Option
	label  string
	kind   optionKind
}

// optionOrder 是选项在界面上的顺序，第一行是测速模式。
var optionOrder = []optionRow{
	{OptionSpeedMode, "测速模式", kindMode},
	{OptionFilter, "过滤正则", kindText},
	{OptionBlock, "屏蔽关键字", kindText},
	{OptionDownloadSize, "下载大小", kindText},
	{OptionUploadSize, "上传大小", kindText},
	{OptionConcurrent, "下载并发", kindText},
	{OptionParallel, "节点并行", kindText},
	{OptionTimeout, "单请求超时", kindText},
	{OptionEarlyStop, "提前结束数量", kindText},
	{OptionMaxLatency, "延迟上限", kindText},
	{OptionMaxPacketLoss, "丢包率上限", kindText},
	{OptionMinDownload, "最低下载速度", kindText},
	{OptionMinUpload, "最低上传速度", kindText},
	{OptionImageSpeedOnly, "结果图只留有速度", kindBool},
	{OptionNoImage, "关闭自动结果图", kindBool},
	{OptionOutputPath, "输出路径", kindOutput},
	{OptionRename, "重命名", kindBool},
	{OptionRenameTemplate, "重命名模板", kindText},
	{OptionGistToken, "Gist token", kindText},
	{OptionGistAddress, "Gist 地址", kindText},
	{OptionRepoToken, "仓库 token", kindText},
	{OptionRepoAddress, "仓库地址", kindText},
	{OptionRepoFilePath, "仓库文件路径", kindText},
	{OptionRepoBranch, "仓库分支", kindText},
	{OptionServerURL, "测速服务器", kindText},
	{OptionUA, "拉取订阅 UA", kindText},
}

// 界面默认值照顾双击直用：并行 6 加速整轮测速，结果图只留有速度的行。
// 初次打开与切模式共用这一份默认值，命令行默认值不受影响。
func defaultOptions() Options {
	return Options{
		Filter: ".+", Mode: "download", DownloadSize: "50", UploadSize: "20",
		Concurrent: "4", Parallel: "6", Timeout: "5s", MaxLatency: "1s",
		MaxPacketLoss: "100", MinDownload: "5", MinUpload: "2", Rename: true,
		ImageSpeedOnly: true,
	}
}

func (o *Options) changeMode(delta int) {
	o.Mode = cycleMode(o.Mode, delta)
	defaults := defaultOptions()
	// 不按可用状态筛选：灰行也重置，切回其他模式时不恢复旧的调整。
	for _, row := range optionOrder {
		if optionAdjustable(row.option) {
			o.setText(row.option, defaults.value(row.option))
		}
	}
	o.ImageSpeedOnly = defaults.ImageSpeedOnly
	o.NoImage = defaults.NoImage
	o.Rename = defaults.Rename
}

// OptionState 决定哪些项可用。Mode 为 fast、download 或 full；
// OutputMode 与 OutputPath 一起决定输出设置会不会生效。
type OptionState struct {
	Mode       string
	OutputMode OutputMode
	OutputPath string
}

// OutputOpen 报告输出设置落定后会不会写输出配置：当前目录下恒开
// （自动命名不需要词干），自定义要有非空词干，关闭不开。
func (s OptionState) OutputOpen() bool {
	switch s.OutputMode {
	case OutputModeDefaultPath:
		return true
	case OutputModeCustom:
		return strings.TrimSpace(s.OutputPath) != ""
	default:
		return false
	}
}

// Enabled 报告这一项当前能不能改。
func (s OptionState) Enabled(option Option) bool {
	mode := strings.ToLower(strings.TrimSpace(s.Mode))
	switch option {
	case OptionDownloadSize, OptionMinDownload, OptionImageSpeedOnly:
		return mode != "fast"
	case OptionUploadSize, OptionMinUpload:
		return mode == "full"
	case OptionRename, OptionRenameTemplate, OptionGistToken, OptionGistAddress,
		OptionRepoToken, OptionRepoAddress, OptionRepoFilePath, OptionRepoBranch:
		return s.OutputOpen()
	default:
		return true
	}
}

// defaultHint 是文本行留空时在「默认」后标注的说明，让用户知道不填会沿用什么。
func defaultHint(option Option) string {
	switch option {
	case OptionBlock:
		return "默认 (不过滤)"
	case OptionEarlyStop:
		return "默认 (关闭)"
	case OptionRenameTemplate:
		return "默认 (自动格式)"
	case OptionGistToken, OptionGistAddress, OptionRepoToken, OptionRepoAddress, OptionRepoFilePath, OptionRepoBranch:
		return "默认 (不更新)"
	case OptionServerURL:
		return "默认 (Chrome 官方地址)"
	case OptionUA:
		return "默认 (mihomo 内核)"
	}
	return ""
}

// optionAdjustable 报告选项能否用左右键或点击步进（区别于纯打字的文本行）。
func optionAdjustable(option Option) bool {
	switch option {
	case OptionDownloadSize, OptionUploadSize, OptionConcurrent, OptionParallel,
		OptionEarlyStop, OptionTimeout, OptionMaxLatency, OptionMaxPacketLoss,
		OptionMinDownload, OptionMinUpload:
		return true
	}
	return false
}

// validateText 检查一个文本项当前填的字合不合法。空串交给默认值，不算错。
func validateText(option Option, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	switch option {
	case OptionFilter:
		if _, err := regexp.Compile(value); err != nil {
			return err
		}
	case OptionDownloadSize, OptionUploadSize, OptionConcurrent, OptionParallel, OptionEarlyStop:
		if _, err := strconv.Atoi(value); err != nil {
			return err
		}
	case OptionMaxPacketLoss, OptionMinDownload, OptionMinUpload:
		if _, err := strconv.ParseFloat(value, 64); err != nil {
			return err
		}
	case OptionTimeout, OptionMaxLatency:
		if _, err := time.ParseDuration(value); err != nil {
			return err
		}
	case OptionOutputPath:
		// 目录意图校验与命令行 -o 共用同一份口径
		// （paths.go ValidateOutputPath，见 GLOSSARY.md「后缀补全」词条）。
		if _, err := ValidateOutputPath(value); err != nil {
			return err
		}
	}
	return nil
}

// acceptsInput 报告文本项接受哪些字符，数字类选项在输入时就拦掉无关字符，
// 免得回车才报「不合法」。nil 表示不限。
func acceptsInput(option Option) func(rune) bool {
	switch option {
	case OptionDownloadSize, OptionUploadSize, OptionConcurrent, OptionParallel, OptionEarlyStop:
		return func(r rune) bool { return r >= '0' && r <= '9' }
	case OptionMaxPacketLoss, OptionMinDownload, OptionMinUpload:
		return func(r rune) bool { return r >= '0' && r <= '9' || r == '.' }
	case OptionTimeout, OptionMaxLatency:
		// 允许 5s、500ms、1m 这类时长写法。
		return func(r rune) bool {
			return r >= '0' && r <= '9' || r == '.' || strings.ContainsRune("smhd", r)
		}
	}
	return nil
}

func (o *Options) value(option Option) string {
	switch option {
	case OptionSpeedMode:
		return o.Mode
	case OptionFilter:
		return o.Filter
	case OptionBlock:
		return o.Block
	case OptionDownloadSize:
		return o.DownloadSize
	case OptionUploadSize:
		return o.UploadSize
	case OptionConcurrent:
		return o.Concurrent
	case OptionParallel:
		return o.Parallel
	case OptionTimeout:
		return o.Timeout
	case OptionEarlyStop:
		return o.EarlyStop
	case OptionMaxLatency:
		return o.MaxLatency
	case OptionMaxPacketLoss:
		return o.MaxPacketLoss
	case OptionMinDownload:
		return o.MinDownload
	case OptionMinUpload:
		return o.MinUpload
	case OptionImageSpeedOnly:
		return boolText(o.ImageSpeedOnly)
	case OptionNoImage:
		return boolText(o.NoImage)
	case OptionOutputPath:
		return o.OutputPath
	case OptionRename:
		return boolText(o.Rename)
	case OptionRenameTemplate:
		return o.RenameTemplate
	case OptionGistToken:
		return o.GistToken
	case OptionGistAddress:
		return o.GistAddress
	case OptionRepoToken:
		return o.RepoToken
	case OptionRepoAddress:
		return o.RepoAddress
	case OptionRepoFilePath:
		return o.RepoFilePath
	case OptionRepoBranch:
		return o.RepoBranch
	case OptionServerURL:
		return o.ServerURL
	case OptionUA:
		return o.UserAgent
	default:
		return ""
	}
}

func (o *Options) setText(option Option, text string) {
	switch option {
	case OptionFilter:
		o.Filter = text
	case OptionBlock:
		o.Block = text
	case OptionDownloadSize:
		o.DownloadSize = text
	case OptionUploadSize:
		o.UploadSize = text
	case OptionConcurrent:
		o.Concurrent = text
	case OptionParallel:
		o.Parallel = text
	case OptionTimeout:
		o.Timeout = text
	case OptionEarlyStop:
		o.EarlyStop = text
	case OptionMaxLatency:
		o.MaxLatency = text
	case OptionMaxPacketLoss:
		o.MaxPacketLoss = text
	case OptionMinDownload:
		o.MinDownload = text
	case OptionMinUpload:
		o.MinUpload = text
	case OptionOutputPath:
		o.OutputPath = text
	case OptionRenameTemplate:
		o.RenameTemplate = text
	case OptionGistToken:
		o.GistToken = text
	case OptionGistAddress:
		o.GistAddress = text
	case OptionRepoToken:
		o.RepoToken = text
	case OptionRepoAddress:
		o.RepoAddress = text
	case OptionRepoFilePath:
		o.RepoFilePath = text
	case OptionRepoBranch:
		o.RepoBranch = text
	case OptionServerURL:
		o.ServerURL = text
	case OptionUA:
		o.UserAgent = text
	}
}

func (o *Options) toggle(option Option) {
	switch option {
	case OptionImageSpeedOnly:
		o.ImageSpeedOnly = !o.ImageSpeedOnly
	case OptionNoImage:
		o.NoImage = !o.NoImage
	case OptionRename:
		o.Rename = !o.Rename
	}
}

// adjust 对数字类选项按 delta 步进，左右键调参数用。非数字类选项无操作。
// 步长贴着日常调整习惯：大小 ±5MB、计数 ±1、超时 ±1s、延迟上限 ±100ms、
// 丢包率 ±5%、速度门槛 ±1MB/s。上下限防止按出没有意义的值。
func (o *Options) adjust(option Option, delta int) {
	value := o.value(option)
	switch option {
	case OptionDownloadSize, OptionUploadSize:
		if v, ok := stepInt(value, delta*5, 1, 4096); ok {
			o.setText(option, v)
		}
	case OptionConcurrent, OptionParallel:
		if v, ok := stepInt(value, delta, 1, 64); ok {
			o.setText(option, v)
		}
	case OptionEarlyStop:
		if v, ok := stepInt(value, delta, 0, 10000); ok {
			o.setText(option, v)
		}
	case OptionTimeout:
		if v, ok := stepDuration(value, time.Duration(delta)*time.Second, time.Second, 10*time.Minute); ok {
			o.setText(option, v)
		}
	case OptionMaxLatency:
		if v, ok := stepDuration(value, time.Duration(delta)*100*time.Millisecond, 0, time.Minute); ok {
			o.setText(option, v)
		}
	case OptionMaxPacketLoss:
		if v, ok := stepFloat(value, float64(delta)*5, 0, 100); ok {
			o.setText(option, v)
		}
	case OptionMinDownload, OptionMinUpload:
		if v, ok := stepFloat(value, float64(delta), 0, 1000); ok {
			o.setText(option, v)
		}
	}
}

func stepInt(value string, delta, low, high int) (string, bool) {
	n, err := strconv.Atoi(value)
	if err != nil {
		// 值被手改坏了就不动，回车时校验会指出这一行。
		return value, false
	}
	return strconv.Itoa(min(max(n+delta, low), high)), true
}

func stepDuration(value string, delta, low, high time.Duration) (string, bool) {
	d, err := time.ParseDuration(value)
	if err != nil {
		return value, false
	}
	d += delta
	if d < low {
		d = low
	}
	if d > high {
		d = high
	}
	return d.String(), true
}

func stepFloat(value string, delta, low, high float64) (string, bool) {
	f, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return value, false
	}
	f = min(max(f+delta, low), high)
	return strconv.FormatFloat(f, 'f', -1, 64), true
}

// cycleMode 在快速、下载、完整之间按 delta 方向循环。
func cycleMode(mode string, delta int) string {
	modes := []string{"fast", "download", "full"}
	for i, m := range modes {
		if m == mode {
			return modes[(i+delta+len(modes))%len(modes)]
		}
	}
	return "download"
}

// validateEnabledRows 校验所有可用行。返回第一个填错的行。
func (m Model) validateEnabledRows() (optionRow, error) {
	state := m.optionState()
	for _, row := range optionOrder {
		if !state.Enabled(row.option) {
			continue
		}
		if row.kind != kindText && row.kind != kindOutput {
			continue
		}
		if row.option == OptionOutputPath && m.options.OutputMode != OutputModeCustom {
			// 关闭与当前目录下态不消费自定义词干：残留值不校验、
			// 不拦回车（自定义态留空视同关闭，空值校验本身也不报错）。
			continue
		}
		if err := validateText(row.option, m.options.value(row.option)); err != nil {
			if errors.Is(err, ErrOutputPathDirectory) {
				// 目录意图的原因对用户有用：状态行带上原因，其余选项
				// 维持「不合法」的短包装。
				return row, fmt.Errorf("「%s」不合法：%v", row.label, err)
			}
			return row, fmt.Errorf("「%s」不合法", row.label)
		}
	}
	return optionRow{}, nil
}

func boolText(on bool) string {
	if on {
		return "开"
	}
	return "关"
}
