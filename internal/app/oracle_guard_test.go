package app_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// oracleTest is the test whose continued existence this file guards.
const oracleTest = "TestCharacterization"

// guardedGolden deliberately repeats the path from characterization_test.go
// rather than referencing the constant declared there. Sharing it would make
// this file fail to compile when that one is deleted — reporting an undefined
// identifier instead of the guard's actual complaint, in precisely the case the
// guard exists to explain.
const guardedGolden = "../../testdata/fixtures.golden"

// TestCharacterizationIsRegistered fails when the behaviour oracle stops
// existing.
//
// Without it, deleting or renaming TestCharacterization makes `go test ./...`
// run less and stay green: the only check on behaviour equivalence disappears
// and nothing reports it. That is the failure this guard converts into a red
// run.
//
// It is a static read of the package's source, so it does not depend on
// TestCharacterization executing — a guard that ran only when the guarded test
// ran would inherit the hole it exists to close. For the same reason it lives
// in its own file: deleting characterization_test.go leaves this behind to
// complain.
//
// Its limit, stated rather than engineered around: deleting both files defeats
// it. Nothing inside the suite can prevent that. What the guard buys is turning
// a silent one-file deletion into a deliberate two-file deletion that is
// obvious in review.
func TestCharacterizationIsRegistered(t *testing.T) {
	// The golden's presence, asserted here as well as in the oracle itself, so
	// a deleted golden is reported even if the oracle is the thing that went
	// missing.
	golden, err := os.ReadFile(guardedGolden)
	if err != nil {
		t.Errorf("golden %s: %v", guardedGolden, err)
	} else if strings.TrimSpace(string(golden)) == "" {
		t.Errorf("golden %s is empty", guardedGolden)
	}

	file, decl := findTestFunc(t, ".", oracleTest)
	if decl == nil {
		t.Fatalf("%s is not defined in any _test.go file in this package. "+
			"The behaviour oracle has been deleted or renamed; restore it rather than removing this guard.", oracleTest)
	}

	if tag := buildConstraint(t, file); tag != "" {
		t.Errorf("%s is excluded from normal builds by %q in %s; it would never run",
			oracleTest, tag, filepath.Base(file))
	}

	if skip := findSkipCall(decl); skip != "" {
		t.Errorf("%s calls t.%s, so the behaviour oracle does not run", oracleTest, skip)
	}
}

// findTestFunc returns the path and declaration of the named function, or a nil
// declaration if no _test.go file in dir declares it.
func findTestFunc(t *testing.T, dir, name string) (string, *ast.FuncDecl) {
	t.Helper()

	paths, err := filepath.Glob(filepath.Join(dir, "*_test.go"))
	if err != nil {
		t.Fatalf("globbing %s: %v", dir, err)
	}

	fset := token.NewFileSet()
	for _, path := range paths {
		parsed, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		for _, d := range parsed.Decls {
			if fn, ok := d.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Name.Name == name {
				return path, fn
			}
		}
	}
	return "", nil
}

// buildConstraint returns the build-constraint line guarding path, or "" if the
// file carries none. Only the text above the package clause can constrain the
// file, so that is all it reads.
func buildConstraint(t *testing.T, path string) string {
	t.Helper()

	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}

	header, _, _ := strings.Cut(string(src), "\npackage ")
	for _, line := range strings.Split(header, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "//go:build") || strings.HasPrefix(line, "// +build") {
			return line
		}
	}
	return ""
}

// findSkipCall returns the name of the first Skip-family call in the function
// body, or "" if it contains none.
func findSkipCall(fn *ast.FuncDecl) string {
	var found string
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if found != "" {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && strings.HasPrefix(sel.Sel.Name, "Skip") {
			found = sel.Sel.Name
			return false
		}
		return true
	})
	return found
}
