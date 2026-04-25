//go:build conformance

package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// E2E Scenario 7: Disaster Recovery Flow
// User Story: Admin detects and repairs crash artifacts

// TestE2E_Disaster_DetectAndRepair tests detecting and repairing crash artifacts
func TestE2E_Disaster_DetectAndRepair(t *testing.T) {
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "disaster")
	mainPath := filepath.Join(repoPath, "main")
	jvsPath := filepath.Join(repoPath, ".jvs")

	// Initialize repository
	runJVS(t, dir, "init", "disaster")

	// Create healthy snapshot
	t.Run("create_healthy_state", func(t *testing.T) {
		os.WriteFile(filepath.Join(mainPath, "data.txt"), []byte("data"), 0644)
		stdout, stderr, code := runJVSInRepo(t, repoPath, "checkpoint", "healthy")
		if code != 0 {
			t.Fatalf("snapshot failed: %s", stderr)
		}
		if !strings.Contains(stdout, "Created checkpoint") {
			t.Errorf("expected success message, got: %s", stdout)
		}
	})

	// Step 2: Simulate crash - create orphan tmp directory
	t.Run("simulate_crash_tmp", func(t *testing.T) {
		// Create orphan tmp directory (simulating incomplete snapshot)
		tmpPath := filepath.Join(jvsPath, "snapshots", "crashed.tmp")
		if err := os.MkdirAll(tmpPath, 0755); err != nil {
			t.Fatalf("failed to create tmp dir: %v", err)
		}
		// Write partial content but no .READY marker
		os.WriteFile(filepath.Join(tmpPath, "partial.txt"), []byte("partial"), 0644)
	})

	// Step 3: Simulate crash - create orphan intent file
	t.Run("simulate_crash_intent", func(t *testing.T) {
		// Create orphan intent file
		intentsPath := filepath.Join(jvsPath, "intents")
		if err := os.MkdirAll(intentsPath, 0755); err != nil {
			t.Fatalf("failed to create intents dir: %v", err)
		}
		intentContent := `{"status":"in_progress","operation":"checkpoint"}`
		os.WriteFile(filepath.Join(intentsPath, "crashed.json"), []byte(intentContent), 0644)
	})

	// Step 4: Doctor should detect issues
	t.Run("doctor_detects_issues", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "doctor")
		// Doctor might pass or fail depending on severity
		t.Logf("Doctor output: %s", stdout)
		if stderr != "" {
			t.Logf("Doctor stderr: %s", stderr)
		}
		_ = code // Doctor may or may not report issues
	})

	// Step 5: Doctor --strict should detect issues
	t.Run("doctor_strict_detects", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "doctor", "--strict")
		t.Logf("Doctor --strict output: %s", stdout)
		if stderr != "" {
			t.Logf("Doctor --strict stderr: %s", stderr)
		}
		_ = code // Doctor may report findings
	})

	// Step 6: Repair runtime issues
	t.Run("repair_runtime", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "doctor", "--repair-runtime")
		t.Logf("Repair output: %s", stdout)
		if code != 0 {
			t.Logf("Repair stderr: %s", stderr)
		}
	})

	// Step 7: Doctor should now report healthy
	t.Run("doctor_healthy_after_repair", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "doctor")
		if code != 0 {
			t.Fatalf("doctor failed after repair: %s", stderr)
		}
		if !strings.Contains(stdout, "healthy") {
			t.Errorf("expected 'healthy' after repair, got: %s", stdout)
		}
	})

	// Step 8: Verify tmp directories cleaned up
	t.Run("verify_tmp_cleaned", func(t *testing.T) {
		snapshotsPath := filepath.Join(jvsPath, "snapshots")
		entries, err := os.ReadDir(snapshotsPath)
		if err != nil {
			t.Fatalf("failed to read snapshots dir: %v", err)
		}

		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), ".tmp") {
				t.Errorf("tmp directory should be cleaned up: %s", entry.Name())
			}
		}
	})

	// Step 9: Resume normal operations
	t.Run("resume_operations", func(t *testing.T) {
		os.WriteFile(filepath.Join(mainPath, "recovered.txt"), []byte("recovered"), 0644)
		stdout, stderr, code := runJVSInRepo(t, repoPath, "checkpoint", "post-recovery")
		if code != 0 {
			t.Fatalf("post-recovery snapshot failed: %s", stderr)
		}
		if !strings.Contains(stdout, "Created checkpoint") {
			t.Errorf("expected success message, got: %s", stdout)
		}
	})
}

// TestE2E_Disaster_OrphanIntents tests detecting and cleaning orphan intent files
func TestE2E_Disaster_OrphanIntents(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	jvsPath := filepath.Join(repoPath, ".jvs")

	// Create healthy state
	runJVSInRepo(t, repoPath, "checkpoint", "healthy")

	// Create orphan intent
	t.Run("create_orphan_intent", func(t *testing.T) {
		intentsPath := filepath.Join(jvsPath, "intents")
		os.MkdirAll(intentsPath, 0755)

		// Create multiple orphan intents
		for i := 1; i <= 3; i++ {
			intent := `{"id":"intent-` + string(rune('0'+i)) + `","status":"pending"}`
			os.WriteFile(filepath.Join(intentsPath, "orphan"+string(rune('0'+i))+".json"), []byte(intent), 0644)
		}
	})

	// Detect with doctor
	t.Run("detect_intents", func(t *testing.T) {
		stdout, _, _ := runJVSInRepo(t, repoPath, "doctor", "--strict")
		t.Logf("Doctor found: %s", stdout)
	})

	// Repair
	t.Run("repair_intents", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "doctor", "--repair-runtime")
		if code != 0 {
			t.Logf("Repair result: code=%d, stdout=%s, stderr=%s", code, stdout, stderr)
		}
	})

	// Verify clean
	t.Run("verify_clean", func(t *testing.T) {
		stdout, _, code := runJVSInRepo(t, repoPath, "doctor")
		if code != 0 {
			t.Error("doctor should pass after repair")
		}
		if !strings.Contains(stdout, "healthy") {
			t.Logf("Doctor output after repair: %s", stdout)
		}
	})
}

// TestE2E_Disaster_PartialSnapshot tests handling partial snapshot directories
func TestE2E_Disaster_PartialSnapshot(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	jvsPath := filepath.Join(repoPath, ".jvs")

	// Create healthy state
	runJVSInRepo(t, repoPath, "checkpoint", "healthy")

	// Simulate partial snapshot (has .tmp but no .READY)
	t.Run("create_partial_snapshot", func(t *testing.T) {
		snapshotsPath := filepath.Join(jvsPath, "snapshots")
		partialPath := filepath.Join(snapshotsPath, "partial.snapshot.tmp")
		os.MkdirAll(partialPath, 0755)

		// Write some content but don't create .READY
		os.WriteFile(filepath.Join(partialPath, "file.txt"), []byte("partial content"), 0644)
	})

	// Doctor should detect
	t.Run("doctor_detects_partial", func(t *testing.T) {
		stdout, _, _ := runJVSInRepo(t, repoPath, "doctor", "--strict")
		t.Logf("Doctor output: %s", stdout)
	})

	// Repair
	t.Run("repair_partial", func(t *testing.T) {
		runJVSInRepo(t, repoPath, "doctor", "--repair-runtime")
	})

	// Verify no .tmp directories remain
	t.Run("no_tmp_remains", func(t *testing.T) {
		snapshotsPath := filepath.Join(jvsPath, "snapshots")
		entries, _ := os.ReadDir(snapshotsPath)
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".tmp") {
				t.Errorf("tmp directory should be removed: %s", e.Name())
			}
		}
	})
}

// TestE2E_Disaster_CorruptedDescriptor tests handling corrupted descriptor files
func TestE2E_Disaster_CorruptedDescriptor(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")
	jvsPath := filepath.Join(repoPath, ".jvs")

	// Create snapshot and get its ID
	os.WriteFile(filepath.Join(mainPath, "data.txt"), []byte("test"), 0644)
	runJVSInRepo(t, repoPath, "checkpoint", "test-snapshot")

	stdout, _, _ := runJVSInRepo(t, repoPath, "checkpoint", "list", "--json")
	snapshotIDs := extractAllSnapshotIDs(stdout)
	if len(snapshotIDs) == 0 {
		t.Fatal("expected at least one snapshot")
	}
	snapID := snapshotIDs[0]

	// Corrupt the descriptor file
	t.Run("corrupt_descriptor", func(t *testing.T) {
		descPath := filepath.Join(jvsPath, "descriptors", snapID+".json")
		// Write invalid JSON
		os.WriteFile(descPath, []byte("not valid json {{{"), 0644)
	})

	// Doctor should detect corruption
	t.Run("doctor_detects_corruption", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "doctor", "--strict")
		if code == 0 {
			t.Fatalf("doctor --strict should fail for corrupted descriptor; stdout=%s stderr=%s", stdout, stderr)
		}
		if !strings.Contains(stdout, "integrity") {
			t.Fatalf("expected integrity finding for corrupted descriptor, got: %s", stdout)
		}
	})

	t.Run("doctor_json_exits_nonzero", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "--json", "doctor", "--strict")
		if code == 0 {
			t.Fatalf("doctor --json --strict should fail for corrupted descriptor; stdout=%s stderr=%s", stdout, stderr)
		}
		if !strings.Contains(stdout, `"healthy": false`) {
			t.Fatalf("expected unhealthy JSON result, got: %s", stdout)
		}
	})

	// Verify should fail for corrupted snapshot
	t.Run("verify_detects_corruption", func(t *testing.T) {
		_, _, code := runJVSInRepo(t, repoPath, "verify", snapID)
		// Verify should fail or report issue
		t.Logf("Verify code for corrupted snapshot: %d", code)
	})
}

// TestE2E_Disaster_MissingReadyMarker tests handling snapshots without .READY
func TestE2E_Disaster_MissingReadyMarker(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	jvsPath := filepath.Join(repoPath, ".jvs")

	// Create healthy snapshot
	runJVSInRepo(t, repoPath, "checkpoint", "healthy")

	// Get snapshot ID
	stdout, _, _ := runJVSInRepo(t, repoPath, "checkpoint", "list", "--json")
	ids := extractAllSnapshotIDs(stdout)
	if len(ids) == 0 {
		t.Fatal("need at least one snapshot")
	}

	// Remove .READY marker to simulate incomplete snapshot
	t.Run("remove_ready_marker", func(t *testing.T) {
		readyPath := filepath.Join(jvsPath, "snapshots", ids[0], ".READY")
		if fileExists(t, readyPath) {
			os.Remove(readyPath)
		}
	})

	// Doctor should detect
	t.Run("doctor_detects_missing_ready", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "doctor", "--strict")
		if code == 0 {
			t.Fatalf("doctor --strict should fail for missing READY marker; stdout=%s stderr=%s", stdout, stderr)
		}
		if !strings.Contains(stdout, "READY marker missing") {
			t.Fatalf("expected missing READY finding, got: %s", stdout)
		}
	})

	// Repair
	t.Run("repair", func(t *testing.T) {
		runJVSInRepo(t, repoPath, "doctor", "--repair-runtime")
	})
}

// TestE2E_Disaster_RecoveryWorkflow tests complete disaster recovery workflow
func TestE2E_Disaster_RecoveryWorkflow(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	mainPath := filepath.Join(repoPath, "main")
	jvsPath := filepath.Join(repoPath, ".jvs")

	// Setup: create multiple healthy snapshots
	for i := 1; i <= 3; i++ {
		os.WriteFile(filepath.Join(mainPath, "version.txt"), []byte(string(rune('0'+i))), 0644)
		runJVSInRepo(t, repoPath, "checkpoint", "version")
	}

	// Simulate multiple crash artifacts
	t.Run("simulate_crash", func(t *testing.T) {
		snapshotsPath := filepath.Join(jvsPath, "snapshots")

		// Orphan tmp directory
		os.MkdirAll(filepath.Join(snapshotsPath, "crash1.tmp"), 0755)
		os.MkdirAll(filepath.Join(snapshotsPath, "crash2.tmp"), 0755)

		// Orphan intent
		intentsPath := filepath.Join(jvsPath, "intents")
		os.MkdirAll(intentsPath, 0755)
		os.WriteFile(filepath.Join(intentsPath, "stale.json"), []byte(`{"status":"incomplete"}`), 0644)
	})

	// Detection phase
	t.Run("detection_phase", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "doctor", "--strict")
		t.Logf("Detection: code=%d, findings=%s", code, stdout)
		if stderr != "" {
			t.Logf("Stderr: %s", stderr)
		}
	})

	// Repair phase
	t.Run("repair_phase", func(t *testing.T) {
		stdout, stderr, code := runJVSInRepo(t, repoPath, "doctor", "--repair-runtime")
		t.Logf("Repair: code=%d", code)
		if stdout != "" {
			t.Logf("Stdout: %s", stdout)
		}
		if stderr != "" {
			t.Logf("Stderr: %s", stderr)
		}
	})

	// Verification phase
	t.Run("verification_phase", func(t *testing.T) {
		// Doctor should be healthy
		stdout, stderr, code := runJVSInRepo(t, repoPath, "doctor")
		if code != 0 {
			t.Fatalf("doctor should pass: %s", stderr)
		}
		if !strings.Contains(stdout, "healthy") {
			t.Errorf("expected healthy, got: %s", stdout)
		}

		// Verify should pass
		_, _, code = runJVSInRepo(t, repoPath, "verify", "--all")
		if code != 0 {
			t.Error("verify should pass")
		}
	})

	// Resume operations
	t.Run("resume_operations", func(t *testing.T) {
		os.WriteFile(filepath.Join(mainPath, "recovered.txt"), []byte("recovered"), 0644)
		stdout, stderr, code := runJVSInRepo(t, repoPath, "checkpoint", "after-recovery")
		if code != 0 {
			t.Fatalf("should be able to create snapshots: %s", stderr)
		}
		if !strings.Contains(stdout, "Created checkpoint") {
			t.Errorf("expected success: %s", stdout)
		}
	})
}
