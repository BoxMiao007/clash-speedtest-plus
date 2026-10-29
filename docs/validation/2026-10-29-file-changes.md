# Viva GUI 重构：交付文件变更记录

> 生成范围：`5411565..c35a776`（实现提交 + 首次文档提交）。之后的提交只继续补文档（本文件、ADR 状态注记、规格状态与验收勾选），不再改代码。
> 状态字母：A 新增、M 修改、D 删除、R 重命名（R100 内容不变，R088 有修改）。

## 统计

         29 A
         68 D
         21 M
          1 R088
         11 R100

## 完整清单

```text
A	.gitattributes
M	.gitignore
M	.goreleaser.yaml
M	.goreleaser.yml
M	AGENTS.md
M	GLOSSARY.md
M	README.md
M	docs/adr/0002-quit-kills-process.md
M	docs/adr/0003-auto-export-result-image.md
M	docs/adr/0007-two-level-ui-esc-returns-to-picker.md
M	docs/adr/0008-image-metric-scale-per-column.md
M	docs/adr/0010-multi-source-runs.md
M	docs/adr/0011-artifacts-follow-source-name.md
M	docs/adr/0012-image-sequence-numbering.md
M	docs/adr/0013-name-from-config-always-on.md
M	docs/adr/0014-merged-export-file.md
M	docs/adr/0015-single-tui-program-for-queue.md
M	docs/adr/0016-export-full-profile.md
A	docs/adr/0017-viva-gui.md
A	docs/notes/viva-ui-api.md
A	docs/specs/0001-viva-gui.md
A	docs/validation/2026-10-29-viva-gui.md
A	docs/validation/headless-walkthrough.log
M	download-server/download-server.go
D	gist/gist.go
D	gist/gist_test.go
D	gist/repo.go
D	gist/repo_test.go
M	go.mod
M	go.sum
A	internal/filter/filter.go
A	internal/filter/filter_test.go
A	internal/gui/actions.go
A	internal/gui/app.go
A	internal/gui/app_test.go
A	internal/gui/contextmenu.go
A	internal/gui/dialogs.go
A	internal/gui/engine.go
A	internal/gui/format.go
A	internal/gui/ids.go
A	internal/gui/state.go
A	internal/gui/view_nodes.go
A	internal/gui/view_results.go
R100	speedtester/mode.go	internal/speedtester/mode.go
R100	speedtester/mode_test.go	internal/speedtester/mode_test.go
A	internal/speedtester/nodes.go
A	internal/speedtester/nodes_test.go
R100	speedtester/progress.go	internal/speedtester/progress.go
R100	speedtester/progress_test.go	internal/speedtester/progress_test.go
R100	speedtester/scheduler.go	internal/speedtester/scheduler.go
R100	speedtester/scheduler_test.go	internal/speedtester/scheduler_test.go
R100	speedtester/server_test.go	internal/speedtester/server_test.go
R088	speedtester/speedtester.go	internal/speedtester/speedtester.go
R100	speedtester/speedtester_test.go	internal/speedtester/speedtester_test.go
R100	speedtester/subscription.go	internal/speedtester/subscription.go
R100	speedtester/subscription_test.go	internal/speedtester/subscription_test.go
A	internal/speedtester/testdata/broken.yaml
A	internal/speedtester/testdata/duplicate_names.yaml
A	internal/speedtester/testdata/empty.yaml
A	internal/speedtester/testdata/nodes.yaml
R100	speedtester/zeroreader.go	internal/speedtester/zeroreader.go
A	internal/store/store.go
A	internal/store/store_test.go
D	ip/location.go
D	ip/rename.go
D	ip/rename_test.go
M	main.go
A	main_headless.go
D	main_test.go
D	output/flags/cn.png
D	output/flags/de.png
D	output/flags/fr.png
D	output/flags/gb.png
D	output/flags/hk.png
D	output/flags/jp.png
D	output/flags/kr.png
D	output/flags/sg.png
D	output/flags/tw.png
D	output/flags/us.png
D	output/format.go
D	output/format_test.go
D	output/image_sequence_test.go
D	output/image_speed_test.go
D	output/mode.go
D	output/mode_test.go
D	output/resultimage.go
D	output/resultimage_test.go
D	output/tsv.go
D	output/tsv_test.go
D	picker/configs.go
D	picker/configs_test.go
D	picker/fetch.go
D	picker/fetch_test.go
D	picker/model.go
D	picker/model_test.go
D	picker/options.go
D	picker/options_test.go
D	picker/outputmode_test.go
D	picker/paths.go
D	picker/paths_test.go
D	picker/sources.go
D	picker/sources_test.go
D	picker/styles.go
D	picker/view.go
D	result_filter.go
D	result_filter_test.go
A	scripts/headless-walkthrough.sh
D	tui/click_matrix_test.go
D	tui/detail.go
D	tui/detail_scroll_test.go
D	tui/detail_test.go
D	tui/help.go
D	tui/help_test.go
D	tui/image_speed_test.go
D	tui/inflight_text_test.go
D	tui/layout.go
D	tui/model.go
D	tui/model_update_test.go
D	tui/perf.go
D	tui/perf_test.go
D	tui/queue.go
D	tui/queue_switch_test.go
D	tui/queue_test.go
D	tui/quit_test.go
D	tui/rounds_test.go
D	tui/scroll_test.go
D	tui/sort.go
D	tui/sort_test.go
D	tui/table.go
D	tui/table_test.go
```
