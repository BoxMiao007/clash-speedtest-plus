package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/BoxMiao007/clash-speedtest-plus/gist"
	"github.com/BoxMiao007/clash-speedtest-plus/ip"
	"github.com/BoxMiao007/clash-speedtest-plus/output"
	"github.com/BoxMiao007/clash-speedtest-plus/picker"
	"github.com/BoxMiao007/clash-speedtest-plus/speedtester"
	"github.com/BoxMiao007/clash-speedtest-plus/tui"
	tea "github.com/charmbracelet/bubbletea"
	mihomolog "github.com/metacubex/mihomo/log"
	"gopkg.in/yaml.v2"
)

// Version information injected via ldflags during build
var (
	version = "dev"
	commit  = "unknown"
)

// outputAuto 表示输出模式处于「默认当前路径」：每轮不填词干，按基名或
// 时间戳自动命名。命令行 -o 只有自定义语义、没有自动模式入口，该开关
// 只来自选源界面（见 CONTEXT.md「输出模式」词条）。
var outputAuto bool

var (
	configPathsConfig = flag.String("c", "", "配置文件路径，也支持 http(s) 地址；逗号分隔多个源，每个源各自一轮分别测速")
	filterRegexConfig = flag.String("f", ".+", "按节点名过滤，使用正则")
	blockKeywords     = flag.String("b", "", "按关键字屏蔽节点，多个关键字用 | 分隔（例如 -b 'rate|x1|1x'）")
	serverURL         = flag.String("server-url", "https://dl.google.com/chrome/mac/universal/stable/GGRO/googlechrome.dmg", "测速服务器地址或直接下载地址")
	speedMode         = flag.String("speed-mode", "download", "测速模式：fast、download、full")
	downloadSize      = flag.Int("download-size", 50, "下载测试大小（单位：MB）")
	uploadSize        = flag.Int("upload-size", 20, "上传测试大小，仅完整模式（单位：MB）")
	timeout           = flag.Duration("timeout", time.Second*5, "单个请求超时")
	concurrent        = flag.Int("concurrent", 4, "同一节点的下载并发连接数")
	parallel          = intFlag("p", "parallel", 1, "同时测试的节点数")
	noImage           = flag.Bool("no-image", false, "关闭自动导出结果表图，仍可按 s 手动保存")
	imageSpeedOnly    = flag.Bool("image-speed-only", false, "结果图只保留下载或上传速度大于 0 的行；快速模式会忽略")
	outputPath        = stringFlag("o", "output", "", "输出配置文件路径，不带 .yaml/.yml 后缀时自动补 .yaml")
	gistToken         = flag.String("gist-token", "", "用于更新 Gist 的 GitHub token")
	gistAddress       = flag.String("gist-address", "", "要更新的 Gist 地址或 ID（文件名使用输出文件名）")
	repoToken         = flag.String("repo-token", "", "用于更新仓库文件的 GitHub token")
	repoAddress       = flag.String("repo-address", "", "仓库地址或 owner/repo")
	repoFilePath      = flag.String("repo-file-path", "", "仓库中的文件路径（默认使用输出文件名）")
	repoBranch        = flag.String("repo-branch", "", "仓库分支（默认使用仓库默认分支）")
	maxLatency        = flag.Duration("max-latency", time.Second, "过滤高于该值的延迟")
	maxPacketLoss     = flag.Float64("max-packet-loss", 100, "过滤高于该值的丢包率（单位：%）")
	minDownloadSpeed  = flag.Float64("min-download-speed", 5, "过滤低于该值的下载速度（单位：MB/s）")
	minUploadSpeed    = flag.Float64("min-upload-speed", 2, "过滤低于该值的上传速度（单位：MB/s，仅完整模式）")
	earlyStop         = flag.Int("early-stop", 0, "过筛结果达到该数量后提前结束（0 为关闭）")
	renameNodes       = flag.Bool("rename", true, "用 IP 位置和速度重命名输出节点")
	renameTemplate    = flag.String("rename-template", "", "重命名模板（Go text/template）。占位符：{{.Flag}}、{{.CountryCode}}、{{.Index}}、{{.Direction}}、{{.Speed}}、{{.SpeedUnit}}、{{.LatencyMs}}、{{.DownloadSpeedMBps}}、{{.UploadSpeedMBps}}。空为默认格式")
	fastMode          = flag.Bool("fast", false, "快速模式（等同 --speed-mode fast）")
	versionFlag       = flag.Bool("v", false, "显示版本信息")
	userAgent         = flag.String("ua", "", "拉取 http(s) 配置时使用的 User-Agent（默认使用 mihomo 内核 UA）")
)

type launchKind int

const (
	launchCLI launchKind = iota
	launchPicker
)

// launchChoice 决定这次启动进选源界面还是命令行。
// 只有不带任何参数、并且标准输出是终端时才进选源界面。
func launchChoice(args []string, stdoutIsTerminal bool) launchKind {
	if len(args) == 0 && stdoutIsTerminal {
		return launchPicker
	}
	return launchCLI
}

// runPickerModel 跑一遍选源界面。model 可以是上一轮按 Esc 返回时保留的状态，
// 勾选、地址和选项原样继续。
func runPickerModel(model picker.Model) (picker.Model, bool) {
	program := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseAllMotion())
	final, err := program.Run()
	if err != nil {
		log.Fatalf("选源界面运行失败: %s", err)
	}
	finished := final.(picker.Model)
	return finished, finished.Started()
}

// buildTester 按当前参数组装测速器。选源界面和命令行共用。
func buildTester() (*speedtester.SpeedTester, speedtester.SpeedMode, resultFilter, *earlyStopper, error) {
	requestedMode := speedtester.SpeedModeFast
	var err error
	if !*fastMode {
		requestedMode, err = speedtester.ParseSpeedMode(*speedMode)
		if err != nil {
			return nil, requestedMode, resultFilter{}, nil, fmt.Errorf("解析测速模式失败: %w", err)
		}
	}
	tester, err := speedtester.New(&speedtester.Config{
		ConfigPaths:      *configPathsConfig,
		FilterRegex:      *filterRegexConfig,
		BlockRegex:       *blockKeywords,
		ServerURL:        *serverURL,
		DownloadSize:     *downloadSize * 1024 * 1024,
		UploadSize:       *uploadSize * 1024 * 1024,
		Timeout:          *timeout,
		Concurrent:       *concurrent,
		Parallel:         *parallel,
		MaxPacketLoss:    *maxPacketLoss,
		MaxLatency:       *maxLatency,
		MinDownloadSpeed: *minDownloadSpeed * 1024 * 1024,
		MinUploadSpeed:   *minUploadSpeed * 1024 * 1024,
		Mode:             requestedMode,
		OutputPath:       *outputPath,
		UserAgent:        *userAgent,
	})
	if err != nil {
		return nil, requestedMode, resultFilter{}, nil, err
	}
	effectiveMode := tester.Mode()
	resultFilter := newResultFilter(effectiveMode)
	stopper, err := newEarlyStopper(*earlyStop, resultFilter)
	if err != nil {
		return nil, effectiveMode, resultFilter, nil, err
	}
	return tester, effectiveMode, resultFilter, stopper, nil
}

// executableDir 是程序文件所在目录。选源和产物都以它为锚。
func executableDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

// applyPickerOptions 把选源界面选好的选项写进命令行参数。
// 空值沿用命令行默认；解析失败也沿用默认，回车前界面已校验过一遍。
func applyPickerOptions(o picker.Options) {
	if o.Filter != "" {
		*filterRegexConfig = o.Filter
	}
	*blockKeywords = o.Block
	if mode, err := speedtester.ParseSpeedMode(o.Mode); err == nil {
		*speedMode = string(mode)
	}
	if v, err := strconv.Atoi(o.DownloadSize); err == nil {
		*downloadSize = v
	}
	if v, err := strconv.Atoi(o.UploadSize); err == nil {
		*uploadSize = v
	}
	if v, err := strconv.Atoi(o.Concurrent); err == nil {
		*concurrent = v
	}
	if v, err := strconv.Atoi(o.Parallel); err == nil {
		*parallel = v
	}
	if v, err := time.ParseDuration(o.Timeout); err == nil {
		*timeout = v
	}
	if v, err := strconv.Atoi(o.EarlyStop); err == nil {
		*earlyStop = v
	}
	if v, err := time.ParseDuration(o.MaxLatency); err == nil {
		*maxLatency = v
	}
	if v, err := strconv.ParseFloat(o.MaxPacketLoss, 64); err == nil {
		*maxPacketLoss = v
	}
	if v, err := strconv.ParseFloat(o.MinDownload, 64); err == nil {
		*minDownloadSpeed = v
	}
	if v, err := strconv.ParseFloat(o.MinUpload, 64); err == nil {
		*minUploadSpeed = v
	}
	*imageSpeedOnly = o.ImageSpeedOnly
	*noImage = o.NoImage
	// 输出设置按三态解释（见 CONTEXT.md「输出模式」）：自定义写词干，
	// 默认当前路径开自动命名，关闭清空——按 Esc 重跑时关掉输出必须真的
	// 关掉，不能沿用上一轮的值。
	switch o.OutputMode {
	case picker.OutputModeCustom:
		*outputPath = o.OutputPath
		outputAuto = false
	case picker.OutputModeDefaultPath:
		*outputPath = ""
		outputAuto = true
	default:
		*outputPath = ""
		outputAuto = false
	}
	*renameNodes = o.Rename
	*renameTemplate = o.RenameTemplate
	*gistToken = o.GistToken
	*gistAddress = o.GistAddress
	*repoToken = o.RepoToken
	*repoAddress = o.RepoAddress
	*repoFilePath = o.RepoFilePath
	*repoBranch = o.RepoBranch
	if o.ServerURL != "" {
		*serverURL = o.ServerURL
	}
	*userAgent = o.UserAgent
}

// normalizeOutputFlag 在 flag 解析后应用 -o 的「后缀补全」：
// 不带 .yaml/.yml 后缀（不分大小写）时补 .yaml，写全时文件名一字不差。
// 以 / 或 \ 结尾是目录意图，启动即报错，不进测速。留空（含纯空白）仍
// 表示不输出。校验口径与选源界面共用 picker.ValidateOutputPath。
func normalizeOutputFlag() error {
	value, err := picker.ValidateOutputPath(*outputPath)
	if err != nil {
		return fmt.Errorf("输出路径不合法：%w", err)
	}
	*outputPath = picker.CompleteYAMLSuffix(value)
	return nil
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "用法：clash-speedtest-plus [选项]\n")
		printFlagDefaults(flag.CommandLine)
	}
	flag.Parse()
	mihomolog.SetLevel(mihomolog.SILENT)

	execDir := executableDir()

	// Handle version flag
	if *versionFlag {
		fmt.Printf("clash-speedtest-plus version %s (commit %s)\n", version, commit)
		os.Exit(0)
	}

	// -o 的后缀补全与目录意图校验在解析后立刻做：错误在测速开始前暴露。
	if err := normalizeOutputFlag(); err != nil {
		log.Fatalln(err)
	}

	args := os.Args[1:]
	switch {
	case launchChoice(args, output.IsTerminalFile(os.Stdout)) == launchPicker:
		configs, err := picker.ListConfigs(execDir)
		if err != nil {
			log.Printf("读取程序目录失败: %s", err)
		}
		model := picker.New(picker.Session{Configs: configs, ExecDir: execDir})
		for {
			finished, started := runPickerModel(model)
			if !started {
				return
			}
			applyPickerOptions(finished.Options())
			for _, used := range finished.UsedFlagged() {
				fmt.Fprintf(os.Stderr, "已用补过参数的地址: %s\n", used)
			}
			// 多源一一分开：每个源各自一轮、各出各的产物，互不混合。
			if runSourceQueue(finished.SourceList(), execDir, true) {
				// 按过 Esc：本轮中断、剩余轮作废，回选源界面重排。
				model = finished
				continue
			}
			return
		}
	case len(args) == 0:
		// 没有终端，进不了选源界面。
		flag.Usage()
		os.Exit(1)
	default:
		runSourceQueue(picker.SplitConfigArg(*configPathsConfig), execDir, false)
	}
}

// runSourceQueue 逐源跑完整测速：每个源一轮，测完自动进下一轮；
// 选源路径按 Esc 中断本轮并作废剩余轮（返回 true），命令行路径返回恒为 false。
// 交互路径由一个 tea.Program 承载整个队列（ADR-0015），非交互（TSV/管道）
// 保持逐轮同步执行。
func runSourceQueue(sources []picker.SourceSpec, execDir string, escapeToParent bool) bool {
	if len(sources) == 0 {
		log.Fatalln("请指定配置文件")
	}
	// 输出路径只锚一次；每轮从原始路径算自己的产物名，免得基名层层叠加。
	// 各轮会把全局改成自己的最终名，所以锚定值必须存局部变量。
	*outputPath = picker.ResolveOutputPath(execDir, *outputPath)
	originalOutputPath := *outputPath
	// 输出模式非关闭：自定义词干（命令行 -o 或选源自定义态）或默认当前路径
	// （outputAuto）。「合并导出」以此为前提（CONTEXT.md「合并导出」词条）。
	outputOpen := originalOutputPath != "" || outputAuto

	var escaped bool
	if output.DetermineOutputMode(output.IsTerminalFile) == output.OutputModeInteractive {
		escaped = runQueueProgram(sources, execDir, originalOutputPath, outputOpen, escapeToParent)
	} else {
		// 非交互模式没有界面：stdout 保留给 TSV/管道，逐轮同步跑。
		startSeq := output.NextImageSequence(execDir)
		// 各轮过筛节点按完成顺序收集，队列走完后供「合并导出」求并集。
		var completed [][]map[string]any
		for i, src := range sources {
			// 单源不画轮次（跟「多源才亮轮数」的口径一致），多源标「第 X/N 轮 源名」。
			label := ""
			if len(sources) > 1 {
				label = fmt.Sprintf("第 %d/%d 轮 %s", i+1, len(sources), src.DisplayName)
			}
			// 序号按队列位置排：跳过的轮也占号，图上的号和界面「第 X/N 轮」对得齐。
			round := speedRound{
				source:     src,
				label:      label,
				seq:        startSeq + i,
				hasMore:    i < len(sources)-1,
				outputPath: originalOutputPath,
			}
			outcome, proxies := runSpeedTest(execDir, round)
			if outcome == roundSkipped {
				continue
			}
			completed = append(completed, proxies)
		}
		// 逐轮同步跑完且没有致命退出才到这里：非交互没有 Esc/退出全部语义，
		// 等价队列正常结束，按「合并导出」条件写（ADR-0014）。
		if path := writeMergedExport(tui.QueueResult{}, outputOpen, execDir, completed, time.Now()); path != "" {
			fmt.Fprintf(os.Stderr, "已保存合并配置: %s\n", path)
		}
	}
	// 全局在轮内被改写成各轮最终名，离开队列前恢复成用户原始路径，
	// 免得选源界面清空输出路径时残留名被当原始路径再叠一层。
	*outputPath = originalOutputPath
	return escaped
}

// speedRound 是队列里的一轮：测哪个源、界面怎么标、产物名带什么序号、
// 输出写到哪、之后还有没有下一轮。所有轮次信息收在一起，免得参数表越滚越长。
type speedRound struct {
	source     picker.SourceSpec
	label      string // 多轮时「第 X/N 轮 源名」；单轮空
	seq        int    // 产物序号，加在图与输出配置文件名最前
	hasMore    bool   // 之后还有轮：本轮测完保存完由队列壳推进
	outputPath string // 用户原始输出路径（锚定后），每轮从它算最终名
}

// roundOutcome 是一轮非交互测速的结局：正常结束，或源加载不出节点跳过。
type roundOutcome int

const (
	roundDone roundOutcome = iota
	roundSkipped
)

// setupRound 把全局参数切到本轮：-c 指向本轮源，输出路径按三种输出模式
// 从用户原始路径算出本轮最终名（见 roundOutputPath；序号与结果图同轮同号）。
// 返回图名基名，供交互轮与非交互导出共用。
func setupRound(execDir string, round speedRound) string {
	src := round.source
	*configPathsConfig = src.Value
	if *configPathsConfig == "" {
		log.Fatalln("请指定配置文件")
	}
	// 「产物跟随文件名」恒定开启，只跟本地配置文件；订阅源没有可跟的名字，保持默认。
	nameBase := ""
	if !src.FromSubscription {
		nameBase = output.CleanNameBase(src.DisplayName)
	}

	// 每轮从用户原始输出路径算起最终产物名（见 roundOutputPath）。
	*outputPath = roundOutputPath(round.outputPath, outputAuto, execDir, nameBase, time.Now(), round.seq)
	return nameBase
}

// runSpeedTest 跑非交互（TSV/管道）路径的一轮：同步收集结果、写 TSV、
// 落盘 yaml 与结果图、等上传。交互路径由 runQueueProgram 承载，不经过这里。
// 返回本轮结局与过筛后的节点列表（输出关闭时为 nil），供合并导出求并集。
func runSpeedTest(execDir string, round speedRound) (roundOutcome, []map[string]any) {
	src := round.source
	nameBase := setupRound(execDir, round)

	speedTester, effectiveMode, resultFilter, stopper, err := buildTester()
	if err != nil {
		log.Fatalf("%s", err)
	}

	allProxies, err := speedTester.LoadProxies()
	if err != nil {
		log.Fatalf("加载节点失败: %s", err)
	}
	if len(allProxies) == 0 {
		// 多源队列里一个源空了不该废掉整个队列：说明原因后跳过继续。
		who := round.label
		if who == "" {
			who = src.DisplayName
		}
		fmt.Fprintf(os.Stderr, "%s：解析出 0 个节点，跳过这一轮\n", who)
		return roundSkipped, nil
	}

	// 非交互模式没有进度行，多轮时轮次提示走 stderr，stdout 保留给 TSV/管道。
	if round.label != "" {
		fmt.Fprintf(os.Stderr, "%s\n", round.label)
	}
	tsvWriter, err := output.NewTSVWriter(os.Stdout, effectiveMode)
	if err != nil {
		log.Fatalf("创建 TSV 输出失败: %s", err)
	}

	results := make([]*speedtester.Result, 0, len(allProxies))

	// TSV mode: collect results synchronously
	speedTester.TestProxiesUntil(allProxies, nil, func(result *speedtester.Result) bool {
		results = append(results, result)

		if err := tsvWriter.WriteRow(result, len(results)-1); err != nil {
			log.Printf("写入 TSV 行失败: %s", err)
		}
		return stopper.ShouldContinue(result)
	})

	results = output.SortResults(results, effectiveMode)

	var savedProxies []map[string]any
	if *outputPath != "" {
		proxies, err := saveConfigInterruptible(*outputPath, results, resultFilter)
		if err != nil {
			fmt.Fprintf(os.Stderr, "保存配置失败: %s\n", err)
			os.Exit(1)
		}
		savedProxies = proxies
		// 非交互模式路径走 stderr，stdout 保留给 TSV/管道输出。
		fmt.Fprintf(os.Stderr, "已保存配置: %s\n", *outputPath)
	}
	if effectiveMode.IsFast() && *imageSpeedOnly {
		fmt.Fprintf(os.Stderr, "%s\n", tui.FastImageSpeedIgnored())
	}
	if !*noImage {
		exportNonInteractiveImage(execDir, src.DisplayName, nameBase, round.seq, results, effectiveMode, len(allProxies), imageSpeedFilterEnabled(effectiveMode))
	}
	waitForUpload()
	return roundDone, savedProxies
}

// roundOutputPath 算出一轮的最终产物路径，保存与上传都读它。自定义模式
// 按「产物跟随文件名」叠基名与序号（订阅轮没有基名靠序号区分，序号停用
// 才退回时间戳）；「默认当前路径」没有用户词干，本地源按「基名-导出.yaml」
// 自动命名，订阅轮用「导出-时间戳」兜底（见 CONTEXT.md「输出模式」「产物
// 序号」词条）。两种模式的产物序号与结果图同轮同号。
func roundOutputPath(customPath string, auto bool, execDir, nameBase string, now time.Time, seq int) string {
	switch {
	case customPath != "":
		return output.FollowedConfigExportPath(customPath, nameBase, now, seq)
	case auto:
		return filepath.Join(execDir, output.AutoExportPath(nameBase, now, seq))
	default:
		return ""
	}
}

// queueRunner 为队列壳准备每一轮：切全局参数、组装测速器、启动测试 goroutine、
// 配好轮视图。prepare 由壳在轮推进时调用（ADR-0015）。
type queueRunner struct {
	sources            []picker.SourceSpec
	execDir            string
	originalOutputPath string
	outputOpen         bool // 输出模式非关闭：「合并导出」的前提
	escapeToParent     bool
	startSeq           int
	voided             *atomic.Bool
	mu                 sync.Mutex
	current            *speedtester.SpeedTester // 最近一轮启动的测试，作废时停掉
	mergeMu            sync.Mutex
	mergedRounds       [][]map[string]any // 正常测完轮的过筛节点，按完成顺序，供合并导出
}

// recordMergedRound 收下一轮过筛后的节点列表（与该轮写盘同一构建），供队列
// 正常结束后合并导出。saver 在 bubbletea 命令 goroutine 里执行，与收尾读取
// 并发，需加锁。
func (r *queueRunner) recordMergedRound(proxies []map[string]any) {
	r.mergeMu.Lock()
	defer r.mergeMu.Unlock()
	r.mergedRounds = append(r.mergedRounds, proxies)
}

// collectedRounds 返回已收集的各轮过筛节点（按完成顺序）。
func (r *queueRunner) collectedRounds() [][]map[string]any {
	r.mergeMu.Lock()
	defer r.mergeMu.Unlock()
	return r.mergedRounds
}

// prepare 准备第 index 轮并返回就绪的轮视图。源解析不出节点时返回 skipped；
// 致命错误返回 err，由队列壳退出界面后再报错终止，终端能正常还原。
func (r *queueRunner) prepare(index int) (tui.QueueRound, bool, error) {
	src := r.sources[index]
	// 单源不画轮次（跟「多源才亮轮数」的口径一致），多源标「第 X/N 轮 源名」。
	label := ""
	if len(r.sources) > 1 {
		label = fmt.Sprintf("第 %d/%d 轮 %s", index+1, len(r.sources), src.DisplayName)
	}
	// 序号按队列位置排：跳过的轮也占号，图上的号和界面「第 X/N 轮」对得齐。
	round := speedRound{
		source:     src,
		label:      label,
		seq:        r.startSeq + index,
		hasMore:    index < len(r.sources)-1,
		outputPath: r.originalOutputPath,
	}
	nameBase := setupRound(r.execDir, round)

	speedTester, effectiveMode, resultFilter, stopper, err := buildTester()
	if err != nil {
		return nil, false, err
	}

	allProxies, err := speedTester.LoadProxies()
	if err != nil {
		return nil, false, fmt.Errorf("加载节点失败: %w", err)
	}
	if len(allProxies) == 0 {
		// 多源队列里一个源空了不该废掉整个队列：说明原因后跳过继续。
		who := round.label
		if who == "" {
			who = src.DisplayName
		}
		fmt.Fprintf(os.Stderr, "%s：解析出 0 个节点，跳过这一轮\n", who)
		return nil, true, nil
	}
	if r.voided.Load() {
		return nil, true, nil
	}

	hasOutput := *outputPath != ""
	resultChannel := make(chan *speedtester.Result, len(allProxies))

	// 进度事件通道：把节点开始/阶段变化/瞬时速度转发给 TUI 在测行。
	progressChannel := make(chan speedtester.Progress, 256)
	speedTester.SetProgressFunc(func(p speedtester.Progress) {
		// TUI 退出后不再消费，非阻塞丢弃避免测试 goroutine 卡死。
		select {
		case progressChannel <- p:
		default:
		}
	})

	// 提前结束信号：过筛数达到限额后关闸，TUI 收到后隐藏空格、停止显示剩余。
	earlyStopSignal := make(chan struct{})
	var earlyStopOnce sync.Once
	notifyEarlyStop := func() {
		earlyStopOnce.Do(func() { close(earlyStopSignal) })
	}

	// Start testing in goroutine to send results to channel
	go func() {
		onStart := func(name, proxyType string) {
			select {
			case progressChannel <- speedtester.Progress{Name: name, Type: proxyType, Phase: speedtester.PhaseLatency}:
			default:
			}
		}
		speedTester.TestProxiesUntil(allProxies, onStart, func(result *speedtester.Result) bool {
			resultChannel <- result
			if !stopper.ShouldContinue(result) {
				notifyEarlyStop()
				return false
			}
			return true
		})
		close(resultChannel)
	}()
	r.mu.Lock()
	r.current = speedTester
	r.mu.Unlock()
	if r.voided.Load() {
		// 队列已作废（Esc/退出全部）：停掉刚启动的测试，丢弃本轮。
		r.stopCurrent()
		return nil, true, nil
	}

	// Create and configure TUI for this round
	model := tui.NewTUIModelWithEngine(effectiveMode, len(allProxies), resultChannel, speedTester, progressChannel, earlyStopSignal)
	if r.escapeToParent {
		model.SetEscapeToParent(true)
	}
	model.SetImageExport(r.execDir, !*noImage)
	model.SetImageSpeedOnly(*imageSpeedOnly)
	model.NoteFastImageSpeedIgnored()
	model.SetImageSource(src.DisplayName)
	model.SetRoundLabel(round.label)
	model.SetAutoAdvance(round.hasMore)
	model.SetImageNameBase(nameBase)
	model.SetImageSeq(round.seq)
	if hasOutput {
		// 快照本轮最终路径：下一轮 prepare 会改写全局，保存回调不能读全局。
		roundSavePath := *outputPath
		model.SetConfigSaver(func(done []*speedtester.Result) (string, error) {
			sorted := output.SortResults(append([]*speedtester.Result(nil), done...), effectiveMode)
			// 各轮写盘与合并导出共用同一构建：先过筛重命名，再落盘并记入
			// 合并来源，保证两边口径完全一致（ADR-0014）。
			proxies := buildProxies(sorted, resultFilter)
			r.recordMergedRound(proxies)
			if err := writeConfigFile(roundSavePath, proxies); err != nil {
				return "", err
			}
			return roundSavePath, nil
		})
	}
	if uploadNeeded() {
		uploadCtx, cancelUpload := context.WithCancel(context.Background())
		uploadPath := *outputPath
		model.SetUploader(func() string {
			uploadConfig(uploadCtx, uploadPath)
			if uploadCtx.Err() != nil {
				return "上传已中断"
			}
			return "上传已结束"
		}, cancelUpload)
	}
	return model, false, nil
}

// stopCurrent 停掉最近一轮还在跑的测试：正常结束时它是空操作，
// Esc/退出全部后兜底掐掉在途 prepare 刚启动的测试 goroutine。
func (r *queueRunner) stopCurrent() {
	r.mu.Lock()
	tester := r.current
	r.current = nil
	r.mu.Unlock()
	if tester != nil {
		tester.Stop()
	}
}

// runQueueProgram 用一个 tea.Program 承载整个队列（ADR-0015）：每轮一个视图、
// 全部保留在内存里，轮与轮自动推进，界面全程不重启。outputOpen 表示输出模式
// 非关闭（「合并导出」的前提）。返回是否按 Esc 回选源界面。
func runQueueProgram(sources []picker.SourceSpec, execDir, originalOutputPath string, outputOpen, escapeToParent bool) bool {
	runner := &queueRunner{
		sources:            sources,
		execDir:            execDir,
		originalOutputPath: originalOutputPath,
		outputOpen:         outputOpen,
		escapeToParent:     escapeToParent,
		startSeq:           output.NextImageSequence(execDir),
		voided:             &atomic.Bool{},
	}
	// 第一轮在程序启动前备好：致命错误仍在进界面之前暴露（与现状一致），
	// 头几个 0 节点的源在这里跳过。
	firstIndex := 0
	var first tui.QueueRound
	for ; firstIndex < len(sources); firstIndex++ {
		view, skipped, err := runner.prepare(firstIndex)
		if err != nil {
			log.Fatalf("%s", err)
		}
		if !skipped {
			first = view
			break
		}
	}
	if first == nil {
		// 所有源都解析不出节点：逐源提示后直接结束（与现状一致）。
		return false
	}
	// 离开队列时兜底停掉可能还在后台跑的测试（正常结束是空操作）。
	defer runner.stopCurrent()

	q := tui.NewQueueModel(first, firstIndex, runner.prepare, len(sources), runner.voided)
	p := tea.NewProgram(
		q,
		tea.WithAltScreen(),
		tea.WithMouseAllMotion(),
	)
	final, err := p.Run()
	if err != nil {
		log.Fatalf("界面运行失败: %s", err)
	}
	result := final.(tui.QueueModel).Result()
	// 各轮离开 TUI 后补一行状态（已保存路径/失败原因），方便复制。
	for _, status := range result.Statuses {
		fmt.Fprintf(os.Stderr, "%s\n", status)
	}
	// 「合并导出」先于退出处理判断：Esc 回选源、Ctrl+C 退出全部、保存失败、
	// 准备错误都算队列未正常结束，不写（CONTEXT.md「合并导出」、ADR-0014）。
	if path := writeMergedExport(result, runner.outputOpen, execDir, runner.collectedRounds(), time.Now()); path != "" {
		fmt.Fprintf(os.Stderr, "已保存合并配置: %s\n", path)
	}
	if result.SetupErr != nil {
		log.Fatalf("%s", result.SetupErr)
	}
	if result.Escaped {
		// 按过 Esc：本轮中断、剩余轮作废，回选源界面重排。
		return true
	}
	if result.Failed {
		os.Exit(1)
	}
	return false
}

func imageSpeedFilterEnabled(mode speedtester.SpeedMode) bool {
	return *imageSpeedOnly && !mode.IsFast()
}

func exportNonInteractiveImage(execDir, imageSource, nameBase string, seq int, results []*speedtester.Result, mode speedtester.SpeedMode, total int, speedOnly bool) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	done := make(chan struct{})
	go func() {
		defer close(done)
		writeNonInteractiveImage(ctx, execDir, imageSource, nameBase, seq, results, mode, total, speedOnly)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		if err := output.RemovePartialImages(execDir); err != nil {
			fmt.Fprintf(os.Stderr, "删除半截图失败: %s\n", err)
		}
		os.Exit(1)
	}
}

func writeNonInteractiveImage(ctx context.Context, execDir, imageSource, nameBase string, seq int, results []*speedtester.Result, mode speedtester.SpeedMode, total int, speedOnly bool) {
	if ctx.Err() != nil {
		return
	}
	fmt.Fprintf(os.Stderr, "正在保存\n")
	status := "已完成"
	if *earlyStop > 0 && len(results) < total {
		status = "已提前结束"
	}
	rows := output.BuildImageRows(results, mode)
	filtered := output.FilterImageRowsBySpeed(rows, speedOnly)
	summary := output.SummaryLine(time.Now(), mode, status, len(results), total)
	if speedOnly {
		untested := max(total-len(results)-filtered.Testing, 0)
		summary = output.AppendImageSpeedCounts(summary, len(filtered.Rows), filtered.Invalid, filtered.Testing, untested)
	}
	spec := output.ImageSpec{
		Mode:     mode,
		Source:   imageSource,
		Summary:  summary,
		Headers:  output.GetHeaders(mode),
		Rows:     filtered.Rows,
		Now:      time.Now(),
		NameBase: nameBase,
		Seq:      seq,
	}
	path, warning, err := output.WriteResultImage(execDir, spec)
	if err != nil {
		fmt.Fprintf(os.Stderr, "保存结果图失败: %s\n", err)
		os.Exit(1)
		return
	}
	fmt.Fprintf(os.Stderr, "%s\n", output.JoinStatus("已保存 "+path, warning))
}

// saveConfigInterruptible 过筛、重命名并写盘，Ctrl+C 可中断。成功时返回
// 构建出的节点列表：与各轮写盘同一构建，供「合并导出」复用（ADR-0014）。
func saveConfigInterruptible(path string, results []*speedtester.Result, filter resultFilter) ([]map[string]any, error) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	type saveOutcome struct {
		proxies []map[string]any
		err     error
	}
	ch := make(chan saveOutcome, 1)
	go func() {
		if ctx.Err() != nil {
			ch <- saveOutcome{err: ctx.Err()}
			return
		}
		proxies := buildProxies(results, filter)
		ch <- saveOutcome{proxies: proxies, err: writeConfigFile(path, proxies)}
	}()
	select {
	case out := <-ch:
		return out.proxies, out.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// buildProxies 按过筛口径把测速结果收成写盘用的节点列表：各轮写盘与合并
// 导出共用同一构建，保证过滤与重命名口径完全一致（ADR-0014）。
func buildProxies(results []*speedtester.Result, filter resultFilter) []map[string]any {
	proxies := make([]map[string]any, 0)
	nameCount := make(map[string]int) // Track name usage to avoid duplicates

	for _, result := range results {
		if !filter.Match(result) {
			continue
		}

		proxyConfig := result.ProxyConfig
		if proxyConfig["name"] == nil || proxyConfig["server"] == nil {
			continue
		}
		if *renameNodes {
			location, err := ip.GetIPLocation(proxyConfig["server"].(string))
			if err != nil || location.CountryCode == "" {
				proxies = append(proxies, proxyConfig)
				continue
			}
			name, err := ip.GenerateNodeNameFromTemplate(*renameTemplate, location.CountryCode, result.Latency, result.DownloadSpeed, result.UploadSpeed, nameCount)
			if err != nil {
				log.Printf("重命名模板解析失败: %s，改用默认名称", err)
				name = ip.GenerateNodeName(location.CountryCode, result.Latency, result.DownloadSpeed, result.UploadSpeed, nameCount)
			}
			proxyConfig["name"] = name
		}
		proxies = append(proxies, proxyConfig)
	}
	return proxies
}

// writeConfigFile 把节点列表 marshal 成合格配置写盘。路径由调用方显式传入：
// 队列壳下各轮并存，不读全局，避免与下一轮的全局改写竞争。
func writeConfigFile(path string, proxies []map[string]any) error {
	config := &speedtester.RawConfig{
		Proxies: proxies,
	}
	yamlData, err := yaml.Marshal(config)
	if err != nil {
		return err
	}

	if err := os.WriteFile(path, yamlData, 0o644); err != nil {
		return err
	}
	return nil
}

// writeMergedExport 按「合并导出」写合并文件（CONTEXT.md「合并导出」词条、
// ADR-0014）：输出模式非关闭、队列正常结束（无 Esc 回选源、无 Ctrl+C 退出
// 全部、无保存失败、无准备错误）、且至少两轮正常测完才写。内容是各轮过筛
// 节点（与各轮写盘同一构建）按完成顺序求并集，重名节点跳过、保留先完成轮
// 的；文件名「导出-合并-时间戳」不带产物序号，也不参与 Gist/仓库上传。
// 非交互路径没有中断语义，传零值 QueueResult 即视为正常结束。返回写入路径；
// 没触发或写失败返回空串，写失败只报 stderr 不影响退出码。
func writeMergedExport(result tui.QueueResult, outputOpen bool, execDir string, rounds [][]map[string]any, now time.Time) string {
	if result.Escaped || result.ExitAll || result.Failed || result.SetupErr != nil {
		return ""
	}
	if !outputOpen || len(rounds) < 2 {
		return ""
	}
	seen := make(map[string]bool)
	proxies := make([]map[string]any, 0)
	for _, round := range rounds {
		for _, proxy := range round {
			name := fmt.Sprint(proxy["name"])
			if seen[name] {
				continue
			}
			seen[name] = true
			proxies = append(proxies, proxy)
		}
	}
	path := filepath.Join(execDir, output.MergedExportPath(now))
	if err := writeConfigFile(path, proxies); err != nil {
		fmt.Fprintf(os.Stderr, "保存合并配置失败: %s\n", err)
		return ""
	}
	return path
}

// uploadConfig 把已写好的 yaml 同步到 Gist 或仓库。失败只记日志，不影响退出码。
// path 是本轮的最终输出路径，由调用方显式传入：队列壳下各轮并存，
// 不再读全局，避免与下一轮的全局改写竞争。
func uploadConfig(ctx context.Context, path string) {
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("读取待上传配置失败: %s", err)
		return
	}
	outputFilename := filepath.Base(filepath.Clean(path))
	uploader := gist.NewUploader(nil)
	if *gistToken != "" && *gistAddress != "" {
		if ctx.Err() != nil {
			return
		}
		if err := uploader.UpdateFile(*gistToken, *gistAddress, outputFilename, data); err != nil {
			log.Printf("更新 Gist 失败: %s", err)
		}
	}
	if *repoToken != "" && *repoAddress != "" {
		if ctx.Err() != nil {
			return
		}
		repositoryFilePath := strings.TrimSpace(*repoFilePath)
		if repositoryFilePath == "" {
			repositoryFilePath = outputFilename
		}
		if err := uploader.UpdateRepoFile(*repoToken, *repoAddress, repositoryFilePath, *repoBranch, data); err != nil {
			log.Printf("更新仓库文件失败: %s", err)
		}
	}
}

func uploadNeeded() bool {
	return (*gistToken != "" && *gistAddress != "") || (*repoToken != "" && *repoAddress != "")
}

// intFlag 把短名和长名绑到同一个整数。标准库一个 flag 只能有一个名字，
// 长名仍可解析，但帮助里只留短名一行并注明长名。
func intFlag(shortName, longName string, value int, usage string) *int {
	p := flag.Int(shortName, value, usage+"（也可写 -"+longName+"）")
	flag.IntVar(p, longName, value, hiddenFlagUsage)
	return p
}

func stringFlag(shortName, longName, value, usage string) *string {
	p := flag.String(shortName, value, usage+"（也可写 -"+longName+"）")
	flag.StringVar(p, longName, value, hiddenFlagUsage)
	return p
}

// 简写和常用参数排在帮助前面，其余按名字排序。
var commonFlagOrder = []string{
	"c", "f", "b", "o", "p", "v", "fast", "speed-mode",
	"concurrent", "download-size", "upload-size", "timeout",
	"early-stop", "no-image",
}

const hiddenFlagUsage = "\x00"

func printFlagDefaults(fs *flag.FlagSet) {
	var flags []*flag.Flag
	fs.VisitAll(func(f *flag.Flag) {
		if f.Usage == hiddenFlagUsage {
			return
		}
		flags = append(flags, f)
	})
	order := map[string]int{}
	for i, name := range commonFlagOrder {
		order[name] = i
	}
	sort.SliceStable(flags, func(i, j int) bool {
		ai, aok := order[flags[i].Name]
		bi, bok := order[flags[j].Name]
		switch {
		case aok && bok:
			return ai < bi
		case aok:
			return true
		case bok:
			return false
		default:
			return flags[i].Name < flags[j].Name
		}
	})
	short := true
	for _, f := range flags {
		if short && len(f.Name) > 1 {
			fmt.Fprint(fs.Output(), "\n")
			short = false
		}
		name, usage := flag.UnquoteUsage(f)
		s := fmt.Sprintf("  -%s", f.Name)
		if len(name) > 0 {
			s += " " + name
		}
		if len(s) <= 4 {
			s += "\t"
		} else {
			s += "\n    \t"
		}
		s += formatFlagUsage(usage, f)
		fmt.Fprint(fs.Output(), s, "\n")
	}
}

// formatFlagUsage 把默认值和单位收进同一对括号，避免帮助里再单独折一行。
func formatFlagUsage(usage string, f *flag.Flag) string {
	usage = strings.ReplaceAll(usage, "\n", " ")
	unit := ""
	alias := ""
	if before, after, ok := strings.Cut(usage, "（单位："); ok {
		usage = strings.TrimSpace(before)
		unit, _, _ = strings.Cut(after, "）")
		unit, _, _ = strings.Cut(unit, "，")
	}
	if before, after, ok := strings.Cut(usage, "（也可写 -"); ok {
		usage = strings.TrimSpace(before)
		alias, _, _ = strings.Cut(after, "）")
		alias = "-" + alias
	}
	var notes []string
	if alias != "" {
		notes = append(notes, "也可写 "+alias)
	}
	if !isZeroValue(f, f.DefValue) {
		notes = append(notes, "默认: "+formatFlagDefault(f.DefValue))
	}
	if unit != "" {
		notes = append(notes, "单位："+unit)
	}
	if len(notes) == 0 {
		return usage
	}
	if usage == "" {
		return "（" + strings.Join(notes, " | ") + "）"
	}
	return usage + "（" + strings.Join(notes, " | ") + "）"
}

func formatFlagDefault(value string) string {
	if value == "" {
		return `""`
	}
	return value
}

func isZeroValue(f *flag.Flag, value string) bool {
	typ := reflect.TypeOf(f.Value)
	var z reflect.Value
	if typ.Kind() == reflect.Pointer {
		z = reflect.New(typ.Elem())
	} else {
		z = reflect.Zero(typ)
	}
	return value == z.Interface().(flag.Value).String()
}

// waitForUpload 非交互等待上传。Ctrl+C 立刻中断，并在 stderr 写正在上传。
func waitForUpload() {
	if !uploadNeeded() || *outputPath == "" {
		return
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	fmt.Fprintf(os.Stderr, "正在上传\n")
	done := make(chan struct{})
	go func() {
		defer close(done)
		uploadConfig(ctx, *outputPath)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		fmt.Fprintf(os.Stderr, "上传已中断\n")
	}
}
