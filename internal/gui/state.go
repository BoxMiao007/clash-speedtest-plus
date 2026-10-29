package gui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
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

// viewKind 是主内容区的两种视图；下载速度按钮会切到 viewResults。
type viewKind uint8

const (
	viewNodes viewKind = iota
	viewResults
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

	engine      Runner
	run         *runState
	pendingMu   sync.Mutex
	pending     []runEvent
	invalidator *app.Invalidator
	closed      bool
}

type entryState struct {
	entry      store.Entry
	ids        entryIDs
	selected   bool
	loaded     bool
	loadErr    string
	missing    bool
	nodes      []*nodeState
	group      *groupState
	checkLabel string

	pathText   string
	absText    string
	statusText string
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
	lossText     string
	speedText    string
	statusText   string
	clipText     string
}

type entryForm struct {
	editing  bool
	id       string
	name     flex.Field
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
	Finished func(error)
}

// Runner 是 GUI 与测速引擎之间的窄接口；生产实现包 internal/speedtester，
// 测试注入假实现，避免测试联网。
type Runner interface {
	Run(ctx context.Context, batch []RunNode, handlers Handlers) error
	Stop()
}

type runEventKind uint8

const (
	eventStart runEventKind = iota
	eventProgress
	eventResult
	eventFinished
)

type runEvent struct {
	kind     runEventKind
	key      string
	progress speedtester.Progress
	result   *speedtester.Result
	err      error
}

type runState struct {
	running bool
	stopped bool
	total   int
	done    int
	active  int
	batch   []*nodeState
	cancel  context.CancelFunc
	text    string
}

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
		tree:      window,
		ids:       newIDs(window),
		baseDir:   baseDir,
		dataDir:   dataDir,
		theme:     theme,
		active:    viewNodes,
		presetSel: -1,
		byKey:     make(map[string]*nodeState),
		status:    "就绪",
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
		settings := database.Settings()
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
		s.readEntry(entry)
	}
	if len(s.entries) > 0 {
		s.status = fmt.Sprintf("已加载 %d 个测试文件", len(s.entries))
	} else if s.storeErr == "" {
		s.status = "还没有测试文件条目，先新建一个"
	}
}

func (s *state) decorateEntry(entry *entryState) {
	entry.checkLabel = "选择条目 " + entry.entry.Name
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

// readEntry 读取一个条目文件，并把节点列表与已有状态按稳定身份合并：
// 身份匹配的节点保留勾选与测试结果，新节点默认勾选，消失的节点移除。
func (s *state) readEntry(entry *entryState) {
	if s.running() {
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
	if s.store == nil {
		s.status = "数据目录不可用，过滤设置未保存"
		return
	}
	err := s.store.SetSettings(store.Settings{
		FilterMode:       string(s.filterMode),
		FilterExpression: s.filterField.Text(),
		FilterCombine:    string(s.filterCombine),
	})
	if err != nil {
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

// rebuildResults 生成下载速度视图的行：当前批次 + 历史结果，按有效下载速度
// 降序；测速中的行用瞬时速度参与排序。排序只在状态变化时做，不在 Paint 里做。
func (s *state) rebuildResults() {
	rows := s.results[:0]
	batch := make(map[string]bool)
	if s.run != nil {
		for _, node := range s.run.batch {
			batch[node.key] = true
		}
	}
	for _, node := range s.nodes {
		if node.hasResult || batch[node.key] || node.phase == phaseTesting || node.phase == phaseQueued {
			rows = append(rows, node)
		}
	}
	sort.Slice(rows, func(left, right int) bool {
		a, b := rows[left], rows[right]
		speedA, speedB := s.effectiveSpeed(a), s.effectiveSpeed(b)
		if speedA != speedB {
			return speedA > speedB
		}
		rankA, rankB := phaseRank(a.phase), phaseRank(b.phase)
		if rankA != rankB {
			return rankA > rankB
		}
		return a.key < b.key
	})
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
	if s.run == nil {
		s.resultText = fmt.Sprintf("共 %d 条结果", len(s.results))
		return
	}
	if s.run.running {
		s.run.text = fmt.Sprintf("已完成 %d/%d · 在测 %d", s.run.done, s.run.total, s.run.active)
	} else if s.run.stopped {
		s.run.text = fmt.Sprintf("已停止 · 完成 %d/%d", s.run.done, s.run.total)
	} else {
		s.run.text = fmt.Sprintf("已完成 %d/%d", s.run.done, s.run.total)
	}
	s.resultText = s.run.text
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
	node.latencyText = formatLatency(time.Duration(node.result.LatencyMS * float64(time.Millisecond)))
	node.lossText = formatLoss(node.result.PacketLoss)
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
