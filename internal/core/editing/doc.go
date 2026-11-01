// Package editing turns edit intents and commands into transactions as pure functions.
//
// New constructs a stateless Editor; its zero value is also usable. Every call
// receives its text, AST, primary selection, file metadata and preferences in an
// EditContext. The caller applies the returned transaction and then calls
// FollowUp with the updated context to obtain an independent input-rule step.
// New source uses the dominant line ending; existing source is never normalized.
//
// Markdown and plain-text paste are supported. Rich paste and table commands
// belong to BE-1.6. Grapheme deletion handles combining marks, emoji sequences,
// flags and Hangul locally; it does not implement all Unicode grapheme rules.
package editing
