package picker

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/faceair/clash-speedtest/speedtester"
)

// fetchSubscriptions 把地址框里的订阅地址逐个规范化，
// 内容写成程序目录下的临时文件，供本次测速当配置用。
// 返回写出的文件和实际用过的地址。任何一条失败都算失败。
func fetchSubscriptions(execDir string, urls []string) ([]string, []string, error) {
	var files []string
	var used []string
	for _, rawURL := range urls {
		body, usedURL, err := speedtester.NormalizeSubscription(rawURL, httpFetch)
		if err != nil {
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

func httpFetch(url string) (string, error) {
	resp, err := http.Get(url)
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
func defaultFetch(execDir string) func([]string) ([]string, []string, error) {
	return func(urls []string) ([]string, []string, error) {
		return fetchSubscriptions(execDir, urls)
	}
}
