package jvs

import (
	"fmt"
	"os"

	"github.com/agentsmith-project/jvs/internal/engine"
	"github.com/agentsmith-project/jvs/pkg/model"
)

// DetectEngine returns the best available save point materialization engine for the given path.
// Detection priority: juicefs-clone > reflink-copy > copy.
// The path should be the repository root or intended repository location.
func DetectEngine(path string) model.EngineType {
	eng, err := engine.DetectEngine(path)
	if err != nil {
		return model.EngineCopy
	}
	return eng.Name()
}

// ValidateEngine checks whether the given engine type is usable at the given path.
// Returns nil if the engine can operate correctly, or an error describing why not.
func ValidateEngine(path string, engineType model.EngineType) error {
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("path not accessible: %w", err)
	}

	eng := engine.NewEngine(engineType)

	// For juicefs-clone, verify we're actually on JuiceFS
	if engineType == model.EngineJuiceFSClone {
		testDir, err := os.MkdirTemp(path, ".jvs-engine-test-")
		if err != nil {
			return fmt.Errorf("cannot create test directory: %w", err)
		}
		defer os.RemoveAll(testDir)

		testSrc := testDir + "/src"
		testDst := testDir + "/dst"
		if err := os.MkdirAll(testSrc, 0755); err != nil {
			return fmt.Errorf("cannot create test source: %w", err)
		}

		result, err := eng.Clone(testSrc, testDst)
		if err != nil {
			return fmt.Errorf("engine %s clone test failed: %w", engineType, err)
		}
		if result.Degraded {
			return fmt.Errorf("engine %s degraded at %s: %v", engineType, path, result.Degradations)
		}
		return nil
	}

	// For copy engine, always valid if path exists
	if engineType == model.EngineCopy {
		return nil
	}

	// For reflink, test with a real file
	if engineType == model.EngineReflinkCopy {
		testDir, err := os.MkdirTemp(path, ".jvs-engine-test-")
		if err != nil {
			return fmt.Errorf("cannot create test directory: %w", err)
		}
		defer os.RemoveAll(testDir)

		testSrc := testDir + "/src"
		if err := os.MkdirAll(testSrc, 0755); err != nil {
			return fmt.Errorf("cannot create test source: %w", err)
		}
		if err := os.WriteFile(testSrc+"/test", []byte("test"), 0600); err != nil {
			return fmt.Errorf("cannot write test file: %w", err)
		}

		testDst := testDir + "/dst"
		result, err := eng.Clone(testSrc, testDst)
		if err != nil {
			return fmt.Errorf("engine %s clone test failed: %w", engineType, err)
		}
		if result.Degraded {
			return fmt.Errorf("engine %s degraded at %s: %v", engineType, path, result.Degradations)
		}
		return nil
	}

	return fmt.Errorf("unknown engine type: %s", engineType)
}
