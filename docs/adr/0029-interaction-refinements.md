# ADR-0029：交互优化（打开目录入口、三点详情弹窗、Shift/Ctrl 多选）

- 状态：已接受
- 日期：2026-10-30
- 关联：[ADR-0028](0028-experience-gaps.md)、[ADR-0020](0020-merged-test-view.md)、[ADR-0026](0026-row-active-state.md)

## 背景

ADR-0028 交付后用户提出三项交互改进：打开配置目录的入口要直接出现在「节点来源」面板里；节点详情的打开方式与展示形态要改（行尾竖向三点图标、弹窗从图标处展开而不是底部面板）；节点要支持 Shift/Ctrl 多选。

## 决策

1. **「打开目录」放进节点来源卡片**：条目卡片第二行按钮变为「编辑 / 打开目录 / 删除」。「打开目录」对本地条目打开配置文件所在目录，对订阅条目打开缓存目录（`data/subscriptions/`），与节点右键菜单的「打开配置文件所在目录」共用 `openEntryDirectory(entry)`。右键菜单入口保留。
2. **详情只由行尾三点图标（⋮）打开**：每行末尾（「上次测速」列之后、滚动条留白之前）加 20px 宽的竖向三点图标；点行本身改为**切换勾选**（与复选框等价），不再打开详情。双击的第二下忽略，避免两次切换抵消。图标在选中/固定行用主题色，其余用 Muted。
3. **详情弹窗从图标位置展开**：用 `flex.Popup().Below(锚点, 6, flex.End)` 从图标下方展开 420×252 的卡片（放不下自动改到上方），右缘对齐图标向左伸，不占列表布局。弹窗内是节点名 + 「关闭详情」按钮 + 详情正文（Paragraph 12 行）。关闭途径：再点图标、点弹窗内「关闭详情」、点弹窗外任意处（全窗口 dismiss 命中）、Esc、目标行滚出视口（`detailSeen` 每帧由行绘制刷新，没画到目标行就关）、节点被过滤/删除（既有 `pruneTransientAnchors`）。
4. **Shift/Ctrl 多选**（在行点击上）：
   - **普通点击**：切换该行勾选，并把该行记为范围锚点（`rangeAnchorKey`）。
   - **Ctrl 点击**：切换单个节点（加入或移出），不清其它勾选，同样更新锚点。
   - **Shift 点击**：选中锚点到该行之间（按当前可见顺序，即 `nodeItems`）的全部节点；只增不减——范围外已勾选的保持原样；锚点不更新（连续 Shift 从同一起点扩展）；锚点行不可见时退化为单选该行。被过滤隐藏的节点不参与范围。
   - 行点击仍把该行标成 `activeNode`（操作行高亮语义保留，ADR-0026）。
5. **实现要点——左键在 pointer 事件 pass 处理**：Viva 的 click 事件 pass（`deliverClick`）里 `canvas.pointer` 已被清空，`PointerOn` 拿不到修饰键与位置。行与图标的左键都在 `PointerOn(id)` 的 pointer pass 里处理 `PointerUp`（主键、`ClickCount == 1`），随后 `canvas.Consume()` 阻止重复的 click pass。**pointer pass 结束后不运行 AfterPaint 队列**，所以处理里直接改状态（与右键菜单同一模式），不能走 `AfterPaint`。
6. **dismiss 命中与弹窗同帧**：`paintDetailPopup` 先声明全窗口 dismiss 命中再声明弹窗内容（后声明优先）；dismiss 点击后本帧必须 `return`，不再声明弹窗（`detailNode` 已清空，往下走会解引用空指针）。

## 取舍

- 普通点击从「打开详情」改为「切换勾选」：详情有了专属入口（三点图标）后，行点击空了出来；勾选模型下「点击=单选并清空其它」太危险（误点一次清掉全部勾选），「点击=切换」与复选框一致且可逆。
- 详情弹窗固定尺寸（正文 12 行）不做内容自适应：详情文本结构固定，超长字段在 Paragraph 里换行截断，成本最低。
- Shift 范围「只增不减」：与常见文件管理器的框选习惯一致，误操作可再用 Ctrl 点击撤销单个。
- 锚点在滚动后不跟随（弹窗位置固定），目标行滚出视口即关闭：避免维护「行 y ↔ 弹窗位置」的每帧同步，交互可接受。
- 底部详情面板与 `detailPanelHeight` 移除：首帧视口估算不再扣面板高度，列表可用高度增加 148px。

## 影响

- `internal/gui/ids.go`：`entryIDs.openDir`、`nodeIDs.knob`、`detailDismiss/detailClose`。
- `internal/gui/state.go`：`rangeAnchorKey`、`detailAnchor`、`detailSeen`、`nodeState.knobLabel`（预计算无障碍名）。
- `internal/gui/actions.go`：`openEntryDirectory(entry)`、`handleRowClick`、`selectRangeTo`。
- `internal/gui/view_speedtest.go`：行尾三点图标与 pointer-pass 点击处理、详情弹窗（`paintDetailPopup`）、移除底部面板、卡片「打开目录」。
- `internal/gui/contextmenu.go`：菜单打开目录传条目。
- 测试：gui 66 → **70 个用例**；三个旧行为用例重写（点行开详情 → 点图标开弹窗/点行切换勾选）。
