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

	newEntry      ui.ID
	entryScroll   ui.ID
	entryCollapse ui.ID
	entryExpand   ui.ID
	entrySplitter ui.ID

	// 滚动条身份：节点来源、节点列表、设置三处各一条。
	entryBar    ui.ID
	nodeBar     ui.ID
	settingsBar ui.ID

	filterField  ui.ID
	filterApply  ui.ID
	savePreset   ui.ID
	renamePreset ui.ID
	deletePreset ui.ID

	readNodes     ui.ID
	excludeFailed ui.ID
	startTest     ui.ID
	stopTest      ui.ID
	pauseTest     ui.ID

	// 节点来源面板与全局选择：全选/全不选、订阅批量重新获取。
	selectAll    ui.ID
	selectNone   ui.ID
	refetchAll   ui.ID
	about        ui.ID
	clearResults ui.ID
	previewImage ui.ID

	// 测速视图：轮次切换、导出、详情。
	roundPrev   ui.ID
	roundNext   ui.ID
	exportYAML  ui.ID
	exportImage ui.ID
	exportTSV   ui.ID
	retryExport ui.ID
	closeDetail ui.ID
	sortLatency ui.ID
	sortJitter  ui.ID
	sortLoss    ui.ID
	sortSpeed   ui.ID
	sortUpload  ui.ID
	sortTested  ui.ID

	// 设置视图。
	settingsScroll  ui.ID
	settingsSave    ui.ID
	settingsReset   ui.ID
	settingsMode    ui.Scope
	settingsOutput  ui.Scope
	setDownload     ui.ID
	setUpload       ui.ID
	setTimeout      ui.ID
	setConcurrent   ui.ID
	setParallel     ui.ID
	setServer       ui.ID
	setUA           ui.ID
	setEarlyStop    ui.ID
	setMaxLatency   ui.ID
	setMaxLoss      ui.ID
	setMinDownload  ui.ID
	setMinUpload    ui.ID
	setOutputPath   ui.ID
	setOutputBrowse ui.ID
	setImage        ui.ID
	setSpeedOnly    ui.ID
	setRename       ui.ID
	setRenameTmpl   ui.ID
	setGistToken    ui.ID
	setGistAddress  ui.ID
	setRepoToken    ui.ID
	setRepoAddress  ui.ID
	setRepoFile     ui.ID
	setRepoBranch   ui.ID

	entryDialogScope ui.ID
	entryName        ui.ID
	entryKind        ui.Scope
	entryURL         ui.ID
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

	aboutScope ui.ID
	aboutClose ui.ID

	previewScope ui.ID
	previewClose ui.ID
	previewZoom  ui.ID

	// 节点详情弹窗（从行尾三点图标展开）。
	detailDismiss ui.ID
	detailClose   ui.ID

	menuDismiss    ui.ID
	menuRoot       ui.ID
	menuCopy       ui.ID
	menuTest       ui.ID
	menuSelect     ui.ID
	menuUnselect   ui.ID
	menuExclude    ui.ID
	menuCopyMulti  ui.ID
	menuTestMulti  ui.ID
	menuTestOne    ui.ID
	menuUnselMulti ui.ID
	menuExclMulti  ui.ID
	menuOpenDir    ui.ID
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
	value.entryCollapse = tree.ID(page, "entry-collapse")
	value.entryExpand = tree.ID(page, "entry-expand")
	value.entrySplitter = tree.ID(page, "entry-splitter")
	value.entryBar = tree.ID(page, "entry-scrollbar")
	value.nodeBar = tree.ID(page, "node-scrollbar")
	value.settingsBar = tree.ID(page, "settings-scrollbar")

	value.filterField = tree.ID(page, "filter-expression")
	value.filterApply = tree.ID(page, "filter-apply")
	value.savePreset = tree.ID(page, "preset-save")
	value.renamePreset = tree.ID(page, "preset-rename")
	value.deletePreset = tree.ID(page, "preset-delete")

	value.readNodes = tree.ID(page, "read-nodes")
	value.excludeFailed = tree.ID(page, "exclude-failed")
	value.startTest = tree.ID(page, "start-test")
	value.stopTest = tree.ID(page, "stop-test")
	value.pauseTest = tree.ID(page, "pause-test")
	value.selectAll = tree.ID(page, "select-all-nodes")
	value.selectNone = tree.ID(page, "select-none-nodes")
	value.refetchAll = tree.ID(page, "refetch-all-subscriptions")
	value.about = tree.ID(page, "about")
	value.clearResults = tree.ID(page, "clear-results")
	value.previewImage = tree.ID(page, "preview-image")

	value.roundPrev = tree.ID(page, "round-prev")
	value.roundNext = tree.ID(page, "round-next")
	value.exportYAML = tree.ID(page, "export-yaml")
	value.exportImage = tree.ID(page, "export-image")
	value.exportTSV = tree.ID(page, "export-tsv")
	value.retryExport = tree.ID(page, "retry-export")
	value.closeDetail = tree.ID(page, "close-detail")
	value.sortLatency = tree.ID(page, "sort-latency")
	value.sortJitter = tree.ID(page, "sort-jitter")
	value.sortLoss = tree.ID(page, "sort-loss")
	value.sortSpeed = tree.ID(page, "sort-speed")
	value.sortUpload = tree.ID(page, "sort-upload")
	value.sortTested = tree.ID(page, "sort-tested")

	value.settingsScroll = tree.ID(page, "settings-scroll")
	value.settingsSave = tree.ID(page, "settings-save")
	value.settingsReset = tree.ID(page, "settings-reset")
	value.settingsMode = tree.Sub(page, tree.Name("settings-mode"))
	value.settingsOutput = tree.Sub(page, tree.Name("settings-output"))
	value.setDownload = tree.ID(page, "set-download")
	value.setUpload = tree.ID(page, "set-upload")
	value.setTimeout = tree.ID(page, "set-timeout")
	value.setConcurrent = tree.ID(page, "set-concurrent")
	value.setParallel = tree.ID(page, "set-parallel")
	value.setServer = tree.ID(page, "set-server")
	value.setUA = tree.ID(page, "set-ua")
	value.setEarlyStop = tree.ID(page, "set-early-stop")
	value.setMaxLatency = tree.ID(page, "set-max-latency")
	value.setMaxLoss = tree.ID(page, "set-max-loss")
	value.setMinDownload = tree.ID(page, "set-min-download")
	value.setMinUpload = tree.ID(page, "set-min-upload")
	value.setOutputPath = tree.ID(page, "set-output-path")
	value.setOutputBrowse = tree.ID(page, "set-output-browse")
	value.setImage = tree.ID(page, "set-image")
	value.setSpeedOnly = tree.ID(page, "set-speed-only")
	value.setRename = tree.ID(page, "set-rename")
	value.setRenameTmpl = tree.ID(page, "set-rename-template")
	value.setGistToken = tree.ID(page, "set-gist-token")
	value.setGistAddress = tree.ID(page, "set-gist-address")
	value.setRepoToken = tree.ID(page, "set-repo-token")
	value.setRepoAddress = tree.ID(page, "set-repo-address")
	value.setRepoFile = tree.ID(page, "set-repo-file")
	value.setRepoBranch = tree.ID(page, "set-repo-branch")

	value.entryDialogScope = tree.ID(value.dialogs, "entry-scope")
	value.entryName = tree.ID(value.dialogs, "entry-name")
	value.entryKind = tree.Sub(value.dialogs, tree.Name("entry-kind"))
	value.entryURL = tree.ID(value.dialogs, "entry-url")
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

	value.aboutScope = tree.ID(value.dialogs, "about-scope")
	value.aboutClose = tree.ID(value.dialogs, "about-close")
	value.previewScope = tree.ID(value.dialogs, "preview-scope")
	value.previewClose = tree.ID(value.dialogs, "preview-close")
	value.previewZoom = tree.ID(value.dialogs, "preview-zoom")

	value.detailDismiss = tree.ID(value.dialogs, "detail-dismiss")
	value.detailClose = tree.ID(value.dialogs, "detail-close")

	value.menuDismiss = tree.ID(value.menu, "dismiss")
	value.menuRoot = tree.ID(value.menu, "root")
	value.menuCopy = tree.ID(value.menu, "copy-one")
	value.menuTest = tree.ID(value.menu, "test-one")
	value.menuSelect = tree.ID(value.menu, "select-one")
	value.menuUnselect = tree.ID(value.menu, "unselect-one")
	value.menuExclude = tree.ID(value.menu, "exclude-one")
	value.menuCopyMulti = tree.ID(value.menu, "copy-many")
	value.menuTestMulti = tree.ID(value.menu, "test-many")
	value.menuTestOne = tree.ID(value.menu, "test-pointed")
	value.menuUnselMulti = tree.ID(value.menu, "unselect-many")
	value.menuExclMulti = tree.ID(value.menu, "exclude-many")
	value.menuOpenDir = tree.ID(value.menu, "open-entry-directory")
	return value
}

// entryIDs 是每个条目的动态身份。条目有稳定 ID，所以删除/重排其它条目时
// 这些身份不变。
type entryIDs struct {
	scope       ui.Scope
	check       ui.ID
	card        ui.ID
	read        ui.ID
	emptyRead   ui.ID
	refetch     ui.ID
	edit        ui.ID
	openDir     ui.ID
	remove      ui.ID
	selectAll   ui.ID
	selectNone  ui.ID
	groupToggle ui.ID
}

func registerEntryIDs(tree *ui.Tree, parent ui.Scope, entryID string) entryIDs {
	scope := tree.Sub(parent, tree.Name("entry:"+entryID))
	return entryIDs{
		scope:       scope,
		check:       tree.ID(scope, "check"),
		card:        tree.ID(scope, "card"),
		read:        tree.ID(scope, "read"),
		emptyRead:   tree.ID(scope, "empty-read"),
		refetch:     tree.ID(scope, "refetch"),
		edit:        tree.ID(scope, "edit"),
		openDir:     tree.ID(scope, "open-dir"),
		remove:      tree.ID(scope, "remove"),
		selectAll:   tree.ID(scope, "select-all"),
		selectNone:  tree.ID(scope, "select-none"),
		groupToggle: tree.ID(scope, "group-toggle"),
	}
}

// nodeIDs 是节点行的动态身份。身份来自节点稳定身份，不来自列表位置；
// 下载速度视图实时重排时，行 ID 不变。
type nodeIDs struct {
	scope ui.Scope
	row   ui.ID
	check ui.ID
	// knob 是行尾的竖向三点图标：打开详情的唯一入口。
	knob ui.ID
}

func registerNodeIDs(tree *ui.Tree, entry entryIDs, nodeID string) nodeIDs {
	scope := tree.Sub(entry.scope, tree.Name("node:"+nodeID))
	return nodeIDs{
		scope: scope,
		row:   tree.ID(scope, "row"),
		check: tree.ID(scope, "check"),
		knob:  tree.ID(scope, "detail-knob"),
	}
}

func (value ids) viewIndex(active viewKind) int {
	if active == viewSettings {
		return 1
	}
	return 0
}
