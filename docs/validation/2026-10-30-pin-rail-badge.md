# 单测固定、折叠窄栏与徽标对齐验证记录（2026-10-30）

- 环境：Windows 11（amd64）、AMD Ryzen 7 4800H、Go 1.27.3（模块指令 1.26）
- 关联：[ADR-0022](../adr/0022-pin-rail-badge.md)、[规格 0001](../specs/0001-viva-gui.md)
- 前一轮：[展示细节修复](2026-10-30-display-fixes.md)

## 1. 范围

按试用截图修三处：

1. 「单独测试此节点」时结果到达后行会被排序抽走，且没有「正在操作哪一行」的视觉提示。
2. 侧边栏折叠后的 56px 窄栏很难看（贴边按钮 + 大片空白）。
3. 软/硬路径徽标文字没有在底色里居中、底色过浅。

## 2. 实现摘要

- **单测固定与高亮**：右键 `单独测试此节点`（或单选的 `立即测试此节点`）记录 `pinNode/pinIndex`；组内排序时先取出该行，排完其余节点再按原位置插回。该行用 `Primary` 14% 底色、节点名换成 `Primary`，一直保留到点表头或开始新一轮测速。整批测速会清掉固定，照常实时排序。
- **起测不再滚动**：`startRounds` 原来会 `scrollToRound()` 把当前轮分组滚到顶部，对手点的那一行就是一次跳动；现在只有自动推进/手动换轮才滚动定位。
- **折叠窄栏重做**：宽度 56 → 44px，顶部居中 `»` 点击区（`Text` + `Hit`，悬停浅底），下面逐字竖排「节点来源」；不再画贴边的「展开」按钮。
- **徽标胶囊重做**：显式 `Width/Height` 的 `Row`，文字在剩余宽度里居中；宽度取 `max(平台测量, 1.35em × 字数) + 12`；底色 `MutedForeground` 16%、文字 `Foreground` 85%、圆角 4。

## 3. Driver 测试

`go test ./internal/gui -count=1`：44 个用例全部通过（上一轮 43 个 + 本轮 1 个）。

- `TestSingleNodeTestPinsRowUntilSort`（新增）：三节点中单独测中间那个，测完仍停在第 2 行、`pinNode` 保持；点表头排序后解除固定。
- `TestEntriesPanelCollapsesAndResizesByDrag`（更新）：折叠后检查 `折叠` 按钮消失、`»` 可见、展开命中存在；点展开命中恢复。
- 其余 42 个用例（条目、订阅、过滤、预设、导出、上传、暂停、提前结束、空闲、数据目录、滚动条、合并视图）保持通过。

```
gofmt -l .               # 无输出
go vet ./...             # 通过
go test ./... -count=1   # 8 个包全部 ok
```

## 4. headless 走查

`scripts/headless-walkthrough.sh` 的折叠步骤改为点击「展开节点来源」命中、断言窄栏出现 `»`；其余流程不变。

结果：`PASS：headless 走查通过`；`layout` 在 1180×800 下 `0 problems`（徽标居中、窄栏无溢出）。日志见 [headless-walkthrough.log](headless-walkthrough.log)。

## 5. 截图核对（headless `shot`，不随仓库提交）

1. 徽标：`软路径` / `硬路径` 与 `Trojan` / `Vless` / `Shadowsocks` 都是灰底胶囊，文字居中、左右留白对称，白底上明显可辨。
2. 折叠窄栏：44px 栏，顶部 `»`、下面竖排「节点来源」，没有贴边按钮或大片空白。
3. 单测：右键 `单独测试此节点` 后列表不滚动、不动位；被点的 B-05 行显示蓝色浅底与蓝色节点名，结果（失败/100%）到达后仍在 B-04 与 B-06 之间。

## 6. 基准（1180×800，`go test -bench`，2000 次）

| 场景 | 时间 | 内存 | 分配 |
| --- | --- | --- | --- |
| 300 节点 | 76,254 ns/op | 380 B/op | 0 allocs/op |
| 20 节点 | 77,652 ns/op | 378 B/op | 0 allocs/op |
| 空数据 | 17,253 ns/op | 68 B/op | 0 allocs/op |

比上一轮（95,364 / 87,607 / 20,572 ns/op）更快：徽标从「文字加底色 + 最小宽度」改成显式尺寸胶囊后，测量与绘制都更简单。仍为 0 allocs/op，空闲不绘制。

## 7. 未验证项

- 真实窗口下固定行的浅蓝底色与主题色名称的对比度、高 DPI 下的徽标字号。
- 固定在表头排序前的语义是否会让用户困惑（表格箭头不再完全代表该行位置）；界面没有额外提示。
- 窄栏 `»` 只有 32px 宽点击区，真实鼠标下的命中手感未测。
- 超长协议名的胶囊宽度只按字符估算，未与真实字体逐一核对。
- `-race`：本机无 gcc，仍无法运行。

## 8. 文件变更

修改：

- `internal/gui/state.go`：`pinNode/pinIndex`、`nodeIndexInGroup`、`sortGroupRows` 取出/插回。
- `internal/gui/actions.go`：单测设置固定、整批测速清除固定、起测不再滚动。
- `internal/gui/view_speedtest.go`：固定行配色、折叠窄栏、徽标胶囊（`paintPill`）。
- `internal/gui/app_test.go`：新增单测固定用例；`interaction_test.go` 折叠断言适配窄栏。
- `scripts/headless-walkthrough.sh`：点击「展开节点来源」、断言 `»`。
- 文档：`docs/specs/0001-viva-gui.md`、`GLOSSARY.md`、`README.md`、`AGENTS.md`、新增 [ADR-0022](../adr/0022-pin-rail-badge.md)、本文件、`headless-walkthrough.log`。

## 9. 测试 exe

- 版本：`v2.8.2-viva-test`，对应代码提交 `af052ff`
- 大小：51,145,728 字节（上一版 v2.8.1 为 51,141,120 字节，+4.5 KB）
- SHA-256：`5f86ebbbb6c6b555c33542e3a7d574ba8de6a8e8dcedfaef21887251c3e64be4`
- 形态：PE32+ GUI x86-64

```bash
CGO_ENABLED=0 go build -trimpath \
  -ldflags "-s -w -H=windowsgui -X main.version=v2.8.2-viva-test -X main.commit=af052ff" \
  -o clash-speedtest-plus.exe .
```
