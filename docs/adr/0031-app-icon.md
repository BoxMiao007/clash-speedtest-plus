# ADR-0031：应用图标（表盘弧 + 闪电）

- 状态：已接受
- 日期：2026-11-04
- 关联：[ADR-0030](0030-theme-and-design-tokens.md)（配色令牌）、[ADR-0017](0017-gui-architecture.md)

## 背景

界面重构（ADR-0030）之后，程序仍然没有自己的图标：窗口和任务栏显示的是系统默认图标，exe 在资源管理器里也没有图标，标题栏只能用一个「速」字色块代替品牌标记。用户要求设计一个合适的图标。

## 决策

1. **图形**：深色圆角方块（圆角 = 边长 21.8%）+ 蓝色表盘弧 + 白色闪电。表盘弧表达「测速」（缺口朝下的仪表盘），闪电表达「高速」并充当指针。两个实心大形状保证 16px 任务栏尺寸仍可辨认。
2. **配色**沿用 ADR-0030 的令牌：底色 `#171F2E → #080B11` 渐变 + 中心蓝色辉光，表盘弧 `#8BDCFF → #4C8DFF → #2563EB`（中段等于深色主题 `palette.Accent`），闪电 `#F8FAFC`。表盘弧最暗一段不能沉进底色，否则 16px 下半个环消失。
3. **可复现生成**：`scripts/make-icon.py`（Pillow + numpy，4 倍超采样后 LANCZOS 缩放）从同一组参数导出 SVG、9 档 PNG 与多尺寸 ICO，并同步写运行时用的 `internal/gui/icon-256.png`、`icon-48.png`。图标是设计资产，不手改 PNG。
4. **窗口/任务栏图标走 Viva 配置**：`app.Config.Icon`（`WindowIcon{Size, RGBA}`）。`internal/gui/icon.go` 用 `go:embed` 打包 256px PNG，启动时解码成直通 RGBA，只算一次；不做运行时 SVG 光栅化（不引新依赖）。
5. **标题栏品牌块用同一份图**：flex 只占 24×24 位置，位图在 `view.Draw` 之后按本帧矩形画（与结果图预览同一套做法），位图 ID 与预览的位图分开（渲染器按 ID 缓存纹理）。
6. **exe 文件图标用 rsrc 资源**：`rsrc_windows_amd64.syso` / `rsrc_windows_arm64.syso` 入库，文件名带 GOOS/GOARCH 后缀，Go 只把匹配当前构建的文件链进去——因此 Linux 构建与另一种架构都不受影响（已用三种目标交叉构建验证）。
7. **小尺寸优先**：图标先在 16/20/24/32px 下放大对比再定稿；表盘弧用 mask 画（不用逐段 `draw.line` 拼，接缝会留扇贝锯齿），圆头画在线带中心线上。

## 取舍

- **不用 CJK 字形做图标**：单字「速」在 16px 会糊；字形只适合大尺寸。品牌块因此从「速」字色块改成图标位图。
- **exe 图标用二进制 syso 入库**：仓库没有 codegen 步骤，`go generate` 需要额外安装 rsrc；入库两个 66KB 的资源文件换取「`go build` 就有图标」，符合现有构建习惯。
- **不做运行时 SVG 光栅化**：会引入依赖（ADR 要求新增依赖须测量体积/内存）；PNG 打包只增加约 40KB。
- **不生成 macOS .icns**：Viva 的 macOS 后端依赖 cgo，发布矩阵不含 macOS；将来做 macOS 包时再从 PNG 生成 icns。

## 影响

- 新增 `scripts/make-icon.py`、`assets/icon/`（SVG + 9 档 PNG + ICO + 说明）、`internal/gui/icon.go`、`internal/gui/icon_test.go`、`rsrc_windows_{amd64,arm64}.syso`。
- `internal/gui/app.go`：`app.Config.Icon` 与标题栏品牌位图（`paintBrand`）。
- 测试：`internal/gui` 78 → 81 个用例（图标解码/通道格式/配置携带）。
- 文档：根目录 README 增加「图标」小节；`assets/icon/README.md` 记录设计理由与重新生成步骤。
