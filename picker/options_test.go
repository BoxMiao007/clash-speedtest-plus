package picker

import "testing"

// 每个选项行都要能报出自己的值：新增开关漏了 value 分支的话，
// 界面上那行永远是空白（默认开着却看不到「开」）。
func TestEveryOptionHasValue(t *testing.T) {
	options := Options{
		Filter: "f", Block: "b", Mode: "full", DownloadSize: "50", UploadSize: "20",
		Concurrent: "4", Parallel: "6", Timeout: "5s", EarlyStop: "9", MaxLatency: "1s",
		MaxPacketLoss: "100", MinDownload: "5", MinUpload: "2",
		ImageSpeedOnly: true, NameFromConfig: true, NoImage: true,
		OutputPath: "o.yaml", Rename: true, RenameTemplate: "t",
		GistToken: "g", GistAddress: "ga", RepoToken: "r", RepoAddress: "ra",
		RepoFilePath: "p", RepoBranch: "br", ServerURL: "s", UserAgent: "ua",
	}
	for _, row := range optionOrder {
		if got := options.value(row.option); got == "" {
			t.Fatalf("选项 %d 的值为空", row.option)
		}
	}
	if options.value(OptionNameFromConfig) != "开" {
		t.Fatalf("NameFromConfig 值 = %q, want 开", options.value(OptionNameFromConfig))
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

	download := OptionState{Mode: "download", OutputPath: "out.yaml"}
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
}
