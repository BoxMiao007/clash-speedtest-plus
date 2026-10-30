# 分组标题折叠与固定验证记录（2026-10-30）

- 环境：Windows 11（amd64）、AMD Ryzen 7 4800H、Go 1.27.3（模块指令 1.26）
- 关联：[ADR-0023](../adr/0023-group-collapse-sticky.md)、[规格 0001](../specs/0001-viva-gui.md)
- 前一轮：[单测固定、折叠窄栏与徽标对齐](2026-10-30-pin-rail-badge.md)。后续：[审查修复](2026-10-30-audit-fixes.md)。

## 1. 范围

按截图标注：条目分组标题（`米贝 · 189 个节点 · 已选 189 · 全选/全不选`）要能折叠该组节点，并固定在列表顶部，滚到下一个订阅条目时自动换成下一组的标题。

## 2. 实现摘要

- **折叠**：组标题左侧新增 `▾` / `▸` 箭头（`Text` + `Hit`，悬停浅底）。点它折叠该条目的节点行，标题追加「已折叠」；状态记 `entryState.collapsed`，只影响展示，勾选、测速与导出范围不变。
- **固定标题**：列表滚过某个组标题后，把该组标题画在列表顶部（32px + 1px 分隔线），并按「标题内容坐标 − 滚动偏移 ≤ 0」自动切换到下一组。
- **被盖住的行裁剪**：固定标题存在时，完全落在 33px 内的行不画；跨过 33px 的行放进从 33px 开始、裁到行底的容器，内容按原偏移绘制。命中区随裁剪收窄，所以固定标题不会盖出可点击的隐藏行，内容也不位移（不跳动）。
- **身份唯一**：固定标题复用该条目自己的 `groupToggle/selectAll/selectNone`；同一组的流内标题跳过绘制。
- **文案预计算**：折叠箭头与「折叠/展开 <条目> 的节点」在 `rebuild`/`toggleGroup` 算好，绘制路径不拼字符串（保持 0 allocs/op）。

## 3. Driver 测试

`go test ./internal/gui -count=1`：46 个用例全部通过（上一轮 44 个 + 本轮 2 个）。

- `TestGroupHeaderCollapsesRows`（新增）：点箭头后节点行消失、标题出现「已折叠」、勾选数不变；再点恢复。
- `TestStickyGroupHeaderTracksScroll`（新增）：两个 30 节点的条目，滚到第一组内时第一组标题固定在列表顶部且只出现一次；滚到底自动换成第二组，固定标题上的「全不选」能取消第二组全部勾选。
- 其余 44 个用例保持通过。

```
gofmt -l .               # 无输出
go vet ./...             # 通过
go test ./... -count=1   # 8 个包全部 ok
```

## 4. headless 走查

`scripts/headless-walkthrough.sh` 在测速后增加：`折叠 机场 A 的节点` → 断言 `已折叠` → `展开 机场 A 的节点`。折叠/展开、固定标题、四个滚动状态下的 `layout` 都是 0 problems。

结果：`PASS：headless 走查通过`；日志见 [headless-walkthrough.log](headless-walkthrough.log)。

## 5. 截图核对（headless `shot`，不随仓库提交）

1. 滚动 300px：顶部出现固定的「`▾ 机场 A 30 个节点 · 已选 30 全选 全不选`」，其下第一行的上半截被裁掉，只露出副标题，没有空白带。
2. 在固定标题上点折叠：机场 A 的行消失，状态栏「已折叠 机场 A」，固定标题自动切到机场 B（因为 A 已折叠、B 的标题移到了视口上方）。
3. 滚回顶部展开：标题恢复 `▾`，行全部回来。
4. 滚到底：固定标题是「机场 B」，A 的标题与行都不再绘制。

## 6. 基准（1180×800，`go test -bench`，2000 次）

| 场景 | 时间 | 内存 | 分配 |
| --- | --- | --- | --- |
| 300 节点 | 76,214 ns/op | 394 B/op | 0 allocs/op |
| 20 节点 | 74,956 ns/op | 389 B/op | 0 allocs/op |
| 空数据 | 16,741 ns/op | 68 B/op | 0 allocs/op |

与上一轮基本持平。中途曾在绘制里拼折叠无障碍文案，基准出现 1 alloc/op；改成 `groupState.arrow/toggleLabel` 预计算后回到 0 allocs/op。

## 7. 未验证项

- 真实窗口下固定标题切换的观感（无推挤动画，切换是瞬时的）与鼠标滚轮跟随。
- 折叠状态不持久化：重启后所有分组展开。
- 跨过顶部的行只显示下半截；若行高或标题高度以后调整，`groupBarHeight` 需要同步。
- 高 DPI 下箭头字号、固定标题 1px 分隔线的清晰度。
- `-race`：本机无 gcc，仍无法运行。

## 8. 文件变更

修改：

- `internal/gui/ids.go`：`entryIDs.groupToggle`。
- `internal/gui/state.go`：`entryState.collapsed`、`groupState.collapsed/arrow/toggleLabel`、`buildNodeItems` 折叠分支、`rebuild` 预计算。
- `internal/gui/actions.go`：`toggleGroup`、`groupToggleText`。
- `internal/gui/view_speedtest.go`：固定标题浮层、被盖住行的裁剪、组标题箭头。
- `internal/gui/interaction_test.go`：折叠与固定标题两个用例。
- `scripts/headless-walkthrough.sh`：折叠/展开与 `已折叠` 断言。
- 文档：`docs/specs/0001-viva-gui.md`、`GLOSSARY.md`、`README.md`、`AGENTS.md`、新增 [ADR-0023](../adr/0023-group-collapse-sticky.md)、本文件、`headless-walkthrough.log`。

## 9. 测试 exe

- 版本：`v2.8.3-viva-test`，对应代码提交 `26a4503`
- 大小：51,157,504 字节（上一版 v2.8.2 为 51,145,728 字节，+12 KB）
- SHA-256：`5f8d7616ccc6864a88c60b946f037ce4d14244744c8218435acbe91f829df1da`
- 形态：PE32+ GUI x86-64

```bash
CGO_ENABLED=0 go build -trimpath \
  -ldflags "-s -w -H=windowsgui -X main.version=v2.8.3-viva-test -X main.commit=26a4503" \
  -o clash-speedtest-plus.exe .
```
