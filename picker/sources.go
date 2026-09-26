package picker

import (
	"strings"
	"unicode"
)

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
