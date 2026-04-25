//go:build conformance

package conformance

import (
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
