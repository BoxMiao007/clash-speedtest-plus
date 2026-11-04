# 跨切面静态审查：应用外壳 / 引擎接缝 / 规格完整性 / 无障碍

审查范围：'internal/gui/app.go'、'engine.go'、'ids.go'、'state.go'、'actions.go'、'shortcuts.go'、'window.go'、'preview.go'、'dialogs.go'、'view_speedtest.go'、'view_settings.go'、'export.go'、'contextmenu.go'，对照 'docs/specs/0001-viva-gui.md' 第 8 节与第 12 节验收清单，参考 ADR-0017 / 0018 / 0028。只做只读静态分析，未运行构建/测试。

## 一、合理的交互点（正面清单）

- 双视图切换固定在顶部分段控件，默认「测速」；过滤条在 'paintPage' 中无条件先于内容绘制，两个视图都能看到并修改同一份过滤状态（app.go:172-176、214-290；规格 8.1）。过滤范围同时作用于显示、测速与导出（state.go:233-243、startSelectedTest），与规格 8.3 一致。
- 空闲不绘制有代码保障：'internal/gui' 内无 'time.Tick' / 'time.After' 定时器；后台 goroutine 只在测速、订阅拉取、导出、预览解码期间存在（grep 'go func' 共 7 处，均有明确结束点）；后台事件统一进 'pending' 队列，由 'Invalidator' 唤醒、'applyPending' 在窗口线程合并（actions.go:441-468）。'TestIdleAfterRun' 也覆盖了空闲无帧。
- 后台 I/O 与窗口线程分离：订阅拉取、每轮导出、手动导出、PNG 解码都在 goroutine（state.go:869-872、export.go:292-295、399-432、preview.go:37-40）；绘制路径不碰网络与文件对话框。
- 数据目录不可写/不可用时降级为只读打开并弹出明确说明，状态栏固定显示「数据目录只读 · …」；保存操作追加「未保存」（state.go:514-541、1357-1375；测试 app_test.go:989-1006）。
- 过滤非法表达式保留上一个有效 matcher，只在过滤条下方显示错误，不清空列表（state.go:886-898、app.go:279-282）。
- 空态、过滤无匹配、条目未读取三类提示都有对应分支（view_speedtest.go:672-704），不是空白页。
- 无障碍名称大多预计算：行、复选框、三点图标、表头排序、分组折叠、分隔条都有 label；按钮/复选框/分段控件由 Viva 组件提供 focus ring 与键盘语义。
- 关于对话框列出的快捷键与应用级实现一致（dialogs.go:44-52 对照 shortcuts.go:8-53）：F1、Ctrl+A、Ctrl+Shift+A、Ctrl+R、Ctrl+Enter、Esc。
- 对比度抽查：浅色主题 'MutedForeground' #737373 对白底约 4.7:1，正文 10px 仍达 AA；深色主题更宽裕；未发现明显配色硬伤（未做像素级实测，见待确认）。

## 二、问题清单

### [X-01] 条目编辑/重读/删除后，轮次账本仍引用已消失节点，结果条与导出会复活旧节点
- 严重级别：严重
- 类型：状态流转 / 数据一致性
- 位置：'internal/gui/state.go:1140-1160'（rebuildResults 按 'round.nodes' 生成结果行）、'internal/gui/actions.go:998-1016'（confirmDeleteEntry 只删 entries，不动 rounds）、'internal/gui/state.go:758-807'（applyNodes 只替换 entry.nodes，不动 rounds）、'internal/gui/actions.go:953-972'（编辑条目后同样不清理 rounds）
- 复现步骤：
  1. 用一个含 2 个节点的本地 YAML 建条目并跑完一轮（产生 's.rounds[0]'，两节点都有结果）。
  2. 把 YAML 改成只剩 1 个节点，点条目卡片「重新获取」。实际：'entry.nodes' 只剩 1 个，但结果条仍按 'round.nodes' 显示已消失节点的结果（'s.results' 来自 rounds），「导出配置」仍可把消失节点写回 yaml。
  3. 或直接跑完一轮后删除该条目（「删除」→确认）。实际：条目从左侧消失，但结果条仍是「第 1/1 轮 · 已完成 …」，手动导出继续使用已删除条目的 'round.entry' 与节点配置。
- 期望 vs 实际：期望重读/编辑/删除条目时，裁剪 's.rounds' 中已不存在的条目/节点，或复位轮次账本；实际账本未同步，可能把已删除条目的节点凭据写进导出文件（隐私/正确性风险），且结果条文案与界面不一致。
- 依据：规格 8.2「节点被过滤/删除时详情与固定行自动清空」体现的状态一致性原则；Nielsen #4 一致性、#1 系统状态可见；现有测试 'TestRefetchKeepsResultAndDropsRemovedNode'（app_test.go:466-506）只断言 'entry.nodes'，未覆盖 's.rounds' / 's.results'，属覆盖空洞。
- 置信度：高（代码路径确定，未动态复现但可静态推导）。

### [X-02] 模态对话框没有隔离应用级快捷键、滚轮与拖放（F1 会覆盖对话框，Ctrl+Enter 可在对话框后面开测）
- 严重级别：严重
- 类型：模态语义 / 键盘与输入隔离
- 位置：'internal/gui/shortcuts.go:32-52'（onKey 不检查 'a.dialog'，F1 / Ctrl+Enter / 其余键都直接处理）、'shortcuts.go:8-27'（Ctrl+A/R 不检查 dialog）、'internal/gui/app.go:106-121'（onScroll 不检查 dialog，只检查 presetOpen/menu）、'internal/gui/window.go:35-55'（onFileDrop 不检查 dialog）、'internal/gui/dialogs.go:54'（openAbout 直接 'a.dialog = dialogAbout'）
- 复现步骤：
  1. 点「清空结果」打开确认框（无文本输入框），按 F1。实际：'openAbout()' 把 'a.dialog' 改为 'dialogAbout'，确认框被顶掉；关掉「关于」后确认框不会回来，清空操作需重新发起。
  2. 点「新建条目」，在名称输入框里输入内容后按 Ctrl+Enter（Viva 对单行 Field 的 Enter 会送到应用 onKey）。实际：'startSelectedTest()' 在弹窗背后直接开测，对话框仍显示，状态栏变成「开始测试…」。
  3. 弹窗打开时把鼠标移到节点列表滚轮，或拖入 YAML 文件。实际：列表仍滚动 / 条目仍被添加，模态背景未被阻断。
- 期望 vs 实际：模态对话框打开期间，应用级快捷键、滚轮滚动、文件拖放应被阻断或先关闭对话框；实际全都能穿透。可能丢失未保存表单（F1），或误触发测速。
- 依据：Windows 桌面设计规范（modal dialog 阻断背景交互）、Nielsen #3 用户控制与自由、#5 错误预防；ADR-0028 快捷键集合未声明可穿透模态。
- 置信度：高（F1/Ctrl+Enter/滚轮/拖放均无 'a.dialog' 守卫；Ctrl+Enter 在单行 Field 聚焦时仍进 onKey 有 Viva driver.go:555 依据）。

### [X-03] 测速进行中仍可通过分组「全选/全不选」和右键「选中/排除」改勾选，与工具条禁用不一致
- 严重级别：一般
- 类型：状态流转 / 一致性
- 位置：'internal/gui/view_speedtest.go:663-668'（分组标题的全选/全不选按钮无 '.Disabled(a.running())'）、'internal/gui/actions.go:73-78'（setGroupSelected 无 running 守卫）、'internal/gui/contextmenu.go:50-73' 与 'contextmenu.go:185-200'（右键选中/排除项在运行中未禁用，activateMenu 直接改 selected）
- 复现步骤：
  1. 选多个节点开始测速（运行中）。
  2. 点某分组标题的「全不选」。实际：该组节点勾选被清空，工具条「已选 x / 共 y」立即变化；但 's.run.batch' 与 's.rounds[].nodes' 是开测时快照，不会随之变化，后续仍按旧集合测/导出。
  3. 或运行中右键节点选「排除此节点」。实际同样改变勾选与计数。
- 期望 vs 实际：同一份「运行中禁用批量修改」的保护应一致；实际工具条「全选/全不选」「排除上次失败」已禁用，分组级与右键菜单却仍生效，造成界面勾选与已排队轮次脱节。
- 依据：Nielsen #4 一致性、#1 系统状态可见；规格 8.5「运行中禁用读取/重新获取/新建/删除条目」的同一状态约束思路。
- 置信度：高。

### [X-04] 自定义 Hit 控件（节点行、三点图标、条目卡片、分组箭头、表头）没有可见焦点，行与三点图标没有键盘激活路径
- 严重级别：一般
- 类型：无障碍 / 键盘可达性
- 位置：'internal/gui/view_speedtest.go:283-285'（行 'Hit(node.ids.row, …)'）、'359-368'（⋮ 只用 'PointerOn' 处理 PointerUp）、'372-377'（行点击同样只用 'PointerOn'）、'590-599'（条目卡片 'Hit' + 'Clicked'，但无 FocusVisible 样式）、'645-652'（分组箭头）、'222-229'（表头排序单元格）；整个 'view_speedtest.go' 没有 'FocusVisible()' 分支
- 复现步骤：
  1. 键盘按 Tab 逐次移动焦点，焦点会落在节点行/⋮/卡片/分组箭头上（Viva '.Hit' 默认 'RoleButton' 且可聚焦，flex/box.go:45-50、ui/canvas.go:1167-1185），但界面没有任何焦点环或底色变化，用户看不出焦点在哪里。
  2. 焦点停在某节点行时按 Enter 或 Space：'router.KeyDown' 会派发一次 click，但行代码只监听 'canvas.PointerOn'，没有 'canvas.Clicked(node.ids.row)' 分支，结果什么都不发生；⋮ 同理，键盘无法打开详情。
- 期望 vs 实际：所有可聚焦控件都应有可见焦点，行/⋮ 应能被 Enter/Space 激活；实际「无障碍 label 已写好但键盘操作是空操作」，属于可聚焦却不可用的伪控件。
- 依据：WCAG 2.1.1（键盘）、2.4.7（焦点可见）；Windows 桌面规范（所有命令可用键盘到达）；Nielsen #6 识别而非记忆。
- 置信度：高（代码中无 FocusVisible 绘制，行/⋮ 仅 PointerOn）。

### [X-05] 节点列表与设置表单没有键盘滚动，虚拟列表视口外的节点键盘不可达
- 严重级别：一般
- 类型：无障碍 / 键盘可达性
- 位置：'internal/gui/app.go:106-121'（onScroll 只处理滚轮，没有 PageUp/PageDown/方向键滚动）、'internal/gui/shortcuts.go:32-53'（onKey 除 F1/Ctrl+Enter/Esc 外只把事件交给分隔条）、'internal/gui/view_speedtest.go:67-96'（只声明视口内的行，视口外节点根本不注册 Hit）
- 复现步骤：
  1. 准备 50+ 节点，窗口只显示前 ~10 行；按 Tab 只能在前几行与工具条间循环，PageDown/方向键没有任何反应。
  2. 直到节点不可见后，用户无法把焦点移到第 20 行，也无法滚动到该行；长设置表单同样只能靠滚轮，键盘无法翻页。
- 期望 vs 实际：桌面应用的可滚动区域应支持 PageUp/PageDown/方向键或至少 Tab 到边缘时自动滚动；实际仅鼠标滚轮/拖动可用，纯键盘用户无法访问视口外内容。
- 依据：Windows 桌面规范 / WCAG 2.1.1；Nielsen #7 灵活高效（键盘用户效率）。
- 置信度：高（代码中无按键滚动路径，虚拟列表只注册可见行）。

### [X-06] 详情浮层把整窗和面板声明为可聚焦「按钮」，没有焦点陷阱，且键盘没有打开入口
- 严重级别：一般
- 类型：无障碍 / 层语义
- 位置：'internal/gui/view_speedtest.go:440-444'（'canvas.HitWithLabel(a.ids.detailDismiss, 全窗口矩形, "关闭节点详情")'）、'470-472'（'canvas.HitWithLabel(a.ids.detailClose, rect, "节点详情")'）、'451-486'（只用 PushLayer，无 PushFocusScope）
- 复现步骤：
  1. 鼠标点某行 ⋮ 打开详情。
  2. 连续按 Tab。实际：焦点会进入一个覆盖整个窗口、肉眼不可见的「按钮（关闭节点详情）」，以及一个「节点详情」按钮；没有焦点区域限制，Tab 也能继续跑到背后的行/工具条。
  3. 只用键盘时无法打开详情（⋮ 只有 PointerOn，见 X-04）。
- 期望 vs 实际：详情浮层应作为非模态弹层处理：背景 dismiss 使用 'SkipFocus'/'Backdrop'（Viva 的 'components.Dialog' 就是这么做），面板本身不应暴露为按钮；实际产生不可见 Tab 停靠点与错误的辅助技术语义。
- 依据：WCAG 4.1.2（名称/角色/值）、2.4.3（焦点顺序）；Nielsen #4 一致性；Viva 'HitWithOptions' Backdrop/SkipFocus 约定（ui/canvas.go:341-405、components/dialog.go:180-184）。
- 置信度：高。

### [X-07] 测速进行中「导出配置/结果图/结果表」仍可点击，会导出半成品且无警告
- 严重级别：一般
- 类型：状态流转 / 导出交互
- 位置：'internal/gui/view_speedtest.go:188-197'（'disabled := a.exporting > 0 || len(a.results) == 0'，不含 'a.running()'）、'internal/gui/state.go:1143-1157'（'s.results' 包含 'phaseQueued' / 'phaseTesting' 节点）、'internal/gui/export.go:363-371'（startManualExport 只按 results 判断）
- 复现步骤：
  1. 选 10 个节点开测，结果条出现后立刻点「导出配置」。
  2. 等到前 3 个节点出结果后再点一次。实际：'s.results' 因包含排队/在测节点而一直非空，按钮可点；第二次点击会把当前 3 条结果写成一个 yaml（其余 7 个节点缺失），状态栏只显示「正在导出…/已导出」，不提示不完整。
- 期望 vs 实际：期望导出入口在测速中禁用，或至少二次确认/明确「仅导出已完成结果」；实际无提示地输出半成品，用户可能拿它当完整结果发布。
- 依据：规格 8.7 手动导出「只处理当前轮」的完整语义；Nielsen #5 错误预防、#1 系统状态可见。
- 置信度：高（机制确定；是否允许进行中导出规格未明说，列入待确认）。

### [X-08] 多轮自动导出的成功路径/上传警告被下一轮状态覆盖，用户看不到产物位置或警告
- 严重级别：一般
- 类型：反馈 / 状态栏文案
- 位置：'internal/gui/actions.go:793-806'（setExportStatus 写入 's.status = "已导出：…"'）、'actions.go:787-789'（紧接着 advanceRound）、'actions.go:655-669'（advanceRound 调 startRound）、'actions.go:343-347'（startRound 覆盖 status 为「第 N/M 轮开始…」）；结果条只在 'a.exporting > 0' 时显示导出文案（view_speedtest.go:1275-1277）
- 复现步骤：
  1. 3 个条目开测、开启自动导出。
  2. 第 1 轮导出成功后，状态栏「已导出：…」在同一事件里被「第 2/3 轮开始：…」覆盖；用户只能看到后者。上传失败 warning（'setExportStatus' 的 warning 分支）同样被覆盖，后续轮次继续跑时用户完全不知道上一轮上传失败。
- 期望 vs 实际：期望产物路径/上传警告保留在结果条或可查看的历史状态中；实际只有单轮最后一轮才可能保住「已导出」，多轮场景下导出结果与警告不可见。
- 依据：Nielsen #1 系统状态可见、#9 帮助用户识别与恢复错误；规格 8.7「上传失败只作为状态栏提示」要求提示确实可见。
- 置信度：高。

### [X-09] 「清空测速结果」确认框的危险按钮文案固定为「删除」
- 严重级别：一般
- 类型：文案 / 危险操作
- 位置：'internal/gui/dialogs.go:191-213'，尤其 '206'：'if f.Button(a.ids.confirmOK, "删除").Destructive().Clicked()'
- 复现步骤：有测速结果时点结果条「清空结果」→ 弹出标题「清空测速结果」的确认框。实际：确认按钮写的是「删除」，而操作是清空结果，不删除条目/节点。
- 期望 vs 实际：按钮应写「清空」；「删除」会让用户误以为会删除条目或数据文件，与确认框标题/正文不一致。删除条目、删除预设复用同一个对话框，但三者的动作词不同，不能硬编码「删除」。
- 依据：Nielsen #2 贴近真实世界、#4 一致性；Windows 危险操作确认规范（动词与动作一致）。
- 置信度：高。

### [X-10] 把所有条目都取消勾选后，空态文案误导且没有恢复入口
- 严重级别：一般
- 类型：空态 / 引导
- 位置：'internal/gui/view_speedtest.go:672-697'（totalCount==0 时固定显示「当前条目还没有读取到节点。」，且只遍历 'selected' 条目显示「读取」按钮）、'internal/gui/state.go:947-957'（只有 selected 条目才计入 totalCount）
- 复现步骤：
  1. 有 2 个已读取节点的条目，把左侧两个条目卡片的复选框都取消勾选。
  2. 右侧立即变成空态。实际文案：「当前条目还没有读取到节点。」；下面没有任何条目行（因为循环跳过未选中条目），也没有提示去勾选左侧条目。
- 期望 vs 实际：期望显示「未选择任何节点来源，请在左侧勾选条目」并给出可操作入口；实际把「未选择条目」误报成「条目没读到节点」，用户在空页面里找不到下一步。
- 依据：Nielsen #1 系统状态可见、#9 帮助诊断；空态设计规范（说明原因 + 下一步动作）。
- 置信度：高。

### [X-11] 过滤设置（模式/组合/应用/预设）在绘制事件里同步写 settings.json，与项目「窗口线程不做 I/O」目标冲突
- 严重级别：一般
- 类型：性能 / 绘制路径副作用
- 位置：'internal/gui/app.go:221-227'（Choice 选中即 'setFilterMode'）、'app.go:238-244'（'setFilterCombine'）、'app.go:253-256'（「应用」按钮 'compileFilter' + 'persistSettings'）、'internal/gui/state.go:900-925'（setFilterMode/setFilterCombine/persistSettings 同步 'store.SetSettings'）；对照 'internal/gui/window.go:12-13'「拖动窗口高频回调，这里只更新内存；真正写盘在设置保存或退出时」
- 复现步骤：
  1. 在过滤条反复点「简化/正则」「全部命中/任一命中」，每次点击都在本帧绘制事件里同步重写一次 'data/settings.json'。
  2. 在机械硬盘/被安全软件扫描的目录下，或 settings.json 变大后，每次切换会出现可感知卡顿。
- 期望 vs 实际：过滤选择属于高频轻量交互，应与窗口几何一样只写内存、延迟到保存/退出时落盘；实际在绘制路径同步原子写文件，违背项目自述的「窗口线程不做 I/O」原则。
- 依据：项目约定（AGENTS.md「不要在 Paint/布局里同步做网络或文件对话框」的精神、window.go 注释）；性能响应性（系统应及时响应）。
- 置信度：高（代码事实；实际卡顿程度依赖磁盘，未实测）。

### [X-12] 确认/警告/关于/预览对话框没有设置 InitialFocusID，打开后键盘焦点为空
- 严重级别：轻微
- 类型：键盘 / 焦点管理
- 位置：'internal/gui/dialogs.go:191-198'（确认框）、'215-223'（警告框）、'57-64'（关于）、'79-86'（预览）均无 'InitialFocusID'；对照 '113-121'（条目对话框）与 '160-168'（预设对话框）设置了 'InitialFocusID'
- 复现步骤：点「清空结果」打开确认框后直接按 Enter。实际：刚打开时 'SetActiveScope' 会因原焦点不在新 scope 内把焦点清空，Enter 无任何反应；需要先按一次 Tab 才能聚焦到按钮。关于/预览/警告框同理。
- 期望 vs 实际：模态框打开时应把键盘焦点放到默认动作（危险确认通常放「取消」或首个按钮），与条目/预设对话框保持一致；实际焦点悬空，键盘用户多一步且第一下 Enter 无效。
- 依据：桌面平台对话框焦点规范；Nielsen #4 一致性、#6 识别而非记忆；Viva 'components.DialogOptions.InitialFocusID' 文档（components/dialog.go:16-18）。
- 置信度：高（代码可直接对照；实际驱动行为依据 components/overlay_initial_focus_test.go 语义）。

### [X-13] 规格 8.2 / 10.2 的详情面板描述已与现行实现冲突（文档完整性）
- 严重级别：轻微
- 类型：规格完整性 / 文档漂移
- 位置：'docs/specs/0001-viva-gui.md:168'「点行展开/收起详情」、':176'「详情面板：固定在列表下方，占 148px」、':306' 测试用例 8、':324' 测试用例 26；实现为行尾 ⋮ 打开的 paint 阶段浮层（view_speedtest.go:397-486，ADR-0029）
- 复现步骤：按规格 8.2 验收：期望点行打开底部 148px 详情面板；实际点行只标记行、不打开详情，详情由 ⋮ 打开为浮层。规格第 12 节又写明以 ADR-0029 为准，形成自相矛盾。
- 期望 vs 实际：期望规格 8.2/10.2 同步更新为「⋮ 打开浮层、无底部固定面板」；实际基线文档仍保留被 ADR 取代的描述，会误导后续验收与新成员。
- 依据：规格完整性（第 8/10/12 节一致性）、ADR-0029；此项为文档质量。
- 置信度：高。

## 三、功能缺失清单（缺失功能 + 影响范围）

1. **纯键盘完成核心操作的能力缺失**：详情只能鼠标点 ⋮ 打开；节点列表/设置表单无键盘滚动；行/卡片/⋮/分组箭头无可见焦点。影响：仅键盘或辅助技术用户无法查看详情、无法访问视口外节点（对应 X-04、X-05、X-06）。
2. **模态对话框的交互隔离缺失**：应用级快捷键、滚轮、拖放可穿透模态；F1 可覆盖当前对话框。影响：所有用户可能丢失未保存输入或在弹窗背后误开测（X-02）。
3. **条目删除/重读后的结果账本清理缺失**：删除条目、修改路径、重新获取后，旧轮次/旧节点仍留在结果条与导出集合。影响：结果与导出内容错误，可能把已删除条目的节点凭据写回文件（X-01）。
4. **测速中的导出防护缺失**：导出按钮未禁用、无「仅导出已完成结果」提示。影响：用户可能发布不完整或半成品配置（X-07）。
5. **Windows 拖放提示蒙层实际缺失（待确认）**：'dropActive' 只在 'FileDropOver' 置位，而 AGENTS 说明 Windows 只报 drop、不报 over/leave，'paintDropHint' 在 Windows 上可能永不出现；影响：拖放前没有「松开添加」的视觉确认，只有事后状态栏（window.go:35-55、155-173）。
6. **设置错误反馈的字段定位能力缺失**：设置校验只显示页面顶部一行中文错误，不把焦点移到出错字段、不滚动到该字段；影响：20+ 字段的长表单中用户需要自行寻找（view_settings.go:337-341、collectSettings）。

## 四、待确认项

1. **分组计数与工具条计数口径是否刻意不同**：工具条 '已选 x / 共 y' 统计所有条目的所有节点（ADR-0028 明确同口径），分组标题 'N 个节点 · 已选 M' 只统计过滤命中的可见节点（state.go:951-982）。过滤生效时两组数字会对不上；请确认是否应在分组标题注明「可见」或统一口径。
2. **设置校验「就地报错」的验收标准**：规格 8.6 写「非法值在保存时就地报错，不写盘」，实现是页顶单行 + 'Ellipsis()'（view_settings.go:337-341）。这是否满足「就地」需产品确认。
3. **暂停/停止在引擎 tester 就绪前的竞态**：'startRound' 先 'go s.engine.Run'，而 'engineRunner.Pause/Stop' 在 'runner.tester == nil' 时直接返回（engine.go:172-197）；若用户在 tester 赋值前点暂停/停止，UI 状态已变但引擎未暂停/未停止，取消 context 也没有传给 'TestProxiesUntil'（engine.go:92-150 只检查一次 ctx）。请确认是否需要在 'Run' 内监听 ctx 或在 tester 就绪前排队 Pause/Stop。
4. **F1 在输入框聚焦时是否可达**：Viva Driver 会把单行 Field 聚焦时的非 Tab/Esc/Enter 键交给平台文本系统（driver.go:555），因此 F1 在输入框聚焦时可能被吞掉，与关于文案「F1 关于」不一致；需在真实 Windows 键盘验证。
5. **窗口几何跨屏恢复的可见性**：启动直接用保存的 'Origin'（app.go:63-68），'Normalize' 只校验尺寸不校验位置（store.go:229-240）；副屏拔掉后窗口可能完全在屏幕外。ADR-0028 明确「位置允许负值、只校验尺寸」，请确认是否接受该风险（建议启动时校验与现有显示器交集，越界回退居中）。
6. **测速中手动导出是否应允许**：规格 8.7 未明说；当前实现允许（X-07）。若产品认为「随时可导出现有结果」是合理能力，应补充文案「仅导出已完成节点」；否则应禁用入口。
