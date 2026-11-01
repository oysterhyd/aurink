package contract

// DocID 标识一个打开的文档会话（与路径无关，重命名不变）。
type DocID string

// SelRange 是一段选区：Anchor 为固定端，Head 为光标端，均为源码字节偏移。
type SelRange struct{ Anchor, Head int }

// Range 返回有序区间。
func (s SelRange) Range() Range {
	if s.Anchor <= s.Head {
		return Range{s.Anchor, s.Head}
	}
	return Range{s.Head, s.Anchor}
}

// Empty 报告是否为单光标。
func (s SelRange) Empty() bool { return s.Anchor == s.Head }

// Selection 的 Ranges[0] 为主选区。v1 只使用一个选区，多光标为预留。
type Selection struct{ Ranges []SelRange }

// Caret 返回位于 off 的单光标。
func Caret(off int) Selection { return Selection{Ranges: []SelRange{{off, off}}} }

// Primary 返回主选区。
func (s Selection) Primary() SelRange {
	if len(s.Ranges) == 0 {
		return SelRange{}
	}
	return s.Ranges[0]
}

// Origin 说明一次修改从何而来，用于历史合并、自动保存与事件过滤。
type Origin uint8

const (
	OriginUser      Origin = iota + 1 // 键入、删除
	OriginIME                         // 输入法提交
	OriginCommand                     // 格式/块命令、命令面板
	OriginInputRule                   // 键入触发的自动格式化
	OriginPaste
	OriginDrop
	OriginSource // 源码模式
	OriginReload // 外部修改重载
	OriginUndo
	OriginRedo
	OriginAPI // 程序化修改
)

// Transaction 是一次原子修改。Edits 的区间基于事务应用前的文本，必须有序、
// 互不重叠。
type Transaction struct {
	Edits []TextEdit
	// Selection 是应用后的选区；nil 表示把旧选区映射过本次修改。
	Selection *Selection
	Origin    Origin
	// Coalesce 非空时，与上一步相同键、时间相近且位置相邻的修改合并为一步撤销。
	Coalesce string
	// Label 是撤销菜单显示的文案键，如 "editing_undo_typing"。
	Label string
}

// DocEventKind 是文档事件的种类。
type DocEventKind uint8

const (
	DocTextChanged DocEventKind = iota + 1
	DocSelectionChanged
	DocReplaced // 整篇替换：重载、恢复
	DocMetaChanged
	DocReadOnlyChanged
)

// DocEvent 在主线程上发给订阅者。
type DocEvent struct {
	Kind    DocEventKind
	Version Version
	Changes []Change
	// Dirty 是内容或结构发生变化的块（含新块），视图据此作废缓存。
	Dirty  []BlockID
	Origin Origin
}

// DocumentModel 是一个打开文档的唯一真相：源码文本、解析树、选区与历史。
// 写方法只能在主线程调用；Text()/AST() 返回的快照可在任意 goroutine 读取。
type DocumentModel interface {
	ID() DocID
	Text() Text
	AST() AST
	Version() Version
	Meta() DocMeta
	SetMeta(m DocMeta)
	Selection() Selection
	// SetSelection 改变选区，不产生历史步骤（但被下一步历史记为"选区前"）。
	SetSelection(sel Selection)
	// Apply 应用事务。错误码：CodeInvalid（区间非法）、CodeReadOnly。
	Apply(tx Transaction) (Version, error)
	// Replace 整篇替换（外部重载、恢复副本），记为一步可撤销历史。
	Replace(text string, origin Origin) Version
	Undo() bool
	Redo() bool
	CanUndo() bool
	CanRedo() bool
	UndoLabel() string
	RedoLabel() string
	// BreakCoalescing 使下一次修改不与上一步合并（光标跳转、失焦时调用）。
	BreakCoalescing()
	ReadOnly() bool
	SetReadOnly(ro bool)
	// Subscribe 注册事件回调（主线程调用），返回取消函数。
	Subscribe(fn func(DocEvent)) (unsubscribe func())
}

// DocumentFactory 创建文档模型（core/document 实现，docsession 使用）。
type DocumentFactory interface {
	New(id DocID, text string, meta DocMeta, exts Extensions) DocumentModel
}
