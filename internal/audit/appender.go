// Package audit provides audit logging with hash chain verification.
package audit

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/jvs-project/jvs/pkg/jsonutil"
	"github.com/jvs-project/jvs/pkg/model"
)

// FileAppender appends audit records to a JSONL file with hash chain.
type FileAppender struct {
	path string
	mu   sync.Mutex
}

const (
	ErrorCodeAuditRecordMalformed     = "E_AUDIT_RECORD_MALFORMED"
	ErrorCodeAuditChainBroken         = "E_AUDIT_CHAIN_BROKEN"
	ErrorCodeAuditRecordHashMismatch  = "E_AUDIT_RECORD_HASH_MISMATCH"
	ErrorCodeAuditRecordHashRecompute = "E_AUDIT_RECORD_HASH_RECOMPUTE_FAILED"
)

// Issue describes an audit log integrity issue.
type Issue struct {
	Line      int
	Severity  string
	ErrorCode string
	Message   string
}

// NewFileAppender creates a new FileAppender.
func NewFileAppender(path string) *FileAppender {
	return &FileAppender{path: path}
}

// Append adds a new audit record to the log.
func (a *FileAppender) Append(eventType model.AuditEventType, worktreeName string, snapshotID model.SnapshotID, details map[string]any) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(a.path), 0755); err != nil {
		return fmt.Errorf("create audit dir: %w", err)
	}

	file, err := os.OpenFile(a.path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("open audit log: %w", err)
	}
	defer file.Close()

	if err := lockFile(file); err != nil {
		return fmt.Errorf("flock audit log: %w", err)
	}
	defer unlockFile(file)

	// Get previous record hash
	prevHash, err := a.getLastRecordHashLocked(file)
	if err != nil {
		return fmt.Errorf("get last record hash: %w", err)
	}

	// Create record
	record := &model.AuditRecord{
		Timestamp:    time.Now().UTC(),
		EventType:    eventType,
		SnapshotID:   snapshotID,
		WorktreeName: worktreeName,
		Details:      details,
		PrevHash:     prevHash,
	}

	// Compute record hash (before setting RecordHash field)
	recordHash, err := computeRecordHash(record)
	if err != nil {
		return fmt.Errorf("compute record hash: %w", err)
	}
	record.RecordHash = recordHash

	// Serialize to JSONL
	line, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal audit record: %w", err)
	}

	// Seek to end and append
	if _, err := file.Seek(0, 2); err != nil {
		return fmt.Errorf("seek to end: %w", err)
	}
	if _, err := file.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("write audit record: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync audit log: %w", err)
	}

	return nil
}

// GetLastRecordHash returns the hash of the last record in the log.
func (a *FileAppender) GetLastRecordHash() (model.HashValue, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	file, err := os.Open(a.path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("open audit log: %w", err)
	}
	defer file.Close()

	return a.getLastRecordHashLocked(file)
}

// VerifyFile verifies audit record hashes and hash-chain linkage.
func VerifyFile(path string) ([]Issue, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open audit log: %w", err)
	}
	defer file.Close()

	var issues []Issue
	var expectedPrevHash model.HashValue
	lineNum := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lineNum++
		var record model.AuditRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			issues = append(issues, Issue{
				Line:      lineNum,
				Severity:  "warning",
				ErrorCode: ErrorCodeAuditRecordMalformed,
				Message:   fmt.Sprintf("malformed record at line %d", lineNum),
			})
			continue
		}

		if record.PrevHash != expectedPrevHash {
			issues = append(issues, Issue{
				Line:      lineNum,
				Severity:  "critical",
				ErrorCode: ErrorCodeAuditChainBroken,
				Message:   fmt.Sprintf("audit hash chain broken at line %d", lineNum),
			})
		}

		computedHash, err := computeRecordHash(&record)
		if err != nil {
			issues = append(issues, Issue{
				Line:      lineNum,
				Severity:  "warning",
				ErrorCode: ErrorCodeAuditRecordHashRecompute,
				Message:   fmt.Sprintf("cannot recompute audit record hash at line %d: %v", lineNum, err),
			})
		} else if computedHash != record.RecordHash {
			issues = append(issues, Issue{
				Line:      lineNum,
				Severity:  "critical",
				ErrorCode: ErrorCodeAuditRecordHashMismatch,
				Message:   fmt.Sprintf("audit record hash mismatch at line %d", lineNum),
			})
		}

		expectedPrevHash = record.RecordHash
	}
	if err := scanner.Err(); err != nil {
		return issues, fmt.Errorf("scan audit log: %w", err)
	}
	return issues, nil
}

func (a *FileAppender) getLastRecordHashLocked(file *os.File) (model.HashValue, error) {
	// Read from beginning to find last record
	if _, err := file.Seek(0, 0); err != nil {
		return "", fmt.Errorf("seek to start: %w", err)
	}

	var lastHash model.HashValue
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var record model.AuditRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			continue // skip malformed lines
		}
		lastHash = record.RecordHash
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("scan audit log: %w", err)
	}

	return lastHash, nil
}

func computeRecordHash(record *model.AuditRecord) (model.HashValue, error) {
	// Create a copy without RecordHash for hash computation
	hashRecord := &model.AuditRecord{
		Timestamp:    record.Timestamp,
		EventType:    record.EventType,
		SnapshotID:   record.SnapshotID,
		WorktreeName: record.WorktreeName,
		Details:      record.Details,
		PrevHash:     record.PrevHash,
		// RecordHash intentionally omitted
	}

	data, err := jsonutil.CanonicalMarshal(hashRecord)
	if err != nil {
		return "", fmt.Errorf("canonical marshal: %w", err)
	}

	hash := sha256.Sum256(data)
	return model.HashValue(hex.EncodeToString(hash[:])), nil
}
