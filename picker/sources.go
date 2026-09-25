package picker

import "strings"

// SplitSubscriptionText 把选源界面地址框里的文字拆成订阅地址。
// 「逗号加空格」和换行都是分隔；不带空格的逗号留在地址里。空段丢掉。
func SplitSubscriptionText(text string) []string {
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, ", ", "\n")
	var urls []string
	for _, part := range strings.Split(normalized, "\n") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		urls = append(urls, part)
	}
	return urls
}
