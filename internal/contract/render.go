package contract

import (
	"context"
	"io"
)

// RenderKind 是渲染请求种类。
type RenderKind uint8

const (
	RenderMathInline RenderKind = iota + 1
	RenderMathBlock
	RenderDiagram // Mermaid
)

// RenderRequest 是一次渲染请求。
type RenderRequest struct {
	Kind     RenderKind
	Source   string
	Lang     string // RenderDiagram 的语言，如 "mermaid"
	Dark     bool
	FontSize float32
	Color    string  // 文本颜色 "#RRGGBB"
	Scale    float32 // 屏幕缩放，PNG 的像素 = DIP × Scale；0 视为 1
}

// RenderResult 是渲染结果；尺寸为 DIP。
type RenderResult struct {
	// SVG 和 PNG 成功时恰有一个非空。SVG 必须完整落在 Native 支持的静态子集内
	// （path、基本形状、分组等），不得残留 text。
	SVG []byte
	// PNG 用于图表，以及 SVG 子集不能完整表示的数学公式。像素 = DIP × Scale；
	// 数学的 Ascent 仍须有效。
	PNG    []byte
	Width  float32
	Height float32
	Ascent float32 // 行内公式基线以上的高度
	Scale  float32
}

// RenderHost 是一个不显示的浏览器页面（adapters/webviewhost 用 MyGo 的 WebView 实现，08 D8）。
// 方法阻塞，禁止在主线程调用；core/render 串行调用，同一时刻至多一个调用。
type RenderHost interface {
	// Available 报告系统的浏览器运行时是否存在。为 false 时不得调用 Start。
	Available() bool
	// Start 创建页面并以非空 baseURL 加载 page，页面脚本就绪后返回。已启动时先 Stop。
	Start(ctx context.Context, page, baseURL string) error
	// Eval 在页面里求值 script（结果是一个 Promise 或值），返回其 JSON。
	// 页面的渲染进程退出时返回 CodeUnavailable。ctx 结束时返回 CodeCanceled 或 CodeTimeout。
	Eval(ctx context.Context, script string) ([]byte, error)
	// Stop 关闭页面，释放浏览器进程。未启动时什么也不做。
	Stop()
}

// 渲染页面的协议（resources/render 的页面与 core/render 之间，08 D7–D9）：
//   - core/render 求值 "aurink.render(" + JSON(RenderRequest) + ")"。JSON 字段名为
//     kind（1 行内公式、2 块公式、3 图表）、source、lang、dark、fontSize、color、scale。
//   - 页面返回 {"svg": "...", "png": "<base64>", "width": w, "height": h, "ascent": a,
//     "scale": s}，svg 与 png 恰有一个非空；失败时返回 {"error": "说明"}。
//   - 公式的 SVG 不含 <text> 时返回 svg，否则返回 png；图表一律 png（CR-005）。
//   - core/render 再检查一次：svg 含 <text、png 解不开、尺寸不为正，都按 CodeRender 处理，不进缓存。

// Renderer 渲染数学与图表（core/render 实现）。Render 阻塞且带缓存，禁止在主线程调用。
type Renderer interface {
	// Available 报告渲染后端（WebView 运行时）是否可用。
	Available() bool
	// Render 错误码：CodeUnavailable、CodeRender（含错误详情）、CodeTimeout、CodeCanceled。
	Render(ctx context.Context, req RenderRequest) (RenderResult, error)
}

// ExportFormat 是导出格式。
type ExportFormat string

const (
	ExportHTML ExportFormat = "html"
	ExportPDF  ExportFormat = "pdf"
)

// ExportOptions 是导出选项。
type ExportOptions struct {
	Format      ExportFormat
	Title       string
	Dark        bool
	TOC         bool
	EmbedImages bool
	SoftBreak   SoftBreakMode // 单个换行：SoftBreakNewline 为 <br />，SoftBreakSpace 为空格
	// Fragment 为 true 时只输出正文片段（复制为 HTML）。
	Fragment bool
	BaseDir  string     // 解析相对图片路径
	PageSize string     // PDF："A4"、"Letter"
	Margins  [4]float32 // PDF 页边距（英寸）：上、右、下、左
	// OnProgress 可为 nil；在 Export 的调用 goroutine 上以 0–1 回调，单调不减，
	// 成功时最后一次为 1。
	OnProgress func(fraction float32)
	// Highlight 可为 nil；为代码块返回语法记号（D17）。core 不能导入 ui/editor，
	// 由 app 用 MyGo 的词法分析实现。
	Highlight func(lang, code string) []CodeToken
	// Renderer 可为 nil；非 nil 时公式和 mermaid 代码块以渲染结果导出（SVG 内联，PNG 为
	// data: URL）。为 nil 或渲染失败时，公式导出为 <span class="math"> / <div class="math">
	// 里的 TeX 源码，图表导出为代码块（08 D14）。
	Renderer Renderer
}

// CodeToken 是代码块中的一个记号。导出为 <span class="tok-Kind">。
type CodeToken struct {
	Range Range  // 在 code 中，字节偏移，有序且不重叠
	Kind  string // 如 "keyword"、"string"、"comment"，只含 a-z 和 -
}

// 导出的 HTML（BE-2.8）：
//   - 完整文档为 <!DOCTYPE html>，主题 CSS 内嵌在 <style>，Title 进 <title>；
//     Fragment 只输出正文，没有 <style>。
//   - 标题带 id：小写化，空白变为 -，去掉字母、数字、-、_ 以外的字符（中文保留），
//     重复的依次加 -1、-2。为空时用 "section"。TOC 为 true 时正文前有
//     <nav class="toc">，链接指向这些 id。
//   - 代码块为 <pre><code class="language-x">，有 Highlight 时内含记号 span。
//   - 段落内软换行按 SoftBreak 输出：默认 <br />，SoftBreakSpace 输出换行空白。
//   - 脚注引用为 <sup class="footnote-ref"><a href="#fn-标签">，定义集中在正文末尾的
//     <section class="footnotes">，带回链。
//   - 原始 HTML 按 02 的安全范围：不输出 <script>、<iframe>、<object>、<embed>、
//     on* 属性和 javascript: 链接，其余原样保留。
//   - EmbedImages 为 true 时，BaseDir 下能读到的本地图片以 data: URL 内嵌；读不到的
//     保留原链接，不报错。网络图片不下载。
//   - ctx 取消时返回 CodeCanceled。

// Exporter 导出文档（core/export 实现）。阻塞，禁止在主线程调用。
type Exporter interface {
	Export(ctx context.Context, t Text, ast AST, opts ExportOptions, w io.Writer) error
}

// PDFOptions 是 HTML 转 PDF 的选项。
type PDFOptions struct {
	PageSize   string
	Landscape  bool
	Margins    [4]float32
	Background bool
}

// Printer 把 HTML 排版为 PDF（app 以 WebView PrintToPDF 实现）。阻塞。
type Printer interface {
	HTMLToPDF(ctx context.Context, html, baseURL string, opts PDFOptions) ([]byte, error)
}
