// Package worktree provides worktree management operations.
package worktree

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jvs-project/jvs/internal/audit"
	"github.com/jvs-project/jvs/internal/repo"
	"github.com/jvs-project/jvs/internal/snapshotpayload"
	"github.com/jvs-project/jvs/pkg/fsutil"
	"github.com/jvs-project/jvs/pkg/model"
	"github.com/jvs-project/jvs/pkg/pathutil"
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

func (m *Manager) createMaterializedSnapshotWorktree(name string, snapshotID model.SnapshotID, cloneFunc func(src, dst string) error) (*model.WorktreeConfig, error) {
	if err := pathutil.ValidateName(name); err != nil {
		return nil, err
	}

	configPath, err := m.configPathForNewWorktree(name)
	if err != nil {
		return nil, err
	}

	opts, err := snapshotpayload.OptionsForSnapshot(m.repoRoot, snapshotID)
	if err != nil {
		return nil, fmt.Errorf("load snapshot descriptor: %w", err)
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

	// Materialize snapshot content to worktree
	snapshotDir, err := repo.SnapshotPath(m.repoRoot, snapshotID)
	if err != nil {
		return nil, fmt.Errorf("load snapshot descriptor: %w", err)
	}
	if err := snapshotpayload.Materialize(snapshotDir, stagingPath, opts, cloneFunc); err != nil {
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
	return m.payloadPathForMutation(name)
}

// Rename renames a worktree.
func (m *Manager) Rename(oldName, newName string) error {
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
	rollback := renameRollbackLedger{}
	if oldName != "main" {
		oldPayload, err = m.payloadPathForMutation(oldName)
		if err != nil {
			return err
		}
		newPayload, err = m.newPayloadPathForMutation(newName)
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
	if err := writeWorktreeConfig(m.repoRoot, newName, &newCfg); err != nil {
		err = fmt.Errorf("write config after rename: %w", err)
		if fsutil.IsCommitUncertain(err) {
			return fmt.Errorf("%w; not rolling back because config rewrite is uncertain", err)
		}
		return rollback.rollback(err)
	}

	return nil
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
	if err := pathutil.ValidateName(name); err != nil {
		return err
	}
	if name == "main" {
		return errors.New("cannot remove main worktree")
	}

	// Get config before removal for audit logging
	cfg, _ := repo.LoadWorktreeConfig(m.repoRoot, name)

	payloadPath, err := m.payloadPathForMutation(name)
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
	if cfg != nil {
		auditPath := filepath.Join(m.repoRoot, ".jvs", "audit", "audit.jsonl")
		auditLogger := audit.NewFileAppender(auditPath)
		auditLogger.Append(model.EventTypeWorktreeRemove, name, "", map[string]any{
			"head_snapshot_id": string(cfg.HeadSnapshotID),
		})
	}

	return nil
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

	stagingPath, err := os.MkdirTemp(parent, "."+name+".staging-*")
	if err != nil {
		return "", "", fmt.Errorf("create payload staging: %w", err)
	}
	return payloadPath, stagingPath, nil
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
	return fsutil.RenameNoReplaceAndSync(stagingPath, payloadPath)
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
