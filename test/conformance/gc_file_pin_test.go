//go:build conformance

package conformance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jvs-project/jvs/internal/integrity"
	"github.com/jvs-project/jvs/pkg/model"
)

func createOldOrphanedSnapshot(t *testing.T, repoPath string) string {
	t.Helper()

	mainPath := filepath.Join(repoPath, "main")
	requireWriteFile(t, filepath.Join(mainPath, "base.txt"), "base")
	baseOut, stderr, code := runJVSInRepo(t, repoPath, "snapshot", "base", "--json")
	if code != 0 {
		t.Fatalf("base snapshot failed: %s", stderr)
	}
	baseID := extractFirstSnapshotID(baseOut)
	if baseID == "" {
		t.Fatalf("base snapshot id not found in output: %s", baseOut)
	}

	_, stderr, code = runJVSInRepo(t, repoPath, "worktree", "fork", baseID, "temp")
	if code != 0 {
		t.Fatalf("worktree fork failed: %s", stderr)
	}

	tempPath := filepath.Join(repoPath, "worktrees", "temp")
	requireWriteFile(t, filepath.Join(tempPath, "temp.txt"), "temp")
	tempOut, stderr, code := runJVSInWorktree(t, repoPath, "temp", "snapshot", "temp", "--json")
	if code != 0 {
		t.Fatalf("temp snapshot failed: %s", stderr)
	}
	tempID := extractFirstSnapshotID(tempOut)
	if tempID == "" {
		t.Fatalf("temp snapshot id not found in output: %s", tempOut)
	}

	ageSnapshotDescriptor(t, repoPath, tempID)
	_, stderr, code = runJVSInRepo(t, repoPath, "worktree", "remove", "temp")
	if code != 0 {
		t.Fatalf("worktree remove failed: %s", stderr)
	}
	return tempID
}

func ageSnapshotDescriptor(t *testing.T, repoPath, snapshotID string) {
	t.Helper()

	descriptorPath := filepath.Join(repoPath, ".jvs", "descriptors", snapshotID+".json")
	data, err := os.ReadFile(descriptorPath)
	if err != nil {
		t.Fatalf("read descriptor: %v", err)
	}
	var descriptor model.Descriptor
	if err := json.Unmarshal(data, &descriptor); err != nil {
		t.Fatalf("parse descriptor: %v", err)
	}
	descriptor.CreatedAt = time.Now().Add(-48 * time.Hour).UTC()
	checksum, err := integrity.ComputeDescriptorChecksum(&descriptor)
	if err != nil {
		t.Fatalf("compute descriptor checksum: %v", err)
	}
	descriptor.DescriptorChecksum = checksum
	data, err = json.MarshalIndent(descriptor, "", "  ")
	if err != nil {
		t.Fatalf("marshal descriptor: %v", err)
	}
	if err := os.WriteFile(descriptorPath, data, 0644); err != nil {
		t.Fatalf("write descriptor: %v", err)
	}
}

func writeDocumentedGCPin(t *testing.T, repoPath, snapshotID string) {
	t.Helper()

	pinsDir := filepath.Join(repoPath, ".jvs", "gc", "pins")
	if err := os.MkdirAll(pinsDir, 0755); err != nil {
		t.Fatalf("create pins dir: %v", err)
	}
	pin := map[string]any{
		"pin_id":      snapshotID,
		"snapshot_id": snapshotID,
		"reason":      "conformance file pin",
		"created_at":  time.Now().UTC().Format(time.RFC3339Nano),
		"expires_at":  nil,
	}
	data, err := json.MarshalIndent(pin, "", "  ")
	if err != nil {
		t.Fatalf("marshal pin: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pinsDir, snapshotID+".json"), data, 0644); err != nil {
		t.Fatalf("write pin: %v", err)
	}
}

func requireWriteFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write file %s: %v", path, err)
	}
}

func TestGC_FilePinAtDocumentedPathProtectsSnapshot(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	snapshotID := createOldOrphanedSnapshot(t, repoPath)
	writeDocumentedGCPin(t, repoPath, snapshotID)

	planOut, stderr, code := runJVSInRepo(t, repoPath, "gc", "plan", "--json")
	if code != 0 {
		t.Fatalf("gc plan failed: %s", stderr)
	}

	if got := extractJSONField(planOut, "candidate_count"); got != "0" {
		t.Fatalf("expected file-pinned snapshot to be protected, candidate_count=%s plan=%s", got, planOut)
	}
	if got := extractJSONField(planOut, "protected_by_pin"); got != "1" {
		t.Fatalf("expected protected_by_pin=1, got %s plan=%s", got, planOut)
	}
}

func TestGC_RunFailsWhenPinProtectsPlannedCandidate(t *testing.T) {
	repoPath, _ := initTestRepo(t)
	snapshotID := createOldOrphanedSnapshot(t, repoPath)

	planOut, stderr, code := runJVSInRepo(t, repoPath, "gc", "plan", "--json")
	if code != 0 {
		t.Fatalf("gc plan failed: %s", stderr)
	}
	planID := extractPlanID(planOut)
	if planID == "" {
		t.Fatalf("plan id not found in output: %s", planOut)
	}
	if got := extractJSONField(planOut, "candidate_count"); got != "1" {
		t.Fatalf("expected one GC candidate before pin, got %s plan=%s", got, planOut)
	}

	writeDocumentedGCPin(t, repoPath, snapshotID)

	_, stderr, code = runJVSInRepo(t, repoPath, "gc", "run", "--plan-id", planID)
	if code == 0 {
		t.Fatalf("gc run should fail after candidate becomes pinned")
	}
	if !strings.Contains(stderr, "E_GC_PLAN_MISMATCH") {
		t.Fatalf("expected E_GC_PLAN_MISMATCH, stderr=%s", stderr)
	}
}
