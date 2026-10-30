// Package export 组装并写出 Clash/Mihomo 导出文件，并提供「导出」与「提前
// 结束」共用的阈值过滤。
//
// 导出形态是固定骨架（GLOSSARY.md「导出文件形态」、ADR-0016）：mixed-port、
// select 与 url-test 两组、国内直连与 MATCH 兜底，除骨架外不加任何字段，
// 不可配置是有意为之。GUI 各轮导出、合并导出与手动保存都走同一份
// BuildProfile/WriteYAML，三处形态天然一致。
package export

import (
	"fmt"
	"os"
	"time"

	"github.com/BoxMiao007/clash-speedtest-plus/internal/speedtester"
	"gopkg.in/yaml.v2"
)

// 导出骨架常量（ADR-0016）：完整 profile 的固定形态，不可配置是有意为之。
const (
	// MixedPort 是导出 profile 的 mixed-port 监听端口。
	MixedPort = 7890
	// SelectGroupName 是手动选组组名，同时是 MATCH 兜底的指向。
	SelectGroupName = "节点选择"
	// AutoGroupName 是自动测速组组名，也是 select 组的第一个成员。
	AutoGroupName = "自动选择"
	// AutoGroupURL 是 url-test 组探测延迟用的地址。
	AutoGroupURL = "http://www.gstatic.com/generate_204"
	// AutoGroupInterval 是 url-test 组的测速间隔（单位：秒）。
	AutoGroupInterval = 300
)

// Filter 是导出/提前结束用的阈值过滤，对应旧 result_filter.go 的 resultFilter。
// 延迟与速度阈值 0 表示不限制；丢包阈值 0 表示「只放行零丢包」，要放过任意
// 丢包必须显式写负数（<0 视为不限制）。
type Filter struct {
	MaxLatency     time.Duration
	MaxPacketLoss  float64 // 百分比，<0 视为不限制
	MinDownloadBps float64
	MinUploadBps   float64
	DownloadTested bool // 模式会测下载速度（fast 模式为 false）
	UploadEnabled  bool // full 模式为 true
}

// Match 判断一条测速结果是否过筛。口径与旧 resultFilter.Match 一致：延迟、
// 丢包、下载、上传四道阈值各自独立；挡位为 0（不限制）时跳过。
func (f Filter) Match(result *speedtester.Result) bool {
	if result == nil {
		return false
	}
	if f.MaxLatency > 0 && result.Latency > f.MaxLatency {
		return false
	}
	if f.MaxPacketLoss >= 0 && result.PacketLoss > f.MaxPacketLoss {
		return false
	}
	// fast 模式不会测速，DownloadSpeed 为 0，此时只按延迟和丢包筛选。
	if f.DownloadTested && f.MinDownloadBps > 0 && result.DownloadSpeed < f.MinDownloadBps {
		return false
	}
	if f.UploadEnabled && f.MinUploadBps > 0 && result.UploadSpeed < f.MinUploadBps {
		return false
	}
	// 组员名必须是字符串：上游已跳过缺 name 的节点，这里兜底要求 name 与
	// server 都在，不让不完整节点混进导出文件。
	if result.ProxyConfig == nil || result.ProxyConfig["name"] == nil || result.ProxyConfig["server"] == nil {
		return false
	}
	return true
}

// BuildProfile 把过筛节点组装成完整可用 profile（GLOSSARY.md「导出文件形态」、
// ADR-0016）：监听端口、select/url-test 两组、国内直连与 MATCH 兜底，除骨架外
// 不加任何字段。0 节点退回纯清单：空组引用是无效配置，而「输出开着必写文件」
// 是既有预期。节点名与组名撞名时组名避让，不改用户节点名。
func BuildProfile(proxies []map[string]any) *speedtester.RawConfig {
	if len(proxies) == 0 {
		return &speedtester.RawConfig{Proxies: proxies}
	}
	names := make([]string, 0, len(proxies))
	taken := make(map[string]bool, len(proxies)+2)
	for _, proxy := range proxies {
		// 组员名必须是字符串：上游 buildProxies 已跳过缺 name 的节点，这里
		// 兜底跳过 name 缺失或非字符串的节点，不让 <nil> 混进组员列表。
		name, ok := proxy["name"].(string)
		if !ok {
			continue
		}
		names = append(names, name)
		taken[name] = true
	}
	// 两个组独立避让；先落定的组名也占位，后定的不会再撞上它。
	selectName := UniqueGroupName(SelectGroupName, taken)
	taken[selectName] = true
	autoName := UniqueGroupName(AutoGroupName, taken)
	taken[autoName] = true
	return &speedtester.RawConfig{
		MixedPort: MixedPort,
		Proxies:   proxies,
		ProxyGroups: []speedtester.ProxyGroup{
			{
				Name:    selectName,
				Type:    "select",
				Proxies: append([]string{autoName}, names...),
			},
			{
				Name:     autoName,
				Type:     "url-test",
				Proxies:  names,
				URL:      AutoGroupURL,
				Interval: AutoGroupInterval,
			},
		},
		Rules: []string{
			"IP-CIDR,127.0.0.0/8,DIRECT,no-resolve",
			"IP-CIDR,10.0.0.0/8,DIRECT,no-resolve",
			"IP-CIDR,192.168.0.0/16,DIRECT,no-resolve",
			"GEOIP,CN,DIRECT",
			"MATCH," + selectName,
		},
	}
}

// UniqueGroupName 给组名避让：候选名与 taken（节点名与已落定组名）撞名时
// 追加 -1、-2 递增后缀，后缀也撞就继续递增；不修改用户节点名（ADR-0016）。
func UniqueGroupName(base string, taken map[string]bool) string {
	name := base
	for i := 1; taken[name]; i++ {
		name = fmt.Sprintf("%s-%d", base, i)
	}
	return name
}

// WriteYAML 把过筛节点写成完整可用 profile（形态见 BuildProfile）。路径由
// 调用方显式传入：GUI 各轮并存，不读全局，避免与下一轮的写入竞争。
func WriteYAML(path string, proxies []map[string]any) error {
	yamlData, err := yaml.Marshal(BuildProfile(proxies))
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, yamlData, 0o644); err != nil {
		return err
	}
	return nil
}
