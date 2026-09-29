package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/BoxMiao007/clash-speedtest-plus/output"
	"github.com/BoxMiao007/clash-speedtest-plus/picker"
	"github.com/BoxMiao007/clash-speedtest-plus/speedtester"
	"github.com/BoxMiao007/clash-speedtest-plus/tui"
	"gopkg.in/yaml.v2"
)

func TestLaunchOpensPickerOnlyWithoutArgsOnTerminal(t *testing.T) {
	if got := launchChoice(nil, true); got != launchPicker {
		t.Fatalf("无参数且有终端应进选源，得到 %v", got)
	}
	if got := launchChoice([]string{"-c", "a.yaml"}, true); got != launchCLI {
		t.Fatalf("有参数应走命令行，得到 %v", got)
	}
	if got := launchChoice(nil, false); got != launchCLI {
		t.Fatalf("无终端应走命令行，得到 %v", got)
	}
}

func TestNameFromConfigFlagRemoved(t *testing.T) {
	// 「产物跟随文件名」已写死为开（ADR-0013），老脚本传该 flag 应报 unknown flag。
	if flag.CommandLine.Lookup("name-from-config") != nil {
		t.Fatal("-name-from-config 应已删除")
	}
}

func TestApplyPickerOptionsWritesFlags(t *testing.T) {
	applyPickerOptions(picker.Options{
		Filter: "HK", Block: "x1", Mode: "full",
		DownloadSize: "80", UploadSize: "30", Concurrent: "8", Parallel: "4",
		Timeout: "9s", EarlyStop: "20", MaxLatency: "2s", MaxPacketLoss: "50",
		MinDownload: "6", MinUpload: "3", ImageSpeedOnly: true, NoImage: true,
		OutputMode: picker.OutputModeCustom, OutputPath: "out.yaml",
		Rename: false, RenameTemplate: "{{.Index}}",
		GistToken: "gt", GistAddress: "ga", RepoToken: "rt", RepoAddress: "user/repo",
		RepoFilePath: "p.yaml", RepoBranch: "dev", ServerURL: "https://s.example.com", UserAgent: "ua/1",
	})
	switch {
	case *filterRegexConfig != "HK":
		t.Fatalf("filter = %q", *filterRegexConfig)
	case *blockKeywords != "x1":
		t.Fatalf("block = %q", *blockKeywords)
	case *speedMode != "full":
		t.Fatalf("mode = %q", *speedMode)
	case *downloadSize != 80 || *uploadSize != 30:
		t.Fatalf("size = %d/%d", *downloadSize, *uploadSize)
	case *concurrent != 8 || *parallel != 4:
		t.Fatalf("concurrency = %d/%d", *concurrent, *parallel)
	case *timeout != 9*time.Second || *maxLatency != 2*time.Second:
		t.Fatalf("durations = %v/%v", *timeout, *maxLatency)
	case *earlyStop != 20 || *maxPacketLoss != 50:
		t.Fatalf("early-stop/loss = %v/%v", *earlyStop, *maxPacketLoss)
	case *minDownloadSpeed != 6 || *minUploadSpeed != 3:
		t.Fatalf("speeds = %v/%v", *minDownloadSpeed, *minUploadSpeed)
	case !*imageSpeedOnly || !*noImage:
		t.Fatalf("image flags = %v/%v", *imageSpeedOnly, *noImage)
	case *outputPath != "out.yaml":
		t.Fatalf("output = %q", *outputPath)
	case *renameNodes:
		t.Fatal("rename 应关上")
	case *renameTemplate != "{{.Index}}":
		t.Fatalf("template = %q", *renameTemplate)
	case *gistToken != "gt" || *gistAddress != "ga":
		t.Fatalf("gist = %q/%q", *gistToken, *gistAddress)
	case *repoToken != "rt" || *repoAddress != "user/repo":
		t.Fatalf("repo = %q/%q", *repoToken, *repoAddress)
	case *repoFilePath != "p.yaml" || *repoBranch != "dev":
		t.Fatalf("repo path/branch = %q/%q", *repoFilePath, *repoBranch)
	case *serverURL != "https://s.example.com":
		t.Fatalf("server = %q", *serverURL)
	case *userAgent != "ua/1":
		t.Fatalf("ua = %q", *userAgent)
	}
}

func TestApplyPickerOptionsKeepsDefaultsOnEmpty(t *testing.T) {
	before := *filterRegexConfig
	applyPickerOptions(picker.Options{})
	if *filterRegexConfig != before {
		t.Fatalf("filter 被清空: %q", *filterRegexConfig)
	}
}

func TestParallelShortAndLongFlagsShareValue(t *testing.T) {
	orig := flag.CommandLine
	t.Cleanup(func() { flag.CommandLine = orig })

	cases := []struct {
		args []string
		want int
	}{
		{[]string{"-p", "3"}, 3},
		{[]string{"-parallel", "4"}, 4},
		{[]string{"-p=5"}, 5},
		{[]string{"--parallel=6"}, 6},
	}
	for _, tc := range cases {
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		flag.CommandLine = fs
		p := intFlag("p", "parallel", 1, "同时测试的节点数")
		if err := fs.Parse(tc.args); err != nil {
			t.Fatalf("%v: %v", tc.args, err)
		}
		if *p != tc.want {
			t.Fatalf("%v 得到 %d，期望 %d", tc.args, *p, tc.want)
		}
	}
}

func TestParallelFlagHelpShowsBothNames(t *testing.T) {
	orig := flag.CommandLine
	t.Cleanup(func() { flag.CommandLine = orig })

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	var buf bytes.Buffer
	fs.SetOutput(&buf)
	flag.CommandLine = fs
	_ = intFlag("p", "parallel", 1, "同时测试的节点数")
	_ = flag.Bool("v", false, "显示版本信息")
	_ = flag.Bool("fast", false, "快速模式")
	printFlagDefaults(fs)
	help := buf.String()
	if !strings.Contains(help, "同时测试的节点数（也可写 -parallel | 默认: 1）") {
		t.Fatalf("帮助应在 -p 一行注明 -parallel:\n%s", help)
	}
	if strings.Contains(help, "\n  -parallel ") {
		t.Fatalf("-parallel 不应再单独占一行:\n%s", help)
	}
	if !strings.Contains(help, "显示版本信息\n\n  -fast") {
		t.Fatalf("单字母简写和完整参数名之间应空一行:\n%s", help)
	}
}

func TestOutputShortAndLongFlagsShareValue(t *testing.T) {
	orig := flag.CommandLine
	t.Cleanup(func() { flag.CommandLine = orig })

	cases := []struct {
		args []string
		want string
	}{
		{[]string{"-o", "a.yaml"}, "a.yaml"},
		{[]string{"-output", "b.yaml"}, "b.yaml"},
		{[]string{"--output=c.yaml"}, "c.yaml"},
	}
	for _, tc := range cases {
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		flag.CommandLine = fs
		p := stringFlag("o", "output", "", "输出配置文件路径，不带 .yaml/.yml 后缀时自动补 .yaml")
		if err := fs.Parse(tc.args); err != nil {
			t.Fatalf("%v: %v", tc.args, err)
		}
		if *p != tc.want {
			t.Fatalf("%v 得到 %q，期望 %q", tc.args, *p, tc.want)
		}
	}
}

func TestNormalizeOutputFlagCompletesSuffix(t *testing.T) {
	orig := outputPath
	t.Cleanup(func() { outputPath = orig })

	cases := []struct{ in, want string }{
		{"result", "result.yaml"},
		{"result.", "result.yaml"},
		{"result.txt", "result.txt.yaml"},
		{"result.yml", "result.yml"},
		{"result.YAML", "result.YAML"},
		{".yaml", ".yaml"},
		{"dir/result", "dir/result.yaml"},
		{"", ""},
		{"  result  ", "result.yaml"},
		{" ", ""},
		{"\t", ""},
	}
	for _, tc := range cases {
		*outputPath = tc.in
		if err := normalizeOutputFlag(); err != nil {
			t.Fatalf("-o %q 不应报错: %v", tc.in, err)
		}
		if *outputPath != tc.want {
			t.Fatalf("-o %q 落定为 %q，期望 %q", tc.in, *outputPath, tc.want)
		}
	}
}

func TestNormalizeOutputFlagRejectsTrailingSeparator(t *testing.T) {
	orig := outputPath
	t.Cleanup(func() { outputPath = orig })

	for _, in := range []string{"out/", `out\`, "/tmp/", "out/ "} {
		*outputPath = in
		err := normalizeOutputFlag()
		if err == nil {
			t.Fatalf("-o %q 应报目录意图错误", in)
		}
		if !strings.Contains(err.Error(), "输出路径不合法") {
			t.Fatalf("-o %q 的错误文案应含「输出路径不合法」: %v", in, err)
		}
		if *outputPath != in {
			t.Fatalf("报错时不应改写 -o 的值: %q 变成 %q", in, *outputPath)
		}
	}
}

// 落定后的输出路径要按「产物跟随文件名」和「产物序号」参与最终产物名：
// -o result 落定为 result.yaml，文件轮带基名、序号排最前（1.机场A-result.yaml）。
func TestCompletedOutputPathFeedsExportName(t *testing.T) {
	orig := outputPath
	t.Cleanup(func() { outputPath = orig })

	*outputPath = "result"
	if err := normalizeOutputFlag(); err != nil {
		t.Fatalf("-o result 不应报错: %v", err)
	}
	anchored := picker.ResolveOutputPath("/opt/clash-speedtest-plus", *outputPath)
	final := output.FollowedConfigExportPath(anchored, "机场A", time.Now(), 1)
	want := filepath.Join("/opt/clash-speedtest-plus", "1.机场A-result.yaml")
	if final != want {
		t.Fatalf("最终产物名 = %q，期望 %q", final, want)
	}
}

// 每轮最终产物路径由输出模式决定：自定义模式与 v2.3.0 完全一致（回归钉住），
// 「当前目录下」自动命名——本地源「基名-导出.yaml」，订阅轮「导出-时间戳」。
func TestRoundOutputPathFollowsOutputMode(t *testing.T) {
	now := time.Date(2026, 9, 27, 15, 30, 1, 0, time.Local)
	execDir := filepath.Join("/", "opt", "clash-speedtest-plus")
	cases := []struct {
		name       string
		customPath string
		auto       bool
		nameBase   string
		seq        int
		want       string
	}{
		{
			"自定义文件轮（v2.3.0 行为）",
			filepath.Join(execDir, "result.yaml"), false, "机场A", 1,
			filepath.Join(execDir, "1.机场A-result.yaml"),
		},
		{
			"自定义订阅轮靠序号（v2.3.0 行为）",
			filepath.Join(execDir, "result.yaml"), false, "", 2,
			filepath.Join(execDir, "2.result.yaml"),
		},
		{"关闭不加自动", "", false, "机场A", 3, ""},
		{
			"自动本地源按基名-导出",
			"", true, "机场A", 1,
			filepath.Join(execDir, "1.机场A-导出.yaml"),
		},
		{
			"自动订阅轮导出-时间戳兜底",
			"", true, "", 2,
			filepath.Join(execDir, "2.导出-20260927-153001.yaml"),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := roundOutputPath(tc.customPath, tc.auto, execDir, tc.nameBase, now, tc.seq)
			if got != tc.want {
				t.Fatalf("roundOutputPath = %q，期望 %q", got, tc.want)
			}
		})
	}
}

// applyPickerOptions 按输出模式三态解释输出设置（见 CONTEXT.md「输出模式」）：
// 自定义写词干，当前目录下开自动命名，关闭清空——按 Esc 重跑时关掉输出
// 必须真的关掉，不能沿用上一轮的值。
func TestApplyPickerOptionsInterpretsOutputMode(t *testing.T) {
	origOutputPath, origAuto := outputPath, outputAuto
	t.Cleanup(func() { outputPath, outputAuto = origOutputPath, origAuto })

	*outputPath, outputAuto = "stale.yaml", true

	applyPickerOptions(picker.Options{OutputMode: picker.OutputModeCustom, OutputPath: "out.yaml"})
	if *outputPath != "out.yaml" || outputAuto {
		t.Fatalf("自定义态应写词干关自动: path=%q auto=%v", *outputPath, outputAuto)
	}

	applyPickerOptions(picker.Options{OutputMode: picker.OutputModeDefaultPath})
	if *outputPath != "" || !outputAuto {
		t.Fatalf("当前目录下态应清词干开自动: path=%q auto=%v", *outputPath, outputAuto)
	}

	applyPickerOptions(picker.Options{OutputMode: picker.OutputModeClosed})
	if *outputPath != "" || outputAuto {
		t.Fatalf("关闭态应清掉输出设置: path=%q auto=%v", *outputPath, outputAuto)
	}
}

func TestImageSpeedOnlyHelpMentionsResultImage(t *testing.T) {
	orig := flag.CommandLine
	t.Cleanup(func() { flag.CommandLine = orig })

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	var buf bytes.Buffer
	fs.SetOutput(&buf)
	flag.CommandLine = fs
	_ = flag.Bool("image-speed-only", false, "结果图只保留下载或上传速度大于 0 的行；快速模式会忽略")
	printFlagDefaults(fs)
	help := buf.String()
	if !strings.Contains(help, "结果图只保留下载或上传速度大于 0 的行") {
		t.Fatalf("帮助应说明结果图过滤范围:\n%s", help)
	}
}

func TestHelpPutsCommonFlagsFirstAndUsesMegabytes(t *testing.T) {
	orig := flag.CommandLine
	t.Cleanup(func() { flag.CommandLine = orig })

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	var buf bytes.Buffer
	fs.SetOutput(&buf)
	flag.CommandLine = fs
	_ = flag.String("c", "", "配置文件路径，也支持 http(s) 地址")
	_ = flag.Int("download-size", 50, "下载测试大小（单位：MB）")
	_ = flag.Int("upload-size", 20, "上传测试大小，仅完整模式（单位：MB）")
	_ = stringFlag("o", "output", "", "输出配置文件路径，不带 .yaml/.yml 后缀时自动补 .yaml")
	_ = flag.String("gist-token", "", "用于更新 Gist 的 GitHub token")
	printFlagDefaults(fs)
	help := buf.String()
	if strings.Contains(help, "52428800") || strings.Contains(help, "20971520") {
		t.Fatalf("帮助不应再显示字节数:\n%s", help)
	}
	if !strings.Contains(help, "下载测试大小（默认: 50 | 单位：MB）") {
		t.Fatalf("下载大小应把默认值和单位写在同一行:\n%s", help)
	}
	if !strings.Contains(help, "上传测试大小，仅完整模式（默认: 20 | 单位：MB）") {
		t.Fatalf("上传大小应保留模式说明:\n%s", help)
	}
	if strings.Contains(help, "(default ") {
		t.Fatalf("帮助不应再把默认值单独折行:\n%s", help)
	}
	c := strings.Index(help, "  -c ")
	o := strings.Index(help, "  -o ")
	gist := strings.Index(help, "  -gist-token ")
	if c < 0 || o < 0 || gist < 0 || !(c < o && o < gist) {
		t.Fatalf("常用参数应排在前面:\n%s", help)
	}
	if strings.Contains(help, "\n  -output ") {
		t.Fatalf("-output 不应再单独占一行:\n%s", help)
	}
}

// writeMergedExport 的触发条件（CONTEXT.md「合并导出」词条、ADR-0014）：
// 输出模式非关闭、队列正常测完（实际测完轮数等于期望轮数）、至少两轮。
// 关闭态、单轮、Esc 中断、保存失败、准备错误都不写；中途 q/Ctrl+C 留下没
// 测完的轮（轮数对不上期望）不写；全部测完后的 q/Ctrl+C 只是离开方式，照写。
func TestWriteMergedExportTriggers(t *testing.T) {
	now := time.Date(2026, 9, 27, 15, 30, 1, 0, time.Local)
	roundA := []map[string]any{{"name": "A", "server": "1.1.1.1", "type": "ss"}}
	roundB := []map[string]any{{"name": "B", "server": "2.2.2.2", "type": "ss"}}
	twoRounds := [][]map[string]any{roundA, roundB}
	cases := []struct {
		name       string
		result     tui.QueueResult
		outputOpen bool
		rounds     [][]map[string]any
		expected   int
		wantFile   bool
	}{
		{"输出开启两轮正常结束", tui.QueueResult{}, true, twoRounds, 2, true},
		{"输出关闭不写", tui.QueueResult{}, false, twoRounds, 2, false},
		{"单轮不写", tui.QueueResult{}, true, [][]map[string]any{roundA}, 1, false},
		{"没有轮不写", tui.QueueResult{}, true, nil, 0, false},
		{"Esc 中断不写", tui.QueueResult{Escaped: true}, true, twoRounds, 2, false},
		{"全部测完后 Ctrl+C 照写", tui.QueueResult{ExitAll: true}, true, twoRounds, 2, true},
		{"中途 Ctrl+C 留缺口不写", tui.QueueResult{ExitAll: true}, true, [][]map[string]any{roundA}, 2, false},
		{"保存失败不写", tui.QueueResult{Failed: true}, true, twoRounds, 2, false},
		{"准备错误不写", tui.QueueResult{SetupErr: errors.New("加载节点失败")}, true, twoRounds, 2, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := writeMergedExport(tc.result, tc.outputOpen, dir, tc.rounds, tc.expected, now)
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatalf("读临时目录失败: %v", err)
			}
			if !tc.wantFile {
				if path != "" {
					t.Fatalf("不应触发合并导出，却返回 %q", path)
				}
				if len(entries) != 0 {
					t.Fatalf("不应写任何文件，目录里有 %d 个", len(entries))
				}
				return
			}
			want := filepath.Join(dir, "导出-合并-20260927-153001.yaml")
			if path != want {
				t.Fatalf("合并文件路径 = %q，期望 %q", path, want)
			}
			if len(entries) != 1 {
				t.Fatalf("目录里应只有合并文件，实际 %d 个", len(entries))
			}
		})
	}
}

// 合并内容是各轮过筛节点并集：过滤口径与各轮写盘完全一致（同一 buildProxies
// 构建，对应交互轮 saver 与非交互写盘的生产组合），重名节点跳过、保留先完成
// 轮的。重命名在同一次构建里生效，合并拿到的就是重命名后的节点。
func TestWriteMergedExportUnitesRoundsWithRoundCaliber(t *testing.T) {
	origRename := *renameNodes
	*renameNodes = false
	t.Cleanup(func() { *renameNodes = origRename })

	// 与 newResultFilter 同口径：完整模式按下载速度过筛。
	filter := resultFilter{
		mode:             speedtester.SpeedModeDownload,
		maxPacketLoss:    100,
		minDownloadSpeed: 5 * 1024 * 1024,
		downloadSize:     50,
	}
	results1 := []*speedtester.Result{
		{ProxyName: "快", ProxyConfig: map[string]any{"name": "快", "server": "1.1.1.1", "type": "ss"}, DownloadSpeed: 10 * 1024 * 1024},
		{ProxyName: "慢", ProxyConfig: map[string]any{"name": "慢", "server": "2.2.2.2", "type": "ss"}, DownloadSpeed: 1 * 1024 * 1024},
	}
	results2 := []*speedtester.Result{
		// 与第一轮重名的节点：server 不同，合并必须保留第一轮的版本。
		{ProxyName: "快", ProxyConfig: map[string]any{"name": "快", "server": "3.3.3.3", "type": "ss"}, DownloadSpeed: 9 * 1024 * 1024},
		{ProxyName: "新", ProxyConfig: map[string]any{"name": "新", "server": "4.4.4.4", "type": "ss"}, DownloadSpeed: 8 * 1024 * 1024},
	}

	dir := t.TempDir()
	now := time.Date(2026, 9, 27, 15, 30, 1, 0, time.Local)

	// 各轮写盘走生产组合（buildProxies 一次构建、writeConfigFile 落盘）。
	for i, results := range [][]*speedtester.Result{results1, results2} {
		roundPath := filepath.Join(dir, fmt.Sprintf("round-%d.yaml", i+1))
		if err := writeConfigFile(roundPath, buildProxies(results, filter)); err != nil {
			t.Fatalf("写第 %d 轮文件失败: %v", i+1, err)
		}
	}

	rounds := [][]map[string]any{buildProxies(results1, filter), buildProxies(results2, filter)}
	mergedPath := writeMergedExport(tui.QueueResult{}, true, dir, rounds, len(rounds), now)
	wantPath := filepath.Join(dir, "导出-合并-20260927-153001.yaml")
	if mergedPath != wantPath {
		t.Fatalf("合并文件路径 = %q，期望 %q", mergedPath, wantPath)
	}

	type parsedProxy struct {
		Name   string `yaml:"name"`
		Server string `yaml:"server"`
	}
	readProxies := func(path string) []parsedProxy {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("读 %s 失败: %v", path, err)
		}
		var cfg speedtester.RawConfig
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			t.Fatalf("解析 %s 失败: %v", path, err)
		}
		proxies := make([]parsedProxy, 0, len(cfg.Proxies))
		for _, proxy := range cfg.Proxies {
			name, _ := proxy["name"].(string)
			server, _ := proxy["server"].(string)
			proxies = append(proxies, parsedProxy{Name: name, Server: server})
		}
		return proxies
	}

	assertProxies := func(who string, got []parsedProxy, want []parsedProxy) {
		if len(got) != len(want) {
			t.Fatalf("%s 节点 = %v，期望 %v", who, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("%s 节点[%d] = %v，期望 %v", who, i, got[i], want[i])
			}
		}
	}

	// 每轮文件各留各的（含第二轮自己的重名版本）。
	assertProxies("第 1 轮", readProxies(filepath.Join(dir, "round-1.yaml")),
		[]parsedProxy{{Name: "快", Server: "1.1.1.1"}})
	assertProxies("第 2 轮", readProxies(filepath.Join(dir, "round-2.yaml")),
		[]parsedProxy{{Name: "快", Server: "3.3.3.3"}, {Name: "新", Server: "4.4.4.4"}})

	// 合并文件：过筛掉「慢」，重名「快」保留先完成轮的，并集按完成顺序排列。
	assertProxies("合并", readProxies(mergedPath),
		[]parsedProxy{{Name: "快", Server: "1.1.1.1"}, {Name: "新", Server: "4.4.4.4"}})
}
