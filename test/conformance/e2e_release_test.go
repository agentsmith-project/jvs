//go:build conformance

package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// E2E Scenario 3: Release Management Flow
// User Story: Release manager tags versions, verifies releases, creates release branches

// TestE2E_Release_VersionTagging tests version management with multiple tags
func TestE2E_Release_VersionTagging(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "releases")
	mainPath := filepath.Join(repoPath, "main")
	versionPath := filepath.Join(mainPath, "VERSION")

	// Initialize repository
	runJVS(t, dir, "init", "releases")

	// Step 1: Create alpha release with multiple tags
	t.Run("alpha_release", func(t *testing.T) {
		os.WriteFile(versionPath, []byte("1.0.0-alpha"), 0644)
		stdout, stderr, code := runJVSInRepo(t, repoPath, "checkpoint", "alpha",
			"--tag", "alpha", "--tag", "v1.0.0-alpha")
		if code != 0 {
			t.Fatalf("alpha snapshot failed: %s", stderr)
		}
		if !strings.Contains(stdout, "Created checkpoint") {
			t.Errorf("expected success message, got: %s", stdout)
		}
	})

	// Step 2: Create beta release
	t.Run("beta_release", func(t *testing.T) {
		os.WriteFile(versionPath, []byte("1.0.0-beta"), 0644)
		stdout, stderr, code := runJVSInRepo(t, repoPath, "checkpoint", "beta",
			"--tag", "beta", "--tag", "v1.0.0-beta")
		if code != 0 {
			t.Fatalf("beta snapshot failed: %s", stderr)
		}
		if !strings.Contains(stdout, "Created checkpoint") {
			t.Errorf("expected success message, got: %s", stdout)
		}
	})

	// Step 3: Create stable release with multiple tags
	t.Run("stable_release", func(t *testing.T) {
		os.WriteFile(versionPath, []byte("1.0.0"), 0644)
		stdout, stderr, code := runJVSInRepo(t, repoPath, "checkpoint", "stable",
			"--tag", "stable", "--tag", "v1.0.0", "--tag", "release")
		if code != 0 {
			t.Fatalf("stable snapshot failed: %s", stderr)
		}
		if !strings.Contains(stdout, "Created checkpoint") {
			t.Errorf("expected success message, got: %s", stdout)
		}
	})

	// Step 4: Filter history by tag
	t.Run("history_filter_by_tag", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "--json", "checkpoint", "list")
		if code != 0 {
			t.Fatalf("checkpoint list failed: %s", stderr)
		}
		if count := getCheckpointCountByTag(t, stdout, "release"); count != 1 {
			t.Errorf("expected one release checkpoint, got %d: %s", count, stdout)
		}
	})

	// Step 5: Restore by tag
	t.Run("restore_by_tag", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "restore", "v1.0.0-beta")
		if code != 0 {
			t.Fatalf("restore by tag failed: %s", stderr)
		}

		content := readFile(t, mainPath, "VERSION")
		if content != "1.0.0-beta" {
			t.Errorf("expected '1.0.0-beta', got '%s'", content)
		}

		if !strings.Contains(stdout, "Workspace current differs from latest") {
			t.Errorf("expected historical status guidance after restore to beta, got stdout=%s stderr=%s", stdout, stderr)
		}
		status := readWorkspaceStatus(t, repoPath)
		if status.AtLatest || status.Current == status.Latest {
			t.Fatalf("expected current to differ from latest, got current=%s latest=%s", status.Current, status.Latest)
		}
	})

	// Step 6: Fork hotfix branch from restored state
	t.Run("fork_hotfix_branch", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "fork", "hotfix-1.0.1")
		if code != 0 {
			t.Fatalf("fork failed: %s", stderr)
		}
		if !strings.Contains(stdout, "Created workspace") {
			t.Errorf("expected success message, got: %s", stdout)
		}

		// Verify hotfix worktree exists
		hotfixPath := filepath.Join(repoPath, "worktrees", "hotfix-1.0.1")
		if !fileExists(t, filepath.Join(hotfixPath, "VERSION")) {
			t.Error("hotfix worktree should have VERSION file")
		}

		// Verify hotfix has beta content
		content := readFile(t, hotfixPath, "VERSION")
		if content != "1.0.0-beta" {
			t.Errorf("expected '1.0.0-beta' in hotfix, got '%s'", content)
		}
	})

	// Step 7: Work in hotfix branch
	t.Run("hotfix_development", func(t *testing.T) {
		hotfixPath := filepath.Join(repoPath, "worktrees", "hotfix-1.0.1")
		versionPath := filepath.Join(hotfixPath, "VERSION")

		os.WriteFile(versionPath, []byte("1.0.1"), 0644)
		stdout, stderr, code := runJVSInWorktree(t, repoPath, "hotfix-1.0.1",
			"checkpoint", "hotfix", "--tag", "v1.0.1", "--tag", "hotfix")
		if code != 0 {
			t.Fatalf("hotfix snapshot failed: %s", stderr)
		}
		if !strings.Contains(stdout, "Created checkpoint") {
			t.Errorf("expected success message, got: %s", stdout)
		}
	})

	// Step 8: Verify worktrees are independent
	t.Run("verify_independence", func(t *testing.T) {
		// Main workspace should still be at the historical beta checkpoint.
		mainContent := readFile(t, mainPath, "VERSION")
		if mainContent != "1.0.0-beta" {
			t.Errorf("main should still be at beta, got '%s'", mainContent)
		}

		// Hotfix worktree should be at 1.0.1
		hotfixContent := readFile(t, filepath.Join(repoPath, "worktrees", "hotfix-1.0.1"), "VERSION")
		if hotfixContent != "1.0.1" {
			t.Errorf("hotfix should be at 1.0.1, got '%s'", hotfixContent)
		}
	})
}

// TestE2E_Release_TagFiltering tests various tag filtering scenarios
func TestE2E_Release_TagFiltering(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")

	// Create multiple releases with different tag patterns
	os.WriteFile(filepath.Join(mainPath, "ver.txt"), []byte("2.0.0"), 0644)
	runJVSInRepo(t, repoPath, "checkpoint", "v2.0", "--tag", "v2.0.0", "--tag", "v2", "--tag", "release")

	os.WriteFile(filepath.Join(mainPath, "ver.txt"), []byte("2.1.0"), 0644)
	runJVSInRepo(t, repoPath, "checkpoint", "v2.1", "--tag", "v2.1.0", "--tag", "v2", "--tag", "release")

	os.WriteFile(filepath.Join(mainPath, "ver.txt"), []byte("3.0.0"), 0644)
	runJVSInRepo(t, repoPath, "checkpoint", "v3.0", "--tag", "v3.0.0", "--tag", "v3", "--tag", "release")

	// Filter by v2 tag - should show 2 results
	t.Run("filter_v2", func(t *testing.T) {
		stdout, _, _ := runJVSInRepo(t, repoPath, "--json", "checkpoint", "list")
		count := getCheckpointCountByTag(t, stdout, "v2")
		if count != 2 {
			t.Errorf("expected 2 v2 releases, got %d", count)
		}
	})

	// Filter by release tag - should show all 3
	t.Run("filter_release", func(t *testing.T) {
		stdout, _, _ := runJVSInRepo(t, repoPath, "--json", "checkpoint", "list")
		count := getCheckpointCountByTag(t, stdout, "release")
		if count != 3 {
			t.Errorf("expected 3 releases, got %d", count)
		}
	})

	// Filter by specific version - should show 1
	t.Run("filter_specific", func(t *testing.T) {
		stdout, _, _ := runJVSInRepo(t, repoPath, "--json", "checkpoint", "list")
		count := getCheckpointCountByTag(t, stdout, "v2.1.0")
		if count != 1 {
			t.Errorf("expected 1 v2.1.0 release, got %d", count)
		}
	})
}

// TestE2E_Release_ForkFromTag tests forking a worktree from a tagged snapshot
func TestE2E_Release_ForkFromTag(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")

	// Create a release with tag
	os.WriteFile(filepath.Join(mainPath, "app.config"), []byte("version=5.0"), 0644)
	runJVSInRepo(t, repoPath, "checkpoint", "release 5.0", "--tag", "v5.0.0", "--tag", "production")

	// Continue development in main
	os.WriteFile(filepath.Join(mainPath, "app.config"), []byte("version=6.0-beta"), 0644)
	runJVSInRepo(t, repoPath, "checkpoint", "dev 6.0")

	// Fork from production tag
	t.Run("fork_from_tag", func(t *testing.T) {
		_, stderr, code := runJVSInRepo(t, repoPath, "fork", "production", "prod-maintenance")
		if code != 0 {
			t.Fatalf("fork from tag failed: %s", stderr)
		}

		// Verify forked worktree has production content
		content := readFile(t, filepath.Join(repoPath, "worktrees", "prod-maintenance"), "app.config")
		if !strings.Contains(content, "version=5.0") {
			t.Errorf("forked worktree should have production content, got: %s", content)
		}
	})

	// Fork from specific version tag
	t.Run("fork_from_version_tag", func(t *testing.T) {
		_, stderr, code := runJVSInRepo(t, repoPath, "fork", "v5.0.0", "v5-hotfix")
		if code != 0 {
			t.Fatalf("fork from version tag failed: %s", stderr)
		}

		// Verify forked worktree has correct content
		content := readFile(t, filepath.Join(repoPath, "worktrees", "v5-hotfix"), "app.config")
		if !strings.Contains(content, "version=5.0") {
			t.Errorf("forked worktree should have v5.0 content, got: %s", content)
		}
	})
}
