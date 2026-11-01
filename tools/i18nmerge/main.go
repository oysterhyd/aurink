// Command i18nmerge merges i18n/fragments into locales and regenerates
// locales/catalog_gen.go, under the key rules K1–K6 of
// docs/05-delivery-plan.md §3.4. Run it from the repository root.
//
//	go run ./tools/i18nmerge         check the fragments, write the generated files
//	go run ./tools/i18nmerge -check  also fail when the committed files differ
//
// Owner: INT (docs/05-delivery-plan.md §3.1).
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/token"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/egoist/mygo/i18n"
)

const (
	fragmentsDir = "i18n/fragments"
	localesDir   = "locales"
	catalogGen   = "catalog_gen.go"
)

var langs = []string{"en", "zh-Hans"}

// packages are the first key segments of each owner (K2).
var packages = map[string][]string{
	"int":  {"app", "watch", "clipboard"},
	"be-1": {"text", "markdown", "projection", "document", "editing", "richpaste"},
	"be-2": {"fsstore", "docsession", "workspace", "search", "export", "render", "settings", "commands", "webviewhost"},
	"fe-1": {"liveedit", "source"},
	"fe-2": {"theme", "motion", "effects", "kit", "shell", "palette", "panels"},
}

var (
	keyRe         = regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z][a-z0-9]*)+$`) // K1
	mygoKeyRe     = regexp.MustCompile(`^mygo\.[A-Za-z][A-Za-z0-9_]*$`)      // K4
	placeholderRe = regexp.MustCompile(`^[a-z][A-Za-z0-9]*$`)                // K5
)

func main() {
	check := flag.Bool("check", false, "fail when the committed generated files differ")
	flag.Parse()
	if errs := run(*check); len(errs) > 0 {
		for _, err := range errs {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}
}

func run(check bool) []error {
	files, errs := loadFragments(fragmentsDir)
	errs = append(errs, validate(files)...)
	if len(errs) > 0 {
		return errs
	}
	catalogs, err := merge(files)
	if err != nil {
		return []error{err}
	}
	if check {
		errs = compareCatalogs(catalogs)
	} else if err := writeCatalogs(catalogs); err != nil {
		errs = []error{err}
	}
	if len(errs) > 0 {
		return errs
	}
	if err := generate(check); err != nil {
		return []error{err}
	}
	if err := goTool("i18n", "check"); err != nil {
		return []error{err}
	}
	return nil
}

// fragment is one file of i18n/fragments, "<owner>.<lang>.json".
type fragment struct {
	name, owner, lang string
	messages          map[string]string
	meta              map[string]json.RawMessage // of "@<key>" entries, by key
}

// loadFragments reads every file of the fragments directory, sorted by name.
func loadFragments(dir string) ([]*fragment, []error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, []error{err}
	}
	var (
		files []*fragment
		errs  []error
	)
	for _, e := range entries {
		owner, lang, ok := parseName(e.Name())
		if !ok {
			errs = append(errs, fmt.Errorf("%s: K6: not <owner>.<lang>.json of a known owner and language", e.Name()))
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			errs = append(errs, err)
			continue
		}
		f, err := parseFragment(e.Name(), owner, lang, data)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		files = append(files, f)
	}
	return files, errs
}

func parseName(name string) (owner, lang string, ok bool) {
	base, found := strings.CutSuffix(name, ".json")
	if !found {
		return "", "", false
	}
	owner, lang, found = strings.Cut(base, ".")
	_, known := packages[owner]
	return owner, lang, found && known && slices.Contains(langs, lang)
}

// parseFragment reads the top level by tokens, since decoding to a map would
// hide a key written twice.
func parseFragment(name, owner, lang string, data []byte) (*fragment, error) {
	f := &fragment{name: name, owner: owner, lang: lang, messages: map[string]string{}, meta: map[string]json.RawMessage{}}
	dec := json.NewDecoder(bytes.NewReader(data))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return nil, fmt.Errorf("%s: not a JSON object", name)
	}
	seen := map[string]bool{}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		key := tok.(string)
		if seen[key] {
			return nil, fmt.Errorf("%s: %q is written twice", name, key)
		}
		seen[key] = true
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, fmt.Errorf("%s: %q: %w", name, key, err)
		}
		switch {
		case key == "@@locale":
			var locale string
			if err := json.Unmarshal(raw, &locale); err != nil || locale != lang {
				return nil, fmt.Errorf("%s: @@locale is not %q", name, lang)
			}
		case strings.HasPrefix(key, "@"):
			f.meta[key[1:]] = raw
		default:
			var msg string
			if err := json.Unmarshal(raw, &msg); err != nil {
				return nil, fmt.Errorf("%s: %q is not a string", name, key)
			}
			f.messages[key] = msg
		}
	}
	if _, err := dec.Token(); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return f, nil
}

// validate checks K1–K6 and that no key is in two fragments.
func validate(files []*fragment) []error {
	var errs []error
	report := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }

	byOwner := map[string]map[string]*fragment{}
	firstSeen := map[string]map[string]string{} // lang, key → file
	for _, f := range files {
		if byOwner[f.owner] == nil {
			byOwner[f.owner] = map[string]*fragment{}
		}
		byOwner[f.owner][f.lang] = f
		if firstSeen[f.lang] == nil {
			firstSeen[f.lang] = map[string]string{}
		}
		for _, key := range sortedKeys(f.messages) {
			if other, dup := firstSeen[f.lang][key]; dup {
				report("%s: %q is also in %s", f.name, key, other)
			}
			firstSeen[f.lang][key] = f.name
			checkKey(f, key, report)
		}
		checkMeta(f, report)
	}
	for _, owner := range sortedKeys(byOwner) {
		en, zh := byOwner[owner]["en"], byOwner[owner]["zh-Hans"]
		if en == nil || zh == nil {
			report("%s: K6: needs both %s.en.json and %s.zh-Hans.json", owner, owner, owner)
			continue
		}
		checkPair(en, zh, report)
	}
	return errs
}

func checkKey(f *fragment, key string, report func(string, ...any)) {
	if strings.HasPrefix(key, "mygo.") {
		if f.owner != "int" {
			report("%s: %q: K4: only the int fragments can hold mygo.<key>", f.name, key)
		} else if !mygoKeyRe.MatchString(key) {
			report("%s: %q: K4: not mygo.<key>", f.name, key)
		}
	} else if !keyRe.MatchString(key) {
		report("%s: %q: K1: not lower snake case with at least two segments", f.name, key)
	} else if first, _, _ := strings.Cut(key, "_"); !slices.Contains(packages[f.owner], first) {
		report("%s: %q: K2: %q is not a package of %s", f.name, key, first, f.owner)
	}
	args, err := messageArgs(f.messages[key])
	if err != nil {
		report("%s: %q: %v", f.name, key, err)
	}
	for _, arg := range args {
		checkPlaceholder(f, key, arg, report)
	}
}

func checkPlaceholder(f *fragment, key, name string, report func(string, ...any)) {
	if !placeholderRe.MatchString(name) || token.IsKeyword(name) {
		report("%s: %q: K5: placeholder %q is not lower camel case, or is a Go keyword", f.name, key, name)
	}
}

// checkMeta checks the "@<key>" entries: the English fragment describes every
// key, and only the English fragment has them.
func checkMeta(f *fragment, report func(string, ...any)) {
	for _, key := range sortedKeys(f.meta) {
		if f.lang != "en" {
			report("%s: @%s: K6: only the English fragment describes keys", f.name, key)
		} else if _, ok := f.messages[key]; !ok {
			report("%s: @%s: describes no key", f.name, key)
		}
	}
	if f.lang != "en" {
		return
	}
	for _, key := range sortedKeys(f.messages) {
		var meta struct {
			Description  string                     `json:"description"`
			Placeholders map[string]json.RawMessage `json:"placeholders"`
		}
		raw, ok := f.meta[key]
		if ok {
			if err := json.Unmarshal(raw, &meta); err != nil {
				report("%s: @%s: %v", f.name, key, err)
			}
		}
		if strings.TrimSpace(meta.Description) == "" {
			report("%s: %q: K6: no @%s.description", f.name, key, key)
		}
		for _, name := range sortedKeys(meta.Placeholders) {
			checkPlaceholder(f, key, name, report)
		}
	}
}

// checkPair checks that both languages of an owner have the same keys and the
// same placeholders (K6).
func checkPair(en, zh *fragment, report func(string, ...any)) {
	for _, key := range sortedKeys(en.messages) {
		msg, ok := zh.messages[key]
		if !ok {
			report("%s: K6: lacks %q", zh.name, key)
			continue
		}
		enArgs, errEn := messageArgs(en.messages[key])
		zhArgs, errZh := messageArgs(msg)
		if errEn == nil && errZh == nil && !slices.Equal(enArgs, zhArgs) {
			report("%s: %q: K6: placeholders %v differ from %s's %v", zh.name, key, zhArgs, en.name, enArgs)
		}
	}
	for _, key := range sortedKeys(zh.messages) {
		if _, ok := en.messages[key]; !ok {
			report("%s: K6: lacks %q", en.name, key)
		}
	}
}

// messageArgs returns the sorted names of a message's arguments.
func messageArgs(msg string) ([]string, error) {
	m, err := i18n.ParseMessage(msg)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, arg := range m.Args() {
		names = append(names, arg.Name)
	}
	slices.Sort(names)
	return names, nil
}

// merge returns the file contents of locales, by file name, with keys sorted.
func merge(files []*fragment) (map[string][]byte, error) {
	catalogs := map[string][]byte{}
	for _, lang := range langs {
		messages := map[string]string{}
		meta := map[string]json.RawMessage{}
		for _, f := range files {
			if f.lang == lang {
				maps.Copy(messages, f.messages)
				maps.Copy(meta, f.meta)
			}
		}
		data, err := encodeCatalog(lang, messages, meta)
		if err != nil {
			return nil, err
		}
		catalogs[lang+".json"] = data
	}
	return catalogs, nil
}

func encodeCatalog(lang string, messages map[string]string, meta map[string]json.RawMessage) ([]byte, error) {
	var b bytes.Buffer
	fmt.Fprintf(&b, "{\n  \"@@locale\": %q", lang)
	entry := func(key string, value []byte) error {
		var indented bytes.Buffer
		if err := json.Indent(&indented, value, "  ", "  "); err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
		fmt.Fprintf(&b, ",\n  %s: %s", encodeString(key), indented.Bytes())
		return nil
	}
	for _, key := range sortedKeys(messages) {
		if err := entry(key, encodeString(messages[key])); err != nil {
			return nil, err
		}
		if raw, ok := meta[key]; ok {
			if err := entry("@"+key, raw); err != nil {
				return nil, err
			}
		}
	}
	b.WriteString("\n}\n")
	return b.Bytes(), nil
}

// encodeString writes a JSON string keeping <, > and & as they are.
func encodeString(s string) []byte {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		panic(err) // a string always encodes
	}
	return bytes.TrimSpace(b.Bytes())
}

func writeCatalogs(catalogs map[string][]byte) error {
	for _, name := range sortedKeys(catalogs) {
		if err := os.WriteFile(filepath.Join(localesDir, name), catalogs[name], 0o644); err != nil {
			return err
		}
	}
	return nil
}

func compareCatalogs(catalogs map[string][]byte) []error {
	var errs []error
	for _, name := range sortedKeys(catalogs) {
		got, err := os.ReadFile(filepath.Join(localesDir, name))
		if err != nil {
			errs = append(errs, err)
		} else if !bytes.Equal(got, catalogs[name]) {
			errs = append(errs, fmt.Errorf("%s/%s is out of date: run go run ./tools/i18nmerge", localesDir, name))
		}
	}
	return errs
}

// generate regenerates catalog_gen.go. With check it fails when that changes
// the file, and leaves the file as it was.
func generate(check bool) error {
	path := filepath.Join(localesDir, catalogGen)
	before, err := os.ReadFile(path)
	if err != nil && !(os.IsNotExist(err) && !check) {
		return err
	}
	if err := goTool("generate"); err != nil {
		return err
	}
	if !check {
		return nil
	}
	after, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !bytes.Equal(before, after) {
		if err := os.WriteFile(path, before, 0o644); err != nil {
			return err
		}
		return fmt.Errorf("%s is out of date: run go run ./tools/i18nmerge", path)
	}
	return nil
}

func goTool(args ...string) error {
	cmd := exec.Command("go", append([]string{"tool", "mygo"}, args...)...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("mygo %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

func sortedKeys[V any](m map[string]V) []string {
	return slices.Sorted(maps.Keys(m))
}
