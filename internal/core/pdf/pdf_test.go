package pdf

import (
	"context"
	"errors"
	"io"
	"math"
	"strings"
	"testing"
	"time"

	"aurink/internal/contract"
	"aurink/internal/core/markdown"
	"aurink/internal/core/text"
)

type exportFunc func(context.Context, contract.Text, contract.AST, contract.ExportOptions, io.Writer) error

func (f exportFunc) Export(ctx context.Context, t contract.Text, a contract.AST, o contract.ExportOptions, w io.Writer) error {
	return f(ctx, t, a, o, w)
}

type printFunc func(context.Context, string, string, contract.PDFOptions) ([]byte, error)

func (f printFunc) HTMLToPDF(ctx context.Context, html, base string, o contract.PDFOptions) ([]byte, error) {
	return f(ctx, html, base, o)
}

type renderFunc func(context.Context, contract.RenderRequest) (contract.RenderResult, error)

func (f renderFunc) Available() bool { return true }
func (f renderFunc) Render(ctx context.Context, req contract.RenderRequest) (contract.RenderResult, error) {
	return f(ctx, req)
}

func fixture(t *testing.T) (contract.Text, contract.AST) {
	t.Helper()
	src := "$x$ and $y$\n"
	p, err := markdown.New(src, 1, contract.ExtDefault)
	if err != nil {
		t.Fatal(err)
	}
	return text.New(src, 1), p.Snapshot()
}

func TestExportOptionsAndProgress(t *testing.T) {
	txt, ast := fixture(t)
	var calls []float32
	r := renderFunc(func(context.Context, contract.RenderRequest) (contract.RenderResult, error) {
		return contract.RenderResult{}, nil
	})
	e := exportFunc(func(ctx context.Context, _ contract.Text, _ contract.AST, o contract.ExportOptions, w io.Writer) error {
		if o.Format != contract.ExportHTML || o.Fragment || o.Dark || !o.EmbedImages || o.Renderer == nil {
			t.Errorf("export options: %+v", o)
		}
		o.OnProgress(0)
		_, _ = o.Renderer.Render(ctx, contract.RenderRequest{Kind: contract.RenderMathInline, Source: "x"})
		_, _ = o.Renderer.Render(ctx, contract.RenderRequest{Kind: contract.RenderMathInline, Source: "y"})
		o.OnProgress(1)
		_, _ = io.WriteString(w, "<html>test</html>")
		return nil
	})
	p := printFunc(func(_ context.Context, html, base string, o contract.PDFOptions) ([]byte, error) {
		if html != "<html>test</html>" || !strings.HasPrefix(base, "file:///") || !strings.HasSuffix(base, "/") {
			t.Errorf("html %q base %q", html, base)
		}
		if o.PageSize != "A4" || o.Margins != [4]float32{.6, .6, .6, .6} || !o.Background {
			t.Errorf("print options: %+v", o)
		}
		return []byte("pdf"), nil
	})
	got, err := Export(context.Background(), txt, ast, contract.ExportOptions{Format: contract.ExportPDF, Dark: true, Fragment: true, Renderer: r, BaseDir: t.TempDir(), OnProgress: func(f float32) { calls = append(calls, f) }}, e, p)
	if err != nil || string(got) != "pdf" {
		t.Fatalf("result %q, error %v", got, err)
	}
	if len(calls) < 4 || calls[0] != 0 || calls[len(calls)-1] != 1 {
		t.Fatalf("progress %v", calls)
	}
	for i := 1; i < len(calls); i++ {
		if calls[i] < calls[i-1] {
			t.Fatalf("progress %v", calls)
		}
	}
	for _, step := range []float32{.25, .5} {
		found := false
		for _, got := range calls {
			if math.Abs(float64(got-step)) < .00001 {
				found = true
			}
		}
		if !found {
			t.Fatalf("render-weighted step %v missing from %v", step, calls)
		}
	}
}

func TestLetterMarginsAndEmptyBase(t *testing.T) {
	txt, ast := fixture(t)
	called := false
	e := exportFunc(func(_ context.Context, _ contract.Text, _ contract.AST, _ contract.ExportOptions, w io.Writer) error {
		called = true
		_, _ = io.WriteString(w, "html")
		return nil
	})
	p := printFunc(func(_ context.Context, _ string, base string, opts contract.PDFOptions) ([]byte, error) {
		if base != "" || opts.PageSize != "Letter" || opts.Margins != [4]float32{.3, .4, .5, .6} {
			t.Errorf("base=%q options=%+v", base, opts)
		}
		return []byte("pdf"), nil
	})
	data, err := Export(context.Background(), txt, ast, contract.ExportOptions{PageSize: "Letter", Margins: [4]float32{.3, .4, .5, .6}}, e, p)
	if err != nil || string(data) != "pdf" || !called {
		t.Fatalf("data=%q err=%v called=%v", data, err, called)
	}
	for i := range 4 {
		margins := [4]float32{.3, .4, .5, .6}
		margins[i] = -0.1
		called = false
		data, err = Export(context.Background(), txt, ast, contract.ExportOptions{Margins: margins}, e, p)
		if contract.CodeOf(err) != contract.CodeInvalid || data != nil || called {
			t.Fatalf("margin %v: data=%q err=%v called=%v", margins, data, err, called)
		}
	}
}

func TestFailuresAndCancellation(t *testing.T) {
	txt, ast := fixture(t)
	printer := printFunc(func(context.Context, string, string, contract.PDFOptions) ([]byte, error) { return []byte("pdf"), nil })
	exporter := exportFunc(func(_ context.Context, _ contract.Text, _ contract.AST, _ contract.ExportOptions, w io.Writer) error {
		_, _ = io.WriteString(w, "html")
		return nil
	})
	for _, size := range []string{"legal", "a4"} {
		if data, err := Export(context.Background(), txt, ast, contract.ExportOptions{PageSize: size}, exporter, printer); contract.CodeOf(err) != contract.CodeInvalid || data != nil {
			t.Fatalf("size %q: %q %v", size, data, err)
		}
	}
	for _, failure := range []string{"export", "print"} {
		var progress []float32
		e, p := exporter, printer
		if failure == "export" {
			e = func(context.Context, contract.Text, contract.AST, contract.ExportOptions, io.Writer) error {
				return errors.New("export failed")
			}
		}
		if failure == "print" {
			p = func(context.Context, string, string, contract.PDFOptions) ([]byte, error) {
				return nil, errors.New("print failed")
			}
		}
		data, err := Export(context.Background(), txt, ast, contract.ExportOptions{OnProgress: func(f float32) { progress = append(progress, f) }}, e, p)
		if err == nil || data != nil || len(progress) == 0 || progress[len(progress)-1] == 1 {
			t.Fatalf("%s: data=%q err=%v progress=%v", failure, data, err, progress)
		}
	}
	for _, phase := range []string{"export", "print"} {
		t.Run(phase, func(t *testing.T) {
			entered := make(chan struct{})
			e, p := exporter, printer
			if phase == "export" {
				e = func(ctx context.Context, _ contract.Text, _ contract.AST, _ contract.ExportOptions, _ io.Writer) error {
					close(entered)
					<-ctx.Done()
					return ctx.Err()
				}
			}
			if phase == "print" {
				p = func(ctx context.Context, _ string, _ string, _ contract.PDFOptions) ([]byte, error) {
					close(entered)
					<-ctx.Done()
					return nil, ctx.Err()
				}
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			result := make(chan error, 1)
			go func() {
				data, err := Export(ctx, txt, ast, contract.ExportOptions{}, e, p)
				if data != nil {
					result <- errors.New("canceled export returned bytes")
					return
				}
				result <- err
			}()
			select {
			case <-entered:
			case <-time.After(10 * time.Second):
				t.Fatal("phase not entered")
			}
			cancel()
			select {
			case err := <-result:
				if contract.CodeOf(err) != contract.CodeCanceled {
					t.Fatalf("error %v", err)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("cancellation did not return")
			}
		})
	}
}
