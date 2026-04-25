package cli

import (
	"os"
	"strings"

	"github.com/jvs-project/jvs/internal/engine"
	"github.com/jvs-project/jvs/pkg/config"
	"github.com/jvs-project/jvs/pkg/model"
)

// detectEngine returns the best available engine for the repository.
func detectEngine(repoRoot string) model.EngineType {
	return resolveEffectiveEngine(repoRoot)
}

func newCloneEngine(repoRoot string) engine.Engine {
	return engine.NewEngine(detectEngine(repoRoot))
}

func resolveEffectiveEngine(repoRoot string) model.EngineType {
	if selected, ok := snapshotEngineFromEnv(); ok {
		return resolveEngineChoice(repoRoot, selected)
	}
	if selected, ok := legacyEngineFromEnv(); ok {
		return resolveEngineChoice(repoRoot, selected)
	}
	if cfg, err := config.Load(repoRoot); err == nil && cfg.DefaultEngine != "" {
		return resolveEngineChoice(repoRoot, cfg.DefaultEngine)
	}
	return autoDetectEngine(repoRoot)
}

func snapshotEngineFromEnv() (model.EngineType, bool) {
	return parseSnapshotEngineValue(os.Getenv("JVS_SNAPSHOT_ENGINE"))
}

func legacyEngineFromEnv() (model.EngineType, bool) {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("JVS_ENGINE"))) {
	case "juicefs":
		return model.EngineJuiceFSClone, true
	case "reflink":
		return model.EngineReflinkCopy, true
	case "copy":
		return model.EngineCopy, true
	case "auto":
		return model.EngineType("auto"), true
	default:
		return "", false
	}
}

func parseSnapshotEngineValue(value string) (model.EngineType, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(model.EngineJuiceFSClone):
		return model.EngineJuiceFSClone, true
	case string(model.EngineReflinkCopy):
		return model.EngineReflinkCopy, true
	case string(model.EngineCopy):
		return model.EngineCopy, true
	case "auto":
		return model.EngineType("auto"), true
	default:
		return "", false
	}
}

func resolveEngineChoice(repoRoot string, selected model.EngineType) model.EngineType {
	switch selected {
	case model.EngineJuiceFSClone, model.EngineReflinkCopy, model.EngineCopy:
		return selected
	case "", model.EngineType("auto"):
		return autoDetectEngine(repoRoot)
	default:
		return autoDetectEngine(repoRoot)
	}
}

func autoDetectEngine(repoRoot string) model.EngineType {
	eng, err := engine.DetectEngineAuto(repoRoot)
	if err != nil {
		return model.EngineCopy // fallback
	}
	return eng.Name()
}
