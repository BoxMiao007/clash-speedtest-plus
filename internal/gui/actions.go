package gui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BoxMiao007/clash-speedtest-plus/internal/filter"
	"github.com/BoxMiao007/clash-speedtest-plus/internal/output"
	"github.com/BoxMiao007/clash-speedtest-plus/internal/speedtester"
	"github.com/BoxMiao007/clash-speedtest-plus/internal/store"
)

func (s *state) toggleEntry(entry *entryState) {
	if s.running() {
		return
	}
	entry.selected = !entry.selected
	if entry.selected {
		s.readEntry(entry)
	} else {
		s.status = "已隐藏条目：" + entry.entry.Name
		s.rebuild()
	}
}

func (s *state) readSelectedEntries() {
	if s.running() {
		s.status = "测速进行中，先停止再读取节点"
		return
	}
	count := 0
	for _, entry := range s.entries {
		if !entry.selected {
			continue
		}
		s.readEntry(entry)
		count++
	}
	if count == 0 {
		s.status = "先勾选要读取的条目"
		return
	}
	s.status = "已读取 " + itoa(count) + " 个条目"
}

func (s *state) refetchEntry(entry *entryState) {
	if s.running() {
		s.status = "测速进行中，先停止再重新获取"
		return
	}
	if entry.entry.IsSubscription() {
		s.readSubscription(entry, true)
		return
	}
	s.readEntry(entry)
	if entry.loadErr == "" {
		s.status = "已重新获取节点：" + entry.entry.Name + "（" + itoa(len(entry.nodes)) + " 个）"
	} else {
		s.status = "重新获取失败：" + entry.loadErr
	}
}

func (s *state) toggleNode(node *nodeState) {
	node.selected = !node.selected
	s.rebuild()
}

func (s *state) setGroupSelected(group *groupState, selected bool) {
	for _, node := range group.nodes {
		node.selected = selected
	}
	s.rebuild()
}

func (s *state) excludeFailed() {
	count := 0
	for _, entry := range s.entries {
		if !entry.selected {
			continue
		}
		for _, node := range entry.nodes {
			if node.selected && node.hasResult && !node.result.OK {
				node.selected = false
				count++
			}
		}
	}
	s.rebuild()
	if count == 0 {
		s.status = "没有上次失败且仍被勾选的节点"
		return
	}
	s.status = "已排除 " + itoa(count) + " 个上次失败的节点"
}

func (s *state) selectedNodes() []*nodeState {
	nodes := make([]*nodeState, 0, s.selectedCount)
	for _, entry := range s.entries {
		if !entry.selected {
			continue
		}
		for _, node := range entry.nodes {
			if node.selected {
				nodes = append(nodes, node)
			}
		}
	}
	return nodes
}

func (s *state) startSelectedTest() {
	rounds := make([]*roundState, 0, len(s.entries))
	for _, entry := range s.entries {
		if !entry.selected {
			continue
		}
		nodes := make([]*nodeState, 0, len(entry.nodes))
		for _, node := range entry.nodes {
			if node.selected && node.proxy != nil && s.match(node.name) {
				nodes = append(nodes, node)
			}
		}
		if len(nodes) == 0 {
			continue
		}
		rounds = append(rounds, &roundState{entry: entry, nodes: nodes})
	}
	if len(rounds) == 0 {
		if s.selectedCount == 0 {
			s.status = "请先勾选要测试的节点"
		} else {
			s.status = "当前过滤条件下没有可测试的节点"
		}
		return
	}
	s.startRounds(rounds)
}

// startNodesTest 立即测试指定节点（右键菜单）。按条目分组，每组一轮。
func (s *state) startNodesTest(nodes []*nodeState) {
	if s.running() {
		s.status = "测速进行中，先停止本轮"
		return
	}
	rounds := make([]*roundState, 0, 2)
	index := make(map[*entryState]int, 2)
	for _, node := range nodes {
		if node == nil || node.proxy == nil {
			continue
		}
		position, ok := index[node.entry]
		if !ok {
			position = len(rounds)
			index[node.entry] = position
			rounds = append(rounds, &roundState{entry: node.entry})
		}
		rounds[position].nodes = append(rounds[position].nodes, node)
	}
	if len(rounds) == 0 {
		s.status = "请先勾选要测试的节点"
		return
	}
	s.startRounds(rounds)
}

// startRounds 开始一轮或多轮测速：每个选中条目各自一轮，按条目顺序推进。
func (s *state) startRounds(rounds []*roundState) {
	if s.running() {
		s.status = "测速进行中，先停止本轮"
		return
	}
	s.rounds = rounds
	s.roundIndex = 0
	s.earlyStopped = false
	s.paused = false
	s.detailKey = ""
	s.detailNode = nil
	s.exportSeq = output.NextImageSequence(s.baseDir)
	s.active = viewResults
	s.startRound(0)
}

// startRound 启动第 index 轮。每轮独立配置引擎，结果只落到本轮节点。
func (s *state) startRound(index int) {
	if index < 0 || index >= len(s.rounds) {
		return
	}
	round := s.rounds[index]
	round.done = false
	round.stopped = false
	round.early = false
	round.exported = false
	round.err = ""
	s.earlyStopped = false
	s.paused = false
	s.refreshModeFlags()

	ctx, cancel := context.WithCancel(context.Background())
	run := &runState{
		running: true,
		total:   len(round.nodes),
		batch:   round.nodes,
		cancel:  cancel,
		round:   index,
	}
	s.run = run
	for _, node := range round.nodes {
		node.phase = phaseQueued
		node.liveSpeed = 0
		s.refreshNodeDisplay(node)
	}
	if len(s.rounds) > 1 {
		s.status = fmt.Sprintf("第 %d/%d 轮开始：%s", index+1, len(s.rounds), round.entry.entry.Name)
	} else {
		s.status = "开始测试 " + itoa(len(round.nodes)) + " 个节点"
	}
	s.engine.Configure(testConfigFromSettings(s.settings))
	s.rebuild()

	runNodes := make([]RunNode, 0, len(round.nodes))
	for _, node := range round.nodes {
		runNodes = append(runNodes, RunNode{Key: node.key, Proxy: node.proxy})
	}
	roundIndex := index
	go func() {
		err := s.engine.Run(ctx, runNodes, Handlers{
			Start: func(key, _ string) {
				s.enqueue(runEvent{kind: eventStart, key: key, round: roundIndex})
			},
			Progress: func(progress speedtester.Progress) {
				s.enqueue(runEvent{kind: eventProgress, progress: progress, round: roundIndex})
			},
			Result: func(result *speedtester.Result) {
				s.enqueue(runEvent{kind: eventResult, result: result, round: roundIndex})
			},
			EarlyStop: func() {
				s.enqueue(runEvent{kind: eventEarlyStop, round: roundIndex})
			},
		})
		s.enqueue(runEvent{kind: eventFinished, err: err, round: roundIndex})
	}()
}

func (s *state) stopTest() {
	if s.run == nil || !s.run.running {
		return
	}
	s.run.stopped = true
	if s.run.cancel != nil {
		s.run.cancel()
	}
	s.engine.Stop()
	s.status = "正在停止测速…"
}

// togglePause 在测速过程中暂停/继续派发新节点；在测节点会跑完手头这一批。
func (s *state) togglePause() {
	if s.run == nil || !s.run.running {
		return
	}
	if s.paused {
		s.paused = false
		s.run.paused = false
		s.engine.Resume()
		s.status = "已继续测速"
	} else {
		s.paused = true
		s.run.paused = true
		s.engine.Pause()
		s.status = "已暂停测速"
	}
	s.refreshModeFlags()
	s.refreshRunText()
}

// switchRound 在多轮队列里前后查看某一轮；测速中只切视图，不影响正在跑的轮。
func (s *state) switchRound(delta int) {
	if len(s.rounds) == 0 {
		return
	}
	next := s.roundIndex + delta
	if next < 0 || next >= len(s.rounds) {
		return
	}
	s.roundIndex = next
	s.detailKey = ""
	s.detailNode = nil
	s.rebuild()
	s.status = fmt.Sprintf("第 %d/%d 轮：%s", next+1, len(s.rounds), s.rounds[next].entry.entry.Name)
}

func (s *state) enqueue(event runEvent) {
	s.pendingMu.Lock()
	s.pending = append(s.pending, event)
	s.pendingMu.Unlock()
	if s.invalidator != nil {
		s.invalidator.Invalidate()
	}
}

// applyPending 由 OnWake 在窗口线程调用：把后台事件并入模型、重排结果。
func (s *state) applyPending() {
	s.pendingMu.Lock()
	events := s.pending
	s.pending = nil
	s.pendingMu.Unlock()
	if len(events) == 0 {
		return
	}
	for _, event := range events {
		s.applyEvent(event)
	}
	s.refreshRunText()
	s.rebuildResults()
}

func (s *state) applyEvent(event runEvent) {
	switch event.kind {
	case eventStart:
		if s.run == nil || event.round != s.run.round {
			return
		}
		node := s.byKey[event.key]
		if node == nil {
			return
		}
		node.phase = phaseTesting
		if s.run != nil {
			s.run.active++
		}
		s.refreshNodeDisplay(node)
	case eventProgress:
		if s.run == nil || event.round != s.run.round {
			return
		}
		node := s.byKey[event.progress.Name]
		if node == nil {
			return
		}
		node.phase = phaseTesting
		node.liveSpeed = event.progress.InstantSpeed
		if event.progress.Phase == speedtester.PhaseDownload && event.progress.DownloadSpeed > 0 {
			node.liveSpeed = event.progress.DownloadSpeed
		}
		if event.progress.Latency > 0 {
			node.result.LatencyMS = float64(event.progress.Latency.Milliseconds())
			node.result.PacketLoss = event.progress.PacketLoss
		}
		s.refreshNodeDisplay(node)
	case eventResult:
		if s.run == nil || event.round != s.run.round {
			return
		}
		node := s.byKey[event.result.ProxyName]
		if node == nil {
			return
		}
		stored := resultToStored(event.result, time.Now())
		node.result = stored
		node.hasResult = true
		node.phase = phaseDone
		node.liveSpeed = stored.DownloadBps
		s.refreshNodeDisplay(node)
		if s.run != nil {
			s.run.done++
			if s.run.active > 0 {
				s.run.active--
			}
		}
		if s.store != nil {
			if err := s.store.PutResult(node.entry.entry.ID, node.id, stored); err != nil {
				s.status = "保存测速结果失败：" + err.Error()
			}
		}
	case eventFinished:
		if s.run == nil || event.round != s.run.round {
			return
		}
		round := s.currentRound()
		s.run.running = false
		s.run.active = 0
		s.paused = false
		if s.run.cancel != nil {
			s.run.cancel()
			s.run.cancel = nil
		}
		for _, node := range s.run.batch {
			if node.phase == phaseQueued {
				node.phase = phaseIdle
				s.refreshNodeDisplay(node)
			}
		}
		if round != nil {
			round.done = !s.run.stopped
			round.stopped = s.run.stopped
			round.early = s.earlyStopped
			if event.err != nil && !s.run.stopped {
				round.err = event.err.Error()
			}
		}
		switch {
		case s.run.stopped:
			s.status = "已停止测速"
			s.rebuild()
			return
		case event.err != nil:
			s.status = "测速失败：" + event.err.Error()
			s.rebuild()
			return
		case s.earlyStopped:
			s.status = "本轮已提前结束（达到过筛限额）"
			if round != nil {
				round.early = true
			}
		default:
			s.status = "本轮测速完成"
		}
		s.rebuildResults()
		s.afterRoundFinished()
	case eventEarlyStop:
		if s.run == nil || event.round != s.run.round {
			return
		}
		s.earlyStopped = true
		if round := s.currentRound(); round != nil {
			round.early = true
		}
		s.status = "已提前结束（达到过筛限额）"
		s.rebuild()
	case eventFetched:
		s.applyFetched(event)
	case eventExported:
		s.applyExported(event)
	}
}

// currentRound 返回正在看的这一轮；没有轮次时返回 nil。
func (s *state) currentRound() *roundState {
	if s.roundIndex < 0 || s.roundIndex >= len(s.rounds) {
		return nil
	}
	return s.rounds[s.roundIndex]
}

// afterRoundFinished 在本轮正常结束后先导出（输出开着时），导出完成再推进
// 下一轮；没有导出任务时直接推进。停止或失败不推进、不导出。
func (s *state) afterRoundFinished() {
	round := s.currentRound()
	if round == nil {
		return
	}
	if round.stopped || round.err != "" {
		return
	}
	if s.needsExport() {
		if s.exporting > 0 {
			// 上一轮还在写盘：挂起本轮，等那次导出结束后接着写，避免并发写同名产物。
			s.pendingExport = round
			return
		}
		s.startRoundExport(round)
		return
	}
	s.advanceRound()
}

// advanceRound 推进到下一轮；已是最后一轮时收尾（合并导出）。
func (s *state) advanceRound() {
	next := s.roundIndex + 1
	if next < len(s.rounds) {
		s.roundIndex = next
		s.detailKey = ""
		s.detailNode = nil
		s.startRound(next)
		return
	}
	s.allRoundsDone()
}

// allRoundsDone 在所有轮结束后写合并导出（≥2 轮、输出开着、全部正常完成）。
func (s *state) allRoundsDone() {
	merged := s.writeMergedExport()
	if merged != "" {
		s.status = "全部轮次完成，已保存合并配置：" + merged
	} else if len(s.rounds) > 1 {
		s.status = "全部轮次完成"
	}
	s.rebuild()
}

// applyFetched 处理订阅拉取结果：写缓存、解析节点、按身份合并。
func (s *state) applyFetched(event runEvent) {
	entry := s.entryByID(event.key)
	if entry == nil {
		return
	}
	entry.fetching = false
	if event.err != nil {
		entry.loadErr = event.err.Error()
		s.status = "拉取订阅失败：" + entry.entry.Name + "（" + event.err.Error() + "）"
		s.rebuild()
		return
	}
	if s.store == nil {
		entry.loadErr = "数据目录不可用，无法缓存订阅"
		s.status = entry.loadErr
		s.rebuild()
		return
	}
	path, err := s.store.WriteSubscription(entry.entry.ID, event.body)
	if err != nil {
		entry.loadErr = err.Error()
		s.status = "写入订阅缓存失败：" + err.Error()
		s.rebuild()
		return
	}
	entry.absText = "缓存 · " + path
	proxies, err := speedtester.ParseSubscriptionNodes(event.body, s.settings.UserAgent)
	if err != nil {
		entry.loadErr = err.Error()
		s.status = "解析订阅失败：" + err.Error()
		s.rebuild()
		return
	}
	s.applyNodes(entry, proxies)
	if event.path != "" && event.path != entry.entry.URL {
		s.status = "已用补过参数的地址拉取：" + event.path
	} else {
		s.status = "已拉取订阅 " + entry.entry.Name + "：" + itoa(len(proxies)) + " 个节点"
	}
}

// applyExported 处理一次导出结束：记录状态；轮次导出完成后推进下一轮，
// 手动导出只更新状态。导出的轮次用指针记录，队列被新一轮替换后旧事件
// 只更新状态，不碰轮次与推进。
func (s *state) applyExported(event runEvent) {
	if s.exporting > 0 {
		s.exporting--
	}
	ref := event.roundRef
	if ref == nil {
		s.setExportStatus(event)
		s.rebuild()
		return
	}
	if event.err == nil {
		ref.exported = true
		ref.proxies = event.proxies
	}
	s.setExportStatus(event)
	// 新一轮已经开始，或这一轮已不在当前队列里：只更新状态。
	if s.running() || s.roundPosition(ref) < 0 {
		s.pendingExport = nil
		s.rebuild()
		return
	}
	if event.err != nil {
		ref.err = event.err.Error()
		s.pendingExport = nil
		s.rebuild()
		return
	}
	if s.pendingExport != nil {
		pending := s.pendingExport
		s.pendingExport = nil
		s.startRoundExport(pending)
		s.rebuild()
		return
	}
	if ref != s.currentRound() {
		s.rebuild()
		return
	}
	s.advanceRound()
}

// setExportStatus 把一次导出的结果写进状态栏。
func (s *state) setExportStatus(event runEvent) {
	switch {
	case event.err != nil:
		s.status = "导出失败：" + event.err.Error()
	case event.warning != "":
		s.status = "已导出：" + event.path + "（" + event.warning + "）"
	case event.path != "":
		s.status = "已导出：" + event.path
	}
}

// roundPosition 返回轮次在队列里的位置；不在当前队列返回 -1。
func (s *state) roundPosition(round *roundState) int {
	for index, candidate := range s.rounds {
		if candidate == round {
			return index
		}
	}
	return -1
}

func (s *state) openEntryDialog(entry *entryState) {
	s.dialog = dialogEntry
	s.entryForm.err = ""
	if entry == nil {
		s.entryForm.editing = false
		s.entryForm.id = ""
		s.entryForm.kind = 0
		s.entryForm.name.SetText("")
		s.entryForm.url.SetText("")
		s.entryForm.path = ""
		s.entryForm.pathText = "未选择文件"
		return
	}
	s.entryForm.editing = true
	s.entryForm.id = entry.entry.ID
	s.entryForm.kind = 0
	s.entryForm.name.SetText(entry.entry.Name)
	s.entryForm.url.SetText(entry.entry.URL)
	if entry.entry.IsSubscription() {
		s.entryForm.kind = 1
		s.entryForm.path = ""
		s.entryForm.pathText = "订阅条目直接使用地址"
		return
	}
	absolute, _ := s.resolvePath(entry.entry.Path)
	s.entryForm.path = absolute
	s.entryForm.pathText = absolute
}

func (s *state) chooseEntryPath() {
	paths, err := appOpenFiles()
	if err != nil {
		if isCanceled(err) {
			return
		}
		s.entryForm.err = "选择文件失败：" + err.Error()
		return
	}
	if len(paths) == 0 {
		return
	}
	s.entryForm.path = paths[0]
	s.entryForm.pathText = paths[0]
	s.entryForm.err = ""
}

func (s *state) saveEntryForm() {
	name := strings.TrimSpace(s.entryForm.name.Text())
	if name == "" {
		s.entryForm.err = "名称不能为空"
		return
	}
	for _, entry := range s.entries {
		if entry.entry.ID == s.entryForm.id {
			continue
		}
		if entry.entry.Name == name {
			s.entryForm.err = "名称已存在"
			return
		}
	}
	if s.entryForm.kind == 1 {
		s.saveSubscriptionEntry(name)
		return
	}
	absolute := strings.TrimSpace(s.entryForm.path)
	if absolute == "" {
		s.entryForm.err = "请选择配置文件"
		return
	}
	if !filepath.IsAbs(absolute) {
		absolute = filepath.Join(s.baseDir, filepath.FromSlash(absolute))
	}
	info, err := os.Stat(absolute)
	if err != nil {
		s.entryForm.err = "文件不存在或无法访问"
		return
	}
	if info.IsDir() {
		s.entryForm.err = "请选择文件，不是目录"
		return
	}
	stored := s.storePath(absolute)

	entry := &entryState{entry: store.Entry{ID: s.entryForm.id, Name: name, Kind: store.EntryKindLocal, Path: stored}}
	if entry.entry.ID == "" {
		entry.entry.ID = store.NewID()
	}
	if s.store != nil {
		if err := s.store.PutEntry(entry.entry); err != nil {
			s.entryForm.err = "保存条目失败：" + err.Error()
			return
		}
	}
	s.applyEntryForm(entry, name)
}

// saveSubscriptionEntry 保存订阅条目：地址必须是 http(s)，新建后立即异步
// 拉取；地址改过的已有条目也强制重新拉取。
func (s *state) saveSubscriptionEntry(name string) {
	url := strings.TrimSpace(s.entryForm.url.Text())
	if url == "" {
		s.entryForm.err = "订阅地址不能为空"
		return
	}
	lower := strings.ToLower(url)
	if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
		s.entryForm.err = "订阅地址必须以 http:// 或 https:// 开头"
		return
	}
	previous := s.entryByID(s.entryForm.id)
	entry := &entryState{entry: store.Entry{
		ID:   s.entryForm.id,
		Name: name,
		Kind: store.EntryKindSubscription,
		URL:  url,
	}}
	if entry.entry.ID == "" {
		entry.entry.ID = store.NewID()
	}
	if s.store != nil {
		if err := s.store.PutEntry(entry.entry); err != nil {
			s.entryForm.err = "保存条目失败：" + err.Error()
			return
		}
	}
	urlChanged := previous == nil || !previous.entry.IsSubscription() || previous.entry.URL != url
	s.applyEntryForm(entry, name)
	if target := s.entryByID(entry.entry.ID); target != nil && urlChanged {
		s.readSubscription(target, true)
	}
}

// applyEntryForm 把表单结果并回条目列表；新建条目直接读取，已有条目更新后
// 重新读取（本地文件同步、订阅按缓存或强制拉取规则）。
func (s *state) applyEntryForm(entry *entryState, name string) {
	existing := s.entryByID(entry.entry.ID)
	if existing == nil {
		entry.ids = registerEntryIDs(s.tree, s.ids.page, entry.entry.ID)
		entry.selected = true
		s.decorateEntry(entry)
		s.entries = append(s.entries, entry)
		s.dialog = dialogNone
		s.readEntry(entry)
		s.noteSaved("已新建条目：" + name)
		return
	}
	wasSubscription := existing.entry.IsSubscription()
	existing.entry = entry.entry
	existing.loaded = existing.loaded && wasSubscription == entry.entry.IsSubscription()
	s.decorateEntry(existing)
	s.dialog = dialogNone
	s.readEntry(existing)
	s.noteSaved("已更新条目：" + name)
}

func (s *state) askDeleteEntry(entry *entryState) {
	s.dialog = dialogConfirm
	s.confirm = confirmState{
		kind:    confirmDeleteEntry,
		entryID: entry.entry.ID,
		title:   "删除条目",
		body:    "删除「" + entry.entry.Name + "」？该条目的测速结果会一并删除，此操作不可撤销。",
	}
}

func (s *state) askDeletePreset() {
	if s.presetSel < 0 || s.presetSel >= len(s.presets) {
		return
	}
	preset := s.presets[s.presetSel]
	s.dialog = dialogConfirm
	s.confirm = confirmState{
		kind:       confirmDeletePreset,
		presetName: preset.Name,
		title:      "删除预设",
		body:       "删除过滤预设「" + preset.Name + "」？",
	}
}

func (s *state) confirmDelete() {
	switch s.confirm.kind {
	case confirmDeleteEntry:
		if s.store != nil {
			if err := s.store.DeleteEntry(s.confirm.entryID); err != nil {
				s.status = "删除条目失败：" + err.Error()
				s.dialog = dialogNone
				return
			}
		}
		kept := s.entries[:0]
		for _, entry := range s.entries {
			if entry.entry.ID != s.confirm.entryID {
				kept = append(kept, entry)
			}
		}
		s.entries = kept
		s.rebuild()
		s.noteSaved("已删除条目")
	case confirmDeletePreset:
		if s.store != nil {
			if err := s.store.DeletePreset(s.confirm.presetName); err != nil {
				s.status = "删除预设失败：" + err.Error()
				s.dialog = dialogNone
				return
			}
		}
		s.loadPresets()
		s.presetSel = -1
		s.status = "已删除预设"
	}
	s.dialog = dialogNone
}

func (s *state) openPresetSaveDialog() {
	s.dialog = dialogPreset
	s.presetForm.rename = false
	s.presetForm.oldName = ""
	s.presetForm.err = ""
	s.presetForm.name.SetText("")
	if s.presetSel >= 0 && s.presetSel < len(s.presets) {
		s.presetForm.name.SetText(s.presets[s.presetSel].Name)
	}
}

func (s *state) openPresetRenameDialog() {
	if s.presetSel < 0 || s.presetSel >= len(s.presets) {
		return
	}
	s.dialog = dialogPreset
	s.presetForm.rename = true
	s.presetForm.oldName = s.presets[s.presetSel].Name
	s.presetForm.err = ""
	s.presetForm.name.SetText(s.presets[s.presetSel].Name)
}

func (s *state) savePresetForm() {
	name := strings.TrimSpace(s.presetForm.name.Text())
	if name == "" {
		s.presetForm.err = "预设名称不能为空"
		return
	}
	if s.presetForm.rename {
		old := s.presetForm.oldName
		if name != old && s.presetIndex(name) >= 0 {
			s.presetForm.err = "预设名称已存在"
			return
		}
		index := s.presetIndex(old)
		if index < 0 {
			s.presetForm.err = "原预设已不存在"
			return
		}
		preset := s.presets[index]
		preset.Name = name
		if s.store != nil {
			if err := s.store.PutPreset(preset); err != nil {
				s.presetForm.err = "重命名失败：" + err.Error()
				return
			}
			if name != old {
				if err := s.store.DeletePreset(old); err != nil {
					s.presetForm.err = "重命名失败：" + err.Error()
					return
				}
			}
		}
		s.loadPresets()
		s.presetSel = s.presetIndex(name)
		s.noteSaved("已重命名预设：" + name)
	} else {
		preset := store.Preset{
			Name:       name,
			Mode:       string(s.filterMode),
			Expression: s.filterField.Text(),
			Combine:    string(s.filterCombine),
		}
		if s.store != nil {
			if err := s.store.PutPreset(preset); err != nil {
				s.presetForm.err = "保存预设失败：" + err.Error()
				return
			}
		}
		s.loadPresets()
		s.presetSel = s.presetIndex(name)
		s.noteSaved("已保存预设：" + name)
	}
	s.dialog = dialogNone
}

func (s *state) applyPreset(index int) {
	if index < 0 || index >= len(s.presets) {
		return
	}
	preset := s.presets[index]
	s.presetSel = index
	s.filterMode = filter.Mode(preset.Mode)
	s.filterCombine = filter.Combine(preset.Combine)
	s.filterField.SetText(preset.Expression)
	s.compileFilter()
	s.persistSettings()
	s.noteSaved("已应用预设：" + preset.Name)
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	negative := value < 0
	if negative {
		value = -value
	}
	var buffer [20]byte
	index := len(buffer)
	for value > 0 {
		index--
		buffer[index] = byte('0' + value%10)
		value /= 10
	}
	if negative {
		index--
		buffer[index] = '-'
	}
	return string(buffer[index:])
}
