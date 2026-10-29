package store_test

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/BoxMiao007/clash-speedtest-plus/internal/store"
)

func openTempStore(t *testing.T) (*store.Store, string) {
	t.Helper()
	base := t.TempDir()
	st, err := store.Open(store.Options{BaseDir: base})
	if err != nil {
		t.Fatalf("Open 失败：%v", err)
	}
	return st, base
}

func readVersion(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取 %s 失败：%v", path, err)
	}
	var envelope struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatalf("%s 不是有效 JSON：%v", path, err)
	}
	return envelope.Version
}

func assertResult(t *testing.T, got, want store.NodeResult) {
	t.Helper()
	if got.LatencyMS != want.LatencyMS ||
		got.JitterMS != want.JitterMS ||
		got.PacketLoss != want.PacketLoss ||
		got.DownloadBps != want.DownloadBps ||
		got.UploadBps != want.UploadBps ||
		got.DownloadError != want.DownloadError ||
		got.UploadError != want.UploadError ||
		got.OK != want.OK {
		t.Errorf("结果字段不一致：got %+v，want %+v", got, want)
	}
	if !got.TestedAt.Equal(want.TestedAt) {
		t.Errorf("TestedAt = %v，期望 %v", got.TestedAt, want.TestedAt)
	}
}

func TestOpenCreatesDataDir(t *testing.T) {
	base := t.TempDir()
	st, err := store.Open(store.Options{BaseDir: base})
	if err != nil {
		t.Fatalf("Open 失败：%v", err)
	}
	if st.BaseDir() != base {
		t.Errorf("BaseDir() = %q，期望 %q", st.BaseDir(), base)
	}
	wantDataDir := filepath.Join(base, "data")
	if st.DataDir() != wantDataDir {
		t.Errorf("DataDir() = %q，期望 %q", st.DataDir(), wantDataDir)
	}
	info, err := os.Stat(wantDataDir)
	if err != nil {
		t.Fatalf("数据目录未创建：%v", err)
	}
	if !info.IsDir() {
		t.Fatalf("数据目录 %q 不是目录", wantDataDir)
	}

	// 全新数据目录：四项均为空值，不报错。
	if got := st.Settings(); got != (store.Settings{}) {
		t.Errorf("Settings() = %+v，期望零值", got)
	}
	if got := st.Entries(); len(got) != 0 {
		t.Errorf("Entries() = %+v，期望为空", got)
	}
	if got := st.Presets(); len(got) != 0 {
		t.Errorf("Presets() = %+v，期望为空", got)
	}
	if _, ok := st.Result("a", "b"); ok {
		t.Error("空数据目录里 Result 不应命中")
	}
}

func TestOpenExplicitDataDir(t *testing.T) {
	base := t.TempDir()
	dataDir := filepath.Join(t.TempDir(), "自定义数据")
	st, err := store.Open(store.Options{BaseDir: base, DataDir: dataDir})
	if err != nil {
		t.Fatalf("Open 失败：%v", err)
	}
	if st.DataDir() != dataDir {
		t.Errorf("DataDir() = %q，期望 %q", st.DataDir(), dataDir)
	}
	if _, err := os.Stat(dataDir); err != nil {
		t.Fatalf("注入的数据目录未创建：%v", err)
	}
}

func TestOpenWithoutBaseDirFails(t *testing.T) {
	if _, err := store.Open(store.Options{}); err == nil {
		t.Fatal("缺少程序目录时期望 Open 返回错误，实际为 nil")
	}
}

func TestPersistenceRoundTrip(t *testing.T) {
	st, base := openTempStore(t)

	settings := store.Settings{FilterMode: "advanced", FilterExpression: "^HK-", FilterCombine: "or"}
	if err := st.SetSettings(settings); err != nil {
		t.Fatalf("SetSettings 失败：%v", err)
	}

	entryA := store.Entry{ID: store.NewID(), Name: "订阅 A", Path: "test/a.yaml"}
	entryB := store.Entry{ID: store.NewID(), Name: "订阅 B", Path: filepath.Join(t.TempDir(), "b.yaml")}
	if err := st.PutEntry(entryA); err != nil {
		t.Fatalf("PutEntry 失败：%v", err)
	}
	if err := st.PutEntry(entryB); err != nil {
		t.Fatalf("PutEntry 失败：%v", err)
	}

	preset := store.Preset{Name: "香港", Mode: "simple", Expression: "香港 !试用", Combine: "and"}
	if err := st.PutPreset(preset); err != nil {
		t.Fatalf("PutPreset 失败：%v", err)
	}

	testedAt := time.Date(2026, 10, 29, 12, 30, 0, 0, time.UTC)
	result := store.NodeResult{
		LatencyMS:     12.5,
		JitterMS:      1.5,
		PacketLoss:    0.25,
		DownloadBps:   1024 * 1024,
		UploadBps:     512 * 1024,
		DownloadError: "",
		UploadError:   "上传超时",
		TestedAt:      testedAt,
		OK:            true,
	}
	if err := st.PutResult(entryA.ID, "node-1", result); err != nil {
		t.Fatalf("PutResult 失败：%v", err)
	}

	dataDir := filepath.Join(base, "data")
	for _, name := range []string{"settings.json", "entries.json", "presets.json", "results.json"} {
		path := filepath.Join(dataDir, name)
		if got := readVersion(t, path); got != 1 {
			t.Errorf("%s 的 version = %d，期望 1", name, got)
		}
	}

	// 模拟重启：重新 Open 同一个程序目录。
	reopened, err := store.Open(store.Options{BaseDir: base})
	if err != nil {
		t.Fatalf("重新 Open 失败：%v", err)
	}
	if got := reopened.Settings(); got != settings {
		t.Errorf("重启后 Settings() = %+v，期望 %+v", got, settings)
	}
	if got := reopened.Entries(); !slices.Equal(got, []store.Entry{entryA, entryB}) {
		t.Errorf("重启后 Entries() = %+v，期望 %+v", got, []store.Entry{entryA, entryB})
	}
	if got := reopened.Presets(); !slices.Equal(got, []store.Preset{preset}) {
		t.Errorf("重启后 Presets() = %+v，期望 %+v", got, []store.Preset{preset})
	}
	got, ok := reopened.Result(entryA.ID, "node-1")
	if !ok {
		t.Fatal("重启后未读到节点结果")
	}
	assertResult(t, got, result)
	if _, ok := reopened.Result(entryA.ID, "不存在"); ok {
		t.Error("不存在的节点不应命中")
	}
}

func TestEntriesAndPresetsAreCopies(t *testing.T) {
	st, _ := openTempStore(t)
	entry := store.Entry{ID: "id-1", Name: "原始", Path: "a.yaml"}
	if err := st.PutEntry(entry); err != nil {
		t.Fatalf("PutEntry 失败：%v", err)
	}
	preset := store.Preset{Name: "预设", Mode: "simple", Expression: "香港", Combine: "and"}
	if err := st.PutPreset(preset); err != nil {
		t.Fatalf("PutPreset 失败：%v", err)
	}

	entries := st.Entries()
	entries[0].Name = "被外部改坏"
	entries = append(entries, store.Entry{ID: "id-2", Name: "外部追加", Path: "b.yaml"})
	if got := st.Entries(); len(got) != 1 || got[0] != entry {
		t.Errorf("Entries() 未返回副本：%+v", got)
	}

	presets := st.Presets()
	presets[0].Expression = "被外部改坏"
	presets = append(presets, store.Preset{Name: "外部追加"})
	if got := st.Presets(); len(got) != 1 || got[0] != preset {
		t.Errorf("Presets() 未返回副本：%+v", got)
	}
}

func TestPutEntryOverwritesByID(t *testing.T) {
	st, base := openTempStore(t)
	entry := store.Entry{ID: "fixed-id", Name: "旧名称", Path: "old.yaml"}
	if err := st.PutEntry(entry); err != nil {
		t.Fatalf("PutEntry 失败：%v", err)
	}
	entry.Name = "新名称"
	entry.Path = "new.yaml"
	if err := st.PutEntry(entry); err != nil {
		t.Fatalf("PutEntry 覆盖失败：%v", err)
	}
	if got := st.Entries(); !slices.Equal(got, []store.Entry{entry}) {
		t.Errorf("Entries() = %+v，期望按 ID 覆盖为 %+v", got, []store.Entry{entry})
	}

	reopened, err := store.Open(store.Options{BaseDir: base})
	if err != nil {
		t.Fatalf("重新 Open 失败：%v", err)
	}
	if got := reopened.Entries(); !slices.Equal(got, []store.Entry{entry}) {
		t.Errorf("重启后 Entries() = %+v，期望 %+v", got, []store.Entry{entry})
	}
}

func TestDeleteEntryRemovesResults(t *testing.T) {
	st, base := openTempStore(t)
	entryA := store.Entry{ID: "a", Name: "条目 A", Path: "a.yaml"}
	entryB := store.Entry{ID: "b", Name: "条目 B", Path: "b.yaml"}
	for _, entry := range []store.Entry{entryA, entryB} {
		if err := st.PutEntry(entry); err != nil {
			t.Fatalf("PutEntry 失败：%v", err)
		}
	}
	result := store.NodeResult{LatencyMS: 10, OK: true, TestedAt: time.Date(2026, 10, 29, 8, 0, 0, 0, time.UTC)}
	if err := st.PutResult(entryA.ID, "node-1", result); err != nil {
		t.Fatalf("PutResult 失败：%v", err)
	}
	if err := st.PutResult(entryB.ID, "node-1", result); err != nil {
		t.Fatalf("PutResult 失败：%v", err)
	}

	if err := st.DeleteEntry(entryA.ID); err != nil {
		t.Fatalf("DeleteEntry 失败：%v", err)
	}
	if got := st.Entries(); !slices.Equal(got, []store.Entry{entryB}) {
		t.Errorf("删除后 Entries() = %+v，期望只剩 %+v", got, []store.Entry{entryB})
	}
	if _, ok := st.Result(entryA.ID, "node-1"); ok {
		t.Error("DeleteEntry 未清掉该条目的结果")
	}
	if _, ok := st.Result(entryB.ID, "node-1"); !ok {
		t.Error("DeleteEntry 误删了其它条目的结果")
	}

	// 删除不存在条目应当幂等。
	if err := st.DeleteEntry("不存在"); err != nil {
		t.Errorf("DeleteEntry(不存在的 ID) 返回错误：%v", err)
	}

	reopened, err := store.Open(store.Options{BaseDir: base})
	if err != nil {
		t.Fatalf("重新 Open 失败：%v", err)
	}
	if _, ok := reopened.Result(entryA.ID, "node-1"); ok {
		t.Error("重启后被删除条目的结果又出现了")
	}
	if _, ok := reopened.Result(entryB.ID, "node-1"); !ok {
		t.Error("重启后其它条目的结果丢失")
	}
}

func TestPutPresetOverwriteAndDelete(t *testing.T) {
	st, base := openTempStore(t)
	preset := store.Preset{Name: "香港", Mode: "simple", Expression: "香港", Combine: "and"}
	if err := st.PutPreset(preset); err != nil {
		t.Fatalf("PutPreset 失败：%v", err)
	}
	preset.Expression = "香港 !试用"
	preset.Combine = "or"
	if err := st.PutPreset(preset); err != nil {
		t.Fatalf("PutPreset 覆盖失败：%v", err)
	}
	other := store.Preset{Name: "日本", Mode: "advanced", Expression: "^JP-", Combine: "and"}
	if err := st.PutPreset(other); err != nil {
		t.Fatalf("PutPreset 失败：%v", err)
	}
	if got := st.Presets(); !slices.Equal(got, []store.Preset{preset, other}) {
		t.Errorf("Presets() = %+v，期望 %+v", got, []store.Preset{preset, other})
	}

	if err := st.DeletePreset(preset.Name); err != nil {
		t.Fatalf("DeletePreset 失败：%v", err)
	}
	if got := st.Presets(); !slices.Equal(got, []store.Preset{other}) {
		t.Errorf("删除后 Presets() = %+v，期望只剩 %+v", got, []store.Preset{other})
	}
	if err := st.DeletePreset("不存在"); err != nil {
		t.Errorf("DeletePreset(不存在的名称) 返回错误：%v", err)
	}

	reopened, err := store.Open(store.Options{BaseDir: base})
	if err != nil {
		t.Fatalf("重新 Open 失败：%v", err)
	}
	if got := reopened.Presets(); !slices.Equal(got, []store.Preset{other}) {
		t.Errorf("重启后 Presets() = %+v，期望 %+v", got, []store.Preset{other})
	}
}

func TestResultPersistsImmediately(t *testing.T) {
	st, base := openTempStore(t)
	result := store.NodeResult{LatencyMS: 20, DownloadBps: 2048, OK: true, TestedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}
	if err := st.PutResult("entry", "node", result); err != nil {
		t.Fatalf("PutResult 失败：%v", err)
	}
	reopened, err := store.Open(store.Options{BaseDir: base})
	if err != nil {
		t.Fatalf("重新 Open 失败：%v", err)
	}
	got, ok := reopened.Result("entry", "node")
	if !ok {
		t.Fatal("PutResult 后立即重开未读到结果")
	}
	assertResult(t, got, result)

	if err := st.DeleteResults("entry"); err != nil {
		t.Fatalf("DeleteResults 失败：%v", err)
	}
	if _, ok := st.Result("entry", "node"); ok {
		t.Error("DeleteResults 后结果仍存在")
	}
	reopened, err = store.Open(store.Options{BaseDir: base})
	if err != nil {
		t.Fatalf("重新 Open 失败：%v", err)
	}
	if _, ok := reopened.Result("entry", "node"); ok {
		t.Error("DeleteResults 后重启结果又出现了")
	}
}

func TestStorePathSoftRoundTrip(t *testing.T) {
	st, base := openTempStore(t)
	inside := filepath.Join(base, "test", "a.yaml")

	soft := st.StorePath(inside)
	if soft != "test/a.yaml" {
		t.Errorf("StorePath(%q) = %q，期望 %q", inside, soft, "test/a.yaml")
	}
	if strings.Contains(soft, `\`) {
		t.Errorf("软路径必须用 / 分隔，实际为 %q", soft)
	}

	abs, isSoft := st.ResolvePath(soft)
	if !isSoft {
		t.Errorf("ResolvePath(%q) 的 soft = false，期望 true", soft)
	}
	if abs != inside {
		t.Errorf("ResolvePath(%q) = %q，期望 %q", soft, abs, inside)
	}
	if !st.IsInsideBase(inside) {
		t.Errorf("IsInsideBase(%q) = false，期望 true", inside)
	}

	// 带 .. 的绝对路径先 Clean 再判断。
	dirty := filepath.Join(base, "sub", "..", "y.yaml")
	if got := st.StorePath(dirty); got != "y.yaml" {
		t.Errorf("StorePath(%q) = %q，期望 %q", dirty, got, "y.yaml")
	}
}

func TestStorePathOutsideAbsolute(t *testing.T) {
	st, _ := openTempStore(t)
	outside := filepath.Join(t.TempDir(), "b.yaml")

	if got := st.StorePath(outside); got != filepath.Clean(outside) {
		t.Errorf("StorePath(%q) = %q，期望保留清理后的绝对路径", outside, got)
	}
	abs, isSoft := st.ResolvePath(outside)
	if isSoft {
		t.Errorf("ResolvePath(%q) 的 soft = true，期望 false", outside)
	}
	if abs != outside {
		t.Errorf("ResolvePath(%q) = %q，期望原样返回", outside, abs)
	}
	if st.IsInsideBase(outside) {
		t.Errorf("IsInsideBase(%q) = true，期望 false", outside)
	}

	// 与程序目录共享前缀的兄弟路径不算在目录内。
	_, base := st, st.BaseDir()
	sibling := base + "-other"
	if st.IsInsideBase(sibling) {
		t.Errorf("IsInsideBase(%q) = true，期望 false", sibling)
	}
	if got := st.StorePath(sibling); got != filepath.Clean(sibling) {
		t.Errorf("StorePath(%q) = %q，期望保留绝对路径", sibling, got)
	}
}

func TestStorePathBaseDirItself(t *testing.T) {
	st, base := openTempStore(t)
	if got := st.StorePath(base); got != filepath.Clean(base) {
		t.Errorf("StorePath(程序目录) = %q，期望保留绝对路径 %q", got, filepath.Clean(base))
	}
	if st.IsInsideBase(base) {
		t.Errorf("IsInsideBase(程序目录) = true，期望 false")
	}
	abs, isSoft := st.ResolvePath(base)
	if isSoft || abs != base {
		t.Errorf("ResolvePath(程序目录) = (%q, %v)，期望 (%q, false)", abs, isSoft, base)
	}
	if got := st.StorePath(""); got != "" {
		t.Errorf("StorePath(空串) = %q，期望空串", got)
	}
}

func TestStorePathWindowsCaseInsensitive(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("仅在 Windows 上验证大小写不敏感的包含判断")
	}
	st, base := openTempStore(t)
	upper := strings.ToUpper(base)
	inside := filepath.Join(upper, "Sub", "a.yaml")

	if got := st.StorePath(inside); got != "Sub/a.yaml" {
		t.Errorf("StorePath(%q) = %q，期望 %q", inside, got, "Sub/a.yaml")
	}
	if !st.IsInsideBase(inside) {
		t.Errorf("Windows 下 IsInsideBase(%q) 应为 true", inside)
	}
	abs, isSoft := st.ResolvePath("Sub/a.yaml")
	if !isSoft {
		t.Error("软路径 ResolvePath 的 soft 应为 true")
	}
	if want := filepath.Join(base, "Sub", "a.yaml"); !strings.EqualFold(abs, want) {
		t.Errorf("ResolvePath 拼出的绝对路径 = %q，期望 %q（大小写不敏感比较）", abs, want)
	}
}

func TestStorePathUnixCaseSensitive(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("仅在 Linux 上验证大小写敏感的包含判断（macOS 文件系统可能不区分大小写）")
	}
	st, base := openTempStore(t)
	upper := strings.ToUpper(base)
	if upper == base {
		t.Skip("临时目录路径不含字母，无法改写大小写")
	}
	inside := filepath.Join(upper, "a.yaml")
	if st.IsInsideBase(inside) {
		t.Errorf("Unix 下 IsInsideBase(%q) 应为 false", inside)
	}
	if got := st.StorePath(inside); got != filepath.Clean(inside) {
		t.Errorf("StorePath(%q) = %q，期望保留绝对路径", inside, got)
	}
}

func TestCorruptJSONReturnsError(t *testing.T) {
	cases := []struct {
		file    string
		content string
	}{
		{"settings.json", `{"version":1,`},
		{"entries.json", `不是 JSON`},
		{"presets.json", `{"version":1,"presets":"不是数组"}`},
		{"results.json", `{"version":1,"results":[]}`},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			base := t.TempDir()
			dataDir := filepath.Join(base, "data")
			if err := os.MkdirAll(dataDir, 0o755); err != nil {
				t.Fatalf("创建数据目录失败：%v", err)
			}
			if err := os.WriteFile(filepath.Join(dataDir, c.file), []byte(c.content), 0o644); err != nil {
				t.Fatalf("写入夹具失败：%v", err)
			}
			if _, err := store.Open(store.Options{BaseDir: base}); err == nil {
				t.Fatalf("%s 内容损坏时期望 Open 返回错误，实际为 nil", c.file)
			}
		})
	}
}

func TestVersionTooNewReturnsError(t *testing.T) {
	for _, file := range []string{"settings.json", "entries.json", "presets.json", "results.json"} {
		t.Run(file, func(t *testing.T) {
			base := t.TempDir()
			dataDir := filepath.Join(base, "data")
			if err := os.MkdirAll(dataDir, 0o755); err != nil {
				t.Fatalf("创建数据目录失败：%v", err)
			}
			if err := os.WriteFile(filepath.Join(dataDir, file), []byte(`{"version":2}`), 0o644); err != nil {
				t.Fatalf("写入夹具失败：%v", err)
			}
			if _, err := store.Open(store.Options{BaseDir: base}); err == nil {
				t.Fatalf("%s 版本为 2 时期望 Open 返回错误，实际为 nil", file)
			}
		})
	}
}

func TestOpenUnwritableDataDirFails(t *testing.T) {
	base := t.TempDir()
	blocker := filepath.Join(base, "blocker")
	if err := os.WriteFile(blocker, []byte("占位文件"), 0o644); err != nil {
		t.Fatalf("写入占位文件失败：%v", err)
	}
	dataDir := filepath.Join(blocker, "data")
	if _, err := store.Open(store.Options{BaseDir: base, DataDir: dataDir}); err == nil {
		t.Fatal("数据目录无法创建时期望 Open 返回错误，实际为 nil")
	}
}

func TestNewID(t *testing.T) {
	seen := make(map[string]bool, 64)
	for range 64 {
		id := store.NewID()
		if len(id) != 32 {
			t.Fatalf("NewID() = %q，长度 %d，期望 32", id, len(id))
		}
		if _, err := hex.DecodeString(id); err != nil {
			t.Fatalf("NewID() = %q 不是合法 hex：%v", id, err)
		}
		if seen[id] {
			t.Fatalf("NewID() 生成重复 ID：%q", id)
		}
		seen[id] = true
	}
}

func TestWriteLeavesNoTempFiles(t *testing.T) {
	st, base := openTempStore(t)
	if err := st.SetSettings(store.Settings{FilterMode: "simple"}); err != nil {
		t.Fatalf("SetSettings 失败：%v", err)
	}
	if err := st.PutEntry(store.Entry{ID: "id", Name: "条目", Path: "a.yaml"}); err != nil {
		t.Fatalf("PutEntry 失败：%v", err)
	}
	if err := st.PutPreset(store.Preset{Name: "预设", Mode: "simple", Expression: "香港"}); err != nil {
		t.Fatalf("PutPreset 失败：%v", err)
	}
	if err := st.PutResult("id", "node", store.NodeResult{OK: true}); err != nil {
		t.Fatalf("PutResult 失败：%v", err)
	}

	entries, err := os.ReadDir(filepath.Join(base, "data"))
	if err != nil {
		t.Fatalf("读取数据目录失败：%v", err)
	}
	want := map[string]bool{"settings.json": true, "entries.json": true, "presets.json": true, "results.json": true}
	for _, entry := range entries {
		if !want[entry.Name()] {
			t.Errorf("数据目录里出现意外文件：%s", entry.Name())
		}
	}
	if len(entries) != len(want) {
		t.Errorf("数据目录里有 %d 个文件，期望 %d 个", len(entries), len(want))
	}
}
