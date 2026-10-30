package gui

import (
	"strings"

	"github.com/vivaui/viva/flex"
)

func (a *App) paintDialogs(f *flex.Frame) {
	switch a.dialog {
	case dialogEntry:
		a.paintEntryDialog(f)
	case dialogPreset:
		a.paintPresetDialog(f)
	case dialogConfirm:
		a.paintConfirmDialog(f)
	case dialogAlert:
		a.paintAlertDialog(f)
	case dialogAbout:
		a.paintAboutDialog(f)
	case dialogPreview:
		a.paintPreviewDialog(f)
	}
}

// openAbout 打开「关于与帮助」：版本、构建提交、数据目录和全部快捷键。
// 正文在打开时拼一次，绘制只读字符串。
func (a *App) openAbout() {
	var builder strings.Builder
	builder.WriteString("订阅测速")
	if a.version != "" {
		builder.WriteString(" · ")
		builder.WriteString(a.version)
	}
	builder.WriteString("\n")
	commit := a.commit
	if commit == "" {
		commit = "未注入"
	}
	builder.WriteString("构建提交：")
	builder.WriteString(commit)
	builder.WriteString("\n数据目录：")
	builder.WriteString(a.dataDir)
	builder.WriteString("\n\n快捷键\n")
	builder.WriteString("F1            关于与帮助\n")
	builder.WriteString("Ctrl+A        全选所有节点\n")
	builder.WriteString("Ctrl+Shift+A  全不选\n")
	builder.WriteString("Ctrl+R        重新获取全部订阅\n")
	builder.WriteString("Ctrl+Enter    开始测速\n")
	builder.WriteString("Esc           关闭详情或对话框\n\n")
	builder.WriteString("测速模式：快速只测延迟；下载测延迟与下载速度；完整再加测上传。\n")
	builder.WriteString("全部数据都写在程序目录的 data/ 下，拷贝整个程序目录即可迁移。")
	a.aboutText = builder.String()
	a.dialog = dialogAbout
}

func (a *App) paintAboutDialog(f *flex.Frame) {
	dialog := f.Dialog(flex.DialogSpec{
		Title:     "关于与帮助",
		Width:     580,
		Scope:     a.ids.aboutScope,
		DismissID: a.ids.aboutClose,
		CloseID:   a.ids.aboutClose,
	})
	dialog.Body(func() {
		f.Paragraph(a.aboutText, 24).FontSize(12)
	})
	dialog.Footer(func() {
		f.Spacer()
		if f.Button(a.ids.aboutClose, "关闭").Clicked() {
			a.dialog = dialogNone
		}
	})
	if dialog.Dismissed() {
		a.dialog = dialogNone
	}
}

func (a *App) paintPreviewDialog(f *flex.Frame) {
	dialog := f.Dialog(flex.DialogSpec{
		Title:     "结果图预览",
		Width:     960,
		Scope:     a.ids.previewScope,
		DismissID: a.ids.previewClose,
		CloseID:   a.ids.previewClose,
	})
	dialog.Body(func() {
		// 位图在 view.Draw 之后按这个区域绘制；区域几何来自上一帧布局。
		a.previewArea = f.Stack().Height(480).FillWidth().Clip()
	})
	dialog.Footer(func() {
		f.Text(a.preview.text).FontSize(11).Muted().Ellipsis().Grow(1)
		if a.preview.bitmap.Width > 0 {
			if f.Button(a.ids.previewZoom, a.previewZoomLabel()).Outline().Clicked() {
				f.Canvas().AfterPaint(func() { a.togglePreviewZoom() })
			}
		}
		if f.Button(a.ids.previewClose, "关闭").Clicked() {
			a.closePreview()
		}
	})
	if dialog.Dismissed() {
		a.closePreview()
	}
}

// closePreview 关闭预览对话框并丢掉位图引用，下一次预览重新解码。
func (a *App) closePreview() {
	a.preview.open = false
	a.dialog = dialogNone
}

func (a *App) paintEntryDialog(f *flex.Frame) {
	dialog := f.Dialog(flex.DialogSpec{
		Title:          a.entryDialogTitle(),
		Width:          560,
		Scope:          a.ids.entryDialogScope,
		DismissID:      a.ids.entryCancel,
		CloseID:        a.ids.entryCancel,
		InitialFocusID: a.ids.entryName,
	})
	dialog.Body(func() {
		f.Column().Gap(10).Children(func() {
			f.Field(&a.entryForm.name, a.ids.entryName, "名称", "例如：机场 A")
			f.Row().Gap(10).Items(flex.Center).Children(func() {
				f.Text("类型").FontSize(12).Muted().Width(48)
				if index, ok := f.Choice(a.ids.entryKind, entryKindLabels, a.entryForm.kind).ChosenIndex(); ok {
					a.entryForm.kind = index
				}
			})
			if a.entryForm.kind == 1 {
				f.Field(&a.entryForm.url, a.ids.entryURL, "订阅链接", "https://example.com/subscribe")
				f.Text("支持 Clash/Mihomo 配置、整段 base64 与需要补 flag=meta 的订阅。").FontSize(11).Muted()
			} else {
				f.Row().Gap(8).Items(flex.Center).Children(func() {
					f.Text(a.entryForm.pathText).FontSize(12).Muted().Ellipsis().Grow(1)
					if f.Button(a.ids.entryChoose, "选择文件…").Small().Outline().Clicked() {
						f.Canvas().AfterPaint(func() { a.chooseEntryPath() })
					}
				})
			}
			if a.entryForm.err != "" {
				f.Text(a.entryForm.err).FontSize(12).Color(a.theme.Danger)
			}
		})
	})
	dialog.Footer(func() {
		if f.Button(a.ids.entryCancel, "取消").Outline().Clicked() {
			a.dialog = dialogNone
		}
		if f.Button(a.ids.entrySave, "保存").Clicked() {
			f.Canvas().AfterPaint(func() { a.saveEntryForm() })
		}
	})
	if dialog.Dismissed() {
		a.dialog = dialogNone
	}
}

func (a *App) paintPresetDialog(f *flex.Frame) {
	dialog := f.Dialog(flex.DialogSpec{
		Title:          a.presetDialogTitle(),
		Width:          420,
		Scope:          a.ids.presetDialogScope,
		DismissID:      a.ids.presetCancel,
		CloseID:        a.ids.presetCancel,
		InitialFocusID: a.ids.presetName,
	})
	dialog.Body(func() {
		f.Column().Gap(10).Children(func() {
			f.Field(&a.presetForm.name, a.ids.presetName, "预设名称", "例如：香港 IPLC")
			if a.presetForm.err != "" {
				f.Text(a.presetForm.err).FontSize(12).Color(a.theme.Danger)
			}
			f.Text("预设保存当前过滤模式、表达式和与/或组合，可跨条目使用。").FontSize(11).Muted()
		})
	})
	dialog.Footer(func() {
		if f.Button(a.ids.presetCancel, "取消").Outline().Clicked() {
			a.dialog = dialogNone
		}
		if f.Button(a.ids.presetSave, "保存").Clicked() {
			f.Canvas().AfterPaint(func() { a.savePresetForm() })
		}
	})
	if dialog.Dismissed() {
		a.dialog = dialogNone
	}
}

func (a *App) paintConfirmDialog(f *flex.Frame) {
	dialog := f.Dialog(flex.DialogSpec{
		Title:     a.confirm.title,
		Width:     460,
		Scope:     a.ids.confirmScope,
		DismissID: a.ids.confirmCancel,
		CloseID:   a.ids.confirmCancel,
	})
	dialog.Body(func() {
		f.Paragraph(a.confirm.body, 6).FontSize(12).Muted()
	})
	dialog.Footer(func() {
		if f.Button(a.ids.confirmCancel, "取消").Outline().Clicked() {
			a.dialog = dialogNone
		}
		if f.Button(a.ids.confirmOK, "删除").Destructive().Clicked() {
			f.Canvas().AfterPaint(func() { a.confirmDelete() })
		}
	})
	if dialog.Dismissed() {
		a.dialog = dialogNone
	}
}

func (a *App) paintAlertDialog(f *flex.Frame) {
	dialog := f.Dialog(flex.DialogSpec{
		Title:     a.alert.title,
		Width:     540,
		Scope:     a.ids.alertScope,
		DismissID: a.ids.alertOK,
		CloseID:   a.ids.alertOK,
		NoClose:   true,
	})
	dialog.Body(func() {
		f.Paragraph(a.alert.body, 8).FontSize(12).Muted()
	})
	dialog.Footer(func() {
		if f.Button(a.ids.alertOK, "确定").Clicked() {
			a.alert.open = false
			a.dialog = dialogNone
		}
	})
	if dialog.Dismissed() {
		a.alert.open = false
		a.dialog = dialogNone
	}
}
