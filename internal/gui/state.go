package gui

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/BoxMiao007/clash-speedtest-plus/internal/filter"
	"github.com/BoxMiao007/clash-speedtest-plus/internal/speedtester"
	"github.com/BoxMiao007/clash-speedtest-plus/internal/store"
	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/flex"
	"github.com/vivaui/viva/ui"
)

type nodeResult = store.NodeResult

// viewKind 是主内容区的三种视图。
type viewKind uint8

const (
	viewNodes viewKind = iota
	viewResults
	viewSettings
)

type nodePhase uint8

const (
	phaseIdle nodePhase = iota
	phaseQueued
	phaseTesting
	phaseDone
)

type dialogKind uint8

const (
	dialogNone dialogKind = iota
	dialogEntry
	dialogPreset
	dialogConfirm
	dialogAlert
)

type confirmKind uint8

const (
	confirmDeleteEntry confirmKind = iota
	confirmDeletePreset
)

var errNoStore = errors.New("数据目录不可用")

// state 是窗口线程独占的模型：Paint 只读它，后台测速只通过 pending 队列
// 把事件交回，由 OnWake 在窗口线程并入。
type state struct {
	tree     *ui.Tree
	ids      ids
	baseDir  string
	dataDir  string
	store    *store.Store
	storeErr string

	theme  ui.Theme
	status string
	active viewKind
	window ui.Size

	entries []*entryState
	nodes   []*nodeState
	visible []*nodeState
	groups  []*groupState
	byKey   map[string]*nodeState
	results []*nodeState

	selectedCount int
	totalCount    int
	countText     string
	resultText    string
	dataText      string

	filterMode    filter.Mode
	filterField   flex.Field
	filterCombine filter.Combine
	matcher       *filter.Matcher
	filterErr     string

	presets      []store.Preset
	presetSel    int
	presetLabels []string
	presetOpen   flex.SelectState

	dialog     dialogKind
	entryForm  entryForm
	presetForm presetForm
	confirm    confirmState
	alert      alertState

	menu menuState

	nodeItems     []listItem
	nodeContent   float32
	nodeOffset    float32
	nodeViewport  ui.Rect
	nodeListBox   flex.Box
	nodeListValid bool

	resultItems     []listItem
	resultContent   float32
	resultOffset    float32
	resultViewport  ui.Rect
	resultListBox   flex.Box
	resultListValid bool

	// 节点来源面板：折叠状态、拖动后的宽度，以及滚动条的拖动状态。
	// 宽度由 flex.Splitter 维护（Pos = 面板宽度）；折叠时不声明 Splitter。
	entriesCollapsed bool
	entriesSplit     flex.SplitterState
	entryScrollBox   flex.Box
	entryScrollValid bool

	settingsScrollBox   flex.Box
	settingsScrollValid bool

	// 四条滚动条的拖动状态：滑块抓取点与是否正在拖。偏移量仍由列表
	// （nodeOffset / resultOffset）或 flex.ScrollState 持有。
	entryBar    barState
	nodeBar     barState
	resultBar   barState
	settingsBar barState

	engine      Runner
	run         *runState
	pendingMu   sync.Mutex
	pending     []runEvent
	invalidator *app.Invalidator
	closed      bool

	// settings 是窗口内可调的全部选项（测速参数、阈值、输出、上传）。
	// 启动时从 settings.json 读取，保存设置视图时整体写回。
	settings store.Settings

	// 结果视图的排序与详情。排序只在状态变化时做，Paint 只读字段。
	sortKey    sortKey
	sortAsc    bool
	detailKey  string
	detailNode *nodeState
	detailText string

	// 预计算的表头与按钮文案：排序箭头、暂停/继续、上传列是否显示。
	showUpload    bool
	pauseLabel    string
	headerLatency string
	headerJitter  string
	headerLoss    string
	headerSpeed   string
	headerUpload  string
	headerTested  string

	// 轮次：每个选中条目一轮，按条目顺序自动推进；全部正常完成后写合并导出。
	rounds       []*roundState
	roundIndex   int
	autoAdvance  bool
	paused       bool
	earlyStopped bool

	// 导出：后台任务只写 pending 事件，窗口线程更新状态。
	// exporting 是进行中的导出任务数（允许上一轮导出未完成时下一轮继续测）；
	// pendingExport 是一轮测完但还没轮到写盘时挂起的轮次。
	exporting     int
	exportRound   int
	exportText    string
	exportSeq     int
	pendingExport *roundState

	settingsForm settingsForm
}

type entryState struct {
	entry      store.Entry
	ids        entryIDs
	selected   bool
	loaded     bool
	loadErr    string
	missing    bool
	fetching   bool
	nodes      []*nodeState
	group      *groupState
	checkLabel string

	pathText   string
	absText    string
	statusText string
	readLabel  string
	fetchLabel string
}

type groupState struct {
	entry     *entryState
	nodes     []*nodeState
	selected  int
	countText string
}

type listItemKind uint8

const (
	itemHeader listItemKind = iota
	itemNode
)

// listItem 是虚拟列表的一行：分组标题或节点行。y 是内容坐标，绘制时减去
// 滚动偏移；只绘制视口内的行，节点很多时也不会每帧给所有行排版和塑字。
type listItem struct {
	kind   listItemKind
	y      float32
	height float32
	group  *groupState
	node   *nodeState
}

type nodeState struct {
	entry *entryState
	key   string
	id    string
	ids   nodeIDs

	name   string
	typ    string
	server string
	port   string
	proxy  *speedtester.CProxy

	result    nodeResult
	hasResult bool

	selected  bool
	phase     nodePhase
	liveSpeed float64

	serverText   string
	subtitle     string
	rowLabel     string
	checkLabel   string
	lastTestText string
	infoText     string
	summaryText  string
	latencyText  string
	jitterText   string
	lossText     string
	speedText    string
	uploadText   string
	statusText   string
	clipText     string
}

type entryForm struct {
	editing  bool
	id       string
	kind     int // 0 = 本地文件，1 = 订阅地址
	name     flex.Field
	url      flex.Field
	path     string
	pathText string
	err      string
}

type presetForm struct {
	rename  bool
	oldName string
	name    flex.Field
	err     string
}

type confirmState struct {
	kind       confirmKind
	entryID    string
	presetName string
	title      string
	body       string
}

type alertState struct {
	open  bool
	title string
	body  string
}

type menuState struct {
	open   bool
	x, y   float32
	nodes  []*nodeState
	multi  bool
	clip   string
	items  []menuEntry
	width  float32
	height float32
}

// RunNode 是一次测试要交给引擎的节点；Key 在结果回调里原样返回。
type RunNode struct {
	Key   string
	Proxy *speedtester.CProxy
}

type Handlers struct {
	Start    func(key, proxyType string)
	Progress func(speedtester.Progress)
	Result   func(*speedtester.Result)
	// EarlyStop 在过筛结果达到提前结束限额时由引擎调用；随后本轮不再派发新节点。
	EarlyStop func()
	Finished  func(error)
}

// Runner 是 GUI 与测速引擎之间的窄接口；生产实现包 internal/speedtester，
// 测试注入假实现，避免测试联网。
type Runner interface {
	// Configure 在每次开测前把当前参数交给引擎；实现必须自己拷贝一份。
	Configure(config TestConfig)
	Run(ctx context.Context, batch []RunNode, handlers Handlers) error
	Pause()
	Resume()
	Stop()
}

type runEventKind uint8

const (
	eventStart runEventKind = iota
	eventProgress
	eventResult
	eventFinished
	// eventEarlyStop 是引擎达到提前结束限额、停止派发新节点。
	eventEarlyStop
	// eventExported 是某一轮的导出任务结束（yaml/图片/上传都算在里面）。
	eventExported
	// eventFetched 是订阅拉取结束，body 是规范化后的配置正文。
	eventFetched
)

type runEvent struct {
	kind     runEventKind
	key      string
	progress speedtester.Progress
	result   *speedtester.Result
	err      error
	round    int
	roundRef *roundState
	path     string
	warning  string
	body     []byte
	proxies  []map[string]any
}

type runState struct {
	running bool
	stopped bool
	paused  bool
	total   int
	done    int
	active  int
	round   int
	batch   []*nodeState
	cancel  context.CancelFunc
	text    string
}

// roundState 是一轮测速：对应一个选中条目，独立测速、独立导出；
// 全部轮正常完成后额外写一份「合并导出」。
type roundState struct {
	entry    *entryState
	nodes    []*nodeState
	done     bool
	stopped  bool
	early    bool
	exported bool
	err      string
	// proxies 是本轮导出后过筛的节点，供最后的「合并导出」求并集。
	proxies []map[string]any
}

// sortKey 是结果视图可排序的列。默认：fast 模式按延迟升序，其余按下载速度降序。
type sortKey uint8

const (
	sortBySpeed sortKey = iota
	sortByLatency
	sortByJitter
	sortByLoss
	sortByUpload
	sortByTested
	sortByName
)

func newState(opts Options) (*state, error) {
	baseDir, err := filepath.Abs(opts.BaseDir)
	if err != nil {
		return nil, fmt.Errorf("程序目录：%w", err)
	}
	dataDir := opts.DataDir
	if dataDir == "" {
		dataDir = filepath.Join(baseDir, "data")
	}
	if dataDir, err = filepath.Abs(dataDir); err != nil {
		return nil, fmt.Errorf("数据目录：%w", err)
	}

	theme := opts.Theme
	if theme.Background == (ui.Color{}) {
		theme = ui.LightTheme()
	}

	window := &ui.Tree{}
	value := &state{
		tree:         window,
		ids:          newIDs(window),
		baseDir:      baseDir,
		dataDir:      dataDir,
		theme:        theme,
		active:       viewNodes,
		presetSel:    -1,
		byKey:        make(map[string]*nodeState),
		status:       "就绪",
		entriesSplit: flex.SplitterState{Pos: entriesPanelWidth},
	}
	value.dataText = "数据目录：" + dataDir
	value.engine = opts.Runner
	if value.engine == nil {
		value.engine = &engineRunner{}
	}

	database, openErr := store.Open(store.Options{BaseDir: baseDir, DataDir: dataDir})
	if openErr != nil {
		value.storeErr = openErr.Error()
		value.status = "数据目录不可用，改动不会保存"
		value.alert = alertState{
			open:  true,
			title: "数据目录不可用",
			body:  fmt.Sprintf("无法使用数据目录 %s：%v\n条目、预设和测速结果不会保存，也不会写到其它位置。", dataDir, openErr),
		}
		value.dialog = dialogAlert
		value.dataText = "数据目录不可用 · " + dataDir
	} else {
		value.store = database
		value.settings = database.Settings()
		settings := value.settings
		value.filterMode = filter.Mode(settings.FilterMode)
		value.filterCombine = filter.Combine(settings.FilterCombine)
		value.filterField.SetText(settings.FilterExpression)
		switch value.filterMode {
		case filter.ModeSimple, filter.ModeAdvanced:
		default:
			value.filterMode = filter.ModeSimple
		}
		switch value.filterCombine {
		case filter.CombineAnd, filter.CombineOr:
		default:
			value.filterCombine = filter.CombineAnd
		}
	}
	if value.store == nil {
		value.settings = store.DefaultSettings()
	}
	value.settings = value.settings.Normalize()
	value.settingsForm.load(value.settings)
	value.resetSort()
	value.refreshSortHeaders()
	value.refreshModeFlags()

	normalized := filter.Normalize(filter.Spec{Mode: value.filterMode, Combine: value.filterCombine})
	value.filterMode, value.filterCombine = normalized.Mode, normalized.Combine

	value.loadPresets()
	value.reloadEntries()
	value.compileFilter()
	value.rebuild()
	return value, nil
}

func (s *state) loadPresets() {
	s.presets = nil
	if s.store != nil {
		s.presets = append(s.presets, s.store.Presets()...)
	}
	s.refreshPresetLabels()
}

func (s *state) refreshPresetLabels() {
	labels := make([]string, 0, len(s.presets)+1)
	labels = append(labels, "不使用预设")
	for _, preset := range s.presets {
		labels = append(labels, preset.Name)
	}
	s.presetLabels = labels
	if s.presetSel >= len(s.presets) {
		s.presetSel = -1
	}
}

func (s *state) presetIndex(name string) int {
	for index, preset := range s.presets {
		if preset.Name == name {
			return index
		}
	}
	return -1
}

// reloadEntries 从 store 读条目并同步读取每个条目的节点；应用启动时默认
// 全选所有条目，打开就能直接看到节点。
func (s *state) reloadEntries() {
	s.entries = s.entries[:0]
	if s.store == nil {
		return
	}
	for _, record := range s.store.Entries() {
		entry := &entryState{
			entry:    record,
			ids:      registerEntryIDs(s.tree, s.ids.page, record.ID),
			selected: true,
		}
		s.decorateEntry(entry)
		s.entries = append(s.entries, entry)
	}
	for _, entry := range s.entries {
		s.readEntryInitial(entry)
	}
	if len(s.entries) > 0 {
		s.status = fmt.Sprintf("已加载 %d 个条目", len(s.entries))
	} else if s.storeErr == "" {
		s.status = "还没有节点来源条目，先新建一个"
	}
}

func (s *state) decorateEntry(entry *entryState) {
	entry.checkLabel = "选择条目 " + entry.entry.Name
	entry.readLabel, entry.fetchLabel = "读取", "重新获取"
	if entry.entry.IsSubscription() {
		entry.readLabel, entry.fetchLabel = "读取缓存", "重新获取"
		entry.pathText = "订阅 · " + entry.entry.URL
		entry.absText = ""
		entry.missing = false
		if path, err := s.subscriptionPath(entry); err == nil {
			entry.absText = "缓存 · " + path
			if _, statErr := os.Stat(path); statErr != nil {
				entry.statusText = "尚未拉取订阅"
				return
			}
		} else {
			entry.absText = "（数据目录不可用，无法缓存订阅）"
			entry.statusText = "数据目录不可用"
			return
		}
		if entry.loaded {
			entry.statusText = fmt.Sprintf("已读取 %d 个节点", len(entry.nodes))
		} else {
			entry.statusText = "缓存就绪"
		}
		return
	}
	entry.pathText = formatPathKind(entry.entry.Path)
	absolute, _ := s.resolvePath(entry.entry.Path)
	entry.absText = absolute
	entry.missing = false
	if entry.entry.Path == "" {
		entry.statusText = "未设置路径"
		return
	}
	if _, err := os.Stat(absolute); err != nil {
		entry.missing = true
		entry.statusText = "文件不存在或已移动"
		return
	}
	if entry.loaded {
		entry.statusText = fmt.Sprintf("已读取 %d 个节点", len(entry.nodes))
	} else {
		entry.statusText = "未读取"
	}
}

func (s *state) resolvePath(stored string) (string, bool) {
	if s.store != nil {
		return s.store.ResolvePath(stored)
	}
	if filepath.IsAbs(stored) {
		return stored, false
	}
	return filepath.Join(s.baseDir, filepath.FromSlash(stored)), true
}

func (s *state) storePath(absolute string) string {
	if s.store != nil {
		return s.store.StorePath(absolute)
	}
	return filepath.Clean(absolute)
}

// readEntryInitial 是启动时的读取：订阅条目只用缓存，不联网。这样打开窗口
// 不会因为订阅多而卡在网络请求上；要拉新内容点「重新获取」。
func (s *state) readEntryInitial(entry *entryState) {
	if !entry.entry.IsSubscription() {
		s.readEntry(entry)
		return
	}
	path, err := s.subscriptionPath(entry)
	if err != nil {
		entry.loadErr = err.Error()
		return
	}
	proxies, err := speedtester.LoadNodesFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			entry.loadErr = err.Error()
		}
		return
	}
	s.applyNodes(entry, proxies)
}

// readEntry 读取一个条目：本地文件直接解析；订阅条目优先用缓存，没有缓存
// 时异步拉取。节点列表与已有状态按稳定身份合并：身份匹配的节点保留勾选与
// 测试结果，新节点默认勾选，消失的节点移除。
func (s *state) readEntry(entry *entryState) {
	if s.running() {
		return
	}
	if entry.entry.IsSubscription() {
		s.readSubscription(entry, false)
		return
	}
	if entry.entry.Path == "" {
		entry.loadErr = "未设置路径"
		entry.statusText = "未设置路径"
		s.rebuild()
		return
	}
	absolute, _ := s.resolvePath(entry.entry.Path)
	entry.absText = absolute
	proxies, err := speedtester.LoadNodesFile(absolute)
	if err != nil {
		entry.loadErr = err.Error()
		entry.missing = errors.Is(err, os.ErrNotExist)
		if entry.missing {
			entry.statusText = "文件不存在或已移动"
		} else {
			entry.statusText = "读取失败：" + err.Error()
		}
		s.rebuild()
		return
	}
	s.applyNodes(entry, proxies)
}

// applyNodes 把刚解析出的节点并入条目：按稳定身份保留勾选与上次结果。
func (s *state) applyNodes(entry *entryState, proxies []*speedtester.CProxy) {
	previous := make(map[string]*nodeState, len(entry.nodes))
	for _, node := range entry.nodes {
		previous[node.id] = node
	}
	seen := make(map[string]int, len(proxies))
	next := make([]*nodeState, 0, len(proxies))
	for _, proxy := range proxies {
		base := nodeIdentity(proxy)
		seen[base]++
		id := base
		if seen[base] > 1 {
			id = base + "#" + strconv.Itoa(seen[base])
		}
		node, ok := previous[id]
		if !ok {
			node = &nodeState{
				entry:    entry,
				id:       id,
				key:      entry.entry.ID + "/" + id,
				ids:      registerNodeIDs(s.tree, entry.ids, id),
				selected: true,
			}
			if s.store != nil {
				if stored, exists := s.store.Result(entry.entry.ID, id); exists {
					node.result = stored
					node.hasResult = true
				}
			}
		}
		node.entry = entry
		node.proxy = proxy
		node.name = proxy.Name()
		node.typ = proxy.Type().String()
		node.server = configString(proxy, "server")
		node.port = configString(proxy, "port")
		if node.phase == phaseIdle {
			node.statusText = ""
		}
		s.refreshNodeDisplay(node)
		next = append(next, node)
	}
	entry.nodes = next
	entry.loaded = true
	entry.loadErr = ""
	entry.missing = false
	entry.statusText = fmt.Sprintf("已读取 %d 个节点", len(next))
	s.rebuild()
}

// readSubscription 读取订阅条目。force 为真时无条件重新拉取；否则有缓存
// 直接解析缓存，没有缓存才异步拉取（应用启动、勾选条目都不会阻塞在网络）。
func (s *state) readSubscription(entry *entryState, force bool) {
	if entry.entry.URL == "" {
		entry.loadErr = "未设置订阅地址"
		entry.statusText = "未设置订阅地址"
		s.rebuild()
		return
	}
	if entry.fetching {
		entry.statusText = "正在拉取订阅…"
		s.rebuild()
		return
	}
	path, err := s.subscriptionPath(entry)
	if err != nil {
		entry.loadErr = err.Error()
		entry.statusText = "数据目录不可用，无法缓存订阅"
		s.rebuild()
		return
	}
	if !force {
		if proxies, err := speedtester.LoadNodesFile(path); err == nil {
			s.applyNodes(entry, proxies)
			entry.statusText = fmt.Sprintf("已读取 %d 个节点（缓存）", len(proxies))
			return
		} else if !errors.Is(err, os.ErrNotExist) {
			entry.loadErr = err.Error()
			entry.statusText = "订阅缓存无法解析：" + err.Error()
			s.rebuild()
			return
		}
	}
	s.startSubscriptionFetch(entry)
}

func (s *state) subscriptionPath(entry *entryState) (string, error) {
	if s.store == nil {
		return "", errNoStore
	}
	return s.store.SubscriptionPath(entry.entry.ID)
}

// startSubscriptionFetch 在后台拉取订阅，完成后通过 pending 事件回到窗口
// 线程写缓存、解析节点。网络请求不在绘制或布局路径上。
func (s *state) startSubscriptionFetch(entry *entryState) {
	entry.fetching = true
	entry.loadErr = ""
	entry.statusText = "正在拉取订阅…"
	s.rebuild()
	userAgent := s.settings.UserAgent
	target := entry.entry.URL
	entryID := entry.entry.ID
	go func() {
		body, used, err := speedtester.FetchSubscriptionBody(target, userAgent)
		s.enqueue(runEvent{kind: eventFetched, key: entryID, body: body, path: used, err: err})
	}()
}

func configString(proxy *speedtester.CProxy, key string) string {
	if proxy == nil || proxy.Config == nil {
		return ""
	}
	value, ok := proxy.Config[key]
	if !ok || value == nil {
		return ""
	}
	return fmt.Sprintf("%v", value)
}

// compileFilter 编译当前过滤表达式；高级模式写错时保留上一个有效过滤器，
// 只把错误显示出来。
func (s *state) compileFilter() {
	spec := filter.Normalize(filter.Spec{Mode: s.filterMode, Expression: s.filterField.Text(), Combine: s.filterCombine})
	matcher, err := filter.Compile(spec)
	if err != nil {
		s.filterErr = err.Error()
		return
	}
	s.matcher = matcher
	s.filterErr = ""
	s.rebuild()
}

func (s *state) setFilterMode(mode filter.Mode) {
	s.filterMode = mode
	s.compileFilter()
	s.persistSettings()
}

func (s *state) setFilterCombine(combine filter.Combine) {
	s.filterCombine = combine
	s.compileFilter()
	s.persistSettings()
}

func (s *state) persistSettings() {
	settings := s.settings
	settings.FilterMode = string(s.filterMode)
	settings.FilterExpression = s.filterField.Text()
	settings.FilterCombine = string(s.filterCombine)
	s.settings = settings
	if s.store == nil {
		s.status = "数据目录不可用，过滤设置未保存"
		return
	}
	if err := s.store.SetSettings(settings); err != nil {
		s.status = "保存过滤设置失败：" + err.Error()
	}
}

func (s *state) match(name string) bool {
	return s.matcher == nil || s.matcher.Match(name)
}

// rebuild 从条目与过滤结果重算可见节点、分组和计数。所有显示字符串在这里
// 预计算，Paint 只读字段。
func (s *state) rebuild() {
	s.nodes = s.nodes[:0]
	for _, entry := range s.entries {
		s.nodes = append(s.nodes, entry.nodes...)
	}
	s.byKey = make(map[string]*nodeState, len(s.nodes))
	for _, node := range s.nodes {
		s.byKey[node.key] = node
	}

	visible := s.visible[:0]
	groups := s.groups[:0]
	selectedCount, total := 0, 0
	for _, entry := range s.entries {
		entry.group = nil
		if !entry.selected {
			continue
		}
		group := &groupState{entry: entry}
		for _, node := range entry.nodes {
			total++
			if node.selected {
				selectedCount++
			}
			if !s.match(node.name) {
				continue
			}
			visible = append(visible, node)
			group.nodes = append(group.nodes, node)
			if node.selected {
				group.selected++
			}
		}
		entry.statusText = entryStatusText(entry)
		if len(group.nodes) > 0 {
			group.countText = fmt.Sprintf("%d 个节点 · 已选 %d", len(group.nodes), group.selected)
			entry.group = group
			groups = append(groups, group)
		}
	}
	s.visible = visible
	s.groups = groups
	s.selectedCount = selectedCount
	s.totalCount = total
	s.countText = fmt.Sprintf("已选 %d / 共 %d 个节点", selectedCount, total)
	s.buildNodeItems()
	s.rebuildResults()
}

func (s *state) buildNodeItems() {
	items := s.nodeItems[:0]
	y := float32(8)
	for _, group := range s.groups {
		items = append(items, listItem{kind: itemHeader, y: y, height: 32, group: group})
		y += 32
		for _, node := range group.nodes {
			items = append(items, listItem{kind: itemNode, y: y, height: 38, node: node})
			y += 38
		}
		y += 10
	}
	if len(items) > 0 {
		y += 8
	}
	s.nodeItems = items
	s.nodeContent = y
	s.clampNodeOffset()
}

func (s *state) clampNodeOffset() {
	maximum := s.nodeContent - s.nodeViewport.Height
	if maximum < 0 {
		maximum = 0
	}
	if s.nodeOffset > maximum {
		s.nodeOffset = maximum
	}
	if s.nodeOffset < 0 {
		s.nodeOffset = 0
	}
}

func (s *state) clampResultOffset() {
	maximum := s.resultContent - s.resultViewport.Height
	if maximum < 0 {
		maximum = 0
	}
	if s.resultOffset > maximum {
		s.resultOffset = maximum
	}
	if s.resultOffset < 0 {
		s.resultOffset = 0
	}
}

func entryStatusText(entry *entryState) string {
	if entry.entry.IsSubscription() {
		if entry.fetching {
			return "正在拉取订阅…"
		}
		if entry.loadErr != "" {
			return "订阅读取失败：" + entry.loadErr
		}
		if entry.loaded {
			return fmt.Sprintf("已读取 %d 个节点", len(entry.nodes))
		}
		return "尚未拉取订阅"
	}
	if entry.loadErr != "" {
		if entry.missing {
			return "文件不存在或已移动"
		}
		return "读取失败：" + entry.loadErr
	}
	if entry.loaded {
		return fmt.Sprintf("已读取 %d 个节点", len(entry.nodes))
	}
	return "未读取"
}

// resetSort 把排序恢复成当前模式的默认：fast 按延迟升序，其余按下载速度降序。
func (s *state) resetSort() {
	if s.settings.Mode == "fast" {
		s.sortKey, s.sortAsc = sortByLatency, true
		return
	}
	s.sortKey, s.sortAsc = sortBySpeed, false
}

// rebuildResults 生成下载速度视图的行。有轮次时只看当前轮：本轮已测结果
// 与在测/排队节点；没有轮次时退回全部历史结果。排序只在状态变化时做，
// Paint 只读字段。
func (s *state) rebuildResults() {
	rows := s.results[:0]
	if len(s.rounds) > 0 && s.roundIndex >= 0 && s.roundIndex < len(s.rounds) {
		for _, node := range s.rounds[s.roundIndex].nodes {
			if node.hasResult || node.phase == phaseTesting || node.phase == phaseQueued {
				rows = append(rows, node)
			}
		}
	} else {
		for _, node := range s.nodes {
			if node.hasResult {
				rows = append(rows, node)
			}
		}
	}
	s.sortRows(rows)
	s.results = rows
	items := s.resultItems[:0]
	y := float32(8)
	for _, node := range rows {
		items = append(items, listItem{kind: itemNode, y: y, height: 36, node: node})
		y += 38
	}
	if len(items) > 0 {
		y += 8
	}
	s.resultItems = items
	s.resultContent = y
	s.clampResultOffset()
	s.refreshRunText()
}

// sortRows 按当前排序列排序；主列相等时在测行排前、再按稳定身份，保证
// 同一帧内顺序确定、行身份不随重排丢失。
func (s *state) sortRows(rows []*nodeState) {
	sort.SliceStable(rows, func(left, right int) bool {
		a, b := rows[left], rows[right]
		if cmp := s.compareRows(a, b); cmp != 0 {
			if s.sortAsc {
				return cmp < 0
			}
			return cmp > 0
		}
		rankA, rankB := phaseRank(a.phase), phaseRank(b.phase)
		if rankA != rankB {
			return rankA > rankB
		}
		return a.key < b.key
	})
}

// compareRows 比较两行的排序列，返回 -1/0/1。未测到的值统一当作最差
// （延迟、抖动取无穷大，速度与丢包按 0 参与比较）。
func (s *state) compareRows(a, b *nodeState) int {
	switch s.sortKey {
	case sortByLatency:
		return compareFloat(latencySortValue(a), latencySortValue(b))
	case sortByJitter:
		return compareFloat(latencySortValue2(a.result.JitterMS), latencySortValue2(b.result.JitterMS))
	case sortByLoss:
		return compareFloat(a.result.PacketLoss, b.result.PacketLoss)
	case sortByUpload:
		return compareFloat(a.result.UploadBps, b.result.UploadBps)
	case sortByTested:
		return compareFloat(float64(a.result.TestedAt.Unix()), float64(b.result.TestedAt.Unix()))
	case sortByName:
		return strings.Compare(a.name, b.name)
	default:
		return compareFloat(s.effectiveSpeed(a), s.effectiveSpeed(b))
	}
}

func latencySortValue(node *nodeState) float64 { return latencySortValue2(node.result.LatencyMS) }

func latencySortValue2(milliseconds float64) float64 {
	if milliseconds <= 0 {
		return math.MaxFloat64
	}
	return milliseconds
}

func compareFloat(left, right float64) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

func (s *state) effectiveSpeed(node *nodeState) float64 {
	switch node.phase {
	case phaseTesting:
		return node.liveSpeed
	case phaseQueued:
		return -1
	}
	if node.hasResult {
		return node.result.DownloadBps
	}
	return -1
}

func phaseRank(phase nodePhase) int {
	switch phase {
	case phaseTesting:
		return 3
	case phaseDone:
		return 2
	case phaseQueued:
		return 1
	default:
		return 0
	}
}

func (s *state) refreshRunText() {
	roundPrefix := ""
	if len(s.rounds) > 0 {
		roundPrefix = fmt.Sprintf("第 %d/%d 轮 · ", s.roundIndex+1, len(s.rounds))
	}
	if s.run == nil || !s.run.running {
		switch {
		case s.run != nil && s.run.total > 0:
			s.resultText = fmt.Sprintf("%s已完成 %d/%d", roundPrefix, s.run.done, s.run.total)
		case len(s.results) > 0:
			s.resultText = fmt.Sprintf("%s共 %d 条结果", roundPrefix, len(s.results))
		default:
			s.resultText = roundPrefix + "还没有结果"
		}
		if len(s.rounds) > 0 && s.roundIndex < len(s.rounds) {
			round := s.rounds[s.roundIndex]
			if round.done {
				s.resultText += " · 本轮完成"
				if round.early {
					s.resultText += "（提前结束）"
				}
			}
		}
		if s.exporting > 0 {
			s.resultText += " · " + s.exportText
		}
		return
	}
	s.run.text = fmt.Sprintf("%s已完成 %d/%d · 在测 %d", roundPrefix, s.run.done, s.run.total, s.run.active)
	if s.paused {
		s.run.text += " · 已暂停"
	}
	s.resultText = s.run.text
	if s.exporting > 0 {
		s.resultText += " · " + s.exportText
	}
}

func (s *state) running() bool { return s.run != nil && s.run.running }

func (s *state) refreshNodeDisplay(node *nodeState) {
	node.serverText = formatServer(node.proxy)
	node.subtitle = node.serverText + " · " + node.typ
	node.rowLabel = "节点 " + node.entry.entry.Name + " / " + node.name
	node.checkLabel = "选择节点 " + node.entry.entry.Name + " / " + node.name
	if node.typ == "" && node.proxy != nil {
		node.typ = node.proxy.Type().String()
		node.subtitle = node.serverText + " · " + node.typ
	}
	node.lastTestText = formatTested(node.result.TestedAt)
	node.latencyText = formatLatency(durationMillis(node.result.LatencyMS))
	node.jitterText = formatLatency(durationMillis(node.result.JitterMS))
	node.lossText = formatLoss(node.result.PacketLoss)
	node.uploadText = formatSpeed(node.result.UploadBps)
	node.summaryText = ""
	switch {
	case node.phase == phaseTesting:
		node.speedText = formatSpeed(node.liveSpeed)
		node.statusText = "测试中"
	case node.phase == phaseQueued:
		node.speedText = "—"
		node.statusText = "等待中"
	case node.hasResult:
		node.speedText = formatSpeed(node.result.DownloadBps)
		if node.result.OK {
			node.statusText = "完成"
			node.summaryText = node.speedText
		} else {
			node.statusText = "失败"
			node.summaryText = "失败"
		}
	default:
		node.speedText = "—"
		node.statusText = ""
	}
	node.infoText = node.lastTestText
	if node.summaryText != "" {
		node.infoText += " · " + node.summaryText
	}
	node.clipText = clipboardText(node)
}

func durationMillis(value float64) time.Duration {
	return time.Duration(value * float64(time.Millisecond))
}

func (s *state) notify(message string) { s.status = message }

// noteSaved 报告一次本应持久化的操作。数据目录不可用时，成功文案必须带上
// 未保存提示，不能让用户以为已经落盘。
func (s *state) noteSaved(message string) {
	if s.store == nil {
		s.status = message + "（数据目录不可用，未保存）"
		return
	}
	s.status = message
}

func (s *state) storeError(action string, err error) {
	if err == nil {
		return
	}
	s.status = action + "失败：" + err.Error()
}

func (s *state) close() {
	if s.closed {
		return
	}
	s.closed = true
	if s.run != nil && s.run.running {
		s.stopTest()
	}
	s.persistSettings()
}

func (s *state) showAlert(title, body string) {
	s.alert = alertState{open: true, title: title, body: body}
	s.dialog = dialogAlert
}

func (s *state) entryByID(id string) *entryState {
	for _, entry := range s.entries {
		if entry.entry.ID == id {
			return entry
		}
	}
	return nil
}
