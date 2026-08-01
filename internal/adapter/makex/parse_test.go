package makex

// Table tests for the pure parsing core. These are the reason the parser was
// split from its impure edges (ADR-M002): every case here runs without make on
// the box and without touching the filesystem, so the inputs are visible in the
// test rather than produced by a subprocess.
//
// Known limit, stated rather than engineered around: a hand-trimmed database
// sample can drift from what make really emits. TestCharacterization in
// internal/app covers that — it runs real make over the fixtures end to end.
// These tests cover the parsing logic; that oracle covers the input assumption.

import (
	"testing"

	"github.com/Gaetan-Jaminon/mkx/internal/domain"
)

// sampleSource is the Makefile both samples below describe. It carries phony
// and non-phony targets, targets with and without prerequisites, targets with
// and without a `## comment`, and a pattern rule.
const sampleSource = `.PHONY: all clean

all: build test ## Build and test everything
	@echo all

build: main.o ## Compile
	@echo build

test: build
	@echo test

main.o: main.c
	@echo obj

clean:
	@echo clean

%.o: %.c
	@echo pattern
`

// sampleDatabase is real `make -pRrq` output for sampleSource (GNU Make 4.3),
// trimmed to the sections the parser reads and with the per-target status
// comments shortened. Three properties of the real output are load-bearing and
// preserved verbatim:
//
//   - "# Implicit Rules" precedes "# Files", so the pattern rule sits in the
//     preamble the parser skips.
//   - make prints targets in hash order, not source order.
//   - ".PHONY:" is printed after the targets it describes, which is why
//     parseDatabase reads the database in two passes.
const sampleDatabase = `# GNU Make 4.3
# Make data base, printed on Sat Aug  1 10:20:46 2026

# Implicit Rules

%.o: %.c
#  recipe to execute (from 'Makefile', line 19):
	@echo pattern

# 1 implicit rules, 0 (0.0%) terminal.
# Files

# Not a target:
Makefile:
#  Implicit rule search has been done.
#  File has been updated.

clean:
#  Phony target (prerequisite of .PHONY).
#  File does not exist.
#  recipe to execute (from 'Makefile', line 16):
	@echo clean

# Not a target:
.DEFAULT:
#  Implicit rule search has not been done.

all: build test
#  Phony target (prerequisite of .PHONY).
#  File does not exist.
#  recipe to execute (from 'Makefile', line 4):
	@echo all

build: main.o
#  Implicit rule search has not been done.
#  File does not exist.
#  recipe to execute (from 'Makefile', line 7):
	@echo build

# Not a target:
main.c:
#  Implicit rule search has been done.
#  File does not exist.

test: build
#  Implicit rule search has not been done.
#  Modification time never checked.
#  recipe to execute (from 'Makefile', line 10):
	@echo test

main.o: main.c
#  Implicit rule search has not been done.
#  File does not exist.
#  recipe to execute (from 'Makefile', line 13):
	@echo obj

.PHONY: all clean
#  Implicit rule search has not been done.

# files hash-table stats:
# Load=9/1024=1%, Rehash=0, Collisions=0/27=0%
`

// databaseWant is what sampleDatabase parses to, before descriptions are
// applied. Order is make's, not alphabetical — Discover sorts afterwards.
func databaseWant() []domain.Target {
	return []domain.Target{
		{Name: "clean", Phony: domain.PhonyYes},
		{Name: "all", Phony: domain.PhonyYes, Prerequisites: []string{"build", "test"}},
		{Name: "build", Phony: domain.PhonyNo, Prerequisites: []string{"main.o"}},
		{Name: "test", Phony: domain.PhonyNo, Prerequisites: []string{"build"}},
		{Name: "main.o", Phony: domain.PhonyNo, Prerequisites: []string{"main.c"}},
	}
}

// TestParseDatabase covers the primary `make -pRrq` path: which lines count as
// targets, the phony set read from the resolved `.PHONY:` entry, and make's
// resolved prerequisite lists.
func TestParseDatabase(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want []domain.Target
	}{
		{
			// clean is phony with no prerequisites, all is phony with two,
			// build/test/main.o are not phony and each has one. The pattern
			// rule and the "# Not a target:" entries are absent from the result.
			name: "phony and non-phony, with and without prerequisites",
			out:  sampleDatabase,
			want: databaseWant(),
		},
		{
			// No .PHONY entry at all: the phony set is empty, so every target
			// reads PhonyNo. That is the honest answer rather than a degraded
			// one — make omits the entry precisely because nothing is phony.
			name: "no .PHONY entry means every target is PhonyNo",
			out:  "# Files\n\nbuild: main.o\n\t@echo build\n",
			want: []domain.Target{
				{Name: "build", Phony: domain.PhonyNo, Prerequisites: []string{"main.o"}},
			},
		},
		{
			// Two .PHONY declarations in a Makefile merge into one resolved
			// line, so the parser never has to accumulate across entries — but
			// it must still read every name on the line it does get.
			name: "every name on the .PHONY line is read",
			out:  "# Files\n\nall:\n\nbuild:\n\nclean:\n\n.PHONY: all clean\n",
			want: []domain.Target{
				{Name: "all", Phony: domain.PhonyYes},
				{Name: "build", Phony: domain.PhonyNo},
				{Name: "clean", Phony: domain.PhonyYes},
			},
		},
		{
			// The same target printed twice keeps its first appearance.
			name: "duplicate target lines are collapsed",
			out:  "# Files\n\nbuild: first\n\nbuild: second\n",
			want: []domain.Target{
				{Name: "build", Phony: domain.PhonyNo, Prerequisites: []string{"first"}},
			},
		},
		{
			// "|" is make's separator for order-only prerequisites, not a
			// prerequisite itself. Verified against GNU Make 4.3, which prints
			// the line as written: "all: build | dir".
			name: "the order-only separator is not a prerequisite",
			out:  "# Files\n\nall: build | dir\n",
			want: []domain.Target{
				{Name: "all", Phony: domain.PhonyNo, Prerequisites: []string{"build", "dir"}},
			},
		},
		{
			// A double-colon rule splits on its first colon, leaving the second
			// in the prerequisite text. It is syntax, not a prerequisite.
			name: "the residue of a double-colon rule is not a prerequisite",
			out:  "# Files\n\nboth:: one\n",
			want: []domain.Target{
				{Name: "both", Phony: domain.PhonyNo, Prerequisites: []string{"one"}},
			},
		},
		{
			// A target whose only prerequisites are order-only still reports
			// nil rather than an empty slice.
			name: "dropping every token leaves nil, not an empty slice",
			out:  "# Files\n\nall: |\n",
			want: []domain.Target{
				{Name: "all", Phony: domain.PhonyNo},
			},
		},
		{
			// Everything above the "# Files" marker is preamble: variables,
			// implicit rules, and the directory stack all contain colons.
			name: "target-shaped lines above # Files are ignored",
			out:  "CFLAGS := -O2\n\n# Implicit Rules\n\n%.o: %.c\n\ndecoy: not-a-target\n",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertTargets(t, parseDatabase(tt.out), tt.want)
		})
	}
}

// TestParseMakefile covers the regex fallback. Its defining property is what it
// does *not* report: phony status stays PhonyUnknown and Prerequisites stays
// nil, so a fallback-discovered target is never mistaken for one make resolved.
func TestParseMakefile(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []domain.Target
	}{
		{
			// Recorded as-built, and narrower than the database path in two
			// ways worth seeing side by side with databaseWant(): the fallback
			// reports PhonyUnknown and nil prerequisites for everything, and it
			// does not see `test` or `main.o` at all — its regex matches a
			// target carrying prerequisites only when a `##` comment follows.
			name: "fallback reports PhonyUnknown and nil prerequisites",
			src:  sampleSource,
			want: []domain.Target{
				{Name: "all", Description: "Build and test everything", Phony: domain.PhonyUnknown},
				{Name: "build", Description: "Compile", Phony: domain.PhonyUnknown},
				{Name: "clean", Description: "", Phony: domain.PhonyUnknown},
			},
		},
		{
			name: "the .PHONY declaration is not itself a target",
			src:  ".PHONY: all clean\n\nall: ## Everything\n",
			want: []domain.Target{
				{Name: "all", Description: "Everything", Phony: domain.PhonyUnknown},
			},
		},
		{
			name: "duplicate target lines are collapsed",
			src:  "build: ## First\nbuild: ## Second\n",
			want: []domain.Target{
				{Name: "build", Description: "First", Phony: domain.PhonyUnknown},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertTargets(t, parseMakefile(tt.src), tt.want)
		})
	}
}

// TestDescriptions covers the `## comment` convention on its own, and applied
// over database results — the composition Discover performs when the primary
// path succeeds.
func TestDescriptions(t *testing.T) {
	t.Run("read from source", func(t *testing.T) {
		tests := []struct {
			name string
			src  string
			want map[string]string
		}{
			{
				name: "only the targets that carry a ## comment",
				src:  sampleSource,
				want: map[string]string{
					"all":   "Build and test everything",
					"build": "Compile",
				},
			},
			{
				name: "a single # is not the convention",
				src:  "build: # Compile\n",
				want: map[string]string{},
			},
			{
				name: "surrounding whitespace is trimmed",
				src:  "build:   ##    Compile   \n",
				want: map[string]string{"build": "Compile"},
			},
			{
				name: "no Makefile source at all",
				src:  "",
				want: map[string]string{},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got := descriptions(tt.src)
				if len(got) != len(tt.want) {
					t.Fatalf("got %d descriptions, want %d: %v", len(got), len(tt.want), got)
				}
				for name, want := range tt.want {
					if got[name] != want {
						t.Errorf("%s: got %q, want %q", name, got[name], want)
					}
				}
			})
		}
	})

	t.Run("applied over database results", func(t *testing.T) {
		targets := parseDatabase(sampleDatabase)
		applyDescriptions(targets, descriptions(sampleSource))

		want := databaseWant()
		want[1].Description = "Build and test everything" // all
		want[2].Description = "Compile"                   // build

		assertTargets(t, targets, want)
	})

	t.Run("a target with no entry keeps the description it has", func(t *testing.T) {
		targets := []domain.Target{{Name: "build", Description: "from the fallback"}}
		applyDescriptions(targets, map[string]string{"other": "unrelated"})

		assertTargets(t, targets, []domain.Target{
			{Name: "build", Description: "from the fallback"},
		})
	})
}

// TestZeroTargets covers a Makefile that produces no targets on either path:
// an empty result rather than a panic, which is what lets Discover fall through
// from the primary path to the fallback and then report nothing.
func TestZeroTargets(t *testing.T) {
	// A Makefile holding only a variable and a comment declares no rules, so
	// make's "# Files" section contains nothing but its own bookkeeping.
	const emptyDatabase = `# Files

# Not a target:
Makefile:
#  Implicit rule search has been done.

# files hash-table stats:
`
	const emptySource = "# a comment\nCFLAGS := -O2\n"

	tests := []struct {
		name string
		got  []domain.Target
	}{
		{name: "database path, empty # Files section", got: parseDatabase(emptyDatabase)},
		{name: "database path, make produced no output at all", got: parseDatabase("")},
		{name: "fallback path, no rules in the source", got: parseMakefile(emptySource)},
		{name: "fallback path, no Makefile to read", got: parseMakefile("")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.got) != 0 {
				t.Errorf("got %d targets, want 0: %+v", len(tt.got), tt.got)
			}
		})
	}
}

// TestPhonyStatusString pins the strings the characterization golden records,
// so a rename shows up here rather than only as golden churn.
func TestPhonyStatusString(t *testing.T) {
	tests := []struct {
		status domain.PhonyStatus
		want   string
	}{
		{domain.PhonyUnknown, "unknown"},
		{domain.PhonyNo, "no"},
		{domain.PhonyYes, "yes"},
	}

	for _, tt := range tests {
		if got := tt.status.String(); got != tt.want {
			t.Errorf("PhonyStatus(%d).String() = %q, want %q", tt.status, got, tt.want)
		}
	}
}

// assertTargets compares parsed targets against the expectation in order, field
// by field. domain.Target holds a slice and so is not comparable with ==; the
// helper is hand-rolled to avoid taking a module dependency for it.
//
// nil and empty prerequisite lists are treated as distinct: the parser
// normalises "no prerequisites" to nil deliberately, and a helper that blurred
// the two would stop checking that.
func assertTargets(t *testing.T, got, want []domain.Target) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("got %d targets, want %d:\n got: %+v\nwant: %+v", len(got), len(want), got, want)
	}
	for i := range want {
		g, w := got[i], want[i]
		if g.Name != w.Name {
			t.Errorf("target %d name: got %q, want %q", i, g.Name, w.Name)
		}
		if g.Description != w.Description {
			t.Errorf("target %d (%s) description: got %q, want %q", i, w.Name, g.Description, w.Description)
		}
		if g.Phony != w.Phony {
			t.Errorf("target %d (%s) phony: got %s, want %s", i, w.Name, g.Phony, w.Phony)
		}
		if (g.Prerequisites == nil) != (w.Prerequisites == nil) {
			t.Errorf("target %d (%s) prerequisites: got %#v, want %#v", i, w.Name, g.Prerequisites, w.Prerequisites)
			continue
		}
		if len(g.Prerequisites) != len(w.Prerequisites) {
			t.Errorf("target %d (%s) prerequisites: got %q, want %q", i, w.Name, g.Prerequisites, w.Prerequisites)
			continue
		}
		for j := range w.Prerequisites {
			if g.Prerequisites[j] != w.Prerequisites[j] {
				t.Errorf("target %d (%s) prerequisites: got %q, want %q", i, w.Name, g.Prerequisites, w.Prerequisites)
				break
			}
		}
	}
}
