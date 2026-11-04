package gui

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/BoxMiao007/clash-speedtest-plus/internal/store"
	"github.com/vivaui/viva/ui"
)

// onFrameChange 记录窗口在屏幕上的位置与尺寸。拖动窗口会高频回调，这里只
// 更新内存；真正写盘发生在设置保存或退出时，窗口线程不做 I/O。
func (a *App) onFrameChange(frame ui.Rect) {
	a.windowFrame = frame
	a.windowPlaced = true
}

// captureWindowFrame 把最近一次窗口几何并入 settings 的内存副本，随下一次
// SetSettings 一起落盘。只读数据目录下什么也不写。
func (s *state) captureWindowFrame() {
	if !s.windowPlaced {
		return
	}
	s.settings.WindowWidth = int(s.windowFrame.Width)
	s.settings.WindowHeight = int(s.windowFrame.Height)
	s.settings.WindowX = int(s.windowFrame.X)
	s.settings.WindowY = int(s.windowFrame.Y)
	s.settings.WindowSet = true
	s.settings = s.settings.Normalize()
}

// onFileDrop 处理系统拖入的文件：移动经过时提示可松开，松开后把每个 YAML
// 配置添加成本地条目；其它类型忽略并说明原因。
func (a *App) onFileDrop(event ui.FileDropEvent) ui.EventResult {
	// 模态对话框打开时忽略文件拖放，避免在录入时后台添加条目。
	if a.dialog != dialogNone {
		return ui.EventResult{}
	}
	switch event.Phase {
	case ui.FileDropOver:
		if a.dropActive {
			return ui.EventResult{Handled: true}
		}
		a.dropActive = true
		a.dropText = "松开鼠标，把配置文件添加为本地条目"
		return ui.EventResult{Handled: true, Repaint: true}
	case ui.FileDropLeave:
		if !a.dropActive {
			return ui.EventResult{}
		}
		a.dropActive = false
		return ui.EventResult{Handled: true, Repaint: true}
	case ui.FileDropDrop:
		a.dropActive = false
		a.addDroppedFiles(event.Paths)
		return ui.EventResult{Handled: true, Repaint: true}
	}
	return ui.EventResult{}
}

// addDroppedFiles 把拖入的配置文件逐条加入条目列表并立即读取节点。目录与
// 非 YAML 文件跳过，最后用一条状态说明结果，避免用户不知道哪几个没生效。
func (s *state) addDroppedFiles(paths []string) {
	if s.running() {
		s.status = "测速进行中，先停止再添加条目"
		return
	}
	if s.store == nil {
		s.status = "数据目录不可用，无法添加条目"
		return
	}
	added, skipped := 0, 0
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil || info.IsDir() || !isConfigFile(path) {
			skipped++
			continue
		}
		// 同一路径重复拖入不再生成重复条目。
		if absolute, absErr := filepath.Abs(path); absErr == nil {
			absolute = filepath.Clean(absolute)
			duplicated := false
			for _, entry := range s.entries {
				if entry.entry.IsSubscription() {
					continue
				}
				if existing, ok := s.resolvePath(entry.entry.Path); ok && existing == absolute {
					duplicated = true
					break
				}
			}
			if duplicated {
				skipped++
				continue
			}
		}
		if err := s.addLocalEntry(path); err != nil {
			s.status = "添加条目失败：" + err.Error()
			return
		}
		added++
	}
	switch {
	case added == 0 && skipped > 0:
		s.status = "没有可添加的配置文件（只接受 .yaml/.yml）"
	case skipped > 0:
		s.status = "已添加 " + itoa(added) + " 个条目，跳过 " + itoa(skipped) + " 个非配置文件"
	case added > 0:
		s.status = "已添加 " + itoa(added) + " 个条目"
	}
}

// addLocalEntry 新建一条本地文件条目并读取节点。名称取文件名（去扩展名），
// 重名时追加序号；路径按 store 的软/硬路径规则保存。
func (s *state) addLocalEntry(path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	absolute = filepath.Clean(absolute)
	base := strings.TrimSuffix(filepath.Base(absolute), filepath.Ext(absolute))
	record := store.Entry{
		ID:   store.NewID(),
		Name: s.uniqueEntryName(base),
		Kind: store.EntryKindLocal,
		Path: s.storePath(absolute),
	}
	if err := s.store.PutEntry(record); err != nil {
		return err
	}
	entry := &entryState{
		entry:    record,
		ids:      registerEntryIDs(s.tree, s.ids.page, record.ID),
		selected: true,
	}
	s.decorateEntry(entry)
	s.entries = append(s.entries, entry)
	s.readEntry(entry)
	return nil
}

// uniqueEntryName 返回不与现有条目重名的名称：重名时依次尝试「名称 2」、
// 「名称 3」，与手动新建条目的重名提示保持一致的可读结果。
func (s *state) uniqueEntryName(base string) string {
	base = strings.TrimSpace(base)
	if base == "" {
		base = "本地条目"
	}
	used := make(map[string]bool, len(s.entries))
	for _, entry := range s.entries {
		used[entry.entry.Name] = true
	}
	if !used[base] {
		return base
	}
	for index := 2; ; index++ {
		candidate := base + " " + itoa(index)
		if !used[candidate] {
			return candidate
		}
	}
}

// isConfigFile 只接受 Clash/Mihomo 常见的 YAML 扩展名（大小写不敏感）。
func isConfigFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
		return true
	default:
		return false
	}
}

// paintDropHint 在窗口中央画拖放提示。整窗淡色蒙层把注意力引到提示条上，
// 松开后提示消失、文件变成条目。
func (a *App) paintDropHint(canvas *ui.Canvas) {
	width, height := a.window.Width, a.window.Height
	if width <= 0 || height <= 0 {
		return
	}
	// 蒙层压暗整窗：拖动时注意力只留在提示卡片上。
	canvas.Fill(ui.Rect{Width: width, Height: height}, alpha(a.theme.Background, 0.72))
	boxWidth := canvas.TextWidth(a.dropText, 15) + 72
	if boxWidth > width-48 {
		boxWidth = width - 48
	}
	if boxWidth < 220 {
		boxWidth = 220
	}
	box := ui.Rect{X: (width - boxWidth) / 2, Y: height/2 - 38, Width: boxWidth, Height: 76}
	// 外圈淡强调色当光晕：深色底上卡片边界更清楚，也不依赖阴影层级。
	canvas.Rounded(ui.Rect{X: box.X - 2, Y: box.Y - 2, Width: box.Width + 4, Height: box.Height + 4},
		a.theme.Metrics.RadiusSurface+2, alpha(a.palette.Accent, 0.35))
	canvas.RoundedBorder(box, a.theme.Metrics.RadiusSurface, 1, a.theme.Border, a.palette.Raised)
	canvas.LabelCenter(a.dropText, box, ui.TextStyle{Size: 15, Weight: ui.WeightMedium, Color: a.theme.Foreground})
}
