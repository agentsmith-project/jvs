package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/agentsmith-project/jvs/internal/engine"
	"github.com/agentsmith-project/jvs/internal/repo"
	"github.com/agentsmith-project/jvs/internal/snapshot"
	"github.com/agentsmith-project/jvs/pkg/config"
	"github.com/agentsmith-project/jvs/pkg/fsutil"
	"github.com/agentsmith-project/jvs/pkg/model"
	"github.com/agentsmith-project/jvs/pkg/uuidutil"
)

var newTransferEngine = engine.NewEngine

func existingDirectory(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("stat directory: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("path is not a directory: %s", abs)
	}
	return filepath.Clean(abs), nil
}

func rejectContainsJVS(root string) error {
	var found string
	errContainsJVS := errors.New("source contains .jvs")
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Name() == repo.JVSDirName {
			found = path
			return errContainsJVS
		}
		return nil
	})
	if errors.Is(err, errContainsJVS) {
		return fmt.Errorf("source must not contain %s metadata: %s", repo.JVSDirName, found)
	}
	if err != nil {
		return fmt.Errorf("scan source: %w", err)
	}
	return nil
}

func rejectDangerousOverlap(aLabel, aPath, bLabel, bPath string) error {
	aResolved, err := resolveSetupPath(aPath)
	if err != nil {
		return fmt.Errorf("resolve %s path: %w", aLabel, err)
	}
	bResolved, err := resolveSetupPath(bPath)
	if err != nil {
		return fmt.Errorf("resolve %s path: %w", bLabel, err)
	}

	if pathContains(aResolved.Physical, bResolved.Physical) || pathContains(bResolved.Physical, aResolved.Physical) {
		return fmt.Errorf(
			"dangerous physical path overlap between %s (%s -> %s) and %s (%s -> %s)",
			aLabel,
			aResolved.Lexical,
			aResolved.Physical,
			bLabel,
			bResolved.Lexical,
			bResolved.Physical,
		)
	}
	return nil
}

type setupPathResolution struct {
	Lexical  string
	Physical string
}

func resolveSetupPath(path string) (setupPathResolution, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return setupPathResolution{}, fmt.Errorf("resolve absolute path: %w", err)
	}
	abs = filepath.Clean(abs)
	physical, err := resolvePhysicalPath(abs)
	if err != nil {
		return setupPathResolution{}, err
	}
	return setupPathResolution{
		Lexical:  abs,
		Physical: physical,
	}, nil
}

func resolvePhysicalPath(abs string) (string, error) {
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
			parts := append([]string{filepath.Clean(resolvedAncestor)}, suffix...)
			return filepath.Clean(filepath.Join(parts...)), nil
		}
		if err != nil && !os.IsNotExist(err) {
			return "", fmt.Errorf("resolve symlinks for existing ancestor %s: %w", ancestor, err)
		}
	}
}

func pathContains(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && !filepath.IsAbs(rel)
}

func planTransfer(source, dest, repoRoot string) (*engine.TransferPlan, error) {
	destinationPath := dest
	if _, err := os.Stat(destinationPath); os.IsNotExist(err) {
		destinationPath = filepath.Dir(destinationPath)
	}
	return engine.PlanTransfer(engine.TransferPlanRequest{
		SourcePath:      source,
		DestinationPath: destinationPath,
		CapabilityPath:  repoRoot,
		RequestedEngine: requestedTransferEngine(repoRoot),
	})
}

func requestedTransferEngine(repoRoot string) model.EngineType {
	if selected, ok := snapshotEngineFromEnv(); ok {
		return selected
	}
	if selected, ok := legacyEngineFromEnv(); ok {
		return selected
	}
	if cfg, err := config.Load(repoRoot); err == nil && cfg.DefaultEngine != "" {
		return cfg.DefaultEngine
	}
	return engine.EngineAuto
}

func cloneDirectory(source, dest string, plan *engine.TransferPlan) (*engine.CloneResult, error) {
	eng := newTransferEngine(plan.TransferEngine)
	result, err := engine.CloneToNew(eng, source, dest)
	if err != nil {
		return nil, err
	}
	completeTransferPlan(plan, result)
	return result, nil
}

func transferIntoMainWorkspace(source, mainWorkspace, repoRoot string) (*engine.TransferPlan, error) {
	stagingPath, err := mainWorkspaceTransferStagingPath(repoRoot)
	if err != nil {
		return nil, err
	}
	cleanupStaging := true
	defer func() {
		if cleanupStaging {
			_ = os.RemoveAll(stagingPath)
		}
	}()

	transferPlan, err := planTransfer(source, stagingPath, repoRoot)
	if err != nil {
		return nil, fmt.Errorf("plan transfer: %w", err)
	}
	if _, err := cloneDirectory(source, stagingPath, transferPlan); err != nil {
		return nil, fmt.Errorf("clone into staging: %w", err)
	}
	if err := publishMainWorkspaceStaging(stagingPath, mainWorkspace); err != nil {
		return nil, fmt.Errorf("publish main workspace: %w", err)
	}
	cleanupStaging = false
	return transferPlan, nil
}

func mainWorkspaceTransferStagingPath(repoRoot string) (string, error) {
	for range 16 {
		path := filepath.Join(repoRoot, ".main.transfer-"+uuidutil.NewV4()[:8])
		if _, err := os.Lstat(path); os.IsNotExist(err) {
			return path, nil
		} else if err != nil {
			return "", fmt.Errorf("stat transfer staging: %w", err)
		}
	}
	return "", fmt.Errorf("allocate transfer staging path")
}

func publishMainWorkspaceStaging(stagingPath, mainWorkspace string) error {
	if err := validateExistingEmptyDirectory(mainWorkspace); err != nil {
		return err
	}
	if err := os.Remove(mainWorkspace); err != nil {
		return fmt.Errorf("remove empty main workspace: %w", err)
	}
	if err := fsutil.RenameNoReplaceAndSync(stagingPath, mainWorkspace); err != nil {
		return fmt.Errorf("rename staging into main workspace: %w", err)
	}
	return nil
}

func validateExistingEmptyDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("stat directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("directory is symlink: %s", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("path is not directory: %s", path)
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return fmt.Errorf("read directory: %w", err)
	}
	if len(entries) != 0 {
		return fmt.Errorf("directory must be empty: %s", path)
	}
	return nil
}

func createInitialCheckpoint(repoRoot, note string, tags []string) (*model.Descriptor, error) {
	creator := snapshot.NewCreator(repoRoot, detectEngine(repoRoot))
	desc, err := creator.Create("main", note, tags)
	if err != nil {
		return nil, err
	}
	if err := verifyLifecycleCheckpoint(repoRoot, desc.SnapshotID); err != nil {
		return nil, err
	}
	return desc, nil
}

func effectiveTransferMode(engineType model.EngineType, result *engine.CloneResult) string {
	if result == nil {
		return string(engineType)
	}
	if engineType != model.EngineCopy && result.Degraded {
		return string(model.EngineCopy)
	}
	return string(engineType)
}

func completeTransferPlan(plan *engine.TransferPlan, result *engine.CloneResult) {
	if plan == nil {
		return
	}
	plan.DegradedReasons = appendUniqueStrings(plan.DegradedReasons, degradedReasons(result)...)
	if result != nil && result.Degraded && plan.TransferEngine != model.EngineCopy {
		plan.EffectiveEngine = model.EngineCopy
		plan.OptimizedTransfer = false
		return
	}
	plan.EffectiveEngine = plan.TransferEngine
	plan.OptimizedTransfer = transferIsOptimized(plan.TransferEngine)
}

func transferIsOptimized(engineType model.EngineType) bool {
	return engineType == model.EngineJuiceFSClone || engineType == model.EngineReflinkCopy
}

func degradedReasons(result *engine.CloneResult) []string {
	if result == nil || len(result.Degradations) == 0 {
		return []string{}
	}
	return result.Degradations
}

func appendUniqueStrings(base []string, values ...string) []string {
	seen := make(map[string]struct{}, len(base)+len(values))
	out := make([]string, 0, len(base)+len(values))
	for _, value := range append(append([]string{}, base...), values...) {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

type setupMaterialization struct {
	EffectiveEngine      model.EngineType
	MetadataPreservation model.MetadataPreservation
	PerformanceClass     string
}

func applySetupJSONFields(output map[string]any, capabilities *engine.CapabilityReport, effectiveEngine model.EngineType, warnings []string) {
	applySetupMaterializationJSONFields(output, capabilities, setupMaterializationForEngine(effectiveEngine), warnings)
}

func applySetupMaterializationJSONFields(output map[string]any, capabilities *engine.CapabilityReport, materialization setupMaterialization, warnings []string) {
	output["capabilities"] = capabilities
	output["effective_engine"] = materialization.EffectiveEngine
	output["metadata_preservation"] = materialization.MetadataPreservation
	output["performance_class"] = materialization.PerformanceClass
	output["warnings"] = stableStringSlice(warnings)
}

func setupMaterializationForEngine(effectiveEngine model.EngineType) setupMaterialization {
	return setupMaterialization{
		EffectiveEngine:      effectiveEngine,
		MetadataPreservation: engine.MetadataPreservationForEngine(effectiveEngine),
		PerformanceClass:     engine.PerformanceClassForEngine(effectiveEngine),
	}
}

func setupMaterializationFromDescriptor(desc *model.Descriptor, fallback model.EngineType) setupMaterialization {
	effectiveEngine := fallback
	metadataPreservation := model.MetadataPreservation{}
	performanceClass := ""

	if desc != nil {
		effectiveEngine = desc.EffectiveEngine
		if effectiveEngine == "" {
			effectiveEngine = desc.ActualEngine
		}
		if effectiveEngine == "" {
			effectiveEngine = desc.Engine
		}
		if desc.MetadataPreservation != nil {
			metadataPreservation = *desc.MetadataPreservation
		}
		performanceClass = desc.PerformanceClass
	}

	if effectiveEngine == "" {
		effectiveEngine = fallback
	}
	if metadataPreservation == (model.MetadataPreservation{}) {
		metadataPreservation = engine.MetadataPreservationForEngine(effectiveEngine)
	}
	if performanceClass == "" {
		performanceClass = engine.PerformanceClassForEngine(effectiveEngine)
	}

	return setupMaterialization{
		EffectiveEngine:      effectiveEngine,
		MetadataPreservation: metadataPreservation,
		PerformanceClass:     performanceClass,
	}
}

func applyTransferJSONFields(output map[string]any, plan *engine.TransferPlan, materializationDesc *model.Descriptor) {
	if plan == nil {
		return
	}
	output["requested_engine"] = plan.RequestedEngine
	output["transfer_engine"] = plan.TransferEngine
	output["transfer_mode"] = string(plan.EffectiveEngine)
	output["optimized_transfer"] = plan.OptimizedTransfer
	output["degraded_reasons"] = stableStringSlice(plan.DegradedReasons)
	applySetupMaterializationJSONFields(output, plan.Capabilities, setupMaterializationFromDescriptor(materializationDesc, plan.EffectiveEngine), plan.Warnings)
}

func stableStringSlice(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	return values
}
