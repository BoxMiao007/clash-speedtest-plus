package export

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/BoxMiao007/clash-speedtest-plus/internal/speedtester"
	"gopkg.in/yaml.v2"
)

// 写盘产物的解析视图：只声明外部可见的 yaml 字段，断言不依赖写盘实现的结构。
type writtenProfile struct {
	MixedPort   int              `yaml:"mixed-port"`
	Proxies     []map[string]any `yaml:"proxies"`
	ProxyGroups []struct {
		Name     string   `yaml:"name"`
		Type     string   `yaml:"type"`
		Proxies  []string `yaml:"proxies"`
		URL      string   `yaml:"url"`
		Interval int      `yaml:"interval"`
	} `yaml:"proxy-groups"`
	Rules []string `yaml:"rules"`
}

// writeAndParseProfile 走真实写盘再解析回来，供导出形态断言共用。
func writeAndParseProfile(t *testing.T, proxies []map[string]any) ([]byte, writtenProfile) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "out.yaml")
	if err := WriteYAML(path, proxies); err != nil {
		t.Fatalf("写盘失败: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读盘失败: %v", err)
	}
	var profile writtenProfile
	if err := yaml.Unmarshal(data, &profile); err != nil {
		t.Fatalf("解析写盘产物失败: %v\n%s", err, data)
	}
	return data, profile
}

// fakeProxy 手搓一个写盘用的假节点（测试不走内核解析，字段够断言即可）。
func fakeProxy(name, server string) map[string]any {
	return map[string]any{"name": name, "type": "ss", "server": server, "port": 8388}
}

// wantExportRules 组装导出骨架的期望 rules：直连四条与 MATCH 兜底共用一份
// 字面量，MATCH 引用最终落定的「节点选择」组名（ADR-0016）。
func wantExportRules(selectName string) []string {
	return append([]string{
		"IP-CIDR,127.0.0.0/8,DIRECT,no-resolve",
		"IP-CIDR,10.0.0.0/8,DIRECT,no-resolve",
		"IP-CIDR,192.168.0.0/16,DIRECT,no-resolve",
		"GEOIP,CN,DIRECT",
	}, "MATCH,"+selectName)
}

// WriteYAML 写出的应是完整可用 profile（GLOSSARY.md「导出文件形态」、
// ADR-0016）：mixed-port 7890、select/url-test 两组、国内直连与 MATCH 兜底，
// 除骨架外不加任何字段，也不再有空 proxy-providers 噪音行。
func TestWriteYAMLWritesFullProfile(t *testing.T) {
	proxies := []map[string]any{
		fakeProxy("香港 01", "1.2.3.4"),
		fakeProxy("日本 02", "5.6.7.8"),
	}
	data, profile := writeAndParseProfile(t, proxies)

	if strings.Contains(string(data), "proxy-providers") {
		t.Fatalf("不应出现 proxy-providers 行:\n%s", data)
	}
	var keys map[string]any
	if err := yaml.Unmarshal(data, &keys); err != nil {
		t.Fatalf("解析顶层键失败: %v", err)
	}
	if len(keys) != 4 {
		t.Fatalf("顶层应只有骨架四键，实际 %v", keys)
	}
	for _, key := range []string{"mixed-port", "proxies", "proxy-groups", "rules"} {
		if _, ok := keys[key]; !ok {
			t.Fatalf("顶层缺 %s 键: %v", key, keys)
		}
	}

	if MixedPort != 7890 {
		t.Fatalf("MixedPort 常量 = %d，期望 7890", MixedPort)
	}
	if profile.MixedPort != MixedPort {
		t.Fatalf("mixed-port = %d，期望 %d", profile.MixedPort, MixedPort)
	}
	if len(profile.Proxies) != 2 || fmt.Sprint(profile.Proxies[0]["name"]) != "香港 01" || fmt.Sprint(profile.Proxies[1]["name"]) != "日本 02" {
		t.Fatalf("proxies 应原样带全部过筛节点: %v", profile.Proxies)
	}
	if len(profile.ProxyGroups) != 2 {
		t.Fatalf("应有两个组，实际 %d 个: %+v", len(profile.ProxyGroups), profile.ProxyGroups)
	}
	selectGroup, autoGroup := profile.ProxyGroups[0], profile.ProxyGroups[1]
	if selectGroup.Name != SelectGroupName || selectGroup.Type != "select" {
		t.Fatalf("第一组应是 select「%s」: %+v", SelectGroupName, selectGroup)
	}
	wantMembers := []string{AutoGroupName, "香港 01", "日本 02"}
	if !reflect.DeepEqual(selectGroup.Proxies, wantMembers) {
		t.Fatalf("select 组成员 = %v，期望 %v", selectGroup.Proxies, wantMembers)
	}
	if autoGroup.Name != AutoGroupName || autoGroup.Type != "url-test" {
		t.Fatalf("第二组应是 url-test「%s」: %+v", AutoGroupName, autoGroup)
	}
	if !reflect.DeepEqual(autoGroup.Proxies, []string{"香港 01", "日本 02"}) {
		t.Fatalf("url-test 组成员 = %v，期望全部节点", autoGroup.Proxies)
	}
	if autoGroup.URL != "http://www.gstatic.com/generate_204" || autoGroup.Interval != 300 {
		t.Fatalf("url-test 组 url/interval = %q/%d", autoGroup.URL, autoGroup.Interval)
	}
	wantRules := wantExportRules(SelectGroupName)
	if !reflect.DeepEqual(profile.Rules, wantRules) {
		t.Fatalf("rules = %v，期望 %v", profile.Rules, wantRules)
	}
}

// 0 节点退回纯清单：只有 proxies: []，无端口无组无规则——空组引用是无效
// 配置，而「输出开着必写文件」是既有预期（ADR-0016）。
func TestWriteYAMLZeroNodesFallsBackToPlainList(t *testing.T) {
	data, profile := writeAndParseProfile(t, []map[string]any{})
	if string(data) != "proxies: []\n" {
		t.Fatalf("0 节点应只写纯清单，实际:\n%s", data)
	}
	if len(profile.Proxies) != 0 || profile.MixedPort != 0 || len(profile.ProxyGroups) != 0 || len(profile.Rules) != 0 {
		t.Fatalf("0 节点不应带骨架字段: %+v", profile)
	}
	if BuildProfile(nil) == nil || len(BuildProfile(nil).Proxies) != 0 {
		t.Fatalf("nil 节点同样退回纯清单")
	}
}

// 节点名与组名撞名时组名追加递增后缀避让：两个组独立检查，后缀也撞就继续
// 递增（多组同时撞则逐个避让）；rules 的 MATCH 引用避让后的最终「节点选择」
// 组名；用户节点名保持原样（ADR-0016）。
func TestBuildProfileAvoidsGroupNameCollision(t *testing.T) {
	cases := []struct {
		name       string
		nodeNames  []string
		wantSelect string
		wantAuto   string
	}{
		{"无撞名", []string{"香港 01", "日本 02"}, "节点选择", "自动选择"},
		{"节点选择撞名", []string{"节点选择", "日本 02"}, "节点选择-1", "自动选择"},
		{"自动选择撞名", []string{"自动选择", "日本 02"}, "节点选择", "自动选择-1"},
		{"自动选择后缀也撞继续递增", []string{"自动选择", "自动选择-1", "香港 01"}, "节点选择", "自动选择-2"},
		{"节点选择后缀也撞继续递增", []string{"节点选择", "节点选择-1"}, "节点选择-2", "自动选择"},
		{"两组同时撞逐个避让", []string{"节点选择", "自动选择"}, "节点选择-1", "自动选择-1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			proxies := make([]map[string]any, 0, len(tc.nodeNames))
			for i, name := range tc.nodeNames {
				proxies = append(proxies, fakeProxy(name, fmt.Sprintf("1.1.1.%d", i+1)))
			}
			_, profile := writeAndParseProfile(t, proxies)

			if len(profile.ProxyGroups) != 2 {
				t.Fatalf("应有两个组，实际 %d 个", len(profile.ProxyGroups))
			}
			selectGroup, autoGroup := profile.ProxyGroups[0], profile.ProxyGroups[1]
			if selectGroup.Name != tc.wantSelect || selectGroup.Type != "select" {
				t.Fatalf("select 组 = %s(%s)，期望 %s", selectGroup.Name, selectGroup.Type, tc.wantSelect)
			}
			if autoGroup.Name != tc.wantAuto || autoGroup.Type != "url-test" {
				t.Fatalf("url-test 组 = %s(%s)，期望 %s", autoGroup.Name, autoGroup.Type, tc.wantAuto)
			}
			// select 组成员：避让后的自动组名 + 全部节点名（用户节点名不动）。
			wantMembers := append([]string{tc.wantAuto}, tc.nodeNames...)
			if !reflect.DeepEqual(selectGroup.Proxies, wantMembers) {
				t.Fatalf("select 组成员 = %v，期望 %v", selectGroup.Proxies, wantMembers)
			}
			wantRules := wantExportRules(tc.wantSelect)
			if !reflect.DeepEqual(profile.Rules, wantRules) {
				t.Fatalf("rules = %v，期望 %v", profile.Rules, wantRules)
			}
			gotNames := make([]string, 0, len(profile.Proxies))
			for _, proxy := range profile.Proxies {
				gotNames = append(gotNames, fmt.Sprint(proxy["name"]))
			}
			if !reflect.DeepEqual(gotNames, tc.nodeNames) {
				t.Fatalf("节点名不应被改动: %v，期望 %v", gotNames, tc.nodeNames)
			}
		})
	}
}

// UniqueGroupName 只看 taken，不改传入的 map；撞名时从 -1 起追加后缀。
func TestUniqueGroupName(t *testing.T) {
	taken := map[string]bool{"节点选择": true, "节点选择-1": true}
	if got := UniqueGroupName("节点选择", taken); got != "节点选择-2" {
		t.Fatalf("撞名应继续递增: %q", got)
	}
	if got := UniqueGroupName("节点选择-1", taken); got != "节点选择-1-1" {
		t.Fatalf("候选名本身撞名应在完整候选名后追加后缀: %q", got)
	}
	if got := UniqueGroupName("新组", taken); got != "新组" {
		t.Fatalf("未撞名应原样返回: %q", got)
	}
	if len(taken) != 2 || !taken["节点选择-1"] {
		t.Fatalf("UniqueGroupName 不应修改 taken: %v", taken)
	}
	if got := UniqueGroupName("新组", nil); got != "新组" {
		t.Fatalf("nil taken 应原样返回: %q", got)
	}
}

// Filter 与旧 resultFilter.Match 口径一致：延迟、丢包、下载、上传四道阈值
// 各自独立，挡位为 0（不限制）时跳过；缺 name/server 的结果一律不过筛。
func TestFilterMatch(t *testing.T) {
	validResult := func() *speedtester.Result {
		return &speedtester.Result{
			ProxyConfig:   map[string]any{"name": "proxy-a", "server": "1.1.1.1"},
			Latency:       50 * time.Millisecond,
			PacketLoss:    1,
			DownloadSpeed: 10 * 1024 * 1024,
			UploadSpeed:   5 * 1024 * 1024,
		}
	}

	baseFilter := Filter{
		MaxLatency:     100 * time.Millisecond,
		MaxPacketLoss:  10,
		MinDownloadBps: 5 * 1024 * 1024,
		MinUploadBps:   2 * 1024 * 1024,
		DownloadTested: true,
		UploadEnabled:  true,
	}

	if !baseFilter.Match(validResult()) {
		t.Fatalf("达标结果应过筛")
	}

	cases := []struct {
		name   string
		filter Filter
		change func(*speedtester.Result)
		want   bool
	}{
		{"延迟达标", baseFilter, func(*speedtester.Result) {}, true},
		{"延迟超阈值", baseFilter, func(r *speedtester.Result) { r.Latency = 150 * time.Millisecond }, false},
		{"丢包超阈值", baseFilter, func(r *speedtester.Result) { r.PacketLoss = 20 }, false},
		{"丢包等于阈值", baseFilter, func(r *speedtester.Result) { r.PacketLoss = 10 }, true},
		{"丢包负数视为不限", Filter{MaxPacketLoss: -1}, func(r *speedtester.Result) { r.PacketLoss = 99 }, true},
		{"下载低于阈值", baseFilter, func(r *speedtester.Result) { r.DownloadSpeed = 1024 }, false},
		{"下载等于阈值", baseFilter, func(r *speedtester.Result) { r.DownloadSpeed = 5 * 1024 * 1024 }, true},
		{"测下载的挡位为 0 不限", Filter{MaxPacketLoss: -1, DownloadTested: true}, func(r *speedtester.Result) { r.DownloadSpeed = 0 }, true},
		{"不测下载时忽略下载速度", Filter{MaxPacketLoss: -1, MinDownloadBps: 5 * 1024 * 1024, DownloadTested: false}, func(r *speedtester.Result) { r.DownloadSpeed = 0 }, true},
		{"上传低于阈值", baseFilter, func(r *speedtester.Result) { r.UploadSpeed = 1024 }, false},
		{"只测下载时不看上传", Filter{MaxPacketLoss: -1, MinUploadBps: 2 * 1024 * 1024, DownloadTested: true}, func(r *speedtester.Result) { r.UploadSpeed = 0 }, true},
		{"零值 Filter 不限延迟与速度", Filter{}, func(r *speedtester.Result) {
			r.Latency = time.Hour
			r.DownloadSpeed = 0
			r.UploadSpeed = 0
			r.PacketLoss = 0
		}, true},
		{"丢包阈值为 0 只放行零丢包", Filter{}, func(r *speedtester.Result) { r.PacketLoss = 1 }, false},
		{"缺 name 不过筛", baseFilter, func(r *speedtester.Result) { delete(r.ProxyConfig, "name") }, false},
		{"缺 server 不过筛", baseFilter, func(r *speedtester.Result) { delete(r.ProxyConfig, "server") }, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := validResult()
			tc.change(result)
			if got := tc.filter.Match(result); got != tc.want {
				t.Fatalf("Match = %v，期望 %v", got, tc.want)
			}
		})
	}

	if baseFilter.Match(nil) {
		t.Fatalf("nil 结果必须不过筛")
	}
}
