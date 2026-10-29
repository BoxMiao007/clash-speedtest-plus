package speedtester

import (
	"fmt"
	"log"
	"os"
	"sort"

	"github.com/metacubex/mihomo/adapter"
	"github.com/metacubex/mihomo/adapter/provider"
	"github.com/metacubex/mihomo/constant"
	"gopkg.in/yaml.v2"
)

// proxyFetchFunc 按地址取回一份配置正文。
type proxyFetchFunc func(targetURL string) ([]byte, error)

// namedProxy 是解析结果中的一项。name 是结果表里的键：直接节点用节点名，
// provider 节点用 "[provider] 节点名"（与旧 LoadProxies 的 map 键一致）；
// proxy 是节点本身，Name() 保持配置里的原始节点名。
type namedProxy struct {
	name  string
	proxy *CProxy
}

// LoadNodesFile 读取一份 Clash/Mihomo 配置，返回其中全部可用节点。
// 顺序：proxies 按 YAML 文件顺序，其后 proxy-providers 按 provider 名、节点名排序。
// 不去重 server:port，不按名字过滤（GUI 自己做）。proxies 内同名节点视为错误
// （与内核一致）；provider 节点是独立命名空间，跨 provider 重名不算错误。
func LoadNodesFile(path string) ([]*CProxy, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取节点文件失败: %w", err)
	}
	parsed, err := parseProxyConfig(body, fetchConfigWithDefaultUA)
	if err != nil {
		return nil, fmt.Errorf("解析节点文件失败: %w", err)
	}
	nodes := make([]*CProxy, 0, len(parsed))
	for _, item := range parsed {
		nodes = append(nodes, item.proxy)
	}
	return nodes, nil
}

// fetchConfigWithDefaultUA 用 mihomo 内核默认 UA 拉取 provider 正文。
func fetchConfigWithDefaultUA(targetURL string) ([]byte, error) {
	return fetchConfigWithUA(targetURL, defaultFetchConfigUA())
}

// parseProxyConfig 解析一份配置正文，返回按文件顺序的节点。
// 直接节点在前，provider 节点按 provider 名、节点名排序追加在后；最后按
// 与 LoadProxies 相同的类型白名单过滤，并做映射 IPv6 到 IPv4 的归一化。
// provider 拉取失败只记录日志并跳过该 provider，不让整个文件失败。
func parseProxyConfig(body []byte, fetch proxyFetchFunc) ([]namedProxy, error) {
	rawCfg := &RawConfig{
		Proxies: []map[string]any{},
	}
	if err := yaml.Unmarshal(body, rawCfg); err != nil {
		return nil, fmt.Errorf("unable to parse config: %w", err)
	}

	nodes, err := parseDirectProxies(rawCfg.Proxies)
	if err != nil {
		return nil, err
	}

	providerNames := make([]string, 0, len(rawCfg.Providers))
	for name := range rawCfg.Providers {
		providerNames = append(providerNames, name)
	}
	sort.Strings(providerNames)
	for _, name := range providerNames {
		providerNodes, err := parseProviderProxies(name, rawCfg.Providers[name], fetch)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, providerNodes...)
	}
	return filterSupportedProxies(nodes), nil
}

// parseDirectProxies 按 YAML 文件顺序解析 proxies。重名与内核一致直接报错，
// 且重名检查发生在类型过滤之前，保持旧 LoadProxies 的行为。
func parseDirectProxies(configs []map[string]any) ([]namedProxy, error) {
	nodes := make([]namedProxy, 0, len(configs))
	seen := make(map[string]struct{}, len(configs))
	for i, config := range configs {
		proxy, err := adapter.ParseProxy(config)
		if err != nil {
			return nil, fmt.Errorf("proxy %d: %w", i, err)
		}
		if _, exist := seen[proxy.Name()]; exist {
			return nil, fmt.Errorf("proxy %s is the duplicate name", proxy.Name())
		}
		seen[proxy.Name()] = struct{}{}
		nodes = append(nodes, namedProxy{
			name:  proxy.Name(),
			proxy: &CProxy{Proxy: proxy, Config: config},
		})
	}
	return nodes, nil
}

// parseProviderProxies 解析并拉取一个 proxy-provider，返回带 "[provider] "
// 结果键的节点，并按节点名排序。拉取失败时记录日志并跳过，与旧 LoadProxies 一致。
func parseProviderProxies(name string, config map[string]any, fetch proxyFetchFunc) ([]namedProxy, error) {
	if name == provider.ReservedName {
		return nil, fmt.Errorf("can not defined a provider called `%s`", provider.ReservedName)
	}
	// v1.19.31 起新增的 Tunnel 参数只是挂在节点上的 API 钩子，
	// 测速场景没有 mihomo 的 tunnel 实例，传 nil 即可。
	pd, err := provider.ParseProxyProvider(name, config, nil)
	if err != nil {
		return nil, fmt.Errorf("parse proxy provider %s error: %w", name, err)
	}
	// Initial 会在 interval > 0 时起常驻拉取协程；这里用完即关闭，避免
	// GUI 读取节点后在空闲期留下 goroutine/timer（规格第 3 节）。
	if closer, ok := any(pd).(interface{ Close() error }); ok {
		defer func() {
			_ = closer.Close()
		}()
	}
	// Initial 负责下载 provider 正文；失败只跳过该 provider。
	if err := pd.Initial(); err != nil {
		log.Printf("initial proxy provider %s error: %s", pd.Name(), err)
		return nil, nil
	}

	rawURL, _ := config["url"].(string)
	if rawURL == "" {
		// file/inline provider 不经过 HTTP 拉取，旧路径拿不到原始节点 Config，跳过。
		log.Printf("proxy provider %s has no url, skipped", pd.Name())
		return nil, nil
	}
	body, err := fetch(rawURL)
	if err != nil {
		log.Printf("failed to fetch config: %s", err)
		return nil, nil
	}
	providerCfg := &RawConfig{
		Proxies: []map[string]any{},
	}
	if err := yaml.Unmarshal(body, providerCfg); err != nil {
		return nil, fmt.Errorf("unable to parse config: %w", err)
	}

	// 保留 provider 原始配置，供测速结果与 GUI 身份使用；provider 的
	// filter/exclude-filter 只影响 pd.Proxies()，这里按节点名取交集即可。
	configs := make(map[string]map[string]any, len(providerCfg.Proxies))
	for _, mapping := range providerCfg.Proxies {
		proxyName, nameOK := mapping["name"].(string)
		if !nameOK || mapping["server"] == nil {
			continue
		}
		configs[proxyName] = mapping
	}
	proxies := pd.Proxies()
	nodes := make([]namedProxy, 0, len(proxies))
	for _, proxy := range proxies {
		nodes = append(nodes, namedProxy{
			name:  fmt.Sprintf("[%s] %s", name, proxy.Name()),
			proxy: &CProxy{Proxy: proxy, Config: configs[proxy.Name()]},
		})
	}
	sort.SliceStable(nodes, func(i, j int) bool {
		return nodes[i].proxy.Name() < nodes[j].proxy.Name()
	})
	return nodes, nil
}

// filterSupportedProxies 应用与 LoadProxies 相同的类型白名单与节点地址归一化。
func filterSupportedProxies(nodes []namedProxy) []namedProxy {
	supported := make([]namedProxy, 0, len(nodes))
	for _, node := range nodes {
		switch node.proxy.Type() {
		case constant.Shadowsocks, constant.ShadowsocksR, constant.Snell, constant.Socks5, constant.Http,
			constant.Vmess, constant.Vless, constant.Trojan, constant.Hysteria, constant.Hysteria2,
			constant.WireGuard, constant.Tuic, constant.Ssh, constant.Mieru, constant.AnyTLS, constant.Sudoku:
		default:
			continue
		}
		if server, ok := node.proxy.Config["server"]; ok {
			if serverString, ok := server.(string); ok {
				node.proxy.Config["server"] = convertMappedIPv6ToIPv4(serverString)
			}
		}
		supported = append(supported, node)
	}
	return supported
}
