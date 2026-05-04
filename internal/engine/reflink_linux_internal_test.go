//go:build linux

package engine

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReflinkCloneToNewFICLONEFailureCleanupPreservesLateReplacement(t *testing.T) {
	tests := []struct {
		name            string
		replaceCreated  func(t *testing.T, dst string)
		assertPreserved func(t *testing.T, dst string)
	}{
		{
			name: "symlink replacement",
			replaceCreated: func(t *testing.T, dst string) {
				outside := filepath.Join(t.TempDir(), "outside.txt")
				require.NoError(t, os.WriteFile(outside, []byte("outside original"), 0644))
				require.NoError(t, os.Remove(dst))
				if err := os.Symlink(outside, dst); err != nil {
					t.Skipf("symlinks not supported: %v", err)
				}
			},
			assertPreserved: func(t *testing.T, dst string) {
				info, err := os.Lstat(dst)
				require.NoError(t, err)
				assert.NotZero(t, info.Mode()&os.ModeSymlink)
			},
		},
		{
			name: "file replacement",
			replaceCreated: func(t *testing.T, dst string) {
				require.NoError(t, os.Remove(dst))
				require.NoError(t, os.WriteFile(dst, []byte("user replacement"), 0644))
			},
			assertPreserved: func(t *testing.T, dst string) {
				content, err := os.ReadFile(dst)
				require.NoError(t, err)
				assert.Equal(t, "user replacement", string(content))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srcDir := t.TempDir()
			src := filepath.Join(srcDir, "payload.txt")
			require.NoError(t, os.WriteFile(src, []byte("payload"), 0644))
			dst := filepath.Join(t.TempDir(), "late-leaf")

			oldClone := reflinkFileClone
			oldHook := reflinkFileToNewAfterCreateHook
			reflinkFileClone = func(_, _ uintptr) syscall.Errno {
				return syscall.EXDEV
			}
			reflinkFileToNewAfterCreateHook = func(dst string) error {
				tt.replaceCreated(t, dst)
				return nil
			}
			t.Cleanup(func() {
				reflinkFileClone = oldClone
				reflinkFileToNewAfterCreateHook = oldHook
			})

			result, err := NewReflinkEngine().CloneToNew(src, dst)
			require.Error(t, err)
			assert.Nil(t, result)
			assert.Contains(t, err.Error(), "changed before cleanup")
			tt.assertPreserved(t, dst)
		})
	}
}
