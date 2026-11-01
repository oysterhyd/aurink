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
	Color    string // 文本颜色 "#RRGGBB"
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
	// Fragment 为 true 时只输出正文片段（复制为 HTML）。
	Fragment bool
	BaseDir  string     // 解析相对图片路径
	PageSize string     // PDF："A4"、"Letter"
	Margins  [4]float32 // PDF 页边距（英寸）：上、右、下、左
	// OnProgress 可为 nil；在 Export 的调用 goroutine 上以 0–1 回调。
	OnProgress func(fraction float32)
}

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
