//go:build conformance

package conformance

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const (
	separatedRestoreSmokeAuditSentinelName    = "release-smoke-control-sentinel.txt"
	separatedRestoreSmokeAuditSentinelContent = "release smoke control sentinel\n"
	separatedRestoreSmokeRuntimeMarkerName    = "release-smoke-runtime-marker.txt"
	separatedRestoreSmokeRuntimeMarkerContent = "release smoke runtime marker\n"
)

func TestStorySeparatedRestoreRunStableStateDoctorStrictAndClone(t *testing.T) {
	base := t.TempDir()
	cleanCWD := filepath.Join(base, "operator-cwd")
	if err := os.MkdirAll(cleanCWD, 0755); err != nil {
		t.Fatalf("create clean cwd: %v", err)
	}

	sourceControl := filepath.Join(base, "source-control")
	sourcePayload := filepath.Join(base, "source-payload")
	initData := separatedRestoreSmokeInit(t, cleanCWD, sourceControl, sourcePayload, "main")
	requireSeparatedControlSetupFields(t, initData, sourcePayload)
	seedSeparatedRestoreSmokeControlMarkers(t, sourceControl)
	requireSeparatedRestoreSmokePayloadHasNoControlMetadata(t, sourcePayload)

	createFiles(t, sourcePayload, map[string]string{
		"README.md":               "payload v1\n",
		"repo_id":                 "user payload file named repo_id v1\n",
		"runtime/user-cache.txt":  "user payload runtime data v1\n",
		"snapshots/user-note.txt": "user payload snapshot notes v1\n",
		"src/app.js":              "console.log('v1')\n",
		"views/user-facing.txt":   "user payload view data v1\n",
	})
	firstSave, _ := separatedRestoreSmokeJSON(t, cleanCWD, sourceControl, sourcePayload, "main", "save", "-m", "payload v1")
	v1, _ := firstSave["save_point_id"].(string)
	if v1 == "" {
		t.Fatalf("first save missing save_point_id: %#v", firstSave)
	}
	requireSeparatedRestoreSmokePayloadHasNoControlMetadata(t, sourcePayload)

	createFiles(t, sourcePayload, map[string]string{
		"README.md":               "payload v2\n",
		"repo_id":                 "user payload file named repo_id v2\n",
		"runtime/user-cache.txt":  "user payload runtime data v2\n",
		"snapshots/user-note.txt": "user payload snapshot notes v2\n",
		"src/app.js":              "console.log('v2')\n",
		"tmp/generated.txt":       "v2 only\n",
		"views/user-facing.txt":   "user payload view data v2\n",
	})
	secondSave, _ := separatedRestoreSmokeJSON(t, cleanCWD, sourceControl, sourcePayload, "main", "save", "-m", "payload v2")
	v2, _ := secondSave["save_point_id"].(string)
	requireDifferentSavePoints(t, v1, v2)
	requireSeparatedRestoreSmokePayloadHasNoControlMetadata(t, sourcePayload)

	beforePreview := captureSeparatedLifecycleStoryRoot(t, sourcePayload)
	preview, _ := separatedRestoreSmokeJSON(t, cleanCWD, sourceControl, sourcePayload, "main", "restore", v1)
	planID, _ := preview["plan_id"].(string)
	if preview["mode"] != "preview" || preview["source_save_point"] != v1 || planID == "" {
		t.Fatalf("restore preview should target v1 and create a plan: %#v", preview)
	}
	requireSeparatedRestoreSmokePayloadUnchanged(t, beforePreview, sourcePayload)
	requireSeparatedRestoreSmokePayloadHasNoControlMetadata(t, sourcePayload)

	pendingRecovery, _ := separatedRestoreSmokeJSON(t, cleanCWD, sourceControl, sourcePayload, "main", "recovery", "status")
	requireSeparatedRestoreSmokeNoActiveRecovery(t, pendingRecovery)
	requireSeparatedRestoreSmokePendingRestoreState(t, pendingRecovery, planID)

	pendingDoctor, _ := separatedRestoreSmokeDoctorJSONAllowExit(t, cleanCWD, sourceControl, sourcePayload, "main", "--strict")
	if pendingDoctor["healthy"] != false {
		t.Fatalf("doctor strict should report pending restore preview unhealthy: %#v", pendingDoctor)
	}
	requireSeparatedRestoreSmokeDoctorCheck(t, pendingDoctor, "recovery_state", "E_RECOVERY_BLOCKING")

	pendingTargetControl := filepath.Join(base, "pending-target-control")
	pendingTargetPayload := filepath.Join(base, "pending-target-payload")
	stdout, stderr, code := runJVS(t, cleanCWD,
		"--json",
		"--control-root", sourceControl,
		"--workspace", "main",
		"repo", "clone",
		pendingTargetPayload,
		"--target-control-root", pendingTargetControl,
		"--save-points", "main",
	)
	env := requireSeparatedControlJSONError(t, stdout, stderr, code, "E_RECOVERY_BLOCKING")
	if !strings.Contains(env.Error.Message, planID) || !strings.Contains(env.Error.Hint, "restore --run "+planID) {
		t.Fatalf("clone blocker should name pending restore run command: %#v", env.Error)
	}
	requireAbsolutePathAbsent(t, pendingTargetControl)
	requireAbsolutePathAbsent(t, pendingTargetPayload)

	restoreRun, _ := separatedRestoreSmokeJSON(t, cleanCWD, sourceControl, sourcePayload, "main", "restore", "--run", planID)
	if restoreRun["mode"] != "run" || restoreRun["restored_save_point"] != v1 || restoreRun["content_source"] != v1 {
		t.Fatalf("restore run should make v1 the current content source: %#v", restoreRun)
	}
	requireSeparatedRestoreSmokePayloadV1(t, sourcePayload)
	requireSeparatedRestoreSmokePayloadHasNoControlMetadata(t, sourcePayload)

	recovery, _ := separatedRestoreSmokeJSON(t, cleanCWD, sourceControl, sourcePayload, "main", "recovery", "status")
	if recovery["mode"] != "status" {
		t.Fatalf("recovery status mode = %#v, want status in %#v", recovery["mode"], recovery)
	}
	if plans := jsonStringArrayOfObjectsLen(t, recovery["plans"]); plans != 0 {
		t.Fatalf("recovery status active plans count = %d, want 0 in %#v", plans, recovery["plans"])
	}

	sourceDoctor, _ := separatedRestoreSmokeDoctorJSON(t, cleanCWD, sourceControl, sourcePayload, "main", "--strict")
	if sourceDoctor["healthy"] != true {
		t.Fatalf("source doctor strict should be healthy after restore run: %#v", sourceDoctor)
	}
	requireSeparatedRestoreSmokeControlMarkers(t, sourceControl)

	targetControl := filepath.Join(base, "target-control")
	targetPayload := filepath.Join(base, "target-payload")
	cloneOut, cloneErr, cloneCode := runJVS(t, cleanCWD,
		"--json",
		"--control-root", sourceControl,
		"--workspace", "main",
		"repo", "clone",
		targetPayload,
		"--target-control-root", targetControl,
		"--save-points", "main",
	)
	if cloneCode != 0 {
		t.Fatalf("separated clone after restore run failed: stdout=%s stderr=%s", cloneOut, cloneErr)
	}
	requireNoPayloadRootJSONVocabulary(t, cloneOut)
	clone := requireSeparatedCloneExternalControlRootJSON(t, cloneOut, cloneErr, targetControl, targetPayload)
	if clone["source_repo_id"] == clone["target_repo_id"] {
		t.Fatalf("clone reused source repo_id: %#v", clone)
	}
	requireSeparatedCloneTransfers(t, clone, sourceControl, sourcePayload, targetControl, targetPayload)

	requireSeparatedRestoreSmokePayloadV1(t, sourcePayload)
	requireSeparatedRestoreSmokePayloadV1(t, targetPayload)
	requireSeparatedRestoreSmokePayloadHasNoControlMetadata(t, sourcePayload)
	requireSeparatedRestoreSmokePayloadHasNoControlMetadata(t, targetPayload)

	targetDoctor, _ := separatedRestoreSmokeDoctorJSON(t, cleanCWD, targetControl, targetPayload, "main", "--strict")
	if targetDoctor["healthy"] != true {
		t.Fatalf("target doctor strict should be healthy after clone: %#v", targetDoctor)
	}
	requireSeparatedRestoreSmokeControlMarkers(t, sourceControl)
}

func TestStorySeparatedRestorePreviewRunCommandFromCleanCWD(t *testing.T) {
	fixture := setupSeparatedRestoreSmokeTwoSavePointFixture(t)

	preview, _ := separatedRestoreSmokeJSON(t, fixture.cleanCWD, fixture.sourceControl, fixture.sourcePayload, "main", "restore", fixture.v1)
	planID, _ := preview["plan_id"].(string)
	runCommand, _ := preview["run_command"].(string)
	if preview["mode"] != "preview" || preview["source_save_point"] != fixture.v1 || planID == "" || runCommand == "" {
		t.Fatalf("restore preview should expose runnable command for v1: %#v", preview)
	}

	stdout, stderr, code := runSeparatedRestoreSmokePublishedCommand(t, fixture.cleanCWD, runCommand)
	if code != 0 {
		t.Fatalf("restore preview run_command should run from clean cwd\ncommand=%q\nstdout=%s\nstderr=%s", runCommand, stdout, stderr)
	}
	requireSeparatedRestoreSmokePayloadV1(t, fixture.sourcePayload)
	requireSeparatedRestoreSmokePayloadHasNoControlMetadata(t, fixture.sourcePayload)

	recovery, _ := separatedRestoreSmokeJSON(t, fixture.cleanCWD, fixture.sourceControl, fixture.sourcePayload, "main", "recovery", "status")
	requireSeparatedRestoreSmokeNoActiveRecovery(t, recovery)
	requireSeparatedRestoreSmokeNoRestoreState(t, recovery)
	sourceDoctor, _ := separatedRestoreSmokeDoctorJSON(t, fixture.cleanCWD, fixture.sourceControl, fixture.sourcePayload, "main", "--strict")
	if sourceDoctor["healthy"] != true {
		t.Fatalf("doctor strict should be healthy after published run_command: %#v", sourceDoctor)
	}
}

func TestStorySeparatedRestoreStalePreviewRecoveryCommandCleanupAndCloneConsistency(t *testing.T) {
	fixture := setupSeparatedRestoreSmokeTwoSavePointFixture(t)

	firstPreview, _ := separatedRestoreSmokeJSON(t, fixture.cleanCWD, fixture.sourceControl, fixture.sourcePayload, "main", "restore", fixture.v1)
	firstPlanID, _ := firstPreview["plan_id"].(string)
	if firstPreview["mode"] != "preview" || firstPreview["source_save_point"] != fixture.v1 || firstPlanID == "" {
		t.Fatalf("restore preview should create the initial v1 plan: %#v", firstPreview)
	}
	restored, _ := separatedRestoreSmokeJSON(t, fixture.cleanCWD, fixture.sourceControl, fixture.sourcePayload, "main", "restore", "--run", firstPlanID)
	if restored["mode"] != "run" || restored["restored_save_point"] != fixture.v1 {
		t.Fatalf("initial restore run should move content to v1: %#v", restored)
	}
	requireSeparatedRestoreSmokePayloadV1(t, fixture.sourcePayload)
	requireSeparatedRestoreSmokePayloadHasNoControlMetadata(t, fixture.sourcePayload)

	stalePreview, _ := separatedRestoreSmokeJSON(t, fixture.cleanCWD, fixture.sourceControl, fixture.sourcePayload, "main", "restore", fixture.v2)
	stalePlanID, _ := stalePreview["plan_id"].(string)
	if stalePreview["mode"] != "preview" || stalePreview["source_save_point"] != fixture.v2 || stalePlanID == "" {
		t.Fatalf("stale restore preview should create a pending v2 plan: %#v", stalePreview)
	}
	requireSeparatedRestoreSmokePayloadV1(t, fixture.sourcePayload)
	requireSeparatedRestoreSmokePayloadHasNoControlMetadata(t, fixture.sourcePayload)
	createFiles(t, fixture.sourcePayload, map[string]string{
		"README.md": "operator local change after stale preview\n",
	})

	staleRecovery, _ := separatedRestoreSmokeJSON(t, fixture.cleanCWD, fixture.sourceControl, fixture.sourcePayload, "main", "recovery", "status")
	requireSeparatedRestoreSmokeNoActiveRecovery(t, staleRecovery)
	staleState := requireSeparatedRestoreSmokeStaleRestoreState(t, staleRecovery, stalePlanID)
	recommendedCommand, _ := staleState["recommended_next_command"].(string)
	if recommendedCommand == "" {
		t.Fatalf("stale restore state missing recommended_next_command: %#v", staleState)
	}

	pendingDoctor, _ := separatedRestoreSmokeDoctorJSONAllowExit(t, fixture.cleanCWD, fixture.sourceControl, fixture.sourcePayload, "main", "--strict")
	if pendingDoctor["healthy"] != false {
		t.Fatalf("doctor strict should report stale restore preview unhealthy: %#v", pendingDoctor)
	}
	requireSeparatedRestoreSmokeDoctorCheck(t, pendingDoctor, "recovery_state", "E_RECOVERY_BLOCKING")

	cloneTargetControl := filepath.Join(fixture.base, "blocked-target-control")
	cloneTargetPayload := filepath.Join(fixture.base, "blocked-target-payload")
	stdout, stderr, code := runJVS(t, fixture.cleanCWD,
		"--json",
		"--control-root", fixture.sourceControl,
		"--workspace", "main",
		"repo", "clone",
		cloneTargetPayload,
		"--target-control-root", cloneTargetControl,
		"--save-points", "main",
	)
	env := requireSeparatedControlJSONError(t, stdout, stderr, code, "E_RECOVERY_BLOCKING")
	if !strings.Contains(env.Error.Message, stalePlanID) || !strings.Contains(env.Error.Hint, "restore discard "+stalePlanID) {
		t.Fatalf("clone blocker should name stale restore discard command: %#v", env.Error)
	}
	requireAbsolutePathAbsent(t, filepath.Join(cloneTargetControl, ".jvs"))
	requireAbsolutePathAbsent(t, filepath.Join(cloneTargetPayload, ".jvs"))

	stdout, stderr, code = runSeparatedRestoreSmokePublishedCommand(t, fixture.cleanCWD, recommendedCommand)
	if code != 0 {
		t.Fatalf("recovery recommended_next_command should discard stale restore preview from clean cwd\ncommand=%q\nstdout=%s\nstderr=%s", recommendedCommand, stdout, stderr)
	}
	if got := readAbsoluteFile(t, filepath.Join(fixture.sourcePayload, "README.md")); got != "operator local change after stale preview\n" {
		t.Fatalf("restore discard should leave local workspace content unchanged, got README.md=%q", got)
	}
	requireSeparatedRestoreSmokePayloadHasNoControlMetadata(t, fixture.sourcePayload)

	recovered, _ := separatedRestoreSmokeJSON(t, fixture.cleanCWD, fixture.sourceControl, fixture.sourcePayload, "main", "recovery", "status")
	requireSeparatedRestoreSmokeNoActiveRecovery(t, recovered)
	requireSeparatedRestoreSmokeNoRestoreState(t, recovered)
	recoveredDoctor, _ := separatedRestoreSmokeDoctorJSON(t, fixture.cleanCWD, fixture.sourceControl, fixture.sourcePayload, "main", "--strict")
	if recoveredDoctor["healthy"] != true {
		t.Fatalf("doctor strict should be healthy after stale preview cleanup: %#v", recoveredDoctor)
	}

	finalPreview, _ := separatedRestoreSmokeJSON(t, fixture.cleanCWD, fixture.sourceControl, fixture.sourcePayload, "main", "restore", fixture.v2, "--discard-unsaved")
	finalPlanID, _ := finalPreview["plan_id"].(string)
	if finalPreview["mode"] != "preview" || finalPreview["source_save_point"] != fixture.v2 || finalPlanID == "" {
		t.Fatalf("final restore preview should create a v2 plan after stale discard: %#v", finalPreview)
	}
	finalRun, _ := separatedRestoreSmokeJSON(t, fixture.cleanCWD, fixture.sourceControl, fixture.sourcePayload, "main", "restore", "--run", finalPlanID)
	if finalRun["mode"] != "run" || finalRun["restored_save_point"] != fixture.v2 {
		t.Fatalf("final restore run should move content to v2 after stale discard: %#v", finalRun)
	}
	requireSeparatedRestoreSmokePayloadV2(t, fixture.sourcePayload)

	finalTargetControl := filepath.Join(fixture.base, "final-target-control")
	finalTargetPayload := filepath.Join(fixture.base, "final-target-payload")
	stdout, stderr, code = runJVS(t, fixture.cleanCWD,
		"--json",
		"--control-root", fixture.sourceControl,
		"--workspace", "main",
		"repo", "clone",
		finalTargetPayload,
		"--target-control-root", finalTargetControl,
		"--save-points", "main",
	)
	if code != 0 {
		t.Fatalf("clone after stale restore cleanup failed: stdout=%s stderr=%s", stdout, stderr)
	}
	clone := requireSeparatedCloneExternalControlRootJSON(t, stdout, stderr, finalTargetControl, finalTargetPayload)
	requireSeparatedCloneTransfers(t, clone, fixture.sourceControl, fixture.sourcePayload, finalTargetControl, finalTargetPayload)
	requireSeparatedRestoreSmokePayloadV2(t, finalTargetPayload)
	requireSeparatedRestoreSmokePayloadHasNoControlMetadata(t, finalTargetPayload)
	requireSeparatedRestoreSmokeControlMarkers(t, fixture.sourceControl)
}

func TestStorySeparatedRestoreMalformedStateBlocksRecoveryDoctorAndCloneWithoutControlPlaneLeak(t *testing.T) {
	fixture := setupSeparatedRestoreSmokeTwoSavePointFixture(t)

	preview, _ := separatedRestoreSmokeJSON(t, fixture.cleanCWD, fixture.sourceControl, fixture.sourcePayload, "main", "restore", fixture.v1)
	planID, _ := preview["plan_id"].(string)
	if preview["mode"] != "preview" || preview["source_save_point"] != fixture.v1 || planID == "" {
		t.Fatalf("restore preview should create a real pending plan for v1: %#v", preview)
	}
	createFiles(t, filepath.Join(fixture.sourceControl, ".jvs", "restore-plans"), map[string]string{
		"zzzz-corrupt.json": "{not-json\n",
	})

	wantDoctorCommand := "jvs --control-root " + fixture.sourceControl + " --workspace main doctor --strict --json"
	statusOut, statusErr, statusCode := runJVS(t, fixture.cleanCWD,
		"--json",
		"--control-root", fixture.sourceControl,
		"--workspace", "main",
		"recovery", "status",
	)
	statusEnv := requireSeparatedControlJSONError(t, statusOut, statusErr, statusCode, "E_RECOVERY_BLOCKING")
	if !strings.Contains(statusEnv.Error.Message, "Restore plan zzzz-corrupt") || !strings.Contains(statusEnv.Error.Message, "not valid JSON") {
		t.Fatalf("recovery status should fail closed on malformed restore state: %#v", statusEnv.Error)
	}
	if !strings.Contains(statusEnv.Error.Hint, wantDoctorCommand) {
		t.Fatalf("recovery status hint should use full clean-CWD selector %q: %#v", wantDoctorCommand, statusEnv.Error)
	}
	requireSeparatedRestoreMalformedNoControlLeak(t, statusOut, statusErr)

	doctorOut, doctorErr, doctorCode := runJVS(t, fixture.cleanCWD,
		"--json",
		"--control-root", fixture.sourceControl,
		"--workspace", "main",
		"doctor", "--strict",
	)
	if doctorCode != 1 {
		t.Fatalf("doctor strict should exit unhealthy with code 1, got %d\nstdout=%s\nstderr=%s", doctorCode, doctorOut, doctorErr)
	}
	doctorEnv := requirePureJSONEnvelope(t, doctorOut, doctorErr, true)
	if doctorEnv.RepoRoot == nil || *doctorEnv.RepoRoot != fixture.sourceControl {
		t.Fatalf("doctor repo_root = %#v, want %q\n%s", doctorEnv.RepoRoot, fixture.sourceControl, doctorOut)
	}
	doctor := decodeContractDataMap(t, doctorOut)
	requireSeparatedControlDoctorData(t, doctor, fixture.sourceControl, fixture.sourcePayload, "main")
	if doctor["healthy"] != false {
		t.Fatalf("doctor strict should report malformed restore state unhealthy: %#v", doctor)
	}
	recoveryCheck := requireSeparatedRestoreMalformedDoctorCheck(t, doctor)
	if recoveryCheck["recommended_next_command"] != wantDoctorCommand {
		t.Fatalf("doctor recovery_state recommended command = %#v, want %q in %#v", recoveryCheck["recommended_next_command"], wantDoctorCommand, recoveryCheck)
	}
	requireSeparatedRestoreMalformedNoControlLeak(t, doctorOut, doctorErr)

	targetControl := filepath.Join(fixture.base, "malformed-target-control")
	targetPayload := filepath.Join(fixture.base, "malformed-target-payload")
	cloneOut, cloneErr, cloneCode := runJVS(t, fixture.cleanCWD,
		"--json",
		"--control-root", fixture.sourceControl,
		"--workspace", "main",
		"repo", "clone",
		targetPayload,
		"--target-control-root", targetControl,
		"--save-points", "main",
	)
	cloneEnv := requireSeparatedControlJSONError(t, cloneOut, cloneErr, cloneCode, "E_RECOVERY_BLOCKING")
	if !strings.Contains(cloneEnv.Error.Message, "restore plan zzzz-corrupt") {
		t.Fatalf("clone should block on malformed restore state: %#v", cloneEnv.Error)
	}
	if !strings.Contains(cloneEnv.Error.Hint, wantDoctorCommand) {
		t.Fatalf("clone hint should use full clean-CWD selector %q: %#v", wantDoctorCommand, cloneEnv.Error)
	}
	requireSeparatedRestoreMalformedNoControlLeak(t, cloneOut, cloneErr)
	requireAbsolutePathAbsent(t, targetControl)
	requireAbsolutePathAbsent(t, targetPayload)
	requireSeparatedRestoreSmokePayloadHasNoControlMetadata(t, fixture.sourcePayload)
}

type separatedRestoreSmokeTwoSavePointFixture struct {
	base          string
	cleanCWD      string
	sourceControl string
	sourcePayload string
	v1            string
	v2            string
}

func setupSeparatedRestoreSmokeTwoSavePointFixture(t *testing.T) separatedRestoreSmokeTwoSavePointFixture {
	t.Helper()

	base := t.TempDir()
	cleanCWD := filepath.Join(base, "operator-cwd")
	if err := os.MkdirAll(cleanCWD, 0755); err != nil {
		t.Fatalf("create clean cwd: %v", err)
	}

	sourceControl := filepath.Join(base, "source-control")
	sourcePayload := filepath.Join(base, "source-payload")
	initData := separatedRestoreSmokeInit(t, cleanCWD, sourceControl, sourcePayload, "main")
	requireSeparatedControlSetupFields(t, initData, sourcePayload)
	seedSeparatedRestoreSmokeControlMarkers(t, sourceControl)
	requireSeparatedRestoreSmokePayloadHasNoControlMetadata(t, sourcePayload)

	createFiles(t, sourcePayload, map[string]string{
		"README.md":               "payload v1\n",
		"repo_id":                 "user payload file named repo_id v1\n",
		"runtime/user-cache.txt":  "user payload runtime data v1\n",
		"snapshots/user-note.txt": "user payload snapshot notes v1\n",
		"src/app.js":              "console.log('v1')\n",
		"views/user-facing.txt":   "user payload view data v1\n",
	})
	firstSave, _ := separatedRestoreSmokeJSON(t, cleanCWD, sourceControl, sourcePayload, "main", "save", "-m", "payload v1")
	v1, _ := firstSave["save_point_id"].(string)
	if v1 == "" {
		t.Fatalf("first save missing save_point_id: %#v", firstSave)
	}
	requireSeparatedRestoreSmokePayloadHasNoControlMetadata(t, sourcePayload)

	createFiles(t, sourcePayload, map[string]string{
		"README.md":               "payload v2\n",
		"repo_id":                 "user payload file named repo_id v2\n",
		"runtime/user-cache.txt":  "user payload runtime data v2\n",
		"snapshots/user-note.txt": "user payload snapshot notes v2\n",
		"src/app.js":              "console.log('v2')\n",
		"tmp/generated.txt":       "v2 only\n",
		"views/user-facing.txt":   "user payload view data v2\n",
	})
	secondSave, _ := separatedRestoreSmokeJSON(t, cleanCWD, sourceControl, sourcePayload, "main", "save", "-m", "payload v2")
	v2, _ := secondSave["save_point_id"].(string)
	requireDifferentSavePoints(t, v1, v2)
	requireSeparatedRestoreSmokePayloadHasNoControlMetadata(t, sourcePayload)

	return separatedRestoreSmokeTwoSavePointFixture{
		base:          base,
		cleanCWD:      cleanCWD,
		sourceControl: sourceControl,
		sourcePayload: sourcePayload,
		v1:            v1,
		v2:            v2,
	}
}

func separatedRestoreSmokeInit(t *testing.T, cwd, controlRoot, payloadRoot, workspace string) map[string]any {
	t.Helper()
	stdout, stderr, code := runJVS(t, cwd,
		"init",
		payloadRoot,
		"--control-root", controlRoot,
		"--workspace", workspace,
		"--json",
	)
	if code != 0 {
		t.Fatalf("separated restore smoke init failed: stdout=%s stderr=%s", stdout, stderr)
	}
	requireNoPayloadRootJSONVocabulary(t, stdout)
	return requireSeparatedControlAuthoritativeJSON(t, stdout, stderr, controlRoot, payloadRoot, workspace)
}

func separatedRestoreSmokeJSON(t *testing.T, cwd, controlRoot, workspaceFolder, workspace string, args ...string) (map[string]any, string) {
	t.Helper()
	fullArgs := []string{"--json", "--control-root", controlRoot, "--workspace", workspace}
	fullArgs = append(fullArgs, args...)
	stdout, stderr, code := runJVS(t, cwd, fullArgs...)
	if code != 0 {
		t.Fatalf("jvs %v failed\nstdout=%s\nstderr=%s", fullArgs, stdout, stderr)
	}
	requireNoPayloadRootJSONVocabulary(t, stdout)
	return requireSeparatedControlAuthoritativeJSON(t, stdout, stderr, controlRoot, workspaceFolder, workspace), stdout
}

func separatedRestoreSmokeDoctorJSON(t *testing.T, cwd, controlRoot, workspaceFolder, workspace string, args ...string) (map[string]any, string) {
	t.Helper()
	fullArgs := []string{"--json", "--control-root", controlRoot, "--workspace", workspace, "doctor"}
	fullArgs = append(fullArgs, args...)
	stdout, stderr, code := runJVS(t, cwd, fullArgs...)
	if code != 0 {
		t.Fatalf("jvs %v failed\nstdout=%s\nstderr=%s", fullArgs, stdout, stderr)
	}
	requireNoPayloadRootJSONVocabulary(t, stdout)
	env := requirePureJSONEnvelope(t, stdout, stderr, true)
	if env.RepoRoot == nil || *env.RepoRoot != controlRoot {
		t.Fatalf("separated doctor JSON repo_root = %#v, want %q\n%s", env.RepoRoot, controlRoot, stdout)
	}
	data := decodeContractDataMap(t, stdout)
	requireSeparatedControlDoctorData(t, data, controlRoot, workspaceFolder, workspace)
	return data, stdout
}

func separatedRestoreSmokeDoctorJSONAllowExit(t *testing.T, cwd, controlRoot, workspaceFolder, workspace string, args ...string) (map[string]any, string) {
	t.Helper()
	fullArgs := []string{"--json", "--control-root", controlRoot, "--workspace", workspace, "doctor"}
	fullArgs = append(fullArgs, args...)
	stdout, stderr, code := runJVS(t, cwd, fullArgs...)
	if code != 0 && code != 1 {
		t.Fatalf("jvs %v failed with unexpected exit code %d\nstdout=%s\nstderr=%s", fullArgs, code, stdout, stderr)
	}
	requireNoPayloadRootJSONVocabulary(t, stdout)
	env := requirePureJSONEnvelope(t, stdout, stderr, true)
	if env.RepoRoot == nil || *env.RepoRoot != controlRoot {
		t.Fatalf("separated doctor JSON repo_root = %#v, want %q\n%s", env.RepoRoot, controlRoot, stdout)
	}
	data := decodeContractDataMap(t, stdout)
	requireSeparatedControlDoctorData(t, data, controlRoot, workspaceFolder, workspace)
	return data, stdout
}

func requireNoPayloadRootJSONVocabulary(t *testing.T, stdout string) {
	t.Helper()
	for _, forbidden := range []string{`"payload_root"`, `"source_payload_root"`, `"target_payload_root"`} {
		if strings.Contains(stdout, forbidden) {
			t.Fatalf("public JSON exposes old payload-root vocabulary %s:\n%s", forbidden, stdout)
		}
	}
}

func requireSeparatedRestoreSmokePayloadUnchanged(t *testing.T, before separatedLifecycleStoryRootSnapshot, payloadRoot string) {
	t.Helper()
	after := captureSeparatedLifecycleStoryRoot(t, payloadRoot)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("restore preview mutated payload tree\nbefore=%#v\nafter=%#v", before, after)
	}
}

func requireSeparatedRestoreSmokePayloadV1(t *testing.T, payloadRoot string) {
	t.Helper()
	for rel, want := range map[string]string{
		"README.md":               "payload v1\n",
		"repo_id":                 "user payload file named repo_id v1\n",
		"runtime/user-cache.txt":  "user payload runtime data v1\n",
		"snapshots/user-note.txt": "user payload snapshot notes v1\n",
		"src/app.js":              "console.log('v1')\n",
		"views/user-facing.txt":   "user payload view data v1\n",
	} {
		if got := readAbsoluteFile(t, filepath.Join(payloadRoot, filepath.FromSlash(rel))); got != want {
			t.Fatalf("%s %s = %q, want %q", payloadRoot, rel, got, want)
		}
	}
	requireAbsolutePathAbsent(t, filepath.Join(payloadRoot, "tmp", "generated.txt"))
}

func requireSeparatedRestoreSmokePayloadV2(t *testing.T, payloadRoot string) {
	t.Helper()
	for rel, want := range map[string]string{
		"README.md":               "payload v2\n",
		"repo_id":                 "user payload file named repo_id v2\n",
		"runtime/user-cache.txt":  "user payload runtime data v2\n",
		"snapshots/user-note.txt": "user payload snapshot notes v2\n",
		"src/app.js":              "console.log('v2')\n",
		"tmp/generated.txt":       "v2 only\n",
		"views/user-facing.txt":   "user payload view data v2\n",
	} {
		if got := readAbsoluteFile(t, filepath.Join(payloadRoot, filepath.FromSlash(rel))); got != want {
			t.Fatalf("%s %s = %q, want %q", payloadRoot, rel, got, want)
		}
	}
}

func seedSeparatedRestoreSmokeControlMarkers(t *testing.T, controlRoot string) {
	t.Helper()
	for rel, content := range map[string]string{
		filepath.ToSlash(filepath.Join("audit", separatedRestoreSmokeAuditSentinelName)):   separatedRestoreSmokeAuditSentinelContent,
		filepath.ToSlash(filepath.Join("runtime", separatedRestoreSmokeRuntimeMarkerName)): separatedRestoreSmokeRuntimeMarkerContent,
	} {
		path := filepath.Join(controlRoot, ".jvs", filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("create control marker directory for %s: %v", rel, err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("write control marker %s: %v", rel, err)
		}
	}
}

func requireSeparatedRestoreSmokeControlMarkers(t *testing.T, controlRoot string) {
	t.Helper()
	for rel, want := range map[string]string{
		filepath.ToSlash(filepath.Join("audit", separatedRestoreSmokeAuditSentinelName)):   separatedRestoreSmokeAuditSentinelContent,
		filepath.ToSlash(filepath.Join("runtime", separatedRestoreSmokeRuntimeMarkerName)): separatedRestoreSmokeRuntimeMarkerContent,
	} {
		if got := readAbsoluteFile(t, filepath.Join(controlRoot, ".jvs", filepath.FromSlash(rel))); got != want {
			t.Fatalf("control marker %s = %q, want %q", rel, got, want)
		}
	}
}

func requireSeparatedRestoreSmokePayloadHasNoControlMetadata(t *testing.T, payloadRoot string) {
	t.Helper()
	requireAbsolutePathAbsent(t, filepath.Join(payloadRoot, ".jvs"))
	forbiddenNames := map[string]string{
		separatedRestoreSmokeAuditSentinelName: "audit sentinel",
		separatedRestoreSmokeRuntimeMarkerName: "runtime marker",
	}
	forbiddenContent := map[string]string{
		separatedRestoreSmokeAuditSentinelContent: "audit sentinel",
		separatedRestoreSmokeRuntimeMarkerContent: "runtime marker",
	}
	if err := filepath.WalkDir(payloadRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if reason, ok := forbiddenNames[entry.Name()]; ok {
			t.Fatalf("payload exposes control %s %q at %s", reason, entry.Name(), path)
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		content := string(data)
		for forbidden, reason := range forbiddenContent {
			if strings.Contains(content, forbidden) {
				t.Fatalf("payload exposes control %s content at %s", reason, path)
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("walk payload %s: %v", payloadRoot, err)
	}
}

func jsonStringArrayOfObjectsLen(t *testing.T, raw any) int {
	t.Helper()
	items, ok := raw.([]any)
	if !ok {
		t.Fatalf("value should be a JSON array: %#v", raw)
	}
	for _, item := range items {
		if _, ok := item.(map[string]any); !ok {
			t.Fatalf("array item should be an object: %#v", item)
		}
	}
	return len(items)
}

func requireSeparatedRestoreSmokeNoActiveRecovery(t *testing.T, recovery map[string]any) {
	t.Helper()
	if recovery["mode"] != "status" {
		t.Fatalf("recovery status mode = %#v, want status in %#v", recovery["mode"], recovery)
	}
	if plans := jsonStringArrayOfObjectsLen(t, recovery["plans"]); plans != 0 {
		t.Fatalf("recovery status active plans count = %d, want 0 in %#v", plans, recovery["plans"])
	}
}

func requireSeparatedRestoreSmokeNoRestoreState(t *testing.T, recovery map[string]any) {
	t.Helper()
	if _, ok := recovery["restore_state"]; ok {
		t.Fatalf("recovery status should not report restore_state after cleanup: %#v", recovery["restore_state"])
	}
}

func requireSeparatedRestoreSmokePendingRestoreState(t *testing.T, recovery map[string]any, planID string) map[string]any {
	t.Helper()
	restoreState, ok := recovery["restore_state"].(map[string]any)
	if !ok {
		t.Fatalf("recovery status missing pending restore_state: %#v", recovery)
	}
	if restoreState["state"] != "pending_restore_preview" || restoreState["blocking"] != true || restoreState["plan_id"] != planID {
		t.Fatalf("pending restore_state mismatch, want plan %s: %#v", planID, restoreState)
	}
	recommended, _ := restoreState["recommended_next_command"].(string)
	if !strings.Contains(recommended, "restore --run "+planID) {
		t.Fatalf("pending restore_state recommended command should run plan %s: %#v", planID, restoreState)
	}
	return restoreState
}

func requireSeparatedRestoreSmokeStaleRestoreState(t *testing.T, recovery map[string]any, planID string) map[string]any {
	t.Helper()
	restoreState, ok := recovery["restore_state"].(map[string]any)
	if !ok {
		t.Fatalf("recovery status missing stale restore_state: %#v", recovery)
	}
	if restoreState["state"] != "stale_restore_preview" || restoreState["blocking"] != true || restoreState["plan_id"] != planID {
		t.Fatalf("stale restore_state mismatch, want plan %s: %#v", planID, restoreState)
	}
	recommended, _ := restoreState["recommended_next_command"].(string)
	if !strings.Contains(recommended, "restore discard "+planID) {
		t.Fatalf("stale restore_state recommended command should discard plan %s: %#v", planID, restoreState)
	}
	return restoreState
}

func requireSeparatedRestoreSmokeDoctorCheck(t *testing.T, doctorData map[string]any, name, wantCode string) {
	t.Helper()
	checks, ok := doctorData["checks"].([]any)
	if !ok {
		t.Fatalf("doctor checks should be an array: %#v", doctorData["checks"])
	}
	for _, raw := range checks {
		check, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("doctor check should be an object: %#v", raw)
		}
		if check["name"] != name {
			continue
		}
		if check["status"] != "failed" || check["error_code"] != wantCode {
			t.Fatalf("doctor check %s = %#v, want failed %s", name, check, wantCode)
		}
		return
	}
	t.Fatalf("doctor checks missing %s: %#v", name, checks)
}

func requireSeparatedRestoreMalformedDoctorCheck(t *testing.T, doctorData map[string]any) map[string]any {
	t.Helper()
	checks, ok := doctorData["checks"].([]any)
	if !ok {
		t.Fatalf("doctor checks should be an array: %#v", doctorData["checks"])
	}
	for _, raw := range checks {
		check, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("doctor check should be an object: %#v", raw)
		}
		if check["name"] != "recovery_state" {
			continue
		}
		if check["status"] != "failed" || check["error_code"] != "E_RECOVERY_BLOCKING" {
			t.Fatalf("doctor recovery_state check = %#v, want failed E_RECOVERY_BLOCKING", check)
		}
		message, _ := check["message"].(string)
		if !strings.Contains(message, "Restore plan zzzz-corrupt") || !strings.Contains(message, "not valid JSON") {
			t.Fatalf("doctor recovery_state message should explain malformed restore state: %#v", check)
		}
		return check
	}
	t.Fatalf("doctor checks missing recovery_state: %#v", checks)
	return nil
}

func requireSeparatedRestoreMalformedNoControlLeak(t *testing.T, outputs ...string) {
	t.Helper()
	combined := strings.Join(outputs, "\n")
	for _, forbidden := range []string{
		".jvs/restore-plans",
		".jvs/recovery-plans",
		"payload_root",
		"source_payload_root",
		"target_payload_root",
	} {
		if strings.Contains(combined, forbidden) {
			t.Fatalf("public output exposes control-plane detail %q:\n%s", forbidden, combined)
		}
	}
}

func runSeparatedRestoreSmokePublishedCommand(t *testing.T, cwd, command string) (stdout, stderr string, exitCode int) {
	t.Helper()
	if strings.TrimSpace(command) == "" {
		t.Fatalf("published command is empty")
	}
	binDir := t.TempDir()
	jvsOnPath := filepath.Join(binDir, "jvs")
	if err := os.Symlink(jvsBinary, jvsOnPath); err != nil {
		wrapper := "#!/bin/sh\nexec " + separatedRestoreSmokeShellQuote(jvsBinary) + " \"$@\"\n"
		if writeErr := os.WriteFile(jvsOnPath, []byte(wrapper), 0755); writeErr != nil {
			t.Fatalf("create jvs command shim after symlink error %v: %v", err, writeErr)
		}
	}

	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err := cmd.Run()
	stdout = stdoutBuf.String()
	stderr = stderrBuf.String()
	if err == nil {
		return stdout, stderr, 0
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return stdout, stderr, exitErr.ExitCode()
	}
	return stdout, stderr, 1
}

func separatedRestoreSmokeShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
