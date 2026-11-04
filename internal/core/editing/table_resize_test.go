package editing

import (
	"fmt"
	"strings"
	"testing"

	"aurink/internal/contract"
	"aurink/internal/core/document"
)

func TestTableResizeDeleteAndBackspace(t *testing.T) {
	resize := func(rows, cols int) contract.EditCommand {
		return contract.EditCommand{ID: contract.CmdTableResize, Rows: rows, Cols: cols}
	}
	back := contract.Intent{Kind: contract.IntentDeleteBackward}
	cases := []struct {
		name, before, after string
		cmd                 contract.EditCommand
		intent              contract.Intent
		changed             bool
		rows, cols          int
	}{
		{
			name:   "grow preserves cell bytes and alignment",
			before: "|  a¦  |  b\\|c |\n| :--- | ---: |\n| 1 | two\t |",
			after:  "|  a¦  |  b\\|c |  |\n| :--- | ---: | --- |\n| 1 | two\t |  |\n|  |  |  |",
			cmd:    resize(2, 3), changed: true, rows: 2, cols: 3,
		},
		{
			name:   "shrink removed row and column",
			before: "| a | b | c |\n| :--- | :-: | ---: |\n| d | e | f |\n| g | h | i¦ |",
			after:  "| a | b |\n| :--- | :-: |\n| d | ¦e |",
			cmd:    resize(1, 2), changed: true, rows: 1, cols: 2,
		},
		{
			name:   "shrink column in same row",
			before: "| a | b¦c |\n| :--- | ---: |\n| d | e |",
			after:  "| ¦a |\n| :--- |\n| d |",
			cmd:    resize(1, 1), changed: true, rows: 1, cols: 1,
		},
		{
			name:   "shrink row keeps column",
			before: "| a | b |\n| --- | :-: |\n| d | e |\n| f | g¦ |",
			after:  "| a | b |\n| --- | :-: |\n| d | ¦e |",
			cmd:    resize(1, 2), changed: true, rows: 1, cols: 2,
		},
		{
			name:   "shrink to header",
			before: "| a | b |\n| --- | :-: |\n| d | e¦ |",
			after:  "| a | ¦b |\n| --- | :-: |",
			cmd:    resize(0, 2), changed: true, rows: 0, cols: 2,
		},
		{
			name:   "grow from header alone",
			before: "| a¦ | b |\n| --- | :-: |", after: "| a¦ | b |\n| --- | :-: |\n|  |  |",
			cmd: resize(1, 2), changed: true, rows: 1, cols: 2,
		},
		{
			name:   "one column without outer pipes",
			before: "¦a|b\n---|:-:\nx|y", after: "¦a|\n---|\nx|",
			cmd: resize(1, 1), changed: true, rows: 1, cols: 1,
		},
		{
			name:   "grow without outer pipes",
			before: "¦a|b\n---|:-:\nx|y", after: "¦a|b|  |\n---|:-:| --- \nx|y|  |",
			cmd: resize(1, 3), changed: true, rows: 1, cols: 3,
		},
		{
			name:   "removed cell moves to implicit surviving cell",
			before: "| a | b | c |\n| --- | --- | --- |\n| d |\n| e | f | g¦ |",
			after:  "| a | b | c |\n| --- | --- | --- |\n| d |  | ¦ |",
			cmd:    resize(1, 3), changed: true, rows: 1, cols: 3,
		},
		{
			name:   "delete preserves surrounding blank lines",
			before: "| a | b |\n| --- | :-: |\n| 1 | 2¦ |", after: "¦",
			cmd: contract.EditCommand{ID: contract.CmdTableDelete}, changed: true,
		},
		{
			name:   "backspace moves within a row",
			before: "| a | ¦ |\n| --- | --- |\n| 1 | 2 |", after: "| a¦ |  |\n| --- | --- |\n| 1 | 2 |",
			intent: back,
		},
		{
			name:   "backspace skips delimiter",
			before: "| a | b |\n| --- | --- |\n| ¦ | 2 |", after: "| a | b¦ |\n| --- | --- |\n|  | 2 |",
			intent: back,
		},
		{
			name:   "backspace materializes implicit previous cell",
			before: "| a | b | c |\n| --- | --- | --- |\n| d |\n| ¦ | e | f |",
			after:  "| a | b | c |\n| --- | --- | --- |\n| d |  | ¦ |\n|  | e | f |",
			intent: back, changed: true,
		},
		{
			name:   "first empty cell with text elsewhere stays",
			before: "| ¦ |  |\n| --- | --- |\n|  | text |", after: "| ¦ |  |\n| --- | --- |\n|  | text |",
			intent: back,
		},
		{
			name:   "first empty cell with header text stays",
			before: "| ¦ | text |\n| --- | --- |\n|  |  |", after: "| ¦ | text |\n| --- | --- |\n|  |  |",
			intent: back,
		},
		{
			name:   "backspace at start of text stays",
			before: "| ¦text |  |\n| --- | --- |\n|  |  |", after: "| ¦text |  |\n| --- | --- |\n|  |  |",
			intent: back,
		},
		{
			name:   "backspace deletes empty table",
			before: "| ¦ |  |\n| --- | --- |\n|  |  |", after: "¦",
			intent: back, changed: true,
		},
	}
	containers := []struct{ name, first, rest string }{
		{"plain", "", ""}, {"quote", "> ", "> "}, {"list", "- ", "  "},
	}
	for _, tc := range cases {
		for _, container := range containers {
			for _, nl := range []string{"\n", "\r\n"} {
				t.Run(fmt.Sprintf("%s/%s/%q", tc.name, container.name, nl), func(t *testing.T) {
					wrap := func(s string) string {
						lines := strings.Split(s, "\n")
						for i := range lines {
							if i == 0 {
								lines[i] = container.first + lines[i]
							} else {
								lines[i] = container.rest + lines[i]
							}
						}
						return "x" + nl + nl + strings.Join(lines, nl) + nl + nl + "y"
					}
					src, sel := marked(wrap(tc.before))
					ctx := contextFor(t, src, sel, nl)
					model := (document.Factory{}).New("table", src, ctx.Meta, ctx.Prefs.Exts)
					model.SetSelection(sel)
					ctx.Text, ctx.AST = model.Text(), model.AST()
					var tx contract.Transaction
					var ok bool
					if tc.cmd.ID != "" {
						var err error
						tx, ok, err = New().Run(ctx, tc.cmd)
						if err != nil {
							t.Fatal(err)
						}
					} else {
						tx, ok = New().HandleIntent(ctx, tc.intent)
					}
					if !ok || (len(tx.Edits) > 0) != tc.changed {
						t.Fatalf("handled=%v, transaction=%+v", ok, tx)
					}
					if _, err := model.Apply(tx); err != nil {
						t.Fatal(err)
					}
					if got, want := show(model.Text().String(), model.Selection()), wrap(tc.after); got != want {
						t.Fatalf("got %q, want %q", got, want)
					}
					if tc.cmd.ID == contract.CmdTableResize {
						ctx.Text, ctx.AST, ctx.Selection = model.Text(), model.AST(), model.Selection()
						table, found := findTable(ctx)
						if !found || len(table.rows)-1 != tc.rows || len(table.rows[0].cells) != tc.cols {
							t.Fatalf("resized table is not %d x %d: %+v", tc.rows, tc.cols, table)
						}
					}
					if tc.changed {
						if !model.Undo() || model.CanUndo() || model.Text().String() != src || show(src, model.Selection()) != wrap(tc.before) {
							t.Fatal("one undo did not restore the original text and selection")
						}
					} else if model.CanUndo() {
						t.Fatal("caret movement or no-op added an undo step")
					}
				})
			}
		}
	}
}

func TestTableResizeInvalidDimensions(t *testing.T) {
	src, sel := marked("| ¦a | b |\n| --- | :-: |\n| 1 | 2 |")
	ctx := contextFor(t, src, sel, "\n")
	for _, size := range [][2]int{{-1, 2}, {1, 0}, {1, -1}, {1, 2}} {
		if _, ok, err := New().Run(ctx, contract.EditCommand{ID: contract.CmdTableResize, Rows: size[0], Cols: size[1]}); ok || err != nil {
			t.Errorf("resize %v: ok=%v err=%v", size, ok, err)
		}
	}
}

func TestBackspaceThroughEmptyTable(t *testing.T) {
	for _, nl := range []string{"\n", "\r\n"} {
		for _, c := range []struct{ name, first, rest string }{
			{"plain", "", ""}, {"quote", "> ", "> "}, {"list", "- ", "  "},
		} {
			t.Run(fmt.Sprintf("%s/%q", c.name, nl), func(t *testing.T) {
				wrap := func(s string) string {
					lines := strings.Split(s, "\n")
					for i := range lines {
						if i == 0 {
							lines[i] = c.first + lines[i]
						} else {
							lines[i] = c.rest + lines[i]
						}
					}
					return "x" + nl + nl + strings.Join(lines, nl) + nl + nl + "y"
				}
				before := "|  |  |\n| --- | --- |\n|  | ¦ |"
				src, sel := marked(wrap(before))
				ctx := contextFor(t, src, sel, nl)
				model := (document.Factory{}).New("table", src, ctx.Meta, ctx.Prefs.Exts)
				model.SetSelection(sel)
				for i, want := range []string{
					"|  |  |\n| --- | --- |\n| ¦ |  |",
					"|  | ¦ |\n| --- | --- |\n|  |  |",
					"| ¦ |  |\n| --- | --- |\n|  |  |",
					"¦",
				} {
					ctx.Text, ctx.AST, ctx.Selection = model.Text(), model.AST(), model.Selection()
					tx, ok := New().HandleIntent(ctx, contract.Intent{Kind: contract.IntentDeleteBackward})
					if !ok || (len(tx.Edits) != 0) != (i == 3) {
						t.Fatalf("step %d: handled=%v, transaction=%+v", i, ok, tx)
					}
					if _, err := model.Apply(tx); err != nil {
						t.Fatal(err)
					}
					if got := show(model.Text().String(), model.Selection()); got != wrap(want) {
						t.Fatalf("step %d: got %q, want %q", i, got, wrap(want))
					}
				}
				if !model.Undo() || model.CanUndo() || model.Text().String() != src {
					t.Fatal("one undo did not restore the table")
				}
			})
		}
	}
}
