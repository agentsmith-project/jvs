package fsutil

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRenameNoReplaceAndSyncRenamesWhenDestinationMissing(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	require.NoError(t, os.WriteFile(src, []byte("source"), 0644))

	err := RenameNoReplaceAndSync(src, dst)
	if errors.Is(err, ErrRenameNoReplaceUnsupported) {
		t.Skip("platform does not support atomic rename no-replace")
	}
	require.NoError(t, err)

	_, err = os.Lstat(src)
	require.True(t, os.IsNotExist(err))
	data, err := os.ReadFile(dst)
	require.NoError(t, err)
	require.Equal(t, []byte("source"), data)
}

func TestRenameNoReplaceAndSyncDoesNotReplaceExistingDestination(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	require.NoError(t, os.WriteFile(src, []byte("source"), 0644))
	require.NoError(t, os.WriteFile(dst, []byte("destination"), 0644))

	err := RenameNoReplaceAndSync(src, dst)
	require.Error(t, err)
	require.False(t, IsCommitUncertain(err))
	if !errors.Is(err, ErrRenameNoReplaceUnsupported) {
		require.ErrorIs(t, err, os.ErrExist)
	}

	srcData, err := os.ReadFile(src)
	require.NoError(t, err)
	require.Equal(t, []byte("source"), srcData)
	dstData, err := os.ReadFile(dst)
	require.NoError(t, err)
	require.Equal(t, []byte("destination"), dstData)
}

func TestRenameNoReplaceAndSyncDoesNotFsyncWhenRenameUnsupported(t *testing.T) {
	oldRenameNoReplaceOp := renameNoReplaceOp
	oldFsyncDir := fsyncDir
	renameNoReplaceOp = func(oldpath, newpath string) error {
		return &os.LinkError{
			Op:  "rename no-replace",
			Old: oldpath,
			New: newpath,
			Err: ErrRenameNoReplaceUnsupported,
		}
	}
	fsyncDir = func(string) error {
		t.Fatal("fsync must not run after unsupported rename no-replace")
		return nil
	}
	t.Cleanup(func() {
		renameNoReplaceOp = oldRenameNoReplaceOp
		fsyncDir = oldFsyncDir
	})

	err := RenameNoReplaceAndSync("src", "dst")
	require.ErrorIs(t, err, ErrRenameNoReplaceUnsupported)
	require.False(t, IsCommitUncertain(err))
}

func TestRenameNoReplaceAndSyncFsyncFailureIsCommitUncertain(t *testing.T) {
	oldRenameNoReplaceOp := renameNoReplaceOp
	oldFsyncDir := fsyncDir
	renameNoReplaceOp = func(string, string) error {
		return nil
	}
	fsyncDir = func(string) error {
		return errors.New("injected directory fsync failure")
	}
	t.Cleanup(func() {
		renameNoReplaceOp = oldRenameNoReplaceOp
		fsyncDir = oldFsyncDir
	})

	err := RenameNoReplaceAndSync("src", filepath.Join("dir", "dst"))
	require.Error(t, err)
	require.True(t, IsCommitUncertain(err), "post-rename fsync failure must be classified as an uncertain commit")
	require.False(t, errors.Is(err, ErrRenameNoReplaceUnsupported))
}

func TestRenameNoReplaceFallbackFailsClosed(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "rename_noreplace_fallback.go", nil, 0)
	require.NoError(t, err)

	var forbidden []string
	usesUnsupportedError := false
	ast.Inspect(file, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.SelectorExpr:
			if ident, ok := n.X.(*ast.Ident); ok && ident.Name == "os" && (n.Sel.Name == "Lstat" || n.Sel.Name == "Rename") {
				forbidden = append(forbidden, "os."+n.Sel.Name)
			}
		case *ast.Ident:
			if n.Name == "ErrRenameNoReplaceUnsupported" {
				usesUnsupportedError = true
			}
		}
		return true
	})

	require.Empty(t, forbidden, "fallback must not use raceable precheck+rename calls")
	require.True(t, usesUnsupportedError, "fallback must fail closed with ErrRenameNoReplaceUnsupported")
}
