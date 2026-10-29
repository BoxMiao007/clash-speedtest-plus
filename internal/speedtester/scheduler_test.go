package speedtester

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunProxyTestsSerialOrder(t *testing.T) {
	tasks := []proxyTask{{name: "c"}, {name: "a"}, {name: "b"}}
	var got []string
	runProxyTests(context.Background(), 1, nil, tasks, func(name string, _ *CProxy) *Result {
		return &Result{ProxyName: name}
	}, nil, func(result *Result) bool {
		got = append(got, result.ProxyName)
		return true
	})
	want := []string{"c", "a", "b"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestSortedProxyTasks(t *testing.T) {
	proxies := map[string]*CProxy{
		"zeta":  {},
		"alpha": {},
		"mu":    {},
	}
	tasks := sortedProxyTasks(proxies)
	if len(tasks) != 3 {
		t.Fatalf("got %d tasks", len(tasks))
	}
	if tasks[0].name != "alpha" || tasks[1].name != "mu" || tasks[2].name != "zeta" {
		t.Fatalf("unexpected order: %+v", tasks)
	}
}

func TestRunProxyTestsParallelCap(t *testing.T) {
	tasks := make([]proxyTask, 8)
	for i := range tasks {
		tasks[i] = proxyTask{name: string(rune('a' + i))}
	}
	var current, max int32
	var mu sync.Mutex
	runProxyTests(context.Background(), 3, nil, tasks, func(name string, _ *CProxy) *Result {
		n := atomic.AddInt32(&current, 1)
		mu.Lock()
		if n > max {
			max = n
		}
		mu.Unlock()
		time.Sleep(40 * time.Millisecond)
		atomic.AddInt32(&current, -1)
		return &Result{ProxyName: name}
	}, nil, func(*Result) bool { return true })
	if max > 3 {
		t.Fatalf("parallel cap exceeded: max in-flight %d", max)
	}
	if max < 2 {
		t.Fatalf("expected some parallelism, max in-flight %d", max)
	}
}

func TestRunProxyTestsEarlyStopDrainsInFlight(t *testing.T) {
	tasks := make([]proxyTask, 10)
	for i := range tasks {
		tasks[i] = proxyTask{name: string(rune('a' + i))}
	}
	var started atomic.Int32
	var finished atomic.Int32
	runProxyTests(context.Background(), 3, nil, tasks, func(name string, _ *CProxy) *Result {
		started.Add(1)
		time.Sleep(30 * time.Millisecond)
		return &Result{ProxyName: name}
	}, nil, func(*Result) bool {
		n := finished.Add(1)
		return n < 2
	})
	if finished.Load() < 2 {
		t.Fatalf("expected at least 2 finished, got %d", finished.Load())
	}
	if started.Load() > 2+3-1 {
		t.Fatalf("too many started after early stop: started=%d finished=%d", started.Load(), finished.Load())
	}
	if started.Load() != finished.Load() {
		t.Fatalf("in-flight should drain: started=%d finished=%d", started.Load(), finished.Load())
	}
}

func TestPauseGateBlocksDispatch(t *testing.T) {
	pause := NewPauseGate()
	pause.Pause()
	tasks := []proxyTask{{name: "a"}, {name: "b"}}
	var started atomic.Int32
	done := make(chan struct{})
	go func() {
		runProxyTests(context.Background(), 1, pause, tasks, func(name string, _ *CProxy) *Result {
			started.Add(1)
			return &Result{ProxyName: name}
		}, nil, func(*Result) bool { return true })
		close(done)
	}()
	time.Sleep(50 * time.Millisecond)
	if started.Load() != 0 {
		t.Fatalf("dispatch should wait while paused, started=%d", started.Load())
	}
	pause.Resume()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for resume")
	}
	if started.Load() != 2 {
		t.Fatalf("expected 2 started after resume, got %d", started.Load())
	}
}

func TestOnStartCalledBeforeResult(t *testing.T) {
	tasks := []proxyTask{{name: "a"}}
	var events []string
	runProxyTests(context.Background(), 1, nil, tasks, func(name string, _ *CProxy) *Result {
		events = append(events, "test:"+name)
		return &Result{ProxyName: name}
	}, func(name, _ string) {
		events = append(events, "start:"+name)
	}, func(*Result) bool {
		events = append(events, "done")
		return true
	})
	want := []string{"start:a", "test:a", "done"}
	if len(events) != len(want) {
		t.Fatalf("got %v, want %v", events, want)
	}
	for i := range want {
		if events[i] != want[i] {
			t.Fatalf("got %v, want %v", events, want)
		}
	}
}
