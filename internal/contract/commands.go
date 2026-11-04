package contract

// CommandID 是命令的稳定标识，命名空间：file. edit. format. block. table.
// insert. view. nav. app.
type CommandID string

// 约定的命令 ID。新增命令须在此登记（次版本变更）。
const (
	CmdFileNew          CommandID = "file.new"
	CmdFileOpen         CommandID = "file.open"
	CmdFileOpenFolder   CommandID = "file.openFolder"
	CmdFileSave         CommandID = "file.save"
	CmdFileSaveAs       CommandID = "file.saveAs"
	CmdFileRename       CommandID = "file.rename"
	CmdFileRevert       CommandID = "file.revert"
	CmdFileClose        CommandID = "file.close"
	CmdFileReopenClosed CommandID = "file.reopenClosed"
	CmdFileExportHTML   CommandID = "file.exportHTML"
	CmdFileExportPDF    CommandID = "file.exportPDF"
	CmdFilePrint        CommandID = "file.print"
	CmdFileReveal       CommandID = "file.revealInFolder"

	CmdEditUndo         CommandID = "edit.undo"
	CmdEditRedo         CommandID = "edit.redo"
	CmdEditCut          CommandID = "edit.cut"
	CmdEditCopy         CommandID = "edit.copy"
	CmdEditPaste        CommandID = "edit.paste"
	CmdEditPastePlain   CommandID = "edit.pastePlain"
	CmdEditCopyMarkdown CommandID = "edit.copyMarkdown"
	CmdEditCopyHTML     CommandID = "edit.copyHTML"
	CmdEditSelectAll    CommandID = "edit.selectAll"
	CmdEditFind         CommandID = "edit.find"
	CmdEditReplace      CommandID = "edit.replace"
	CmdEditFindNext     CommandID = "edit.findNext"
	CmdEditFindPrev     CommandID = "edit.findPrev"

	CmdFormatStrong    CommandID = "format.strong"
	CmdFormatEmphasis  CommandID = "format.emphasis"
	CmdFormatUnderline CommandID = "format.underline"
	CmdFormatStrike    CommandID = "format.strike"
	CmdFormatCode      CommandID = "format.code"
	CmdFormatHighlight CommandID = "format.highlight"
	CmdFormatSup       CommandID = "format.sup"
	CmdFormatSub       CommandID = "format.sub"
	CmdFormatLink      CommandID = "format.link"
	CmdFormatClear     CommandID = "format.clear"

	CmdBlockParagraph   CommandID = "block.paragraph"
	CmdBlockHeading     CommandID = "block.heading" // EditCommand.Level
	CmdBlockHeading1    CommandID = "block.heading1"
	CmdBlockHeading2    CommandID = "block.heading2"
	CmdBlockHeading3    CommandID = "block.heading3"
	CmdBlockHeading4    CommandID = "block.heading4"
	CmdBlockHeading5    CommandID = "block.heading5"
	CmdBlockHeading6    CommandID = "block.heading6"
	CmdBlockQuote       CommandID = "block.quote"
	CmdBlockBulletList  CommandID = "block.bulletList"
	CmdBlockOrderedList CommandID = "block.orderedList"
	CmdBlockTaskList    CommandID = "block.taskList"
	CmdBlockToggleTask  CommandID = "block.toggleTask"
	CmdBlockCode        CommandID = "block.code" // EditCommand.Text = 语言
	CmdBlockMath        CommandID = "block.math"
	CmdBlockMermaid     CommandID = "block.mermaid"
	CmdBlockHR          CommandID = "block.hr"
	CmdBlockTOC         CommandID = "block.toc"
	CmdBlockIndent      CommandID = "block.indent"
	CmdBlockOutdent     CommandID = "block.outdent"
	CmdBlockMoveUp      CommandID = "block.moveUp"
	CmdBlockMoveDown    CommandID = "block.moveDown"

	CmdTableInsert    CommandID = "table.insert" // Rows, Cols
	CmdTableRowAbove  CommandID = "table.rowAbove"
	CmdTableRowBelow  CommandID = "table.rowBelow"
	CmdTableColLeft   CommandID = "table.colLeft"
	CmdTableColRight  CommandID = "table.colRight"
	CmdTableDeleteRow CommandID = "table.deleteRow"
	CmdTableDeleteCol CommandID = "table.deleteCol"
	CmdTableAlign     CommandID = "table.align" // EditCommand.Align
	CmdTableFormat    CommandID = "table.format"
	CmdTableNextCell  CommandID = "table.nextCell"
	CmdTablePrevCell  CommandID = "table.prevCell"
	// CmdTableResize 把光标所在表格调整为 Rows 个正文行（不含表头）、Cols 列：
	// 在下方、右侧补空单元格或删去多余的行列（内容随之删去），对齐保留；
	// Rows < 0、Cols < 1 或尺寸不变时不适用（Typora 的“调整表格”，2026-11-04）。
	CmdTableResize CommandID = "table.resize" // Rows, Cols
	// CmdTableDelete 删除光标所在的整张表格，光标留在原处的空段落（Typora 工具条的删除）。
	CmdTableDelete CommandID = "table.delete"

	CmdInsertImages      CommandID = "insert.images" // EditCommand.Paths
	CmdInsertLink        CommandID = "insert.link"   // URL, Text
	CmdInsertFootnote    CommandID = "insert.footnote"
	CmdInsertFrontMatter CommandID = "insert.frontMatter"

	CmdViewToggleSidebar CommandID = "view.toggleSidebar"
	CmdViewFiles         CommandID = "view.files"
	CmdViewOutline       CommandID = "view.outline"
	CmdViewSearch        CommandID = "view.search"
	CmdViewLive          CommandID = "view.live"
	CmdViewSource        CommandID = "view.source"
	CmdViewRead          CommandID = "view.read"
	CmdViewFocus         CommandID = "view.focus"
	CmdViewTypewriter    CommandID = "view.typewriter"
	CmdViewZoomIn        CommandID = "view.zoomIn"
	CmdViewZoomOut       CommandID = "view.zoomOut"
	CmdViewZoomReset     CommandID = "view.zoomReset"
	CmdViewThemeLight    CommandID = "view.themeLight"
	CmdViewThemeDark     CommandID = "view.themeDark"
	CmdViewThemeSystem   CommandID = "view.themeSystem"

	CmdNavQuickOpen       CommandID = "nav.quickOpen"
	CmdNavPalette         CommandID = "nav.palette"
	CmdNavGoToHeading     CommandID = "nav.goToHeading"
	CmdNavNextTab         CommandID = "nav.nextTab"
	CmdNavPrevTab         CommandID = "nav.prevTab"
	CmdNavSearchWorkspace CommandID = "nav.searchWorkspace"
	CmdNavGoToTab1        CommandID = "nav.goToTab1"
	CmdNavGoToTab2        CommandID = "nav.goToTab2"
	CmdNavGoToTab3        CommandID = "nav.goToTab3"
	CmdNavGoToTab4        CommandID = "nav.goToTab4"
	CmdNavGoToTab5        CommandID = "nav.goToTab5"
	CmdNavGoToTab6        CommandID = "nav.goToTab6"
	CmdNavGoToTab7        CommandID = "nav.goToTab7"
	CmdNavGoToTab8        CommandID = "nav.goToTab8"
	CmdNavGoToTab9        CommandID = "nav.goToTab9"

	CmdAppSettings CommandID = "app.settings"
	CmdAppAbout    CommandID = "app.about"
	CmdAppQuit     CommandID = "app.quit"
)

// Mods 是与平台无关的修饰键。
type Mods uint8

const (
	ModPrimary Mods = 1 << iota // macOS 为 ⌘，其他平台为 Ctrl
	ModShift
	ModAlt     // ⌥ / Alt
	ModCtrlMac // 仅 macOS 的 Control 键
)

// Chord 是一个按键组合。Key 名：A–Z、0–9、F1–F12、Enter、Tab、Escape、
// Backspace、Delete、Up、Down、Left、Right、Home、End、PageUp、PageDown、
// Space，以及标点 ` - = [ ] \ ; ' , . /。
type Chord struct {
	Mods Mods
	Key  string
}

// KeyBinding 把按键绑定到命令。Platform 为空表示所有平台，否则为 GOOS。
type KeyBinding struct {
	Command  CommandID
	Chord    Chord
	Platform string
}

// BindingConflict 是同一按键被多个命令占用。
type BindingConflict struct {
	Chord    Chord
	Commands []CommandID
}

// Command 是一条可执行命令，由各模块启动时注册。
type Command struct {
	ID       CommandID
	Title    string // i18n 文案键
	Category string // i18n 文案键
	// Palette 为 true 时出现在命令面板。
	Palette bool
	// Enabled 为 nil 表示总是可用；在主线程调用，必须廉价。
	Enabled func() bool
	// Checked 为 nil 表示非勾选型命令。
	Checked func() bool
	// Run 在主线程执行，不得阻塞。
	Run func() error
}

// CommandRegistry 是菜单、命令面板与快捷键共用的命令表（core/commands 实现）。
// 全部方法在主线程调用。
type CommandRegistry interface {
	// Register 注册命令；ID 重复返回 CodeExists。
	Register(cmds ...Command) error
	Lookup(id CommandID) (Command, bool)
	All() []Command
	// Run 执行命令：未知返回 CodeNotFound，不可用返回 CodeInvalid。
	Run(id CommandID) error
	// SetDefaults 设置默认绑定（按平台过滤后生效）。
	SetDefaults(b []KeyBinding)
	// SetUserBindings 设置用户覆盖并返回冲突。
	SetUserBindings(b []KeyBinding) []BindingConflict
	// Resolve 返回按键当前绑定的命令。
	Resolve(c Chord) (CommandID, bool)
	// KeysFor 返回命令当前的按键（菜单与面板显示用）。
	KeysFor(id CommandID) []Chord
}
