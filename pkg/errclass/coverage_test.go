package errclass_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/agentsmith-project/jvs/pkg/errclass"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJVSError_Error_WithoutMessage(t *testing.T) {
	// When Message is empty, only Code should be returned
	err := &errclass.JVSError{Code: "E_TEST_ERROR"}
	assert.Equal(t, "E_TEST_ERROR", err.Error())
}

func TestJVSError_Error_EmptyCode(t *testing.T) {
	// Edge case: empty code with message
	err := &errclass.JVSError{Code: "", Message: "message only"}
	assert.Equal(t, ": message only", err.Error())
}

func TestJVSError_Error_BothEmpty(t *testing.T) {
	// Edge case: both empty - returns empty string since fmt.Sprintf("", "") = ""
	err := &errclass.JVSError{Code: "", Message: ""}
	assert.Equal(t, "", err.Error())
}

func TestJVSError_Is_DifferentCode(t *testing.T) {
	err1 := errclass.ErrNameInvalid.WithMessage("message")
	err2 := errclass.ErrPathEscape.WithMessage("message")

	// Should not match because different Codes
	require.False(t, errors.Is(err1, err2))
	require.False(t, errors.Is(err2, err1))
}

func TestJVSError_Is_WithStandardError(t *testing.T) {
	// JVSError should not match standard errors
	err := errclass.ErrNameInvalid.WithMessage("test")
	require.False(t, errors.Is(err, errors.New("some error")))
	require.False(t, errors.Is(errors.New("some error"), err))
}

func TestJVSError_Is_NilTarget(t *testing.T) {
	err := errclass.ErrNameInvalid.WithMessage("test")
	// errors.Is with nil target returns false
	require.False(t, errors.Is(err, nil))
}

func TestJVSError_Message(t *testing.T) {
	err := errclass.ErrNameInvalid
	assert.Empty(t, err.Message, "base error should have no message")

	errWithMsg := err.WithMessage("custom message")
	assert.Equal(t, "custom message", errWithMsg.Message)
}

func TestJVSError_WithMessage(t *testing.T) {
	baseErr := errclass.ErrNameInvalid

	// WithMessage should create a new error with the same Code
	err1 := baseErr.WithMessage("message 1")
	err2 := baseErr.WithMessage("message 2")

	assert.Equal(t, "E_NAME_INVALID", err1.Code)
	assert.Equal(t, "E_NAME_INVALID", err2.Code)
	assert.Equal(t, "message 1", err1.Message)
	assert.Equal(t, "message 2", err2.Message)

	// Original should be unchanged
	assert.Empty(t, baseErr.Message)
}

func TestJVSError_WithMessagef(t *testing.T) {
	baseErr := errclass.ErrNameInvalid

	// WithMessagef should create a new error with formatted message
	err := baseErr.WithMessagef("invalid value: %s", "test_value")

	assert.Equal(t, "E_NAME_INVALID", err.Code)
	assert.Equal(t, "invalid value: test_value", err.Message)
	assert.Contains(t, err.Error(), "invalid value: test_value")
}

func TestJVSError_WithMessagef_VariousFormats(t *testing.T) {
	baseErr := errclass.ErrGCPlanMismatch

	tests := []struct {
		name     string
		format   string
		args     []any
		expected string
	}{
		{
			name:     "single string",
			format:   "plan %s not found",
			args:     []any{"abc123"},
			expected: "plan abc123 not found",
		},
		{
			name:     "multiple strings",
			format:   "%s: %d snapshots affected",
			args:     []any{"plan1", 42},
			expected: "plan1: 42 snapshots affected",
		},
		{
			name:     "integer only",
			format:   "count: %d",
			args:     []any{100},
			expected: "count: 100",
		},
		{
			name:     "mixed types",
			format:   "operation %s failed at step %d with code %s",
			args:     []any{"clone", 3, "E_FAIL"},
			expected: "operation clone failed at step 3 with code E_FAIL",
		},
		{
			name:     "empty format",
			format:   "",
			args:     []any{},
			expected: "",
		},
		{
			name:     "special characters",
			format:   "error: %s! (retry in %d seconds)",
			args:     []any{"timeout", 30},
			expected: "error: timeout! (retry in 30 seconds)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := baseErr.WithMessagef(tt.format, tt.args...)
			assert.Equal(t, tt.expected, err.Message)
			assert.Equal(t, "E_GC_PLAN_MISMATCH", err.Code)
		})
	}
}

func TestJVSError_WithMessagef_PreservesCode(t *testing.T) {
	// Test all error classes preserve their Code through WithMessagef
	errors := []*errclass.JVSError{
		errclass.ErrNameInvalid,
		errclass.ErrPathEscape,
		errclass.ErrDescriptorCorrupt,
		errclass.ErrPayloadHashMismatch,
		errclass.ErrLineageBroken,
		errclass.ErrPartialSnapshot,
		errclass.ErrGCPlanMismatch,
		errclass.ErrFormatUnsupported,
		errclass.ErrAuditChainBroken,
	}

	codes := []string{
		"E_NAME_INVALID",
		"E_PATH_ESCAPE",
		"E_DESCRIPTOR_CORRUPT",
		"E_PAYLOAD_HASH_MISMATCH",
		"E_LINEAGE_BROKEN",
		"E_PARTIAL_SNAPSHOT",
		"E_GC_PLAN_MISMATCH",
		"E_FORMAT_UNSUPPORTED",
		"E_AUDIT_CHAIN_BROKEN",
	}

	for i, baseErr := range errors {
		t.Run(codes[i], func(t *testing.T) {
			err := baseErr.WithMessagef("test %d", i)
			assert.Equal(t, codes[i], err.Code, "code should be preserved")
			assert.Equal(t, fmt.Sprintf("test %d", i), err.Message)
		})
	}
}

func TestJVSError_WithMessagef_WithNilArgs(t *testing.T) {
	// WithMessagef with no args (just format string)
	baseErr := errclass.ErrPayloadHashMismatch
	err := baseErr.WithMessagef("no args test")

	assert.Equal(t, "E_PAYLOAD_HASH_MISMATCH", err.Code)
	assert.Equal(t, "no args test", err.Message)
}

func TestJVSError_WithMessagef_IntFormatting(t *testing.T) {
	baseErr := errclass.ErrLineageBroken

	err := baseErr.WithMessagef("broken at snapshot %d of chain", 5)
	assert.Equal(t, "broken at snapshot 5 of chain", err.Message)

	// Test multiple ints
	err = baseErr.WithMessagef("snapshots %d and %d are broken", 1, 2)
	assert.Equal(t, "snapshots 1 and 2 are broken", err.Message)
}

func TestJVSError_WithMessagef_FloatFormatting(t *testing.T) {
	baseErr := errclass.ErrFormatUnsupported

	err := baseErr.WithMessagef("version %f not supported", 2.5)
	assert.Equal(t, "version 2.500000 not supported", err.Message)
}

func TestJVSError_WithMessagef_BoolFormatting(t *testing.T) {
	baseErr := errclass.ErrGCPlanMismatch

	err := baseErr.WithMessagef("allow prune: %v, dry run: %v", true, false)
	assert.Equal(t, "allow prune: true, dry run: false", err.Message)
}

func TestJVSError_WithMessagef_StringFormatting(t *testing.T) {
	baseErr := errclass.ErrPathEscape

	err := baseErr.WithMessagef("path '%s' contains parent reference", "../etc/passwd")
	assert.Equal(t, "path '../etc/passwd' contains parent reference", err.Message)
}

func TestJVSError_WithMessagef_EscapedString(t *testing.T) {
	baseErr := errclass.ErrNameInvalid

	err := baseErr.WithMessagef("name %q is invalid", "test<>name")
	assert.Contains(t, err.Message, "test<>name")
}

func TestJVSError_WithMessagef_VerboseFormatting(t *testing.T) {
	baseErr := errclass.ErrDescriptorCorrupt

	err := baseErr.WithMessagef("checksum: %x, size: %d, expected: %x", 0xdeadbeef, 1024, 0xcafe1234)
	assert.Contains(t, err.Message, "deadbeef")
	assert.Contains(t, err.Message, "1024")
	assert.Contains(t, err.Message, "cafe1234")
}

func TestJVSError_WithMessagef_ComplexFormatting(t *testing.T) {
	baseErr := errclass.ErrAuditChainBroken

	// Complex nested struct formatting
	type AuditEntry struct {
		ID       string
		Sequence int
	}
	entry := AuditEntry{ID: "abc123", Sequence: 42}

	err := baseErr.WithMessagef("audit chain broken at entry %+v", entry)
	assert.Contains(t, err.Message, "abc123")
	assert.Contains(t, err.Message, "42")
}

func TestJVSError_Error_CombinesCodeAndMessage(t *testing.T) {
	// Test that Error() properly combines Code and Message
	tests := []struct {
		name     string
		code     string
		message  string
		expected string
	}{
		{
			name:     "both present",
			code:     "E_TEST",
			message:  "test message",
			expected: "E_TEST: test message",
		},
		{
			name:     "only code",
			code:     "E_CODE_ONLY",
			message:  "",
			expected: "E_CODE_ONLY",
		},
		{
			name:     "only message",
			code:     "",
			message:  "message only",
			expected: ": message only",
		},
		{
			name:     "both empty",
			code:     "",
			message:  "",
			expected: "",
		},
		{
			name:     "message with colon",
			code:     "E_TEST",
			message:  "message: with: colons",
			expected: "E_TEST: message: with: colons",
		},
		{
			name:     "code with colon",
			code:     "E_CODE:TEST",
			message:  "message",
			expected: "E_CODE:TEST: message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &errclass.JVSError{Code: tt.code, Message: tt.message}
			assert.Equal(t, tt.expected, err.Error())
		})
	}
}

func TestJVSError_WithMessage_Chaining(t *testing.T) {
	// Test that WithMessage can be chained multiple times
	baseErr := errclass.ErrGCPlanMismatch

	err1 := baseErr.WithMessage("first message")
	err2 := err1.WithMessage("second message")
	err3 := err2.WithMessagef("third message: %s", "detail")

	assert.Equal(t, "E_GC_PLAN_MISMATCH", err1.Code)
	assert.Equal(t, "first message", err1.Message)

	assert.Equal(t, "E_GC_PLAN_MISMATCH", err2.Code)
	assert.Equal(t, "second message", err2.Message)

	assert.Equal(t, "E_GC_PLAN_MISMATCH", err3.Code)
	assert.Equal(t, "third message: detail", err3.Message)
}

func TestJVSError_Is_MultipleTargets(t *testing.T) {
	// Test that Is works with multiple possible targets
	err := errclass.ErrNameInvalid.WithMessage("test")

	// Create slice of errors with same code
	sameCodeErrors := []error{
		errclass.ErrNameInvalid,
		errclass.ErrNameInvalid.WithMessage("different message"),
		err,
	}

	for _, target := range sameCodeErrors {
		assert.True(t, errors.Is(err, target), "should match error with same code")
	}

	// Different codes should not match
	differentCodes := []error{
		errclass.ErrPathEscape,
		errclass.ErrDescriptorCorrupt,
		errors.New("standard error"),
	}

	for _, target := range differentCodes {
		assert.False(t, errors.Is(err, target), "should not match different code")
	}
}

func TestJVSError_Is_Wrapping(t *testing.T) {
	// Test Is behavior when wrapping/wrapped by other errors
	jvsErr := errclass.ErrPayloadHashMismatch.WithMessage("hash mismatch")

	// Wrap in standard error
	wrapped := fmt.Errorf("wrapped: %w", jvsErr)

	// errors.Is should unwrap and match
	assert.True(t, errors.Is(wrapped, errclass.ErrPayloadHashMismatch))
	assert.True(t, errors.Is(wrapped, jvsErr))
}

func TestJVSError_As(t *testing.T) {
	// Test errors.As behavior
	err := errclass.ErrLineageBroken.WithMessage("lineage broken")

	var jvsErr *errclass.JVSError
	require.True(t, errors.As(err, &jvsErr))
	assert.Equal(t, "E_LINEAGE_BROKEN", jvsErr.Code)
	assert.Equal(t, "lineage broken", jvsErr.Message)
}

func TestJVSError_WithMessagef_NewInstance(t *testing.T) {
	// Ensure WithMessagef always returns a new instance
	baseErr := errclass.ErrPartialSnapshot

	err1 := baseErr.WithMessagef("test %s", "1")
	err2 := baseErr.WithMessagef("test %s", "2")

	// Should be different instances
	assert.NotSame(t, err1, err2)

	// But same code
	assert.Equal(t, err1.Code, err2.Code)
}

func TestAllErrorClasses_HaveValidFormat(t *testing.T) {
	// All error codes must start with "E_" and be uppercase
	allCodes := []string{
		errclass.ErrNameInvalid.Code,
		errclass.ErrPathEscape.Code,
		errclass.ErrDescriptorCorrupt.Code,
		errclass.ErrPayloadHashMismatch.Code,
		errclass.ErrLineageBroken.Code,
		errclass.ErrPartialSnapshot.Code,
		errclass.ErrGCPlanMismatch.Code,
		errclass.ErrFormatUnsupported.Code,
		errclass.ErrAuditChainBroken.Code,
	}

	for _, code := range allCodes {
		assert.True(t, len(code) > 2, "code should be longer than 2 chars")
		assert.Equal(t, "E_", code[0:2], "code should start with E_: "+code)
	}
}

func TestAllErrorClasses_IsStable(t *testing.T) {
	// Test that error classes are stable (can be reliably compared)
	// This is important for error matching in production code

	// Test that same error class always has same code
	for i := 0; i < 10; i++ {
		assert.Equal(t, "E_NAME_INVALID", errclass.ErrNameInvalid.Code)
	}

	// Test that errors.Is works consistently across calls
	err1 := errclass.ErrPathEscape.WithMessage("msg1")
	err2 := errclass.ErrPathEscape.WithMessage("msg2")

	// Both should match the base error class
	require.True(t, errors.Is(err1, errclass.ErrPathEscape))
	require.True(t, errors.Is(err2, errclass.ErrPathEscape))
}
