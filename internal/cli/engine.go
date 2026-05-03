package cli

import (
	"os"
	"strings"

	"github.com/agentsmith-project/jvs/internal/engine"
	"github.com/agentsmith-project/jvs/pkg/config"
	"github.com/agentsmith-project/jvs/pkg/model"
)

func requestedTransferEngine(repoRoot string) model.EngineType {
	if selected, ok := snapshotEngineFromEnv(); ok {
		return normalizeRequestedTransferEngine(selected)
	}
	if selected, ok := legacyEngineFromEnv(); ok {
		return normalizeRequestedTransferEngine(selected)
	}
	if cfg, err := config.Load(repoRoot); err == nil && cfg.DefaultEngine != "" {
		return normalizeRequestedTransferEngine(cfg.DefaultEngine)
	}
	return engine.EngineAuto
}

func normalizeRequestedTransferEngine(selected model.EngineType) model.EngineType {
	switch selected {
	case model.EngineJuiceFSClone, model.EngineReflinkCopy, model.EngineCopy, engine.EngineAuto:
		return selected
	default:
		return engine.EngineAuto
	}
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
