//go:build conformance

package conformance

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type storyCommandResult struct {
	stdout string
	stderr string
	code   int
}

type storyJSONResult struct {
	storyCommandResult
	env storyJSONEnvelope
}

type storyJSONEnvelope struct {
	SchemaVersion int             `json:"schema_version"`
	Command       string          `json:"command"`
	OK            bool            `json:"ok"`
	RepoRoot      *string         `json:"repo_root"`
	Workspace     *string         `json:"workspace"`
	Data          json.RawMessage `json:"data"`
	Error         *storyJSONError `json:"error"`
}

type storyJSONError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint"`
}

type storyCheckpointRecord struct {
	CheckpointID       string   `json:"checkpoint_id"`
	ParentCheckpointID string   `json:"parent_checkpoint_id"`
	Workspace          string   `json:"workspace"`
	Note               string   `json:"note"`
	Tags               []string `json:"tags"`
	Engine             string   `json:"engine"`
	EffectiveEngine    string   `json:"effective_engine"`
	PerformanceClass   string   `json:"performance_class"`
	IntegrityState     string   `json:"integrity_state"`
}

type storyWorkspaceStatus struct {
	Current       string   `json:"current"`
	Latest        string   `json:"latest"`
	Dirty         bool     `json:"dirty"`
	AtLatest      bool     `json:"at_latest"`
	Workspace     string   `json:"workspace"`
	Repo          string   `json:"repo"`
	Engine        string   `json:"engine"`
	RecoveryHints []string `json:"recovery_hints"`
}

type storyWorkspaceRecord struct {
	Workspace      string `json:"workspace"`
	BaseCheckpoint string `json:"base_checkpoint"`
	Current        string `json:"current"`
	Latest         string `json:"latest"`
}

type storyWorkspacePath struct {
	Workspace string `json:"workspace"`
	Path      string `json:"path"`
}

type storyDiffResult struct {
	FromCheckpoint string            `json:"from_checkpoint"`
	ToCheckpoint   string            `json:"to_checkpoint"`
	Added          []storyDiffChange `json:"added"`
	Removed        []storyDiffChange `json:"removed"`
	Modified       []storyDiffChange `json:"modified"`
	TotalAdded     int               `json:"total_added"`
	TotalRemoved   int               `json:"total_removed"`
	TotalModified  int               `json:"total_modified"`
}

type storyDiffChange struct {
	Path string `json:"path"`
	Type string `json:"type"`
}

func storyRun(t *testing.T, cwd string, args ...string) storyCommandResult {
	t.Helper()
	stdout, stderr, code := runJVS(t, cwd, args...)
	return storyCommandResult{stdout: stdout, stderr: stderr, code: code}
}

func storyRunJSON(t *testing.T, cwd string, args ...string) storyJSONResult {
	t.Helper()
	jsonArgs := append([]string{"--json"}, args...)
	result := storyRun(t, cwd, jsonArgs...)
	env := storyDecodeJSONEnvelope(t, result.stdout)
	return storyJSONResult{storyCommandResult: result, env: env}
}

func storyDecodeJSONEnvelope(t *testing.T, stdout string) storyJSONEnvelope {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(stdout))
	var env storyJSONEnvelope
	if err := dec.Decode(&env); err != nil {
		t.Fatalf("decode JSON envelope: %v\n%s", err, stdout)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		t.Fatalf("JSON stdout must contain exactly one value: %s", stdout)
	}
	if len(env.Data) == 0 {
		t.Fatalf("JSON envelope missing data field: %s", stdout)
	}
	if env.SchemaVersion != 1 {
		t.Fatalf("schema_version = %d, want 1\n%s", env.SchemaVersion, stdout)
	}
	return env
}

func storyJSONData[T any](t *testing.T, result storyJSONResult) T {
	t.Helper()
	var data T
	if err := json.Unmarshal(result.env.Data, &data); err != nil {
		t.Fatalf("decode JSON data for %s: %v\n%s", result.env.Command, err, result.stdout)
	}
	return data
}

func storyRequireSuccess(t *testing.T, result storyCommandResult, context string) {
	t.Helper()
	if result.code != 0 {
		t.Fatalf("%s failed: stdout=%s stderr=%s", context, result.stdout, result.stderr)
	}
}

func storyRequireFailure(t *testing.T, result storyCommandResult, context string) {
	t.Helper()
	if result.code == 0 {
		t.Fatalf("%s unexpectedly succeeded: stdout=%s stderr=%s", context, result.stdout, result.stderr)
	}
}

func storyRequireJSONSuccess(t *testing.T, result storyJSONResult, command string) {
	t.Helper()
	storyRequireSuccess(t, result.storyCommandResult, command)
	if strings.TrimSpace(result.stderr) != "" {
		t.Fatalf("%s JSON wrote stderr: %q", command, result.stderr)
	}
	if !result.env.OK {
		t.Fatalf("%s returned error envelope: %s", command, result.stdout)
	}
	if result.env.Command != command {
		t.Fatalf("JSON command = %q, want %q\n%s", result.env.Command, command, result.stdout)
	}
	if string(result.env.Data) == "null" {
		t.Fatalf("%s success envelope must include data: %s", command, result.stdout)
	}
	if result.env.Error != nil {
		t.Fatalf("%s success envelope included error: %s", command, result.stdout)
	}
}

func storyRequireJSONFailure(t *testing.T, result storyJSONResult, command, code string) {
	t.Helper()
	storyRequireFailure(t, result.storyCommandResult, command)
	if strings.TrimSpace(result.stderr) != "" {
		t.Fatalf("%s JSON error wrote stderr: %q", command, result.stderr)
	}
	if result.env.OK {
		t.Fatalf("%s returned ok envelope for failure: %s", command, result.stdout)
	}
	if result.env.Command != command {
		t.Fatalf("JSON command = %q, want %q\n%s", result.env.Command, command, result.stdout)
	}
	if string(result.env.Data) != "null" {
		t.Fatalf("%s failure envelope data = %s, want null", command, result.env.Data)
	}
	if result.env.Error == nil {
		t.Fatalf("%s failure envelope missing error: %s", command, result.stdout)
	}
	if code != "" && result.env.Error.Code != code {
		t.Fatalf("%s error code = %q, want %q\n%s", command, result.env.Error.Code, code, result.stdout)
	}
}

func storyRequireContains(t *testing.T, value, want, context string) {
	t.Helper()
	if !strings.Contains(value, want) {
		t.Fatalf("%s missing %q in:\n%s", context, want, value)
	}
}

func storyCombinedOutput(result storyCommandResult) string {
	return result.stdout + result.stderr
}

func storyNewRepo(t *testing.T, name string) (base, repoRoot, mainPath string) {
	t.Helper()
	base = t.TempDir()
	repoRoot = filepath.Join(base, name)
	result := storyRun(t, base, "--no-color", "init", repoRoot)
	storyRequireSuccess(t, result, "init repo")
	mainPath = filepath.Join(repoRoot, "main")
	return base, repoRoot, mainPath
}

func storyWriteText(t *testing.T, root, name, content string) {
	t.Helper()
	storyWriteBytes(t, root, name, []byte(content))
}

func storyWriteBytes(t *testing.T, root, name string, content []byte) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("create parent for %s: %v", path, err)
	}
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func storyMkdir(t *testing.T, root, name string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, name), 0755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Join(root, name), err)
	}
}

func storyRemove(t *testing.T, root, name string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.RemoveAll(path); err != nil {
		t.Fatalf("remove %s: %v", path, err)
	}
}

func storyReadText(t *testing.T, root, name string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(root, name))
	if err != nil {
		t.Fatalf("read %s: %v", filepath.Join(root, name), err)
	}
	return string(content)
}

func storyReadBytes(t *testing.T, root, name string) []byte {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(root, name))
	if err != nil {
		t.Fatalf("read %s: %v", filepath.Join(root, name), err)
	}
	return content
}

func storyRequireText(t *testing.T, root, name, want string) {
	t.Helper()
	if got := storyReadText(t, root, name); got != want {
		t.Fatalf("%s content = %q, want %q", filepath.Join(root, name), got, want)
	}
}

func storyRequireBytes(t *testing.T, root, name string, want []byte) {
	t.Helper()
	got := storyReadBytes(t, root, name)
	if string(got) != string(want) {
		t.Fatalf("%s bytes = %v, want %v", filepath.Join(root, name), got, want)
	}
}

func storyRequirePathExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected path to exist %s: %v", path, err)
	}
}

func storyRequirePathMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected path to be missing %s: %v", path, err)
	}
}

func storyCheckpointHuman(t *testing.T, repoRoot, note string, tags ...string) storyCheckpointRecord {
	t.Helper()
	args := []string{"--no-color", "checkpoint", note}
	for _, tag := range tags {
		args = append(args, "--tag", tag)
	}
	result := storyRun(t, filepath.Join(repoRoot, "main"), args...)
	storyRequireSuccess(t, result, "checkpoint "+note)
	storyRequireContains(t, result.stdout, "Created checkpoint", "checkpoint output")
	return storyLatestCheckpoint(t, repoRoot)
}

func storyCheckpointJSONAt(t *testing.T, cwd, note string, tags ...string) storyCheckpointRecord {
	t.Helper()
	args := []string{"checkpoint", note}
	for _, tag := range tags {
		args = append(args, "--tag", tag)
	}
	result := storyRunJSON(t, cwd, args...)
	storyRequireJSONSuccess(t, result, "checkpoint")
	record := storyJSONData[storyCheckpointRecord](t, result)
	if record.CheckpointID == "" {
		t.Fatalf("checkpoint JSON missing checkpoint_id: %s", result.stdout)
	}
	return record
}

func storyLatestCheckpoint(t *testing.T, repoRoot string) storyCheckpointRecord {
	t.Helper()
	records := storyCheckpointList(t, filepath.Join(repoRoot, "main"))
	if len(records) == 0 {
		t.Fatalf("expected at least one checkpoint")
	}
	return records[0]
}

func storyCheckpointList(t *testing.T, cwd string) []storyCheckpointRecord {
	t.Helper()
	result := storyRunJSON(t, cwd, "checkpoint", "list")
	storyRequireJSONSuccess(t, result, "checkpoint list")
	return storyJSONData[[]storyCheckpointRecord](t, result)
}

func storyStatus(t *testing.T, cwd string, args ...string) storyWorkspaceStatus {
	t.Helper()
	allArgs := append([]string{}, args...)
	allArgs = append(allArgs, "status")
	result := storyRunJSON(t, cwd, allArgs...)
	storyRequireJSONSuccess(t, result, "status")
	return storyJSONData[storyWorkspaceStatus](t, result)
}

func storyRequireStatus(t *testing.T, got storyWorkspaceStatus, current, latest string, dirty, atLatest bool) {
	t.Helper()
	if got.Current != current || got.Latest != latest || got.Dirty != dirty || got.AtLatest != atLatest {
		t.Fatalf("status = current:%q latest:%q dirty:%t at_latest:%t, want current:%q latest:%q dirty:%t at_latest:%t",
			got.Current, got.Latest, got.Dirty, got.AtLatest, current, latest, dirty, atLatest)
	}
}

func storyFindCheckpointByTag(t *testing.T, records []storyCheckpointRecord, tag string) storyCheckpointRecord {
	t.Helper()
	for _, record := range records {
		for _, got := range record.Tags {
			if got == tag {
				return record
			}
		}
	}
	t.Fatalf("checkpoint tag %q not found in %#v", tag, records)
	return storyCheckpointRecord{}
}

func storyHasWorkspace(records []storyWorkspaceRecord, name string) bool {
	for _, record := range records {
		if record.Workspace == name {
			return true
		}
	}
	return false
}

func storyUniquePrefix(id string) string {
	if len(id) <= 1 {
		return id
	}
	return id[:len(id)-1]
}
