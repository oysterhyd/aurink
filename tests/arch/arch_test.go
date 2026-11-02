package arch

import (
	"os/exec"
	"strings"
	"testing"
)

const (
	mod  = "aurink/"
	mygo = "github.com/egoist/mygo"
)

// thirdParty is the dependency whitelist of docs/05-delivery-plan.md §3.3,
// apart from MyGo itself.
var thirdParty = []string{"golang.org/x/text", "github.com/rivo/uniseg", "gopkg.in/yaml.v3"}

// rule is one row of docs/03-architecture.md §1.1. An import outside the
// standard library must match an entry of allow, as a path or a path prefix.
type rule struct {
	pkg   string // package path, or its subtree
	allow []string
}

var contract = mod + "internal/contract"

var rules = []rule{
	{mod + "internal/contract", nil},
	{mod + "internal/views/editorapi", []string{contract}},
	{mod + "internal/core", append([]string{contract, mod + "internal/core", mygo + "/ui/markdown/parser"}, thirdParty...)},
	{mod + "internal/adapters", append([]string{contract, mod + "internal/adapters", mygo}, thirdParty...)},
	{mod + "internal/views", append([]string{contract, mod + "internal/views", mod + "locales", mygo + "/ui", mygo + "/i18n", mygo + "/plugins/glass"}, thirdParty...)},
	{mod + "internal/mock", []string{contract, mygo + "/ui/markdown/parser"}},
	{mod + "internal/contracttest", []string{contract, mod + "internal/mock", mod + "tests/corpus"}},
	{mod + "internal/app", []string{mod, mygo, "golang.org/x/"}},
	{mod + "locales", []string{mygo + "/i18n", mygo + "/locales"}},
	{mod + "resources", []string{mygo + "/ui"}},
}

// unrestricted packages do not take part in the layering.
var unrestricted = []string{mod + "tests", mod + "tools"}

func under(path, prefix string) bool {
	return path == prefix || strings.HasPrefix(path, strings.TrimSuffix(prefix, "/")+"/")
}

// ruleFor returns the most specific rule of pkg.
func ruleFor(pkg string) *rule {
	var best *rule
	for i := range rules {
		if under(pkg, rules[i].pkg) && (best == nil || len(rules[i].pkg) > len(best.pkg)) {
			best = &rules[i]
		}
	}
	return best
}

func isStd(path string) bool {
	first, _, _ := strings.Cut(path, "/")
	return !strings.Contains(first, ".") && first != "aurink"
}

// testOnly reports whether a test file of pkg may import imp even though pkg's
// rule does not allow it: real implementations run the contract suites and the
// corpus (docs/04-contracts.md §7), and views may use the mocks
// (docs/05-delivery-plan.md §4).
func testOnly(pkg, imp string) bool {
	return under(imp, mod+"internal/contracttest") || under(imp, mod+"tests/corpus") ||
		(under(pkg, mod+"internal/views") && under(imp, mod+"internal/mock"))
}

// violations returns the imports of pkg that its rule does not allow.
func violations(pkg string, imports []string, inTest bool) []string {
	r := ruleFor(pkg)
	if r == nil {
		return nil
	}
	var bad []string
	for _, imp := range imports {
		if isStd(imp) || under(imp, pkg) {
			continue
		}
		ok := inTest && testOnly(pkg, imp)
		for _, a := range r.allow {
			ok = ok || under(imp, a) || (strings.HasSuffix(a, "/") && strings.HasPrefix(imp, a))
		}
		if !ok {
			bad = append(bad, imp)
		}
	}
	return bad
}

func TestImportRules(t *testing.T) {
	out, err := exec.Command("go", "list", "-f",
		`{{.ImportPath}}|{{join .Imports " "}}|{{join .TestImports " "}} {{join .XTestImports " "}}`, "aurink/...").Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		parts := strings.SplitN(line, "|", 3)
		pkg := parts[0]
		// build/ is mygo build's output, ignored by git, with a copy of resources in it.
		if pkg == "aurink" || under2(pkg, unrestricted) || strings.HasPrefix(pkg, "aurink/build/") {
			continue
		}
		if ruleFor(pkg) == nil {
			t.Errorf("%s has no rule: add it to docs/03-architecture.md §1.1 and this test", pkg)
			continue
		}
		for _, imp := range violations(pkg, strings.Fields(parts[1]), false) {
			t.Errorf("%s must not import %s (docs/03-architecture.md §1.1)", pkg, imp)
		}
		for _, imp := range violations(pkg, strings.Fields(parts[2]), true) {
			if !strings.HasPrefix(imp, "github.com/egoist/mygo/ui") {
				t.Errorf("tests of %s must not import %s", pkg, imp)
			}
		}
	}
}

// Only test files get the exception for the contract suites.
func TestContractSuitesAreTestOnly(t *testing.T) {
	core, suite := mod+"internal/core/text", mod+"internal/contracttest"
	if bad := violations(core, []string{suite}, true); len(bad) != 0 {
		t.Errorf("a core test cannot use the suites: %v", bad)
	}
	if bad := violations(core, []string{suite}, false); len(bad) != 1 {
		t.Errorf("core code can use the suites: %v", bad)
	}
}

func under2(pkg string, prefixes []string) bool {
	for _, p := range prefixes {
		if under(pkg, p) {
			return true
		}
	}
	return false
}

// The rules catch deliberate violations, one per row of §1.1.
func TestRulesDetectViolations(t *testing.T) {
	for _, tt := range []struct {
		pkg, imp string
		allowed  bool
	}{
		{mod + "internal/contract", mygo + "/ui", false},
		{mod + "internal/contract", mod + "internal/core/text", false},
		{mod + "internal/contract", "strings", true},
		{mod + "internal/views/editorapi", mod + "internal/views/kit", false},
		{mod + "internal/views/editorapi", contract, true},
		{mod + "internal/core/text", mygo, false},
		{mod + "internal/core/text", mygo + "/ui", false},
		{mod + "internal/core/text", mod + "internal/views/kit", false},
		{mod + "internal/core/text", mod + "internal/adapters/watch", false},
		{mod + "internal/core/text", mod + "internal/app", false},
		{mod + "internal/core/markdown", mygo + "/ui/markdown/parser", true},
		{mod + "internal/core/markdown", "golang.org/x/text/unicode/norm", true},
		{mod + "internal/core/markdown", "github.com/evil/lib", false},
		{mod + "internal/core/document", mod + "internal/core/text", true},
		{mod + "internal/adapters/watch", mod + "internal/core/text", false},
		{mod + "internal/adapters/watch", mod + "internal/views/kit", false},
		{mod + "internal/adapters/watch", mod + "internal/app", false},
		{mod + "internal/adapters/watch", mygo, true},
		{mod + "internal/views/shell", mod + "internal/core/text", false},
		{mod + "internal/views/shell", mod + "internal/adapters/watch", false},
		{mod + "internal/views/shell", mod + "internal/app", false},
		{mod + "internal/views/shell", mod + "internal/mock", false},
		{mod + "internal/views/shell", mygo, false},
		{mod + "internal/views/shell", mygo + "/ui/richtext", true},
		{mod + "internal/views/shell", mod + "internal/views/kit", true},
		{mod + "internal/mock", mod + "internal/core/text", false},
		{mod + "internal/mock", mod + "internal/views/kit", false},
		{mod + "internal/mock", mod + "internal/adapters/watch", false},
		{mod + "internal/mock", mygo + "/ui/markdown/parser", true},
		{mod + "internal/mock", mygo + "/ui", false},
		{mod + "internal/app", mod + "internal/core/text", true},
		{mod + "internal/app", mygo + "/ui", true},
	} {
		got := len(violations(tt.pkg, []string{tt.imp}, false)) == 0
		if got != tt.allowed {
			t.Errorf("%s importing %s: allowed = %v, want %v", tt.pkg, tt.imp, got, tt.allowed)
		}
	}
	if len(violations(mod+"internal/views/shell", []string{mod + "internal/mock"}, true)) != 0 {
		t.Error("test files of views may use the mocks")
	}
}
