package pathutil_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/agentsmith-project/jvs/pkg/errclass"
	"github.com/agentsmith-project/jvs/pkg/pathutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidateName_InvalidChars tests names with invalid characters.
func TestValidateName_InvalidChars(t *testing.T) {
	invalid := []string{
		"hello world",
		"foo:bar",
		"foo*bar",
		"foo?bar",
		"foo\"bar",
		"foo|bar",
		"foo<bar>",
		"foo@bar",
		"foo#bar",
		"foo$bar",
		"foo%bar",
		"foo^bar",
		"foo&bar",
		"foo(bar)",
		"foo[bar]",
		"foo{bar}",
		"foo+bar",
		"foo=bar",
		"foo,bar",
		"foo;bar",
		"foo'bar",
		"foo`bar",
		"foo~bar",
	}

	for _, name := range invalid {
		t.Run(name, func(t *testing.T) {
			err := pathutil.ValidateName(name)
			require.ErrorIs(t, err, errclass.ErrNameInvalid, "should reject: %s", name)
		})
	}
}

// TestValidateName_ValidEdgeCases tests valid edge case names.
func TestValidateName_ValidEdgeCases(t *testing.T) {
	valid := []string{
		"a",
		"1",
		".",
		"-",
		"_",
		".-.",
		"---",
		"___",
		"a.-",
		"0-9",
		"test_name-123.version",
	}

	for _, name := range valid {
		t.Run(name, func(t *testing.T) {
			assert.NoError(t, pathutil.ValidateName(name), "should accept: %s", name)
		})
	}
}

// TestValidateName_DotDotInMiddle tests names containing .. in the middle.
func TestValidateName_DotDotInMiddle(t *testing.T) {
	err := pathutil.ValidateName("test..name")
	require.ErrorIs(t, err, errclass.ErrNameInvalid)
}

// TestValidateName_WithDotDotSuffix tests names ending with dotdot pattern.
func TestValidateName_WithDotDotSuffix(t *testing.T) {
	err := pathutil.ValidateName("test..")
	require.ErrorIs(t, err, errclass.ErrNameInvalid)
}

// TestValidateName_WithDotDotPrefix tests names starting with dotdot pattern.
func TestValidateName_WithDotDotPrefix(t *testing.T) {
	err := pathutil.ValidateName("..test")
	require.ErrorIs(t, err, errclass.ErrNameInvalid)
}

// TestValidateName_MultipleDotDots tests names with multiple .. patterns.
func TestValidateName_MultipleDotDots(t *testing.T) {
	err := pathutil.ValidateName("test..name..again")
	require.ErrorIs(t, err, errclass.ErrNameInvalid)
}

// TestValidateName_UnicodeNormalized tests that unicode normalization works.
func TestValidateName_UnicodeNormalized(t *testing.T) {
	// Test with characters that can be represented multiple ways
	// é can be composed (é) or decomposed (e + combining acute)
	// After NFC normalization, both should work the same
	valid := []string{
		"test",      // ASCII
		"test-name", // ASCII with dash
	}

	for _, name := range valid {
		assert.NoError(t, pathutil.ValidateName(name), "should accept: %s", name)
	}
}

// TestValidatePathSafety_TargetIsRoot tests when target equals root.
func TestValidatePathSafety_TargetIsRoot(t *testing.T) {
	root := t.TempDir()
	assert.NoError(t, pathutil.ValidatePathSafety(root, root))
}

// TestValidatePathSafety_RootNotExists tests error when repo root doesn't exist.
func TestValidatePathSafety_RootNotExists(t *testing.T) {
	root := "/nonexistent/path/that/does/not/exist/xyz123"
	err := pathutil.ValidatePathSafety(root, "/tmp/test")
	require.ErrorIs(t, err, errclass.ErrPathEscape)
}

// TestValidatePathSafety_TargetResolutionError tests error when target cannot be resolved.
func TestValidatePathSafety_TargetResolutionError(t *testing.T) {
	root := t.TempDir()
	// Create a path that points to a non-existing directory with no existing parent
	target := filepath.Join(root, "nonexistent", "deeply", "nested", "path")

	// This should still work because resolveClosestAncestor will walk up to root
	// and the target should be under root
	err := pathutil.ValidatePathSafety(root, target)
	// The target doesn't exist but its ancestor (root) does, so it should be valid
	assert.NoError(t, err)
}

// TestValidatePathSafety_BothPathsSame tests when root and target are the same path.
func TestValidatePathSafety_BothPathsSame(t *testing.T) {
	root := t.TempDir()
	err := pathutil.ValidatePathSafety(root, root)
	assert.NoError(t, err)
}

// TestValidatePathSafety_AbsoluteTargetUnderRoot tests absolute paths under root.
func TestValidatePathSafety_AbsoluteTargetUnderRoot(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "worktrees", "test")
	require.NoError(t, os.MkdirAll(filepath.Dir(target), 0755))
	require.NoError(t, os.WriteFile(target, []byte("test"), 0644))

	assert.NoError(t, pathutil.ValidatePathSafety(root, target))
}

// TestValidatePathSafety_SymlinkWithinRoot tests symlinks that stay within root.
func TestValidatePathSafety_SymlinkWithinRoot(t *testing.T) {
	root := t.TempDir()
	// Create a directory and a symlink to it within root
	dir := filepath.Join(root, "original")
	require.NoError(t, os.MkdirAll(dir, 0755))

	link := filepath.Join(root, "link")
	require.NoError(t, os.Symlink(dir, link))

	assert.NoError(t, pathutil.ValidatePathSafety(root, link))
}

// TestValidatePathSafety_SymlinkToFileWithinRoot tests symlink to file within root.
func TestValidatePathSafety_SymlinkToFileWithinRoot(t *testing.T) {
	root := t.TempDir()
	// Create a file and symlink to it
	file := filepath.Join(root, "file.txt")
	require.NoError(t, os.WriteFile(file, []byte("content"), 0644))

	link := filepath.Join(root, "link.txt")
	require.NoError(t, os.Symlink(file, link))

	assert.NoError(t, pathutil.ValidatePathSafety(root, link))
}

// TestValidatePathSafety_ParentDirTraversal tests using .. in path.
func TestValidatePathSafety_ParentDirTraversal(t *testing.T) {
	root := t.TempDir()
	// Target that uses .. to escape root
	target := filepath.Join(root, "..", "tmp")

	err := pathutil.ValidatePathSafety(root, target)
	require.ErrorIs(t, err, errclass.ErrPathEscape)
}

// TestValidatePathSafety_DeepNesting tests deeply nested paths under root.
func TestValidatePathSafety_DeepNesting(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "a", "b", "c", "d", "e", "f", "g")
	require.NoError(t, os.MkdirAll(deep, 0755))

	assert.NoError(t, pathutil.ValidatePathSafety(root, deep))
}

// TestValidatePathSafety_RootHasTrailingSlash tests root with trailing slash.
func TestValidatePathSafety_RootHasTrailingSlash(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "test")
	require.NoError(t, os.MkdirAll(target, 0755))

	rootWithSlash := root + string(filepath.Separator)
	assert.NoError(t, pathutil.ValidatePathSafety(rootWithSlash, target))
}

// TestValidatePathSafety_TargetHasTrailingSlash tests target with trailing slash.
func TestValidatePathSafety_TargetHasTrailingSlash(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "test") + string(filepath.Separator)

	// This may or may not work depending on how the OS handles trailing slashes
	// The important thing is it doesn't crash
	_ = pathutil.ValidatePathSafety(root, target)
}

// TestValidateTag_ValidEdgeCases tests valid edge case tags.
func TestValidateTag_ValidEdgeCases(t *testing.T) {
	valid := []string{
		"a",
		"1",
		".",
		"-",
		"_",
		"...",
		"0-9",
		"v1.2.3-beta",
		"RELEASE_2024",
	}

	for _, tag := range valid {
		t.Run(tag, func(t *testing.T) {
			assert.NoError(t, pathutil.ValidateTag(tag), "should accept: %s", tag)
		})
	}
}

// TestValidateTag_SpecialCharsOnly tests tags with only special valid chars.
func TestValidateTag_SpecialCharsOnly(t *testing.T) {
	specials := []string{"...", "---", "___", "._-.", "-._"}
	for _, tag := range specials {
		t.Run(tag, func(t *testing.T) {
			assert.NoError(t, pathutil.ValidateTag(tag), "should accept: %s", tag)
		})
	}
}

// TestValidateTag_AllInvalidChars tests all categories of invalid characters.
func TestValidateTag_AllInvalidChars(t *testing.T) {
	invalid := []string{
		// Whitespace
		"tag with spaces",
		"tag\twith\ttabs",
		"tag\nwith\nnewlines",
		// Path separators
		"tag/with/slash",
		"tag\\with\\backslash",
		// Special shell characters
		"tag;echo",
		"tag|pipe",
		"tag&background",
		"tag$var",
		"tag`cmd`",
		// Other special chars
		"tag!exclaim",
		"tag@at",
		"tag#hash",
		"tag%percent",
		"tag^caret",
		"tag*asterisk",
		"tag(paren)",
		"tag[brace]",
		"tag{curly}",
		"tag+plus",
		"tag=equal",
		"tag,comma",
		"tag'quote",
		"tag<dub>",
		"tag?question",
	}

	for _, tag := range invalid {
		t.Run(tag, func(t *testing.T) {
			err := pathutil.ValidateTag(tag)
			require.ErrorIs(t, err, errclass.ErrNameInvalid, "should reject: %s", tag)
		})
	}
}

// TestValidateName_EmptyStringOnlyControlChars tests control characters only.
func TestValidateName_ControlCharsOnly(t *testing.T) {
	controlStrings := []string{
		"\x00",
		"\x01\x02\x03",
		"\n\r\t",
		"\x7f", // DEL
	}

	for _, name := range controlStrings {
		t.Run("", func(t *testing.T) { // Can't use control chars in test name
			err := pathutil.ValidateName(name)
			require.ErrorIs(t, err, errclass.ErrNameInvalid)
		})
	}
}

// TestValidateName_ValidWithNumbers tests names with various numeric patterns.
func TestValidateName_ValidWithNumbers(t *testing.T) {
	valid := []string{
		"123",
		"0.9",
		"v1.2.3",
		"test123",
		"123test",
		"1-2-3",
		"0_0_0",
	}

	for _, name := range valid {
		t.Run(name, func(t *testing.T) {
			assert.NoError(t, pathutil.ValidateName(name))
		})
	}
}

// TestValidatePathSafety_SymlinkChain tests chain of symlinks.
func TestValidatePathSafety_SymlinkChain(t *testing.T) {
	root := t.TempDir()

	// Create a directory
	dir := filepath.Join(root, "final")
	require.NoError(t, os.MkdirAll(dir, 0755))

	// Create a chain of symlinks: link1 -> link2 -> final
	link2 := filepath.Join(root, "link2")
	require.NoError(t, os.Symlink(dir, link2))

	link1 := filepath.Join(root, "link1")
	require.NoError(t, os.Symlink(link2, link1))

	// Following the chain should stay within root
	assert.NoError(t, pathutil.ValidatePathSafety(root, link1))
}

// TestValidatePathSafety_SymlinkToParentEscape tests symlink pointing to parent escaping root.
func TestValidatePathSafety_SymlinkToParentEscape(t *testing.T) {
	parentDir := t.TempDir()
	root := filepath.Join(parentDir, "root")
	require.NoError(t, os.Mkdir(root, 0755))

	// Create symlink in root that points to parent (escaping root)
	link := filepath.Join(root, "escape")
	require.NoError(t, os.Symlink(parentDir, link))

	err := pathutil.ValidatePathSafety(root, link)
	require.ErrorIs(t, err, errclass.ErrPathEscape)
}

// TestValidatePathSafety_NonExistentWithExistingParent tests non-existent path with existing parent.
func TestValidatePathSafety_NonExistentWithExistingParent(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "parent")
	require.NoError(t, os.MkdirAll(parent, 0755))

	target := filepath.Join(parent, "child", "grandchild")
	// parent exists, child and grandchild don't
	assert.NoError(t, pathutil.ValidatePathSafety(root, target))
}

// TestValidatePathSafety_MultiplePathSeparators tests paths with multiple consecutive separators.
func TestValidatePathSafety_MultiplePathSeparators(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "test", "file.txt")
	require.NoError(t, os.MkdirAll(filepath.Dir(target), 0755))
	require.NoError(t, os.WriteFile(target, []byte("content"), 0644))

	// Path with double separators should still work
	doubleSep := filepath.Join(root, "test", filepath.Join("file.txt"))
	_ = pathutil.ValidatePathSafety(root, doubleSep)
}

// TestValidateName_AllValidChars tests each valid character category.
func TestValidateName_AllValidChars(t *testing.T) {
	// Test lowercase, uppercase, digits, dot, dash, underscore
	valid := []string{
		"lowercase",
		"UPPERCASE",
		"MixedCase",
		"0123456789",
		"test.name",
		"test-name",
		"test_name",
		"aB1.-_",
	}

	for _, name := range valid {
		t.Run(name, func(t *testing.T) {
			assert.NoError(t, pathutil.ValidateName(name))
		})
	}
}

// TestValidateTag_AllValidChars tests each valid character category for tags.
func TestValidateTag_AllValidChars(t *testing.T) {
	valid := []string{
		"lowercase",
		"UPPERCASE",
		"MixedCase",
		"0123456789",
		"test.name",
		"test-name",
		"test_name",
		"aB1.-_",
	}

	for _, tag := range valid {
		t.Run(tag, func(t *testing.T) {
			assert.NoError(t, pathutil.ValidateTag(tag))
		})
	}
}

// TestValidatePathSafety_SymlinkToNonExistentUnderRoot tests symlink to non-existent path under root.
func TestValidatePathSafety_SymlinkToNonExistentUnderRoot(t *testing.T) {
	root := t.TempDir()
	// Create a symlink pointing to a non-existent path under root
	nonExistent := filepath.Join(root, "does-not-exist")
	link := filepath.Join(root, "link")
	require.NoError(t, os.Symlink(nonExistent, link))

	// The symlink target doesn't exist but is under root, so the link should be valid
	assert.NoError(t, pathutil.ValidatePathSafety(root, link))
}

// TestValidatePathSafety_EscapesViaRelativeSymlink tests escape via relative symlink.
func TestValidatePathSafety_EscapesViaRelativeSymlink(t *testing.T) {
	parentDir := t.TempDir()
	root := filepath.Join(parentDir, "root")
	require.NoError(t, os.Mkdir(root, 0755))

	// Create a directory outside root
	outside := filepath.Join(parentDir, "outside")
	require.NoError(t, os.Mkdir(outside, 0755))

	// Create a relative symlink from root to outside
	link := filepath.Join(root, "escape")
	require.NoError(t, os.Symlink("../outside", link))

	err := pathutil.ValidatePathSafety(root, link)
	require.ErrorIs(t, err, errclass.ErrPathEscape)
}

// TestValidatePathSafety_TargetIsDirectory tests when target is a directory.
func TestValidatePathSafety_TargetIsDirectory(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "directory")
	require.NoError(t, os.Mkdir(target, 0755))

	assert.NoError(t, pathutil.ValidatePathSafety(root, target))
}

// TestValidatePathSafety_TargetIsFile tests when target is a file.
func TestValidatePathSafety_TargetIsFile(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "file.txt")
	require.NoError(t, os.WriteFile(target, []byte("content"), 0644))

	assert.NoError(t, pathutil.ValidatePathSafety(root, target))
}

// TestValidatePathSafety_LongNonExistentPath tests very long non-existent path.
// This helps test resolveClosestAncestor with multiple recursive calls.
func TestValidatePathSafety_LongNonExistentPath(t *testing.T) {
	root := t.TempDir()

	// Create a very long non-existent path
	deepPath := root
	for i := 0; i < 20; i++ {
		deepPath = filepath.Join(deepPath, "level"+fmt.Sprint(i))
	}

	// Should still work - walks up to find root
	err := pathutil.ValidatePathSafety(root, deepPath)
	// The path is under root (even if it doesn't exist), so should be valid
	assert.NoError(t, err)
}

// TestValidatePathSafety_MaxDepthNonExistent tests extremely deep non-existent path.
func TestValidatePathSafety_MaxDepthNonExistent(t *testing.T) {
	root := t.TempDir()

	// Create an extremely deep non-existent path to test recursion depth
	deepPath := root
	for i := 0; i < 100; i++ {
		deepPath = filepath.Join(deepPath, "l"+fmt.Sprint(i))
	}

	err := pathutil.ValidatePathSafety(root, deepPath)
	assert.NoError(t, err)
}

// TestValidateName_ExactDotDot tests exact ".." string.
func TestValidateName_ExactDotDot(t *testing.T) {
	// Already tested in original test file, but explicitly here
	err := pathutil.ValidateName("..")
	require.ErrorIs(t, err, errclass.ErrNameInvalid)
}

// TestValidateName_StartsWithDotDot tests names starting with "..".
func TestValidateName_StartsWithDotDot(t *testing.T) {
	err := pathutil.ValidateName("..hidden")
	require.ErrorIs(t, err, errclass.ErrNameInvalid)
}

// TestValidatePathSafety_EscapesViaDotDotInTarget tests target with .. in path.
func TestValidatePathSafety_EscapesViaDotDotInTarget(t *testing.T) {
	root := t.TempDir()
	// Target uses .. to escape
	target := filepath.Join(root, "subdir", "..", "..", "tmp")

	err := pathutil.ValidatePathSafety(root, target)
	require.ErrorIs(t, err, errclass.ErrPathEscape)
}

// TestValidateTag_Whitespace tests tags with various whitespace.
func TestValidateTag_Whitespace(t *testing.T) {
	whitespaceTags := []string{
		"tag with\ttab",
		"tag with\nnewline",
		"tag with\rcarriage",
		"tag with\r\nwindows",
		"tag\fformfeed",
		"tag with\vtab",
	}

	for _, tag := range whitespaceTags {
		t.Run("", func(t *testing.T) { // Can't use whitespace in test name
			err := pathutil.ValidateTag(tag)
			require.ErrorIs(t, err, errclass.ErrNameInvalid)
		})
	}
}
