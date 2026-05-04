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
	ErrNotRepo                   = &JVSError{Code: "E_NOT_REPO"}
	ErrNotWorkspace              = &JVSError{Code: "E_NOT_WORKSPACE"}
	ErrTargetMismatch            = &JVSError{Code: "E_TARGET_MISMATCH"}
	ErrUsage                     = &JVSError{Code: "E_USAGE"}
	ErrNameInvalid               = &JVSError{Code: "E_NAME_INVALID"}
	ErrPathEscape                = &JVSError{Code: "E_PATH_ESCAPE"}
	ErrDescriptorCorrupt         = &JVSError{Code: "E_DESCRIPTOR_CORRUPT"}
	ErrSavePointHashMismatch     = &JVSError{Code: "E_SAVE_POINT_HASH_MISMATCH"}
	ErrLineageBroken             = &JVSError{Code: "E_LINEAGE_BROKEN"}
	ErrPartialSavePoint          = &JVSError{Code: "E_PARTIAL_SAVE_POINT"}
	ErrCleanupPlanMismatch       = &JVSError{Code: "E_CLEANUP_PLAN_MISMATCH"}
	ErrFormatUnsupported         = &JVSError{Code: "E_FORMAT_UNSUPPORTED"}
	ErrAuditChainBroken          = &JVSError{Code: "E_AUDIT_CHAIN_BROKEN"}
	ErrRepoBusy                  = &JVSError{Code: "E_REPO_BUSY"}
	ErrLockConflict              = &JVSError{Code: "E_LOCK_CONFLICT"}
	ErrLifecyclePending          = &JVSError{Code: "E_LIFECYCLE_PENDING"}
	ErrLifecycleUnsafeCWD        = &JVSError{Code: "E_LIFECYCLE_UNSAFE_CWD"}
	ErrLifecycleIdentityMismatch = &JVSError{Code: "E_LIFECYCLE_IDENTITY_MISMATCH"}

	ErrControlWorkspaceOverlap          = &JVSError{Code: "E_CONTROL_WORKSPACE_OVERLAP"}
	ErrWorkspaceInsideControl           = &JVSError{Code: "E_WORKSPACE_INSIDE_CONTROL"}
	ErrControlInsideWorkspace           = &JVSError{Code: "E_CONTROL_INSIDE_WORKSPACE"}
	ErrPathBoundaryEscape               = &JVSError{Code: "E_PATH_BOUNDARY_ESCAPE"}
	ErrControlMissing                   = &JVSError{Code: "E_CONTROL_MISSING"}
	ErrControlMalformed                 = &JVSError{Code: "E_CONTROL_MALFORMED"}
	ErrWorkspaceMissing                 = &JVSError{Code: "E_WORKSPACE_MISSING"}
	ErrRepoIDMismatch                   = &JVSError{Code: "E_REPO_ID_MISMATCH"}
	ErrWorkspaceMismatch                = &JVSError{Code: "E_WORKSPACE_MISMATCH"}
	ErrPermissionDenied                 = &JVSError{Code: "E_PERMISSION_DENIED"}
	ErrExplicitTargetRequired           = &JVSError{Code: "E_EXPLICIT_TARGET_REQUIRED"}
	ErrWorkspaceControlMarkerPresent    = &JVSError{Code: "E_WORKSPACE_CONTROL_MARKER_PRESENT"}
	ErrTargetRootOccupied               = &JVSError{Code: "E_TARGET_ROOT_OCCUPIED"}
	ErrSourceDirty                      = &JVSError{Code: "E_SOURCE_DIRTY"}
	ErrAtomicPublishBlocked             = &JVSError{Code: "E_ATOMIC_PUBLISH_BLOCKED"}
	ErrImportedHistoryProtectionMissing = &JVSError{Code: "E_IMPORTED_HISTORY_PROTECTION_MISSING"}
	ErrExternalLifecycleUnsupported     = &JVSError{Code: "E_EXTERNAL_LIFECYCLE_UNSUPPORTED"}
	ErrActiveOperationBlocking          = &JVSError{Code: "E_ACTIVE_OPERATION_BLOCKING"}
	ErrRecoveryBlocking                 = &JVSError{Code: "E_RECOVERY_BLOCKING"}
)
