package contract

// IntentKind 是键入意图的种类。
type IntentKind uint8

const (
	IntentInsertText IntentKind = iota + 1
	IntentNewline               // Enter
	IntentLineBreak             // Shift+Enter：硬换行
	IntentDeleteBackward
	IntentDeleteForward
	IntentIndent  // Tab
	IntentOutdent // Shift+Tab
	IntentPaste
)

// Granularity 是删除的粒度。
type Granularity uint8

const (
	GranChar Granularity = iota // 字素
	GranWord
	GranLine
)

// PastePayload 是剪贴板内容的各种表示（视图读取后交给编辑逻辑）。Editing 是纯函数，不做
// 格式转换：剪贴板有 HTML 或 RTF 时，视图先用 PasteConverter 转好放进 Markdown（D11、D12）。
// Editing 用 Markdown，没有时或 AsPlain 时用 Plain；代码块里总是用 Plain；表格单元格里
// 换行变为 <br>，| 转义为 \|。
type PastePayload struct {
	Plain    string
	HTML     string
	RTF      string
	Markdown string
	// AsPlain 表示"粘贴为纯文本"。
	AsPlain bool
}

// Intent 是一次键入意图。
type Intent struct {
	Kind  IntentKind
	Text  string
	Gran  Granularity
	Paste *PastePayload
}

// EditPrefs 是影响编辑逻辑的偏好。
type EditPrefs struct {
	BulletMarker  byte   // '-'
	StrongDelim   string // "**"
	EmphasisDelim string // "*"
	FenceDelim    string // "```"
	AutoPair      bool
	TabSize       int
	Exts          Extensions
}

// EditContext 是编辑逻辑的全部输入（纯函数，无副作用）。
type EditContext struct {
	Text      Text
	AST       AST
	Selection Selection
	Meta      DocMeta
	Prefs     EditPrefs
}

// EditCommand 是一条格式/结构命令及其参数。
type EditCommand struct {
	ID    CommandID
	Level int    // 标题级别、缩进层数
	Text  string // 代码语言、链接文字
	URL   string
	Rows  int
	Cols  int
	Align Align
	Paths []string // 已导入资源的相对路径
}

// TriState 是选区内某格式的状态。
type TriState uint8

const (
	Off TriState = iota
	On
	Mixed
)

// ListKind 是列表种类。
type ListKind uint8

const (
	ListNone ListKind = iota
	ListBullet
	ListOrdered
	ListTask
)

// FormatState 描述选区的格式，用于菜单勾选与格式条高亮。
type FormatState struct {
	Strong, Emphasis, Strike, Code, Highlight, Sup, Sub, Link TriState
	LinkDest                                                  string
	Block                                                     BlockKind
	Level                                                     int
	List                                                      ListKind
	InTable, InCode                                           bool
}

// Editing 把意图与命令转换为事务（core/editing 实现，纯函数）。
type Editing interface {
	// HandleIntent 在选区处执行意图，包含输入规则；ok=false 表示不处理。
	HandleIntent(ctx EditContext, in Intent) (tx Transaction, ok bool)
	// FollowUp 在键入的事务应用之后调用，返回该状态触发的输入规则事务（OriginInputRule），
	// 成为独立的一步撤销；ok=false 表示没有规则触发。
	FollowUp(ctx EditContext) (tx Transaction, ok bool)
	// Run 执行命令；ok=false 表示当前上下文不适用。
	Run(ctx EditContext, cmd EditCommand) (tx Transaction, ok bool, err error)
	Formatting(ctx EditContext) FormatState
}

// PasteConverter 把富文本剪贴板内容转换为 Markdown（adapters/richpaste 实现）。
type PasteConverter interface {
	HTMLToMarkdown(html string) (string, error)
	RTFToMarkdown(rtf string) (string, error)
}

// ClipData 是剪贴板上的内容，各格式可为空（FE-1.5）。
type ClipData struct {
	Text     string
	Markdown string // text/markdown
	HTML     string // 片段
	RTF      string
	Files    []string // 复制的文件，绝对路径
	PNG      []byte   // 复制的图片
}

// Clipboard 读写系统剪贴板的多种格式（adapters/clipboard 用 MyGo 的 Clipboard 实现；
// 视图不能导入 mygo/transfer，03 §1.1）。主线程调用。
type Clipboard interface {
	Read() ClipData
	Write(d ClipData) error
}
