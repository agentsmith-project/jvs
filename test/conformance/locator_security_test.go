//go:build conformance

package conformance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocatorSecurityRejectsForgedLocatorMismatchWithRepoFlag(t *testing.T) {
	targetRepo, targetCleanup := initTestRepo(t)
	defer targetCleanup()
	currentRepo, currentCleanup := initTestRepo(t)
	defer currentCleanup()

	createFiles(t, targetRepo, map[string]string{"README.md": "target\n"})
	savePoint(t, targetRepo, "target baseline")

	child := filepath.Join(currentRepo, "nested", "task")
	if err := os.MkdirAll(child, 0755); err != nil {
		t.Fatalf("create nested current repo child: %v", err)
	}
	writeConformanceWorkspaceLocator(t, child, targetRepo)

	stdout, stderr, code := runJVS(t, child, "--json", "--repo", targetRepo, "history")
	if code == 0 {
		t.Fatalf("forged locator mismatch unexpectedly succeeded: stdout=%s stderr=%s", stdout, stderr)
	}
	env := requirePureJSONEnvelope(t, stdout, stderr, false)
	if env.Error == nil || env.Error.Code != "E_TARGET_MISMATCH" {
		t.Fatalf("forged locator mismatch error = %#v, want E_TARGET_MISMATCH\n%s", env.Error, stdout)
	}
}

func TestLocatorSecurityRepoFlagPathPrefersPhysicalAncestorOverForgedLocator(t *testing.T) {
	targetRepo, targetCleanup := initTestRepo(t)
	defer targetCleanup()
	attackerRepo, attackerCleanup := initTestRepo(t)
	defer attackerCleanup()

	child := filepath.Join(targetRepo, "nested", "task")
	if err := os.MkdirAll(child, 0755); err != nil {
		t.Fatalf("create nested target repo child: %v", err)
	}
	writeConformanceWorkspaceLocator(t, child, attackerRepo)

	outside := t.TempDir()
	stdout, stderr, code := runJVS(t, outside, "--json", "--repo", child, "status")
	if code != 0 {
		t.Fatalf("status with physical --repo target failed: stdout=%s stderr=%s", stdout, stderr)
	}
	env := requirePureJSONEnvelope(t, stdout, stderr, true)
	if env.RepoRoot == nil || *env.RepoRoot != targetRepo {
		t.Fatalf("--repo resolved repo_root = %#v, want %q\n%s", env.RepoRoot, targetRepo, stdout)
	}
	if env.Workspace == nil || *env.Workspace != "main" {
		t.Fatalf("--repo resolved workspace = %#v, want main\n%s", env.Workspace, stdout)
	}
}

func TestLocatorSecurityRepoFlagTargetPathMalformedLocatorIsHardFailure(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()

	child := filepath.Join(repoPath, "nested", "task")
	if err := os.MkdirAll(child, 0755); err != nil {
		t.Fatalf("create nested repo child: %v", err)
	}
	if err := os.WriteFile(filepath.Join(child, ".jvs"), []byte("{not-json"), 0644); err != nil {
		t.Fatalf("write invalid locator: %v", err)
	}

	outside := t.TempDir()
	stdout, stderr, code := runJVS(t, outside, "--json", "--repo", child, "status")
	if code == 0 {
		t.Fatalf("malformed target locator unexpectedly defaulted to main: stdout=%s stderr=%s", stdout, stderr)
	}
	env := requirePureJSONEnvelope(t, stdout, stderr, false)
	if env.Error == nil || !strings.Contains(env.Error.Message, "parse JVS workspace locator") {
		t.Fatalf("malformed target locator error = %#v, want parse locator context\n%s", env.Error, stdout)
	}
}

func TestLocatorSecurityRepoFlagExternalWorkspaceLocatorFallback(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()

	createFiles(t, repoPath, map[string]string{"app.txt": "v1\n"})
	base := savePoint(t, repoPath, "baseline before workspace")
	stdout, stderr, code := runJVSInRepo(t, repoPath, "--json", "workspace", "new", "../feature", "--from", base)
	if code != 0 {
		t.Fatalf("workspace new failed: stdout=%s stderr=%s", stdout, stderr)
	}

	featurePath := filepath.Join(filepath.Dir(repoPath), "feature")
	outside := t.TempDir()
	stdout, stderr, code = runJVS(t, outside, "--json", "--repo", featurePath, "--workspace", "feature", "status")
	if code != 0 {
		t.Fatalf("status with external workspace --repo target failed: stdout=%s stderr=%s", stdout, stderr)
	}
	env := requirePureJSONEnvelope(t, stdout, stderr, true)
	if env.RepoRoot == nil || *env.RepoRoot != repoPath {
		t.Fatalf("--repo external workspace resolved repo_root = %#v, want %q\n%s", env.RepoRoot, repoPath, stdout)
	}
	if env.Workspace == nil || *env.Workspace != "feature" {
		t.Fatalf("--repo external workspace resolved workspace = %#v, want feature\n%s", env.Workspace, stdout)
	}
	status := decodeContractDataMap(t, stdout)
	if status["folder"] != featurePath {
		t.Fatalf("--repo external workspace status folder = %#v, want %q\n%s", status["folder"], featurePath, stdout)
	}
}

func TestLocatorSecurityExplicitWorkspaceCannotBypassExternalLocatorRepoMismatch(t *testing.T) {
	repoA, cleanupA := initTestRepo(t)
	defer cleanupA()
	repoB, cleanupB := initTestRepo(t)
	defer cleanupB()

	createFiles(t, repoA, map[string]string{"app.txt": "repo a\n"})
	base := savePoint(t, repoA, "repo a base")
	stdout, stderr, code := runJVSInRepo(t, repoA, "--json", "workspace", "new", "../feature", "--from", base)
	if code != 0 {
		t.Fatalf("workspace new failed: stdout=%s stderr=%s", stdout, stderr)
	}
	featurePath := filepath.Join(filepath.Dir(repoA), "feature")

	stdout, stderr, code = runJVS(t, featurePath, "--json", "--repo", repoB, "--workspace", "main", "status")
	if code == 0 {
		t.Fatalf("explicit --workspace bypassed external locator repo mismatch: stdout=%s stderr=%s", stdout, stderr)
	}
	env := requirePureJSONEnvelope(t, stdout, stderr, false)
	if env.Error == nil || env.Error.Code != "E_TARGET_MISMATCH" {
		t.Fatalf("external locator mismatch error = %#v, want E_TARGET_MISMATCH\n%s", env.Error, stdout)
	}
	if !strings.Contains(env.Error.Message, repoA) || !strings.Contains(env.Error.Message, repoB) {
		t.Fatalf("external locator mismatch should mention both repos: %#v", env.Error)
	}

	stdout, stderr, code = runJVS(t, featurePath, "--json", "--repo", repoA, "--workspace", "main", "status")
	if code != 0 {
		t.Fatalf("same-repo explicit workspace target failed: stdout=%s stderr=%s", stdout, stderr)
	}
	status := decodeContractDataMap(t, stdout)
	if status["workspace"] != "main" || status["folder"] != repoA {
		t.Fatalf("same-repo explicit workspace should target main clearly: %#v", status)
	}
}

func TestLocatorSecurityRepoFlagMalformedLocatorInCurrentWorkspaceIsHardFailure(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()

	child := filepath.Join(repoPath, "nested", "task")
	if err := os.MkdirAll(child, 0755); err != nil {
		t.Fatalf("create nested repo child: %v", err)
	}
	if err := os.WriteFile(filepath.Join(child, ".jvs"), []byte("{not-json"), 0644); err != nil {
		t.Fatalf("write invalid locator: %v", err)
	}

	stdout, stderr, code := runJVS(t, child, "--json", "--repo", repoPath, "history")
	if code == 0 {
		t.Fatalf("malformed locator unexpectedly defaulted to main: stdout=%s stderr=%s", stdout, stderr)
	}
	env := requirePureJSONEnvelope(t, stdout, stderr, false)
	if env.Error == nil || !strings.Contains(env.Error.Message, "parse JVS workspace locator") {
		t.Fatalf("malformed locator error = %#v, want parse locator context\n%s", env.Error, stdout)
	}
}

func TestLocatorSecurityInvalidLocatorDoesNotFallThroughToAncestorRepository(t *testing.T) {
	repoPath, cleanup := initTestRepo(t)
	defer cleanup()

	createFiles(t, repoPath, map[string]string{"README.md": "safe ancestor\n"})
	savePoint(t, repoPath, "ancestor baseline")

	child := filepath.Join(repoPath, "nested", "task")
	if err := os.MkdirAll(child, 0755); err != nil {
		t.Fatalf("create nested repo child: %v", err)
	}
	if err := os.WriteFile(filepath.Join(child, ".jvs"), []byte("{not-json"), 0644); err != nil {
		t.Fatalf("write invalid locator: %v", err)
	}

	stdout, stderr, code := runJVS(t, child, "--json", "status")
	if code == 0 {
		t.Fatalf("invalid locator unexpectedly fell through to ancestor repo: stdout=%s stderr=%s", stdout, stderr)
	}
	env := requirePureJSONEnvelope(t, stdout, stderr, false)
	if env.Error == nil || env.Error.Code != "E_NOT_REPO" {
		t.Fatalf("invalid locator error = %#v, want E_NOT_REPO\n%s", env.Error, stdout)
	}

	status := jvsJSONData(t, repoPath, "status")
	if status["workspace"] != "main" || status["folder"] != repoPath {
		t.Fatalf("ancestor repo status should still work from its root: %#v", status)
	}
}

func TestLocatorSecurityRepoFlagPropagatesControlDiscoveryError(t *testing.T) {
	targetRepo, targetCleanup := initTestRepo(t)
	defer targetCleanup()
	currentRepo, currentCleanup := initTestRepo(t)
	defer currentCleanup()

	child := filepath.Join(currentRepo, "nested", "task")
	if err := os.MkdirAll(child, 0755); err != nil {
		t.Fatalf("create nested current repo child: %v", err)
	}
	if err := os.Symlink(".jvs", filepath.Join(child, ".jvs")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	stdout, stderr, code := runJVS(t, child, "--json", "--repo", targetRepo, "history")
	if code == 0 {
		t.Fatalf("control discovery error unexpectedly succeeded: stdout=%s stderr=%s", stdout, stderr)
	}
	env := requirePureJSONEnvelope(t, stdout, stderr, false)
	if env.Error == nil || env.Error.Code != "E_USAGE" {
		t.Fatalf("control discovery error = %#v, want E_USAGE\n%s", env.Error, stdout)
	}
	if !strings.Contains(env.Error.Message, "stat JVS control directory") {
		t.Fatalf("control discovery message = %q, want stat context", env.Error.Message)
	}
}

func TestLocatorSecurityMalformedWorkspaceLocatorIsNotRepairEvidence(t *testing.T) {
	for _, tc := range []struct {
		name     string
		repoRoot string
	}{
		{name: "blank", repoRoot: ""},
		{name: "relative", repoRoot: "relative/repo"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sourceRepo, cleanup := initTestRepo(t)
			defer cleanup()

			createFiles(t, sourceRepo, map[string]string{"app.txt": "v1\n"})
			base := savePoint(t, sourceRepo, "baseline before workspace")
			stdout, stderr, code := runJVSInRepo(t, sourceRepo, "--json", "workspace", "new", "../feature", "--from", base)
			if code != 0 {
				t.Fatalf("workspace new failed: stdout=%s stderr=%s", stdout, stderr)
			}

			sourceFeature := filepath.Join(filepath.Dir(sourceRepo), "feature")
			copiedRepo := filepath.Join(t.TempDir(), "fresh-volume", "project")
			copiedFeature := filepath.Join(filepath.Dir(copiedRepo), "feature")
			copyMigrationTree(t, sourceRepo, copiedRepo)
			copyMigrationTree(t, sourceFeature, copiedFeature)
			malformed := writeConformanceWorkspaceLocator(t, copiedFeature, tc.repoRoot)

			stdout, stderr, code = runJVSInRepo(t, copiedRepo, "--json", "doctor", "--strict", "--repair-runtime")
			if code == 0 {
				t.Fatalf("doctor repair-runtime unexpectedly accepted malformed locator: stdout=%s stderr=%s", stdout, stderr)
			}
			assertMigrationDoctorFinding(t, stdout, stderr, "E_WORKSPACE_PATH_BINDING_INVALID")

			after, err := os.ReadFile(filepath.Join(copiedFeature, ".jvs"))
			if err != nil {
				t.Fatalf("read malformed locator after repair: %v", err)
			}
			if !json.Valid(after) || !json.Valid(malformed) {
				t.Fatalf("locator test data must be JSON: before=%q after=%q", malformed, after)
			}
			var beforeValue, afterValue any
			if err := json.Unmarshal(malformed, &beforeValue); err != nil {
				t.Fatalf("decode malformed locator before repair: %v", err)
			}
			if err := json.Unmarshal(after, &afterValue); err != nil {
				t.Fatalf("decode malformed locator after repair: %v", err)
			}
			if before := beforeValue.(map[string]any); before["repo_root"] != tc.repoRoot {
				t.Fatalf("test locator repo_root = %#v, want %q", before["repo_root"], tc.repoRoot)
			}
			if got := afterValue.(map[string]any); got["repo_root"] != tc.repoRoot {
				t.Fatalf("malformed locator was overwritten: %#v", got)
			}
		})
	}
}

func writeConformanceWorkspaceLocator(t *testing.T, dir, repoRoot string) []byte {
	t.Helper()

	repoID := "malformed-repo-id"
	if repoRoot != "" && filepath.IsAbs(repoRoot) {
		data, err := os.ReadFile(filepath.Join(repoRoot, ".jvs", "repo_id"))
		if err == nil {
			repoID = strings.TrimSpace(string(data))
		}
	}
	data, err := json.Marshal(map[string]any{
		"type":           "jvs-workspace",
		"format_version": 1,
		"repo_root":      repoRoot,
		"repo_id":        repoID,
		"workspace_name": "main",
	})
	if err != nil {
		t.Fatalf("marshal locator: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".jvs"), data, 0644); err != nil {
		t.Fatalf("write locator: %v", err)
	}
	return data
}
