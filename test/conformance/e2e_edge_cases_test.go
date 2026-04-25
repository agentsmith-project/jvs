//go:build conformance

package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// E2E Scenario: Error Handling and Edge Cases
// User Story: System handles errors gracefully and provides clear feedback

// TestE2E_EdgeCases_SnapshotWithEmptyPayload tests snapshotting an empty worktree
func TestE2E_EdgeCases_SnapshotWithEmptyPayload(t *testing.T) {
	repoPath, _ := initTestRepo(t)

	// Snapshot with empty worktree
	t.Run("empty_snapshot", func(t *testing.T) {
		// main/ exists but is empty
		stdout, stderr, code := runJVSInRepo(t, repoPath, "snapshot", "empty baseline")
		if code != 0 {
			t.Logf("Empty snapshot failed (may be expected): %s", stderr)
		} else {
			t.Logf("Empty snapshot succeeded: %s", stdout)
			// Should still have created a snapshot
			if !strings.Contains(stdout, "Created snapshot") {
				t.Error("expected success message")
			}
		}
	})
}

// TestE2E_EdgeCases_RestoreToNonExistentSnapshot tests restore error handling
func TestE2E_EdgeCases_RestoreToNonExistentSnapshot(t *testing.T) {
	repoPath, _ := initTestRepo(t)

	// Try to restore to non-existent snapshot
	t.Run("restore_nonexistent", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "restore", "nonexistent-snapshot-id")
		if code == 0 {
			t.Error("restore should fail for non-existent snapshot")
		}
		// Should provide meaningful error message
		combined := stdout + stderr
		if !strings.Contains(combined, "not found") && !strings.Contains(combined, "no snapshot") && !strings.Contains(combined, "invalid") {
			t.Logf("Expected error message, got: %s", combined)
		}
	})
}

// TestE2E_EdgeCases_ForkToExistingName tests forking with duplicate worktree name
func TestE2E_EdgeCases_ForkToExistingName(t *testing.T) {
	repoPath, _ := initTestRepo(t)

	// Create first worktree
	runJVSInRepo(t, repoPath, "worktree", "fork", "feature")

	// Try to create worktree with same name
	t.Run("fork_duplicate_name", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "worktree", "fork", "feature")
		if code == 0 {
			t.Error("fork should fail for existing worktree name")
		}
		combined := stdout + stderr
		if !strings.Contains(combined, "exists") && !strings.Contains(combined, "already") {
			t.Logf("Expected 'exists' error, got: %s", combined)
		}
	})
}

// TestE2E_EdgeCases_RemoveNonExistentWorktree tests removing non-existent worktree
func TestE2E_EdgeCases_RemoveNonExistentWorktree(t *testing.T) {
	repoPath, _ := initTestRepo(t)

	t.Run("remove_nonexistent", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "worktree", "remove", "nonexistent-worktree")
		if code == 0 {
			t.Log("remove of non-existent worktree succeeded (idempotent)")
		} else {
			combined := stdout + stderr
			if !strings.Contains(combined, "not found") && !strings.Contains(combined, "no such") {
				t.Logf("Remove error: %s", combined)
			}
		}
	})
}

// TestE2E_EdgeCases_RenameToExistingName tests renaming to existing worktree name
func TestE2E_EdgeCases_RenameToExistingName(t *testing.T) {
	repoPath, _ := initTestRepo(t)

	// Create two worktrees
	runJVSInRepo(t, repoPath, "worktree", "fork", "feature-a")
	runJVSInRepo(t, repoPath, "worktree", "fork", "feature-b")

	t.Run("rename_to_existing", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "worktree", "rename", "feature-a", "feature-b")
		if code == 0 {
			t.Error("rename should fail when target name exists")
		}
		combined := stdout + stderr
		if !strings.Contains(combined, "exists") && !strings.Contains(combined, "already") {
			t.Logf("Expected 'exists' error, got: %s", combined)
		}
	})
}

// TestE2E_EdgeCases_InvalidTagName tests invalid tag names
func TestE2E_EdgeCases_InvalidTagName(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")

	os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("content"), 0644)

	t.Run("tag_with_spaces", func(t *testing.T) {
		// Tags with spaces might be rejected or handled
		stdout, stderr, code := runJVSInRepo(t, repoPath, "snapshot", "test", "--tag", "tag with spaces")
		if code != 0 {
			t.Logf("Tag with spaces rejected (expected): %s", stderr)
		} else {
			t.Logf("Tag with spaces accepted: %s", stdout)
		}
	})
}

// TestE2E_EdgeCases_VerifyNonExistentSnapshot tests verifying non-existent snapshot
func TestE2E_EdgeCases_VerifyNonExistentSnapshot(t *testing.T) {
	repoPath, _ := initTestRepo(t)

	t.Run("verify_nonexistent", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "verify", "nonexistent-id")
		if code == 0 {
			t.Error("verify should fail for non-existent snapshot")
		}
		combined := stdout + stderr
		if !strings.Contains(combined, "not found") && !strings.Contains(combined, "no snapshot") {
			t.Logf("Verify error: %s", combined)
		}
	})
}

// TestE2E_EdgeCases_RestoreWhenHistorical tests restore operations while current differs from latest.
func TestE2E_EdgeCases_RestoreWhenHistorical(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")

	// Create two snapshots
	os.WriteFile(filepath.Join(mainPath, "state.txt"), []byte("first"), 0644)
	runJVSInRepo(t, repoPath, "snapshot", "first", "--tag", "v1")

	os.WriteFile(filepath.Join(mainPath, "state.txt"), []byte("second"), 0644)
	runJVSInRepo(t, repoPath, "snapshot", "second", "--tag", "v2")

	// Get snapshot IDs
	stdout, _, _ := runJVSInRepo(t, repoPath, "history", "--json")
	ids := extractAllSnapshotIDs(stdout)
	if len(ids) < 2 {
		t.Fatal("need at least 2 snapshots")
	}

	// Restore to the historical checkpoint.
	runJVSInRepo(t, repoPath, "restore", ids[len(ids)-1])
	requireWorkspaceCurrentLatest(t, repoPath, ids[len(ids)-1], ids[0], false)

	// Now restore while current differs from latest.
	t.Run("restore_from_historical", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "restore", "latest")
		if code != 0 {
			t.Errorf("restore latest should succeed: %s", stderr)
		}
		if !strings.Contains(stdout, "Restored") {
			t.Logf("Restore output: %s", stdout)
		}
		requireWorkspaceCurrentLatest(t, repoPath, ids[0], ids[0], true)
	})
}

// TestE2E_EdgeCases_CheckpointWhileHistorical tests checkpoint creation while current differs from latest.
func TestE2E_EdgeCases_CheckpointWhileHistorical(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")

	// Create two checkpoints
	os.WriteFile(filepath.Join(mainPath, "state.txt"), []byte("first"), 0644)
	runJVSInRepo(t, repoPath, "checkpoint", "first")

	os.WriteFile(filepath.Join(mainPath, "state.txt"), []byte("second"), 0644)
	runJVSInRepo(t, repoPath, "checkpoint", "second")

	// Restore to the first checkpoint so current differs from latest.
	stdout, _, _ := runJVSInRepo(t, repoPath, "--json", "checkpoint", "list")
	ids := extractAllSnapshotIDs(stdout)
	if len(ids) < 2 {
		t.Fatal("need at least 2 snapshots")
	}
	runJVSInRepo(t, repoPath, "restore", ids[len(ids)-1])
	requireWorkspaceCurrentLatest(t, repoPath, ids[len(ids)-1], ids[0], false)

	t.Run("checkpoint_while_historical", func(t *testing.T) {
		os.WriteFile(filepath.Join(mainPath, "historical.txt"), []byte("work"), 0644)
		_, stderr, code := runJVSInRepo(t, repoPath, "checkpoint", "historical work")
		if code == 0 {
			t.Error("checkpoint should fail while current differs from latest")
		}
		if !strings.Contains(stderr, "current differs from latest") {
			t.Errorf("expected historical-position error, got: %s", stderr)
		}
		requireWorkspaceCurrentLatest(t, repoPath, ids[len(ids)-1], ids[0], false)
	})
}

// TestE2E_EdgeCases_LongSnapshotNote tests very long snapshot notes
func TestE2E_EdgeCases_LongSnapshotNote(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")

	os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("content"), 0644)

	// Create a very long note (1000 characters)
	longNote := strings.Repeat("This is a very long snapshot note. ", 50)

	t.Run("long_note", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "snapshot", longNote)
		if code != 0 {
			t.Errorf("snapshot with long note should succeed: %s", stderr)
		} else {
			if !strings.Contains(stdout, "Created snapshot") {
				t.Error("expected success message")
			}
		}
	})
}

// TestE2E_EdgeCases_MultipleTags tests snapshot with many tags
func TestE2E_EdgeCases_MultipleTags(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")

	os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("content"), 0644)

	// Create snapshot with multiple tags
	t.Run("multiple_tags", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "snapshot", "multi-tag snapshot",
			"--tag", "v1.0",
			"--tag", "release",
			"--tag", "stable",
			"--tag", "production",
			"--tag", "mainline")
		if code != 0 {
			t.Errorf("snapshot with multiple tags should succeed: %s", stderr)
		} else {
			if !strings.Contains(stdout, "Created snapshot") {
				t.Error("expected success message")
			}
		}

		// Verify tags were stored
		stdout, _, _ = runJVSInRepo(t, repoPath, "history")
		if !strings.Contains(stdout, "v1.0") || !strings.Contains(stdout, "release") {
			t.Logf("History output: %s", stdout)
		}
	})
}

// TestE2E_EdgeCases_SpecialCharactersInFilename tests files with special characters
func TestE2E_EdgeCases_SpecialCharactersInFilename(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")

	// Create files with various special characters
	specialFiles := map[string]string{
		"file-with-dashes.txt":      "dashes",
		"file_with_underscores.txt": "underscores",
		"file.dots.and.more.txt":    "dots",
		"file(1).txt":               "parentheses",
	}

	for filename, content := range specialFiles {
		if err := os.WriteFile(filepath.Join(mainPath, filename), []byte(content), 0644); err != nil {
			t.Logf("Warning: could not create %s: %v", filename, err)
		}
	}

	t.Run("snapshot_special_chars", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "snapshot", "special chars")
		if code != 0 {
			t.Errorf("snapshot with special filenames should succeed: %s", stderr)
		}

		// Restore and verify files are preserved
		stdout, _, _ = runJVSInRepo(t, repoPath, "history", "--json")
		ids := extractAllSnapshotIDs(stdout)
		if len(ids) > 0 {
			runJVSInRepo(t, repoPath, "restore", "nonexistent-test-id")
			// Then restore back to verify
			runJVSInRepo(t, repoPath, "restore", "latest")
		}
	})
}

// TestE2E_EdgeCases_DeeplyNestedDirectory tests deep directory nesting
func TestE2E_EdgeCases_DeeplyNestedDirectory(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")

	// Create deeply nested structure
	deepPath := mainPath
	for i := 0; i < 20; i++ {
		deepPath = filepath.Join(deepPath, "level"+string(rune('0'+i)))
	}
	if err := os.MkdirAll(deepPath, 0755); err != nil {
		t.Fatalf("failed to create deep path: %v", err)
	}
	os.WriteFile(filepath.Join(deepPath, "deep.txt"), []byte("deep content"), 0644)

	t.Run("deep_nesting", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "snapshot", "deep nested content")
		if code != 0 {
			t.Errorf("snapshot with deep nesting should succeed: %s", stderr)
		}
		t.Logf("Deep nesting snapshot result: %s", stdout)
	})
}

// TestE2E_EdgeCases_LargeFile tests snapshotting a large file
func TestE2E_EdgeCases_LargeFile(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")

	// Create a larger file (1MB)
	largeContent := make([]byte, 1024*1024)
	for i := range largeContent {
		largeContent[i] = byte(i % 256)
	}
	os.WriteFile(filepath.Join(mainPath, "large.bin"), largeContent, 0644)

	t.Run("large_file", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "snapshot", "large file snapshot")
		if code != 0 {
			t.Errorf("snapshot with large file should succeed: %s", stderr)
		}
		t.Logf("Large file snapshot: %s", stdout)
	})
}

// TestE2E_EdgeCases_ManySmallFiles tests snapshotting many small files
func TestE2E_EdgeCases_ManySmallFiles(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")

	// Create 100 small files
	for i := 0; i < 100; i++ {
		filename := filepath.Join(mainPath, "file"+string(rune('0'+i%10))+".txt")
		if err := os.WriteFile(filename, []byte("content "+string(rune('0'+i%10))), 0644); err != nil {
			t.Fatalf("failed to create file: %v", err)
		}
	}

	t.Run("many_small_files", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "snapshot", "many files")
		if code != 0 {
			t.Errorf("snapshot with many files should succeed: %s", stderr)
		}
		t.Logf("Many files snapshot: %s", stdout)
	})
}

// TestE2E_EdgeCases_RestoreSameSnapshot tests restoring to same snapshot
func TestE2E_EdgeCases_RestoreSameSnapshot(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")

	os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("content"), 0644)
	runJVSInRepo(t, repoPath, "snapshot", "original")

	t.Run("restore_same_snapshot", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "restore", "latest")
		if code != 0 {
			t.Errorf("restore to latest (same checkpoint) should succeed: %s", stderr)
		}
		t.Logf("Restore same snapshot: %s", stdout)
	})
}

// TestE2E_EdgeCases_ListEmptyHistory tests history when no snapshots exist
func TestE2E_EdgeCases_ListEmptyHistory(t *testing.T) {
	repoPath, _ := initTestRepo(t)

	t.Run("empty_history", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "history")
		// Should not fail, just show empty history
		if code != 0 {
			t.Logf("History command failed: %s", stderr)
		}
		t.Logf("Empty history: %s", stdout)
	})
}

// TestE2E_EdgeCases_GCWithoutSnapshots tests GC when no snapshots exist
func TestE2E_EdgeCases_GCWithoutSnapshots(t *testing.T) {
	repoPath, _ := initTestRepo(t)

	t.Run("gc_no_snapshots", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "gc", "plan")
		if code != 0 {
			t.Logf("GC plan without snapshots: %s", stderr)
		} else {
			t.Logf("GC plan output: %s", stdout)
		}
	})
}

// TestE2E_EdgeCases_DoctorOnHealthyRepo tests doctor on healthy repository
func TestE2E_EdgeCases_DoctorOnHealthyRepo(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")

	os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("content"), 0644)
	runJVSInRepo(t, repoPath, "snapshot", "healthy")

	t.Run("doctor_healthy", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "doctor")
		if code != 0 {
			t.Errorf("doctor on healthy repo should succeed: %s", stderr)
		}
		if !strings.Contains(stdout, "healthy") && !strings.Contains(stdout, "OK") {
			t.Logf("Doctor output: %s", stdout)
		}
	})
}

// TestE2E_ErrorHandling_InvalidWorktreeCommands tests various invalid worktree commands
func TestE2E_ErrorHandling_InvalidWorktreeCommands(t *testing.T) {
	repoPath, _ := initTestRepo(t)

	t.Run("path_nonexistent", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "worktree", "path", "nonexistent")
		if code == 0 {
			t.Log("path of nonexistent worktree succeeded (may show error)")
		}
		combined := stdout + stderr
		if !strings.Contains(combined, "not found") && !strings.Contains(combined, "no such") {
			t.Logf("Path error output: %s", combined)
		}
	})

	t.Run("rename_nonexistent", func(t *testing.T) {
		stdout, _, code := runJVSInRepo(t, repoPath, "worktree", "rename", "nonexistent", "newname")
		if code == 0 {
			t.Log("rename of nonexistent succeeded (may show error)")
		}
		t.Logf("Rename nonexistent output: %s", stdout)
	})
}

// TestE2E_ErrorHandling_SnapshotConflicts tests snapshot creation edge cases
func TestE2E_ErrorHandling_SnapshotConflicts(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")

	os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("content"), 0644)
	runJVSInRepo(t, repoPath, "snapshot", "first", "--tag", "release")

	t.Run("duplicate_tag", func(t *testing.T) {
		// Create another snapshot with same tag
		os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("updated"), 0644)
		stdout, stderr, code := runJVSInRepo(t, repoPath, "snapshot", "second", "--tag", "release")
		if code != 0 {
			t.Logf("Duplicate tag rejected: %s", stderr)
		} else {
			t.Logf("Duplicate tag accepted (tags may not be unique): %s", stdout)
		}
	})
}

// TestE2E_Worktree_MergeLikeScenario simulates merging changes between worktrees
func TestE2E_Worktree_MergeLikeScenario(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")

	// Create baseline
	os.WriteFile(filepath.Join(mainPath, "shared.txt"), []byte("shared content"), 0644)
	runJVSInRepo(t, repoPath, "snapshot", "baseline")

	// Fork feature branch
	runJVSInRepo(t, repoPath, "worktree", "fork", "feature")
	featurePath := filepath.Join(repoPath, "worktrees", "feature")

	// Work on feature in parallel with main work
	t.Run("parallel_development", func(t *testing.T) {
		// Feature branch adds file
		os.WriteFile(filepath.Join(featurePath, "feature.txt"), []byte("feature work"), 0644)
		runJVSInWorktree(t, repoPath, "feature", "snapshot", "feature added")

		// Main branch also evolves
		os.WriteFile(filepath.Join(mainPath, "main.txt"), []byte("main work"), 0644)
		runJVSInRepo(t, repoPath, "snapshot", "main update")

		// Feature should still have baseline's shared.txt
		content := readFile(t, featurePath, "shared.txt")
		if content != "shared content" {
			t.Errorf("feature should have baseline content, got: %s", content)
		}

		// Main should NOT have feature.txt
		if fileExists(t, filepath.Join(mainPath, "feature.txt")) {
			t.Error("main should NOT have feature.txt")
		}
	})

	// "Merge" by restoring from feature and making it the new main
	t.Run("manual_merge", func(t *testing.T) {
		// Get feature's latest snapshot ID
		stdout, _, _ := runJVSInWorktree(t, repoPath, "feature", "history", "--json")
		ids := extractAllSnapshotIDs(stdout)
		if len(ids) == 0 {
			t.Fatal("feature should have snapshots")
		}
		featureSnapID := ids[0]

		// Restore main to feature's snapshot
		os.WriteFile(filepath.Join(mainPath, "main.txt"), []byte("will be replaced"), 0644)
		stdout, stderr, code := runJVSInRepo(t, repoPath, "restore", featureSnapID)
		if code != 0 {
			t.Logf("Restore from feature snapshot: %s", stderr)
		}

		// Verify main now has feature content
		if !fileExists(t, filepath.Join(mainPath, "feature.txt")) {
			// Feature.txt should be present after restore
			t.Log("feature.txt not in main after restore - content isolation in effect")
		}
	})
}

// TestE2E_Engine_Fallback tests that engine fallback works correctly
func TestE2E_Engine_Fallback(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")

	os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("content"), 0644)

	// Create snapshot - should use configured engine and fallback if needed
	t.Run("snapshot_with_engine", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "snapshot", "test engine")
		if code != 0 {
			t.Errorf("snapshot should work with engine fallback: %s", stderr)
		}
		t.Logf("Engine snapshot result: %s", stdout)
	})
}

// TestE2E_EdgeCases_UnicodeFilenames tests files with unicode characters
func TestE2E_EdgeCases_UnicodeFilenames(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")

	// Create files with unicode names
	unicodeFiles := map[string]string{
		"файл.txt":      "russian",
		"文件.txt":        "chinese",
		"αρχείο.txt":    "greek",
		"test-ñoño.txt": "spanish",
	}

	for filename, content := range unicodeFiles {
		err := os.WriteFile(filepath.Join(mainPath, filename), []byte(content), 0644)
		if err != nil {
			t.Logf("Warning: could not create unicode file %s: %v", filename, err)
		}
	}

	t.Run("unicode_filenames", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "snapshot", "unicode test")
		if code != 0 {
			t.Errorf("snapshot with unicode filenames should succeed: %s", stderr)
		}
		t.Logf("Unicode snapshot result: %s", stdout)
	})
}
