package gui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BoxMiao007/clash-speedtest-plus/internal/filter"
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
		s.status = "先勾选要读取的测试文件"
		return
	}
	s.status = "已读取 " + itoa(count) + " 个测试文件"
}

func (s *state) refetchEntry(entry *entryState) {
	if s.running() {
		s.status = "测速进行中，先停止再重新获取"
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
	s.startNodesTest(s.selectedNodes())
}

func (s *state) startNodesTest(nodes []*nodeState) {
	if s.running() {
		s.status = "测速进行中，先停止本轮"
		return
	}
	batch := make([]*nodeState, 0, len(nodes))
	for _, node := range nodes {
		if node == nil || node.proxy == nil {
			continue
		}
		batch = append(batch, node)
	}
	if len(batch) == 0 {
		s.status = "请先勾选要测试的节点"
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	run := &runState{
		running: true,
		total:   len(batch),
		batch:   append([]*nodeState(nil), batch...),
		cancel:  cancel,
	}
	s.run = run
	s.active = viewResults
	for _, node := range batch {
		node.phase = phaseQueued
		node.liveSpeed = 0
		s.refreshNodeDisplay(node)
	}
	s.status = "开始测试 " + itoa(len(batch)) + " 个节点"
	s.rebuild()
	s.refreshRunText()

	runNodes := make([]RunNode, 0, len(batch))
	for _, node := range batch {
		runNodes = append(runNodes, RunNode{Key: node.key, Proxy: node.proxy})
	}
	go func() {
		err := s.engine.Run(ctx, runNodes, Handlers{
			Start: func(key, _ string) {
				s.enqueue(runEvent{kind: eventStart, key: key})
			},
			Progress: func(progress speedtester.Progress) {
				s.enqueue(runEvent{kind: eventProgress, progress: progress})
			},
			Result: func(result *speedtester.Result) {
				s.enqueue(runEvent{kind: eventResult, result: result})
			},
		})
		s.enqueue(runEvent{kind: eventFinished, err: err})
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
		if s.run == nil {
			return
		}
		s.run.running = false
		s.run.active = 0
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
		switch {
		case event.err != nil && !s.run.stopped:
			s.status = "测速失败：" + event.err.Error()
		case s.run.stopped:
			s.status = "已停止测速"
		default:
			s.status = "测速完成"
		}
		if s.store == nil {
			s.status += "（数据目录不可用，结果未保存）"
		}
	}
}

func (s *state) openEntryDialog(entry *entryState) {
	s.dialog = dialogEntry
	s.entryForm.err = ""
	if entry == nil {
		s.entryForm.editing = false
		s.entryForm.id = ""
		s.entryForm.name.SetText("")
		s.entryForm.path = ""
		s.entryForm.pathText = "未选择文件"
		return
	}
	s.entryForm.editing = true
	s.entryForm.id = entry.entry.ID
	s.entryForm.name.SetText(entry.entry.Name)
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

	entry := &entryState{entry: store.Entry{ID: s.entryForm.id, Name: name, Path: stored}}
	if entry.entry.ID == "" {
		entry.entry.ID = store.NewID()
	}
	if s.store != nil {
		if err := s.store.PutEntry(entry.entry); err != nil {
			s.entryForm.err = "保存条目失败：" + err.Error()
			return
		}
	}

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
	existing.entry = entry.entry
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
