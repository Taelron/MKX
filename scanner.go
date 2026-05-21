package main

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var defaultExcludes = map[string]bool{
	"node_modules": true,
	"vendor":       true,
	"__pycache__":  true,
	".terraform":   true,
	"build":        true,
	"dist":         true,
	"target":       true,
	"bin":          true,
	"obj":          true,
}

type project struct {
	Name        string
	Path        string
	Description string
	Targets     []target
}

func scanWorkspace(root string, excludes map[string]bool, maxDepth int) ([]project, error) {
	var projects []project

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		if !d.IsDir() {
			return nil
		}

		// skip root itself
		if path == root {
			return nil
		}

		name := d.Name()

		// skip hidden directories
		if strings.HasPrefix(name, ".") {
			return fs.SkipDir
		}

		// skip excluded directories
		if excludes[name] {
			return fs.SkipDir
		}

		// check depth
		rel, _ := filepath.Rel(root, path)
		depth := strings.Count(rel, string(filepath.Separator)) + 1
		if depth > maxDepth {
			return fs.SkipDir
		}

		// check for Makefile
		for _, mf := range []string{"Makefile", "makefile"} {
			if _, err := fs.Stat(os.DirFS(path), mf); err == nil {
				targets, _ := parseTargets(path)
				projects = append(projects, project{
					Name:        rel,
					Path:        path,
					Description: readProjectDescription(path),
					Targets:     targets,
				})
				break
			}
		}

		return nil
	})

	// order projects alphabetically by name, case-insensitive
	sort.Slice(projects, func(i, j int) bool {
		return strings.ToLower(projects[i].Name) < strings.ToLower(projects[j].Name)
	})

	return projects, err
}

// readProjectDescription extracts the first non-heading, non-empty line from README.md.
func readProjectDescription(dir string) string {
	path := findReadme(dir)
	if path == "" {
		return ""
	}

	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		return line
	}
	return ""
}
