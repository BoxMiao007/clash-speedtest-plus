# 审查修复验证记录（2026-10-30）

- 环境：Windows 11（amd64）、AMD Ryzen 7 4800H、Go 1.27.3（模块指令 1.26）
- 关联：[ADR-0024](../adr/0024-audit-fixes.md)、[规格 0001](../specs/0001-viva-gui.md)
- 前一轮：[分组标题折叠与固定](2026-10-30-group-sticky.md)

## 1. 范围

对 `internal/gui` 做了一次只读审查（测速视图交互 / 状态与轮次导出两条线），本轮修掉确认的高、中危问题，并补了「点击节点条目缺失选中效果」：

1. 条目卡片只有复选框能点，点卡片没有选中态。
2. 整卡命中后，禁用按钮的点击会穿透成取消勾选。
3. 最小面板宽度下标题、路径徽标被压字，动作按钮溢出。
4. 多选右键「立即测试所选节点」绕过过滤。
5. 表头与行的名字列收缩基准不同，窄窗口下数值列错位。
6. 详情面板不跟结果刷新，超长文案画出卡片。
7. 测速中切轮会污染轮次账本、误导出空配置；手动导出挡住自动导出时队列停住。
8. 停止后行永久「测试中」。
9. 拉取中改订阅地址会写回旧地址的正文；拉取无超时。
10. 菜单缓存的节点指针在后台重读后可能作用到已消失节点；详情/固定行在重读、过滤后不清理。

## 2. 修复摘要

- **卡片选中**：整卡 `Hit` + 点空白切换勾选；选中时主题色 6% 淡底 + 45% 边框；卡片按钮优先，`fetching` 时卡片不接管点击。卡片文案预计算。
- **窄面板**：最小宽度 220 → 260；动作按钮排成两行（读取/重新获取、编辑/删除）；标题 `MinWidth(78)`、徽标 `Shrink(0)`。
- **列对齐**：表头与行的名字列都 `Basis(0)+Grow(1)`。
- **右键测试**：`startNodesTest` 按 `match` 过滤；无命中时提示。
- **详情**：`refreshNodeDisplay` 末尾重算文案（并放在展示字段之后），面板加 `Clip`。
- **轮次**：新增 `runRound()`，完成/提前结束/推进只认 `run.round`；`advanceRound` 从运行轮推进；`switchRound` 只改视图索引。
- **导出**：手动导出结束时消费 `pendingExport`（队列仍在时补启动）；停止时把 `phaseTesting` 一并复位。
- **订阅**：`fetchSeq/fetchURL` 代次，地址改动可开新请求、旧结果丢弃；拉取客户端 60s 超时；关窗后 `enqueue` 丢弃。
- **锚点**：排序后校验详情/固定行仍在可见分组，不在就清空；菜单激活时按 key 重新解析节点。
- **其它**：空状态读取按钮改用独立身份；组标题折叠走 `AfterPaint`；滚动区域有效标记每帧清零；列表虚拟化用窗口高度作绘制下限（拉高窗口不留白）。

## 3. Driver 测试

`go test ./internal/gui -count=1`：51 个用例全部通过（上一轮 46 + 本轮 5）。

新增：

- `TestEntryCardClickTogglesSelection`：点卡片切换条目勾选，节点行随勾选出现/消失。
- `TestContextMenuTestRespectsFilter`：过滤生效时右键测试只测命中节点。
- `TestDetailPanelRefreshesAfterRun`：详情跟随结果更新（含再次刷新）。
- `TestHiddenNodesClearDetailAndPin`：节点被过滤后详情与固定行清空。
- `TestRoundSwitchDuringRunKeepsRunningRound`：运行中切轮，运行轮仍完成、被翻到的轮不被误标；用 `gateRunner` 卡住第一轮制造时序。

```
gofmt -l .               # 无输出
go vet ./...             # 通过
go test ./... -count=1   # 8 个包全部 ok
```

## 4. headless 走查

`PASS：headless 走查通过`；`layout` 0 problems（含折叠/展开、拖动分隔条到最小宽度、设置页）。日志见 [headless-walkthrough.log](headless-walkthrough.log)。

截图核对（不随仓库提交）：

1. 默认窗口：两张条目卡片都显示蓝色选中底与边框；动作按钮两行。
2. 拖到最小 260px：标题「节点来源」完整、`软路径` 徽标不压字、按钮两行不溢出。
3. 点卡片后：该条目取消勾选、卡片恢复白底，右侧分组消失；再点恢复。
4. 详情：打开后列表填满，文案在面板内。

## 5. 基准（1180×800，`go test -bench`，2000 次）

| 场景 | 时间 | 内存 | 分配 |
| --- | --- | --- | --- |
| 300 节点 | 约 102,000 ns/op | 458 B/op | 0 allocs/op |
| 20 节点 | 约 100,000 ns/op | 453 B/op | 0 allocs/op |
| 空数据 | 16,877 ns/op | 68 B/op | 0 allocs/op |

比上一轮（76,214 ns/op）慢约 35%：列表虚拟化改用窗口高度作绘制下限，每帧多画几行（被 Clip 裁掉），换来「拉高窗口不留白」。仍为 0 allocs/op，空闲不绘制。

## 6. 已知未修（记录在案）

- **导出命名**：`exportSeq` 只在开测时赋值，手动导出与同轮自动产物可能同名覆盖（低概率、需写盘）。
- **导出失败**：失败分支清掉 `pendingExport`，另一轮已测完的导出被放弃；是否重试需产品口径。
- **只读数据目录**：探针失败时连读取都禁用，既有条目/设置不可见，与 spec「照常打开、仅禁用持久化」有差距。
- **滚动条右缘**：列表行最右 10px 属于滚动条轨道，右键在该区域不出菜单（落在行右 padding 内）。
- **双击行**会开→关详情；**空态复用**已修，但详情在列表为空时仍隐藏（清空过滤后回来）。
- **固定行/详情行**底色接近（14% / 10%），同时存在时不好区分。
- 真实窗口下的卡片蓝色浓度、高 DPI、`-race` 仍未验证。

## 7. 文件变更

修改：

- `internal/gui/view_speedtest.go`：卡片命中/选中态、两行按钮、标题与徽标宽度、名字列 Basis、详情刷新与 Clip、组标题 AfterPaint、绘制下限。
- `internal/gui/actions.go`：`runRound()` 口径、`advanceRound` 基准、`pendingExport` 消费、`startNodesTest` 过滤、停止相位复位、`enqueue` 关闭守卫、`scrollToRound` 偏移保持。
- `internal/gui/state.go`：`fetchSeq/fetchURL`、`nodeByKey`、`pruneTransientAnchors`、`refreshNodeDisplay` 顺序、滚动标记。
- `internal/gui/ids.go`：`entryIDs.card/emptyRead`。
- `internal/gui/contextmenu.go`：菜单目标按 key 重新解析。
- `internal/gui/app.go`：每帧清空滚动区域有效标记。
- `internal/speedtester/speedtester.go`：拉取客户端 60s 超时。
- `internal/gui/interaction_test.go`、`app_test.go`、`parity_test.go`：5 个新用例与 `gateRunner`。
- 文档：`docs/specs/0001-viva-gui.md`、`GLOSSARY.md`、`README.md`、`AGENTS.md`、新增 [ADR-0024](../adr/0024-audit-fixes.md)、本文件、`headless-walkthrough.log`。

## 8. 收尾修复（同日）

上一节「已知未修」中可修的部分已处理：

- **只读数据目录**：新增 `store.OpenReadOnly`，GUI 在可写探针失败时只读打开，已有条目/设置照常显示；写操作跳过并提示「未保存」；测速结果不再逐条报保存失败；订阅拉取成功但只读时照常解析、提示未写缓存。`TestDataDirUnavailableShowsAlertAndDoesNotFallBack` 改为断言「数据目录不可写 / 只读」；新增 store 用例 `TestOpenReadOnlyLoadsDataAndRejectsWrites`。
- **手动导出序号**：手动导出用 `output.NextImageSequence(目标目录)` 现取序号，不再复用 `exportSeq`，与同轮自动产物不会同名覆盖。
- **导出失败**：停止推进的规格不变，但不再清空 `pendingExport`，用户手动导出成功后会补写；起测时清空，避免污染新队列。
- **行右缘**：行与表头右侧各留 10px，滚动条轨道不再抢行点击；新增 `TestRowHitsStopBeforeScrollBar`。
- **双击**：`ClickCount > 1` 时不再切换详情。
- **行底色**：详情行改为 `MutedForeground` 14% 灰底，和主题色固定行区分。

测试：`internal/gui` 52 个 Driver 用例 + `internal/store` 只读用例，全部通过；走查 `PASS`、`layout` 0 problems。

仍未验证：真实窗口双击手感、只读目录在真实权限下的表现、手动导出覆盖场景的端到端文件检查。

## 9. 节点行选中态与卡片回退（同日）

- **卡片回退默认**：去掉整卡主题色底/边框，恢复白底 + 普通边框；是否选中看复选框。整卡点击切换勾选保留。
- **节点行选中态**：新增 `activeNode`，单击打开详情、右键打开菜单时标记该行；行用主题色 14% 淡底 + 主题色节点名（固定单测行 20%，仍更强）。关闭详情清除，点另一行切换，节点被过滤/重读时由锚点清理一起清掉。
- 新增 `TestRowClickAndRightClickMarkActive`（gui 53 个用例）；走查 `PASS`、`layout` 0 problems；截图核对了卡片白底、单击行蓝底 + 主题色名称、右键行蓝底 + 菜单。
- 未验证：真实窗口下 14% 底色的观感、右键菜单关闭后高亮保留是否符合预期。

## 10. 悬停不覆盖选中态（同日）

试用反馈「鼠标指向为什么会消失」：行的 `HoverBackground` 固定用 `Muted`，鼠标一指向选中行，蓝色底就被灰底替换，看起来像选中效果消失。

- 固定行：底色 Primary 20%，悬停加深到 28%；
- 当前操作行：14% → 22%；
- 已勾选/测试中的灰底行：悬停用 10% 加深灰；
- 默认行：悬停仍是 `Muted`。

headless 截图（点击 A-01 后把鼠标移入/移出该行）确认选中行的蓝底持续存在；`layout` 0 problems。

## 11. 测试 exe

- 版本：`v2.8.7-viva-test`，对应代码提交 `HEADFIX`
- 大小：51,172,864 字节（与上一版基本持平）
- SHA-256：`9bb87e6d2a0b85d8a66add0e8f607932396f7005e998b909ce9898668160f124`
- 形态：PE32+ GUI x86-64

```bash
CGO_ENABLED=0 go build -trimpath \
  -ldflags "-s -w -H=windowsgui -X main.version=v2.8.7-viva-test -X main.commit=HEADFIX" \
  -o clash-speedtest-plus.exe .
```
