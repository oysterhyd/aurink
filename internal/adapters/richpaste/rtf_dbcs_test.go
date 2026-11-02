package richpaste

import "testing"

func TestRTFCodePagesAndFonts(t *testing.T) {
	tests := []struct {
		name, rtf, want string
	}{
		{"GBK", `{\rtf1\ansi\ansicpg936 \'c4\'e3\'ba\'c3}`, "你好"},
		{"Big5", `{\rtf1\ansi\ansicpg950 \'a7\'41\'a6\'6e}`, "你好"},
		{"Shift JIS", `{\rtf1\ansi\ansicpg932 \'93\'fa\'96\'7b}`, "日本"},
		{"Korean UHC", `{\rtf1\ansi\ansicpg949 \'be\'c8\'b3\'e7}`, "안녕"},
		{"Korean UHC extension", `{\rtf1\ansi\ansicpg949 \'81A}`, "갂"},
		{"escaped lead and literal trail", `{\rtf1\ansi\ansicpg936 \'81@}`, "丂"},
		{"literal lead and escaped trail", "{\\rtf1\\ansi\\ansicpg936 " + string([]byte{0x81}) + `\'40}`, "丂"},
		{"escaped backslash trail", `{\rtf1\ansi\ansicpg932 \'83\\}`, "ソ"},
		{"GBK font overrides document", `{\rtf1\ansi\ansicpg1252{\fonttbl{\f0\fnil\fcharset0 Arial;}{\f1\fnil\fcharset134 SimSun;}}\f1 \'c4\'e3\'ba\'c3}`, "你好"},
		{"unicode wins over GBK fallback", `{\rtf1\ansi\ansicpg936\uc2 \u20320\'c4\'e3\u22909\'ba\'c3}`, "你好"},
		{"GBK resumes after unicode fallback", `{\rtf1\ansi\ansicpg936\uc2 \u20320\'c4\'e3\'ba\'c3}`, "你好"},
		{"unicode wins over mixed GBK fallback", `{\rtf1\ansi\ansicpg936\uc2 \u19970\'81@X}`, "丂X"},
		{"unicode skip is scoped", `{\rtf1\ansi\ansicpg936\uc1 \u65?{\uc2 \u20320\'c4\'e3}\u66?}`, "A你B"},
		{"unicode skips control symbol", `{\rtf1\ansi\uc1 \u65\*B}`, "AB"},
		{"unicode skips control word", `{\rtf1\ansi\uc1 \u65\emdash B}`, "AB"},
		{"unicode skip ends at group boundary", `{\rtf1\ansi\uc2 {\u65?}B}`, "AB"},
		{"font switches and group restore", `{\rtf1\ansi\ansicpg1252\deff0{\fonttbl{\f0\fcharset0 Arial;}{\f1\fcharset134 SimSun;}{\f2\fcharset128 MS Gothic;}{\f3\fcharset129 Gulim;}{\f4\fcharset136 Ming;}{\f5\fcharset204 Cyrillic;}}\f1 \'c4\'e3 {\f2 \'93\'fa} \'ba\'c3 {\f3 \'be\'c8\'b3\'e7} {\f4 \'a7\'41\'a6\'6e} {\f5 \'cf\'f0\'e8\'e2\'e5\'f2} \f0 caf\'e9}`, "你 日 好 안녕 你好 Привет café"},
		{"default font", `{\rtf1\ansi\ansicpg1252\deff1{\fonttbl{\f0\fcharset0 Arial;}{\f1\fcharset134 SimSun;}} \'c4\'e3}`, "你"},
		{"direct charset switch", `{\rtf1\ansi\ansicpg1252\fcharset134 \'c4\'e3}`, "你"},
		{"plain restores default font", `{\rtf1\ansi\ansicpg1252\deff0{\fonttbl{\f0\fcharset0 Arial;}{\f1\fcharset134 SimSun;}}\f1 \'c4\'e3\plain  caf\'e9}`, "你 café"},
		{"group restores document code page", `{\rtf1\ansi\ansicpg1252 caf\'e9 {\ansicpg936 \'c4\'e3} caf\'e9}`, "café 你 café"},
		{"unknown font falls back to document", `{\rtf1\ansi\ansicpg1251{\fonttbl{\f0\fcharset1 Arial;}}\f0 \'cf\'f0\'e8\'e2\'e5\'f2}`, "Привет"},
		{"formatting around multibyte text", `{\rtf1\ansi\ansicpg936 a {\b \'c4\'e3\'ba\'c3} z}`, "a **你好** z"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := New().RTFToMarkdown(tt.rtf)
			if err != nil || got != tt.want {
				t.Fatalf("got %q, want %q, error %v", got, tt.want, err)
			}
		})
	}
}
