package fsstore

import (
	"bytes"
	"encoding/binary"
	"errors"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

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
		if !allowBinary && isBinary(data) {
			return "", meta, contract.CodeBinary
		}
		if !utf8.Valid(data) {
			return "", meta, contract.CodeEncoding
		}
		text = string(data)
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
