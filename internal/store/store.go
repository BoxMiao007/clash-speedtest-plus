// Package store 是 GUI 的本地持久化层：数据目录探测、条目/预设/设置/测速结果读写，
// 以及软硬路径互转。全部数据都落在程序目录下的 data/，拷贝整个程序目录即可迁移。
package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// currentVersion 是数据文件格式版本。加载到更高版本时拒绝启动持久化，
// 避免旧程序改写新格式数据后丢字段。
const currentVersion = 1

const (
	settingsFileName = "settings.json"
	entriesFileName  = "entries.json"
	presetsFileName  = "presets.json"
	resultsFileName  = "results.json"

	fileMode = 0o644
	dirMode  = 0o755
)

// Entry 是一个测试文件条目。
type Entry struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"` // 软路径：相对程序目录、/ 分隔；硬路径：绝对路径
}

// Preset 是一个过滤预设，跨条目通用。
type Preset struct {
	Name       string `json:"name"`
	Mode       string `json:"mode"` // "simple" | "advanced"
	Expression string `json:"expression"`
	Combine    string `json:"combine"` // "and" | "or"
}

// NodeResult 是一个节点的最近一次测速结果。
type NodeResult struct {
	LatencyMS     float64   `json:"latency_ms"`
	JitterMS      float64   `json:"jitter_ms"`
	PacketLoss    float64   `json:"packet_loss"`
	DownloadBps   float64   `json:"download_bps"`
	UploadBps     float64   `json:"upload_bps"`
	DownloadError string    `json:"download_error,omitempty"`
	UploadError   string    `json:"upload_error,omitempty"`
	TestedAt      time.Time `json:"tested_at"`
	OK            bool      `json:"ok"`
}

// Settings 是过滤条的持久化状态。
type Settings struct {
	FilterMode       string `json:"filter_mode"`
	FilterExpression string `json:"filter_expression"`
	FilterCombine    string `json:"filter_combine"`
}

// Options 是 Open 的输入。BaseDir 是程序目录（生产环境由 os.Executable 推导）；
// DataDir 为空时取 BaseDir/data，测试可以注入其它目录。
type Options struct {
	BaseDir string
	DataDir string
}

// 每个数据文件的顶层形状都带 version，便于以后迁移。
type settingsFile struct {
	Version  int      `json:"version"`
	Settings Settings `json:"settings"`
}

type entriesFile struct {
	Version int     `json:"version"`
	Entries []Entry `json:"entries"`
}

type presetsFile struct {
	Version int      `json:"version"`
	Presets []Preset `json:"presets"`
}

type resultsFile struct {
	Version int                              `json:"version"`
	Results map[string]map[string]NodeResult `json:"results"`
}

// Store 是打开后的数据目录。所有导出方法都可以并发调用。
type Store struct {
	baseDir string
	dataDir string

	mu       sync.RWMutex
	settings Settings
	entries  []Entry
	presets  []Preset
	results  map[string]map[string]NodeResult
}

// Open 创建（如需要）数据目录并验证可写，然后读取四项数据。
// 数据文件不存在视为空；JSON 损坏或 version 高于 currentVersion 时返回错误，
// 由界面层决定禁用持久化，而不是静默改写到别处。
func Open(options Options) (*Store, error) {
	if strings.TrimSpace(options.BaseDir) == "" {
		return nil, errors.New("程序目录不能为空")
	}
	baseDir, err := filepath.Abs(options.BaseDir)
	if err != nil {
		return nil, fmt.Errorf("解析程序目录失败：%w", err)
	}
	dataDir := strings.TrimSpace(options.DataDir)
	if dataDir == "" {
		dataDir = filepath.Join(baseDir, "data")
	} else {
		dataDir, err = filepath.Abs(dataDir)
		if err != nil {
			return nil, fmt.Errorf("解析数据目录失败：%w", err)
		}
	}
	if err := os.MkdirAll(dataDir, dirMode); err != nil {
		return nil, fmt.Errorf("创建数据目录失败：%w", err)
	}
	if err := probeWritable(dataDir); err != nil {
		return nil, err
	}

	st := &Store{
		baseDir: filepath.Clean(baseDir),
		dataDir: filepath.Clean(dataDir),
		results: map[string]map[string]NodeResult{},
	}
	settings, err := loadFile[settingsFile](filepath.Join(st.dataDir, settingsFileName))
	if err != nil {
		return nil, err
	}
	st.settings = settings.Settings
	entries, err := loadFile[entriesFile](filepath.Join(st.dataDir, entriesFileName))
	if err != nil {
		return nil, err
	}
	if entries.Entries != nil {
		st.entries = entries.Entries
	} else {
		st.entries = []Entry{}
	}
	presets, err := loadFile[presetsFile](filepath.Join(st.dataDir, presetsFileName))
	if err != nil {
		return nil, err
	}
	if presets.Presets != nil {
		st.presets = presets.Presets
	} else {
		st.presets = []Preset{}
	}
	results, err := loadFile[resultsFile](filepath.Join(st.dataDir, resultsFileName))
	if err != nil {
		return nil, err
	}
	if results.Results != nil {
		st.results = results.Results
	}
	return st, nil
}

// probeWritable 在数据目录里创建临时文件、写入后删除；任何一步失败都视为不可写。
func probeWritable(dir string) error {
	probe, err := os.CreateTemp(dir, ".write-probe-*")
	if err != nil {
		return fmt.Errorf("数据目录不可写：%w", err)
	}
	name := probe.Name()
	defer os.Remove(name)
	if _, err := probe.Write([]byte("ok")); err != nil {
		probe.Close()
		return fmt.Errorf("数据目录不可写：%w", err)
	}
	if err := probe.Close(); err != nil {
		return fmt.Errorf("数据目录不可写：%w", err)
	}
	return nil
}

// loadFile 读取并校验一个带 version 的数据文件；文件不存在时返回零值。
func loadFile[T any](path string) (T, error) {
	var value T
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return value, nil
	}
	if err != nil {
		return value, fmt.Errorf("读取 %s 失败：%w", filepath.Base(path), err)
	}
	var header struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return value, fmt.Errorf("%s 不是有效的 JSON：%w", filepath.Base(path), err)
	}
	// 版本 0 视为字段缺失的兼容情况；高于当前版本才拒绝。
	if header.Version > currentVersion {
		return value, fmt.Errorf("%s 的版本 %d 高于支持的最高版本 %d", filepath.Base(path), header.Version, currentVersion)
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return value, fmt.Errorf("%s 的内容无法解析：%w", filepath.Base(path), err)
	}
	return value, nil
}

// writeFile 用「同目录临时文件 + rename」原子替换目标文件，附带 0644 权限。
func (s *Store) writeFile(name string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 %s 失败：%w", name, err)
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(s.dataDir, "."+name+".tmp-*")
	if err != nil {
		return fmt.Errorf("创建 %s 临时文件失败：%w", name, err)
	}
	tmpName := tmp.Name()
	committed := false
	defer func() {
		if !committed {
			tmp.Close()
			os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("写入 %s 失败：%w", name, err)
	}
	if err := tmp.Chmod(fileMode); err != nil {
		return fmt.Errorf("设置 %s 权限失败：%w", name, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("关闭 %s 临时文件失败：%w", name, err)
	}
	if err := os.Rename(tmpName, filepath.Join(s.dataDir, name)); err != nil {
		return fmt.Errorf("替换 %s 失败：%w", name, err)
	}
	committed = true
	return nil
}

// NewID 生成 16 字节随机数的 hex 字符串，用作条目 ID。
func NewID() string {
	var raw [16]byte
	// crypto/rand.Read 自 Go 1.24 起不会返回错误：随机源失败时运行时直接崩溃。
	_, _ = rand.Read(raw[:])
	return hex.EncodeToString(raw[:])
}

// BaseDir 返回程序目录的绝对路径。
func (s *Store) BaseDir() string { return s.baseDir }

// DataDir 返回数据目录的绝对路径。
func (s *Store) DataDir() string { return s.dataDir }

// Entries 返回全部条目的副本，调用方可以自由修改。
func (s *Store) Entries() []Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Entry, len(s.entries))
	copy(out, s.entries)
	return out
}

// Presets 返回全部过滤预设的副本。
func (s *Store) Presets() []Preset {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Preset, len(s.presets))
	copy(out, s.presets)
	return out
}

// Settings 返回过滤条设置。
func (s *Store) Settings() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settings
}

// SetSettings 覆盖过滤条设置并立即落盘。
func (s *Store) SetSettings(settings Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settings = settings
	return s.writeFile(settingsFileName, settingsFile{Version: currentVersion, Settings: settings})
}

// PutEntry 按 ID 覆盖已有条目，否则追加；写入失败时返回错误，内存状态保持最新。
func (s *Store) PutEntry(entry Entry) error {
	if strings.TrimSpace(entry.ID) == "" {
		return errors.New("条目 ID 不能为空")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	replaced := false
	for index := range s.entries {
		if s.entries[index].ID == entry.ID {
			s.entries[index] = entry
			replaced = true
			break
		}
	}
	if !replaced {
		s.entries = append(s.entries, entry)
	}
	return s.writeFile(entriesFileName, entriesFile{Version: currentVersion, Entries: s.entries})
}

// DeleteEntry 删除条目，同时删除该条目的全部测速结果。删除不存在的条目视为成功。
func (s *Store) DeleteEntry(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	entryChanged := false
	for index := range s.entries {
		if s.entries[index].ID == id {
			s.entries = append(s.entries[:index], s.entries[index+1:]...)
			entryChanged = true
			break
		}
	}
	_, hasResults := s.results[id]
	if hasResults {
		delete(s.results, id)
	}
	if entryChanged {
		if err := s.writeFile(entriesFileName, entriesFile{Version: currentVersion, Entries: s.entries}); err != nil {
			return err
		}
	}
	if hasResults {
		return s.writeFile(resultsFileName, resultsFile{Version: currentVersion, Results: s.results})
	}
	return nil
}

// PutPreset 按名称覆盖已有预设，否则追加。
func (s *Store) PutPreset(preset Preset) error {
	if strings.TrimSpace(preset.Name) == "" {
		return errors.New("预设名称不能为空")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	replaced := false
	for index := range s.presets {
		if s.presets[index].Name == preset.Name {
			s.presets[index] = preset
			replaced = true
			break
		}
	}
	if !replaced {
		s.presets = append(s.presets, preset)
	}
	return s.writeFile(presetsFileName, presetsFile{Version: currentVersion, Presets: s.presets})
}

// DeletePreset 按名称删除预设；删除不存在的名称视为成功。
func (s *Store) DeletePreset(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := false
	for index := range s.presets {
		if s.presets[index].Name == name {
			s.presets = append(s.presets[:index], s.presets[index+1:]...)
			changed = true
			break
		}
	}
	if !changed {
		return nil
	}
	return s.writeFile(presetsFileName, presetsFile{Version: currentVersion, Presets: s.presets})
}

// Result 读取某个条目下节点的最近结果。
func (s *Store) Result(entryID, nodeID string) (NodeResult, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	nodes, ok := s.results[entryID]
	if !ok {
		return NodeResult{}, false
	}
	result, ok := nodes[nodeID]
	return result, ok
}

// PutResult 保存单个节点的结果并立即落盘。
func (s *Store) PutResult(entryID, nodeID string, result NodeResult) error {
	if entryID == "" || nodeID == "" {
		return errors.New("条目 ID 与节点 ID 不能为空")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	nodes, ok := s.results[entryID]
	if !ok {
		nodes = map[string]NodeResult{}
		s.results[entryID] = nodes
	}
	nodes[nodeID] = result
	return s.writeFile(resultsFileName, resultsFile{Version: currentVersion, Results: s.results})
}

// DeleteResults 删除某个条目的全部测速结果；条目不存在时视为成功。
func (s *Store) DeleteResults(entryID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.results[entryID]; !ok {
		return nil
	}
	delete(s.results, entryID)
	return s.writeFile(resultsFileName, resultsFile{Version: currentVersion, Results: s.results})
}

// ResolvePath 把条目里保存的路径还原成绝对路径；soft 表示保存的是软路径。
// 软路径按 / 分隔存储，这里先换成系统分隔符再相对程序目录拼接；硬路径原样返回。
func (s *Store) ResolvePath(stored string) (abs string, soft bool) {
	if stored == "" {
		return "", false
	}
	if filepath.IsAbs(stored) {
		return stored, false
	}
	return filepath.Join(s.baseDir, filepath.FromSlash(stored)), true
}

// StorePath 把绝对路径转成条目里保存的形态：位于程序目录内时转成 / 分隔的相对路径，
// 程序目录本身或目录之外保留清理后的绝对路径。
func (s *Store) StorePath(abs string) string {
	abs = strings.TrimSpace(abs)
	if abs == "" {
		return ""
	}
	clean := filepath.Clean(abs)
	if !filepath.IsAbs(clean) {
		// 非绝对路径按软路径规范成 / 分隔，保证存储口径一致。
		return filepath.ToSlash(clean)
	}
	if !s.IsInsideBase(clean) {
		return clean
	}
	rel, err := filepath.Rel(s.baseDir, clean)
	if err != nil {
		return clean
	}
	return filepath.ToSlash(rel)
}

// IsInsideBase 判断绝对路径是否位于程序目录内（不含程序目录本身）。
// Windows 下按大小写不敏感比较，Unix 下保持大小写敏感。
func (s *Store) IsInsideBase(abs string) bool {
	if abs == "" || !filepath.IsAbs(abs) {
		return false
	}
	rel, err := filepath.Rel(s.baseDir, filepath.Clean(abs))
	if err != nil {
		// 不同盘符（或不同卷）无法比较，一律视为不在目录内。
		return false
	}
	if rel == "." || rel == ".." {
		return false
	}
	return !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
