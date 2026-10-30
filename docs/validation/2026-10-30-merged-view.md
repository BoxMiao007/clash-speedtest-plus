# 合并节点选择与测速结果视图验证记录（2026-10-30）

- 代码基线：`9701492`（功能）/ `0ce7403`（测试）
- 环境：Windows 11（amd64）、AMD Ryzen 7 4800H、Go 1.27.3（模块指令 1.26）
- 关联：[ADR-0020](../adr/0020-merged-test-view.md)、[规格 0001](../specs/0001-viva-gui.md)
- 前一轮：[交互补全](2026-10-30-interaction.md)

## 1. 范围

把「节点选择」与「测速结果」两个视图合并成单一的「测速」视图：同一张表完成勾选、开测、看结果、排序、详情与手动导出；窗口只剩 `测速` / `设置` 两个视图。不改测速引擎、导出流程与数据格式。

## 2. 实现摘要

- **一张表**：按条目分组的节点表，列为节点（副标题 `server:port · 类型`）、状态、延迟、抖动、丢包、下载速度、上传速度（完整模式）、上次测速；行首复选框勾选，点行打开详情。
- **工具条两行**：第一行是选中数、`排除上次失败`、`读取节点`、`下载速度`（运行中换成暂停/继续与停止）；第二行是结果条（进度文案、`上一轮`/`下一轮`、三个导出按钮），只在跑过测速、有结果或导出中出现。
- **排序在组内**：表头点击仍可切列与反转，但只重排每个条目分组内部；`nodeState.order` 记录文件原始序号，主键相同时保持读取顺序，未测速时不会变成身份哈希顺序。
- **轮次改为滚动定位**：列表始终显示全部节点；自动推进与手动切换都把该轮条目分组滚到顶部，进度与手动导出跟随该轮。
- **删除残留**：独立结果视图、`返回选择` 按钮、结果列表滚动条与 `view_results.go`/`view_nodes.go` 移除，合并进 `view_speedtest.go`。

过程中修掉三个问题：

1. 表头 `节点` 与数据列错位 120px：`f.Spacer()` 默认 `grow=1`，固定宽度的占位 Spacer 仍在吃剩余空间，把表头文字推走；改为 `Spacer().Width(colCheck).Grow(0)`，并将占位宽设为复选框实际宽度 40px。
2. 行距 38px 但行高 40px，相邻行命中区重叠 2px，`layout` 报 `collide`；行高与步进统一为 40px。
3. 合并后表头进入每帧绘制路径，`"按"+label+"排序"` 每次分配一个字符串，绘制从 0 变成 5 allocs/op；改为在 `refreshSortHeaders` 里预计算 `sortHit*` 字段后恢复 0 allocs/op。

## 3. Driver 测试

`go test ./internal/gui -count=1`：41 个用例全部通过（上一轮 40 个 + 本轮 1 个）。

- `TestStartTestSortsBySpeedAndKeepsRowIDs`：开测后 `active` 仍是 `viewTest`，结果按速度排序，行命中不变。
- `TestResultRowTogglesDetailPanel`（扩展）：点行只开详情、不改勾选；点复选框切换勾选且不打开详情。
- `TestRoundSwitchScrollsToEntryGroup`（新增）：两条目各 30 节点，测完停在最后一轮并滚到第二组；`上一轮` 回到顶部、`下一轮` 再滚回第二组。
- `TestMultipleEntriesRunSequentialRounds`、`TestScrollbarsFollowOverflow` 等原有用例改用 `测速` 标签，并在合并视图里断言结果列与滚动条。
- 其他 36 个用例（条目、订阅、过滤、预设、导出、上传、暂停、提前结束、空闲、数据目录）保持通过。

```
gofmt -l .               # 无输出
go vet ./...             # 通过
go test ./... -count=1   # 8 个包全部 ok
```

## 4. headless 走查

`scripts/headless-walkthrough.sh`（`-tags vivahead` + `VIVA_HEADLESS=1`）改为点击 `测速` 标签、移除 `返回选择`；走查在合并视图里完成设置保存、测速、导出、过滤、预设、新建条目、折叠/展开、拖动分隔条与设置滚动条。

结果：`PASS：headless 走查通过`；`layout` 在 1180×800 与最小窗口下均为 `0 problems`（列对齐、行命中与表头文字都在控件内）。日志见 [headless-walkthrough.log](headless-walkthrough.log)。

## 5. 截图核对（headless `shot`，不随仓库提交）

1. 首屏：`测速` / `设置` 两个标签；左侧条目面板 + 右侧带表头的节点表，行首复选框，状态为「未测试」。
2. 测速后：不切视图；结果条出现 `第 1/1 轮 · 已完成 3/3 · 本轮完成`，行内显示失败、丢包 100%、上次测速；表头与数据列右边缘对齐。
3. 详情：点行后底部详情面板显示类型、服务器、条目、延迟/抖动/丢包、下载/上传速度与上次测速，右上角「关闭详情」。

## 6. 基准（1180×800，`go test -bench`，2000 次）

| 场景 | 时间 | 内存 | 分配 |
| --- | --- | --- | --- |
| 300 节点 | 65,805 ns/op | 269 B/op | 0 allocs/op |
| 20 节点 | 65,277 ns/op | 267 B/op | 0 allocs/op |
| 空数据 | 16,371 ns/op | 68 B/op | 0 allocs/op |

与合并前（48,028 / 47,433 / 14,125 ns/op）相比每次重绘慢约 37%：可见行现在多画 4–5 个结果列，表头也常驻。基准只画视口内的行；应用空闲仍然完全不绘制，只有交互与测速事件触发重绘。

## 7. 未验证项

- 真实窗口下点行与点复选框的手感、双击行是否误触、拖动滚动条时的重排稳定性。
- 高 DPI 下 7–8 列的列宽与省略号位置。
- 窗口接近 1000px 时节点名列压缩成省略号的观感（既有行为，见规格 Out of Scope）。
- 多轮长时间运行（数百节点）时按事件重排的开销：只在每批 pending 事件后重排一次，未做真机压力测试。
- `-race`：本机无 gcc，仍无法运行。

## 8. 文件变更

新增：

- `internal/gui/view_speedtest.go`：合并视图（条目面板、工具条、结果表、详情、空态）。
- `docs/adr/0020-merged-test-view.md`：本轮决策记录。
- `docs/validation/2026-10-30-merged-view.md`：本文件。

删除：

- `internal/gui/view_nodes.go`、`internal/gui/view_results.go`：并入 `view_speedtest.go`。

修改：

- `internal/gui/app.go`：两视图标签、`paintTest`、滚动路由、去掉结果视图相关状态。
- `internal/gui/state.go`：`viewTest`/`viewSettings`、节点 `order`、分组内排序、结果集合与结果条文案。
- `internal/gui/ids.go`：视图索引、移除 `backToSelect`/`resultBar`。
- `internal/gui/actions.go`：开测不切视图、轮次滚动定位、每批事件后重排。
- `internal/gui/scrollbar.go`：去掉结果列表滚动条，保留三条。
- `internal/gui/view_settings.go`：结果图开关说明改为「测速页」。
- `internal/gui/app_test.go`、`parity_test.go`、`interaction_test.go`：适配两视图并补契约。
- `scripts/headless-walkthrough.sh`：`测速` 标签、去掉 `返回选择`。
- `docs/specs/0001-viva-gui.md`、`GLOSSARY.md`、`README.md`、`AGENTS.md`、ADR-0007/0017/0018：同步。
- `docs/validation/headless-walkthrough.log`：本轮日志。

## 9. 测试 exe

- 版本：`v2.8.0-viva-test`，对应代码提交 `9701492`
- 大小：51,139,072 字节（上一版 v2.7.1 为 51,151,360 字节，-12 KB：删掉两个视图文件与结果列表代码）
- SHA-256：`e98f310b0d41ba4d6403156075ef16466e21109e27dcb444c2b01a4e8ea94262`
- 形态：PE32+ GUI x86-64

```bash
CGO_ENABLED=0 go build -trimpath \
  -ldflags "-s -w -H=windowsgui -X main.version=v2.8.0-viva-test -X main.commit=9701492" \
  -o clash-speedtest-plus.exe .
```
