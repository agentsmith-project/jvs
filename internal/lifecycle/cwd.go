package lifecycle

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/agentsmith-project/jvs/pkg/errclass"
)

// CWDSafetyRequest describes a mutation that would move or delete AffectedRoot.
type CWDSafetyRequest struct {
	CWD             string
	AffectedRoot    string
	SafeNextCommand string
}

// UnsafeCWDError carries the user-facing safe command while remaining
// comparable with errclass.ErrLifecycleUnsafeCWD.
type UnsafeCWDError struct {
	CWD             string
	AffectedRoot    string
	SafeNextCommand string
	Cause           error
}

func (e *UnsafeCWDError) Error() string {
	return e.jvsError().Error()
}

func (e *UnsafeCWDError) Is(target error) bool {
	return errors.Is(e.jvsError(), target)
}

func (e *UnsafeCWDError) As(target any) bool {
	targetJVS, ok := target.(**errclass.JVSError)
	if !ok {
		return false
	}
	*targetJVS = e.jvsError()
	return true
}

func (e *UnsafeCWDError) jvsError() *errclass.JVSError {
	message := "JVS will not move/delete the folder you are currently standing in. No files were changed."
	if e.SafeNextCommand != "" {
		message += " Safe next command: " + e.SafeNextCommand
	}
	if e.Cause != nil {
		message += " Safety check: " + e.Cause.Error()
	}
	return errclass.ErrLifecycleUnsafeCWD.WithMessage(message)
}

// CheckCWDOutsideAffectedTree fails closed if cwd is lexically or physically
// inside a tree that will be moved or deleted.
func CheckCWDOutsideAffectedTree(req CWDSafetyRequest) error {
	cwd := req.CWD
	if strings.TrimSpace(cwd) == "" {
		current, err := os.Getwd()
		if err != nil {
			return unsafeCWDError(req, fmt.Errorf("read current directory: %w", err))
		}
		cwd = current
	}
	req.CWD = cwd

	affectedLexical, err := cleanAbsPath(req.AffectedRoot)
	if err != nil {
		return unsafeCWDError(req, fmt.Errorf("resolve affected folder: %w", err))
	}
	cwdLexical, err := cleanAbsPath(cwd)
	if err != nil {
		return unsafeCWDError(req, fmt.Errorf("resolve current folder: %w", err))
	}
	inside, err := cleanAbsPathContains(affectedLexical, cwdLexical)
	if err != nil {
		return unsafeCWDError(req, fmt.Errorf("compare lexical paths: %w", err))
	}
	if inside {
		return unsafeCWDError(req, nil)
	}

	affectedPhysical, err := existingPhysicalPath(req.AffectedRoot)
	if err != nil {
		return unsafeCWDError(req, fmt.Errorf("resolve affected folder physically: %w", err))
	}
	cwdPhysical, err := existingPhysicalPath(cwd)
	if err != nil {
		return unsafeCWDError(req, fmt.Errorf("resolve current folder physically: %w", err))
	}
	inside, err = cleanAbsPathContains(affectedPhysical, cwdPhysical)
	if err != nil {
		return unsafeCWDError(req, fmt.Errorf("compare physical paths: %w", err))
	}
	if inside {
		return unsafeCWDError(req, nil)
	}
	return nil
}

func unsafeCWDError(req CWDSafetyRequest, cause error) error {
	return &UnsafeCWDError{
		CWD:             req.CWD,
		AffectedRoot:    req.AffectedRoot,
		SafeNextCommand: req.SafeNextCommand,
		Cause:           cause,
	}
}

func cleanAbsPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("path is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func existingPhysicalPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	physical, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	return filepath.Clean(physical), nil
}

func cleanAbsPathContains(baseAbs, pathAbs string) (bool, error) {
	rel, err := filepath.Rel(baseAbs, pathAbs)
	if err != nil {
		return false, err
	}
	return relPathContained(rel), nil
}

func relPathContained(rel string) bool {
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && !filepath.IsAbs(rel))
}
