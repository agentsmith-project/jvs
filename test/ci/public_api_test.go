package ci

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublicPackagesDoNotExposeTestHooks(t *testing.T) {
	root := repoRoot(t)
	pkgRoot := filepath.Join(root, "pkg")

	var checkedFiles int
	err := filepath.WalkDir(pkgRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case "testdata", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}

		checkedFiles++
		file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		ast.Inspect(file, func(node ast.Node) bool {
			switch n := node.(type) {
			case *ast.FuncDecl:
				assertNoPublicTestHook(t, root, path, n.Name.Name)
			case *ast.TypeSpec:
				assertNoPublicTestHook(t, root, path, n.Name.Name)
			case *ast.ValueSpec:
				for _, name := range n.Names {
					assertNoPublicTestHook(t, root, path, name.Name)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walk public packages: %v", err)
	}
	if checkedFiles == 0 {
		t.Fatalf("expected to check at least one public package file")
	}
}

func assertNoPublicTestHook(t *testing.T, root, path, name string) {
	t.Helper()
	if !ast.IsExported(name) || !strings.Contains(name, "ForTest") {
		return
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		rel = path
	}
	t.Errorf("%s exposes public test hook %s", rel, name)
}
