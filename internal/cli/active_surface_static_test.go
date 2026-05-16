package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestActiveRootSurfaceRegistersOnlyCollapsedCommands(t *testing.T) {
	cmd := createTestRootCmd()
	active := map[string]bool{}
	for _, child := range cmd.Commands() {
		active[child.Name()] = true
	}

	for _, required := range []string{"afscp", "completion", "doctor", "init", "repo", "status"} {
		if !active[required] {
			t.Fatalf("active root surface missing %q; got %v", required, active)
		}
	}
	for _, forbidden := range []string{"restore", "save"} {
		if active[forbidden] {
			t.Fatalf("legacy command %q is still registered in the active root surface", forbidden)
		}
	}
}

func TestLegacyCleanupAndRecoveryAreNotRegisteredOnProductionRoot(t *testing.T) {
	for _, path := range []string{
		"save.go",
		"restore.go",
		"gc.go",
		"recovery.go",
	} {
		t.Run(path, func(t *testing.T) {
			body := readCLISourceForStaticGuard(t, path)
			if strings.Contains(body, "rootCmd.AddCommand") {
				t.Fatalf("%s still registers a hidden legacy root command", path)
			}
		})
	}
}

func TestActivePublicJSONDoesNotExposeSaveProfileContract(t *testing.T) {
	body := readCLISourceForStaticGuard(t, "public_json.go")
	lower := strings.ToLower(body)
	for _, forbidden := range []string{
		"save_profile",
		"e_payload_hash_mismatch",
	} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("public_json.go still exposes legacy JSON contract fragment %q", forbidden)
		}
	}
}

func TestActiveCLIHelpersAvoidPayloadHashAndCapacityPreflight(t *testing.T) {
	for _, path := range []string{
		"capacity_active.go",
		"dirty.go",
		"doctor.go",
	} {
		t.Run(path, func(t *testing.T) {
			body := readCLISourceForStaticGuard(t, path)
			lower := strings.ToLower(body)
			for _, forbidden := range []string{
				"computepayloadroothash",
				"internal/integrity",
				"internal/verify",
				"treesize",
				"filepath.walk",
				"walkdir",
				"payload hash",
				"content hash",
				"capacity preflight",
			} {
				if strings.Contains(lower, forbidden) {
					t.Fatalf("%s contains forbidden active hot-path fragment %q", path, forbidden)
				}
			}
		})
	}
}

func readCLISourceForStaticGuard(t *testing.T, name string) string {
	t.Helper()

	body, err := os.ReadFile(filepath.Join(".", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(body)
}
