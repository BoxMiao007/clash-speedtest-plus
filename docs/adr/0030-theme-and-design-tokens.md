# ADR-0030：主题（深色/浅色/跟随系统）与设计令牌

- 状态：已接受
- 日期：2026-11-03
- 关联：[ADR-0017](0017-gui-architecture.md)、[ADR-0026](0026-row-active-state.md)、[ADR-0029](0029-interaction-refinements.md)

## 背景

功能已经齐全，但用户反馈界面偏旧、缺少深色主题。原实现只有 Viva 默认浅色（`ui.LightTheme()`）：颜色直接写在各个视图里，间距与字号没有统一尺度，对话框的标题、按钮排布和错误提示各写一套，视觉上不统一。

## 决策

1. **主题是设置项**：`store.Settings.Theme`，取值 `dark` / `light` / `system`，默认 `dark`。空值与非法值在 `Settings.Normalize` 里补成 `dark`，旧版本数据文件不用迁移。
2. **一处解析 token**：`internal/gui/theme.go` 的 `themeMode` / `parseThemeMode` / `resolveTheme` 产出 `ui.Theme` + `palette`；`state.theme` / `state.palette` 由 `applyThemeMode` 一次性折算，绘制路径只读，不在每帧算颜色。
3. **palette 补 ui.Theme 没有的语义色**：`Accent`（强调/焦点/进度）、`AccentSoft`、`Track`、`RowHover`、`RowActive`、`RowPin`、`RowMarked`、`Grid`、`Sunken`（表头/凹陷面）、`Raised`（弹窗/浮层）、`Ghost`。组件只认 `ui.Theme`，应用自己的绘制用 palette。
4. **布局令牌统一**：`pagePadX` / `headerH` / `filterBarH` / `footerH` / `rowH` / `groupBarH` / `gapXS…gapL` / `fontCaption…fontTitle`。行高同时被 `state`（列表几何、`buildNodeItems`）与 `view_speedtest`（绘制）引用，避免两处写死不同数字导致行重叠。
5. **两个切换入口，同一个状态**：标题栏快捷分段控件（`ids.themeScope`）与设置页「界面 · 配色」（`ids.settingsTheme`）；改动立即生效、立即落盘，并触发重绘。
6. **跟随系统**：`themeSystem` 读 `app.SystemAppearance()`，用 `app.WatchSystemAppearance` 订阅系统深浅色切换；回调只置 `themeStale` 并请求重绘，下一帧绘制开始处重算一次 token。`app.SetNativeAppearance` 让系统自绘部件（macOS 标题栏、文件对话框）跟界面一致。
7. **视觉语言**：卡片 = Card 底 + `RadiusSurface` + 1px Border；凹陷面 = Sunken；浮层 = Raised + `ShadowMD/LG`。行悬停只在当前底色上叠强调色（沿用 ADR-0026，不覆盖选中态）；成功/失败用 `theme.Success` / `theme.Danger`；数值列等宽右对齐；节点行之间加 1px `Grid` 分隔线；条目来源面板用 Sunken 区分于内容区。
8. **浮层沿用 Viva 组件**：对话框继续用 `flex.Dialog`、菜单继续用 `components.ContextMenu`（宽度/高度测量复用组件），只补视觉——底部按钮右对齐、错误用 `noticeChip`（danger 12% 衬底）、危险菜单项用 destructive variant、拖放蒙层压暗背景 + 强调色光晕。
9. **窄窗口先丢次要信息**：内容区宽度 < 820px 时节点行不再画协议徽标，优先保住 `server:port`（`nodeTypeFits()`）。判定只用宽度算术，不做逐帧字符串拼接。

## 取舍

- **不改窗口结构、不改任何既有文案、ui.Tree 身份与交互语义**。视觉重构不能改变可测行为：`go test ./...`、`scripts/headless-walkthrough.sh` 全绿是验收条件。
- **默认深色而不是跟随系统**：用户明确要黑色主题；「跟随系统」作为可选项保留。
- **主题色只做强调，不给整卡上色**：条目卡片仍保持默认卡片底色，选中与否由复选框表达（ADR-0026 仍然成立）。
- **深色不纯黑**：底 `#0B0E14`，比纯黑柔和，长时间看测速表不刺眼；卡片 `#12161F`、抬升 `#171C26`。
- **行高沿用 40px**：紧凑到 34px 需要同时改列表几何与固定标题裁剪，收益小于回归风险；改用细分隔线与字号层级提升可读性。

## 影响

- 新增 `internal/gui/theme.go`（模式解析、两套 token、`palette`、布局常量、`mix`/`alpha`/`pillBox`/`statusDot`/`noticeChip`）。
- 新增 `internal/gui/theme_test.go`：模式解析、标题栏切换落盘、重启沿用、保存设置不丢主题。
- `internal/store/store.go`：`Settings.Theme` + `DefaultTheme` + `Normalize`；`store_test` 的期望值补 `Theme`。
- `internal/gui/app.go`：`Options.ThemeMode`（测试固定配色）、标题栏品牌块与主题开关、页脚状态灯、系统外观订阅与 `themeStale` 重算。
- `internal/gui/state.go`：`themeMode`/`palette` 字段、加载设置后应用主题、`persistSettings` 写回 `theme`、列表几何改用 `rowH`/`groupBarH`。
- `internal/gui/view_settings.go`：新增「界面」分区与配色控件；`collectSettings` 保留 `Theme`（表单不收集它，但不能清掉）。
- `internal/gui/view_speedtest.go`、`dialogs.go`、`contextmenu.go`、`window.go`、`ids.go`：视觉重构（行分隔线、状态色、等宽数值、卡片化面板、弹窗强调条、拖放蒙层）。
- 测试：`internal/gui` 74 → 78 个用例。
- `qa/probe9.sh`：深浅两套主题与浮层的无头截图走查（`qa/shots/91…97`）。
