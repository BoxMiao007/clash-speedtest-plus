package gui

import (
	"testing"

	"github.com/BoxMiao007/clash-speedtest-plus/internal/store"
)

// TestThemeModeParseAndKeys 校验设置里的字符串、下标与模式三者能互相还原：
// 设置文件读进来、分段控件选中的下标、落盘时写回的字符串都走这一组转换。
func TestThemeModeParseAndKeys(t *testing.T) {
	cases := []struct {
		key  string
		mode themeMode
	}{
		{"dark", themeDark},
		{"light", themeLight},
		{"system", themeSystem},
		{"", themeDark},
		{"neon", themeDark},
	}
	for _, item := range cases {
		if got := parseThemeMode(item.key); got != item.mode {
			t.Fatalf("parseThemeMode(%q) = %v，期望 %v", item.key, got, item.mode)
		}
	}
	for index, key := range themeModeKeys {
		mode := themeModeAt(index)
		if mode.key() != key {
			t.Fatalf("themeModeAt(%d).key() = %q，期望 %q", index, mode.key(), key)
		}
		if mode.index() != index {
			t.Fatalf("themeModeAt(%d).index() = %d", index, mode.index())
		}
	}
}

// TestThemeSwitchFromHeaderPersists 验证标题栏切配色：界面立即换 token，
// 并写进 data/settings.json，下次启动沿用。
func TestThemeSwitchFromHeaderPersists(t *testing.T) {
	dir := t.TempDir()
	application, driver := openTestApp(t, dir, newFakeRunner(nil))
	defer application.Close()
	if application.themeMode != themeDark {
		t.Fatalf("默认配色应为深色，实际 %v", application.themeMode)
	}
	clickLabel(t, driver, "浅色")
	if application.themeMode != themeLight {
		t.Fatalf("点「浅色」后应为浅色，实际 %v", application.themeMode)
	}
	if application.theme.Background != lightTheme().Background {
		t.Fatalf("切到浅色后背景 token 没换：%+v", application.theme.Background)
	}
	if application.palette != lightPalette() {
		t.Fatalf("切到浅色后 palette 没换：%+v", application.palette)
	}
	database, err := store.Open(store.Options{BaseDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if saved := database.Settings().Theme; saved != store.ThemeLight {
		t.Fatalf("设置里的 theme = %q，期望 %q", saved, store.ThemeLight)
	}
}

// TestSavedThemeModeAppliesOnStartup 验证重启后沿用上次选的配色。
func TestSavedThemeModeAppliesOnStartup(t *testing.T) {
	dir := t.TempDir()
	database, err := store.Open(store.Options{BaseDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	settings := database.Settings()
	settings.Theme = store.ThemeLight
	if err := database.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	application, err := New(Options{BaseDir: dir, Version: "test", Runner: newFakeRunner(nil)})
	if err != nil {
		t.Fatal(err)
	}
	defer application.Close()
	if application.themeMode != themeLight {
		t.Fatalf("启动后应沿用浅色，实际 %v", application.themeMode)
	}
}

// TestSavingSettingsKeepsTheme 验证「保存设置」不会把配色清掉：设置表单不
// 收集主题，但 collectSettings 必须把它带回去。
func TestSavingSettingsKeepsTheme(t *testing.T) {
	dir := t.TempDir()
	application, driver := openTestApp(t, dir, newFakeRunner(nil))
	defer application.Close()
	clickLabel(t, driver, "浅色")
	clickLabel(t, driver, "设置")
	clickLabel(t, driver, "保存设置")
	if application.themeMode != themeLight {
		t.Fatalf("保存设置后配色被改成了 %v", application.themeMode)
	}
	database, err := store.Open(store.Options{BaseDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if saved := database.Settings().Theme; saved != store.ThemeLight {
		t.Fatalf("保存设置后 theme = %q，期望 %q", saved, store.ThemeLight)
	}
}
