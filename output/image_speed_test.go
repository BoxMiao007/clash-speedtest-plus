package output

import (
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/speedtester"
)

func TestImageSpeedOnlyKeepsRowsWithDownloadOrUploadSpeed(t *testing.T) {
	results := []*speedtester.Result{
		{ProxyName: "无速度", ProxyType: "SS", Latency: 100 * time.Millisecond},
		{ProxyName: "有下载", ProxyType: "SS", Latency: 80 * time.Millisecond, DownloadSpeed: 2 * 1024 * 1024},
		{ProxyName: "下载报错", ProxyType: "SS", Latency: 90 * time.Millisecond, DownloadError: "timeout"},
		{ProxyName: "只有上传", ProxyType: "SS", Latency: 70 * time.Millisecond, UploadSpeed: 1024 * 1024},
	}

	rows := BuildImageRows(results, speedtester.SpeedModeFull)
	kept := FilterImageRowsBySpeed(rows, true)

	if len(kept.Rows) != 2 {
		t.Fatalf("应留下下载或上传速度大于 0 的两行，实际 %d", len(kept.Rows))
	}
	if kept.Rows[0].Cells[0] != "1." || kept.Rows[0].Cells[1] != "有下载" {
		t.Fatalf("第一行应重编为 1. 有下载，实际 %#v", kept.Rows[0].Cells)
	}
	if kept.Rows[1].Cells[0] != "2." || kept.Rows[1].Cells[1] != "只有上传" {
		t.Fatalf("第二行应重编为 2. 只有上传，实际 %#v", kept.Rows[1].Cells)
	}
	if kept.Invalid != 2 || kept.Testing != 0 {
		t.Fatalf("无效应为 2、测试中应为 0，实际 无效 %d 测试中 %d", kept.Invalid, kept.Testing)
	}
}

func TestImageSpeedSummaryOmitsZeroCounts(t *testing.T) {
	now := time.Date(2026, 9, 25, 15, 4, 5, 0, time.UTC)
	base := SummaryLine(now, speedtester.SpeedModeDownload, "已完成", 50, 100)

	all := AppendImageSpeedCounts(base, 12, 38, 4)
	if all != "2026-09-25 15:04:05  下载  已完成  50/100（有效 12，无效 38，测试中 4）" {
		t.Fatalf("三项都应写上: %q", all)
	}
	finished := AppendImageSpeedCounts(base, 26, 175, 0)
	if finished != "2026-09-25 15:04:05  下载  已完成  50/100（有效 26，无效 175）" {
		t.Fatalf("测试中为 0 应省略: %q", finished)
	}
	onlyTesting := AppendImageSpeedCounts(base, 0, 0, 4)
	if onlyTesting != "2026-09-25 15:04:05  下载  已完成  50/100（测试中 4）" {
		t.Fatalf("有效和无效为 0 应省略: %q", onlyTesting)
	}
	onlyValid := AppendImageSpeedCounts(base, 50, 0, 0)
	if onlyValid != "2026-09-25 15:04:05  下载  已完成  50/100（有效 50）" {
		t.Fatalf("只有效时应写出有效: %q", onlyValid)
	}
	none := AppendImageSpeedCounts(base, 0, 0, 0)
	if none != base {
		t.Fatalf("三项都是 0 不应加括号: %q", none)
	}
}
