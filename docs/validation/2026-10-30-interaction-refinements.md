# 验证记录：交互优化（2026-10-30）

对应 [ADR-0029](../adr/0029-interaction-refinements.md)：打开目录入口进面板、行尾三点详情弹窗、Shift/Ctrl 多选。

## 自动化验证

- `gofmt -l .`：无输出。
- `go vet ./...`：通过。
- `go test ./... -count=1`：全绿；`internal/gui` 66 → **70 个用例**：
  - `TestShiftAndCtrlClickMultiSelect`：全不选后普通点击记锚点；Shift 点击 A-04 选中 A-01..A-04；Ctrl 点击加入/移出 A-05；Shift 缩小范围不取消范围外勾选。
  - `TestEntryCardOpensDirectory`：卡片「打开目录」按钮触发 `openDirectory` 接缝，目标 = 配置文件所在目录。
  - `TestDetailPopupDismissesOnOutsideClick`：点弹窗外关闭详情并清选中态。
  - `TestDetailPopupKeepsListRowsFilled`（重写）：弹窗不改变列表视口高度，视口内行照常绘制。
  - `TestRowClickAndRightClickMarkActive`（重写）：点行切换勾选并高亮、不开详情；点 ⋮ 开详情；「关闭详情」清选中态；右键开菜单。
  - `TestResultRowTogglesDetailPanel`（重写）：点 ⋮ 开/关弹窗，点行与复选框都是切换勾选。
- headless 走查：`PASS`，`layout` 0 problems。
- 绘制基准（300 节点）：105,838 ns/op，**0 allocs/op**（中途发现三点图标无障碍名在绘制路径拼串导致 20 allocs/op，已改为 `nodeState.knobLabel` 预计算）。

## 实现要点回顾

- Viva 的 click 事件 pass（`deliverClick`）中 `canvas.pointer` 已清空，`PointerOn` 拿不到修饰键与位置——行/图标左键必须在 pointer 事件 pass 里处理 `PointerUp` 并 `Consume`；pointer pass 结束后不运行 AfterPaint 队列，处理里直接改状态。
- 详情弹窗用 `flex.Popup().Below(锚点, 6, flex.End)`：从图标下方展开、放不下自动向上、右缘对齐向左伸。
- dismiss 点击的同一事件 pass 里关闭弹窗后必须 `return`，不能再声明弹窗内容（空指针）。

## 测试包

`clash-speedtest-plus.exe`：`v2.9.0-viva-test`，对应提交 `a0e43d1`，51,383,808 字节，SHA-256 `c59688732b531a4fbe368785bb1c1f7395abb62c1140c45e25e0bdca05751fc9`。

## 未覆盖（待真机确认）

- ⋮ 图标在高 DPI 下的字形渲染与点击热区手感。
- 弹窗在窗口边缘（最右/最底行）的翻转位置。
- Shift 范围跨组（跨条目）选择在真实数据上的可用性。
