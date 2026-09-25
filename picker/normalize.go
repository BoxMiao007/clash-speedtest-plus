package picker

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"

	"gopkg.in/yaml.v2"
)

// FetchFunc 按地址取回订阅正文。
type FetchFunc func(url string) (string, error)

type clashDocument struct {
	Proxies   []any `yaml:"proxies"`
	Providers any   `yaml:"proxy-providers"`
}

// NormalizeSubscription 把订阅正文变成 Clash/Mihomo yaml。
// 顺序是原样 yaml、整段 base64、再在没有 flag 参数时补 flag=meta 重试。
// proxies 或 proxy-providers 里至少有一项才算成功。used 是实际采用的地址。
func NormalizeSubscription(rawURL string, fetch FetchFunc) (body string, used string, err error) {
	original, err := fetch(rawURL)
	if err != nil {
		return "", "", err
	}
	if normalized, ok := acceptClash(original); ok {
		return normalized, rawURL, nil
	}

	withFlag, flagged := appendFlagMeta(rawURL)
	if !flagged {
		return "", "", fmt.Errorf("订阅内容不是 Clash/Mihomo 配置")
	}
	retried, err := fetch(withFlag)
	if err != nil {
		return "", "", err
	}
	if normalized, ok := acceptClash(retried); ok {
		return normalized, withFlag, nil
	}
	return "", "", fmt.Errorf("订阅内容不是 Clash/Mihomo 配置")
}

func acceptClash(text string) (string, bool) {
	for _, candidate := range candidates(text) {
		var doc clashDocument
		if err := yaml.Unmarshal([]byte(candidate), &doc); err != nil {
			continue
		}
		if len(doc.Proxies) > 0 || hasProviders(doc.Providers) {
			return candidate, true
		}
	}
	return "", false
}

func candidates(text string) []string {
	text = strings.TrimSpace(text)
	out := []string{text}
	if decoded, err := base64.StdEncoding.DecodeString(text); err == nil {
		out = append(out, strings.TrimSpace(string(decoded)))
	}
	return out
}

func hasProviders(providers any) bool {
	switch typed := providers.(type) {
	case map[any]any:
		return len(typed) > 0
	case map[string]any:
		return len(typed) > 0
	default:
		return false
	}
}

func appendFlagMeta(rawURL string) (string, bool) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return rawURL, false
	}
	query := parsed.Query()
	if _, exists := query["flag"]; exists {
		return rawURL, false
	}
	if parsed.RawQuery == "" {
		parsed.RawQuery = "flag=meta"
	} else {
		parsed.RawQuery += "&flag=meta"
	}
	return parsed.String(), true
}
