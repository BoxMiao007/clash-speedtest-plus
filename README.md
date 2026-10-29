# Clash Speedtest Plus

本仓库是 [faceair/clash-speedtest](https://github.com/faceair/clash-speedtest) 的下游，协议仍为 [GPL-3.0](LICENSE)。本项目使用 Vibe Coding 开发。

基于 Clash/Mihomo 核心测代理节点的延迟、抖动、丢包和下载速度，不依赖另外的 Clash/Mihomo 进程。界面用 [Viva](https://vivaui.dev) 构建：Go 原生、GPU 绘制，没有 WebView/HTML/CSS/JS。

Windows / Linux（amd64、arm64）二进制在 [Releases](https://github.com/BoxMiao007/clash-speedtest-plus/releases)。macOS 的 Viva 后端依赖 cgo，需要在 Mac 上从源码构建（见下文）。也可以从源码构建，见下文。

> **Windows 首次运行**：exe 没有代码签名，SmartScreen 可能提示「发布者未知」。点「仍要运行」即可。同一文件放行过一次后不再提示；重新下载的新版本第一次仍会提示。

## 直接试用本次构建

仓库根目录的 `clash-speedtest-plus.exe` 是本次 Viva 重构的本地测试构建（Windows GUI 子系统，未入库）：

- 版本：`v2.6.0-viva-test`，对应提交 `710c44e`
- 大小：49,884,672 字节；SHA-256：`c794d1e5c9aa986d9c39a1f2f0cc3b000a4c133b90d0695d21bcf1d8a7264173`
- 直接双击运行；数据写在 exe 同目录的 `data/`，第一次打开为空
- 点「新建条目」→ 命名 →「选择文件…」选一份本地 Clash/Mihomo yaml → 保存
- 试用完可删除 exe 与 `data/`，不会影响源码仓库

构建命令：

```bash
CGO_ENABLED=0 go build -trimpath \
  -ldflags "-s -w -H=windowsgui -X main.version=v2.6.0-viva-test -X main.commit=710c44e" \
  -o clash-speedtest-plus.exe .
```

## 界面

- **测试文件**：左侧面板管理条目（自定义名称 + 本地 Clash/Mihomo 配置路径）。新建/重命名/改路径/删除；路径通过系统文件对话框选择。
- **节点选择**：按条目分组展示节点，复选框选择；组内全选/全不选；「排除上次失败」一键取消上次失败节点的勾选。
- **过滤**：默认简化模式，空格分隔关键词，`!` 排除，`*` / `?` 通配，正向词可选「与 / 或」；切到正则模式是完整 Go 正则（区分大小写）。过滤条件可保存成命名预设，跨条目通用。
- **下载速度**：点工具栏「下载速度」开始测速并自动切换。结果按下载速度实时降序；状态列区分等待中 / 测试中 / 完成 / 失败。
- **右键菜单**：单个节点与多选节点用不同措辞（「复制此节点信息」vs「复制所选节点信息」），单层平铺。
- **重新获取节点**：重新读文件后，身份匹配的节点保留结果与上次测速时间，新节点显示「未测试」，消失的节点移除。

## 数据目录

所有持久化数据都在**程序文件所在目录**的 `data/` 下：

```
<程序目录>/data/
  settings.json   过滤模式/表达式/组合方式
  entries.json    测试文件条目
  presets.json    过滤预设
  results.json    按条目与节点身份保存的测速结果
```

不写用户主目录、AppData 或注册表；拷贝整个程序目录即可迁移。程序目录不可写时窗口内会明确报错，并禁用持久化，不会改存别处。

条目路径支持两种形态，界面上会标出当前形态：

- **软路径**：相对程序目录，例如 `test/a.yaml`；程序目录整体迁移后仍然有效。
- **硬路径**：绝对路径，例如 `D:\订阅\a.yaml`。程序目录内的绝对路径保存时自动转成软路径。

从旧版升级：旧版没有持久化条目，需要重新添加一次测试文件；旧版的命令行 flag、TUI、结果表图、Gist/仓库上传、IP 改名、导出配置在本分支已移除。

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

它会构建 `-tags vivahead` 的二进制，用 `VIVA_HEADLESS=1` 走线协议操作界面，校验条目、分组、右键菜单、过滤、预设、结果写入与持久化；不打开窗口。

## 设计文档

- 需求基线：`docs/specs/0001-viva-gui.md`
- 架构决策：`docs/adr/0017-viva-gui.md`
- 术语：`GLOSSARY.md`

## License

[GPL-3.0](LICENSE)。上游项目与协议见 [faceair/clash-speedtest](https://github.com/faceair/clash-speedtest)。
