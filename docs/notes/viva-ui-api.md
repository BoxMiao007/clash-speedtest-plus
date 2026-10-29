# Viva v0.2.0 GUI 交互 API 验证笔记（spike）

> 对象：`github.com/vivaui/viva v0.2.0`（源码 `C:/Users/BOXMIAO/go/pkg/mod/github.com/vivaui/viva@v0.2.0`）。
> 规格：`docs/specs/0001-viva-gui.md` 第 3、5、7、8、10 节。
> 环境：Windows 11 10.0.26300.9711、Go 1.27.3 windows/amd64、16 逻辑核；`GOPROXY=https://goproxy.cn,direct`。
> spike：`/tmp/viva-spike`（独立 module，`require github.com/vivaui/viva v0.2.0`；代码不复制进仓库）。
> 结论采集：2026-10-29。本文件是本次任务唯一写入仓库的文件。

## 0. 结论摘要

| # | 交互点 | v0.2.0 结论 | spike 测试 |
|---|---|---|---|
| 1 | 右键菜单 | `Box.Hit` + `canvas.PointerOn` + `components.BeginContextMenu` 可用；菜单项用 `canvas.Clicked(item.ID)` 判定；点外/Esc 靠 `DismissID`+`Viewport`；Driver `ClickButton(..., PointerButtonSecondary)` 能触发 | `TestContextMenu*`、`TestRowHit*`、`TestRightClickOnRowChild*` |
| 2 | 重排稳定身份 | 加载时 `tree.Sub` + `tree.ID` 预计算，帧间与重排后 ID 不变；点击新位置命中正确节点；`ui.Tree` 无注销且非并发安全 | `TestRowIDsSurviveReorderAndClicksFollowTheNode`、`TestNewRowGetsItsOwnIdentity` |
| 3 | 对话框 + 文本 | `flex.Dialog` + `flex.Field` + `Config.TextInput = flex.Fields(&field)` 可用；`Driver.Type`、Enter→`Submitted`、Esc/遮罩/关闭按钮→`Dismissed` 全部走通；`InitialFocusID` 可省一次点击 | `TestDialog*`、`TestTypeWithoutTextInput*` |
| 4 | 下拉预设 | `flex.Select(scope,&state,labels,selected).Picked()` 只在该选项的点击事件帧返回一次；触发项/选项/点外/Esc 的 ID 由 scope 固定分配 | `TestSelectPickedTiming` |
| 5 | 分段控件 | `flex.Choice(scope,labels,selected).ChosenIndex()` 可用 | `TestSegmentedChosenIndex` |
| 6 | 滚动列表 | `flex.ScrollArea(id)` + `Config.OnScroll = view.Wheel` + `Driver.Scroll` 可用；滚轮位移 150px、越界钳制、尺寸变化后钳制均验证 | `TestScrollArea*` |
| 7 | 窗口外壳/分配 | Header(固定) / Content(`Grow(1)`) / Footer(固定) + `Resize` 正常；复用 `flex.View` 的暖机声明 0 allocs/op，Driver 整帧重绘 0 allocs/op（测量口径见 §7） | `TestShell*`、`BenchmarkShell*` |
| 8 | headless | `-tags vivahead` + `import _ ".../headless/auto"` + `VIVA_HEADLESS=1` 可走通 `click/texts/rightclick/clip/layout/quit`；非截图命令不创建 D3D 渲染器；`shot` 才需要 Direct3D 11 | 手工走查，见 §8 |

`go test ./... -count=1`：21 个测试全绿；`go vet ./...` 通过（输出见 §11）。

补充：本仓库的节点/结果列表用 `Stack().Clip()` + `Absolute().Top()` 自己虚拟化，而不是 `ScrollArea`；
这组 API 的裁剪行为也已实测，见 §7 的补充小节。两种方案不要混用：`View.Wheel` 只认 `ScrollArea`。

---

## 1. 右键菜单：行 Hit + PointerOn + BeginContextMenu

### 结论

- 行用 `f.Row()...Hit(rowID, label)` 声明命中；在同一个 Paint 里读 `canvas.PointerOn(rowID)`。
  只有 **`event.Kind == ui.PointerDown` 且 `event.Button == ui.PointerButtonSecondary`** 才是右键按下。
  `PointerOn` 对所有按键都投递、且**一次点击投递 press+release 两个事件**（spike 实测一个 `Driver.Click` 让同一个
  `PointerOn` 命中两次），所以必须同时过滤 `Kind` 与 `Button`，否则释放时也会再开一次菜单。
- 菜单用 `components.BeginContextMenu(canvas, theme, bounds, options)` 画，`options` 的 `Scope`/`DismissID`
  在 v0.2.0 是 **`ui.ID`（不是 `ui.Scope`）**；`Viewport` 传**整个窗口矩形**。
- 菜单项点击判定：在声明 `menu.AddItem(item)` 的同一个 Paint 里 `canvas.Clicked(item.ID)`。它和普通控件一样，
  只在点击的事件帧为真。命中后自行 `m.menu = m.menu.Dismiss()`（或 `Close()`）。
- 点外关闭：`Viewport` 非零时菜单会在窗口背面声明一个 `DismissID` 命中；点它 `canvas.Clicked(dismissID)` 为真。
  Escape 由运行时路由到同一个 `DismissID`（`BeginContextMenu` 内部 `SetEscapeTarget`），同样用 `Clicked` 读。
- `Viewport` 同时负责把菜单夹在窗口内（贴边时平移/裁短），所以不要传行矩形或局部矩形。
- 单层平铺：`ContextMenu` 自身只有一层；`ContextMenuOptions.Submenu` 只用于“某个子菜单的内容”，没有自动展开子菜单的 API。
- Driver：`driver.ClickButton(x, y, ui.PointerButtonSecondary)` 就能打开；之后用普通的 `driver.Click` 点菜单项。

### 最小片段（v0.2.0）

```go
import (
	"github.com/vivaui/viva/components" // BeginContextMenu / ContextMenuItem
	"github.com/vivaui/viva/flex"
	"github.com/vivaui/viva/ui"
)
```

```go
// 包级（只声明一次）：菜单的身份与项都在一组稳定的 scope 里。
var (
	menuScope = tree.ID(menuSub, "scope")   // ui.ID
	dismissID = tree.ID(menuSub, "dismiss") // ui.ID
	copyID    = tree.ID(menuSub, "copy")
	testID    = tree.ID(menuSub, "test")
)

// 右键项在“状态变化”时重建，不在 Paint 里做 append/字符串拼接。
func (m *model) rebuildMenu() {
	m.menuItems = append(m.menuItems[:0],
		components.ContextMenuItem{ID: copyID, Label: "复制此节点信息"},
		components.ContextMenuItem{ID: testID, Label: "立即测试此节点", Disabled: m.running},
	)
}

// Paint 内：行 + 右键按下。
for index := range m.rows {
	row := &m.rows[index]
	line := f.Row().Height(32).FillWidth().Gap(8).Pad(4).Hit(row.rowID, row.label)
	line.Children(func() {
		if f.Checkbox(row.checkID, row.checked).Clicked() {
			row.checked = !row.checked
		}
		f.Text(row.label).Grow(1)
	})
	if event, ok := canvas.PointerOn(row.rowID); ok &&
		event.Kind == ui.PointerDown && event.Button == ui.PointerButtonSecondary {
		m.menu = m.menu.Open()
		m.menuAt = ui.Point{X: event.X, Y: event.Y}
		m.menuFor = row.label
		canvas.Consume()
	}
}
```

```go
// Paint 内：菜单本身（m.menu 是 components.ContextMenuState，零值关闭）。
if m.menu.IsOpen() {
	// 高度 = 上下 4px 内边距 + 每一项的实际行高；少了会被静默裁掉，多了会空一块。
	height := float32(8)
	for _, item := range m.menuItems {
		height += components.ContextMenuItemHeight(theme, item, 0)
	}
	menu := components.BeginContextMenu(canvas, theme,
		ui.Rect{X: m.menuAt.X, Y: m.menuAt.Y, Width: 200, Height: height},
		components.ContextMenuOptions{
			Layer:     1,         // 页面是 0；对话框在 2；菜单必须在它要盖住的层之上
			Scope:     menuScope, // ui.ID
			DismissID: dismissID, // ui.ID；点外 + Esc 都报它
			Viewport:  viewport,  // 整个窗口：ui.Rect{Width: size.Width, Height: size.Height}
		})
	for _, item := range m.menuItems {
		menu.AddItem(item)
		if canvas.Clicked(item.ID) {
			m.onMenu(item.ID) // item.ID 即动作；multi/single 文案在 rebuildMenu 里决定
			m.menu = m.menu.Dismiss()
		}
	}
	if canvas.Clicked(dismissID) {
		m.menu = m.menu.Dismiss()
	}
	menu.End()
}
```

Driver 测试写法（`app.Driver`）：

```go
var canvas ui.Canvas
driver, err := app.NewDriver(config, &canvas) // config.Tree 必须指向声明 ID 的 tree
if err != nil { t.Fatal(err) }
defer driver.Close()

hit := /* 在 driver.Snapshot().Hits 中按 ID 找到 */
x, y := hit.Bounds.X+hit.Bounds.Width/2, hit.Bounds.Y+hit.Bounds.Height/2
_ = driver.ClickButton(x, y, ui.PointerButtonSecondary) // 打开
_ = driver.Click(itemX, itemY)                          // 菜单项
_ = driver.Key(ui.KeyEscape, 0)                         // 关闭
```

### 坑

1. **只认 PointerDown**：右键释放也会投递到目标，条件必须包含 `Kind == ui.PointerDown`。
2. **菜单高度是调用方算的**：`bounds.Height` 不够时，超出部分既不会画也不会产生 hit，且**不报错**。高度用
   `components.ContextMenuItemHeight(theme, item, 0)` 累加 + 8px 内边距；有 `Description` 的项更高。
3. **`Viewport` 传整个窗口**：它既是点外关闭区，也是夹边约束；传行矩形会导致点外关不掉、菜单被错误裁剪。
4. **菜单表面是 `Block`**：右键落在已打开的菜单表面上不会穿透，也不会移动菜单；`PassSecondary` 的“一次右键换目标”
   只有按在**菜单外的背板**上才成立。
5. **子控件会吞掉行的 PointerOn**：行内 `flex.Checkbox`（以及任何带 ID 的子控件）是更靠前的 hit，右键按在
   复选框上时 `canvas.PointerOn(rowID)` 为 false。必须在同一 Paint 里也处理子控件的 `PointerOn(checkID)`，
   映射回该行的菜单；spike 里验证过（`TestRowHitDoesNotSeePressesOnItsOwnChildControl`）。
6. **Layer 选择**：页面 0；`flex.Dialog` 在默认层 +2；`f.Select` 的弹层在 +1。要让右键菜单盖住某层，`Layer` 必须更大。
7. **`Scope`/`DismissID` 是 `ui.ID`**：`tree.Sub` 返回 `ui.Scope`，不能直接塞进来；用 `tree.ID(sub, "scope")` 或 `sub.ID(tree.Name("scope"))`。
8. **禁用项**：`Disabled: true` 保留行但路由不投递点击，菜单也不会因此关闭（可直接用于“测速中立即测试置灰”）。
9. **官网文档差异**：<https://vivaui.dev/en/docs/overlays/> 的菜单章节用 `f.Popup()+Hit` 手写，未提
   `components.BeginContextMenu`；v0.2.0 源码里该 API 存在且行为如本笔记（`components/context_menu.go`）。
   规格 8.4 要求的正是后者，按源码写。

---

## 2. 列表实时重排：`ui.Tree` 稳定身份

### 结论

- 给节点行建身份的正确时机是**数据读取/状态变化**，不是 Paint：
  `scope := tree.Sub(listScope, tree.Name("node:"+stableKey))`，`rowID := tree.ID(scope, "row")`，
  把 `rowID` 存进节点模型；Paint 只读 `rowID`。
- 行 ID 与显示顺序无关：重排（`sort.SliceStable` 等，在状态变化路径做）后 `Snapshot().Hits` 中同一节点的
  `hit.ID` 不变，只是 `Bounds` 变了；点击新位置命中的是同一节点。
- `tree.Name` 会缓存词、`tree.Sub` 会缓存 scope，**重复调用是幂等的**；但 `Tree` 没有任何注销接口。
- `ui.Tree` 不是并发安全的：`Sub/Name/ID` 只能在窗口线程调用。后台读文件/引擎回调里不能碰 Tree。

### 最小片段（v0.2.0）

```go
import (
	"sort"

	"github.com/vivaui/viva/flex"
	"github.com/vivaui/viva/ui"
)

var (
	tree      = ui.Tree{}
	page      = tree.Feature("nodes")
	listScope = tree.Sub(page, tree.Name("rows"))
)

type nodeRow struct {
	key   string // 规格 5 的稳定身份（sha256 前 16 位 hex，含 #2/#3 后缀）
	label string
	rowID ui.ID
	boxID ui.ID
}

// 数据加载路径（窗口线程）：每个节点只注册一次身份。
func newNodeRow(key, label string) nodeRow {
	scope := tree.Sub(listScope, tree.Name("node:"+key))
	return nodeRow{key: key, label: label, rowID: tree.ID(scope, "row"), boxID: tree.ID(scope, "box")}
}

// 排序同样是状态变化：改模型切片，不动 ID。
func (m *model) sortBySpeed() {
	sort.SliceStable(m.rows, func(i, j int) bool { return m.rows[i].speed > m.rows[j].speed })
}
```

```go
// Paint 内：只遍历模型、只读预计算 ID。
for index := range m.rows {
	row := &m.rows[index]
	line := f.Row().Height(30).FillWidth().Hit(row.rowID, row.label)
	line.Children(func() {
		f.Checkbox(row.boxID, row.checked)
		f.Text(row.label)
		f.Text(row.speedText) // 速度文案在状态变化时格式化好
	})
	if line.Clicked() {
		m.selected = row
	}
}
```

Driver 断言（spike `TestRowIDsSurviveReorderAndClicksFollowTheNode`）：

```go
before := driver.Snapshot()
clickID(t, driver, sortButtonID) // 触发重排（状态变化路径）
after := driver.Snapshot()
// 同一节点：before/after 都能按 rowID 找到，且 Bounds.Y 变了、ID 没变。
// 点击新位置：用 after 中该 rowID 的 Bounds 中心点击，模型选中的仍是它。
```

### 坑

1. **不要每帧 `tree.Sub`/`tree.Name`**：动态 `"node:"+key` 的字符串拼接和 map/slice 写入都在加载时做；
   帧内调用会分配，并违反规格 3.3/3.4。
2. **`ui.Tree` 只增不减**：`Name`/`Sub` 的内部表没有删除 API。会话内见过的不同节点身份会一直占着；
   节点 key 用内容哈希（同节点重载复用同一 key）时增长有界，但节点参数变化会累积。规格没要求回收，实测可接受，
   但不要用“每次刷新随机 key”之类的方案。
3. **并发**：`Tree` 的文档明确“不是并发安全”。若节点读取在后台 goroutine，身份注册要等 `OnWake`
   在窗口线程并入模型时再做，不能在读文件的 goroutine 里 `tree.Sub`。
4. **`tree.ID` 的 Name 是全局词表**：不同 scope 下同名（如 "row"）会复用同一个 `Name`，这是正常的；
   但两个不同含义的控件不要在同一 scope 下用同一个词。
5. **节点消失再出现**：身份由 key 决定，所以“重新获取节点”后同一 key 的 rowID 自动复用；前提是别在节点列表
   清理时把 ui.Tree 里的登记也一起清掉（没有 API，也清不掉）。
6. **点击判定与子控件同 §1 坑 5**：整行点击切换勾选只覆盖复选框之外的区域；复选框本身要单独处理。

---

## 3. 对话框 + 文本输入：`flex.Dialog` + `flex.Field`

### 结论

- `f.Dialog(flex.DialogSpec{...})` 在打开时声明即可；`Scope`/`DismissID`/`CloseID`/`InitialFocusID` 在 v0.2.0
  都是 **`ui.ID`**。`d.Body(...)` 放任意 flex 子控件，`d.Footer(...)` 放按钮（最后一个贴右下角）。
- 文本输入三件套：模型里 `name flex.Field`；`config.TextInput = flex.Fields(&m.name)`；
  绘制 `f.Field(&m.name, nameID, "名称", "输入名称")`。
- Driver 用法：点击输入框命中 → `driver.Type("…")`；单行框里按 Enter → 在声明 `Field` 的那个 Box 上
  `.Submitted()` 为真；点保存按钮 → `.Clicked()`；Esc / 点遮罩 / 关闭按钮 → `d.Dismissed()`。
- `InitialFocusID: nameID` 让对话框打开即聚焦输入框，可直接 `driver.Type`，不必先点一下。
- `DismissID` 是遮罩与 Escape 的落点；`CloseID` 是右上角关闭按钮。只给 `CloseID` 时 Escape 也关，
  但点遮罩不会关（遮罩没有 dismiss 命中）。

### 最小片段（v0.2.0）

```go
import (
	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/flex"
	"github.com/vivaui/viva/ui"
)

var (
	dialogScope = tree.ID(dialogSub, "scope")   // ui.ID，v0.2.0 不是 ui.Scope
	dismissID   = tree.ID(dialogSub, "dismiss") // 遮罩 + Esc
	closeID     = tree.ID(dialogSub, "close")   // 右上角 ×
	nameID      = tree.ID(dialogSub, "name")
	saveID      = tree.ID(dialogSub, "save")
	cancelID    = tree.ID(dialogSub, "cancel")
)

type model struct {
	open bool
	name flex.Field
}

func (m *model) config() app.Config {
	config := app.Config{Tree: &tree, Width: 900, Height: 600}
	config.TextInput = flex.Fields(&m.name) // 没有它 Driver.Type 返回 app.ErrNoTextInput
	return config
}

// Paint 内。
if m.open {
	d := f.Dialog(flex.DialogSpec{
		Title: "新建条目", Width: 400,
		Scope: dialogScope, DismissID: dismissID, CloseID: closeID,
		InitialFocusID: nameID, // 可选：打开即聚焦
	})
	d.Body(func() {
		// 在声明 Field 的这个 Box 上读 Submitted：单行框 Enter 不当作点击。
		if f.Field(&m.name, nameID, "名称", "输入名称").Submitted() {
			m.save(m.name.Text())
			m.open = false
		}
	})
	d.Footer(func() {
		if f.Button(cancelID, "取消").Outline().Clicked() {
			m.open = false
		}
		if f.Button(saveID, "保存").Clicked() {
			m.save(m.name.Text())
			m.open = false
		}
	})
	if d.Dismissed() { // 遮罩 /×/Esc 都走这里
		m.open = false
	}
}
```

Driver 测试：

```go
clickID(t, driver, openButtonID) // 打开
clickID(t, driver, nameID)       // 聚焦输入框
_ = driver.Type("香港机场")       // 进 flex.Field 的 text.Editor
snap := driver.Snapshot()
_ = snap.Focused == nameID && snap.Editor != nil && snap.Editor.Value == "香港机场"

_ = driver.Key(ui.KeyEnter, 0)   // → Field.Submitted
_ = driver.Key(ui.KeyEscape, 0)  // → Dialog.Dismissed（字段有焦点时也先给应用）
_ = driver.Click(4, 4)           // 点遮罩 → Dialog.Dismissed
```

### 坑

1. **`Config.TextInput` 必须有**：缺了它字段仍可获得焦点，但 `driver.Type` 返回 `app.ErrNoTextInput`；
   `flex.Fields` 用 `f.Field` 写入的 `field.id` 匹配焦点命中，因此字段至少要被画过一次。
2. **Enter 只有单行框才是 `Submitted`**：多行 `f.Textarea` 保留换行，没有 `Submitted`。`Submitted` 只在
   `f.Field`/`f.Input` 返回的 Box 上有效，且必须写在声明它的同一个 Paint 里。
3. **Esc 的目标**：`Dismissed()` 只看 `DismissID`/`CloseID`；两者都为零时 Esc 什么也不做。若希望“取消”与 Esc 同义，
   把 `DismissID` 设成取消按钮的 ID（官网 overlays 示例就是这样）。
4. **`InitialFocusID` 必须是已声明的、可聚焦的命中 ID**：随便造一个不存在的 ID，打开后焦点原位不动（spike 验证）。
5. **`Scope` 类型是 `ui.ID`**：`tree.Sub` 给的是 `ui.Scope`；写 `dialogSub.ID(tree.Name("scope"))` 或 `tree.ID(dialogSub,"scope")`。
6. **保存路径不要依赖 Editor 的瞬时状态**：`m.name.Text()` 在点击/提交的当帧就可用；但如果你在 `SetText` 之后
   同一帧又读，注意它是同步操作（spike 里 `SetText("")` 后重新输入正常）。
7. **遮罩点击不包括对话框内部空白**：`DismissID` 是窗口级背景；点在对话框表面上会被表面 `Block` 掉，不会误关。

---

## 4. 下拉预设：`flex.Select` 与 `Picked` 时机

### 结论

- `f.Select(scope, &state, labels, selected).Picked()`：scope 是控件**私有**的 `ui.Scope`，选项编号为
  `scope.At(0)..At(len-1)`，触发项是 `scope.At(len(labels))`，再往后运行时保留：
  `At(len+1)` 点外/Esc、`At(len+2)` 列表焦点 scope、`At(len+3)`/`At(len+4)` 上下滚动箭头。
- `Picked()` 只在“点击选项”的那一个事件帧返回 `(index, true)`；点触发项只是开/关列表，返回 false；
  点外部/按 Esc 关闭列表，返回 false。应用在返回 true 时立刻改 `selected` 即可“选中即生效”。
- **预 Select 的时机**：没有任何点击时 `Picked` 不返回；`Picked` 在事件帧里把 `state.Open` 置 false（选项）
  或翻转（触发项），所以不要在它返回之后又覆盖 `state.Open`。
- Driver 定位：先点 `scope.At(len)` 打开，再点 `scope.At(i)`；打开状态下这些 hit 才存在。

### 最小片段（v0.2.0）

```go
import (
	"github.com/vivaui/viva/flex"
	"github.com/vivaui/viva/ui"
)

// 每个下拉一个私有 scope，别和其它控件共用。
var presetScope = tree.Sub(filterPage, tree.Name("preset"))

type model struct {
	presetState flex.SelectState // 零值关闭
	preset      int
	presetLabels []string // 常驻，别每帧新建：Select 会保留到 End
}

// Paint 内：
if index, ok := f.Select(presetScope, &m.presetState, m.presetLabels, m.preset).Picked(); ok {
	m.preset = index // 选中即生效；同一帧关闭列表
}

// 滚轮：列表打开时它先吃滚轮，否则交给页面。
config.OnScroll = func(event ui.ScrollEvent) ui.EventResult {
	if m.presetState.Open {
		return m.presetState.Wheel(event)
	}
	return m.view.Wheel(event)
}
```

Driver 测试（spike `TestSelectPickedTiming`）：

```go
var trigger ui.ID = presetScope.At(len(labels)) // 触发项
clickID(t, driver, trigger)                     // 打开
clickID(t, driver, presetScope.At(2))           // 选第 3 项
// m.preset == 2、picks 恰好 +1、state.Open == false
// 之后的普通重绘（如 driver.Resize）不会再次触发 Picked
```

### 坑

1. **scope 专用**：`At(len)..At(len+4)` 被控件占用；与别的控件共用 scope 会撞 ID。源码注释明确要求“给它自己的 scope”。
2. **标签切片要稳定**：`f.Select` 保留 `labels` 直到 `End`，`Picked` 也读它；每帧新建切片既不必要也会分配。
3. **预选值不会触发 Picked**：`selected` 只决定显示；要“应用某预设”必须用户点击或用代码直接改模型。
4. **列表打开时接管滚轮**：`Config.OnScroll` 必须先问 `SelectState.Wheel`，否则滚轮会滚到底下的列表。
5. **选项很多时**：列表会在窗口内开窗显示 + 上下箭头；`state.First` 由控件维护，应用不要手改。
6. **点击外部关闭**：由控件内部的 `At(len+1)` 完成，不需要 `DismissID`；`Picked` 返回 false，列表关闭。

---

## 5. 分段控件：`flex.Choice` / `ChosenIndex`

### 结论

- 用 `f.Choice(scope, labels, selected)`：选项 ID 是 `scope.At(0..len-1)`，`ChosenIndex()` 直接给被点击的序号，
  省掉手写 ID。每个分段控件也要自己的 scope。
- 若用底层 `f.Segmented([]components.SegmentOption{...}, selectedID)`，则要自己用 `canvas.Clicked(option.ID)`
  判定，没有 `ChosenIndex`（它只认 `f.Choice` 建的控件）。

### 最小片段（v0.2.0）

```go
import (
	"github.com/vivaui/viva/flex"
	"github.com/vivaui/viva/ui"
)

var modeScope = tree.Sub(filterPage, tree.Name("mode"))
var modeLabels = []string{"简化", "正则"} // 常驻

// Paint 内：
if index, ok := f.Choice(modeScope, modeLabels, m.mode).ChosenIndex(); ok {
	m.mode = index // 0=简化, 1=正则
}
```

Driver：`clickID(t, driver, modeScope.At(1))`，断言 `m.mode == 1` 且下一帧出现对应文案。

### 坑

1. `ChosenIndex` 只在点击事件帧为真；普通重绘返回 false（与 `Picked`/`Clicked` 同一模型）。
2. 分段控件会给每个选项加独立 hit，可用键盘左右移动（组件语义），测试断言不要依赖序号宽度。
3. `Choice` 的 options 由 labels 生成并保留到 `End`，labels 同样要常驻。

---

## 6. 滚动列表：`ScrollArea` + `View.Wheel` + `Driver.Scroll`

### 结论

- `f.ScrollArea(id)` 的偏移由 `flex.View` 按 ID 保存；`Config.OnScroll = view.Wheel` 把滚轮交给指针下最近的
  ScrollArea。`id` 为零会 panic（源码显式检查）。
- `Driver.Scroll(ui.ScrollEvent{X, Y, DeltaY})` 能驱动它：非精确滚轮 `DeltaY` 至少按 24px 走；正值向下。
- 验证结果：一次 150px 滚轮后 `view.ScrollState(id).Offset == 150`，同一坐标下的行从 3 变成 8；
  指针在区域外滚动不生效；向上越过顶部钳到 0；窗口变高后下一帧把 offset 钳到新的最大值（900 → 680）。
- 命中区域被裁剪：滚出可视区的行不会产生可点击 hit（`Canvas.HitWithOptions` 会与当前 clip 求交）。

### 最小片段（v0.2.0）

```go
import (
	"github.com/vivaui/viva/flex"
	"github.com/vivaui/viva/ui"
)

var listAreaID = tree.ID(page, "node-list")

// Paint 内：
f.ScrollArea(listAreaID).Grow(1).FillWidth().Children(func() {
	for index := range m.rows {
		row := &m.rows[index]
		line := f.Row().Height(30).FillWidth().Hit(row.rowID, row.label)
		line.Children(func() { f.Text(row.label) })
		if line.Clicked() { m.selected = row }
	}
})

// Config 里（一个页面只有一个滚动区时；多个区也用 View.Wheel）：
config.OnScroll = m.view.Wheel

// 需要读偏移/程序化滚动时，每次重新取，不要长期持有指针：
offset := m.view.ScrollState(listAreaID).Offset
```

Driver：

```go
_ = driver.Scroll(ui.ScrollEvent{X: 200, Y: 150, DeltaY: 150})
_ = driver.Scroll(ui.ScrollEvent{X: 200, Y: 150, DeltaY: -500}) // 钳到 0
```

### 坑

1. **状态会掉**：某帧没有声明的 ScrollArea，其偏移在帧末被清掉并复用（例如切到“下载速度”视图再切回，
   列表回到顶部）。要保留就每帧都声明，或由应用自己存偏移。
2. **`ScrollState` 指针不要跨帧保存**：文档要求每次用 `view.ScrollState(id)` 重新取。
3. **多个区 + Select**：先 `SelectState.Wheel`，再 `view.Wheel`；表格之类的自滚动组件也要排在前面。
4. **`ScrollArea` 没有 hit**：它只是布局容器，行点击需要行自己 `Hit`。
5. 滚动事件是 `Precise` 与否影响步长；Driver 默认非精确（视为滚轮），一格至少 24px。

---

## 7. 窗口外壳：View 复用、Resize、Fill/Grow、分配

### 结论

- 外壳用一列：Header 固定高、Content `Grow(1)`、Footer 固定高，全部 `FillWidth()`；`Driver.Resize` 后布局立即正确：
  900×600 时 header `0..48`、body `48..552`、footer `552..592`；600×420 时 body 高 332、footer 从 380 开始。
- `flex.View` 复用（模型里一个字段）、Paint 回调与闭包只创建一次、文案在状态变化时算好，暖机后**声明阶段 0 allocs/op**。
- 测量（Windows 11 / Ryzen 7 4800H / Go 1.27.3，`go test -bench BenchmarkShell -benchtime=1s -count=3 -benchmem`）：
  - `BenchmarkShellDeclaration`：3.82–3.94 µs/op，**0 B/op，0 allocs/op**（用确定性 stub measurer 直接 `view.Draw`，
    只测声明+布局，不含平台字形测量与 publish）。
  - `BenchmarkShellDriverRepaint`：4.74–5.09 µs/op，**0 B/op，0 allocs/op**（经 `Driver.Resize` 的整帧重绘，
    同尺寸、内容不变）。
  - `testing.AllocsPerRun(200, run)` 断言为 0。
  - **口径限制**：这是“外壳 + 12 行、无文本测量/publish 变化”的数字，不能外推成真实界面零分配；真实界面若在 Paint
    里 `fmt.Sprintf`/字符串拼接/append 仍会分配。规格 3.7 的其他指标（二进制体积/启动/空闲内存）不在本 spike 范围。

### 最小片段（v0.2.0）

```go
import (
	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/flex"
	"github.com/vivaui/viva/ui"
)

type model struct {
	view flex.View // 复用，不要每帧 new
	// ... 预计算好的文案字段
}

func paintShell(f *flex.Frame, m *model) {
	f.Column().Fill().Children(func() {
		f.Row().Height(48).FillWidth().Hit(headerID, "header").Children(func() {
			f.Text("节点选择")
		})
		body := f.Column().Grow(1).FillWidth().Hit(bodyID, "body")
		body.Children(func() {
			f.ScrollArea(listAreaID).Grow(1).FillWidth().Children(func() {
				for index := range m.rows {
					row := &m.rows[index]
					f.Row().Height(30).FillWidth().Hit(row.rowID, row.label).Children(func() {
						f.Text(row.label)
					})
				}
			})
		})
		f.Row().Height(40).FillWidth().Hit(footerID, "footer").Children(func() {
			if f.Button(downloadID, "下载速度").Clicked() {
				m.start()
			}
		})
	})
}

func (m *model) config() app.Config {
	config := app.Config{Tree: &tree, Title: "订阅测速工具", Width: 900, Height: 600}
	body := func(f *flex.Frame) { paintShell(f, m) } // 只建一次
	config.Paint = func(canvas *ui.Canvas, size ui.Size) {
		bounds := ui.Rect{Width: size.Width, Height: size.Height}
		canvas.Fill(bounds, ui.LightTheme().Background)
		m.view.Draw(canvas, ui.LightTheme(), bounds, body)
	}
	return config
}
```

### 补充：Stack + Absolute（本仓库虚拟滚动列表的用法）

结论：`f.Stack()` 里的 `.Absolute().Top(y).Left(0).Right(0)` 子项可以超出 Stack 的矩形；**只有给 Stack 加
`.Clip()` 才会同时裁剪绘制和命中**。裁剪后 `Canvas.HitWithOptions` 会把命中矩形与当前 clip 求交：
部分可见的行只保留可见部分（实测 40px 行被截成 20px），完全不可见的行不产生 hit。绝对定位子项不参与
Stack 的尺寸计算，Stack 仍需 `Grow`/显式高度。

```go
// Paint 内：手动虚拟滚动列表（本仓库 view_nodes.go / view_results.go 的思路）。
list := f.Stack().Grow(1).FillWidth().Clip()
list.Children(func() {
	viewport := m.viewportHeight // 上一帧 list.Rect().Height，或按窗口估算
	for index := range m.items {
		item := &m.items[index]
		top := item.y - m.offset
		if top+item.height < 0 || top > viewport {
			continue // 只为性能；越界行反正会被 Clip 裁掉
		}
		row := f.Column().Absolute().Top(top).Left(0).Right(0).Height(item.height)
		row.Children(func() { ... })
	}
})
```

坑：

1. 不加 `.Clip()`：子项画出 Stack 外且 **hit 保持完整矩形**（实测），点击列表外的空白也可能命中；
   虚拟列表必须 `.Clip()`。
2. 加了 `.Clip()`：部分可见行的 `Snapshot().Hits` 矩形是裁剪后的，测试不要按整行高断言；
   完全不可见的行没有 hit（点击安全）。
3. `Absolute` 子项不撑大 Stack，Stack 需要 `Grow(1)` 或显式 `Height`。
4. 这种方案下滚轮要自己算（本仓库 `onScroll` + `scrollList` 就是这样）；`View.Wheel` 只对 `ScrollArea` 生效。
5. 行 `Top` 随 offset 每帧变化，但行 ID 不变——与 §2 的稳定身份一致；点击仍按坐标命中正确行。

### 坑

1. **不要在 Paint 里读 `Box.Rect()` 做布局**：它要等这一帧 `End` 之后才有值；跨控件定位用 `f.Popup().BelowBox(...)` 之类。
2. **闭包**：`view.Draw(..., func(f *flex.Frame){...})` 每帧新建一个捕获变量的闭包会把分配带进帧；把回调提到
   `config.Paint` 外层/结构体方法。
3. **字符串**：`"已选 " + strconv.Itoa(n) + " / 共 …"` 这类格式化必须在状态变化时做，Paint 只引用字段。
4. Resize 的尺寸单位是逻辑像素，`Driver.Resize(w, h int)`；`App.Config.Width/Height` 也是 `int`。
5. `OnResize` 适合派生尺寸相关状态（规格里没要求），但 Paint 只能读状态。

---

## 8. headless：构建、走查、GPU 依赖

### 结论

- 两个文件即可：正常入口 `main.go`；`main_headless.go` 加 `//go:build vivahead` 并空白导入
  `github.com/vivaui/viva/headless/auto`。`go build -tags vivahead` 出的二进制在 `VIVA_HEADLESS=1` 时走行协议，
  不设该环境变量则仍开窗口（所以 CI 里只跑带 tag 的产物）。
- 本次走查（`rightclick "节点 A"` 打开菜单 → `click "复制此节点信息"` → `clip` 读到复制的多行文本 → `layout`
  报 `ok 0 problems` → `quit`）全部通过；每条命令以恰好一行 `ok ...`/`error ...` 结束。
- Windows 上 **非截图命令不依赖 GPU**：`headless/session.go` 的 `Session.Image()`（`shot` 走它）里渲染器是
  懒创建的（`session.renderer == nil` 才 `newRenderer()`），`headless/render_windows.go` 的渲染器是
  Direct3D 11 offscreen。`click/texts/rightclick/layout/hits/focus` 都不创建它；`shot`/`record` 才创建，
  因此 `shot` 需要可用的 D3D11 设备（GPU 或 WARP）。本机 `shot` 成功（73,197 B PNG）。
- 本机二进制体积（`go build` 产物）：带 `vivahead` 7,325,184 B；不带 5,492,224 B，差约 +1.83 MB
  （含 headless session + D3D 离屏渲染；仅供参考，不是本 spike 的正式体积测量）。

### 最小片段（v0.2.0）

```go
// main_headless.go
//go:build vivahead

package main

import _ "github.com/vivaui/viva/headless/auto"
```

```bash
# 构建与运行
GOPROXY=https://goproxy.cn,direct go build -tags vivahead -o bin/app.exe .
VIVA_HEADLESS=1 ./bin/app.exe < script.txt
```

`script.txt`（本次实际走查脚本，一行一条）：

```text
size
texts
click "读取节点"
texts
rightclick "节点 A"
texts
click "复制此节点信息"
clip
texts
layout
quit
```

输出摘要（真实输出，节选）：

```text
ok 900x560 scale=2
x=16 y=17.5 w=780 h=21 size=14 "订阅测速工具"
x=818 y=12 w=76 h=32 size=14 "读取节点"
x=22 y=79.5 w=40.865234 h=21 size=14 "节点 A"
...
ok
x=462 y=94 w=176 h=32 size=14 "复制此节点信息"
x=462 y=126 w=176 h=32 size=14 "立即测试此节点"
...
ok "节点 A\n香港 01\n未测试"
...
ok 0 problems
ok bye
```

### 坑

1. **命令参数里的路径要用 Windows 路径**：`shot /tmp/x.png` 在 Windows 上会被当成字面路径，报
   `The system cannot find the path specified.`；用 `shot C:/Users/.../x.png`。行协议本身按空格切词，含空格用 `"..."`。
2. **`click "文本"` 找的是 hit 的 label，不是画出来的 Text**：只有 `Hit`/`Button` 等控件才有可点击 label；
   纯 `f.Text("订阅测速工具")` 会得到 `error headless: no control labelled "订阅测速工具"`。节点行要能 headless 点击，
   必须 `f.Row()...Hit(rowID, "节点 A")`（spike 已验证）。
3. **`texts` 是子串过滤**（`texts "订阅"` 也可以），不带参数列全部；`layout` 只报可疑文本，永远 `ok N problems` 而不是失败。
4. **`VIVA_HEADLESS` 非空即可**（值为 `1` 只是习惯）；不设置就是普通窗口程序。
5. 行协议命令集合以 `headless/control.go` 中 `Serve` 上方注释为准（本笔记不复制全表，避免过期）。
6. `openfiles/savefile` 等文件对话框命令是“排队下一次系统对话框结果”，`Driver` 测试里对应
   `QueueOpenFiles`/`QueueSaveFile`（规格 10.2 用例 1 会用到）。
7. headless 的 `Editor` 在本机 `Snapshot().Editor` 可见；Windows 离屏渲染的截图不画原生编辑器
   （`rendersTextEditor = false`，见 `headless/render_windows.go`）。

---

## 9. 与 `docs/specs/0001-viva-gui.md` 的冲突/限制清单

1. **§8.2「整行点击也可切换勾选」**：行 `Hit` 不会收到落在子控件（`flex.Checkbox`）上的点击。整行切换只覆盖
   复选框之外的区域；复选框自身用 `.Clicked()`。右键同理（§1 坑 5）。功能不冲突，但实现要按两个身份分别处理。
2. **§3.4 / §5 身份注册位置**：`ui.Tree` 非并发安全且无注销。规格要求“读取时注册”，若读取在后台 goroutine，
   必须把注册搬到 `OnWake` 的窗口线程阶段；否则是真实的数据竞争。Tree 也不能回收消失节点的名字。
3. **§8.4 右键菜单**：源码 API 满足规格；但“单层平铺”没有子菜单机制，`ContextMenuOptions.Submenu` 只给子菜单内容用，
   应用不要指望它自动展开。点外 + Esc 由 `Viewport`+`DismissID` 完成，需显式设置两者。
4. **§7.3 预设“下拉选中即生效”**：成立，但生效点是 `Picked()` 返回 true 的那个事件帧；只在 `Picked` 里改模型，
   不要在别处再覆盖 `state.Open`。
5. **§8.3 视图切换与滚动位置**：同一帧不声明的 ScrollArea 会在帧末丢掉偏移；在“节点选择/下载速度”来回切时，
   列表会回顶部。规格没要求保留，若产品上想要保留需应用自己记。
6. **`Scope` 参数类型**：v0.2.0 的 `flex.DialogSpec`、`components.ContextMenuOptions` 用 `ui.ID`，而
   `flex.Select`、`flex.Choice` 用 `ui.Scope`。规格没写类型，但实现时不要混用（`tree.Sub` 结果不能直接给 Dialog）。
7. **官网 overlays 指南与源码不一致**：官网用 `Popup()+Hit` 讲菜单、没提 `BeginContextMenu`；规格 8.4 要求
   `BeginContextMenu`，以 v0.2.0 源码为准（`components/context_menu.go`），本笔记按源码验证。
8. **Go 版本**：viva v0.2.0 的 `go.mod` 是 `go 1.26`，模块 `go` 指令必须 ≥1.26（本分支 `go.mod` 与 `AGENTS.md`
   都已按 1.26 更新；本机实测 Go 1.27.3）。
9. **拖拽/系统文件对话框**：本 spike 未验证真实窗口的 IME、焦点丢失、高 DPI、原生文件对话框；这些仍按规格 10.4 列为未验证项。

---

## 10. 对本仓库当前实现（internal/gui）的检查

以下是按本笔记的实测行为对 `internal/gui` 的对照检查，**前两条建议改**；第三条是确认用法正确。

1. **`handleNodePointer` 少一个 `PointerDown` 过滤**（`internal/gui/view_nodes.go`）。
   `PointerOn` 一次点击会投递 press+release 两个事件（§1 实测），当前代码只判断了 `Button == Secondary`，
   所以一次右键 `openNodeMenu` 会被调用两次（第二次在释放时，重新算 entries/clip、重设菜单位置；当前结果幂等，
   但多一次分配，且如果以后在打开菜单时改状态会双触发）。建议：
   ```go
   func (a *App) handleNodePointer(canvas *ui.Canvas, id ui.ID, node *nodeState) {
       event, ok := canvas.PointerOn(id)
       if !ok || event.Kind != ui.PointerDown || event.Button != ui.PointerButtonSecondary {
           return
       }
       a.openNodeMenu(canvas, node, event.X, event.Y)
       canvas.Consume()
   }
   ```
2. **在 Paint 里同步打开系统文件对话框**（`internal/gui/actions.go` 的 `chooseEntryPath` → `app.OpenFiles`）。
   Viva 的 `ui.Canvas.AfterPaint` 文档明确要求“可能阻塞或打开原生对话框的点击处理”走 `AfterPaint`：
   ```go
   if f.Button(a.ids.entryChoose, "选择文件…").Small().Outline().Clicked() {
       canvas := f.Canvas()
       canvas.AfterPaint(func() { a.chooseEntryPath() })
   }
   ```
   Driver 测试能过是因为 `QueueOpenFiles` 在驱动层拦截；真实窗口里在 Paint 期间开模态对话框不属于受支持用法。
   同类风险：`saveEntryForm` 的 `os.Stat` + JSON 落盘、`readEntry` 的 `LoadNodesFile`（YAML 解析）都在点击处理里
   同步执行；文件大时会卡一帧。要不要移到 `AfterPaint`/后台按产品取舍，但原生对话框这条建议直接改。
3. **右键在复选框上也能开菜单**：`paintNodeRow` 里对 `row` 和 `check` 各调一次 `handleNodePointer`，
   正是 §1 坑 5 要求的写法，正确；菜单高度/分隔线 9px/`ContextMenuItemWidth` 的算法也与源码一致
   （`ContextMenuItemWidth` 已含 8px 内边距，`bounds.Width = width+8` 只是多 8px 余量，不会裁字）。
4. **Stack+Absolute 方案**：`.Clip()` 已加，裁掉的行也做了 cull，正确；注意别把 `ScrollArea`/`view.Wheel`
   混进来（当前 `onScroll` 是手动 offset，自洽）。

---

## 11. 复现：spike 测试与命令

```bash
# 位置：/tmp/viva-spike（独立 module，不进仓库）
cd /tmp/viva-spike
export GOPROXY=https://goproxy.cn,direct

gofmt -l .                 # 无输出
go vet ./...               # 通过
go test ./... -count=1     # ok  viva-spike  0.083s；21 个测试全过
go test -run '^$' -bench 'BenchmarkShell' -benchtime=1s -count=3 -benchmem
```

测试清单（20 个）：

```text
TestSmokeDriver
TestAbsoluteRowsAreClippedByStackClip
TestContextMenuRightClickAndItemClick
TestContextMenuDismissesOnOutsidePressAndEscape
TestContextMenuMovesToAnotherRowInOneRightClick
TestRowHitDoesNotSeePressesOnItsOwnChildControl
TestRightClickOnRowChildOpensTheRowMenuWhenHandled
TestContextMenuHeightMustCoverEveryItem
TestRowIDsSurviveReorderAndClicksFollowTheNode
TestNewRowGetsItsOwnIdentity
TestDialogFieldTypeEnterAndSave
TestDialogInitialFocus
TestDialogEscapeScrimAndCloseButton
TestDialogEscapeWithOnlyCloseButton
TestSelectPickedTiming
TestSegmentedChosenIndex
TestTypeWithoutTextInputReturnsErrNoTextInput
TestScrollAreaWheel
TestScrollAreaOffsetClampedAfterShrink
TestShellLayoutAndResize
TestShellDeclarationAllocations
```

benchmark 原始输出：

```text
BenchmarkShellDeclaration-16      274005   3944 ns/op   0 B/op   0 allocs/op
BenchmarkShellDeclaration-16      302572   3823 ns/op   0 B/op   0 allocs/op
BenchmarkShellDeclaration-16      324898   3872 ns/op   0 B/op   0 allocs/op
BenchmarkShellDriverRepaint-16    227061   4742 ns/op   0 B/op   0 allocs/op
BenchmarkShellDriverRepaint-16    266551   4901 ns/op   0 B/op   0 allocs/op
BenchmarkShellDriverRepaint-16    216666   5093 ns/op   0 B/op   0 allocs/op
```

headless 复现：

```bash
cd /tmp/viva-spike
export GOPROXY=https://goproxy.cn,direct
go build -tags vivahead -o bin/app.exe ./headlessapp
VIVA_HEADLESS=1 ./bin/app.exe < walk.txt        # walk.txt 见 §8
```

本仓库自己的走查入口是 `scripts/headless-walkthrough.sh`（构建真实 GUI 的 `-tags vivahead` 产物）；
本笔记 §8 的输出来自仓库外的 spike 小应用，只用于验证 API，不是对真实界面的走查记录。

> 再次强调：spike 代码只用于验证，没有复制进仓库；仓库里本次只新增 `docs/notes/viva-ui-api.md`。
