package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/jvs-project/jvs/internal/engine"
	"github.com/jvs-project/jvs/internal/repo"
	"github.com/jvs-project/jvs/internal/snapshot"
	"github.com/jvs-project/jvs/pkg/model"
)

func existingDirectory(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("stat directory: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("path is not a directory: %s", abs)
	}
	return filepath.Clean(abs), nil
}

func rejectContainsJVS(root string) error {
	var found string
	errContainsJVS := errors.New("source contains .jvs")
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Name() == repo.JVSDirName {
			found = path
			return errContainsJVS
		}
		return nil
	})
	if errors.Is(err, errContainsJVS) {
		return fmt.Errorf("source must not contain %s metadata: %s", repo.JVSDirName, found)
	}
	if err != nil {
		return fmt.Errorf("scan source: %w", err)
	}
	return nil
}

func rejectDangerousOverlap(aLabel, aPath, bLabel, bPath string) error {
	aAbs, err := filepath.Abs(aPath)
	if err != nil {
		return fmt.Errorf("resolve %s path: %w", aLabel, err)
	}
	bAbs, err := filepath.Abs(bPath)
	if err != nil {
		return fmt.Errorf("resolve %s path: %w", bLabel, err)
	}
	aAbs = filepath.Clean(aAbs)
	bAbs = filepath.Clean(bAbs)

	if pathContains(aAbs, bAbs) || pathContains(bAbs, aAbs) {
		return fmt.Errorf("dangerous path overlap between %s (%s) and %s (%s)", aLabel, aAbs, bLabel, bAbs)
	}
	return nil
}

func pathContains(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && !filepath.IsAbs(rel)
}

func cloneDirectory(source, dest string, eng engine.Engine) (*engine.CloneResult, model.EngineType, error) {
	result, err := eng.Clone(source, dest)
	if err != nil {
		return nil, eng.Name(), err
	}
	return result, eng.Name(), nil
}

func createInitialCheckpoint(repoRoot, note string, tags []string) (*model.Descriptor, error) {
	creator := snapshot.NewCreator(repoRoot, detectEngine(repoRoot))
	return creator.Create("main", note, tags)
}

func effectiveTransferMode(engineType model.EngineType, result *engine.CloneResult) string {
	if result == nil {
		return string(engineType)
	}
	if engineType != model.EngineCopy && result.Degraded {
		return string(model.EngineCopy)
	}
	return string(engineType)
}

func degradedReasons(result *engine.CloneResult) []string {
	if result == nil || len(result.Degradations) == 0 {
		return []string{}
	}
	return result.Degradations
}
