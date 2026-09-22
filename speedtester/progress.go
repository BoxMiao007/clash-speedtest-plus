package speedtester

import (
	"context"
	"io"
	"sync/atomic"
	"time"
)

const progressInterval = 200 * time.Millisecond

type Phase int

const (
	PhaseLatency Phase = iota
	PhaseDownload
	PhaseUpload
)

type Progress struct {
	Name          string
	Type          string
	Phase         Phase
	Latency       time.Duration
	Jitter        time.Duration
	PacketLoss    float64
	InstantSpeed  float64
	DownloadSpeed float64
	UploadSpeed   float64
}

type ProgressFunc func(Progress)

func InstantSpeed(bytes int64, elapsed time.Duration) float64 {
	if elapsed <= 0 || bytes <= 0 {
		return 0
	}
	return float64(bytes) / elapsed.Seconds()
}

type byteCounter struct {
	n atomic.Int64
}

func (c *byteCounter) add(n int64) {
	if c == nil || n <= 0 {
		return
	}
	c.n.Add(n)
}

func (c *byteCounter) load() int64 {
	if c == nil {
		return 0
	}
	return c.n.Load()
}

type countingReader struct {
	r io.Reader
	c *byteCounter
}

func (cr *countingReader) Read(p []byte) (int, error) {
	n, err := cr.r.Read(p)
	cr.c.add(int64(n))
	return n, err
}

func (st *SpeedTester) SetProgressFunc(fn ProgressFunc) {
	st.onProgress = fn
}

func (st *SpeedTester) emitProgress(p Progress) {
	if st == nil || st.onProgress == nil {
		return
	}
	st.onProgress(p)
}

func (st *SpeedTester) watchProgress(emit func()) func() {
	if st.onProgress == nil {
		return func() {}
	}
	ctx, cancel := context.WithCancel(st.ctx)
	go func() {
		ticker := time.NewTicker(progressInterval)
		defer ticker.Stop()
		emit()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				emit()
			}
		}
	}()
	return cancel
}
