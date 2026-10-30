# ADR-0025：收尾修复：只读数据目录、导出唯一序号与行右缘

- 状态：已接受
- 日期：2026-10-30
- 关联：[ADR-0017](0017-viva-gui.md)、[ADR-0024](0024-audit-fixes.md)、[规格 0001](../specs/0001-viva-gui.md)

## 背景

上一轮审查列出了若干未修项：导出失败会丢掉挂起的另一轮、手动导出与同轮自动产物可能同名覆盖、数据目录只读时连读取都被禁用、列表右缘 10px 属于滚动条轨道导致右键不出菜单、双击行会开→关详情、固定行与详情行底色接近。

## 决策

1. **只读打开数据目录**：`store.Open` 保持「可写才成功」；新增 `store.OpenReadOnly`，不创建目录、不探测，只加载 `settings/entries/presets/results`。GUI 在 `Open` 失败时退到只读：已有数据照常显示，弹「数据目录不可写」提示，状态栏显示「数据目录只读 ·」，写操作跳过并标注「未保存」。`Store` 增 `readOnly` 与 `ErrReadOnly`，写方法统一拒绝。
2. **导出失败保留挂起轮**：按规格仍停止推进，但不再清空 `pendingExport`；用户修正设置后手动导出成功时会补写该轮。新一轮测速开始时清空 `pendingExport`，避免旧队列污染。
3. **手动导出独立序号**：手动导出的 seq 用 `output.NextImageSequence(目标目录)` 现取，不再复用批次的 `exportSeq`；同轮自动产物与手动产物不会同名覆盖。
4. **行右缘让位给滚动条**：行容器与表头右侧各留 10px（`PadSides(0,30,0,20)`），滚动条轨道的命中不再盖住行的点击/右键。
5. **双击第二下不切换**：行的详情切换检查 `PointerOn` 的 `ClickCount`，大于 1 时忽略，避免双击刚打开就被关掉。
6. **详情行改用加深灰底**：`MutedForeground` 14%，与主题色固定行（Primary 14%）区分。
7. **只读时订阅仍可用**：拉取成功后跳过缓存写入，但照常解析节点并提示「数据目录只读，未写缓存」。

## 取舍

- 只读时保存类操作在内存里生效、状态栏提示「未保存」，对话框照常关闭；用户能看到结果，但重启会丢，这是「照常打开、只禁用持久化」的直接结果。
- 行内容宽度少 10px，窄窗口下省略更早触发；换来行右缘不再被滚动条抢点击。
- 双击只接受单击切换，连续双击的第二下被忽略（不影响复选框）。

## 影响

- `internal/store/store.go`：`OpenReadOnly`、`ReadOnly()`、`readOnly` 字段、`ErrReadOnly`、写方法守卫。
- `internal/gui/state.go`：只读回退与提示、`readOnly()/savable()`、`noteSaved`、`persistSettings`、结果写盘跳过。
- `internal/gui/actions.go`：导出失败保留 `pendingExport`、起测清空、订阅只读解析、写调用改 `savable()`。
- `internal/gui/export.go`：手动导出序号。
- `internal/gui/view_speedtest.go`：行/表头右侧留白、双击忽略、详情行底色。
- Driver 测试 51 → 52、store 测试 +1；走查 `layout` 仍 0 problems。
