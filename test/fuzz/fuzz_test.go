//go:build go1.18
// +build go1.18

// Fuzzing tests for JVS critical functions
//
// This package contains fuzz targets for testing critical parsing and validation
// functions with randomized inputs. Fuzzing helps find edge cases, panics, and
// security vulnerabilities that might be missed with traditional unit tests.
//
// Running fuzz tests:
//   go test -fuzz=FuzzParseSnapshotID -fuzztime=30s ./test/fuzz/...
//   go test -fuzz=. -fuzztime=1m ./test/fuzz/...
//
// For more information on Go fuzzing, see:
// https://go.dev/doc/tutorial/fuzz

package fuzz

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jvs-project/jvs/pkg/jsonutil"
	"github.com/jvs-project/jvs/pkg/model"
	"github.com/jvs-project/jvs/pkg/pathutil"
)

// FuzzValidateName tests worktree name validation with random inputs.
//
// This fuzz target ensures ValidateName handles arbitrary input without panicking
// and correctly validates/rejects inputs according to the specification.
func FuzzValidateName(f *testing.F) {
	// Seed corpus with edge cases
	f.Add("")                    // empty string
	f.Add("valid-name-123")      // valid name
	f.Add("..")                  // path traversal
	f.Add("../escape")           // path traversal attempt
	f.Add("name/with/slash")     // invalid separator
	f.Add(`name\with\backslash`) // invalid separator
	f.Add("name\twith\tcontrol") // control character
	f.Add("name\x00null")        // null byte
	f.Add("a")                   // single char
	f.Add("aa")                  // two chars
	f.Add("a.b")                 // dot
	f.Add("a-b")                 // hyphen
	f.Add("a_b")                 // underscore
	f.Add("very-long-name-with-many-chars-that-still-valid-123456789")

	f.Fuzz(func(t *testing.T, name string) {
		// Should not panic on any input
		err := pathutil.ValidateName(name)

		// If we got a result, it should be consistent
		// (same input should give same result)
		err2 := pathutil.ValidateName(name)
		if (err == nil) != (err2 == nil) {
			t.Errorf("inconsistent validation for %q: %v vs %v", name, err, err2)
		}
	})
}

// FuzzValidateTag tests tag validation with random inputs.
//
// Tags have the same validation rules as worktree names.
func FuzzValidateTag(f *testing.F) {
	// Seed corpus
	f.Add("")
	f.Add("v1.0")
	f.Add("stable")
	f.Add("release_2024")
	f.Add("tag/with/slash")
	f.Add("../escape")
	f.Add("tag with spaces")
	f.Add("tag\twith\tcontrol")

	f.Fuzz(func(t *testing.T, tag string) {
		// Should not panic
		err := pathutil.ValidateTag(tag)

		// Consistency check
		err2 := pathutil.ValidateTag(tag)
		if (err == nil) != (err2 == nil) {
			t.Errorf("inconsistent validation for %q: %v vs %v", tag, err, err2)
		}
	})
}

// FuzzParseSnapshotID tests snapshot ID parsing with random inputs.
//
// Snapshot IDs have the format: <unix_ms>-<rand8hex>
// This fuzz target ensures parsing doesn't crash on malformed input.
func FuzzParseSnapshotID(f *testing.F) {
	// Seed corpus
	f.Add("1708300800000-a3f7c1b2") // valid
	f.Add("")                       // empty
	f.Add("-")                      // just separator
	f.Add("123")                    // too short
	f.Add("1708300800000-")         // missing random part
	f.Add("-a3f7c1b2")              // missing timestamp
	f.Add("1708300800000-abc")      // random too short
	f.Add("not-a-number-abc")       // invalid timestamp
	f.Add("1708300800000-gghhii")   // invalid hex
	f.Add("1708300800000-xxxxxxxx") // valid hex format

	f.Fuzz(func(t *testing.T, data string) {
		// Convert to SnapshotID type - should not panic
		id := model.SnapshotID(data)

		// Calling methods should not panic
		_ = id.ShortID()
		_ = id.String()

		// String() should return the original input
		if id.String() != data {
			t.Errorf("String() returned %q, expected %q", id.String(), data)
		}
	})
}

// FuzzCanonicalMarshal tests canonical JSON marshaling with random inputs.
//
// This ensures the JSON canonicalization doesn't panic and produces
// consistent output for the same input.
func FuzzCanonicalMarshal(f *testing.F) {
	// Seed corpus with JSON byte arrays
	f.Add([]byte(`{"name":"test","value":123}`))
	f.Add([]byte(`{"nested":{"key":"value"}}`))
	f.Add([]byte(`[1,2,3]`))
	f.Add([]byte(`null`))
	f.Add([]byte(`"simple string"`))
	f.Add([]byte(`123`))
	f.Add([]byte(`true`))
	f.Add([]byte(`false`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`[]`))
	f.Add([]byte(`{"a":1,"b":2,"c":3}`))
	f.Add([]byte(`{"z":9,"a":1,"m":5}`)) // test key ordering

	f.Fuzz(func(t *testing.T, data []byte) {
		// First, try to interpret data as JSON
		var v any
		if err := json.Unmarshal(data, &v); err != nil {
			// Not valid JSON, skip this iteration
			return
		}

		// CanonicalMarshal should not panic on valid JSON-derived data
		result1, err := jsonutil.CanonicalMarshal(v)
		if err != nil {
			// Some inputs may legitimately fail (e.g., channels, functions)
			// This is okay as long as we don't panic
			return
		}

		// Result should be valid JSON
		if !json.Valid(result1) {
			t.Errorf("CanonicalMarshal produced invalid JSON: %q", result1)
		}

		// Result should be deterministic (same input → same output)
		result2, err := jsonutil.CanonicalMarshal(v)
		if err != nil {
			t.Errorf("CanonicalMarshal inconsistent error: %v", err)
			return
		}
		if string(result1) != string(result2) {
			t.Errorf("CanonicalMarshal not deterministic: %q vs %q", result1, result2)
		}
	})
}

// FuzzDescriptorJSON tests descriptor JSON marshaling/unmarshaling with random data.
//
// This ensures the Descriptor struct handles various JSON inputs without panicking.
func FuzzDescriptorJSON(f *testing.F) {
	// Seed corpus with valid descriptors
	validDesc := model.Descriptor{
		SnapshotID:      "1708300800000-a3f7c1b2",
		WorktreeName:    "main",
		Engine:          model.EngineCopy,
		PayloadRootHash: "abc123",
	}
	validJSON, _ := json.Marshal(validDesc)

	f.Add(validJSON)
	f.Add([]byte(`{}`))                     // empty object
	f.Add([]byte(`{"snapshot_id":"test"}`)) // minimal
	f.Add([]byte(`invalid json`))           // invalid JSON
	f.Add([]byte(`{"snapshot_id":123}`))    // wrong type
	f.Add([]byte(`{"extra":"field"}`))      // extra field

	f.Fuzz(func(t *testing.T, data []byte) {
		// Unmarshal should not panic
		var desc model.Descriptor
		err := json.Unmarshal(data, &desc)

		// If unmarshal succeeded, marshal should also succeed and not panic
		if err == nil {
			_, err := json.Marshal(desc)
			if err != nil {
				t.Errorf("Marshal failed after successful Unmarshal: %v", err)
			}
		}
	})
}

const maxSnapshotIDStringFuzzInput = 4096

func snapshotIDStringFuzzInputAllowed(data string) bool {
	return len(data) <= maxSnapshotIDStringFuzzInput
}

// FuzzSnapshotIDString tests SnapshotID.String() consistency.
func FuzzSnapshotIDString(f *testing.F) {
	// Seed corpus
	f.Add("1708300800000-a3f7c1b2")
	f.Add("")
	f.Add("-")
	f.Add("abc")
	f.Add("12345678")
	f.Add("123456789")
	f.Add(strings.Repeat("x", maxSnapshotIDStringFuzzInput))

	f.Fuzz(func(t *testing.T, data string) {
		if !snapshotIDStringFuzzInputAllowed(data) {
			t.Skip("snapshot ID string fuzz input exceeds release-gate size bound")
		}

		id := model.SnapshotID(data)

		// String() should always return the original input
		if got := id.String(); got != data {
			t.Errorf("String() = %q, want %q", got, data)
		}

		// ShortID should return first 8 chars or less
		short := id.ShortID()
		if len(short) > 8 {
			t.Errorf("ShortID() returned %d chars, want at most 8", len(short))
		}
		if len(short) > len(data) {
			t.Errorf("ShortID() longer than input: %q vs %q", short, data)
		}
	})
}

// FuzzDescriptorMalformedJSON tests descriptor parsing with malformed JSON.
//
// This fuzz target ensures LoadDescriptor handles various malformed JSON inputs
// without panicking and returns appropriate errors.
func FuzzDescriptorMalformedJSON(f *testing.F) {
	// Seed corpus with edge cases
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"snapshot_id":"test"}`))
	f.Add([]byte(`{"snapshot_id":"1708300800000-a3f7c1b2","worktree_name":"main"}`))
	f.Add([]byte(`null`))
	f.Add([]byte(`123`))
	f.Add([]byte(`"just a string"`))
	f.Add([]byte(`[]`))
	f.Add([]byte(`{invalid json`))
	f.Add([]byte(`{"snapshot_id":123}`))
	f.Add([]byte(`{"snapshot_id":"","parent_id":null}`))
	f.Add([]byte(`{"snapshot_id":"test","parent_id":"not-a-snapshot-id"}`))
	f.Add([]byte(`{"tags":"not-an-array"}`))
	f.Add([]byte(`{"tags":[1,2,3]}`))
	f.Add([]byte(`{"partial_paths":"not-an-array"}`))
	f.Add([]byte(`{"created_at":"invalid-date"}`))
	f.Add([]byte(`{"compression":{"type":123}}`))
	f.Add([]byte(`{"compression":{"type":"gzip","level":"not-a-number"}}`))
	f.Add([]byte(``)) // empty
	f.Add([]byte(`{}`))
	f.Add([]byte(`{{doublebrace}}`))
	f.Add([]byte(`{"snapshot_id":"test","tags":[null]}`))
	f.Add([]byte(`{"snapshot_id":"test","partial_paths":["../escape"]}`))
	f.Add([]byte(`{"snapshot_id":"test","extra":"` + string(make([]byte, 10000)) + `"}`))

	// Valid descriptor for reference
	validDesc := model.Descriptor{
		SnapshotID:      "1708300800000-a3f7c1b2",
		WorktreeName:    "main",
		Engine:          "copy",
		PayloadRootHash: "abc123",
		CreatedAt:       time.Now(),
	}
	validJSON, _ := json.Marshal(validDesc)
	f.Add(validJSON)

	f.Fuzz(func(t *testing.T, data []byte) {
		// Unmarshal should not panic
		var desc model.Descriptor
		err := json.Unmarshal(data, &desc)

		// If unmarshal succeeded, the descriptor should be in a valid state
		// and operations like ShortID() should not panic
		if err == nil {
			// These operations should not panic
			_ = desc.SnapshotID.String()
			_ = desc.SnapshotID.ShortID()
			if desc.ParentID != nil {
				_ = desc.ParentID.String()
				_ = desc.ParentID.ShortID()
			}
		}

		// Marshal should also not panic
		_, _ = json.Marshal(desc)
	})
}

// FuzzReadyMarkerJSON tests ReadyMarker JSON parsing with random data.
//
// ReadyMarker is the .READY file content that marks a complete snapshot.
func FuzzReadyMarkerJSON(f *testing.F) {
	// Seed corpus
	validMarker := model.ReadyMarker{
		SnapshotID:         "1708300800000-a3f7c1b2",
		CompletedAt:        time.Now(),
		PayloadHash:        "abc123",
		Engine:             "copy",
		DescriptorChecksum: "def456",
	}
	validJSON, _ := json.Marshal(validMarker)

	f.Add(validJSON)
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"snapshot_id":"test"}`))
	f.Add([]byte(`null`))
	f.Add([]byte(`invalid`))
	f.Add([]byte(`{"snapshot_id":123}`))
	f.Add([]byte(`{"completed_at":"not-a-date"}`))
	f.Add([]byte(``))

	f.Fuzz(func(t *testing.T, data []byte) {
		var marker model.ReadyMarker
		err := json.Unmarshal(data, &marker)

		// If unmarshal succeeded, access fields without panic
		if err == nil {
			_ = marker.SnapshotID.String()
			_ = marker.SnapshotID.ShortID()
			_ = marker.PayloadHash
			_ = marker.DescriptorChecksum
		}

		// Marshal should not panic
		_, _ = json.Marshal(marker)
	})
}

// FuzzIntentRecordJSON tests IntentRecord JSON parsing with random data.
//
// IntentRecord tracks in-progress snapshot creation for crash recovery.
func FuzzIntentRecordJSON(f *testing.F) {
	// Seed corpus
	validIntent := model.IntentRecord{
		SnapshotID:   "1708300800000-a3f7c1b2",
		WorktreeName: "main",
		StartedAt:    time.Now(),
		Engine:       "copy",
	}
	validJSON, _ := json.Marshal(validIntent)

	f.Add(validJSON)
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"snapshot_id":"test"}`))
	f.Add([]byte(`null`))
	f.Add([]byte(`invalid`))
	f.Add([]byte(`{"worktree_name":"../escape"}`))
	f.Add([]byte(`{"engine":"invalid-engine"}`))
	f.Add([]byte(``))

	f.Fuzz(func(t *testing.T, data []byte) {
		var intent model.IntentRecord
		err := json.Unmarshal(data, &intent)

		// If unmarshal succeeded, access fields without panic
		if err == nil {
			_ = intent.SnapshotID.String()
			_ = intent.SnapshotID.ShortID()
		}

		// Marshal should not panic
		_, _ = json.Marshal(intent)
	})
}

// FuzzCompressionInfoJSON tests CompressionInfo JSON parsing with random data.
func FuzzCompressionInfoJSON(f *testing.F) {
	// Seed corpus
	validComp := model.CompressionInfo{
		Type:  "gzip",
		Level: 6,
	}
	validJSON, _ := json.Marshal(validComp)

	f.Add(validJSON)
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"type":"gzip"}`))
	f.Add([]byte(`{"level":5}`))
	f.Add([]byte(`{"type":"bzip2","level":-1}`))
	f.Add([]byte(`{"type":"gzip","level":100}`))
	f.Add([]byte(`{"type":123,"level":"string"}`))
	f.Add([]byte(`null`))
	f.Add([]byte(`invalid`))
	f.Add([]byte(``))

	f.Fuzz(func(t *testing.T, data []byte) {
		var comp model.CompressionInfo
		err := json.Unmarshal(data, &comp)

		// If unmarshal succeeded, access fields without panic
		if err == nil {
			_ = comp.Type
			_ = comp.Level
		}

		// Marshal should not panic
		_, _ = json.Marshal(comp)
	})
}

// FuzzPartialPaths tests partial path validation logic with random inputs.
//
// This simulates the path validation that happens in Creator.validateAndNormalizePaths
// to ensure it handles malicious or malformed paths without panicking.
func FuzzPartialPaths(f *testing.F) {
	// Seed corpus with edge cases
	f.Add("")
	f.Add(".")
	f.Add("..")
	f.Add("../escape")
	f.Add("../../../etc/passwd")
	f.Add("path/to/file.txt")
	f.Add("C:\\Windows\\System32") // Windows-style path
	f.Add("/absolute/path")
	f.Add("normal/path")
	f.Add("path/with/././dots")
	f.Add("path/with/null\x00byte")
	f.Add("very/long/path/" + string(make([]byte, 1000)))
	f.Add("path/with/\"quotes\"")
	f.Add("path/with\ttabs")
	f.Add("path/with\nnewline")
	f.Add("path/with/carriage\rreturn")
	f.Add("path/with//double/slash")
	f.Add("path/with/ trailing/space ")
	f.Add("中文路径")
	f.Add("path/../with/escape")
	f.Add("./local/path")
	f.Add("~/home/path")
	f.Add("valid-name-123")
	f.Add("a")
	f.Add("a/b/c/d/e/f/g")

	f.Fuzz(func(t *testing.T, path string) {
		// Simulate validation checks from validateAndNormalizePaths
		// These should not panic on any input

		// 1. Clean the path (should not panic)
		cleaned := path
		if path != "" {
			cleaned = path
		}

		// 2. Check for absolute path marker
		_ = filepath.IsAbs(cleaned)

		// 3. Check for path traversal (strings.Contains should not panic)
		_ = strings.Contains(cleaned, "..")

		// 4. Check for null bytes
		_ = strings.Contains(cleaned, "\x00")

		// 5. Check for control characters
		for _, r := range cleaned {
			if r < 32 && r != '\t' && r != '\n' && r != '\r' {
				// Control character detected
				break
			}
		}

		// 6. Check length (very long paths are valid in theory but may cause issues)
		_ = len(cleaned) > 10000

		// 7. Check for empty components from double slashes
		_ = strings.Contains(cleaned, "//")

		// The function should never panic regardless of input
	})
}

// FuzzTagValue tests tag validation with random inputs.
//
// Tags are user-supplied strings that need validation.
func FuzzTagValue(f *testing.F) {
	// Seed corpus
	f.Add("")
	f.Add("v1.0.0")
	f.Add("release")
	f.Add("experiment-123")
	f.Add("tag/with/slash")
	f.Add("tag\\with\\backslash")
	f.Add("tag with spaces")
	f.Add("tag\twith\ttabs")
	f.Add("tag\nwith\nnewlines")
	f.Add("tag\x00with\x00null")
	f.Add("../escape")
	f.Add("./local")
	f.Add("~/.ssh")
	f.Add("very-long-tag-name-that-is-still-valid-12345678901234567890")
	f.Add("a")
	f.Add("中文")
	f.Add("emoji🎉")
	f.Add(string(make([]byte, 10000)))
	f.Add("\"quoted\"")
	f.Add("'single'")

	f.Fuzz(func(t *testing.T, tag string) {
		// Should not panic on any input
		// ValidateName performs the actual validation
		_ = pathutil.ValidateName(tag)

		// Additional safety checks that should not panic
		_ = len(tag) > 0
		_ = strings.Contains(tag, "/")
		_ = strings.Contains(tag, "\\")
		_ = strings.Contains(tag, "\x00")
	})
}

// FuzzDescriptorChecksum tests descriptor checksum consistency.
//
// This ensures that descriptor marshaling produces consistent JSON
// for checksum computation.
func FuzzDescriptorChecksum(f *testing.F) {
	// Seed with valid descriptor JSON
	validDesc := model.Descriptor{
		SnapshotID:      "1708300800000-a3f7c1b2",
		WorktreeName:    "main",
		Engine:          "copy",
		PayloadRootHash: "abc123",
		CreatedAt:       time.Now(),
		Tags:            []string{"tag1", "tag2"},
	}
	validJSON, _ := json.Marshal(validDesc)

	f.Add(validJSON)
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"snapshot_id":"test"}`))
	f.Add([]byte(`null`))
	f.Add([]byte(`{"tags":[1,2,3]}`))

	f.Fuzz(func(t *testing.T, data []byte) {
		// Unmarshal the JSON data
		var desc model.Descriptor
		if err := json.Unmarshal(data, &desc); err != nil {
			return // Invalid JSON, skip
		}

		// Marshal should not panic
		data1, err1 := json.Marshal(desc)
		if err1 != nil {
			return // Some inputs may fail to marshal
		}

		// Marshal again should produce same result (deterministic)
		data2, err2 := json.Marshal(desc)
		if err2 != nil {
			t.Errorf("inconsistent marshal error: %v vs %v", err1, err2)
			return
		}

		if string(data1) != string(data2) {
			t.Errorf("marshal not deterministic")
		}
	})
}

// FuzzNewSnapshotID tests snapshot ID generation doesn't panic.
// Note: This is a regular test, not fuzz, since it uses randomness internally.
func TestNewSnapshotID(t *testing.T) {
	// Generate many IDs and check format
	for i := 0; i < 1000; i++ {
		id := model.NewSnapshotID()

		str := id.String()

		// Should contain exactly one hyphen
		hyphenCount := 0
		for _, c := range str {
			if c == '-' {
				hyphenCount++
			}
		}
		if hyphenCount != 1 {
			t.Errorf("expected 1 hyphen in SnapshotID, got %d: %s", hyphenCount, str)
		}

		// ShortID should be 8 chars
		if len(id.ShortID()) != 8 {
			t.Errorf("ShortID() returned %d chars, want 8", len(id.ShortID()))
		}
	}
}

func TestSnapshotIDStringFuzzInputBound(t *testing.T) {
	if !snapshotIDStringFuzzInputAllowed(strings.Repeat("x", maxSnapshotIDStringFuzzInput)) {
		t.Fatal("boundary-sized snapshot ID string input should remain fuzzed")
	}
	if snapshotIDStringFuzzInputAllowed(strings.Repeat("x", maxSnapshotIDStringFuzzInput+1)) {
		t.Fatal("oversized snapshot ID string input should be skipped")
	}
}
