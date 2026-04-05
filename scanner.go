package main

import (
	"io/fs"
	"os"
	"path/filepath"
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
	Name    string
	Path    string
	Targets []target
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
					Name:    rel,
					Path:    path,
					Targets: targets,
				})
				break
			}
		}

		return nil
	})

	return projects, err
}
