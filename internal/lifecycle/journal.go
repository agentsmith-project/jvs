// Package lifecycle contains durable primitives for repo and workspace
// lifecycle operations.
package lifecycle

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/agentsmith-project/jvs/pkg/fsutil"
	"github.com/agentsmith-project/jvs/pkg/pathutil"
)

const (
	// SchemaVersion is the lifecycle operation journal schema version.
	SchemaVersion = 1
	// PhaseConsumed marks an operation that should no longer appear as pending.
	PhaseConsumed = "consumed"
)

// OperationRecord is the durable lifecycle journal record stored as JSON under
// .jvs/lifecycle/operations/<operation-id>.json.
type OperationRecord struct {
	SchemaVersion          int            `json:"schema_version"`
	OperationID            string         `json:"operation_id"`
	OperationType          string         `json:"operation_type"`
	RepoID                 string         `json:"repo_id"`
	Phase                  string         `json:"phase"`
	RecommendedNextCommand string         `json:"recommended_next_command"`
	CreatedAt              time.Time      `json:"created_at"`
	UpdatedAt              time.Time      `json:"updated_at"`
	LastError              string         `json:"last_error,omitempty"`
	Metadata               map[string]any `json:"metadata,omitempty"`
}

// OperationsDir returns the lifecycle journal directory for repoRoot.
func OperationsDir(repoRoot string) string {
	return filepath.Join(repoRoot, ".jvs", "lifecycle", "operations")
}

// OperationPath returns the journal path for a lifecycle operation ID.
func OperationPath(repoRoot, operationID string) (string, error) {
	if err := validateOperationID(operationID); err != nil {
		return "", err
	}
	return filepath.Join(OperationsDir(repoRoot), operationID+".json"), nil
}

// WriteOperation validates and atomically writes a lifecycle journal record.
func WriteOperation(repoRoot string, record OperationRecord) error {
	record, err := normalizeOperationRecord(record)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(OperationsDir(repoRoot), 0755); err != nil {
		return fmt.Errorf("create lifecycle operations directory: %w", err)
	}
	path, err := OperationPath(repoRoot, record.OperationID)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal lifecycle operation journal: %w", err)
	}
	if err := fsutil.AtomicWrite(path, data, 0600); err != nil {
		return fmt.Errorf("write lifecycle operation journal: %w", err)
	}
	return nil
}

// ReadOperation reads one lifecycle journal record by operation ID.
func ReadOperation(repoRoot, operationID string) (OperationRecord, error) {
	path, err := OperationPath(repoRoot, operationID)
	if err != nil {
		return OperationRecord{}, err
	}
	return readOperationPath(path)
}

// ListPendingOperations returns all non-consumed lifecycle journal records in a
// stable creation-time order.
func ListPendingOperations(repoRoot string) ([]OperationRecord, error) {
	dir := OperationsDir(repoRoot)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read lifecycle operations directory: %w", err)
	}

	var pending []OperationRecord
	for _, entry := range entries {
		if entry.IsDir() {
			return nil, fmt.Errorf("unexpected lifecycle operation directory: %s", entry.Name())
		}
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		if err := validateOperationID(id); err != nil {
			return nil, fmt.Errorf("invalid lifecycle operation filename %s: %w", entry.Name(), err)
		}
		record, err := readOperationPath(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		if record.Phase == PhaseConsumed {
			continue
		}
		pending = append(pending, record)
	}
	sort.Slice(pending, func(i, j int) bool {
		left, right := pending[i], pending[j]
		if !left.CreatedAt.Equal(right.CreatedAt) {
			return left.CreatedAt.Before(right.CreatedAt)
		}
		return left.OperationID < right.OperationID
	})
	return pending, nil
}

// ConsumeOperation removes a completed lifecycle journal record from the
// pending list.
func ConsumeOperation(repoRoot, operationID string) error {
	path, err := OperationPath(repoRoot, operationID)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("consume lifecycle operation journal: %w", err)
	}
	if err := fsutil.FsyncDir(filepath.Dir(path)); err != nil {
		return fmt.Errorf("fsync lifecycle operations directory: %w", err)
	}
	return nil
}

func readOperationPath(path string) (OperationRecord, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return OperationRecord{}, fmt.Errorf("read lifecycle operation journal: %w", err)
	}
	var record OperationRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return OperationRecord{}, fmt.Errorf("parse lifecycle operation journal %s: %w", path, err)
	}
	record, err = normalizeOperationRecord(record)
	if err != nil {
		return OperationRecord{}, fmt.Errorf("invalid lifecycle operation journal %s: %w", path, err)
	}
	return record, nil
}

func normalizeOperationRecord(record OperationRecord) (OperationRecord, error) {
	if record.SchemaVersion == 0 {
		record.SchemaVersion = SchemaVersion
	}
	if record.SchemaVersion != SchemaVersion {
		return OperationRecord{}, fmt.Errorf("unsupported lifecycle journal schema_version %d", record.SchemaVersion)
	}
	if err := validateOperationID(record.OperationID); err != nil {
		return OperationRecord{}, err
	}
	if strings.TrimSpace(record.OperationType) == "" {
		return OperationRecord{}, fmt.Errorf("operation_type is required")
	}
	if strings.TrimSpace(record.RepoID) == "" {
		return OperationRecord{}, fmt.Errorf("repo_id is required")
	}
	if strings.TrimSpace(record.Phase) == "" {
		return OperationRecord{}, fmt.Errorf("phase is required")
	}
	if strings.TrimSpace(record.RecommendedNextCommand) == "" {
		return OperationRecord{}, fmt.Errorf("recommended_next_command is required")
	}
	now := time.Now().UTC()
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}
	if record.UpdatedAt.IsZero() {
		record.UpdatedAt = record.CreatedAt
	}
	return record, nil
}

func validateOperationID(operationID string) error {
	if err := pathutil.ValidateName(operationID); err != nil {
		return fmt.Errorf("invalid operation_id: %w", err)
	}
	return nil
}
