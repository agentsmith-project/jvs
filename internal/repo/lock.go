package repo

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/agentsmith-project/jvs/pkg/errclass"
	"github.com/agentsmith-project/jvs/pkg/fsutil"
)

const (
	mutationLocksDirName = "locks"
	mutationLockDirName  = "repo.lock"
	mutationLockOwner    = "owner.json"
	mutationLockStaleAge = 30 * time.Second
)

// MutationLock is a no-wait repository-wide lock for metadata/payload
// mutations. It is implemented with atomic mkdir so contenders fail
// immediately with E_REPO_BUSY instead of blocking.
type MutationLock struct {
	path      string
	ownerPath string
	released  bool
}

// MutationLockStatus describes the observed state of the repository mutation
// lock.
type MutationLockStatus string

const (
	MutationLockAbsent  MutationLockStatus = "absent"
	MutationLockHeld    MutationLockStatus = "held"
	MutationLockStale   MutationLockStatus = "stale"
	MutationLockInvalid MutationLockStatus = "invalid"
)

// MutationLockOwner is the process identity written by the lock holder.
type MutationLockOwner struct {
	Operation string    `json:"operation"`
	PID       int       `json:"pid"`
	Hostname  string    `json:"hostname,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// MutationLockInspection is a fail-closed view of the repository mutation lock.
type MutationLockInspection struct {
	Path         string
	OwnerPath    string
	Owner        *MutationLockOwner
	Status       MutationLockStatus
	SafeToRemove bool
	Reason       string
}

type processLiveness int

const (
	processLivenessUnknown processLiveness = iota
	processAlive
	processGone
)

type processLivenessChecker func(pid int) (processLiveness, error)

var mutationLockProcessLiveness processLivenessChecker = defaultProcessLiveness

type mutationLockOwnerDiskRecord struct {
	Operation  string    `json:"operation"`
	PID        int       `json:"pid"`
	Hostname   string    `json:"hostname,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	AcquiredAt time.Time `json:"acquired_at,omitempty"`
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

// InspectMutationLock returns a non-mutating, fail-closed view of repo.lock.
func InspectMutationLock(repoRoot string) (MutationLockInspection, error) {
	return inspectMutationLock(repoRoot, time.Now().UTC())
}

// RemoveStaleMutationLock removes repo.lock only when inspection proves the
// owner is on this host, the process is gone, and the lock is old enough.
func RemoveStaleMutationLock(repoRoot string) (MutationLockInspection, bool, error) {
	inspection, err := InspectMutationLock(repoRoot)
	if err != nil {
		return inspection, false, err
	}
	if !inspection.SafeToRemove {
		return inspection, false, nil
	}

	if ok, err := mutationLockContainsOnlyOwner(inspection.Path); err != nil {
		if os.IsNotExist(err) {
			latest, inspectErr := InspectMutationLock(repoRoot)
			if inspectErr != nil {
				return inspection, false, inspectErr
			}
			return latest, false, nil
		}
		return inspection, false, fmt.Errorf("inspect stale mutation lock contents: %w", err)
	} else if !ok {
		return inspection, false, fmt.Errorf("stale mutation lock contains unexpected entries")
	}

	latest, err := InspectMutationLock(repoRoot)
	if err != nil {
		return inspection, false, err
	}
	if !sameMutationLockIdentity(inspection, latest) || !latest.SafeToRemove {
		return latest, false, nil
	}

	if err := os.Remove(latest.OwnerPath); err != nil && !os.IsNotExist(err) {
		return inspection, false, fmt.Errorf("remove stale mutation lock owner: %w", err)
	}
	if err := os.Remove(latest.Path); err != nil && !os.IsNotExist(err) {
		return inspection, false, fmt.Errorf("remove stale mutation lock: %w", err)
	}
	if err := fsutil.FsyncDir(filepath.Dir(latest.Path)); err != nil {
		return inspection, false, fmt.Errorf("fsync stale mutation lock parent: %w", err)
	}
	return latest, true, nil
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
	record := MutationLockOwner{
		Operation: operation,
		PID:       os.Getpid(),
		Hostname:  hostname,
		CreatedAt: time.Now().UTC(),
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

func inspectMutationLock(repoRoot string, now time.Time) (MutationLockInspection, error) {
	locksDir, err := mutationLocksDirForRead(repoRoot)
	if err != nil {
		return MutationLockInspection{}, err
	}
	lockPath := filepath.Join(locksDir, mutationLockDirName)
	ownerPath := filepath.Join(lockPath, mutationLockOwner)
	inspection := MutationLockInspection{
		Path:      lockPath,
		OwnerPath: ownerPath,
		Status:    MutationLockAbsent,
	}

	info, err := os.Lstat(lockPath)
	if err != nil {
		if os.IsNotExist(err) {
			inspection.Reason = "repo lock is absent"
			return inspection, nil
		}
		return inspection, fmt.Errorf("inspect mutation lock path: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		inspection.Status = MutationLockInvalid
		inspection.Reason = fmt.Sprintf("mutation lock path is not a directory: %s", lockPath)
		return inspection, nil
	}

	owner, err := readMutationLockOwner(ownerPath)
	if err != nil {
		inspection.Status = MutationLockInvalid
		inspection.Reason = err.Error()
		return inspection, nil
	}
	inspection.Owner = owner
	return classifyMutationLockInspection(inspection, now), nil
}

func mutationLocksDirForRead(repoRoot string) (string, error) {
	root := filepath.Clean(repoRoot)
	if err := validateExistingRealDir(root); err != nil {
		return "", err
	}
	jvsDir := filepath.Join(root, JVSDirName)
	if err := validateExistingRealDir(jvsDir); err != nil {
		return "", err
	}
	locksDir := filepath.Join(jvsDir, mutationLocksDirName)
	info, err := os.Lstat(locksDir)
	if err != nil {
		if os.IsNotExist(err) {
			return locksDir, nil
		}
		return "", fmt.Errorf("inspect mutation locks directory: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("mutation locks path is not a directory: %s", locksDir)
	}
	return locksDir, nil
}

func readMutationLockOwner(path string) (*MutationLockOwner, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect mutation lock owner: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("mutation lock owner is not a regular file: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read mutation lock owner: %w", err)
	}
	var disk mutationLockOwnerDiskRecord
	if err := json.Unmarshal(data, &disk); err != nil {
		return nil, fmt.Errorf("parse mutation lock owner: %w", err)
	}
	createdAt := disk.CreatedAt
	if createdAt.IsZero() {
		createdAt = disk.AcquiredAt
	}
	return &MutationLockOwner{
		Operation: disk.Operation,
		PID:       disk.PID,
		Hostname:  disk.Hostname,
		CreatedAt: createdAt,
	}, nil
}

func classifyMutationLockInspection(inspection MutationLockInspection, now time.Time) MutationLockInspection {
	owner := inspection.Owner
	if owner == nil {
		inspection.Status = MutationLockInvalid
		inspection.Reason = "mutation lock owner missing"
		return inspection
	}
	if owner.PID <= 0 {
		inspection.Status = MutationLockInvalid
		inspection.Reason = "mutation lock owner pid invalid"
		return inspection
	}
	if owner.CreatedAt.IsZero() {
		inspection.Status = MutationLockInvalid
		inspection.Reason = "mutation lock owner created_at missing"
		return inspection
	}

	localHost, err := os.Hostname()
	if err != nil || localHost == "" {
		inspection.Status = MutationLockHeld
		inspection.Reason = "local hostname unavailable"
		return inspection
	}
	if owner.Hostname == "" {
		inspection.Status = MutationLockHeld
		inspection.Reason = "mutation lock owner hostname unknown"
		return inspection
	}
	if !strings.EqualFold(owner.Hostname, localHost) {
		inspection.Status = MutationLockHeld
		inspection.Reason = fmt.Sprintf("mutation lock owner host differs: %s", owner.Hostname)
		return inspection
	}

	liveness, err := mutationLockProcessLiveness(owner.PID)
	if err != nil {
		inspection.Status = MutationLockHeld
		inspection.Reason = fmt.Sprintf("mutation lock owner pid status unknown: %v", err)
		return inspection
	}
	switch liveness {
	case processAlive:
		inspection.Status = MutationLockHeld
		inspection.Reason = fmt.Sprintf("mutation lock owner pid %d is alive", owner.PID)
		return inspection
	case processGone:
	default:
		inspection.Status = MutationLockHeld
		inspection.Reason = "mutation lock owner pid status unknown"
		return inspection
	}

	age := now.Sub(owner.CreatedAt)
	if age < mutationLockStaleAge {
		inspection.Status = MutationLockHeld
		inspection.Reason = fmt.Sprintf("mutation lock owner pid %d is gone but lock age %s is below stale threshold", owner.PID, age.Round(time.Second))
		return inspection
	}

	inspection.Status = MutationLockStale
	inspection.SafeToRemove = true
	inspection.Reason = fmt.Sprintf("mutation lock owner pid %d is gone on this host", owner.PID)
	return inspection
}

func mutationLockContainsOnlyOwner(path string) (bool, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return false, err
	}
	return len(entries) == 1 && entries[0].Name() == mutationLockOwner, nil
}

func sameMutationLockIdentity(a, b MutationLockInspection) bool {
	return a.Path == b.Path &&
		a.OwnerPath == b.OwnerPath &&
		sameMutationLockOwner(a.Owner, b.Owner)
}

func sameMutationLockOwner(a, b *MutationLockOwner) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Operation == b.Operation &&
		a.PID == b.PID &&
		a.Hostname == b.Hostname &&
		a.CreatedAt.Equal(b.CreatedAt)
}

func defaultProcessLiveness(pid int) (processLiveness, error) {
	if pid <= 0 {
		return processLivenessUnknown, fmt.Errorf("invalid pid %d", pid)
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return processLivenessUnknown, err
	}
	err = process.Signal(syscall.Signal(0))
	if err == nil {
		return processAlive, nil
	}
	if errors.Is(err, os.ErrProcessDone) || errors.Is(err, syscall.ESRCH) {
		return processGone, nil
	}
	if errors.Is(err, syscall.EPERM) {
		return processAlive, nil
	}
	return processLivenessUnknown, err
}
