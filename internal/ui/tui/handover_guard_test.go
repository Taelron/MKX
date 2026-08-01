package tui

// The structural half of ADR-M003's "RepoState is invalidated by every
// handover".
//
// Update invalidates on the handoverDone wrapper, which makes invalidation
// automatic for any handover that goes *through* handover(). This guard covers
// the ones that do not: a tea.Exec written directly, returning its own message
// or nil, bypasses the wrapper entirely and silently keeps a stale RepoState
// on screen. That is not hypothetical — readme.go did exactly this before
// TAE-58, returning nil from its tea.Exec callback, so Update never saw the
// handover at all.
//
// The check is static over the package's own source, in the spirit of
// internal/app/oracle_guard_test.go: an invariant made un-forgettable rather
// than trusted to a reviewer noticing.
//
// There is deliberately no file allowlist. Which files hold a tea.Exec today
// is exactly the fact a new file makes stale, and an allowlist would let that
// new file pass by not being on it.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// wrapperType is the type every tea.Exec callback must construct.
const wrapperType = "handoverDone"

// execViolation is one tea.Exec whose callback does not construct the wrapper.
type execViolation struct {
	file string
	line int
	why  string
}

// checkExecCallbacks reports every tea.Exec call in a parsed file whose
// completion callback fails to construct wrapperType.
//
// It is factored as a pure function over an already-parsed file — rather than
// as a test that walks the tree itself — for one reason: it can then be driven
// from a table of synthetic sources below. A guard that has only ever been run
// against compliant code has demonstrated nothing except that it does not
// crash.
func checkExecCallbacks(fset *token.FileSet, file *ast.File, name string) []execViolation {
	var violations []execViolation

	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Exec" {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok || pkg.Name != "tea" {
			return true
		}

		line := fset.Position(call.Pos()).Line

		// tea.Exec(cmd, callback) — the callback is the second argument.
		if len(call.Args) < 2 {
			violations = append(violations, execViolation{
				file: name, line: line,
				why: "tea.Exec called with no completion callback",
			})
			return true
		}

		if !constructsWrapper(call.Args[1]) {
			violations = append(violations, execViolation{
				file: name, line: line,
				why: "the tea.Exec callback does not construct " + wrapperType,
			})
		}
		return true
	})

	return violations
}

// constructsWrapper reports whether expr — a tea.Exec completion callback —
// contains a composite literal of wrapperType anywhere in its body.
//
// Containment rather than "returns exactly": handoverDone{inner: done(ce)} is
// the compliant shape in exec.go, and a callback that builds one in a local
// before returning it is equally fine. What is being caught is a callback that
// never mentions the wrapper at all.
func constructsWrapper(expr ast.Expr) bool {
	found := false
	ast.Inspect(expr, func(n ast.Node) bool {
		if found {
			return false
		}
		lit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		if id, ok := lit.Type.(*ast.Ident); ok && id.Name == wrapperType {
			found = true
			return false
		}
		return true
	})
	return found
}

// TestEveryHandoverInvalidatesRepoState is the guard proper: it scans this
// package's real source.
func TestEveryHandoverInvalidatesRepoState(t *testing.T) {
	fset := token.NewFileSet()

	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("globbing package sources: %v", err)
	}

	var violations []execViolation
	execSites := 0

	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		execSites += countExecCalls(parsed)
		violations = append(violations, checkExecCallbacks(fset, parsed, path)...)
	}

	for _, v := range violations {
		t.Errorf("%s:%d: %s. Per ADR-M003 every terminal handover invalidates RepoState, "+
			"and Update keys that on %s. Route this through handover(), or return %s{} from the callback.",
			v.file, v.line, v.why, wrapperType, wrapperType)
	}

	// Non-vacuity. A glob that matched nothing, or a tea.Exec detector that
	// stopped detecting, would leave the loop above with nothing to complain
	// about and report green. Two are known to exist: the shared handover() in
	// exec.go and the README viewer in readme.go.
	if execSites < 2 {
		t.Errorf("found %d tea.Exec call sites, expected at least 2; "+
			"the scan is not reaching this package's source and the guard is passing vacuously", execSites)
	}
}

// countExecCalls counts tea.Exec calls, whatever their callbacks look like.
func countExecCalls(file *ast.File) int {
	n := 0
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Exec" {
			if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "tea" {
				n++
			}
		}
		return true
	})
	return n
}

// TestHandoverGuardRejectsBypasses is the half that matters.
//
// A green run of a verification mechanism is the weakest possible evidence it
// works — it is equally consistent with a checker that returns "fine" for
// everything. So the checker is driven over synthetic sources that are known
// bypasses, including the exact shape readme.go had before this issue, and
// asserted to reject each one.
func TestHandoverGuardRejectsBypasses(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		wantBad int
	}{
		{
			name:    "compliant: wrapper constructed inline",
			wantBad: 0,
			src: `package tui
func f() tea.Cmd {
	return tea.Exec(ce, func(error) tea.Msg { return handoverDone{inner: done(ce)} })
}`,
		},
		{
			name:    "compliant: bare wrapper, nothing to report",
			wantBad: 0,
			src: `package tui
func f() tea.Cmd {
	return tea.Exec(&readmeExec{path: p}, func(error) tea.Msg { return handoverDone{} })
}`,
		},
		{
			name:    "compliant: wrapper built in a local first",
			wantBad: 0,
			src: `package tui
func f() tea.Cmd {
	return tea.Exec(ce, func(error) tea.Msg {
		msg := handoverDone{inner: execFinishedMsg{}}
		return msg
	})
}`,
		},
		{
			// readme.go's shape before TAE-58. Update never saw this handover,
			// so no rule expressed as "invalidate in the message's case" could
			// ever have covered it.
			name:    "bypass: callback returns nil",
			wantBad: 1,
			src: `package tui
func f() tea.Cmd {
	return tea.Exec(&readmeExec{path: p}, func(error) tea.Msg { return nil })
}`,
		},
		{
			name:    "bypass: callback returns an unwrapped message",
			wantBad: 1,
			src: `package tui
func f() tea.Cmd {
	return tea.Exec(ce, func(error) tea.Msg { return execFinishedMsg{exitCode: 0} })
}`,
		},
		{
			// The subtlest one: a message type whose *name* resembles the
			// wrapper is not the wrapper.
			name:    "bypass: a differently-named composite literal",
			wantBad: 1,
			src: `package tui
func f() tea.Cmd {
	return tea.Exec(ce, func(error) tea.Msg { return handoverFinished{} })
}`,
		},
		{
			name:    "bypass: two unwrapped handovers are both reported",
			wantBad: 2,
			src: `package tui
func f() tea.Cmd {
	if x {
		return tea.Exec(a, func(error) tea.Msg { return nil })
	}
	return tea.Exec(b, func(error) tea.Msg { return gitPullFinishedMsg{} })
}`,
		},
		{
			name:    "bypass: one compliant and one not, only the bad one is reported",
			wantBad: 1,
			src: `package tui
func f() tea.Cmd {
	if x {
		return tea.Exec(a, func(error) tea.Msg { return handoverDone{} })
	}
	return tea.Exec(b, func(error) tea.Msg { return nil })
}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			parsed, err := parser.ParseFile(fset, tt.name+".go", tt.src, parser.SkipObjectResolution)
			if err != nil {
				t.Fatalf("parsing the synthetic source: %v", err)
			}

			got := checkExecCallbacks(fset, parsed, tt.name+".go")
			if len(got) != tt.wantBad {
				t.Fatalf("checkExecCallbacks reported %d violations, want %d: %+v", len(got), tt.wantBad, got)
			}
		})
	}
}

// TestHandoverGuardSeesTheRealCallSites pins the detector against the actual
// files, so a refactor that renamed or moved the handovers cannot leave the
// guard scanning a package with nothing in it and still reporting green.
func TestHandoverGuardSeesTheRealCallSites(t *testing.T) {
	fset := token.NewFileSet()

	for _, path := range []string{"exec.go", "readme.go"} {
		parsed, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		if n := countExecCalls(parsed); n == 0 {
			t.Errorf("%s holds no tea.Exec call; if the handover moved, move this expectation with it "+
				"rather than deleting it", path)
		}
	}
}
