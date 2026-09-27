package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/faceair/clash-speedtest/speedtester"
)

// 多源分别测速时，进度行最前面画「第 X/N 轮 源名」；单源不画。
func TestRoundLabelPrefixesProgressLine(t *testing.T) {
	resultChannel := make(chan *speedtester.Result, 10)
	model := NewTUIModel(speedtester.SpeedModeDownload, 1, resultChannel)
	model.SetRoundLabel("第 2/3 轮 机场B")
	model.testing = true
	if got := model.progressLine(); got[:len("第 2/3 轮 机场B · ")] != "第 2/3 轮 机场B · " {
		t.Fatalf("进度行应以轮次开头: %q", got)
	}

	single := NewTUIModel(speedtester.SpeedModeDownload, 1, resultChannel)
	single.testing = true
	if got := single.progressLine(); got[:len("测试中")] != "测试中" {
		t.Fatalf("单源进度行不应有轮次前缀: %q", got)
	}
}

// 还有后续轮时（autoAdvance），本轮测完、自动保存完成后应直接退出进下一轮，
// 而不是停在界面等退出键。
func TestAutoAdvanceQuitsAfterFinalSave(t *testing.T) {
	dir := inRepoTempDir(t)
	resultChannel := make(chan *speedtester.Result, 10)
	model := NewTUIModel(speedtester.SpeedModeDownload, 1, resultChannel)
	model.SetImageExport(dir, true)
	model.SetAutoAdvance(true)
	model.results = append(model.results, &speedtester.Result{
		ProxyName: "香港 01",
		ProxyType: "SS",
		Latency:   100 * time.Millisecond,
	})
	model.updateTableRows()

	updated, cmd := model.Update(doneMsg{})
	m := updated.(tuiModel)
	if cmd == nil {
		t.Fatal("doneMsg 应触发自动保存")
	}
	msg := cmd()
	if saved, ok := msg.(imageSavedMsg); !ok || !saved.quit {
		t.Fatalf("自动推进轮的保存应带退出标志: %#v", msg)
	}
	updated, quitCmd := m.Update(msg)
	m = updated.(tuiModel)
	if !m.quitting {
		t.Fatal("保存完成后应自动进入退出状态")
	}
	if _, ok := quitCmd().(tea.QuitMsg); !ok {
		t.Fatalf("保存完成后应自动退出: %T", quitCmd())
	}
}

// 最后一轮（autoAdvance=false）测完保存后不自动退出，停界面等退出键。
func TestLastRoundStaysAfterFinalSave(t *testing.T) {
	dir := inRepoTempDir(t)
	resultChannel := make(chan *speedtester.Result, 10)
	model := NewTUIModel(speedtester.SpeedModeDownload, 1, resultChannel)
	model.SetImageExport(dir, true)
	model.results = append(model.results, &speedtester.Result{
		ProxyName: "香港 01",
		ProxyType: "SS",
		Latency:   100 * time.Millisecond,
	})
	model.updateTableRows()

	updated, cmd := model.Update(doneMsg{})
	m := updated.(tuiModel)
	if cmd == nil {
		t.Fatal("doneMsg 应触发自动保存")
	}
	msg := cmd()
	if saved, ok := msg.(imageSavedMsg); ok && saved.quit {
		t.Fatal("最后一轮的保存不应带退出标志")
	}
	updated, _ = m.Update(msg)
	m = updated.(tuiModel)
	if m.quitting {
		t.Fatal("最后一轮保存后应停在界面")
	}
}
