package model

import (
	"fmt"
	"time"
)

const GCPlanSchemaVersion = 1

const (
	GCProtectionReasonHistory              = "history"
	GCProtectionReasonOpenView             = "open_view"
	GCProtectionReasonActiveRecovery       = "active_recovery"
	GCProtectionReasonActiveOperation      = "active_operation"
	GCProtectionReasonImportedCloneHistory = "imported_clone_history"
)

// Pin protects a snapshot from garbage collection.
type Pin struct {
	PinID      string     `json:"pin_id,omitempty"`
	SnapshotID SnapshotID `json:"snapshot_id"`
	PinnedAt   time.Time  `json:"pinned_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at,omitempty"`
	Reason     string     `json:"reason,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
}

// GCProtectionGroup explains why save points are protected from cleanup.
type GCProtectionGroup struct {
	Reason     string       `json:"reason"`
	Count      int          `json:"count"`
	SavePoints []SnapshotID `json:"save_points"`
}

// GCPlan is the output of gc plan phase.
type GCPlan struct {
	SchemaVersion          int                 `json:"schema_version"`
	RepoID                 string              `json:"repo_id"`
	PlanID                 string              `json:"plan_id"`
	CreatedAt              time.Time           `json:"created_at"`
	ProtectedSet           []SnapshotID        `json:"protected_set"`
	ProtectionGroups       []GCProtectionGroup `json:"protection_groups"`
	ProtectedByLineage     int                 `json:"protected_by_lineage"`
	CandidateCount         int                 `json:"candidate_count"`
	ToDelete               []SnapshotID        `json:"to_delete"`
	DeletableBytesEstimate int64               `json:"deletable_bytes_estimate"`
	ProtectedByRetention   int                 `json:"-"`
	RetentionPolicy        RetentionPolicy     `json:"-"`
}

// Tombstone marks a snapshot as deleted but not yet reclaimed.
type Tombstone struct {
	SnapshotID  SnapshotID `json:"snapshot_id"`
	DeletedAt   time.Time  `json:"deleted_at"`
	Reclaimable bool       `json:"reclaimable"`
	GCState     string     `json:"gc_state,omitempty"`
	Reason      string     `json:"reason,omitempty"`
}

const (
	GCStateMarked    = "marked"
	GCStateCommitted = "committed"
	GCStateFailed    = "failed"
)

// DefaultRetentionPolicy returns the default retention policy.
func DefaultRetentionPolicy() RetentionPolicy {
	return RetentionPolicy{
		KeepMinSnapshots: 0,
		KeepMinAge:       0,
	}
}

// RetentionPolicy configures internal compatibility retention rules. The v0
// public CLI does not expose retention flags; default GC protection is live
// workspace lineage plus in-progress operation intents.
//
// Snapshots are protected if they match ANY of these rules:
// - Within the last N snapshots (KeepMinSnapshots)
// - Created within the last duration (KeepMinAge)
// - Part of a worktree's lineage
type RetentionPolicy struct {
	// KeepMinSnapshots ensures at least N snapshots are always kept.
	// The most recent snapshots by creation time are protected.
	KeepMinSnapshots int `json:"-"`

	// KeepMinAge protects snapshots younger than this duration.
	// Snapshots created within this time window are never deleted.
	KeepMinAge time.Duration `json:"-"`
}

// Validate checks if the retention policy is valid.
func (rp *RetentionPolicy) Validate() error {
	if rp.KeepMinSnapshots < 0 {
		return &InvalidRetentionPolicyError{
			Field:  "keep_min_snapshots",
			Reason: "must be non-negative",
			Value:  rp.KeepMinSnapshots,
		}
	}
	if rp.KeepMinAge < 0 {
		return &InvalidRetentionPolicyError{
			Field:  "keep_min_age",
			Reason: "must be non-negative",
			Value:  rp.KeepMinAge,
		}
	}
	return nil
}

// InvalidRetentionPolicyError is returned when a retention policy is invalid.
type InvalidRetentionPolicyError struct {
	Field  string
	Reason string
	Value  interface{}
}

func (e *InvalidRetentionPolicyError) Error() string {
	return fmt.Sprintf("invalid retention policy: %s %s (got: %v)", e.Field, e.Reason, e.Value)
}
