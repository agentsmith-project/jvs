package cli

import (
	"encoding/json"
	"fmt"

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

func applySeparatedControlMapFields(out map[string]any, ctx *repo.SeparatedContext, doctorStrict string) {
	if out == nil || ctx == nil {
		return
	}
	out["control_root"] = ctx.ControlRoot
	if _, ok := out["folder"]; !ok {
		out["folder"] = ctx.PayloadRoot
	}
	if _, ok := out["workspace"]; !ok {
		out["workspace"] = ctx.Workspace
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
	out, err := jsonObjectMap(data)
	if err != nil {
		return nil, err
	}
	applySeparatedControlMapFields(out, ctx, doctorStrict)
	return out, nil
}

func jsonObjectMap(data any) (map[string]any, error) {
	switch value := data.(type) {
	case map[string]any:
		out := make(map[string]any, len(value))
		for key, item := range value {
			out[key] = item
		}
		return out, nil
	case map[string]string:
		out := make(map[string]any, len(value))
		for key, item := range value {
			out[key] = item
		}
		return out, nil
	default:
		raw, err := json.Marshal(data)
		if err != nil {
			return nil, fmt.Errorf("encode external control root JSON data: %w", err)
		}
		var out map[string]any
		if err := json.Unmarshal(raw, &out); err != nil {
			return nil, fmt.Errorf("decode external control root JSON data object: %w", err)
		}
		if out == nil {
			return nil, fmt.Errorf("external control root JSON data must be an object")
		}
		return out, nil
	}
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
