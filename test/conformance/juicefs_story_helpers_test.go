//go:build conformance && juicefs_e2e

package conformance

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const (
	juiceFSE2EOptInEnv   = "JVS_JUICEFS_E2E"
	juiceFSE2ERequireEnv = "JVS_JUICEFS_E2E_REQUIRED"
	juiceFSE2ERootEnv    = "JVS_JUICEFS_E2E_ROOT"

	juiceFSEngineName       = "juicefs-clone"
	juiceFSPerformanceClass = "constant-time-metadata-clone"
)

type juiceFSStoryEnv struct {
	BaseDir        string
	MetaPath       string
	BucketPath     string
	MountPath      string
	CachePath      string
	LogPath        string
	VolumeName     string
	JuiceFSVersion string
	JVSBuildInfo   string
	Capability     storySetupJSON
}

type storyCapability struct {
	Available  bool     `json:"available"`
	Supported  bool     `json:"supported"`
	Confidence string   `json:"confidence"`
	Warnings   []string `json:"warnings"`
}

type storyCapabilityReport struct {
	TargetPath           string          `json:"target_path"`
	ProbePath            string          `json:"probe_path"`
	WriteProbe           bool            `json:"write_probe"`
	Write                storyCapability `json:"write"`
	JuiceFS              storyCapability `json:"juicefs"`
	Reflink              storyCapability `json:"reflink"`
	Copy                 storyCapability `json:"copy"`
	RecommendedEngine    string          `json:"recommended_engine"`
	MetadataPreservation map[string]any  `json:"metadata_preservation"`
	PerformanceClass     string          `json:"performance_class"`
	Warnings             []string        `json:"warnings"`
}

type storySetupJSON struct {
	TargetPath           string                 `json:"target_path"`
	ProbePath            string                 `json:"probe_path"`
	WriteProbe           bool                   `json:"write_probe"`
	Write                storyCapability        `json:"write"`
	JuiceFS              storyCapability        `json:"juicefs"`
	Reflink              storyCapability        `json:"reflink"`
	Copy                 storyCapability        `json:"copy"`
	RecommendedEngine    string                 `json:"recommended_engine"`
	Capabilities         *storyCapabilityReport `json:"capabilities"`
	EffectiveEngine      string                 `json:"effective_engine"`
	PerformanceClass     string                 `json:"performance_class"`
	MetadataPreservation map[string]any         `json:"metadata_preservation"`
	Warnings             []string               `json:"warnings"`

	RepoRoot             string   `json:"repo_root"`
	MainWorkspace        string   `json:"main_workspace"`
	Scope                string   `json:"scope"`
	RequestedScope       string   `json:"requested_scope"`
	Provenance           any      `json:"provenance"`
	InitialCheckpoint    string   `json:"initial_checkpoint"`
	Engine               string   `json:"engine"`
	RequestedEngine      string   `json:"requested_engine"`
	TransferEngine       string   `json:"transfer_engine"`
	TransferMode         string   `json:"transfer_mode"`
	OptimizedTransfer    bool     `json:"optimized_transfer"`
	DegradedReasons      []string `json:"degraded_reasons"`
	RuntimeStateExcluded bool     `json:"runtime_state_excluded"`
}

type storyCheckpointJSON struct {
	CheckpointID     string   `json:"checkpoint_id"`
	Workspace        string   `json:"workspace"`
	Note             string   `json:"note"`
	Tags             []string `json:"tags"`
	Engine           string   `json:"engine"`
	ActualEngine     string   `json:"actual_engine"`
	EffectiveEngine  string   `json:"effective_engine"`
	DegradedReasons  []string `json:"degraded_reasons"`
	PerformanceClass string   `json:"performance_class"`
	IntegrityState   string   `json:"integrity_state"`
}

type storyStatusJSON struct {
	Current  string `json:"current"`
	Latest   string `json:"latest"`
	AtLatest bool   `json:"at_latest"`
	Dirty    bool   `json:"dirty"`
}

type storyCLIEnvelope struct {
	Command string          `json:"command"`
	OK      bool            `json:"ok"`
	Data    json.RawMessage `json:"data"`
	Error   json.RawMessage `json:"error"`
}

type juiceFSCommandResult struct {
	stdout   string
	stderr   string
	exitCode int
}

func requireJuiceFSE2EGate(t *testing.T) {
	t.Helper()
	if os.Getenv(juiceFSE2EOptInEnv) == "1" {
		return
	}
	juiceFSE2EUnavailable(t, "%s=1 is required to run JuiceFS user-story E2E tests", juiceFSE2EOptInEnv)
}

func requireLocalJuiceFSE2E(t *testing.T) *juiceFSStoryEnv {
	t.Helper()
	requireJuiceFSE2EGate(t)

	juicefsPath, err := exec.LookPath("juicefs")
	if err != nil {
		juiceFSE2EUnavailable(t, "juicefs binary not found in PATH: %v", err)
	}
	versionResult := runJuiceFSStoryCommand(t, 10*time.Second, "", "juicefs", "version")
	if versionResult.exitCode != 0 {
		juiceFSE2EUnavailable(t, "juicefs version failed: stdout=%s stderr=%s", versionResult.stdout, versionResult.stderr)
	}

	baseDir := createJuiceFSBaseDir(t)
	env := &juiceFSStoryEnv{
		BaseDir:        baseDir,
		MetaPath:       filepath.Join(baseDir, "meta.db"),
		BucketPath:     filepath.Join(baseDir, "bucket"),
		MountPath:      filepath.Join(baseDir, "mnt"),
		CachePath:      filepath.Join(baseDir, "cache"),
		LogPath:        filepath.Join(baseDir, "juicefs.log"),
		VolumeName:     fmt.Sprintf("jvs-e2e-%d", time.Now().UnixNano()),
		JuiceFSVersion: strings.TrimSpace(versionResult.stdout),
		JVSBuildInfo:   detectJVSBuildInfo(t),
	}
	t.Logf("JuiceFS binary: %s", juicefsPath)

	for _, dir := range []string{env.BucketPath, env.MountPath, env.CachePath} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("create JuiceFS E2E dir %s: %v", dir, err)
		}
	}

	metaURL := "sqlite3://" + env.MetaPath
	formatResult := runJuiceFSStoryCommand(t, 30*time.Second, "", "juicefs", "format", metaURL, env.VolumeName, "--storage", "file", "--bucket", env.BucketPath, "--trash-days", "0")
	if formatResult.exitCode != 0 {
		juiceFSE2EUnavailable(t, "juicefs format failed: stdout=%s stderr=%s", formatResult.stdout, formatResult.stderr)
	}

	mountResult := runJuiceFSStoryCommand(t, 30*time.Second, "", "juicefs", "mount", metaURL, env.MountPath, "-d", "--backup-meta", "0", "--cache-dir", env.CachePath, "--log", env.LogPath, "--no-usage-report")
	if mountResult.exitCode != 0 {
		juiceFSE2EUnavailable(t, "juicefs mount failed: stdout=%s stderr=%s", mountResult.stdout, mountResult.stderr)
	}
	t.Cleanup(func() {
		env.cleanupMount(t)
	})

	capability, capabilityOut, err := pollJuiceFSCapability(t, env.MountPath, 30*time.Second)
	if err != nil {
		juiceFSE2EUnavailable(t, "mounted JuiceFS capability probe failed: %v\nlast output:\n%s", err, capabilityOut)
	}
	env.Capability = capability
	if !capability.JuiceFS.Supported || capability.RecommendedEngine != juiceFSEngineName || capability.EffectiveEngine != juiceFSEngineName {
		juiceFSE2EUnavailable(t, "mounted path did not report supported %s capability: %s", juiceFSEngineName, capabilityOut)
	}

	t.Logf(
		"JuiceFS E2E evidence: juicefs_version=%q jvs_build=%q os=%s mount=%s meta=%s bucket=%s recommended_engine=%s effective_engine=%s",
		env.JuiceFSVersion,
		env.JVSBuildInfo,
		runtime.GOOS+"/"+runtime.GOARCH,
		env.MountPath,
		env.MetaPath,
		env.BucketPath,
		env.Capability.RecommendedEngine,
		env.Capability.EffectiveEngine,
	)

	return env
}

func createJuiceFSBaseDir(t *testing.T) string {
	t.Helper()
	root := os.Getenv(juiceFSE2ERootEnv)
	if root == "" {
		base := filepath.Join(t.TempDir(), "juicefs-e2e")
		if err := os.MkdirAll(base, 0755); err != nil {
			t.Fatalf("create JuiceFS E2E base dir: %v", err)
		}
		return base
	}
	if err := os.MkdirAll(root, 0755); err != nil {
		juiceFSE2EUnavailable(t, "create %s=%s: %v", juiceFSE2ERootEnv, root, err)
	}
	base, err := os.MkdirTemp(root, "jvs-juicefs-e2e-")
	if err != nil {
		juiceFSE2EUnavailable(t, "create JuiceFS E2E base dir under %s: %v", root, err)
	}
	t.Cleanup(func() {
		if active, err := mountPathActiveAtOrUnder(base); err == nil && active {
			t.Logf("not removing %s because a mount under it still appears active", base)
			return
		}
		if err := os.RemoveAll(base); err != nil {
			t.Logf("remove JuiceFS E2E base dir %s: %v", base, err)
		}
	})
	return base
}

func (env *juiceFSStoryEnv) cleanupMount(t *testing.T) {
	t.Helper()
	if env == nil || env.MountPath == "" {
		return
	}
	result := runJuiceFSStoryCommand(t, 30*time.Second, "", "juicefs", "umount", "--flush", env.MountPath)
	if result.exitCode != 0 {
		t.Logf("juicefs umount --flush failed for %s: stdout=%s stderr=%s", env.MountPath, result.stdout, result.stderr)
		force := runJuiceFSStoryCommand(t, 30*time.Second, "", "juicefs", "umount", "--force", env.MountPath)
		if force.exitCode != 0 {
			t.Logf("juicefs umount --force failed for %s: stdout=%s stderr=%s", env.MountPath, force.stdout, force.stderr)
		}
	}

	deadline := time.Now().Add(10 * time.Second)
	for {
		active, err := mountPathActive(env.MountPath)
		if err != nil {
			t.Logf("cannot confirm mount cleanup for %s: %v", env.MountPath, err)
			return
		}
		if !active {
			return
		}
		if time.Now().After(deadline) {
			t.Errorf("mount path still active after cleanup: %s", env.MountPath)
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func pollJuiceFSCapability(t *testing.T, mountPath string, timeout time.Duration) (storySetupJSON, string, error) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastOut string
	var lastErr string
	var lastCode int
	var lastDecodeErr error
	for {
		stdout, stderr, code := runJVS(t, mountPath, "--json", "capability", mountPath, "--write-probe")
		lastOut = stdout
		lastErr = stderr
		lastCode = code
		if code == 0 && strings.TrimSpace(stderr) == "" {
			var capability storySetupJSON
			if err := decodeStoryCLIJSON(stdout, &capability); err == nil {
				if capability.Write.Supported && capability.Copy.Supported {
					return capability, stdout, nil
				}
			} else {
				lastDecodeErr = err
			}
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if lastDecodeErr != nil {
		return storySetupJSON{}, lastOut, lastDecodeErr
	}
	return storySetupJSON{}, lastOut, fmt.Errorf("capability probe did not become ready: exit=%d stderr=%s", lastCode, lastErr)
}

func runJuiceFSStoryCommand(t *testing.T, timeout time.Duration, cwd, name string, args ...string) juiceFSCommandResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	if cwd != "" {
		cmd.Dir = cwd
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	result := juiceFSCommandResult{
		stdout: stdout.String(),
		stderr: stderr.String(),
	}
	if ctx.Err() == context.DeadlineExceeded {
		result.exitCode = 124
		result.stderr += fmt.Sprintf("\ncommand timed out after %s", timeout)
		return result
	}
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.exitCode = exitErr.ExitCode()
		} else {
			result.exitCode = 1
			result.stderr += err.Error()
		}
		return result
	}
	return result
}

func decodeStoryCLIJSON(stdout string, v any) error {
	var envelope storyCLIEnvelope
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		return fmt.Errorf("decode JSON envelope: %w", err)
	}
	if !envelope.OK {
		return fmt.Errorf("JSON envelope for %s reported error: %s", envelope.Command, string(envelope.Error))
	}
	if err := json.Unmarshal(envelope.Data, v); err != nil {
		return fmt.Errorf("decode JSON data for %s: %w", envelope.Command, err)
	}
	return nil
}

func runJVSJSON[T any](t *testing.T, cwd string, args ...string) T {
	t.Helper()
	allArgs := append([]string{"--json"}, args...)
	stdout, stderr, code := runJVS(t, cwd, allArgs...)
	if code != 0 {
		t.Fatalf("jvs %s failed: stdout=%s stderr=%s", strings.Join(args, " "), stdout, stderr)
	}
	if strings.TrimSpace(stderr) != "" {
		t.Fatalf("jvs %s wrote stderr in JSON mode: %s", strings.Join(args, " "), stderr)
	}
	var out T
	if err := decodeStoryCLIJSON(stdout, &out); err != nil {
		t.Fatalf("decode jvs %s JSON: %v\n%s", strings.Join(args, " "), err, stdout)
	}
	return out
}

func requireJuiceFSSetupEngine(t *testing.T, data storySetupJSON, context string) {
	t.Helper()
	if data.EffectiveEngine != juiceFSEngineName {
		t.Fatalf("%s effective_engine = %q, want %q", context, data.EffectiveEngine, juiceFSEngineName)
	}
	if data.PerformanceClass != juiceFSPerformanceClass {
		t.Fatalf("%s performance_class = %q, want %q", context, data.PerformanceClass, juiceFSPerformanceClass)
	}
	if data.Capabilities == nil {
		t.Fatalf("%s missing capabilities object", context)
	}
	if !data.Capabilities.JuiceFS.Supported {
		t.Fatalf("%s capabilities.juicefs.supported = false, want true", context)
	}
	if data.Capabilities.RecommendedEngine != juiceFSEngineName {
		t.Fatalf("%s capabilities.recommended_engine = %q, want %q", context, data.Capabilities.RecommendedEngine, juiceFSEngineName)
	}
}

func requireJuiceFSCheckpointEngine(t *testing.T, checkpoint storyCheckpointJSON, context string) {
	t.Helper()
	if checkpoint.Engine != juiceFSEngineName {
		t.Fatalf("%s engine = %q, want %q", context, checkpoint.Engine, juiceFSEngineName)
	}
	if checkpoint.ActualEngine != juiceFSEngineName {
		t.Fatalf("%s actual_engine = %q, want %q", context, checkpoint.ActualEngine, juiceFSEngineName)
	}
	if checkpoint.EffectiveEngine != juiceFSEngineName {
		t.Fatalf("%s effective_engine = %q, want %q", context, checkpoint.EffectiveEngine, juiceFSEngineName)
	}
	if checkpoint.PerformanceClass != juiceFSPerformanceClass {
		t.Fatalf("%s performance_class = %q, want %q", context, checkpoint.PerformanceClass, juiceFSPerformanceClass)
	}
	if len(checkpoint.DegradedReasons) != 0 {
		t.Fatalf("%s degraded_reasons = %#v, want empty", context, checkpoint.DegradedReasons)
	}
}

func requireOptimizedJuiceFSTransfer(t *testing.T, data storySetupJSON, context string) {
	t.Helper()
	requireJuiceFSSetupEngine(t, data, context)
	if data.TransferEngine != juiceFSEngineName {
		t.Fatalf("%s transfer_engine = %q, want %q", context, data.TransferEngine, juiceFSEngineName)
	}
	if data.TransferMode != juiceFSEngineName {
		t.Fatalf("%s transfer_mode = %q, want %q", context, data.TransferMode, juiceFSEngineName)
	}
	if !data.OptimizedTransfer {
		t.Fatalf("%s optimized_transfer = false, want true", context)
	}
	if len(data.DegradedReasons) != 0 {
		t.Fatalf("%s degraded_reasons = %#v, want empty", context, data.DegradedReasons)
	}
}

func requireFileContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(got) != want {
		t.Fatalf("%s content = %q, want %q", path, string(got), want)
	}
}

func requireNoPath(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("%s exists or cannot be checked: %v", path, err)
	}
}

func stringSliceContainsFragment(values []string, fragment string) bool {
	for _, value := range values {
		if strings.Contains(strings.ToLower(value), strings.ToLower(fragment)) {
			return true
		}
	}
	return false
}

func detectJVSBuildInfo(t *testing.T) string {
	t.Helper()
	if jvsBinary == "" || jvsBinary == "jvs" {
		return "unavailable"
	}
	if _, err := os.Stat(jvsBinary); err != nil {
		return "unavailable"
	}
	result := runJuiceFSStoryCommand(t, 10*time.Second, "", "go", "version", "-m", jvsBinary)
	if result.exitCode != 0 {
		return "unavailable"
	}
	lines := strings.Split(strings.TrimSpace(result.stdout), "\n")
	if len(lines) == 0 {
		return "unavailable"
	}
	return strings.Join(lines[:min(3, len(lines))], "; ")
}

func mountPathActive(path string) (bool, error) {
	return scanProcMounts(func(mountPath string) bool {
		return filepath.Clean(mountPath) == filepath.Clean(path)
	})
}

func mountPathActiveAtOrUnder(path string) (bool, error) {
	base := filepath.Clean(path)
	return scanProcMounts(func(mountPath string) bool {
		return juiceFSPathContains(base, filepath.Clean(mountPath))
	})
}

func scanProcMounts(match func(string) bool) (bool, error) {
	file, err := os.Open("/proc/mounts")
	if err != nil {
		return false, fmt.Errorf("open /proc/mounts: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		if match(unescapeProcMountPath(fields[1])) {
			return true, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return false, err
	}
	return false, nil
}

func juiceFSPathContains(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && !filepath.IsAbs(rel))
}

func unescapeProcMountPath(path string) string {
	replacer := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)
	return replacer.Replace(path)
}

func juiceFSE2EUnavailable(t *testing.T, format string, args ...any) {
	t.Helper()
	message := fmt.Sprintf(format, args...)
	if os.Getenv(juiceFSE2ERequireEnv) == "1" {
		t.Fatalf("JuiceFS E2E required but unavailable: %s", message)
	}
	t.Skipf("skipping JuiceFS E2E: %s", message)
}
