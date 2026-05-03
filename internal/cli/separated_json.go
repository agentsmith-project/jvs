package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/pkg/errclass"
)

const separatedDoctorStrictNotRun = "not_run"

type separatedControlJSONFields struct {
	ControlRoot          string `json:"control_root,omitempty"`
	PayloadRoot          string `json:"payload_root,omitempty"`
	RepoMode             string `json:"repo_mode,omitempty"`
	WorkspaceName        string `json:"workspace_name,omitempty"`
	SeparatedControl     *bool  `json:"separated_control,omitempty"`
	BoundaryValidated    *bool  `json:"boundary_validated,omitempty"`
	LocatorAuthoritative *bool  `json:"locator_authoritative,omitempty"`
	DoctorStrict         string `json:"doctor_strict,omitempty"`
}

func separatedControlFields(ctx *repo.SeparatedContext, doctorStrict string) separatedControlJSONFields {
	if ctx == nil {
		return separatedControlJSONFields{}
	}
	return separatedControlJSONFields{
		ControlRoot:          ctx.ControlRoot,
		PayloadRoot:          ctx.PayloadRoot,
		RepoMode:             ctx.Repo.Mode,
		WorkspaceName:        ctx.Workspace,
		SeparatedControl:     boolPtr(ctx.Repo.Mode == repo.RepoModeSeparatedControl),
		BoundaryValidated:    boolPtr(ctx.BoundaryValidated),
		LocatorAuthoritative: boolPtr(ctx.LocatorAuthoritative),
		DoctorStrict:         doctorStrict,
	}
}

func applySeparatedControlMapFields(out map[string]any, ctx *repo.SeparatedContext, doctorStrict string) {
	if out == nil || ctx == nil {
		return
	}
	out["control_root"] = ctx.ControlRoot
	out["payload_root"] = ctx.PayloadRoot
	out["repo_mode"] = ctx.Repo.Mode
	out["workspace_name"] = ctx.Workspace
	out["separated_control"] = ctx.Repo.Mode == repo.RepoModeSeparatedControl
	out["boundary_validated"] = ctx.BoundaryValidated
	out["locator_authoritative"] = ctx.LocatorAuthoritative
	out["doctor_strict"] = doctorStrict
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
			return nil, fmt.Errorf("encode separated JSON data: %w", err)
		}
		var out map[string]any
		if err := json.Unmarshal(raw, &out); err != nil {
			return nil, fmt.Errorf("decode separated JSON data object: %w", err)
		}
		if out == nil {
			return nil, fmt.Errorf("separated JSON data must be an object")
		}
		return out, nil
	}
}

func validateSeparatedPayloadSymlinkBoundary(ctx *repo.SeparatedContext) error {
	if ctx == nil {
		return nil
	}
	rootReal, err := filepath.EvalSymlinks(ctx.PayloadRoot)
	if err != nil {
		return errclass.ErrPathBoundaryEscape.WithMessagef("resolve payload root boundary: %v", err)
	}
	rootReal = filepath.Clean(rootReal)
	return filepath.WalkDir(ctx.PayloadRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return errclass.ErrPathBoundaryEscape.WithMessagef("walk payload boundary: %v", walkErr)
		}
		if path == ctx.PayloadRoot {
			return nil
		}
		if entry.Type()&os.ModeSymlink == 0 {
			return nil
		}
		targetReal, err := filepath.EvalSymlinks(path)
		rel, relErr := filepath.Rel(ctx.PayloadRoot, path)
		if relErr != nil {
			rel = path
		}
		rel = filepath.ToSlash(rel)
		if err != nil {
			return errclass.ErrPathBoundaryEscape.WithMessagef("payload symlink cannot be resolved safely: %s: %v", rel, err)
		}
		targetReal = filepath.Clean(targetReal)
		if !pathWithinRoot(rootReal, targetReal) {
			return errclass.ErrPathBoundaryEscape.WithMessagef("payload symlink escapes workspace boundary: %s", rel)
		}
		return nil
	})
}

func pathWithinRoot(root, path string) bool {
	if root == "" || path == "" {
		return false
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func boolPtr(value bool) *bool {
	return &value
}
