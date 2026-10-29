package gui

import (
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
	}
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
			f.Row().Gap(8).Items(flex.Center).Children(func() {
				f.Text(a.entryForm.pathText).FontSize(12).Muted().Ellipsis().Grow(1)
				if f.Button(a.ids.entryChoose, "选择文件…").Small().Outline().Clicked() {
					f.Canvas().AfterPaint(func() { a.chooseEntryPath() })
				}
			})
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
