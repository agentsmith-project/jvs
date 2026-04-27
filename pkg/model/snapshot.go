package model

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"time"
)

// SnapshotID is the unique identifier for a snapshot: <unix_ms>-<rand8hex>
type SnapshotID string

var snapshotIDRegex = regexp.MustCompile(`^\d{13}-[0-9a-f]{8}$`)

// NewSnapshotID generates a new unique snapshot ID.
func NewSnapshotID() SnapshotID {
	ts := time.Now().UnixMilli()
	var randBytes [4]byte
	if _, err := rand.Read(randBytes[:]); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return SnapshotID(fmt.Sprintf("%013d-%s", ts, hex.EncodeToString(randBytes[:])))
}

// ShortID returns the first 8 characters for display.
func (id SnapshotID) ShortID() string {
	s := string(id)
	if len(s) >= 8 {
		return s[:8]
	}
	return s
}

// String returns the full snapshot ID as string.
func (id SnapshotID) String() string {
	return string(id)
}

// Validate rejects non-canonical or path-like snapshot IDs.
func (id SnapshotID) Validate() error {
	if !snapshotIDRegex.MatchString(string(id)) {
		return fmt.Errorf("invalid snapshot ID %q: must match ^\\d{13}-[0-9a-f]{8}$", string(id))
	}
	return nil
}

// IsValid reports whether id is in canonical on-disk snapshot ID form.
func (id SnapshotID) IsValid() bool {
	return id.Validate() == nil
}

// Descriptor is the on-disk snapshot metadata.
type Descriptor struct {
	SnapshotID           SnapshotID            `json:"snapshot_id"`
	ParentID             *SnapshotID           `json:"parent_id,omitempty"`
	StartedFrom          *SnapshotID           `json:"started_from,omitempty"`
	RestoredFrom         *SnapshotID           `json:"restored_from,omitempty"`
	RestoredPaths        []RestoredPathSource  `json:"restored_paths,omitempty"`
	WorktreeName         string                `json:"worktree_name"`
	CreatedAt            time.Time             `json:"created_at"`
	Note                 string                `json:"note,omitempty"`
	Tags                 []string              `json:"tags,omitempty"`
	Engine               EngineType            `json:"engine"`
	ActualEngine         EngineType            `json:"actual_engine,omitempty"`
	EffectiveEngine      EngineType            `json:"effective_engine,omitempty"`
	DegradedReasons      []string              `json:"degraded_reasons,omitempty"`
	MetadataPreservation *MetadataPreservation `json:"metadata_preservation,omitempty"`
	PerformanceClass     string                `json:"performance_class,omitempty"`
	PayloadRootHash      HashValue             `json:"payload_root_hash"`
	DescriptorChecksum   HashValue             `json:"descriptor_checksum"`
	IntegrityState       IntegrityState        `json:"integrity_state"`
	// PartialPaths is set for partial snapshots, listing the specific paths included.
	// Empty or nil means a full worktree snapshot.
	PartialPaths []string `json:"partial_paths,omitempty"`
	// Compression stores compression metadata if the snapshot is compressed.
	Compression *CompressionInfo `json:"compression,omitempty"`
}

// CompressionInfo stores compression metadata for snapshots.
type CompressionInfo struct {
	Type  string `json:"type"`  // e.g., "gzip"
	Level int    `json:"level"` // Compression level (0-9)
}

// ReadyMarker is the .READY file content indicating complete snapshot.
type ReadyMarker struct {
	SnapshotID         SnapshotID `json:"snapshot_id"`
	CompletedAt        time.Time  `json:"completed_at"`
	PayloadHash        HashValue  `json:"payload_root_hash"`
	Engine             EngineType `json:"engine"`
	DescriptorChecksum HashValue  `json:"descriptor_checksum"`
}

// IntentRecord tracks in-progress snapshot creation for crash recovery.
type IntentRecord struct {
	SnapshotID   SnapshotID `json:"snapshot_id"`
	WorktreeName string     `json:"worktree_name"`
	StartedAt    time.Time  `json:"started_at"`
	Engine       EngineType `json:"engine"`
}
