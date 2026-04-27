// Package restoreplan builds and persists preview plans for destructive
// whole-workspace restore operations.
package restoreplan

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/agentsmith-project/jvs/internal/engine"
	"github.com/agentsmith-project/jvs/internal/integrity"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/internal/snapshotpayload"
	"github.com/agentsmith-project/jvs/internal/worktree"
	"github.com/agentsmith-project/jvs/pkg/fsutil"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/agentsmith-project/jvs/pkg/uuidutil"
)

const (
	SchemaVersion = 1
	sampleLimit   = 5
)

type Options struct {
	DiscardUnsaved bool `json:"discard_unsaved,omitempty"`
	SaveFirst      bool `json:"save_first,omitempty"`
}

type ChangeSummary struct {
	Count   int      `json:"count"`
	Samples []string `json:"samples,omitempty"`
}

type ManagedFilesImpact struct {
	Overwrite ChangeSummary `json:"overwrite"`
	Delete    ChangeSummary `json:"delete"`
	Create    ChangeSummary `json:"create"`
}

type Plan struct {
	SchemaVersion           int                `json:"schema_version"`
	RepoID                  string             `json:"repo_id"`
	PlanID                  string             `json:"plan_id"`
	CreatedAt               time.Time          `json:"created_at"`
	Folder                  string             `json:"folder"`
	Workspace               string             `json:"workspace"`
	SourceSavePoint         model.SnapshotID   `json:"source_save_point"`
	NewestSavePoint         *model.SnapshotID  `json:"newest_save_point"`
	HistoryHead             *model.SnapshotID  `json:"history_head"`
	ExpectedNewestSavePoint *model.SnapshotID  `json:"expected_newest_save_point"`
	ExpectedFolderEvidence  string             `json:"expected_folder_evidence"`
	Options                 Options            `json:"options,omitempty"`
	ManagedFiles            ManagedFilesImpact `json:"managed_files"`
	RunCommand              string             `json:"run_command"`
}

func Create(repoRoot, workspaceName string, sourceID model.SnapshotID, engineType model.EngineType, options Options) (*Plan, error) {
	if options.DiscardUnsaved && options.SaveFirst {
		return nil, fmt.Errorf("--discard-unsaved and --save-first cannot be used together")
	}
	if sourceID == "" {
		return nil, fmt.Errorf("source save point is required")
	}
	if err := sourceID.Validate(); err != nil {
		return nil, fmt.Errorf("source save point: %w", err)
	}

	repoID, err := currentRepoID(repoRoot)
	if err != nil {
		return nil, err
	}
	mgr := worktree.NewManager(repoRoot)
	cfg, err := mgr.Get(workspaceName)
	if err != nil {
		return nil, fmt.Errorf("load workspace: %w", err)
	}
	folder, err := mgr.Path(workspaceName)
	if err != nil {
		return nil, fmt.Errorf("workspace folder: %w", err)
	}
	evidence, err := WorkspaceEvidence(repoRoot, workspaceName)
	if err != nil {
		return nil, err
	}
	impact, err := ComputeManagedImpact(repoRoot, workspaceName, sourceID, engineType)
	if err != nil {
		return nil, err
	}

	expectedNewest := snapshotIDPtrOrNil(cfg.LatestSnapshotID)
	planID := uuidutil.NewV4()
	plan := &Plan{
		SchemaVersion:           SchemaVersion,
		RepoID:                  repoID,
		PlanID:                  planID,
		CreatedAt:               time.Now().UTC(),
		Folder:                  folder,
		Workspace:               workspaceName,
		SourceSavePoint:         sourceID,
		NewestSavePoint:         cloneSnapshotIDPtr(expectedNewest),
		HistoryHead:             cloneSnapshotIDPtr(expectedNewest),
		ExpectedNewestSavePoint: cloneSnapshotIDPtr(expectedNewest),
		ExpectedFolderEvidence:  evidence,
		Options:                 options,
		ManagedFiles:            impact,
		RunCommand:              "jvs restore --run " + planID,
	}
	if err := Write(repoRoot, plan); err != nil {
		return nil, err
	}
	return plan, nil
}

func Write(repoRoot string, plan *Plan) error {
	if plan == nil {
		return fmt.Errorf("restore plan is required")
	}
	if err := os.MkdirAll(filepath.Join(repoRoot, repo.JVSDirName, "restore-plans"), 0755); err != nil {
		return fmt.Errorf("create restore plan directory: %w", err)
	}
	path, err := repo.RestorePlanPathForWrite(repoRoot, plan.PlanID)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal restore plan: %w", err)
	}
	return fsutil.AtomicWrite(path, data, 0644)
}

func Load(repoRoot, planID string) (*Plan, error) {
	path, err := repo.RestorePlanPathForRead(repoRoot, planID)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("restore plan %q not found", planID)
		}
		return nil, fmt.Errorf("restore plan %q is not readable", planID)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("restore plan %q not found", planID)
		}
		return nil, fmt.Errorf("restore plan %q is not readable", planID)
	}
	var plan Plan
	if err := json.Unmarshal(data, &plan); err != nil {
		return nil, fmt.Errorf("restore plan %q is not valid JSON", planID)
	}
	if plan.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("restore plan %q has unsupported schema version", planID)
	}
	if plan.PlanID != planID {
		return nil, fmt.Errorf("restore plan %q plan_id does not match request", planID)
	}
	repoID, err := currentRepoID(repoRoot)
	if err != nil {
		return nil, err
	}
	if plan.RepoID != repoID {
		return nil, fmt.Errorf("restore plan %q belongs to a different repository", planID)
	}
	return &plan, nil
}

func ValidateTarget(repoRoot, workspaceName string, plan *Plan) error {
	if plan == nil {
		return fmt.Errorf("restore plan is required")
	}
	if plan.Workspace != workspaceName {
		return changedSincePreviewError()
	}
	mgr := worktree.NewManager(repoRoot)
	cfg, err := mgr.Get(workspaceName)
	if err != nil {
		return fmt.Errorf("load workspace: %w", err)
	}
	folder, err := mgr.Path(workspaceName)
	if err != nil {
		return fmt.Errorf("workspace folder: %w", err)
	}
	if folder != plan.Folder {
		return changedSincePreviewError()
	}
	currentNewest := snapshotIDPtrOrNil(cfg.LatestSnapshotID)
	if !sameSnapshotIDPtr(currentNewest, plan.ExpectedNewestSavePoint) {
		return changedSincePreviewError()
	}
	evidence, err := WorkspaceEvidence(repoRoot, workspaceName)
	if err != nil {
		return err
	}
	if evidence != plan.ExpectedFolderEvidence {
		return changedSincePreviewError()
	}
	return nil
}

func ValidateSource(repoRoot, workspaceName string, plan *Plan, engineType model.EngineType) error {
	if plan == nil {
		return fmt.Errorf("restore plan is required")
	}
	if plan.SourceSavePoint == "" {
		return sourceNotRestorableError(fmt.Errorf("source save point is required"))
	}
	if err := snapshot.VerifySnapshot(repoRoot, plan.SourceSavePoint, true); err != nil {
		return sourceNotRestorableError(err)
	}
	desc, err := snapshot.LoadDescriptor(repoRoot, plan.SourceSavePoint)
	if err != nil {
		return sourceNotRestorableError(err)
	}
	if desc.SnapshotID != plan.SourceSavePoint {
		return sourceNotRestorableError(fmt.Errorf("descriptor save point ID %s does not match requested %s", desc.SnapshotID, plan.SourceSavePoint))
	}
	boundary, err := repo.WorktreeManagedPayloadBoundary(repoRoot, workspaceName)
	if err != nil {
		return sourceNotRestorableError(err)
	}
	sourceRoot, cleanup, err := materializeSource(repoRoot, plan.SourceSavePoint, desc, engineType)
	if err != nil {
		return sourceNotRestorableError(err)
	}
	defer cleanup()
	if err := snapshotpayload.CheckReservedWorkspacePayloadRoot(sourceRoot); err != nil {
		return sourceNotRestorableError(err)
	}
	if err := repo.ValidateManagedPayloadOnly(boundary, sourceRoot); err != nil {
		return sourceNotRestorableError(err)
	}
	return nil
}

func WorkspaceEvidence(repoRoot, workspaceName string) (string, error) {
	boundary, err := repo.WorktreeManagedPayloadBoundary(repoRoot, workspaceName)
	if err != nil {
		return "", fmt.Errorf("workspace path: %w", err)
	}
	if err := snapshotpayload.CheckReservedWorkspacePayloadRoot(boundary.Root); err != nil {
		return "", err
	}
	hash, err := integrity.ComputePayloadRootHashWithExclusions(boundary.Root, boundary.ExcludesRelativePath)
	if err != nil {
		return "", fmt.Errorf("scan folder evidence: %w", err)
	}
	return string(hash), nil
}

func ComputeManagedImpact(repoRoot, workspaceName string, sourceID model.SnapshotID, engineType model.EngineType) (ManagedFilesImpact, error) {
	desc, err := snapshot.LoadDescriptor(repoRoot, sourceID)
	if err != nil {
		return ManagedFilesImpact{}, fmt.Errorf("load source save point: %w", err)
	}
	if desc.SnapshotID != sourceID {
		return ManagedFilesImpact{}, fmt.Errorf("load source save point: descriptor save point ID %s does not match requested %s", desc.SnapshotID, sourceID)
	}
	boundary, err := repo.WorktreeManagedPayloadBoundary(repoRoot, workspaceName)
	if err != nil {
		return ManagedFilesImpact{}, fmt.Errorf("workspace path: %w", err)
	}
	if err := snapshotpayload.CheckReservedWorkspacePayloadRoot(boundary.Root); err != nil {
		return ManagedFilesImpact{}, err
	}
	sourceRoot, cleanup, err := materializeSource(repoRoot, sourceID, desc, engineType)
	if err != nil {
		return ManagedFilesImpact{}, err
	}
	defer cleanup()
	if err := snapshotpayload.CheckReservedWorkspacePayloadRoot(sourceRoot); err != nil {
		return ManagedFilesImpact{}, fmt.Errorf("source save point payload: %w", err)
	}
	if err := repo.ValidateManagedPayloadOnly(boundary, sourceRoot); err != nil {
		return ManagedFilesImpact{}, err
	}

	currentFiles, err := scanManagedFiles(boundary.Root, boundary.ExcludesRelativePath)
	if err != nil {
		return ManagedFilesImpact{}, fmt.Errorf("scan folder: %w", err)
	}
	sourceFiles, err := scanManagedFiles(sourceRoot, boundary.ExcludesRelativePath)
	if err != nil {
		return ManagedFilesImpact{}, fmt.Errorf("scan source save point: %w", err)
	}
	return compareManagedFiles(currentFiles, sourceFiles), nil
}

func materializeSource(repoRoot string, sourceID model.SnapshotID, desc *model.Descriptor, engineType model.EngineType) (string, func(), error) {
	tmpParent, err := os.MkdirTemp(filepath.Join(repoRoot, repo.JVSDirName), "restore-preview-*")
	if err != nil {
		return "", func() {}, fmt.Errorf("create restore preview staging: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(tmpParent) }
	sourceRoot := filepath.Join(tmpParent, "source")
	snapshotDir, err := repo.SnapshotPathForRead(repoRoot, sourceID)
	if err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("source save point path: %w", err)
	}
	eng := engine.NewEngine(engineType)
	if err := snapshotpayload.MaterializeToNew(snapshotDir, sourceRoot, snapshotpayload.OptionsFromDescriptor(desc), func(src, dst string) error {
		_, err := engine.CloneToNew(eng, src, dst)
		return err
	}); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("materialize source save point: %w", err)
	}
	return sourceRoot, cleanup, nil
}

type fileSignature struct {
	Kind string
	Mode os.FileMode
	Size int64
	Hash string
}

func scanManagedFiles(root string, excluded func(rel string) bool) (map[string]fileSignature, error) {
	files := map[string]fileSignature{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("relative path: %w", err)
		}
		rel = filepath.ToSlash(rel)
		if excluded != nil && excluded(rel) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("stat %s: %w", rel, err)
		}
		if info.IsDir() {
			return nil
		}
		sig, err := signatureForPath(path, info)
		if err != nil {
			return fmt.Errorf("hash %s: %w", rel, err)
		}
		files[rel] = sig
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

func signatureForPath(path string, info os.FileInfo) (fileSignature, error) {
	sig := fileSignature{
		Mode: info.Mode().Perm(),
		Size: info.Size(),
	}
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		target, err := os.Readlink(path)
		if err != nil {
			return fileSignature{}, fmt.Errorf("read symlink: %w", err)
		}
		sum := sha256.Sum256([]byte(target))
		sig.Kind = "symlink"
		sig.Hash = hex.EncodeToString(sum[:])
		return sig, nil
	default:
		f, err := os.Open(path)
		if err != nil {
			return fileSignature{}, fmt.Errorf("open file: %w", err)
		}
		defer f.Close()
		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			return fileSignature{}, fmt.Errorf("read file: %w", err)
		}
		sig.Kind = "file"
		sig.Hash = hex.EncodeToString(h.Sum(nil))
		return sig, nil
	}
}

func compareManagedFiles(current, source map[string]fileSignature) ManagedFilesImpact {
	var overwrite, deletePaths, createPaths []string
	for path, currentSig := range current {
		sourceSig, ok := source[path]
		if !ok {
			deletePaths = append(deletePaths, path)
			continue
		}
		if currentSig != sourceSig {
			overwrite = append(overwrite, path)
		}
	}
	for path := range source {
		if _, ok := current[path]; !ok {
			createPaths = append(createPaths, path)
		}
	}
	return ManagedFilesImpact{
		Overwrite: summarizePaths(overwrite),
		Delete:    summarizePaths(deletePaths),
		Create:    summarizePaths(createPaths),
	}
}

func summarizePaths(paths []string) ChangeSummary {
	sort.Strings(paths)
	samples := paths
	if len(samples) > sampleLimit {
		samples = samples[:sampleLimit]
	}
	return ChangeSummary{
		Count:   len(paths),
		Samples: append([]string(nil), samples...),
	}
}

func changedSincePreviewError() error {
	return fmt.Errorf("folder changed since preview; run preview again. No files were changed.")
}

func sourceNotRestorableError(cause error) error {
	return fmt.Errorf("source save point is not restorable: %v. No files were changed.", cause)
}

func currentRepoID(repoRoot string) (string, error) {
	data, err := os.ReadFile(filepath.Join(repoRoot, repo.JVSDirName, repo.RepoIDFile))
	if err != nil {
		return "", fmt.Errorf("read repository identity: %w", err)
	}
	return string(bytesTrimSpace(data)), nil
}

func snapshotIDPtrOrNil(id model.SnapshotID) *model.SnapshotID {
	if id == "" {
		return nil
	}
	value := id
	return &value
}

func cloneSnapshotIDPtr(id *model.SnapshotID) *model.SnapshotID {
	if id == nil {
		return nil
	}
	value := *id
	return &value
}

func sameSnapshotIDPtr(left, right *model.SnapshotID) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}

func bytesTrimSpace(data []byte) []byte {
	start := 0
	for start < len(data) && isSpace(data[start]) {
		start++
	}
	end := len(data)
	for end > start && isSpace(data[end-1]) {
		end--
	}
	return data[start:end]
}

func isSpace(b byte) bool {
	switch b {
	case ' ', '\n', '\r', '\t':
		return true
	default:
		return false
	}
}
