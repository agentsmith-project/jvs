package model_test

import (
	"regexp"
	"testing"
	"time"

	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var snapshotIDPattern = regexp.MustCompile(`^\d{13}-[0-9a-f]{8}$`)

func TestNewSnapshotID_Format(t *testing.T) {
	id := model.NewSnapshotID()
	require.Regexp(t, snapshotIDPattern, string(id))
}

func TestSnapshotID_ShortID(t *testing.T) {
	id := model.SnapshotID("1708300800000-a3f7c1b2")
	assert.Equal(t, "17083008", id.ShortID())
}

func TestSnapshotID_ShortID_ShortInput(t *testing.T) {
	id := model.SnapshotID("abc")
	assert.Equal(t, "abc", id.ShortID())
}

func TestSnapshotID_ShortID_Empty(t *testing.T) {
	id := model.SnapshotID("")
	assert.Equal(t, "", id.ShortID())
}

func TestSnapshotID_String(t *testing.T) {
	id := model.SnapshotID("1708300800000-a3f7c1b2")
	assert.Equal(t, "1708300800000-a3f7c1b2", id.String())
}

func TestSnapshotID_Validate(t *testing.T) {
	tests := []struct {
		name string
		id   model.SnapshotID
	}{
		{name: "generated shape", id: "1708300800000-a3f7c1b2"},
		{name: "all zero random suffix is still canonical", id: "1708300800000-00000000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NoError(t, tt.id.Validate())
			assert.True(t, tt.id.IsValid())
		})
	}
}

func TestSnapshotID_ValidateRejectsUnsafeOrNonCanonical(t *testing.T) {
	invalid := []model.SnapshotID{
		"",
		"/1708300800000-a3f7c1b2",
		"1708300800000/a3f7c1b2",
		`1708300800000\a3f7c1b2`,
		"..",
		"../1708300800000-a3f7c1b2",
		"1708300800000-A3F7C1B2",
		"170830080000-a3f7c1b2",
		"1708300800000-a3f7c1b",
		"1708300800000-a3f7c1b2.json",
	}
	for _, id := range invalid {
		t.Run(string(id), func(t *testing.T) {
			require.Error(t, id.Validate())
			assert.False(t, id.IsValid())
		})
	}
}

func TestNewSnapshotID_Uniqueness(t *testing.T) {
	seen := make(map[model.SnapshotID]bool)
	for i := 0; i < 100; i++ {
		id := model.NewSnapshotID()
		assert.False(t, seen[id], "duplicate: %s", id)
		seen[id] = true
	}
}

func TestWorktreeConfig_IsDetached(t *testing.T) {
	tests := []struct {
		name     string
		config   model.WorktreeConfig
		expected bool
	}{
		{
			name: "not detached when no snapshots",
			config: model.WorktreeConfig{
				Name:             "test",
				HeadSnapshotID:   "",
				LatestSnapshotID: "",
			},
			expected: false,
		},
		{
			name: "not detached when head equals latest",
			config: model.WorktreeConfig{
				Name:             "test",
				HeadSnapshotID:   "1708300800000-a3f7c1b2",
				LatestSnapshotID: "1708300800000-a3f7c1b2",
			},
			expected: false,
		},
		{
			name: "detached when head differs from latest",
			config: model.WorktreeConfig{
				Name:             "test",
				HeadSnapshotID:   "1708300800000-a3f7c1b2",
				LatestSnapshotID: "1708300900000-b4d8e2c3",
			},
			expected: true,
		},
		{
			name: "not detached when latest is empty but head is set",
			config: model.WorktreeConfig{
				Name:             "test",
				HeadSnapshotID:   "1708300800000-a3f7c1b2",
				LatestSnapshotID: "",
			},
			expected: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.config.IsDetached())
		})
	}
}

func TestWorktreeConfig_CanSnapshot(t *testing.T) {
	tests := []struct {
		name     string
		config   model.WorktreeConfig
		expected bool
	}{
		{
			name: "can snapshot when no snapshots exist (first snapshot)",
			config: model.WorktreeConfig{
				Name:             "test",
				HeadSnapshotID:   "",
				LatestSnapshotID: "",
			},
			expected: true,
		},
		{
			name: "can snapshot when at HEAD with snapshots",
			config: model.WorktreeConfig{
				Name:             "test",
				HeadSnapshotID:   "1708300800000-a3f7c1b2",
				LatestSnapshotID: "1708300800000-a3f7c1b2",
			},
			expected: true,
		},
		{
			name: "cannot snapshot when detached",
			config: model.WorktreeConfig{
				Name:             "test",
				HeadSnapshotID:   "1708300800000-a3f7c1b2",
				LatestSnapshotID: "1708300900000-b4d8e2c3",
			},
			expected: false,
		},
		{
			name: "can snapshot when latest is empty (fresh worktree with head set)",
			config: model.WorktreeConfig{
				Name:             "test",
				HeadSnapshotID:   "1708300800000-a3f7c1b2",
				LatestSnapshotID: "",
			},
			expected: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.config.CanSnapshot())
		})
	}
}

func TestDescriptor_Fields(t *testing.T) {
	parentID := model.SnapshotID("1708300700000-12345678")
	desc := model.Descriptor{
		SnapshotID:         "1708300800000-a3f7c1b2",
		ParentID:           &parentID,
		WorktreeName:       "main",
		CreatedAt:          time.Date(2024, 2, 19, 0, 0, 0, 0, time.UTC),
		Note:               "test snapshot",
		Tags:               []string{"v1.0", "release"},
		Engine:             model.EngineJuiceFSClone,
		PayloadRootHash:    "abc123",
		DescriptorChecksum: "def456",
		IntegrityState:     model.IntegrityVerified,
	}

	assert.Equal(t, model.SnapshotID("1708300800000-a3f7c1b2"), desc.SnapshotID)
	assert.Equal(t, &parentID, desc.ParentID)
	assert.Equal(t, "main", desc.WorktreeName)
	assert.Equal(t, "test snapshot", desc.Note)
	assert.Equal(t, []string{"v1.0", "release"}, desc.Tags)
	assert.Equal(t, model.EngineJuiceFSClone, desc.Engine)
}

func TestDescriptor_NoParent(t *testing.T) {
	desc := model.Descriptor{
		SnapshotID:   "1708300800000-a3f7c1b2",
		WorktreeName: "main",
	}

	assert.Nil(t, desc.ParentID)
}

func TestEngineTypes(t *testing.T) {
	assert.Equal(t, model.EngineType("juicefs-clone"), model.EngineJuiceFSClone)
	assert.Equal(t, model.EngineType("reflink-copy"), model.EngineReflinkCopy)
	assert.Equal(t, model.EngineType("copy"), model.EngineCopy)
}

func TestIntegrityStates(t *testing.T) {
	assert.Equal(t, model.IntegrityState("verified"), model.IntegrityVerified)
	assert.Equal(t, model.IntegrityState("tampered"), model.IntegrityTampered)
	assert.Equal(t, model.IntegrityState("unknown"), model.IntegrityUnknown)
}

func TestReadyMarker_Fields(t *testing.T) {
	marker := model.ReadyMarker{
		SnapshotID:         "1708300800000-a3f7c1b2",
		CompletedAt:        time.Date(2024, 2, 19, 0, 0, 0, 0, time.UTC),
		PayloadHash:        "abc123",
		Engine:             model.EngineReflinkCopy,
		DescriptorChecksum: "def456",
	}

	assert.Equal(t, model.SnapshotID("1708300800000-a3f7c1b2"), marker.SnapshotID)
	assert.Equal(t, model.EngineReflinkCopy, marker.Engine)
}

func TestIntentRecord_Fields(t *testing.T) {
	intent := model.IntentRecord{
		SnapshotID:   "1708300800000-a3f7c1b2",
		WorktreeName: "main",
		StartedAt:    time.Date(2024, 2, 19, 0, 0, 0, 0, time.UTC),
		Engine:       model.EngineCopy,
	}

	assert.Equal(t, model.SnapshotID("1708300800000-a3f7c1b2"), intent.SnapshotID)
	assert.Equal(t, "main", intent.WorktreeName)
	assert.Equal(t, model.EngineCopy, intent.Engine)
}

func TestAuditEventTypes(t *testing.T) {
	assert.Equal(t, model.AuditEventType("snapshot_create"), model.EventTypeSnapshotCreate)
	assert.Equal(t, model.AuditEventType("snapshot_delete"), model.EventTypeSnapshotDelete)
	assert.Equal(t, model.AuditEventType("restore"), model.EventTypeRestore)
	assert.Equal(t, model.AuditEventType("worktree_create"), model.EventTypeWorktreeCreate)
	assert.Equal(t, model.AuditEventType("worktree_rename"), model.EventTypeWorktreeRename)
	assert.Equal(t, model.AuditEventType("worktree_remove"), model.EventTypeWorktreeRemove)
	assert.Equal(t, model.AuditEventType("gc_plan"), model.EventTypeGCPlan)
	assert.Equal(t, model.AuditEventType("gc_run"), model.EventTypeGCRun)
}

func TestAuditRecord_Fields(t *testing.T) {
	record := model.AuditRecord{
		Timestamp:    time.Date(2024, 2, 19, 0, 0, 0, 0, time.UTC),
		EventType:    model.EventTypeSnapshotCreate,
		SnapshotID:   "1708300800000-a3f7c1b2",
		WorktreeName: "main",
		Details:      map[string]any{"key": "value"},
		PrevHash:     "prev123",
		RecordHash:   "hash456",
	}

	assert.Equal(t, model.EventTypeSnapshotCreate, record.EventType)
	assert.Equal(t, model.SnapshotID("1708300800000-a3f7c1b2"), record.SnapshotID)
	assert.Equal(t, "main", record.WorktreeName)
}

func TestPin_Fields(t *testing.T) {
	expiresAt := time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)
	pin := model.Pin{
		SnapshotID: "1708300800000-a3f7c1b2",
		PinnedAt:   time.Date(2024, 2, 19, 0, 0, 0, 0, time.UTC),
		Reason:     "important release",
		ExpiresAt:  &expiresAt,
	}

	assert.Equal(t, model.SnapshotID("1708300800000-a3f7c1b2"), pin.SnapshotID)
	assert.Equal(t, "important release", pin.Reason)
	assert.NotNil(t, pin.ExpiresAt)
}

func TestPin_NoExpiry(t *testing.T) {
	pin := model.Pin{
		SnapshotID: "1708300800000-a3f7c1b2",
		PinnedAt:   time.Date(2024, 2, 19, 0, 0, 0, 0, time.UTC),
		Reason:     "permanent",
	}

	assert.Nil(t, pin.ExpiresAt)
}

func TestGCPlan_Fields(t *testing.T) {
	plan := model.GCPlan{
		PlanID:                 "plan-123",
		CreatedAt:              time.Date(2024, 2, 19, 0, 0, 0, 0, time.UTC),
		ProtectedSet:           []model.SnapshotID{"snap1", "snap2"},
		ProtectedByLineage:     5,
		CandidateCount:         10,
		ToDelete:               []model.SnapshotID{"snap3", "snap4"},
		DeletableBytesEstimate: 1024 * 1024,
		RetentionPolicy: model.RetentionPolicy{
			KeepMinSnapshots: 10,
			KeepMinAge:       24 * time.Hour,
		},
	}

	assert.Equal(t, "plan-123", plan.PlanID)
	assert.Equal(t, 2, len(plan.ProtectedSet))
	assert.Equal(t, 5, plan.ProtectedByLineage)
	assert.Equal(t, 10, plan.CandidateCount)
	assert.Equal(t, 2, len(plan.ToDelete))
}

func TestTombstone_Fields(t *testing.T) {
	ts := model.Tombstone{
		SnapshotID:  "1708300800000-a3f7c1b2",
		DeletedAt:   time.Date(2024, 2, 19, 0, 0, 0, 0, time.UTC),
		Reclaimable: true,
	}

	assert.Equal(t, model.SnapshotID("1708300800000-a3f7c1b2"), ts.SnapshotID)
	assert.True(t, ts.Reclaimable)
}

func TestRetentionPolicy_Fields(t *testing.T) {
	policy := model.RetentionPolicy{
		KeepMinSnapshots: 20,
		KeepMinAge:       48 * time.Hour,
	}

	assert.Equal(t, 20, policy.KeepMinSnapshots)
	assert.Equal(t, 48*time.Hour, policy.KeepMinAge)
}

func TestRetentionPolicy_Validate_Valid(t *testing.T) {
	tests := []struct {
		name   string
		policy model.RetentionPolicy
	}{
		{
			name: "zero values are valid",
			policy: model.RetentionPolicy{
				KeepMinSnapshots: 0,
				KeepMinAge:       0,
			},
		},
		{
			name: "positive values are valid",
			policy: model.RetentionPolicy{
				KeepMinSnapshots: 10,
				KeepMinAge:       24 * time.Hour,
			},
		},
		{
			name: "only keep_min_snapshots set",
			policy: model.RetentionPolicy{
				KeepMinSnapshots: 5,
				KeepMinAge:       0,
			},
		},
		{
			name: "only keep_min_age set",
			policy: model.RetentionPolicy{
				KeepMinSnapshots: 0,
				KeepMinAge:       12 * time.Hour,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.policy.Validate()
			assert.NoError(t, err)
		})
	}
}

func TestRetentionPolicy_Validate_Invalid(t *testing.T) {
	tests := []struct {
		name        string
		policy      model.RetentionPolicy
		expectedErr string
	}{
		{
			name: "negative keep_min_snapshots",
			policy: model.RetentionPolicy{
				KeepMinSnapshots: -1,
				KeepMinAge:       0,
			},
			expectedErr: "invalid retention policy: keep_min_snapshots must be non-negative",
		},
		{
			name: "negative keep_min_age",
			policy: model.RetentionPolicy{
				KeepMinSnapshots: 0,
				KeepMinAge:       -1 * time.Hour,
			},
			expectedErr: "invalid retention policy: keep_min_age must be non-negative",
		},
		{
			name: "both negative",
			policy: model.RetentionPolicy{
				KeepMinSnapshots: -5,
				KeepMinAge:       -24 * time.Hour,
			},
			expectedErr: "invalid retention policy: keep_min_snapshots must be non-negative",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.policy.Validate()
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectedErr)
		})
	}
}

func TestInvalidRetentionPolicyError_Error(t *testing.T) {
	err := &model.InvalidRetentionPolicyError{
		Field:  "keep_min_snapshots",
		Reason: "must be non-negative",
		Value:  -1,
	}
	expected := "invalid retention policy: keep_min_snapshots must be non-negative (got: -1)"
	assert.Equal(t, expected, err.Error())
}

func TestDefaultRetentionPolicy(t *testing.T) {
	policy := model.DefaultRetentionPolicy()
	assert.Equal(t, 0, policy.KeepMinSnapshots)
	assert.Equal(t, time.Duration(0), policy.KeepMinAge)
}

func TestGCPlan_ProtectedByRetention(t *testing.T) {
	plan := model.GCPlan{
		PlanID:               "plan-123",
		CreatedAt:            time.Date(2024, 2, 19, 0, 0, 0, 0, time.UTC),
		ProtectedByRetention: 3,
	}
	assert.Equal(t, 3, plan.ProtectedByRetention)
}

func TestCompressionInfo_Fields(t *testing.T) {
	comp := model.CompressionInfo{
		Type:  "gzip",
		Level: 6,
	}
	assert.Equal(t, "gzip", comp.Type)
	assert.Equal(t, 6, comp.Level)
}

func TestDescriptor_Compression(t *testing.T) {
	comp := &model.CompressionInfo{
		Type:  "gzip",
		Level: 9,
	}
	desc := model.Descriptor{
		SnapshotID:  "1708300800000-a3f7c1b2",
		Compression: comp,
	}
	assert.NotNil(t, desc.Compression)
	assert.Equal(t, "gzip", desc.Compression.Type)
	assert.Equal(t, 9, desc.Compression.Level)
}

func TestDescriptor_NoCompression(t *testing.T) {
	desc := model.Descriptor{
		SnapshotID: "1708300800000-a3f7c1b2",
	}
	assert.Nil(t, desc.Compression)
}

func TestDescriptor_PartialPaths(t *testing.T) {
	desc := model.Descriptor{
		SnapshotID:   "1708300800000-a3f7c1b2",
		PartialPaths: []string{"models/", "data/"},
	}
	assert.Equal(t, []string{"models/", "data/"}, desc.PartialPaths)
}

func TestDescriptor_FullSnapshot(t *testing.T) {
	desc := model.Descriptor{
		SnapshotID:   "1708300800000-a3f7c1b2",
		PartialPaths: nil,
	}
	assert.Nil(t, desc.PartialPaths)
}

func TestDescriptor_EmptyPartialPaths(t *testing.T) {
	desc := model.Descriptor{
		SnapshotID:   "1708300800000-a3f7c1b2",
		PartialPaths: []string{},
	}
	assert.Equal(t, []string{}, desc.PartialPaths)
}

func TestHashValue(t *testing.T) {
	hash := model.HashValue("abc123def456")
	assert.Equal(t, "abc123def456", string(hash))
}

func TestHashValue_Empty(t *testing.T) {
	hash := model.HashValue("")
	assert.Equal(t, "", string(hash))
}
