// Package worktree provides worktree management operations.
package worktree

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agentsmith-project/jvs/internal/audit"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/snapshot/publishstate"
	"github.com/agentsmith-project/jvs/internal/snapshotpayload"
	"github.com/agentsmith-project/jvs/pkg/errclass"
	"github.com/agentsmith-project/jvs/pkg/fsutil"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/agentsmith-project/jvs/pkg/pathutil"
	"github.com/agentsmith-project/jvs/pkg/uuidutil"
)

// Manager handles worktree CRUD operations.
type Manager struct {
	repoRoot string
}

var (
	writeWorktreeConfig = repo.WriteWorktreeConfig
	renamePath          = fsutil.RenameNoReplaceAndSync
)

// NewManager creates a new worktree manager.
func NewManager(repoRoot string) *Manager {
	return &Manager{repoRoot: repoRoot}
}

// Create creates a new worktree with the given name.
func (m *Manager) Create(name string, baseSnapshotID *model.SnapshotID) (*model.WorktreeConfig, error) {
	var cfg *model.WorktreeConfig
	err := repo.WithMutationLock(m.repoRoot, "worktree create", func() error {
		var err error
		cfg, err = m.create(name, baseSnapshotID)
		return err
	})
	return cfg, err
}

func (m *Manager) create(name string, baseSnapshotID *model.SnapshotID) (*model.WorktreeConfig, error) {
	if err := pathutil.ValidateName(name); err != nil {
		return nil, err
	}

	configPath, err := m.configPathForNewWorktree(name)
	if err != nil {
		return nil, err
	}

	payloadPath, stagingPath, err := m.preparePayloadStaging(name)
	if err != nil {
		return nil, err
	}
	cleanupStaging := true
	defer func() {
		if cleanupStaging {
			_ = os.RemoveAll(stagingPath)
		}
	}()

	if err := fsyncPayloadStaging(stagingPath); err != nil {
		return nil, fmt.Errorf("fsync payload staging: %w", err)
	}

	if err := m.prepareConfigDir(configPath); err != nil {
		return nil, fmt.Errorf("create config directory: %w", err)
	}

	if err := publishPayloadStaging(stagingPath, payloadPath); err != nil {
		return nil, fmt.Errorf("publish payload directory: %w", err)
	}
	cleanupStaging = false
	published := true

	cfg := &model.WorktreeConfig{
		Name:      name,
		CreatedAt: time.Now().UTC(),
	}
	if baseSnapshotID != nil {
		cfg.HeadSnapshotID = *baseSnapshotID
	}

	if err := writeWorktreeConfig(m.repoRoot, name, cfg); err != nil {
		if fsutil.IsCommitUncertain(err) {
			return nil, fmt.Errorf("write config commit uncertain after publishing payload; leaving payload in place: %w", err)
		}
		if published {
			if cleanupErr := removeOwnedPath(payloadPath); cleanupErr != nil {
				return nil, fmt.Errorf("write config: %w; additionally failed to cleanup payload: %v", err, cleanupErr)
			}
		}
		return nil, fmt.Errorf("write config: %w", err)
	}

	return cfg, nil
}

// CreateFromSnapshot creates a new worktree with content cloned from a snapshot.
// This is similar to Fork but uses "create" semantics (for the --from flag).
func (m *Manager) CreateFromSnapshot(name string, snapshotID model.SnapshotID, cloneFunc func(src, dst string) error) (*model.WorktreeConfig, error) {
	return m.createMaterializedSnapshotWorktree(name, snapshotID, cloneFunc)
}

// CreateStartedFromSnapshot creates a new workspace whose files come from a
// save point, without inheriting that save point as workspace history.
func (m *Manager) CreateStartedFromSnapshot(name string, snapshotID model.SnapshotID, cloneFunc func(src, dst string) error) (*model.WorktreeConfig, error) {
	var cfg *model.WorktreeConfig
	err := repo.WithMutationLock(m.repoRoot, "workspace new from save point", func() error {
		var err error
		cfg, err = m.createStartedFromSnapshotLocked(name, snapshotID, cloneFunc)
		return err
	})
	return cfg, err
}

// PlannedStartedFromPath returns the workspace folder that CreateStartedFromSnapshot
// would publish, without creating staging, payload, or metadata paths.
func (m *Manager) PlannedStartedFromPath(name string) (string, error) {
	if err := pathutil.ValidateName(name); err != nil {
		return "", err
	}
	if _, err := m.configPathForNewWorktree(name); err != nil {
		return "", err
	}
	externalPath, useExternal, err := m.externalStartedFromPayloadPath(name)
	if err != nil {
		return "", err
	}
	if useExternal {
		if err := m.validateExternalPayloadTarget(externalPath); err != nil {
			return "", err
		}
		return externalPath, nil
	}
	return m.plannedMissingPayloadTarget(name)
}

func (m *Manager) createMaterializedSnapshotWorktree(name string, snapshotID model.SnapshotID, cloneFunc func(src, dst string) error) (*model.WorktreeConfig, error) {
	var cfg *model.WorktreeConfig
	err := repo.WithMutationLock(m.repoRoot, "worktree create from snapshot", func() error {
		var err error
		cfg, err = m.createMaterializedSnapshotWorktreeLocked(name, snapshotID, cloneFunc)
		return err
	})
	return cfg, err
}

func (m *Manager) createMaterializedSnapshotWorktreeLocked(name string, snapshotID model.SnapshotID, cloneFunc func(src, dst string) error) (*model.WorktreeConfig, error) {
	if err := pathutil.ValidateName(name); err != nil {
		return nil, err
	}

	configPath, err := m.configPathForNewWorktree(name)
	if err != nil {
		return nil, err
	}

	desc, snapshotDir, err := m.loadPublishedSnapshotForMaterialization(snapshotID)
	if err != nil {
		return nil, fmt.Errorf("load snapshot descriptor: %w", err)
	}
	opts := snapshotpayload.OptionsFromDescriptor(desc)

	payloadPath, stagingPath, err := m.prepareMissingPayloadStaging(name)
	if err != nil {
		return nil, err
	}
	cleanupStaging := true
	defer func() {
		if cleanupStaging {
			_ = os.RemoveAll(stagingPath)
		}
	}()

	// Materialize snapshot content to worktree
	if err := snapshotpayload.MaterializeToNew(snapshotDir, stagingPath, opts, cloneFunc); err != nil {
		return nil, fmt.Errorf("clone snapshot content: %w", err)
	}

	if err := fsyncPayloadStaging(stagingPath); err != nil {
		return nil, fmt.Errorf("fsync payload staging: %w", err)
	}

	if err := m.prepareConfigDir(configPath); err != nil {
		return nil, fmt.Errorf("create config directory: %w", err)
	}

	if err := publishPayloadStaging(stagingPath, payloadPath); err != nil {
		return nil, fmt.Errorf("publish payload directory: %w", err)
	}
	cleanupStaging = false
	published := true

	cfg := &model.WorktreeConfig{
		Name:             name,
		CreatedAt:        time.Now().UTC(),
		BaseSnapshotID:   snapshotID,
		HeadSnapshotID:   snapshotID,
		LatestSnapshotID: snapshotID,
	}

	if err := writeWorktreeConfig(m.repoRoot, name, cfg); err != nil {
		if fsutil.IsCommitUncertain(err) {
			return nil, fmt.Errorf("write config commit uncertain after publishing payload; leaving payload in place: %w", err)
		}
		if published {
			if cleanupErr := removeOwnedPath(payloadPath); cleanupErr != nil {
				return nil, fmt.Errorf("write config: %w; additionally failed to cleanup payload: %v", err, cleanupErr)
			}
		}
		return nil, fmt.Errorf("write config: %w", err)
	}

	return cfg, nil
}

func (m *Manager) createStartedFromSnapshotLocked(name string, snapshotID model.SnapshotID, cloneFunc func(src, dst string) error) (*model.WorktreeConfig, error) {
	if err := pathutil.ValidateName(name); err != nil {
		return nil, err
	}

	configPath, err := m.configPathForNewWorktree(name)
	if err != nil {
		return nil, err
	}

	desc, snapshotDir, err := m.loadPublishedSnapshotForMaterialization(snapshotID)
	if err != nil {
		return nil, fmt.Errorf("load save point: %w", err)
	}
	opts := snapshotpayload.OptionsFromDescriptor(desc)

	payloadPath, stagingPath, realPath, err := m.prepareStartedFromPayloadStaging(name)
	if err != nil {
		return nil, err
	}
	cleanupStaging := true
	defer func() {
		if cleanupStaging {
			_ = os.RemoveAll(stagingPath)
		}
	}()

	if err := snapshotpayload.MaterializeToNew(snapshotDir, stagingPath, opts, cloneFunc); err != nil {
		return nil, fmt.Errorf("copy save point contents: %w", err)
	}
	if err := validateStartedFromMaterializedPayload(stagingPath); err != nil {
		return nil, fmt.Errorf("validate save point contents: %w", err)
	}

	if err := fsyncPayloadStaging(stagingPath); err != nil {
		return nil, fmt.Errorf("fsync workspace staging: %w", err)
	}

	if err := publishPayloadStaging(stagingPath, payloadPath); err != nil {
		if fsutil.IsCommitUncertain(err) {
			if cleanupErr := cleanupNewWorkspaceResidue(payloadPath, configPath); cleanupErr != nil {
				return nil, fmt.Errorf("publish workspace folder commit uncertain: %w; additionally failed to cleanup workspace: %v", err, cleanupErr)
			}
			return nil, fmt.Errorf("publish workspace folder commit uncertain; removed unregistered workspace folder: %w", err)
		}
		return nil, fmt.Errorf("publish workspace folder: %w", err)
	}
	cleanupStaging = false

	if err := m.prepareConfigDir(configPath); err != nil {
		if cleanupErr := cleanupNewWorkspaceResidue(payloadPath, configPath); cleanupErr != nil {
			return nil, fmt.Errorf("create config directory: %w; additionally failed to cleanup workspace: %v", err, cleanupErr)
		}
		return nil, fmt.Errorf("create config directory: %w", err)
	}

	cfg := &model.WorktreeConfig{
		Name:                  name,
		RealPath:              realPath,
		CreatedAt:             time.Now().UTC(),
		HeadSnapshotID:        snapshotID,
		StartedFromSnapshotID: snapshotID,
		PathSources:           model.NewPathSources(),
	}

	if err := writeWorktreeConfig(m.repoRoot, name, cfg); err != nil {
		if fsutil.IsCommitUncertain(err) {
			return nil, fmt.Errorf("write config commit uncertain after publishing workspace folder; leaving folder in place: %w", err)
		}
		if cleanupErr := cleanupNewWorkspaceResidue(payloadPath, configPath); cleanupErr != nil {
			return nil, fmt.Errorf("write config: %w; additionally failed to cleanup workspace: %v", err, cleanupErr)
		}
		return nil, fmt.Errorf("write config: %w", err)
	}

	return cfg, nil
}

func (m *Manager) loadPublishedSnapshotForMaterialization(snapshotID model.SnapshotID) (*model.Descriptor, string, error) {
	state, issue := publishstate.Inspect(m.repoRoot, snapshotID, publishstate.Options{
		RequireReady:             true,
		RequirePayload:           true,
		VerifyDescriptorChecksum: true,
		VerifyPayloadHash:        true,
	})
	if issue != nil {
		return nil, "", publishStateIssueError(issue)
	}
	return state.Descriptor, state.SnapshotDir, nil
}

func validateStartedFromMaterializedPayload(payloadRoot string) error {
	if err := snapshotpayload.CheckReservedWorkspacePayloadRoot(payloadRoot); err != nil {
		return err
	}
	return repo.ValidateManagedPayloadOnly(repo.WorktreePayloadBoundary{
		Root:              payloadRoot,
		ExcludedRootNames: []string{repo.JVSDirName},
	}, payloadRoot)
}

func publishStateIssueError(issue *publishstate.Issue) error {
	return &errclass.JVSError{Code: issue.Code, Message: issue.Message}
}

// List returns all worktrees.
func (m *Manager) List() ([]*model.WorktreeConfig, error) {
	worktreesDir, err := repo.WorktreesDirPath(m.repoRoot)
	if err != nil {
		return nil, fmt.Errorf("read worktrees directory: %w", err)
	}
	entries, err := os.ReadDir(worktreesDir)
	if err != nil {
		return nil, fmt.Errorf("read worktrees directory: %w", err)
	}

	var configs []*model.WorktreeConfig
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("load worktree %s: metadata entry is symlink", entry.Name())
		}
		if !entry.IsDir() {
			continue
		}
		if err := pathutil.ValidateName(entry.Name()); err != nil {
			return nil, fmt.Errorf("load worktree %s: %w", entry.Name(), err)
		}
		cfg, err := repo.LoadWorktreeConfig(m.repoRoot, entry.Name())
		if err != nil {
			return nil, fmt.Errorf("load worktree %s: %w", entry.Name(), err)
		}
		if cfg.Name != entry.Name() {
			return nil, fmt.Errorf("load worktree %s: config name mismatch %q", entry.Name(), cfg.Name)
		}
		configs = append(configs, cfg)
	}
	return configs, nil
}

// Get returns the config for a specific worktree.
func (m *Manager) Get(name string) (*model.WorktreeConfig, error) {
	if err := pathutil.ValidateName(name); err != nil {
		return nil, err
	}
	return repo.LoadWorktreeConfig(m.repoRoot, name)
}

// Path returns the validated payload path for a worktree.
func (m *Manager) Path(name string) (string, error) {
	payloadPath, err := repo.WorktreePayloadPath(m.repoRoot, name)
	if err != nil {
		return "", err
	}
	if sameCleanAbsPath(m.repoRoot, payloadPath) {
		if err := rejectSymlinkPath(payloadPath); err != nil {
			return "", fmt.Errorf("validate payload path: %w", err)
		}
		return payloadPath, nil
	}
	if !pathIsInsideRepo(m.repoRoot, payloadPath) {
		if err := validateExistingRealDir(payloadPath); err != nil {
			return "", fmt.Errorf("validate payload path: %w", err)
		}
		return payloadPath, nil
	}
	if err := m.validatePathForMutation(payloadPath, "payload"); err != nil {
		return "", err
	}
	return payloadPath, nil
}

// Rename renames a worktree.
func (m *Manager) Rename(oldName, newName string) error {
	return repo.WithMutationLock(m.repoRoot, "worktree rename", func() error {
		return m.rename(oldName, newName)
	})
}

func (m *Manager) rename(oldName, newName string) error {
	if err := pathutil.ValidateName(oldName); err != nil {
		return err
	}
	if err := pathutil.ValidateName(newName); err != nil {
		return err
	}
	if oldName == "main" {
		return errors.New("cannot rename main worktree")
	}

	oldConfigDir, err := m.existingConfigDirForMutation(oldName)
	if err != nil {
		return err
	}
	newConfigDir, err := m.newConfigDirForMutation(newName)
	if err != nil {
		return err
	}
	oldCfg, err := repo.LoadWorktreeConfig(m.repoRoot, oldName)
	if err != nil {
		return fmt.Errorf("load config before rename: %w", err)
	}
	if oldCfg.Name != oldName {
		return fmt.Errorf("config name mismatch for %s: %q", oldName, oldCfg.Name)
	}

	var oldPayload, newPayload string
	newRealPath := ""
	rollback := renameRollbackLedger{}
	if oldName != "main" {
		oldPayload, err = m.existingPayloadPathForMoveOrRemove(oldName)
		if err != nil {
			return err
		}
		newPayload, newRealPath, err = m.newPayloadPathForRename(newName, oldCfg)
		if err != nil {
			return err
		}
		if err := renamePath(oldPayload, newPayload); err != nil && !os.IsNotExist(err) {
			if fsutil.IsCommitUncertain(err) {
				return fmt.Errorf("rename payload commit uncertain; not rolling back: %w", err)
			}
			return fmt.Errorf("rename payload: %w", err)
		} else if err == nil {
			rollback.add("payload rename", func() error {
				return renamePath(newPayload, oldPayload)
			})
		}
	}

	// Rename config directory
	if err := renamePath(oldConfigDir, newConfigDir); err != nil {
		err = fmt.Errorf("rename config directory: %w", err)
		if fsutil.IsCommitUncertain(err) {
			return fmt.Errorf("%w; not rolling back because config directory rename is uncertain", err)
		}
		return rollback.rollback(err)
	}
	rollback.add("config directory rename", func() error {
		return renamePath(newConfigDir, oldConfigDir)
	})

	// Update config with new name
	newCfg := *oldCfg
	newCfg.Name = newName
	newCfg.RealPath = newRealPath
	if err := writeWorktreeConfig(m.repoRoot, newName, &newCfg); err != nil {
		err = fmt.Errorf("write config after rename: %w", err)
		if fsutil.IsCommitUncertain(err) {
			return fmt.Errorf("%w; not rolling back because config rewrite is uncertain", err)
		}
		return rollback.rollback(err)
	}

	return nil
}

func (m *Manager) existingPayloadPathForMoveOrRemove(name string) (string, error) {
	payloadPath, err := repo.WorktreePayloadPath(m.repoRoot, name)
	if err != nil {
		return "", err
	}
	if !pathIsInsideRepo(m.repoRoot, payloadPath) {
		if err := validateExistingRealDir(payloadPath); err != nil {
			return "", fmt.Errorf("validate payload path: %w", err)
		}
		return payloadPath, nil
	}
	return m.payloadPathForMutation(name)
}

func (m *Manager) newPayloadPathForRename(newName string, oldCfg *model.WorktreeConfig) (payloadPath string, realPath string, err error) {
	if oldCfg.RealPath == "" {
		payloadPath, err := m.newPayloadPathForMutation(newName)
		return payloadPath, "", err
	}

	payloadPath, err = renamedWorkspaceRealPath(oldCfg.RealPath, newName)
	if err != nil {
		return "", "", err
	}
	if err := m.validateExternalPayloadTarget(payloadPath); err != nil {
		return "", "", err
	}
	return payloadPath, payloadPath, nil
}

func renamedWorkspaceRealPath(oldRealPath, newName string) (string, error) {
	if oldRealPath == "" {
		return "", fmt.Errorf("workspace real path is empty")
	}
	if !filepath.IsAbs(oldRealPath) {
		return "", fmt.Errorf("workspace real path must be absolute: %s", oldRealPath)
	}
	path, err := filepath.Abs(filepath.Join(filepath.Dir(oldRealPath), newName))
	if err != nil {
		return "", err
	}
	return filepath.Clean(path), nil
}

type renameRollbackLedger struct {
	steps []renameRollbackStep
}

type renameRollbackStep struct {
	label string
	run   func() error
}

func (l *renameRollbackLedger) add(label string, run func() error) {
	l.steps = append(l.steps, renameRollbackStep{label: label, run: run})
}

func (l *renameRollbackLedger) rollback(cause error) error {
	for i := len(l.steps) - 1; i >= 0; i-- {
		step := l.steps[i]
		if err := step.run(); err != nil {
			if fsutil.IsCommitUncertain(err) {
				return fmt.Errorf("%w; rollback %s commit uncertain, stopping rollback: %w", cause, step.label, err)
			}
			return fmt.Errorf("%w; rollback %s failed: %w", cause, step.label, err)
		}
	}
	return cause
}

// Remove deletes a worktree. Fails if the worktree is main.
func (m *Manager) Remove(name string) error {
	return repo.WithMutationLock(m.repoRoot, "worktree remove", func() error {
		return m.remove(name)
	})
}

func (m *Manager) remove(name string) error {
	if err := pathutil.ValidateName(name); err != nil {
		return err
	}
	if name == "main" {
		return errors.New("cannot remove main worktree")
	}

	cfg, err := repo.LoadWorktreeConfig(m.repoRoot, name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			absent, absentErr := m.removeTargetAbsent(name)
			if absentErr != nil {
				return absentErr
			}
			if absent {
				return nil
			}
		}
		return fmt.Errorf("load config before remove: %w", err)
	}
	auditPath := filepath.Join(m.repoRoot, ".jvs", "audit", "audit.jsonl")
	auditLogger := audit.NewFileAppender(auditPath)
	if err := auditLogger.EnsureAppendable(); err != nil {
		return fmt.Errorf("audit log not appendable: %w", err)
	}

	payloadPath, err := m.existingPayloadPathForMoveOrRemove(name)
	if err != nil {
		return err
	}
	configDir, err := m.configDirForMutation(name)
	if err != nil {
		return err
	}

	if err := removeOwnedPath(payloadPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove payload: %w", err)
	}

	if err := removeOwnedPath(configDir); err != nil {
		return fmt.Errorf("remove config: %w", err)
	}

	// Audit log the removal
	if err := auditLogger.Append(model.EventTypeWorktreeRemove, name, "", map[string]any{
		"head_snapshot_id": string(cfg.HeadSnapshotID),
	}); err != nil {
		return fmt.Errorf("write audit log: %w", err)
	}

	return nil
}

func (m *Manager) removeTargetAbsent(name string) (bool, error) {
	payloadPath, err := m.payloadPathForMutation(name)
	if err != nil {
		return false, err
	}
	configDir, err := m.configDirForMutation(name)
	if err != nil {
		return false, err
	}

	payloadExists, err := pathExists(payloadPath)
	if err != nil {
		return false, fmt.Errorf("stat payload: %w", err)
	}
	configExists, err := pathExists(configDir)
	if err != nil {
		return false, fmt.Errorf("stat config: %w", err)
	}
	return !payloadExists && !configExists, nil
}

func pathExists(path string) (bool, error) {
	if _, err := os.Lstat(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// UpdateHead atomically updates the head snapshot ID for a worktree.
// This is used by restore to move to a different point in history.
func (m *Manager) UpdateHead(name string, snapshotID model.SnapshotID) error {
	if err := pathutil.ValidateName(name); err != nil {
		return err
	}
	cfg, err := repo.LoadWorktreeConfig(m.repoRoot, name)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	cfg.HeadSnapshotID = snapshotID
	cfg.PathSources = model.NewPathSources()
	return writeWorktreeConfig(m.repoRoot, name, cfg)
}

// SetLatest updates both head and latest snapshot IDs for a worktree.
// This is used by snapshot creation to mark a new latest state.
func (m *Manager) SetLatest(name string, snapshotID model.SnapshotID) error {
	if err := pathutil.ValidateName(name); err != nil {
		return err
	}
	cfg, err := repo.LoadWorktreeConfig(m.repoRoot, name)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	cfg.HeadSnapshotID = snapshotID
	cfg.LatestSnapshotID = snapshotID
	cfg.PathSources = model.NewPathSources()
	return writeWorktreeConfig(m.repoRoot, name, cfg)
}

// RebindRealPath updates the workspace's destination-local real path after the
// caller has determined that the new binding is safe.
func (m *Manager) RebindRealPath(name, realPath string) error {
	if err := pathutil.ValidateName(name); err != nil {
		return err
	}
	canonical, err := repo.ValidateWorktreeRealPathForRepair(m.repoRoot, name, realPath)
	if err != nil {
		return err
	}
	cfg, err := repo.LoadWorktreeConfig(m.repoRoot, name)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if cfg.Name != name {
		return fmt.Errorf("config name mismatch for %s: %q", name, cfg.Name)
	}
	cfg.RealPath = canonical
	return writeWorktreeConfig(m.repoRoot, name, cfg)
}

// Fork creates a new worktree from a snapshot with content cloned.
// The new worktree will be at HEAD state (can create snapshots immediately).
func (m *Manager) Fork(snapshotID model.SnapshotID, name string, cloneFunc func(src, dst string) error) (*model.WorktreeConfig, error) {
	return m.createMaterializedSnapshotWorktree(name, snapshotID, cloneFunc)
}

func (m *Manager) configPathForNewWorktree(name string) (string, error) {
	configPath, err := repo.WorktreeConfigPath(m.repoRoot, name)
	if err != nil {
		return "", err
	}
	rel, err := repoRelativePath(m.repoRoot, configPath)
	if err != nil {
		return "", err
	}
	if err := pathutil.ValidateNoSymlinkParents(m.repoRoot, rel); err != nil {
		return "", fmt.Errorf("validate config path: %w", err)
	}
	if _, err := os.Lstat(configPath); err == nil {
		return "", fmt.Errorf("worktree %s already exists", name)
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("stat config: %w", err)
	}
	return configPath, nil
}

func (m *Manager) prepareConfigDir(configPath string) error {
	rel, err := repoRelativePath(m.repoRoot, configPath)
	if err != nil {
		return err
	}
	if err := pathutil.ValidateNoSymlinkParents(m.repoRoot, rel); err != nil {
		return fmt.Errorf("validate config path: %w", err)
	}
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return err
	}
	if err := validateExistingRealDir(configDir); err != nil {
		return err
	}
	if _, err := os.Lstat(configPath); err == nil {
		return fmt.Errorf("worktree config already exists: %s", configPath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat config: %w", err)
	}
	return nil
}

func (m *Manager) preparePayloadStaging(name string) (string, string, error) {
	payloadPath, parent, err := m.prepareNewPayloadTarget(name)
	if err != nil {
		return "", "", err
	}

	stagingPath, err := os.MkdirTemp(parent, "."+name+".staging-*")
	if err != nil {
		return "", "", fmt.Errorf("create payload staging: %w", err)
	}
	return payloadPath, stagingPath, nil
}

func (m *Manager) prepareMissingPayloadStaging(name string) (string, string, error) {
	payloadPath, parent, err := m.prepareNewPayloadTarget(name)
	if err != nil {
		return "", "", err
	}

	for range 16 {
		stagingPath := filepath.Join(parent, "."+name+".staging-"+uuidutil.NewV4()[:8])
		if _, err := os.Lstat(stagingPath); os.IsNotExist(err) {
			return payloadPath, stagingPath, nil
		} else if err != nil {
			return "", "", fmt.Errorf("stat payload staging: %w", err)
		}
	}
	return "", "", fmt.Errorf("allocate payload staging path")
}

func (m *Manager) prepareStartedFromPayloadStaging(name string) (payloadPath, stagingPath, realPath string, err error) {
	externalPath, useExternal, err := m.externalStartedFromPayloadPath(name)
	if err != nil {
		return "", "", "", err
	}
	if !useExternal {
		payloadPath, stagingPath, err := m.prepareMissingPayloadStaging(name)
		return payloadPath, stagingPath, "", err
	}

	payloadPath, stagingPath, err = m.prepareExternalPayloadStaging(name, externalPath)
	if err != nil {
		return "", "", "", err
	}
	return payloadPath, stagingPath, payloadPath, nil
}

func (m *Manager) externalStartedFromPayloadPath(name string) (string, bool, error) {
	cfg, err := repo.LoadWorktreeConfig(m.repoRoot, "main")
	if err != nil {
		return "", false, fmt.Errorf("load main workspace: %w", err)
	}
	if cfg.RealPath == "" || !sameCleanAbsPath(cfg.RealPath, m.repoRoot) {
		return "", false, nil
	}
	path, err := filepath.Abs(filepath.Join(filepath.Dir(m.repoRoot), name))
	if err != nil {
		return "", false, err
	}
	return filepath.Clean(path), true, nil
}

func (m *Manager) prepareExternalPayloadStaging(name, payloadPath string) (string, string, error) {
	if err := m.validateExternalPayloadTarget(payloadPath); err != nil {
		return "", "", err
	}

	parent := filepath.Dir(payloadPath)
	for range 16 {
		stagingPath := filepath.Join(parent, "."+name+".staging-"+uuidutil.NewV4()[:8])
		if _, err := os.Lstat(stagingPath); os.IsNotExist(err) {
			return payloadPath, stagingPath, nil
		} else if err != nil {
			return "", "", fmt.Errorf("stat workspace staging: %w", err)
		}
	}
	return "", "", fmt.Errorf("allocate workspace staging path")
}

func (m *Manager) validateExternalPayloadTarget(payloadPath string) error {
	parent := filepath.Dir(payloadPath)
	if err := validateExistingRealDir(parent); err != nil {
		return fmt.Errorf("validate workspace parent: %w", err)
	}
	if _, err := os.Lstat(payloadPath); err == nil {
		return fmt.Errorf("workspace folder already exists: %s", payloadPath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat workspace folder: %w", err)
	}
	return m.validateNewWorkspacePayloadPath(payloadPath)
}

func (m *Manager) validateNewWorkspacePayloadPath(payloadPath string) error {
	candidateLexical, err := filepath.Abs(payloadPath)
	if err != nil {
		return fmt.Errorf("resolve workspace folder: %w", err)
	}
	candidateLexical = filepath.Clean(candidateLexical)
	candidatePhysical, err := physicalPathForMissingLeaf(candidateLexical)
	if err != nil {
		return err
	}

	configs, err := m.List()
	if err != nil {
		return err
	}
	for _, cfg := range configs {
		existingPath, err := repo.WorktreePayloadPath(m.repoRoot, cfg.Name)
		if err != nil {
			return err
		}
		existingLexical, err := filepath.Abs(existingPath)
		if err != nil {
			return fmt.Errorf("resolve existing workspace folder: %w", err)
		}
		existingLexical = filepath.Clean(existingLexical)
		existingPhysical, err := filepath.EvalSymlinks(existingLexical)
		if err != nil {
			return fmt.Errorf("resolve existing workspace folder: %w", err)
		}
		if pathsOverlap(candidateLexical, existingLexical) || pathsOverlap(candidatePhysical, existingPhysical) {
			return fmt.Errorf("workspace real path overlap: %s at %s and %s at %s", cfg.Name, existingPath, "new", payloadPath)
		}
	}
	return nil
}

func (m *Manager) plannedMissingPayloadTarget(name string) (string, error) {
	payloadPath, err := repo.WorktreePayloadPath(m.repoRoot, name)
	if err != nil {
		return "", err
	}
	rel, err := repoRelativePath(m.repoRoot, payloadPath)
	if err != nil {
		return "", err
	}
	if err := pathutil.ValidateNoSymlinkParents(m.repoRoot, rel); err != nil {
		return "", fmt.Errorf("validate payload path: %w", err)
	}
	parent := filepath.Dir(payloadPath)
	if err := validateExistingRealDir(parent); err != nil {
		return "", fmt.Errorf("validate payload parent: %w", err)
	}
	if _, err := os.Lstat(payloadPath); err == nil {
		return "", fmt.Errorf("payload path already exists: %s", payloadPath)
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("stat payload path: %w", err)
	}
	return payloadPath, nil
}

func physicalPathForMissingLeaf(path string) (string, error) {
	parent := filepath.Dir(path)
	physicalParent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return "", fmt.Errorf("resolve workspace parent: %w", err)
	}
	return filepath.Join(physicalParent, filepath.Base(path)), nil
}

func pathsOverlap(a, b string) bool {
	aContainsB, err := pathContains(a, b)
	if err != nil {
		return false
	}
	bContainsA, err := pathContains(b, a)
	if err != nil {
		return false
	}
	return aContainsB || bContainsA
}

func pathContains(parent, child string) (bool, error) {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false, err
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && !filepath.IsAbs(rel)), nil
}

func (m *Manager) prepareNewPayloadTarget(name string) (string, string, error) {
	payloadPath, err := repo.WorktreePayloadPath(m.repoRoot, name)
	if err != nil {
		return "", "", err
	}
	rel, err := repoRelativePath(m.repoRoot, payloadPath)
	if err != nil {
		return "", "", err
	}
	if err := pathutil.ValidateNoSymlinkParents(m.repoRoot, rel); err != nil {
		return "", "", fmt.Errorf("validate payload path: %w", err)
	}

	parent := filepath.Dir(payloadPath)
	if err := os.MkdirAll(parent, 0755); err != nil {
		return "", "", fmt.Errorf("create payload parent: %w", err)
	}
	if err := validateExistingRealDir(parent); err != nil {
		return "", "", fmt.Errorf("validate payload parent: %w", err)
	}
	if _, err := os.Lstat(payloadPath); err == nil {
		return "", "", fmt.Errorf("payload path already exists: %s", payloadPath)
	} else if !os.IsNotExist(err) {
		return "", "", fmt.Errorf("stat payload path: %w", err)
	}

	return payloadPath, parent, nil
}

func (m *Manager) payloadPathForMutation(name string) (string, error) {
	payloadPath, err := repo.WorktreePayloadPath(m.repoRoot, name)
	if err != nil {
		return "", err
	}
	if err := m.validatePathForMutation(payloadPath, "payload"); err != nil {
		return "", err
	}
	return payloadPath, nil
}

func (m *Manager) newPayloadPathForMutation(name string) (string, error) {
	payloadPath, err := m.payloadPathForMutation(name)
	if err != nil {
		return "", err
	}
	if _, err := os.Lstat(payloadPath); err == nil {
		return "", fmt.Errorf("payload path already exists: %s", payloadPath)
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("stat payload path: %w", err)
	}
	return payloadPath, nil
}

func (m *Manager) configDirForMutation(name string) (string, error) {
	configDir, err := repo.WorktreeConfigDirPath(m.repoRoot, name)
	if err != nil {
		return "", err
	}
	if err := m.validatePathForMutation(configDir, "config"); err != nil {
		return "", err
	}
	configPath, err := repo.WorktreeConfigPath(m.repoRoot, name)
	if err != nil {
		return "", err
	}
	if err := m.validatePathForMutation(configPath, "config"); err != nil {
		return "", err
	}
	return configDir, nil
}

func (m *Manager) existingConfigDirForMutation(name string) (string, error) {
	configDir, err := m.configDirForMutation(name)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(configDir)
	if err != nil {
		return "", fmt.Errorf("stat config directory: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("config path is not directory: %s", configDir)
	}
	configPath, err := repo.WorktreeConfigPath(m.repoRoot, name)
	if err != nil {
		return "", err
	}
	info, err = os.Lstat(configPath)
	if err != nil {
		return "", fmt.Errorf("stat config file: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("config path is directory: %s", configPath)
	}
	return configDir, nil
}

func (m *Manager) newConfigDirForMutation(name string) (string, error) {
	configDir, err := m.configDirForMutation(name)
	if err != nil {
		return "", err
	}
	if _, err := os.Lstat(configDir); err == nil {
		return "", fmt.Errorf("worktree %s already exists", name)
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("stat config directory: %w", err)
	}
	return configDir, nil
}

func (m *Manager) validatePathForMutation(path, label string) error {
	rel, err := repoRelativePath(m.repoRoot, path)
	if err != nil {
		return err
	}
	if err := pathutil.ValidateNoSymlinkParents(m.repoRoot, rel); err != nil {
		return fmt.Errorf("validate %s path: %w", label, err)
	}
	if err := rejectSymlinkPath(path); err != nil {
		return fmt.Errorf("validate %s path: %w", label, err)
	}
	return nil
}

func publishPayloadStaging(stagingPath, payloadPath string) error {
	if _, err := os.Lstat(payloadPath); err == nil {
		return fmt.Errorf("payload path already exists: %s", payloadPath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat payload path: %w", err)
	}
	return renamePath(stagingPath, payloadPath)
}

func fsyncPayloadStaging(stagingPath string) error {
	if err := fsutil.FsyncTree(stagingPath); err != nil {
		return err
	}
	return fsutil.FsyncDir(stagingPath)
}

func removeOwnedPath(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return os.Remove(path)
	}
	return os.RemoveAll(path)
}

func cleanupNewWorkspaceResidue(payloadPath, configPath string) error {
	if err := ensureNewWorktreeConfigAbsent(configPath); err != nil {
		return err
	}
	var cleanupErrs []string
	if err := removeOwnedPath(payloadPath); err != nil {
		cleanupErrs = append(cleanupErrs, fmt.Sprintf("remove workspace folder: %v", err))
	}
	if err := removeNewWorktreeConfigDir(configPath); err != nil {
		cleanupErrs = append(cleanupErrs, fmt.Sprintf("remove workspace metadata: %v", err))
	}
	if len(cleanupErrs) > 0 {
		return errors.New(strings.Join(cleanupErrs, "; "))
	}
	return nil
}

func ensureNewWorktreeConfigAbsent(configPath string) error {
	if _, err := os.Lstat(configPath); err == nil {
		return fmt.Errorf("config file exists at %s", configPath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat config file: %w", err)
	}
	return nil
}

func removeNewWorktreeConfigDir(configPath string) error {
	configDir := filepath.Dir(configPath)
	if _, err := os.Lstat(configDir); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat metadata directory: %w", err)
	}
	if err := ensureNewWorktreeConfigAbsent(configPath); err != nil {
		return err
	}
	return os.RemoveAll(configDir)
}

func rejectSymlinkPath(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("path is symlink: %s", path)
	}
	return nil
}

func validateExistingRealDir(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("path is symlink: %s", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("path is not directory: %s", path)
	}
	return nil
}

func repoRelativePath(repoRoot, path string) (string, error) {
	absRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return "", fmt.Errorf("absolute repo root: %w", err)
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("absolute path: %w", err)
	}
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return "", fmt.Errorf("relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("path escapes repo root: %s", path)
	}
	return rel, nil
}

func sameCleanAbsPath(a, b string) bool {
	absA, err := filepath.Abs(a)
	if err != nil {
		return false
	}
	absB, err := filepath.Abs(b)
	if err != nil {
		return false
	}
	return filepath.Clean(absA) == filepath.Clean(absB)
}

func pathIsInsideRepo(repoRoot, path string) bool {
	_, err := repoRelativePath(repoRoot, path)
	return err == nil
}
