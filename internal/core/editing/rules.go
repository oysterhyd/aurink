package editing

import "aurink/internal/contract"

// FollowUp supplies the independent input-rule transaction after typing.
func (Editor) FollowUp(ctx contract.EditContext) (contract.Transaction, bool) {
	if !valid(ctx) || !ctx.Selection.Primary().Empty() {
		return contract.Transaction{}, false
	}
	p := ctx.Selection.Primary().Head
	l := lineAt(ctx, p)
	if rawAt(ctx, p) || ancestor(ctx, p, contract.BlockTable) != nil {
		return contract.Transaction{}, false
	}
	tail := l.s[l.content:]
	if p == l.end && l.list < 0 && (tail == "[ ] " || tail == "[x] " || tail == "[X] ") {
		at := l.start + l.content
		return transaction([]contract.TextEdit{edit(at, at, bullet(ctx)+" ")}, contract.Caret(p+2), contract.OriginInputRule, "input_rule"), true
	}
	return contract.Transaction{}, false
}
