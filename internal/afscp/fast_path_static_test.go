package afscp

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFastPathProductionSourceRejectsSlowPathDependencies(t *testing.T) {
	for _, path := range afscpProductionSourceFiles(t) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			source := readAFSCPProductionSource(t, path)
			lowerSource := strings.ToLower(source)
			for _, fragment := range []string{
				"capacitygate",
				"saveprofile",
				"restoreplan",
				"compression",
				"content_root_hash",
				"payload_root_hash",
				"save_profile",
				"restore_plan",
				"restore-preview",
				"restore_run",
				"fsynctree",
				"copy fallback",
				"digest",
			} {
				if strings.Contains(lowerSource, fragment) {
					t.Fatalf("%s contains forbidden direct fast-path fragment %q", path, fragment)
				}
			}

			file := parseAFSCPProductionSource(t, path, source)
			assertAFSCPProductionImportsAreFastPathOnly(t, path, file)
			assertAFSCPProductionCallsAreFastPathOnly(t, path, file)
		})
	}
}

func afscpProductionSourceFiles(t *testing.T) []string {
	t.Helper()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read internal/afscp sources: %v", err)
	}
	var files []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		files = append(files, name)
	}
	return files
}

func readAFSCPProductionSource(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func parseAFSCPProductionSource(t *testing.T, path, source string) *ast.File {
	t.Helper()

	file, err := parser.ParseFile(token.NewFileSet(), path, source, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return file
}

func assertAFSCPProductionImportsAreFastPathOnly(t *testing.T, path string, file *ast.File) {
	t.Helper()

	for _, spec := range file.Imports {
		importPath := strings.Trim(spec.Path.Value, `"`)
		for _, forbidden := range []string{
			"internal/capacitygate",
			"internal/saveprofile",
			"internal/restoreplan",
			"internal/snapshotpayload",
			"internal/compression",
			"internal/integrity",
		} {
			if strings.Contains(importPath, forbidden) {
				t.Fatalf("%s imports forbidden direct fast-path dependency %q", path, importPath)
			}
		}
		if importPath == "crypto/sha256" && filepath.Base(path) != "direct_metadata.go" {
			t.Fatalf("%s imports crypto/sha256 outside metadata checksum implementation", path)
		}
	}
}

func assertAFSCPProductionCallsAreFastPathOnly(t *testing.T, path string, file *ast.File) {
	t.Helper()

	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		receiver, _ := selector.X.(*ast.Ident)
		callName := selector.Sel.Name
		qualified := callName
		if receiver != nil {
			qualified = receiver.Name + "." + callName
		}

		switch qualified {
		case "filepath.Walk", "filepath.WalkDir", "fsutil.FsyncTree":
			t.Fatalf("%s calls forbidden direct fast-path API %s", path, qualified)
		}
		if strings.Contains(strings.ToLower(callName), "copy") {
			t.Fatalf("%s calls forbidden direct copy fallback candidate %s", path, qualified)
		}
		if qualified == "sha256.Sum256" && filepath.Base(path) != "direct_metadata.go" {
			t.Fatalf("%s calls sha256 outside metadata checksum implementation", path)
		}
		return true
	})
}
