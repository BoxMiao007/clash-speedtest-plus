# 交互补全验证记录（折叠调宽 / 滚动条 / 自解释文案）

- 日期：2026-10-30
- 代码基线：`af0032a`（本记录对应的工作区改动尚未提交）
- 环境：Windows amd64、Go 1.27.3、AMD Ryzen 7 4800H
- 关联：[ADR-0019](../adr/0019-interaction-polish.md)、[规格 0001](../specs/0001-viva-gui.md)

## 1. 范围

本轮只处理四个已确认的交互问题，不改测速、导出与数据结构：

1. 「节点来源」可折叠、可拖动调宽。
2. 节点来源、节点列表、结果列表、设置表单在内容溢出时显示可拖动的滚动条。
3. 过滤「与 / 或」改成可理解的「全部命中 / 任一命中」并加说明。
4. 设置「输出」三个勾选框各自带标签与说明。

## 2. 实现摘要

- 折叠：标题栏 `折叠` 收成 56px 窄栏（`展开` + 「来源」），展开恢复；折叠时不声明 Splitter，避免宽度被边界钳制。
- 调宽：`flex.Splitter` 放在「节点来源」与内容之间，`SplitterState.Pos` 即面板宽度，`Mins(220, 420)`；`Config.OnKey` 转交方向键，聚焦后每次 16px。宽度与折叠状态只在运行内保留。
- 滚动条：新增 `internal/gui/scrollbar.go`。两个 `flex.ScrollArea`（节点来源、设置）读 `view.ScrollState(id)` 的 `Offset/Content/Viewport`；节点列表、结果列表用自己的偏移与内容高度。四条都用 `components.BeginScrollArea` 的几何与绘制，`PointerOn` 处理按下/拖动/抬起，`VerticalOffsetForThumb` 折算偏移。滚动条在整页绘制后补画，对话框与右键菜单打开时不画。
- 文案：`combineLabels` 改为 `全部命中` / `任一命中`，右侧常驻说明随选择切换；三个输出开关改为逐行「勾选框 + 标签 + 说明」，勾选框命中区向右扩到标签末尾，点标签也能切换。

## 3. Driver 测试

`go test ./internal/gui -count=1`：40 个用例全部通过（原 33 个 + 本轮 7 个）。新增：

| 用例 | 覆盖点 |
| --- | --- |
| `TestEntriesPanelCollapsesAndResizesByDrag` | 折叠后标题消失、出现「展开」；展开恢复；按住分隔条拖 +120px 后面板变宽、条目区矩形随之变宽 |
| `TestScrollbarsFollowOverflow` | 条目、节点列表、设置、结果四处内容溢出时滚动条命中存在 |
| `TestScrollbarsHideWhenContentFits` | 内容不溢出时不画滚动条 |
| `TestScrollBarDragScrollsNodeList` | 按轨道跳到指针位置、继续下拖、拖到底看到末尾节点 |
| `TestScrollAreaBarDragScrollsSettings` | 设置滚动条拖动改变 `ScrollState.Offset` |
| `TestFilterCombineLabelsExplainMeaning` | 「全部命中 / 任一命中」与两句说明；点「任一命中」后 `filterCombine` 为 `or` |
| `TestOutputSwitchesExplainEachProduct` | 三个开关标签与说明可见；点标签切换 `imageEnabled` |

```
gofmt -l .                         # 无输出
go vet ./...                       # 通过
go test ./... -count=1             # 8 个包全部 ok
```

## 4. headless 走查

`scripts/headless-walkthrough.sh`（`-tags vivahead` + `VIVA_HEADLESS=1`，不开窗口）新增：

- `click "折叠"` → `texts` → `click "展开"`：断言窄栏出现「展开」并恢复。
- `move 324 300` → `down` → `move 424 300` → `up`：断言 `entry-splitter x=420`，即面板宽度由 320 变 420。
- 设置视图 `scroll` 后 `hits`：断言 `settings-scrollbar` 存在。

结果：`PASS：headless 走查通过`，`layout` 在 1180×800 与最小窗口下均无新的溢出/遮挡；日志见 [headless-walkthrough.log](headless-walkthrough.log)。

## 5. 截图核对（headless `shot`，不随仓库提交）

1. 节点视图（6 条目 / 40 节点）：节点来源与节点列表右侧滑块清晰可见；滑块落在 20px 内边距里，不压「未测试」等右对齐文本。
2. 设置视图：右侧滑块可见；拖动/滚轮后内容位置一致。
3. 折叠视图：面板缩为 56px 窄栏，节点列表占满剩余宽度。
4. 输出设置：三个开关各占一行，标签与说明均可读；修复了首版勾选框只有方块、标签未画出的问题。

## 6. 基准（1180×800，`go test -bench`，2000 次）

| 场景 | 时间 | 内存 | 分配 |
| --- | --- | --- | --- |
| 300 节点 | 48,028 ns/op | 283 B/op | 0 allocs/op |
| 20 节点 | 47,433 ns/op | 281 B/op | 0 allocs/op |
| 空数据 | 14,125 ns/op | 64 B/op | 0 allocs/op |

与上一轮（47,335 / 45,922 / 13,308 ns/op）相比增加约 1–6%，来自四条滚动条的几何计算与过滤条多出的一行说明；分配次数不变。滚动条没有为每帧新建闭包或拼字符串。

## 7. 未验证项

- 真实窗口下的鼠标拖动手感、光标形状（分隔条 resize 光标、滚动条悬停变深）与高 DPI 下的 8px 分隔条命中范围。
- 折叠状态与面板宽度不持久化的产品感受（重启回到 320px 展开）。
- 高对比度主题下 `MutedForeground × 0.45` 滑块的可见度。
- `-race`：本机无 gcc，仍无法运行。

## 8. 文件变更

新增：

- `internal/gui/scrollbar.go`：四条滚动条的绘制、命中与拖动。
- `internal/gui/interaction_test.go`：本轮 7 个 Driver 用例与 `drag` 辅助。
- `docs/adr/0019-interaction-polish.md`：本轮决策记录。
- `docs/validation/2026-10-30-interaction.md`：本文件。

修改：

- `internal/gui/app.go`：Splitter/折叠布局、`OnKey`、滚动条收尾、过滤说明文案、组合方式标签。
- `internal/gui/state.go`：折叠/宽度/滚动条状态与区域矩形。
- `internal/gui/view_nodes.go`：面板宽度常量、折叠窄栏、条目滚动区矩形。
- `internal/gui/view_settings.go`：设置滚动区矩形、输出开关逐行标签与说明、`settingsToggleRow`。
- `internal/gui/ids.go`：折叠/展开/分隔条/四条滚动条身份。
- `scripts/headless-walkthrough.sh`：折叠、拖动分隔条、设置滚动条断言。
- `docs/specs/0001-viva-gui.md`、`GLOSSARY.md`、`README.md`、`AGENTS.md`：交互、术语与约定同步。
- `docs/validation/headless-walkthrough.log`：本轮走查日志。
