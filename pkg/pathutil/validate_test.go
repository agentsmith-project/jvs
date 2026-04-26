package pathutil_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agentsmith-project/jvs/pkg/errclass"
	"github.com/agentsmith-project/jvs/pkg/pathutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateName_Valid(t *testing.T) {
	valid := []string{"main", "feature-1", "v1.0", "my_branch", "A-Z.test"}
	for _, name := range valid {
		assert.NoError(t, pathutil.ValidateName(name), "should accept: %s", name)
	}
}

func TestValidateName_Empty(t *testing.T) {
	err := pathutil.ValidateName("")
	require.ErrorIs(t, err, errclass.ErrNameInvalid)
}

func TestValidateName_DotDot(t *testing.T) {
	err := pathutil.ValidateName("..")
	require.ErrorIs(t, err, errclass.ErrNameInvalid)
}

func TestValidateName_Separators(t *testing.T) {
	for _, name := range []string{"a/b", "a\\b"} {
		err := pathutil.ValidateName(name)
		require.ErrorIs(t, err, errclass.ErrNameInvalid, "should reject: %s", name)
	}
}

func TestValidateName_ControlChars(t *testing.T) {
	err := pathutil.ValidateName("hello\x00world")
	require.ErrorIs(t, err, errclass.ErrNameInvalid)
}

func TestValidatePathSafety_UnderRoot(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "worktrees", "test")
	require.NoError(t, os.MkdirAll(target, 0755))
	assert.NoError(t, pathutil.ValidatePathSafety(root, target))
}

func TestValidatePathSafety_Escape(t *testing.T) {
	root := t.TempDir()
	err := pathutil.ValidatePathSafety(root, "/tmp/evil")
	require.ErrorIs(t, err, errclass.ErrPathEscape)
}

func TestValidatePathSafety_SymlinkEscape(t *testing.T) {
	root := t.TempDir()
	link := filepath.Join(root, "escape")
	os.Symlink("/tmp", link)
	err := pathutil.ValidatePathSafety(root, link)
	require.ErrorIs(t, err, errclass.ErrPathEscape)
}

func TestValidatePathSafety_NonExistentTarget(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "worktrees", "new-wt")
	// parent exists, target does not
	require.NoError(t, os.MkdirAll(filepath.Join(root, "worktrees"), 0755))
	assert.NoError(t, pathutil.ValidatePathSafety(root, target))
}

func TestCleanRel(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "plain", path: "models/model.bin", want: filepath.Join("models", "model.bin")},
		{name: "cleans redundant separators", path: "models/./model.bin", want: filepath.Join("models", "model.bin")},
		{name: "allows dotdot in filename", path: "models/a..b", want: filepath.Join("models", "a..b")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := pathutil.CleanRel(tt.path)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCleanRelRejectsUnsafePaths(t *testing.T) {
	for _, p := range []string{"", ".", "/absolute/path", "../escape", "safe/../still-inside", "safe/../../escape", "safe/.."} {
		t.Run(p, func(t *testing.T) {
			_, err := pathutil.CleanRel(p)
			require.Error(t, err)
		})
	}
}

func TestValidateNoSymlinkParents(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "safe"), 0755))
	require.NoError(t, pathutil.ValidateNoSymlinkParents(root, filepath.Join("safe", "file.txt")))

	outside := filepath.Join(t.TempDir(), "outside")
	require.NoError(t, os.MkdirAll(outside, 0755))
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}
	err := pathutil.ValidateNoSymlinkParents(root, filepath.Join("link", "file.txt"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "symlink")

	require.NoError(t, pathutil.ValidateNoSymlinkParents(root, "link"), "final symlink leaf is data, not a traversed parent")
}

func TestValidateTag_Valid(t *testing.T) {
	valid := []string{"v1.0", "release", "bugfix-123", "my_tag", "A-Z.test"}
	for _, tag := range valid {
		assert.NoError(t, pathutil.ValidateTag(tag), "should accept: %s", tag)
	}
}

func TestValidateTag_Empty(t *testing.T) {
	err := pathutil.ValidateTag("")
	require.ErrorIs(t, err, errclass.ErrNameInvalid)
}

func TestValidateTag_Invalid(t *testing.T) {
	invalid := []string{"tag with space", "tag/slash", "tag\\slash", "tag!", "tag@"}
	for _, tag := range invalid {
		err := pathutil.ValidateTag(tag)
		require.ErrorIs(t, err, errclass.ErrNameInvalid, "should reject: %s", tag)
	}
}
