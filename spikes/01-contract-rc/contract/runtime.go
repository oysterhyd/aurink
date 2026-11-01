package contract

// Dispatcher 把函数投递到主线程执行并请求重绘（app 中为 Window.Update）。
// 任意 goroutine 可调用；不得阻塞等待。
type Dispatcher interface {
	Post(fn func())
}

// NoticeLevel 是通知级别。
type NoticeLevel uint8

const (
	NoticeInfo NoticeLevel = iota
	NoticeSuccess
	NoticeWarning
	NoticeError
)

// Notice 是服务请求界面显示的一条通知。
type Notice struct {
	Level   NoticeLevel
	Message string // 已本地化的标题
	Detail  string // 可选的已本地化说明
	Action  string // 可选按钮文本
	Run     func() // 按钮动作（主线程）
	// Progress 为 nil 时不显示进度条，否则为 0–1。
	Progress *float32
	// Sticky 为 true 时不自动消失。
	Sticky bool
}

// NoticeHandle 指向一条已显示的通知。主线程调用。
type NoticeHandle interface {
	Update(n Notice)
	Dismiss()
}

// Notifier 显示通知（views/shell 实现）。主线程调用。
type Notifier interface {
	Notify(n Notice) NoticeHandle
}
