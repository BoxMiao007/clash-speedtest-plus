# 验证记录：GUI 补齐 TUI 能力（2026-10-30）

> 对应：[规格 0001](../specs/0001-viva-gui.md)、[ADR-0018](../adr/0018-tui-parity.md)。
> 环境：Windows 11（amd64）、AMD Ryzen 7 4800H、Go 1.27.3（模块指令 1.26）。
> 所有测试离线：订阅用 `httptest`，引擎用假实现，节点夹具全是 `127.0.0.1` / `example.com` 假值。

## 1. 改动范围

- 条目分本地文件 / 订阅地址；订阅正文缓存到 `data/subscriptions/<id>.yaml`，启动只读缓存。
- 新增「设置」视图：模式、大小、超时、并发、并行、服务器、UA、提前结束、四项阈值、输出模式与路径、结果图、重命名模板、Gist/仓库上传；保存到 `settings.json`（版本 2）。
- 测速按条目分轮、自动推进、轮间切换；暂停/继续、提前结束、停止。
- 每轮自动导出：完整 profile yaml、结果图 PNG、Gist/仓库上传；多轮完成后合并导出。
- 结果视图：表头排序、行详情、上传列（完整模式）、手动导出配置/结果图/结果表。
- 过滤结果现在决定测速与导出范围（与旧 TUI 的 `-f` 一致）。
- 恢复 `internal/output`、`internal/ip`、`internal/gist`；新增 `internal/export`；`go.mod` 新增 `golang.org/x/image`。
- 修复结果图目录被进程工作目录限制的问题：程序目录与 cwd 不同盘时（桌面启动、测试临时目录）写图会报 `Rel: can't make … relative to …`，现在按显式传入的绝对目录写。

## 2. Driver 测试（`internal/gui`，33 个）

第一版 17 个用例保留并继续通过；本次新增 16 个：

| 用例 | 覆盖 |
|---|---|
| `TestSubscriptionEntryFetchesOnceAndLoadsCache` | 新建订阅条目 → 拉取一次 → 写缓存 → 重启只读缓存不再请求 |
| `TestEntryKindSwitchToSubscription` | 本地条目改成订阅地址后保存为订阅、地址变化强制重拉 |
| `TestMultipleEntriesRunSequentialRounds` | 两个条目各自一轮、自动推进并停在最后一轮 |
| `TestSettingsSavePersistsAndConfiguresEngine` | 设置保存写盘、下一轮引擎收到新参数、完整模式显示上传列 |
| `TestRoundExportWritesProfile` | 输出模式「当前目录下」自动写 yaml，含 `mixed-port: 7890`、两组与 `MATCH` |
| `TestFilterLimitsTestedNodes` | 过滤命中 1 个时只把 1 个节点交给引擎 |
| `TestPauseResumeUpdatesStatus` | 暂停/继续按钮文案与状态 |
| `TestResultRowTogglesDetailPanel` | 点行展开详情、再点收起 |
| `TestSortHeadersReorderResults` | 点表头换列、再点反转方向 |
| `TestManualTSVExport` | 手动导出结果表，表头与节点行齐全 |
| `TestExportedStoppedRoundDoesNotAdvance` | 已停止的轮导出结束后不推进剩余轮次 |
| `TestEarlyStopCounterCountsOnlyAcceptedResults` | 提前结束只数过筛结果，限额 0 不停止 |
| `TestBuildProxiesAppliesThresholds` | 导出过筛挡掉慢节点、不改原配置、缺 name/server 跳过 |
| `TestRoundExportWritesResultImage` | 结果图开启时每轮结束写出非空 PNG |
| `TestBuildProxiesRenamesWithLocation` | 重命名只改导出副本、套模板、查询失败保留原名 |
| `TestUploadConfigSendsToGistAndRepo` | 按设置把配置分发到 Gist 与仓库，参数与文件名正确 |

其他包：store 25（新增订阅缓存、设置归一化、版本 1 兼容）、filter 16、speedtester 31（新增 4 个订阅拉取用例）、output 35、export 5、ip 6、gist 5。

复跑命令与结果：

```
gofmt -l .            # 无输出
go vet ./...          # 通过
go test ./... -count=1
ok  internal/export  / filter / gist / gui / ip / output / speedtester / store
```

`go test ./... -count=3` 连跑同样全绿；`-race` 未执行（本机没有 gcc，`-race` 需要 cgo）。

## 3. headless 走查（`scripts/headless-walkthrough.sh`）

构建 `-tags vivahead` 二进制，`VIVA_HEADLESS=1` 走线协议，不打开窗口。本次在原有步骤上增加：

- 进入「设置」视图（`texts` 断言「测速与输出设置」）。
- 点「下载大小（MB）」→ `select 0 4096` → 输入 `30` →「保存设置」。
- 测速前置：预置 `settings.json` 的 `output_mode=auto`、关闭结果图与重命名、阈值全放开。
- 测速后等 `已完成 1/1` 与「已导出」，再返回选择、过滤、预设、新建条目、`layout`、`quit`。

结果：

```
== 持久化断言 ==
settings.json 的 output_mode=auto、download_mb=30
entries.json 的机场 B / 软路径 b.yaml
presets.json 的只看HK
results.json 的失败结果
导出 yaml 含 mixed-port: 7890 与 MATCH,节点选择
layout: ok 0 problems
PASS：headless 走查通过
```

日志：[headless-walkthrough.log](headless-walkthrough.log)。

## 4. 绘制基准（2000 次迭代）

| 基准 | ns/op | B/op | allocs/op |
|---|---|---|---|
| 300 节点节点列表 | 47,335 | 285 | 0 |
| 20 节点节点列表 | 45,922 | 281 | 0 |
| 空列表 | 13,308 | 64 | 0 |

设置视图与结果视图新增的字段都在状态变化时预计算（排序箭头、暂停文案、详情文本），绘制路径仍为 0 allocs/op；ns/op 多次运行互有 ±5% 波动，B/op 与 allocs/op 稳定。

## 5. 体积 / 启动 / 空闲内存

构建命令：`CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o ...`（发布版另加 `-H=windowsgui`）。

| 指标 | 第一版 GUI | 本次 | 变化 |
|---|---|---|---|
| stripped 二进制 | 49,884,672 B | 51,121,152 B | +1,236,480 B（+2.48%） |
| 启动到首帧（headless，暖机） | 空 90–92 ms；300 节点 107–120 ms | 空 85–94 ms；300 节点 106–110 ms | 基本持平 |
| 空闲 RSS（headless，静置 2s） | 空 22,816 KB；300 节点 30,216 KB | 空 22,400 KB；300 节点 23,572 KB | 本次采样更低，见下 |
| 空闲私有内存（同次采样） | 60,136 / 67,252 KB | 60,964 / 60,548 KB | — |

说明：headless 不创建 GPU 渲染器，上表**不含真实窗口的 GPU 显存与呈现开销**；冷启动约 650–700 ms，为 DLL/字体冷加载，表中取 5 次暖机范围。体积增量来自恢复的结果图/上传/重命名代码与 `golang.org/x/image`。空闲 RSS 两次采样方法相同但进程调度与系统缓存不同，数值只作量级参考，不作为优化结论。

## 6. 未在真实窗口验证的项目

- IME、真实键盘、焦点丢失、高 DPI、窗口管理器关闭、真实 GPU 渲染外观与帧率。
- 真实订阅地址（含 base64 与 `flag=meta` 重试）只在本机 `httptest` 与单测里验证，未用真实机场地址跑通。
- Gist / GitHub 仓库上传：`gist` 包请求用 httptest，GUI 分发用假 uploader 覆盖；没有用真实 token 验证。
- IP 归属地重命名会访问 `ip-api.com`，本次未联网验证真实返回；GUI 分支用假查询覆盖，`internal/ip` 单测覆盖模板。
- 结果图的写出由 Driver 用例与 `internal/output` 单测覆盖，但真实窗口字体下的最终外观没有人工比对。
- 系统文件对话框真实交互未验证（Driver 里用 `QueueOpenFiles` 拦截）。
- 未执行 `-race`（本机无 gcc）。

## 7. 已知限制

- 上传 token 明文存 `data/settings.json`，无系统钥匙串集成。
- 导出时逐节点查询 IP 归属地（每个节点一次 HTTP 请求），节点多时导出慢；可关闭重命名规避。
- 订阅缓存可能过期，需要用户点「重新获取」。
- 结果仍按整文件重写 `results.json`，节点很多时写入开销随结果数增长。
- 启动时同步读取本地条目文件；订阅条目只读缓存（不联网），但本地文件很大时首帧会等待。
- 暂停只停止派发新节点，不中断在测节点的当前请求。
- 输出模式默认关闭；需要导出必须到设置里开启。

## 8. 测试 exe

- 文件：`clash-speedtest-plus.exe`（仓库根目录，`*.exe` 已忽略，不随仓库分发）
- 版本：`v2.7.0-viva-test`，提交 `0b6edca`
- 大小：51,122,688 字节；SHA-256：`9e6ed46d998b93c10225ad20587b217b86138675a8b458527364a478c501d654`
- 构建：`CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -H=windowsgui -X main.version=v2.7.0-viva-test -X main.commit=0b6edca" -o clash-speedtest-plus.exe .`
- 文件类型：PE32+ GUI（x86-64）
- 交付文件清单：[2026-10-30-file-changes.md](2026-10-30-file-changes.md)
