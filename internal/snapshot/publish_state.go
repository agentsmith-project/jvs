package snapshot

import (
	"github.com/jvs-project/jvs/internal/snapshot/publishstate"
	"github.com/jvs-project/jvs/pkg/errclass"
	"github.com/jvs-project/jvs/pkg/model"
)

const (
	PublishStateCodeSnapshotIDInvalid          = publishstate.CodeSnapshotIDInvalid
	PublishStateCodeDescriptorMissing          = publishstate.CodeDescriptorMissing
	PublishStateCodeDescriptorCorrupt          = publishstate.CodeDescriptorCorrupt
	PublishStateCodeDescriptorChecksumMismatch = publishstate.CodeDescriptorChecksumMismatch
	PublishStateCodeReadyMissing               = publishstate.CodeReadyMissing
	PublishStateCodeReadyInvalid               = publishstate.CodeReadyInvalid
	PublishStateCodeReadyDescriptorMissing     = publishstate.CodeReadyDescriptorMissing
	PublishStateCodePayloadMissing             = publishstate.CodePayloadMissing
	PublishStateCodePayloadInvalid             = publishstate.CodePayloadInvalid
	PublishStateCodePayloadHashMismatch        = publishstate.CodePayloadHashMismatch
)

type PublishStateOptions = publishstate.Options
type PublishState = publishstate.State
type PublishStateIssue = publishstate.Issue

// InspectPublishState validates READY/descriptor/payload consistency for one
// checkpoint. Expected damage is returned as a classified issue instead of an
// unstructured error so callers can fail closed with stable codes.
func InspectPublishState(repoRoot string, snapshotID model.SnapshotID, opts PublishStateOptions) (*PublishState, *PublishStateIssue) {
	return publishstate.Inspect(repoRoot, snapshotID, opts)
}

// PublishStateIssueError converts a classified publish-state issue into the
// stable public error type used by CLI JSON and package callers.
func PublishStateIssueError(issue *PublishStateIssue) error {
	if issue == nil {
		return nil
	}
	return &errclass.JVSError{Code: issue.Code, Message: issue.Message}
}

// PublishReadyMarkerExists reports whether a snapshot dir has a regular
// publish marker leaf and rejects invalid marker leaves without following
// symlinks.
func PublishReadyMarkerExists(snapshotDir string) (bool, error) {
	return publishstate.ReadyMarkerExists(snapshotDir)
}
