package gui

import "github.com/vivaui/viva/ui"

// ids 是界面里所有静态控件的身份。身份是 ui.Tree 里的名字，不是手写编号；
// 每帧复用同一份，焦点、悬停、按压都按身份跟踪。
type ids struct {
	page      ui.Scope
	viewScope ui.Scope
	modeScope ui.Scope
	combScope ui.Scope
	preset    ui.Scope
	dialogs   ui.Scope
	menu      ui.Scope

	newEntry    ui.ID
	entryScroll ui.ID

	filterField  ui.ID
	filterApply  ui.ID
	savePreset   ui.ID
	renamePreset ui.ID
	deletePreset ui.ID

	readNodes     ui.ID
	excludeFailed ui.ID
	startTest     ui.ID
	backToSelect  ui.ID
	stopTest      ui.ID

	entryDialogScope ui.ID
	entryName        ui.ID
	entryChoose      ui.ID
	entryCancel      ui.ID
	entrySave        ui.ID

	presetDialogScope ui.ID
	presetName        ui.ID
	presetCancel      ui.ID
	presetSave        ui.ID

	confirmScope  ui.ID
	confirmCancel ui.ID
	confirmOK     ui.ID

	alertScope ui.ID
	alertOK    ui.ID

	menuDismiss    ui.ID
	menuRoot       ui.ID
	menuCopy       ui.ID
	menuTest       ui.ID
	menuSelect     ui.ID
	menuUnselect   ui.ID
	menuExclude    ui.ID
	menuCopyMulti  ui.ID
	menuTestMulti  ui.ID
	menuUnselMulti ui.ID
	menuExclMulti  ui.ID
}

func newIDs(tree *ui.Tree) ids {
	page := tree.Feature("app")
	var value ids
	value.page = page
	value.viewScope = tree.Sub(page, tree.Name("view-switch"))
	value.modeScope = tree.Sub(page, tree.Name("filter-mode"))
	value.combScope = tree.Sub(page, tree.Name("filter-combine"))
	value.preset = tree.Sub(page, tree.Name("preset-select"))
	value.dialogs = tree.Sub(page, tree.Name("dialogs"))
	value.menu = tree.Sub(page, tree.Name("node-menu"))

	value.newEntry = tree.ID(page, "new-entry")
	value.entryScroll = tree.ID(page, "entry-scroll")

	value.filterField = tree.ID(page, "filter-expression")
	value.filterApply = tree.ID(page, "filter-apply")
	value.savePreset = tree.ID(page, "preset-save")
	value.renamePreset = tree.ID(page, "preset-rename")
	value.deletePreset = tree.ID(page, "preset-delete")

	value.readNodes = tree.ID(page, "read-nodes")
	value.excludeFailed = tree.ID(page, "exclude-failed")
	value.startTest = tree.ID(page, "start-test")
	value.backToSelect = tree.ID(page, "back-to-select")
	value.stopTest = tree.ID(page, "stop-test")

	value.entryDialogScope = tree.ID(value.dialogs, "entry-scope")
	value.entryName = tree.ID(value.dialogs, "entry-name")
	value.entryChoose = tree.ID(value.dialogs, "entry-choose")
	value.entryCancel = tree.ID(value.dialogs, "entry-cancel")
	value.entrySave = tree.ID(value.dialogs, "entry-save")

	value.presetDialogScope = tree.ID(value.dialogs, "preset-scope")
	value.presetName = tree.ID(value.dialogs, "preset-name")
	value.presetCancel = tree.ID(value.dialogs, "preset-cancel")
	value.presetSave = tree.ID(value.dialogs, "preset-save-confirm")

	value.confirmScope = tree.ID(value.dialogs, "confirm-scope")
	value.confirmCancel = tree.ID(value.dialogs, "confirm-cancel")
	value.confirmOK = tree.ID(value.dialogs, "confirm-ok")

	value.alertScope = tree.ID(value.dialogs, "alert-scope")
	value.alertOK = tree.ID(value.dialogs, "alert-ok")

	value.menuDismiss = tree.ID(value.menu, "dismiss")
	value.menuRoot = tree.ID(value.menu, "root")
	value.menuCopy = tree.ID(value.menu, "copy-one")
	value.menuTest = tree.ID(value.menu, "test-one")
	value.menuSelect = tree.ID(value.menu, "select-one")
	value.menuUnselect = tree.ID(value.menu, "unselect-one")
	value.menuExclude = tree.ID(value.menu, "exclude-one")
	value.menuCopyMulti = tree.ID(value.menu, "copy-many")
	value.menuTestMulti = tree.ID(value.menu, "test-many")
	value.menuUnselMulti = tree.ID(value.menu, "unselect-many")
	value.menuExclMulti = tree.ID(value.menu, "exclude-many")
	return value
}

// entryIDs 是每个条目的动态身份。条目有稳定 ID，所以删除/重排其它条目时
// 这些身份不变。
type entryIDs struct {
	scope      ui.Scope
	check      ui.ID
	read       ui.ID
	refetch    ui.ID
	edit       ui.ID
	remove     ui.ID
	selectAll  ui.ID
	selectNone ui.ID
}

func registerEntryIDs(tree *ui.Tree, parent ui.Scope, entryID string) entryIDs {
	scope := tree.Sub(parent, tree.Name("entry:"+entryID))
	return entryIDs{
		scope:      scope,
		check:      tree.ID(scope, "check"),
		read:       tree.ID(scope, "read"),
		refetch:    tree.ID(scope, "refetch"),
		edit:       tree.ID(scope, "edit"),
		remove:     tree.ID(scope, "remove"),
		selectAll:  tree.ID(scope, "select-all"),
		selectNone: tree.ID(scope, "select-none"),
	}
}

// nodeIDs 是节点行的动态身份。身份来自节点稳定身份，不来自列表位置；
// 下载速度视图实时重排时，行 ID 不变。
type nodeIDs struct {
	scope ui.Scope
	row   ui.ID
	check ui.ID
}

func registerNodeIDs(tree *ui.Tree, entry entryIDs, nodeID string) nodeIDs {
	scope := tree.Sub(entry.scope, tree.Name("node:"+nodeID))
	return nodeIDs{
		scope: scope,
		row:   tree.ID(scope, "row"),
		check: tree.ID(scope, "check"),
	}
}

func (value ids) viewIndex(active viewKind) int {
	if active == viewResults {
		return 1
	}
	return 0
}
