// Package architecture checks source imports independently of build constraints.
package architecture

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
)

const module = "github.com/tushardhara/dream/"

func within(path, prefix string) bool { return path == prefix || strings.HasPrefix(path, prefix+"/") }

func forbidden(from, target string) bool {
	local := strings.TrimPrefix(target, module)
	// Labels/judge outputs and their store may only be imported by the evaluator
	// and its explicit host. This also closes an adapter-mediated reverse import.
	if strings.HasPrefix(target, module) && (within(local, "evals") || within(local, "adapters/evaluation")) {
		return !within(from, "evals") && !within(from, "adapters/evaluation") && !within(from, "cmd/hws-eval")
	}

	if within(from, "core") {
		// Core has no external libraries or transport. Local helpers must live in core,
		// so an indirect dependency cannot tunnel through another local package.
		if strings.HasPrefix(target, module) {
			return !within(local, "core")
		}
		return strings.Contains(strings.Split(target, "/")[0], ".") || within(target, "net") || within(target, "os") || within(target, "syscall") || within(target, "database") || within(target, "plugin")
	}
	allowed := []string(nil)
	switch {
	case within(from, "app/graph"):
		allowed = []string{"core", "app/graph"}
	case within(from, "examples/graphclient"):
		allowed = []string{"core", "app/graph", "examples/graphclient"}
	case within(from, "simulator"):
		allowed = []string{"core", "simulator"}
	case within(from, "app/hws"):
		allowed = []string{"core", "simulator", "app/graph", "app/hws"}
	default:
		return false
	}
	if !strings.HasPrefix(target, module) {
		// Consumer packages stay standard-library-only until a dependency is reviewed.
		return strings.Contains(strings.Split(target, "/")[0], ".") || within(target, "net") || within(target, "os") || within(target, "syscall") || within(target, "database") || within(target, "plugin")
	}
	for _, p := range allowed {
		if within(local, p) {
			return false
		}
	}
	return true
}

// Check parses every .go file, including tests and platform/custom-tag variants.
// Fixtures, vendored dependencies and hidden directories are not production code.
func Check(root string) ([]string, error) {
	var violations []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && (d.Name() == "testdata" || d.Name() == "vendor" || strings.HasPrefix(d.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		rel, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range f.Imports {
			target, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				return err
			}
			if forbidden(filepath.ToSlash(rel), target) {
				violations = append(violations, fmt.Sprintf("%s imports %s", path, target))
			}
		}
		return nil
	})
	return violations, err
}
