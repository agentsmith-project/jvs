package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/internal/worktree"
	"github.com/agentsmith-project/jvs/pkg/errclass"
	"github.com/agentsmith-project/jvs/pkg/model"
)

var reservedCheckpointRefs = map[string]struct{}{
	"current": {},
	"latest":  {},
	"dirty":   {},
}

var (
	errRefReserved  = &errclass.JVSError{Code: "E_REF_RESERVED"}
	errRefNotFound  = &errclass.JVSError{Code: "E_REF_NOT_FOUND"}
	errRefAmbiguous = &errclass.JVSError{Code: "E_REF_AMBIGUOUS"}
)

func isReservedCheckpointRef(ref string) bool {
	_, ok := reservedCheckpointRefs[ref]
	return ok
}

func validateCheckpointRefName(name string) error {
	if isReservedCheckpointRef(name) {
		return errRefReserved.WithMessagef("%q is a reserved ref", name)
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
		return "", errclass.ErrUsage.WithMessage("checkpoint ref is required")
	}

	switch ref {
	case "current":
		return resolveWorkspaceCheckpointRef(repoRoot, workspaceName, "current")
	case "latest":
		return resolveWorkspaceCheckpointRef(repoRoot, workspaceName, "latest")
	case "dirty":
		return "", errRefReserved.WithMessagef("%q is a reserved ref for working changes, not a checkpoint", ref)
	}

	if id := model.SnapshotID(ref); id.IsValid() {
		if _, err := snapshot.LoadDescriptor(repoRoot, id); err != nil {
			return "", checkpointRefDescriptorError(repoRoot, ref, id, err)
		}
		return id, nil
	}

	all, err := snapshot.ListCatalogEntries(repoRoot)
	if err != nil {
		return "", err
	}

	var idMatches []snapshot.CatalogEntry
	for _, entry := range all {
		if strings.HasPrefix(string(entry.SnapshotID), ref) {
			idMatches = append(idMatches, entry)
		}
	}

	var tagMatches []snapshot.CatalogEntry
	for _, entry := range all {
		if entry.DescriptorErr != nil {
			continue
		}
		for _, tag := range entry.Descriptor.Tags {
			if tag == ref {
				tagMatches = append(tagMatches, entry)
				break
			}
		}
	}

	corruptEntry, hasCatalogDescriptorError := firstCatalogDescriptorError(all)
	knownAmbiguous := len(idMatches) > 1 || len(tagMatches) > 1 || (len(idMatches) > 0 && len(tagMatches) > 0 && idMatches[0].SnapshotID != tagMatches[0].SnapshotID)
	switch {
	case knownAmbiguous:
		return "", errRefAmbiguous.WithMessagef("ambiguous checkpoint ref %q", ref)
	case hasCatalogDescriptorError:
		return "", checkpointRefDescriptorError(repoRoot, ref, corruptEntry.SnapshotID, corruptEntry.DescriptorErr)
	case len(idMatches) == 1:
		return resolvedCatalogEntryID(repoRoot, ref, idMatches[0])
	case len(tagMatches) == 1:
		return tagMatches[0].SnapshotID, nil
	default:
		return "", errRefNotFound.WithMessagef("checkpoint ref %q not found", ref)
	}
}

func resolveCheckpointRefInWorkspace(repoRoot, workspaceName, ref string) (model.SnapshotID, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", errclass.ErrUsage.WithMessage("checkpoint ref is required")
	}

	switch ref {
	case "current":
		return resolveWorkspaceCheckpointRef(repoRoot, workspaceName, "current")
	case "latest":
		return resolveWorkspaceCheckpointRef(repoRoot, workspaceName, "latest")
	case "dirty":
		return "", errRefReserved.WithMessagef("%q is a reserved ref for working changes, not a checkpoint", ref)
	}

	if id := model.SnapshotID(ref); id.IsValid() {
		desc, err := snapshot.LoadDescriptor(repoRoot, id)
		if err != nil {
			return "", checkpointRefDescriptorError(repoRoot, ref, id, err)
		}
		if desc.WorktreeName != workspaceName {
			return "", errRefNotFound.WithMessagef("checkpoint ref %q not found", ref)
		}
		return id, nil
	}

	all, err := snapshot.ListCatalogEntries(repoRoot)
	if err != nil {
		return "", err
	}

	var idMatches []snapshot.CatalogEntry
	for _, entry := range all {
		if !strings.HasPrefix(string(entry.SnapshotID), ref) {
			continue
		}
		if entry.DescriptorErr != nil || entry.Descriptor.WorktreeName == workspaceName {
			idMatches = append(idMatches, entry)
		}
	}

	var tagMatches []snapshot.CatalogEntry
	for _, entry := range all {
		if entry.DescriptorErr != nil || entry.Descriptor.WorktreeName != workspaceName {
			continue
		}
		for _, tag := range entry.Descriptor.Tags {
			if tag == ref {
				tagMatches = append(tagMatches, entry)
				break
			}
		}
	}

	corruptEntry, hasCatalogDescriptorError := firstCatalogDescriptorError(all)
	knownAmbiguous := len(idMatches) > 1 || len(tagMatches) > 1 || (len(idMatches) > 0 && len(tagMatches) > 0 && idMatches[0].SnapshotID != tagMatches[0].SnapshotID)
	switch {
	case knownAmbiguous:
		return "", errRefAmbiguous.WithMessagef("ambiguous checkpoint ref %q", ref)
	case hasCatalogDescriptorError:
		return "", checkpointRefDescriptorError(repoRoot, ref, corruptEntry.SnapshotID, corruptEntry.DescriptorErr)
	case len(idMatches) == 1:
		return resolvedCatalogEntryID(repoRoot, ref, idMatches[0])
	case len(tagMatches) == 1:
		return tagMatches[0].SnapshotID, nil
	default:
		return "", errRefNotFound.WithMessagef("checkpoint ref %q not found", ref)
	}
}

func firstCatalogDescriptorError(entries []snapshot.CatalogEntry) (snapshot.CatalogEntry, bool) {
	for _, entry := range entries {
		if entry.DescriptorErr != nil {
			return entry, true
		}
	}
	return snapshot.CatalogEntry{}, false
}

func resolvedCatalogEntryID(repoRoot, ref string, entry snapshot.CatalogEntry) (model.SnapshotID, error) {
	if entry.DescriptorErr != nil {
		return "", checkpointRefDescriptorError(repoRoot, ref, entry.SnapshotID, entry.DescriptorErr)
	}
	return entry.SnapshotID, nil
}

func checkpointRefDescriptorError(repoRoot, ref string, id model.SnapshotID, err error) error {
	if snapshot.IsDescriptorNotFound(err) {
		exists, existsErr := snapshot.PublishedSnapshotExists(repoRoot, id)
		if existsErr != nil {
			return errclass.ErrDescriptorCorrupt.WithMessagef("checkpoint ref %q publication is invalid: %v", ref, existsErr)
		}
		if !exists {
			return errRefNotFound.WithMessagef("checkpoint ref %q not found", ref)
		}
		if _, issue := snapshot.InspectPublishState(repoRoot, id, snapshot.PublishStateOptions{RequireReady: true}); issue != nil {
			return snapshot.PublishStateIssueError(issue)
		}
	}
	var jvsErr *errclass.JVSError
	if errors.As(err, &jvsErr) {
		return err
	}
	if errors.Is(err, errclass.ErrDescriptorCorrupt) {
		return err
	}
	return errclass.ErrDescriptorCorrupt.WithMessagef("load descriptor for checkpoint ref %q: %v", ref, err)
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
		return "", errRefNotFound.WithMessagef("workspace has no %s checkpoint", ref)
	}
	return id, nil
}
