package afscp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAFSCPDirectRequiresControlRootAndHomePair(t *testing.T) {
	base := t.TempDir()
	controlRoot := filepath.Join(base, "control")
	home := filepath.Join(base, "home")
	require.NoError(t, os.Mkdir(controlRoot, 0755))
	require.NoError(t, os.Mkdir(home, 0755))

	for _, tc := range []struct {
		name     string
		selector Selector
	}{
		{
			name: "missing control root",
			selector: Selector{
				Home: home,
			},
		},
		{
			name: "missing home",
			selector: Selector{
				ControlRoot: controlRoot,
			},
		},
		{
			name: "relative control root",
			selector: Selector{
				ControlRoot: "relative-control",
				Home:        home,
			},
		},
		{
			name: "relative home",
			selector: Selector{
				ControlRoot: controlRoot,
				Home:        "relative-home",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ValidateSelector(tc.selector)
			requireDirectError(t, err, ErrorCodeInvalidArgument, ExitInvalidArgument)
		})
	}
}

func TestAFSCPDirectRejectsHomeContainingJVSMetadata(t *testing.T) {
	base := t.TempDir()
	controlRoot := filepath.Join(base, "control")
	home := filepath.Join(base, "home")
	require.NoError(t, os.Mkdir(controlRoot, 0755))
	require.NoError(t, os.Mkdir(home, 0755))
	require.NoError(t, os.Mkdir(filepath.Join(home, ".jvs"), 0755))

	_, err := ValidateSelector(Selector{
		ControlRoot: controlRoot,
		Home:        home,
	})
	requireDirectError(t, err, ErrorCodeInvalidArgument, ExitInvalidArgument)
}

func TestAFSCPDirectMetadataSelectorRejectsHomeContainingJVSMetadata(t *testing.T) {
	base := t.TempDir()
	controlRoot := filepath.Join(base, "control")
	home := filepath.Join(base, "home")
	require.NoError(t, os.Mkdir(controlRoot, 0755))
	require.NoError(t, os.Mkdir(home, 0755))
	require.NoError(t, os.Mkdir(filepath.Join(home, ".jvs"), 0755))

	_, err := ValidateMetadataSelector(Selector{
		ControlRoot: controlRoot,
		Home:        home,
	})
	requireDirectError(t, err, ErrorCodeInvalidArgument, ExitInvalidArgument)
}

func TestAFSCPDirectRejectsOverlappingRoots(t *testing.T) {
	base := t.TempDir()
	controlRoot := filepath.Join(base, "control")
	home := filepath.Join(base, "control", "home")
	require.NoError(t, os.MkdirAll(home, 0755))

	_, err := ValidateSelector(Selector{
		ControlRoot: controlRoot,
		Home:        home,
	})
	requireDirectError(t, err, ErrorCodeInvalidArgument, ExitInvalidArgument)
}

func TestAFSCPDirectJSONDoesNotLeakRootsOrLegacyHashFields(t *testing.T) {
	base := t.TempDir()
	controlRoot := filepath.Join(base, "control")
	home := filepath.Join(base, "home")
	require.NoError(t, os.Mkdir(controlRoot, 0755))
	require.NoError(t, os.Mkdir(home, 0755))

	result, err := NewService().List(context.Background(), Request{
		Selector: Selector{
			ControlRoot: controlRoot,
			Home:        home,
		},
	})
	require.NoError(t, err)

	payload, err := json.Marshal(SuccessEnvelope(CommandList, result))
	require.NoError(t, err)
	directJSON := string(payload)

	assert.NotContains(t, directJSON, controlRoot)
	assert.NotContains(t, directJSON, home)
	assert.NotContains(t, directJSON, filepath.Base(controlRoot))
	assert.NotContains(t, directJSON, filepath.Base(home))
	for _, forbidden := range []string{
		"payload_root_hash",
		"content_root_hash",
		"save_profile",
		"expected_folder_evidence",
	} {
		assert.NotContains(t, strings.ToLower(directJSON), forbidden)
	}
}

func TestAFSCPDirectExitCodesMapStableErrors(t *testing.T) {
	for _, tc := range []struct {
		code ErrorCode
		want int
	}{
		{code: ErrorCodeInvalidArgument, want: ExitInvalidArgument},
		{code: ErrorCodeMetadataInvalid, want: ExitMetadata},
		{code: ErrorCodeJournalRecoveryRequired, want: ExitMetadata},
		{code: ErrorCodeLocked, want: ExitLocked},
		{code: ErrorCodeCloneUnavailable, want: ExitStorage},
		{code: ErrorCodeCloneFailed, want: ExitStorage},
		{code: ErrorCodeSavePointNotFound, want: ExitInvalidArgument},
		{code: ErrorCodeInternal, want: ExitInternal},
	} {
		t.Run(string(tc.code), func(t *testing.T) {
			assert.Equal(t, tc.want, ExitCode(NewError(tc.code, "operator-safe message", false)))
		})
	}
}

func requireDirectError(t *testing.T, err error, wantCode ErrorCode, wantExit int) {
	t.Helper()

	require.Error(t, err)
	var directErr *Error
	require.ErrorAs(t, err, &directErr)
	assert.Equal(t, wantCode, directErr.Code)
	assert.Equal(t, wantExit, ExitCode(err))
}
