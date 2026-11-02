package editing

import (
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"

	"aurink/internal/contract"
)

func insertTable(ctx contract.EditContext, cmd contract.EditCommand) (contract.Transaction, bool, error) {
	if cmd.Rows < 1 || cmd.Cols < 1 || cmd.Rows > 10000 || cmd.Cols > 1000 || selectionHas(ctx, contract.BlockTable, contract.BlockCode, contract.BlockHTML, contract.BlockFrontMatter, contract.BlockMath) {
		return contract.Transaction{}, false, nil
	}
	r := ctx.Selection.Primary().Range()
	if !r.Empty() {
		return contract.Transaction{}, false, nil
	}
	row := "|" + strings.Repeat("  |", cmd.Cols)
	separator := "|" + strings.Repeat(" --- |", cmd.Cols)
	var b strings.Builder
	b.WriteString(row)
	b.WriteString(eol(ctx) + separator)
	for range cmd.Rows {
		b.WriteString(eol(ctx) + row)
	}
	l := lineAt(ctx, r.Start)
	at, prefix := r.Start, ""
	if strings.TrimSpace(l.s) != "" {
		at = l.end
		prefix = eol(ctx) + eol(ctx)
	}
	s := prefix + b.String()
	return transaction([]contract.TextEdit{edit(at, at, s)}, contract.Caret(at+len(prefix)+2), contract.OriginCommand, "table"), true, nil
}

var footnoteLabel = regexp.MustCompile(`\[\^(\d+)\]`)

func insertCommand(ctx contract.EditContext, cmd contract.EditCommand) (contract.Transaction, bool, error) {
	r := ctx.Selection.Primary().Range()
	switch cmd.ID {
	case contract.CmdInsertImages:
		if len(cmd.Paths) == 0 {
			return contract.Transaction{}, false, nil
		}
		images := make([]string, 0, len(cmd.Paths))
		for _, p := range cmd.Paths {
			if p == "" || strings.ContainsAny(p, "\r\n") {
				return contract.Transaction{}, false, contract.E(contract.CodeInvalid, "editing.Run", nil)
			}
			name := path.Base(strings.ReplaceAll(p, "\\", "/"))
			if decoded, err := url.PathUnescape(name); err == nil {
				name = decoded
			}
			name = strings.TrimSuffix(name, path.Ext(name))
			name = strings.NewReplacer("\\", "\\\\", "[", "\\[", "]", "\\]").Replace(name)
			images = append(images, "!["+name+"]("+linkDestination(p)+")")
		}
		return replace(r, strings.Join(images, eol(ctx)), contract.OriginCommand, "insert"), true, nil
	case contract.CmdInsertFootnote:
		if selectionHas(ctx, contract.BlockCode, contract.BlockTable, contract.BlockHTML, contract.BlockFrontMatter, contract.BlockMath) {
			return contract.Transaction{}, false, nil
		}
		src := ctx.Text.String()
		used := map[int]bool{}
		for _, m := range footnoteLabel.FindAllStringSubmatch(src, -1) {
			if n, err := strconv.Atoi(m[1]); err == nil {
				used[n] = true
			}
		}
		n := 1
		for used[n] {
			n++
		}
		label := strconv.Itoa(n)
		ref, def := "[^"+label+"]", "[^"+label+"]: "
		at := len(src)
		if strings.HasSuffix(src, "\r\n") {
			at -= 2
		} else if strings.HasSuffix(src, "\n") {
			at--
		}
		if at < r.End {
			at = len(src)
		}
		prefix := eol(ctx) + eol(ctx)
		if at > 0 && src[at-1] == '\n' {
			prefix = eol(ctx)
		}
		if at == 0 {
			prefix = ""
		}
		if at == r.End {
			s := ref + prefix + def
			return replace(r, s, contract.OriginCommand, "insert"), true, nil
		}
		edits := []contract.TextEdit{edit(r.Start, r.End, ref), edit(at, at, prefix+def)}
		caret := at + len(ref) - r.Len() + len(prefix) + len(def)
		return transaction(edits, contract.Caret(caret), contract.OriginCommand, "insert"), true, nil
	}
	return contract.Transaction{}, false, nil
}
