package picker

import (
	"path/filepath"
	"strings"
	"unicode"
)

// SourceSpec 是参与测速的一个源。多源时每个源各自一轮。
// Value 喂给测速（文件路径，或拉取订阅写出的临时文件）；
// DisplayName 画来源行和提示用（文件名或原订阅地址）。
// FromSubscription 标记订阅源：它没有可跟随的文件名，产物命名忽略。
type SourceSpec struct {
	Value            string
	DisplayName      string
	FromSubscription bool
}

// SplitConfigArg 把命令行 -c 的多源参数拆成源列表：逗号分隔，
// 空段剔除；http 前缀是订阅源，其余是本地文件。
func SplitConfigArg(arg string) []SourceSpec {
	var sources []SourceSpec
	for _, part := range strings.Split(arg, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.HasPrefix(part, "http") {
			sources = append(sources, SourceSpec{Value: part, DisplayName: part, FromSubscription: true})
		} else {
			sources = append(sources, SourceSpec{Value: part, DisplayName: filepath.Base(part)})
		}
	}
	return sources
}

// SplitSubscriptionText 把选源界面地址框里的文字拆成订阅地址。
// 「逗号加空格」和换行都是分隔；不带空格的逗号留在地址里。空段丢掉。
func SplitSubscriptionText(text string) []string {
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, ", ", "\n")
	var urls []string
	for _, part := range strings.Split(normalized, "\n") {
		// 控制字符（如粘贴带进来的 \x00）不可见也不算空白，
		// 必须剥掉，否则拉取时 url.Parse 直接报错。
		part = strings.TrimFunc(part, unicode.IsControl)
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		urls = append(urls, part)
	}
	return urls
}
