package picker

import "testing"

// 输出路径的落定前口径（与命令行 -o 共用 ValidateOutputPath）：
// 纯空白视同留空不报错，以 / 或 \ 结尾（含空白后才是分隔符）是目录意图，
// 回车校验要拦下；写全后缀不报错。
func TestValidateTextOutputPath(t *testing.T) {
	if err := validateText(OptionOutputPath, ""); err != nil {
		t.Fatalf("留空仍是不输出: %v", err)
	}
	if err := validateText(OptionOutputPath, " "); err != nil {
		t.Fatalf("纯空白应视同留空: %v", err)
	}
	if err := validateText(OptionOutputPath, "result.yaml"); err != nil {
		t.Fatalf("写全后缀不应报错: %v", err)
	}
	for _, value := range []string{"abc/", `abc\`, "abc/ ", "out/ ", " /tmp/ "} {
		if err := validateText(OptionOutputPath, value); err == nil {
			t.Fatalf("以分隔符结尾的 %q 应报目录意图", value)
		}
	}
}

// 每个选项行都要能报出自己的值：新增开关漏了 value 分支的话，
// 界面上那行永远是空白（默认开着却看不到「开」）。
func TestEveryOptionHasValue(t *testing.T) {
	options := Options{
		Filter: "f", Block: "b", Mode: "full", DownloadSize: "50", UploadSize: "20",
		Concurrent: "4", Parallel: "6", Timeout: "5s", EarlyStop: "9", MaxLatency: "1s",
		MaxPacketLoss: "100", MinDownload: "5", MinUpload: "2",
		ImageSpeedOnly: true, NoImage: true,
		OutputMode: OutputModeCustom, OutputPath: "o.yaml",
		Rename: true, RenameTemplate: "t",
		GistToken: "g", GistAddress: "ga", RepoToken: "r", RepoAddress: "ra",
		RepoFilePath: "p", RepoBranch: "br", ServerURL: "s", UserAgent: "ua",
	}
	for _, row := range optionOrder {
		if got := options.value(row.option); got == "" {
			t.Fatalf("选项 %d 的值为空", row.option)
		}
	}
}

func TestOptionEnabledFollowsModeAndOutput(t *testing.T) {
	fast := OptionState{Mode: "fast"}
	if fast.Enabled(OptionDownloadSize) || fast.Enabled(OptionMinDownload) || fast.Enabled(OptionImageSpeedOnly) {
		t.Fatalf("fast mode left speed options enabled: %+v", fast)
	}
	if fast.Enabled(OptionUploadSize) || fast.Enabled(OptionMinUpload) {
		t.Fatal("fast mode left upload options enabled")
	}

	download := OptionState{Mode: "download", OutputMode: OutputModeCustom, OutputPath: "out.yaml"}
	if !download.Enabled(OptionDownloadSize) || !download.Enabled(OptionMinDownload) || !download.Enabled(OptionImageSpeedOnly) {
		t.Fatal("download mode disabled download options")
	}
	if download.Enabled(OptionUploadSize) || download.Enabled(OptionMinUpload) {
		t.Fatal("download mode left upload options enabled")
	}
	if !download.Enabled(OptionRename) || !download.Enabled(OptionGistToken) {
		t.Fatal("output path should enable rename and upload")
	}

	noOutput := OptionState{Mode: "full"}
	if noOutput.Enabled(OptionRename) || noOutput.Enabled(OptionRenameTemplate) || noOutput.Enabled(OptionGistToken) || noOutput.Enabled(OptionRepoToken) {
		t.Fatal("empty output left rename or upload enabled")
	}
	if !noOutput.Enabled(OptionUploadSize) || !noOutput.Enabled(OptionMinUpload) {
		t.Fatal("full mode disabled upload options")
	}

	// 输出开关跟着输出模式走（见 GLOSSARY.md「输出模式」）：当前目录下
	// 恒开（自动命名不需要词干），自定义要有非空词干，关闭不开。
	auto := OptionState{Mode: "download", OutputMode: OutputModeDefaultPath}
	if !auto.Enabled(OptionRename) || !auto.Enabled(OptionGistAddress) || !auto.Enabled(OptionRepoFilePath) {
		t.Fatal("当前目录下态应让重命名与上传可用")
	}
	emptyCustom := OptionState{Mode: "download", OutputMode: OutputModeCustom, OutputPath: "  "}
	if emptyCustom.Enabled(OptionRename) || emptyCustom.Enabled(OptionRepoToken) {
		t.Fatal("自定义态词干为空时重命名与上传应不可用")
	}
}
