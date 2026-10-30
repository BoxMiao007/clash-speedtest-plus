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
// 版本 2 起：条目增加 Kind/URL（订阅条目），设置增加测速/输出/上传字段。
// 版本 1 的文件按字段缺失读取，再由 Settings.Normalize 补齐默认值。
const currentVersion = 2

// ErrReadOnly 表示数据目录不可写：读操作照常，写操作一律拒绝。
var ErrReadOnly = errors.New("数据目录只读，不能保存")

const (
	settingsFileName = "settings.json"
	entriesFileName  = "entries.json"
	presetsFileName  = "presets.json"
	resultsFileName  = "results.json"

	fileMode = 0o644
	dirMode  = 0o755
)

// 条目种类。Kind 为空按本地文件处理：版本 1 的数据没有这个字段。
const (
	EntryKindLocal        = "local"
	EntryKindSubscription = "subscription"
)

// Entry 是一个节点来源条目：本地配置文件或 http(s) 订阅地址。
type Entry struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind,omitempty"` // 空 = 本地文件（版本 1 兼容）
	Path string `json:"path,omitempty"` // 本地：软路径（相对程序目录、/ 分隔）或硬路径（绝对）
	URL  string `json:"url,omitempty"`  // 订阅：http(s) 地址
}

// EffectiveKind 把空 Kind 归一成本地文件。
func (e Entry) EffectiveKind() string {
	if e.Kind == EntryKindSubscription {
		return EntryKindSubscription
	}
	return EntryKindLocal
}

// IsSubscription 报告条目是否来自订阅地址。
func (e Entry) IsSubscription() bool { return e.EffectiveKind() == EntryKindSubscription }

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

// 输出模式（见 GLOSSARY.md「输出模式」）：关闭 / 当前目录下自动命名 / 自定义词干。
const (
	OutputModeOff    = "off"
	OutputModeAuto   = "auto"
	OutputModeCustom = "custom"
)

// 测速默认值，与旧 TUI 的 flag 默认值保持一致（ADR-0001、ADR-0017）。
const (
	DefaultMode            = "download"
	DefaultDownloadMB      = 50
	DefaultUploadMB        = 20
	DefaultTimeoutMS       = 5000
	DefaultConcurrent      = 4
	DefaultParallel        = 6
	DefaultServerURL       = "https://dl.google.com/chrome/mac/universal/stable/GGRO/googlechrome.dmg"
	DefaultMaxLatencyMS    = 1000
	DefaultMaxPacketLoss   = 100
	DefaultMinDownloadMBps = 5
	DefaultMinUploadMBps   = 2
)

// Settings 是窗口内可调的全部选项：过滤条、测速参数、阈值、输出与上传。
// 零值经 Normalize 补齐成旧 TUI 的默认值，因此版本 1 的设置文件可以原样读取。
type Settings struct {
	FilterMode       string `json:"filter_mode"`
	FilterExpression string `json:"filter_expression"`
	FilterCombine    string `json:"filter_combine"`

	Mode       string `json:"mode"`        // fast | download | full
	DownloadMB int    `json:"download_mb"` // 下载测试大小
	UploadMB   int    `json:"upload_mb"`   // 上传测试大小（仅 full）
	TimeoutMS  int    `json:"timeout_ms"`  // 单个请求超时
	Concurrent int    `json:"concurrent"`  // 同一节点的下载并发连接数
	Parallel   int    `json:"parallel"`    // 同时测试的节点数
	ServerURL  string `json:"server_url"`
	UserAgent  string `json:"user_agent"` // 空 = 内核默认 UA
	EarlyStop  int    `json:"early_stop"` // 过筛结果达到该数量后提前结束，0 = 关闭

	MaxLatencyMS    int     `json:"max_latency_ms"`    // 0 = 不限制
	MaxPacketLoss   float64 `json:"max_packet_loss"`   // 百分比；<0 = 不限制
	MinDownloadMBps float64 `json:"min_download_mbps"` // 仅非 fast 模式生效
	MinUploadMBps   float64 `json:"min_upload_mbps"`   // 仅 full 模式生效

	OutputMode     string `json:"output_mode"` // off | auto | custom
	OutputPath     string `json:"output_path"` // custom 模式的词干（软/硬路径）
	ImageEnabled   bool   `json:"image_enabled"`
	ImageSpeedOnly bool   `json:"image_speed_only"`
	RenameEnabled  bool   `json:"rename_enabled"`
	RenameTemplate string `json:"rename_template"` // 空 = 内置模板

	GistToken    string `json:"gist_token"`
	GistAddress  string `json:"gist_address"`
	RepoToken    string `json:"repo_token"`
	RepoAddress  string `json:"repo_address"`
	RepoFilePath string `json:"repo_file_path"`
	RepoBranch   string `json:"repo_branch"`

	// 窗口几何：上次退出时的内容区尺寸与屏幕位置。WindowSet 为 false 时
	// 用默认尺寸并交给系统摆放（首次启动、旧版本数据）。位置允许为负：
	// 多显示器时副屏原点可能在主屏左侧或上方。
	WindowWidth  int  `json:"window_width,omitempty"`
	WindowHeight int  `json:"window_height,omitempty"`
	WindowX      int  `json:"window_x,omitempty"`
	WindowY      int  `json:"window_y,omitempty"`
	WindowSet    bool `json:"window_set,omitempty"`
}

// DefaultSettings 返回旧 TUI 的默认参数组合。
func DefaultSettings() Settings {
	return Settings{
		Mode:            DefaultMode,
		DownloadMB:      DefaultDownloadMB,
		UploadMB:        DefaultUploadMB,
		TimeoutMS:       DefaultTimeoutMS,
		Concurrent:      DefaultConcurrent,
		Parallel:        DefaultParallel,
		ServerURL:       DefaultServerURL,
		MaxLatencyMS:    DefaultMaxLatencyMS,
		MaxPacketLoss:   DefaultMaxPacketLoss,
		MinDownloadMBps: DefaultMinDownloadMBps,
		MinUploadMBps:   DefaultMinUploadMBps,
		OutputMode:      OutputModeOff,
		ImageEnabled:    true,
		RenameEnabled:   true,
	}
}

// Normalize 补齐零值字段并纠正非法值，返回可直接使用的设置。
// 只处理数值与枚举：过滤表达式由 internal/filter 自己归一化。
func (s Settings) Normalize() Settings {
	defaults := DefaultSettings()
	switch s.Mode {
	case "fast", "download", "full":
	default:
		s.Mode = defaults.Mode
	}
	if s.DownloadMB <= 0 {
		s.DownloadMB = defaults.DownloadMB
	}
	if s.UploadMB <= 0 {
		s.UploadMB = defaults.UploadMB
	}
	if s.TimeoutMS <= 0 {
		s.TimeoutMS = defaults.TimeoutMS
	}
	if s.Concurrent <= 0 {
		s.Concurrent = defaults.Concurrent
	}
	if s.Parallel <= 0 {
		s.Parallel = defaults.Parallel
	}
	if strings.TrimSpace(s.ServerURL) == "" {
		s.ServerURL = defaults.ServerURL
	}
	if s.EarlyStop < 0 {
		s.EarlyStop = 0
	}
	if s.MaxLatencyMS < 0 {
		s.MaxLatencyMS = 0
	}
	// 丢包阈值保留 -1「不限制」语义，其余非法负数按默认值处理。
	if s.MaxPacketLoss < 0 && s.MaxPacketLoss != -1 {
		s.MaxPacketLoss = defaults.MaxPacketLoss
	}
	if s.MaxPacketLoss > 100 {
		s.MaxPacketLoss = 100
	}
	if s.MinDownloadMBps < 0 {
		s.MinDownloadMBps = 0
	}
	if s.MinUploadMBps < 0 {
		s.MinUploadMBps = 0
	}
	switch s.OutputMode {
	case OutputModeOff, OutputModeAuto:
	case OutputModeCustom:
		// 自定义词干为空等于没开输出，避免留下写不出文件的半开状态。
		if strings.TrimSpace(s.OutputPath) == "" {
			s.OutputMode = OutputModeOff
		}
	default:
		s.OutputMode = defaults.OutputMode
	}
	// 窗口几何：尺寸过小/过大按默认处理，避免一启动就得到一条看不见的窗口。
	// 位置允许负值（副屏在主屏左侧或上方），只看尺寸是否合理。
	if s.WindowSet {
		if s.WindowWidth < 800 || s.WindowHeight < 560 || s.WindowWidth > 10000 || s.WindowHeight > 10000 {
			s.WindowWidth, s.WindowHeight = 0, 0
			s.WindowX, s.WindowY = 0, 0
			s.WindowSet = false
		}
	} else {
		s.WindowWidth, s.WindowHeight = 0, 0
		s.WindowX, s.WindowY = 0, 0
	}
	return s
}

// UploadEnabled 报告是否配置了 Gist 或仓库上传（地址与 token 齐全）。
func (s Settings) UploadEnabled() bool {
	gistReady := strings.TrimSpace(s.GistToken) != "" && strings.TrimSpace(s.GistAddress) != ""
	repoReady := strings.TrimSpace(s.RepoToken) != "" && strings.TrimSpace(s.RepoAddress) != ""
	return gistReady || repoReady
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
	baseDir  string
	dataDir  string
	readOnly bool

	mu       sync.RWMutex
	settings Settings
	entries  []Entry
	presets  []Preset
	results  map[string]map[string]NodeResult
}

// Open 创建（如需要）数据目录并验证可写，然后读取四项数据。
// 数据文件不存在视为空；JSON 损坏或 version 高于 currentVersion 时返回错误，
// 由界面层决定禁用持久化，而不是静默改写到别处。
func Open(options Options) (*Store, error) { return openStore(options, true) }

// OpenReadOnly 在数据目录不可写时尽量加载已有数据：不创建目录、不探测可写，
// 写方法统一返回 ErrReadOnly。目录或文件缺失时按空数据打开。
func OpenReadOnly(options Options) (*Store, error) { return openStore(options, false) }

// ReadOnly 报告这个 Store 是否只读。
func (s *Store) ReadOnly() bool { return s.readOnly }

func openStore(options Options, writable bool) (*Store, error) {
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
	if writable {
		if err := os.MkdirAll(dataDir, dirMode); err != nil {
			return nil, fmt.Errorf("创建数据目录失败：%w", err)
		}
		if err := probeWritable(dataDir); err != nil {
			return nil, err
		}
	}

	st := &Store{
		baseDir:  filepath.Clean(baseDir),
		dataDir:  filepath.Clean(dataDir),
		readOnly: !writable,
		results:  map[string]map[string]NodeResult{},
	}
	settings, err := loadSettings(filepath.Join(st.dataDir, settingsFileName))
	if err != nil {
		return nil, err
	}
	st.settings = settings
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

// loadSettings 读取设置文件。文件缺失时返回默认设置；文件里出现的键覆盖
// 默认值，缺失的键（版本 1 的老文件）保留默认值。布尔开关没有「非法值」
// 可判断，只能靠键是否存在决定默认值，所以不能解析成零值再 Normalize。
func loadSettings(path string) (Settings, error) {
	value := DefaultSettings()
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return value, nil
	}
	if err != nil {
		return value, fmt.Errorf("读取 %s 失败：%w", filepath.Base(path), err)
	}
	var envelope struct {
		Version  int             `json:"version"`
		Settings json.RawMessage `json:"settings"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return value, fmt.Errorf("%s 不是有效的 JSON：%w", filepath.Base(path), err)
	}
	if envelope.Version > currentVersion {
		return value, fmt.Errorf("%s 的版本 %d 高于支持的最高版本 %d", filepath.Base(path), envelope.Version, currentVersion)
	}
	if len(envelope.Settings) > 0 {
		if err := json.Unmarshal(envelope.Settings, &value); err != nil {
			return value, fmt.Errorf("%s 的内容无法解析：%w", filepath.Base(path), err)
		}
	}
	return value.Normalize(), nil
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
	if s.readOnly {
		return ErrReadOnly
	}
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

// subscriptionsDir 是订阅正文缓存目录，位于数据目录之下。
const subscriptionsDir = "subscriptions"

// SubscriptionPath 返回条目订阅缓存的绝对路径。ID 由 NewID 生成（16 字节 hex），
// 这里仍做一次校验，避免带分隔符的 ID 把文件写到缓存目录之外。
func (s *Store) SubscriptionPath(id string) (string, error) {
	if strings.TrimSpace(id) == "" {
		return "", errors.New("条目 ID 不能为空")
	}
	if id != filepath.Base(id) || id == "." || id == ".." {
		return "", fmt.Errorf("条目 ID 不合法：%q", id)
	}
	return filepath.Join(s.dataDir, subscriptionsDir, id+".yaml"), nil
}

// WriteSubscription 把订阅正文原子写入缓存文件并返回绝对路径。
// 缓存只在程序目录内，重新获取时整文件替换。
func (s *Store) WriteSubscription(id string, body []byte) (string, error) {
	if s.readOnly {
		return "", ErrReadOnly
	}
	path, err := s.SubscriptionPath(id)
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return "", fmt.Errorf("创建订阅缓存目录失败：%w", err)
	}
	tmp, err := os.CreateTemp(dir, "."+id+".tmp-*")
	if err != nil {
		return "", fmt.Errorf("创建订阅缓存临时文件失败：%w", err)
	}
	tmpName := tmp.Name()
	committed := false
	defer func() {
		if !committed {
			tmp.Close()
			os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(body); err != nil {
		return "", fmt.Errorf("写入订阅缓存失败：%w", err)
	}
	if err := tmp.Chmod(fileMode); err != nil {
		return "", fmt.Errorf("设置订阅缓存权限失败：%w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("关闭订阅缓存临时文件失败：%w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return "", fmt.Errorf("替换订阅缓存失败：%w", err)
	}
	committed = true
	return path, nil
}

// RemoveSubscription 删除条目的订阅缓存；缓存不存在视为成功。
func (s *Store) RemoveSubscription(id string) error {
	if s.readOnly {
		return ErrReadOnly
	}
	path, err := s.SubscriptionPath(id)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("删除订阅缓存失败：%w", err)
	}
	return nil
}

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

// SetSettings 覆盖设置并立即落盘；写入前先归一化，非法值不会进文件。
func (s *Store) SetSettings(settings Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settings = settings.Normalize()
	return s.writeFile(settingsFileName, settingsFile{Version: currentVersion, Settings: s.settings})
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
	// 订阅缓存跟着条目走；删除失败不回滚条目，缓存残留最多占一点磁盘。
	_ = s.RemoveSubscription(id)
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

// ClearResults 清空全部条目的测速结果并立即落盘。只清结果，不动条目与订阅缓存。
func (s *Store) ClearResults() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.results) == 0 {
		return nil
	}
	s.results = map[string]map[string]NodeResult{}
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
