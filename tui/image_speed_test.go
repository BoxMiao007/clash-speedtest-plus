package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/speedtester"
)

func TestFastModeIgnoresImageSpeedOnly(t *testing.T) {
	model := NewTUIModel(speedtester.SpeedModeFast, 2, make(chan *speedtester.Result, 1))
	model.results = []*speedtester.Result{
		{ProxyName: "快速节点", ProxyType: "SS", Latency: 80 * time.Millisecond},
	}
	model.currentProxy = 1
	model.SetImageSpeedOnly(true)
	model.NoteFastImageSpeedIgnored()

	if model.statusText != "快速模式没有速度，已忽略 -image-speed-only" {
		t.Fatalf("启动应提示已忽略: %q", model.statusText)
	}
	spec := model.imageSpec(true)
	if len(spec.Rows) != 1 || spec.Rows[0].Cells[1] != "快速节点" {
		t.Fatalf("快速模式结果图应保持全量: %#v", spec.Rows)
	}
	if strings.Contains(spec.Summary, "无效") || strings.Contains(spec.Summary, "测试中") {
		t.Fatalf("忽略开关后摘要不应加括号: %q", spec.Summary)
	}
}

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
	if !strings.HasSuffix(spec.Summary, "2/4（有效 2，无效 1，测试中 1）") {
		t.Fatalf("摘要应带有效、无效和测试中: %q", spec.Summary)
	}
}
