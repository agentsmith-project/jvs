package errclass

import "fmt"

// JVSError is a stable, machine-readable error class for JVS operations.
// It implements the error interface and supports error comparison via Is().
type JVSError struct {
	Code    string
	Message string
	Hint    string
}

func (e *JVSError) Error() string {
	if e.Message == "" {
		return e.Code
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *JVSError) Is(target error) bool {
	t, ok := target.(*JVSError)
	return ok && e.Code == t.Code
}

// WithMessage returns a new JVSError with the same Code but a specific message.
func (e *JVSError) WithMessage(msg string) *JVSError {
	return &JVSError{Code: e.Code, Message: msg, Hint: e.Hint}
}

// WithMessagef returns a new JVSError with a formatted message.
func (e *JVSError) WithMessagef(format string, args ...any) *JVSError {
	return &JVSError{Code: e.Code, Message: fmt.Sprintf(format, args...), Hint: e.Hint}
}

// WithHint returns a new JVSError with the same Code and Message plus a hint.
func (e *JVSError) WithHint(hint string) *JVSError {
	return &JVSError{Code: e.Code, Message: e.Message, Hint: hint}
}

// All stable error classes for v0.x.
var (
	ErrNotRepo             = &JVSError{Code: "E_NOT_REPO"}
	ErrNotWorkspace        = &JVSError{Code: "E_NOT_WORKSPACE"}
	ErrUsage               = &JVSError{Code: "E_USAGE"}
	ErrNameInvalid         = &JVSError{Code: "E_NAME_INVALID"}
	ErrPathEscape          = &JVSError{Code: "E_PATH_ESCAPE"}
	ErrDescriptorCorrupt   = &JVSError{Code: "E_DESCRIPTOR_CORRUPT"}
	ErrPayloadHashMismatch = &JVSError{Code: "E_PAYLOAD_HASH_MISMATCH"}
	ErrLineageBroken       = &JVSError{Code: "E_LINEAGE_BROKEN"}
	ErrPartialSnapshot     = &JVSError{Code: "E_PARTIAL_SNAPSHOT"}
	ErrGCPlanMismatch      = &JVSError{Code: "E_GC_PLAN_MISMATCH"}
	ErrFormatUnsupported   = &JVSError{Code: "E_FORMAT_UNSUPPORTED"}
	ErrAuditChainBroken    = &JVSError{Code: "E_AUDIT_CHAIN_BROKEN"}
	ErrRepoBusy            = &JVSError{Code: "E_REPO_BUSY"}
	ErrLockConflict        = &JVSError{Code: "E_LOCK_CONFLICT"}
)
