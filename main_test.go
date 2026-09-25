package main

import (
	"bytes"
	"flag"
	"strings"
	"testing"
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
		p := stringFlag("o", "output", "", "输出配置文件路径")
		if err := fs.Parse(tc.args); err != nil {
			t.Fatalf("%v: %v", tc.args, err)
		}
		if *p != tc.want {
			t.Fatalf("%v 得到 %q，期望 %q", tc.args, *p, tc.want)
		}
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
	_ = stringFlag("o", "output", "", "输出配置文件路径")
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
