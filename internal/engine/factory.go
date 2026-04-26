// Package engine provides snapshot engines for copying worktree data.
// Engines support different cloning strategies: juicefs-clone, reflink-copy, and copy.
package engine

import (
	"github.com/agentsmith-project/jvs/pkg/model"
)

// NewEngine creates an engine based on the specified type.
// Falls back to CopyEngine if the requested engine is not available.
func NewEngine(engineType model.EngineType) Engine {
	switch engineType {
	case model.EngineJuiceFSClone:
		return NewJuiceFSEngine()
	case model.EngineReflinkCopy:
		return NewReflinkEngine()
	default:
		return NewCopyEngine()
	}
}
