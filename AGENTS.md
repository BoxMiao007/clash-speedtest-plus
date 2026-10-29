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

- `internal/gui/`：Viva 应用与全部界面（节点选择 / 下载速度双视图、过滤条、预设、右键菜单、对话框）。`Runner` 接口是 GUI 与引擎的唯一接缝。
- `internal/speedtester/`：测速、订阅规范化、调度、`LoadNodesFile`。
- `internal/store/`：数据目录探测、条目/预设/结果 JSON、软硬路径互转。
- `internal/filter/`：简化关键词过滤与高级正则。
- `docs/specs/0001-viva-gui.md`：本分支需求基线。

界面代码遵守 Viva 规则：空闲不绘制；绘制/布局路径无反射、无 `map[string]any`、无常驻分配；命中身份一律 `ui.Tree` 的名字，行身份用节点稳定身份而非列表位置；点击在控件声明处用 `Clicked()` / `PointerOn` 处理。

用户可见文案用简体中文。标识符和协议字段保持原文。

## 不要猜错的约定

- **节点并行**是同时测几个节点（GUI 固定 `6`）。**下载并发**是同一节点的连接数（GUI 固定 `4`）。不要混用，也不要改 `-concurrent` 的旧语义（ADR-0001）。
- 程序目录 = `os.Executable()` 所在目录。**全部持久化数据**写在 `<程序目录>/data/`（`settings.json` / `entries.json` / `presets.json` / `results.json`），不写主目录、AppData、注册表；目录不可写时窗口内报错并禁用持久化，不换位置（ADR-0017）。
- 条目路径是软路径（相对程序目录、`/` 分隔）或硬路径（绝对路径）；程序目录内的绝对路径保存时自动转软路径。`go run` 的程序目录是临时构建目录，数据不会落在仓库根。
- **节点身份** = 协议 + 名称 + 服务器 + 端口 + 规范化参数序列化的 sha256 前缀（yaml.v2 排序键，能处理嵌套 map）；同一文件里的完全相同节点追加 `#2`、`#3`。重新获取节点时按身份保留结果与上次测速时间，不依赖列表顺序。
- 下载速度视图的行按有效下载速度实时降序；行命中身份来自节点身份，重排不换 ID。
- 过滤简化模式：空格分词、`!` 排除、`*`/`?` 通配、大小写不敏感、与/或可选；高级模式是完整 Go 正则（区分大小写）。预设保存模式 + 表达式 + 组合方式，跨条目通用。
- Clash/Mihomo 配置用 `gopkg.in/yaml.v2` 解析。不要改成 yaml.v3。
- `test/test.yaml`、`*.local.yaml`、`clash-speedtest-plus-sub-*.yaml` 含节点凭据，禁止入库。测试夹具只用 `127.0.0.1` / `example.com` 假值。

## 版本管理
版本号遵循 SemVer 2.0.0。本项目没有版本文件，版本由 git tag `vX.Y.Z` 表示。
除非用户明确要求发版，否则不要创建 tag。发版时只打 annotated tag，不修改源码中的版本号。
不需要维护 CHANGELOG。
