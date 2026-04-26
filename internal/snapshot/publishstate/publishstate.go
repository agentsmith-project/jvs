package publishstate

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jvs-project/jvs/internal/integrity"
	"github.com/jvs-project/jvs/internal/repo"
	"github.com/jvs-project/jvs/internal/snapshotpayload"
	"github.com/jvs-project/jvs/pkg/model"
)

const (
	CodeSnapshotIDInvalid          = "E_SNAPSHOT_ID_INVALID"
	CodeDescriptorMissing          = "E_DESCRIPTOR_MISSING"
	CodeDescriptorCorrupt          = "E_DESCRIPTOR_CORRUPT"
	CodeDescriptorChecksumMismatch = "E_DESCRIPTOR_CHECKSUM_MISMATCH"
	CodeReadyMissing               = "E_READY_MISSING"
	CodeReadyInvalid               = "E_READY_INVALID"
	CodeReadyDescriptorMissing     = "E_READY_DESCRIPTOR_MISSING"
	CodePayloadMissing             = "E_PAYLOAD_MISSING"
	CodePayloadInvalid             = "E_PAYLOAD_INVALID"
	CodePayloadHashMismatch        = "E_PAYLOAD_HASH_MISMATCH"
)

var errDescriptorMissing = errors.New("descriptor missing")

// Options controls how strictly Inspect checks a checkpoint publication.
type Options struct {
	RequireReady             bool
	RequirePayload           bool
	VerifyDescriptorChecksum bool
	VerifyPayloadHash        bool
}

// State describes the readable pieces of a checkpoint publication.
type State struct {
	SnapshotID     model.SnapshotID
	SnapshotDir    string
	DescriptorPath string
	Descriptor     *model.Descriptor
	Ready          bool
	ReadyPath      string
	ReadyMarker    *model.ReadyMarker
}

// Issue is the shared machine-readable classification for damaged checkpoint
// publication state.
type Issue struct {
	Code    string
	Message string
	Path    string
}

func (i *Issue) Error() string {
	if i == nil {
		return ""
	}
	return i.Message
}

type readyMarkerRecord struct {
	path   string
	marker model.ReadyMarker
}

// Inspect validates READY/descriptor/payload consistency for one checkpoint.
func Inspect(repoRoot string, snapshotID model.SnapshotID, opts Options) (*State, *Issue) {
	state := &State{SnapshotID: snapshotID}
	if err := snapshotID.Validate(); err != nil {
		return state, issue(CodeSnapshotIDInvalid, fmt.Sprintf("invalid snapshot ID: %v", err), "")
	}

	var readyMarkers []readyMarkerRecord
	snapshotDir, snapshotErr := repo.SnapshotPathForRead(repoRoot, snapshotID)
	snapshotExists := snapshotErr == nil
	if snapshotExists {
		state.SnapshotDir = snapshotDir
		markers, err := readReadyMarkers(snapshotDir)
		if err != nil {
			return state, issue(CodeReadyInvalid, fmt.Sprintf("READY marker invalid: %v", err), readyIssuePath(err, snapshotDir))
		}
		readyMarkers = markers
		state.Ready = len(readyMarkers) > 0
		if state.Ready {
			state.ReadyPath = readyMarkers[0].path
			marker := readyMarkers[0].marker
			state.ReadyMarker = &marker
		}
		if opts.RequireReady && !state.Ready {
			return state, issue(CodeReadyMissing, "READY marker missing", filepath.Join(snapshotDir, ".READY"))
		}
	} else if snapshotErr != nil && !errors.Is(snapshotErr, os.ErrNotExist) {
		return state, issue(CodePayloadInvalid, fmt.Sprintf("snapshot path invalid: %v", snapshotErr), "")
	}

	descriptorPath, pathErr := repo.SnapshotDescriptorPath(repoRoot, snapshotID)
	if pathErr == nil {
		state.DescriptorPath = descriptorPath
	}
	desc, err := loadDescriptor(repoRoot, snapshotID)
	if err != nil {
		code := descriptorCode(err)
		if state.Ready && code == CodeDescriptorMissing {
			code = CodeReadyDescriptorMissing
		}
		return state, issue(code, err.Error(), descriptorPath)
	}
	state.Descriptor = desc

	for _, marker := range readyMarkers {
		if err := validateReadyMarker(marker.marker, snapshotID, desc); err != nil {
			return state, issue(CodeReadyInvalid, err.Error(), marker.path)
		}
	}

	if opts.VerifyDescriptorChecksum {
		computedChecksum, err := integrity.ComputeDescriptorChecksum(desc)
		if err != nil {
			return state, issue(CodeDescriptorCorrupt, fmt.Sprintf("compute checksum: %v", err), descriptorPath)
		}
		if computedChecksum != desc.DescriptorChecksum {
			return state, issue(CodeDescriptorChecksumMismatch, "descriptor checksum mismatch", descriptorPath)
		}
	}

	if opts.RequirePayload && !snapshotExists {
		return state, issue(CodePayloadMissing, "payload missing", snapshotDir)
	}

	if opts.VerifyPayloadHash {
		if !snapshotExists {
			return state, issue(CodePayloadMissing, "payload missing", snapshotDir)
		}
		computedHash, err := snapshotpayload.ComputeHash(snapshotDir, snapshotpayload.OptionsFromDescriptor(desc))
		if err != nil {
			return state, issue(CodePayloadInvalid, fmt.Sprintf("compute payload hash: %v", err), snapshotDir)
		}
		if computedHash != desc.PayloadRootHash {
			return state, issue(CodePayloadHashMismatch, "payload hash mismatch", snapshotDir)
		}
	}

	return state, nil
}

// ReadyMarkerExists reports whether a snapshot dir has a regular publish
// marker leaf and rejects invalid leaves without following symlinks.
func ReadyMarkerExists(snapshotDir string) (bool, error) {
	_, err := readReadyMarkerLeaves(snapshotDir)
	if err != nil {
		return false, err
	}
	for _, name := range readyMarkerNames {
		if _, err := os.Lstat(filepath.Join(snapshotDir, name)); err == nil {
			return true, nil
		}
	}
	return false, nil
}

var readyMarkerNames = []string{".READY", ".READY.gz"}

type readyMarkerLeaf struct {
	path string
}

type readyMarkerPathError struct {
	path string
	err  error
}

func (e *readyMarkerPathError) Error() string {
	return e.err.Error()
}

func (e *readyMarkerPathError) Unwrap() error {
	return e.err
}

func readReadyMarkers(snapshotDir string) ([]readyMarkerRecord, error) {
	leaves, err := readReadyMarkerLeaves(snapshotDir)
	if err != nil {
		return nil, err
	}
	records := make([]readyMarkerRecord, 0, len(leaves))
	for _, leaf := range leaves {
		data, err := os.ReadFile(leaf.path)
		if err != nil {
			return nil, &readyMarkerPathError{path: leaf.path, err: fmt.Errorf("read ready marker %s: %w", leaf.path, err)}
		}
		var marker model.ReadyMarker
		if err := json.Unmarshal(data, &marker); err != nil {
			return nil, &readyMarkerPathError{path: leaf.path, err: fmt.Errorf("parse ready marker %s: %w", leaf.path, err)}
		}
		records = append(records, readyMarkerRecord{path: leaf.path, marker: marker})
	}
	return records, nil
}

func readReadyMarkerLeaves(snapshotDir string) ([]readyMarkerLeaf, error) {
	var leaves []readyMarkerLeaf
	for _, name := range readyMarkerNames {
		path := filepath.Join(snapshotDir, name)
		info, err := os.Lstat(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, &readyMarkerPathError{path: path, err: fmt.Errorf("stat ready marker %s: %w", path, err)}
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, &readyMarkerPathError{path: path, err: fmt.Errorf("ready marker is symlink: %s", path)}
		}
		if !info.Mode().IsRegular() {
			return nil, &readyMarkerPathError{path: path, err: fmt.Errorf("ready marker is not regular file: %s", path)}
		}
		leaves = append(leaves, readyMarkerLeaf{path: path})
	}
	return leaves, nil
}

func validateReadyMarker(marker model.ReadyMarker, snapshotID model.SnapshotID, desc *model.Descriptor) error {
	if marker.SnapshotID != snapshotID {
		return fmt.Errorf("READY snapshot_id %q does not match requested %q", marker.SnapshotID, snapshotID)
	}
	if marker.Engine != desc.Engine {
		return fmt.Errorf("READY engine %q does not match descriptor %q", marker.Engine, desc.Engine)
	}
	if marker.PayloadHash != desc.PayloadRootHash {
		return fmt.Errorf("READY payload_root_hash %q does not match descriptor %q", marker.PayloadHash, desc.PayloadRootHash)
	}
	if marker.DescriptorChecksum != desc.DescriptorChecksum {
		return fmt.Errorf("READY descriptor_checksum %q does not match descriptor %q", marker.DescriptorChecksum, desc.DescriptorChecksum)
	}
	return nil
}

func loadDescriptor(repoRoot string, snapshotID model.SnapshotID) (*model.Descriptor, error) {
	path, err := repo.SnapshotDescriptorPathForRead(repoRoot, snapshotID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", errDescriptorMissing, snapshotID)
		}
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", errDescriptorMissing, snapshotID)
		}
		return nil, fmt.Errorf("read descriptor: %w", err)
	}
	var desc model.Descriptor
	if err := json.Unmarshal(data, &desc); err != nil {
		return nil, fmt.Errorf("parse descriptor: %w", err)
	}
	if desc.SnapshotID != snapshotID {
		return nil, fmt.Errorf("descriptor snapshot ID %q does not match requested %q", desc.SnapshotID, snapshotID)
	}
	return &desc, nil
}

func issue(code, message, path string) *Issue {
	return &Issue{
		Code:    code,
		Message: message,
		Path:    path,
	}
}

func descriptorCode(err error) string {
	if errors.Is(err, errDescriptorMissing) || errors.Is(err, os.ErrNotExist) || os.IsNotExist(err) {
		return CodeDescriptorMissing
	}
	return CodeDescriptorCorrupt
}

func readyIssuePath(err error, fallback string) string {
	var pathErr *readyMarkerPathError
	if errors.As(err, &pathErr) && pathErr.path != "" {
		return pathErr.path
	}
	return fallback
}
