package tfutil_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// stringPointerValueExempt lists every call site of types.StringPointerValue
// in internal/resources and internal/datasources that is allowed to remain,
// keyed by file path (relative to the repo root) and then by the expression
// being assigned, with the reason it is exempt.
//
// Why this test exists: Dokploy represents an unset optional string two ways
// in the same record. A field never set reads back as JSON null; a field set
// and then cleared through the UI reads back as a literal "".
// types.StringPointerValue preserves the "", Terraform configuration that
// omits the attribute holds null, and the resulting `"" -> null` diff cannot
// be applied away. acf76ab fixed this as a manual sweep across project,
// application, database, domain and mount. Nothing enforced it, so the rule
// lived in a commit message.
//
// The two-level shape and the rot check below mirror
// internal/client/census_test.go's censusExempt deliberately: an exemption
// that outlives its subject is how these tables rot.
var stringPointerValueExempt = map[string]map[string]string{
	// applicationId and composeId are the mutually exclusive parent pointers
	// on a domain, and they are foreign keys rather than free text, which is
	// what takes them out of the "" hazard entirely. Probed live against
	// Dokploy v0.29.13 on 2026-07-29, on a domain with an application parent:
	//
	//   domain.one            -> "composeId": null          (never "")
	//   domain.update {"composeId": ""}     -> stored value stays null
	//   domain.update {"applicationId": ""} -> stored value keeps the old id
	//
	// So the server neither returns "" for the unset half nor accepts "" as a
	// value to store: an empty string is treated as absent on the way in. The
	// StringOrNull collapse would therefore be a no-op here, and using the
	// pointer form keeps this read path honest about a column whose only two
	// states are "a real id" and null.
	"internal/resources/domain/model.go": {
		"m.ApplicationID": "FK column, not free text; domain.one returns null for the unset half and domain.update refuses to store \"\" (verified live v0.29.13, 2026-07-29)",
		"m.ComposeID":     "same as ApplicationID; the mirror case (a domain with a compose parent) is probed in wave 5a task 6",
	},
}

// guardRoots are the trees this test walks. Only resource and data-source
// packages: internal/client models the wire, where a *string faithfully
// represents what the server sent, and collapsing there would destroy the
// very distinction the resource layer needs to make.
var guardRoots = []string{"../resources", "../datasources"}

func TestNoStringPointerValueOutsideExemptions(t *testing.T) {
	found, err := findStringPointerValueCalls(guardRoots)
	if err != nil {
		t.Fatalf("walking guard roots: %v", err)
	}

	// Every call site found must be exempt.
	for _, file := range sortedKeys(found) {
		for _, target := range sortedKeys(found[file]) {
			reason, ok := stringPointerValueExempt[file][target]
			if !ok {
				t.Errorf("%s: %s uses types.StringPointerValue, which preserves \"\" and produces an unappliable \"\" -> null diff. Use tfutil.StringOrNull, or add an exemption with a reason.", file, target)
				continue
			}
			if reason == "" {
				t.Errorf("%s: %s: exemptions must carry a reason", file, target)
			}
		}
	}

	// Every exemption must still name a real call site. This is the rot
	// check: an exemption for code that no longer exists is worse than no
	// exemption, because it reads as a live caveat.
	for _, file := range sortedKeys(stringPointerValueExempt) {
		for _, target := range sortedKeys(stringPointerValueExempt[file]) {
			if !found[file][target] {
				t.Errorf("stringPointerValueExempt[%q][%q]: no such call site any more - drop the exemption", file, target)
			}
		}
	}
}

// findStringPointerValueCalls returns file -> assignment target -> true for
// every types.StringPointerValue call under the given roots, skipping test
// files.
//
// It fails closed: a call in an expression form it cannot render is an
// error, not a silent pass. A guard that quietly ignores what it does not
// understand is the failure mode blind_field_test.go was written after -
// SaveApplicationEnvironment hid two pinned fields inside an inline map for
// three releases precisely because reflection could not see them.
func findStringPointerValueCalls(roots []string) (map[string]map[string]bool, error) {
	found := map[string]map[string]bool{}

	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}

			fset := token.NewFileSet()
			file, perr := parser.ParseFile(fset, path, nil, 0)
			if perr != nil {
				return fmt.Errorf("parsing %s: %w", path, perr)
			}
			key := repoRelative(path)

			var walkErr error
			fail := func(pos token.Pos, form string) {
				if walkErr == nil {
					walkErr = fmt.Errorf("%s:%d: types.StringPointerValue used as %s - extend the guard to render this form", key, fset.Position(pos).Line, form)
				}
			}

			ast.Inspect(file, func(n ast.Node) bool {
				switch node := n.(type) {
				case *ast.AssignStmt:
					for i, rhs := range node.Rhs {
						if !isStringPointerValue(rhs) || i >= len(node.Lhs) {
							continue
						}
						record(found, key, render(node.Lhs[i]))
					}
				case *ast.ValueSpec:
					// var x = types.StringPointerValue(p)
					for i, v := range node.Values {
						if !isStringPointerValue(v) || i >= len(node.Names) {
							continue
						}
						record(found, key, render(node.Names[i]))
					}
				case *ast.KeyValueExpr:
					if isStringPointerValue(node.Value) {
						record(found, key, render(node.Key))
					}
				case *ast.CallExpr:
					// A call used as a bare argument is a form this guard
					// cannot attribute to a target.
					for _, arg := range node.Args {
						if isStringPointerValue(arg) {
							fail(node.Pos(), "a call argument")
						}
					}
				case *ast.ReturnStmt:
					for _, res := range node.Results {
						if isStringPointerValue(res) {
							fail(node.Pos(), "a return value")
						}
					}
				}
				return true
			})
			return walkErr
		})
		if err != nil {
			return nil, err
		}
	}
	return found, nil
}

func record(found map[string]map[string]bool, file, target string) {
	if found[file] == nil {
		found[file] = map[string]bool{}
	}
	found[file][target] = true
}

// isStringPointerValue reports whether e is a call to types.StringPointerValue
// or to the basetypes constructor it aliases.
//
// The package qualifier is deliberately NOT checked. types.StringPointerValue
// is an alias for basetypes.NewStringPointerValue, and either spelling - or an
// import alias for either package - produces the same "" preserving value. A
// guard keyed on the literal identifier `types` would be evaded by a one-word
// import alias, which is precisely the kind of hole that makes a guard worse
// than none. Matching on the function name alone can in principle collide with
// an unrelated same-named function; that would surface as a failure demanding
// an exemption with a reason, which is the correct fail-closed outcome.
func isStringPointerValue(e ast.Expr) bool {
	call, ok := e.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	return sel.Sel.Name == "StringPointerValue" || sel.Sel.Name == "NewStringPointerValue"
}

// render prints a selector or identifier expression as source text, e.g.
// "m.ApplicationID". Anything else renders as "?" and will not match an
// exemption, so it fails loudly rather than silently.
func render(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		return render(v.X) + "." + v.Sel.Name
	default:
		return "?"
	}
}

// repoRelative turns "../resources/domain/model.go" into
// "internal/resources/domain/model.go" so exemption keys read as repo paths.
func repoRelative(path string) string {
	return "internal/" + strings.TrimPrefix(filepath.ToSlash(path), "../")
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
