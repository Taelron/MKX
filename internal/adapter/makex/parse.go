package makex

import (
	"bufio"
	"regexp"
	"strings"

	"github.com/Gaetan-Jaminon/mkx/internal/domain"
)

// This file holds the parsing half of the Make adapter. Every function here is
// pure: it takes captured text and returns values, reading no file and running
// no subprocess. The impure edges — running make, reading the Makefile — live
// in makex.go, so the parser is testable without make on the box (ADR-M002).

// parseDatabase extracts targets from the output of `make -pRrq`.
//
// It reads the database in two passes because `.PHONY:` is printed after the
// targets it describes: the first pass collects the target lines and the phony
// set, the second stamps each target with its status. A Makefile declaring no
// phony targets produces no `.PHONY:` entry at all, so the set is empty and
// every target is correctly PhonyNo.
//
// The caller is responsible for pinning the locale on the make invocation —
// the section markers this keys on are English (ADR-M002).
func parseDatabase(out string) []domain.Target {
	var targets []domain.Target
	seen := make(map[string]bool)
	phony := make(map[string]bool)
	scanner := bufio.NewScanner(strings.NewReader(out))
	inFiles := false

	for scanner.Scan() {
		line := scanner.Text()

		// skip the "# Files" section marker
		if line == "# Files" {
			inFiles = true
			continue
		}
		if strings.HasPrefix(line, "# Not a target:") {
			// next line is not a real target, skip it
			if scanner.Scan() {
				// consumed the fake target line
			}
			continue
		}

		if !inFiles {
			continue
		}

		// The resolved .PHONY entry, read before the leading-dot skip below
		// discards it. Multiple .PHONY declarations in the Makefile merge into
		// this one line. Phony status is taken from here rather than from the
		// per-target comment make prints, because a comment is localised prose
		// while this is target-name text.
		if declared, ok := strings.CutPrefix(line, ".PHONY:"); ok {
			for _, name := range strings.Fields(declared) {
				phony[name] = true
			}
			continue
		}

		// target lines look like "name: deps"
		if len(line) == 0 || line[0] == '#' || line[0] == '\t' || line[0] == '.' {
			continue
		}

		idx := strings.IndexByte(line, ':')
		if idx <= 0 {
			continue
		}

		name := strings.TrimSpace(line[:idx])

		// filter out pattern rules, variable refs, internal targets
		if strings.ContainsAny(name, "%$") {
			continue
		}
		if strings.HasPrefix(name, ".") {
			continue
		}
		if seen[name] {
			continue
		}
		seen[name] = true

		targets = append(targets, domain.Target{
			Name:          name,
			Prerequisites: prerequisites(line[idx+1:]),
		})
	}

	for i := range targets {
		if phony[targets[i].Name] {
			targets[i].Phony = domain.PhonyYes
		} else {
			targets[i].Phony = domain.PhonyNo
		}
	}

	return targets
}

// prerequisites splits the text after a target line's colon.
//
// Two tokens make's syntax puts in that text are not prerequisites and are
// dropped: "|" separates normal prerequisites from order-only ones, and ":" is
// what remains of the second colon in a double-colon rule, which the caller's
// split on the first colon leaves behind.
//
// It returns nil rather than an empty slice when there are none: strings.Fields
// yields a non-nil empty slice for empty input, and the distinction between no
// prerequisites and unknown prerequisites is visible to consumers.
func prerequisites(after string) []string {
	var prereqs []string
	for _, field := range strings.Fields(after) {
		if field == "|" || field == ":" {
			continue
		}
		prereqs = append(prereqs, field)
	}
	return prereqs
}

var makeTargetRe = regexp.MustCompile(`^([a-zA-Z_][a-zA-Z0-9_.-]*):\s*(?:.*?##\s*(.*))?$`)

// parseMakefile extracts targets from Makefile source — the fallback used when
// make errors or the database yields nothing.
//
// It populates neither Phony nor Prerequisites. Phony therefore stays
// PhonyUnknown, which is the ADR-M002 requirement, and Prerequisites stays nil:
// the regex sees as-written, unexpanded text ($(OBJS) stays literal), which is
// a different thing from make's resolved list. Recording one as the other is
// the same conflation the ADR forbids for phony.
func parseMakefile(src string) []domain.Target {
	var targets []domain.Target
	seen := make(map[string]bool)

	scanner := bufio.NewScanner(strings.NewReader(src))
	for scanner.Scan() {
		matches := makeTargetRe.FindStringSubmatch(scanner.Text())
		if matches == nil {
			continue
		}
		name := matches[1]
		if seen[name] {
			continue
		}
		seen[name] = true
		targets = append(targets, domain.Target{
			Name:        name,
			Description: strings.TrimSpace(matches[2]),
		})
	}
	return targets
}

var descRe = regexp.MustCompile(`^([a-zA-Z_][a-zA-Z0-9_.-]*):\s*.*?##\s*(.*)$`)

// descriptions reads the `## comment` convention out of Makefile source,
// keyed by target name. It is MkX's only demand on the Makefiles it reads.
func descriptions(src string) map[string]string {
	descs := make(map[string]string)

	scanner := bufio.NewScanner(strings.NewReader(src))
	for scanner.Scan() {
		if matches := descRe.FindStringSubmatch(scanner.Text()); matches != nil {
			descs[matches[1]] = strings.TrimSpace(matches[2])
		}
	}
	return descs
}

// applyDescriptions overlays descriptions onto targets in place. Targets with
// no entry keep whatever description they already carry.
func applyDescriptions(targets []domain.Target, descs map[string]string) {
	for i := range targets {
		if d, ok := descs[targets[i].Name]; ok {
			targets[i].Description = d
		}
	}
}
