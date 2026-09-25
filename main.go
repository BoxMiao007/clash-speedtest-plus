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
	"strings"
	"sync"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/faceair/clash-speedtest/gist"
	"github.com/faceair/clash-speedtest/ip"
	"github.com/faceair/clash-speedtest/output"
	"github.com/faceair/clash-speedtest/speedtester"
	"github.com/faceair/clash-speedtest/tui"
	mihomolog "github.com/metacubex/mihomo/log"
	"gopkg.in/yaml.v2"
)

// Version information injected via ldflags during build
var (
	version = "dev"
	commit  = "unknown"
)

var (
	configPathsConfig = flag.String("c", "", "配置文件路径，也支持 http(s) 地址")
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
	outputPath        = stringFlag("o", "output", "", "输出配置文件路径")
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

func main() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "用法：clash-speedtest [选项]\n")
		printFlagDefaults(flag.CommandLine)
	}
	flag.Parse()
	mihomolog.SetLevel(mihomolog.SILENT)

	// Handle version flag
	if *versionFlag {
		fmt.Printf("clash-speedtest version %s (commit %s)\n", version, commit)
		os.Exit(0)
	}

	if *configPathsConfig == "" {
		log.Fatalln("请指定配置文件")
	}

	var err error
	requestedMode := speedtester.SpeedModeFast
	if !*fastMode {
		requestedMode, err = speedtester.ParseSpeedMode(*speedMode)
		if err != nil {
			log.Fatalf("解析测速模式失败: %s", err)
		}
	}

	speedTester, err := speedtester.New(&speedtester.Config{
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
		log.Fatalf("创建测速器失败: %s", err)
	}
	effectiveMode := speedTester.Mode()
	resultFilter := newResultFilter(effectiveMode)
	stopper, err := newEarlyStopper(*earlyStop, resultFilter)
	if err != nil {
		log.Fatalf("创建提前结束失败: %s", err)
	}

	allProxies, err := speedTester.LoadProxies()
	if err != nil {
		log.Fatalf("加载节点失败: %s", err)
	}

	outputMode := output.DetermineOutputMode(output.IsTerminalFile)

	var tsvWriter *output.TSVWriter
	if outputMode == output.OutputModeTSV {
		var err error
		tsvWriter, err = output.NewTSVWriter(os.Stdout, effectiveMode)
		if err != nil {
			log.Fatalf("创建 TSV 输出失败: %s", err)
		}
	}

	results := make([]*speedtester.Result, 0, len(allProxies))

	if outputMode == output.OutputModeInteractive {
		collectResults := *outputPath != ""
		// Run TUI for Interactive mode
		resultChannel := make(chan *speedtester.Result, len(allProxies))
		resultsDone := make(chan struct{})

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
				if collectResults {
					results = append(results, result)
				}
				resultChannel <- result
				if !stopper.ShouldContinue(result) {
					notifyEarlyStop()
					return false
				}
				return true
			})
			close(resultChannel)
			close(resultsDone)
		}()

		// Create and run TUI
		model := tui.NewTUIModelWithEngine(effectiveMode, len(allProxies), resultChannel, speedTester, progressChannel, earlyStopSignal)
		model.SetImageExport(".", !*noImage)
		model.SetImageSpeedOnly(*imageSpeedOnly)
		model.NoteFastImageSpeedIgnored()
		model.SetImageSource(*configPathsConfig)
		if collectResults {
			model.SetConfigSaver(func(done []*speedtester.Result) (string, error) {
				sorted := output.SortResults(append([]*speedtester.Result(nil), done...), effectiveMode)
				if err := saveConfig(sorted, resultFilter); err != nil {
					return "", err
				}
				return *outputPath, nil
			})
		}
		if uploadNeeded() {
			uploadCtx, cancelUpload := context.WithCancel(context.Background())
			defer cancelUpload()
			model.SetUploader(func() string {
				uploadConfig(uploadCtx)
				if uploadCtx.Err() != nil {
					return "上传已中断"
				}
				return "上传已结束"
			}, cancelUpload)
		}
		p := tea.NewProgram(
			model,
			tea.WithAltScreen(),
			tea.WithMouseAllMotion(),
		)
		finalModel, err := p.Run()
		if err != nil {
			log.Fatalf("界面运行失败: %s", err)
		}
		finished, _ := finalModel.(tui.Model)
		status, failed := finished.ExitStatus()
		if status != "" {
			fmt.Fprintf(os.Stderr, "%s\n", status)
		}
		if failed {
			os.Exit(1)
		}
		return
	}

	// TSV mode: collect results synchronously
	speedTester.TestProxiesUntil(allProxies, nil, func(result *speedtester.Result) bool {
		results = append(results, result)

		if tsvWriter != nil {
			if err := tsvWriter.WriteRow(result, len(results)-1); err != nil {
				log.Printf("写入 TSV 行失败: %s", err)
			}
		}
		return stopper.ShouldContinue(result)
	})

	results = output.SortResults(results, effectiveMode)

	if *outputPath != "" {
		err = saveConfigInterruptible(results, resultFilter)
		if err != nil {
			fmt.Fprintf(os.Stderr, "保存配置失败: %s\n", err)
			os.Exit(1)
		}
		// 非交互模式路径走 stderr，stdout 保留给 TSV/管道输出。
		fmt.Fprintf(os.Stderr, "已保存配置: %s\n", *outputPath)
	}
	if effectiveMode.IsFast() && *imageSpeedOnly {
		fmt.Fprintf(os.Stderr, "%s\n", tui.FastImageSpeedIgnored())
	}
	if !*noImage {
		exportNonInteractiveImage(results, effectiveMode, len(allProxies), imageSpeedFilterEnabled(effectiveMode))
	}
	waitForUpload()
}

func imageSpeedFilterEnabled(mode speedtester.SpeedMode) bool {
	return *imageSpeedOnly && !mode.IsFast()
}

func exportNonInteractiveImage(results []*speedtester.Result, mode speedtester.SpeedMode, total int, speedOnly bool) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	done := make(chan struct{})
	go func() {
		defer close(done)
		writeNonInteractiveImage(ctx, results, mode, total, speedOnly)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		if err := output.RemovePartialImages("."); err != nil {
			fmt.Fprintf(os.Stderr, "删除半截图失败: %s\n", err)
		}
		os.Exit(1)
	}
}

func writeNonInteractiveImage(ctx context.Context, results []*speedtester.Result, mode speedtester.SpeedMode, total int, speedOnly bool) {
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
		summary = output.AppendImageSpeedCounts(summary, len(filtered.Rows), filtered.Invalid, filtered.Testing)
	}
	spec := output.ImageSpec{
		Mode:    mode,
		Source:  *configPathsConfig,
		Summary: summary,
		Headers: output.GetHeaders(mode),
		Rows:    filtered.Rows,
		Now:     time.Now(),
	}
	path, warning, err := output.WriteResultImage(".", spec)
	if err != nil {
		fmt.Fprintf(os.Stderr, "保存结果图失败: %s\n", err)
		os.Exit(1)
		return
	}
	fmt.Fprintf(os.Stderr, "%s\n", output.JoinStatus("已保存 "+path, warning))
}

func saveConfigInterruptible(results []*speedtester.Result, filter resultFilter) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errCh := make(chan error, 1)
	go func() {
		if ctx.Err() != nil {
			errCh <- ctx.Err()
			return
		}
		errCh <- saveConfig(results, filter)
	}()
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func saveConfig(results []*speedtester.Result, filter resultFilter) error {
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

	config := &speedtester.RawConfig{
		Proxies: proxies,
	}
	yamlData, err := yaml.Marshal(config)
	if err != nil {
		return err
	}

	if err := os.WriteFile(*outputPath, yamlData, 0o644); err != nil {
		return err
	}
	return nil
}

// uploadConfig 把已写好的 yaml 同步到 Gist 或仓库。失败只记日志，不影响退出码。
func uploadConfig(ctx context.Context) {
	if *outputPath == "" {
		return
	}
	data, err := os.ReadFile(*outputPath)
	if err != nil {
		log.Printf("读取待上传配置失败: %s", err)
		return
	}
	outputFilename := filepath.Base(filepath.Clean(*outputPath))
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
		uploadConfig(ctx)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		fmt.Fprintf(os.Stderr, "上传已中断\n")
	}
}
