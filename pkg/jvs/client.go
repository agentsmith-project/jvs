package jvs

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/agentsmith-project/jvs/internal/engine"
	"github.com/agentsmith-project/jvs/internal/gc"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/restore"
	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/internal/verify"
	"github.com/agentsmith-project/jvs/internal/worktree"
	"github.com/agentsmith-project/jvs/pkg/model"
)

// Client provides high-level JVS operations on a repository.
type Client struct {
	repoRoot   string
	repoID     string
	engineType model.EngineType
}

// InitOptions configures repository initialization.
type InitOptions struct {
	Name       string           // Repository name (validated: alphanumeric, hyphens, underscores)
	EngineType model.EngineType // Save point materialization engine; empty string triggers auto-detection
}

// SavePointID identifies a save point in the public library facade.
type SavePointID string

// String returns the save point ID as a string.
func (id SavePointID) String() string {
	return string(id)
}

func (id SavePointID) modelID() model.SnapshotID {
	return model.SnapshotID(id)
}

// SaveOptions configures save point creation.
type SaveOptions struct {
	WorkspaceName string   // Target workspace; defaults to "main"
	Message       string   // Human-readable description
	Tags          []string // Organization tags
}

// SavePoint is the public library view of a saved workspace state.
type SavePoint struct {
	SavePointID        SavePointID          `json:"save_point_id"`
	WorkspaceName      string               `json:"workspace_name"`
	CreatedAt          time.Time            `json:"created_at"`
	Message            string               `json:"message,omitempty"`
	Tags               []string             `json:"tags,omitempty"`
	Engine             model.EngineType     `json:"engine"`
	PayloadRootHash    model.HashValue      `json:"payload_root_hash"`
	DescriptorChecksum model.HashValue      `json:"descriptor_checksum"`
	IntegrityState     model.IntegrityState `json:"integrity_state"`
}

// RestoreOptions configures workspace restore.
type RestoreOptions struct {
	WorkspaceName string // Target workspace; defaults to "main"
	Target        string // Save point ID prefix or tag name
}

// CleanupOptions configures cleanup preview.
type CleanupOptions struct{}

// CleanupPlan is the public library view of a cleanup plan.
type CleanupPlan struct {
	PlanID                   string        `json:"plan_id"`
	CreatedAt                time.Time     `json:"created_at"`
	ProtectedSavePoints      []SavePointID `json:"protected_save_points"`
	ProtectedByHistory       int           `json:"protected_by_history"`
	CandidateCount           int           `json:"candidate_count"`
	ReclaimableSavePoints    []SavePointID `json:"reclaimable_save_points"`
	ReclaimableBytesEstimate int64         `json:"reclaimable_bytes_estimate"`
}

func (o *SaveOptions) workspace() string {
	if o.WorkspaceName == "" {
		return "main"
	}
	return o.WorkspaceName
}

func (o *RestoreOptions) workspace() string {
	if o.WorkspaceName == "" {
		return "main"
	}
	return o.WorkspaceName
}

// Init initializes a new JVS repository at the given path.
func Init(path string, opts InitOptions) (*Client, error) {
	name := opts.Name
	if name == "" {
		name = filepath.Base(path)
	}

	r, err := repo.Init(path, name)
	if err != nil {
		return nil, fmt.Errorf("jvs init: %w", err)
	}

	engineType := opts.EngineType
	if engineType == "" {
		engineType = detectEngineType(path)
	}

	return &Client{
		repoRoot:   r.Root,
		repoID:     r.RepoID,
		engineType: engineType,
	}, nil
}

// Open opens an existing JVS repository at or above the given path.
func Open(path string) (*Client, error) {
	r, err := repo.Discover(path)
	if err != nil {
		return nil, fmt.Errorf("jvs open: %w", err)
	}

	engineType := detectEngineType(r.Root)

	return &Client{
		repoRoot:   r.Root,
		repoID:     r.RepoID,
		engineType: engineType,
	}, nil
}

// OpenOrInit opens an existing repository, or initializes a new one if none exists.
// This is the recommended entry point for sandbox-manager integration.
func OpenOrInit(path string, opts InitOptions) (*Client, error) {
	if _, err := repo.Discover(path); err == nil {
		return Open(path)
	} else if !isNoRepoError(err) {
		return nil, fmt.Errorf("jvs open: %w", err)
	}
	return Init(path, opts)
}

// Save creates a new save point for the workspace.
// The workspace must not be in detached state.
func (c *Client) Save(ctx context.Context, opts SaveOptions) (*SavePoint, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}

	workspaceName := opts.workspace()
	wtMgr := worktree.NewManager(c.repoRoot)
	cfg, err := wtMgr.Get(workspaceName)
	if err != nil {
		return nil, fmt.Errorf("get workspace: %w", err)
	}
	if cfg.IsDetached() {
		return nil, fmt.Errorf("cannot save in detached state")
	}
	if err := checkContext(ctx); err != nil {
		return nil, err
	}

	creator := snapshot.NewCreator(c.repoRoot, c.engineType)
	desc, err := creator.Create(workspaceName, opts.Message, opts.Tags)
	if err != nil {
		return nil, err
	}
	return publicSavePoint(desc), nil
}

// Restore restores a workspace to a specific save point identified by opts.Target.
// Target can be a save point ID prefix or tag name.
func (c *Client) Restore(ctx context.Context, opts RestoreOptions) error {
	if err := checkContext(ctx); err != nil {
		return err
	}

	workspaceName := opts.workspace()
	target := strings.TrimSpace(opts.Target)
	if target == "" {
		return fmt.Errorf("restore target is required")
	}

	// Internal storage still resolves by descriptor ID first, then tag.
	desc, err := snapshot.FindOne(c.repoRoot, target)
	if err != nil {
		desc, err = snapshot.FindByTag(c.repoRoot, target)
		if err != nil {
			return fmt.Errorf("resolve target %q: %w", target, err)
		}
	}
	if err := checkContext(ctx); err != nil {
		return err
	}

	restorer := restore.NewRestorer(c.repoRoot, c.engineType)
	return restorer.Restore(workspaceName, desc.SnapshotID)
}

// RestoreLatest restores a workspace to its most recent save point.
// Returns nil if the workspace has no save points.
func (c *Client) RestoreLatest(ctx context.Context, workspaceName string) error {
	if err := checkContext(ctx); err != nil {
		return err
	}

	if workspaceName == "" {
		workspaceName = "main"
	}

	has, err := c.HasSavePoints(ctx, workspaceName)
	if err != nil {
		return err
	}
	if !has {
		return nil
	}
	if err := checkContext(ctx); err != nil {
		return err
	}

	restorer := restore.NewRestorer(c.repoRoot, c.engineType)
	return restorer.RestoreToLatest(workspaceName)
}

// History returns save points for a workspace, sorted newest first.
// Pass limit <= 0 for all save points.
func (c *Client) History(ctx context.Context, workspaceName string, limit int) ([]*SavePoint, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}

	if workspaceName == "" {
		workspaceName = "main"
	}

	opts := snapshot.FilterOptions{WorktreeName: workspaceName}
	results, err := snapshot.Find(c.repoRoot, opts)
	if err != nil {
		return nil, err
	}

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	savePoints := make([]*SavePoint, 0, len(results))
	for _, desc := range results {
		savePoints = append(savePoints, publicSavePoint(desc))
	}
	return savePoints, nil
}

// LatestSavePoint returns the most recent save point for a workspace.
// Returns nil, nil if no save points exist.
func (c *Client) LatestSavePoint(ctx context.Context, workspaceName string) (*SavePoint, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}

	if workspaceName == "" {
		workspaceName = "main"
	}

	wtMgr := worktree.NewManager(c.repoRoot)
	cfg, err := wtMgr.Get(workspaceName)
	if err != nil {
		return nil, fmt.Errorf("get workspace: %w", err)
	}

	if cfg.LatestSnapshotID == "" {
		return nil, nil
	}

	desc, err := snapshot.LoadDescriptor(c.repoRoot, cfg.LatestSnapshotID)
	if err != nil {
		return nil, err
	}
	return publicSavePoint(desc), nil
}

// HasSavePoints returns true if the workspace has at least one save point.
func (c *Client) HasSavePoints(ctx context.Context, workspaceName string) (bool, error) {
	if err := checkContext(ctx); err != nil {
		return false, err
	}

	if workspaceName == "" {
		workspaceName = "main"
	}

	wtMgr := worktree.NewManager(c.repoRoot)
	cfg, err := wtMgr.Get(workspaceName)
	if err != nil {
		return false, fmt.Errorf("get workspace: %w", err)
	}

	return cfg.LatestSnapshotID != "", nil
}

// Verify checks a save point's integrity.
func (c *Client) Verify(ctx context.Context, savePointID SavePointID) error {
	if err := checkContext(ctx); err != nil {
		return err
	}
	result, err := verify.NewVerifier(c.repoRoot).VerifySnapshot(savePointID.modelID(), true)
	if err != nil {
		return err
	}
	if result.TamperDetected {
		if result.Error != "" {
			return fmt.Errorf("verify save point: %s", result.Error)
		}
		return fmt.Errorf("verify save point: tamper detected")
	}
	return nil
}

// PreviewCleanup creates a cleanup plan without deleting anything.
func (c *Client) PreviewCleanup(ctx context.Context, _ CleanupOptions) (*CleanupPlan, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}

	collector := gc.NewCollector(c.repoRoot)

	plan, err := collector.Plan()
	if err != nil {
		return nil, fmt.Errorf("cleanup plan: %w", err)
	}
	return publicCleanupPlan(plan), nil
}

// RunCleanup executes a previously created cleanup plan by ID.
func (c *Client) RunCleanup(ctx context.Context, planID string) error {
	if err := checkContext(ctx); err != nil {
		return err
	}
	collector := gc.NewCollector(c.repoRoot)
	return collector.Run(planID)
}

// RepoRoot returns the absolute path to the repository root.
func (c *Client) RepoRoot() string {
	return c.repoRoot
}

// RepoID returns the unique repository identifier.
func (c *Client) RepoID() string {
	return c.repoID
}

// EngineType returns the save point materialization engine in use.
func (c *Client) EngineType() model.EngineType {
	return c.engineType
}

// WorkspacePath returns the filesystem path to a workspace payload directory.
// This is the path that should be mounted into agent pods as /workspace.
func (c *Client) WorkspacePath(workspaceName string) string {
	if workspaceName == "" {
		workspaceName = "main"
	}
	path, err := worktree.NewManager(c.repoRoot).Path(workspaceName)
	if err != nil {
		return ""
	}
	return path
}

// detectEngineType auto-detects the best engine for the given path.
func detectEngineType(path string) model.EngineType {
	eng, err := engine.DetectEngine(path)
	if err != nil {
		return model.EngineCopy
	}
	return eng.Name()
}

func checkContext(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

func publicSavePoint(desc *model.Descriptor) *SavePoint {
	if desc == nil {
		return nil
	}
	return &SavePoint{
		SavePointID:        SavePointID(desc.SnapshotID),
		WorkspaceName:      desc.WorktreeName,
		CreatedAt:          desc.CreatedAt,
		Message:            desc.Note,
		Tags:               append([]string(nil), desc.Tags...),
		Engine:             desc.Engine,
		PayloadRootHash:    desc.PayloadRootHash,
		DescriptorChecksum: desc.DescriptorChecksum,
		IntegrityState:     desc.IntegrityState,
	}
}

func publicSavePointIDs(ids []model.SnapshotID) []SavePointID {
	if len(ids) == 0 {
		return nil
	}
	savePointIDs := make([]SavePointID, 0, len(ids))
	for _, id := range ids {
		savePointIDs = append(savePointIDs, SavePointID(id))
	}
	return savePointIDs
}

func publicCleanupPlan(plan *model.GCPlan) *CleanupPlan {
	if plan == nil {
		return nil
	}
	return &CleanupPlan{
		PlanID:                   plan.PlanID,
		CreatedAt:                plan.CreatedAt,
		ProtectedSavePoints:      publicSavePointIDs(plan.ProtectedSet),
		ProtectedByHistory:       plan.ProtectedByLineage,
		CandidateCount:           plan.CandidateCount,
		ReclaimableSavePoints:    publicSavePointIDs(plan.ToDelete),
		ReclaimableBytesEstimate: plan.DeletableBytesEstimate,
	}
}

func isNoRepoError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "no JVS repository found")
}
