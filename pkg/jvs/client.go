package jvs

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/jvs-project/jvs/internal/engine"
	"github.com/jvs-project/jvs/internal/gc"
	"github.com/jvs-project/jvs/internal/repo"
	"github.com/jvs-project/jvs/internal/restore"
	"github.com/jvs-project/jvs/internal/snapshot"
	"github.com/jvs-project/jvs/internal/verify"
	"github.com/jvs-project/jvs/internal/worktree"
	"github.com/jvs-project/jvs/pkg/model"
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
	EngineType model.EngineType // Snapshot engine; empty string triggers auto-detection
}

// SnapshotOptions configures snapshot creation.
type SnapshotOptions struct {
	WorktreeName string   // Target worktree; defaults to "main"
	Note         string   // Human-readable description
	Tags         []string // Organization tags
	PartialPaths []string // Specific paths to snapshot; nil/empty means full snapshot
}

// RestoreOptions configures snapshot restore.
type RestoreOptions struct {
	WorktreeName string // Target worktree; defaults to "main"
	Target       string // Snapshot ID, tag name, or "HEAD" for latest
}

// GCOptions configures garbage collection.
type GCOptions struct {
	KeepMinSnapshots int           // Overrides default minimum snapshot retention when non-zero
	KeepMinAge       time.Duration // Overrides default age retention when non-zero
	DryRun           bool
}

func (o *SnapshotOptions) worktree() string {
	if o.WorktreeName == "" {
		return "main"
	}
	return o.WorktreeName
}

func (o *RestoreOptions) worktree() string {
	if o.WorktreeName == "" {
		return "main"
	}
	return o.WorktreeName
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

// Snapshot creates a new snapshot of the worktree.
// The worktree must not be in detached state.
func (c *Client) Snapshot(ctx context.Context, opts SnapshotOptions) (*model.Descriptor, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}

	wt := opts.worktree()
	wtMgr := worktree.NewManager(c.repoRoot)
	cfg, err := wtMgr.Get(wt)
	if err != nil {
		return nil, fmt.Errorf("get worktree: %w", err)
	}
	if cfg.IsDetached() {
		return nil, fmt.Errorf("cannot create snapshot in detached state")
	}
	if err := checkContext(ctx); err != nil {
		return nil, err
	}

	creator := snapshot.NewCreator(c.repoRoot, c.engineType)
	if len(opts.PartialPaths) > 0 {
		return creator.CreatePartial(wt, opts.Note, opts.Tags, opts.PartialPaths)
	}
	return creator.Create(wt, opts.Note, opts.Tags)
}

// Restore restores a worktree to a specific snapshot identified by opts.Target.
// Target can be a snapshot ID prefix, tag name, or "HEAD" for the latest.
func (c *Client) Restore(ctx context.Context, opts RestoreOptions) error {
	if err := checkContext(ctx); err != nil {
		return err
	}

	wt := opts.worktree()

	if opts.Target == "HEAD" || opts.Target == "" {
		return c.RestoreLatest(ctx, wt)
	}

	// Try as snapshot ID first (exact or prefix match)
	desc, err := snapshot.FindOne(c.repoRoot, opts.Target)
	if err != nil {
		// Try as tag
		desc, err = snapshot.FindByTag(c.repoRoot, opts.Target)
		if err != nil {
			return fmt.Errorf("resolve target %q: %w", opts.Target, err)
		}
	}
	if err := checkContext(ctx); err != nil {
		return err
	}

	restorer := restore.NewRestorer(c.repoRoot, c.engineType)
	return restorer.Restore(wt, desc.SnapshotID)
}

// RestoreLatest restores a worktree to its most recent snapshot.
// Returns nil if the worktree has no snapshots (nothing to restore).
func (c *Client) RestoreLatest(ctx context.Context, worktreeName string) error {
	if err := checkContext(ctx); err != nil {
		return err
	}

	if worktreeName == "" {
		worktreeName = "main"
	}

	has, err := c.HasSnapshots(ctx, worktreeName)
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
	return restorer.RestoreToLatest(worktreeName)
}

// History returns snapshot descriptors for a worktree, sorted newest first.
// Pass limit <= 0 for all snapshots.
func (c *Client) History(ctx context.Context, worktreeName string, limit int) ([]*model.Descriptor, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}

	if worktreeName == "" {
		worktreeName = "main"
	}

	opts := snapshot.FilterOptions{WorktreeName: worktreeName}
	results, err := snapshot.Find(c.repoRoot, opts)
	if err != nil {
		return nil, err
	}

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

// LatestSnapshot returns the most recent snapshot descriptor for a worktree.
// Returns nil, nil if no snapshots exist.
func (c *Client) LatestSnapshot(ctx context.Context, worktreeName string) (*model.Descriptor, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}

	if worktreeName == "" {
		worktreeName = "main"
	}

	wtMgr := worktree.NewManager(c.repoRoot)
	cfg, err := wtMgr.Get(worktreeName)
	if err != nil {
		return nil, fmt.Errorf("get worktree: %w", err)
	}

	if cfg.LatestSnapshotID == "" {
		return nil, nil
	}

	return snapshot.LoadDescriptor(c.repoRoot, cfg.LatestSnapshotID)
}

// HasSnapshots returns true if the worktree has at least one snapshot.
func (c *Client) HasSnapshots(ctx context.Context, worktreeName string) (bool, error) {
	if err := checkContext(ctx); err != nil {
		return false, err
	}

	if worktreeName == "" {
		worktreeName = "main"
	}

	wtMgr := worktree.NewManager(c.repoRoot)
	cfg, err := wtMgr.Get(worktreeName)
	if err != nil {
		return false, fmt.Errorf("get worktree: %w", err)
	}

	return cfg.LatestSnapshotID != "", nil
}

// Verify checks a snapshot's integrity (descriptor checksum + optional payload hash).
func (c *Client) Verify(ctx context.Context, snapshotID model.SnapshotID) error {
	if err := checkContext(ctx); err != nil {
		return err
	}
	result, err := verify.NewVerifier(c.repoRoot).VerifySnapshot(snapshotID, true)
	if err != nil {
		return err
	}
	if result.TamperDetected {
		if result.Error != "" {
			return fmt.Errorf("verify snapshot: %s", result.Error)
		}
		return fmt.Errorf("verify snapshot: tamper detected")
	}
	return nil
}

// GC creates and optionally executes a garbage collection plan.
// If DryRun is true, returns the plan without deleting anything.
func (c *Client) GC(ctx context.Context, opts GCOptions) (*model.GCPlan, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}

	policy, err := retentionPolicyFromOptions(opts)
	if err != nil {
		return nil, err
	}

	collector := gc.NewCollector(c.repoRoot)

	plan, err := collector.PlanWithPolicy(policy)
	if err != nil {
		return nil, fmt.Errorf("gc plan: %w", err)
	}

	if opts.DryRun {
		return plan, nil
	}
	if err := checkContext(ctx); err != nil {
		return plan, err
	}

	if err := collector.Run(plan.PlanID); err != nil {
		return plan, fmt.Errorf("gc run: %w", err)
	}

	return plan, nil
}

// RunGC executes a previously created GC plan by ID.
func (c *Client) RunGC(ctx context.Context, planID string) error {
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

// EngineType returns the snapshot engine in use.
func (c *Client) EngineType() model.EngineType {
	return c.engineType
}

// WorktreePayloadPath returns the filesystem path to a worktree's payload directory.
// This is the path that should be mounted into agent pods as /workspace.
func (c *Client) WorktreePayloadPath(worktreeName string) string {
	if worktreeName == "" {
		worktreeName = "main"
	}
	path, err := worktree.NewManager(c.repoRoot).Path(worktreeName)
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

func retentionPolicyFromOptions(opts GCOptions) (model.RetentionPolicy, error) {
	policy := model.DefaultRetentionPolicy()
	if opts.KeepMinSnapshots != 0 {
		policy.KeepMinSnapshots = opts.KeepMinSnapshots
	}
	if opts.KeepMinAge != 0 {
		policy.KeepMinAge = opts.KeepMinAge
	}
	if err := policy.Validate(); err != nil {
		return model.RetentionPolicy{}, err
	}
	return policy, nil
}

func isNoRepoError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "no JVS repository found")
}
