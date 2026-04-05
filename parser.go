package main

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

type target struct {
	Name        string
	Description string
}

// parseTargets extracts Make targets from a project directory.
// Tries `make -pRrq` first, falls back to regex parsing.
func parseTargets(projectPath string) ([]target, error) {
	targets, err := parseWithMake(projectPath)
	if err != nil || len(targets) == 0 {
		return parseWithRegex(projectPath)
	}
	return targets, nil
}

func parseWithMake(projectPath string) ([]target, error) {
	cmd := exec.Command("make", "-pRrq", "-C", projectPath)
	cmd.Stdin = nil
	out, _ := cmd.Output() // make -q exits non-zero, that's OK

	var targets []target
	seen := make(map[string]bool)
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
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

		targets = append(targets, target{Name: name})
	}

	// enrich with descriptions from the Makefile comments
	enrichDescriptions(projectPath, targets)

	return targets, nil
}

var makeTargetRe = regexp.MustCompile(`^([a-zA-Z_][a-zA-Z0-9_.-]*):\s*(?:.*?##\s*(.*))?$`)

func parseWithRegex(projectPath string) ([]target, error) {
	for _, name := range []string{"Makefile", "makefile"} {
		f, err := os.Open(filepath.Join(projectPath, name))
		if err != nil {
			continue
		}
		defer f.Close()

		var targets []target
		seen := make(map[string]bool)
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			matches := makeTargetRe.FindStringSubmatch(scanner.Text())
			if matches == nil {
				continue
			}
			tName := matches[1]
			if seen[tName] {
				continue
			}
			seen[tName] = true
			targets = append(targets, target{
				Name:        tName,
				Description: strings.TrimSpace(matches[2]),
			})
		}
		return targets, nil
	}
	return nil, nil
}

var descRe = regexp.MustCompile(`^([a-zA-Z_][a-zA-Z0-9_.-]*):\s*.*?##\s*(.*)$`)

func enrichDescriptions(projectPath string, targets []target) {
	for _, name := range []string{"Makefile", "makefile"} {
		f, err := os.Open(filepath.Join(projectPath, name))
		if err != nil {
			continue
		}
		defer f.Close()

		descs := make(map[string]string)
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			matches := descRe.FindStringSubmatch(scanner.Text())
			if matches != nil {
				descs[matches[1]] = strings.TrimSpace(matches[2])
			}
		}

		for i := range targets {
			if d, ok := descs[targets[i].Name]; ok {
				targets[i].Description = d
			}
		}
		return
	}
}
