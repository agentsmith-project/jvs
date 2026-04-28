package cli

import (
	"os"
	"path/filepath"

	"github.com/agentsmith-project/jvs/internal/capacitygate"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/restoreplan"
	"github.com/agentsmith-project/jvs/internal/snapshotpayload"
	"github.com/agentsmith-project/jvs/internal/worktree"
	"github.com/agentsmith-project/jvs/pkg/model"
)

const (
	metadataFloor = 1 << 20
)

var restoreRunCapacityGate = capacitygate.Default()
var saveCapacityGate = capacitygate.Default()
var workspaceNewCapacityGate = capacitygate.Default()

func checkSaveCapacity(repoRoot, workspaceName string) error {
	folder, err := workspaceFolder(repoRoot, workspaceName)
	if err != nil {
		return err
	}
	mgr := worktree.NewManager(repoRoot)
	cfg, err := mgr.Get(workspaceName)
	if err != nil {
		return err
	}
	boundary, err := repo.WorktreeManagedPayloadBoundary(repoRoot, workspaceName)
	if err != nil {
		return err
	}
	workspaceBytes, err := capacitygate.TreeSize(boundary.Root, boundary.ExcludesRelativePath)
	if err != nil {
		return err
	}

	components := []capacitygate.Component{
		{Name: "save point payload", Path: filepath.Join(repoRoot, repo.JVSDirName, "snapshots", "save-capacity-probe.tmp"), Bytes: workspaceBytes},
		{Name: "save point metadata", Path: filepath.Join(repoRoot, repo.JVSDirName), Bytes: metadataFloor},
	}
	if len(cfg.PathSources) > 0 {
		components, err = appendPathSourceReconcileCapacityComponents(repoRoot, cfg, components)
		if err != nil {
			return err
		}
	}

	_, err = saveCapacityGate.Check(capacitygate.Request{
		Operation:       "save",
		Folder:          folder,
		Workspace:       workspaceName,
		Components:      components,
		FailureMessages: []string{"No save point was created.", "History was not changed.", "No files were changed."},
	})
	return err
}

func checkRestorePreviewPreDirtyCapacity(repoRoot, workspaceName string, sourceID model.SnapshotID, path string) error {
	folder, err := workspaceFolder(repoRoot, workspaceName)
	if err != nil {
		return err
	}
	mgr := worktree.NewManager(repoRoot)
	cfg, err := mgr.Get(workspaceName)
	if err != nil {
		return err
	}

	components, err := restorePreviewSourceCapacityComponents(repoRoot, sourceID)
	if err != nil {
		return err
	}
	if path != "" || len(cfg.PathSources) > 0 {
		components, err = appendExpectedWorkspaceCapacityComponents(repoRoot, cfg, components)
		if err != nil {
			return err
		}
	}

	_, err = restoreRunCapacityGate.Check(capacitygate.Request{
		Operation:       "restore preview",
		Folder:          folder,
		Workspace:       workspaceName,
		SourceSavePoint: string(sourceID),
		Path:            path,
		Components:      components,
		FailureMessages: []string{"No restore plan was created.", "No files were changed."},
	})
	return err
}

func checkRestoreRunCapacity(repoRoot, workspaceName string, plan *restoreplan.Plan, snapshotDir string, desc *model.Descriptor) error {
	mgrBoundary, err := repo.WorktreeManagedPayloadBoundary(repoRoot, workspaceName)
	if err != nil {
		return err
	}
	sourceEstimate, err := snapshotpayload.EstimateMaterializationCapacity(snapshotDir, snapshotpayload.OptionsFromDescriptor(desc))
	if err != nil {
		return err
	}

	workspaceBytes, err := capacitygate.TreeSize(mgrBoundary.Root, mgrBoundary.ExcludesRelativePath)
	if err != nil {
		return err
	}
	targetBackupBytes := workspaceBytes
	if plan.EffectiveScope() == restoreplan.ScopePath {
		targetBackupBytes, err = capacitygate.TreeSizeWithin(mgrBoundary.Root, plan.Path, mgrBoundary.ExcludesRelativePath)
		if err != nil {
			return err
		}
	}

	var saveFirstBytes int64
	if plan.Options.SaveFirst {
		saveFirstBytes = saturatingAdd(workspaceBytes, metadataFloor)
	}

	components := []capacitygate.Component{
		{Name: "source validation hash", Path: filepath.Join(os.TempDir(), "jvs-payload-hash-probe"), Bytes: sourceEstimate.PeakBytes},
		{Name: "restore hash", Path: filepath.Join(os.TempDir(), "jvs-payload-hash-restore-probe"), Bytes: sourceEstimate.PeakBytes},
		{Name: "source validation", Path: filepath.Join(repoRoot, repo.JVSDirName, "restore-preview-probe", "source"), Bytes: sourceEstimate.PeakBytes},
		{Name: "restore payload", Path: mgrBoundary.Root + ".restore-tmp-probe", Bytes: sourceEstimate.PeakBytes},
		{Name: "workspace backup", Path: mgrBoundary.Root + ".restore-backup-probe", Bytes: targetBackupBytes},
		{Name: "save-first safety save", Path: filepath.Join(repoRoot, repo.JVSDirName, "snapshots", "capacity-probe"), Bytes: saveFirstBytes},
		{Name: "metadata", Path: filepath.Join(repoRoot, repo.JVSDirName), Bytes: metadataFloor},
	}
	if plan.Options.SaveFirst {
		cfg, err := worktree.NewManager(repoRoot).Get(workspaceName)
		if err != nil {
			return err
		}
		if len(cfg.PathSources) > 0 {
			components, err = appendPathSourceReconcileCapacityComponents(repoRoot, cfg, components)
			if err != nil {
				return err
			}
		}
	}

	_, err = restoreRunCapacityGate.Check(capacitygate.Request{
		Operation:       "restore run",
		Folder:          plan.Folder,
		Workspace:       workspaceName,
		SourceSavePoint: string(plan.SourceSavePoint),
		Path:            plan.Path,
		Components:      components,
		FailureMessages: []string{"No save point was created.", "History was not changed.", "No files were changed."},
	})
	return err
}

func checkWorkspaceNewCapacity(repoRoot, workspaceName string, sourceID model.SnapshotID) error {
	mgr := worktree.NewManager(repoRoot)
	folder, err := mgr.PlannedStartedFromPath(workspaceName)
	if err != nil {
		return err
	}
	state, err := restoreplan.InspectSourceReadOnly(repoRoot, sourceID)
	if err != nil {
		return err
	}
	estimate, err := snapshotpayload.EstimateMaterializationCapacity(state.SnapshotDir, snapshotpayload.OptionsFromDescriptor(state.Descriptor))
	if err != nil {
		return err
	}

	components := []capacitygate.Component{
		{Name: "source hash", Path: filepath.Join(os.TempDir(), "jvs-payload-hash-workspace-new-source-probe"), Bytes: estimate.PeakBytes},
		{Name: "workspace folder", Path: folder, Bytes: estimate.PeakBytes},
		{Name: "workspace metadata", Path: filepath.Join(repoRoot, repo.JVSDirName, "worktrees", workspaceName), Bytes: metadataFloor},
	}
	_, err = workspaceNewCapacityGate.Check(capacitygate.Request{
		Operation:       "workspace new",
		Folder:          folder,
		Workspace:       workspaceName,
		SourceSavePoint: string(sourceID),
		Components:      components,
		FailureMessages: []string{"No workspace was created.", "No files were changed."},
	})
	return err
}

func restorePreviewSourceCapacityComponents(repoRoot string, sourceID model.SnapshotID) ([]capacitygate.Component, error) {
	state, err := restoreplan.InspectSourceReadOnly(repoRoot, sourceID)
	if err != nil {
		return nil, err
	}
	estimate, err := snapshotpayload.EstimateMaterializationCapacity(state.SnapshotDir, snapshotpayload.OptionsFromDescriptor(state.Descriptor))
	if err != nil {
		return nil, err
	}
	return []capacitygate.Component{
		{Name: "source hash", Path: filepath.Join(os.TempDir(), "jvs-payload-hash-probe"), Bytes: estimate.PeakBytes},
		{Name: "source validation", Path: filepath.Join(repoRoot, repo.JVSDirName, "restore-preview-probe", "source"), Bytes: estimate.PeakBytes},
		{Name: "impact preview", Path: filepath.Join(repoRoot, repo.JVSDirName, "restore-preview-probe", "impact"), Bytes: estimate.PeakBytes},
		{Name: "restore plan", Path: filepath.Join(repoRoot, repo.JVSDirName, "restore-plans"), Bytes: metadataFloor},
	}, nil
}

func appendExpectedWorkspaceCapacityComponents(repoRoot string, cfg *model.WorktreeConfig, components []capacitygate.Component) ([]capacitygate.Component, error) {
	expectedRoot := filepath.Join(os.TempDir(), "jvs-expected-workspace-probe", "expected")
	if cfg == nil || cfg.HeadSnapshotID == "" {
		components = append(components, capacitygate.Component{Name: "expected workspace", Path: expectedRoot, Bytes: 0})
	} else {
		var err error
		components, _, err = appendSavePointMaterializationCapacityComponents(repoRoot, cfg.HeadSnapshotID, components, "expected workspace", expectedRoot)
		if err != nil {
			return nil, err
		}
	}

	if cfg != nil {
		for _, source := range cfg.PathSources.RestoredPaths() {
			sourceRoot := filepath.Join(os.TempDir(), "jvs-expected-path-source-probe", "source")
			var estimate snapshotpayload.MaterializationCapacityEstimate
			var err error
			components, estimate, err = appendSavePointMaterializationCapacityComponents(repoRoot, source.SourceSnapshotID, components, "expected path source", sourceRoot)
			if err != nil {
				return nil, err
			}
			components = append(components, capacitygate.Component{
				Name:  "expected path overlay",
				Path:  filepath.Join(expectedRoot, filepath.FromSlash(source.TargetPath)),
				Bytes: estimate.PeakBytes,
			})
		}
	}
	return components, nil
}

func appendPathSourceReconcileCapacityComponents(repoRoot string, cfg *model.WorktreeConfig, components []capacitygate.Component) ([]capacitygate.Component, error) {
	reconcileRoot := filepath.Join(os.TempDir(), "jvs-path-source-reconcile-probe", "expected")
	components = append(components, capacitygate.Component{
		Name: "path source reconciliation",
		Path: reconcileRoot,
	})
	if cfg == nil {
		return components, nil
	}
	for _, source := range cfg.PathSources.RestoredPaths() {
		sourceRoot := filepath.Join(os.TempDir(), "jvs-expected-path-source-probe", "source")
		var estimate snapshotpayload.MaterializationCapacityEstimate
		var err error
		components, estimate, err = appendSavePointMaterializationCapacityComponents(repoRoot, source.SourceSnapshotID, components, "expected path source", sourceRoot)
		if err != nil {
			return nil, err
		}
		components = append(components, capacitygate.Component{
			Name:  "path source reconciliation overlay",
			Path:  filepath.Join(reconcileRoot, filepath.FromSlash(source.TargetPath)),
			Bytes: estimate.PeakBytes,
		})
	}
	return components, nil
}

func appendSavePointMaterializationCapacityComponents(repoRoot string, sourceID model.SnapshotID, components []capacitygate.Component, name, targetPath string) ([]capacitygate.Component, snapshotpayload.MaterializationCapacityEstimate, error) {
	state, err := restoreplan.InspectSourceReadOnly(repoRoot, sourceID)
	if err != nil {
		return nil, snapshotpayload.MaterializationCapacityEstimate{}, err
	}
	estimate, err := snapshotpayload.EstimateMaterializationCapacity(state.SnapshotDir, snapshotpayload.OptionsFromDescriptor(state.Descriptor))
	if err != nil {
		return nil, snapshotpayload.MaterializationCapacityEstimate{}, err
	}
	components = append(components,
		capacitygate.Component{Name: name + " hash", Path: filepath.Join(os.TempDir(), "jvs-payload-hash-"+name+"-probe"), Bytes: estimate.PeakBytes},
		capacitygate.Component{Name: name, Path: targetPath, Bytes: estimate.PeakBytes},
	)
	return components, estimate, nil
}

func saturatingAdd(a, b int64) int64 {
	if b > 0 && a > maxInt64-b {
		return maxInt64
	}
	return a + b
}

const maxInt64 = int64(^uint64(0) >> 1)
