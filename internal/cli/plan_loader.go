package cli

import (
	"encoding/json"
	"fmt"
	"os"
)

type repoScopedPlan interface {
	repoScopedPlanMetadata() repoScopedPlanMetadata
}

type repoScopedPlanMetadata struct {
	schemaVersion int
	planID        string
	repoID        string
}

type repoScopedPlanLoadOptions struct {
	name          string
	schemaVersion int
	path          func(root, planID string, missingOK bool) (string, error)
	validate      func() error
}

func loadRepoScopedPlan(root, planID string, plan repoScopedPlan, opts repoScopedPlanLoadOptions) error {
	path, err := opts.path(root, planID, false)
	if err != nil {
		return repoScopedPlanReadError(opts.name, planID, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return repoScopedPlanReadError(opts.name, planID, err)
	}
	if err := json.Unmarshal(data, plan); err != nil {
		return fmt.Errorf("%s %q is not valid JSON", opts.name, planID)
	}
	meta := plan.repoScopedPlanMetadata()
	if meta.schemaVersion != opts.schemaVersion {
		return fmt.Errorf("%s %q has unsupported schema version", opts.name, planID)
	}
	if meta.planID != planID {
		return fmt.Errorf("%s %q plan_id does not match request", opts.name, planID)
	}
	if opts.validate != nil {
		if err := opts.validate(); err != nil {
			return err
		}
	}
	repoID, err := workspaceCurrentRepoID(root)
	if err != nil {
		return err
	}
	if meta.repoID != repoID {
		return fmt.Errorf("%s %q belongs to a different repository", opts.name, planID)
	}
	return nil
}

func repoScopedPlanReadError(name, planID string, err error) error {
	if os.IsNotExist(err) {
		return fmt.Errorf("%s %q not found", name, planID)
	}
	return fmt.Errorf("%s %q is not readable", name, planID)
}

func (plan *repoMovePlan) repoScopedPlanMetadata() repoScopedPlanMetadata {
	return repoScopedPlanMetadata{
		schemaVersion: plan.SchemaVersion,
		planID:        plan.PlanID,
		repoID:        plan.RepoID,
	}
}

func (plan *repoDetachPlan) repoScopedPlanMetadata() repoScopedPlanMetadata {
	return repoScopedPlanMetadata{
		schemaVersion: plan.SchemaVersion,
		planID:        plan.PlanID,
		repoID:        plan.RepoID,
	}
}

func (plan *workspaceMovePlan) repoScopedPlanMetadata() repoScopedPlanMetadata {
	return repoScopedPlanMetadata{
		schemaVersion: plan.SchemaVersion,
		planID:        plan.PlanID,
		repoID:        plan.RepoID,
	}
}

func (plan *workspaceDeletePlan) repoScopedPlanMetadata() repoScopedPlanMetadata {
	return repoScopedPlanMetadata{
		schemaVersion: plan.SchemaVersion,
		planID:        plan.PlanID,
		repoID:        plan.RepoID,
	}
}
