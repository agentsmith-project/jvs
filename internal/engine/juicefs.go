package engine

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/agentsmith-project/jvs/pkg/fsutil"
	"github.com/agentsmith-project/jvs/pkg/model"
)

// JuiceFSEngine performs clone using `juicefs clone` command.
// When juicefs is unavailable or the source is not on JuiceFS,
// it falls back to the copy engine.
type JuiceFSEngine struct {
	CopyEngine      *CopyEngine // Fallback
	isOnJuiceFSFunc func(string) bool
}

// NewJuiceFSEngine creates a new JuiceFSEngine.
func NewJuiceFSEngine() *JuiceFSEngine {
	return &JuiceFSEngine{
		CopyEngine: NewCopyEngine(),
	}
}

// Name returns the engine type.
func (e *JuiceFSEngine) Name() model.EngineType {
	return model.EngineJuiceFSClone
}

// Clone performs a juicefs clone if available, falls back to copy otherwise.
// Returns a degraded result if juicefs is not available or not on JuiceFS.
func (e *JuiceFSEngine) Clone(src, dst string) (*CloneResult, error) {
	return e.clone(src, dst, false)
}

// CloneToNew performs a juicefs clone into an owned destination path whose leaf
// must not already exist. If juicefs leaves a partial leaf before failing, it is
// removed before fallback copy.
func (e *JuiceFSEngine) CloneToNew(src, dst string) (*CloneResult, error) {
	if err := PrepareCloneToNewDestination(dst); err != nil {
		return nil, err
	}
	return e.clone(src, dst, true)
}

func (e *JuiceFSEngine) clone(src, dst string, ownedNewDestination bool) (*CloneResult, error) {
	// Check if juicefs command is available
	if !e.isJuiceFSAvailable() {
		// Fall back to copy engine
		result, err := e.copyFallback(src, dst, ownedNewDestination)
		if err != nil {
			return nil, err
		}
		result.AddDegradation("juicefs-not-available", model.EngineCopy)
		return result, nil
	}

	// Check if source is on JuiceFS
	if !e.isSourceOnJuiceFS(src) {
		// Fall back to copy engine
		result, err := e.copyFallback(src, dst, ownedNewDestination)
		if err != nil {
			return nil, err
		}
		result.AddDegradation("not-on-juicefs", model.EngineCopy)
		return result, nil
	}

	// Execute juicefs clone
	if ownedNewDestination {
		if err := revalidateCloneToNewDestination(dst); err != nil {
			return nil, err
		}
	}
	commandDst := dst
	var staging *juiceFSCloneStaging
	if ownedNewDestination {
		var err error
		staging, err = prepareJuiceFSCloneStaging(dst)
		if err != nil {
			return nil, err
		}
		commandDst = staging.payload
	}
	cmd := exec.Command("juicefs", "clone", src, commandDst, "-p")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		context := juiceFSCommandContext(stdout.String(), stderr.String())
		if ownedNewDestination {
			if cleanupErr := cleanupJuiceFSCloneStaging(staging); cleanupErr != nil {
				return nil, fmt.Errorf("%s; cleanup partial destination: %w", juiceFSCloneFailureMessage(context), cleanupErr)
			}
		}
		// Fall back to copy on failure
		result, err := e.copyFallback(src, dst, ownedNewDestination)
		if err != nil {
			if context != "" {
				return nil, fmt.Errorf("%s, fallback copy: %w", juiceFSCloneFailureMessage(context), err)
			}
			return nil, fmt.Errorf("juicefs clone failed, fallback copy: %w", err)
		}
		result.AddDegradation("juicefs-clone-failed", model.EngineCopy)
		if context != "" {
			result.AddDegradation("juicefs-clone-context: "+context, model.EngineCopy)
		}
		return result, nil
	}

	if ownedNewDestination {
		if err := publishJuiceFSCloneStaging(staging, dst); err != nil {
			if cleanupErr := cleanupJuiceFSCloneStaging(staging); cleanupErr != nil {
				return nil, fmt.Errorf("publish juicefs clone: %v; cleanup partial destination: %w", err, cleanupErr)
			}
			result, fallbackErr := e.copyFallback(src, dst, true)
			if fallbackErr != nil {
				return nil, fmt.Errorf("publish juicefs clone: %v, fallback copy: %w", err, fallbackErr)
			}
			result.AddDegradation("juicefs-publish-failed", model.EngineCopy)
			return result, nil
		}
		if err := cleanupJuiceFSCloneStaging(staging); err != nil {
			return nil, fmt.Errorf("cleanup juicefs clone staging: %w", err)
		}
	}
	return NewCloneResult(model.EngineJuiceFSClone), nil
}

type juiceFSCloneStaging struct {
	dir     string
	payload string
	info    os.FileInfo
}

func prepareJuiceFSCloneStaging(dst string) (*juiceFSCloneStaging, error) {
	if err := validateCloneDestinationParent(dst); err != nil {
		return nil, fmt.Errorf("clone destination parent is not safe: %w", err)
	}
	stagingDir, err := os.MkdirTemp(filepath.Dir(dst), ".jvs-juicefs-partial-")
	if err != nil {
		return nil, fmt.Errorf("create juicefs clone staging: %w", err)
	}
	info, err := os.Lstat(stagingDir)
	if err != nil {
		_ = os.RemoveAll(stagingDir)
		return nil, fmt.Errorf("stat juicefs clone staging: %w", err)
	}
	staging := &juiceFSCloneStaging{
		dir:     stagingDir,
		payload: filepath.Join(stagingDir, "payload"),
		info:    info,
	}
	if err := revalidateCloneToNewDestination(staging.payload); err != nil {
		_ = os.RemoveAll(stagingDir)
		return nil, fmt.Errorf("prepare juicefs clone staging: %w", err)
	}
	return staging, nil
}

func publishJuiceFSCloneStaging(staging *juiceFSCloneStaging, dst string) error {
	if err := validateJuiceFSCloneStaging(staging); err != nil {
		return err
	}
	if err := revalidateCloneToNewDestination(dst); err != nil {
		return err
	}
	if _, err := os.Lstat(staging.payload); err != nil {
		return fmt.Errorf("stat juicefs partial destination: %w", err)
	}
	if err := fsutil.RenameNoReplaceAndSync(staging.payload, dst); err != nil {
		return fmt.Errorf("publish juicefs partial destination: %w", err)
	}
	return nil
}

func cleanupJuiceFSCloneStaging(staging *juiceFSCloneStaging) error {
	if staging == nil {
		return nil
	}
	info, err := os.Lstat(staging.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat juicefs clone staging: %w", err)
	}
	if err := validateJuiceFSCloneStagingInfo(staging, info); err != nil {
		return err
	}
	if err := os.RemoveAll(staging.dir); err != nil {
		return err
	}
	if _, statErr := os.Lstat(staging.dir); statErr == nil {
		return fmt.Errorf("partial destination staging still exists: %s", staging.dir)
	} else if !os.IsNotExist(statErr) {
		return fmt.Errorf("verify partial destination cleanup: %w", statErr)
	}
	return nil
}

func validateJuiceFSCloneStaging(staging *juiceFSCloneStaging) error {
	if staging == nil {
		return fmt.Errorf("juicefs clone staging is required")
	}
	info, err := os.Lstat(staging.dir)
	if err != nil {
		return fmt.Errorf("stat juicefs clone staging: %w", err)
	}
	return validateJuiceFSCloneStagingInfo(staging, info)
}

func validateJuiceFSCloneStagingInfo(staging *juiceFSCloneStaging, info os.FileInfo) error {
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("juicefs clone staging is symlink: %s", staging.dir)
	}
	if !info.IsDir() {
		return fmt.Errorf("juicefs clone staging is not a directory: %s", staging.dir)
	}
	if !os.SameFile(staging.info, info) {
		return fmt.Errorf("juicefs clone staging changed before cleanup; no files were removed")
	}
	return nil
}

func (e *JuiceFSEngine) copyFallback(src, dst string, ownedNewDestination bool) (*CloneResult, error) {
	copyEngine := e.CopyEngine
	if copyEngine == nil {
		copyEngine = NewCopyEngine()
	}
	if ownedNewDestination {
		return copyEngine.CloneToNew(src, dst)
	}
	return copyEngine.Clone(src, dst)
}

func (e *JuiceFSEngine) isJuiceFSAvailable() bool {
	_, err := exec.LookPath("juicefs")
	return err == nil
}

func (e *JuiceFSEngine) isSourceOnJuiceFS(path string) bool {
	if e.isOnJuiceFSFunc != nil {
		return e.isOnJuiceFSFunc(path)
	}
	return e.isOnJuiceFS(path)
}

func (e *JuiceFSEngine) isOnJuiceFS(path string) bool {
	// Resolve to absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}

	// Read /proc/mounts to find JuiceFS mount points
	file, err := os.Open("/proc/mounts")
	if err != nil {
		// Fallback for non-Linux systems: check if juicefs command exists
		// This is a conservative fallback - it won't correctly detect JuiceFS
		// on macOS or other systems without /proc/mounts
		return e.isJuiceFSAvailable()
	}
	defer file.Close()

	// Find the longest matching JuiceFS mount point
	var bestMount string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		// fields[0] = device, fields[1] = mount point, fields[2] = fs type
		fsType := fields[2]
		mountPoint := fields[1]

		// Check if it's a JuiceFS mount (fs type contains "juicefs")
		if strings.Contains(strings.ToLower(fsType), "juicefs") {
			// Check if our path is under this mount point
			if strings.HasPrefix(absPath, mountPoint) && len(mountPoint) > len(bestMount) {
				bestMount = mountPoint
			}
		}
	}

	return bestMount != ""
}

func juiceFSCloneFailureMessage(context string) string {
	if context != "" {
		return "juicefs clone failed (" + context + ")"
	}
	return "juicefs clone failed"
}

func juiceFSCommandContext(stdout, stderr string) string {
	stderr = strings.TrimSpace(stderr)
	stdout = strings.TrimSpace(stdout)
	switch {
	case stderr != "" && stdout != "":
		return limitDiagnostic("stderr: " + stderr + "; stdout: " + stdout)
	case stderr != "":
		return limitDiagnostic("stderr: " + stderr)
	case stdout != "":
		return limitDiagnostic("stdout: " + stdout)
	default:
		return ""
	}
}

func limitDiagnostic(value string) string {
	const max = 512
	if len(value) <= max {
		return value
	}
	return value[:max] + "...(truncated)"
}

// DetectEngine auto-detects the best available engine for the given repository.
// Detection order: juicefs-clone (if on JuiceFS), reflink-copy (if supported), copy.
func DetectEngine(repoRoot string) (Engine, error) {
	// Check environment variable first
	if engineType := os.Getenv("JVS_ENGINE"); engineType != "" {
		switch engineType {
		case "juicefs":
			return NewJuiceFSEngine(), nil
		case "reflink":
			return NewReflinkEngine(), nil
		case "copy":
			return NewCopyEngine(), nil
		}
	}

	return DetectEngineAuto(repoRoot)
}

// DetectEngineAuto auto-detects the best available engine without considering
// environment overrides.
func DetectEngineAuto(repoRoot string) (Engine, error) {
	// Auto-detect based on filesystem
	// 1. Check if on JuiceFS
	juicefsEngine := NewJuiceFSEngine()
	if juicefsEngine.isOnJuiceFS(repoRoot) && juicefsEngine.isJuiceFSAvailable() {
		return juicefsEngine, nil
	}

	// 2. Check if reflink is supported (btrfs, xfs, apfs)
	// Test on the target filesystem, not system temp dir
	reflinkEngine := NewReflinkEngine()
	testDir, err := os.MkdirTemp(repoRoot, ".jvs-reflink-test-")
	if err == nil {
		testFile := testDir + "/test"
		os.WriteFile(testFile, []byte("test"), 0600)
		testClone := testDir + "/clone"
		info, _ := os.Stat(testFile)
		if reflinkFile(testFile, testClone, info) == nil {
			os.RemoveAll(testDir)
			return reflinkEngine, nil
		}
		os.RemoveAll(testDir)
	}

	// 3. Fall back to copy
	return NewCopyEngine(), nil
}
