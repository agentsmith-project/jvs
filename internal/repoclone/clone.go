// Package repoclone implements local JVS project cloning.
package repoclone

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/agentsmith-project/jvs/internal/capacitygate"
	"github.com/agentsmith-project/jvs/internal/clonehistory"
	"github.com/agentsmith-project/jvs/internal/doctor"
	"github.com/agentsmith-project/jvs/internal/engine"
	"github.com/agentsmith-project/jvs/internal/integrity"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/internal/snapshotpayload"
	"github.com/agentsmith-project/jvs/internal/transfer"
	"github.com/agentsmith-project/jvs/internal/workspacepath"
	"github.com/agentsmith-project/jvs/internal/worktree"
	"github.com/agentsmith-project/jvs/pkg/errclass"
	"github.com/agentsmith-project/jvs/pkg/fsutil"
	"github.com/agentsmith-project/jvs/pkg/model"
)

type SavePointsMode string

const (
	SavePointsModeAll  SavePointsMode = "all"
	SavePointsModeMain SavePointsMode = "main"
)

const operationRepoClone = "repo_clone"
const cloneMetadataFloor = 1 << 20

var cloneCapacityGate = capacitygate.Default()

// SetCapacityGateForTest installs a clone capacity gate for tests.
func SetCapacityGateForTest(gate capacitygate.Gate) func() {
	previous := cloneCapacityGate
	cloneCapacityGate = gate
	return func() {
		cloneCapacityGate = previous
	}
}

type Options struct {
	SourceRepoRoot  string
	TargetPath      string
	SavePointsMode  SavePointsMode
	DryRun          bool
	RequestedEngine model.EngineType
	TransferPlanner transfer.EnginePlanner
	Hooks           Hooks
}

type Hooks struct {
	BeforePublish func(stagingPath, targetPath string) error
}

type Result struct {
	Operation                  string             `json:"operation"`
	SourceRepoRoot             string             `json:"source_repo_root"`
	TargetRepoRoot             string             `json:"target_repo_root"`
	SourceRepoID               string             `json:"source_repo_id"`
	TargetRepoID               string             `json:"target_repo_id,omitempty"`
	SavePointsMode             SavePointsMode     `json:"save_points_mode"`
	SavePointsCopiedCount      int                `json:"save_points_copied_count"`
	SavePointsCopied           []model.SnapshotID `json:"save_points_copied"`
	WorkspacesCreated          []string           `json:"workspaces_created"`
	SourceWorkspacesNotCreated []string           `json:"source_workspaces_not_created"`
	RuntimeStateCopied         bool               `json:"runtime_state_copied"`
	CloneManifest              string             `json:"clone_manifest,omitempty"`
	DoctorStrict               string             `json:"doctor_strict,omitempty"`
	DryRun                     bool               `json:"dry_run,omitempty"`
	Transfers                  []transfer.Record  `json:"transfers"`
	NewestSavePoint            string             `json:"newest_save_point,omitempty"`
}

type preparedClone struct {
	options                    Options
	source                     *repo.Repo
	target                     string
	mode                       SavePointsMode
	sourceMain                 *model.WorktreeConfig
	sourceWorkspaces           []*model.WorktreeConfig
	sourceWorkspacesNotCreated []string
	savePoints                 []model.SnapshotID
}

type transferPlans struct {
	saveIntent transfer.Intent
	savePlan   *engine.TransferPlan
	mainIntent transfer.Intent
	mainPlan   *engine.TransferPlan
}

// Clone plans or executes a local JVS project clone.
func Clone(options Options) (*Result, error) {
	prepared, err := prepare(options)
	if err != nil {
		return nil, err
	}
	if prepared.options.DryRun {
		return prepared.dryRunResult()
	}
	return prepared.execute()
}

func prepare(options Options) (*preparedClone, error) {
	mode := options.SavePointsMode
	if mode == "" {
		mode = SavePointsModeAll
	}
	if mode != SavePointsModeAll && mode != SavePointsModeMain {
		return nil, errclass.ErrUsage.WithMessage("invalid --save-points value: use all or main")
	}
	options.SavePointsMode = mode

	if IsRemoteLikeInput(options.TargetPath) || IsRemoteLikeInput(options.SourceRepoRoot) {
		return nil, remoteLikeInputError()
	}

	source, err := discoverSource(options.SourceRepoRoot)
	if err != nil {
		return nil, err
	}
	target, err := normalizeTargetPath(options.TargetPath)
	if err != nil {
		return nil, err
	}

	wtMgr := worktree.NewManager(source.Root)
	workspaces, err := wtMgr.List()
	if err != nil {
		return nil, fmt.Errorf("cannot clone: source workspaces cannot be listed: %w", err)
	}
	sortWorkspaces(workspaces)
	if err := rejectTargetInsideSourceWorkspaces(source.Root, target, workspaces); err != nil {
		return nil, err
	}
	if err := validateTargetMissing(target); err != nil {
		return nil, err
	}
	mainCfg, err := wtMgr.Get("main")
	if err != nil {
		return nil, fmt.Errorf("cannot clone: source main workspace is not readable: %w", err)
	}
	if err := rejectDirtySourceWorkspaces(source.Root, workspaces); err != nil {
		return nil, err
	}

	savePoints, err := selectedSavePoints(source.Root, mode, mainCfg)
	if err != nil {
		return nil, err
	}
	if err := validateSelectedSavePoints(source.Root, savePoints); err != nil {
		return nil, err
	}

	return &preparedClone{
		options:                    options,
		source:                     source,
		target:                     target,
		mode:                       mode,
		sourceMain:                 mainCfg,
		sourceWorkspaces:           workspaces,
		sourceWorkspacesNotCreated: nonMainWorkspaceNames(workspaces),
		savePoints:                 savePoints,
	}, nil
}

func discoverSource(sourceRoot string) (*repo.Repo, error) {
	start := strings.TrimSpace(sourceRoot)
	if start == "" {
		start = "."
	}
	r, err := repo.Discover(start)
	if err != nil {
		return nil, fmt.Errorf("cannot clone: source is not a JVS project: %w", err)
	}
	root, err := filepath.Abs(r.Root)
	if err != nil {
		return nil, fmt.Errorf("resolve source repository: %w", err)
	}
	if physical, err := filepath.EvalSymlinks(root); err == nil {
		root = physical
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("resolve source repository: %w", err)
	}
	r.Root = filepath.Clean(root)
	return r, nil
}

func normalizeTargetPath(targetPath string) (string, error) {
	if strings.TrimSpace(targetPath) == "" {
		return "", errclass.ErrUsage.WithMessage("repo clone requires a target folder")
	}
	target, err := filepath.Abs(targetPath)
	if err != nil {
		return "", fmt.Errorf("resolve target folder: %w", err)
	}
	return filepath.Clean(target), nil
}

func validateTargetMissing(target string) error {
	if _, err := os.Lstat(target); err == nil {
		return errclass.ErrUsage.
			WithMessage("Cannot clone: target folder already exists.").
			WithHint("Choose a new folder path. JVS will not merge into an existing folder.")
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat target folder: %w", err)
	}
	return nil
}

func rejectTargetInsideSourceWorkspaces(repoRoot, target string, workspaces []*model.WorktreeConfig) error {
	targetLexical, targetPhysical, err := clonePathForms(target)
	if err != nil {
		return fmt.Errorf("resolve target folder: %w", err)
	}

	for _, cfg := range workspaces {
		if cfg == nil {
			continue
		}
		boundary, err := repo.WorktreeManagedPayloadBoundary(repoRoot, cfg.Name)
		if err != nil {
			return fmt.Errorf("cannot clone: source workspace %q path cannot be inspected: %w", cfg.Name, err)
		}
		rootLexical, rootPhysical, err := clonePathForms(boundary.Root)
		if err != nil {
			return fmt.Errorf("cannot clone: source workspace %q path cannot be inspected: %w", cfg.Name, err)
		}
		if pathInsideAnyRoot([]string{rootLexical, rootPhysical}, []string{targetLexical, targetPhysical}) {
			return targetInsideSourceWorkspaceError()
		}
	}
	return nil
}

func targetInsideSourceWorkspaceError() error {
	return errclass.ErrUsage.
		WithMessage("Cannot clone: target cannot be inside a source workspace.").
		WithHint("Choose a folder outside the source project/workspaces to keep the source unchanged.")
}

func clonePathForms(path string) (lexical, physical string, err error) {
	lexical, err = filepath.Abs(path)
	if err != nil {
		return "", "", err
	}
	lexical = filepath.Clean(lexical)
	physical, err = physicalPathForPossiblyMissingPath(lexical)
	if err != nil {
		return "", "", err
	}
	return lexical, physical, nil
}

func physicalPathForPossiblyMissingPath(path string) (string, error) {
	clean := filepath.Clean(path)
	resolved, err := filepath.EvalSymlinks(clean)
	if err == nil {
		return filepath.Clean(resolved), nil
	}
	if !os.IsNotExist(err) {
		return "", fmt.Errorf("resolve symlinks for %s: %w", clean, err)
	}

	ancestor := clean
	var suffix []string
	for {
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			return "", fmt.Errorf("no existing ancestor for %s", clean)
		}
		suffix = append([]string{filepath.Base(ancestor)}, suffix...)
		ancestor = parent

		resolvedAncestor, err := filepath.EvalSymlinks(ancestor)
		if err == nil {
			parts := append([]string{filepath.Clean(resolvedAncestor)}, suffix...)
			return filepath.Clean(filepath.Join(parts...)), nil
		}
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("resolve symlinks for existing ancestor %s: %w", ancestor, err)
		}
	}
}

func pathInsideAnyRoot(roots, paths []string) bool {
	uniqueRoots := appendUniqueStrings(nil, roots...)
	uniquePaths := appendUniqueStrings(nil, paths...)
	for _, root := range uniqueRoots {
		for _, path := range uniquePaths {
			if pathInsideOrEqual(root, path) {
				return true
			}
		}
	}
	return false
}

func pathInsideOrEqual(root, path string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && !filepath.IsAbs(rel))
}

func rejectDirtySourceWorkspaces(repoRoot string, workspaces []*model.WorktreeConfig) error {
	for _, cfg := range workspaces {
		if cfg == nil {
			continue
		}
		dirty, err := workspaceDirty(repoRoot, cfg.Name)
		if err != nil {
			return fmt.Errorf("cannot clone: source workspace %q cannot be inspected: %w", cfg.Name, err)
		}
		if dirty {
			return &errclass.JVSError{
				Code:    "E_UNSAVED_CHANGES",
				Message: fmt.Sprintf("Cannot clone: source workspace %q has unsaved changes.", cfg.Name),
				Hint:    "Save those changes as a save point first if you want them included. JVS repo clone only creates target workspace \"main\".",
			}
		}
	}
	return nil
}

func selectedSavePoints(repoRoot string, mode SavePointsMode, mainCfg *model.WorktreeConfig) ([]model.SnapshotID, error) {
	switch mode {
	case SavePointsModeAll:
		return allReadySavePoints(repoRoot)
	case SavePointsModeMain:
		return mainClosureSavePoints(repoRoot, mainCfg)
	default:
		return nil, fmt.Errorf("unsupported save points mode %q", mode)
	}
}

func allReadySavePoints(repoRoot string) ([]model.SnapshotID, error) {
	entries, err := snapshot.ListCatalogEntries(repoRoot)
	if err != nil {
		return nil, err
	}
	ids := make([]model.SnapshotID, 0, len(entries))
	for _, entry := range entries {
		if entry.DescriptorErr != nil {
			return nil, fmt.Errorf("save point %s descriptor is not readable: %w", entry.SnapshotID, entry.DescriptorErr)
		}
		ids = append(ids, entry.SnapshotID)
	}
	sortSnapshotIDs(ids)
	return ids, nil
}

func mainClosureSavePoints(repoRoot string, mainCfg *model.WorktreeConfig) ([]model.SnapshotID, error) {
	seen := make(map[model.SnapshotID]bool)
	var ids []model.SnapshotID
	var queue []model.SnapshotID
	enqueue := func(id model.SnapshotID) {
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		queue = append(queue, id)
	}

	if mainCfg != nil {
		enqueue(mainCfg.HeadSnapshotID)
		enqueue(mainCfg.LatestSnapshotID)
		enqueue(mainCfg.BaseSnapshotID)
		enqueue(mainCfg.StartedFromSnapshotID)
		for _, source := range mainCfg.PathSources {
			enqueue(source.SourceSnapshotID)
		}
	}

	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if err := validateReadySavePoint(repoRoot, id); err != nil {
			return nil, err
		}
		desc, err := snapshot.LoadDescriptor(repoRoot, id)
		if err != nil {
			return nil, fmt.Errorf("load save point %s: %w", id, err)
		}
		ids = append(ids, id)
		if desc.ParentID != nil {
			enqueue(*desc.ParentID)
		}
		if desc.StartedFrom != nil {
			enqueue(*desc.StartedFrom)
		}
		if desc.RestoredFrom != nil {
			enqueue(*desc.RestoredFrom)
		}
		for _, restored := range desc.RestoredPaths {
			enqueue(restored.SourceSnapshotID)
		}
	}
	sortSnapshotIDs(ids)
	return ids, nil
}

func validateSelectedSavePoints(repoRoot string, ids []model.SnapshotID) error {
	for _, id := range ids {
		if err := validateReadySavePoint(repoRoot, id); err != nil {
			return err
		}
	}
	return nil
}

func validateReadySavePoint(repoRoot string, id model.SnapshotID) error {
	_, issue := snapshot.InspectPublishState(repoRoot, id, snapshot.PublishStateOptions{
		RequireReady:             true,
		RequirePayload:           true,
		VerifyDescriptorChecksum: true,
		VerifyPayloadHash:        true,
	})
	if issue != nil {
		return fmt.Errorf("save point %s is not ready: %s", id, issue.Message)
	}
	return nil
}

func (p *preparedClone) dryRunResult() (*Result, error) {
	mainBoundary, err := repo.WorktreeManagedPayloadBoundary(p.source.Root, "main")
	if err != nil {
		return nil, fmt.Errorf("source main workspace path: %w", err)
	}
	intents := p.transferIntents(p.target, mainBoundary, transfer.ResultKindExpected, transfer.PermissionScopePreviewOnly)
	records, err := p.planTransferRecords(intents)
	if err != nil {
		return nil, err
	}
	result := p.baseResult()
	result.DryRun = true
	result.Transfers = records
	return result, nil
}

func (p *preparedClone) execute() (*Result, error) {
	parent := filepath.Dir(p.target)
	if _, err := os.Stat(parent); err != nil {
		return nil, fmt.Errorf("stat target parent: %w", err)
	}
	mainBoundary, err := repo.WorktreeManagedPayloadBoundary(p.source.Root, "main")
	if err != nil {
		return nil, fmt.Errorf("source main workspace path: %w", err)
	}
	if err := p.checkCapacity(mainBoundary); err != nil {
		return nil, err
	}

	staging, err := os.MkdirTemp(parent, "."+filepath.Base(p.target)+".clone-staging-")
	if err != nil {
		return nil, fmt.Errorf("create clone staging: %w", err)
	}
	published := false
	defer func() {
		if !published {
			_ = os.RemoveAll(staging)
		}
	}()

	targetRepo, err := repo.InitAdoptedWorkspace(staging)
	if err != nil {
		return nil, fmt.Errorf("initialize target staging repo: %w", err)
	}

	intents := p.transferIntents(staging, mainBoundary, transfer.ResultKindFinal, transfer.PermissionScopeExecution)
	plans, records, err := p.planTransfers(intents)
	if err != nil {
		return nil, err
	}

	if err := p.copySavePointStorage(staging, plans.savePlan, intents.saveIntent, &records[0]); err != nil {
		return nil, err
	}
	if err := p.copyMainWorkspace(mainBoundary, staging, plans.mainPlan, intents.mainIntent, &records[1]); err != nil {
		return nil, err
	}
	if err := writeTargetMainConfig(staging, p.sourceMain, staging); err != nil {
		return nil, err
	}
	if p.mode == SavePointsModeAll {
		if err := clonehistory.WriteManifest(staging, clonehistory.Manifest{
			SchemaVersion:      clonehistory.ManifestSchemaVersion,
			Operation:          clonehistory.OperationRepoClone,
			SourceRepoID:       p.source.RepoID,
			TargetRepoID:       targetRepo.RepoID,
			SavePointsMode:     clonehistory.SavePointsModeAll,
			RuntimeStateCopied: false,
			ProtectionReason:   model.GCProtectionReasonImportedCloneHistory,
			ImportedSavePoints: p.savePoints,
		}); err != nil {
			return nil, fmt.Errorf("write imported clone history manifest: %w", err)
		}
	}
	if err := validateCopiedSavePoints(staging, p.savePoints); err != nil {
		return nil, err
	}
	if err := checkDoctorStrict(staging); err != nil {
		return nil, fmt.Errorf("doctor strict before publish: %w", err)
	}
	if err := writeTargetMainConfig(staging, p.sourceMain, p.target); err != nil {
		return nil, err
	}
	if p.options.Hooks.BeforePublish != nil {
		if err := p.options.Hooks.BeforePublish(staging, p.target); err != nil {
			return nil, fmt.Errorf("before publish hook: %w", err)
		}
	}
	if err := fsutil.RenameNoReplaceAndSync(staging, p.target); err != nil {
		return nil, fmt.Errorf("publish clone target: %w", err)
	}
	published = true

	if err := checkDoctorStrict(p.target); err != nil {
		return nil, fmt.Errorf("doctor strict after publish: %w", err)
	}

	result := p.baseResult()
	result.TargetRepoID = targetRepo.RepoID
	result.DoctorStrict = "passed"
	result.Transfers = records
	if p.mode == SavePointsModeAll {
		result.CloneManifest = clonehistory.ManifestPath(p.target)
	}
	return result, nil
}

func (p *preparedClone) checkCapacity(mainBoundary repo.WorktreePayloadBoundary) error {
	mainBytes, err := capacitygate.TreeSize(mainBoundary.Root, mainBoundary.ExcludesRelativePath)
	if err != nil {
		return fmt.Errorf("estimate source main workspace size: %w", err)
	}
	savePointBytes, err := p.selectedSavePointStorageBytes()
	if err != nil {
		return err
	}

	_, err = cloneCapacityGate.Check(capacitygate.Request{
		Operation: "repo clone",
		Folder:    p.target,
		Workspace: "main",
		Components: []capacitygate.Component{
			{Name: "repo clone save point storage", Path: filepath.Join(p.target, repo.JVSDirName, "snapshots"), Bytes: savePointBytes},
			{Name: "repo clone main workspace", Path: p.target, Bytes: mainBytes},
			{Name: "repo clone control metadata", Path: filepath.Join(p.target, repo.JVSDirName), Bytes: cloneMetadataFloor},
		},
		FailureMessages: []string{"Source was not changed.", "Target was not created."},
	})
	return err
}

func (p *preparedClone) selectedSavePointStorageBytes() (int64, error) {
	var total int64
	for _, id := range p.savePoints {
		snapshotDir, err := repo.SnapshotPathForRead(p.source.Root, id)
		if err != nil {
			return 0, fmt.Errorf("source save point payload %s: %w", id, err)
		}
		bytes, err := capacitygate.TreeSize(snapshotDir, nil)
		if err != nil {
			return 0, fmt.Errorf("estimate save point payload %s size: %w", id, err)
		}
		total = saturatingAddInt64(total, bytes)

		descriptorPath, err := repo.SnapshotDescriptorPathForRead(p.source.Root, id)
		if err != nil {
			return 0, fmt.Errorf("source save point descriptor %s: %w", id, err)
		}
		bytes, err = capacitygate.TreeSize(descriptorPath, nil)
		if err != nil {
			return 0, fmt.Errorf("estimate save point descriptor %s size: %w", id, err)
		}
		total = saturatingAddInt64(total, bytes)
	}
	return total, nil
}

func saturatingAddInt64(left, right int64) int64 {
	if right <= 0 {
		return left
	}
	if left > (1<<63)-1-right {
		return (1 << 63) - 1
	}
	return left + right
}

func (p *preparedClone) baseResult() *Result {
	result := &Result{
		Operation:                  operationRepoClone,
		SourceRepoRoot:             p.source.Root,
		TargetRepoRoot:             p.target,
		SourceRepoID:               p.source.RepoID,
		SavePointsMode:             p.mode,
		SavePointsCopied:           append([]model.SnapshotID(nil), p.savePoints...),
		SavePointsCopiedCount:      len(p.savePoints),
		WorkspacesCreated:          []string{"main"},
		SourceWorkspacesNotCreated: append([]string(nil), p.sourceWorkspacesNotCreated...),
		RuntimeStateCopied:         false,
		Transfers:                  []transfer.Record{},
	}
	if p.sourceMain != nil && p.sourceMain.LatestSnapshotID != "" {
		result.NewestSavePoint = string(p.sourceMain.LatestSnapshotID)
	}
	return result
}

func (p *preparedClone) transferIntents(materializationRoot string, mainBoundary repo.WorktreePayloadBoundary, kind transfer.ResultKind, scope transfer.PermissionScope) transferPlans {
	saveDestination := filepath.Join(materializationRoot, repo.JVSDirName)
	savePublished := filepath.Join(p.target, repo.JVSDirName)
	if kind == transfer.ResultKindExpected {
		saveDestination = savePublished
	}
	mainDestination := materializationRoot
	mainPublished := p.target
	if kind == transfer.ResultKindExpected {
		mainDestination = p.target
	}
	return transferPlans{
		saveIntent: transfer.Intent{
			TransferID:                 "repo-clone-save-points",
			Operation:                  operationRepoClone,
			Phase:                      "save_point_storage_copy",
			Primary:                    true,
			ResultKind:                 kind,
			PermissionScope:            scope,
			SourceRole:                 "save_point_storage",
			SourcePath:                 filepath.Join(p.source.Root, repo.JVSDirName),
			DestinationRole:            "target_save_point_storage",
			MaterializationDestination: saveDestination,
			CapabilityProbePath:        saveDestination,
			PublishedDestination:       savePublished,
			RequestedEngine:            p.options.RequestedEngine,
		},
		mainIntent: transfer.Intent{
			TransferID:                 "repo-clone-main-workspace",
			Operation:                  operationRepoClone,
			Phase:                      "main_workspace_materialization",
			Primary:                    true,
			ResultKind:                 kind,
			PermissionScope:            scope,
			SourceRole:                 "source_main_current_state",
			SourcePath:                 mainBoundary.Root,
			DestinationRole:            "target_main_workspace",
			MaterializationDestination: mainDestination,
			CapabilityProbePath:        filepath.Dir(p.target),
			PublishedDestination:       mainPublished,
			RequestedEngine:            p.options.RequestedEngine,
		},
	}
}

func (p *preparedClone) planTransferRecords(intents transferPlans) ([]transfer.Record, error) {
	_, records, err := p.planTransfers(intents)
	return records, err
}

func (p *preparedClone) planTransfers(intents transferPlans) (transferPlans, []transfer.Record, error) {
	savePlan, err := transfer.PlanIntent(p.options.TransferPlanner, intents.saveIntent)
	if err != nil {
		return transferPlans{}, nil, fmt.Errorf("plan save point storage transfer: %w", err)
	}
	mainPlan, err := transfer.PlanIntent(p.options.TransferPlanner, intents.mainIntent)
	if err != nil {
		return transferPlans{}, nil, fmt.Errorf("plan main workspace transfer: %w", err)
	}
	plans := transferPlans{
		saveIntent: intents.saveIntent,
		savePlan:   savePlan,
		mainIntent: intents.mainIntent,
		mainPlan:   mainPlan,
	}
	records := []transfer.Record{
		transfer.RecordFromPlanAndRuntime(intents.saveIntent, savePlan, nil),
		transfer.RecordFromPlanAndRuntime(intents.mainIntent, mainPlan, nil),
	}
	return plans, records, nil
}

func (p *preparedClone) copySavePointStorage(staging string, plan *engine.TransferPlan, intent transfer.Intent, record *transfer.Record) error {
	eng := engine.NewEngine(plan.TransferEngine)
	runtime := engine.NewCloneResult(plan.TransferEngine)
	for _, id := range p.savePoints {
		srcSnapshot, err := repo.SnapshotPathForRead(p.source.Root, id)
		if err != nil {
			return fmt.Errorf("source save point payload %s: %w", id, err)
		}
		dstSnapshot, err := repo.SnapshotPath(staging, id)
		if err != nil {
			return err
		}
		result, err := engine.CloneToNew(eng, srcSnapshot, dstSnapshot)
		if err != nil {
			return fmt.Errorf("copy save point payload %s: %w", id, err)
		}
		mergeCloneResult(runtime, result)

		srcDescriptor, err := repo.SnapshotDescriptorPathForRead(p.source.Root, id)
		if err != nil {
			return fmt.Errorf("source save point descriptor %s: %w", id, err)
		}
		dstDescriptor, err := repo.SnapshotDescriptorPath(staging, id)
		if err != nil {
			return err
		}
		result, err = engine.CloneToNew(eng, srcDescriptor, dstDescriptor)
		if err != nil {
			return fmt.Errorf("copy save point descriptor %s: %w", id, err)
		}
		mergeCloneResult(runtime, result)
	}
	if err := fsutil.FsyncDir(filepath.Join(staging, repo.JVSDirName)); err != nil {
		return fmt.Errorf("fsync save point storage: %w", err)
	}
	*record = transfer.RecordFromPlanAndRuntime(intent, plan, runtime)
	return nil
}

func (p *preparedClone) copyMainWorkspace(boundary repo.WorktreePayloadBoundary, staging string, plan *engine.TransferPlan, intent transfer.Intent, record *transfer.Record) error {
	eng := engine.NewEngine(plan.TransferEngine)
	runtime := engine.NewCloneResult(plan.TransferEngine)
	entries, err := os.ReadDir(boundary.Root)
	if err != nil {
		return fmt.Errorf("read source main workspace: %w", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if boundary.ExcludesRelativePath(name) {
			continue
		}
		result, err := engine.CloneToNew(eng, filepath.Join(boundary.Root, name), filepath.Join(staging, name))
		if err != nil {
			return fmt.Errorf("copy main workspace entry %s: %w", name, err)
		}
		mergeCloneResult(runtime, result)
	}
	if err := fsutil.FsyncDir(staging); err != nil {
		return fmt.Errorf("fsync main workspace: %w", err)
	}
	*record = transfer.RecordFromPlanAndRuntime(intent, plan, runtime)
	return nil
}

func mergeCloneResult(combined, next *engine.CloneResult) {
	if combined == nil || next == nil {
		return
	}
	if next.ActualEngine != "" {
		combined.ActualEngine = next.ActualEngine
	}
	if next.EffectiveEngine == model.EngineCopy || combined.EffectiveEngine == "" {
		combined.EffectiveEngine = next.EffectiveEngine
		combined.MetadataPreservation = engine.MetadataPreservationForEngine(next.EffectiveEngine)
		combined.PerformanceClass = engine.PerformanceClassForEngine(next.EffectiveEngine)
	}
	if next.Degraded {
		combined.Degraded = true
	}
	combined.Degradations = appendUniqueStrings(combined.Degradations, next.Degradations...)
}

func writeTargetMainConfig(repoRoot string, source *model.WorktreeConfig, realPath string) error {
	cfg := &model.WorktreeConfig{
		Name:        "main",
		RealPath:    realPath,
		CreatedAt:   time.Now().UTC(),
		PathSources: model.NewPathSources(),
	}
	if source != nil {
		cfg.BaseSnapshotID = source.BaseSnapshotID
		cfg.HeadSnapshotID = source.HeadSnapshotID
		cfg.LatestSnapshotID = source.LatestSnapshotID
		cfg.StartedFromSnapshotID = source.StartedFromSnapshotID
		cfg.PathSources = source.PathSources.Clone()
	}
	return repo.WriteWorktreeConfig(repoRoot, "main", cfg)
}

func validateCopiedSavePoints(repoRoot string, ids []model.SnapshotID) error {
	for _, id := range ids {
		if err := validateReadySavePoint(repoRoot, id); err != nil {
			return fmt.Errorf("copied %w", err)
		}
	}
	return nil
}

func checkDoctorStrict(repoRoot string) error {
	result, err := doctor.NewDoctor(repoRoot).Check(true)
	if err != nil {
		return err
	}
	if result.Healthy {
		return nil
	}
	if len(result.Findings) == 0 {
		return fmt.Errorf("repository is unhealthy")
	}
	return fmt.Errorf("%s: %s", result.Findings[0].ErrorCode, result.Findings[0].Description)
}

func workspaceDirty(repoRoot, workspaceName string) (bool, error) {
	mgr := worktree.NewManager(repoRoot)
	cfg, err := mgr.Get(workspaceName)
	if err != nil {
		return false, fmt.Errorf("load workspace: %w", err)
	}
	boundary, err := repo.WorktreeManagedPayloadBoundary(repoRoot, workspaceName)
	if err != nil {
		return false, fmt.Errorf("workspace path: %w", err)
	}
	if err := snapshotpayload.CheckReservedWorkspacePayloadRoot(boundary.Root); err != nil {
		return false, err
	}

	if cfg.HeadSnapshotID == "" {
		return workspaceHasManagedContent(boundary)
	}

	if len(cfg.PathSources) > 0 {
		expectedRoot, cleanup, err := workspacepath.MaterializeExpectedWorkspace(repoRoot, cfg, boundary)
		if err != nil {
			return false, err
		}
		defer cleanup()
		matches, err := workspacepath.ManagedPathEqual(boundary.Root, expectedRoot, "", boundary.ExcludesRelativePath)
		if err != nil {
			return false, fmt.Errorf("compare workspace to known sources: %w", err)
		}
		return !matches, nil
	}

	desc, err := snapshot.LoadDescriptor(repoRoot, cfg.HeadSnapshotID)
	if err != nil {
		return false, fmt.Errorf("load content source save point: %w", err)
	}
	if len(desc.PartialPaths) > 0 {
		return true, nil
	}

	hash, err := integrity.ComputePayloadRootHashWithExclusions(boundary.Root, boundary.ExcludesRelativePath)
	if err != nil {
		return false, fmt.Errorf("hash workspace: %w", err)
	}
	return hash != desc.PayloadRootHash, nil
}

func workspaceHasManagedContent(boundary repo.WorktreePayloadBoundary) (bool, error) {
	hasContent := false
	err := filepath.Walk(boundary.Root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path == boundary.Root {
			return nil
		}
		rel, err := filepath.Rel(boundary.Root, path)
		if err != nil {
			return fmt.Errorf("relative path: %w", err)
		}
		if boundary.ExcludesRelativePath(rel) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		hasContent = true
		return filepath.SkipDir
	})
	if err != nil {
		return false, fmt.Errorf("scan workspace: %w", err)
	}
	return hasContent, nil
}

func sortWorkspaces(workspaces []*model.WorktreeConfig) {
	sort.Slice(workspaces, func(i, j int) bool {
		left, right := "", ""
		if workspaces[i] != nil {
			left = workspaces[i].Name
		}
		if workspaces[j] != nil {
			right = workspaces[j].Name
		}
		return left < right
	})
}

func nonMainWorkspaceNames(workspaces []*model.WorktreeConfig) []string {
	var names []string
	for _, cfg := range workspaces {
		if cfg == nil || cfg.Name == "main" {
			continue
		}
		names = append(names, cfg.Name)
	}
	sort.Strings(names)
	return names
}

func sortSnapshotIDs(ids []model.SnapshotID) {
	sort.Slice(ids, func(i, j int) bool {
		return string(ids[i]) < string(ids[j])
	})
}

func appendUniqueStrings(base []string, values ...string) []string {
	seen := make(map[string]bool, len(base)+len(values))
	out := make([]string, 0, len(base)+len(values))
	for _, value := range append(base, values...) {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func remoteLikeInputError() error {
	return errclass.ErrUsage.
		WithMessage("JVS repo clone only copies a local or mounted JVS project.").
		WithHint("Remote URLs and git-style sources are not supported. Use a local path with --repo, then provide the target folder.")
}

var remoteSchemePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*://`)
var remoteSCPPattern = regexp.MustCompile(`^[^/@\\\s]+@[^/:\\\s]+:.+`)

// IsRemoteLikeInput reports remote URL and git/scp-style inputs while leaving
// ordinary local paths, including Windows drive paths, alone.
func IsRemoteLikeInput(input string) bool {
	value := strings.TrimSpace(input)
	if value == "" {
		return false
	}
	if remoteSchemePattern.MatchString(value) {
		return true
	}
	if isWindowsDrivePath(value) {
		return false
	}
	return remoteSCPPattern.MatchString(value) || isHostOnlySCPInput(value)
}

func isWindowsDrivePath(value string) bool {
	if len(value) < 3 {
		return false
	}
	first := value[0]
	return ((first >= 'A' && first <= 'Z') || (first >= 'a' && first <= 'z')) &&
		value[1] == ':' &&
		(value[2] == '\\' || value[2] == '/')
}

func isHostOnlySCPInput(value string) bool {
	if strings.ContainsAny(value, " \t\r\n") {
		return false
	}
	colon := strings.IndexByte(value, ':')
	if colon <= 0 || colon == len(value)-1 {
		return false
	}
	host := value[:colon]
	path := value[colon+1:]
	if len(host) == 1 && isASCIIAlpha(host[0]) {
		return false
	}
	if strings.ContainsAny(host, `/\`) || strings.Contains(path, `\`) {
		return false
	}
	if strings.Contains(path, "/") {
		return true
	}
	return strings.Contains(host, ".")
}

func isASCIIAlpha(value byte) bool {
	return (value >= 'A' && value <= 'Z') || (value >= 'a' && value <= 'z')
}
