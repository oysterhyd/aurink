// Command i18ncheck checks locales against the key rules K1–K5 of
// docs/05-delivery-plan.md §3.4, that locales/catalog_gen.go is up to date, and
// runs mygo i18n check. Run it from the repository root.
package main

import (
	"bytes"
	"encoding/json"
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

const localesDir = "locales"

// packages are the allowed first key segments (K2).
var packages = []string{
	"app", "watch", "clipboard",
	"text", "markdown", "projection", "document", "editing", "richpaste",
	"fsstore", "docsession", "workspace", "search", "export", "render", "settings", "commands", "webviewhost",
	"liveedit", "source",
	"theme", "motion", "effects", "kit", "shell", "palette", "panels",
}

var (
	keyRe         = regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z][a-z0-9]*)+$`) // K1
	mygoKeyRe     = regexp.MustCompile(`^mygo\.[A-Za-z][A-Za-z0-9_]*$`)      // K4
	placeholderRe = regexp.MustCompile(`^[a-z][A-Za-z0-9]*$`)                // K5
)

func main() {
	if errs := run(); len(errs) > 0 {
		for _, err := range errs {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}
}

func run() []error {
	en, err := loadCatalog("en")
	if err != nil {
		return []error{err}
	}
	zh, err := loadCatalog("zh-Hans")
	if err != nil {
		return []error{err}
	}
	if errs := validate(en, zh); len(errs) > 0 {
		return errs
	}
	if err := checkGenerated(); err != nil {
		return []error{err}
	}
	if err := goTool("i18n", "check"); err != nil {
		return []error{err}
	}
	return nil
}

// catalog is one file of locales.
type catalog struct {
	name     string
	messages map[string]string
	meta     map[string]json.RawMessage // of "@<key>" entries, by key
}

func loadCatalog(lang string) (*catalog, error) {
	name := lang + ".json"
	data, err := os.ReadFile(filepath.Join(localesDir, name))
	if err != nil {
		return nil, err
	}
	return parseCatalog(name, data)
}

func parseCatalog(name string, data []byte) (*catalog, error) {
	var entries map[string]json.RawMessage
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	c := &catalog{name: name, messages: map[string]string{}, meta: map[string]json.RawMessage{}}
	for key, raw := range entries {
		switch {
		case key == "@@locale":
		case strings.HasPrefix(key, "@"):
			c.meta[key[1:]] = raw
		default:
			var msg string
			if err := json.Unmarshal(raw, &msg); err != nil {
				return nil, fmt.Errorf("%s: %q is not a string", name, key)
			}
			c.messages[key] = msg
		}
	}
	return c, nil
}

// validate checks K1–K5 for both catalogs.
func validate(en, zh *catalog) []error {
	var errs []error
	report := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }

	for _, c := range []*catalog{en, zh} {
		for _, key := range sortedKeys(c.messages) {
			checkKey(c, key, report)
		}
	}
	for _, key := range sortedKeys(en.messages) {
		checkDescription(en, key, report)
		msg, ok := zh.messages[key]
		if !ok {
			report("%s: K3: lacks %q", zh.name, key)
			continue
		}
		enArgs, errEn := messageArgs(en.messages[key])
		zhArgs, errZh := messageArgs(msg)
		if errEn == nil && errZh == nil && !slices.Equal(enArgs, zhArgs) {
			report("%s: %q: K3: placeholders %v differ from %s's %v", zh.name, key, zhArgs, en.name, enArgs)
		}
	}
	for _, key := range sortedKeys(zh.messages) {
		if _, ok := en.messages[key]; !ok {
			report("%s: K3: lacks %q", en.name, key)
		}
	}
	return errs
}

func checkKey(c *catalog, key string, report func(string, ...any)) {
	if strings.HasPrefix(key, "mygo.") {
		if !mygoKeyRe.MatchString(key) {
			report("%s: %q: K4: not mygo.<key>", c.name, key)
		}
	} else if !keyRe.MatchString(key) {
		report("%s: %q: K1: not lower snake case with at least two segments", c.name, key)
	} else if first, _, _ := strings.Cut(key, "_"); !slices.Contains(packages, first) {
		report("%s: %q: K2: %q is not a package", c.name, key, first)
	}
	args, err := messageArgs(c.messages[key])
	if err != nil {
		report("%s: %q: %v", c.name, key, err)
	}
	for _, arg := range args {
		if !placeholderRe.MatchString(arg) || token.IsKeyword(arg) {
			report("%s: %q: K5: placeholder %q is not lower camel case, or is a Go keyword", c.name, key, arg)
		}
	}
}

func checkDescription(en *catalog, key string, report func(string, ...any)) {
	var meta struct {
		Description string `json:"description"`
	}
	if raw, ok := en.meta[key]; ok {
		if err := json.Unmarshal(raw, &meta); err != nil {
			report("%s: @%s: %v", en.name, key, err)
		}
	}
	if strings.TrimSpace(meta.Description) == "" {
		report("%s: %q: K3: no @%s.description", en.name, key, key)
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

// checkGenerated fails when mygo generate changes catalog_gen.go, and leaves
// the file as it was.
func checkGenerated() error {
	path := filepath.Join(localesDir, "catalog_gen.go")
	before, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := goTool("generate"); err != nil {
		return err
	}
	after, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !bytes.Equal(before, after) {
		if err := os.WriteFile(path, before, 0o644); err != nil {
			return err
		}
		return fmt.Errorf("%s is out of date: run go tool mygo generate", path)
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
