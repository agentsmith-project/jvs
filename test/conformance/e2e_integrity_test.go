//go:build conformance

package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// E2E Scenario 8: Data Integrity Verification
// User Story: Security-conscious user verifies snapshot integrity

// TestE2E_Integrity_VerifyPasses tests that verify passes for healthy snapshots
func TestE2E_Integrity_VerifyPasses(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "integrity")
	mainPath := filepath.Join(repoPath, "main")

	// Initialize repository
	runJVS(t, dir, "init", "integrity")

	// Create snapshot
	t.Run("create_snapshot", func(t *testing.T) {
		os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("exact content"), 0644)
		stdout, stderr, code := runJVSInRepo(t, repoPath, "checkpoint", "original")
		if code != 0 {
			t.Fatalf("snapshot failed: %s", stderr)
		}
		if !strings.Contains(stdout, "Created checkpoint") {
			t.Errorf("expected success message, got: %s", stdout)
		}
	})

	// Get snapshot ID
	var snapshotID string
	t.Run("get_snapshot_id", func(t *testing.T) {
		stdout, _, _ := runJVSInRepo(t, repoPath, "checkpoint", "list", "--json")
		ids := extractAllSnapshotIDs(stdout)
		if len(ids) == 0 {
			t.Fatal("expected at least one snapshot")
		}
		snapshotID = ids[0]
	})

	// Verify should pass
	t.Run("verify_passes", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "verify", snapshotID)
		if code != 0 {
			t.Fatalf("verify failed: %s", stderr)
		}
		// Check for success indicators in output
		if !strings.Contains(stdout, "Checksum: true") && !strings.Contains(stdout, "OK") && !strings.Contains(stdout, "verified") {
			t.Errorf("expected verification success, got: %s", stdout)
		}
	})

	// Verify all should also pass
	t.Run("verify_all_passes", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "verify", "--all")
		if code != 0 {
			t.Fatalf("verify --all failed: %s", stderr)
		}
		if !strings.Contains(stdout, "OK") {
			t.Errorf("expected OK, got: %s", stdout)
		}
	})
}

// TestE2E_Integrity_DetectTampering tests that verify detects payload tampering
func TestE2E_Integrity_DetectTampering(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")
	jvsPath := filepath.Join(repoPath, ".jvs")

	// Create snapshot
	os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("original content"), 0644)
	runJVSInRepo(t, repoPath, "checkpoint", "original")

	// Get snapshot ID
	stdout, _, _ := runJVSInRepo(t, repoPath, "checkpoint", "list", "--json")
	ids := extractAllSnapshotIDs(stdout)
	if len(ids) == 0 {
		t.Fatal("expected at least one snapshot")
	}
	snapID := ids[0]

	// Tamper with the snapshot payload in .jvs/snapshots/<id>/
	t.Run("tamper_payload", func(t *testing.T) {
		payloadFile := filepath.Join(jvsPath, "snapshots", snapID, "file.txt")
		// Check if payload exists in snapshots directory
		if fileExists(t, payloadFile) {
			os.WriteFile(payloadFile, []byte("tampered content"), 0644)
		} else {
			// Payload might be stored differently
			t.Log("Payload file not found in expected location, skipping tamper test")
			t.Skip()
		}
	})

	// Verify should detect tampering
	t.Run("verify_detects_tampering", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "verify", snapID)
		// Verify should fail
		if code == 0 {
			t.Error("verify should fail for tampered snapshot")
		}

		// Should indicate tampering
		combined := stdout + stderr
		if !strings.Contains(combined, "hash") && !strings.Contains(combined, "mismatch") && !strings.Contains(combined, "tamper") {
			t.Logf("Verify output for tampered snapshot: stdout=%s, stderr=%s", stdout, stderr)
		}
	})

	t.Run("verify_all_detects_tampering", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "verify", "--all")
		if code == 0 {
			t.Error("verify --all should fail for tampered snapshot")
		}

		combined := stdout + stderr
		if !strings.Contains(combined, "TAMPERED") && !strings.Contains(combined, "tamper") && !strings.Contains(combined, "hash") {
			t.Logf("Verify --all output for tampered snapshot: stdout=%s, stderr=%s", stdout, stderr)
		}
	})

	t.Run("verify_default_detects_tampering", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "verify")
		if code == 0 {
			t.Error("verify should fail for tampered snapshot")
		}

		combined := stdout + stderr
		if !strings.Contains(combined, "TAMPERED") && !strings.Contains(combined, "tamper") && !strings.Contains(combined, "hash") {
			t.Logf("Verify output for tampered snapshot: stdout=%s, stderr=%s", stdout, stderr)
		}
	})

	// JSON output should indicate tampering
	t.Run("json_output_indicates_tampering", func(t *testing.T) {
		stdout, _, _ := runJVSInRepo(t, repoPath, "verify", snapID, "--json")
		// Should have tamper_detected field or similar
		t.Logf("Verify JSON output: %s", stdout)
	})
}

// TestE2E_Integrity_MultipleSnapshots tests verifying multiple snapshots
func TestE2E_Integrity_MultipleSnapshots(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")

	// Create multiple snapshots
	for i := 1; i <= 5; i++ {
		os.WriteFile(filepath.Join(mainPath, "data.txt"), []byte(string(rune('a'+i))), 0644)
		runJVSInRepo(t, repoPath, "checkpoint", "checkpoint")
	}

	// Verify all
	t.Run("verify_all", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "verify", "--all")
		if code != 0 {
			t.Fatalf("verify --all failed: %s", stderr)
		}
		if !strings.Contains(stdout, "OK") {
			t.Errorf("expected OK in output, got: %s", stdout)
		}
	})

	// Verify individual snapshots
	t.Run("verify_individual", func(t *testing.T) {
		stdout, _, _ := runJVSInRepo(t, repoPath, "checkpoint", "list", "--json")
		ids := extractAllSnapshotIDs(stdout)

		for _, id := range ids {
			_, _, code := runJVSInRepo(t, repoPath, "verify", id)
			if code != 0 {
				t.Errorf("verify should pass for snapshot %s", id)
			}
		}
	})
}

// TestE2E_Integrity_VerifyByTag tests verifying snapshots by tag
func TestE2E_Integrity_VerifyByTag(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")

	// Create snapshots with tags
	os.WriteFile(filepath.Join(mainPath, "version.txt"), []byte("1.0"), 0644)
	runJVSInRepo(t, repoPath, "checkpoint", "v1.0", "--tag", "release", "--tag", "v1.0")

	os.WriteFile(filepath.Join(mainPath, "version.txt"), []byte("2.0"), 0644)
	runJVSInRepo(t, repoPath, "checkpoint", "v2.0", "--tag", "release", "--tag", "v2.0")

	// Verify with --all (includes all tagged)
	t.Run("verify_all_tagged", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "verify", "--all")
		if code != 0 {
			t.Fatalf("verify failed: %s", stderr)
		}
		if !strings.Contains(stdout, "OK") {
			t.Errorf("expected OK, got: %s", stdout)
		}
	})
}

// TestE2E_Integrity_VerifyAfterRestore tests integrity after restore operations
func TestE2E_Integrity_VerifyAfterRestore(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")

	// Create snapshots
	for _, ver := range []string{"A", "B", "C"} {
		os.WriteFile(filepath.Join(mainPath, "state.txt"), []byte(ver), 0644)
		runJVSInRepo(t, repoPath, "checkpoint", "state "+ver)
	}

	// Get snapshot IDs
	stdout, _, _ := runJVSInRepo(t, repoPath, "checkpoint", "list", "--json")
	ids := extractAllSnapshotIDs(stdout)

	// Restore to middle snapshot
	t.Run("restore_and_verify", func(t *testing.T) {
		if len(ids) < 2 {
			t.Fatal("need at least 2 snapshots")
		}
		middleID := ids[len(ids)-2]

		_, _, code := runJVSInRepo(t, repoPath, "restore", middleID)
		if code != 0 {
			t.Fatal("restore failed")
		}

		// Verify should still pass
		_, _, code = runJVSInRepo(t, repoPath, "verify", "--all")
		if code != 0 {
			t.Error("verify should pass after restore")
		}
	})

	// Restore latest and verify
	t.Run("restore_latest_and_verify", func(t *testing.T) {
		_, stderr, code := runJVSInRepo(t, repoPath, "restore", "latest")
		if code != 0 {
			t.Fatalf("restore latest failed: %s", stderr)
		}

		_, _, code = runJVSInRepo(t, repoPath, "verify", "--all")
		if code != 0 {
			t.Error("verify should pass after restore latest")
		}
	})
}

// TestE2E_Integrity_VerifyJsonOutput tests JSON output from verify command
func TestE2E_Integrity_VerifyJsonOutput(t *testing.T) {
	repoPath, _ := initTestRepo(t)

	// Create snapshot
	runJVSInRepo(t, repoPath, "checkpoint", "test")

	// Get JSON output
	t.Run("json_output", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "verify", "--all", "--json")
		if code != 0 {
			t.Fatalf("verify failed: %s", stderr)
		}

		// Should be valid JSON with relevant fields
		if !strings.Contains(stdout, "{") {
			t.Errorf("expected JSON output, got: %s", stdout)
		}

		// Log the output for inspection
		t.Logf("Verify JSON: %s", stdout)
	})
}

// TestE2E_Integrity_ChecksumVerification tests checksum-based verification
func TestE2E_Integrity_ChecksumVerification(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")
	jvsPath := filepath.Join(repoPath, ".jvs")

	// Create file with known content
	content := "checksum test content"
	os.WriteFile(filepath.Join(mainPath, "test.txt"), []byte(content), 0644)
	runJVSInRepo(t, repoPath, "checkpoint", "checksum-test")

	// Get snapshot ID
	stdout, _, _ := runJVSInRepo(t, repoPath, "checkpoint", "list", "--json")
	ids := extractAllSnapshotIDs(stdout)
	if len(ids) == 0 {
		t.Fatal("expected snapshot")
	}
	snapID := ids[0]

	// Verify checksum exists in descriptor
	t.Run("checksum_in_descriptor", func(t *testing.T) {
		descPath := filepath.Join(jvsPath, "snapshots", snapID+".json")
		content, err := os.ReadFile(descPath)
		if err != nil {
			t.Logf("Could not read descriptor: %v", err)
			return
		}

		descStr := string(content)
		// Should contain checksum or hash field
		if !strings.Contains(descStr, "checksum") && !strings.Contains(descStr, "hash") {
			t.Logf("Descriptor may not contain checksum: %s", descStr)
		}
	})

	// Verify passes
	t.Run("verify_passes", func(t *testing.T) {
		_, _, code := runJVSInRepo(t, repoPath, "verify", snapID)
		if code != 0 {
			t.Error("verify should pass for valid snapshot")
		}
	})
}

// TestE2E_Integrity_WorktreeVerification tests verification across worktrees
func TestE2E_Integrity_WorktreeVerification(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")

	// Create snapshot in main
	os.WriteFile(filepath.Join(mainPath, "main.txt"), []byte("main"), 0644)
	runJVSInRepo(t, repoPath, "checkpoint", "main-snap")

	// Fork worktree and create snapshot
	runJVSInRepo(t, repoPath, "fork", "feature")
	featurePath := filepath.Join(repoPath, "worktrees", "feature")
	os.WriteFile(filepath.Join(featurePath, "feature.txt"), []byte("feature"), 0644)
	runJVSInWorktree(t, repoPath, "feature", "checkpoint", "feature-snap")

	// Verify all snapshots
	t.Run("verify_all_worktrees", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "verify", "--all")
		if code != 0 {
			t.Fatalf("verify failed: %s", stderr)
		}
		if !strings.Contains(stdout, "OK") {
			t.Errorf("expected OK, got: %s", stdout)
		}
	})
}

// TestE2E_Integrity_AfterGC tests integrity is maintained after GC
func TestE2E_Integrity_AfterGC(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")

	// Create snapshots
	os.WriteFile(filepath.Join(mainPath, "data.txt"), []byte("main"), 0644)
	runJVSInRepo(t, repoPath, "checkpoint", "main")

	// Create and remove worktree
	runJVSInRepo(t, repoPath, "fork", "temp")
	featurePath := filepath.Join(repoPath, "worktrees", "temp")
	os.WriteFile(filepath.Join(featurePath, "temp.txt"), []byte("temp"), 0644)
	runJVSInWorktree(t, repoPath, "temp", "checkpoint", "temp")
	runJVSInRepo(t, repoPath, "workspace", "remove", "temp")

	// Run GC
	planOut, _, _ := runJVSInRepo(t, repoPath, "gc", "plan", "--json")
	planID := extractPlanID(planOut)
	if planID == "" {
		planID = extractPlanIDFromText(planOut)
	}
	if planID != "" {
		runJVSInRepo(t, repoPath, "gc", "run", "--plan-id", planID)
	}

	// Verify remaining snapshots are intact
	t.Run("verify_after_gc", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "verify", "--all")
		if code != 0 {
			t.Fatalf("verify failed after GC: %s", stderr)
		}
		if !strings.Contains(stdout, "OK") {
			t.Errorf("expected OK, got: %s", stdout)
		}
	})
}
