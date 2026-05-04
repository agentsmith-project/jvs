package cli

import (
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/pkg/errclass"
)

const separatedDoctorStrictNotRun = "not_run"

type separatedControlJSONFields struct {
	ControlRoot string `json:"control_root,omitempty"`
}

func separatedControlFields(ctx *repo.SeparatedContext, doctorStrict string) separatedControlJSONFields {
	if ctx == nil {
		return separatedControlJSONFields{}
	}
	return separatedControlJSONFields{
		ControlRoot: ctx.ControlRoot,
	}
}

func outputJSONWithSeparatedControl(data any, ctx *repo.SeparatedContext, doctorStrict string) error {
	if !jsonOutput {
		return nil
	}
	withFields, err := separatedControlJSONData(data, ctx, doctorStrict)
	if err != nil {
		return err
	}
	return outputJSON(withFields)
}

func separatedControlJSONData(data any, ctx *repo.SeparatedContext, doctorStrict string) (any, error) {
	if ctx == nil {
		return data, nil
	}
	fields := map[string]any{
		"control_root": ctx.ControlRoot,
	}
	defaultFields := map[string]any{
		"folder":    ctx.PayloadRoot,
		"workspace": ctx.Workspace,
	}
	return publicJSONDataWithObjectFields(data, fields, defaultFields, "external control root JSON data must be an object"), nil
}

func validateSeparatedPayloadSymlinkBoundary(ctx *repo.SeparatedContext) error {
	if ctx == nil {
		return nil
	}
	_, err := validateSeparatedPayloadSymlinkBoundaryForExpectedRoot(ctx, ctx.PayloadRoot)
	return err
}

func validateAndRefreshSeparatedPayloadBoundary(ctx *cliDiscoveryContext) error {
	if ctx == nil || ctx.Separated == nil {
		return nil
	}
	revalidated, err := validateSeparatedPayloadSymlinkBoundaryForExpectedRoot(ctx.Separated, ctx.Separated.PayloadRoot)
	if err != nil {
		return err
	}
	ctx.Separated = revalidated
	ctx.Repo = revalidated.Repo
	ctx.Workspace = revalidated.Workspace
	recordResolvedTarget(revalidated.ControlRoot, revalidated.Workspace)
	return nil
}

func validateSeparatedPayloadSymlinkBoundaryForExpectedRoot(ctx *repo.SeparatedContext, expectedPayloadRoot string) (*repo.SeparatedContext, error) {
	if ctx == nil {
		return nil, nil
	}
	revalidated, err := revalidateSeparatedContext(ctx, expectedPayloadRoot)
	if err != nil {
		return nil, err
	}
	if err := repo.ValidateSeparatedPayloadSymlinkBoundary(revalidated); err != nil {
		return nil, err
	}
	return revalidated, nil
}

func revalidateSeparatedContext(ctx *repo.SeparatedContext, expectedPayloadRoot string) (*repo.SeparatedContext, error) {
	if ctx == nil {
		return nil, nil
	}
	if ctx.Repo == nil {
		return nil, errclass.ErrRepoIDMismatch.WithMessage("expected repo_id is required")
	}
	return repo.RevalidateSeparatedContext(repo.SeparatedContextRevalidationRequest{
		ControlRoot:         ctx.ControlRoot,
		Workspace:           ctx.Workspace,
		ExpectedRepoID:      ctx.Repo.RepoID,
		ExpectedPayloadRoot: expectedPayloadRoot,
	})
}
