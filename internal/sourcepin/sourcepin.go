// Package sourcepin manages active source save point pins.
package sourcepin

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/pkg/fsutil"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/agentsmith-project/jvs/pkg/pathutil"
	"github.com/agentsmith-project/jvs/pkg/uuidutil"
)

const pinFileSuffix = ".json"

type Manager struct {
	repoRoot string
}

type Handle struct {
	manager *Manager
	Pin     model.Pin
}

func NewManager(repoRoot string) *Manager {
	return &Manager{repoRoot: repoRoot}
}

func (m *Manager) Create(snapshotID model.SnapshotID, reason string) (*Handle, error) {
	return m.CreateWithID(snapshotID, "source-"+uuidutil.NewV4(), reason)
}

func (m *Manager) CreateWithID(snapshotID model.SnapshotID, pinID, reason string) (*Handle, error) {
	if err := snapshotID.Validate(); err != nil {
		return nil, fmt.Errorf("snapshot ID: %w", err)
	}
	if err := validatePinID(pinID); err != nil {
		return nil, err
	}
	pinsDir, err := m.ensurePinsDir()
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	pin := model.Pin{
		PinID:      pinID,
		SnapshotID: snapshotID,
		PinnedAt:   now,
		CreatedAt:  now,
		Reason:     reason,
	}
	data, err := json.MarshalIndent(pin, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal active source pin: %w", err)
	}
	path := filepath.Join(pinsDir, pinFileName(pinID))
	if err := writePinNoReplace(path, data, 0644); err != nil {
		return nil, fmt.Errorf("write active source pin: %w", err)
	}
	return &Handle{manager: m, Pin: pin}, nil
}

func (h *Handle) Release() error {
	if h == nil || h.manager == nil {
		return nil
	}
	return h.manager.RemoveIfMatches(h.Pin)
}

func (m *Manager) Remove(pinID string) error {
	if err := validatePinID(pinID); err != nil {
		return err
	}
	return fmt.Errorf("active source pin removal requires matching pin identity")
}

func (m *Manager) RemoveIfMatches(expected model.Pin) error {
	if err := validatePinID(expected.PinID); err != nil {
		return err
	}
	if err := expected.SnapshotID.Validate(); err != nil {
		return fmt.Errorf("snapshot ID: %w", err)
	}
	pinsDir, err := repo.GCPinsDirPath(m.repoRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("resolve active source pins: %w", err)
	}
	path := filepath.Join(pinsDir, pinFileName(expected.PinID))
	infoBefore, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat active source pin: %w", err)
	}
	if infoBefore.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("active source pin is not safe to remove")
	}
	if !infoBefore.Mode().IsRegular() {
		return fmt.Errorf("active source pin is not a regular file")
	}
	current, err := readPinFile(path, pinFileName(expected.PinID))
	if err != nil {
		return err
	}
	if !samePinIdentity(*current, expected) {
		return fmt.Errorf("active source pin changed; refusing to remove it")
	}
	infoAfter, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat active source pin: %w", err)
	}
	if !os.SameFile(infoBefore, infoAfter) {
		return fmt.Errorf("active source pin changed; refusing to remove it")
	}
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("remove active source pin: %w", err)
	}
	if err := fsutil.FsyncDir(pinsDir); err != nil {
		return fmt.Errorf("sync active source pins: %w", err)
	}
	return nil
}

func (m *Manager) Read(pinID string) (*model.Pin, error) {
	if err := validatePinID(pinID); err != nil {
		return nil, err
	}
	path, err := repo.GCPinPathForRead(m.repoRoot, pinFileName(pinID))
	if err != nil {
		return nil, fmt.Errorf("read active source pin: %w", err)
	}
	pin, err := readPinFile(path, pinFileName(pinID))
	if err != nil {
		return nil, err
	}
	return pin, nil
}

func (m *Manager) ProtectedSnapshotIDs() ([]model.SnapshotID, error) {
	pins, err := m.List()
	if err != nil {
		return nil, err
	}
	protected := make(map[model.SnapshotID]bool)
	for _, pin := range pins {
		protected[pin.SnapshotID] = true
	}
	ids := make([]model.SnapshotID, 0, len(protected))
	for id := range protected {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, nil
}

func (m *Manager) List() ([]model.Pin, error) {
	pinsDir, err := repo.GCPinsDirPath(m.repoRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("active source pins: %w", err)
	}
	entries, err := os.ReadDir(pinsDir)
	if err != nil {
		return nil, fmt.Errorf("read active source pins: %w", err)
	}

	pins := make([]model.Pin, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.Type()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("active source pin %q is a symlink", name)
		}
		if entry.IsDir() {
			return nil, fmt.Errorf("active source pin %q is not a regular file", name)
		}
		if _, err := pinIDFromFileName(name); err != nil {
			return nil, err
		}
		path, err := repo.GCPinPathForRead(m.repoRoot, name)
		if err != nil {
			return nil, fmt.Errorf("active source pin %q is not readable: %w", name, err)
		}
		pin, err := readPinFile(path, name)
		if err != nil {
			return nil, err
		}
		pins = append(pins, *pin)
	}
	sort.Slice(pins, func(i, j int) bool { return pins[i].PinID < pins[j].PinID })
	return pins, nil
}

func (m *Manager) ensurePinsDir() (string, error) {
	gcDir, err := repo.GCDirPath(m.repoRoot)
	if err != nil {
		return "", fmt.Errorf("resolve gc control data: %w", err)
	}
	pinsDir := filepath.Join(gcDir, "pins")
	info, err := os.Lstat(pinsDir)
	if err != nil {
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("stat active source pins: %w", err)
		}
		if err := os.Mkdir(pinsDir, 0755); err != nil && !os.IsExist(err) {
			return "", fmt.Errorf("create active source pins: %w", err)
		}
		if err := fsutil.FsyncDir(gcDir); err != nil {
			return "", fmt.Errorf("sync active source pins: %w", err)
		}
		return pinsDir, nil
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("active source pins path is a symlink")
	}
	if !info.IsDir() {
		return "", fmt.Errorf("active source pins path is not a directory")
	}
	return pinsDir, nil
}

func readPinFile(path, fileName string) (*model.Pin, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read active source pin %q: %w", fileName, err)
	}
	var pin model.Pin
	if err := json.Unmarshal(data, &pin); err != nil {
		return nil, fmt.Errorf("active source pin %q is not valid JSON: %w", fileName, err)
	}
	pinID, err := pinIDFromFileName(fileName)
	if err != nil {
		return nil, err
	}
	if err := validatePin(pin, pinID); err != nil {
		return nil, fmt.Errorf("active source pin %q: %w", fileName, err)
	}
	return &pin, nil
}

func validatePin(pin model.Pin, expectedPinID string) error {
	if pin.PinID == "" {
		return fmt.Errorf("pin ID is required")
	}
	if pin.PinID != expectedPinID {
		return fmt.Errorf("pin ID does not match file name")
	}
	if err := validatePinID(pin.PinID); err != nil {
		return err
	}
	if err := pin.SnapshotID.Validate(); err != nil {
		return fmt.Errorf("snapshot ID: %w", err)
	}
	return nil
}

func samePinIdentity(left, right model.Pin) bool {
	return left.PinID == right.PinID &&
		left.SnapshotID == right.SnapshotID &&
		left.Reason == right.Reason &&
		left.CreatedAt.Equal(right.CreatedAt) &&
		left.PinnedAt.Equal(right.PinnedAt)
}

func validatePinID(pinID string) error {
	if strings.TrimSpace(pinID) != pinID || pinID == "" || pinID == "." {
		return fmt.Errorf("pin ID must be a safe name")
	}
	if err := pathutil.ValidateName(pinID); err != nil {
		return fmt.Errorf("pin ID must be a safe name: %w", err)
	}
	return nil
}

func pinFileName(pinID string) string {
	return pinID + pinFileSuffix
}

func pinIDFromFileName(fileName string) (string, error) {
	if !strings.HasSuffix(fileName, pinFileSuffix) {
		return "", fmt.Errorf("active source pin %q must be a JSON file", fileName)
	}
	if err := pathutil.ValidateName(fileName); err != nil {
		return "", fmt.Errorf("active source pin file name is unsafe: %w", err)
	}
	pinID := strings.TrimSuffix(fileName, pinFileSuffix)
	if err := validatePinID(pinID); err != nil {
		return "", err
	}
	if pinFileName(pinID) != fileName {
		return "", fmt.Errorf("active source pin file name is not canonical")
	}
	return pinID, nil
}

func writePinNoReplace(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".jvs-pin-*")
	if err != nil {
		return fmt.Errorf("create temporary pin: %w", err)
	}
	tmpPath := tmp.Name()
	success := false
	defer func() {
		if !success {
			_ = tmp.Close()
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("write temporary pin: %w", err)
	}
	if err := tmp.Chmod(perm); err != nil {
		return fmt.Errorf("chmod temporary pin: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync temporary pin: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temporary pin: %w", err)
	}
	if err := fsutil.RenameNoReplaceAndSync(tmpPath, path); err != nil {
		if os.IsExist(err) || errors.Is(err, os.ErrExist) {
			return fmt.Errorf("active source pin already exists")
		}
		if errors.Is(err, fsutil.ErrRenameNoReplaceUnsupported) {
			return fmt.Errorf("no-replace pin create unsupported: %w", err)
		}
		return err
	}
	success = true
	return nil
}
