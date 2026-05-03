package errclass_test

import (
	"testing"

	"github.com/agentsmith-project/jvs/pkg/errclass"
	"github.com/stretchr/testify/require"
)

func TestSeparatedControlStableCodes(t *testing.T) {
	tests := map[string]*errclass.JVSError{
		"E_CONTROL_PAYLOAD_OVERLAP":             errclass.ErrControlPayloadOverlap,
		"E_PAYLOAD_INSIDE_CONTROL":              errclass.ErrPayloadInsideControl,
		"E_CONTROL_INSIDE_PAYLOAD":              errclass.ErrControlInsidePayload,
		"E_PATH_BOUNDARY_ESCAPE":                errclass.ErrPathBoundaryEscape,
		"E_CONTROL_MISSING":                     errclass.ErrControlMissing,
		"E_CONTROL_MALFORMED":                   errclass.ErrControlMalformed,
		"E_PAYLOAD_MISSING":                     errclass.ErrPayloadMissing,
		"E_REPO_ID_MISMATCH":                    errclass.ErrRepoIDMismatch,
		"E_WORKSPACE_MISMATCH":                  errclass.ErrWorkspaceMismatch,
		"E_PERMISSION_DENIED":                   errclass.ErrPermissionDenied,
		"E_EXPLICIT_TARGET_REQUIRED":            errclass.ErrExplicitTargetRequired,
		"E_PAYLOAD_LOCATOR_PRESENT":             errclass.ErrPayloadLocatorPresent,
		"E_TARGET_ROOT_OCCUPIED":                errclass.ErrTargetRootOccupied,
		"E_SOURCE_DIRTY":                        errclass.ErrSourceDirty,
		"E_ATOMIC_PUBLISH_BLOCKED":              errclass.ErrAtomicPublishBlocked,
		"E_IMPORTED_HISTORY_PROTECTION_MISSING": errclass.ErrImportedHistoryProtectionMissing,
		"E_SEPARATED_LIFECYCLE_UNSUPPORTED":     errclass.ErrSeparatedLifecycleUnsupported,
		"E_ACTIVE_OPERATION_BLOCKING":           errclass.ErrActiveOperationBlocking,
		"E_RECOVERY_BLOCKING":                   errclass.ErrRecoveryBlocking,
	}

	for code, err := range tests {
		t.Run(code, func(t *testing.T) {
			require.NotNil(t, err)
			require.Equal(t, code, err.Code)
		})
	}
}
