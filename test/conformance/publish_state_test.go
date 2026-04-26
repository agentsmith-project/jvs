//go:build conformance

package conformance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPublishStateConformanceMatrix(t *testing.T) {
	cases := []struct {
		name          string
		wantCode      string
		mutate        func(t *testing.T, repoPath, checkpointID string)
		visible       bool
		expectList    bool
		repairMayPass bool
	}{
		{
			name:       "missing_ready",
			wantCode:   "E_READY_MISSING",
			expectList: true,
			mutate: func(t *testing.T, repoPath, checkpointID string) {
				t.Helper()
				requireRemovePublishStatePath(t, filepath.Join(repoPath, ".jvs", "snapshots", checkpointID, ".READY"))
			},
		},
		{
			name:       "invalid_ready_leaf",
			wantCode:   "E_READY_INVALID",
			expectList: false,
			mutate: func(t *testing.T, repoPath, checkpointID string) {
				t.Helper()
				readyPath := filepath.Join(repoPath, ".jvs", "snapshots", checkpointID, ".READY")
				requireRemovePublishStatePath(t, readyPath)
				if err := os.Mkdir(readyPath, 0755); err != nil {
					t.Fatalf("create invalid READY leaf: %v", err)
				}
			},
		},
		{
			name:       "malformed_ready_json",
			wantCode:   "E_READY_INVALID",
			expectList: false,
			mutate: func(t *testing.T, repoPath, checkpointID string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(repoPath, ".jvs", "snapshots", checkpointID, ".READY"), []byte("{not json"), 0644); err != nil {
					t.Fatalf("write malformed READY: %v", err)
				}
			},
		},
		{
			name:       "ready_field_mismatch",
			wantCode:   "E_READY_INVALID",
			expectList: false,
			mutate: func(t *testing.T, repoPath, checkpointID string) {
				t.Helper()
				mutatePublishStateReadyJSON(t, repoPath, checkpointID, func(marker map[string]any) {
					marker["payload_root_hash"] = "bad-ready-payload-hash"
				})
			},
		},
		{
			name:       "descriptor_invalid_json",
			wantCode:   "E_DESCRIPTOR_CORRUPT",
			expectList: true,
			mutate: func(t *testing.T, repoPath, checkpointID string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(repoPath, ".jvs", "descriptors", checkpointID+".json"), []byte("{not json"), 0644); err != nil {
					t.Fatalf("write malformed descriptor: %v", err)
				}
			},
		},
		{
			name:          "descriptor_checksum_mismatch",
			wantCode:      "E_DESCRIPTOR_CHECKSUM_MISMATCH",
			visible:       true,
			expectList:    true,
			repairMayPass: true,
			mutate: func(t *testing.T, repoPath, checkpointID string) {
				t.Helper()
				const badChecksum = "bad-descriptor-checksum"
				mutatePublishStateDescriptorJSON(t, repoPath, checkpointID, func(desc map[string]any) {
					desc["descriptor_checksum"] = badChecksum
				})
				mutatePublishStateReadyJSON(t, repoPath, checkpointID, func(marker map[string]any) {
					marker["descriptor_checksum"] = badChecksum
				})
			},
		},
		{
			name:       "ready_without_descriptor",
			wantCode:   "E_READY_DESCRIPTOR_MISSING",
			expectList: true,
			mutate: func(t *testing.T, repoPath, checkpointID string) {
				t.Helper()
				requireRemovePublishStatePath(t, filepath.Join(repoPath, ".jvs", "descriptors", checkpointID+".json"))
			},
		},
		{
			name:       "descriptor_without_payload",
			wantCode:   "E_PAYLOAD_MISSING",
			expectList: true,
			mutate: func(t *testing.T, repoPath, checkpointID string) {
				t.Helper()
				if err := os.RemoveAll(filepath.Join(repoPath, ".jvs", "snapshots", checkpointID)); err != nil {
					t.Fatalf("remove checkpoint payload: %v", err)
				}
			},
		},
		{
			name:          "payload_mismatch",
			wantCode:      "E_PAYLOAD_HASH_MISMATCH",
			visible:       true,
			expectList:    true,
			repairMayPass: true,
			mutate: func(t *testing.T, repoPath, checkpointID string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(repoPath, ".jvs", "snapshots", checkpointID, "tampered.txt"), []byte("tampered"), 0644); err != nil {
					t.Fatalf("tamper checkpoint payload: %v", err)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repoPath, checkpointID := createPublishStateVictim(t, tc.name)
			tc.mutate(t, repoPath, checkpointID)
			runtimeTmp := addPublishStateRuntimeTmp(t, repoPath)
			addPublishStateRuntimeIntent(t, repoPath)

			if tc.expectList {
				listOut, stderr, code := runJVSInRepo(t, repoPath, "--json", "checkpoint", "list")
				if code != 0 {
					t.Fatalf("checkpoint list failed: stdout=%s stderr=%s", listOut, stderr)
				}
				if strings.TrimSpace(stderr) != "" {
					t.Fatalf("checkpoint list --json wrote stderr: %q", stderr)
				}
				assertPublishStateListVisibility(t, listOut, checkpointID, tc.visible)
			} else {
				listOut, stderr, code := runJVSInRepo(t, repoPath, "--json", "checkpoint", "list")
				if code == 0 && publishStateListContains(listOut, checkpointID) {
					t.Fatalf("checkpoint list exposed invalid publish state %s: %s", checkpointID, listOut)
				}
				if code != 0 && strings.TrimSpace(stderr) != "" {
					t.Fatalf("checkpoint list JSON error wrote stderr: %q", stderr)
				}
			}

			verifyOut, stderr, code := runJVSInRepo(t, repoPath, "--json", "verify", checkpointID)
			if code == 0 {
				t.Fatalf("verify accepted damaged publish state: stdout=%s stderr=%s", verifyOut, stderr)
			}
			if strings.TrimSpace(stderr) != "" {
				t.Fatalf("verify --json wrote stderr: %q", stderr)
			}
			requireJSONDataOrEnvelopeErrorCode(t, verifyOut, tc.wantCode)

			doctorOut, stderr, code := runJVSInRepo(t, repoPath, "--json", "doctor", "--strict")
			if code == 0 {
				t.Fatalf("doctor --strict accepted damaged publish state: stdout=%s stderr=%s", doctorOut, stderr)
			}
			if strings.TrimSpace(stderr) != "" {
				t.Fatalf("doctor --strict --json wrote stderr: %q", stderr)
			}
			requireDoctorFindingCode(t, doctorOut, tc.wantCode)

			repairOut, stderr, code := runJVSInRepo(t, repoPath, "--json", "doctor", "--repair-runtime")
			if code == 0 && !tc.repairMayPass {
				t.Fatalf("doctor --repair-runtime should remain unhealthy for final damage: stdout=%s stderr=%s", repairOut, stderr)
			}
			if strings.TrimSpace(stderr) != "" {
				t.Fatalf("doctor --repair-runtime --json wrote stderr: %q", stderr)
			}
			if fileExists(t, runtimeTmp) {
				t.Fatalf("doctor --repair-runtime did not clean runtime tmp: %s", repairOut)
			}
			assertPublishStateDamageRetained(t, repoPath, checkpointID, tc.name)

			gcPlanOut, stderr, code := runJVSInRepo(t, repoPath, "--json", "gc", "plan")
			if code == 0 {
				t.Fatalf("gc plan accepted damaged publish state: stdout=%s stderr=%s", gcPlanOut, stderr)
			}
			if strings.TrimSpace(stderr) != "" {
				t.Fatalf("gc plan --json wrote stderr: %q", stderr)
			}
			requireEnvelopeErrorCode(t, gcPlanOut, tc.wantCode)
			assertPublishStateDamageRetained(t, repoPath, checkpointID, tc.name)

			planID := writePublishStateGCPlan(t, repoPath, checkpointID)
			gcRunOut, stderr, code := runJVSInRepo(t, repoPath, "--json", "gc", "run", "--plan-id", planID)
			if code == 0 {
				t.Fatalf("gc run accepted damaged publish state: stdout=%s stderr=%s", gcRunOut, stderr)
			}
			if strings.TrimSpace(stderr) != "" {
				t.Fatalf("gc run --json wrote stderr: %q", stderr)
			}
			requireEnvelopeErrorCode(t, gcRunOut, tc.wantCode)
			assertPublishStateDamageRetained(t, repoPath, checkpointID, tc.name)

			restoreOut, stderr, code := runJVSInRepo(t, repoPath, "--json", "restore", checkpointID)
			if code == 0 {
				t.Fatalf("restore accepted damaged publish state: stdout=%s stderr=%s", restoreOut, stderr)
			}
			if strings.TrimSpace(stderr) != "" {
				t.Fatalf("restore --json wrote stderr: %q", stderr)
			}
			requireEnvelopeErrorCode(t, restoreOut, tc.wantCode)

			forkOut, stderr, code := runJVSInRepo(t, repoPath, "--json", "fork", checkpointID, "bad-fork")
			if code == 0 {
				t.Fatalf("fork accepted damaged publish state: stdout=%s stderr=%s", forkOut, stderr)
			}
			if strings.TrimSpace(stderr) != "" {
				t.Fatalf("fork --json wrote stderr: %q", stderr)
			}
			requireEnvelopeErrorCode(t, forkOut, tc.wantCode)
			if fileExists(t, filepath.Join(repoPath, "worktrees", "bad-fork")) {
				t.Fatalf("fork created workspace from damaged publish state: %s", forkOut)
			}

			fullClone := filepath.Join(filepath.Dir(repoPath), tc.name+"-full-clone")
			cloneOut, stderr, code := runJVS(t, filepath.Dir(repoPath), "--json", "clone", repoPath, fullClone, "--scope", "full")
			if code == 0 {
				t.Fatalf("clone full accepted damaged publish state: stdout=%s stderr=%s", cloneOut, stderr)
			}
			if strings.TrimSpace(stderr) != "" {
				t.Fatalf("clone full --json wrote stderr: %q", stderr)
			}
			requireEnvelopeErrorCode(t, cloneOut, tc.wantCode)
			if _, err := os.Stat(fullClone); !os.IsNotExist(err) {
				t.Fatalf("clone full created destination for damaged source: %v", err)
			}
		})
	}
}

func TestCheckpointListMalformedReadyUsesStablePublishStateCode(t *testing.T) {
	repoPath, checkpointID := createPublishStateVictim(t, "list-malformed-ready-code")
	if err := os.WriteFile(filepath.Join(repoPath, ".jvs", "snapshots", checkpointID, ".READY"), []byte("{not json"), 0644); err != nil {
		t.Fatalf("write malformed READY: %v", err)
	}

	stdout, stderr, code := runJVSInRepo(t, repoPath, "--json", "checkpoint", "list")
	if code == 0 {
		t.Fatalf("checkpoint list accepted malformed READY: stdout=%s stderr=%s", stdout, stderr)
	}
	if strings.TrimSpace(stderr) != "" {
		t.Fatalf("checkpoint list --json wrote stderr: %q", stderr)
	}
	requireEnvelopeErrorCode(t, stdout, "E_READY_INVALID")
}

func createPublishStateVictim(t *testing.T, name string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	repoName := "publish-state-" + name
	if stdout, stderr, code := runJVS(t, dir, "init", repoName); code != 0 {
		t.Fatalf("init failed: stdout=%s stderr=%s", stdout, stderr)
	}
	repoPath := filepath.Join(dir, repoName)
	mainPath := filepath.Join(repoPath, "main")
	if err := os.WriteFile(filepath.Join(mainPath, "base.txt"), []byte("base"), 0644); err != nil {
		t.Fatalf("write base: %v", err)
	}
	if stdout, stderr, code := runJVSInRepo(t, repoPath, "--json", "checkpoint", "base"); code != 0 {
		t.Fatalf("base checkpoint failed: stdout=%s stderr=%s", stdout, stderr)
	}
	if err := os.WriteFile(filepath.Join(mainPath, "victim.txt"), []byte(name), 0644); err != nil {
		t.Fatalf("write victim: %v", err)
	}
	checkpointOut, stderr, code := runJVSInRepo(t, repoPath, "--json", "checkpoint", "victim "+name)
	if code != 0 {
		t.Fatalf("victim checkpoint failed: stdout=%s stderr=%s", checkpointOut, stderr)
	}
	checkpointID, _ := decodeContractSmokeDataMap(t, checkpointOut)["checkpoint_id"].(string)
	if checkpointID == "" {
		t.Fatalf("victim checkpoint output missing checkpoint_id: %s", checkpointOut)
	}
	if err := os.WriteFile(filepath.Join(mainPath, "healthy.txt"), []byte("healthy current"), 0644); err != nil {
		t.Fatalf("write healthy current: %v", err)
	}
	if stdout, stderr, code := runJVSInRepo(t, repoPath, "--json", "checkpoint", "healthy current"); code != 0 {
		t.Fatalf("healthy current checkpoint failed: stdout=%s stderr=%s", stdout, stderr)
	}
	return repoPath, checkpointID
}

func addPublishStateRuntimeTmp(t *testing.T, repoPath string) string {
	t.Helper()
	tmpPath := filepath.Join(repoPath, ".jvs", "snapshots", "runtime-crash.tmp")
	if err := os.MkdirAll(tmpPath, 0755); err != nil {
		t.Fatalf("create runtime tmp: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpPath, "partial.txt"), []byte("partial"), 0644); err != nil {
		t.Fatalf("write runtime tmp: %v", err)
	}
	return tmpPath
}

func addPublishStateRuntimeIntent(t *testing.T, repoPath string) {
	t.Helper()
	intentsDir := filepath.Join(repoPath, ".jvs", "intents")
	if err := os.MkdirAll(intentsDir, 0755); err != nil {
		t.Fatalf("create intents dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(intentsDir, "stale.json"), []byte(`{"status":"crashed"}`), 0644); err != nil {
		t.Fatalf("write runtime intent: %v", err)
	}
}

func assertPublishStateListVisibility(t *testing.T, stdout, checkpointID string, visible bool) {
	t.Helper()
	got := publishStateListContains(stdout, checkpointID)
	if got != visible {
		t.Fatalf("checkpoint list visibility for %s = %t, want %t: %s", checkpointID, got, visible, stdout)
	}
}

func publishStateListContains(stdout, checkpointID string) bool {
	for _, id := range extractAllSnapshotIDs(stdout) {
		if id == checkpointID {
			return true
		}
	}
	return false
}

func assertPublishStateDamageRetained(t *testing.T, repoPath, checkpointID, caseName string) {
	t.Helper()
	descriptorPath := filepath.Join(repoPath, ".jvs", "descriptors", checkpointID+".json")
	snapshotDir := filepath.Join(repoPath, ".jvs", "snapshots", checkpointID)

	switch caseName {
	case "descriptor_without_payload":
		if !fileExists(t, descriptorPath) {
			t.Fatalf("final damaged descriptor was deleted")
		}
	case "ready_without_descriptor":
		if !fileExists(t, snapshotDir) {
			t.Fatalf("final damaged checkpoint payload was deleted")
		}
	default:
		if !fileExists(t, descriptorPath) {
			t.Fatalf("final damaged descriptor was deleted")
		}
		if !fileExists(t, snapshotDir) {
			t.Fatalf("final damaged checkpoint payload was deleted")
		}
	}
}

func writePublishStateGCPlan(t *testing.T, repoPath, checkpointID string) string {
	t.Helper()
	repoIDData, err := os.ReadFile(filepath.Join(repoPath, ".jvs", "repo_id"))
	if err != nil {
		t.Fatalf("read repo_id: %v", err)
	}
	planID := "publish-state-" + strings.ReplaceAll(checkpointID, "-", "")
	plan := map[string]any{
		"schema_version":  1,
		"repo_id":         strings.TrimSpace(string(repoIDData)),
		"plan_id":         planID,
		"created_at":      time.Now().UTC().Format(time.RFC3339Nano),
		"protected_set":   []string{},
		"candidate_count": 1,
		"to_delete":       []string{checkpointID},
	}
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		t.Fatalf("marshal gc plan: %v", err)
	}
	gcDir := filepath.Join(repoPath, ".jvs", "gc")
	if err := os.MkdirAll(gcDir, 0755); err != nil {
		t.Fatalf("create gc dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gcDir, planID+".json"), data, 0644); err != nil {
		t.Fatalf("write gc plan: %v", err)
	}
	return planID
}

func requireEnvelopeErrorCode(t *testing.T, stdout, want string) {
	t.Helper()
	env := decodeContractSmokeEnvelope(t, stdout)
	if env.OK {
		t.Fatalf("expected error envelope code %s, got ok: %s", want, stdout)
	}
	errData, ok := env.Error.(map[string]any)
	if !ok || errData["code"] != want {
		t.Fatalf("expected error code %s, got %#v\n%s", want, env.Error, stdout)
	}
}

func mutatePublishStateReadyJSON(t *testing.T, repoPath, checkpointID string, mutate func(map[string]any)) {
	t.Helper()
	mutatePublishStateJSONFile(t, filepath.Join(repoPath, ".jvs", "snapshots", checkpointID, ".READY"), mutate)
}

func mutatePublishStateDescriptorJSON(t *testing.T, repoPath, checkpointID string, mutate func(map[string]any)) {
	t.Helper()
	mutatePublishStateJSONFile(t, filepath.Join(repoPath, ".jvs", "descriptors", checkpointID+".json"), mutate)
}

func mutatePublishStateJSONFile(t *testing.T, path string, mutate func(map[string]any)) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	mutate(doc)
	data, err = json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatalf("marshal %s: %v", path, err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func requireRemovePublishStatePath(t *testing.T, path string) {
	t.Helper()
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove %s: %v", path, err)
	}
}
