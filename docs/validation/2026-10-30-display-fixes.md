# 展示细节修复验证记录（2026-10-30）

- 代码基线：`39df308`（功能）/ `0942159`（测试与文档）
- 环境：Windows 11（amd64）、AMD Ryzen 7 4800H、Go 1.27.3（模块指令 1.26）
- 关联：[ADR-0021](../adr/0021-entry-badge-menu-detail.md)、[规格 0001](../specs/0001-viva-gui.md)
- 前一轮：[合并视图](2026-10-30-merged-view.md)。后续：单测固定、折叠窄栏与徽标对齐，见 [2026-10-30-pin-rail-badge.md](2026-10-30-pin-rail-badge.md)。

## 1. 范围

按试用截图修三处展示/交互问题，不改测速、导出与数据格式：

1. 条目卡片重复显示两条路径；要求只留一条，软/硬路径用徽标标出。
2. 节点协议名混在副标题里，要求用徽标突出。
3. 多选右键菜单缺少「单独测试右键指向的节点」。
4. 打开节点详情后，列表底部出现大片空白。

## 2. 实现摘要

- **条目路径**：卡片只显示一条路径（本地为解析后的完整路径，订阅为 URL），前面加 `软路径` / `硬路径` / `订阅` 徽标；订阅条目的缓存文件路径不再显示，缓存位置由 README 说明。
- **协议徽标**：行副标题拆成 `server:port` 与协议徽标（`paintPill`，主题 `Muted` 底、圆角、字号 9）；长服务器名最多 180px，超出省略。
- **多选单独测试**：`menuState` 记录右键指向的节点，多选菜单新增 `单独测试此节点`（测速中置灰），只把该节点交给引擎；单选的 `立即测试此节点` 保持原语义。
- **详情留白**：`nodeViewport` 是上一帧列表（Stack）的实际高度、已扣掉详情面板；绘制时再次扣 `detailPanelHeight` 使虚拟化阈值偏小，底部约 4 行被跳过。现在只在首帧没有记录时用窗口高度估算并扣详情高度。
- **中文徽标宽度**：Viva 布局检查的参考字宽把 CJK 按约 1.35em 估，平台排版按 1em；自动宽度的中文徽标会被判为截断。`paintPill` 对含非 ASCII 的标签设置 `1.45em × 字数 + 内边距` 的最小宽度，两种度量都放得下；ASCII 协议名保持自动宽度。

## 3. Driver 测试

`go test ./internal/gui -count=1`：43 个用例全部通过（上一轮 41 个 + 本轮 2 个）。

- `TestEntryCardShowsPathBadgeAndSinglePath`（新增）：软/硬路径的徽标与单条路径；旧「软路径 ·」前缀不再出现。
- `TestDetailPanelKeepsListRowsFilled`（新增）：打开详情后，视口内的每一行都必须有命中（修复前底部约 4 行缺失）。
- `TestContextMenuWordingSingleAndMulti`（扩展）：多选菜单包含 `单独测试此节点`；执行后引擎批次只有右键指向的那一个节点、只生成一轮。

```
gofmt -l .               # 无输出
go vet ./...             # 通过
go test ./... -count=1   # 8 个包全部 ok
```

## 4. headless 走查

`scripts/headless-walkthrough.sh` 增加：条目卡片出现 `软路径` 徽标；多选右键菜单出现 `单独测试此节点`（复制所选后关闭菜单，再回到单测流程）。

结果：`PASS：headless 走查通过`；`layout` 在 1180×800 下 `0 problems`，中文徽标不再被判截断。日志见 [headless-walkthrough.log](headless-walkthrough.log)。

## 5. 截图核对（headless `shot`，不随仓库提交）

1. 条目卡片：`软条目` 与 `硬条目` 各只有一行路径，前面分别是「软路径」「硬路径」徽标；不再出现两行同一路径。
2. 节点行：副标题显示 `127.0.0.1:21000`，紧跟 `Trojan` / `Vless` / `Shadowsocks` 徽标；协议不再是一串纯文本。
3. 详情：点行后列表缩到详情面板上方，可见行连续填满（B-00 至 B-05 部分可见），没有空白带。
4. 多选右键：`复制所选节点信息` / `立即测试所选节点` / `单独测试此节点` / 分隔线 / `取消选中所选节点` / `排除所选节点`。

## 6. 基准（1180×800，`go test -bench`，2000 次）

| 场景 | 时间 | 内存 | 分配 |
| --- | --- | --- | --- |
| 300 节点 | 95,364 ns/op | 285 B/op | 0 allocs/op |
| 20 节点 | 87,607 ns/op | 283 B/op | 0 allocs/op |
| 空数据 | 20,572 ns/op | 68 B/op | 0 allocs/op |

与上一轮（65,805 / 65,277 / 16,371 ns/op）相比再慢约 30–40%：每个可见行多画一个协议徽标（文字 + 圆角底色），徽标还带最小宽度测量。基准只画视口内的行，空闲仍然完全不绘制；若后续行数密集，可考虑把协议徽标改为只画文字或用图标，当前先保留可读性。

## 7. 未验证项

- 真实窗口下徽标的观感与高 DPI 下的字号/圆角；徽标是纯展示控件，不参与命中。
- 较长协议名（如 `ShadowsocksR`）与超长服务器名共存时的挤压表现（服务器名已限 180px 省略）。
- 订阅条目去掉缓存路径后的可发现性：用户需要按 README 的 `data/subscriptions/<条目 ID>.yaml` 规则找文件。
- `-race`：本机无 gcc，仍无法运行。

## 8. 文件变更

修改：

- `internal/gui/state.go`：`entryState.pathText/pathBadge` 取代 `pathText/absText`；`decorateEntry` 订阅分支去掉缓存路径；移除未使用的 `node.subtitle`。
- `internal/gui/format.go`：`formatPathKind` → `pathFormBadge`（只返回形态）。
- `internal/gui/actions.go`：写入订阅缓存后不再拼缓存路径文案。
- `internal/gui/view_speedtest.go`：卡片路径徽标、协议徽标（`paintPill`）、详情视口修复。
- `internal/gui/contextmenu.go`、`ids.go`：多选 `单独测试此节点` 与指向节点状态。
- `internal/gui/app_test.go`：路径徽标、详情填充、多选单测三处断言。
- `scripts/headless-walkthrough.sh`：徽标与多选菜单断言。
- 文档：`docs/specs/0001-viva-gui.md`、`GLOSSARY.md`、`README.md`、`AGENTS.md`、新增 [ADR-0021](../adr/0021-entry-badge-menu-detail.md)、本文件、`headless-walkthrough.log`。

## 9. 测试 exe

- 版本：`v2.8.1-viva-test`，对应代码提交 `39df308`
- 大小：51,141,120 字节（上一版 v2.8.0 为 51,139,072 字节，+2 KB）
- SHA-256：`c3d608ce5213f82f29b656c168e260f4711ed85ab19eedd8dcce8868044fca3e`
- 形态：PE32+ GUI x86-64

```bash
CGO_ENABLED=0 go build -trimpath \
  -ldflags "-s -w -H=windowsgui -X main.version=v2.8.1-viva-test -X main.commit=39df308" \
  -o clash-speedtest-plus.exe .
```
