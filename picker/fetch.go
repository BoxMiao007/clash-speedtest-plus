package picker

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/faceair/clash-speedtest/speedtester"
)

// defaultSubscriptionUA 是拉订阅的默认 User-Agent。多数机场订阅
// 按 UA 分流：clash 系标识返回 Clash/Mihomo yaml，别的返回分享链接。
const defaultSubscriptionUA = "clash.meta"

// fetchSubscriptions 把地址框里的订阅地址逐个规范化，
// 内容写成程序目录下的临时文件，供本次测速当配置用。
// ua 是本次请求用的 User-Agent。返回写出的文件和实际用过的地址。
// 任何一条失败都算失败。
func fetchSubscriptions(execDir, ua string, urls []string) ([]string, []string, error) {
	var files []string
	var used []string
	for _, rawURL := range urls {
		body, usedURL, err := speedtester.NormalizeSubscription(rawURL, fetchWithUA(ua))
		if err != nil {
			if errors.Is(err, speedtester.ErrNotClashSubscription) {
				return nil, nil, fmt.Errorf("%s：%s（可在「拉取订阅 UA」填 clash.meta 等标识后重试）", rawURL, err)
			}
			return nil, nil, fmt.Errorf("%s：%s", rawURL, err)
		}
		file, err := writeSubscriptionFile(execDir, body)
		if err != nil {
			return nil, nil, fmt.Errorf("%s：%s", rawURL, err)
		}
		files = append(files, file)
		if usedURL != rawURL {
			used = append(used, usedURL)
		}
	}
	return files, used, nil
}

// fetchWithUA 构造带 User-Agent 的拉取函数。UA 为空时用默认的 clash 标识。
func fetchWithUA(ua string) func(string) (string, error) {
	if strings.TrimSpace(ua) == "" {
		ua = defaultSubscriptionUA
	}
	return func(url string) (string, error) {
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return "", err
		}
		req.Header.Set("User-Agent", ua)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", err
		}
		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		return string(body), nil
	}
}

func writeSubscriptionFile(execDir, body string) (string, error) {
	if execDir == "" {
		execDir = "."
	}
	file, err := os.CreateTemp(execDir, "clash-speedtest-sub-*.yaml")
	if err != nil {
		return "", err
	}
	defer file.Close()
	if _, err := file.WriteString(strings.TrimSpace(body)); err != nil {
		return "", err
	}
	return file.Name(), nil
}

// defaultFetch 是没有注入时的拉取实现。
func defaultFetch(execDir, ua string) func([]string) ([]string, []string, error) {
	return func(urls []string) ([]string, []string, error) {
		return fetchSubscriptions(execDir, ua, urls)
	}
}
