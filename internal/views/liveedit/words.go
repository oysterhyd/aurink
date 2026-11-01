package liveedit

import (
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		n += utf16.RuneLen(r)
	}
	return n
}

// utf16ByteOffset returns the byte of s at UTF-16 unit u, keeping a surrogate
// pair whole.
func utf16ByteOffset(s string, u int) int {
	n := 0
	for i := 0; i < len(s); {
		if n >= u {
			return i
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		n += utf16.RuneLen(r)
		i += size
	}
	return len(s)
}

type wordClass uint8

const (
	classSpace wordClass = iota
	classPunct
	classWord
	classHan
	classKana
)

func classOf(r rune) wordClass {
	switch {
	case unicode.IsSpace(r):
		return classSpace
	case unicode.Is(unicode.Han, r):
		return classHan
	case unicode.In(r, unicode.Hiragana, unicode.Katakana):
		return classKana
	case r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r):
		return classWord
	}
	return classPunct
}

// nextWord returns the UTF-16 index after the word that follows unit u of s:
// spaces are skipped, then one run of a class.
func nextWord(s string, u int) int {
	i := utf16ByteOffset(s, u)
	for i < len(s) {
		r, size := utf8.DecodeRuneInString(s[i:])
		if classOf(r) != classSpace {
			break
		}
		i += size
	}
	i = runEnd(s, i)
	return utf16Len(s[:i])
}

// prevWord returns the UTF-16 index of the start of the word before unit u of s.
func prevWord(s string, u int) int {
	i := utf16ByteOffset(s, u)
	for i > 0 {
		r, size := utf8.DecodeLastRuneInString(s[:i])
		if classOf(r) != classSpace {
			break
		}
		i -= size
	}
	i = runStart(s, i)
	return utf16Len(s[:i])
}

// wordAt returns the UTF-16 range of the run at unit u of s, what a double click
// selects. At the end of s, or of a line, it takes the run before.
func wordAt(s string, u int) (from, to int) {
	i := utf16ByteOffset(s, u)
	if i >= len(s) || s[i] == '\n' {
		if i == 0 {
			return u, u
		}
		_, size := utf8.DecodeLastRuneInString(s[:i])
		i -= size
	}
	return utf16Len(s[:runStart(s, i)]), utf16Len(s[:runEnd(s, i)])
}

func runEnd(s string, i int) int {
	if i >= len(s) {
		return i
	}
	r, size := utf8.DecodeRuneInString(s[i:])
	c := classOf(r)
	for i += size; i < len(s); i += size {
		r, size = utf8.DecodeRuneInString(s[i:])
		if classOf(r) != c {
			break
		}
	}
	return i
}

// runStart returns the start of the run holding the rune before byte i, or, when
// i is inside a run, of that run.
func runStart(s string, i int) int {
	if i >= len(s) {
		if i == 0 {
			return 0
		}
		r, _ := utf8.DecodeLastRuneInString(s)
		return runBack(s, i, classOf(r))
	}
	r, _ := utf8.DecodeRuneInString(s[i:])
	back := runBack(s, i, classOf(r))
	if back == i && i > 0 {
		p, _ := utf8.DecodeLastRuneInString(s[:i])
		return runBack(s, i, classOf(p))
	}
	return back
}

func runBack(s string, i int, c wordClass) int {
	for i > 0 {
		r, size := utf8.DecodeLastRuneInString(s[:i])
		if classOf(r) != c {
			break
		}
		i -= size
	}
	return i
}
