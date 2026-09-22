package speedtester

import (
	"context"
	"sort"
	"sync"
)

// StartFunc is called when a node begins testing. It may be nil.
type StartFunc func(name, proxyType string)

// PauseGate blocks dispatch of new nodes while paused. In-flight tests keep running.
type PauseGate struct {
	mu      sync.Mutex
	paused  bool
	waiters []chan struct{}
}

func NewPauseGate() *PauseGate {
	return &PauseGate{}
}

func (g *PauseGate) Pause() {
	if g == nil {
		return
	}
	g.mu.Lock()
	g.paused = true
	g.mu.Unlock()
}

func (g *PauseGate) Resume() {
	if g == nil {
		return
	}
	g.mu.Lock()
	g.paused = false
	for _, ch := range g.waiters {
		close(ch)
	}
	g.waiters = nil
	g.mu.Unlock()
}

func (g *PauseGate) Paused() bool {
	if g == nil {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.paused
}

func (g *PauseGate) WaitIfPaused(ctx context.Context) error {
	if g == nil {
		return ctx.Err()
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		g.mu.Lock()
		if !g.paused {
			g.mu.Unlock()
			return ctx.Err()
		}
		ch := make(chan struct{})
		g.waiters = append(g.waiters, ch)
		g.mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ch:
		}
	}
}

type proxyTask struct {
	name  string
	proxy *CProxy
}

func sortedProxyTasks(proxies map[string]*CProxy) []proxyTask {
	names := make([]string, 0, len(proxies))
	for name := range proxies {
		names = append(names, name)
	}
	sort.Strings(names)
	tasks := make([]proxyTask, 0, len(names))
	for _, name := range names {
		tasks = append(tasks, proxyTask{name: name, proxy: proxies[name]})
	}
	return tasks
}

// runProxyTests dispatches tasks with a worker cap. tester is serialized.
// Returning false from tester stops dispatching new tasks; in-flight tasks still finish.
func runProxyTests(
	ctx context.Context,
	parallel int,
	pause *PauseGate,
	tasks []proxyTask,
	testFn func(name string, proxy *CProxy) *Result,
	onStart StartFunc,
	tester func(*Result) bool,
) {
	if parallel <= 1 {
		runProxyTestsSerial(ctx, pause, tasks, testFn, onStart, tester)
		return
	}

	dispatchCtx, stopDispatch := context.WithCancel(ctx)
	defer stopDispatch()

	sem := make(chan struct{}, parallel)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, task := range tasks {
		if err := pause.WaitIfPaused(dispatchCtx); err != nil {
			break
		}
		if dispatchCtx.Err() != nil {
			break
		}
		select {
		case <-dispatchCtx.Done():
			goto wait
		case sem <- struct{}{}:
		}
		if err := pause.WaitIfPaused(dispatchCtx); err != nil {
			<-sem
			break
		}
		if dispatchCtx.Err() != nil {
			<-sem
			break
		}
		task := task
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			if ctx.Err() != nil {
				return
			}
			if onStart != nil && task.proxy != nil {
				onStart(task.name, task.proxy.Type().String())
			} else if onStart != nil {
				onStart(task.name, "")
			}
			result := testFn(task.name, task.proxy)
			if result == nil {
				return
			}
			mu.Lock()
			cont := tester(result)
			if !cont {
				stopDispatch()
			}
			mu.Unlock()
		}()
	}
wait:
	wg.Wait()
}

func runProxyTestsSerial(
	ctx context.Context,
	pause *PauseGate,
	tasks []proxyTask,
	testFn func(name string, proxy *CProxy) *Result,
	onStart StartFunc,
	tester func(*Result) bool,
) {
	for _, task := range tasks {
		if err := pause.WaitIfPaused(ctx); err != nil {
			return
		}
		if ctx.Err() != nil {
			return
		}
		if onStart != nil && task.proxy != nil {
			onStart(task.name, task.proxy.Type().String())
		} else if onStart != nil {
			onStart(task.name, "")
		}
		result := testFn(task.name, task.proxy)
		if result == nil {
			return
		}
		if !tester(result) {
			return
		}
	}
}
