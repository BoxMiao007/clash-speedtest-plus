package gui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BoxMiao007/clash-speedtest-plus/internal/export"
	"github.com/BoxMiao007/clash-speedtest-plus/internal/gist"
	"github.com/BoxMiao007/clash-speedtest-plus/internal/ip"
	"github.com/BoxMiao007/clash-speedtest-plus/internal/output"
	"github.com/BoxMiao007/clash-speedtest-plus/internal/speedtester"
	"github.com/BoxMiao007/clash-speedtest-plus/internal/store"
)

// 手动导出的三种产物：配置 yaml、结果图 PNG、结果表 TSV。
type manualExportKind uint8

const (
	manualYAML manualExportKind = iota
	manualImage
	manualTSV
)

// needsExport 报告本轮结束后是否有自动产物要写：配置、图片或上传任一开着。
// 上传必须有配置产物可传，输出关闭时不算。
func (s *state) needsExport() bool {
	upload := s.settings.UploadEnabled() && s.settings.OutputMode != store.OutputModeOff
	return s.settings.OutputMode != store.OutputModeOff || s.settings.ImageEnabled || upload
}

// roundNameBase 是产物名前缀：本地文件条目取文件名基名；订阅条目没有可跟的
// 文件名，返回空串（输出模式「当前目录下」改用时间戳兜底）。
func (s *state) roundNameBase(round *roundState) string {
	if round == nil || round.entry == nil || round.entry.entry.IsSubscription() {
		return ""
	}
	return output.CleanNameBase(filepath.Base(round.entry.entry.Path))
}

// entrySourceLabel 是结果图顶部与轮次提示用的来源名。
func entrySourceLabel(entry *entryState) string {
	if entry.entry.IsSubscription() {
		return output.SourceLabel(entry.entry.URL)
	}
	return entry.entry.Name
}

// roundExportPath 按输出模式算出本轮配置路径：关闭返回空串；自定义词干补
// .yaml 并经「产物跟随文件名」加基名/序号；当前目录下按基名或时间戳自动命名。
func (s *state) roundExportPath(nameBase string, now time.Time, seq int) string {
	switch s.settings.OutputMode {
	case store.OutputModeCustom:
		abs, _ := s.resolvePath(strings.TrimSpace(s.settings.OutputPath))
		if abs == "" {
			return ""
		}
		lower := strings.ToLower(abs)
		if !strings.HasSuffix(lower, ".yaml") && !strings.HasSuffix(lower, ".yml") {
			abs += ".yaml"
		}
		return output.FollowedConfigExportPath(abs, nameBase, now, seq)
	case store.OutputModeAuto:
		return filepath.Join(s.baseDir, output.AutoExportPath(nameBase, now, seq))
	default:
		return ""
	}
}

// roundResults 把本轮节点折算成测速结果（带节点配置快照），并按模式排序。
func (s *state) roundResults(round *roundState) []*speedtester.Result {
	results := make([]*speedtester.Result, 0, len(round.nodes))
	for _, node := range round.nodes {
		if !node.hasResult || node.proxy == nil {
			continue
		}
		results = append(results, nodeToResult(node))
	}
	mode, err := speedtester.ParseSpeedMode(s.settings.Mode)
	if err != nil {
		mode = speedtester.SpeedModeDownload
	}
	return output.SortResults(results, mode)
}

// nodeToResult 把界面上的节点状态还原成引擎结果，并复制一份配置，供导出
// 重命名时不改到界面显示用的原始配置。
func nodeToResult(node *nodeState) *speedtester.Result {
	return &speedtester.Result{
		ProxyName:     node.name,
		ProxyType:     node.typ,
		ProxyConfig:   cloneProxyConfig(node.proxy.Config),
		Latency:       durationMillis(node.result.LatencyMS),
		Jitter:        durationMillis(node.result.JitterMS),
		PacketLoss:    node.result.PacketLoss,
		DownloadSpeed: node.result.DownloadBps,
		DownloadError: node.result.DownloadError,
		UploadSpeed:   node.result.UploadBps,
		UploadError:   node.result.UploadError,
	}
}

// cloneProxyConfig 浅拷贝节点配置；导出重命名会改 name 字段，不能动界面上的原件。
func cloneProxyConfig(config map[string]any) map[string]any {
	if config == nil {
		return nil
	}
	cloned := make(map[string]any, len(config))
	for key, value := range config {
		cloned[key] = value
	}
	return cloned
}

// roundExport 是一轮导出任务的输入快照：构造后不再读可变状态，后台 goroutine
// 只碰自己这份。
type roundExport struct {
	Settings store.Settings
	Dir      string // 程序目录：结果图与默认产物都写在这里
	Path     string // 已算好的配置路径；空串表示不写配置
	Source   string // 结果图顶部来源
	NameBase string
	Seq      int
	Results  []*speedtester.Result
	Total    int
	Now      time.Time
	Early    bool
}

// runRoundExport 写一轮的全部产物：配置 yaml、结果图、Gist/仓库上传。
// 返回写出的主路径、图片/上传的附加提示、过筛后的节点（供合并导出）和错误。
// 配置或图片写失败才算错误；上传失败只进提示，不阻塞后续轮次。
func runRoundExport(job roundExport) (path string, warning string, proxies []map[string]any, err error) {
	mode, modeErr := speedtester.ParseSpeedMode(job.Settings.Mode)
	if modeErr != nil {
		mode = speedtester.SpeedModeDownload
	}
	// 过筛与重命名只在真要写配置时做：重命名要按 IP 查归属地（联网），
	// 输出关闭时不该为「合并导出」空跑一遍网络请求。
	if job.Path != "" {
		filter := testFilter(testConfigFromSettings(job.Settings))
		proxies = buildProxies(job.Results, filter, job.Settings.RenameEnabled, job.Settings.RenameTemplate)
		if err := export.WriteYAML(job.Path, proxies); err != nil {
			return "", "", proxies, fmt.Errorf("保存配置失败：%w", err)
		}
		path = job.Path
	}
	if job.Settings.ImageEnabled {
		imagePath, imageWarning, imageErr := writeResultImage(job, mode)
		if imageErr != nil {
			// 结果图是附加产物：失败只提示，不拖住后续轮次；配置写入失败才算错误。
			warning = output.JoinStatus(warning, "保存结果图失败："+imageErr.Error())
		} else {
			path = output.JoinStatus(path, imagePath)
			warning = output.JoinStatus(warning, imageWarning)
		}
	}
	if job.Settings.UploadEnabled() && job.Path != "" {
		if uploadErr := uploadConfig(job.Settings, job.Path); uploadErr != nil {
			warning = output.JoinStatus(warning, uploadErr.Error())
		}
	}
	return path, warning, proxies, nil
}

// writeResultImage 按旧 TUI 的口径生成结果图：完成状态、汇总行、阈值配色；
// 「仅速度行」只在非 fast 模式生效，fast 模式会忽略并保持全量。
func writeResultImage(job roundExport, mode speedtester.SpeedMode) (string, string, error) {
	status := "已完成"
	if job.Early {
		status = "已提前结束"
	}
	rows := output.BuildImageRows(job.Results, mode)
	speedOnly := job.Settings.ImageSpeedOnly && !mode.IsFast()
	filtered := output.FilterImageRowsBySpeed(rows, speedOnly)
	summary := output.SummaryLine(job.Now, mode, status, len(job.Results), job.Total)
	if speedOnly {
		untested := max(job.Total-len(job.Results)-filtered.Testing, 0)
		summary = output.AppendImageSpeedCounts(summary, len(filtered.Rows), filtered.Invalid, filtered.Testing, untested)
	}
	spec := output.ImageSpec{
		Mode:     mode,
		Source:   job.Source,
		Summary:  summary,
		Headers:  output.GetHeaders(mode),
		Rows:     filtered.Rows,
		Now:      job.Now,
		NameBase: job.NameBase,
		Seq:      job.Seq,
	}
	return output.WriteResultImage(job.Dir, spec)
}

// ipLookup 是重命名用的归属地查询；测试替换成假实现，避免联网。
var ipLookup = ip.GetIPLocation

// buildProxies 按过筛口径把结果收成写盘用的节点列表：阈值过滤、可选按 IP
// 归属地与速度重命名。与旧 TUI 的 buildProxies 同一口径（ADR-0014）。
func buildProxies(results []*speedtester.Result, filter export.Filter, rename bool, template string) []map[string]any {
	proxies := make([]map[string]any, 0, len(results))
	nameCount := make(map[string]int, len(results))
	for _, result := range results {
		if !filter.Match(result) {
			continue
		}
		proxyConfig := cloneProxyConfig(result.ProxyConfig)
		if proxyConfig == nil || proxyConfig["name"] == nil || proxyConfig["server"] == nil {
			continue
		}
		if rename {
			location, err := ipLookup(fmt.Sprint(proxyConfig["server"]))
			if err != nil || location.CountryCode == "" {
				proxies = append(proxies, proxyConfig)
				continue
			}
			name, renameErr := ip.GenerateNodeNameFromTemplate(template, location.CountryCode, result.Latency, result.DownloadSpeed, result.UploadSpeed, nameCount)
			if renameErr != nil {
				name = ip.GenerateNodeName(location.CountryCode, result.Latency, result.DownloadSpeed, result.UploadSpeed, nameCount)
			}
			proxyConfig["name"] = name
		}
		proxies = append(proxies, proxyConfig)
	}
	return proxies
}

// uploader 是上传动作的窄接口；测试注入假实现，避免真实网络。
type uploader interface {
	UpdateFile(token, address, filename string, content []byte) error
	UpdateRepoFile(token, address, filePath, branch string, content []byte) error
}

// newUploader 生产环境用 gist 包；测试替换成记录调用的假实现。
var newUploader = func() uploader { return gist.NewUploader(nil) }

// uploadConfig 把已写好的配置同步到 Gist 或仓库。失败只作为提示返回。
func uploadConfig(settings store.Settings, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("读取待上传配置失败：%w", err)
	}
	filename := filepath.Base(filepath.Clean(path))
	uploader := newUploader()
	if settings.GistToken != "" && settings.GistAddress != "" {
		if err := uploader.UpdateFile(settings.GistToken, settings.GistAddress, filename, data); err != nil {
			return fmt.Errorf("更新 Gist 失败：%w", err)
		}
	}
	if settings.RepoToken != "" && settings.RepoAddress != "" {
		repoPath := strings.TrimSpace(settings.RepoFilePath)
		if repoPath == "" {
			repoPath = filename
		}
		if err := uploader.UpdateRepoFile(settings.RepoToken, settings.RepoAddress, repoPath, settings.RepoBranch, data); err != nil {
			return fmt.Errorf("更新仓库文件失败：%w", err)
		}
	}
	return nil
}

// startRoundExport 在后台写一轮的产物，完成后用 eventExported 回到窗口线程。
// 允许上一轮导出未完成时继续（任务用轮次指针回填状态），但同一轮不会重复启动。
func (s *state) startRoundExport(round *roundState) {
	if round == nil {
		return
	}
	index := s.roundPosition(round)
	if index < 0 {
		return
	}
	now := time.Now()
	nameBase := s.roundNameBase(round)
	seq := s.exportSeq + index
	job := roundExport{
		Settings: s.settings,
		Dir:      s.baseDir,
		Path:     s.roundExportPath(nameBase, now, seq),
		Source:   entrySourceLabel(round.entry),
		NameBase: nameBase,
		Seq:      seq,
		Results:  s.roundResults(round),
		Total:    len(round.nodes),
		Now:      now,
		Early:    round.early,
	}
	s.exporting++
	s.exportRound = index
	s.exportText = "正在导出…"
	s.refreshRunText()
	go func() {
		path, warning, proxies, err := runRoundExport(job)
		s.enqueue(runEvent{kind: eventExported, round: index, roundRef: round, path: path, warning: warning, err: err, proxies: proxies})
	}()
}

// writeMergedExport 写「合并导出」：≥2 轮、输出开着、全部轮正常完成时，
// 按完成顺序求并集，重名节点保留先完成轮的。返回写入路径；没触发返回空串。
func (s *state) writeMergedExport() string {
	if len(s.rounds) < 2 || s.settings.OutputMode == store.OutputModeOff {
		return ""
	}
	seen := make(map[string]bool)
	proxies := make([]map[string]any, 0)
	for _, round := range s.rounds {
		if !round.done || round.stopped || round.err != "" {
			return ""
		}
		for _, proxy := range round.proxies {
			name := fmt.Sprint(proxy["name"])
			if seen[name] {
				continue
			}
			seen[name] = true
			proxies = append(proxies, proxy)
		}
	}
	path := filepath.Join(s.baseDir, output.MergedExportPath(time.Now()))
	if err := export.WriteYAML(path, proxies); err != nil {
		s.status = "保存合并配置失败：" + err.Error()
		return ""
	}
	return path
}

// currentResults 返回当前视图可导出的结果：有轮次时用当前轮，否则用本轮
// 会话的全部历史结果。
func (s *state) currentResults() []*speedtester.Result {
	if round := s.currentRound(); round != nil {
		return s.roundResults(round)
	}
	mode, err := speedtester.ParseSpeedMode(s.settings.Mode)
	if err != nil {
		mode = speedtester.SpeedModeDownload
	}
	results := make([]*speedtester.Result, 0, len(s.nodes))
	for _, node := range s.nodes {
		if !node.hasResult || node.proxy == nil {
			continue
		}
		results = append(results, nodeToResult(node))
	}
	return output.SortResults(results, mode)
}

// startManualExport 启动一次手动导出（配置 / 结果图 / 结果表）。
func (s *state) startManualExport(kind manualExportKind) {
	if s.exporting > 0 {
		s.status = "正在导出，请稍候"
		return
	}
	results := s.currentResults()
	if len(results) == 0 {
		s.status = "还没有可导出的测速结果"
		return
	}
	settings := s.settings
	now := time.Now()
	nameBase := ""
	if round := s.currentRound(); round != nil {
		nameBase = s.roundNameBase(round)
	}
	source := ""
	if round := s.currentRound(); round != nil {
		source = entrySourceLabel(round.entry)
	}
	// 手动导出单独取一个未被占用的序号，避免和同轮自动产物同名覆盖。
	exportDir := s.baseDir
	if s.settings.OutputMode == store.OutputModeCustom {
		if abs, _ := s.resolvePath(strings.TrimSpace(s.settings.OutputPath)); abs != "" {
			exportDir = filepath.Dir(abs)
		}
	}
	seq := output.NextImageSequence(exportDir)
	s.exporting++
	s.exportText = "正在导出…"
	s.refreshRunText()
	s.status = "正在导出…"

	switch kind {
	case manualYAML:
		path := s.roundExportPath(nameBase, now, seq)
		if path == "" {
			path = filepath.Join(s.baseDir, output.AutoExportPath(nameBase, now, seq))
		}
		go func() {
			filter := testFilter(testConfigFromSettings(settings))
			proxies := buildProxies(results, filter, settings.RenameEnabled, settings.RenameTemplate)
			err := export.WriteYAML(path, proxies)
			var warning string
			if err == nil && settings.UploadEnabled() {
				if uploadErr := uploadConfig(settings, path); uploadErr != nil {
					warning = uploadErr.Error()
				}
			}
			s.enqueue(runEvent{kind: eventExported, round: -1, path: path, warning: warning, err: err, proxies: proxies})
		}()
	case manualImage:
		total := len(results)
		early := false
		if round := s.currentRound(); round != nil {
			total = len(round.nodes)
			early = round.early
		}
		job := roundExport{Settings: settings, Dir: s.baseDir, Source: source, NameBase: nameBase, Seq: seq, Results: results, Total: total, Now: now, Early: early}
		go func() {
			mode, modeErr := speedtester.ParseSpeedMode(settings.Mode)
			if modeErr != nil {
				mode = speedtester.SpeedModeDownload
			}
			path, warning, err := writeResultImage(job, mode)
			s.enqueue(runEvent{kind: eventExported, round: -1, path: path, warning: warning, err: err})
		}()
	case manualTSV:
		path := filepath.Join(s.baseDir, "结果表-"+now.Format("20060102-150405")+".tsv")
		go func() {
			err := writeTSV(path, results, settings.Mode)
			s.enqueue(runEvent{kind: eventExported, round: -1, path: path, err: err})
		}()
	}
}

// writeTSV 把结果表写成 TSV 文件（与旧非交互输出同一列口径）。
func writeTSV(path string, results []*speedtester.Result, modeValue string) error {
	mode, err := speedtester.ParseSpeedMode(modeValue)
	if err != nil {
		mode = speedtester.SpeedModeDownload
	}
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("创建结果表失败：%w", err)
	}
	defer file.Close()
	writer, err := output.NewTSVWriter(file, mode)
	if err != nil {
		return err
	}
	return writer.WriteRows(results)
}
