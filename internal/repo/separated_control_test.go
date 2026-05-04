package repo_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/pkg/errclass"
	"github.com/stretchr/testify/require"
)

func TestSeparatedInitRejectsBoundaryRootsWithoutMutation(t *testing.T) {
	for _, tc := range []struct {
		name        string
		controlRoot func(string) string
		payloadRoot func(string) string
		wantErr     error
		wantMissing []func(string) string
	}{
		{
			name:        "same root",
			controlRoot: func(base string) string { return filepath.Join(base, "repo") },
			payloadRoot: func(base string) string { return filepath.Join(base, "repo") },
			wantErr:     errclass.ErrControlPayloadOverlap,
			wantMissing: []func(string) string{
				func(base string) string { return filepath.Join(base, "repo") },
			},
		},
		{
			name:        "payload inside control",
			controlRoot: func(base string) string { return filepath.Join(base, "control") },
			payloadRoot: func(base string) string { return filepath.Join(base, "control", "payload") },
			wantErr:     errclass.ErrPayloadInsideControl,
			wantMissing: []func(string) string{
				func(base string) string { return filepath.Join(base, "control") },
			},
		},
		{
			name:        "control inside payload",
			controlRoot: func(base string) string { return filepath.Join(base, "payload", "control") },
			payloadRoot: func(base string) string { return filepath.Join(base, "payload") },
			wantErr:     errclass.ErrControlInsidePayload,
			wantMissing: []func(string) string{
				func(base string) string { return filepath.Join(base, "payload") },
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := t.TempDir()
			_, err := repo.InitSeparatedControl(tc.controlRoot(base), tc.payloadRoot(base), "main")
			require.ErrorIs(t, err, tc.wantErr)
			for _, missing := range tc.wantMissing {
				require.NoFileExists(t, missing(base))
			}
		})
	}
}

func TestSeparatedInitRejectsPhysicalAlias(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior differs on Windows")
	}

	base := t.TempDir()
	physical := filepath.Join(base, "physical")
	require.NoError(t, os.MkdirAll(physical, 0755))
	controlRoot := filepath.Join(base, "control-link")
	payloadRoot := filepath.Join(base, "payload-link")
	require.NoError(t, os.Symlink(physical, controlRoot))
	require.NoError(t, os.Symlink(physical, payloadRoot))

	_, err := repo.InitSeparatedControl(controlRoot, payloadRoot, "main")
	require.ErrorIs(t, err, errclass.ErrControlPayloadOverlap)
	require.NoDirExists(t, filepath.Join(physical, ".jvs"))
}

func TestSeparatedInitRejectsOccupiedControlRootWithoutMutatingPayload(t *testing.T) {
	base := t.TempDir()
	controlRoot := filepath.Join(base, "control")
	payloadRoot := filepath.Join(base, "payload")
	require.NoError(t, os.MkdirAll(controlRoot, 0755))
	sentinel := filepath.Join(controlRoot, "sentinel.txt")
	require.NoError(t, os.WriteFile(sentinel, []byte("keep\n"), 0644))

	_, err := repo.InitSeparatedControl(controlRoot, payloadRoot, "main")
	require.ErrorIs(t, err, errclass.ErrTargetRootOccupied)
	require.FileExists(t, sentinel)
	require.NoFileExists(t, payloadRoot)
}

func TestSeparatedInitAdoptsExistingNonEmptyPayloadWithoutControlMarker(t *testing.T) {
	base := t.TempDir()
	controlRoot := filepath.Join(base, "control")
	payloadRoot := filepath.Join(base, "payload")
	require.NoError(t, os.MkdirAll(filepath.Join(payloadRoot, "src"), 0755))
	userFile := filepath.Join(payloadRoot, "src", "app.txt")
	require.NoError(t, os.WriteFile(userFile, []byte("user data\n"), 0644))
	require.NoError(t, os.Chmod(userFile, 0640))
	originalMTime := time.Date(2024, 2, 3, 4, 5, 6, 0, time.UTC)
	require.NoError(t, os.Chtimes(userFile, originalMTime, originalMTime))
	before, err := os.Stat(userFile)
	require.NoError(t, err)

	r, err := repo.InitSeparatedControl(controlRoot, payloadRoot, "main")
	require.NoError(t, err)
	require.Equal(t, controlRoot, r.Root)
	require.DirExists(t, filepath.Join(controlRoot, ".jvs"))
	require.NoFileExists(t, filepath.Join(payloadRoot, ".jvs"))
	require.FileExists(t, userFile)
	after, err := os.Stat(userFile)
	require.NoError(t, err)
	require.Equal(t, before.Mode(), after.Mode())
	require.Equal(t, before.ModTime(), after.ModTime())

	cfg, err := repo.LoadWorktreeConfig(controlRoot, "main")
	require.NoError(t, err)
	require.Equal(t, "main", cfg.Name)
	require.Equal(t, payloadRoot, cfg.RealPath)
}

func TestSeparatedInitRejectsPayloadSymlinkEscapeWithoutControlMutation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior differs on Windows")
	}

	base := t.TempDir()
	controlRoot := filepath.Join(base, "control")
	payloadRoot := filepath.Join(base, "payload")
	outsideRoot := filepath.Join(base, "outside")
	require.NoError(t, os.MkdirAll(payloadRoot, 0755))
	require.NoError(t, os.MkdirAll(outsideRoot, 0755))
	outsideFile := filepath.Join(outsideRoot, "secret.txt")
	require.NoError(t, os.WriteFile(outsideFile, []byte("outside\n"), 0644))
	linkPath := filepath.Join(payloadRoot, "escape")
	if err := os.Symlink(outsideFile, linkPath); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	_, err := repo.InitSeparatedControl(controlRoot, payloadRoot, "main")
	require.ErrorIs(t, err, errclass.ErrPathBoundaryEscape)
	require.NoFileExists(t, filepath.Join(controlRoot, ".jvs"))
	require.NoFileExists(t, controlRoot)
	require.FileExists(t, linkPath)
	require.FileExists(t, outsideFile)
}

func TestSeparatedInitRejectsPayloadRootSymlinkWithoutMutation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior differs on Windows")
	}

	base := t.TempDir()
	controlRoot := filepath.Join(base, "control")
	realPayload := filepath.Join(base, "real-payload")
	payloadRoot := filepath.Join(base, "payload-link")
	require.NoError(t, os.MkdirAll(realPayload, 0755))
	userFile := filepath.Join(realPayload, "app.txt")
	require.NoError(t, os.WriteFile(userFile, []byte("user data\n"), 0644))
	if err := os.Symlink(realPayload, payloadRoot); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	_, err := repo.InitSeparatedControl(controlRoot, payloadRoot, "main")
	require.ErrorIs(t, err, errclass.ErrTargetRootOccupied)
	require.FileExists(t, userFile)
	require.NoFileExists(t, controlRoot)
}

func TestSeparatedInitRejectsPayloadRootControlMarkerWithoutMutation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		marker func(*testing.T, string)
	}{
		{
			name: "file",
			marker: func(t *testing.T, payloadRoot string) {
				t.Helper()
				require.NoError(t, os.WriteFile(filepath.Join(payloadRoot, ".jvs"), []byte("untrusted\n"), 0644))
			},
		},
		{
			name: "directory",
			marker: func(t *testing.T, payloadRoot string) {
				t.Helper()
				require.NoError(t, os.Mkdir(filepath.Join(payloadRoot, ".jvs"), 0755))
			},
		},
		{
			name: "symlink",
			marker: func(t *testing.T, payloadRoot string) {
				t.Helper()
				if err := os.Symlink("elsewhere", filepath.Join(payloadRoot, ".jvs")); err != nil {
					t.Skipf("symlink unavailable: %v", err)
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := t.TempDir()
			controlRoot := filepath.Join(base, "control")
			payloadRoot := filepath.Join(base, "payload")
			require.NoError(t, os.MkdirAll(payloadRoot, 0755))
			tc.marker(t, payloadRoot)

			_, err := repo.InitSeparatedControl(controlRoot, payloadRoot, "main")
			require.ErrorIs(t, err, errclass.ErrPayloadLocatorPresent)
			require.NoFileExists(t, controlRoot)
		})
	}
}

func TestSeparatedInitCreatesControlOnlyAndStoresPayloadRealPath(t *testing.T) {
	base := t.TempDir()
	controlRoot := filepath.Join(base, "control")
	payloadRoot := filepath.Join(base, "payload")

	r, err := repo.InitSeparatedControl(controlRoot, payloadRoot, "main")
	require.NoError(t, err)
	require.Equal(t, controlRoot, r.Root)
	require.Equal(t, repo.RepoModeSeparatedControl, r.Mode)
	require.DirExists(t, filepath.Join(controlRoot, ".jvs"))
	require.NoFileExists(t, filepath.Join(payloadRoot, ".jvs"))

	mode, err := repo.LoadRepoMode(controlRoot)
	require.NoError(t, err)
	require.Equal(t, repo.RepoModeSeparatedControl, mode)

	cfg, err := repo.LoadWorktreeConfig(controlRoot, "main")
	require.NoError(t, err)
	require.Equal(t, "main", cfg.Name)
	require.Equal(t, payloadRoot, cfg.RealPath)
}

func TestSeparatedInitRejectsNonMainWithoutMutation(t *testing.T) {
	base := t.TempDir()
	controlRoot := filepath.Join(base, "control")
	payloadRoot := filepath.Join(base, "payload")

	_, err := repo.InitSeparatedControl(controlRoot, payloadRoot, "feature")
	require.ErrorIs(t, err, errclass.ErrWorkspaceMismatch)
	require.NoDirExists(t, controlRoot)
	require.NoDirExists(t, payloadRoot)
}

func TestRepoModeDefaultsEmbeddedWhenMetadataMissing(t *testing.T) {
	repoRoot := filepath.Join(t.TempDir(), "repo")
	r, err := repo.Init(repoRoot, "repo")
	require.NoError(t, err)
	require.Equal(t, repo.RepoModeEmbeddedControl, r.Mode)

	require.NoError(t, os.Remove(filepath.Join(repoRoot, ".jvs", repo.RepoModeFile)))
	mode, err := repo.LoadRepoMode(repoRoot)
	require.NoError(t, err)
	require.Equal(t, repo.RepoModeEmbeddedControl, mode)

	opened, err := repo.OpenControlRoot(repoRoot)
	require.NoError(t, err)
	require.Equal(t, repo.RepoModeEmbeddedControl, opened.Mode)
}

func TestResolveSeparatedContextUsesExplicitControlRootAndRegistry(t *testing.T) {
	base := t.TempDir()
	controlRoot := filepath.Join(base, "control")
	payloadRoot := filepath.Join(base, "payload")
	otherRoot := filepath.Join(base, "other")
	_, err := repo.InitSeparatedControl(controlRoot, payloadRoot, "main")
	require.NoError(t, err)
	_, err = repo.Init(otherRoot, "other")
	require.NoError(t, err)

	ctx, err := repo.ResolveSeparatedContext(repo.SeparatedContextRequest{
		ControlRoot: controlRoot,
		Workspace:   "main",
	})
	require.NoError(t, err)
	require.Equal(t, controlRoot, ctx.ControlRoot)
	require.Equal(t, payloadRoot, ctx.PayloadRoot)
	require.Equal(t, repo.RepoModeSeparatedControl, ctx.Repo.Mode)
	require.False(t, ctx.LocatorAuthoritative)
	require.True(t, ctx.BoundaryValidated)
}

func TestResolveSeparatedContextPayloadLocatorPresentFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name   string
		marker func(*testing.T, string)
	}{
		{
			name: "file",
			marker: func(t *testing.T, payloadRoot string) {
				t.Helper()
				require.NoError(t, os.WriteFile(filepath.Join(payloadRoot, ".jvs"), []byte("untrusted\n"), 0644))
			},
		},
		{
			name: "directory",
			marker: func(t *testing.T, payloadRoot string) {
				t.Helper()
				require.NoError(t, os.Mkdir(filepath.Join(payloadRoot, ".jvs"), 0755))
			},
		},
		{
			name: "symlink",
			marker: func(t *testing.T, payloadRoot string) {
				t.Helper()
				if err := os.Symlink("elsewhere", filepath.Join(payloadRoot, ".jvs")); err != nil {
					t.Skipf("symlink unavailable: %v", err)
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := t.TempDir()
			controlRoot := filepath.Join(base, "control")
			payloadRoot := filepath.Join(base, "payload")
			_, err := repo.InitSeparatedControl(controlRoot, payloadRoot, "main")
			require.NoError(t, err)
			tc.marker(t, payloadRoot)

			_, err = repo.ResolveSeparatedContext(repo.SeparatedContextRequest{
				ControlRoot: controlRoot,
				Workspace:   "main",
			})
			require.ErrorIs(t, err, errclass.ErrPayloadLocatorPresent)
		})
	}
}

func TestWorktreeManagedPayloadBoundarySeparatedPayloadLocatorPresentFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name   string
		marker func(*testing.T, string)
	}{
		{
			name: "file",
			marker: func(t *testing.T, payloadRoot string) {
				t.Helper()
				require.NoError(t, os.WriteFile(filepath.Join(payloadRoot, ".jvs"), []byte("untrusted\n"), 0644))
			},
		},
		{
			name: "directory",
			marker: func(t *testing.T, payloadRoot string) {
				t.Helper()
				require.NoError(t, os.Mkdir(filepath.Join(payloadRoot, ".jvs"), 0755))
			},
		},
		{
			name: "symlink",
			marker: func(t *testing.T, payloadRoot string) {
				t.Helper()
				if err := os.Symlink("elsewhere", filepath.Join(payloadRoot, ".jvs")); err != nil {
					t.Skipf("symlink unavailable: %v", err)
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := t.TempDir()
			controlRoot := filepath.Join(base, "control")
			payloadRoot := filepath.Join(base, "payload")
			_, err := repo.InitSeparatedControl(controlRoot, payloadRoot, "main")
			require.NoError(t, err)
			tc.marker(t, payloadRoot)

			_, err = repo.WorktreeManagedPayloadBoundary(controlRoot, "main")
			require.ErrorIs(t, err, errclass.ErrPayloadLocatorPresent)
		})
	}
}

func TestWorktreeManagedPayloadBoundarySeparatedDoesNotReadPayloadLocatorContent(t *testing.T) {
	base := t.TempDir()
	controlRoot := filepath.Join(base, "control")
	payloadRoot := filepath.Join(base, "payload")
	_, err := repo.InitSeparatedControl(controlRoot, payloadRoot, "main")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(payloadRoot, ".jvs"), []byte("{not-json"), 0644))

	_, err = repo.WorktreeManagedPayloadBoundary(controlRoot, "main")
	require.ErrorIs(t, err, errclass.ErrPayloadLocatorPresent)
	require.NotContains(t, err.Error(), "parse JVS workspace locator")
}

func TestRevalidateSeparatedContext(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		base := t.TempDir()
		controlRoot := filepath.Join(base, "control")
		payloadRoot := filepath.Join(base, "payload")
		r, err := repo.InitSeparatedControl(controlRoot, payloadRoot, "main")
		require.NoError(t, err)

		ctx, err := repo.RevalidateSeparatedContext(repo.SeparatedContextRevalidationRequest{
			ControlRoot:         controlRoot,
			Workspace:           "main",
			ExpectedRepoID:      r.RepoID,
			ExpectedPayloadRoot: payloadRoot,
		})
		require.NoError(t, err)
		require.Equal(t, payloadRoot, ctx.PayloadRoot)
	})

	t.Run("control repo mode changed", func(t *testing.T) {
		base := t.TempDir()
		controlRoot := filepath.Join(base, "control")
		payloadRoot := filepath.Join(base, "payload")
		r, err := repo.InitSeparatedControl(controlRoot, payloadRoot, "main")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(controlRoot, ".jvs", repo.RepoModeFile), []byte(repo.RepoModeEmbeddedControl+"\n"), 0600))

		_, err = repo.RevalidateSeparatedContext(repo.SeparatedContextRevalidationRequest{
			ControlRoot:         controlRoot,
			Workspace:           "main",
			ExpectedRepoID:      r.RepoID,
			ExpectedPayloadRoot: payloadRoot,
		})
		require.ErrorIs(t, err, errclass.ErrControlMalformed)
	})

	t.Run("repo id changed", func(t *testing.T) {
		base := t.TempDir()
		controlRoot := filepath.Join(base, "control")
		payloadRoot := filepath.Join(base, "payload")
		r, err := repo.InitSeparatedControl(controlRoot, payloadRoot, "main")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(controlRoot, ".jvs", repo.RepoIDFile), []byte("different-repo-id\n"), 0600))

		_, err = repo.RevalidateSeparatedContext(repo.SeparatedContextRevalidationRequest{
			ControlRoot:         controlRoot,
			Workspace:           "main",
			ExpectedRepoID:      r.RepoID,
			ExpectedPayloadRoot: payloadRoot,
		})
		require.ErrorIs(t, err, errclass.ErrRepoIDMismatch)
	})

	t.Run("registry payload root changed", func(t *testing.T) {
		base := t.TempDir()
		controlRoot := filepath.Join(base, "control")
		payloadRoot := filepath.Join(base, "payload")
		otherPayloadRoot := filepath.Join(base, "other-payload")
		r, err := repo.InitSeparatedControl(controlRoot, payloadRoot, "main")
		require.NoError(t, err)
		require.NoError(t, os.Mkdir(otherPayloadRoot, 0755))
		cfg, err := repo.LoadWorktreeConfig(controlRoot, "main")
		require.NoError(t, err)
		cfg.RealPath = otherPayloadRoot
		require.NoError(t, repo.WriteWorktreeConfig(controlRoot, "main", cfg))

		_, err = repo.RevalidateSeparatedContext(repo.SeparatedContextRevalidationRequest{
			ControlRoot:         controlRoot,
			Workspace:           "main",
			ExpectedRepoID:      r.RepoID,
			ExpectedPayloadRoot: payloadRoot,
		})
		require.ErrorIs(t, err, errclass.ErrPathBoundaryEscape)
	})

	t.Run("payload root missing", func(t *testing.T) {
		base := t.TempDir()
		controlRoot := filepath.Join(base, "control")
		payloadRoot := filepath.Join(base, "payload")
		r, err := repo.InitSeparatedControl(controlRoot, payloadRoot, "main")
		require.NoError(t, err)
		require.NoError(t, os.RemoveAll(payloadRoot))

		_, err = repo.RevalidateSeparatedContext(repo.SeparatedContextRevalidationRequest{
			ControlRoot:         controlRoot,
			Workspace:           "main",
			ExpectedRepoID:      r.RepoID,
			ExpectedPayloadRoot: payloadRoot,
		})
		require.ErrorIs(t, err, errclass.ErrPayloadMissing)
	})

	t.Run("payload locator present", func(t *testing.T) {
		base := t.TempDir()
		controlRoot := filepath.Join(base, "control")
		payloadRoot := filepath.Join(base, "payload")
		r, err := repo.InitSeparatedControl(controlRoot, payloadRoot, "main")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(payloadRoot, ".jvs"), []byte("untrusted\n"), 0644))

		_, err = repo.RevalidateSeparatedContext(repo.SeparatedContextRevalidationRequest{
			ControlRoot:         controlRoot,
			Workspace:           "main",
			ExpectedRepoID:      r.RepoID,
			ExpectedPayloadRoot: payloadRoot,
		})
		require.ErrorIs(t, err, errclass.ErrPayloadLocatorPresent)
	})
}

func TestValidateSeparatedPayloadSymlinkBoundary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior differs on Windows")
	}

	for _, tc := range []struct {
		name   string
		target func(*testing.T, string, string, string) string
	}{
		{
			name: "control root",
			target: func(t *testing.T, base, controlRoot, payloadRoot string) string {
				t.Helper()
				return controlRoot
			},
		},
		{
			name: "outside payload",
			target: func(t *testing.T, base, controlRoot, payloadRoot string) string {
				t.Helper()
				outside := filepath.Join(base, "outside")
				require.NoError(t, os.Mkdir(outside, 0755))
				return outside
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := t.TempDir()
			controlRoot := filepath.Join(base, "control")
			payloadRoot := filepath.Join(base, "payload")
			r, err := repo.InitSeparatedControl(controlRoot, payloadRoot, "main")
			require.NoError(t, err)
			ctx := &repo.SeparatedContext{
				Repo:        r,
				ControlRoot: controlRoot,
				PayloadRoot: payloadRoot,
				Workspace:   "main",
			}

			require.NoError(t, os.Symlink(tc.target(t, base, controlRoot, payloadRoot), filepath.Join(payloadRoot, "link")))

			err = repo.ValidateSeparatedPayloadSymlinkBoundary(ctx)
			require.ErrorIs(t, err, errclass.ErrPathBoundaryEscape)
		})
	}
}

func TestResolveSeparatedContextStableErrorCodes(t *testing.T) {
	t.Run("missing control root", func(t *testing.T) {
		_, err := repo.ResolveSeparatedContext(repo.SeparatedContextRequest{
			ControlRoot: filepath.Join(t.TempDir(), "missing"),
			Workspace:   "main",
		})
		require.ErrorIs(t, err, errclass.ErrControlMissing)
	})

	t.Run("malformed control root", func(t *testing.T) {
		controlRoot := filepath.Join(t.TempDir(), "control")
		require.NoError(t, os.MkdirAll(controlRoot, 0755))
		_, err := repo.ResolveSeparatedContext(repo.SeparatedContextRequest{
			ControlRoot: controlRoot,
			Workspace:   "main",
		})
		require.ErrorIs(t, err, errclass.ErrControlMalformed)
	})

	t.Run("missing payload root", func(t *testing.T) {
		base := t.TempDir()
		controlRoot := filepath.Join(base, "control")
		payloadRoot := filepath.Join(base, "payload")
		_, err := repo.InitSeparatedControl(controlRoot, payloadRoot, "main")
		require.NoError(t, err)
		require.NoError(t, os.RemoveAll(payloadRoot))
		_, err = repo.ResolveSeparatedContext(repo.SeparatedContextRequest{
			ControlRoot: controlRoot,
			Workspace:   "main",
		})
		require.ErrorIs(t, err, errclass.ErrPayloadMissing)
	})

	t.Run("workspace mismatch", func(t *testing.T) {
		base := t.TempDir()
		controlRoot := filepath.Join(base, "control")
		payloadRoot := filepath.Join(base, "payload")
		_, err := repo.InitSeparatedControl(controlRoot, payloadRoot, "main")
		require.NoError(t, err)
		_, err = repo.ResolveSeparatedContext(repo.SeparatedContextRequest{
			ControlRoot: controlRoot,
			Workspace:   "other",
		})
		require.ErrorIs(t, err, errclass.ErrWorkspaceMismatch)
	})

	t.Run("repo id mismatch", func(t *testing.T) {
		base := t.TempDir()
		controlRoot := filepath.Join(base, "control")
		payloadRoot := filepath.Join(base, "payload")
		_, err := repo.InitSeparatedControl(controlRoot, payloadRoot, "main")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(controlRoot, ".jvs", repo.RepoIDFile), []byte(""), 0600))
		_, err = repo.ResolveSeparatedContext(repo.SeparatedContextRequest{
			ControlRoot: controlRoot,
			Workspace:   "main",
		})
		require.True(t, errors.Is(err, errclass.ErrRepoIDMismatch) || errors.Is(err, errclass.ErrControlMalformed), "got %v", err)
	})
}
