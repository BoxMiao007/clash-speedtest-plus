package picker

import (
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
)

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
	{OptionOutputPath, "输出路径", kindText},
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

// OptionState 决定哪些项可用。Mode 为 fast、download 或 full。
type OptionState struct {
	Mode       string
	OutputPath string
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
		return strings.TrimSpace(s.OutputPath) != ""
	default:
		return true
	}
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

// validateEnabledRows 校验所有可用行。返回第一个填错的行。
func (m Model) validateEnabledRows() (optionRow, error) {
	state := m.optionState()
	for _, row := range optionOrder {
		if !state.Enabled(row.option) {
			continue
		}
		if row.kind != kindText {
			continue
		}
		if err := validateText(row.option, m.options.value(row.option)); err != nil {
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
