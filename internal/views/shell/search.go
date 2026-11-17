package shell

import (
	"context"
	"path/filepath"
	"strconv"

	"aurink/internal/contract"
	"aurink/internal/views/theme"
	"aurink/locales"
	"github.com/egoist/mygo/ui"
)

// searchMax caps the matches the panel keeps: past it the search stops and
// says so. searchPerFile caps one file's.
const (
	searchMax     = 2000
	searchPerFile = 100
)

// wsearch is the sidebar's search over the open folder (FE-2.3). Each change
// of the query or its options cancels the running search and starts another;
// its batches stream in on the main thread, and those of a canceled one are
// dropped by their generation.
type wsearch struct {
	query                   string
	caseSensitive, word, re bool
	focus                   bool

	gen     int
	cancel  context.CancelFunc
	running bool
	// ran is what the results answer, to restart only on a change.
	ran     contract.Query
	ranRoot string

	results []contract.FileMatches
	matches int
	capped  bool
	err     string
	folded  map[string]bool
	rows    []searchRow
	list    ui.ListState
	// Built counts the rows built in the last frame, for tests.
	Built int
}

// searchRow is a file's header (match < 0) or one of its matches.
type searchRow struct{ file, match int }

func (w *wsearch) layout() {
	// A new slice: a click folding a file lays out again while the list still
	// walks the rows it began with.
	w.rows = make([]searchRow, 0, len(w.rows))
	for fi, f := range w.results {
		w.rows = append(w.rows, searchRow{fi, -1})
		if w.folded[f.Path] {
			continue
		}
		for mi := range f.Matches {
			w.rows = append(w.rows, searchRow{fi, mi})
		}
	}
}

func (w *wsearch) stop() {
	if w.cancel != nil {
		w.cancel()
		w.cancel = nil
	}
	w.running = false
}

// restart searches again when the query, its options or the folder changed.
func (s *Shell) restartSearch() {
	w := &s.search
	q := contract.Query{Text: w.query, CaseSensitive: w.caseSensitive, WholeWord: w.word, Regex: w.re}
	root := ""
	if s.o.Workspace != nil {
		root = s.o.Workspace.Root()
	}
	if q == w.ran && root == w.ranRoot {
		return
	}
	w.ran, w.ranRoot = q, root
	w.stop()
	w.gen++
	w.results, w.matches, w.capped, w.err = nil, 0, false, ""
	w.layout()
	if q.Text == "" || root == "" || s.o.Search == nil {
		return
	}
	gen := w.gen
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel, w.running = cancel, true
	opts := contract.SearchOptions{Ignore: s.o.Workspace.Ignored, PerFile: searchPerFile}
	post := s.o.Dispatcher.Post
	finder := s.o.Search
	go func() {
		err := finder.Search(ctx, root, q, opts, func(batch []contract.FileMatches) {
			post(func() {
				if gen != w.gen {
					return
				}
				for _, f := range batch {
					if w.matches >= searchMax {
						w.capped = true
						w.stop()
						break
					}
					if n := searchMax - w.matches; len(f.Matches) > n {
						f.Matches, w.capped = f.Matches[:n], true
					}
					w.results = append(w.results, f)
					w.matches += len(f.Matches)
				}
				w.layout()
				s.invalidate()
			})
		})
		post(func() {
			if gen != w.gen {
				return
			}
			w.running, w.cancel = false, nil
			if err != nil && contract.CodeOf(err) != contract.CodeCanceled {
				if contract.CodeOf(err) == contract.CodeInvalid && q.Regex {
					w.err = locales.ShellSearchBadRegex()
				} else {
					w.err = err.Error()
				}
			}
			cancel()
			s.invalidate()
		})
	}()
}

// SearchWorkspace shows the search panel with the focus in its field.
func (s *Shell) searchWorkspace() {
	s.showPanel(panelSearch)
	s.search.focus = true
}

func (s *Shell) searchPanel(c *ui.Context, t *theme.Tokens) {
	w := &s.search
	root := ""
	if s.o.Workspace != nil {
		root = s.o.Workspace.Root()
	}
	ui.Column(c).Padding(4, 8, 6).Gap(6).Children(func() {
		f := ui.SearchField(c.Key("query"), &w.query).Placeholder(locales.ShellSearchPlaceholder()).
			Label(locales.ShellSearchPlaceholder()).FillWidth()
		if w.focus {
			w.focus = false
			f.Focus()
		}
		ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
			s.searchToggle(c.Key("case"), t, "Aa", locales.ShellSearchCase(), &w.caseSensitive)
			s.searchToggle(c.Key("word"), t, "W", locales.ShellSearchWord(), &w.word)
			s.searchToggle(c.Key("re"), t, ".*", locales.ShellSearchRegex(), &w.re)
			ui.Box(c).Grow(1)
			if status := s.searchStatus(root); status != "" {
				ui.Text(c, status).FontSize(theme.UIBadge).TextColor(t.Muted).SingleLine()
			}
		})
		if w.err != "" {
			ui.Text(c, w.err).FontSize(theme.UIBadge).TextColor(t.Danger)
		}
	})
	s.restartSearch()
	w.Built = 0
	rows := w.rows
	ui.List(c.Key("results"), &w.list, len(rows), func(i int) {
		w.Built++
		r := rows[i]
		if r.match < 0 {
			s.searchFileRow(c.Key("f"+strconv.Itoa(r.file)), t, root, r.file)
		} else {
			s.searchMatchRow(c.Key("m"+strconv.Itoa(r.file)+"."+strconv.Itoa(r.match)), t, r.file, r.match)
		}
	}).Grow(1).MinHeight(0).Padding(0, 8, 8)
}

func (s *Shell) searchStatus(root string) string {
	w := &s.search
	switch {
	case root == "":
		return locales.ShellNoFolder()
	case w.query == "":
		return ""
	case w.running:
		return locales.ShellSearching()
	case w.err != "":
		return ""
	case w.matches == 0:
		return locales.ShellSearchNone()
	case w.capped:
		return locales.ShellSearchCapped(strconv.Itoa(w.matches))
	}
	return locales.ShellSearchCount(strconv.Itoa(w.matches), strconv.Itoa(len(w.results)))
}

func (s *Shell) searchToggle(c *ui.Context, t *theme.Tokens, text, label string, on *bool) {
	b := ui.Row(c).Height(22).MinWidth(26).Padding(0, 6).Radius(5).AlignItems(ui.Center).Justify(ui.Center).
		Role(ui.RoleToggleButton).Checked(*on).Label(label).Tooltip(label).KeepFocus()
	switch {
	case *on:
		b.Background(t.AccentSoft)
	case b.Hovered():
		b.Background(t.Hover)
	}
	b.Children(func() {
		x := ui.Text(c, text).FontSize(theme.UIBadge).FontWeight(600)
		if *on {
			x.TextColor(t.Accent)
		} else {
			x.TextColor(t.Muted)
		}
	})
	if b.Clicked() {
		*on = !*on
		s.invalidate()
	}
}

func (s *Shell) searchFileRow(c *ui.Context, t *theme.Tokens, root string, fi int) {
	w := &s.search
	f := w.results[fi]
	rel, err := filepath.Rel(root, filepath.Dir(f.Path))
	if err != nil || rel == "." {
		rel = ""
	}
	row := ui.Row(c).Height(26).FillWidth().Padding(0, 6).Gap(6).AlignItems(ui.Center).Radius(theme.RadiusRow).
		Role(ui.RoleButton).Label(filepath.Base(f.Path)).Expanded(!w.folded[f.Path]).Focusable()
	if row.Hovered() {
		row.Background(t.Hover)
	}
	row.Children(func() {
		ui.Text(c, filepath.Base(f.Path)).FontSize(theme.UIText).FontWeight(600).SingleLine().Shrink(0)
		ui.Text(c, filepath.ToSlash(rel)).FontSize(theme.UIBadge).TextColor(t.Faint).SingleLine().Grow(1).Shrink(1).MinWidth(0)
		ui.Text(c, strconv.Itoa(len(f.Matches))).FontSize(theme.UIBadge).TextColor(t.Muted).
			Padding(1, 6).Radius(8).Background(t.Surface)
	})
	if row.Clicked() {
		if w.folded == nil {
			w.folded = map[string]bool{}
		}
		w.folded[f.Path] = !w.folded[f.Path]
		w.layout()
		s.invalidate()
	}
}

func (s *Shell) searchMatchRow(c *ui.Context, t *theme.Tokens, fi, mi int) {
	f := s.search.results[fi]
	m := f.Matches[mi]
	row := ui.Row(c).Height(24).FillWidth().Padding(0, 6).PaddingStart(18).AlignItems(ui.Center).Radius(theme.RadiusRow).
		Role(ui.RoleButton).Label(m.Preview).Focusable()
	if row.Hovered() {
		row.Background(t.Hover)
	}
	row.Children(func() {
		pre, hit, post := splitHit(m.Preview, m.Hit)
		ui.Row(c).Shrink(1).MinWidth(0).ClipX().Children(func() {
			ui.Text(c.Key(0), pre).FontSize(theme.UIBadge + 1).TextColor(t.Muted).NoWrap()
			ui.Text(c.Key(1), hit).FontSize(theme.UIBadge + 1).NoWrap().Background(t.Mark).Radius(2)
			ui.Text(c.Key(2), post).FontSize(theme.UIBadge + 1).TextColor(t.Muted).NoWrap()
		})
	})
	if row.Clicked() {
		s.openAt(f.Path, m.Range.Start)
	}
}

// splitHit cuts a preview around its hit, keeping a little of the text before
// it so a hit far along the line still shows.
func splitHit(preview string, hit contract.Range) (pre, mid, post string) {
	start, end := max(0, min(hit.Start, len(preview))), max(0, min(hit.End, len(preview)))
	end = max(start, end)
	pre, mid, post = preview[:start], preview[start:end], preview[end:]
	const keep = 24
	if len(pre) > keep {
		cut := len(pre) - keep
		for cut < len(pre) && pre[cut]&0xC0 == 0x80 {
			cut++
		}
		pre = "…" + pre[cut:]
	}
	return
}

// openAt opens a file, or goes to its tab, and puts the caret at off.
func (s *Shell) openAt(path string, off int) {
	s.o.Sessions.Open(path, func(x contract.Session, err error) {
		if err != nil {
			s.fail(err)
			return
		}
		d := s.attach(x)
		s.activate(d)
		s.goToOffset(d, off)
	})
}

// Search is the search panel's state, for tests.
func (s *Shell) SearchRowsBuilt() int { return s.search.Built }

// SearchResults are the files and matches the search panel holds, for tests.
func (s *Shell) SearchResults() []contract.FileMatches { return s.search.results }

// SetSearch types a query into the search panel, for tests.
func (s *Shell) SetSearch(q string) {
	s.search.query = q
	s.invalidate()
}
