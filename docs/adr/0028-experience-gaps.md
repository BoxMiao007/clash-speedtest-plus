# ADR-0028：体验补齐（全局选择、清空结果、拖放、窗口记忆、结果图预览等）

- 状态：已接受
- 日期：2026-10-30
- 关联：[ADR-0027](0027-feature-gaps.md)、[ADR-0020](0020-merged-view.md)、[ADR-0023](0023-group-collapse-sticky.md)、[规格 0001](../specs/0001-viva-gui.md)

## 背景

功能正确性补齐后（ADR-0027），仍有一批「能用但不顺手」的体验缺口：多组全选要逐组点、清结果只能删条目、输出路径只能手输、加本地条目只能走对话框、版本号只在标题栏、窗口每次启动回默认尺寸、结果图只能导出后自己打开、排查配置问题要手动找目录、多个订阅要逐个重新获取。本 ADR 记录一次性补齐这九项的决策。

## 决策

1. **全局全选/全不选**：测速工具条加「全选」「全不选」，作用于**所有条目的所有节点**（不受过滤影响），与「已选 x / 共 y」统计同口径；分组级「全选/全不选」保留。快捷键 Ctrl+A / Ctrl+Shift+A 同一动作。
2. **清空测速结果**：结果条加「清空结果」（有结果才出现），先弹确认框再执行：`store.ClearResults()` 清盘 + 内存节点复位为「未测试」+ 清轮次账本与挂起导出。条目、节点、订阅缓存不动。测速或导出进行中禁用。
3. **输出目录浏览**：设置页「输出路径」行加「浏览…」，用 `app.OpenDirectory` 选目录；沿用原文件名（无则 `导出.yaml`），只替换目录部分。
4. **拖放添加条目**：窗口级 `OnFileDrop`。拖动经过时整窗淡色蒙层加提示条；松开后把每个 `.yaml/.yml` 文件加成本地条目（名称取文件名去扩展名，重名追加序号），立即读取节点并写盘；目录与非 YAML 跳过并汇总提示。Windows 只报 drop，不报 over/leave。
5. **关于与帮助**：页脚「关于」按钮 + F1 打开对话框，内容为版本、构建提交（`gui.Options.Commit`，main.go 传入）、数据目录与全部快捷键。正文打开时拼一次，绘制只读。
6. **窗口几何记忆**：`Settings` 增加 `window_width/height/x/y/set`；启动时尺寸与 `Origin` 用保存值（`Normalize` 拒绝过小/过大的尺寸）。`OnFrameChange` 只更新内存（拖动高频回调不做磁盘 I/O），保存设置或退出时随 `SetSettings` 落盘。保存设置不会清掉窗口字段。
7. **结果图预览**：结果条加「预览结果图」（最近一次成功导出的 PNG），打开预览对话框；PNG 在后台 goroutine 解码成 **预乘 BGRA** 位图（Viva `ui.Bitmap` 的格式），经 pending 事件回窗口线程，`components.PaintImageView` 绘制（几何取对话框内容区上一帧布局，与节点视口同一套做法）。支持「适应窗口/原始大小」切换；解码失败在对话框内报错。
8. **打开配置所在目录**：节点右键菜单加「打开配置文件所在目录」（订阅条目为「打开订阅缓存目录」），打开该节点所属条目的目录。Viva 的 `app.OpenPath` 只接受普通文件，目录交给平台命令（explorer/open/xdg-open），参数直接传进程不经 shell；以包级变量 `openDirectory` 作测试接缝。
9. **订阅全部重新获取**：节点来源面板加「全部重新获取」（存在订阅条目时显示），对每个订阅条目执行强制拉取（走既有 `fetchSeq` 代次守卫）；快捷键 Ctrl+R 同一动作。

**快捷键集合**（关于对话框原样列出）：F1 关于 · Ctrl+A 全选 · Ctrl+Shift+A 全不选 · Ctrl+R 重新获取全部订阅 · Ctrl+Enter 开始测速 · Esc 关闭详情。输入框聚焦时标准编辑和弦先被平台文本系统消费，应用快捷键不与编辑冲突。

## 取舍

- 全选/全不选不区分过滤命中：与计数同口径，避免「全选后计数对不上」的困惑；被过滤隐藏的节点也会被选中。
- 清空结果不提供「仅清当前条目」：删除条目已连带清结果（既有行为），本次只补「全量重置」。
- 窗口几何存 `settings.json` 而非独立文件：它本就是「窗口内可调状态」的持久化位置，不新增数据文件。位置允许负值（副屏在主屏左侧/上方），只校验尺寸。
- 拖放不弹确认：拖入即添加是常见意图，误拖的条目可直接删除；一次拖多个文件逐条添加。
- 预览不做缩放拖拽/滚轮，只做两档切换：结果图是固定版式，两档够用。
- 打开目录用 `os/exec` 而非引入新依赖；路径来自用户自己的条目数据，不经 shell 无注入面。

## 影响

- `internal/store/store.go`：`Settings` 窗口字段 + 校验；`ClearResults`。
- `internal/gui/state.go`：新对话框/确认/事件类型；预览、拖放、窗口几何、最近结果图状态。
- `internal/gui/window.go`（新）：OnFrameChange、拖放处理、`addLocalEntry`/`uniqueEntryName`、拖放提示绘制。
- `internal/gui/shortcuts.go`（新）：OnShortcut、应用级 OnKey（F1/Ctrl+Enter/Esc）。
- `internal/gui/preview.go`（新）：预览状态机、PNG→预乘 BGRA 解码、位图绘制。
- `internal/gui/openpath.go`（新）：`openDirectory` 接缝与平台实现。
- `internal/gui/actions.go`：全选/全不选、清空结果、订阅批量重新获取、打开条目目录、导出事件记录 lastImage。
- `internal/gui/dialogs.go`：关于、预览对话框。
- `internal/gui/view_speedtest.go`：工具条全选/全不选、结果条预览/清空按钮、面板「全部重新获取」。
- `internal/gui/view_settings.go`：浏览按钮；collectSettings 保留窗口字段。
- `internal/gui/app.go`：Commit 选项、窗口几何恢复、新回调接线、页脚「关于」。
- `main.go`：传 commit。
- Driver 测试 57 → 66；走查 `PASS`、`layout` 0 problems；绘制基准 0 allocs/op 保持。
