//go:build conformance

package conformance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestContract_CapabilityHardlinkMetadataDoesNotPromiseOccurrenceDetection(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "capability-target")
	if err := os.Mkdir(target, 0755); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runJVS(t, dir, "--json", "capability", target)
	if code != 0 {
		t.Fatalf("capability failed: stdout=%s stderr=%s", stdout, stderr)
	}
	metadata, ok := decodeContractSmokeDataMap(t, stdout)["metadata_preservation"].(map[string]any)
	if !ok {
		t.Fatalf("capability metadata_preservation must be an object: %s", stdout)
	}
	requireMetadataOwnership(t, metadata, stdout)
	hardlinks, _ := metadata["hardlinks"].(string)
	lower := strings.ToLower(hardlinks)
	if !strings.Contains(lower, "not guaranteed") {
		t.Fatalf("hardlink metadata must state v0 identity is not guaranteed: %q\n%s", hardlinks, stdout)
	}
	for _, forbidden := range []string{"report", "detect"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("hardlink metadata must not promise per-occurrence %s behavior: %q\n%s", forbidden, hardlinks, stdout)
		}
	}
}

func TestContract_SetupAndCheckpointDescriptorMetadataOwnership(t *testing.T) {
	dir := t.TempDir()
	repoName := "metadata-ownership"
	stdout, stderr, code := runJVS(t, dir, "--json", "init", repoName)
	if code != 0 {
		t.Fatalf("init failed: stdout=%s stderr=%s", stdout, stderr)
	}
	metadata, ok := decodeContractSmokeDataMap(t, stdout)["metadata_preservation"].(map[string]any)
	if !ok {
		t.Fatalf("init metadata_preservation must be an object: %s", stdout)
	}
	requireMetadataOwnership(t, metadata, stdout)

	repoPath := filepath.Join(dir, repoName)
	mainPath := filepath.Join(repoPath, "main")
	if err := os.WriteFile(filepath.Join(mainPath, "file.txt"), []byte("v1"), 0644); err != nil {
		t.Fatal(err)
	}
	checkpointOut, stderr, code := runJVSInRepo(t, repoPath, "--json", "checkpoint", "metadata ownership")
	if code != 0 {
		t.Fatalf("checkpoint failed: stdout=%s stderr=%s", checkpointOut, stderr)
	}
	checkpointData := decodeContractSmokeDataMap(t, checkpointOut)
	metadata, ok = checkpointData["metadata_preservation"].(map[string]any)
	if !ok {
		t.Fatalf("checkpoint metadata_preservation must be an object: %s", checkpointOut)
	}
	requireMetadataOwnership(t, metadata, checkpointOut)

	checkpointID, ok := checkpointData["checkpoint_id"].(string)
	if !ok || checkpointID == "" {
		t.Fatalf("checkpoint output missing checkpoint_id: %s", checkpointOut)
	}
	descriptorData, err := os.ReadFile(filepath.Join(repoPath, ".jvs", "descriptors", checkpointID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var descriptor map[string]any
	if err := json.Unmarshal(descriptorData, &descriptor); err != nil {
		t.Fatalf("decode descriptor: %v\n%s", err, descriptorData)
	}
	metadata, ok = descriptor["metadata_preservation"].(map[string]any)
	if !ok {
		t.Fatalf("descriptor metadata_preservation must be an object: %s", descriptorData)
	}
	requireMetadataOwnership(t, metadata, string(descriptorData))
}

func TestContract_CloneCurrentMetadataOwnership(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	dest := filepath.Join(dir, "dest")
	if stdout, stderr, code := runJVS(t, dir, "init", "source"); code != 0 {
		t.Fatalf("source init failed: stdout=%s stderr=%s", stdout, stderr)
	}
	if err := os.WriteFile(filepath.Join(source, "main", "file.txt"), []byte("v1"), 0644); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code := runJVS(t, dir, "--json", "clone", source, dest, "--scope", "current")
	if code != 0 {
		t.Fatalf("clone current failed: stdout=%s stderr=%s", stdout, stderr)
	}
	metadata, ok := decodeContractSmokeDataMap(t, stdout)["metadata_preservation"].(map[string]any)
	if !ok {
		t.Fatalf("clone metadata_preservation must be an object: %s", stdout)
	}
	requireMetadataOwnership(t, metadata, stdout)
}

func requireMetadataOwnership(t *testing.T, metadata map[string]any, context string) {
	t.Helper()
	if value, _ := metadata["ownership"].(string); value == "" {
		t.Fatalf("metadata_preservation.ownership must be non-empty: %s", context)
	}
}
