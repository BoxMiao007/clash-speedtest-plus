## Agent skills

### Issue tracker

Issues 放在本仓库的 GitHub Issues，通过 `gh` CLI 读写。详见 `docs/agents/issue-tracker.md`。

### Triage labels

使用默认五类角色标签：`needs-triage`、`needs-info`、`ready-for-agent`、`ready-for-human`、`wontfix`。详见 `docs/agents/triage-labels.md`。

### Domain docs

单上下文：根目录 `CONTEXT.md` + `docs/adr/`。改行为前先读相关 ADR。术语以 `CONTEXT.md` 为准，不要换近义词。详见 `docs/agents/domain.md`。

## 命令

- Go 1.24。模块 `github.com/BoxMiao007/clash-speedtest-plus`。
- 测试：`go test ./...`。单测：`go test ./speedtester -run TestName`。不需要外部服务，也不要拿真实订阅跑测。
- 仓库没有 lint、typecheck 或 codegen 步骤。源码里没有 `//go:generate`。
- 本地二进制：`go build -o clash-speedtest-plus .`。`-v` 的版本来自 ldflags `-X main.version` / `-X main.commit`，本地构建默认是 `dev` / `unknown`。
- 发布二进制才带 `-tags=with_gvisor` 和 `CGO_ENABLED=0`，见 `.goreleaser.yaml`。日常测试不要加这个 tag。
- 发版打 git tag 触发 `.github/workflows/release.yml`。版本号次位只到 9：`1.9` 之后是 `2.0.0`，没有 `1.10`（ADR-0009）。
- `.goreleaser.yaml` 和 `.goreleaser.yml` 都在。改发布配置时两份一起改；内容冲突时以 `.goreleaser.yaml` 为准。

## 布局

单模块，不是 monorepo。入口是根目录 `main.go`，`download-server/` 是另一个 `package main`。

- `speedtester/`：测速、订阅规范化、调度。
- `picker/`：无参数启动时的选源界面。
- `tui/`：测速表格界面。
- `output/`：结果表图、TSV。国旗在 `output/flags/`，由 `//go:embed` 打进二进制。
- `gist/`：Gist 与 GitHub 仓库上传。
- `ip/`：按 IP 位置重命名节点。

用户可见文案用简体中文。标识符和协议字段保持原文。

## 不要猜错的约定

- **节点并行**是 `-p` / `-parallel`（同时测几个节点）。**下载并发**是 `-concurrent`（同一节点的连接数）。不要混用，也不要改 `-concurrent` 的旧语义（ADR-0001）。
- 命令行 `-parallel` 默认 `1`。选源界面的「节点并行」默认 `6`。两套默认值不是 bug。
- 不带参数且 stdout 是终端才进选源界面。管道或无终端时打印用法并以非 0 退出。带任何参数走命令行。
- 结果图和相对路径的 `-o` 写到**程序文件所在目录**，不是启动时的当前目录。绝对路径照写（ADR-0004）。`go run` 的程序目录是临时构建目录，产物不会落在仓库根。
- 合格配置只认程序目录一层的 `.yaml`，不认 `.yml`。`proxies` 或 `proxy-providers` 至少有一项才能勾选。
- 多个测速源各自一轮，不混测。订阅地址用「逗号加空格」或换行分开；不带空格的逗号仍是 URL 的一部分。
- 订阅规范化顺序：原样 yaml → 整段 base64 → 地址没有 `flag` 时补 `flag=meta` 再请求。不认分享链接，也不认 JSON。选源默认 UA 是 `clash.meta`；命令行 `-c` 走 mihomo 内核 UA，空 `-ua` 不覆盖它。
- Clash/Mihomo 配置用 `gopkg.in/yaml.v2` 解析。不要改成 yaml.v3。
- 「产物跟随文件名」写死开启。`-name-from-config` 已删除，不要加回来（ADR-0013）。
- `test/test.yaml`、`*.local.yaml`、`clash-speedtest-plus-sub-*.yaml` 含节点凭据，禁止入库。`picker/` 里已有的订阅 yaml 是本地拉取物，不是测试夹具。
