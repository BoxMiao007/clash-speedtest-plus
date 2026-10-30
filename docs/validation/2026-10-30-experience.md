# 验证记录：体验补齐（2026-10-30）

对应 [ADR-0028](../adr/0028-experience-gaps.md)：全局全选/全不选、清空结果、输出目录浏览、拖放添加条目、关于与帮助、窗口几何记忆、结果图预览、打开配置所在目录、订阅全部重新获取。

## 自动化验证

- `gofmt -l .`：无输出。
- `go vet ./...`：通过。
- `go test ./... -count=1`：全绿；`internal/gui` 57 → **66 个用例**，新增 9 个：
  - `TestSelectAllNoneCoversAllGroups`：两个条目四个节点，取消两个勾选后全选恢复 4、全不选归 0。
  - `TestClearResultsWipesStoreAndRows`：测速后清空，内存节点复位、轮次清空、磁盘结果删除、条目保留；确认框先行。
  - `TestOutputDirectoryBrowseFillsStem`：`QueueOpenDirectory` 驱动「浏览…」，输出路径变成 `<所选目录>/导出.yaml`。
  - `TestDroppedConfigBecomesEntry`：拖入 yaml+txt，只新增 1 个条目、名称取「拖入」、读取 2 个节点、写盘。
  - `TestAboutDialogShowsShortcutHelp`：F1 打开关于，内容含 Ctrl+A/Ctrl+R/Ctrl+Enter 与构建提交；页脚「关于」同样可开。
  - `TestWindowGeometryIsRemembered`：Resize+MoveWindow 后关闭，settings.json 记录 1280×860@(140,90)；重启按同几何打开。
  - `TestSavedSettingsKeepWindowGeometry`：保存设置不清掉窗口字段。
  - `TestPreviewDecodesResultImage`：8×4 PNG 解码为 8×4 位图，纯红像素验证预乘 BGRA 通道序（0,0,255,255）。
  - `TestContextMenuOffersEntryDirectory`：本地条目菜单「打开配置文件所在目录」、订阅条目「打开订阅缓存目录」，`openDirectory` 接缝捕获目标目录。
  - `TestWindowSettingsRoundTrip`：窗口字段含负坐标（副屏）写读一致，重启使用保存几何。
- headless 走查（`scripts/headless-walkthrough.sh`）：`PASS`，`layout` 0 problems。
- 绘制基准（300 节点）：110,799 ns/op，**0 allocs/op**（上轮 98,907 ns/op，新增按钮与订阅行 +12%，无分配回归）。

## 实现要点回顾

- 预览位图必须转成预乘 BGRA（Viva `ui.Bitmap` 约定）：NRGBA 逐像素乘 alpha、RGBA 只换通道序、其余走 `At()` 通用路径；解码在后台 goroutine，`eventPreview` 带 version 防止连续点预览时旧结果覆盖新请求。
- 窗口几何只在 `OnFrameChange` 写内存，保存设置/退出时随 `SetSettings` 落盘；拖动窗口不产生磁盘写入。
- 拖放提示是纯绘制（蒙层 + 提示条），不注册命中，不挡交互。
- 打开目录的 `openDirectory` 是包级变量接缝，测试替换避免真的拉起文件管理器。

## 未覆盖（待真机确认）

- 真实窗口的拖放悬停蒙层视觉、资源管理器打开目录（测试用接缝替换）。
- 高 DPI 下预览对话框的位图缩放清晰度。
- 多显示器负坐标窗口在真实系统上的恢复（测试只验证配置读取）。
- F1 在 Windows 上与系统帮助冲突的行为。
