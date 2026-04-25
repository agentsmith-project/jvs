package cli

import (
	"fmt"
	"strings"

	"github.com/jvs-project/jvs/internal/snapshot"
	"github.com/jvs-project/jvs/internal/worktree"
	"github.com/jvs-project/jvs/pkg/errclass"
	"github.com/jvs-project/jvs/pkg/model"
)

var reservedCheckpointRefs = map[string]struct{}{
	"current": {},
	"latest":  {},
	"dirty":   {},
}

func isReservedCheckpointRef(ref string) bool {
	_, ok := reservedCheckpointRefs[ref]
	return ok
}

func validateCheckpointRefName(name string) error {
	if isReservedCheckpointRef(name) {
		return fmt.Errorf("%q is a reserved ref", name)
	}
	return nil
}

func validatePublicWorkspaceName(name string) error {
	if isReservedCheckpointRef(name) {
		return errclass.ErrNameInvalid.WithMessagef("%q is a reserved ref and cannot be used as a workspace name", name)
	}
	return nil
}

func resolveCheckpointRef(repoRoot, workspaceName, ref string) (model.SnapshotID, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fmt.Errorf("checkpoint ref is required")
	}

	switch ref {
	case "current":
		return resolveWorkspaceCheckpointRef(repoRoot, workspaceName, "current")
	case "latest":
		return resolveWorkspaceCheckpointRef(repoRoot, workspaceName, "latest")
	case "dirty":
		return "", fmt.Errorf("%q is a reserved ref for working changes, not a checkpoint", ref)
	}

	if id := model.SnapshotID(ref); id.IsValid() {
		if _, err := snapshot.LoadDescriptor(repoRoot, id); err != nil {
			return "", fmt.Errorf("checkpoint %q not found", ref)
		}
		return id, nil
	}

	all, err := snapshot.ListAll(repoRoot)
	if err != nil {
		return "", err
	}

	var idMatches []*model.Descriptor
	for _, desc := range all {
		if strings.HasPrefix(string(desc.SnapshotID), ref) {
			idMatches = append(idMatches, desc)
		}
	}

	var tagMatches []*model.Descriptor
	for _, desc := range all {
		for _, tag := range desc.Tags {
			if tag == ref {
				tagMatches = append(tagMatches, desc)
				break
			}
		}
	}

	switch {
	case len(idMatches) == 1 && len(tagMatches) == 0:
		return idMatches[0].SnapshotID, nil
	case len(idMatches) == 0 && len(tagMatches) == 1:
		return tagMatches[0].SnapshotID, nil
	case len(idMatches) == 1 && len(tagMatches) == 1 && idMatches[0].SnapshotID == tagMatches[0].SnapshotID:
		return idMatches[0].SnapshotID, nil
	case len(idMatches) > 1 || len(tagMatches) > 1 || (len(idMatches) > 0 && len(tagMatches) > 0):
		return "", fmt.Errorf("ambiguous checkpoint ref %q", ref)
	default:
		return "", fmt.Errorf("checkpoint ref %q not found", ref)
	}
}

func resolveWorkspaceCheckpointRef(repoRoot, workspaceName, ref string) (model.SnapshotID, error) {
	if workspaceName == "" {
		return "", fmt.Errorf("%q requires running inside a workspace", ref)
	}

	cfg, err := worktree.NewManager(repoRoot).Get(workspaceName)
	if err != nil {
		return "", fmt.Errorf("load workspace: %w", err)
	}

	var id model.SnapshotID
	switch ref {
	case "current":
		id = cfg.HeadSnapshotID
	case "latest":
		id = cfg.LatestSnapshotID
	}
	if id == "" {
		return "", fmt.Errorf("workspace has no %s checkpoint", ref)
	}
	return id, nil
}
