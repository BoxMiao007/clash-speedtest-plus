package main

import (
	"bytes"
	"flag"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/BoxMiao007/clash-speedtest-plus/output"
	"github.com/BoxMiao007/clash-speedtest-plus/picker"
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
// 「默认当前路径」自动命名——本地源「基名-导出.yaml」，订阅轮「导出-时间戳」。
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
// 自定义写词干，默认当前路径开自动命名，关闭清空——按 Esc 重跑时关掉输出
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
		t.Fatalf("默认当前路径态应清词干开自动: path=%q auto=%v", *outputPath, outputAuto)
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
