# ADR-0019：界面交互补全（折叠调宽、滚动条、自解释文案）

- 状态：已接受
- 日期：2026-10-30
- 关联：[ADR-0017](0017-viva-gui.md)（GUI 架构）、[ADR-0018](0018-tui-parity.md)（功能补齐）、[规格 0001](../specs/0001-viva-gui.md)

## 背景

功能补齐后试用反馈集中在四件事：

1. 「节点来源」区域不能折叠，也不能拖动改大小，条目多时挤占节点列表。
2. 界面各区域都没有滚动条；内容超出视口只能靠滚轮，用户不知道下面还有内容，也没法用指针拖。
3. 过滤条上的「与 / 或」看不出控制什么。
4. 设置「输出」一节三个勾选框只写「导出结果图」「结果图只保留有速度的行」「按 IP 归属地与速度重命名」，用户不知道各自影响哪种产物。

Viva 的能力边界很明确：`flex.ScrollArea` 只做裁剪与偏移、不自绘滚动条（`flex/frame.go` 注释写明滚动条归应用）；`flex.Splitter` 是现成可拖分隔条（8px、指针捕获、resize 光标、方向键、最小尺寸）；`components.ScrollArea` 提供滚动条几何、绘制与滑块命中；`components.Collapsible` 只有 canvas 级，没有 flex 版折叠容器。

## 决策

1. **折叠 + 拖动调宽用 `flex.Splitter`**。第一栏宽度直接给 `SplitterState.Pos`，声明 `f.Splitter(id, &state).Mins(220, 420)` 后由框架处理按下捕获、拖动、光标与边界；`Config.OnKey` 只把按键转给 `SplitterState.Key`，聚焦后方向键每次 16px。折叠时不声明 Splitter，改画 56px 窄栏（`展开` 按钮 + 「来源」字样），展开恢复原宽度。
2. **折叠状态与宽度不落盘**。它们是纯界面状态，写进 `settings.json` 要么升数据版本、要么加「仅界面」字段；为两个可随时重设的值扩大持久化面不划算。规格的 Out of Scope 明确写出这一点。
3. **滚动条归应用侧，统一在 `internal/gui/scrollbar.go`**。`flex.ScrollArea`（节点来源、设置）用 `view.ScrollState(id)` 取 `Offset/Content/Viewport`；节点列表与结果列表用各自的偏移与内容高度。四条滚动条共用同一套：`components.BeginScrollArea(...).Offset(...).VerticalScrollbar(id, label, "")` 画轨道/滑块并注册命中，`PointerOn` 处理按下、拖动与抬起，`VerticalOffsetForThumb` 把指针位置折算成偏移；拖动结果写回列表偏移或 `ScrollState.ScrollTo`。
4. **滚动条在整页绘制之后补画**。先让 flex 完成内容裁剪与排版，再读 `Box.Rect()` 与 `ScrollState`，画在最上层；对话框或右键菜单打开时不画，避免盖住浮层。滑块用 `MutedForeground` 降低透明度（仍是主题令牌），因为 `Border` 在白底上几乎不可见。行内容本来就留了 20px 右侧内边距，10px 滚动条落在内边距里，不压文字。
5. **过滤组合改为「全部命中 / 任一命中」+ 就地说明**。分段控件右侧固定一行 10px 说明：选中「全部命中」显示「每个关键词都要出现」，「任一命中」显示「任一关键词出现即可」。没有做悬停 tooltip：说明需要常驻可读，tooltip 还要额外悬停动作，且 `flex` 侧没有现成接缝。存储值仍是 `and`/`or`，预设与 `settings.json` 不受影响。
6. **输出三个开关改为「标签 + 说明」逐行排列**。勾选框只画方块，标签是旁边的 `Text`；把勾选框的命中区向右扩到标签末尾（`HitOutset`，宽度用 `Canvas.TextWidth` 量），点标签也能切换。三个说明分别点明「只影响结果图 / 不影响配置」「关闭后仍可手动保存」「会逐条查询 IP，节点多时慢」。

## 取舍

- 「与 / 或 → 全部命中 / 任一命中」少了布尔代数味道，但配合一行说明，用户不需要知道 `and`/`or` 就能选对。过滤条因此多占约 90px，1180 宽下布局仍无溢出。
- 滚动条只做纵向：`flex` 的 `ScrollState` 只有单个 `Offset`，横向滚动要自己实现整套偏移，超出本次需求；单行过长仍按既有规则省略。
- 滚动条覆盖在内容上而不是占布局宽度：省去重新排版，代价是右侧内容必须留出 10px 内边距（现有 20px 内边距已满足）。
- 折叠状态不持久化，重启回到展开、默认 320px。用户若需要长期偏好，再按「仅界面字段」加设置项。
- 分隔条冷启动不显示 3px 高亮，只在悬停/拖动时加粗；这是 `flex.Splitter` 的既有外观，没有为它另画装饰。

## 影响

- 新增 `internal/gui/scrollbar.go`；`app.go`、`state.go`、`view_nodes.go`、`view_settings.go`、`ids.go` 有对应改动。
- 新增 7 个 `app.Driver` 用例（折叠/展开、拖动调宽、四处滚动条出现与隐藏、节点列表拖动、设置拖动、过滤文案、输出开关文案），`scripts/headless-walkthrough.sh` 增加折叠/展开、拖动分隔条与设置滚动条的断言。
- 绘制仍为 0 allocs/op（`48028 ns/op, 283 B/op`，300 节点，1180×800），绘制路径没有为滚动条新建闭包或字符串。
- 规格、GLOSSARY、README、AGENTS 同步；验证记录见 `docs/validation/2026-10-30-interaction.md`。
