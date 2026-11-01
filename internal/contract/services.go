package contract

import (
	"context"
	"time"
)

// FileStamp 用于检测外部修改。
type FileStamp struct {
	ModTime time.Time
	Size    int64
}

// FileContent 是读入的文件。
type FileContent struct {
	Path  string
	Text  string // 已解码、已剥离 BOM 的 UTF-8，换行保持原样
	Meta  DocMeta
	Stamp FileStamp
}

// ReadOptions 控制读取。
type ReadOptions struct {
	MaxSize     int64 // 0 表示默认 64 MB；超出返回 CodeTooLarge
	AllowBinary bool  // false 时二进制返回 CodeBinary
}

// FileStore 读写文件（core/fsstore 实现）。全部方法阻塞，禁止在主线程调用。
type FileStore interface {
	Read(path string, opts ReadOptions) (FileContent, error)
	// Write 原子写入；expect 非 nil 且磁盘戳不符时返回 CodeConflict。
	Write(path string, text string, meta DocMeta, expect *FileStamp) (FileStamp, error)
	Stat(path string) (FileStamp, error)
}

// DocState 是会话状态，变化时经 Session.OnState 通知。
type DocState struct {
	ID        DocID
	Path      string // 未命名为 ""
	Name      string // 显示名
	Edited    bool
	Saving    bool
	Conflict  bool
	Deleted   bool
	ReadOnly  bool
	LargeFile bool
	SavedAt   time.Time
	Err       *Error // 最近一次保存/读取错误
}

// CloseDecision 是关闭请求的结果。
type CloseDecision uint8

const (
	CloseDone        CloseDecision = iota // 已关闭
	CloseNeedsPrompt                      // 有未保存修改且无法自动保存，需要询问用户
)

// Session 是一个打开的文件（core/docsession 实现）。方法在主线程调用、立即返回；
// done 回调在主线程触发。
type Session interface {
	ID() DocID
	Model() DocumentModel
	State() DocState
	Save(done func(error))
	SaveAs(path string, done func(error))
	Revert(done func(error))
	// KeepMine 冲突时以当前内容覆盖磁盘。
	KeepMine(done func(error))
	// LoadTheirs 冲突时载入磁盘版本（作为可撤销的一步）。
	LoadTheirs(done func(error))
	// Close 关闭会话；force 为 true 时丢弃未保存修改（恢复副本同时删除）。
	Close(force bool) CloseDecision
	OnState(fn func(DocState)) (off func())
}

// SessionsEventKind 是会话集合事件种类。
type SessionsEventKind uint8

const (
	SessionOpened SessionsEventKind = iota + 1
	SessionClosed
	SessionRestored
)

// SessionsEvent 在主线程上通知。
type SessionsEvent struct {
	Kind SessionsEventKind
	ID   DocID
}

// Sessions 管理全部打开的会话，同一路径只有一个会话。方法在主线程调用。
type Sessions interface {
	Open(path string, done func(Session, error))
	New() Session
	Get(id DocID) (Session, bool)
	FindByPath(path string) (Session, bool)
	All() []Session
	// Restore 启动时恢复崩溃副本与上次会话。
	Restore(done func([]Session, error))
	Subscribe(fn func(SessionsEvent)) (off func())
}

// Entry 是工作区中的一个文件或目录。
type Entry struct {
	Path    string
	Name    string
	IsDir   bool
	Size    int64
	ModTime time.Time
}

// WorkspaceEventKind 是工作区事件种类。
type WorkspaceEventKind uint8

const (
	WsRootChanged WorkspaceEventKind = iota + 1
	WsListed                         // 目录列表已加载/刷新
	WsCreated
	WsRemoved
	WsRenamed
	WsModified
	WsDegraded // 条目超过监听上限，降级为仅监听已展开目录
)

// WorkspaceEvent 在主线程上通知。
type WorkspaceEvent struct {
	Kind    WorkspaceEventKind
	Path    string
	OldPath string
	IsDir   bool
}

// Workspace 是打开的文件夹（core/workspace 实现）。方法在主线程调用、立即返回。
type Workspace interface {
	Root() string
	SetRoot(path string, done func(error))
	// List 返回缓存的目录列表；loaded=false 时开始异步加载并在完成后发出 WsListed。
	List(dir string) (entries []Entry, loaded bool)
	Refresh(dir string)
	Create(dir, name string, isDir bool, done func(path string, err error))
	Rename(path, newName string, done func(newPath string, err error))
	Move(paths []string, dstDir string, done func(err error))
	Trash(paths []string, done func(err error))
	// SetShowAll 切换显示全部文件或仅 Markdown 与图片。
	SetShowAll(all bool)
	Subscribe(fn func(WorkspaceEvent)) (off func())
}

// Assets 把图片导入文档的资源目录，返回可写入 Markdown 的相对路径。阻塞。
type Assets interface {
	ImportFiles(docPath string, files []string) ([]string, error)
	ImportImage(docPath string, png []byte, nameHint string) (string, error)
}

// Recent 是最近打开的文件与文件夹（主线程）。
type Recent interface {
	Files() []string
	Folders() []string
	Add(path string, isDir bool)
	Clear()
}

// Query 是查找条件。
type Query struct {
	Text          string
	CaseSensitive bool
	WholeWord     bool
	Regex         bool
}

// Match 是一处匹配。
type Match struct {
	Range   Range
	Line    int
	Preview string // 所在行（过长时截断）
	Hit     Range  // 匹配在 Preview 中的区间
}

// Finder 在一个文本快照中查找（纯函数，可在任意 goroutine）。
type Finder interface {
	// Find 返回至多 limit 个匹配（0 不限）；正则错误返回 CodeInvalid。
	Find(t Text, q Query, limit int) ([]Match, error)
}

// FileMatches 是一个文件中的匹配。
type FileMatches struct {
	Path    string
	Matches []Match
}

// WorkspaceSearch 在目录树中搜索。阻塞；onBatch 在调用者 goroutine 上按批回调。
type WorkspaceSearch interface {
	Search(ctx context.Context, root string, q Query, onBatch func([]FileMatches)) error
}

// FuzzyItem 是模糊匹配的候选项。
type FuzzyItem struct {
	Text   string
	Detail string
}

// FuzzyHit 是一个匹配结果；Positions 为 Text 中命中的 rune 下标。
type FuzzyHit struct {
	Index     int
	Score     int
	Positions []int
}

// Fuzzy 是模糊匹配（纯函数）。
type Fuzzy interface {
	Match(query string, items []FuzzyItem, limit int) []FuzzyHit
}
