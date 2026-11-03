package liveedit

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"aurink/internal/contract"
	"github.com/egoist/mygo/ui"
)

const (
	maxImageBytes = 64 << 20
	placeholderW  = 280
	placeholderH  = 120
)

type imageState struct {
	src    ui.ImageSource
	failed bool
}

// imagePath resolves an image's source against the document's folder. Only local
// files load in M1, whatever the scheme says.
func (e *Editor) imagePath(source string) string {
	if source == "" || strings.Contains(source, "://") {
		return ""
	}
	if u, err := url.PathUnescape(source); err == nil {
		source = u
	}
	if filepath.IsAbs(source) {
		return source
	}
	if e.opts.DocDir == nil {
		return ""
	}
	dir := e.opts.DocDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, filepath.FromSlash(source))
}

// image returns what is known of an image, starting its load off the main thread.
func (e *Editor) image(source string) *imageState {
	st := e.images[source]
	if st != nil {
		return st
	}
	st = &imageState{}
	if e.images == nil {
		e.images = map[string]*imageState{}
	}
	e.images[source] = st
	path := e.imagePath(source)
	if path == "" || e.opts.Dispatcher == nil {
		st.failed = true
		return st
	}
	d := e.opts.Dispatcher
	go func() {
		var src ui.ImageSource
		info, err := os.Stat(path)
		if err == nil && info.Size() <= maxImageBytes {
			var data []byte
			if data, err = os.ReadFile(path); err == nil {
				src, err = ui.DecodeImage(data)
			}
		}
		d.Post(func() {
			st.src, st.failed = src, err != nil || src == nil
			// The row's size changes with the image.
			e.dropAllForms()
			e.invalidate()
		})
	}()
	return st
}

// objectSpan reserves an inline object's box; paintObjects draws into it.
func (e *Editor) objectSpan(s *contract.Segment, m *rowMetrics) ui.Span {
	w, h := float32(placeholderW), float32(placeholderH)
	if s.Object != nil && s.Object.Kind == contract.ObjImage {
		if st := e.image(s.Object.Source); st.src != nil {
			iw, ih := ui.ImageSize(st.src)
			if iw > 0 && ih > 0 {
				w, h = iw, ih
				if w > m.width {
					w, h = m.width, ih*m.width/iw
				}
			}
		}
	} else if s.Object != nil && s.Object.Kind == contract.ObjFootnoteRef {
		return noteSpan(s, m)
	} else {
		w, h = m.font.Size, m.font.Size
	}
	return ui.Span{Text: s.Display, ObjectWidth: max(w, 1), ObjectHeight: max(h, 1)}
}

// paintObjects draws images, or their placeholder when they did not load.
func (rc *rowContext) paintObjects() {
	objs := rc.l.layout.Objects()
	if len(objs) == 0 {
		return
	}
	k := 0
	x, y := rc.textX(), rc.textY()
	for i := range rc.l.proj.Segments {
		s := &rc.l.proj.Segments[i]
		if s.Kind != contract.SegObject {
			continue
		}
		if k >= len(objs) {
			return
		}
		r := objs[k].Rect
		k++
		box := ui.Rect{X: x + r.X, Y: y + r.Y, W: r.W, H: r.H}
		if s.Object != nil && s.Object.Kind == contract.ObjFootnoteRef {
			rc.paintNote(box, s.Object.Source)
			continue
		}
		if s.Object == nil || s.Object.Kind != contract.ObjImage {
			continue
		}
		st := rc.e.image(s.Object.Source)
		if st.src != nil && rc.e.flying(rc.row, s.Object.Source) {
			continue
		}
		if st.src != nil {
			rc.p.Image(st.src, box, ui.Contain)
			continue
		}
		rc.p.Fill(box, rc.t.Surface, 8)
		rc.p.Stroke(box, rc.t.Border, 8, 1)
		if st.failed {
			text := s.Object.Alt
			if text == "" {
				text = s.Object.Source
			}
			rc.p.TextLayout(rc.e.label(text, 12), box.X+12, box.Y+12, rc.t.Muted)
		}
	}
}
