# Clash Speedtest Plus

本仓库是 [faceair/clash-speedtest](https://github.com/faceair/clash-speedtest) 的下游，协议仍为 [GPL-3.0](LICENSE)。本项目使用 Vibe Coding 开发。

基于 Clash/Mihomo 核心测代理节点的延迟、抖动、丢包和下载速度，不依赖另外的 Clash/Mihomo 进程。界面用 [Viva](https://vivaui.dev) 构建：Go 原生、GPU 绘制，没有 WebView/HTML/CSS/JS。

Windows / Linux（amd64、arm64）二进制在 [Releases](https://github.com/BoxMiao007/clash-speedtest-plus/releases)。macOS 的 Viva 后端依赖 cgo，需要在 Mac 上从源码构建（见下文）。也可以从源码构建，见下文。

> **Windows 首次运行**：exe 没有代码签名，SmartScreen 可能提示「发布者未知」。点「仍要运行」即可。同一文件放行过一次后不再提示；重新下载的新版本第一次仍会提示。

## 直接试用本次构建

仓库根目录的 `clash-speedtest-plus.exe` 是本次 Viva 重构的本地测试构建（Windows GUI 子系统，未入库）：

- 版本：`v2.8.3-viva-test`，对应提交 `26a4503`
- 大小：51,157,504 字节；SHA-256：`5f8d7616ccc6864a88c60b946f037ce4d14244744c8218435acbe91f829df1da`
- 直接双击运行；数据写在 exe 同目录的 `data/`，第一次打开为空
- 点「新建条目」→ 命名 → 选「本地文件」用文件对话框挑一份 Clash/Mihomo yaml，或选「订阅地址」填 http(s) 链接 → 保存
- 要导出配置：先到「设置」把输出模式改成「当前目录下」或「自定义」，再点「下载速度」；每轮测完自动写 yaml 与结果图
- 试用完可删除 exe 与 `data/`，不会影响源码仓库

构建命令：

```bash
CGO_ENABLED=0 go build -trimpath \
  -ldflags "-s -w -H=windowsgui -X main.version=v2.8.3-viva-test -X main.commit=26a4503" \
  -o clash-speedtest-plus.exe .
```

## 界面

窗口只有两个视图：`测速` 与 `设置`。节点选择和测速结果合并在同一个「测速」视图里，用一张表完成勾选、开测、看结果、排序、详情和手动导出。

- **节点来源**：左侧面板管理条目，支持本地 Clash/Mihomo 配置文件与 http(s) 订阅地址。新建/重命名/改路径或地址/删除；订阅条目拉取后缓存到 `data/subscriptions/`，启动只读缓存、点「重新获取」才联网。卡片只显示一条路径（本地是完整路径，订阅是 URL），软/硬路径与订阅用徽标标出；面板可点「折叠」收成 44px 窄栏（`»` 展开、竖排「节点来源」），也可拖动右边界调宽（方向键可微调）。
- **测速表**：按条目分组展示节点，复选框选择（组内全选/全不选；「排除上次失败」一键取消失败节点的勾选）。组标题可用 `▾`/`▸` 折叠；滚动时组标题固定在列表顶部，滚到下一个条目自动切换，被盖住的行不显示也不可点。节点副标题是 `server:port` 加协议徽标；表头列出状态、延迟、抖动、丢包、下载速度、上传速度（完整模式）和上次测速。点表头排序（组内排序），点行展开详情；列表溢出时右侧出现可拖动的滚动条。
- **过滤**：默认简化模式，空格分隔关键词，`!` 排除，`*` / `?` 通配，正向词可选「全部命中 / 任一命中」并带一句说明；切到正则模式是完整 Go 正则（区分大小写）。过滤条件可保存成命名预设，跨条目通用。过滤结果同时决定测哪些节点、导出哪些节点。
- **下载速度**：点工具栏「下载速度」开始测速，**不切视图**。每个选中条目各一轮、自动推进；结果在原表里按当前排序列实时重排（默认下载速度降序），测速中的行显示瞬时速度。多轮时结果条上的「上一轮 / 下一轮」把对应条目分组滚到顶部。
- **暂停/继续与提前结束**：测速中可暂停派发新节点；设置里可让过筛结果达到指定数量时提前结束。
- **设置**：模式（快速/下载/完整）、下载与上传大小、超时、单节点并发、节点并行、服务器 URL、拉取 UA、提前结束、最高延迟/最高丢包/最低下载/最低上传四项阈值、输出模式与路径、三个产物开关（每轮保存结果图 / 结果图只保留有速度的节点 / 导出时按 IP 归属地重命名）、重命名模板、Gist/仓库上传凭据。每个开关带一行说明；全部保存到 `data/settings.json`。
- **导出**：每轮结束按输出模式自动写完整 profile（`mixed-port: 7890` + select/url-test 两组 + 国内直连 + MATCH）、可选写结果图 PNG、可选上传 Gist/仓库；多轮全部完成后额外写合并导出。结果条上另有「导出配置 / 结果图 / 结果表」手动导出。
- **右键菜单**：单个节点与多选节点用不同措辞（「复制此节点信息」vs「复制所选节点信息」），单层平铺；多选时另有「单独测试此节点」，只测右键指向的那个节点。单测的行会原位固定并高亮，结果到达不跳动，点表头排序或重新测速后解除。
- **重新获取节点**：重新读文件后，身份匹配的节点保留结果与上次测速时间，新节点显示「未测试」，消失的节点移除。

## 数据目录

所有持久化数据都在**程序文件所在目录**的 `data/` 下：

```
<程序目录>/data/
  settings.json       过滤、测速参数、阈值、输出、重命名、上传
  entries.json        节点来源条目（本地文件 / 订阅地址）
  presets.json        过滤预设
  results.json        按条目与节点身份保存的测速结果
  subscriptions/      订阅正文缓存（<条目 ID>.yaml）
  mihomo/             proxy-providers 的内核缓存（使用 provider 时产生）
```

不写用户主目录、AppData 或注册表；拷贝整个程序目录即可迁移。程序目录不可写时窗口内会明确报错，并禁用持久化，不会改存别处。

条目路径支持两种形态，界面上会标出当前形态：

- **软路径**：相对程序目录，例如 `test/a.yaml`；程序目录整体迁移后仍然有效。
- **硬路径**：绝对路径，例如 `D:\订阅\a.yaml`。程序目录内的绝对路径保存时自动转成软路径。

从旧版升级：旧版没有持久化条目，需要重新添加一次测试文件或订阅地址。旧版的命令行 flag 与 TUI 已移除，对应能力都搬进了窗口：订阅地址、测速参数、阈值、导出配置、结果图、TSV、Gist/仓库上传、IP 改名、多轮队列、暂停/继续。

上传 token 以明文保存在 `data/settings.json`，只在本机使用；不要把数据目录发给别人。输出模式默认关闭，需要导出时到「设置」里选「当前目录下」或「自定义」。

## 从源码构建

需要 Go 1.26+。

```bash
go build -o clash-speedtest-plus .
```

Windows 想要双击不弹控制台窗口：

```bash
go build -ldflags "-H=windowsgui" -o clash-speedtest-plus.exe .
```

发布二进制由 `.goreleaser.yaml` 构建（Windows/Linux，`-tags=with_gvisor`、`CGO_ENABLED=0`、stripped）。macOS 需要本机 cgo：`CGO_ENABLED=1 go build -o clash-speedtest-plus .`。`main.version` / `main.commit` 由 ldflags 注入，本地构建显示 `dev`。

## 验证

```bash
gofmt -l .
go vet ./...
go test ./... -count=1
```

GUI 行为用 Viva 的 `app.Driver` 驱动真实绘制、路由、焦点和文本输入，测速引擎换成假实现，不联网。无窗口的端到端走查：

```bash
scripts/headless-walkthrough.sh
```

它会构建 `-tags vivahead` 的二进制，用 `VIVA_HEADLESS=1` 走线协议操作界面，校验条目、分组、右键菜单、过滤、预设、设置保存、自动导出、结果写入与持久化；不打开窗口。

## 设计文档

- 需求基线：[docs/specs/0001-viva-gui.md](docs/specs/0001-viva-gui.md)
- 架构决策：[ADR-0017](docs/adr/0017-viva-gui.md)（GUI 架构）、[ADR-0018](docs/adr/0018-tui-parity.md)（补齐 TUI 产物能力）
- 术语：[GLOSSARY.md](GLOSSARY.md)
- 无窗口验证记录：[docs/validation/2026-10-29-viva-gui.md](docs/validation/2026-10-29-viva-gui.md)、[docs/validation/2026-10-30-tui-parity.md](docs/validation/2026-10-30-tui-parity.md)
- 交付文件变更：[docs/validation/2026-10-29-file-changes.md](docs/validation/2026-10-29-file-changes.md)、[docs/validation/2026-10-30-file-changes.md](docs/validation/2026-10-30-file-changes.md)

## License

[GPL-3.0](LICENSE)。上游项目与协议见 [faceair/clash-speedtest](https://github.com/faceair/clash-speedtest)。
