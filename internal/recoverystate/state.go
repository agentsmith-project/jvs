// Package recoverystate classifies external restore/recovery state that can
// block mutations on separated-control repositories.
package recoverystate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/agentsmith-project/jvs/internal/recovery"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/restoreplan"
)

type Kind string

const (
	KindStable                  Kind = "stable"
	KindPendingRestorePreview   Kind = "pending_restore_preview"
	KindStaleRestorePreview     Kind = "stale_restore_preview"
	KindActiveRecovery          Kind = "active_recovery"
	KindCompletedRestoreResidue Kind = "completed_restore_residue"
	KindMalformedBlocking       Kind = "malformed_blocking"
)

const (
	CollectionRestorePlans  = "restore plans"
	CollectionRecoveryPlans = "recovery plans"
)

type State struct {
	Kind           Kind
	PlanID         string
	RecoveryPlanID string
	Collection     string
	Message        string
	NextCommand    string
	Cause          error
}

func (s State) Blocking() bool {
	switch s.Kind {
	case KindPendingRestorePreview, KindStaleRestorePreview, KindActiveRecovery, KindMalformedBlocking:
		return true
	default:
		return false
	}
}

func (s State) RecommendedJVSCommand() string {
	if strings.TrimSpace(s.NextCommand) == "" {
		return ""
	}
	return "jvs " + s.NextCommand
}

func (s State) RecommendedJVSCommandFor(separated *repo.SeparatedContext) string {
	if strings.TrimSpace(s.NextCommand) == "" {
		return ""
	}
	if separated == nil {
		return s.RecommendedJVSCommand()
	}
	workspace := strings.TrimSpace(separated.Workspace)
	if workspace == "" {
		workspace = "main"
	}
	return "jvs --control-root " + shellQuoteArg(separated.ControlRoot) + " --workspace " + shellQuoteArg(workspace) + " " + s.NextCommand
}

func Inspect(repoRoot, workspaceName string, separated *repo.SeparatedContext) (State, error) {
	workspaceName = effectiveWorkspace(workspaceName, separated)
	recoveryPlans, err := recovery.NewManager(repoRoot).List()
	if err != nil {
		return malformedRecoveryState(err), nil
	}

	restoreState, err := inspectRestorePlans(repoRoot, workspaceName, separated, recoveryPlans)
	if err != nil {
		return State{}, err
	}
	if restoreState.Kind == KindMalformedBlocking {
		return restoreState, nil
	}
	if activeState := inspectActiveRecoveryPlans(repoRoot, workspaceName, separated, recoveryPlans); activeState.Kind != "" {
		return activeState, nil
	}
	if restoreState.Blocking() {
		return restoreState, nil
	}
	if restoreState.Kind == KindCompletedRestoreResidue {
		return restoreState, nil
	}
	return State{Kind: KindStable}, nil
}

func inspectActiveRecoveryPlans(repoRoot, workspaceName string, separated *repo.SeparatedContext, recoveryPlans []recovery.Plan) State {
	var active State
	for _, plan := range recoveryPlans {
		if state := ClassifyRecoveryPlanBinding(repoRoot, workspaceName, separated, &plan); state.Kind != "" {
			return state
		}
		if plan.Status != recovery.StatusActive {
			continue
		}
		if separated == nil && workspaceName != "" && plan.Workspace != workspaceName {
			continue
		}
		if active.Kind == "" {
			active = activeRecoveryState(plan.PlanID)
		}
	}
	return active
}

func inspectRestorePlans(repoRoot, workspaceName string, separated *repo.SeparatedContext, recoveryPlans []recovery.Plan) (State, error) {
	restorePlansDir := filepath.Join(repoRoot, repo.JVSDirName, "restore-plans")
	if dirState, exists := inspectRestorePlansDirectory(restorePlansDir); dirState.Kind != "" {
		return dirState, nil
	} else if !exists {
		return State{Kind: KindStable}, nil
	}
	entries, err := os.ReadDir(restorePlansDir)
	if err != nil {
		return malformedRestorePlansDirectory(err.Error()), nil
	}

	var completed State
	var blocking State
	for _, entry := range entries {
		name := entry.Name()
		if entry.Type()&os.ModeSymlink != 0 {
			return malformedRestoreEntry(name, "symlink"), nil
		}
		if entry.IsDir() {
			return malformedRestoreEntry(name, "directory"), nil
		}
		if !strings.HasSuffix(name, ".json") {
			continue
		}

		planID := strings.TrimSuffix(name, ".json")
		plan, err := restoreplan.Load(repoRoot, planID)
		if err != nil {
			return malformedRestorePlan(planID, err), nil
		}
		if !plan.IsRunnable() {
			continue
		}
		if err := validateSeparatedRestorePlanIdentity(workspaceName, separated, plan); err != nil {
			return malformedRestorePlan(planID, err), nil
		}
		if err := validateSeparatedRestorePlanBoundary(repoRoot, workspaceName, separated, plan); err != nil {
			return malformedRestorePlan(planID, err), nil
		}
		recoveryPlanID, malformedResolved := matchingResolvedRecoveryPlanID(repoRoot, workspaceName, separated, recoveryPlans, plan)
		if malformedResolved.Kind != "" {
			return malformedResolved, nil
		}
		if recoveryPlanID != "" {
			completed = completedRestoreResidueState(plan.PlanID, recoveryPlanID)
			continue
		}

		targetKind, err := restorePlanTargetState(repoRoot, workspaceName, plan)
		if err != nil {
			return malformedRestorePlan(planID, err), nil
		}
		switch targetKind {
		case KindPendingRestorePreview:
			if blocking.Kind == "" {
				blocking = pendingRestoreState(plan.PlanID)
			}
		case KindStaleRestorePreview:
			if blocking.Kind == "" {
				blocking = staleRestoreState(plan.PlanID)
			}
		}
	}
	if blocking.Kind != "" {
		return blocking, nil
	}
	if completed.Kind != "" {
		return completed, nil
	}
	return State{Kind: KindStable}, nil
}

func inspectRestorePlansDirectory(path string) (State, bool) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return State{}, false
		}
		return malformedRestorePlansDirectory(err.Error()), true
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return malformedRestorePlansDirectory("symlink"), true
	}
	if !info.IsDir() {
		return malformedRestorePlansDirectory("not a directory"), true
	}
	return State{}, true
}

func restorePlanTargetState(repoRoot, workspaceName string, plan *restoreplan.Plan) (Kind, error) {
	if workspaceName == "" && plan != nil {
		workspaceName = plan.Workspace
	}
	var err error
	switch plan.EffectiveScope() {
	case restoreplan.ScopeWhole:
		err = restoreplan.ValidateTarget(repoRoot, workspaceName, plan)
	case restoreplan.ScopePath:
		err = restoreplan.ValidatePathTarget(repoRoot, workspaceName, plan)
	default:
		return "", fmt.Errorf("restore plan scope is not supported")
	}
	if err == nil {
		return KindPendingRestorePreview, nil
	}
	if restoreplan.IsChangedSincePreview(err) {
		return KindStaleRestorePreview, nil
	}
	return "", err
}

func matchingResolvedRecoveryPlanID(repoRoot, workspaceName string, separated *repo.SeparatedContext, plans []recovery.Plan, restorePlan *restoreplan.Plan) (string, State) {
	for _, plan := range plans {
		if restorePlan == nil || plan.Status != recovery.StatusResolved || plan.RestorePlanID != restorePlan.PlanID {
			continue
		}
		if err := validateResolvedRecoveryMatchesRestorePlan(plan, restorePlan); err != nil {
			return "", malformedRecoveryPlan(plan.PlanID, err)
		}
		if err := validateSeparatedResolvedRecoveryPlanIdentity(workspaceName, separated, &plan); err != nil {
			return "", malformedRecoveryPlan(plan.PlanID, err)
		}
		if err := validateSeparatedRecoveryPlanBoundary(repoRoot, workspaceName, separated, &plan); err != nil {
			return "", malformedRecoveryPlan(plan.PlanID, err)
		}
		return plan.PlanID, State{}
	}
	return "", State{}
}

func validateResolvedRecoveryMatchesRestorePlan(plan recovery.Plan, restorePlan *restoreplan.Plan) error {
	if restorePlan == nil {
		return fmt.Errorf("resolved recovery restore plan identity is missing")
	}
	if plan.Workspace != restorePlan.Workspace {
		return fmt.Errorf("resolved recovery workspace identity mismatch: plan workspace %q, restore plan workspace %q", plan.Workspace, restorePlan.Workspace)
	}
	if filepath.Clean(plan.Folder) != filepath.Clean(restorePlan.Folder) {
		return fmt.Errorf("resolved recovery workspace root identity mismatch: recovery workspace folder %q, restore plan workspace folder %q", plan.Folder, restorePlan.Folder)
	}
	if plan.SourceSavePoint != restorePlan.SourceSavePoint {
		return fmt.Errorf("resolved recovery source save point identity mismatch: recovery source %q, restore plan source %q", plan.SourceSavePoint, restorePlan.SourceSavePoint)
	}
	if plan.Path != restorePlan.Path {
		return fmt.Errorf("resolved recovery path identity mismatch: recovery path %q, restore plan path %q", plan.Path, restorePlan.Path)
	}
	switch restorePlan.EffectiveScope() {
	case restoreplan.ScopeWhole:
		if plan.Operation != recovery.OperationRestore {
			return fmt.Errorf("resolved recovery operation identity mismatch: recovery operation %q, restore plan scope %q", plan.Operation, restorePlan.EffectiveScope())
		}
	case restoreplan.ScopePath:
		if plan.Operation != recovery.OperationRestorePath {
			return fmt.Errorf("resolved recovery operation identity mismatch: recovery operation %q, restore plan scope %q", plan.Operation, restorePlan.EffectiveScope())
		}
	default:
		return fmt.Errorf("restore plan scope is not supported")
	}
	return nil
}

// ClassifyRecoveryPlanBinding validates that a recovery plan belongs to the
// selected external control root/workspace binding before callers expose it.
func ClassifyRecoveryPlanBinding(repoRoot, workspaceName string, separated *repo.SeparatedContext, plan *recovery.Plan) State {
	if separated == nil || plan == nil {
		return State{}
	}
	switch plan.Status {
	case recovery.StatusActive:
		if err := validateSeparatedActiveRecoveryPlanIdentity(workspaceName, separated, plan); err != nil {
			return malformedRecoveryPlan(plan.PlanID, err)
		}
	case recovery.StatusResolved:
		if err := validateSeparatedResolvedRecoveryPlanIdentity(workspaceName, separated, plan); err != nil {
			return malformedRecoveryPlan(plan.PlanID, err)
		}
	default:
		if err := validateSeparatedRecoveryPlanIdentity("recovery", workspaceName, separated, plan); err != nil {
			return malformedRecoveryPlan(plan.PlanID, err)
		}
	}
	if err := validateSeparatedRecoveryPlanBoundary(repoRoot, workspaceName, separated, plan); err != nil {
		return malformedRecoveryPlan(plan.PlanID, err)
	}
	return State{}
}

func validateSeparatedRestorePlanIdentity(workspaceName string, separated *repo.SeparatedContext, plan *restoreplan.Plan) error {
	if separated == nil || plan == nil {
		return nil
	}
	expectedWorkspace := effectiveWorkspace(workspaceName, separated)
	if expectedWorkspace != "" && plan.Workspace != expectedWorkspace {
		return fmt.Errorf("restore plan workspace identity mismatch: plan workspace %q, selected workspace %q", plan.Workspace, expectedWorkspace)
	}
	if separated.Repo != nil && separated.Repo.RepoID != "" && plan.RepoID != separated.Repo.RepoID {
		return fmt.Errorf("restore plan repo identity mismatch: plan repo_id %q, selected repo_id %q", plan.RepoID, separated.Repo.RepoID)
	}
	expectedFolder := strings.TrimSpace(separated.PayloadRoot)
	if expectedFolder != "" && filepath.Clean(plan.Folder) != filepath.Clean(expectedFolder) {
		return fmt.Errorf("restore plan workspace folder identity mismatch: plan workspace folder %q, selected workspace folder %q", plan.Folder, expectedFolder)
	}
	return nil
}

func validateSeparatedRestorePlanBoundary(repoRoot, workspaceName string, separated *repo.SeparatedContext, plan *restoreplan.Plan) error {
	if separated == nil || plan == nil {
		return nil
	}
	return validateSeparatedPlanBoundary(repoRoot, workspaceName, separated, plan.Folder)
}

func validateSeparatedRecoveryPlanBoundary(repoRoot, workspaceName string, separated *repo.SeparatedContext, plan *recovery.Plan) error {
	if separated == nil || plan == nil {
		return nil
	}
	return validateSeparatedPlanBoundary(repoRoot, workspaceName, separated, plan.Folder)
}

func validateSeparatedActiveRecoveryPlanIdentity(workspaceName string, separated *repo.SeparatedContext, plan *recovery.Plan) error {
	return validateSeparatedRecoveryPlanIdentity("active recovery", workspaceName, separated, plan)
}

func validateSeparatedResolvedRecoveryPlanIdentity(workspaceName string, separated *repo.SeparatedContext, plan *recovery.Plan) error {
	return validateSeparatedRecoveryPlanIdentity("resolved recovery", workspaceName, separated, plan)
}

func validateSeparatedRecoveryPlanIdentity(label, workspaceName string, separated *repo.SeparatedContext, plan *recovery.Plan) error {
	if separated == nil || plan == nil {
		return nil
	}
	expectedWorkspace := effectiveWorkspace(workspaceName, separated)
	if expectedWorkspace != "" && plan.Workspace != expectedWorkspace {
		return fmt.Errorf("%s workspace identity mismatch: plan workspace %q, selected workspace %q", label, plan.Workspace, expectedWorkspace)
	}
	if separated.Repo != nil && separated.Repo.RepoID != "" && plan.RepoID != separated.Repo.RepoID {
		return fmt.Errorf("%s repo identity mismatch: plan repo_id %q, selected repo_id %q", label, plan.RepoID, separated.Repo.RepoID)
	}
	if strings.TrimSpace(plan.PreWorktreeState.Name) == "" {
		return fmt.Errorf("%s workspace name identity is missing", label)
	}
	if plan.PreWorktreeState.Name != plan.Workspace {
		return fmt.Errorf("%s workspace name identity mismatch: pre-recovery workspace name %q, plan workspace %q", label, plan.PreWorktreeState.Name, plan.Workspace)
	}
	if strings.TrimSpace(plan.PreWorktreeState.RealPath) == "" {
		return fmt.Errorf("%s workspace root identity is missing", label)
	}
	if filepath.Clean(plan.PreWorktreeState.RealPath) != filepath.Clean(plan.Folder) {
		return fmt.Errorf("%s workspace root identity mismatch: pre-recovery workspace folder %q, plan workspace folder %q", label, plan.PreWorktreeState.RealPath, plan.Folder)
	}
	return nil
}

func validateSeparatedPlanBoundary(repoRoot, workspaceName string, separated *repo.SeparatedContext, expectedFolder string) error {
	expectedRepoID := ""
	if separated.Repo != nil {
		expectedRepoID = separated.Repo.RepoID
	} else {
		r, err := repo.OpenControlRoot(repoRoot)
		if err != nil {
			return err
		}
		expectedRepoID = r.RepoID
	}
	revalidated, err := repo.RevalidateSeparatedContext(repo.SeparatedContextRevalidationRequest{
		ControlRoot:         separated.ControlRoot,
		Workspace:           effectiveWorkspace(workspaceName, separated),
		ExpectedRepoID:      expectedRepoID,
		ExpectedPayloadRoot: expectedFolder,
	})
	if err != nil {
		return err
	}
	return repo.ValidateSeparatedPayloadSymlinkBoundary(revalidated)
}

func effectiveWorkspace(workspaceName string, separated *repo.SeparatedContext) string {
	if strings.TrimSpace(workspaceName) != "" {
		return workspaceName
	}
	if separated != nil {
		return separated.Workspace
	}
	return ""
}

func pendingRestoreState(planID string) State {
	command := "restore --run " + planID
	return State{
		Kind:        KindPendingRestorePreview,
		PlanID:      planID,
		Collection:  CollectionRestorePlans,
		Message:     "Restore plan " + planID + " is pending.",
		NextCommand: command,
	}
}

func staleRestoreState(planID string) State {
	command := "restore discard " + planID
	return State{
		Kind:        KindStaleRestorePreview,
		PlanID:      planID,
		Collection:  CollectionRestorePlans,
		Message:     "Restore plan " + planID + " is stale because the workspace folder changed after preview.",
		NextCommand: command,
	}
}

func activeRecoveryState(planID string) State {
	command := "recovery status " + planID
	return State{
		Kind:           KindActiveRecovery,
		RecoveryPlanID: planID,
		Collection:     CollectionRecoveryPlans,
		Message:        "Recovery plan " + planID + " is active.",
		NextCommand:    command,
	}
}

func completedRestoreResidueState(planID, recoveryPlanID string) State {
	command := "recovery status " + recoveryPlanID
	return State{
		Kind:           KindCompletedRestoreResidue,
		PlanID:         planID,
		RecoveryPlanID: recoveryPlanID,
		Collection:     CollectionRestorePlans,
		Message:        "Restore plan " + planID + " was already resolved by recovery plan " + recoveryPlanID + ".",
		NextCommand:    command,
	}
}

func malformedRestoreEntry(name, reason string) State {
	return State{
		Kind:        KindMalformedBlocking,
		PlanID:      strings.TrimSuffix(name, ".json"),
		Collection:  CollectionRestorePlans,
		Message:     "Restore plan entry " + name + " is unsafe: " + publicRecoveryVocabulary(reason) + ".",
		NextCommand: "doctor --strict --json",
	}
}

func malformedRestorePlansDirectory(reason string) State {
	return State{
		Kind:        KindMalformedBlocking,
		Collection:  CollectionRestorePlans,
		Message:     "Restore plans directory cannot be inspected safely: " + publicRecoveryVocabulary(reason) + ".",
		NextCommand: "doctor --strict --json",
	}
}

func malformedRestorePlan(planID string, err error) State {
	return State{
		Kind:        KindMalformedBlocking,
		PlanID:      planID,
		Collection:  CollectionRestorePlans,
		Message:     "Restore plan " + planID + " cannot be inspected safely: " + publicRecoveryVocabulary(err.Error()) + ".",
		NextCommand: "doctor --strict --json",
		Cause:       err,
	}
}

func malformedRecoveryPlan(planID string, err error) State {
	return State{
		Kind:           KindMalformedBlocking,
		RecoveryPlanID: planID,
		Collection:     CollectionRecoveryPlans,
		Message:        "Recovery plan " + planID + " cannot be inspected safely: " + publicRecoveryVocabulary(err.Error()) + ".",
		NextCommand:    "doctor --strict --json",
		Cause:          err,
	}
}

func malformedRecoveryState(err error) State {
	return State{
		Kind:        KindMalformedBlocking,
		Collection:  CollectionRecoveryPlans,
		Message:     "Recovery state cannot be inspected safely: " + publicRecoveryVocabulary(err.Error()) + ".",
		NextCommand: "doctor --strict --json",
		Cause:       err,
	}
}

func publicRecoveryVocabulary(value string) string {
	replacer := strings.NewReplacer(
		"workspace registry", "control data",
		"Workspace registry", "Control data",
		"registry has", "control data records",
		"Registry has", "Control data records",
		"payload root", "workspace folder",
		"payload boundary", "workspace boundary",
		"payload symlink", "workspace folder symlink",
		"payload", "workspace folder",
		".jvs/restore-plans", "restore plans",
		".jvs/recovery-plans", "recovery plans",
		".jvs", "control data",
		"control leaf", "control data entry",
		"control directory", "control data directory",
		"control path", "control data path",
		"worktrees", "workspaces",
		"worktree", "workspace",
		"snapshots", "save points",
		"snapshot", "save point",
		"gc", "cleanup",
		"internal", "JVS",
		"HEAD", "source",
		"head", "source",
	)
	return replacer.Replace(value)
}

func shellQuoteArg(arg string) string {
	if arg == "" {
		return "''"
	}
	for _, r := range arg {
		if isShellSafeRune(r) {
			continue
		}
		return "'" + strings.ReplaceAll(arg, "'", "'\\''") + "'"
	}
	return arg
}

func isShellSafeRune(r rune) bool {
	if r >= 'a' && r <= 'z' {
		return true
	}
	if r >= 'A' && r <= 'Z' {
		return true
	}
	if r >= '0' && r <= '9' {
		return true
	}
	switch r {
	case '_', '-', '.', '/', ':', '@', '%', '+', '=':
		return true
	default:
		return false
	}
}
