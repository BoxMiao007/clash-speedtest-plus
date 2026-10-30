package gui

import (
	"testing"
	"time"

	"github.com/BoxMiao007/clash-speedtest-plus/internal/export"
	"github.com/BoxMiao007/clash-speedtest-plus/internal/speedtester"
)

// 提前结束只数过筛结果；限额关闭时永不停止。
func TestEarlyStopCounterCountsOnlyAcceptedResults(t *testing.T) {
	counter := &earlyStopCounter{
		limit: 2,
		filter: export.Filter{
			MinDownloadBps: 10 * megabyte,
			DownloadTested: true,
			MaxPacketLoss:  100,
		},
	}
	slow := &speedtester.Result{ProxyConfig: map[string]any{"name": "a", "server": "127.0.0.1"}, Latency: 20 * time.Millisecond, DownloadSpeed: 1 * megabyte}
	fast := &speedtester.Result{ProxyConfig: map[string]any{"name": "b", "server": "127.0.0.1"}, Latency: 20 * time.Millisecond, DownloadSpeed: 20 * megabyte}
	if !counter.accept(slow) {
		t.Fatal("未过筛的结果不应计数")
	}
	if !counter.accept(fast) {
		t.Fatal("第 1 个过筛结果不应触发停止")
	}
	if counter.accept(fast) {
		t.Fatal("第 2 个过筛结果应触发停止")
	}

	disabled := &earlyStopCounter{limit: 0}
	for i := 0; i < 5; i++ {
		if !disabled.accept(fast) {
			t.Fatal("限额为 0 时不应停止")
		}
	}
}
