package gui

import (
	"testing"
	"time"

	"github.com/BoxMiao007/clash-speedtest-plus/internal/store"
)

// 删除条目后轮次账本必须裁掉该条目，避免结果条/导出继续引用已删节点。
func TestPruneRoundsAfterEntryRemoved(t *testing.T) {
	s := &state{}
	e := &entryState{entry: store.Entry{ID: "e1", Name: "A"}}
	n1 := &nodeState{entry: e, key: "e1/n1"}
	n2 := &nodeState{entry: e, key: "e1/n2"}
	e.nodes = []*nodeState{n1, n2}
	s.entries = []*entryState{e}
	s.rounds = []*roundState{{entry: e, nodes: []*nodeState{n1, n2}}}
	s.roundIndex = 0

	s.entries = nil // 模拟条目已删除
	s.pruneRounds()
	if len(s.rounds) != 0 {
		t.Fatalf("删除条目后应清空轮次账本，实际剩 %d 轮", len(s.rounds))
	}
}

// 重读后节点变少时，轮次里只保留仍存在的节点，并重映射到当前节点对象。
func TestPruneRoundsDropsRemovedNodes(t *testing.T) {
	s := &state{}
	e := &entryState{entry: store.Entry{ID: "e1", Name: "A"}}
	n1 := &nodeState{entry: e, key: "e1/n1"}
	n2 := &nodeState{entry: e, key: "e1/n2"}
	s.entries = []*entryState{e}
	s.rounds = []*roundState{{entry: e, nodes: []*nodeState{n1, n2}}}
	s.roundIndex = 0

	fresh := &nodeState{entry: e, key: "e1/n1"} // 重读后仅剩 n1，且是新对象
	e.nodes = []*nodeState{fresh}
	s.pruneRounds()
	if len(s.rounds) != 1 || len(s.rounds[0].nodes) != 1 {
		t.Fatalf("应只保留 1 轮 1 节点，实际 %d 轮", len(s.rounds))
	}
	if s.rounds[0].nodes[0] != fresh {
		t.Fatal("轮次节点应重映射到当前条目里的新节点对象")
	}
}

// 自动导出同轮写配置+图片时，预览路径必须只指向 PNG，而不是复合串。
func TestSetExportStatusTracksImagePath(t *testing.T) {
	s := &state{}
	s.setExportStatus(runEvent{path: "/x/1.a-导出.yaml", imagePath: "/x/1.a-1.png"})
	if s.lastImage != "/x/1.a-1.png" {
		t.Fatalf("lastImage 应为图片路径，实际 %q", s.lastImage)
	}
	if s.status != "已导出：/x/1.a-导出.yaml" {
		t.Fatalf("状态文案应以配置路径为主，实际 %q", s.status)
	}
}

// 开测后的防抖窗口内，点「停止测速」不应把刚开始的测速停掉。
func TestRequestStopIgnoredDuringGuard(t *testing.T) {
	s := &state{run: &runState{running: true}}
	s.stopGuardUntil = time.Now().Add(time.Second)
	s.requestStop()
	if s.run.stopped {
		t.Fatal("防抖窗口内的停止点击应被忽略")
	}
}
