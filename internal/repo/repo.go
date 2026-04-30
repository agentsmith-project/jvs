// Package repo handles JVS repository initialization and discovery.
package repo

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agentsmith-project/jvs/pkg/errclass"
	"github.com/agentsmith-project/jvs/pkg/fsutil"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/agentsmith-project/jvs/pkg/pathutil"
	"github.com/agentsmith-project/jvs/pkg/uuidutil"
)

const (
	// FormatVersion is the current repository format version.
	FormatVersion = 1
	// JVSDirName is the name of the JVS metadata directory.
	JVSDirName = ".jvs"
	// FormatVersionFile is the name of the file storing the format version.
	FormatVersionFile = "format_version"
	// RepoIDFile is the name of the file storing the repository ID.
	RepoIDFile = "repo_id"
)

const workspaceLocatorType = "jvs-workspace"

// Repo represents an initialized JVS repository.
type Repo struct {
	Root          string
	FormatVersion int
	RepoID        string
}

type workspaceLocatorFile struct {
	Type          string `json:"type"`
	FormatVersion int    `json:"format_version"`
	RepoRoot      string `json:"repo_root"`
	RepoID        string `json:"repo_id"`
	WorkspaceName string `json:"workspace_name"`
}

var (
	// ErrControlRepoNotFound marks a control-repo walk that reached the filesystem root.
	ErrControlRepoNotFound     = errors.New("control repository not found")
	errWorktreeRegistryMissing = errors.New("worktree registry missing")
)

// WorktreePayloadBoundary describes the managed portion of a worktree payload.
type WorktreePayloadBoundary struct {
	Root              string
	ExcludedRootNames []string
}

// ExcludesRelativePath reports whether rel is outside the managed payload
// because it is reserved for repository control data.
func (b WorktreePayloadBoundary) ExcludesRelativePath(rel string) bool {
	clean := filepath.Clean(rel)
	if clean == "." {
		return false
	}
	slashClean := filepath.ToSlash(clean)
	for _, name := range b.ExcludedRootNames {
		slashName := filepath.ToSlash(filepath.Clean(name))
		if slashClean == slashName || strings.HasPrefix(slashClean, slashName+"/") {
			return true
		}
	}
	return false
}

// ValidateManagedPayloadOnly verifies that a materialized payload source does
// not contain root-level control data excluded from the managed workspace.
func ValidateManagedPayloadOnly(boundary WorktreePayloadBoundary, payloadRoot string) error {
	for _, name := range boundary.ExcludedRootNames {
		if _, err := os.Lstat(filepath.Join(payloadRoot, name)); err == nil {
			return fmt.Errorf("source payload contains repo control data and is not managed: %s", name)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("stat source control path %s: %w", name, err)
		}
	}
	return nil
}

// ValidateInitTarget returns the absolute target path after enforcing the
// repository creation rules: the target must be missing or empty, must not
// already contain .jvs metadata, and must not be lexically or physically nested
// inside a JVS repo.
func ValidateInitTarget(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("repository path must not be empty")
	}

	target, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve repository path: %w", err)
	}
	target = filepath.Clean(target)

	if err := ensureTargetEmptyOrMissing(target); err != nil {
		return "", err
	}
	if err := rejectNestedInitTarget(target); err != nil {
		return "", err
	}
	physicalTarget, err := resolvePhysicalInitTarget(target)
	if err != nil {
		return "", fmt.Errorf("resolve repository target: %w", err)
	}
	if physicalTarget != target {
		if err := rejectNestedInitTarget(physicalTarget); err != nil {
			return "", fmt.Errorf("%w (physical target: %s)", err, physicalTarget)
		}
	}
	return target, nil
}

// InitTarget creates a new repository at an absolute or relative target path.
func InitTarget(path string) (*Repo, error) {
	target, err := ValidateInitTarget(path)
	if err != nil {
		return nil, err
	}
	return initAt(target)
}

// Init creates a new JVS repository at the specified path.
func Init(path string, name string) (*Repo, error) {
	if err := pathutil.ValidateName(name); err != nil {
		return nil, err
	}
	target, err := ValidateInitTarget(path)
	if err != nil {
		return nil, err
	}
	return initAt(target)
}

// InitAdoptedWorkspace initializes JVS control data inside an existing folder
// and registers that folder itself as the main workspace payload.
func InitAdoptedWorkspace(folder string) (*Repo, error) {
	target, err := validateAdoptedWorkspaceTarget(folder)
	if err != nil {
		return nil, err
	}
	return initAdoptedAt(target)
}

func initAt(path string) (*Repo, error) {
	repoID, err := createControlPlane(path)
	if err != nil {
		return nil, err
	}

	// Create main/ payload directory
	mainDir := filepath.Join(path, "main")
	if err := os.MkdirAll(mainDir, 0755); err != nil {
		return nil, fmt.Errorf("create main directory: %w", err)
	}

	// Create worktrees/ payload directory
	worktreesPayload := filepath.Join(path, "worktrees")
	if err := os.MkdirAll(worktreesPayload, 0755); err != nil {
		return nil, fmt.Errorf("create worktrees directory: %w", err)
	}

	// Write main worktree config
	cfg := &model.WorktreeConfig{
		Name:      "main",
		CreatedAt: time.Now().UTC(),
	}
	if err := WriteWorktreeConfig(path, "main", cfg); err != nil {
		return nil, fmt.Errorf("write main config: %w", err)
	}

	// Fsync parent to ensure durability
	if err := fsutil.FsyncDir(path); err != nil {
		return nil, fmt.Errorf("fsync repo root: %w", err)
	}

	return &Repo{
		Root:          path,
		FormatVersion: FormatVersion,
		RepoID:        repoID,
	}, nil
}

func initAdoptedAt(path string) (*Repo, error) {
	repoID, err := createControlPlane(path)
	if err != nil {
		return nil, err
	}

	cfg := &model.WorktreeConfig{
		Name:      "main",
		RealPath:  path,
		CreatedAt: time.Now().UTC(),
	}
	if err := WriteWorktreeConfig(path, "main", cfg); err != nil {
		return nil, fmt.Errorf("write main config: %w", err)
	}

	if err := fsutil.FsyncDir(path); err != nil {
		return nil, fmt.Errorf("fsync repo root: %w", err)
	}

	return &Repo{
		Root:          path,
		FormatVersion: FormatVersion,
		RepoID:        repoID,
	}, nil
}

func createControlPlane(path string) (string, error) {
	jvsDir := filepath.Join(path, JVSDirName)
	dirs := []string{
		jvsDir,
		filepath.Join(jvsDir, "worktrees", "main"),
		filepath.Join(jvsDir, "snapshots"),
		filepath.Join(jvsDir, "descriptors"),
		filepath.Join(jvsDir, "intents"),
		filepath.Join(jvsDir, "audit"),
		filepath.Join(jvsDir, "locks"),
		filepath.Join(jvsDir, "restore-plans"),
		filepath.Join(jvsDir, "recovery-plans"),
		filepath.Join(jvsDir, "gc"),
		filepath.Join(jvsDir, "gc", "pins"),
		filepath.Join(jvsDir, "gc", "tombstones"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", fmt.Errorf("create directory %s: %w", dir, err)
		}
	}

	if err := os.WriteFile(filepath.Join(jvsDir, FormatVersionFile), []byte("1\n"), 0600); err != nil {
		return "", fmt.Errorf("write format_version: %w", err)
	}

	repoID := uuidutil.NewV4()
	if err := os.WriteFile(filepath.Join(jvsDir, RepoIDFile), []byte(repoID+"\n"), 0600); err != nil {
		return "", fmt.Errorf("write repo_id: %w", err)
	}
	return repoID, nil
}

func validateAdoptedWorkspaceTarget(folder string) (string, error) {
	if strings.TrimSpace(folder) == "" {
		return "", fmt.Errorf("workspace folder must not be empty")
	}

	input, err := filepath.Abs(folder)
	if err != nil {
		return "", fmt.Errorf("resolve workspace folder: %w", err)
	}
	input = filepath.Clean(input)
	if err := rejectExistingMetadataAt(input); err != nil {
		return "", err
	}
	if err := rejectNestedInitTarget(input); err != nil {
		return "", err
	}

	target, err := existingPhysicalPath(input)
	if err != nil {
		return "", fmt.Errorf("resolve workspace folder: %w", err)
	}
	info, err := os.Stat(target)
	if err != nil {
		return "", fmt.Errorf("stat workspace folder: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("workspace folder exists and is not a directory: %s", target)
	}
	if target != input {
		if err := rejectExistingMetadataAt(target); err != nil {
			return "", fmt.Errorf("%w (physical target: %s)", err, target)
		}
		if err := rejectNestedInitTarget(target); err != nil {
			return "", fmt.Errorf("%w (physical target: %s)", err, target)
		}
	}
	return target, nil
}

func ensureTargetEmptyOrMissing(target string) error {
	info, err := os.Lstat(target)
	if err != nil {
		if os.IsNotExist(err) {
			if err := rejectExistingMetadataAt(target); err != nil {
				return err
			}
			return nil
		}
		return fmt.Errorf("stat repository target: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("repository target must not be a symlink: %s", target)
	}
	if !info.IsDir() {
		return fmt.Errorf("repository target exists and is not a directory: %s", target)
	}
	if err := rejectExistingMetadataAt(target); err != nil {
		return err
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		return fmt.Errorf("read repository target: %w", err)
	}
	if len(entries) > 0 {
		return fmt.Errorf("repository target must be empty or not exist: %s", target)
	}
	return nil
}

func rejectExistingMetadataAt(target string) error {
	jvsDir := filepath.Join(target, JVSDirName)
	info, err := os.Stat(jvsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat existing metadata: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("repository target already contains %s: %s", JVSDirName, target)
	}
	return fmt.Errorf("repository target contains reserved %s path: %s", JVSDirName, target)
}

func rejectNestedInitTarget(target string) error {
	parent := filepath.Dir(target)
	for {
		if info, err := os.Stat(filepath.Join(parent, JVSDirName)); err == nil && info.IsDir() {
			return fmt.Errorf("cannot initialize nested repository inside existing JVS repository: %s", target)
		}
		next := filepath.Dir(parent)
		if next == parent {
			return nil
		}
		parent = next
	}
}

func resolvePhysicalInitTarget(abs string) (string, error) {
	resolved, err := filepath.EvalSymlinks(abs)
	if err == nil {
		return filepath.Clean(resolved), nil
	}
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("resolve symlinks for %s: %w", abs, err)
	}

	ancestor := abs
	var suffix []string
	for {
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			return "", fmt.Errorf("no existing ancestor for %s", abs)
		}
		suffix = append([]string{filepath.Base(ancestor)}, suffix...)
		ancestor = parent

		resolvedAncestor, err := filepath.EvalSymlinks(ancestor)
		if err == nil {
			info, statErr := os.Stat(resolvedAncestor)
			if statErr != nil {
				return "", fmt.Errorf("stat resolved ancestor %s: %w", resolvedAncestor, statErr)
			}
			if !info.IsDir() {
				return "", fmt.Errorf("repository target parent is not a directory: %s", ancestor)
			}
			parts := append([]string{filepath.Clean(resolvedAncestor)}, suffix...)
			return filepath.Clean(filepath.Join(parts...)), nil
		}
		if err != nil && !os.IsNotExist(err) {
			return "", fmt.Errorf("resolve symlinks for existing ancestor %s: %w", ancestor, err)
		}
	}
}

// Discover walks up from cwd to find the repo root (directory containing .jvs/).
func Discover(cwd string) (*Repo, error) {
	discovered, err := discoverRepoEvidence(cwd)
	if err != nil {
		return nil, err
	}
	return discovered.repo, nil
}

type repoDiscoveryEvidence struct {
	repo        *Repo
	locator     *workspaceLocatorFile
	locatorPath string
}

func discoverRepoEvidence(cwd string) (*repoDiscoveryEvidence, error) {
	path, err := filepath.Abs(cwd)
	if err != nil {
		return nil, err
	}
	path = filepath.Clean(path)
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		path = filepath.Dir(path)
	}

	for {
		jvsDir := filepath.Join(path, JVSDirName)
		if info, err := os.Stat(jvsDir); err == nil && info.IsDir() {
			r, err := loadRepoAtRoot(path)
			if err != nil {
				return nil, err
			}
			return &repoDiscoveryEvidence{repo: r}, nil
		}
		if r, locator, ok, err := discoverWorkspaceLocator(jvsDir); ok || err != nil {
			if err != nil {
				return nil, err
			}
			return &repoDiscoveryEvidence{repo: r, locator: &locator, locatorPath: jvsDir}, nil
		}

		parent := filepath.Dir(path)
		if parent == path {
			return nil, fmt.Errorf("%w: no JVS repository found (no .jvs/ in parent directories)", ErrControlRepoNotFound)
		}
		path = parent
	}
}

// DiscoverControlRepo walks up from cwd looking only for repository control
// directories. It intentionally ignores workspace locator files so callers can
// detect the physical ancestor repository even when a child locator is forged or
// malformed.
func DiscoverControlRepo(cwd string) (*Repo, error) {
	path, err := filepath.Abs(cwd)
	if err != nil {
		return nil, err
	}
	path = filepath.Clean(path)
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		path = filepath.Dir(path)
	}

	for {
		jvsDir := filepath.Join(path, JVSDirName)
		info, err := os.Stat(jvsDir)
		if err == nil {
			if info.IsDir() {
				return loadRepoAtRoot(path)
			}
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("stat JVS control directory %s: %w", jvsDir, err)
		}

		parent := filepath.Dir(path)
		if parent == path {
			return nil, fmt.Errorf("%w: no JVS repository found (no .jvs/ in parent directories)", ErrControlRepoNotFound)
		}
		path = parent
	}
}

// ValidateWorkspaceLocatorEvidence walks from start toward boundary and returns
// malformed workspace locator errors found before accepting a physical ancestor.
// Well-formed locator files are ignored so a child workspace locator cannot
// override the physical repository selected by the caller.
func ValidateWorkspaceLocatorEvidence(start, boundary string) error {
	path, err := filepath.Abs(start)
	if err != nil {
		return err
	}
	path = filepath.Clean(path)
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		path = filepath.Dir(path)
	} else if err != nil {
		return fmt.Errorf("stat locator scan start %s: %w", path, err)
	}

	boundary, err = filepath.Abs(boundary)
	if err != nil {
		return err
	}
	boundary = filepath.Clean(boundary)

	contained, err := cleanAbsPathContains(boundary, path)
	if err != nil {
		return fmt.Errorf("compute locator scan boundary: %w", err)
	}
	if !contained {
		return fmt.Errorf("locator scan start %s is outside boundary %s", path, boundary)
	}

	for {
		if _, _, err := readWorkspaceLocator(filepath.Join(path, JVSDirName)); err != nil {
			return err
		}
		if path == boundary {
			return nil
		}
		parent := filepath.Dir(path)
		if parent == path {
			return fmt.Errorf("locator scan reached filesystem root before boundary %s", boundary)
		}
		path = parent
	}
}

func loadRepoAtRoot(root string) (*Repo, error) {
	jvsDir := filepath.Join(root, JVSDirName)
	info, err := os.Stat(jvsDir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", jvsDir)
	}
	version, err := readFormatVersion(jvsDir)
	if err != nil {
		return nil, err
	}
	if version > FormatVersion {
		return nil, errclass.ErrFormatUnsupported.WithMessagef(
			"format version %d > supported %d", version, FormatVersion)
	}
	repoID, _ := readRepoID(jvsDir)
	return &Repo{
		Root:          root,
		FormatVersion: version,
		RepoID:        repoID,
	}, nil
}

func discoverWorkspaceLocator(path string) (*Repo, workspaceLocatorFile, bool, error) {
	locator, ok, err := readWorkspaceLocator(path)
	if !ok || err != nil {
		return nil, workspaceLocatorFile{}, ok, err
	}
	repoRoot, err := cleanExistingWorkspaceLocatorRepoRoot(locator.RepoRoot)
	if err != nil {
		return nil, workspaceLocatorFile{}, true, fmt.Errorf("invalid JVS workspace locator %s: %w", path, err)
	}
	r, err := loadRepoAtRoot(repoRoot)
	if err != nil {
		return nil, workspaceLocatorFile{}, true, fmt.Errorf("JVS workspace locator %s points to an invalid repository: %w", path, err)
	}
	if strings.TrimSpace(r.RepoID) == "" {
		return nil, workspaceLocatorFile{}, true, fmt.Errorf("JVS workspace locator %s points to repository without repo_id", path)
	}
	if locator.RepoID != r.RepoID {
		return nil, workspaceLocatorFile{}, true, fmt.Errorf("JVS workspace locator %s repo_id mismatch: locator has %s, repository has %s", path, locator.RepoID, r.RepoID)
	}
	return r, locator, true, nil
}

func readWorkspaceLocator(path string) (workspaceLocatorFile, bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return workspaceLocatorFile{}, false, nil
		}
		return workspaceLocatorFile{}, false, fmt.Errorf("stat JVS locator: %w", err)
	}
	if info.IsDir() {
		return workspaceLocatorFile{}, false, nil
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return workspaceLocatorFile{}, true, fmt.Errorf("JVS workspace locator must not be a symlink: %s", path)
	}
	if !info.Mode().IsRegular() {
		return workspaceLocatorFile{}, true, fmt.Errorf("JVS workspace locator is not a regular file: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return workspaceLocatorFile{}, true, fmt.Errorf("read JVS workspace locator: %w", err)
	}
	var locator workspaceLocatorFile
	if err := json.Unmarshal(data, &locator); err != nil {
		return workspaceLocatorFile{}, true, fmt.Errorf("parse JVS workspace locator: %w", err)
	}
	if locator.Type != workspaceLocatorType {
		return workspaceLocatorFile{}, true, fmt.Errorf("unsupported JVS workspace locator type %q", locator.Type)
	}
	if locator.FormatVersion != FormatVersion {
		return workspaceLocatorFile{}, true, fmt.Errorf("unsupported JVS workspace locator format version %d", locator.FormatVersion)
	}
	repoRoot, err := cleanWorkspaceLocatorRepoRoot(locator.RepoRoot)
	if err != nil {
		return workspaceLocatorFile{}, true, fmt.Errorf("invalid JVS workspace locator repo_root: %w", err)
	}
	locator.RepoRoot = repoRoot
	if strings.TrimSpace(locator.RepoID) == "" {
		return workspaceLocatorFile{}, true, fmt.Errorf("invalid JVS workspace locator repo_id: repo_id is required")
	}
	if err := pathutil.ValidateName(locator.WorkspaceName); err != nil {
		return workspaceLocatorFile{}, true, fmt.Errorf("invalid JVS workspace locator workspace_name: %w", err)
	}
	return locator, true, nil
}

func cleanWorkspaceLocatorRepoRoot(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", fmt.Errorf("repo_root is required")
	}
	if !filepath.IsAbs(raw) {
		return "", fmt.Errorf("repo_root must be absolute: %s", raw)
	}
	return filepath.Clean(raw), nil
}

func cleanExistingWorkspaceLocatorRepoRoot(raw string) (string, error) {
	root, err := cleanWorkspaceLocatorRepoRoot(raw)
	if err != nil {
		return "", err
	}
	return existingPhysicalPath(root)
}

// DiscoverWorktree discovers the repo and maps cwd to a worktree name.
func DiscoverWorktree(cwd string) (*Repo, string, error) {
	discovered, err := discoverRepoEvidence(cwd)
	if err != nil {
		return nil, "", err
	}
	r := discovered.repo

	if discovered.locator != nil {
		name, err := registeredWorktreeFromLocator(r.Root, cwd, *discovered.locator, discovered.locatorPath)
		if err != nil {
			return nil, "", err
		}
		return r, name, nil
	}

	name, err := registeredWorktreeFromPath(r.Root, cwd)
	if err != nil {
		return nil, "", err
	}
	return r, name, nil
}

func registeredWorktreeFromLocator(repoRoot, cwd string, locator workspaceLocatorFile, locatorPath string) (string, error) {
	cfg, err := LoadWorktreeConfig(repoRoot, locator.WorkspaceName)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("JVS workspace locator %s points to workspace %q, but the repository registry does not contain that workspace; run doctor or repair", locatorPath, locator.WorkspaceName)
		}
		return "", fmt.Errorf("JVS workspace locator %s points to workspace %q, but the repository registry could not be read: %w", locatorPath, locator.WorkspaceName, err)
	}
	if cfg.Name != locator.WorkspaceName {
		return "", fmt.Errorf("JVS workspace locator %s points to workspace %q, but the repository registry has config name %q", locatorPath, locator.WorkspaceName, cfg.Name)
	}
	payloadPath, err := WorktreePayloadPath(repoRoot, locator.WorkspaceName)
	if err != nil {
		return "", fmt.Errorf("JVS workspace locator %s points to workspace %q, but the repository registry is inconsistent: %w", locatorPath, locator.WorkspaceName, err)
	}
	inside, err := pathContainsPhysicalPath(payloadPath, cwd)
	if err != nil {
		return "", err
	}
	if !inside {
		return "", fmt.Errorf("JVS workspace locator %s points to workspace %q, but current folder is not inside registered workspace folder %s", locatorPath, locator.WorkspaceName, payloadPath)
	}
	return locator.WorkspaceName, nil
}

func registeredWorktreeFromPath(repoRoot, cwd string) (string, error) {
	insideControl, err := pathInsideControlDir(repoRoot, cwd)
	if err != nil {
		return "", err
	}
	if insideControl {
		return "", nil
	}

	worktreesDir, err := WorktreesDirPath(repoRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("resolve workspaces directory: %w", err)
	}
	entries, err := os.ReadDir(worktreesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read workspaces directory: %w", err)
	}

	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 || !entry.IsDir() {
			continue
		}
		name := entry.Name()
		payloadPath, err := WorktreePayloadPath(repoRoot, name)
		if err != nil {
			continue
		}
		if payloadIsSymlink(payloadPath) {
			continue
		}
		inside, err := pathContainsPhysicalPath(payloadPath, cwd)
		if err != nil {
			return "", err
		}
		if !inside {
			continue
		}
		cfg, err := LoadWorktreeConfig(repoRoot, name)
		if err != nil || cfg.Name != name {
			return "", nil
		}
		return name, nil
	}
	return "", nil
}

func payloadIsSymlink(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode()&os.ModeSymlink != 0
}

func pathInsideControlDir(repoRoot, path string) (bool, error) {
	controlDir := filepath.Join(repoRoot, JVSDirName)
	insideLexical, err := pathContainsLexicalPath(controlDir, path)
	if err != nil {
		return false, err
	}
	if insideLexical {
		return true, nil
	}
	return pathContainsPhysicalPath(controlDir, path)
}

func pathContainsLexicalPath(base, path string) (bool, error) {
	baseAbs, err := cleanAbsPath(base)
	if err != nil {
		return false, err
	}
	pathAbs, err := cleanAbsPath(path)
	if err != nil {
		return false, err
	}
	return cleanAbsPathContains(baseAbs, pathAbs)
}

func pathContainsPhysicalPath(base, path string) (bool, error) {
	basePhysical, err := existingPhysicalPath(base)
	if err != nil {
		return false, nil
	}
	pathPhysical, err := existingPhysicalPath(path)
	if err != nil {
		return false, fmt.Errorf("resolve workspace path: %w", err)
	}
	rel, err := filepath.Rel(basePhysical, pathPhysical)
	if err != nil {
		return false, fmt.Errorf("compute workspace relative path: %w", err)
	}
	return relPathContained(rel), nil
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

func cleanAbsPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
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

// WorktreeConfigPath returns the path to a worktree's config.json.
func WorktreeConfigPath(repoRoot, name string) (string, error) {
	if err := pathutil.ValidateName(name); err != nil {
		return "", err
	}
	return filepath.Join(repoRoot, JVSDirName, "worktrees", name, "config.json"), nil
}

// WorktreeConfigDirPath returns the metadata directory for a worktree.
func WorktreeConfigDirPath(repoRoot, name string) (string, error) {
	if err := pathutil.ValidateName(name); err != nil {
		return "", err
	}
	return filepath.Join(repoRoot, JVSDirName, "worktrees", name), nil
}

// WorktreesDirPath returns the worktrees control directory after validating it.
func WorktreesDirPath(repoRoot string) (string, error) {
	return controlDirPath(repoRoot, "worktrees")
}

// WriteWorktreeConfig atomically writes a worktree config.
func WriteWorktreeConfig(repoRoot, name string, cfg *model.WorktreeConfig) error {
	path, err := safeWorktreeConfigPath(repoRoot, name)
	if err != nil {
		return err
	}
	if err := rejectSymlinkOrDirLeaf(path); err != nil {
		return fmt.Errorf("validate worktree config path: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal worktree config: %w", err)
	}
	return fsutil.AtomicWrite(path, data, 0644)
}

// WriteWorkspaceLocator makes an external workspace discover its owning
// repository when users run jvs from inside that workspace folder.
func WriteWorkspaceLocator(workspaceRoot, repoRoot, workspaceName string) error {
	if err := pathutil.ValidateName(workspaceName); err != nil {
		return err
	}
	workspaceRoot, err := existingPhysicalPath(workspaceRoot)
	if err != nil {
		return fmt.Errorf("resolve workspace folder: %w", err)
	}
	repoRoot, err = cleanExistingWorkspaceLocatorRepoRoot(repoRoot)
	if err != nil {
		return fmt.Errorf("resolve repository root: %w", err)
	}
	r, err := loadRepoAtRoot(repoRoot)
	if err != nil {
		return fmt.Errorf("load repository root: %w", err)
	}
	if strings.TrimSpace(r.RepoID) == "" {
		return fmt.Errorf("repository root has no repo_id: %s", repoRoot)
	}
	locatorPath := filepath.Join(workspaceRoot, JVSDirName)
	if existing, ok, err := readWorkspaceLocator(locatorPath); err != nil {
		return err
	} else if ok {
		existingRoot, err := cleanExistingWorkspaceLocatorRepoRoot(existing.RepoRoot)
		if err == nil && existingRoot == repoRoot && existing.RepoID == r.RepoID && existing.WorkspaceName == workspaceName {
			return nil
		}
		if existing.WorkspaceName != workspaceName {
			return fmt.Errorf("workspace already contains JVS locator bound to repo %s workspace %s", existing.RepoRoot, existing.WorkspaceName)
		}
	}
	if info, err := os.Lstat(locatorPath); err == nil {
		if info.IsDir() {
			return fmt.Errorf("workspace already contains JVS control directory: %s", locatorPath)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("workspace already contains reserved JVS locator path: %s", locatorPath)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat JVS workspace locator: %w", err)
	}

	data, err := json.MarshalIndent(workspaceLocatorFile{
		Type:          workspaceLocatorType,
		FormatVersion: FormatVersion,
		RepoRoot:      repoRoot,
		RepoID:        r.RepoID,
		WorkspaceName: workspaceName,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal JVS workspace locator: %w", err)
	}
	if err := fsutil.AtomicWrite(locatorPath, data, 0644); err != nil {
		return fmt.Errorf("write JVS workspace locator: %w", err)
	}
	return fsutil.FsyncDir(workspaceRoot)
}

// WorkspaceLocatorPresent reports whether workspaceRoot contains a JVS
// workspace locator file.
func WorkspaceLocatorPresent(workspaceRoot string) (bool, error) {
	_, ok, err := readWorkspaceLocator(filepath.Join(workspaceRoot, JVSDirName))
	return ok, err
}

// WorkspaceLocatorMatchesRepo reports whether workspaceRoot contains a valid
// workspace locator that currently resolves to repoRoot. A locator with an
// offline or otherwise stale repo_root is treated as a non-match so repair can
// rewrite it.
func WorkspaceLocatorMatchesRepo(workspaceRoot, repoRoot string) (bool, error) {
	locator, ok, err := readWorkspaceLocator(filepath.Join(workspaceRoot, JVSDirName))
	if !ok || err != nil {
		return false, err
	}
	expectedRoot, err := cleanExistingWorkspaceLocatorRepoRoot(repoRoot)
	if err != nil {
		return false, err
	}
	locatorRoot, err := cleanExistingWorkspaceLocatorRepoRoot(locator.RepoRoot)
	if err != nil {
		return false, nil
	}
	if locatorRoot != expectedRoot {
		return false, nil
	}
	r, err := loadRepoAtRoot(expectedRoot)
	if err != nil {
		return false, err
	}
	return locator.RepoID == r.RepoID, nil
}

// LoadWorktreeConfig loads a worktree config.
func LoadWorktreeConfig(repoRoot, name string) (*model.WorktreeConfig, error) {
	path, err := safeWorktreeConfigPath(repoRoot, name)
	if err != nil {
		return nil, err
	}
	if err := rejectSymlinkOrDirLeaf(path); err != nil {
		return nil, fmt.Errorf("read worktree config: %w", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read worktree config: %w", err)
	}
	var cfg model.WorktreeConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse worktree config: %w", err)
	}
	return &cfg, nil
}

// WorktreePayloadPath returns the payload directory for a worktree.
func WorktreePayloadPath(repoRoot, name string) (string, error) {
	if err := pathutil.ValidateName(name); err != nil {
		return "", err
	}
	candidate, err := worktreePathCandidateForName(repoRoot, name)
	if err != nil {
		return "", err
	}
	if err := validateWorktreeRegistryWithCandidate(repoRoot, candidate); err != nil {
		return "", err
	}
	return candidate.path, nil
}

// WorktreeManagedPayloadBoundary returns the managed payload root and any
// root-level control paths that must be excluded from captures.
func WorktreeManagedPayloadBoundary(repoRoot, name string) (WorktreePayloadBoundary, error) {
	root, err := WorktreePayloadPath(repoRoot, name)
	if err != nil {
		return WorktreePayloadBoundary{}, err
	}
	if err := validatePayloadBoundaryRoot(repoRoot, root); err != nil {
		return WorktreePayloadBoundary{}, err
	}
	excluded, err := worktreeControlExclusions(repoRoot, root)
	if err != nil {
		return WorktreePayloadBoundary{}, err
	}
	return WorktreePayloadBoundary{Root: root, ExcludedRootNames: excluded}, nil
}

// ValidateWorktreeRealPathRegistry verifies that registered workspace payload
// roots do not overlap each other or point into repository control data.
func ValidateWorktreeRealPathRegistry(repoRoot string) error {
	candidates, err := registeredWorktreePathCandidates(repoRoot)
	if err != nil {
		if errors.Is(err, errWorktreeRegistryMissing) {
			return nil
		}
		return err
	}
	return validateWorktreePathCandidates(repoRoot, candidates)
}

// ValidateWorktreeRealPathForRepair validates a replacement real path and
// returns the canonical physical path that should be stored.
func ValidateWorktreeRealPathForRepair(repoRoot, name, realPath string) (string, error) {
	if err := pathutil.ValidateName(name); err != nil {
		return "", err
	}
	candidate, err := configuredWorktreePathCandidate(repoRoot, name, realPath)
	if err != nil {
		return "", err
	}
	if err := validateWorktreeRegistryWithRepairCandidate(repoRoot, candidate); err != nil {
		return "", err
	}
	return candidate.physicalPath, nil
}

type worktreePathCandidate struct {
	name         string
	path         string
	lexicalPath  string
	physicalPath string
}

func worktreePathCandidateForName(repoRoot, name string) (worktreePathCandidate, error) {
	cfg, present, err := loadWorktreeConfigIfPresent(repoRoot, name)
	if err != nil {
		return worktreePathCandidate{}, err
	}
	return worktreePathCandidateFromConfig(repoRoot, name, cfg, present)
}

func loadWorktreeConfigIfPresent(repoRoot, name string) (*model.WorktreeConfig, bool, error) {
	cfg, err := LoadWorktreeConfig(repoRoot, name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return cfg, true, nil
}

func worktreePathCandidateFromConfig(repoRoot, name string, cfg *model.WorktreeConfig, present bool) (worktreePathCandidate, error) {
	if present {
		if cfg == nil {
			return worktreePathCandidate{}, fmt.Errorf("worktree %s config missing", name)
		}
		if cfg.Name != name {
			return worktreePathCandidate{}, fmt.Errorf("worktree %s config name mismatch %q", name, cfg.Name)
		}
		if cfg.RealPath != "" {
			return configuredWorktreePathCandidate(repoRoot, name, cfg.RealPath)
		}
	}

	return fallbackWorktreePathCandidate(repoRoot, name)
}

func configuredWorktreePathCandidate(repoRoot, name, realPath string) (worktreePathCandidate, error) {
	if !filepath.IsAbs(realPath) {
		return worktreePathCandidate{}, fmt.Errorf("worktree %s real path must be absolute: %s", name, realPath)
	}
	lexicalPath, err := cleanAbsPath(realPath)
	if err != nil {
		return worktreePathCandidate{}, fmt.Errorf("resolve worktree real path: %w", err)
	}
	physicalPath, err := existingPhysicalPath(realPath)
	if err != nil {
		return worktreePathCandidate{}, fmt.Errorf("resolve worktree real path: %w", err)
	}
	info, err := os.Stat(physicalPath)
	if err != nil {
		return worktreePathCandidate{}, fmt.Errorf("stat worktree real path: %w", err)
	}
	if !info.IsDir() {
		return worktreePathCandidate{}, fmt.Errorf("worktree real path is not a directory: %s", physicalPath)
	}
	return worktreePathCandidate{
		name:         name,
		path:         physicalPath,
		lexicalPath:  lexicalPath,
		physicalPath: physicalPath,
	}, nil
}

func fallbackWorktreePathCandidate(repoRoot, name string) (worktreePathCandidate, error) {
	path := legacyWorktreePayloadPath(repoRoot, name)
	lexicalPath, err := cleanAbsPath(path)
	if err != nil {
		return worktreePathCandidate{}, err
	}
	physicalPath, err := resolvePhysicalInitTarget(lexicalPath)
	if err != nil {
		return worktreePathCandidate{}, fmt.Errorf("resolve worktree payload path: %w", err)
	}
	return worktreePathCandidate{
		name:         name,
		path:         path,
		lexicalPath:  lexicalPath,
		physicalPath: physicalPath,
	}, nil
}

func legacyWorktreePayloadPath(repoRoot, name string) string {
	if name == "main" {
		return filepath.Join(repoRoot, "main")
	}
	return filepath.Join(repoRoot, "worktrees", name)
}

func validateWorktreeRegistryWithCandidate(repoRoot string, candidate worktreePathCandidate) error {
	candidates, err := registeredWorktreePathCandidates(repoRoot)
	if err != nil {
		if errors.Is(err, errWorktreeRegistryMissing) {
			return nil
		}
		return err
	}

	replaced := false
	for i := range candidates {
		if candidates[i].name == candidate.name {
			candidates[i] = candidate
			replaced = true
			break
		}
	}
	if !replaced {
		candidates = append(candidates, candidate)
	}
	return validateWorktreePathCandidates(repoRoot, candidates)
}

func validateWorktreeRegistryWithRepairCandidate(repoRoot string, candidate worktreePathCandidate) error {
	candidates, err := registeredWorktreePathCandidatesForRepair(repoRoot, candidate.name)
	if err != nil {
		if errors.Is(err, errWorktreeRegistryMissing) {
			return nil
		}
		return err
	}
	candidates = append(candidates, candidate)
	return validateWorktreePathCandidates(repoRoot, candidates)
}

func registeredWorktreePathCandidates(repoRoot string) ([]worktreePathCandidate, error) {
	worktreesDir, err := WorktreesDirPath(repoRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: %v", errWorktreeRegistryMissing, err)
		}
		return nil, fmt.Errorf("resolve workspaces directory: %w", err)
	}
	entries, err := os.ReadDir(worktreesDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: %v", errWorktreeRegistryMissing, err)
		}
		return nil, fmt.Errorf("read workspaces directory: %w", err)
	}

	candidates := make([]worktreePathCandidate, 0, len(entries))
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("worktree metadata entry is symlink: %s", entry.Name())
		}
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if err := pathutil.ValidateName(name); err != nil {
			return nil, fmt.Errorf("worktree metadata entry %s: %w", name, err)
		}
		cfg, err := LoadWorktreeConfig(repoRoot, name)
		if err != nil {
			return nil, fmt.Errorf("load worktree %s: %w", name, err)
		}
		candidate, err := worktreePathCandidateFromConfig(repoRoot, name, cfg, true)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, candidate)
	}
	return candidates, nil
}

func registeredWorktreePathCandidatesForRepair(repoRoot, replacedName string) ([]worktreePathCandidate, error) {
	worktreesDir, err := WorktreesDirPath(repoRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: %v", errWorktreeRegistryMissing, err)
		}
		return nil, fmt.Errorf("resolve workspaces directory: %w", err)
	}
	entries, err := os.ReadDir(worktreesDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: %v", errWorktreeRegistryMissing, err)
		}
		return nil, fmt.Errorf("read workspaces directory: %w", err)
	}

	candidates := make([]worktreePathCandidate, 0, len(entries))
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("worktree metadata entry is symlink: %s", entry.Name())
		}
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if err := pathutil.ValidateName(name); err != nil {
			return nil, fmt.Errorf("worktree metadata entry %s: %w", name, err)
		}
		if name == replacedName {
			continue
		}
		cfg, err := LoadWorktreeConfig(repoRoot, name)
		if err != nil {
			return nil, fmt.Errorf("load worktree %s: %w", name, err)
		}
		candidate, err := worktreePathCandidateFromConfig(repoRoot, name, cfg, true)
		if err != nil {
			if cfg.RealPath != "" && errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}
		candidates = append(candidates, candidate)
	}
	return candidates, nil
}

func validateWorktreePathCandidates(repoRoot string, candidates []worktreePathCandidate) error {
	for _, candidate := range candidates {
		if err := validateWorktreePathControlBoundary(repoRoot, candidate); err != nil {
			return err
		}
	}

	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			if candidatesOverlap(candidates[i], candidates[j]) {
				return fmt.Errorf("worktree real path overlap: %s at %s and %s at %s",
					candidates[i].name, candidates[i].path, candidates[j].name, candidates[j].path)
			}
		}
	}
	return nil
}

func validateWorktreePathControlBoundary(repoRoot string, candidate worktreePathCandidate) error {
	controlLexical, err := cleanAbsPath(filepath.Join(repoRoot, JVSDirName))
	if err != nil {
		return err
	}
	repoLexical, err := cleanAbsPath(repoRoot)
	if err != nil {
		return err
	}
	controlPhysical, err := existingPhysicalPath(controlLexical)
	if err != nil {
		return fmt.Errorf("resolve repo control directory: %w", err)
	}
	repoPhysical, err := existingPhysicalPath(repoRoot)
	if err != nil {
		return fmt.Errorf("resolve repo root: %w", err)
	}

	insideControlLexical, err := cleanAbsPathContains(controlLexical, candidate.lexicalPath)
	if err != nil {
		return err
	}
	insideControlPhysical, err := cleanAbsPathContains(controlPhysical, candidate.physicalPath)
	if err != nil {
		return err
	}
	if insideControlLexical || insideControlPhysical {
		return fmt.Errorf("worktree %s real path points into repo control directory: %s", candidate.name, candidate.path)
	}

	containsControlLexical, err := cleanAbsPathContains(candidate.lexicalPath, controlLexical)
	if err != nil {
		return err
	}
	containsControlPhysical, err := cleanAbsPathContains(candidate.physicalPath, controlPhysical)
	if err != nil {
		return err
	}
	isRepoRoot := candidate.lexicalPath == repoLexical || candidate.physicalPath == repoPhysical
	if (containsControlLexical || containsControlPhysical) && !isRepoRoot {
		return fmt.Errorf("worktree %s real path contains repo control directory: %s", candidate.name, candidate.path)
	}
	return nil
}

func candidatesOverlap(a, b worktreePathCandidate) bool {
	return absPathsOverlap(a.lexicalPath, b.lexicalPath) || absPathsOverlap(a.physicalPath, b.physicalPath)
}

func absPathsOverlap(a, b string) bool {
	aContainsB, err := cleanAbsPathContains(a, b)
	if err != nil {
		return false
	}
	bContainsA, err := cleanAbsPathContains(b, a)
	if err != nil {
		return false
	}
	return aContainsB || bContainsA
}

func validatePayloadBoundaryRoot(repoRoot, root string) error {
	repoAbs, err := cleanAbsPath(repoRoot)
	if err != nil {
		return err
	}
	rootAbs, err := cleanAbsPath(root)
	if err != nil {
		return err
	}
	insideRepo, err := cleanAbsPathContains(repoAbs, rootAbs)
	if err != nil {
		return err
	}
	if insideRepo {
		rel, err := filepath.Rel(repoAbs, rootAbs)
		if err != nil {
			return err
		}
		if rel != "." {
			if err := pathutil.ValidateNoSymlinkParents(repoRoot, rel); err != nil {
				return fmt.Errorf("validate worktree payload path: %w", err)
			}
		}
	}
	info, err := os.Lstat(root)
	if err != nil {
		return fmt.Errorf("stat worktree payload root: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("worktree payload root is symlink: %s", root)
	}
	if !info.IsDir() {
		return fmt.Errorf("worktree payload root is not directory: %s", root)
	}
	return nil
}

func worktreeControlExclusions(repoRoot, payloadRoot string) ([]string, error) {
	controlDir := filepath.Join(repoRoot, JVSDirName)
	containsControl, err := pathContainsLexicalPath(payloadRoot, controlDir)
	if err != nil {
		return nil, err
	}
	if !containsControl {
		containsControl, err = pathContainsPhysicalPath(payloadRoot, controlDir)
		if err != nil {
			return nil, err
		}
	}
	if !containsControl {
		locatorPath := filepath.Join(payloadRoot, JVSDirName)
		containsLocator, err := workspaceLocatorBelongsToRepo(locatorPath, repoRoot)
		if err != nil {
			return nil, err
		}
		if containsLocator {
			return []string{JVSDirName}, nil
		}
		return nil, nil
	}

	rootAbs, err := cleanAbsPath(payloadRoot)
	if err != nil {
		return nil, err
	}
	controlAbs, err := cleanAbsPath(controlDir)
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(rootAbs, controlAbs)
	if err != nil {
		return nil, err
	}
	if rel == JVSDirName {
		return []string{JVSDirName}, nil
	}
	return nil, fmt.Errorf("repo control directory is nested inside worktree payload at unsupported path: %s", rel)
}

func workspaceLocatorBelongsToRepo(locatorPath, repoRoot string) (bool, error) {
	locator, ok, err := readWorkspaceLocator(locatorPath)
	if !ok || err != nil {
		return false, err
	}
	locatorRepoRoot, err := cleanExistingWorkspaceLocatorRepoRoot(locator.RepoRoot)
	if err != nil {
		return false, err
	}
	repoRoot, err = cleanExistingWorkspaceLocatorRepoRoot(repoRoot)
	if err != nil {
		return false, err
	}
	if locatorRepoRoot != repoRoot {
		return false, fmt.Errorf("JVS workspace locator points at %s, not repository %s", locatorRepoRoot, repoRoot)
	}
	r, err := loadRepoAtRoot(repoRoot)
	if err != nil {
		return false, err
	}
	if locator.RepoID != r.RepoID {
		return false, fmt.Errorf("JVS workspace locator repo_id does not match repository %s", repoRoot)
	}
	return true, nil
}

func safeWorktreeConfigPath(repoRoot, name string) (string, error) {
	path, err := WorktreeConfigPath(repoRoot, name)
	if err != nil {
		return "", err
	}
	if err := validateControlDirs(repoRoot, "worktrees", name); err != nil {
		return "", fmt.Errorf("validate worktree config path: %w", err)
	}
	return path, nil
}

func rejectSymlinkOrDirLeaf(path string) error {
	return validateControlLeaf(path, controlLeafRegularFile, true)
}

// SnapshotsDirPath returns the snapshots control directory after validating it.
func SnapshotsDirPath(repoRoot string) (string, error) {
	return controlDirPath(repoRoot, "snapshots")
}

// IntentsDirPath returns the intents control directory after validating it.
func IntentsDirPath(repoRoot string) (string, error) {
	return controlDirPath(repoRoot, "intents")
}

// DescriptorsDirPath returns the descriptors control directory after validating it.
func DescriptorsDirPath(repoRoot string) (string, error) {
	return controlDirPath(repoRoot, "descriptors")
}

// GCDirPath returns the GC control directory after validating it.
func GCDirPath(repoRoot string) (string, error) {
	return controlDirPath(repoRoot, "gc")
}

// GCTombstonesDirPath returns the tombstones control directory after validating it.
func GCTombstonesDirPath(repoRoot string) (string, error) {
	return controlDirPath(repoRoot, "gc", "tombstones")
}

// GCPinsDirPath returns the documented GC pins control directory after validating it.
func GCPinsDirPath(repoRoot string) (string, error) {
	return controlDirPath(repoRoot, "gc", "pins")
}

// LegacyPinsDirPath returns the legacy pins control directory after validating it.
func LegacyPinsDirPath(repoRoot string) (string, error) {
	return controlDirPath(repoRoot, "pins")
}

// SnapshotPath returns the on-disk snapshot storage path for a canonical ID.
func SnapshotPath(repoRoot string, snapshotID model.SnapshotID) (string, error) {
	if err := snapshotID.Validate(); err != nil {
		return "", err
	}
	return controlFilePath(repoRoot, []string{"snapshots"}, string(snapshotID))
}

// SnapshotPathForRead returns an existing snapshot directory path after
// rejecting a symlink or wrong-type final leaf.
func SnapshotPathForRead(repoRoot string, snapshotID model.SnapshotID) (string, error) {
	path, err := SnapshotPath(repoRoot, snapshotID)
	if err != nil {
		return "", err
	}
	if err := validateControlLeaf(path, controlLeafDirectory, false); err != nil {
		return "", err
	}
	return path, nil
}

// SnapshotPathForDelete returns a snapshot directory path after rejecting a
// symlink or wrong-type final leaf. Missing leaves are allowed for idempotent
// delete/retry paths.
func SnapshotPathForDelete(repoRoot string, snapshotID model.SnapshotID) (string, error) {
	path, err := SnapshotPath(repoRoot, snapshotID)
	if err != nil {
		return "", err
	}
	if err := validateControlLeaf(path, controlLeafDirectory, true); err != nil {
		return "", err
	}
	return path, nil
}

// SnapshotTmpPath returns the unpublished temporary snapshot path for a canonical ID.
func SnapshotTmpPath(repoRoot string, snapshotID model.SnapshotID) (string, error) {
	if err := snapshotID.Validate(); err != nil {
		return "", err
	}
	return controlFilePath(repoRoot, []string{"snapshots"}, string(snapshotID)+".tmp")
}

// SnapshotDescriptorPath returns the descriptor path for a canonical snapshot ID.
func SnapshotDescriptorPath(repoRoot string, snapshotID model.SnapshotID) (string, error) {
	if err := snapshotID.Validate(); err != nil {
		return "", err
	}
	return controlFilePath(repoRoot, []string{"descriptors"}, string(snapshotID)+".json")
}

// SnapshotDescriptorPathForRead returns an existing descriptor path after
// rejecting a symlink or wrong-type final leaf.
func SnapshotDescriptorPathForRead(repoRoot string, snapshotID model.SnapshotID) (string, error) {
	path, err := SnapshotDescriptorPath(repoRoot, snapshotID)
	if err != nil {
		return "", err
	}
	if err := validateControlLeaf(path, controlLeafRegularFile, false); err != nil {
		return "", err
	}
	return path, nil
}

// SnapshotDescriptorPathForWrite returns a descriptor path after rejecting a
// symlink or wrong-type existing final leaf. Missing leaves are allowed.
func SnapshotDescriptorPathForWrite(repoRoot string, snapshotID model.SnapshotID) (string, error) {
	path, err := SnapshotDescriptorPath(repoRoot, snapshotID)
	if err != nil {
		return "", err
	}
	if err := validateControlLeaf(path, controlLeafRegularFile, true); err != nil {
		return "", err
	}
	return path, nil
}

// SnapshotDescriptorPathForDelete returns a descriptor path after rejecting a
// symlink or wrong-type existing final leaf. Missing leaves are allowed.
func SnapshotDescriptorPathForDelete(repoRoot string, snapshotID model.SnapshotID) (string, error) {
	path, err := SnapshotDescriptorPath(repoRoot, snapshotID)
	if err != nil {
		return "", err
	}
	if err := validateControlLeaf(path, controlLeafRegularFile, true); err != nil {
		return "", err
	}
	return path, nil
}

// IntentPath returns the intent record path for a canonical snapshot ID.
func IntentPath(repoRoot string, snapshotID model.SnapshotID) (string, error) {
	if err := snapshotID.Validate(); err != nil {
		return "", err
	}
	return controlFilePath(repoRoot, []string{"intents"}, string(snapshotID)+".json")
}

// GCTombstonePath returns the tombstone path for a canonical snapshot ID.
func GCTombstonePath(repoRoot string, snapshotID model.SnapshotID) (string, error) {
	if err := snapshotID.Validate(); err != nil {
		return "", err
	}
	return controlFilePath(repoRoot, []string{"gc", "tombstones"}, string(snapshotID)+".json")
}

// GCTombstonePathForRead returns an existing tombstone path after rejecting a
// symlink or wrong-type final leaf.
func GCTombstonePathForRead(repoRoot string, snapshotID model.SnapshotID) (string, error) {
	path, err := GCTombstonePath(repoRoot, snapshotID)
	if err != nil {
		return "", err
	}
	if err := validateControlLeaf(path, controlLeafRegularFile, false); err != nil {
		return "", err
	}
	return path, nil
}

// GCTombstonePathForWrite returns a tombstone path after rejecting a symlink or
// wrong-type existing final leaf. Missing leaves are allowed.
func GCTombstonePathForWrite(repoRoot string, snapshotID model.SnapshotID) (string, error) {
	path, err := GCTombstonePath(repoRoot, snapshotID)
	if err != nil {
		return "", err
	}
	if err := validateControlLeaf(path, controlLeafRegularFile, true); err != nil {
		return "", err
	}
	return path, nil
}

// GCTombstonePathForDelete returns a tombstone path after rejecting a symlink or
// wrong-type existing final leaf. Missing leaves are allowed.
func GCTombstonePathForDelete(repoRoot string, snapshotID model.SnapshotID) (string, error) {
	path, err := GCTombstonePath(repoRoot, snapshotID)
	if err != nil {
		return "", err
	}
	if err := validateControlLeaf(path, controlLeafRegularFile, true); err != nil {
		return "", err
	}
	return path, nil
}

// GCPinPathForRead returns an existing documented GC pin path after rejecting a
// symlink or wrong-type final leaf.
func GCPinPathForRead(repoRoot, pinFileName string) (string, error) {
	return pinPathForRead(repoRoot, []string{"gc", "pins"}, pinFileName)
}

// LegacyPinPathForRead returns an existing legacy pin path after rejecting a
// symlink or wrong-type final leaf.
func LegacyPinPathForRead(repoRoot, pinFileName string) (string, error) {
	return pinPathForRead(repoRoot, []string{"pins"}, pinFileName)
}

func pinPathForRead(repoRoot string, parentComponents []string, pinFileName string) (string, error) {
	if err := pathutil.ValidateName(pinFileName); err != nil {
		return "", err
	}
	path, err := controlFilePath(repoRoot, parentComponents, pinFileName)
	if err != nil {
		return "", err
	}
	if err := validateControlLeaf(path, controlLeafRegularFile, false); err != nil {
		return "", err
	}
	return path, nil
}

// GCPlanPath returns the path for a GC plan ID after rejecting path-like names.
func GCPlanPath(repoRoot, planID string) (string, error) {
	if err := pathutil.ValidateName(planID); err != nil {
		return "", err
	}
	return controlFilePath(repoRoot, []string{"gc"}, planID+".json")
}

// GCPlanPathForRead returns an existing GC plan path after rejecting a symlink
// or wrong-type final leaf.
func GCPlanPathForRead(repoRoot, planID string) (string, error) {
	path, err := GCPlanPath(repoRoot, planID)
	if err != nil {
		return "", err
	}
	if err := validateControlLeaf(path, controlLeafRegularFile, false); err != nil {
		return "", err
	}
	return path, nil
}

// GCPlanPathForWrite returns a GC plan path after rejecting a symlink or
// wrong-type existing final leaf. Missing leaves are allowed.
func GCPlanPathForWrite(repoRoot, planID string) (string, error) {
	path, err := GCPlanPath(repoRoot, planID)
	if err != nil {
		return "", err
	}
	if err := validateControlLeaf(path, controlLeafRegularFile, true); err != nil {
		return "", err
	}
	return path, nil
}

// GCPlanPathForDelete returns a GC plan path after rejecting a symlink or
// wrong-type existing final leaf. Missing leaves are allowed.
func GCPlanPathForDelete(repoRoot, planID string) (string, error) {
	path, err := GCPlanPath(repoRoot, planID)
	if err != nil {
		return "", err
	}
	if err := validateControlLeaf(path, controlLeafRegularFile, true); err != nil {
		return "", err
	}
	return path, nil
}

// RestorePlanPath returns the path for a restore operation plan ID after
// rejecting path-like names.
func RestorePlanPath(repoRoot, planID string) (string, error) {
	if err := pathutil.ValidateName(planID); err != nil {
		return "", err
	}
	return controlFilePath(repoRoot, []string{"restore-plans"}, planID+".json")
}

// RestorePlanPathForRead returns an existing restore plan path after rejecting
// a symlink or wrong-type final leaf.
func RestorePlanPathForRead(repoRoot, planID string) (string, error) {
	path, err := RestorePlanPath(repoRoot, planID)
	if err != nil {
		return "", err
	}
	if err := validateControlLeaf(path, controlLeafRegularFile, false); err != nil {
		return "", err
	}
	return path, nil
}

// RestorePlanPathForWrite returns a restore plan path after rejecting a
// symlink or wrong-type existing final leaf. Missing leaves are allowed.
func RestorePlanPathForWrite(repoRoot, planID string) (string, error) {
	path, err := RestorePlanPath(repoRoot, planID)
	if err != nil {
		return "", err
	}
	if err := validateControlLeaf(path, controlLeafRegularFile, true); err != nil {
		return "", err
	}
	return path, nil
}

// RecoveryPlansDirPath returns the recovery plan control directory after
// validating it.
func RecoveryPlansDirPath(repoRoot string) (string, error) {
	return controlDirPath(repoRoot, "recovery-plans")
}

// RecoveryPlanPath returns the path for a recovery plan ID after rejecting
// path-like names.
func RecoveryPlanPath(repoRoot, planID string) (string, error) {
	if err := pathutil.ValidateName(planID); err != nil {
		return "", err
	}
	return controlFilePath(repoRoot, []string{"recovery-plans"}, planID+".json")
}

// RecoveryPlanPathForRead returns an existing recovery plan path after
// rejecting a symlink or wrong-type final leaf.
func RecoveryPlanPathForRead(repoRoot, planID string) (string, error) {
	path, err := RecoveryPlanPath(repoRoot, planID)
	if err != nil {
		return "", err
	}
	if err := validateControlLeaf(path, controlLeafRegularFile, false); err != nil {
		return "", err
	}
	return path, nil
}

// RecoveryPlanPathForWrite returns a recovery plan path after rejecting a
// symlink or wrong-type existing final leaf. Missing leaves are allowed.
func RecoveryPlanPathForWrite(repoRoot, planID string) (string, error) {
	path, err := RecoveryPlanPath(repoRoot, planID)
	if err != nil {
		return "", err
	}
	if err := validateControlLeaf(path, controlLeafRegularFile, true); err != nil {
		return "", err
	}
	return path, nil
}

func controlDirPath(repoRoot string, components ...string) (string, error) {
	if err := validateControlDirs(repoRoot, components...); err != nil {
		return "", err
	}
	return controlPath(repoRoot, components...), nil
}

func controlFilePath(repoRoot string, parentComponents []string, leaf string) (string, error) {
	if err := validateControlDirs(repoRoot, parentComponents...); err != nil {
		return "", err
	}
	parts := append([]string{repoRoot, JVSDirName}, parentComponents...)
	parts = append(parts, leaf)
	return filepath.Join(parts...), nil
}

func controlPath(repoRoot string, components ...string) string {
	parts := append([]string{repoRoot, JVSDirName}, components...)
	return filepath.Join(parts...)
}

type controlLeafKind int

const (
	controlLeafRegularFile controlLeafKind = iota
	controlLeafDirectory
)

func validateControlLeaf(path string, kind controlLeafKind, missingOK bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) && missingOK {
			return nil
		}
		return fmt.Errorf("stat control leaf %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("control leaf is symlink: %s", path)
	}

	switch kind {
	case controlLeafRegularFile:
		if !info.Mode().IsRegular() {
			return fmt.Errorf("control leaf is not regular file: %s", path)
		}
	case controlLeafDirectory:
		if !info.IsDir() {
			return fmt.Errorf("control leaf is not directory: %s", path)
		}
	default:
		return fmt.Errorf("unknown control leaf kind for %s", path)
	}
	return nil
}

func validateControlDirs(repoRoot string, components ...string) error {
	current := filepath.Clean(repoRoot)
	if err := validateExistingRealDir(current); err != nil {
		return err
	}

	current = filepath.Join(current, JVSDirName)
	if err := validateExistingRealDir(current); err != nil {
		return err
	}

	for _, component := range components {
		current = filepath.Join(current, component)
		if err := validateExistingRealDir(current); err != nil {
			return err
		}
	}
	return nil
}

func validateExistingRealDir(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("stat control directory %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("control directory is symlink: %s", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("control path is not directory: %s", path)
	}
	return nil
}

func readFormatVersion(jvsDir string) (int, error) {
	data, err := os.ReadFile(filepath.Join(jvsDir, FormatVersionFile))
	if err != nil {
		return 0, fmt.Errorf("read format_version: %w", err)
	}
	var version int
	if _, err := fmt.Sscanf(string(data), "%d", &version); err != nil {
		return 0, fmt.Errorf("parse format_version: %w", err)
	}
	return version, nil
}

func readRepoID(jvsDir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(jvsDir, RepoIDFile))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}
