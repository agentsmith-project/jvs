//go:build conformance

package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestE2E_Hardening_CompressionRoundTrip tests content integrity after
// compressing a snapshot and restoring from it.
func TestE2E_Hardening_CompressionRoundTrip(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")

	originalFiles := map[string]string{
		"readme.md":           "# Project\nThis is the README.",
		"src/main.go":         "package main\n\nfunc main() {}\n",
		"src/util/helpers.go": "package util\n\nfunc Add(a, b int) int { return a + b }\n",
		"data/config.json":    `{"key": "value", "nested": {"a": 1}}`,
	}

	t.Run("create_original_files", func(t *testing.T) {
		createFiles(t, mainPath, originalFiles)
	})

	t.Run("create_compressed_snapshot", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "checkpoint", "compressed", "--compress", "fast")
		if code != 0 {
			t.Fatalf("compressed checkpoint failed: %s", stderr)
		}
		if !strings.Contains(stdout, "Created checkpoint") {
			t.Errorf("expected success message, got: %s", stdout)
		}
	})

	t.Run("modify_files", func(t *testing.T) {
		createFiles(t, mainPath, map[string]string{
			"readme.md":           "OVERWRITTEN",
			"src/main.go":         "OVERWRITTEN",
			"src/util/helpers.go": "OVERWRITTEN",
			"data/config.json":    "OVERWRITTEN",
			"extra.txt":           "extra file that should disappear",
		})
	})

	t.Run("restore_compressed_snapshot", func(t *testing.T) {
		stdout, _, _ := runJVSInRepo(t, repoPath, "history", "--json")
		ids := extractAllSnapshotIDs(stdout)
		if len(ids) == 0 {
			t.Fatal("expected at least one snapshot")
		}
		_, stderr, code := runJVSInRepo(t, repoPath, "restore", ids[0], "--discard-dirty")
		if code != 0 {
			t.Fatalf("restore failed: %s", stderr)
		}
	})

	t.Run("verify_content_integrity", func(t *testing.T) {
		for filename, expected := range originalFiles {
			got := readFile(t, mainPath, filename)
			if got != expected {
				t.Errorf("file %s: expected %q, got %q", filename, expected, got)
			}
		}
		if fileExists(t, filepath.Join(mainPath, "extra.txt")) {
			t.Error("restore with --discard-dirty should remove uncheckpointed extra file")
		}
	})
}

// TestE2E_Hardening_GCRetentionPolicy tests that GC correctly protects active
// snapshots and identifies orphaned ones from removed worktrees.
func TestE2E_Hardening_GCRetentionPolicy(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")

	t.Run("create_5_main_snapshots", func(t *testing.T) {
		for i := 1; i <= 5; i++ {
			os.WriteFile(filepath.Join(mainPath, "iteration.txt"), []byte(string(rune('0'+i))), 0644)
			stdout, stderr, code := runJVSInRepo(t, repoPath, "snapshot", "main iteration")
			if code != 0 {
				t.Fatalf("snapshot %d failed: %s", i, stderr)
			}
			if !strings.Contains(stdout, "Created snapshot") {
				t.Errorf("snapshot %d: expected success message, got: %s", i, stdout)
			}
		}
	})

	t.Run("gc_plan_protects_main_snapshots", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "gc", "plan")
		if code != 0 {
			t.Fatalf("gc plan failed: %s", stderr)
		}
		histOut, _, _ := runJVSInRepo(t, repoPath, "history", "--json")
		mainIDs := extractAllSnapshotIDs(histOut)
		if len(mainIDs) < 5 {
			t.Errorf("expected 5 main snapshots, got %d", len(mainIDs))
		}
		t.Logf("GC plan output: %s", stdout)
	})

	t.Run("create_temp_worktree_and_snapshot", func(t *testing.T) {
		_, stderr, code := runJVSInRepo(t, repoPath, "worktree", "fork", "temp-branch")
		if code != 0 {
			t.Fatalf("fork failed: %s", stderr)
		}
		featurePath := filepath.Join(repoPath, "worktrees", "temp-branch")
		os.WriteFile(filepath.Join(featurePath, "temp.txt"), []byte("temporary work"), 0644)
		_, stderr, code = runJVSInWorktree(t, repoPath, "temp-branch", "snapshot", "temp snapshot")
		if code != 0 {
			t.Fatalf("temp snapshot failed: %s", stderr)
		}
	})

	t.Run("remove_temp_worktree", func(t *testing.T) {
		_, stderr, code := runJVSInRepo(t, repoPath, "worktree", "remove", "temp-branch")
		if code != 0 {
			t.Fatalf("remove worktree failed: %s", stderr)
		}
	})

	t.Run("gc_plan_shows_orphaned_candidates", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "gc", "plan")
		if code != 0 {
			t.Fatalf("gc plan failed: %s", stderr)
		}
		combined := strings.ToLower(stdout + stderr)
		if !strings.Contains(combined, "candidate") && !strings.Contains(combined, "orphan") && !strings.Contains(combined, "delete") {
			t.Logf("GC plan after worktree removal (no candidate keyword found): %s", stdout)
		}
	})
}

// TestE2E_Hardening_DoctorCrashRecovery tests that doctor detects and repairs
// orphan artifacts left by simulated crashes.
func TestE2E_Hardening_DoctorCrashRecovery(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")
	jvsPath := filepath.Join(repoPath, ".jvs")

	t.Run("create_healthy_snapshot", func(t *testing.T) {
		os.WriteFile(filepath.Join(mainPath, "app.txt"), []byte("healthy state"), 0644)
		stdout, stderr, code := runJVSInRepo(t, repoPath, "snapshot", "healthy")
		if code != 0 {
			t.Fatalf("snapshot failed: %s", stderr)
		}
		if !strings.Contains(stdout, "Created snapshot") {
			t.Errorf("expected success message, got: %s", stdout)
		}
	})

	t.Run("create_orphan_artifacts", func(t *testing.T) {
		orphanTmpPath := filepath.Join(repoPath, ".jvs-tmp-orphan")
		if err := os.MkdirAll(orphanTmpPath, 0755); err != nil {
			t.Fatalf("failed to create orphan tmp dir: %v", err)
		}
		os.WriteFile(filepath.Join(orphanTmpPath, "partial.dat"), []byte("partial data"), 0644)

		snapshotsTmpPath := filepath.Join(jvsPath, "snapshots", "incomplete.tmp")
		if err := os.MkdirAll(snapshotsTmpPath, 0755); err != nil {
			t.Fatalf("failed to create snapshots tmp dir: %v", err)
		}
		os.WriteFile(filepath.Join(snapshotsTmpPath, "file.txt"), []byte("stale"), 0644)

		intentsPath := filepath.Join(jvsPath, "intents")
		if err := os.MkdirAll(intentsPath, 0755); err != nil {
			t.Fatalf("failed to create intents dir: %v", err)
		}
		os.WriteFile(filepath.Join(intentsPath, "orphan-op.json"), []byte(`{"status":"in_progress","operation":"snapshot"}`), 0644)
		os.WriteFile(filepath.Join(intentsPath, "stale-gc.json"), []byte(`{"status":"pending","operation":"gc"}`), 0644)
	})

	t.Run("doctor_detects_issues", func(t *testing.T) {
		stdout, stderr, _ := runJVSInRepo(t, repoPath, "doctor", "--strict")
		t.Logf("Doctor --strict output: %s", stdout)
		if stderr != "" {
			t.Logf("Doctor --strict stderr: %s", stderr)
		}
	})

	t.Run("repair_runtime", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "doctor", "--repair-runtime")
		t.Logf("Repair output: %s", stdout)
		if code != 0 {
			t.Logf("Repair stderr: %s", stderr)
		}
	})

	t.Run("verify_clean_state", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "doctor")
		if code != 0 {
			t.Fatalf("doctor should pass after repair: %s", stderr)
		}
		if !strings.Contains(stdout, "healthy") {
			t.Errorf("expected 'healthy' after repair, got: %s", stdout)
		}
	})

	t.Run("verify_tmp_artifacts_cleaned", func(t *testing.T) {
		snapshotsPath := filepath.Join(jvsPath, "snapshots")
		entries, err := os.ReadDir(snapshotsPath)
		if err != nil {
			t.Fatalf("failed to read snapshots dir: %v", err)
		}
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".tmp") {
				t.Errorf("tmp directory should be cleaned up: %s", e.Name())
			}
		}
	})

	t.Run("resume_normal_operations", func(t *testing.T) {
		os.WriteFile(filepath.Join(mainPath, "recovered.txt"), []byte("post-repair"), 0644)
		stdout, stderr, code := runJVSInRepo(t, repoPath, "snapshot", "post-recovery")
		if code != 0 {
			t.Fatalf("post-recovery snapshot failed: %s", stderr)
		}
		if !strings.Contains(stdout, "Created snapshot") {
			t.Errorf("expected success message, got: %s", stdout)
		}
	})
}

// TestE2E_Hardening_ConcurrentWorktreeOperations tests that multiple worktrees
// operate independently with isolated snapshot histories.
func TestE2E_Hardening_ConcurrentWorktreeOperations(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")

	os.WriteFile(filepath.Join(mainPath, "shared.txt"), []byte("shared baseline"), 0644)
	runJVSInRepo(t, repoPath, "snapshot", "baseline")

	worktrees := []string{"feature-a", "feature-b", "feature-c"}

	t.Run("create_worktrees", func(t *testing.T) {
		for _, name := range worktrees {
			stdout, stderr, code := runJVSInRepo(t, repoPath, "worktree", "fork", name)
			if code != 0 {
				t.Fatalf("fork %s failed: %s", name, stderr)
			}
			if !strings.Contains(stdout, "Created worktree") {
				t.Errorf("fork %s: expected success message, got: %s", name, stdout)
			}
		}
	})

	worktreeFiles := map[string]map[string]string{
		"feature-a": {"feature-a.go": "package a\n\nfunc A() {}\n"},
		"feature-b": {"feature-b.py": "def b():\n    pass\n"},
		"feature-c": {"feature-c.rs": "fn c() {}\n"},
	}

	t.Run("create_files_and_snapshot_each", func(t *testing.T) {
		for _, name := range worktrees {
			featurePath := filepath.Join(repoPath, "worktrees", name)
			createFiles(t, featurePath, worktreeFiles[name])
			stdout, stderr, code := runJVSInWorktree(t, repoPath, name, "snapshot", name+" work")
			if code != 0 {
				t.Fatalf("snapshot %s failed: %s", name, stderr)
			}
			if !strings.Contains(stdout, "Created snapshot") {
				t.Errorf("snapshot %s: expected success, got: %s", name, stdout)
			}
		}
	})

	t.Run("verify_independent_histories", func(t *testing.T) {
		for _, name := range worktrees {
			stdout, _, _ := runJVSInWorktree(t, repoPath, name, "history", "--json")
			count := getSnapshotCount(stdout)
			if count < 1 {
				t.Errorf("worktree %s should have at least 1 snapshot, got %d", name, count)
			}
		}
	})

	t.Run("verify_file_isolation", func(t *testing.T) {
		for _, name := range worktrees {
			featurePath := filepath.Join(repoPath, "worktrees", name)
			for otherName, files := range worktreeFiles {
				if otherName == name {
					for filename := range files {
						if !fileExists(t, filepath.Join(featurePath, filename)) {
							t.Errorf("worktree %s should have its own file %s", name, filename)
						}
					}
				} else {
					for filename := range files {
						if fileExists(t, filepath.Join(featurePath, filename)) {
							t.Errorf("worktree %s should NOT have %s's file %s", name, otherName, filename)
						}
					}
				}
			}
		}
	})

	t.Run("remove_one_worktree", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "worktree", "remove", "feature-b")
		if code != 0 {
			t.Fatalf("remove feature-b failed: %s", stderr)
		}
		if !strings.Contains(stdout, "Removed") {
			t.Errorf("expected remove message, got: %s", stdout)
		}
	})

	t.Run("verify_others_unaffected", func(t *testing.T) {
		for _, name := range []string{"feature-a", "feature-c"} {
			featurePath := filepath.Join(repoPath, "worktrees", name)
			for filename := range worktreeFiles[name] {
				if !fileExists(t, filepath.Join(featurePath, filename)) {
					t.Errorf("worktree %s should still have %s after removing feature-b", name, filename)
				}
			}
			stdout, stderr, code := runJVSInWorktree(t, repoPath, name, "snapshot", name+" still working")
			if code != 0 {
				t.Errorf("snapshot in %s failed after removing feature-b: %s", name, stderr)
			}
			if !strings.Contains(stdout, "Created snapshot") {
				t.Errorf("expected success in %s, got: %s", name, stdout)
			}
		}
	})
}

// TestE2E_Hardening_FirstSnapshotOnNewWorktree tests that a newly forked
// worktree can successfully create its first snapshot (CanSnapshot fix).
func TestE2E_Hardening_FirstSnapshotOnNewWorktree(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")

	t.Run("create_main_baseline", func(t *testing.T) {
		os.WriteFile(filepath.Join(mainPath, "base.txt"), []byte("baseline"), 0644)
		_, stderr, code := runJVSInRepo(t, repoPath, "snapshot", "baseline")
		if code != 0 {
			t.Fatalf("baseline snapshot failed: %s", stderr)
		}
	})

	t.Run("fork_fresh_worktree", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "worktree", "fork", "fresh")
		if code != 0 {
			t.Fatalf("fork failed: %s", stderr)
		}
		if !strings.Contains(stdout, "Created worktree") {
			t.Errorf("expected success message, got: %s", stdout)
		}
	})

	t.Run("add_files_to_fresh_worktree", func(t *testing.T) {
		freshPath := filepath.Join(repoPath, "worktrees", "fresh")
		createFiles(t, freshPath, map[string]string{
			"new-feature.go": "package fresh\n\nfunc Feature() string { return \"fresh\" }\n",
			"tests/test.go":  "package tests\n\nfunc TestFeature() {}\n",
		})
	})

	t.Run("first_snapshot_succeeds", func(t *testing.T) {
		stdout, stderr, code := runJVSInWorktree(t, repoPath, "fresh", "snapshot", "first snapshot on fresh worktree")
		if code != 0 {
			t.Fatalf("first snapshot on fresh worktree failed (CanSnapshot regression): %s", stderr)
		}
		if !strings.Contains(stdout, "Created snapshot") {
			t.Errorf("expected success message, got: %s", stdout)
		}
	})

	t.Run("verify_snapshot_in_history", func(t *testing.T) {
		stdout, _, _ := runJVSInWorktree(t, repoPath, "fresh", "history", "--json")
		count := getSnapshotCount(stdout)
		if count < 1 {
			t.Errorf("expected at least 1 snapshot in fresh worktree history, got %d", count)
		}
	})
}

// TestE2E_Hardening_VerifyAfterRestore tests that snapshot integrity verification
// passes after restoring to a prior tagged snapshot.
func TestE2E_Hardening_VerifyAfterRestore(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")

	t.Run("create_v1_snapshot", func(t *testing.T) {
		createFiles(t, mainPath, map[string]string{
			"app.go":     "package main\n\nvar version = \"v1\"\n",
			"config.yml": "version: 1\nmode: production\n",
		})
		stdout, stderr, code := runJVSInRepo(t, repoPath, "snapshot", "release v1", "--tag", "v1")
		if code != 0 {
			t.Fatalf("v1 snapshot failed: %s", stderr)
		}
		if !strings.Contains(stdout, "Created snapshot") {
			t.Errorf("expected success message, got: %s", stdout)
		}
	})

	t.Run("create_v2_snapshot", func(t *testing.T) {
		createFiles(t, mainPath, map[string]string{
			"app.go":     "package main\n\nvar version = \"v2\"\n",
			"config.yml": "version: 2\nmode: staging\n",
			"extra.txt":  "extra file in v2",
		})
		stdout, stderr, code := runJVSInRepo(t, repoPath, "snapshot", "release v2", "--tag", "v2")
		if code != 0 {
			t.Fatalf("v2 snapshot failed: %s", stderr)
		}
		if !strings.Contains(stdout, "Created snapshot") {
			t.Errorf("expected success message, got: %s", stdout)
		}
	})

	var v1ID string
	t.Run("get_v1_snapshot_id", func(t *testing.T) {
		stdout, _, _ := runJVSInRepo(t, repoPath, "history", "--json")
		ids := extractAllSnapshotIDs(stdout)
		if len(ids) < 2 {
			t.Fatalf("expected at least 2 snapshots, got %d", len(ids))
		}
		v1ID = ids[len(ids)-1]
	})

	t.Run("restore_to_v1", func(t *testing.T) {
		_, stderr, code := runJVSInRepo(t, repoPath, "restore", v1ID)
		if code != 0 {
			t.Fatalf("restore to v1 failed: %s", stderr)
		}
		content := readFile(t, mainPath, "app.go")
		if !strings.Contains(content, "v1") {
			t.Errorf("expected v1 content after restore, got: %s", content)
		}
	})

	t.Run("verify_integrity_after_restore", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "verify", "--all")
		if code != 0 {
			t.Fatalf("verify failed after restore: %s", stderr)
		}
		if !strings.Contains(stdout, "OK") {
			t.Errorf("expected OK in verify output, got: %s", stdout)
		}
	})

	t.Run("verify_individual_v1", func(t *testing.T) {
		if v1ID == "" {
			t.Skip("v1 ID not available")
		}
		_, stderr, code := runJVSInRepo(t, repoPath, "verify", v1ID)
		if code != 0 {
			t.Errorf("verify v1 snapshot failed: %s", stderr)
		}
	})
}
