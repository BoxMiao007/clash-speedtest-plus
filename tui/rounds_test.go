package tui

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/BoxMiao007/clash-speedtest-plus/speedtester"
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

// 还有后续轮时（autoAdvance），本轮测完、自动保存完成后不再自行退出，
// 而是标记推进，由队列壳开下一轮的视图（ADR-0015）。
func TestAutoAdvanceRequestsAdvanceAfterFinalSave(t *testing.T) {
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
	updated, _ = m.Update(msg)
	m = updated.(tuiModel)
	if m.quitting {
		t.Fatal("自动推进轮保存完不应自行退出，由队列壳推进")
	}
	if !m.AdvanceRequested() {
		t.Fatal("保存完成后应标记推进下一轮")
	}
}

// 「产物跟随文件名」开着时，测完自动保存的图名带基名前缀；
// 订阅轮不设基名，图名保持默认。
func TestAutoSaveImageNameFollowsBase(t *testing.T) {
	dir := inRepoTempDir(t)
	resultChannel := make(chan *speedtester.Result, 10)
	model := NewTUIModel(speedtester.SpeedModeDownload, 1, resultChannel)
	model.SetImageExport(dir, true)
	model.SetImageNameBase("机场A")
	model.results = append(model.results, &speedtester.Result{
		ProxyName: "香港 01",
		ProxyType: "SS",
		Latency:   100 * time.Millisecond,
	})
	model.updateTableRows()

	updated, cmd := model.Update(doneMsg{})
	_ = updated.(tuiModel)
	msg := cmd()
	saved, ok := msg.(imageSavedMsg)
	if !ok {
		t.Fatalf("应完成自动保存: %#v", msg)
	}
	if !strings.Contains(saved.text, "已保存 "+filepath.Join(dir, "机场A-")) {
		t.Fatalf("图名应带基名前缀: %q", saved.text)
	}
}

// 还没有下一轮时，即便关了自动图、没填输出路径（无产物可写），
// 本轮测完也要标记推进，由队列壳开下一轮的视图，而不是停在界面等按键。
func TestAutoAdvanceRequestsAdvanceWithoutArtifacts(t *testing.T) {
	resultChannel := make(chan *speedtester.Result, 10)
	model := NewTUIModel(speedtester.SpeedModeDownload, 1, resultChannel)
	model.SetImageExport(t.TempDir(), false) // 关自动结果图，也不设 configSaver
	model.SetAutoAdvance(true)
	model.results = append(model.results, &speedtester.Result{
		ProxyName: "香港 01",
		ProxyType: "SS",
		Latency:   100 * time.Millisecond,
	})
	model.updateTableRows()

	updated, _ := model.Update(doneMsg{})
	m := updated.(tuiModel)
	if m.quitting {
		t.Fatal("无产物的自动推进轮测完不应自行退出")
	}
	if !m.AdvanceRequested() {
		t.Fatal("无产物的自动推进轮测完应标记推进下一轮")
	}
}

// 产物序号加在图名最前，手动 s 与自动导出同一套命名。
func TestAutoSaveImageNameCarriesSequence(t *testing.T) {
	dir := inRepoTempDir(t)
	resultChannel := make(chan *speedtester.Result, 10)
	model := NewTUIModel(speedtester.SpeedModeDownload, 1, resultChannel)
	model.SetImageExport(dir, true)
	model.SetImageNameBase("机场A")
	model.SetImageSeq(7)
	model.results = append(model.results, &speedtester.Result{
		ProxyName: "香港 01",
		ProxyType: "SS",
		Latency:   100 * time.Millisecond,
	})
	model.updateTableRows()

	updated, cmd := model.Update(doneMsg{})
	_ = updated.(tuiModel)
	msg := cmd()
	saved, ok := msg.(imageSavedMsg)
	if !ok {
		t.Fatalf("应完成自动保存: %#v", msg)
	}
	if !strings.Contains(saved.text, "7.机场A-") {
		t.Fatalf("图名应带序号前缀: %q", saved.text)
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
