package export

import (
	"strconv"
	"strings"
	"unicode"

	"aurink/internal/contract"
)

type yamlValue struct {
	scalar string
	list   []string
	isList bool
}

// parseFrontMatter accepts only the flat YAML subset used by the header. An
// unrecognized construct leaves the original source visible to the user.
func parseFrontMatter(src string) *contract.FrontMatterMeta {
	values := make(map[string]yamlValue)
	current := ""
	for _, line := range strings.Split(strings.ReplaceAll(strings.ReplaceAll(src, "\r\n", "\n"), "\r", "\n"), "\n") {
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		if strings.HasPrefix(line, "  - ") {
			v, ok := values[current]
			if current == "" || !ok || (!v.isList && v.scalar != "") {
				return nil
			}
			item, ok := yamlScalar(line[4:])
			if !ok {
				return nil
			}
			v.isList = true
			v.list = append(v.list, item)
			values[current] = v
			continue
		}
		current = ""
		if line[0] == ' ' || line[0] == '\t' {
			return nil
		}
		colon := strings.IndexByte(line, ':')
		if colon <= 0 || colon+1 < len(line) && line[colon+1] != ' ' && line[colon+1] != '\t' {
			return nil
		}
		key := line[:colon]
		for _, c := range key {
			if !unicode.IsLetter(c) && !unicode.IsDigit(c) && c != '_' && c != '-' {
				return nil
			}
		}
		if _, exists := values[key]; exists {
			return nil
		}
		raw, ok := yamlUncomment(strings.TrimSpace(line[colon+1:]))
		if !ok {
			return nil
		}
		var v yamlValue
		if strings.HasPrefix(raw, "[") {
			v.list, ok = yamlList(raw)
			v.isList = true
		} else {
			v.scalar, ok = yamlScalar(raw)
		}
		if !ok {
			return nil
		}
		values[key] = v
		if raw == "" {
			current = key
		}
	}
	meta := &contract.FrontMatterMeta{}
	if v, ok := values["title"]; ok {
		if v.isList {
			return nil
		}
		meta.Kicker = v.scalar
	}
	if v, ok := values["tags"]; ok {
		if v.isList {
			meta.Tags = v.list
		} else if v.scalar != "" {
			meta.Tags = []string{v.scalar}
		}
	} else if v, ok := values["keywords"]; ok {
		if v.isList {
			meta.Tags = v.list
		} else if v.scalar != "" {
			meta.Tags = []string{v.scalar}
		}
	}
	if v, ok := values["date"]; ok {
		if v.isList {
			return nil
		}
		meta.Date = v.scalar
	}
	return meta
}

func yamlUncomment(s string) (string, bool) {
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote == '"' && c == '\\' {
			i++
			if i == len(s) {
				return "", false
			}
			continue
		}
		if quote == '\'' && c == '\'' && i+1 < len(s) && s[i+1] == '\'' {
			i++
			continue
		}
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		if (c == '\'' || c == '"') && yamlQuoteStart(s, i) {
			quote = c
			continue
		}
		if c == '#' && (i == 0 || s[i-1] == ' ' || s[i-1] == '\t') {
			return strings.TrimSpace(s[:i]), true
		}
	}
	return strings.TrimSpace(s), quote == 0
}

func yamlQuoteStart(s string, at int) bool {
	for at--; at >= 0 && (s[at] == ' ' || s[at] == '\t'); at-- {
	}
	return at < 0 || s[at] == '[' || s[at] == ','
}

func yamlScalar(raw string) (string, bool) {
	s, ok := yamlUncomment(strings.TrimSpace(raw))
	if !ok {
		return "", false
	}
	if s == "" {
		return "", true
	}
	if s[0] == '\'' {
		if len(s) < 2 || s[len(s)-1] != '\'' {
			return "", false
		}
		inner := s[1 : len(s)-1]
		for i := 0; i < len(inner); i++ {
			if inner[i] == '\'' {
				if i+1 == len(inner) || inner[i+1] != '\'' {
					return "", false
				}
				i++
			}
		}
		return strings.ReplaceAll(inner, "''", "'"), true
	}
	if s[0] == '"' {
		value, err := strconv.Unquote(s)
		return value, err == nil
	}
	if strings.ContainsAny(s, "[]{}\r\n") || strings.Contains(s, ": ") || strings.Contains(s, ":\t") || strings.ContainsAny(s[:1], "&*!|>@`%'") || s == "-" || strings.HasPrefix(s, "- ") {
		return "", false
	}
	return s, true
}

func yamlList(raw string) ([]string, bool) {
	if len(raw) < 2 || raw[len(raw)-1] != ']' {
		return nil, false
	}
	body := strings.TrimSpace(raw[1 : len(raw)-1])
	if body == "" {
		return []string{}, true
	}
	var result []string
	start := 0
	var quote byte
	for i := 0; i <= len(body); i++ {
		if i < len(body) {
			c := body[i]
			if quote == '"' && c == '\\' {
				i++
				if i >= len(body) {
					return nil, false
				}
				continue
			}
			if quote == '\'' && c == '\'' && i+1 < len(body) && body[i+1] == '\'' {
				i++
				continue
			}
			if quote != 0 {
				if c == quote {
					quote = 0
				}
				continue
			}
			if (c == '\'' || c == '"') && yamlQuoteStart(body, i) {
				quote = c
				continue
			}
			if c != ',' {
				continue
			}
		}
		part := strings.TrimSpace(body[start:i])
		if part == "" {
			return nil, false
		}
		value, ok := yamlScalar(part)
		if !ok {
			return nil, false
		}
		result = append(result, value)
		start = i + 1
	}
	return result, quote == 0
}
