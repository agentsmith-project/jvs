//go:build conformance && juicefs_e2e

package conformance

import (
	"os"
	"path/filepath"
	"testing"
)

func TestJuiceFSUserStory_CapabilityAndInitExposeJuiceFSClone(t *testing.T) {
	t.Setenv("JVS_SNAPSHOT_ENGINE", "")
	t.Setenv("JVS_ENGINE", "")
	env := requireLocalJuiceFSE2E(t)

	capability := runJVSJSON[storySetupJSON](t, env.MountPath, "capability", env.MountPath, "--write-probe")
	if !capability.WriteProbe {
		t.Fatal("capability write_probe = false, want true")
	}
	if capability.TargetPath != env.MountPath {
		t.Fatalf("capability target_path = %q, want %q", capability.TargetPath, env.MountPath)
	}
	if !capability.JuiceFS.Available || !capability.JuiceFS.Supported {
		t.Fatalf("capability juicefs = %#v, want available and supported", capability.JuiceFS)
	}
	if capability.RecommendedEngine != juiceFSEngineName {
		t.Fatalf("capability recommended_engine = %q, want %q", capability.RecommendedEngine, juiceFSEngineName)
	}
	requireJuiceFSSetupEngine(t, capability, "capability")

	repoPath := filepath.Join(env.MountPath, "visibility-repo")
	initData := runJVSJSON[storySetupJSON](t, env.MountPath, "init", repoPath)
	if initData.RepoRoot != repoPath {
		t.Fatalf("init repo_root = %q, want %q", initData.RepoRoot, repoPath)
	}
	requireJuiceFSSetupEngine(t, initData, "init")
}

func TestJuiceFSUserStory_CheckpointRestoreRoundTripUsesJuiceFSClone(t *testing.T) {
	t.Setenv("JVS_SNAPSHOT_ENGINE", "")
	t.Setenv("JVS_ENGINE", "")
	env := requireLocalJuiceFSE2E(t)

	repoPath := filepath.Join(env.MountPath, "roundtrip-repo")
	initData := runJVSJSON[storySetupJSON](t, env.MountPath, "init", repoPath)
	requireJuiceFSSetupEngine(t, initData, "init")
	mainPath := filepath.Join(repoPath, "main")

	if err := os.MkdirAll(filepath.Join(mainPath, "docs"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mainPath, "docs", "story.txt"), []byte("draft one\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mainPath, "data.bin"), []byte{0, 1, 2, 3, 254, 255}, 0644); err != nil {
		t.Fatal(err)
	}

	first := runJVSJSON[storyCheckpointJSON](t, mainPath, "checkpoint", "first JuiceFS state", "--tag", "v1")
	requireJuiceFSCheckpointEngine(t, first, "first checkpoint")

	if err := os.WriteFile(filepath.Join(mainPath, "docs", "story.txt"), []byte("draft two\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(mainPath, "data.bin")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mainPath, "new.txt"), []byte("new work\n"), 0644); err != nil {
		t.Fatal(err)
	}

	second := runJVSJSON[storyCheckpointJSON](t, mainPath, "checkpoint", "second JuiceFS state", "--tag", "v2")
	requireJuiceFSCheckpointEngine(t, second, "second checkpoint")
	if second.CheckpointID == first.CheckpointID {
		t.Fatal("second checkpoint reused first checkpoint ID")
	}

	restoreFirst := runJVSJSON[storyStatusJSON](t, mainPath, "restore", first.CheckpointID)
	if restoreFirst.Current != first.CheckpointID || restoreFirst.Latest != second.CheckpointID || restoreFirst.AtLatest {
		t.Fatalf("restore first status = %#v, want current first, latest second, at_latest=false", restoreFirst)
	}
	requireFileContent(t, filepath.Join(mainPath, "docs", "story.txt"), "draft one\n")
	requireFileContent(t, filepath.Join(mainPath, "data.bin"), string([]byte{0, 1, 2, 3, 254, 255}))
	requireNoPath(t, filepath.Join(mainPath, "new.txt"))

	restoreLatest := runJVSJSON[storyStatusJSON](t, mainPath, "restore", "latest")
	if restoreLatest.Current != second.CheckpointID || restoreLatest.Latest != second.CheckpointID || !restoreLatest.AtLatest {
		t.Fatalf("restore latest status = %#v, want current/latest second, at_latest=true", restoreLatest)
	}
	requireFileContent(t, filepath.Join(mainPath, "docs", "story.txt"), "draft two\n")
	requireNoPath(t, filepath.Join(mainPath, "data.bin"))
	requireFileContent(t, filepath.Join(mainPath, "new.txt"), "new work\n")
}

func TestJuiceFSUserStory_ImportAndCloneCurrentUseOptimizedSameMountTransfer(t *testing.T) {
	t.Setenv("JVS_SNAPSHOT_ENGINE", "")
	t.Setenv("JVS_ENGINE", "")
	env := requireLocalJuiceFSE2E(t)

	source := filepath.Join(env.MountPath, "import-source")
	if err := os.MkdirAll(filepath.Join(source, "nested"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "nested", "payload.txt"), []byte("source payload\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "root.txt"), []byte("root payload\n"), 0644); err != nil {
		t.Fatal(err)
	}

	importRepo := filepath.Join(env.MountPath, "imported-repo")
	importData := runJVSJSON[storySetupJSON](t, env.MountPath, "import", source, importRepo)
	if importData.Scope != "import" || importData.RequestedScope != "import" {
		t.Fatalf("import scope = %q/%q, want import/import", importData.Scope, importData.RequestedScope)
	}
	if importData.Engine != juiceFSEngineName {
		t.Fatalf("import initial checkpoint engine = %q, want %q", importData.Engine, juiceFSEngineName)
	}
	requireOptimizedJuiceFSTransfer(t, importData, "import")
	requireFileContent(t, filepath.Join(importRepo, "main", "nested", "payload.txt"), "source payload\n")
	requireFileContent(t, filepath.Join(source, "nested", "payload.txt"), "source payload\n")

	cloneRepo := filepath.Join(env.MountPath, "clone-current-repo")
	cloneData := runJVSJSON[storySetupJSON](t, env.MountPath, "clone", importRepo, cloneRepo, "--scope", "current")
	if cloneData.Scope != "current" || cloneData.RequestedScope != "current" {
		t.Fatalf("clone current scope = %q/%q, want current/current", cloneData.Scope, cloneData.RequestedScope)
	}
	if cloneData.Engine != juiceFSEngineName {
		t.Fatalf("clone current initial checkpoint engine = %q, want %q", cloneData.Engine, juiceFSEngineName)
	}
	requireOptimizedJuiceFSTransfer(t, cloneData, "clone current")
	requireFileContent(t, filepath.Join(cloneRepo, "main", "nested", "payload.txt"), "source payload\n")
	requireFileContent(t, filepath.Join(cloneRepo, "main", "root.txt"), "root payload\n")
}

func TestJuiceFSUserStory_ExplicitJuiceFSFallbackVisibleOutsideMount(t *testing.T) {
	t.Setenv("JVS_SNAPSHOT_ENGINE", "")
	t.Setenv("JVS_ENGINE", "")
	env := requireLocalJuiceFSE2E(t)

	outside := filepath.Join(env.BaseDir, "outside-fallback")
	if err := os.MkdirAll(outside, 0755); err != nil {
		t.Fatal(err)
	}
	outsideCapability := runJVSJSON[storySetupJSON](t, outside, "capability", outside, "--write-probe")
	if outsideCapability.JuiceFS.Supported {
		t.Fatalf("outside fallback target unexpectedly reports JuiceFS support: %#v", outsideCapability.JuiceFS)
	}
	if !stringSliceContainsFragment(outsideCapability.JuiceFS.Warnings, "not on a JuiceFS mount") {
		t.Fatalf("outside fallback capability warnings = %#v, want not-on-JuiceFS explanation", outsideCapability.JuiceFS.Warnings)
	}

	repoPath := filepath.Join(outside, "fallback-repo")
	runJVSJSON[storySetupJSON](t, outside, "init", repoPath)
	mainPath := filepath.Join(repoPath, "main")
	if err := os.WriteFile(filepath.Join(mainPath, "outside.txt"), []byte("outside mount\n"), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("JVS_SNAPSHOT_ENGINE", juiceFSEngineName)
	checkpoint := runJVSJSON[storyCheckpointJSON](t, mainPath, "checkpoint", "forced JuiceFS outside mount")
	if checkpoint.Engine != juiceFSEngineName {
		t.Fatalf("fallback checkpoint requested engine = %q, want %q", checkpoint.Engine, juiceFSEngineName)
	}
	if checkpoint.EffectiveEngine != "copy" || checkpoint.ActualEngine != "copy" {
		t.Fatalf("fallback checkpoint actual/effective = %q/%q, want copy/copy", checkpoint.ActualEngine, checkpoint.EffectiveEngine)
	}
	if !stringSliceContainsFragment(checkpoint.DegradedReasons, "not-on-juicefs") {
		t.Fatalf("fallback checkpoint degraded_reasons = %#v, want not-on-juicefs", checkpoint.DegradedReasons)
	}
}
