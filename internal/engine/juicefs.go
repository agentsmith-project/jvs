package engine

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jvs-project/jvs/pkg/model"
)

// JuiceFSEngine performs clone using `juicefs clone` command.
// When juicefs is unavailable or the source is not on JuiceFS,
// it falls back to the copy engine.
type JuiceFSEngine struct {
	CopyEngine *CopyEngine // Fallback
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
	// Check if juicefs command is available
	if !e.isJuiceFSAvailable() {
		// Fall back to copy engine
		result, err := e.CopyEngine.Clone(src, dst)
		if err != nil {
			return nil, err
		}
		result.AddDegradation("juicefs-not-available", model.EngineCopy)
		return result, nil
	}

	// Check if source is on JuiceFS
	if !e.isOnJuiceFS(src) {
		// Fall back to copy engine
		result, err := e.CopyEngine.Clone(src, dst)
		if err != nil {
			return nil, err
		}
		result.AddDegradation("not-on-juicefs", model.EngineCopy)
		return result, nil
	}

	// Execute juicefs clone
	cmd := exec.Command("juicefs", "clone", src, dst, "-p")
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		// Fall back to copy on failure
		result, err := e.CopyEngine.Clone(src, dst)
		if err != nil {
			return nil, err
		}
		result.AddDegradation("juicefs-clone-failed", model.EngineCopy)
		return result, nil
	}

	return NewCloneResult(model.EngineJuiceFSClone), nil
}

func (e *JuiceFSEngine) isJuiceFSAvailable() bool {
	_, err := exec.LookPath("juicefs")
	return err == nil
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
