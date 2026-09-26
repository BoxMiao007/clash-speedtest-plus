package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/faceair/clash-speedtest/speedtester"
)

func TestInFlightRowMatchesDownloadColumns(t *testing.T) {
	resultChannel := make(chan *speedtester.Result, 1)
	model := NewTUIModel(speedtester.SpeedModeDownload, 1, resultChannel)
	model.windowWidth = 120
	model.windowHeight = 30
	model.updateTableLayout()
	model.applyProgress(speedtester.Progress{
		Name:  "日本712ms",
		Type:  "Vmess",
		Phase: speedtester.PhaseLatency,
	})

	if got, want := len(model.table.Rows()[0]), len(model.table.Columns()); got != want {
		t.Fatalf("在测行列数 = %d，表头列数 = %d", got, want)
	}
	view := model.table.View()
	if !strings.Contains(view, "日本712ms") {
		t.Fatalf("表格未渲染在测节点: %q", view)
	}
}

// TestTUIModelUpdateTableRows tests the updateTableRows function
func TestTUIModelUpdateTableRows(t *testing.T) {
	resultChannel := make(chan *speedtester.Result, 10)
	model := NewTUIModel(speedtester.SpeedModeFull, 2, resultChannel)

	// Add a result
	result := &speedtester.Result{
		ProxyName:     "Test Proxy",
		ProxyType:     "SS",
		Latency:       100 * time.Millisecond,
		Jitter:        50 * time.Millisecond,
		PacketLoss:    5.0,
		DownloadSpeed: 10 * 1024 * 1024,
		UploadSpeed:   5 * 1024 * 1024,
		ProxyConfig:   map[string]any{},
	}

	model.results = append(model.results, result)
	model.updateTableRows()

	// Verify table has one row
	rows := model.table.Rows()
	if len(rows) != 1 {
		t.Errorf("Expected 1 row, got %d", len(rows))
	}
	if len(rows[0]) != 8 {
		t.Errorf("Expected 8 columns in normal mode, got %d", len(rows[0]))
	}
}

// TestTUIModelUpdateTableRowsFastMode tests the updateTableRows function in fast mode
func TestTUIModelUpdateTableRowsFastMode(t *testing.T) {
	resultChannel := make(chan *speedtester.Result, 10)
	model := NewTUIModel(speedtester.SpeedModeFast, 2, resultChannel)

	// Add a result
	result := &speedtester.Result{
		ProxyName:   "Test Proxy",
		ProxyType:   "SS",
		Latency:     100 * time.Millisecond,
		ProxyConfig: map[string]any{},
	}

	model.results = append(model.results, result)
	model.updateTableRows()

	// Verify table has one row
	rows := model.table.Rows()
	if len(rows) != 1 {
		t.Errorf("Expected 1 row, got %d", len(rows))
	}
	if len(rows[0]) != 4 {
		t.Errorf("Expected 4 columns in fast mode, got %d", len(rows[0]))
	}
}

func TestCalculateColumnWidthsFitsWindow(t *testing.T) {
	width := 100
	widths := calculateColumnWidths(width, speedtester.SpeedModeFull)
	if len(widths) != 8 {
		t.Fatalf("expected 8 columns, got %d", len(widths))
	}
	total := 0
	for _, value := range widths {
		total += value
	}
	padding := 2 * len(widths)
	if total+padding > width {
		t.Fatalf("expected total width to fit window: columns=%d padding=%d window=%d", total, padding, width)
	}
}

func TestCalculateColumnWidthsDownloadOnly(t *testing.T) {
	width := 100
	widths := calculateColumnWidths(width, speedtester.SpeedModeDownload)
	if len(widths) != 7 {
		t.Fatalf("expected 7 columns, got %d", len(widths))
	}
	total := 0
	for _, value := range widths {
		total += value
	}
	padding := 2 * len(widths)
	if total+padding > width {
		t.Fatalf("expected total width to fit window: columns=%d padding=%d window=%d", total, padding, width)
	}
}
