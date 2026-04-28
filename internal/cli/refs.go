package cli

import "github.com/agentsmith-project/jvs/pkg/errclass"

var reservedWorkspaceNames = map[string]struct{}{
	"current": {},
	"latest":  {},
	"dirty":   {},
}

func validatePublicWorkspaceName(name string) error {
	if _, ok := reservedWorkspaceNames[name]; ok {
		return errclass.ErrNameInvalid.WithMessagef("%q is reserved and cannot be used as a workspace name", name)
	}
	return nil
}
