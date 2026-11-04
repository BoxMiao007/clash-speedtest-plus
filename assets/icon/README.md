# 应用图标

「订阅测速」的应用图标：深色圆角方块 + 蓝色表盘弧 + 白色闪电。

## 为什么是这个图形

- **表盘弧**表达「测速」：一个没闭合的仪表盘，缺口朝下，是速度表最省笔画的画法。
- **闪电**表达「高速」，同时充当表盘指针，落在弧的中心。
- 两个形状都是实心大色块：16px 任务栏尺寸下仍然分得清，不会糊成一团。
- 配色沿用界面设计令牌（[ADR-0030](../../docs/adr/0030-theme-and-design-tokens.md)）：深底 `#171F2E → #080B11`，表盘弧 `#8BDCFF → #4C8DFF → #2563EB`（中段与深色主题的 `palette.Accent` 同色），闪电 `#F8FAFC`。
- 圆角半径是边长的 21.8%，接近 Windows 11 图标的安全圆角，缩小后不会显得锋利。

## 文件

| 文件 | 用途 |
| --- | --- |
| `app-icon.svg` | 设计源，改色/改比例从这里改 |
| `app-icon-{16,24,32,48,64,128,256,512,1024}.png` | 各尺寸 PNG |
| `app-icon.ico` | 16–256 多尺寸 Windows 图标（exe 资源用） |

运行时用的两档在 `internal/gui/icon-256.png`（窗口/任务栏图标）与 `internal/gui/icon-48.png`（标题栏品牌块），由脚本一并生成。

## 重新生成

```bash
python scripts/make-icon.py            # 输出 assets/icon 全套 + internal/gui 两档
python scripts/make-icon.py --preview qa/icon-preview   # 额外输出对比图
```

脚本依赖 Pillow 与 numpy（仓库其它脚本不需要它们）。`--variant` 可切换设计草案（dial / dial2 / bolt / bars），当前选的是 `dial2`。

exe 文件图标来自仓库根目录的 `rsrc_windows_{amd64,arm64}.syso`，用 `rsrc` 从 `app-icon.ico` 生成；改图标后需要重新生成这两个文件（命令见根目录 README 的「图标」小节）。
