package gui

import (
	"context"
	"sync"
	"time"

	"github.com/BoxMiao007/clash-speedtest-plus/internal/speedtester"
)

const (
	defaultServerURL  = "https://dl.google.com/chrome/mac/universal/stable/GGRO/googlechrome.dmg"
	defaultDownloadMB = 50
	defaultUploadMB   = 20
	defaultParallel   = 6
	defaultConcurrent = 4
	defaultTimeout    = 5 * time.Second
)

// engineRunner 是 Runner 的生产实现：只做参数组装与回调转接，测速逻辑仍在
// internal/speedtester，未重写。
type engineRunner struct {
	mu     sync.Mutex
	tester *speedtester.SpeedTester
}

func (runner *engineRunner) Run(ctx context.Context, batch []RunNode, handlers Handlers) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	config := &speedtester.Config{
		FilterRegex:   ".+",
		ServerURL:     defaultServerURL,
		DownloadSize:  defaultDownloadMB * 1024 * 1024,
		UploadSize:    defaultUploadMB * 1024 * 1024,
		Timeout:       defaultTimeout,
		Concurrent:    defaultConcurrent,
		Parallel:      defaultParallel,
		MaxLatency:    0,
		MaxPacketLoss: 100,
		Mode:          speedtester.SpeedModeDownload,
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
	tester.TestProxiesUntil(proxies, handlers.Start, func(result *speedtester.Result) bool {
		if handlers.Result != nil {
			handlers.Result(result)
		}
		return true
	})
	return nil
}

func (runner *engineRunner) Stop() {
	runner.mu.Lock()
	tester := runner.tester
	runner.mu.Unlock()
	if tester != nil {
		tester.Stop()
	}
}
