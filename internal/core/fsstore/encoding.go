package fsstore

import (
	"bytes"
	"encoding/binary"
	"errors"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"

	"aurink/internal/contract"
)

func decode(data []byte, allowBinary bool) (string, contract.DocMeta, contract.Code) {
	meta := contract.DocMeta{Encoding: "utf-8"}
	switch {
	case bytes.HasPrefix(data, []byte{0xff, 0xfe}):
		meta.Encoding, meta.BOM, data = "utf-16le", true, data[2:]
	case bytes.HasPrefix(data, []byte{0xfe, 0xff}):
		meta.Encoding, meta.BOM, data = "utf-16be", true, data[2:]
	case bytes.HasPrefix(data, []byte{0xef, 0xbb, 0xbf}):
		meta.BOM, data = true, data[3:]
	}
	var text string
	if meta.Encoding == "utf-8" {
		if !allowBinary && bytes.IndexByte(data, 0) >= 0 {
			return "", meta, contract.CodeBinary
		}
		if utf8.Valid(data) {
			text = string(data)
		} else if !meta.BOM && bytes.IndexByte(data, 0) < 0 {
			if decoded, ok := decodeGB18030(data); ok {
				meta.Encoding = "gb18030"
				text = decoded
			}
		}
		if text == "" && len(data) > 0 {
			if !allowBinary && isBinary(data) {
				return "", meta, contract.CodeBinary
			}
			return "", meta, contract.CodeEncoding
		}
	} else {
		if len(data)%2 != 0 {
			return "", meta, contract.CodeEncoding
		}
		units := make([]uint16, len(data)/2)
		order := byteOrder(meta.Encoding)
		for i := range units {
			units[i] = order.Uint16(data[2*i:])
		}
		// Decode replaces malformed surrogates; reject them to avoid data loss.
		for i := 0; i < len(units); i++ {
			u := units[i]
			if u >= 0xd800 && u <= 0xdbff {
				if i+1 == len(units) || units[i+1] < 0xdc00 || units[i+1] > 0xdfff {
					return "", meta, contract.CodeEncoding
				}
				i++
			} else if u >= 0xdc00 && u <= 0xdfff {
				return "", meta, contract.CodeEncoding
			}
		}
		text = string(utf16.Decode(units))
		if !allowBinary && strings.IndexByte(text, 0) >= 0 {
			return "", meta, contract.CodeBinary
		}
	}
	crlf := strings.Count(text, "\r\n")
	lf := strings.Count(text, "\n") - crlf
	meta.Dominant = "\n"
	if crlf > lf {
		meta.Dominant = "\r\n"
	}
	switch {
	case crlf > 0 && lf > 0:
		meta.EOL = contract.EOLMixed
	case crlf > 0:
		meta.EOL = contract.EOLCRLF
	}
	return text, meta, ""
}

func isBinary(data []byte) bool {
	if bytes.IndexByte(data, 0) >= 0 {
		return true
	}
	invalid := 0
	for rest := data; len(rest) > 0; {
		r, n := utf8.DecodeRune(rest)
		if r == utf8.RuneError && n == 1 {
			invalid++
		}
		rest = rest[n:]
	}
	// A few bad bytes indicate an encoding error; over 30% suggests binary.
	return float64(invalid) > float64(len(data))*0.3
}

// decodeGB18030 strictly validates GB18030 1/2/4-byte framing and decodes data
// without introducing replacement characters (docs/09-phase4.md D6).
func decodeGB18030(data []byte) (string, bool) {
	wantFFFD := 0
	for i := 0; i < len(data); {
		b := data[i]
		if b < 0x80 {
			i++
			continue
		}
		if b == 0x80 || b == 0xff || i+1 >= len(data) {
			return "", false
		}
		b2 := data[i+1]
		if b2 >= 0x40 && b2 <= 0xfe && b2 != 0x7f {
			i += 2
			continue
		}
		if b2 >= 0x30 && b2 <= 0x39 && i+3 < len(data) {
			b3, b4 := data[i+2], data[i+3]
			if b3 >= 0x81 && b3 <= 0xfe && b4 >= 0x30 && b4 <= 0x39 {
				if b == 0x84 && b2 == 0x31 && b3 == 0xa4 && b4 == 0x37 {
					wantFFFD++
				}
				i += 4
				continue
			}
		}
		return "", false
	}
	out, err := simplifiedchinese.GB18030.NewDecoder().Bytes(data)
	if err != nil || !utf8.Valid(out) {
		return "", false
	}
	s := string(out)
	if strings.Count(s, "\uFFFD") != wantFFFD {
		return "", false
	}
	return s, true
}

func encode(text string, meta contract.DocMeta) ([]byte, error) {
	if !utf8.ValidString(text) {
		return nil, errors.New("invalid UTF-8 text")
	}
	switch meta.Encoding {
	case "", "utf-8":
		if meta.BOM {
			return append([]byte{0xef, 0xbb, 0xbf}, text...), nil
		}
		return []byte(text), nil
	case "utf-16le", "utf-16be":
		units := utf16.Encode([]rune(text))
		data := make([]byte, 0, 2*len(units)+2)
		order := byteOrder(meta.Encoding)
		if meta.BOM {
			data = order.AppendUint16(data, 0xfeff)
		}
		for _, u := range units {
			data = order.AppendUint16(data, u)
		}
		return data, nil
	default:
		return nil, errors.New("unsupported encoding: " + meta.Encoding)
	}
}

func byteOrder(encoding string) interface {
	binary.ByteOrder
	binary.AppendByteOrder
} {
	if encoding == "utf-16be" {
		return binary.BigEndian
	}
	return binary.LittleEndian
}
