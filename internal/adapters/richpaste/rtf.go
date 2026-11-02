package richpaste

import (
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
)

type rtfState struct {
	marks     marks
	uc        int
	page      encoding.Encoding
	font      int
	charset   encoding.Encoding
	fontTable bool
	fontEntry int
	hidden    bool
	star      bool
}

func rtfCodePage(n int) encoding.Encoding {
	switch n {
	case 437:
		return charmap.CodePage437
	case 850:
		return charmap.CodePage850
	case 852:
		return charmap.CodePage852
	case 855:
		return charmap.CodePage855
	case 866:
		return charmap.CodePage866
	case 874:
		return charmap.Windows874
	case 932:
		return japanese.ShiftJIS
	case 936:
		return simplifiedchinese.GBK
	case 949:
		return korean.EUCKR
	case 950:
		return traditionalchinese.Big5
	case 1250:
		return charmap.Windows1250
	case 1251:
		return charmap.Windows1251
	case 1252:
		return charmap.Windows1252
	case 1253:
		return charmap.Windows1253
	case 1254:
		return charmap.Windows1254
	case 1255:
		return charmap.Windows1255
	case 1256:
		return charmap.Windows1256
	case 1257:
		return charmap.Windows1257
	case 1258:
		return charmap.Windows1258
	}
	return nil
}

// The font's charset takes precedence over the document's ANSI code page.
// DEFAULT_CHARSET (1) and unknown values leave the document code page in use.
func rtfFontCharset(n int) encoding.Encoding {
	switch n {
	case 0:
		return rtfCodePage(1252)
	case 128:
		return rtfCodePage(932)
	case 129:
		return rtfCodePage(949)
	case 134:
		return rtfCodePage(936)
	case 136:
		return rtfCodePage(950)
	case 161:
		return rtfCodePage(1253)
	case 162:
		return rtfCodePage(1254)
	case 163:
		return rtfCodePage(1258)
	case 177:
		return rtfCodePage(1255)
	case 178:
		return rtfCodePage(1256)
	case 186:
		return rtfCodePage(1257)
	case 204:
		return rtfCodePage(1251)
	case 222:
		return rtfCodePage(874)
	case 238:
		return rtfCodePage(1250)
	case 255:
		return rtfCodePage(437)
	}
	return nil
}

func rtfDestination(s string) bool {
	switch s {
	case "fonttbl", "colortbl", "stylesheet", "info", "pict", "object", "objdata", "header", "footer", "headerl", "headerr", "footerl", "footerr", "filetbl", "listtable", "listoverridetable", "generator", "xmlnstbl", "datastore", "themedata", "fldinst":
		return true
	}
	return false
}

func hexByte(s string) (byte, bool) {
	v, err := strconv.ParseUint(s, 16, 8)
	return byte(v), err == nil
}

// RTFToMarkdown reads text and basic character formatting, ignoring unsupported
// controls. Malformed input is recovered as far as possible without an error.
func (Converter) RTFToMarkdown(input string) (string, error) {
	if !strings.HasPrefix(strings.TrimSpace(input), `{\rtf`) {
		return escapeText(input), nil
	}
	state := rtfState{uc: 1, page: charmap.Windows1252, font: -1, fontEntry: -1}
	var stack []rtfState
	fonts := map[int]encoding.Encoding{}
	defaultFont := -1
	var writer inlineWriter
	var paragraphs []string
	var bytes []byte
	skip := 0
	var high rune
	flushHigh := func() {
		if high != 0 {
			writer.add(string(utf8.RuneError), state.marks, false)
			high = 0
		}
	}
	writeRune := func(r rune) {
		if state.hidden {
			return
		}
		if high != 0 {
			if r >= 0xdc00 && r <= 0xdfff {
				writer.add(string(utf16.DecodeRune(high, r)), state.marks, false)
				high = 0
				return
			}
			flushHigh()
		}
		if r >= 0xd800 && r <= 0xdbff {
			high = r
			return
		}
		writer.add(string(r), state.marks, false)
	}
	flushBytes := func() {
		if len(bytes) == 0 {
			return
		}
		page := state.page
		if f := fonts[state.font]; f != nil {
			page = f
		}
		if state.charset != nil {
			page = state.charset
		}
		decoded, err := page.NewDecoder().Bytes(bytes)
		if err != nil {
			decoded, _ = charmap.Windows1252.NewDecoder().Bytes(bytes)
		}
		flushHigh()
		writer.add(string(decoded), state.marks, false)
		bytes = bytes[:0]
	}
	writeByte := func(b byte) {
		if !state.hidden {
			bytes = append(bytes, b)
		}
	}
	flushParagraph := func() {
		flushBytes()
		flushHigh()
		if s := strings.TrimSpace(writer.markdown()); s != "" {
			paragraphs = append(paragraphs, s)
		}
		writer = inlineWriter{}
	}
	for i := 0; i < len(input); {
		ch := input[i]
		switch ch {
		case '{':
			flushBytes()
			skip = 0
			stack = append(stack, state)
			state.star = false
			i++
		case '}':
			flushBytes()
			skip = 0
			if len(stack) > 0 {
				state = stack[len(stack)-1]
				stack = stack[:len(stack)-1]
			}
			i++
		case '\r', '\n':
			i++
		case '\\':
			i++
			if i >= len(input) {
				break
			}
			c := input[i]
			if c == '\'' && i+2 < len(input) {
				if b, ok := hexByte(input[i+1 : i+3]); ok {
					if skip > 0 {
						skip--
					} else {
						writeByte(b)
					}
					i += 3
					continue
				}
			}
			if !isLetter(c) {
				i++
				if skip > 0 {
					skip--
					continue
				}
				if c == '*' {
					flushBytes()
					state.star = true
					continue
				}
				switch c {
				case '\\', '{', '}':
					writeByte(c)
				case '~':
					flushBytes()
					writeRune('\u00a0')
				case '-':
					flushBytes()
					writeRune('\u00ad')
				case '_':
					flushBytes()
					writeRune('\u2011')
				}
				continue
			}
			start := i
			for i < len(input) && isLetter(input[i]) {
				i++
			}
			word := input[start:i]
			numStart := i
			if i < len(input) && input[i] == '-' {
				i++
			}
			for i < len(input) && input[i] >= '0' && input[i] <= '9' {
				i++
			}
			n, numErr := strconv.Atoi(input[numStart:i])
			hasNum := numErr == nil
			if i < len(input) && input[i] == ' ' {
				i++
			}
			if skip > 0 {
				skip--
				continue
			}
			flushBytes()
			if word == "fonttbl" {
				state.fontTable, state.hidden, state.star = true, true, false
				state.fontEntry = -1
				continue
			}
			if state.fontTable {
				switch word {
				case "f":
					if hasNum {
						state.fontEntry = n
					}
				case "fcharset":
					if hasNum && state.fontEntry >= 0 {
						fonts[state.fontEntry] = rtfFontCharset(n)
					}
				}
				continue
			}
			if state.star || rtfDestination(word) {
				state.hidden = true
				state.star = false
			}
			if state.hidden {
				continue
			}
			switch word {
			case "b":
				state.marks.bold = !hasNum || n != 0
			case "i":
				state.marks.italic = !hasNum || n != 0
			case "strike":
				state.marks.strike = !hasNum || n != 0
			case "plain":
				state.marks = marks{}
				state.font, state.charset = defaultFont, nil
			case "deff":
				if hasNum {
					defaultFont = n
					if state.font < 0 {
						state.font = n
					}
				}
			case "f":
				if hasNum {
					state.font, state.charset = n, nil
				}
			case "fcharset":
				if hasNum {
					state.charset = rtfFontCharset(n)
				}
			case "uc":
				if hasNum {
					state.uc = max(0, n)
				}
			case "ansicpg":
				if p := rtfCodePage(n); hasNum && p != nil {
					state.page = p
				}
			case "u":
				if hasNum {
					writeRune(rune(uint16(n)))
					skip = state.uc
				}
			case "par":
				flushParagraph()
			case "line":
				flushHigh()
				writer.add("\\\n", state.marks, true)
			case "tab":
				writeRune('\t')
			case "emdash":
				writeRune('—')
			case "endash":
				writeRune('–')
			case "bullet":
				writeRune('•')
			case "lquote":
				writeRune('‘')
			case "rquote":
				writeRune('’')
			case "ldblquote":
				writeRune('“')
			case "rdblquote":
				writeRune('”')
			}
		default:
			i++
			if skip > 0 {
				skip--
				continue
			}
			writeByte(ch)
		}
	}
	flushParagraph()
	return strings.Join(paragraphs, "\n\n"), nil
}

func isLetter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }
