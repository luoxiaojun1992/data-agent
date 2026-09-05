package migration

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestSeedConsistency_AllPermConstantsSeeded enforces the SPEC-084 §5.3
// "三处同步铁律": every PermXxx permission constant declared in
// domain/model/rbac.go MUST have a corresponding seed entry in rbac_seed.go's
// perms array. A constant that is defined but not seeded would make a fresh
// (empty-DB) deployment return 403 on any route that RequirePermission-guards
// with it, because the permission document would not exist after seeding.
func TestSeedConsistency_AllPermConstantsSeeded(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	pkgDir := filepath.Dir(thisFile)
	rbacGo := filepath.Join(pkgDir, "..", "..", "..", "internal", "domain", "model", "rbac.go")
	seedGo := filepath.Join(pkgDir, "rbac_seed.go")

	constNames := parsePermConstants(t, rbacGo)
	if len(constNames) == 0 {
		t.Fatalf("no PermXxx constants found in %s", rbacGo)
	}
	seedRefs := parsePermRefs(t, seedGo)

	var missing []string
	for name, key := range constNames {
		if !seedRefs[name] {
			missing = append(missing, name+" ("+key+")")
		}
	}
	if len(missing) > 0 {
		t.Errorf("以下权限常量未在 rbac_seed.go 的 perms 数组中 seed（三处同步铁律违反，重新部署会 403）:\n  %s",
			strings.Join(missing, "\n  "))
	}
}

// parsePermConstants returns every `PermXxx = "key"` string constant in a Go
// source file, keyed by constant name.
func parsePermConstants(t *testing.T, path string) map[string]string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	out := map[string]string{}
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range vs.Names {
				if !strings.HasPrefix(name.Name, "Perm") {
					continue
				}
				if i < len(vs.Values) {
					if bl, ok := vs.Values[i].(*ast.BasicLit); ok && bl.Kind == token.STRING {
						out[name.Name] = strings.Trim(bl.Value, `"`)
					}
				}
			}
		}
	}
	return out
}

// parsePermRefs returns every `model.PermXxx` selector reference in a Go source
// file, keyed by the selector name (e.g. "PermAgentView").
func parsePermRefs(t *testing.T, path string) map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	out := map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if x, ok := sel.X.(*ast.Ident); ok && x.Name == "model" && strings.HasPrefix(sel.Sel.Name, "Perm") {
			out[sel.Sel.Name] = true
		}
		return true
	})
	return out
}
