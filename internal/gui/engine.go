package gui

import (
	"context"
	"sync"
	"time"

	"github.com/BoxMiao007/clash-speedtest-plus/internal/export"
	"github.com/BoxMiao007/clash-speedtest-plus/internal/speedtester"
	"github.com/BoxMiao007/clash-speedtest-plus/internal/store"
)

const megabyte = 1024 * 1024

// TestConfig 是引擎需要的那部分设置。开测前由 testConfigFromSettings 从
// settings 快照生成，运行中改设置不影响本轮。
type TestConfig struct {
	Mode       string
	DownloadMB int
	UploadMB   int
	TimeoutMS  int
	Concurrent int
	Parallel   int
	ServerURL  string
	UserAgent  string
	EarlyStop  int

	MaxLatencyMS    int
	MaxPacketLoss   float64
	MinDownloadMBps float64
	MinUploadMBps   float64
}

// testConfigFromSettings 把持久化设置折算成引擎参数；非法值按 Normalize
// 的同一套默认值兜底。
func testConfigFromSettings(settings store.Settings) TestConfig {
	settings = settings.Normalize()
	return TestConfig{
		Mode:            settings.Mode,
		DownloadMB:      settings.DownloadMB,
		UploadMB:        settings.UploadMB,
		TimeoutMS:       settings.TimeoutMS,
		Concurrent:      settings.Concurrent,
		Parallel:        settings.Parallel,
		ServerURL:       settings.ServerURL,
		UserAgent:       settings.UserAgent,
		EarlyStop:       settings.EarlyStop,
		MaxLatencyMS:    settings.MaxLatencyMS,
		MaxPacketLoss:   settings.MaxPacketLoss,
		MinDownloadMBps: settings.MinDownloadMBps,
		MinUploadMBps:   settings.MinUploadMBps,
	}
}

// testFilter 把测试参数折成导出/提前结束共用的阈值过滤。
func testFilter(config TestConfig) export.Filter {
	mode, err := speedtester.ParseSpeedMode(config.Mode)
	if err != nil {
		mode = speedtester.SpeedModeDownload
	}
	return export.Filter{
		MaxLatency:     time.Duration(config.MaxLatencyMS) * time.Millisecond,
		MaxPacketLoss:  config.MaxPacketLoss,
		MinDownloadBps: config.MinDownloadMBps * megabyte,
		MinUploadBps:   config.MinUploadMBps * megabyte,
		DownloadTested: !mode.IsFast(),
		UploadEnabled:  mode.UploadEnabled(),
	}
}

// engineRunner 是 Runner 的生产实现：只做参数组装与回调转接，测速逻辑仍在
// internal/speedtester，未重写。
type engineRunner struct {
	mu     sync.Mutex
	config TestConfig
	tester *speedtester.SpeedTester
}

// Configure 保存本轮参数；Run 时再组装成 speedtester.Config。
func (runner *engineRunner) Configure(config TestConfig) {
	runner.mu.Lock()
	runner.config = config
	runner.mu.Unlock()
}

func (runner *engineRunner) currentConfig() TestConfig {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.config
}

func (runner *engineRunner) Run(ctx context.Context, batch []RunNode, handlers Handlers) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	settings := runner.currentConfig()
	mode, err := speedtester.ParseSpeedMode(settings.Mode)
	if err != nil {
		mode = speedtester.SpeedModeDownload
	}
	config := &speedtester.Config{
		FilterRegex:      ".+",
		ServerURL:        settings.ServerURL,
		DownloadSize:     settings.DownloadMB * megabyte,
		UploadSize:       settings.UploadMB * megabyte,
		Timeout:          time.Duration(settings.TimeoutMS) * time.Millisecond,
		Concurrent:       settings.Concurrent,
		Parallel:         settings.Parallel,
		MaxLatency:       time.Duration(settings.MaxLatencyMS) * time.Millisecond,
		MaxPacketLoss:    settings.MaxPacketLoss,
		MinDownloadSpeed: settings.MinDownloadMBps * megabyte,
		MinUploadSpeed:   settings.MinUploadMBps * megabyte,
		Mode:             mode,
		UserAgent:        settings.UserAgent,
	}
	tester, err := speedtester.New(config)
	if err != nil {
		return err
	}
	runner.mu.Lock()
	runner.tester = tester
	runner.mu.Unlock()
	defer func() {
		runner.mu.Lock()
		runner.tester = nil
		runner.mu.Unlock()
	}()

	proxies := make(map[string]*speedtester.CProxy, len(batch))
	for _, node := range batch {
		if node.Proxy == nil {
			continue
		}
		proxies[node.Key] = node.Proxy
	}
	if handlers.Progress != nil {
		tester.SetProgressFunc(handlers.Progress)
	}
	filter := testFilter(settings)
	matched := 0
	tester.TestProxiesUntil(proxies, handlers.Start, func(result *speedtester.Result) bool {
		if handlers.Result != nil {
			handlers.Result(result)
		}
		if settings.EarlyStop <= 0 {
			return true
		}
		if !filter.Match(result) {
			return true
		}
		matched++
		if matched < settings.EarlyStop {
			return true
		}
		if handlers.EarlyStop != nil {
			handlers.EarlyStop()
		}
		return false
	})
	return nil
}

func (runner *engineRunner) Pause() {
	runner.mu.Lock()
	tester := runner.tester
	runner.mu.Unlock()
	if tester != nil {
		tester.Pause()
	}
}

func (runner *engineRunner) Resume() {
	runner.mu.Lock()
	tester := runner.tester
	runner.mu.Unlock()
	if tester != nil {
		tester.Resume()
	}
}

func (runner *engineRunner) Stop() {
	runner.mu.Lock()
	tester := runner.tester
	runner.mu.Unlock()
	if tester != nil {
		tester.Stop()
	}
}
