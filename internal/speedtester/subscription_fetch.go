package speedtester

// FetchSubscriptionBody 拉取订阅地址并规范成 Clash/Mihomo 正文。
// userAgent 为空时用内核默认 UA；返回规范化后的正文与实际使用的地址
// （补 flag=meta 重试时与入参不同，界面可以据实提示）。
func FetchSubscriptionBody(rawURL, userAgent string) ([]byte, string, error) {
	if userAgent == "" {
		userAgent = defaultFetchConfigUA()
	}
	fetch := func(target string) (string, error) {
		body, err := fetchConfigWithUA(target, userAgent)
		if err != nil {
			return "", err
		}
		return string(body), nil
	}
	text, used, err := NormalizeSubscription(rawURL, fetch)
	if err != nil {
		return nil, "", err
	}
	return []byte(text), used, nil
}

// ParseSubscriptionNodes 解析规范化后的订阅正文。正文里的 proxy-providers
// 也用同一个 UA 拉取；返回顺序与 LoadNodesFile 一致。
func ParseSubscriptionNodes(body []byte, userAgent string) ([]*CProxy, error) {
	if userAgent == "" {
		userAgent = defaultFetchConfigUA()
	}
	parsed, err := parseProxyConfig(body, func(target string) ([]byte, error) {
		return fetchConfigWithUA(target, userAgent)
	})
	if err != nil {
		return nil, err
	}
	nodes := make([]*CProxy, 0, len(parsed))
	for _, item := range parsed {
		nodes = append(nodes, item.proxy)
	}
	return nodes, nil
}
