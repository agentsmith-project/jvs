package afscp

import (
	"os"
	"path/filepath"
	"strings"
)

type Selector struct {
	ControlRoot string
	Home        string
}

type ResolvedSelector struct {
	ControlRoot string
	Home        string
}

func ValidateSelector(selector Selector) (ResolvedSelector, error) {
	controlRoot, home, err := cleanSelectorPair(selector)
	if err != nil {
		return ResolvedSelector{}, err
	}

	controlRoot, err = existingDirectory(controlRoot, "control root")
	if err != nil {
		return ResolvedSelector{}, err
	}
	home, err = existingDirectory(home, "home")
	if err != nil {
		return ResolvedSelector{}, err
	}
	if pathsOverlap(controlRoot, home) {
		return ResolvedSelector{}, NewError(ErrorCodeInvalidArgument, "control root and home must be disjoint directories", false)
	}
	if err := validateHomeMetadataBoundary(home); err != nil {
		return ResolvedSelector{}, err
	}

	return ResolvedSelector{
		ControlRoot: controlRoot,
		Home:        home,
	}, nil
}

func ValidateMetadataSelector(selector Selector) (ResolvedSelector, error) {
	controlRoot, home, err := cleanSelectorPair(selector)
	if err != nil {
		return ResolvedSelector{}, err
	}

	controlRoot, err = existingDirectory(controlRoot, "control root")
	if err != nil {
		return ResolvedSelector{}, err
	}
	if pathsOverlap(controlRoot, home) {
		return ResolvedSelector{}, NewError(ErrorCodeInvalidArgument, "control root and home must be disjoint directories", false)
	}
	if err := validateHomeMetadataBoundary(home); err != nil {
		return ResolvedSelector{}, err
	}

	return ResolvedSelector{
		ControlRoot: controlRoot,
		Home:        home,
	}, nil
}

func ValidateNewTargetSelector(source ResolvedSelector, selector Selector) (ResolvedSelector, error) {
	controlRoot, home, err := cleanSelectorPair(selector)
	if err != nil {
		return ResolvedSelector{}, err
	}
	if pathsOverlap(controlRoot, home) ||
		pathsOverlap(source.ControlRoot, controlRoot) ||
		pathsOverlap(source.ControlRoot, home) ||
		pathsOverlap(source.Home, controlRoot) ||
		pathsOverlap(source.Home, home) {
		return ResolvedSelector{}, NewError(ErrorCodeInvalidArgument, "source and target roots must be disjoint", false)
	}
	if err := requireMissingTarget(controlRoot, "target control root"); err != nil {
		return ResolvedSelector{}, err
	}
	if err := requireMissingTarget(home, "target home"); err != nil {
		return ResolvedSelector{}, err
	}
	return ResolvedSelector{ControlRoot: controlRoot, Home: home}, nil
}

func cleanSelectorPair(selector Selector) (string, string, error) {
	controlRoot := strings.TrimSpace(selector.ControlRoot)
	home := strings.TrimSpace(selector.Home)
	if controlRoot == "" || home == "" {
		return "", "", NewError(ErrorCodeInvalidArgument, "direct selector requires --control-root and --home", false)
	}
	cleanControlRoot, err := cleanAbsolutePath(controlRoot, "control root")
	if err != nil {
		return "", "", err
	}
	cleanHome, err := cleanAbsolutePath(home, "home")
	if err != nil {
		return "", "", err
	}
	return cleanControlRoot, cleanHome, nil
}

func cleanAbsolutePath(path, label string) (string, error) {
	cleaned := filepath.Clean(path)
	if !filepath.IsAbs(cleaned) {
		return "", NewError(ErrorCodeInvalidArgument, label+" must be an absolute path", false)
	}
	return cleaned, nil
}

func existingDirectory(path, label string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", NewError(ErrorCodeInvalidArgument, label+" must be an existing directory", false)
	}
	if !info.IsDir() {
		return "", NewError(ErrorCodeInvalidArgument, label+" must be an existing directory", false)
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", NewError(ErrorCodeInvalidArgument, label+" must be an existing directory", false)
	}
	return filepath.Clean(canonical), nil
}

func pathsOverlap(left, right string) bool {
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	return left == right || pathContains(left, right) || pathContains(right, left)
}

func pathContains(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func homeContainsJVSMetadata(home string) bool {
	_, err := os.Lstat(filepath.Join(home, ".jvs"))
	return err == nil || !os.IsNotExist(err)
}

func validateHomeMetadataBoundary(home string) error {
	if homeContainsJVSMetadata(home) {
		return NewError(ErrorCodeInvalidArgument, "home must not contain JVS metadata", false)
	}
	return nil
}

func requireMissingTarget(path, label string) error {
	if _, err := os.Lstat(path); err == nil {
		return NewError(ErrorCodeInvalidArgument, label+" must be missing", false)
	} else if !os.IsNotExist(err) {
		return NewError(ErrorCodeInvalidArgument, label+" cannot be inspected", false)
	}
	return nil
}
