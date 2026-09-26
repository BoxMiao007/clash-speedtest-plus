package tui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/faceair/clash-speedtest/speedtester"
)

// inRepoTempDir 在工作目录内建临时目录，避免 safeImageDir 拒绝逃逸路径。
func inRepoTempDir(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(cwd, ".quit-test-images")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// TestQuitAfterFinalAutoSaveDoesNotResave 整轮结束自动保存完成后按 q，
// 应直接退出而不是再生成一张结果图。
func TestQuitAfterFinalAutoSaveDoesNotResave(t *testing.T) {
	dir := inRepoTempDir(t)
	resultChannel := make(chan *speedtester.Result, 10)
	model := NewTUIModel(speedtester.SpeedModeDownload, 1, resultChannel)
	model.SetImageExport(dir, true)
	saveCount := 0
	model.SetConfigSaver(func([]*speedtester.Result) (string, error) {
		saveCount++
		return "clash-speedtest.yaml", nil
	})
	model.results = append(model.results, &speedtester.Result{
		ProxyName: "香港 01",
		ProxyType: "SS",
		Latency:   100 * time.Millisecond,
	})
	model.updateTableRows()

	// 整轮结束触发自动保存。
	updated, cmd := model.Update(doneMsg{})
	m := updated.(tuiModel)
	if cmd == nil {
		t.Fatal("doneMsg 应触发自动保存")
	}
	msg := cmd()
	updated, _ = m.Update(msg)
	m = updated.(tuiModel)
	if !m.autoImageDone || m.savingImage {
		t.Fatalf("自动保存应已完成: autoImageDone=%v savingImage=%v", m.autoImageDone, m.savingImage)
	}
	if saveCount != 1 {
		t.Fatalf("自动保存应写一次配置: %d", saveCount)
	}

	// 按 q 应直接退出，不再生成第二张图。
	updated, quitCmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	m = updated.(tuiModel)
	if !m.quitting {
		t.Fatal("按 q 后应进入退出状态")
	}
	if quitCmd == nil {
		t.Fatal("按 q 应返回退出命令")
	}
	if _, ok := quitCmd().(tea.QuitMsg); !ok {
		t.Fatalf("按 q 应直接退出而非再次保存: %T", quitCmd())
	}
	if saveCount != 1 {
		t.Fatalf("按 q 不应再次保存: %d", saveCount)
	}
}

// TestQuitBeforeAutoSaveStillSaves 整轮结束但尚未自动保存时按 q，
// 仍应完成一次保存再退出（防止直接退出丢产物）。
func TestQuitBeforeAutoSaveStillSaves(t *testing.T) {
	dir := inRepoTempDir(t)
	resultChannel := make(chan *speedtester.Result, 10)
	model := NewTUIModel(speedtester.SpeedModeDownload, 1, resultChannel)
	model.SetImageExport(dir, false)
	saveCount := 0
	model.SetConfigSaver(func([]*speedtester.Result) (string, error) {
		saveCount++
		return "clash-speedtest.yaml", nil
	})
	model.testing = false
	model.autoImageDone = false
	model.results = append(model.results, &speedtester.Result{
		ProxyName: "香港 01",
		ProxyType: "SS",
		Latency:   100 * time.Millisecond,
	})
	model.updateTableRows()

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	m := updated.(tuiModel)
	if cmd == nil {
		t.Fatal("尚未自动保存时按 q 应触发保存")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); ok {
		t.Fatal("尚未保存时按 q 不应直接退出")
	}
	updated, _ = m.Update(msg)
	m = updated.(tuiModel)
	if saveCount != 1 {
		t.Fatalf("应写入一次配置: %d", saveCount)
	}
	if !m.quitting {
		t.Fatal("保存完成后应退出")
	}
}

// Esc 返回上一级：仅在启用时生效，详情面板打开时仍只关面板。
func TestEscReturnsToParentWhenEnabled(t *testing.T) {
	model := NewTUIModel(speedtester.SpeedModeDownload, 1, make(chan *speedtester.Result, 1))
	model.SetEscapeToParent(true)
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	got := updated.(tuiModel)
	if !got.EscapedToParent() || cmd == nil {
		t.Fatalf("Esc 应中断并返回上一级: escaped=%v cmd=%v", got.EscapedToParent(), cmd)
	}

	withDetail := NewTUIModel(speedtester.SpeedModeDownload, 1, make(chan *speedtester.Result, 1))
	withDetail.SetEscapeToParent(true)
	withDetail.detailVisible = true
	updated, cmd = withDetail.Update(tea.KeyMsg{Type: tea.KeyEsc})
	got = updated.(tuiModel)
	if got.EscapedToParent() || got.detailVisible {
		t.Fatalf("详情开着时 Esc 应只关详情: escaped=%v detail=%v", got.EscapedToParent(), got.detailVisible)
	}

	plain := NewTUIModel(speedtester.SpeedModeDownload, 1, make(chan *speedtester.Result, 1))
	updated, _ = plain.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if got := updated.(tuiModel); got.EscapedToParent() {
		t.Fatal("未启用时 Esc 不应返回上一级")
	}
}
