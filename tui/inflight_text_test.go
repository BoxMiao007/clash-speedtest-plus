package tui

import (
	"strings"
	"testing"

	"github.com/faceair/clash-speedtest/speedtester"
)

// 在测行的类型和延迟必须完整出现在表格里，不能被样式转义码挤掉。
func TestInFlightTypeAndLatencyAreFullyVisible(t *testing.T) {
	model := testingModel(t, 1)
	model.inFlight["inflight"] = &inFlightNode{
		name:      "inflight",
		proxyType: "Trojan",
		latest:    speedtester.Progress{Name: "inflight", Type: "Trojan", Latency: 123 * 1e6},
	}
	model.updateTableRows()

	view := model.table.View()
	if !strings.Contains(view, "Trojan") {
		t.Fatalf("在测行类型没有显示全:\n%s", view)
	}
	if !strings.Contains(view, "测试中") && !strings.Contains(view, "123ms") {
		t.Fatalf("在测行延迟没有显示全:\n%s", view)
	}
}

func TestImagePutsInFlightBelowFinished(t *testing.T) {
	model := testingModel(t, 1)
	model.results[0].ProxyName = "已完成节点"
	spec := model.imageSpec(false)
	if len(spec.Rows) < 2 {
		t.Fatalf("行数不足: %d", len(spec.Rows))
	}
	if spec.Rows[0].InFlight || spec.Rows[0].Cells[1] != "已完成节点" {
		t.Fatalf("完成行应在上面: %+v", spec.Rows[0])
	}
	if !spec.Rows[len(spec.Rows)-1].InFlight {
		t.Fatalf("在测行应在下面: %+v", spec.Rows[len(spec.Rows)-1])
	}
}
