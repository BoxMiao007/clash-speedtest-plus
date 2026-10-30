# ADR-0029：交互优化（打开目录入口、三点详情弹窗、Shift/Ctrl 多选）

- 状态：已接受
- 日期：2026-10-30
- 关联：[ADR-0028](0028-experience-gaps.md)、[ADR-0020](0020-merged-test-view.md)、[ADR-0026](0026-row-active-state.md)

## 背景

ADR-0028 交付后用户提出三项交互改进：打开配置目录的入口要直接出现在「节点来源」面板里；节点详情的打开方式与展示形态要改（行尾竖向三点图标、弹窗从图标处展开而不是底部面板）；节点要支持 Shift/Ctrl 多选。

## 决策

1. **「打开目录」放进节点来源卡片**：条目卡片第二行按钮变为「编辑 / 打开目录 / 删除」。「打开目录」对本地条目打开配置文件所在目录，对订阅条目打开缓存目录（`data/subscriptions/`），与节点右键菜单的「打开配置文件所在目录」共用 `openEntryDirectory(entry)`。右键菜单入口保留。
2. **行选择（marked）与测速勾选（selected）完全分离**：复选框勾选决定测速范围，点行**不改勾选**。点行维护独立的行选择（`nodeState.marked`）：
   - **普通点击**：只选中该行（清除其它行的选择）并记为范围锚点（`rangeAnchorKey`）。
   - **Ctrl 点击**：把该行加入/移出选择，同样更新锚点。
   - **Shift 点击**：按文件管理器的替换语义，选中锚点到该行之间（按当前可见顺序）的全部节点、清除范围外；锚点不更新；反向范围（锚点在目标之后）同样成立；锚点行不可见时退化为只选目标行。
   - 行选择用主题色高亮（与操作行 `activeNode` 同色系），节点名同步高亮；勾选行保持原来的灰色底。行选择仍会把该行标成 `activeNode`（ADR-0026）。
   - 右键菜单的操作目标改为行选择：marked 多行时菜单走「所选节点」措辞；菜单里的「选中所选节点 / 取消选中所选节点 / 排除所选节点」作用于行选择对应的**勾选**（把选中的行加入/移出测速范围）；「立即测试所选节点」测行选择；「单独测试此节点」测右键指向的行。
3. **详情只由行尾三点图标（⋮）打开**：每行末尾（「上次测速」列之后、滚动条留白之前）加 20px 宽的竖向三点图标；双击的第二下忽略。图标在选中/固定行用主题色，其余用 Muted。
4. **详情弹窗从图标位置展开、完全不透明**：弹窗在 **paint 阶段用 canvas 直接绘制**（`a.paint` 中 `view.Draw` 之后，`paintDetailOverlay`），`PushLayer(1)` 内画不透明白底（`theme.Background`）+ 边框 + 阴影，右缘对齐图标向左伸、下方放不下自动改到上方，整体钳制在窗口内。正文用 `canvas.WrapLines` 换行（最多 14 行）。关闭途径：再点图标、点弹窗外任意处（全窗口 dismiss 命中，layer 0）、Esc、目标行滚出视口（`detailSeen` 每帧由行绘制刷新，没画到目标行就关）、节点被过滤/删除（既有 `pruneTransientAnchors`）。
5. **实现要点——弹窗为什么必须推 layer**：渲染器按 layer 分组合成，**同一层先画全部矩形批、再画全部文字批**，矩形与文字的 append 顺序跨类别不保留。弹窗若与列表同层（layer 0），它的背景矩形画不过列表行的文字——这正是用户看到的「透明弹窗」。`canvas.PushLayer` 只影响其后的 primitive，且必须在 paint 阶段成对调用（build 阶段调用无效）；flex 的 Popup 没有绘制钩子，所以弹窗脱离 flex、在 paint 阶段手绘。
6. **实现要点——左键在 pointer 事件 pass 处理**：Viva 的 click 事件 pass（`deliverClick`）里 `canvas.pointer` 已被清空，`PointerOn` 拿不到修饰键与位置。行与图标的左键都在 `PointerOn(id)` 的 pointer pass 里处理 `PointerUp`（主键、`ClickCount == 1`），随后 `canvas.Consume()` 阻止重复的 click pass。**pointer pass 结束后不运行 AfterPaint 队列**，所以处理里直接改状态（与右键菜单同一模式），不能走 `AfterPaint`。
7. **dismiss 命中与关闭顺序**：dismiss 点击的同一事件 pass 里关闭弹窗后，本帧后续绘制必须跳过弹窗（`detailNode` 已清空，继续绘制会解引用空指针）。

## 取舍

- 行选择与勾选彻底分离（用户澄清后的最终语义）：勾选是「测哪些节点」的持久意图，行选择是「我正在操作哪些行」的瞬时意图，两者高亮与入口都不同——混用会让点行悄悄改变测速范围。
- Shift 范围用文件管理器的替换语义（范围外清除）：可预测、可重来；误清一次选择再用 Ctrl 逐个加回即可。
- 菜单的「选中/取消选中/排除」仍作用于勾选（测速范围），只是目标集合换成行选择：菜单文案与既有约定一致，不需要新词汇。
- 详情弹窗脱离 flex 手绘：代价是弹窗内不再有 flex 控件（「关闭详情」按钮移除，关闭靠点外部/Esc/再点图标）；换来的 是完全可控的不透明层与锚定位置。
- 锚点在滚动后不跟随（弹窗位置固定），目标行滚出视口即关闭：避免维护「行 y ↔ 弹窗位置」的每帧同步，交互可接受。
- 底部详情面板与 `detailPanelHeight` 移除：首帧视口估算不再扣面板高度，列表可用高度增加 148px。

## 影响

- `internal/gui/ids.go`：`entryIDs.openDir`、`nodeIDs.knob`、`detailDismiss`、`detailClose`（复用为弹窗面板命中）。
- `internal/gui/state.go`：`rangeAnchorKey`、`detailAnchor`、`detailSeen`、`nodeState.marked`、`nodeState.knobLabel`（预计算无障碍名）。
- `internal/gui/actions.go`：`openEntryDirectory(entry)`、`handleRowClick`、`clearMarked`、`markedCount`、`markedNodes`、`selectRangeTo`。
- `internal/gui/view_speedtest.go`：行尾三点图标与 pointer-pass 点击处理、详情弹窗（flex 阶段 dismiss + paint 阶段 `paintDetailOverlay`）、移除底部面板、卡片「打开目录」。
- `internal/gui/contextmenu.go`：菜单操作目标改为行选择。
- `scripts/headless-walkthrough.sh`：多选语境改用 `keydown ctrl` + 点击制造。
- 测试：gui 66 → **70 个用例**；点行开详情/点行切换勾选的旧用例按「选择与勾选分离」重写。
