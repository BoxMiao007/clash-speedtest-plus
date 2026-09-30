package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/BoxMiao007/clash-speedtest-plus/speedtester"
	"github.com/charmbracelet/lipgloss"
)

func TestHelpViewShowsQuitAndDetailKeys(t *testing.T) {
	resultChannel := make(chan *speedtester.Result, 1)
	model := NewTUIModel(speedtester.SpeedModeDownload, 1, resultChannel)
	model.windowWidth = 80
	model.windowHeight = 20

	result := &speedtester.Result{
		ProxyName: "Proxy",
		ProxyType: "SS",
		Latency:   120 * time.Millisecond,
	}
	model.results = []*speedtester.Result{result}
	model.updateTableRows()
	model.updateTableLayout()

	helpView := model.help.view()
	if !strings.Contains(helpView, "q/ctrl+c") {
		t.Fatalf("expected help to include quit shortcut, got %q", helpView)
	}
	if strings.Contains(helpView, "esc") {
		t.Fatalf("expected help to hide detail shortcut when detail is closed, got %q", helpView)
	}

	model.toggleDetail(result)
	helpView = model.help.view()
	if !strings.Contains(helpView, "esc") {
		t.Fatalf("expected help to include detail shortcut when detail is visible, got %q", helpView)
	}
}

func lastNonEmptyLine(s string) string {
	lines := strings.Split(s, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(stripANSI(lines[i])) != "" {
			return lines[i]
		}
	}
	return ""
}

func TestHelpViewShowsVersionBadgeAtRight(t *testing.T) {
	resultChannel := make(chan *speedtester.Result, 1)
	model := NewTUIModel(speedtester.SpeedModeDownload, 0, resultChannel)
	model.SetVersion("v2.5.0")
	model.windowWidth = 100
	model.windowHeight = 20
	model.updateTableLayout()

	last := lastNonEmptyLine(model.View())
	plain := strings.TrimRight(stripANSI(last), " ")
	if !strings.HasSuffix(plain, "v2.5.0") {
		t.Fatalf("帮助条右端应是 v2.5.0: %q", plain)
	}
	if !strings.Contains(plain, "q/ctrl+c") {
		t.Fatalf("角标不应挤掉键位帮助: %q", plain)
	}
	if w := lipgloss.Width(stripANSI(last)); w != 100 {
		t.Fatalf("角标应贴右缘，行宽 = %d: %q", w, last)
	}
}

func TestHelpViewHidesVersionBadgeWhenTooNarrow(t *testing.T) {
	resultChannel := make(chan *speedtester.Result, 1)
	model := NewTUIModel(speedtester.SpeedModeDownload, 0, resultChannel)
	model.SetVersion("v2.5.0")
	model.windowWidth = 40
	model.windowHeight = 20
	model.updateTableLayout()

	if view := stripANSI(model.View()); strings.Contains(view, "v2.5.0") {
		t.Fatalf("窄终端应藏角标: %q", view)
	}
}
