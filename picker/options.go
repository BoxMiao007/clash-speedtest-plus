package picker

import "strings"

// Option 是选源界面上的一项。变灰的项不检查、也不传给测速。
type Option int

const (
	OptionDownloadSize Option = iota
	OptionUploadSize
	OptionMinDownload
	OptionMinUpload
	OptionImageSpeedOnly
	OptionRename
	OptionRenameTemplate
	OptionGistToken
	OptionRepoToken
)

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
	case OptionRename, OptionRenameTemplate, OptionGistToken, OptionRepoToken:
		return strings.TrimSpace(s.OutputPath) != ""
	default:
		return true
	}
}
