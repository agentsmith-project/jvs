package audit_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/jvs-project/jvs/internal/audit"
	"github.com/jvs-project/jvs/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileAppender_AppendCreatesJSONL(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.jsonl")

	appender := audit.NewFileAppender(logPath)
	err := appender.Append(model.EventTypeSnapshotCreate, "main", "1708300800000-a3f7c1b2", nil)
	require.NoError(t, err)

	// Verify file exists and has content
	file, err := os.Open(logPath)
	require.NoError(t, err)
	defer file.Close()

	scanner := bufio.NewScanner(file)
	require.True(t, scanner.Scan())
	line := scanner.Text()

	var record model.AuditRecord
	require.NoError(t, json.Unmarshal([]byte(line), &record))
	assert.Equal(t, model.EventTypeSnapshotCreate, record.EventType)
	assert.Equal(t, "main", record.WorktreeName)
}

func TestFileAppender_HashChain(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.jsonl")

	appender := audit.NewFileAppender(logPath)

	// First record
	err := appender.Append(model.EventTypeSnapshotCreate, "main", "id1", nil)
	require.NoError(t, err)

	// Second record
	err = appender.Append(model.EventTypeWorktreeCreate, "feature", "", map[string]any{"base": "main"})
	require.NoError(t, err)

	// Read both records
	file, err := os.Open(logPath)
	require.NoError(t, err)
	defer file.Close()

	var records []model.AuditRecord
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var r model.AuditRecord
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &r))
		records = append(records, r)
	}

	require.Len(t, records, 2)

	// First record has empty prev_hash
	assert.Equal(t, model.HashValue(""), records[0].PrevHash)

	// Second record's prev_hash equals first record's record_hash
	assert.Equal(t, records[0].RecordHash, records[1].PrevHash)

	// Both records have non-empty record_hash
	assert.NotEmpty(t, records[0].RecordHash)
	assert.NotEmpty(t, records[1].RecordHash)
}

func TestVerifyFileDetectsRecordHashTamper(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.jsonl")

	appender := audit.NewFileAppender(logPath)
	require.NoError(t, appender.Append(model.EventTypeSnapshotCreate, "main", "1708300800000-a3f7c1b2", map[string]any{
		"note": "before",
	}))

	data, err := os.ReadFile(logPath)
	require.NoError(t, err)
	lines := bytes.Split(bytes.TrimSpace(data), []byte("\n"))
	require.Len(t, lines, 1)

	var record map[string]any
	require.NoError(t, json.Unmarshal(lines[0], &record))
	details, ok := record["details"].(map[string]any)
	require.True(t, ok)
	details["note"] = "after"
	lines[0], err = json.Marshal(record)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(logPath, append(lines[0], '\n'), 0644))

	issues, err := audit.VerifyFile(logPath)
	require.NoError(t, err)
	require.Len(t, issues, 1)
	assert.Equal(t, "E_AUDIT_RECORD_HASH_MISMATCH", issues[0].ErrorCode)
	assert.Equal(t, 1, issues[0].Line)
}

func TestFileAppender_EnsureAppendableRejectsWellFormedTamper(t *testing.T) {
	for _, tt := range []struct {
		name     string
		tamper   func(t *testing.T, records []model.AuditRecord) []model.AuditRecord
		wantCode string
		wantLine int
	}{
		{
			name: "chain break",
			tamper: func(t *testing.T, records []model.AuditRecord) []model.AuditRecord {
				t.Helper()
				require.Len(t, records, 2)
				return []model.AuditRecord{records[1], records[0]}
			},
			wantCode: audit.ErrorCodeAuditChainBroken,
			wantLine: 1,
		},
		{
			name: "record hash mismatch",
			tamper: func(t *testing.T, records []model.AuditRecord) []model.AuditRecord {
				t.Helper()
				require.NotEmpty(t, records)
				records[0].Details["note"] = "tampered"
				return records
			},
			wantCode: audit.ErrorCodeAuditRecordHashMismatch,
			wantLine: 1,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			logPath := filepath.Join(dir, "audit.jsonl")
			appender := audit.NewFileAppender(logPath)

			require.NoError(t, appender.Append(model.EventTypeSnapshotCreate, "main", "1708300800000-a3f7c1b2", map[string]any{
				"note": "original",
			}))
			require.NoError(t, appender.Append(model.EventTypeWorktreeCreate, "feature", "", map[string]any{
				"base": "main",
			}))

			records := readAuditRecordsForTest(t, logPath)
			writeAuditRecordsForTest(t, logPath, tt.tamper(t, records))

			err := appender.EnsureAppendable()
			require.Error(t, err)

			var integrityErr *audit.IntegrityError
			require.True(t, errors.As(err, &integrityErr), "expected audit integrity error, got %T: %v", err, err)
			assert.Equal(t, tt.wantCode, integrityErr.Code)
			assert.Equal(t, tt.wantLine, integrityErr.Line)
		})
	}
}

func TestFileAppender_ConcurrentAppends(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.jsonl")

	appender := audit.NewFileAppender(logPath)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			appender.Append(model.EventTypeSnapshotCreate, "main", "id", map[string]any{"idx": idx})
		}(i)
	}
	wg.Wait()

	// Verify all records are present
	file, err := os.Open(logPath)
	require.NoError(t, err)
	defer file.Close()

	count := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		count++
	}
	assert.Equal(t, 10, count)
}

func TestFileAppender_GetLastRecordHash(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.jsonl")

	appender := audit.NewFileAppender(logPath)

	// Empty file returns empty hash
	hash, err := appender.GetLastRecordHash()
	require.NoError(t, err)
	assert.Equal(t, model.HashValue(""), hash)

	// After append, returns the record hash
	err = appender.Append(model.EventTypeSnapshotCreate, "main", "id1", nil)
	require.NoError(t, err)

	hash, err = appender.GetLastRecordHash()
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
}

func TestFileAppender_AppendWithDetails(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.jsonl")

	appender := audit.NewFileAppender(logPath)

	details := map[string]any{
		"files_added":   5,
		"files_removed": 2,
		"note":          "test snapshot",
	}

	err := appender.Append(model.EventTypeSnapshotCreate, "main", "snap123", details)
	require.NoError(t, err)

	// Read and verify
	file, err := os.Open(logPath)
	require.NoError(t, err)
	defer file.Close()

	var record model.AuditRecord
	scanner := bufio.NewScanner(file)
	require.True(t, scanner.Scan())
	require.NoError(t, json.Unmarshal(scanner.Bytes(), &record))

	assert.Equal(t, "main", record.WorktreeName)
	assert.Equal(t, model.SnapshotID("snap123"), record.SnapshotID)
	assert.NotNil(t, record.Details)
	assert.Equal(t, float64(5), record.Details["files_added"])
}

func TestFileAppender_HashChainConsistent(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.jsonl")

	appender := audit.NewFileAppender(logPath)

	// Append multiple records
	ids := []model.SnapshotID{model.SnapshotID("id1"), model.SnapshotID("id2"), model.SnapshotID("id3")}
	for _, id := range ids {
		err := appender.Append(model.EventTypeSnapshotCreate, "main", id, nil)
		require.NoError(t, err)
	}

	// Read all records
	file, err := os.Open(logPath)
	require.NoError(t, err)
	defer file.Close()

	var records []model.AuditRecord
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var r model.AuditRecord
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &r))
		records = append(records, r)
	}

	require.Len(t, records, 3)

	// Verify hash chain
	assert.Equal(t, model.HashValue(""), records[0].PrevHash)
	assert.Equal(t, records[0].RecordHash, records[1].PrevHash)
	assert.Equal(t, records[1].RecordHash, records[2].PrevHash)
}

func TestFileAppender_GetLastRecordHashFailsClosedOnMalformedLine(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.jsonl")

	appender := audit.NewFileAppender(logPath)
	require.NoError(t, appender.Append(model.EventTypeSnapshotCreate, "main", "snap1", nil))

	file, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0644)
	require.NoError(t, err)
	_, err = file.Write([]byte("not valid json\n"))
	require.NoError(t, err)
	require.NoError(t, file.Close())

	// GetLastRecordHash must fail closed so future appends cannot silently
	// continue an audit chain with an unknowable segment.
	hash, err := appender.GetLastRecordHash()
	require.Error(t, err)
	assert.Equal(t, model.HashValue(""), hash)

	var integrityErr *audit.IntegrityError
	require.True(t, errors.As(err, &integrityErr), "expected audit integrity error, got %T: %v", err, err)
	assert.Equal(t, audit.ErrorCodeAuditRecordMalformed, integrityErr.Code)
	assert.Equal(t, 2, integrityErr.Line)
}

func TestFileAppender_AppendFailsClosedOnMalformedExistingLog(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.jsonl")
	require.NoError(t, os.WriteFile(logPath, []byte("{malformed audit record}\n"), 0644))

	appender := audit.NewFileAppender(logPath)
	err := appender.Append(model.EventTypeSnapshotCreate, "main", "1708300800000-a3f7c1b2", nil)
	require.Error(t, err)

	var integrityErr *audit.IntegrityError
	require.True(t, errors.As(err, &integrityErr), "expected audit integrity error, got %T: %v", err, err)
	assert.Equal(t, audit.ErrorCodeAuditRecordMalformed, integrityErr.Code)
	assert.Equal(t, 1, integrityErr.Line)

	data, readErr := os.ReadFile(logPath)
	require.NoError(t, readErr)
	assert.Equal(t, "{malformed audit record}\n", string(data), "append must not mutate a malformed audit log")
}

func TestFileAppender_ConcurrentWithHashChain(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.jsonl")

	appender := audit.NewFileAppender(logPath)

	var wg sync.WaitGroup
	numGoroutines := 20
	recordsPerGoroutine := 5

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < recordsPerGoroutine; j++ {
				snapID := model.SnapshotID(fmt.Sprintf("snap-%d-%d", idx, j))
				appender.Append(model.EventTypeSnapshotCreate, "main", snapID, nil)
			}
		}(i)
	}
	wg.Wait()

	// Verify all records were written and hash chain is intact
	file, err := os.Open(logPath)
	require.NoError(t, err)
	defer file.Close()

	var records []model.AuditRecord
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var r model.AuditRecord
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &r))
		records = append(records, r)
	}

	expectedCount := numGoroutines * recordsPerGoroutine
	assert.Equal(t, expectedCount, len(records))

	// Verify hash chain integrity
	for i := 1; i < len(records); i++ {
		assert.Equal(t, records[i-1].RecordHash, records[i].PrevHash,
			"Hash chain broken at record %d", i)
	}
}

func TestFileAppender_DirectoryCreation(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "subdir", "nested", "audit.jsonl")

	// Directory doesn't exist yet
	_, err := os.Stat(filepath.Dir(logPath))
	assert.True(t, os.IsNotExist(err))

	appender := audit.NewFileAppender(logPath)
	err = appender.Append(model.EventTypeSnapshotCreate, "main", "id1", nil)
	require.NoError(t, err)

	// Directory should now exist
	info, err := os.Stat(filepath.Dir(logPath))
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestFileAppender_EmptyDetails(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.jsonl")

	appender := audit.NewFileAppender(logPath)

	// Append with empty details map
	err := appender.Append(model.EventTypeSnapshotCreate, "main", "id1", map[string]any{})
	require.NoError(t, err)

	// Read and verify
	file, err := os.Open(logPath)
	require.NoError(t, err)
	defer file.Close()

	var record model.AuditRecord
	scanner := bufio.NewScanner(file)
	require.True(t, scanner.Scan())
	require.NoError(t, json.Unmarshal(scanner.Bytes(), &record))

	// Empty map gets marshaled as {} and unmarshaled as nil
	assert.Equal(t, model.EventTypeSnapshotCreate, record.EventType)
	assert.Equal(t, "main", record.WorktreeName)
}

func TestFileAppender_NilDetails(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.jsonl")

	appender := audit.NewFileAppender(logPath)

	// Append with nil details
	err := appender.Append(model.EventTypeSnapshotCreate, "main", "id1", nil)
	require.NoError(t, err)

	// Read and verify
	file, err := os.Open(logPath)
	require.NoError(t, err)
	defer file.Close()

	var record model.AuditRecord
	scanner := bufio.NewScanner(file)
	require.True(t, scanner.Scan())
	require.NoError(t, json.Unmarshal(scanner.Bytes(), &record))

	// Nil details should still produce a valid record
	assert.Equal(t, model.EventTypeSnapshotCreate, record.EventType)
	assert.Equal(t, "main", record.WorktreeName)
}

func TestFileAppender_EmptySnapshotID(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.jsonl")

	appender := audit.NewFileAppender(logPath)

	// Some events don't have a snapshot ID (e.g., worktree operations)
	err := appender.Append(model.EventTypeWorktreeCreate, "feature", "", nil)
	require.NoError(t, err)

	// Read and verify
	file, err := os.Open(logPath)
	require.NoError(t, err)
	defer file.Close()

	var record model.AuditRecord
	scanner := bufio.NewScanner(file)
	require.True(t, scanner.Scan())
	require.NoError(t, json.Unmarshal(scanner.Bytes(), &record))

	assert.Equal(t, model.EventTypeWorktreeCreate, record.EventType)
	assert.Equal(t, model.SnapshotID(""), record.SnapshotID)
	assert.Equal(t, "feature", record.WorktreeName)
}

func TestFileAppender_AllEventTypes(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.jsonl")

	appender := audit.NewFileAppender(logPath)

	eventTypes := []model.AuditEventType{
		model.EventTypeSnapshotCreate,
		model.EventTypeRestore,
		model.EventTypeWorktreeCreate,
		model.EventTypeWorktreeRemove,
		model.EventTypeGCRun,
	}

	for _, eventType := range eventTypes {
		err := appender.Append(eventType, "main", "snap123", nil)
		require.NoError(t, err)
	}

	// Verify all events were logged
	file, err := os.Open(logPath)
	require.NoError(t, err)
	defer file.Close()

	var records []model.AuditRecord
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var r model.AuditRecord
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &r))
		records = append(records, r)
	}

	assert.Equal(t, len(eventTypes), len(records))

	// Verify each event type in order
	for i, expectedType := range eventTypes {
		assert.Equal(t, expectedType, records[i].EventType)
	}
}

func TestFileAppender_LargeDetailsMap(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.jsonl")

	appender := audit.NewFileAppender(logPath)

	// Create a large details map
	details := make(map[string]any)
	for i := 0; i < 100; i++ {
		details[fmt.Sprintf("key%d", i)] = fmt.Sprintf("value%d with some longer text", i)
	}

	err := appender.Append(model.EventTypeSnapshotCreate, "main", "snap123", details)
	require.NoError(t, err)

	// Read and verify
	file, err := os.Open(logPath)
	require.NoError(t, err)
	defer file.Close()

	var record model.AuditRecord
	scanner := bufio.NewScanner(file)
	require.True(t, scanner.Scan())
	require.NoError(t, json.Unmarshal(scanner.Bytes(), &record))

	assert.Equal(t, 100, len(record.Details))
	assert.Equal(t, "value99 with some longer text", record.Details["key99"])
}

func TestFileAppender_SpecialCharactersInDetails(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.jsonl")

	appender := audit.NewFileAppender(logPath)

	details := map[string]any{
		"note":     "Test with quotes: \"hello\" and 'world'",
		"path":     "/path/to/file with spaces.txt",
		"unicode":  "Hello 世界 🌍",
		"newlines": "line1\nline2\nline3",
		"special":  "!@#$%^&*()_+-=[]{}|;':\",./<>?",
	}

	err := appender.Append(model.EventTypeSnapshotCreate, "main", "snap123", details)
	require.NoError(t, err)

	// Read and verify
	file, err := os.Open(logPath)
	require.NoError(t, err)
	defer file.Close()

	var record model.AuditRecord
	scanner := bufio.NewScanner(file)
	require.True(t, scanner.Scan())
	require.NoError(t, json.Unmarshal(scanner.Bytes(), &record))

	assert.Equal(t, "Test with quotes: \"hello\" and 'world'", record.Details["note"])
	assert.Equal(t, "Hello 世界 🌍", record.Details["unicode"])
	assert.Equal(t, "line1\nline2\nline3", record.Details["newlines"])
}

func TestFileAppender_NestedDetails(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.jsonl")

	appender := audit.NewFileAppender(logPath)

	details := map[string]any{
		"files": []map[string]any{
			{"path": "file1.txt", "size": 1024},
			{"path": "file2.txt", "size": 2048},
		},
		"metadata": map[string]any{
			"author":  "test",
			"version": 1.0,
			"tags":    []string{"tag1", "tag2"},
		},
		"count": 42,
	}

	err := appender.Append(model.EventTypeSnapshotCreate, "main", "snap123", details)
	require.NoError(t, err)

	// Read and verify
	file, err := os.Open(logPath)
	require.NoError(t, err)
	defer file.Close()

	var record model.AuditRecord
	scanner := bufio.NewScanner(file)
	require.True(t, scanner.Scan())
	require.NoError(t, json.Unmarshal(scanner.Bytes(), &record))

	assert.NotNil(t, record.Details["files"])
	assert.NotNil(t, record.Details["metadata"])
	assert.Equal(t, float64(42), record.Details["count"])
}

func TestFileAppender_NumericDetails(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.jsonl")

	appender := audit.NewFileAppender(logPath)

	details := map[string]any{
		"int_val":    42,
		"float_val":  3.14159,
		"neg_int":    -100,
		"zero":       0,
		"large":      9007199254740991, // Max safe integer
		"bool_true":  true,
		"bool_false": false,
	}

	err := appender.Append(model.EventTypeSnapshotCreate, "main", "snap123", details)
	require.NoError(t, err)

	// Read and verify
	file, err := os.Open(logPath)
	require.NoError(t, err)
	defer file.Close()

	var record model.AuditRecord
	scanner := bufio.NewScanner(file)
	require.True(t, scanner.Scan())
	require.NoError(t, json.Unmarshal(scanner.Bytes(), &record))

	assert.Equal(t, float64(42), record.Details["int_val"])
	assert.Equal(t, 3.14159, record.Details["float_val"])
	assert.Equal(t, float64(-100), record.Details["neg_int"])
	assert.Equal(t, true, record.Details["bool_true"])
	assert.Equal(t, false, record.Details["bool_false"])
}

func TestFileAppender_ConcurrentAppend(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.jsonl")

	appender := audit.NewFileAppender(logPath)

	eventTypes := []model.AuditEventType{
		model.EventTypeSnapshotCreate,
		model.EventTypeSnapshotDelete,
		model.EventTypeRestore,
		model.EventTypeWorktreeCreate,
		model.EventTypeWorktreeRename,
		model.EventTypeWorktreeRemove,
		model.EventTypeGCPlan,
		model.EventTypeGCRun,
		model.EventTypeSnapshotCreate,
		model.EventTypeRestore,
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			err := appender.Append(eventTypes[idx], "main", model.SnapshotID(fmt.Sprintf("snap-%d", idx)), nil)
			assert.NoError(t, err)
		}(i)
	}
	wg.Wait()

	data, err := os.ReadFile(logPath)
	require.NoError(t, err)

	scanner := bufio.NewScanner(bytes.NewReader(data))
	count := 0
	for scanner.Scan() {
		count++
	}
	assert.Equal(t, 10, count)
}

func TestFileAppender_AppendToNonExistentDir(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "a", "b", "c", "d", "audit.jsonl")

	appender := audit.NewFileAppender(logPath)
	err := appender.Append(model.EventTypeSnapshotCreate, "main", "snap-1", nil)
	require.NoError(t, err)

	_, err = os.Stat(logPath)
	require.NoError(t, err, "audit file should have been created along with parent directories")
}

func TestFileAppender_GetLastRecordHash_MultipleRecords(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.jsonl")

	appender := audit.NewFileAppender(logPath)

	// Append multiple records
	for i := 0; i < 5; i++ {
		err := appender.Append(model.EventTypeSnapshotCreate, "main", model.SnapshotID(fmt.Sprintf("snap%d", i)), nil)
		require.NoError(t, err)
	}

	// GetLastRecordHash should return the hash of the last record
	hash, err := appender.GetLastRecordHash()
	require.NoError(t, err)
	assert.NotEmpty(t, hash)

	// Read the file and verify the hash matches the last record
	file, err := os.Open(logPath)
	require.NoError(t, err)
	defer file.Close()

	var records []model.AuditRecord
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var r model.AuditRecord
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &r))
		records = append(records, r)
	}

	require.Len(t, records, 5)
	assert.Equal(t, records[4].RecordHash, hash)
}

func TestFileAppender_RapidSequentialAppends(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.jsonl")

	appender := audit.NewFileAppender(logPath)

	// Rapidly append records without goroutines
	for i := 0; i < 100; i++ {
		err := appender.Append(model.EventTypeSnapshotCreate, "main", model.SnapshotID(fmt.Sprintf("snap%d", i)), nil)
		require.NoError(t, err)
	}

	// Verify all records were written
	file, err := os.Open(logPath)
	require.NoError(t, err)
	defer file.Close()

	count := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		count++
	}
	assert.Equal(t, 100, count)
}

func readAuditRecordsForTest(t *testing.T, path string) []model.AuditRecord {
	t.Helper()

	file, err := os.Open(path)
	require.NoError(t, err)
	defer file.Close()

	var records []model.AuditRecord
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var record model.AuditRecord
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &record))
		records = append(records, record)
	}
	require.NoError(t, scanner.Err())
	return records
}

func writeAuditRecordsForTest(t *testing.T, path string, records []model.AuditRecord) {
	t.Helper()

	var data bytes.Buffer
	for _, record := range records {
		line, err := json.Marshal(record)
		require.NoError(t, err)
		data.Write(line)
		data.WriteByte('\n')
	}
	require.NoError(t, os.WriteFile(path, data.Bytes(), 0644))
}
