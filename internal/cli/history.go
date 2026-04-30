package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/agentsmith-project/jvs/internal/engine"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/internal/snapshotpayload"
	"github.com/agentsmith-project/jvs/internal/worktree"
	"github.com/agentsmith-project/jvs/pkg/color"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/agentsmith-project/jvs/pkg/pathutil"
)

var (
	historyLimit      int
	historyNoteFilter string
	historyPath       string
)

const defaultHistoryLimit = 30

var historyCmd = &cobra.Command{
	Use:   "history [to <save>|from [<save>]]",
	Short: "Show save point history",
	Long: `Show save point history for the active workspace.

Examples:
  jvs history
  jvs history to <save>
  jvs history from <save>
  jvs history from
  jvs history --path notes.md
  jvs history -n 10
  jvs history --grep "baseline"

After finding a candidate, open it with:
  jvs view <save> <path>`,
	Args: validateHistoryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		r, workspaceName, err := discoverRequiredWorktree()
		if err != nil {
			return err
		}
		if historyPathFlagChanged(cmd) {
			if err := validateHistoryPathFlagCombination(cmd); err != nil {
				return historyPathError(err)
			}
			path, err := normalizeHistoryPath(historyPath)
			if err != nil {
				return historyPathError(err)
			}
			result, err := findHistoryPathCandidates(r.Root, workspaceName, path)
			if err != nil {
				return historyPathError(err)
			}
			if jsonOutput {
				return outputJSON(result)
			}
			printHistoryPathCandidates(result)
			return nil
		}

		request, err := parseHistoryRequest(args)
		if err != nil {
			return err
		}
		if historyLimit < 0 {
			return fmt.Errorf("--limit must be 0 or greater")
		}

		result, err := loadHistoryResult(r.Root, workspaceName, request)
		if err != nil {
			return err
		}
		if jsonOutput {
			return outputJSON(result)
		}
		printHistoryResult(result)
		return nil
	},
}

type historyDirection string

const (
	historyDirectionCurrent historyDirection = "current"
	historyDirectionTo      historyDirection = "to"
	historyDirectionFrom    historyDirection = "from"
)

type historyRequest struct {
	direction historyDirection
	saveRef   string
}

type publicHistoryResult struct {
	Workspace                 string                          `json:"workspace"`
	Direction                 string                          `json:"direction"`
	SavePoints                []publicSavePointRecord         `json:"save_points"`
	Nodes                     []publicSavePointRecord         `json:"nodes,omitempty"`
	Edges                     []publicHistoryEdge             `json:"edges,omitempty"`
	WorkspacePointers         []publicHistoryWorkspacePointer `json:"workspace_pointers,omitempty"`
	CurrentPointer            string                          `json:"current_pointer,omitempty"`
	NewestSavePoint           string                          `json:"newest_save_point,omitempty"`
	StartedFromSavePoint      string                          `json:"started_from_save_point,omitempty"`
	TargetSavePoint           string                          `json:"target_save_point,omitempty"`
	StartSavePoint            string                          `json:"start_save_point,omitempty"`
	Limit                     int                             `json:"limit"`
	Truncated                 bool                            `json:"truncated"`
	WorkspaceHasOwnSavePoints bool                            `json:"workspace_has_own_save_points"`
}

type publicHistoryEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Type string `json:"type"`
}

type publicHistoryWorkspacePointer struct {
	Workspace   string `json:"workspace"`
	SavePointID string `json:"save_point_id"`
	Active      bool   `json:"active,omitempty"`
}

func validateHistoryArgs(cmd *cobra.Command, args []string) error {
	if historyPathFlagChanged(cmd) && len(args) > 0 {
		return fmt.Errorf("history --path does not take direction arguments")
	}
	_, err := parseHistoryRequest(args)
	return err
}

func parseHistoryRequest(args []string) (historyRequest, error) {
	switch len(args) {
	case 0:
		return historyRequest{direction: historyDirectionCurrent}, nil
	case 1:
		if args[0] == string(historyDirectionFrom) {
			return historyRequest{direction: historyDirectionFrom}, nil
		}
	case 2:
		switch args[0] {
		case string(historyDirectionTo):
			if strings.TrimSpace(args[1]) == "" {
				return historyRequest{}, fmt.Errorf("history to requires a save point ID")
			}
			return historyRequest{direction: historyDirectionTo, saveRef: args[1]}, nil
		case string(historyDirectionFrom):
			if strings.TrimSpace(args[1]) == "" {
				return historyRequest{}, fmt.Errorf("history from requires a save point ID when a value is provided")
			}
			return historyRequest{direction: historyDirectionFrom, saveRef: args[1]}, nil
		}
	}
	return historyRequest{}, fmt.Errorf("usage: jvs history [to <save>|from [<save>]]")
}

type publicHistoryPathResult struct {
	Folder       string                       `json:"folder"`
	Workspace    string                       `json:"workspace"`
	Path         string                       `json:"path"`
	Candidates   []publicHistoryPathCandidate `json:"candidates"`
	NextCommands []string                     `json:"next_commands"`
}

type publicHistoryPathCandidate struct {
	SavePointID string    `json:"save_point_id"`
	Message     string    `json:"message,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

func historyPathFlagChanged(cmd *cobra.Command) bool {
	flag := cmd.Flags().Lookup("path")
	return flag != nil && flag.Changed
}

func validateHistoryPathFlagCombination(cmd *cobra.Command) error {
	var unsupported []string
	for _, name := range []string{"grep", "limit"} {
		flag := cmd.Flags().Lookup(name)
		if flag != nil && flag.Changed {
			unsupported = append(unsupported, "--"+name)
		}
	}
	if len(unsupported) == 0 {
		return nil
	}
	return fmt.Errorf("history --path only searches path candidates in this workspace for now; remove %s", strings.Join(unsupported, ", "))
}

func normalizeHistoryPath(raw string) (string, error) {
	path, err := normalizeViewPath(raw)
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", fmt.Errorf("path inside save point must be a workspace-relative path")
	}
	return path, nil
}

func findHistoryPathCandidates(repoRoot, workspaceName, path string) (publicHistoryPathResult, error) {
	mgr := worktree.NewManager(repoRoot)
	cfg, err := mgr.Get(workspaceName)
	if err != nil {
		return publicHistoryPathResult{}, fmt.Errorf("load workspace: %w", err)
	}
	folder, err := mgr.Path(workspaceName)
	if err != nil {
		return publicHistoryPathResult{}, fmt.Errorf("workspace folder: %w", err)
	}
	boundary, err := repo.WorktreeManagedPayloadBoundary(repoRoot, workspaceName)
	if err != nil {
		return publicHistoryPathResult{}, fmt.Errorf("workspace managed boundary: %w", err)
	}

	descs, err := loadCurrentWorkspaceSavePointLineage(repoRoot, cfg)
	if err != nil {
		return publicHistoryPathResult{}, err
	}
	candidates := make([]publicHistoryPathCandidate, 0)
	for _, desc := range descs {
		matches, err := savePointContainsHistoryPath(repoRoot, desc, path, boundary)
		if err != nil {
			return publicHistoryPathResult{}, err
		}
		if matches {
			candidates = append(candidates, publicHistoryPathCandidate{
				SavePointID: string(desc.SnapshotID),
				Message:     desc.Note,
				CreatedAt:   desc.CreatedAt,
			})
		}
	}
	result := publicHistoryPathResult{
		Folder:       folder,
		Workspace:    workspaceName,
		Path:         path,
		Candidates:   candidates,
		NextCommands: historyPathNextCommands(path, candidates),
	}
	return result, nil
}

func loadCurrentWorkspaceSavePointLineage(repoRoot string, cfg *model.WorktreeConfig) ([]*model.Descriptor, error) {
	startID := workspaceCurrentPointer(cfg)
	if startID == "" {
		return []*model.Descriptor{}, nil
	}
	var savePoints []*model.Descriptor
	currentID := &startID
	for currentID != nil {
		desc, err := snapshot.LoadDescriptor(repoRoot, *currentID)
		if err != nil {
			return nil, fmt.Errorf("load save point: %w", err)
		}
		savePoints = append(savePoints, desc)
		currentID = desc.ParentID
	}
	return savePoints, nil
}

func savePointContainsHistoryPath(repoRoot string, desc *model.Descriptor, path string, boundary repo.WorktreePayloadBoundary) (bool, error) {
	if desc == nil {
		return false, nil
	}
	state, issue := snapshot.InspectPublishState(repoRoot, desc.SnapshotID, snapshot.PublishStateOptions{
		RequireReady:             true,
		RequirePayload:           true,
		VerifyDescriptorChecksum: true,
		VerifyPayloadHash:        true,
	})
	if issue != nil {
		return false, snapshot.PublishStateIssueError(issue)
	}

	tempParent, err := os.MkdirTemp("", "jvs-history-path-*")
	if err != nil {
		return false, fmt.Errorf("create temporary history path area: %w", err)
	}
	defer func() {
		_ = restoreWriteBits(tempParent)
		_ = os.RemoveAll(tempParent)
	}()

	payloadRoot := filepath.Join(tempParent, "payload")
	opts := snapshotpayload.OptionsFromDescriptor(state.Descriptor)
	if err := snapshotpayload.MaterializeToNew(state.SnapshotDir, payloadRoot, opts, func(src, dst string) error {
		_, err := engine.CloneToNew(engine.NewCopyEngine(), src, dst)
		return err
	}); err != nil {
		return false, fmt.Errorf("read save point payload: %w", err)
	}
	if err := repo.ValidateManagedPayloadOnly(boundary, payloadRoot); err != nil {
		return false, err
	}

	if err := pathutil.ValidateNoSymlinkParents(payloadRoot, path); err != nil {
		return false, fmt.Errorf("path inside save point must not traverse symlinks: %w", err)
	}
	target := filepath.Join(payloadRoot, filepath.FromSlash(path))
	_, err = os.Lstat(target)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, fmt.Errorf("stat path inside save point: %w", err)
}

func historyPathNextCommands(path string, candidates []publicHistoryPathCandidate) []string {
	if len(candidates) == 0 {
		return []string{}
	}
	savePointID := candidates[0].SavePointID
	return []string{
		fmt.Sprintf("jvs view %s -- %s", savePointID, shellQuoteArg(path)),
	}
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

func printHistoryPathCandidates(result publicHistoryPathResult) {
	fmt.Printf("Folder: %s\n", result.Folder)
	fmt.Printf("Workspace: %s\n", result.Workspace)
	fmt.Printf("Candidates for path: %s\n", result.Path)
	if len(result.Candidates) == 0 {
		fmt.Println("No candidates found.")
		fmt.Println("No files were changed.")
		return
	}
	for _, candidate := range result.Candidates {
		message := candidate.Message
		if message == "" {
			message = color.Dim("(no message)")
		}
		fmt.Printf("%s  %s  %s\n",
			color.SnapshotID(candidate.SavePointID),
			color.Dim(candidate.CreatedAt.Format("2006-01-02 15:04")),
			message,
		)
	}
	fmt.Println("Next:")
	for _, command := range result.NextCommands {
		fmt.Printf("  %s\n", command)
	}
	fmt.Println("No files were changed.")
}

func historyPathError(err error) error {
	if err == nil {
		return nil
	}
	message := publicSavePointVocabulary(err.Error())
	if !strings.Contains(message, "No files were changed.") {
		message += ". No files were changed."
	}
	return fmt.Errorf("%s", message)
}

func loadHistoryResult(repoRoot, workspaceName string, request historyRequest) (publicHistoryResult, error) {
	mgr := worktree.NewManager(repoRoot)
	cfg, err := mgr.Get(workspaceName)
	if err != nil {
		return publicHistoryResult{}, fmt.Errorf("load workspace: %w", err)
	}

	switch request.direction {
	case historyDirectionCurrent:
		return loadCurrentHistoryResult(repoRoot, workspaceName, cfg)
	case historyDirectionTo:
		savePointID, err := resolvePublicSavePointID(repoRoot, request.saveRef)
		if err != nil {
			return publicHistoryResult{}, err
		}
		return loadHistoryToResult(repoRoot, workspaceName, cfg, savePointID)
	case historyDirectionFrom:
		startID, err := historyFromStartSavePoint(repoRoot, cfg, request.saveRef)
		if err != nil {
			return publicHistoryResult{}, err
		}
		return loadHistoryFromResult(repoRoot, workspaceName, cfg, startID)
	default:
		return publicHistoryResult{}, fmt.Errorf("unsupported history direction %q", request.direction)
	}
}

func loadCurrentHistoryResult(repoRoot, workspaceName string, cfg *model.WorktreeConfig) (publicHistoryResult, error) {
	currentPointer := workspaceCurrentPointer(cfg)
	savePoints, truncated, err := loadSavePointLineage(repoRoot, currentPointer, historyLimit, historyNoteFilter)
	if err != nil {
		return publicHistoryResult{}, err
	}
	result := baseHistoryResult(workspaceName, cfg, historyDirectionCurrent)
	result.SavePoints = publicSavePoints(savePoints)
	result.Truncated = truncated
	result.CurrentPointer = string(currentPointer)
	return result, nil
}

func loadHistoryToResult(repoRoot, workspaceName string, cfg *model.WorktreeConfig, targetID model.SnapshotID) (publicHistoryResult, error) {
	savePoints, truncated, err := loadSavePointLineage(repoRoot, targetID, historyLimit, historyNoteFilter)
	if err != nil {
		return publicHistoryResult{}, err
	}
	result := baseHistoryResult(workspaceName, cfg, historyDirectionTo)
	result.SavePoints = publicSavePoints(savePoints)
	result.TargetSavePoint = string(targetID)
	result.Truncated = truncated
	return result, nil
}

func loadHistoryFromResult(repoRoot, workspaceName string, cfg *model.WorktreeConfig, startID model.SnapshotID) (publicHistoryResult, error) {
	nodes, edges, truncated, err := loadHistoryDescendantTree(repoRoot, startID, historyLimit, historyNoteFilter)
	if err != nil {
		return publicHistoryResult{}, err
	}
	pointers, err := loadHistoryWorkspacePointers(repoRoot, workspaceName)
	if err != nil {
		return publicHistoryResult{}, err
	}
	result := baseHistoryResult(workspaceName, cfg, historyDirectionFrom)
	result.SavePoints = publicSavePoints(nodes)
	result.Nodes = publicSavePoints(nodes)
	result.Edges = edges
	result.WorkspacePointers = pointers
	result.StartSavePoint = string(startID)
	result.Truncated = truncated
	return result, nil
}

func baseHistoryResult(workspaceName string, cfg *model.WorktreeConfig, direction historyDirection) publicHistoryResult {
	result := publicHistoryResult{
		Workspace:                 workspaceName,
		Direction:                 string(direction),
		SavePoints:                []publicSavePointRecord{},
		Limit:                     historyLimit,
		WorkspaceHasOwnSavePoints: cfg.LatestSnapshotID != "",
	}
	if cfg.LatestSnapshotID != "" {
		result.NewestSavePoint = string(cfg.LatestSnapshotID)
	}
	if cfg.StartedFromSnapshotID != "" {
		result.StartedFromSavePoint = string(cfg.StartedFromSnapshotID)
	}
	return result
}

func workspaceCurrentPointer(cfg *model.WorktreeConfig) model.SnapshotID {
	if cfg == nil {
		return ""
	}
	if cfg.HeadSnapshotID != "" {
		return cfg.HeadSnapshotID
	}
	return cfg.LatestSnapshotID
}

func loadSavePointLineage(repoRoot string, startID model.SnapshotID, limit int, noteFilter string) ([]*model.Descriptor, bool, error) {
	if startID == "" {
		return []*model.Descriptor{}, false, nil
	}
	var savePoints []*model.Descriptor
	currentID := &startID
	for currentID != nil {
		desc, err := snapshot.LoadDescriptor(repoRoot, *currentID)
		if err != nil {
			return nil, false, fmt.Errorf("load save point: %w", err)
		}
		if noteFilter == "" || strings.Contains(desc.Note, noteFilter) {
			savePoints = append(savePoints, desc)
		}
		currentID = desc.ParentID
	}
	return limitHistoryDescriptors(savePoints, limit)
}

func historyFromStartSavePoint(repoRoot string, cfg *model.WorktreeConfig, rawRef string) (model.SnapshotID, error) {
	if strings.TrimSpace(rawRef) != "" {
		return resolvePublicSavePointID(repoRoot, rawRef)
	}
	if cfg.StartedFromSnapshotID != "" {
		return cfg.StartedFromSnapshotID, nil
	}
	currentPointer := workspaceCurrentPointer(cfg)
	if currentPointer == "" {
		return "", nil
	}
	return earliestParentAncestor(repoRoot, currentPointer)
}

func earliestParentAncestor(repoRoot string, startID model.SnapshotID) (model.SnapshotID, error) {
	earliest := startID
	currentID := &startID
	for currentID != nil {
		desc, err := snapshot.LoadDescriptor(repoRoot, *currentID)
		if err != nil {
			return "", fmt.Errorf("load save point: %w", err)
		}
		earliest = desc.SnapshotID
		currentID = desc.ParentID
	}
	return earliest, nil
}

type historyDescendantEdge struct {
	from     model.SnapshotID
	to       model.SnapshotID
	edgeType string
}

func loadHistoryDescendantTree(repoRoot string, startID model.SnapshotID, limit int, noteFilter string) ([]*model.Descriptor, []publicHistoryEdge, bool, error) {
	if startID == "" {
		return []*model.Descriptor{}, nil, false, nil
	}
	all, err := snapshot.Find(repoRoot, snapshot.FilterOptions{})
	if err != nil {
		return nil, nil, false, fmt.Errorf("list save points: %w", err)
	}
	descsByID := make(map[model.SnapshotID]*model.Descriptor, len(all))
	childrenByID := make(map[model.SnapshotID][]historyDescendantEdge)
	for _, desc := range all {
		descsByID[desc.SnapshotID] = desc
		if desc.ParentID != nil {
			edge := historyDescendantEdge{from: *desc.ParentID, to: desc.SnapshotID, edgeType: "parent"}
			childrenByID[edge.from] = append(childrenByID[edge.from], edge)
		}
		if desc.StartedFrom != nil {
			edge := historyDescendantEdge{from: *desc.StartedFrom, to: desc.SnapshotID, edgeType: "started_from"}
			childrenByID[edge.from] = append(childrenByID[edge.from], edge)
		}
	}
	if _, ok := descsByID[startID]; !ok {
		desc, err := snapshot.LoadDescriptor(repoRoot, startID)
		if err != nil {
			return nil, nil, false, fmt.Errorf("load save point: %w", err)
		}
		descsByID[startID] = desc
	}

	for parentID := range childrenByID {
		sort.Slice(childrenByID[parentID], func(i, j int) bool {
			left := descsByID[childrenByID[parentID][i].to]
			right := descsByID[childrenByID[parentID][j].to]
			return descriptorBefore(left, right)
		})
	}

	reachable := collectReachableHistoryNodes(startID, descsByID, childrenByID)
	if noteFilter != "" {
		reachable = filterHistoryFromNodes(startID, reachable, noteFilter)
	}
	selected, truncated := selectHistoryFromNodes(startID, reachable, limit)
	selectedByID := make(map[model.SnapshotID]bool, len(selected))
	for _, desc := range selected {
		selectedByID[desc.SnapshotID] = true
	}
	edges := selectedHistoryEdges(selectedByID, childrenByID)
	return selected, edges, truncated, nil
}

func collectReachableHistoryNodes(startID model.SnapshotID, descsByID map[model.SnapshotID]*model.Descriptor, childrenByID map[model.SnapshotID][]historyDescendantEdge) []*model.Descriptor {
	var nodes []*model.Descriptor
	seen := make(map[model.SnapshotID]bool)
	queue := []model.SnapshotID{startID}
	for len(queue) > 0 {
		currentID := queue[0]
		queue = queue[1:]
		if seen[currentID] {
			continue
		}
		seen[currentID] = true
		desc := descsByID[currentID]
		if desc == nil {
			continue
		}
		nodes = append(nodes, desc)
		for _, edge := range childrenByID[currentID] {
			queue = append(queue, edge.to)
		}
	}
	sortHistoryNodesAscending(nodes)
	return nodes
}

func filterHistoryFromNodes(startID model.SnapshotID, nodes []*model.Descriptor, noteFilter string) []*model.Descriptor {
	filtered := make([]*model.Descriptor, 0, len(nodes))
	for _, desc := range nodes {
		if desc.SnapshotID == startID || strings.Contains(desc.Note, noteFilter) {
			filtered = append(filtered, desc)
		}
	}
	return filtered
}

func selectHistoryFromNodes(startID model.SnapshotID, nodes []*model.Descriptor, limit int) ([]*model.Descriptor, bool) {
	if limit == 0 || len(nodes) <= limit {
		return nodes, false
	}
	if limit <= 0 {
		return nodes, false
	}
	var start *model.Descriptor
	others := make([]*model.Descriptor, 0, len(nodes))
	for _, desc := range nodes {
		if desc.SnapshotID == startID {
			start = desc
			continue
		}
		others = append(others, desc)
	}
	sort.Slice(others, func(i, j int) bool {
		return descriptorAfter(others[i], others[j])
	})
	selected := make([]*model.Descriptor, 0, limit)
	if start != nil {
		selected = append(selected, start)
	}
	remaining := limit - len(selected)
	if remaining > len(others) {
		remaining = len(others)
	}
	if remaining > 0 {
		selected = append(selected, others[:remaining]...)
	}
	sortHistoryNodesAscending(selected)
	return selected, true
}

func selectedHistoryEdges(selectedByID map[model.SnapshotID]bool, childrenByID map[model.SnapshotID][]historyDescendantEdge) []publicHistoryEdge {
	var edges []publicHistoryEdge
	for from, children := range childrenByID {
		if !selectedByID[from] {
			continue
		}
		for _, edge := range children {
			if !selectedByID[edge.to] {
				continue
			}
			edges = append(edges, publicHistoryEdge{
				From: string(edge.from),
				To:   string(edge.to),
				Type: edge.edgeType,
			})
		}
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		if edges[i].To != edges[j].To {
			return edges[i].To < edges[j].To
		}
		return edges[i].Type < edges[j].Type
	})
	return edges
}

func limitHistoryDescriptors(descs []*model.Descriptor, limit int) ([]*model.Descriptor, bool, error) {
	if limit == 0 || len(descs) <= limit {
		return descs, false, nil
	}
	if limit < 0 {
		return nil, false, fmt.Errorf("--limit must be 0 or greater")
	}
	return descs[:limit], true, nil
}

func sortHistoryNodesAscending(nodes []*model.Descriptor) {
	sort.Slice(nodes, func(i, j int) bool {
		return descriptorBefore(nodes[i], nodes[j])
	})
}

func descriptorBefore(left, right *model.Descriptor) bool {
	if left == nil || right == nil {
		return left != nil
	}
	if !left.CreatedAt.Equal(right.CreatedAt) {
		return left.CreatedAt.Before(right.CreatedAt)
	}
	return left.SnapshotID < right.SnapshotID
}

func descriptorAfter(left, right *model.Descriptor) bool {
	if left == nil || right == nil {
		return left != nil
	}
	if !left.CreatedAt.Equal(right.CreatedAt) {
		return left.CreatedAt.After(right.CreatedAt)
	}
	return left.SnapshotID > right.SnapshotID
}

func loadHistoryWorkspacePointers(repoRoot, activeWorkspace string) ([]publicHistoryWorkspacePointer, error) {
	configs, err := worktree.NewManager(repoRoot).List()
	if err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}
	sort.Slice(configs, func(i, j int) bool {
		return configs[i].Name < configs[j].Name
	})
	pointers := make([]publicHistoryWorkspacePointer, 0, len(configs))
	for _, cfg := range configs {
		pointer := workspaceCurrentPointer(cfg)
		if pointer == "" {
			continue
		}
		pointers = append(pointers, publicHistoryWorkspacePointer{
			Workspace:   cfg.Name,
			SavePointID: string(pointer),
			Active:      cfg.Name == activeWorkspace,
		})
	}
	return pointers, nil
}

func printHistoryResult(result publicHistoryResult) {
	switch historyDirection(result.Direction) {
	case historyDirectionFrom:
		printHistoryFromResult(result)
	case historyDirectionTo:
		printHistoryToResult(result)
	default:
		printCurrentHistoryResult(result)
	}
}

func printCurrentHistoryResult(result publicHistoryResult) {
	if result.StartedFromSavePoint != "" {
		fmt.Printf("Workspace started from %s.\n", color.SnapshotID(result.StartedFromSavePoint))
	}
	if result.CurrentPointer != "" && (!result.WorkspaceHasOwnSavePoints || result.CurrentPointer != result.NewestSavePoint) {
		fmt.Printf("Current pointer: %s\n", color.SnapshotID(result.CurrentPointer))
	}
	if result.CurrentPointer != "" && !result.WorkspaceHasOwnSavePoints {
		fmt.Println("Workspace has not created its own save point yet.")
	}
	if len(result.SavePoints) == 0 {
		fmt.Println("No save points yet.")
		return
	}
	fmt.Printf("Save points for workspace %s:\n", result.Workspace)
	printHistorySavePointRecords(result.SavePoints)
	printHistoryTruncationHint(result)
}

func printHistoryToResult(result publicHistoryResult) {
	fmt.Printf("History to %s:\n", color.SnapshotID(result.TargetSavePoint))
	if len(result.SavePoints) == 0 {
		fmt.Println("No save points matched.")
		return
	}
	printHistorySavePointRecords(result.SavePoints)
	printHistoryTruncationHint(result)
}

func printHistoryFromResult(result publicHistoryResult) {
	fmt.Printf("History from %s:\n", color.SnapshotID(result.StartSavePoint))
	if len(result.Nodes) == 0 {
		fmt.Println("No save points found.")
		return
	}
	labelsBySavePoint := historyPointerLabels(result.WorkspacePointers)
	for _, record := range result.Nodes {
		message := record.Message
		if message == "" {
			message = color.Dim("(no message)")
		}
		labels := labelsBySavePoint[record.SavePointID]
		if len(labels) > 0 {
			message += " " + color.Dim(strings.Join(labels, " "))
		}
		fmt.Printf("%s  %s  %s\n",
			color.SnapshotID(model.SnapshotID(record.SavePointID).ShortID()),
			color.Dim(record.CreatedAt.Format("2006-01-02 15:04")),
			message,
		)
	}
	printHistoryTruncationHint(result)
}

func printHistorySavePointRecords(records []publicSavePointRecord) {
	for _, record := range records {
		message := record.Message
		if message == "" {
			message = color.Dim("(no message)")
		}
		fmt.Printf("%s  %s  %s\n",
			color.SnapshotID(model.SnapshotID(record.SavePointID).ShortID()),
			color.Dim(record.CreatedAt.Format("2006-01-02 15:04")),
			message,
		)
	}
}

func printHistoryTruncationHint(result publicHistoryResult) {
	if result.Truncated {
		fmt.Printf("Showing %d save points. Use --limit 0 to show all.\n", len(result.SavePoints))
	}
}

func historyPointerLabels(pointers []publicHistoryWorkspacePointer) map[string][]string {
	labels := make(map[string][]string)
	for _, pointer := range pointers {
		label := "[" + pointer.Workspace + "]"
		labels[pointer.SavePointID] = append(labels[pointer.SavePointID], label)
	}
	for savePointID := range labels {
		sort.Strings(labels[savePointID])
	}
	return labels
}

func init() {
	historyCmd.Flags().IntVarP(&historyLimit, "limit", "n", defaultHistoryLimit, "limit number of save points (0 = no limit)")
	historyCmd.Flags().StringVarP(&historyNoteFilter, "grep", "g", "", "filter by message substring")
	historyCmd.Flags().StringVar(&historyPath, "path", "", "find save points that contain a workspace-relative path")
	rootCmd.AddCommand(historyCmd)
}
