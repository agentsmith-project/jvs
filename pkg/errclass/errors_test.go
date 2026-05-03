package errclass_test

import (
	"errors"
	"testing"

	"github.com/agentsmith-project/jvs/pkg/errclass"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJVSError_Error(t *testing.T) {
	err := errclass.ErrNameInvalid.WithMessage("invalid name")
	assert.Equal(t, "E_NAME_INVALID: invalid name", err.Error())
}

func TestJVSError_Is(t *testing.T) {
	err := errclass.ErrNameInvalid.WithMessage("specific message")
	require.True(t, errors.Is(err, errclass.ErrNameInvalid))
	require.False(t, errors.Is(err, errclass.ErrPathEscape))
}

func TestJVSError_Code(t *testing.T) {
	assert.Equal(t, "E_NAME_INVALID", errclass.ErrNameInvalid.Code)
	assert.Equal(t, "E_PATH_ESCAPE", errclass.ErrPathEscape.Code)
}

func TestJVSError_AllErrorsDefined(t *testing.T) {
	// All v0.x error classes must exist
	all := []error{
		errclass.ErrNotRepo,
		errclass.ErrNotWorkspace,
		errclass.ErrUsage,
		errclass.ErrRepoBusy,
		errclass.ErrNameInvalid,
		errclass.ErrPathEscape,
		errclass.ErrDescriptorCorrupt,
		errclass.ErrPayloadHashMismatch,
		errclass.ErrLineageBroken,
		errclass.ErrPartialSnapshot,
		errclass.ErrGCPlanMismatch,
		errclass.ErrFormatUnsupported,
		errclass.ErrAuditChainBroken,
		errclass.ErrLifecyclePending,
		errclass.ErrLifecycleUnsafeCWD,
		errclass.ErrLifecycleIdentityMismatch,
	}
	assert.Len(t, all, 16)
}

func TestJVSError_CLIContractCodes(t *testing.T) {
	assert.Equal(t, "E_NOT_REPO", errclass.ErrNotRepo.Code)
	assert.Equal(t, "E_NOT_WORKSPACE", errclass.ErrNotWorkspace.Code)
	assert.Equal(t, "E_USAGE", errclass.ErrUsage.Code)
	assert.Equal(t, "E_REPO_BUSY", errclass.ErrRepoBusy.Code)
	assert.Equal(t, "E_LIFECYCLE_PENDING", errclass.ErrLifecyclePending.Code)
	assert.Equal(t, "E_LIFECYCLE_UNSAFE_CWD", errclass.ErrLifecycleUnsafeCWD.Code)
	assert.Equal(t, "E_LIFECYCLE_IDENTITY_MISMATCH", errclass.ErrLifecycleIdentityMismatch.Code)
}
