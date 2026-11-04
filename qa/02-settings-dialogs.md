# 设置视图与外围交互静态审查（task-2）

> 范围：`internal/gui/view_settings.go`、`dialogs.go`、`shortcuts.go`、`window.go`、`preview.go`、`export.go`、`openpath.go`、`scrollbar.go`
> 基线：`docs/specs/0001-viva-gui.md`（8.6 / 8.7）、ADR-0019 / 0024 / 0025 / 0028
> 方法：只读静态审查，未执行 go build / go test / go vet（按任务约束，动态验证由 Lead 负责）。涉及 Viva 框架事件派发语义的条目已标注「待确认」。

## 一、合理的交互点（正面清单）

- **保存前统一校验、失败不写盘**：`view_settings.go:298-315` 先 `collectSettings()` 校验，任何字段非法即 `return`，不会调用 `store.SetSettings`；符合规格 8.6「非法值在保存时就地报错，不写盘」。
- **危险操作有二次确认且运行中禁用**：删除条目 / 删除预设 / 清空结果都走 `dialogConfirm`（`actions.go:974-996,167-182`），「清空结果」按钮在 `running()/exporting` 时 `Disabled`（`view_speedtest.go:184`）。
- **只读数据目录降级清晰**：`noteSaved()`（`state.go:1369-1375`）在只读时给「（数据目录只读，未保存）」后缀，写操作前统一用 `savable()` 守卫（`view_settings.go:309` 等）。
- **窗口几何低 IO**：`OnFrameChange` 只写内存（`window.go:14-17`），保存/退出才落盘；`collectSettings` 显式保留 `WindowWidth/Height/X/Y/Set`（`view_settings.go:222-228`），保存设置不会清掉几何 —— 符合 ADR-0028 第 6 条。
- **拖放只收配置文件**：`window.go:143-151` 仅接受 `.yaml/.yml`，非配置与目录跳过并汇总提示（`window.go:82-89`）。
- **输出目录浏览沿用原文件名**：`view_settings.go:274-295` 保留原 basename，无则用「导出.yaml」，省去手输。
- **预览解码在后台 + 版本号防旧结果覆盖**：`preview.go:37-57` 用 goroutine 解码，`event.version != s.preview.version` 丢弃过期结果，绘制路径无文件 I/O。
- **导出失败有重试入口、手动导出独立序号**：`view_speedtest.go:173-177` + `actions.go:615-630`；`export.go:390` 用 `NextImageSequence` 现取序号，避免与同轮自动产物同名。

## 二、问题清单（按严重度排序）

### [S-01] 配置+结果图同轮导出后，「预览结果图」指向一个不存在的复合路径
- **严重级别**：严重（结果图预览在常见输出配置下不可用）
- **类型**：状态/数据流缺陷（结果路径聚合错误）
- **位置**：`internal/gui/export.go:156`；`internal/gui/actions.go:793-797`；消费方 `internal/gui/view_speedtest.go:178-181`
- **关键代码**：
  - `export.go:156`：`path = output.JoinStatus(path, imagePath)`
  - `actions.go:794-797`：`if event.err == nil && strings.HasSuffix(strings.ToLower(event.path), ".png") { s.lastImage = event.path }`
- **复现步骤**：
  1. 设置页把「输出模式」设为「当前目录下」或「自定义」（会写配置 yaml），保持「每轮自动保存结果图（PNG）」开启（默认开）；
  2. 回测速页测一轮，等自动导出完成（同时写出 `N.xxx.yaml` 与 `N.xxx.png`）；
  3. 点结果条「预览结果图」。
- **期望 vs 实际**：期望预览本轮 PNG；实际 `lastImage` 被赋成 `"...\N.xxx.yaml；...\N.xxx.png"`（`JoinStatus` 用「；」把配置路径与图片路径拼成一个串，恰好以 `.png` 结尾所以骗过了后缀判断），`startImagePreview` 里 `os.Stat` 失败，状态栏显示「结果图不存在：…」。
- **依据**：规格 8.7「结果图：PNG…」+ ADR-0028 第 7 条「预览最近一次成功导出的 PNG」；Nielsen「系统状态可见性」。
- **置信度**：高（代码路径可完整推演；默认 OutputMode=off 时 `path` 为空、恰好正确，所以测试没覆盖到「配置+图」组合）。

### [S-02] 应用级快捷键不区分输入焦点与模态对话框，F1/Ctrl+R/Ctrl+Enter/Ctrl+Shift+A 会在录入或弹窗中误触发
- **严重级别**：严重（F1 会替换正在编辑的对话框，导致表单输入丢失）
- **类型**：快捷键冲突 / 焦点与模态处理
- **位置**：`internal/gui/shortcuts.go:8-28`（onShortcut）、`shortcuts.go:32-54`（onKey）；接线 `app.go:81-82`；`openAbout` 无条件覆盖 `a.dialog`（`dialogs.go:54`）
- **关键代码**：
  - `shortcuts.go:23-25`：`case "r": a.refetchAllSubscriptions()`
  - `shortcuts.go:36-39`：`if event.Key == ui.KeyF1 { a.openAbout() ... }`
  - `shortcuts.go:40-45`：`Ctrl+Enter -> a.startSelectedTest()`
  - 两个函数内均无 `a.dialog != dialogNone` 或「当前焦点是否在输入框」的判断。
- **复现步骤**：
  1. 点「新建条目」打开条目对话框，光标停在「名称」输入框输入中，按 F1 —— 关于对话框替换条目对话框，先前输入的名称/链接丢失（关闭关于后回到测速页，条目对话框不见了）；
  2. 或设置页把光标放进「重命名模板」文本框，按 Ctrl+R —— 后台「重新获取全部订阅」被执行，当前无任何提示；
  3. 或在任意弹窗中按 Ctrl+Enter —— 弹窗背后开始一轮测速。
- **期望 vs 实际**：期望模态对话框/文本输入内只有标准编辑和弦与文本输入生效，全局动作被抑制；实际除 Ctrl+A（文本全选）外，Ctrl+R / Ctrl+Shift+A / Ctrl+Enter / F1 均直接执行全局动作。注释只论证了「标准编辑和弦先被平台文本系统消费」，但这几个不是标准编辑和弦。
- **依据**：Nielsen「用户控制与自由 / 错误预防」；桌面平台惯例「模态对话框独占键盘焦点」；ADR-0028「输入框聚焦时…应用快捷键不与编辑冲突」只覆盖了编辑和弦。
- **置信度**：中高（`shortcuts.go` 无守卫是确定的；Viva 在对话框打开/输入聚焦时是否仍派发 `OnShortcut/OnKey` 需动态确认，见待确认项 Q-2）。

### [S-03] 「清空结果」确认框的确认按钮文案是「删除」
- **严重级别**：一般（误导性文案，非阻断）
- **类型**：文案 / 一致性
- **位置**：`internal/gui/dialogs.go:206`；触发 `actions.go:176-181`
- **关键代码**：`f.Button(a.ids.confirmOK, "删除").Destructive().Clicked()`（三种 confirmKind 共用同一按钮）
- **复现步骤**：有测速结果 → 点结果条「清空结果」→ 弹窗标题「清空测速结果」、正文「清空所有节点的测速结果…此操作无法撤销」，但底部确认按钮写「删除」。
- **期望 vs 实际**：期望按钮为「清空」（与标题/正文/实际动作一致）；实际「删除」。
- **依据**：Nielsen「系统与真实世界匹配」「一致性」；危险操作文案应准确描述后果。
- **置信度**：高（`paintConfirmDialog` 硬编码标签）。

### [S-04] 设置校验只有顶部一条横幅，不定位到具体字段，长错误还被省略号截断
- **严重级别**：一般（体验受损，定位成本高）
- **类型**：表单校验反馈 / 错误定位
- **位置**：`internal/gui/view_settings.go:337-341`；校验逻辑 `view_settings.go:114-230`
- **关键代码**：`f.Text(a.settingsForm.err).FontSize(11).Color(a.theme.Danger).Ellipsis()`，且该行固定在滚动区之上；`collectSettings` 只返回第一条错误文本。
- **复现步骤**：滚到设置页底部，在「仓库地址」只填地址、不填 token（或把输出模式设为自定义但清空输出路径），点「保存设置」→ 顶部出现一行红字，滚动位置仍在底部，出错的字段本身没有红框/行内提示；若错误文本较长（如重命名模板解析错误），整行被省略号截断。
- **期望 vs 实际**：规格 8.6 要求「非法值在保存时**就地**报错」，期望错误出现在对应字段旁并高亮；实际只有顶部单行横幅，不指向字段。
- **依据**：规格 8.6；Nielsen「帮助用户识别、诊断并从错误中恢复」。
- **置信度**：中高。

### [S-05] 数值输入缺少上界与 NaN/Inf 防护，可导致整型/时长溢出或非有限值进入设置
- **严重级别**：一般（可产生错误测速或诡异保存失败）
- **类型**：数值边界 / 输入校验
- **位置**：`internal/gui/view_settings.go:232-263`（parseIntField / parseFloatField）；`internal/store/store.go:175-242`（Normalize 只处理下界）；折算 `internal/gui/engine.go:104-111`
- **关键代码**：
  - `view_settings.go:238-240`：只判 `value < minimum`，无最大值；
  - `view_settings.go:246-252`：`ParseFloat` 后只判 `value < 0`（`NaN`/变量 `Inf` 的比较为假，直接通过）；
  - `engine.go:104-105`：`DownloadSize: settings.DownloadMB * megabyte`、`Timeout: time.Duration(settings.TimeoutMS) * time.Millisecond`。
- **复现步骤**：
  1. 「下载大小（MB）」填 `10000000000000`（1e13）→ `DownloadMB*megabyte` 溢出为负数，下载逻辑行为异常；
  2. 「超时（毫秒）」填 `999999999999999` → `time.Duration(ms)*time.Millisecond` 溢出为负，请求立即超时；
  3. 「最低下载（MB/s）」填 `NaN` 或 `Inf` → 通过校验；可写目录下 `SetSettings` 因 `json: unsupported value: NaN` 失败并报「保存设置失败：…」（错误信息对用户无意义），只读目录下 NaN 留在内存使阈值比较恒为 false。
- **期望 vs 实际**：期望拒绝超出合理范围及非有限值并在字段旁提示；实际仅拦下界，NaN/Inf 与超大值放行。
- **依据**：桌面表单数值边界惯例（输入即约束）；Nielsen「错误预防」；规格 8.6 未声明上限但隐含「可用的参数」。
- **置信度**：中高（`strconv.ParseFloat` 接受 `NaN/Inf`、Go 常量乘法溢出均为确定行为）。

### [S-06] 自定义输出路径保存时不校验目录存在/可写，也不区分「填的是目录」
- **严重级别**：一般（错误延后到后台导出才暴露）
- **类型**：路径校验 / 延迟报错
- **位置**：`internal/gui/view_settings.go:159-163`（仅判非空）；`internal/gui/export.go:53-70`（roundExportPath）
- **关键代码**：`if outputMode == store.OutputModeCustom && outputPath == "" { return ... "必须填输出路径" }` —— 只判空。
- **复现步骤**：
  1. 输出模式选「自定义」，路径填 `Z:/不存在的目录/导出.yaml` → 保存成功 → 测完自动导出失败（状态「导出失败：…」），需手动改设置再「重试导出」；
  2. 路径填一个目录 `D:/订阅`（无扩展名）→ 保存成功 → 导出时 `abs += ".yaml"` 得到同级的 `D:/订阅.yaml`，而非目录内文件，用户预期落空。
- **期望 vs 实际**：期望保存时校验父目录存在/可写、或明确要求文件名；实际仅非空校验。
- **依据**：规格 8.6（输出路径）、8.7（输出模式命名规则）；Nielsen「错误预防」。
- **置信度**：中。

### [S-07] 重命名模板只做 Parse 校验，字段名写错在导出时静默回退内置格式
- **严重级别**：一般（用户自定义被无声丢弃）
- **类型**：模板校验不完整 / 静默失败
- **位置**：`internal/gui/view_settings.go:171-176`（只 Parse）；`internal/ip/rename.go:47-61`（Execute 失败回退默认）
- **关键代码**：
  - `view_settings.go:173`：`if _, err := template.New("rename").Parse(renameTemplate); err != nil {...}`
  - `rename.go:57-60`：Execute 出错时 `return fmt.Sprintf(...内置格式...), nil`（吞掉错误）
- **复现步骤**：设置页「重命名模板」填 `{{.Foo}}`（合法语法、未知字段）→ 保存成功 → 导出时 Execute 报错被吞、静默套用内置格式，界面无任何提示。
- **期望 vs 实际**：期望保存时（或首次导出时）提示模板字段无效；实际校验通过、运行时静默回退。
- **依据**：规格 8.6 只要求「可解析」，但用户视角是「我设的模板没生效」；Nielsen「系统状态可见性」。
- **置信度**：中高。

### [S-08] 滚动条拖动依赖 `PointerOn` 区域，指针横向移出 10px 轨道后可能停止跟随（无显式指针捕获）
- **严重级别**：一般（长列表拖动体验不稳；需动态确认）
- **类型**：滚动条拖动/命中
- **位置**：`internal/gui/scrollbar.go:76-105`
- **关键代码**：
  - `scrollbar.go:76-79`：`event, ok := canvas.PointerOn(id); if !ok { return offset, false }`
  - `dragging` 仍为 true，但因拿不到事件不再更新偏移。
- **复现步骤**：按住节点列表/设置表单滚动条滑块快速下拖，指针稍微偏出轨道右缘（轨道仅约 10px 宽）→ 滑块停止跟随指针，直到指针回到轨道内。
- **期望 vs 实际**：期望按下后捕获指针、移出轨道继续跟随（常见桌面滚动条行为）；实际移出即停更。
- **依据**：桌面平台滚动条规范（thumb drag 需 pointer capture）；ADR-0019 第 3 条。
- **置信度**：低-中（取决于 Viva `PointerOn` 是否隐式保持按下期间的指针路由，见待确认项 Q-1）。

### [S-09] 关闭结果图预览不释放位图，与代码注释不符
- **严重级别**：轻微（大图内存常驻到下次预览）
- **类型**：资源管理
- **位置**：`internal/gui/preview.go:107-111`
- **关键代码**：`func (s *state) closePreview() { a.preview.open = false; a.dialog = dialogNone }` —— 注释写「丢掉位图引用」，但未清 `a.preview.bitmap`。
- **复现步骤**：打开一张大结果图预览 → 点「关闭」→ `a.preview.bitmap` 仍持有整张预乘 BGRA 像素（直到下一次 `startImagePreview` 重置结构体）。
- **期望 vs 实际**：期望关闭即释放；实际保留引用。
- **依据**：规格硬性规范 #2「空闲内存」；注释与实现一致性。
- **置信度**：高（行为确定，影响小）。

### [S-10] 保存设置写盘失败时，内存设置已被替换但列表未重建/排序未复位
- **严重级别**：轻微（状态三方不一致）
- **类型**：状态流转 / 失败回滚
- **位置**：`internal/gui/view_settings.go:307-315`
- **关键代码**：`s.settings = settings` 先于 `store.SetSettings`；失败时 `return`，跳过 `resetSort/refreshSortHeaders/refreshModeFlags/rebuild`。
- **复现步骤**：制造 `SetSettings` 失败（只读盘/权限/磁盘满）→ 保存 → 状态「保存设置失败：…」，但 `s.settings` 已是新值（如改成了完整模式），而排序箭头/上传列等仍是旧模式口径，重启后又回旧值。
- **期望 vs 实际**：期望失败时回滚内存或至少同步重建界面；实际部分生效。
- **依据**：Nielsen「系统状态可见性」；状态一致性。
- **置信度**：中。

### [S-11] 「恢复默认」无二次确认，直接覆盖未保存的输入缓冲
- **严重级别**：轻微（误点丢失未保存输入）
- **类型**：危险/破坏性操作确认
- **位置**：`internal/gui/view_settings.go:330-332`（按钮）、`view_settings.go:266-270`（resetSettingsForm）
- **关键代码**：`f.Button(a.ids.settingsReset, "恢复默认").Small().Outline().Clicked() { a.resetSettingsForm() }`，内部 `settingsForm.load(store.DefaultSettings())` 覆盖全部字段（含输出路径、Gist/仓库 token）。
- **复现步骤**：设置页填好 token 与输出路径但未保存 → 点「恢复默认」→ 全部缓冲被默认值覆盖，原输入丢失（仅状态栏提示「已填入默认参数，点保存生效」）。
- **期望 vs 实际**：期望恢复默认前确认或可撤销；实际直接覆盖。
- **依据**：Nielsen「用户控制与自由 / 错误预防」。
- **置信度**：高（行为确定）。

### [S-12] 拖放遮罩在 Windows 永不显示，且松手后同步解析多个大文件会卡界面、无按路径去重
- **严重级别**：轻微（平台已知限制 + 潜在卡顿）
- **类型**：拖放反馈 / 阻塞 I/O
- **位置**：`internal/gui/window.go:35-56`（Over/Leave）、`window.go:60-90`（addDroppedFiles 同步 readEntry）
- **关键代码**：`case ui.FileDropOver:` 才置 `dropActive=true`；Windows 只报 `FileDropDrop`（ADR-0028 已注明），故 `paintDropHint`（`window.go:155-173`）实际是死路径；`addDroppedFiles` 逐文件 `os.Stat` + `addLocalEntry → readEntry` 全在窗口线程同步执行。
- **复现步骤**：Windows 下从资源管理器拖入多个大 YAML → 拖动过程中无「松开鼠标…」蒙层；松手后同步解析，界面短暂无响应；同一文件重复拖入可生成重复条目（`addDroppedFiles` 不按路径去重）。
- **期望 vs 实际**：期望有拖入反馈且解析不阻塞 UI；实际无蒙层、同步 IO、可重复添加。
- **依据**：规格硬性规范 #7（阻塞操作后台）+ #2（空闲不绘制）；Nielsen「系统状态可见性」。
- **置信度**：中（Windows 只报 drop 已在 ADR-0028 明确，属已知取舍；同步 IO 为代码事实）。

## 三、功能缺失清单

| 缺失功能 | 影响范围 | 依据/说明 |
| --- | --- | --- |
| 设置页无「取消/撤销」与「未保存改动」提示 | 用户改完字段切走再回来，看不到哪些改动未落盘，容易误以为已生效 | 规格 8.6 未要求，但违反「系统状态可见性」；当前只有顶部「保存后下一轮测速生效」静态说明 |
| 无字段级校验反馈（inline error / 字段高亮 / 自动滚动到出错字段） | 见 S-04，长表单底部字段出错时定位困难 | 规格 8.6「就地报错」未真正落实 |
| 无输入长度与数值上界校验 | 见 S-05，可导致溢出/资源耗尽/保存失败 | 桌面表单惯例 |
| 自定义输出路径无目录存在性/可写性校验 | 见 S-06，错误延后到后台导出 | — |
| 重命名模板无字段级校验/效果预览 | 见 S-07，模板静默失效 | — |
| 预览对话框无缩放拖拽/滚轮/另存为/失败重试 | 大图「原始大小」只能看局部，解码失败只能关闭重开 | ADR-0028 明确「只做两档切换」为取舍；列为已知缺失 |
| 导出失败只有状态栏一行文案，无错误详情对话框 | 长错误被省略号截断，难定位（如路径/权限/上传细节） | 与 S-04 同源 |
| 窗口最大化状态未记忆（只记尺寸/位置） | 最大化退出后下次以「最大化尺寸的普通窗口」打开 | ADR-0028 第 6 条只记几何；待确认是否产品意图 |
| 拖放无按路径去重/无撤销 | 见 S-12，重复条目需手动删除 | ADR-0028「误拖可直接删除」为取舍 |

## 四、待确认项

- **Q-1（S-08）指针捕获**：Viva `canvas.PointerOn(id)` 在指针按下后移出命中区域时是否仍派发 `PointerMove/Up`？若无隐式捕获，滚动条拖动会在移出轨道后中断。需动态验证。
- **Q-2（S-02）事件派发时机**：`app.Config.OnShortcut / OnKey` 在模态 `Dialog` 打开、以及文本框聚焦时是否仍被调用？`shortcuts.go` 注释假设「标准编辑和弦先被平台消费」，但未验证非编辑和弦（Ctrl+R/F1/Ctrl+Enter/Ctrl+Shift+A）。
- **Q-3（窗口几何）** `OnFrameChange` 回调的 `ui.Rect` 是内容区还是外框？最大化/最小化时回调值是否异常（如 0 或屏幕尺寸）？若最小化时保存会把 `WindowSet` 冲掉（Normalize 对 <800 尺寸清零）。
- **Q-4（S-01 关联）** `lastImage` 的复合串是否还有其它消费方（除「预览结果图」外）？目前只搜到 `view_speedtest.go:178-181`。
- **Q-5（S-11）** 「恢复默认」覆盖全部缓冲（含口令/路径）是否为产品有意设计（例如视作「重置全部设置」）？若仅为重置测速参数，应拆分。

## 备注

- 本次为静态审查：所有「行为」结论均来自源码路径推导；标「待确认」的条目需在真实 Viva 窗口下复核（尤其 S-02、S-08、Q-3）。
- 未修改任何产品代码，未运行构建/测试命令；仅新增本报告 `qa/02-settings-dialogs.md`。
