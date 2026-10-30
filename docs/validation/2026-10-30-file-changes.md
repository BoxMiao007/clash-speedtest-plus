# 文件变更清单：GUI 补齐 TUI 能力（2026-10-30）

> 范围：`550ebab..9161bcc`（第一版 GUI 文档收尾 → 本次功能补齐与文档更新）。
> 由 `git diff --name-status 550ebab 9161bcc` 生成；本记录文件自身与之后的提交不在清单内。
> 测试用 exe 用 `*.exe` 忽略，不随仓库分发。

统计：新增 34 个、修改 37 个、删除 0 个、重命名 0 个。

## 新增（A）

- `docs/adr/0018-tui-parity.md`
- `docs/validation/2026-10-30-tui-parity.md`
- `internal/export/export.go`
- `internal/export/export_test.go`
- `internal/gist/gist.go`
- `internal/gist/gist_test.go`
- `internal/gist/repo.go`
- `internal/gist/repo_test.go`
- `internal/gui/export.go`
- `internal/gui/parity_test.go`
- `internal/gui/view_settings.go`
- `internal/ip/location.go`
- `internal/ip/rename.go`
- `internal/ip/rename_test.go`
- `internal/output/flags/cn.png`
- `internal/output/flags/de.png`
- `internal/output/flags/fr.png`
- `internal/output/flags/gb.png`
- `internal/output/flags/hk.png`
- `internal/output/flags/jp.png`
- `internal/output/flags/kr.png`
- `internal/output/flags/sg.png`
- `internal/output/flags/tw.png`
- `internal/output/flags/us.png`
- `internal/output/format.go`
- `internal/output/format_test.go`
- `internal/output/image_sequence_test.go`
- `internal/output/image_speed_test.go`
- `internal/output/resultimage.go`
- `internal/output/resultimage_test.go`
- `internal/output/tsv.go`
- `internal/output/tsv_test.go`
- `internal/speedtester/subscription_fetch.go`
- `internal/speedtester/subscription_fetch_test.go`

## 修改（M）

- `.gitattributes`
- `.gitignore`
- `AGENTS.md`
- `GLOSSARY.md`
- `README.md`
- `docs/adr/0001-parallel-vs-concurrent.md`
- `docs/adr/0002-quit-kills-process.md`
- `docs/adr/0003-auto-export-result-image.md`
- `docs/adr/0005-subscription-normalization.md`
- `docs/adr/0006-subscription-fetch-user-agent.md`
- `docs/adr/0007-two-level-ui-esc-returns-to-picker.md`
- `docs/adr/0008-image-metric-scale-per-column.md`
- `docs/adr/0010-multi-source-runs.md`
- `docs/adr/0011-artifacts-follow-source-name.md`
- `docs/adr/0012-image-sequence-numbering.md`
- `docs/adr/0013-name-from-config-always-on.md`
- `docs/adr/0014-merged-export-file.md`
- `docs/adr/0015-single-tui-program-for-queue.md`
- `docs/adr/0016-export-full-profile.md`
- `docs/adr/0017-viva-gui.md`
- `docs/specs/0001-viva-gui.md`
- `docs/validation/headless-walkthrough.log`
- `go.mod`
- `go.sum`
- `internal/gui/actions.go`
- `internal/gui/app.go`
- `internal/gui/app_test.go`
- `internal/gui/dialogs.go`
- `internal/gui/engine.go`
- `internal/gui/format.go`
- `internal/gui/ids.go`
- `internal/gui/state.go`
- `internal/gui/view_nodes.go`
- `internal/gui/view_results.go`
- `internal/store/store.go`
- `internal/store/store_test.go`
- `scripts/headless-walkthrough.sh`

## 说明

- 恢复的旧能力：`internal/output`、`internal/ip`、`internal/gist` 从 `5411565` 取回并改导入到 `internal/speedtester`；新增 `internal/export` 统一导出骨架与阈值过滤。
- GUI 新代码：`internal/gui/export.go`（轮次导出编排）、`view_settings.go`（设置表单与校验）、`parity_test.go`（10 个新 Driver 用例）。
- 数据与引擎：`internal/store` 增加条目种类、订阅缓存、版本 2 设置；`internal/speedtester` 增加订阅拉取与正文解析。
- 文档：规格重写、ADR-0018、ADR-0017 修订说明、相关 ADR 状态更新、README/GLOSSARY/AGENTS、验证记录与 headless 日志。
- 依赖：`go.mod`/`go.sum` 新增 `golang.org/x/image`（结果图渲染）。
