package tui

import (
	"strings"
	"testing"

	"github.com/faceair/clash-speedtest/speedtester"
)

func TestImageSpeedOnlyDropsZeroSpeedInFlight(t *testing.T) {
	model := testingModel(t, 2)
	model.currentProxy = 2
	model.totalProxies = 4
	model.results[0].ProxyName = "有速度"
	model.results[0].DownloadSpeed = 3 * 1024 * 1024
	model.results[1].ProxyName = "无速度"
	model.inFlight["inflight"].latest = speedtester.Progress{Phase: speedtester.PhaseDownload}
	model.inFlight["moving"] = &inFlightNode{
		name:      "moving",
		proxyType: "SS",
		latest: speedtester.Progress{
			Phase:         speedtester.PhaseDownload,
			DownloadSpeed: 1024 * 1024,
		},
	}
	model.inFlightOrder = []string{"inflight", "moving"}
	model.SetImageSpeedOnly(true)

	spec := model.imageSpec(false)
	if len(spec.Rows) != 2 {
		t.Fatalf("应留下有速度的完成行和有瞬时速度的在测行，实际 %d 行", len(spec.Rows))
	}
	if spec.Rows[0].InFlight || spec.Rows[0].Cells[0] != "1." || spec.Rows[0].Cells[1] != "有速度" {
		t.Fatalf("第一行应是 1. 有速度: %#v inFlight=%v", spec.Rows[0].Cells, spec.Rows[0].InFlight)
	}
	if !spec.Rows[1].InFlight || spec.Rows[1].Cells[0] != "2." || spec.Rows[1].Cells[1] != "moving" {
		t.Fatalf("第二行应是灰色的 2. moving: %#v inFlight=%v", spec.Rows[1].Cells, spec.Rows[1].InFlight)
	}
	if !strings.HasSuffix(spec.Summary, "2/4（无效 1，测试中 1）") {
		t.Fatalf("摘要应带无效和测试中: %q", spec.Summary)
	}
}
