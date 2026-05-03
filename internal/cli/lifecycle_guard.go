package cli

import (
	"fmt"
	"strings"

	"github.com/agentsmith-project/jvs/internal/lifecycle"
	"github.com/agentsmith-project/jvs/pkg/errclass"
)

type lifecyclePendingCLIError struct {
	err                    *errclass.JVSError
	recommendedNextCommand string
}

func (e *lifecyclePendingCLIError) Error() string {
	return e.err.Error()
}

func (e *lifecyclePendingCLIError) Unwrap() error {
	return e.err
}

func (e *lifecyclePendingCLIError) RecommendedNextCommand() string {
	return e.recommendedNextCommand
}

func enforceLifecyclePendingGuard(repoRoot string) error {
	if lifecyclePendingGuardSkipsCommand(activeCommandName) {
		return nil
	}

	pending, err := lifecycle.ListPendingOperations(repoRoot)
	if err != nil {
		return errclass.ErrLifecyclePending.
			WithMessagef("cannot inspect pending lifecycle operations: %v", err).
			WithHint("Run jvs doctor --strict to inspect lifecycle operations.")
	}
	if len(pending) == 0 {
		return nil
	}
	if lifecyclePendingInvocationIsRecommended(pending) {
		return nil
	}
	return lifecyclePendingBlockedCommandError(pending[0])
}

func lifecyclePendingGuardSkipsCommand(command string) bool {
	return command == "doctor" || strings.HasPrefix(command, "doctor ")
}

func lifecyclePendingInvocationIsRecommended(pending []lifecycle.OperationRecord) bool {
	invocations := lifecyclePendingRecommendedCommandCandidates()
	if len(invocations) == 0 {
		return false
	}
	for _, record := range pending {
		recommended := strings.TrimSpace(record.RecommendedNextCommand)
		for _, invocation := range invocations {
			if recommended == invocation.command {
				return true
			}
			if invocation.operationType != "" && invocation.planID != "" &&
				record.OperationType == invocation.operationType &&
				(record.OperationID == invocation.planID || lifecyclePendingMetadataString(record, "plan_id") == invocation.planID) {
				return true
			}
		}
	}
	return false
}

type lifecyclePendingRecommendedInvocation struct {
	command       string
	operationType string
	planID        string
}

func lifecyclePendingRecommendedCommandCandidates() []lifecyclePendingRecommendedInvocation {
	switch activeCommandName {
	case "repo move":
		return lifecycleRunCommandCandidate("jvs repo move --run", "repo move", repoMoveRunID)
	case "repo rename":
		return lifecycleRunCommandCandidate("jvs repo rename --run", "repo rename", repoRenameRunID)
	case "repo detach":
		return lifecycleRunCommandCandidate("jvs repo detach --run", "repo detach", repoDetachRunID)
	case "workspace delete":
		return lifecycleRunCommandCandidate("jvs workspace delete --run", "workspace delete", workspaceDeleteRunID)
	case "workspace move":
		return lifecycleRunCommandCandidate("jvs workspace move --run", "workspace move", workspaceMoveRunID)
	case "workspace rename":
		if workspaceRenameDryRun || len(activeCommandArgs) != 2 {
			return nil
		}
		return []lifecyclePendingRecommendedInvocation{{
			command:       "jvs workspace rename " + activeCommandArgs[0] + " " + activeCommandArgs[1],
			operationType: "workspace rename",
		}}
	default:
		return nil
	}
}

func lifecycleRunCommandCandidate(prefix, operationType, planID string) []lifecyclePendingRecommendedInvocation {
	planID = strings.TrimSpace(planID)
	if planID == "" && activeCommand != nil {
		if flag := activeCommand.Flags().Lookup("run"); flag != nil && flag.Changed {
			planID = strings.TrimSpace(flag.Value.String())
		}
	}
	if planID == "" {
		return nil
	}
	return []lifecyclePendingRecommendedInvocation{{
		command:       prefix + " " + planID,
		operationType: operationType,
		planID:        planID,
	}}
}

func lifecyclePendingMetadataString(record lifecycle.OperationRecord, key string) string {
	value, _ := record.Metadata[key].(string)
	return value
}

func lifecyclePendingBlockedCommandError(record lifecycle.OperationRecord) error {
	message := fmt.Sprintf(
		"pending lifecycle operation %s (%s) is in phase %s. This command is blocked until the operation is resumed. No files or history were changed.",
		record.OperationID,
		record.OperationType,
		record.Phase,
	)
	nextCommand := strings.TrimSpace(record.RecommendedNextCommand)
	jvsErr := errclass.ErrLifecyclePending.WithMessage(message)
	if nextCommand != "" {
		jvsErr = jvsErr.WithHint("Recommended next command: " + nextCommand)
	}
	return &lifecyclePendingCLIError{
		err:                    jvsErr,
		recommendedNextCommand: nextCommand,
	}
}
