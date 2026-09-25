package picker

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestViewShowsCheckAndUnselectableReason(t *testing.T) {
	model := New(sessionFixture())
	model.checked[0] = true
	view := model.View()
	if !strings.Contains(view, "a.yaml") || !strings.Contains(view, "✓") {
		t.Fatalf("合格配置未显示勾选:\n%s", view)
	}
	if !strings.Contains(view, "bad.yaml") || !strings.Contains(view, "不是合法的 yaml") {
		t.Fatalf("不可选配置未显示原因:\n%s", view)
	}
}

func TestSpaceAndClickToggleOnlySelectableConfigs(t *testing.T) {
	model := New(sessionFixture())
	model.cursor = 0

	toggled, _ := model.Update(tea.KeyMsg{Type: tea.KeySpace})
	if !toggled.(Model).checked[0] {
		t.Fatal("space did not check the first config")
	}

	clicked, _ := toggled.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, Y: 0})
	if clicked.(Model).checked[0] {
		t.Fatal("click did not uncheck the first config")
	}

	skipped, _ := clicked.Update(tea.KeyMsg{Type: tea.KeyDown})
	skipped, _ = skipped.Update(tea.KeyMsg{Type: tea.KeySpace})
	if skipped.(Model).checked[1] {
		t.Fatal("unselectable config was checked")
	}
}

func TestTypedSubscriptionStartsWithoutCheckedConfig(t *testing.T) {
	model := New(sessionFixture())
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	updated, _ = updated.Update(tea.KeyMsg{Type: tea.KeyDown})
	updated, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("https://example.com/a")})
	updated, _ = updated.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(Model)
	if !got.started {
		t.Fatalf("填了地址却没开始: %q", got.status)
	}
	if !strings.Contains(got.View(), "https://example.com/a") {
		t.Fatalf("地址没有出现在画面:\n%s", got.View())
	}
}

func TestEnterWithoutSourceStaysAndExplains(t *testing.T) {
	model := New(sessionFixture())
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(Model)
	if got.started {
		t.Fatal("started without a source")
	}
	if got.status == "" {
		t.Fatal("missing status")
	}
}

func TestEnterWithBadFilterStaysOnThatField(t *testing.T) {
	model := New(sessionFixture())
	model.checked[0] = true
	model.options.Filter = "("
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(Model)
	if got.started {
		t.Fatal("started with an invalid filter")
	}
	if !strings.Contains(got.status, "过滤") {
		t.Fatalf("status = %q", got.status)
	}
}

func TestFetchingQuitCancelsAndLeaves(t *testing.T) {
	model := New(sessionFixture())
	model.fetching = true
	keys := []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune{'q'}},
		{Type: tea.KeyEsc},
		{Type: tea.KeyCtrlC},
	}
	for _, key := range keys {
		updated, cmd := model.Update(key)
		got := updated.(Model)
		if !got.quitting || cmd == nil {
			t.Fatalf("key %v did not leave: quitting=%v cmd=%v", key, got.quitting, cmd)
		}
	}
}

func sessionFixture() Session {
	return Session{
		Configs: []ConfigEntry{
			{Name: "a.yaml", Path: "/opt/a.yaml", Selectable: true},
			{Name: "bad.yaml", Path: "/opt/bad.yaml", Reason: "不是合法的 yaml"},
		},
	}
}
