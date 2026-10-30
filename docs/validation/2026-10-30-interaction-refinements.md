# 验证记录：交互优化（2026-10-30，含选择/勾选分离修订）

对应 [ADR-0029](../adr/0029-interaction-refinements.md)：打开目录入口进面板、行尾三点详情弹窗（不透明、canvas 直绘）、行选择与勾选分离（Shift/Ctrl 多选）。

## 自动化验证

- `gofmt -l .`：无输出。
- `go vet ./...`：通过。
- `go test ./... -count=1`：全绿；`internal/gui` 66 → **70 个用例**：
  - `TestShiftAndCtrlClickMultiSelect`：普通点击只标记不改勾选；Shift 范围替换语义（含反向范围 A-02..A-05）；Ctrl 加入/移出；全程勾选计数不变。
  - `TestEntryCardOpensDirectory`：卡片「打开目录」按钮触发 `openDirectory` 接缝，目标 = 配置文件所在目录。
  - `TestDetailPopupDismissesOnOutsideClick`：点弹窗外关闭详情并清选中态。
  - `TestDetailPopupKeepsListRowsFilled`（重写）：弹窗不改变列表视口高度，视口内行照常绘制。
  - `TestRowClickAndRightClickMarkActive`（重写）：点行标记行（marked）、不改勾选、不开详情；点 ⋮ 开/关详情；右键开菜单。
  - `TestResultRowTogglesDetailPanel`（重写）：点行与详情都不改勾选，点复选框才切换。
  - `TestContextMenuWordingSingleAndMulti`：多选语境改用 Ctrl+点击制造（行选择），复制/单独测试语义不变。
- headless 走查（`scripts/headless-walkthrough.sh`）：`PASS`，`layout` 0 problems；多选语境改用 `keydown ctrl` + 点击制造，开测前恢复勾选步骤。
- 绘制基准（300 节点）：112,996 ns/op，**0 allocs/op**。
- 弹窗不透明性用 headless 截图验证：修复前 HK-02 行文字透过弹窗；修复后弹窗白底 + 边框 + 阴影完整覆盖列表；Ctrl+点击两行的高亮与勾选计数互不影响也已截图确认。

## 实现要点回顾

- **「透明弹窗」根因**：渲染器按 layer 分组合成，同层先画全部矩形批、再画全部文字批——弹窗背景矩形（layer 0）画不过列表行文字（layer 0）。`flex.Popup` 没有绘制钩子，`PushLayer` 在 build 阶段调用也无效；最终弹窗脱离 flex，在 `a.paint` 的 `view.Draw` 之后用 canvas 手绘，`PushLayer(1)`/`PopLayer` 成对包裹。
- 左键点击在 pointer 事件 pass 处理 `PointerUp` 并 `Consume`（click pass 拿不到修饰键与位置；pointer pass 后不跑 AfterPaint 队列）。
- 行选择（marked）与勾选（selected）分离后，`handleRowClick` 不再调用 `rebuild`（marked 不影响可见集合），勾选相关操作各自 `rebuild`。
- dismiss 点击的同一事件 pass 里关闭弹窗后必须 `return`，不能继续绘制弹窗内容（空指针）。

## 测试包

`clash-speedtest-plus.exe`：`v2.9.1-viva-test`，对应提交 `22810a4`，51,382,272 字节，SHA-256 `c13e0ea3092bc07f664ba8fc9fd6db72b1a42c65d5c4cca52321c2969d31838b`。

## 未覆盖（待真机确认）

- ⋮ 图标在高 DPI 下的字形渲染与点击热区手感。
- 弹窗在窗口边缘（最右/最底行）的翻转位置。
- Shift 范围跨组（跨条目）选择在真实数据上的可用性。
