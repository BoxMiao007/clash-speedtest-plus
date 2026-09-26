package picker

import "testing"

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
