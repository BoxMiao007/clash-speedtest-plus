## Agent skills

### Issue tracker

Issues 放在本仓库的 GitHub Issues，通过 `gh` CLI 读写。详见 `docs/agents/issue-tracker.md`。

### Triage labels

使用默认五类角色标签：`needs-triage`、`needs-info`、`ready-for-agent`、`ready-for-human`、`wontfix`。详见 `docs/agents/triage-labels.md`。

### Domain docs

单上下文：根目录 `GLOSSARY.md` + `docs/adr/`。改行为前先读相关 ADR。术语以 `GLOSSARY.md` 为准，不要换近义词。详见 `docs/agents/domain.md`。

## 命令

- Go 1.26（`go.mod`）。模块 `github.com/BoxMiao007/clash-speedtest-plus`。
- 测试：`go test ./... -count=1`。单测：`go test ./internal/gui -run TestName`。不需要外部服务，也不要拿真实订阅跑测。GUI 用 `app.Driver` + 假引擎，不联网。
- 仓库没有 lint、typecheck 或 codegen 步骤。源码里没有 `//go:generate`。
- 本地二进制：`go build -o clash-speedtest-plus .`。Windows 想要无控制台窗口加 `-ldflags "-H=windowsgui"`。版本号来自 ldflags `-X main.version` / `-X main.commit`，本地默认 `dev` / `unknown`，显示在窗口标题旁。
- 发布二进制带 `-tags=with_gvisor` 和 `CGO_ENABLED=0`，见 `.goreleaser.yaml`。发布矩阵只含 Windows/Linux（amd64、arm64）；Viva 的 macOS 后端依赖 cgo，macOS 需在 Mac 上 `CGO_ENABLED=1` 从源码构建。日常测试不要加 release tag。
- headless 走查：`scripts/headless-walkthrough.sh`（构建 `-tags vivahead`，用 `VIVA_HEADLESS=1` 走线协议，不开窗口）。
- 发版打 git tag 触发 `.github/workflows/release.yml`。版本号次位只到 9：`1.9` 之后是 `2.0.0`，没有 `1.10`（ADR-0009）。
- `.goreleaser.yaml` 和 `.goreleaser.yml` 都在。改发布配置时两份一起改；内容冲突时以 `.goreleaser.yaml` 为准。

## 布局

单模块，不是 monorepo。入口是根目录 `main.go`（Viva GUI），`download-server/` 是另一个 `package main`。

- `internal/gui/`：Viva 应用与全部界面（`测速` / `设置` 两视图、过滤条、预设、右键菜单、对话框）。`测速` 视图把节点选择与结果合并成一张表（`view_speedtest.go`）；`Runner` 接口是 GUI 与引擎的唯一接缝；`export.go` 编排每轮导出，`view_settings.go` 是设置表单，`scrollbar.go` 是三条可见滚动条（绘制、命中与拖动）。
- `internal/speedtester/`：测速、订阅规范化与拉取（`FetchSubscriptionBody` / `ParseSubscriptionNodes`）、调度、`LoadNodesFile`。
- `internal/store/`：数据目录探测、设置/条目/预设/结果 JSON、订阅缓存、软硬路径互转。
- `internal/filter/`：简化关键词过滤与高级正则。
- `internal/output/`：结果图渲染、导出路径与产物序号、TSV、结果表格式化。
- `internal/export/`：导出 profile 骨架与阈值过滤（导出与提前结束共用）。
- `internal/ip/`：IP 归属地查询与节点重命名模板。
- `internal/gist/`：Gist / GitHub 仓库上传。
- `docs/specs/0001-viva-gui.md`：需求基线；ADR-0017（GUI 架构）、ADR-0018（TUI 能力补齐）、ADR-0019（折叠/调宽/滚动条与文案）、ADR-0020（合并节点选择与结果视图）、ADR-0021（路径/协议徽标、单独测试、详情留白）、ADR-0022（单测固定高亮、折叠窄栏、徽标对齐）。

界面代码遵守 Viva 规则：空闲不绘制；绘制/布局路径无反射、无 `map[string]any`、无常驻分配；命中身份一律 `ui.Tree` 的名字，行身份用节点稳定身份而非列表位置；点击在控件声明处用 `Clicked()` / `PointerOn` 处理。

用户可见文案用简体中文。标识符和协议字段保持原文。

## 不要猜错的约定

- **节点并行**是同时测几个节点（默认 `6`）。**下载并发**是同一节点的连接数（默认 `4`）。都可在设置里改；不要混用，也不要改 `-concurrent` 的旧语义（ADR-0001）。
- 程序目录 = `os.Executable()` 所在目录。**全部持久化数据**写在 `<程序目录>/data/`（`settings.json` / `entries.json` / `presets.json` / `results.json` / `subscriptions/` / `mihomo/`），不写主目录、AppData、注册表；目录不可写时窗口内报错并禁用持久化，不换位置（ADR-0017）。
- 数据文件版本为 2；加载更高版本视为错误。版本 1 的文件按字段缺失读取并补默认值（布尔开关按「键是否存在」补，不能先解析零值再归一化）。
- 条目有两类：本地文件（软路径 `/` 分隔或硬路径）与订阅地址（`http(s)`）。订阅正文缓存到 `data/subscriptions/<条目 ID>.yaml`；启动只读缓存不联网，`重新获取`/地址变更才拉取。
- **节点身份** = 协议 + 名称 + 服务器 + 端口 + 规范化参数序列化的 sha256 前缀（yaml.v2 排序键，能处理嵌套 map）；同一文件里的完全相同节点追加 `#2`、`#3`。重新获取节点时按身份保留结果与上次测速时间，不依赖列表顺序。
- 测速按条目分轮：每个选中条目一轮，轮内节点 = 选中 ∩ 过滤命中，测完导出再推进；事件带轮次号，迟到事件按轮次丢弃。
- 节点选择与测速结果共用 `测速` 视图的一张表：复选框负责勾选，点行只打开详情，不要再让点行切勾选；分组保留，排序只在组内进行（`nodeState.order` 兜底未测速顺序）。多轮时 `上一轮`/`下一轮` 只把对应分组滚到顶部，不隐藏其它节点（ADR-0020）。
- 测速表默认 fast 按延迟升序、其余按下载速度降序；点表头切换列、再点反转；行命中身份来自节点身份，重排不换 ID。表头文案与无障碍名称在状态变化时预计算，绘制里不拼字符串。
- 条目卡片只显示一条路径（本地为解析后的完整路径，订阅为 URL），形态用 `软路径`/`硬路径`/`订阅` 徽标；节点协议也用徽标（`paintPill`）。中文徽标宽度取「平台测量」与「1.45em×字数」的较大值再加内边距，否则参考字宽（约 1.35em/字）会把自动宽度判成截断。
- 详情面板下方的 `nodeViewport` 已经是缩短后的列表高度，绘制时不要再扣 `detailPanelHeight`，否则底部行会被跳过、留下空白。
- 多选右键菜单保留 `单独测试此节点`（只测右键指向的节点），单选用 `立即测试此节点`；两者都要在测速中置灰。
- 手动单测会设置 `pinNode/pinIndex`：组内排序时取出该行、排完其余再原位插回，行用主题色高亮；点表头或开始新一轮测速时清空。开测不要主动 `scrollToRound`，只有换轮才滚动定位。
- 折叠窄栏是 44px：`»` 点击区（Text+Hit）加逐字竖排「节点来源」，不要再用贴边按钮。徽标用显式宽高的胶囊（文字居中、`MutedForeground` 16% 底），不要只给 Text 加底色。
- 过滤简化模式：空格分词、`!` 排除、`*`/`?` 通配、大小写不敏感、正向词可选「全部命中 / 任一命中」（界面上不是「与/或」）；高级模式是完整 Go 正则（区分大小写）。预设保存模式 + 表达式 + 组合方式，跨条目通用。过滤结果同时决定测速与导出范围。
- 节点来源面板用 `flex.Splitter` 调宽（`SplitterState.Pos` 是面板宽度，方向键也走它），折叠时只画 44px 窄栏、不再声明 Splitter；折叠状态与宽度是纯界面状态，不落盘。
- 滚动条归应用侧：`flex.ScrollArea` 只做裁剪与偏移，轨道/滑块/拖动在 `scrollbar.go`（几何与绘制复用 `components.ScrollArea`，拖动改 `ScrollState.Offset` 或列表偏移）。不要给滚动条手写数值命中区。
- 导出骨架固定（ADR-0016）：`mixed-port: 7890`、`节点选择` + `自动选择` 两组、国内直连 + `MATCH`；阈值过滤只影响导出，不影响结果表。输出模式默认关闭。
- 重命名只在导出时按 IP 归属地套模板，不改界面显示；上传失败只提示，不影响轮次推进。上传 token 明文存 `data/settings.json`。
- 订阅拉取、导出（含 IP 查询、图片渲染）、上传都在后台 goroutine；不要在 Paint/布局里同步做网络或文件对话框（用 `canvas.AfterPaint` + goroutine）。
- Clash/Mihomo 配置用 `gopkg.in/yaml.v2` 解析。不要改成 yaml.v3。
- `test/test.yaml`、`*.local.yaml`、`clash-speedtest-plus-sub-*.yaml` 含节点凭据，禁止入库。测试夹具只用 `127.0.0.1` / `example.com` 假值。

## 版本管理
版本号遵循 SemVer 2.0.0。本项目没有版本文件，版本由 git tag `vX.Y.Z` 表示。
除非用户明确要求发版，否则不要创建 tag。发版时只打 annotated tag，不修改源码中的版本号。
不需要维护 CHANGELOG。
