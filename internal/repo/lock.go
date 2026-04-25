package repo

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jvs-project/jvs/pkg/errclass"
	"github.com/jvs-project/jvs/pkg/fsutil"
)

const (
	mutationLocksDirName = "locks"
	mutationLockDirName  = "repo.lock"
	mutationLockOwner    = "owner.json"
)

// MutationLock is a no-wait repository-wide lock for metadata/payload
// mutations. It is implemented with atomic mkdir so contenders fail
// immediately with E_REPO_BUSY instead of blocking.
type MutationLock struct {
	path      string
	ownerPath string
	released  bool
}

type mutationLockOwnerRecord struct {
	Operation  string    `json:"operation"`
	PID        int       `json:"pid"`
	Hostname   string    `json:"hostname,omitempty"`
	AcquiredAt time.Time `json:"acquired_at"`
}

// AcquireMutationLock attempts to acquire the repository mutation lock without
// waiting. Call Release when the mutation is complete.
func AcquireMutationLock(repoRoot, operation string) (*MutationLock, error) {
	if operation == "" {
		operation = "mutation"
	}

	locksDir, err := mutationLocksDir(repoRoot)
	if err != nil {
		return nil, err
	}
	lockPath := filepath.Join(locksDir, mutationLockDirName)
	if err := os.Mkdir(lockPath, 0700); err != nil {
		if os.IsExist(err) {
			info, statErr := os.Lstat(lockPath)
			if statErr == nil && info.IsDir() {
				return nil, errclass.ErrRepoBusy.WithMessagef("repository mutation lock is held for %s", operation)
			}
			if statErr != nil {
				return nil, errclass.ErrLockConflict.WithMessagef("inspect mutation lock path: %v", statErr)
			}
			return nil, errclass.ErrLockConflict.WithMessagef("mutation lock path is not a directory: %s", lockPath)
		}
		return nil, fmt.Errorf("create mutation lock: %w", err)
	}

	lock := &MutationLock{
		path:      lockPath,
		ownerPath: filepath.Join(lockPath, mutationLockOwner),
	}

	if err := writeMutationLockOwner(lock.ownerPath, operation); err != nil {
		_ = lock.Release()
		return nil, err
	}
	if err := fsutil.FsyncDir(locksDir); err != nil {
		_ = lock.Release()
		return nil, fmt.Errorf("fsync mutation lock parent: %w", err)
	}
	return lock, nil
}

// WithMutationLock acquires the repository mutation lock, runs fn, and always
// releases the lock before returning.
func WithMutationLock(repoRoot, operation string, fn func() error) error {
	lock, err := AcquireMutationLock(repoRoot, operation)
	if err != nil {
		return err
	}
	err = fn()
	releaseErr := lock.Release()
	if err != nil {
		return err
	}
	return releaseErr
}

// Release releases the mutation lock. It is safe to call more than once.
func (l *MutationLock) Release() error {
	if l == nil || l.released {
		return nil
	}
	if err := os.Remove(l.ownerPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove mutation lock owner: %w", err)
	}
	if err := os.Remove(l.path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove mutation lock: %w", err)
	}
	l.released = true
	return fsutil.FsyncDir(filepath.Dir(l.path))
}

func mutationLocksDir(repoRoot string) (string, error) {
	root := filepath.Clean(repoRoot)
	if err := validateExistingRealDir(root); err != nil {
		return "", err
	}
	jvsDir := filepath.Join(root, JVSDirName)
	if err := validateExistingRealDir(jvsDir); err != nil {
		return "", err
	}
	locksDir := filepath.Join(jvsDir, mutationLocksDirName)
	if err := os.MkdirAll(locksDir, 0755); err != nil {
		return "", fmt.Errorf("create mutation locks directory: %w", err)
	}
	if err := validateExistingRealDir(locksDir); err != nil {
		return "", err
	}
	return locksDir, nil
}

func writeMutationLockOwner(path, operation string) error {
	hostname, _ := os.Hostname()
	record := mutationLockOwnerRecord{
		Operation:  operation,
		PID:        os.Getpid(),
		Hostname:   hostname,
		AcquiredAt: time.Now().UTC(),
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal mutation lock owner: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write mutation lock owner: %w", err)
	}
	return fsutil.FsyncDir(filepath.Dir(path))
}
