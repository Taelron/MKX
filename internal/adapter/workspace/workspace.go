// Package workspace is the filesystem adapter: it walks the workspace looking
// for directories that hold a Makefile, and reads their README files.
package workspace

import (
	"bufio"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Gaetan-Jaminon/mkx/internal/app"
	"github.com/Gaetan-Jaminon/mkx/internal/domain"
)

// DefaultExcludes are the directory names the walk never descends into.
var DefaultExcludes = map[string]bool{
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

// DefaultMaxDepth is how many directory levels below the workspace root the
// walk descends.
const DefaultMaxDepth = 4

// Scanner walks a workspace root. Construct it with NewScanner.
type Scanner struct {
	excludes map[string]bool
	maxDepth int
}

var _ app.WorkspaceScanner = (*Scanner)(nil)

// NewScanner returns a Scanner that skips the named directories and descends
// at most maxDepth levels.
func NewScanner(excludes map[string]bool, maxDepth int) *Scanner {
	return &Scanner{excludes: excludes, maxDepth: maxDepth}
}

// Scan walks root and returns every directory below it that holds a Makefile.
// Targets are left empty; the app layer composes them in.
func (s *Scanner) Scan(_ context.Context, root string) ([]domain.Project, error) {
	var projects []domain.Project

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
		if s.excludes[name] {
			return fs.SkipDir
		}

		// check depth
		rel, _ := filepath.Rel(root, path)
		depth := strings.Count(rel, string(filepath.Separator)) + 1
		if depth > s.maxDepth {
			return fs.SkipDir
		}

		// check for Makefile
		for _, mf := range []string{"Makefile", "makefile"} {
			if _, err := fs.Stat(os.DirFS(path), mf); err == nil {
				projects = append(projects, domain.Project{
					Name:        rel,
					Path:        path,
					Description: readProjectDescription(path),
				})
				break
			}
		}

		return nil
	})

	return projects, err
}

// ReadmePath returns the README path for dir, or "" when it has none.
func (s *Scanner) ReadmePath(_ context.Context, dir string) string {
	return findReadme(dir)
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

// findReadme looks for a README file in the given directory.
func findReadme(dir string) string {
	candidates := []string{"README.md", "readme.md", "Readme.md", "README.MD"}
	for _, name := range candidates {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}
