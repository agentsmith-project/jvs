//go:build conformance

package conformance

import (
	"bufio"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/agentsmith-project/jvs/internal/doctor"
)

var historyBareJQSelector = regexp.MustCompile(`jq\s+-r\s+['"]\.\[(?:\]|[0-9]+)`)
var traceabilityDocRefPattern = regexp.MustCompile("`((?:README\\.md|docs/[^`]+\\.md))`")
var markdownDocLinkPattern = regexp.MustCompile(`\]\(([^)#]+\.md)(?:#[^)]+)?\)`)
var stalePublicReleaseVocabularyPattern = regexp.MustCompile(`\bv7\.(?:[0-9]+|x)\b`)
var makefileCoverageThresholdPattern = regexp.MustCompile(`if\s*\(\$\$3\+0\s*<\s*([0-9]+)\)`)
var releaseReadinessHeadingPattern = regexp.MustCompile(`(?m)^## v0\.[0-9]+\.[0-9]+ - [0-9]{4}-[0-9]{2}-[0-9]{2}$`)
var releaseEvidenceHeadingPattern = regexp.MustCompile(`(?m)^## (v0\.[0-9]+\.[0-9]+) - ([0-9]{4}-[0-9]{2}-[0-9]{2})$`)
var releaseEvidenceClassPattern = regexp.MustCompile("(?mi)^- Evidence class:\\s*`?([^`\\r\\n]+?)`?\\s*$")
var releaseEvidenceCommitPattern = regexp.MustCompile("(?m)^- Final tagged commit: `([0-9a-f]{40})`$")
var releaseEvidenceTagPattern = regexp.MustCompile("(?m)^- Tag: `(v0\\.[0-9]+\\.[0-9]+)`$")
var releaseEvidenceStatusPassPattern = regexp.MustCompile(`(?mi)^- Status:\s*PASS\b`)
var releaseEvidenceGatePassPattern = regexp.MustCompile(`(?mi)\|\s*Release gate\s*\|[^|]*\|\s*PASS\b[^|]*\|`)
var releaseEvidencePublishedArtifactCountPattern = regexp.MustCompile(`(?mi)^- Published artifact count:\s*` + "`?" + `[1-9][0-9]*` + "`?")
var releaseFacingPerformanceClaimPattern = regexp.MustCompile(`(?i)(^|[^A-Za-z0-9_])(?:o\(1\)|instant(?:ly)?|constant-time|constant overhead)([^A-Za-z0-9_]|$)`)
var releaseFacingStorageScopePattern = regexp.MustCompile(`(?i)(^|[^A-Za-z0-9_-])juicefs-clone([^A-Za-z0-9_-]|$)|\bsupported\s+[^A-Za-z0-9_]*juicefs\b`)
var negatedReleaseFacingPerformanceClaimPattern = regexp.MustCompile(`(?i)\bnot\s+(?:an?\s+)?(?:o\(1\)|instant(?:ly)?|constant-time|constant overhead)(?:[^A-Za-z0-9_]|$)`)
var portableLatencyPromisePattern = regexp.MustCompile(`(?i)^\s*\|\s*\d+(?:\.\d+)?\s*(?:kb|mb|gb|tb|kib|mib|gib|tib)\b[^|]*\|.*\b\d+(?:\.\d+)?\s*(?:ms|s|sec|secs|second|seconds)\b`)
var documentedEngineConstantPattern = regexp.MustCompile(`(?m)^\s*(Engine[A-Za-z0-9_]+)\s+EngineType\s*=\s*"([^"]+)"`)
var savePointReferenceClaimPattern = regexp.MustCompile(`(?i)\bsave points?\b.*(?:\bsmall reference files\b|\breferences?,\s*not\s+(?:data\s+)?cop(?:y|ies)\b|\b(?:is|are)\s+(?:a\s+)?(?:metadata\s+)?references?\b)`)
var releaseFacingLegacyProductNounPattern = regexp.MustCompile(`(?i)(^|[^A-Za-z0-9_/])(?:checkpoints?|worktrees?|dirty)([^A-Za-z0-9_/]|$)`)
var staleCurrentRuntimeLockTerminologyPattern = regexp.MustCompile(`(?i)\b(runtime locks?|locks and intents)\b`)
var numberedConformanceTestRef = regexp.MustCompile(`\b[Tt]ests?\s+\d`)
var markdownBulletFieldPattern = regexp.MustCompile("^\\s*-\\s+`([A-Za-z0-9_]+)`")
var backtickedFieldPattern = regexp.MustCompile("`([A-Za-z0-9_]+)`")
var jsonTagFieldPattern = regexp.MustCompile("json:\"([A-Za-z0-9_]+)(?:,[^\"]*)?\"")
var wholeJVSMetadataPathPattern = regexp.MustCompile("`?\\.jvs/`?(?:\\s|$|[),.;:])")
var pathShapedRuntimeMechanismPattern = regexp.MustCompile("`?\\.jvs/(?:locks|intents|gc)(?:/|\\*|\\b)")
var staleCleanupProtectionVocabularyPattern = regexp.MustCompile(`(?i)\b(?:views?/)?source\s+reads?\b`)
var doctorEngineSurfacePattern = regexp.MustCompile("(?i)(?:`?jvs\\s+doctor`?|`?doctor`?)[^.]*\\b(?:engine\\s+(?:visibility|probes?|summary)|capabilit(?:y|ies))\\b|\\b(?:engine\\s+(?:visibility|probes?|summary)|capabilit(?:y|ies))\\b[^.]*`?(?:jvs\\s+doctor|doctor)`?")
var documentedCleanupProtectionReasonPattern = regexp.MustCompile(`(?m)^\s*(CleanupProtectionReason[A-Za-z0-9_]+)\s+CleanupProtectionReason\s*=\s*"([^"]+)"`)
var runtimeRepairActionIDPattern = regexp.MustCompile("`((?:clean|rebind)_[A-Za-z0-9_]+)`")
var staleMigrationRuntimeExcludePattern = regexp.MustCompile(`(?i)\bexclud(?:e|es|ed|ing)\b[^.\n]*(?:runtime\s+(?:cleanup\s+)?state|(?:runtime\s+)?cleanup\s+plan\s+files?)|(?:runtime\s+(?:cleanup\s+)?state|(?:runtime\s+)?cleanup\s+plan\s+files?)[^.\n]*\bexclud(?:e|es|ed|ing)\b`)
var staleMigrationSyncVocabularyPattern = regexp.MustCompile(`(?i)\b(?:sync|syncs|synced|syncing|synchroni[sz](?:e|es|ed|ing|ation))\b`)
var userDocTypedPlaceholderPattern = regexp.MustCompile(`<[a-z][a-z0-9]*(?:-[a-z0-9]+)+>`)
var implicitWorkspaceNewBareNamePattern = regexp.MustCompile(`\bjvs\s+workspace\s+new\s+[A-Za-z0-9][A-Za-z0-9_-]*\s+--from\b`)
var userDocRemoveWorkspaceMentalModelPattern = regexp.MustCompile(`(?i)\bremov(?:e|es|ed|ing)\s+(?:a|the|that|this|selected\s+)?workspace\b|\bworkspace\s+folders?\b[^.\n]*\bremoved\b`)

type unsupportedPublicCLIExampleRule struct {
	name        string
	pattern     *regexp.Regexp
	replacement string
}

type markdownCodeBlock struct {
	startLine int
	text      string
}

type shellCopyCommand struct {
	command     string
	stepIndex   int
	lineIndex   int
	source      string
	destination string
}

type shellCommandStep struct {
	index     int
	lineIndex int
	fields    []string
	opAfter   string
}

func TestDocs_PublicTerminologyContract(t *testing.T) {
	for _, doc := range activePublicContractDocs() {
		t.Run(doc, func(t *testing.T) {
			scanPublicDocLines(t, doc, func(lineNo int, line string) {
				if releaseFacingLegacyProductNounPattern.MatchString(line) {
					t.Fatalf("%s:%d exposes legacy public product noun:\n%s", doc, lineNo, line)
				}
				normalizedLine := strings.ToLower(line)
				for _, forbidden := range publicDocForbiddenTerms() {
					if strings.Contains(normalizedLine, strings.ToLower(forbidden)) {
						t.Fatalf("%s:%d exposes legacy public term %q:\n%s", doc, lineNo, forbidden, line)
					}
				}
			})
		})
	}
}

func TestConformancePublicProfileUsesStableCommands(t *testing.T) {
	for _, dir := range []string{"test/conformance"} {
		entries, err := os.ReadDir(repoFile(t, dir))
		if err != nil {
			t.Fatalf("read %s: %v", dir, err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			if strings.Contains(path, "compat") || strings.Contains(path, "legacy") {
				continue
			}
			assertTestFileUsesStableCommands(t, path)
		}
	}
}

func TestDocs_ConformanceContractTestNamesUseCurrentPublicVocabulary(t *testing.T) {
	staleNameFragments := []string{
		"CheckpointList",
		"CheckpointReference",
		"VerifyAllContract",
		"WorktreeFork",
	}
	for _, path := range []string{
		"test/conformance/public_contract_static_test.go",
		"test/conformance/performance_evidence_static_test.go",
	} {
		t.Run(path, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, repoFile(t, path), nil, 0)
			if err != nil {
				t.Fatalf("parse %s: %v", path, err)
			}
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Name == nil || !strings.HasPrefix(fn.Name.Name, "TestDocs_") {
					continue
				}
				for _, fragment := range staleNameFragments {
					if strings.Contains(fn.Name.Name, fragment) {
						pos := fset.Position(fn.Name.Pos())
						t.Fatalf("%s:%d docs contract test %s uses stale public vocabulary fragment %q", path, pos.Line, fn.Name.Name, fragment)
					}
				}
			}
		})
	}
}

func TestDocs_HistoryJSONExamplesUseEnvelope(t *testing.T) {
	for _, doc := range stablePublicDocs() {
		t.Run(doc, func(t *testing.T) {
			path := repoFile(t, doc)
			file, err := os.Open(path)
			if err != nil {
				t.Fatalf("open %s: %v", doc, err)
			}
			defer file.Close()

			scanner := bufio.NewScanner(file)
			lineNo := 0
			for scanner.Scan() {
				lineNo++
				line := scanner.Text()
				if !strings.Contains(line, "jvs history --json") || !strings.Contains(line, "jq") {
					continue
				}
				if historyBareJQSelector.MatchString(line) {
					t.Fatalf("%s:%d treats history JSON as a top-level array; select from .data instead:\n%s", doc, lineNo, line)
				}
			}
			if err := scanner.Err(); err != nil {
				t.Fatalf("scan %s: %v", doc, err)
			}
		})
	}
}

func TestDocs_HistoryTagIsNotGAPublicSurface(t *testing.T) {
	for _, doc := range historyTagPublicSurfaceDocs() {
		t.Run(doc, func(t *testing.T) {
			scanPublicDocLines(t, doc, func(lineNo int, line string) {
				lower := strings.ToLower(line)
				if strings.Contains(lower, "--tag") {
					t.Fatalf("%s:%d exposes non-GA history tag flag:\n%s", doc, lineNo, line)
				}
				for _, fragment := range []string{
					"history --tag",
					"jvs history --tag",
					"tags are discovery filters",
					"tag lifecycle",
				} {
					if strings.Contains(lower, fragment) {
						t.Fatalf("%s:%d exposes history tag lifecycle/discovery surface %q:\n%s", doc, lineNo, fragment, line)
					}
				}
			})
		})
	}
}

func TestDocs_CleanupDoesNotPromiseHistoryPruningOrWorkspaceDeletion(t *testing.T) {
	for _, doc := range cleanupPublicSurfaceDocs() {
		t.Run(doc, func(t *testing.T) {
			body := releaseFacingClaimBody(t, doc)
			lowerBody := strings.ToLower(body)
			for _, required := range []string{
				"save point storage",
				"reviewed",
			} {
				if !strings.Contains(lowerBody, required) {
					t.Fatalf("%s must describe cleanup as reviewed deletion of save point storage; missing %q", doc, required)
				}
			}

			scanPublicDocLines(t, doc, func(lineNo int, line string) {
				lower := strings.ToLower(line)
				if !strings.Contains(lower, "cleanup") {
					return
				}
				for _, forbidden := range []string{
					"history pruning",
					"prune history",
					"prunes history",
					"workspace deletion",
					"delete workspace folders",
					"deletes workspace folders",
					"cache cleanup",
					"clean cache",
					"retention policy",
					"retention rules",
				} {
					if strings.Contains(lower, forbidden) && !cleanupLineNegatesPromise(lower) {
						t.Fatalf("%s:%d lets cleanup imply history pruning, retention, cache cleanup, or workspace deletion via %q:\n%s",
							doc, lineNo, forbidden, line)
					}
				}
			})
		})
	}
}

func TestDocs_UserCommandsDocumentsWorkspaceManagement(t *testing.T) {
	doc := "docs/user/commands.md"
	body := readRepoFile(t, doc)
	section := markdownSectionByHeading(t, doc, body, "## `jvs workspace`")
	for _, required := range []string{
		"jvs workspace list",
		"jvs workspace path [name]",
		"jvs workspace rename <old> <new>",
		"jvs workspace move <name> <new-folder>",
		"jvs workspace move --run <workspace-move-plan-id>",
		"jvs workspace new <folder> --from <save>",
		"--name <name>",
		"jvs workspace delete <name>",
		"jvs workspace delete --run <workspace-delete-plan-id>",
		"preview-first",
		"does not remove save point storage",
		"jvs cleanup preview",
	} {
		requireReleaseReadinessText(t, "workspace command documentation", section, required)
	}
	if strings.Contains(section, "jvs workspace remove") {
		t.Fatalf("%s workspace command section exposes old remove command:\n%s", doc, section)
	}
	if strings.Contains(section, "jvs workspace delete --run <plan-id>") || strings.Contains(section, "jvs workspace move --run <plan-id>") {
		t.Fatalf("%s workspace command section uses generic workspace lifecycle plan placeholder:\n%s", doc, section)
	}
	if strings.Contains(strings.ToLower(section), "worktree") {
		t.Fatalf("%s workspace command section leaks old worktree vocabulary:\n%s", doc, section)
	}
}

func TestDocs_UserDocsUseDeleteOrDetachWorkspaceVocabulary(t *testing.T) {
	for _, doc := range markdownDocsUnder(t, "docs/user") {
		t.Run(doc, func(t *testing.T) {
			scanPublicDocLines(t, doc, func(lineNo int, line string) {
				if strings.Contains(strings.ToLower(line), "jvs workspace remove") ||
					userDocRemoveWorkspaceMentalModelPattern.MatchString(line) {
					t.Fatalf("%s:%d exposes old remove-workspace mental model; use delete/detach vocabulary:\n%s", doc, lineNo, line)
				}
			})
		})
	}
}

func TestDocs_UserCommandsDocumentsRepoLifecycleManagement(t *testing.T) {
	doc := "docs/user/commands.md"
	body := readRepoFile(t, doc)
	section := markdownSectionByHeading(t, doc, body, "## `jvs repo`")
	for _, required := range []string{
		"jvs repo clone <target-folder>",
		"jvs repo move <new-folder>",
		"jvs repo move --run <repo-move-plan-id>",
		"jvs repo rename <new-folder-name>",
		"jvs repo rename --run <repo-rename-plan-id>",
		"jvs repo detach",
		"jvs repo detach --run <repo-detach-plan-id>",
		"repo_id",
		"save point history",
		"preview-first",
		"basename",
	} {
		requireReleaseReadinessText(t, "repo command documentation", section, required)
	}
	if strings.Contains(section, "jvs repo remove") {
		t.Fatalf("%s repo command section exposes old remove command:\n%s", doc, section)
	}
}

func TestDocs_PublicDocsUseExplicitWorkspaceNewFolderSyntax(t *testing.T) {
	for _, doc := range activePublicContractDocs() {
		if doc == "CONTRIBUTING.md" {
			continue
		}
		t.Run(doc, func(t *testing.T) {
			scanPublicDocLines(t, doc, func(lineNo int, line string) {
				if strings.Contains(line, "jvs workspace new <name> --from") {
					t.Fatalf("%s:%d teaches old implicit workspace-name folder creation; use <folder> and --name only as an override:\n%s", doc, lineNo, line)
				}
				if implicitWorkspaceNewBareNamePattern.MatchString(line) {
					t.Fatalf("%s:%d teaches workspace creation with a bare name-shaped folder; use an explicit path such as ../experiment:\n%s", doc, lineNo, line)
				}
			})
		})
	}
}

func TestDocs_PublicDocsDoNotTeachHistoryAll(t *testing.T) {
	for _, doc := range activePublicContractDocs() {
		if doc == "CONTRIBUTING.md" {
			continue
		}
		t.Run(doc, func(t *testing.T) {
			scanPublicDocLines(t, doc, func(lineNo int, line string) {
				if strings.Contains(line, "jvs history --all") {
					t.Fatalf("%s:%d teaches removed history --all; use history to/from and --limit/-n:\n%s", doc, lineNo, line)
				}
			})
		})
	}
}

func TestDocs_UserDocsTeachWorkspaceFolderAndHistoryDirections(t *testing.T) {
	combined := ""
	for _, doc := range []string{
		"docs/user/README.md",
		"docs/user/quickstart.md",
		"docs/user/concepts.md",
		"docs/user/commands.md",
		"docs/user/examples.md",
		"docs/user/tutorials.md",
		"docs/user/faq.md",
		"docs/user/troubleshooting.md",
		"docs/user/best-practices.md",
	} {
		combined += "\n" + readRepoFile(t, doc)
	}
	for _, required := range []string{
		"jvs workspace new <folder> --from <save>",
		"jvs workspace new ../experiment --from",
		"jvs history to <save>",
		"jvs history from [<save>]",
		"--limit",
		"--limit 0",
	} {
		requireReleaseReadinessText(t, "user workspace/history documentation", combined, required)
	}
}

func TestDocs_UserFacingDocsUseTypedPlanIDPlaceholders(t *testing.T) {
	for _, doc := range userFacingPlanPlaceholderDocs(t) {
		t.Run(doc, func(t *testing.T) {
			for lineNo, line := range strings.Split(readRepoFile(t, doc), "\n") {
				if strings.Contains(line, "<plan-id>") {
					t.Fatalf("%s:%d uses generic <plan-id>; user docs must distinguish <restore-plan-id>, <workspace-delete-plan-id>, <workspace-move-plan-id>, and <cleanup-plan-id>:\n%s", doc, lineNo+1, line)
				}
			}
		})
	}
}

func TestDocs_UserWorkflowPagesExplainTypedPlaceholders(t *testing.T) {
	for _, doc := range userWorkflowPlaceholderDocs() {
		t.Run(doc, func(t *testing.T) {
			body := readRepoFile(t, doc)
			for _, placeholder := range userDocTypedPlaceholders(body) {
				if canonical, ok := userDocCanonicalWorkflowPlaceholderAlias(placeholder); ok {
					t.Fatalf("%s uses typed placeholder %s; use %s for the same workflow value", doc, placeholder, canonical)
				}
				if userDocExplainsPlaceholder(body, placeholder) || userDocLinksPlaceholderExplanation(t, doc, body, placeholder) {
					continue
				}
				t.Fatalf("%s uses typed placeholder %s but does not explain it or link to a user-doc explanation", doc, placeholder)
			}
		})
	}
}

func TestDocs_ReleaseBlockingManifestIncludesPublicDocs(t *testing.T) {
	docs := releaseBlockingContractDocs()
	for _, want := range []string{
		"docs/01_REPO_LAYOUT_SPEC.md",
		"docs/QUICKSTART.md",
		"docs/PRODUCT_PLAN.md",
		"docs/CONSTITUTION.md",
		"docs/03_WORKTREE_SPEC.md",
		"docs/04_SNAPSHOT_SCOPE_AND_LINEAGE_SPEC.md",
		"docs/05_SNAPSHOT_ENGINE_SPEC.md",
		"docs/06_RESTORE_SPEC.md",
		"docs/08_GC_SPEC.md",
		"docs/ARCHITECTURE.md",
		"docs/18_MIGRATION_AND_BACKUP.md",
		"docs/14_TRACEABILITY_MATRIX.md",
		"docs/13_OPERATION_RUNBOOK.md",
		"docs/RELEASE_EVIDENCE.md",
		"CONTRIBUTING.md",
		"SECURITY.md",
		"docs/SIGNING.md",
	} {
		if !staticStringSliceContains(docs, want) {
			t.Fatalf("release-blocking docs manifest must include %s", want)
		}
	}
}

func TestDocs_ReleaseBlockingManifestIncludesLinkedActiveMarkdownDocs(t *testing.T) {
	docs := releaseBlockingContractDocs()
	for _, link := range activePublicMarkdownDocLinks(t) {
		if staticStringSliceContains(nonReleaseFacingDocs(), link.target) {
			continue
		}
		if !staticStringSliceContains(docs, link.target) {
			t.Fatalf("release-blocking docs manifest must include markdown doc %s linked from active public doc %s", link.target, link.source)
		}
	}
}

func TestDocs_PublicCommandManifestCoversReleaseBlockingDocs(t *testing.T) {
	commandDocs := publicCommandDocs()
	for _, doc := range releaseBlockingContractDocs() {
		if !staticStringSliceContains(commandDocs, doc) {
			t.Fatalf("public command docs manifest must cover release-blocking doc %s", doc)
		}
	}
}

func TestDocs_ActivePublicManifestCoversReleaseBlockingDocs(t *testing.T) {
	activeDocs := activePublicContractDocs()
	for _, doc := range releaseBlockingContractDocs() {
		if staticStringSliceContains(nonReleaseFacingDocs(), doc) {
			continue
		}
		if !staticStringSliceContains(activeDocs, doc) {
			t.Fatalf("active public docs manifest must cover release-blocking doc %s", doc)
		}
	}
}

func TestDocs_ArchivedDocsDeclareNonReleaseFacingStatus(t *testing.T) {
	for _, doc := range archivedNonReleaseFacingDocs() {
		t.Run(doc, func(t *testing.T) {
			body := strings.ToLower(readRepoFile(t, doc))
			for _, required := range []string{"archived", "non-release-facing", "not part of the v0 public contract"} {
				if !strings.Contains(body, required) {
					t.Fatalf("%s is excluded from active docs scans but does not declare %q", doc, required)
				}
			}
		})
	}
}

func TestDocs_ActiveNonReleaseFacingDesignDocsDeclareStatus(t *testing.T) {
	for _, doc := range activeNonReleaseFacingDesignDocs() {
		t.Run(doc, func(t *testing.T) {
			rawBody := readRepoFile(t, doc)
			body := strings.ToLower(rawBody)
			for _, required := range []string{"active clean redesign", "non-release-facing", "not part of the v0 public contract"} {
				if !strings.Contains(body, required) {
					t.Fatalf("%s is excluded from v0 release-facing docs scans but does not declare %q", doc, required)
				}
			}
			for _, forbidden := range []string{"**Status:** Archived", "Archived note:"} {
				if strings.Contains(rawBody, forbidden) {
					t.Fatalf("%s is active non-release-facing design work and must not declare archived marker %q", doc, forbidden)
				}
			}
		})
	}
}

func TestDocs_ActiveNonReleaseFacingReferenceDocsDeclareStatus(t *testing.T) {
	for _, doc := range activeNonReleaseFacingReferenceDocs() {
		t.Run(doc, func(t *testing.T) {
			body := strings.ToLower(readRepoFile(t, doc))
			for _, required := range []string{"active reference index", "non-release-facing", "not part of the v0 public contract"} {
				if !strings.Contains(body, required) {
					t.Fatalf("%s is excluded from v0 release-facing docs scans but does not declare %q", doc, required)
				}
			}
		})
	}
}

func TestDocs_ActiveNonReleaseFacingResearchDocsDeclareStatus(t *testing.T) {
	for _, doc := range activeNonReleaseFacingResearchDocs() {
		t.Run(doc, func(t *testing.T) {
			body := strings.ToLower(readRepoFile(t, doc))
			for _, required := range []string{"active product research", "non-release-facing", "not part of the v0 public contract"} {
				if !strings.Contains(body, required) {
					t.Fatalf("%s is product research outside v0 release-facing docs but does not declare %q", doc, required)
				}
			}
			for _, forbidden := range []string{"active release-facing", "release-facing product research"} {
				if strings.Contains(body, forbidden) {
					t.Fatalf("%s must not declare release-facing status marker %q", doc, forbidden)
				}
			}
		})
	}
}

func TestDocs_ActiveNonReleaseFacingExampleDocsDeclareStatus(t *testing.T) {
	for _, doc := range activeNonReleaseFacingExampleDocs() {
		t.Run(doc, func(t *testing.T) {
			body := strings.ToLower(readRepoFile(t, doc))
			for _, required := range []string{"non-release-facing", "non-normative example", "not part of the v0 public contract"} {
				if !strings.Contains(body, required) {
					t.Fatalf("%s is an example outside v0 release-facing docs but does not declare %q", doc, required)
				}
			}
			if strings.Contains(body, "release-facing domain entry") {
				t.Fatalf("%s must not declare the old release-facing domain entry status", doc)
			}
		})
	}
}

func TestDocs_AllMarkdownDocsAreReleaseClassified(t *testing.T) {
	classified := append([]string{}, activePublicContractDocs()...)
	classified = append(classified, archivedNonReleaseFacingDocs()...)
	classified = append(classified, activeNonReleaseFacingDesignDocs()...)
	classified = append(classified, activeNonReleaseFacingReferenceDocs()...)
	classified = append(classified, activeNonReleaseFacingResearchDocs()...)
	classified = append(classified, activeNonReleaseFacingExampleDocs()...)
	for _, doc := range markdownDocsUnder(t, "docs") {
		if !staticStringSliceContains(classified, doc) {
			t.Fatalf("%s must be active release-facing, active non-release-facing, or explicitly archived/non-release-facing", doc)
		}
	}
}

func TestDocs_AllMarkdownAvoidsLegacyPublicDesignSurface(t *testing.T) {
	for _, doc := range allDocsContractMarkdownDocs(t) {
		t.Run(doc, func(t *testing.T) {
			for lineNo, line := range strings.Split(readRepoFile(t, doc), "\n") {
				if allowedAllMarkdownLegacyDesignLine(line) {
					continue
				}
				for _, fragment := range allMarkdownLegacyPublicDesignFragments() {
					if lineContainsUnsupportedPublicDocCommandFragment(line, fragment) {
						t.Fatalf("%s:%d retains legacy public design surface %q:\n%s", doc, lineNo+1, fragment, line)
					}
				}
			}
		})
	}
}

func TestDocs_TraceabilityNormativeDocsAreReleaseBlocking(t *testing.T) {
	docs := releaseBlockingContractDocs()
	for _, doc := range traceabilityNormativeDocs(t) {
		if !staticStringSliceContains(docs, doc) {
			t.Fatalf("release-blocking docs manifest must include traceability normative doc %s", doc)
		}
	}
}

func TestDocs_TraceabilityNormativeDocsParseBareBlocks(t *testing.T) {
	docs := traceabilityNormativeDocs(t)
	for _, want := range []string{
		"docs/02_CLI_SPEC.md",
		"docs/12_RELEASE_POLICY.md",
	} {
		if !staticStringSliceContains(docs, want) {
			t.Fatalf("traceability normative docs must include %s from bare Normative docs blocks, got %v", want, docs)
		}
	}
	if staticStringSliceContains(docs, "docs/21_SAVE_POINT_WORKSPACE_SEMANTICS.md") {
		t.Fatalf("supporting non-release-facing reference must not be counted as a traceability normative doc: %v", docs)
	}
}

func TestDocs_TargetUsersIsSupportingResearchNotGAPromise9Normative(t *testing.T) {
	matrixPromise := markdownSectionByHeading(t,
		"docs/14_TRACEABILITY_MATRIX.md",
		readRepoFile(t, "docs/14_TRACEABILITY_MATRIX.md"),
		"## Promise 9: GA Stories Preserve User Mental Models",
	)
	normativeDocs := textBetweenMarkers(t, matrixPromise, "Normative docs:", "Supporting research:")
	if strings.Contains(normativeDocs, "`docs/TARGET_USERS.md`") {
		t.Fatalf("docs/TARGET_USERS.md is product research and must not be a Promise 9 normative doc")
	}
	supportingResearch := textBetweenMarkers(t, matrixPromise, "Supporting research:", "Evidence:")
	requireReleaseReadinessText(t, "traceability promise 9 supporting research", supportingResearch, "`docs/TARGET_USERS.md`")
}

func TestDocs_TargetUsersIsSupportingResearchNotReleaseBlocking(t *testing.T) {
	if staticStringSliceContains(releaseBlockingContractDocs(), "docs/TARGET_USERS.md") {
		t.Fatalf("docs/TARGET_USERS.md is supporting product research and must not be release-blocking")
	}
	if !staticStringSliceContains(nonReleaseFacingDocs(), "docs/TARGET_USERS.md") {
		t.Fatalf("docs/TARGET_USERS.md must be classified as non-release-facing product research")
	}
}

func TestDocs_TraceabilityNormativeDocsUseV0ReleaseVocabulary(t *testing.T) {
	for _, doc := range traceabilityNormativeDocs(t) {
		t.Run(doc, func(t *testing.T) {
			scanPublicDocLines(t, doc, func(lineNo int, line string) {
				if stalePublicReleaseVocabularyPattern.MatchString(line) {
					t.Fatalf("%s:%d advertises stale v7 release vocabulary:\n%s", doc, lineNo, line)
				}
			})
		})
	}
}

func TestDocs_TraceabilityNormativeDocsAvoidUnsupportedCurrentBehavior(t *testing.T) {
	for _, doc := range traceabilityNormativeDocs(t) {
		t.Run(doc, func(t *testing.T) {
			scanPublicDocLines(t, doc, func(lineNo int, line string) {
				for _, bad := range unsupportedTraceabilityNormativeFragments() {
					if lineContainsUnsupportedPublicDocCommandFragment(line, bad) {
						t.Fatalf("%s:%d documents unsupported/non-v0-stable behavior %q:\n%s", doc, lineNo, bad, line)
					}
				}
				for _, id := range unsupportedDoctorRepairActionIDs() {
					if lineContainsRepairActionID(line, id) {
						t.Fatalf("%s:%d documents unsupported doctor repair action %q:\n%s", doc, lineNo, id, line)
					}
				}
			})
		})
	}
}

func TestDocs_TraceabilityConformanceLinksUseContractAreas(t *testing.T) {
	areas := conformanceContractAreaNames(t)
	matrix := readRepoFile(t, "docs/14_TRACEABILITY_MATRIX.md")
	currentSection := ""
	for lineNo, line := range strings.Split(matrix, "\n") {
		if strings.HasPrefix(line, "## ") {
			currentSection = strings.TrimSpace(strings.TrimPrefix(line, "## "))
		}
		if numberedConformanceTestRef.MatchString(line) {
			t.Fatalf("docs/14_TRACEABILITY_MATRIX.md:%d references obsolete numbered conformance tests:\n%s", lineNo+1, line)
		}
		if !strings.HasPrefix(currentSection, "Promise ") || !strings.Contains(line, "docs/11_CONFORMANCE_TEST_PLAN.md") {
			continue
		}
		if !lineReferencesKnownContractArea(line, areas) {
			t.Fatalf("docs/14_TRACEABILITY_MATRIX.md:%d must reference a named contract area from docs/11_CONFORMANCE_TEST_PLAN.md:\n%s", lineNo+1, line)
		}
	}
}

func TestDocs_TraceabilityPhase4ArtifactsListReleaseEvidenceLedger(t *testing.T) {
	matrix := readRepoFile(t, "docs/14_TRACEABILITY_MATRIX.md")
	section := markdownSectionByHeadingAny(t, "docs/14_TRACEABILITY_MATRIX.md", matrix,
		"## Release gating trace",
		"## Promise 8: Candidate And Final Evidence Are Separate",
	)
	if !strings.Contains(section, "`docs/RELEASE_EVIDENCE.md`") {
		t.Fatalf("Phase 4 GA artifacts must list docs/RELEASE_EVIDENCE.md")
	}
}

func TestDocs_PublicDocsUseV0ReleaseVocabulary(t *testing.T) {
	for _, doc := range publicDocsWithoutHistoricalReleaseMentions() {
		t.Run(doc, func(t *testing.T) {
			scanPublicDocLines(t, doc, func(lineNo int, line string) {
				if stalePublicReleaseVocabularyPattern.MatchString(line) {
					t.Fatalf("%s:%d advertises stale v7 release vocabulary:\n%s", doc, lineNo, line)
				}
			})
		})
	}
}

func TestDocs_RuntimeTerminologyDoesNotDescribeRuntimeLocksAsCurrentBehavior(t *testing.T) {
	for _, doc := range activePublicContractDocs() {
		t.Run(doc, func(t *testing.T) {
			scanPublicDocLines(t, doc, func(lineNo int, line string) {
				if staleCurrentRuntimeLockTerminologyPattern.MatchString(line) {
					t.Fatalf("%s:%d describes runtime locks as current release-facing behavior:\n%s", doc, lineNo, line)
				}
				lowerLine := strings.ToLower(line)
				for _, fragment := range runtimeInternalFileVocabularyFragments() {
					if strings.Contains(lowerLine, fragment) {
						t.Fatalf("%s:%d leaks internal runtime file vocabulary %q; use runtime state or active operations instead:\n%s", doc, lineNo, fragment, line)
					}
				}
			})
		})
	}
}

func TestDocs_TerminologyMigrationStorageNamesAreCompatibilityOnly(t *testing.T) {
	doc := readRepoFile(t, "docs/18_MIGRATION_AND_BACKUP.md")
	section := markdownSectionByHeadingAny(t, "docs/18_MIGRATION_AND_BACKUP.md", doc,
		"## Historical/internal terminology",
		"## Historical/Internal Terminology",
	)
	for _, required := range []string{
		"save point",
		"workspace",
		".jvs/snapshots",
		".jvs/worktrees",
		"internal storage names",
		"not a rollback",
		"no user-facing behavior",
	} {
		requireReleaseReadinessText(t, "migration historical/internal terminology", section, required)
	}
}

func TestDocs_MigrationRuntimeStateUsesPublicRuntimeBoundary(t *testing.T) {
	migration := readRepoFile(t, "docs/18_MIGRATION_AND_BACKUP.md")
	runtimePolicy := markdownSectionByHeadingAny(t, "docs/18_MIGRATION_AND_BACKUP.md", migration,
		"## Runtime-state policy (MUST)",
		"## Runtime-State Policy",
	)
	for _, required := range []string{
		"runtime state",
		"non-portable",
		"jvs doctor --strict --repair-runtime",
		"new cleanup preview",
	} {
		requireReleaseReadinessText(t, "migration runtime-state policy", runtimePolicy, required)
	}
	assertNoPathShapedRuntimeMechanisms(t, "docs/18_MIGRATION_AND_BACKUP.md runtime-state policy", runtimePolicy)

	migrationFlow := markdownSectionByHeadingAny(t, "docs/18_MIGRATION_AND_BACKUP.md", migration,
		"## Migration flow",
		"## Migration Flow",
	)
	for _, required := range []string{
		"non-portable JVS runtime state",
	} {
		requireReleaseReadinessText(t, "migration sync example", migrationFlow, required)
	}
	requireReleaseReadinessText(t, "migration runtime rebuild command", migration, "jvs doctor --strict --repair-runtime")
	assertNoPathShapedRuntimeMechanisms(t, "docs/18_MIGRATION_AND_BACKUP.md migration flow", migrationFlow)

	layout := readRepoFile(t, "docs/01_REPO_LAYOUT_SPEC.md")
	portability := markdownSectionByHeadingAny(t, "docs/01_REPO_LAYOUT_SPEC.md", layout,
		"## Runtime State Boundary",
		"## Internal Runtime Portability Classes",
		"## Portability classes",
		"## Portability Classes",
	)
	for _, required := range []string{
		"portable durable state",
		"non-portable runtime state",
		"jvs doctor --strict --repair-runtime",
	} {
		requireReleaseReadinessText(t, "repo layout runtime-state portability class", portability, required)
	}
	assertNoPathShapedRuntimeMechanisms(t, "docs/01_REPO_LAYOUT_SPEC.md runtime-state portability class", portability)
}

func TestDocs_MigrationFlowDocumentsExecutableColdWholeFolderCopy(t *testing.T) {
	migration := readRepoFile(t, "docs/18_MIGRATION_AND_BACKUP.md")
	body := releaseFacingClaimBody(t, "docs/18_MIGRATION_AND_BACKUP.md")
	for _, required := range []string{
		"offline whole-folder copy",
		"stop all JVS writers",
		"no active operations",
		"jvs status",
		"jvs recovery status",
		"jvs cleanup preview",
		"jvs doctor --strict",
		"ordinary filesystem copy",
		"managed folder/repository as a whole",
		"fresh destination path",
		"destination path must not exist",
		"fails before copying",
		"do not overlay",
		"jvs doctor --strict --repair-runtime",
		"fresh cleanup preview",
	} {
		requireReleaseReadinessText(t, "migration executable cold whole-folder copy", body, required)
	}
	migrationFlow := markdownSectionByHeadingAny(t, "docs/18_MIGRATION_AND_BACKUP.md", migration,
		"## Migration flow",
		"## Migration Flow",
	)
	assertNoPathShapedRuntimeMechanisms(t, "docs/18_MIGRATION_AND_BACKUP.md migration flow", migrationFlow)
}

func TestDocs_MigrationCopyExamplesFailClosedBeforeCopy(t *testing.T) {
	for _, doc := range []string{
		"docs/13_OPERATION_RUNBOOK.md",
		"docs/18_MIGRATION_AND_BACKUP.md",
	} {
		t.Run(doc, func(t *testing.T) {
			body := releaseFacingClaimBody(t, doc)
			for _, required := range []string{
				"fresh destination",
				"destination path must not exist",
				"managed folder/repository as a whole",
				"jvs doctor --strict --repair-runtime",
				"fresh cleanup preview",
			} {
				requireReleaseReadinessText(t, "migration copy contract "+doc, body, required)
			}
			blocks := migrationWholeFolderCopyCodeBlocks(body)
			if len(blocks) == 0 {
				t.Fatalf("%s must include an executable whole-folder copy example", doc)
			}
			for _, block := range blocks {
				if !migrationCopyBlockFailsClosed(block.text) {
					t.Fatalf("%s:%d migration copy example must fail closed before copying when destination exists; chain the destination existence check to mkdir/cp or use set -e before the check:\n%s",
						doc, block.startLine, block.text)
				}
			}
		})
	}
}

func TestDocs_CLICleanupPreviewSummaryUsesStableProtectionReasons(t *testing.T) {
	doc := "docs/02_CLI_SPEC.md"
	body := readRepoFile(t, doc)
	section := markdownSectionByHeadingAny(t, doc, body, "### `jvs cleanup preview [--json]`")
	intro := section
	if marker := strings.Index(intro, "Human output must show:"); marker >= 0 {
		intro = intro[:marker]
	}
	for _, required := range stableCleanupProtectionNaturalTerms() {
		requireReleaseReadinessText(t, "cleanup preview summary stable reason "+required, intro, required)
	}
}

func TestDocs_ReleaseFacingDocsAvoidPathShapedInternalRuntimeMechanisms(t *testing.T) {
	for _, doc := range activePublicContractDocs() {
		t.Run(doc, func(t *testing.T) {
			scanPublicDocLines(t, doc, func(lineNo int, line string) {
				assertNoPathShapedRuntimeMechanismLine(t, doc, lineNo, line)
			})
		})
	}
}

func TestDocs_ReleaseFacingRuntimeBoundaryDocsDocumentRepairCommand(t *testing.T) {
	for _, doc := range releaseFacingRuntimeStateBoundaryDocs() {
		t.Run(doc, func(t *testing.T) {
			body := readRepoFile(t, doc)
			for _, required := range append([]string{"runtime state"}, runtimeStateBoundaryTerms()...) {
				requireReleaseReadinessText(t, "release-facing runtime-state boundary "+doc, body, required)
			}
		})
	}
}

func TestDocs_ReleaseFacingSyncExamplesDoNotCopyJVSMetadata(t *testing.T) {
	for _, doc := range activePublicContractDocs() {
		t.Run(doc, func(t *testing.T) {
			body := releaseFacingClaimBody(t, doc)
			for _, block := range releaseFacingSyncGuidanceBlocks(body) {
				if !syncExampleCopiesJVSMetadata(block.text) {
					continue
				}
				t.Fatalf("%s:%d storage-level sync example copies JVS metadata; use a product-level migration procedure or a documented safe JVS migration boundary instead of an unsafe command example:\n%s",
					doc, block.startLine, block.text)
			}
		})
	}
}

func TestDocs_CleanupProtectionVocabularyUsesStableReasons(t *testing.T) {
	for _, doc := range activePublicContractDocs() {
		t.Run(doc, func(t *testing.T) {
			body := releaseFacingClaimBody(t, doc)
			scanPublicDocLines(t, doc, func(lineNo int, line string) {
				if cleanupProtectionVocabularyContext(line) &&
					staleCleanupProtectionVocabularyPattern.MatchString(line) {
					t.Fatalf("%s:%d uses unstable cleanup protection vocabulary; use workspace history, open views, active recovery plans, and active operations:\n%s", doc, lineNo, line)
				}
				lowerLine := strings.ToLower(line)
				for _, forbidden := range forbiddenCleanupProtectionVocabulary() {
					if strings.Contains(lowerLine, forbidden) {
						t.Fatalf("%s:%d uses unstable cleanup protection vocabulary %q; use workspace history, open views, active recovery plans, and active operations:\n%s", doc, lineNo, forbidden, line)
					}
				}
			})
			for _, block := range markdownNonFencedTextBlocks(body) {
				if !cleanupProtectionBoundaryClaim(block.text) {
					continue
				}
				for _, required := range stableCleanupProtectionNaturalTerms() {
					requireReleaseReadinessText(t, "cleanup protection vocabulary "+doc, block.text, required)
				}
			}
		})
	}
}

func TestDocs_MigrationCopyFailClosedGuardUsesGenericDestinations(t *testing.T) {
	genericPathBlock := `
test ! -e /backup/jvs-repo &&
mkdir -p /backup &&
cp -a /data/jvs-repo /backup/jvs-repo
`
	if !migrationCopyBlockFailsClosed(genericPathBlock) {
		t.Fatalf("migration copy fail-closed guard must accept generic destination paths:\n%s", genericPathBlock)
	}

	singleLineChainedBlock := `
test ! -e "$dst" && mkdir -p "$parent" && cp -a "$src" "$dst"
`
	if !migrationCopyBlockFailsClosed(singleLineChainedBlock) {
		t.Fatalf("migration copy fail-closed guard must accept one-line chained checks:\n%s", singleLineChainedBlock)
	}

	setEBlock := `
set -euo pipefail
test ! -e "$destination"
mkdir -p "$destination_parent"
cp --archive "$source" "$destination"
`
	if !migrationCopyBlockFailsClosed(setEBlock) {
		t.Fatalf("migration copy fail-closed guard must accept set -e with generic variables:\n%s", setEBlock)
	}

	explicitExitBlock := `
if test -e "$destination"; then
  echo "destination exists" >&2
  exit 1
fi
mkdir -p "$destination_parent"
cp -R "$source" "$destination"
`
	if !migrationCopyBlockFailsClosed(explicitExitBlock) {
		t.Fatalf("migration copy fail-closed guard must accept explicit exit with generic variables:\n%s", explicitExitBlock)
	}

	bracketOrExitBlock := `
[ ! -e "$dst" ] || exit 1
mkdir -p "$parent"
cp -R "$src" "$dst"
`
	if !migrationCopyBlockFailsClosed(bracketOrExitBlock) {
		t.Fatalf("migration copy fail-closed guard must accept bracket checks with explicit exit:\n%s", bracketOrExitBlock)
	}

	bracketAndExitBlock := `
[ -e "$dst" ] && exit 1; mkdir -p "$parent"; cp -a "$src" "$dst"
`
	if !migrationCopyBlockFailsClosed(bracketAndExitBlock) {
		t.Fatalf("migration copy fail-closed guard must accept exists-check and-exit before copy:\n%s", bracketAndExitBlock)
	}

	sudoCopyBlock := `
set -e
test ! -e "$dst"
mkdir -p "$parent"
sudo cp --archive "$src" "$dst"
`
	if !migrationCopyBlockFailsClosed(sudoCopyBlock) {
		t.Fatalf("migration copy fail-closed guard must accept privileged filesystem copy commands:\n%s", sudoCopyBlock)
	}

	rsyncCopyBlock := `
[ ! -e "$dst" ] || exit 1
mkdir -p "$parent"
rsync --archive "$src" "$dst"
`
	if !migrationCopyBlockFailsClosed(rsyncCopyBlock) {
		t.Fatalf("migration copy fail-closed guard must accept reasonable filesystem copy commands:\n%s", rsyncCopyBlock)
	}

	rsyncOverlaySourceBlock := `
test ! -e "$dst" && mkdir -p "$parent" && rsync --archive "$src/" "$dst"
`
	if migrationCopyBlockFailsClosed(rsyncOverlaySourceBlock) {
		t.Fatalf("migration copy fail-closed guard must reject rsync source-directory-content operands:\n%s", rsyncOverlaySourceBlock)
	}

	unchainedBlock := `
test ! -e /backup/jvs-repo
mkdir -p /backup
cp -a /data/jvs-repo /backup/jvs-repo
`
	if migrationCopyBlockFailsClosed(unchainedBlock) {
		t.Fatalf("migration copy fail-closed guard must reject unchained checks without set -e:\n%s", unchainedBlock)
	}

	wrongDestinationBlock := `
test ! -e /other/jvs-repo &&
mkdir -p /backup &&
cp -a /data/jvs-repo /backup/jvs-repo
`
	if migrationCopyBlockFailsClosed(wrongDestinationBlock) {
		t.Fatalf("migration copy fail-closed guard must reject checks for a different destination:\n%s", wrongDestinationBlock)
	}

	overlayBlock := `
test ! -e /backup/jvs-repo &&
mkdir -p /backup &&
cp -a /data/jvs-repo/. /backup/jvs-repo/
`
	if migrationCopyBlockFailsClosed(overlayBlock) {
		t.Fatalf("migration copy fail-closed guard must reject overlay-style copy operands:\n%s", overlayBlock)
	}

	for _, tc := range []struct {
		name  string
		block string
	}{
		{
			name: "short-circuited nonexistence guard",
			block: `
true || test ! -e "$dst" &&
mkdir -p "$parent" &&
cp -a "$src" "$dst"
`,
		},
		{
			name: "short-circuited explicit exit guard",
			block: `
false && [ -e "$dst" ] && exit 1
mkdir -p "$parent"
cp -a "$src" "$dst"
`,
		},
		{
			name: "pipelined chained nonexistence guard",
			block: `
echo checking | test ! -e "$dst" &&
mkdir -p "$parent" &&
cp -a "$src" "$dst"
`,
		},
		{
			name: "precreated destination",
			block: `
test ! -e "$dst" &&
mkdir -p "$dst" &&
cp -a "$src" "$dst"
`,
		},
		{
			name: "masked set -e check",
			block: `
set -e
test ! -e "$dst" || true
mkdir -p "$parent"
cp -a "$src" "$dst"
`,
		},
		{
			name: "compound test nonexistence condition",
			block: `
test ! -e "$dst" -o -e "$dst" &&
mkdir -p "$parent" &&
cp -a "$src" "$dst"
`,
		},
		{
			name: "compound bracket nonexistence condition",
			block: `
[ ! -e "$dst" -o -e "$dst" ] &&
mkdir -p "$parent" &&
cp -a "$src" "$dst"
`,
		},
		{
			name: "compound test exists explicit exit",
			block: `
test -e "$dst" -a "$flag" = yes && exit 1
mkdir -p "$parent"
cp -a "$src" "$dst"
`,
		},
		{
			name: "compound bracket exists explicit exit",
			block: `
[ -e "$dst" -a "$flag" = yes ] && exit 1
mkdir -p "$parent"
cp -a "$src" "$dst"
`,
		},
		{
			name: "compound bracket exists if true branch exit",
			block: `
if [ -e "$dst" -a "$flag" = yes ]; then
  exit 1
fi
mkdir -p "$parent"
cp -a "$src" "$dst"
`,
		},
		{
			name: "if condition-list masks destination exists before true branch exit",
			block: `
if [ -e "$dst" ] && false; then
  exit 1
fi
mkdir -p "$parent"
cp -a "$src" "$dst"
`,
		},
		{
			name: "disabled set -e before check",
			block: `
set -e
set +e
test ! -e "$dst"
mkdir -p "$parent"
cp -a "$src" "$dst"
`,
		},
		{
			name: "short-circuited disabled set -e before check",
			block: `
set -e
true && set +e
test ! -e "$dst"
mkdir -p "$parent"
cp -a "$src" "$dst"
`,
		},
		{
			name: "branch disabled set -e before check",
			block: `
set -e
if true; then
  set +e
fi
test ! -e "$dst"
mkdir -p "$parent"
cp -a "$src" "$dst"
`,
		},
		{
			name: "short-circuited set -e before check",
			block: `
false && set -e
test ! -e "$dst"
mkdir -p "$parent"
cp -a "$src" "$dst"
`,
		},
		{
			name: "disabled set -o errexit before check",
			block: `
set -o errexit
set +o errexit
test ! -e "$dst"
mkdir -p "$parent"
cp -a "$src" "$dst"
`,
		},
		{
			name: "pipelined set -e nonexistence check",
			block: `
set -e
test ! -e "$dst" | cat
mkdir -p "$parent"
cp -a "$src" "$dst"
`,
		},
		{
			name: "pipelined set -e bracket nonexistence check",
			block: `
set -e
[ ! -e "$dst" ] | cat
mkdir -p "$parent"
cp -a "$src" "$dst"
`,
		},
		{
			name: "if condition set -e test nonexistence check",
			block: `
set -e
if test ! -e "$dst"; then
  echo ok
fi
mkdir -p "$parent"
cp -a "$src" "$dst"
`,
		},
		{
			name: "if condition set -e bracket nonexistence check",
			block: `
set -e
if [ ! -e "$dst" ]; then
  echo ok
fi
mkdir -p "$parent"
cp -a "$src" "$dst"
`,
		},
		{
			name: "if condition-list later set -e test nonexistence check",
			block: `
set -e
if echo checking; test ! -e "$dst"; then
  :
fi
mkdir -p "$parent"
cp -a "$src" "$dst"
`,
		},
		{
			name: "if condition-list later set -e bracket nonexistence check",
			block: `
set -e
if echo checking; [ ! -e "$dst" ]; then
  :
fi
mkdir -p "$parent"
cp -a "$src" "$dst"
`,
		},
		{
			name: "echoed exit token",
			block: `
[ ! -e "$dst" ] || echo exit
mkdir -p "$parent"
cp -a "$src" "$dst"
`,
		},
		{
			name: "successful exit",
			block: `
if test -e "$dst"; then
  exit 0
fi
mkdir -p "$parent"
cp -a "$src" "$dst"
`,
		},
		{
			name: "skipped branch explicit exit guard",
			block: `
if false; then
  if test -e "$dst"; then
    exit 1
  fi
fi
mkdir -p "$parent"
cp -a "$src" "$dst"
`,
		},
		{
			name: "if true branch pipelined exit",
			block: `
if test -e "$dst"; then
  exit 1 | cat
fi
mkdir -p "$parent"
cp -a "$src" "$dst"
`,
		},
		{
			name: "and chained pipelined exit",
			block: `
[ -e "$dst" ] && exit 1 | cat
mkdir -p "$parent"
cp -a "$src" "$dst"
`,
		},
		{
			name: "backgrounded explicit exit guard",
			block: `
[ -e "$dst" ] && exit 1 &
mkdir -p "$parent"
cp -a "$src" "$dst"
`,
		},
		{
			name: "backgrounded set -e nonexistence check",
			block: `
set -e
test ! -e "$dst" &
mkdir -p "$parent"
cp -a "$src" "$dst"
`,
		},
		{
			name: "conditional true branch exit",
			block: `
if test -e "$dst"; then
  echo "destination exists" || exit 1
fi
mkdir -p "$parent"
cp -a "$src" "$dst"
`,
		},
		{
			name: "loop guarded true branch exit",
			block: `
if test -e "$dst"; then
  while false; do
    exit 1
  done
fi
mkdir -p "$parent"
cp -a "$src" "$dst"
`,
		},
		{
			name: "until guarded true branch exit",
			block: `
if test -e "$dst"; then
  until true; do
    exit 1
  done
fi
mkdir -p "$parent"
cp -a "$src" "$dst"
`,
		},
		{
			name: "empty for true branch exit",
			block: `
if test -e "$dst"; then
  for x in; do
    exit 1
  done
fi
mkdir -p "$parent"
cp -a "$src" "$dst"
`,
		},
		{
			name: "and-conditional true branch exit",
			block: `
if test -e "$dst"; then
  echo "destination exists" && exit 1
fi
mkdir -p "$parent"
cp -a "$src" "$dst"
`,
		},
		{
			name: "reversed positive destination exit",
			block: `
[ -e "$dst" ] || exit 1
mkdir -p "$parent"
cp -a "$src" "$dst"
`,
		},
		{
			name: "echoed set e",
			block: `
echo "set -euo pipefail"
test ! -e "$dst"
mkdir -p "$parent"
cp -a "$src" "$dst"
`,
		},
		{
			name: "case branch scoped set e before check",
			block: `
case "$mode" in
  guarded)
    set -e
    ;;
esac
test ! -e "$dst"
mkdir -p "$parent"
cp -a "$src" "$dst"
`,
		},
		{
			name: "subshell scoped set e before check",
			block: `
(
  set -e
)
test ! -e "$dst"
mkdir -p "$parent"
cp -a "$src" "$dst"
`,
		},
		{
			name: "subshell scoped explicit exit guard",
			block: `
(
  [ -e "$dst" ] && exit 1
)
mkdir -p "$parent"
cp -a "$src" "$dst"
`,
		},
		{
			name: "function body scoped set e before check",
			block: `
guard() {
  set -e
}
test ! -e "$dst"
mkdir -p "$parent"
cp -a "$src" "$dst"
`,
		},
		{
			name: "function body scoped explicit exit guard",
			block: `
guard() {
  [ -e "$dst" ] && exit 1
}
mkdir -p "$parent"
cp -a "$src" "$dst"
`,
		},
		{
			name: "compact function body scoped set e before check",
			block: `
guard(){
  set -e
}
test ! -e "$dst"
mkdir -p "$parent"
cp -a "$src" "$dst"
`,
		},
		{
			name: "compact function body scoped explicit exit guard",
			block: `
guard(){
  [ -e "$dst" ] && exit 1
}
mkdir -p "$parent"
cp -a "$src" "$dst"
`,
		},
		{
			name: "next-line brace function body scoped set e before check",
			block: `
guard()
{
  set -e
}
test ! -e "$dst"
mkdir -p "$parent"
cp -a "$src" "$dst"
`,
		},
		{
			name: "next-line brace function body scoped explicit exit guard",
			block: `
guard()
{
  [ -e "$dst" ] && exit 1
}
mkdir -p "$parent"
cp -a "$src" "$dst"
`,
		},
		{
			name: "precreated destination subdir",
			block: `
test ! -e "$dst" &&
mkdir -p "$dst/subdir" &&
cp -a "$src" "$dst"
`,
		},
		{
			name: "install precreated destination subdir",
			block: `
test ! -e "$dst" &&
install -d "$dst/subdir" &&
cp -a "$src" "$dst"
`,
		},
		{
			name: "install D precreated destination file",
			block: `
test ! -e "$dst" &&
install -D /dev/null "$dst/subdir/file" &&
cp -a "$src" "$dst"
`,
		},
		{
			name: "install D target directory precreated destination subdir",
			block: `
test ! -e "$dst" &&
install -D -t "$dst/subdir" /dev/null &&
cp -a "$src" "$dst"
`,
		},
		{
			name: "install clustered D target directory precreated destination subdir",
			block: `
test ! -e "$dst" &&
install -Dt "$dst/subdir" /dev/null &&
cp -a "$src" "$dst"
`,
		},
		{
			name: "install clustered D verbose target directory precreated destination subdir",
			block: `
test ! -e "$dst" &&
install -Dvt "$dst/subdir" /dev/null &&
cp -a "$src" "$dst"
`,
		},
		{
			name: "install D long target directory precreated destination subdir",
			block: `
test ! -e "$dst" &&
install -D --target-directory "$dst/subdir" /dev/null &&
cp -a "$src" "$dst"
`,
		},
		{
			name: "precreated quoted destination prefix subdir",
			block: `
test ! -e "$dst" &&
mkdir -p "$dst"/subdir &&
cp -a "$src" "$dst"
`,
		},
		{
			name: "precreated braced destination subdir",
			block: `
test ! -e "${dst}" &&
mkdir -p "${dst}/subdir" &&
cp -a "$src" "${dst}"
`,
		},
		{
			name: "precreated quoted braced destination prefix subdir",
			block: `
test ! -e "${dst}" &&
mkdir -p "${dst}"/subdir &&
cp -a "$src" "${dst}"
`,
		},
		{
			name: "precreated literal destination subdir",
			block: `
test ! -e /backup/jvs-repo &&
mkdir -p /backup/jvs-repo/subdir &&
cp -a /data/jvs-repo /backup/jvs-repo
`,
		},
		{
			name: "precreated literal destination subdir with dot segment",
			block: `
test ! -e /backup/jvs-repo &&
mkdir -p /backup/./jvs-repo/subdir &&
cp -a /data/jvs-repo /backup/jvs-repo
`,
		},
		{
			name: "precreated literal destination subdir with dot-dot segment",
			block: `
test ! -e /backup/jvs-repo &&
mkdir -p /backup/other/../jvs-repo/subdir &&
cp -a /data/jvs-repo /backup/jvs-repo
`,
		},
		{
			name: "precreated literal destination subdir with repeated slash",
			block: `
test ! -e /backup/jvs-repo &&
mkdir -p /backup//jvs-repo/subdir &&
cp -a /data/jvs-repo /backup/jvs-repo
`,
		},
		{
			name: "precreated quoted literal destination prefix subdir",
			block: `
test ! -e /backup/jvs-repo &&
mkdir -p "/backup/jvs-repo"/subdir &&
cp -a /data/jvs-repo /backup/jvs-repo
`,
		},
		{
			name: "precreated embedded quoted literal destination subdir",
			block: `
test ! -e /backup/jvs-repo &&
mkdir -p /backup/"jvs-repo"/subdir &&
cp -a /data/jvs-repo /backup/jvs-repo
`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if migrationCopyBlockFailsClosed(tc.block) {
				t.Fatalf("migration copy fail-closed guard must reject %s:\n%s", tc.name, tc.block)
			}
		})
	}
}

func TestDocs_ReleaseFacingDocsAvoidUnsupportedPublicCLIExamples(t *testing.T) {
	for _, doc := range activePublicContractDocs() {
		t.Run(doc, func(t *testing.T) {
			scanPublicDocLines(t, doc, func(lineNo int, line string) {
				for _, rule := range unsupportedPublicCLIExampleRules() {
					if !rule.pattern.MatchString(line) {
						continue
					}
					t.Fatalf("%s:%d documents unsupported public CLI example %s; %s:\n%s",
						doc, lineNo, rule.name, rule.replacement, line)
				}
			})
		})
	}
}

func TestDocs_PublicDocsDoNotAdvertiseRetentionPolicySurface(t *testing.T) {
	for _, doc := range releaseBlockingContractDocs() {
		t.Run(doc, func(t *testing.T) {
			scanPublicDocLines(t, doc, func(lineNo int, line string) {
				lower := strings.ToLower(line)
				for _, forbidden := range []string{
					"garbage collection with retention policies",
					"retention policy, pin sets",
					"retention cleanup",
					"gc policies",
					"tagged snapshots protected",
					"tagged checkpoints protected",
					"explicit pins",
					"minimum retention period",
				} {
					if strings.Contains(lower, forbidden) {
						t.Fatalf("%s:%d advertises non-v0 GC retention/pin surface %q:\n%s", doc, lineNo, forbidden, line)
					}
				}
			})
		})
	}
}

func TestDocs_ReleaseFacingDocsAvoidIgnoreUnmanagedPayloadVocabulary(t *testing.T) {
	for _, doc := range activePublicContractDocs() {
		t.Run(doc, func(t *testing.T) {
			scanPublicDocLines(t, doc, func(lineNo int, line string) {
				lower := strings.ToLower(line)
				for _, forbidden := range []string{
					"ignored/unmanaged",
					"unmanaged",
					"ignore rule",
					"ignore rules",
					"glob policy",
					"glob policies",
				} {
					if strings.Contains(lower, forbidden) {
						t.Fatalf("%s:%d exposes old payload-boundary vocabulary %q; use JVS control data, runtime state, or explicit user-file wording instead:\n%s",
							doc, lineNo, forbidden, line)
					}
				}
			})
		})
	}
}

func TestDocs_EngineTransparencySurfacesExcludeDoctor(t *testing.T) {
	productPlan := readRepoFile(t, "docs/PRODUCT_PLAN.md")
	engineSection := markdownSectionByHeading(t, "docs/PRODUCT_PLAN.md", productPlan, "## Engine Transparency")
	for _, docCase := range []struct {
		doc  string
		body string
	}{
		{doc: "docs/PRODUCT_PLAN.md", body: engineSection},
		{doc: "docs/FAQ.md", body: readRepoFile(t, "docs/FAQ.md")},
	} {
		t.Run(docCase.doc, func(t *testing.T) {
			for _, block := range markdownTextBlocks(docCase.body) {
				normalized := strings.Join(strings.Fields(block), " ")
				if doctorEngineSurfacePattern.MatchString(normalized) {
					t.Fatalf("%s treats doctor as an engine transparency surface; use status, save, history, or command JSON instead:\n%s", docCase.doc, block)
				}
			}
		})
	}

	t.Run("docs/PRODUCT_PLAN.md_surfaces", func(t *testing.T) {
		normalizedEngineSection := strings.Join(strings.Fields(engineSection), " ")
		for _, required := range []string{
			"setup output",
			"status",
			"save point metadata",
			"command JSON",
			"effective engine",
			"fallback",
		} {
			if !strings.Contains(normalizedEngineSection, required) {
				t.Fatalf("docs/PRODUCT_PLAN.md Engine Transparency must name %q as an engine transparency surface", required)
			}
		}
	})
}

func TestDocs_CleanupPreviewJSONFieldsMatchPublicFacade(t *testing.T) {
	fields := jsonFieldsForStruct(t, "internal/cli/public_json.go", "publicCleanupPlan")
	want := publicCleanupPlanJSONFields()
	assertSameStringSet(t, "internal/cli.publicCleanupPlan JSON fields", fields, want)

	for _, docSpec := range []struct {
		doc     string
		fields  func(t *testing.T, doc, section string) []string
		section string
	}{
		{
			doc:     "docs/API_DOCUMENTATION.md",
			section: "### Cleanup",
			fields: func(t *testing.T, doc, section string) []string {
				proseFields := backtickedFieldsAfterLabel(t, doc, section, "Public JSON fields:")
				assertSameStringSet(t, doc+" public cleanup JSON field prose", proseFields, publicCleanupPlanJSONFields())
				return jsonTagFieldsForDocumentedType(t, doc, section, "CleanupPlan")
			},
		},
	} {
		t.Run(docSpec.doc, func(t *testing.T) {
			body := readRepoFile(t, docSpec.doc)
			section := markdownSectionByHeading(t, docSpec.doc, body, docSpec.section)
			docFields := docSpec.fields(t, docSpec.doc, section)
			assertSameStringSet(t, docSpec.doc+" public cleanup JSON fields", docFields, want)
			for _, field := range forbiddenPublicCleanupJSONFieldNames() {
				if strings.Contains(section, "`"+field+"`") {
					t.Fatalf("%s public cleanup section documents non-public field %q", docSpec.doc, field)
				}
			}
		})
	}
}

func TestDocs_CleanupProtectionReasonsMatchPublicFacade(t *testing.T) {
	want := cleanupProtectionReasonConstantsFromFacade(t)
	assertSameStringSet(t, "pkg/jvs CleanupProtectionReason stable tokens", cleanupProtectionReasonTokensFromConstants(want), stableCleanupProtectionReasonTokens())
	body := readRepoFile(t, "docs/API_DOCUMENTATION.md")
	section := markdownSectionByHeading(t, "docs/API_DOCUMENTATION.md", body, "### Cleanup")
	got := documentedCleanupProtectionReasonConstants(section)
	assertSameStringSet(t, "docs/API_DOCUMENTATION.md CleanupProtectionReason constants", got, want)

	for _, constant := range want {
		_, token, ok := strings.Cut(constant, "=")
		if !ok {
			t.Fatalf("invalid cleanup protection reason constant descriptor %q", constant)
		}
		pattern := regexp.MustCompile(`(?m)^\s*-\s+` + regexp.QuoteMeta("`"+token+"`") + `:`)
		if !pattern.MatchString(section) {
			t.Fatalf("docs/API_DOCUMENTATION.md cleanup section must document meaning for reason token %q", token)
		}
	}
}

func publicCleanupPlanJSONFields() []string {
	return []string{
		"plan_id",
		"created_at",
		"protected_save_points",
		"protection_groups",
		"protected_by_history",
		"candidate_count",
		"reclaimable_save_points",
		"reclaimable_bytes_estimate",
	}
}

func forbiddenPublicCleanupJSONFieldNames() []string {
	return []string{
		"protected_checkpoints",
		"protected_by_lineage",
		"delete_checkpoints",
		"to_delete",
		"deletable_bytes_estimate",
		"protected_set",
		"protected_by_pin",
		"protected_by_retention",
		"retention",
		"retention_policy",
		"retention_days",
		"retention_age",
		"min_age",
		"min_age_days",
		"max_age",
		"max_age_days",
		"keep_last",
		"keep_last_checkpoints",
		"keep_last_snapshots",
		"keep_min_snapshots",
		"keep_min_age",
		"keep_max_age",
	}
}

func markdownSectionByHeading(t *testing.T, doc, body, heading string) string {
	t.Helper()
	headingLevel, ok := markdownHeadingLevel(heading)
	if !ok {
		t.Fatalf("invalid markdown heading %q", heading)
	}

	lines := strings.Split(body, "\n")
	start := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == heading {
			start = i
			break
		}
	}
	if start == -1 {
		t.Fatalf("%s missing public GC section heading %q", doc, heading)
	}

	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		level, ok := markdownHeadingLevel(lines[i])
		if ok && level <= headingLevel {
			end = i
			break
		}
	}
	return strings.Join(lines[start:end], "\n")
}

func markdownSectionByHeadingAny(t *testing.T, doc, body string, headings ...string) string {
	t.Helper()
	for _, heading := range headings {
		if strings.Contains(body, heading) {
			return markdownSectionByHeading(t, doc, body, heading)
		}
	}
	t.Fatalf("%s missing any section heading %v", doc, headings)
	return ""
}

func markdownTextBlocks(body string) []string {
	var blocks []string
	var current []string
	flush := func() {
		if len(current) == 0 {
			return
		}
		blocks = append(blocks, strings.Join(current, "\n"))
		current = nil
	}

	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			flush()
			continue
		}
		if strings.HasPrefix(trimmed, "- ") && len(current) > 0 {
			flush()
		}
		current = append(current, trimmed)
	}
	flush()
	return blocks
}

func markdownBulletFieldsAfterLabel(t *testing.T, doc, section, label string) []string {
	t.Helper()
	lines := strings.Split(section, "\n")
	labelIndex := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == label || trimmed == "- "+label {
			labelIndex = i
			break
		}
	}
	if labelIndex == -1 {
		t.Fatalf("%s public GC section missing field-list label %q", doc, label)
	}

	var fields []string
	inList := false
	for _, line := range lines[labelIndex+1:] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if inList {
				break
			}
			continue
		}
		if !strings.HasPrefix(trimmed, "- ") {
			if inList {
				break
			}
			t.Fatalf("%s public GC field-list label %q is not followed by a bullet list", doc, label)
		}
		inList = true
		match := markdownBulletFieldPattern.FindStringSubmatch(line)
		if match == nil {
			t.Fatalf("%s public GC field-list bullet must start with a backticked JSON field:\n%s", doc, line)
		}
		fields = append(fields, match[1])
	}
	if len(fields) == 0 {
		t.Fatalf("%s public GC field-list label %q has no fields", doc, label)
	}
	return fields
}

func backtickedFieldsAfterLabel(t *testing.T, doc, section, label string) []string {
	t.Helper()
	lines := strings.Split(section, "\n")
	start := -1
	for i, line := range lines {
		if strings.Contains(line, label) {
			start = i
			break
		}
	}
	if start == -1 {
		t.Fatalf("%s public GC section missing field-list label %q", doc, label)
	}

	var paragraph []string
	for i := start; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if i > start && trimmed == "" {
			break
		}
		paragraph = append(paragraph, lines[i])
	}

	var fields []string
	for _, match := range backtickedFieldPattern.FindAllStringSubmatch(strings.Join(paragraph, " "), -1) {
		fields = append(fields, match[1])
	}
	if len(fields) == 0 {
		t.Fatalf("%s public GC field-list label %q has no backticked fields", doc, label)
	}
	return fields
}

func jsonTagFieldsForDocumentedType(t *testing.T, doc, section, typeName string) []string {
	t.Helper()
	signature := "type " + typeName + " struct {"
	lines := strings.Split(section, "\n")
	inType := false
	found := false
	var fields []string
	for _, line := range lines {
		if !inType {
			if strings.Contains(line, signature) {
				inType = true
				found = true
			}
			continue
		}
		if strings.TrimSpace(line) == "}" {
			break
		}
		for _, match := range jsonTagFieldPattern.FindAllStringSubmatch(line, -1) {
			fields = append(fields, match[1])
		}
	}
	if !found {
		t.Fatalf("%s public GC section missing documented type %s", doc, typeName)
	}
	if len(fields) == 0 {
		t.Fatalf("%s public GC documented type %s has no JSON fields", doc, typeName)
	}
	return fields
}

func TestDocs_APIDocumentationLimitsStablePublicGoSurface(t *testing.T) {
	doc := readRepoFile(t, "docs/API_DOCUMENTATION.md")
	if strings.Contains(doc, "public API is organized under `pkg/` packages") {
		t.Fatalf("docs/API_DOCUMENTATION.md must not publish every pkg/ package as stable public API")
	}
	if !strings.Contains(doc, "stable v0 Go facade is `pkg/jvs`") {
		t.Fatalf("docs/API_DOCUMENTATION.md must identify pkg/jvs as the stable v0 Go facade")
	}
	if !strings.Contains(doc, "`pkg/model`") || !strings.Contains(doc, "Existing type support") {
		t.Fatalf("docs/API_DOCUMENTATION.md must mark pkg/model as existing type support, not a retention/pin public surface")
	}
}

func TestDocs_APIStableGuidanceDoesNotBypassFacade(t *testing.T) {
	scanPublicDocLines(t, "docs/API_DOCUMENTATION.md", func(lineNo int, line string) {
		for _, forbidden := range apiStableModelBypassFragments() {
			if strings.Contains(line, forbidden) {
				t.Fatalf("docs/API_DOCUMENTATION.md:%d bypasses stable facade with %q:\n%s", lineNo, forbidden, line)
			}
		}
	})
}

func TestDocs_PublicEngineVocabularyMatchesModel(t *testing.T) {
	want := engineConstantsFromModel(t)
	body := readRepoFile(t, "docs/API_DOCUMENTATION.md")
	section := markdownSectionByHeading(t, "docs/API_DOCUMENTATION.md", body, "### EngineType")
	got := documentedEngineConstants(section)
	assertSameStringSet(t, "docs/API_DOCUMENTATION.md EngineType constants", got, want)
}

func TestDocs_APIPublicExamplesUseStableFacade(t *testing.T) {
	body := readRepoFile(t, "docs/API_DOCUMENTATION.md")
	for _, heading := range []string{"## Quick Example", "## Integration Example"} {
		t.Run(heading, func(t *testing.T) {
			section := markdownSectionByHeading(t, "docs/API_DOCUMENTATION.md", body, heading)
			for _, required := range []string{
				`"github.com/agentsmith-project/jvs/pkg/jvs"`,
				"jvs.OpenOrInit(",
				".Save(ctx, jvs.SaveOptions{",
			} {
				if !strings.Contains(section, required) {
					t.Fatalf("docs/API_DOCUMENTATION.md %s must use stable pkg/jvs facade snippet %q", heading, required)
				}
			}
			for _, forbidden := range apiStableFacadeBypassFragments() {
				if strings.Contains(section, forbidden) {
					t.Fatalf("docs/API_DOCUMENTATION.md %s bypasses stable facade with %q", heading, forbidden)
				}
			}
		})
	}
}

func apiStableFacadeBypassFragments() []string {
	return []string{
		"model.NewSnapshotID",
		"model.IntentRecord",
		"model.Descriptor{",
		`"github.com/agentsmith-project/jvs/pkg/model"`,
		`"github.com/agentsmith-project/jvs/pkg/fsutil"`,
		`"github.com/agentsmith-project/jvs/pkg/jsonutil"`,
		".jvs/descriptors",
		"fsutil.AtomicWrite",
		"jsonutil.CanonicalMarshal",
	}
}

func apiStableModelBypassFragments() []string {
	return []string{
		"model.NewSnapshotID",
		"model.IntentRecord",
		"model.Descriptor{",
		`"github.com/agentsmith-project/jvs/pkg/model"`,
		".jvs/descriptors",
	}
}

func TestDocs_PublicCommandExamplesUseStableCommands(t *testing.T) {
	publicRootCommands := publicRootHelpCommandNames(t)
	for _, doc := range publicCommandDocs() {
		t.Run(doc, func(t *testing.T) {
			scanPublicDocLines(t, doc, func(lineNo int, line string) {
				for _, bad := range unsupportedPublicDocCommandFragments() {
					if lineContainsUnsupportedPublicDocCommandFragment(line, bad) {
						t.Fatalf("%s:%d documents unsupported/non-v0-stable command fragment %q:\n%s", doc, lineNo, bad, line)
					}
				}
				for _, id := range unsupportedDoctorRepairActionIDs() {
					if lineContainsRepairActionID(line, id) {
						t.Fatalf("%s:%d documents unsupported doctor repair action %q:\n%s", doc, lineNo, id, line)
					}
				}
				if !strings.Contains(line, "jvs ") {
					return
				}

				for _, fields := range publicDocCommandFieldSets(line) {
					commandPath := publicDocCommandPath(fields)
					if len(commandPath) == 0 {
						continue
					}
					if !stablePublicCommandPath(commandPath, publicRootCommands) {
						t.Fatalf("%s:%d documents unsupported/non-v0-stable command %q:\n%s", doc, lineNo, strings.Join(commandPath, " "), line)
					}
				}
			})
		})
	}
}

func TestDocs_DomainQuickstartsAreNonNormativeExamples(t *testing.T) {
	docs := publicCommandDocs()
	for _, doc := range domainQuickstartDocs() {
		if staticStringSliceContains(docs, doc) {
			t.Fatalf("%s is a domain example and must not be covered by release-facing public command docs contract", doc)
		}
		if !staticStringSliceContains(nonReleaseFacingDocs(), doc) {
			t.Fatalf("%s must be classified as a non-release-facing non-normative example", doc)
		}
	}
}

func TestDocs_StablePublicCommandPathMatchesCurrentHelpSurface(t *testing.T) {
	publicRootCommands := publicRootHelpCommandNames(t)
	for _, commandPath := range [][]string{
		{"init"},
		{"save"},
		{"history"},
		{"view"},
		{"view", "close"},
		{"restore"},
		{"cleanup", "preview"},
		{"cleanup", "run"},
		{"recovery", "status"},
		{"recovery", "resume"},
		{"recovery", "rollback"},
		{"repo", "clone"},
		{"repo", "move"},
		{"repo", "rename"},
		{"repo", "detach"},
		{"workspace", "new"},
		{"workspace", "list"},
		{"workspace", "path"},
		{"workspace", "rename"},
		{"workspace", "move"},
		{"workspace", "delete"},
		{"status"},
		{"doctor"},
	} {
		if !stablePublicCommandPath(commandPath, publicRootCommands) {
			t.Fatalf("current public command path %q must be allowed", strings.Join(commandPath, " "))
		}
	}
	for _, commandPath := range [][]string{
		{"checkpoint"},
		{"fork"},
		{"gc"},
		{"verify"},
		{"capability"},
		{"config"},
		{"conformance"},
		{"import"},
		{"clone"},
		{"info"},
		{"diff"},
		{"workspace", "fork"},
		{"view", "delete"},
		{"recovery", "repair"},
	} {
		if stablePublicCommandPath(commandPath, publicRootCommands) {
			t.Fatalf("legacy/hidden command path %q must not be allowed", strings.Join(commandPath, " "))
		}
	}
}

func TestDocs_CLISpecVisiblePublicCommandsIncludeLifecycleSurface(t *testing.T) {
	doc := "docs/02_CLI_SPEC.md"
	body := readRepoFile(t, doc)
	section := markdownSectionByHeading(t, doc, body, "## Root Help Surface")
	for _, required := range []string{
		"cleanup preview",
		"cleanup run",
		"repo clone",
		"repo move",
		"repo rename",
		"repo detach",
		"workspace list",
		"workspace path",
		"workspace rename",
		"workspace move",
		"workspace new",
		"workspace delete",
	} {
		if !strings.Contains(section, required) {
			t.Fatalf("%s visible public commands must include %q", doc, required)
		}
	}
}

func TestDocs_CLISpecDocumentsRepoAndWorkspaceLifecycleCommands(t *testing.T) {
	doc := "docs/02_CLI_SPEC.md"
	body := readRepoFile(t, doc)
	for _, heading := range []string{
		"### `jvs repo move <new-folder> [--json]`",
		"### `jvs repo move --run <repo-move-plan-id> [--json]`",
		"### `jvs repo rename <new-folder-name> [--json]`",
		"### `jvs repo rename --run <repo-rename-plan-id> [--json]`",
		"### `jvs repo detach [--json]`",
		"### `jvs repo detach --run <repo-detach-plan-id> [--json]`",
		"### `jvs workspace rename <old> <new> [--dry-run] [--json]`",
		"### `jvs workspace move <name> <new-folder>`",
		"### `jvs workspace move --run <workspace-move-plan-id> [--json]`",
		"### `jvs workspace delete <name>`",
		"### `jvs workspace delete --run <workspace-delete-plan-id> [--json]`",
	} {
		if !strings.Contains(body, heading) {
			t.Fatalf("%s missing lifecycle command heading %q", doc, heading)
		}
	}
}

func TestDocs_GCSpecDocumentsImportedCloneHistoryProtectionReason(t *testing.T) {
	doc := "docs/08_GC_SPEC.md"
	body := readRepoFile(t, doc)
	section := markdownSectionByHeading(t, doc, body, "## Protected Save Points")
	for _, required := range []string{"imported clone history", "imported_clone_history"} {
		requireReleaseReadinessText(t, "GC imported clone history protection "+required, section, required)
	}
}

func TestDocs_PublicCommandFieldSetsFindInlineJVSCommands(t *testing.T) {
	line := "Use `jvs save -m baseline`, then `jvs cleanup preview`, then `jvs workspace new ../exp --from abc123`."
	fieldSets := publicDocCommandFieldSets(line)
	var paths []string
	for _, fields := range fieldSets {
		paths = append(paths, strings.Join(publicDocCommandPath(fields), " "))
	}

	for _, want := range []string{"save", "cleanup preview", "workspace new"} {
		if !staticStringSliceContains(paths, want) {
			t.Fatalf("public command scanner missed inline command path %q; got %v", want, paths)
		}
	}
}

func TestDocs_CurrentPublicHelpSurfaceUsesSavePointCommands(t *testing.T) {
	commands := publicRootHelpCommandNames(t)
	for _, want := range []string{
		"init",
		"save",
		"history",
		"view",
		"restore",
		"cleanup",
		"recovery",
		"repo",
		"workspace",
		"status",
		"doctor",
		"completion",
		"help",
	} {
		if !commands[want] {
			t.Fatalf("current public root help surface must expose %q; got %v", want, stringBoolMapKeys(commands))
		}
	}
	for _, legacy := range []string{
		"checkpoint",
		"fork",
		"gc",
		"verify",
		"capability",
		"worktree",
		"snapshot",
	} {
		if commands[legacy] {
			t.Fatalf("current public root help surface must not expose legacy command %q", legacy)
		}
	}
}

func TestDocs_UserGuidesTeachCurrentSavePointCommandFlow(t *testing.T) {
	combined := ""
	for _, doc := range savePointUserGuideDocs() {
		combined += "\n" + readRepoFile(t, doc)
	}
	for _, required := range []string{
		"jvs save",
		"jvs history",
		"jvs view",
		"jvs restore",
		"jvs recovery",
		"jvs workspace new",
		"jvs cleanup preview",
		"jvs cleanup run",
	} {
		if !strings.Contains(combined, required) {
			t.Fatalf("save point user guides must document current public command %q", required)
		}
	}
}

func TestDocs_UserGuidesDoNotTeachLegacyCheckpointMainFlow(t *testing.T) {
	for _, doc := range savePointUserGuideDocs() {
		t.Run(doc, func(t *testing.T) {
			for lineNo, line := range strings.Split(readRepoFile(t, doc), "\n") {
				lower := strings.ToLower(line)
				for _, fragment := range legacyPublicContractFragments() {
					if strings.Contains(lower, fragment) {
						t.Fatalf("%s:%d teaches legacy checkpoint-era main-flow term %q:\n%s", doc, lineNo+1, fragment, line)
					}
				}
			}
		})
	}
}

func TestDocs_LicenseAndContributorCoverageContract(t *testing.T) {
	licensePath := repoFile(t, "LICENSE")
	licenseBody, err := os.ReadFile(licensePath)
	if err != nil {
		t.Fatalf("repository must include LICENSE: %v", err)
	}
	licenseText := string(licenseBody)
	for _, required := range []string{"MIT License", "Permission is hereby granted, free of charge"} {
		if !strings.Contains(licenseText, required) {
			t.Fatalf("LICENSE must be the MIT license and include %q", required)
		}
	}

	makefile := readRepoFile(t, "Makefile")
	match := makefileCoverageThresholdPattern.FindStringSubmatch(makefile)
	if match == nil {
		t.Fatalf("Makefile test-cover target must keep a parseable coverage threshold")
	}
	threshold := match[1]
	contributing := readRepoFile(t, "CONTRIBUTING.md")
	for _, forbidden := range []string{"80%+ test coverage", "target **80%+**"} {
		if strings.Contains(contributing, forbidden) {
			t.Fatalf("CONTRIBUTING.md advertises stale coverage threshold %q; Makefile enforces %s%%", forbidden, threshold)
		}
	}
	if !strings.Contains(contributing, "minimum **"+threshold+"%**") {
		t.Fatalf("CONTRIBUTING.md must document the Makefile coverage threshold as minimum **%s%%**", threshold)
	}
	if !strings.Contains(contributing, "make test-cover") {
		t.Fatalf("CONTRIBUTING.md must point contributors at make test-cover for threshold enforcement")
	}
}

func TestDocs_APIDocumentationDefinesStableExistingTypeInternalBoundaries(t *testing.T) {
	doc := readRepoFile(t, "docs/API_DOCUMENTATION.md")
	for _, required := range []string{
		"## Stable v0 Go Facade: pkg/jvs",
		"## Existing Type Packages",
		"## Internal-Only Packages",
		"Stable",
		"Existing type support",
		"Internal-only",
		"active save point CLI",
	} {
		if !strings.Contains(doc, required) {
			t.Fatalf("docs/API_DOCUMENTATION.md must define stable/existing-type/internal boundary text %q", required)
		}
	}
	for _, forbidden := range []string{
		"Use `pkg/jvs` client methods for init, checkpoint,\n   restore, fork, and GC operations",
		"Creating a checkpoint programmatically:",
	} {
		if strings.Contains(doc, forbidden) {
			t.Fatalf("docs/API_DOCUMENTATION.md still presents legacy API wording as primary guidance: %q", forbidden)
		}
	}
}

func TestDocs_FuzzCommandsDoNotUseRecursivePackagePattern(t *testing.T) {
	for _, doc := range fuzzCommandDocs() {
		t.Run(doc, func(t *testing.T) {
			for i, line := range strings.Split(readRepoFile(t, doc), "\n") {
				if strings.Contains(line, "go test") && strings.Contains(line, "-fuzz") && strings.Contains(line, "./test/fuzz/...") {
					t.Fatalf("%s:%d documents a multi-package Go fuzz command; use make fuzz/make fuzz-list for recursive release fuzzing or a single owning package for one target:\n%s", doc, i+1, line)
				}
			}
		})
	}
}

func TestDocs_PublicLibraryCleanupHidesV0RetentionSurface(t *testing.T) {
	fset := token.NewFileSet()
	path := repoFile(t, "pkg/jvs/client.go")
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse pkg/jvs/client.go: %v", err)
	}

	forbiddenCleanupOptionFields := map[string]bool{
		"KeepMinSnapshots": true,
		"KeepMinAge":       true,
	}
	ast.Inspect(file, func(node ast.Node) bool {
		typeSpec, ok := node.(*ast.TypeSpec)
		if !ok || typeSpec.Name.Name != "CleanupOptions" {
			return true
		}
		structType, ok := typeSpec.Type.(*ast.StructType)
		if !ok {
			t.Fatalf("pkg/jvs.CleanupOptions must remain a struct")
		}
		for _, field := range structType.Fields.List {
			for _, name := range field.Names {
				if forbiddenCleanupOptionFields[name.Name] {
					pos := fset.Position(name.Pos())
					t.Fatalf("%s:%d exposes v0 retention knob %s on public pkg/jvs.CleanupOptions", path, pos.Line, name.Name)
				}
			}
		}
		return false
	})

	ast.Inspect(file, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "PlanWithPolicy" {
			return true
		}
		pos := fset.Position(selector.Sel.Pos())
		t.Fatalf("%s:%d public pkg/jvs.Client.PreviewCleanup must not route through retention policy planning", path, pos.Line)
		return true
	})
}

func TestDocs_PublicModelGCJSONHidesPinAndRetentionFields(t *testing.T) {
	fset := token.NewFileSet()
	path := repoFile(t, "pkg/model/gc.go")
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse pkg/model/gc.go: %v", err)
	}

	forbiddenJSONFields := map[string]bool{
		"protected_by_pin":       true,
		"protected_by_retention": true,
		"retention_policy":       true,
	}
	foundGCPlan := false
	ast.Inspect(file, func(node ast.Node) bool {
		typeSpec, ok := node.(*ast.TypeSpec)
		if !ok || typeSpec.Name.Name != "GCPlan" {
			return true
		}
		foundGCPlan = true
		structType, ok := typeSpec.Type.(*ast.StructType)
		if !ok {
			t.Fatalf("pkg/model.GCPlan must remain a struct")
		}
		for _, field := range structType.Fields.List {
			if field.Tag == nil {
				continue
			}
			tag, err := strconv.Unquote(field.Tag.Value)
			if err != nil {
				t.Fatalf("unquote GCPlan tag %q: %v", field.Tag.Value, err)
			}
			for jsonField := range forbiddenJSONFields {
				if strings.Contains(tag, `json:"`+jsonField) {
					pos := fset.Position(field.Tag.Pos())
					t.Fatalf("%s:%d exposes v0-internal GC field %q in pkg/model.GCPlan JSON", path, pos.Line, jsonField)
				}
			}
		}
		return false
	})
	if !foundGCPlan {
		t.Fatalf("pkg/model.GCPlan not found")
	}

	forbiddenRetentionPolicyJSONFields := map[string]bool{
		"keep_min_snapshots": true,
		"keep_min_age":       true,
	}
	foundRetentionPolicy := false
	ast.Inspect(file, func(node ast.Node) bool {
		typeSpec, ok := node.(*ast.TypeSpec)
		if !ok || typeSpec.Name.Name != "RetentionPolicy" {
			return true
		}
		foundRetentionPolicy = true
		structType, ok := typeSpec.Type.(*ast.StructType)
		if !ok {
			t.Fatalf("pkg/model.RetentionPolicy must remain a struct while internal GC compatibility exists")
		}
		for _, field := range structType.Fields.List {
			if field.Tag == nil {
				continue
			}
			tag, err := strconv.Unquote(field.Tag.Value)
			if err != nil {
				t.Fatalf("unquote RetentionPolicy tag %q: %v", field.Tag.Value, err)
			}
			for jsonField := range forbiddenRetentionPolicyJSONFields {
				if strings.Contains(tag, `json:"`+jsonField) {
					pos := fset.Position(field.Tag.Pos())
					t.Fatalf("%s:%d exposes v0-internal retention field %q in pkg/model.RetentionPolicy JSON", path, pos.Line, jsonField)
				}
			}
		}
		return false
	})
	if !foundRetentionPolicy {
		t.Fatalf("pkg/model.RetentionPolicy not found")
	}
}

func TestDocs_DoctorStrictOwnsAuditChainContract(t *testing.T) {
	securityModel := readRepoFile(t, "docs/09_SECURITY_MODEL.md")
	lowerSecurityModel := strings.ToLower(securityModel)
	if !strings.Contains(lowerSecurityModel, "audit chain validation") && !strings.Contains(lowerSecurityModel, "audit chain checks") {
		t.Fatalf("security model must assign audit chain validation to the current integrity contract")
	}

	for _, doc := range []string{
		"docs/09_SECURITY_MODEL.md",
		"docs/11_CONFORMANCE_TEST_PLAN.md",
		"docs/12_RELEASE_POLICY.md",
		"docs/13_OPERATION_RUNBOOK.md",
	} {
		t.Run(doc, func(t *testing.T) {
			scanPublicDocLines(t, doc, func(lineNo int, line string) {
				lower := strings.ToLower(line)
				if strings.Contains(lower, "jvs verify --all") && strings.Contains(lower, "audit") {
					t.Fatalf("%s:%d routes audit validation through an old hidden verify command; audit chain belongs to doctor --strict:\n%s", doc, lineNo, line)
				}
			})
		})
	}
}

func TestDocs_DoctorRuntimeRepairActionsUsePublicIDs(t *testing.T) {
	expected := publicRuntimeRepairActionIDs()
	assertSameStringSet(t, "doctor.RuntimeRepairActionIDs()", doctor.RuntimeRepairActionIDs(), expected)

	for _, doc := range releaseFacingRuntimeRepairActionDocs() {
		t.Run(doc, func(t *testing.T) {
			body := releaseFacingClaimBody(t, doc)
			assertSameStringSet(t, doc+" runtime repair action IDs", runtimeRepairActionIDsMentioned(body), expected)
			scanPublicDocLines(t, doc, func(lineNo int, line string) {
				for _, id := range unsupportedDoctorRepairActionIDs() {
					if lineContainsRepairActionID(line, id) {
						t.Fatalf("%s:%d documents unsupported doctor repair action %q:\n%s", doc, lineNo, id, line)
					}
				}
			})
		})
	}
}

func TestDocs_MigrationGuidanceUsesFreshDestinationRuntimeRepairFlow(t *testing.T) {
	for _, doc := range releaseFacingMigrationRuntimeFlowDocs() {
		t.Run(doc, func(t *testing.T) {
			body := releaseFacingClaimBody(t, doc)
			for _, required := range []string{
				"offline whole-folder copy",
				"fresh destination",
				"jvs doctor --strict --repair-runtime",
				"fresh cleanup preview",
			} {
				requireReleaseReadinessText(t, "migration runtime repair flow "+doc, body, required)
			}
			scanPublicDocLines(t, doc, func(lineNo int, line string) {
				if staleMigrationRuntimeExcludePattern.MatchString(line) {
					t.Fatalf("%s:%d uses stale exclude/sync runtime migration guidance; require fresh-destination offline copy, destination repair-runtime, and fresh cleanup preview:\n%s", doc, lineNo, line)
				}
				if staleMigrationSyncVocabularyPattern.MatchString(line) {
					t.Fatalf("%s:%d uses stale sync migration vocabulary; release-facing migration guidance must use offline whole-folder copy and fresh destination repair wording:\n%s", doc, lineNo, line)
				}
			})
		})
	}
}

func TestDocs_ChangelogLatestReleaseNotesHaveReadinessSections(t *testing.T) {
	changelog := readRepoFile(t, "docs/99_CHANGELOG.md")
	entry := firstChangelogEntry(changelog)
	for _, heading := range []string{
		"### Known limitations",
		"### Risk labels",
		"### Migration notes",
		"### Release evidence",
	} {
		if !strings.Contains(entry, heading) {
			t.Fatalf("latest changelog entry must include %q", heading)
		}
	}
}

func TestDocs_ReleaseEvidenceLedgerCoversLatestChangelogTagDate(t *testing.T) {
	latestHeading := latestChangelogHeading(t)
	ledger := readRepoFile(t, "docs/RELEASE_EVIDENCE.md")
	if !strings.Contains(ledger, latestHeading) {
		t.Fatalf("release evidence ledger must include latest changelog heading %q", latestHeading)
	}
	entry := releaseEvidenceEntry(t, ledger, latestHeading)
	if releaseEvidenceClaimsFinalTaggedRelease(entry) {
		requireFinalTaggedReleaseEvidence(t, latestHeading, entry)
		return
	}
	requireCandidateReleaseEvidence(t, latestHeading, entry)
}

func TestDocs_V042GAReleaseEvidenceRecordsPublishedRelease(t *testing.T) {
	const heading = "## v0.4.2 - 2026-04-28"
	const commit = "c21b676dfb04d32f8cf3b9fa301e465f6886ca94"
	const runURL = "https://github.com/agentsmith-project/jvs/actions/runs/25056873829"
	const releaseURL = "https://github.com/agentsmith-project/jvs/releases/tag/v0.4.2"

	changelogEntry := changelogEntry(t, readRepoFile(t, "docs/99_CHANGELOG.md"), heading)
	if !strings.Contains(changelogEntry, heading) {
		t.Fatalf("changelog must retain the published v0.4.2 GA release heading %q", heading)
	}
	for _, forbidden := range []string{
		"not final",
		"not tagged",
		"not published",
		"pending final",
	} {
		requireReleaseReadinessAbsentText(t, "published v0.4.2 changelog entry", changelogEntry, forbidden)
	}
	for _, required := range []string{
		"GA release",
		runURL,
		releaseURL,
		"draft=false",
		"prerelease=false",
		"Asset count: `12`",
		"Tag source archive evidence class: `GA candidate readiness`",
		"Final evidence location: GitHub Release page and post-release main ledger",
		"Tag movement: `v0.4.2` was not moved",
	} {
		requireReleaseReadinessText(t, "published v0.4.2 changelog entry", changelogEntry, required)
	}

	entry := releaseEvidenceEntry(t, readRepoFile(t, "docs/RELEASE_EVIDENCE.md"), heading)
	requireFinalTaggedReleaseEvidence(t, heading, entry)
	for _, required := range []string{
		"Evidence class: Final release evidence",
		"Tag: `v0.4.2`",
		"Final tagged commit: `" + commit + "`",
		"Commit message: `ci: publish release signatures as bundles`",
		"Status: PASS",
		runURL,
		releaseURL,
		"draft=false",
		"prerelease=false",
		"Published artifact count: `12`",
		"sha256sum --check --strict SHA256SUMS",
		"jvs-linux-amd64 --help",
		"cosign version: `v3.0.5`",
		"https://github.com/agentsmith-project/jvs/.github/workflows/ci.yml@refs/tags/v0.4.2",
		"https://token.actions.githubusercontent.com",
		"Tag source archive evidence class: `GA candidate readiness`",
		"Final evidence location: GitHub Release page and post-release main ledger",
		"Tag movement: `v0.4.2` was not moved",
	} {
		requireReleaseReadinessText(t, "published v0.4.2 release evidence", entry, required)
	}
	for _, asset := range []string{
		"jvs-darwin-amd64",
		"jvs-darwin-amd64.bundle",
		"jvs-darwin-arm64",
		"jvs-darwin-arm64.bundle",
		"jvs-linux-amd64",
		"jvs-linux-amd64.bundle",
		"jvs-linux-arm64",
		"jvs-linux-arm64.bundle",
		"jvs-windows-amd64.exe",
		"jvs-windows-amd64.exe.bundle",
		"SHA256SUMS",
		"SHA256SUMS.bundle",
	} {
		requireReleaseReadinessText(t, "published v0.4.2 release evidence assets", entry, asset)
	}
	for _, forbidden := range []string{
		"not final",
		"not tagged",
		"not published",
		"pending final",
	} {
		requireReleaseReadinessAbsentText(t, "published v0.4.2 release evidence", entry, forbidden)
	}
}

func TestDocs_ReleaseEvidenceV046GACandidateReadinessRecordsPendingRelease(t *testing.T) {
	const heading = "## v0.4.6 - 2026-05-03"
	const evidenceLink = "RELEASE_EVIDENCE.md#v046---2026-05-03"

	latestHeading := latestChangelogHeading(t)
	if latestHeading != heading {
		t.Fatalf("latest changelog entry must be the v0.4.6 GA candidate heading %q, got %q", heading, latestHeading)
	}

	changelogEntry := changelogEntry(t, readRepoFile(t, "docs/99_CHANGELOG.md"), heading)
	for _, required := range []string{
		"GA candidate",
		evidenceLink,
		"not final",
		"not tagged",
		"not published",
		"pending final tag",
		"Candidate target tag: `v0.4.6`",
		"repo/workspace lifecycle management",
		"repo move/rename/detach",
		"workspace move/rename/delete preview/run",
		"recovery posture",
		"external workspace pending lifecycle evidence",
		"machine-readable `recommended_next_command`",
		"repo clone workflow",
		"filesystem-aware transfer planning/implementation",
	} {
		requireReleaseReadinessText(t, "v0.4.6 changelog candidate entry", changelogEntry, required)
	}

	entry := releaseEvidenceEntry(t, readRepoFile(t, "docs/RELEASE_EVIDENCE.md"), heading)
	requireCandidateReleaseEvidence(t, heading, entry)
	for _, forbidden := range []struct {
		name    string
		pattern *regexp.Regexp
	}{
		{name: "PASS status", pattern: releaseEvidenceStatusPassPattern},
		{name: "release-gate PASS row", pattern: releaseEvidenceGatePassPattern},
		{name: "published artifact count", pattern: releaseEvidencePublishedArtifactCountPattern},
		{name: "final tag line", pattern: releaseEvidenceTagPattern},
		{name: "final tagged commit", pattern: releaseEvidenceCommitPattern},
	} {
		if forbidden.pattern.MatchString(entry) {
			t.Fatalf("v0.4.6 candidate release evidence must not claim %s before final publication", forbidden.name)
		}
	}
}

func TestDocs_ReleaseEvidenceDisclosesSourceArchivePublicationBoundary(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "release policy", body: readRepoFile(t, "docs/12_RELEASE_POLICY.md")},
		{name: "release evidence ledger", body: readRepoFile(t, "docs/RELEASE_EVIDENCE.md")},
		{name: "release changelog", body: firstChangelogEntry(readRepoFile(t, "docs/99_CHANGELOG.md"))},
		{name: "release docs index", body: readRepoFile(t, "docs/release/README.md")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, required := range []string{
				"source archive",
				"tag snapshot",
				"publication final evidence",
				"GitHub Release page",
				"post-release main ledger",
			} {
				requireReleaseReadinessText(t, tc.name, tc.body, required)
			}
		})
	}
}

func TestDocs_V042FinalEvidenceDisclosesCandidateTagSourceArchive(t *testing.T) {
	const finalHeading = "## v0.4.2 - 2026-04-28"
	const tag = "v0.4.2"

	tagSourceLedger := gitShowFileAtRef(t, "refs/tags/"+tag, "docs/RELEASE_EVIDENCE.md")
	tagSourceEntry := releaseEvidenceEntryForVersion(t, tagSourceLedger, tag)
	tagSourceClass := releaseEvidenceClass(t, "v0.4.2 tag source archive", tagSourceEntry)
	if !releaseEvidenceClassIsCandidateReadiness(tagSourceClass) {
		t.Fatalf("v0.4.2 tag source archive must be candidate/readiness evidence, got %q", tagSourceClass)
	}

	missing := candidateTagSourceArchiveDisclosureMissing(tag, tagSourceClass,
		releaseEvidenceDisclosureTarget{
			name: "final release evidence ledger entry",
			body: releaseEvidenceEntry(t, readRepoFile(t, "docs/RELEASE_EVIDENCE.md"), finalHeading),
		},
		releaseEvidenceDisclosureTarget{
			name: "final changelog entry",
			body: changelogEntry(t, readRepoFile(t, "docs/99_CHANGELOG.md"), finalHeading),
		},
	)
	if len(missing) > 0 {
		t.Fatalf("v0.4.2 final release docs must disclose candidate tag source archive:\n%s", strings.Join(missing, "\n"))
	}
}

func TestDocs_LatestFinalReleaseDisclosesCandidateTagSourceArchive(t *testing.T) {
	latestHeading := latestChangelogHeading(t)
	ledger := readRepoFile(t, "docs/RELEASE_EVIDENCE.md")
	finalEntry := releaseEvidenceEntry(t, ledger, latestHeading)
	if !releaseEvidenceClaimsFinalTaggedRelease(finalEntry) {
		t.Skipf("latest release evidence %q is not final tagged release evidence", latestHeading)
	}

	tagMatch := releaseEvidenceTagPattern.FindStringSubmatch(finalEntry)
	if tagMatch == nil {
		t.Fatalf("latest final release evidence %q must record a final Tag line", latestHeading)
	}
	tag := tagMatch[1]

	tagSourceLedger := gitShowFileAtRef(t, "refs/tags/"+tag, "docs/RELEASE_EVIDENCE.md")
	tagSourceEntry := releaseEvidenceEntryForVersion(t, tagSourceLedger, tag)
	tagSourceClass := releaseEvidenceClass(t, tag+" tag source archive", tagSourceEntry)
	if !releaseEvidenceClassIsCandidateReadiness(tagSourceClass) {
		return
	}

	missing := candidateTagSourceArchiveDisclosureMissing(tag, tagSourceClass,
		releaseEvidenceDisclosureTarget{
			name: "latest final release evidence ledger entry",
			body: finalEntry,
		},
		releaseEvidenceDisclosureTarget{
			name: "latest final changelog entry",
			body: firstChangelogEntry(readRepoFile(t, "docs/99_CHANGELOG.md")),
		},
	)
	if len(missing) > 0 {
		t.Fatalf("latest final release docs must disclose candidate tag source archive:\n%s", strings.Join(missing, "\n"))
	}
}

func TestDocs_ReleaseEvidenceTagSourceDisclosureGateRequiresFinalDocs(t *testing.T) {
	const tag = "v0.4.3"
	const class = "GA candidate readiness"

	missing := candidateTagSourceArchiveDisclosureMissing(tag, class,
		releaseEvidenceDisclosureTarget{
			name: "final release evidence ledger entry",
			body: strings.Join([]string{
				"## v0.4.3 - 2026-05-01",
				"",
				"### Release identity",
				"",
				"- Evidence class: Final release evidence",
				"- Status: PASS",
				"- Tag: `v0.4.3`",
				"- Final tagged commit: `0123456789012345678901234567890123456789`",
			}, "\n"),
		},
		releaseEvidenceDisclosureTarget{
			name: "final changelog entry",
			body: strings.Join([]string{
				"## v0.4.3 - 2026-05-01",
				"",
				"### Release evidence",
				"",
				"- Final tag `v0.4.3` points at commit",
				"  `0123456789012345678901234567890123456789`.",
			}, "\n"),
		},
	)
	if len(missing) == 0 {
		t.Fatalf("candidate tag source archive disclosure gate must report missing final docs disclosures")
	}

	joined := strings.Join(missing, "\n")
	for _, required := range []string{
		"final release evidence ledger entry",
		"final changelog entry",
		"Tag source archive evidence class: `GA candidate readiness`",
		"Final evidence location: GitHub Release page and post-release main ledger",
		"https://github.com/agentsmith-project/jvs/releases/tag/v0.4.3",
		"source archive boundary",
		"immutable tag snapshot",
		"publication final evidence",
		"GitHub Release page",
		"post-release main ledger",
		"Tag movement: `v0.4.3` was not moved",
	} {
		if !strings.Contains(joined, required) {
			t.Fatalf("candidate tag source archive disclosure gate diagnostics must include %q; got:\n%s", required, joined)
		}
	}
}

func TestDocs_SavePointWorkspaceSemanticsIsSupportingNonReleaseFacingReference(t *testing.T) {
	overview := readRepoFile(t, "docs/00_OVERVIEW.md")
	overviewSupportingMarker := "Supporting non-release-facing redesign/reference:"
	if !strings.Contains(overview, overviewSupportingMarker) {
		t.Fatalf("docs/00_OVERVIEW.md must classify docs/21_SAVE_POINT_WORKSPACE_SEMANTICS.md under %q", overviewSupportingMarker)
	}
	overviewActiveSpecs := textBetweenMarkers(t, overview, "Current active specs:", overviewSupportingMarker)
	if strings.Contains(overviewActiveSpecs, "`21_SAVE_POINT_WORKSPACE_SEMANTICS.md`") {
		t.Fatalf("docs/21_SAVE_POINT_WORKSPACE_SEMANTICS.md is non-release-facing and must not be listed under Current active specs")
	}
	overviewSupporting := overview[strings.Index(overview, overviewSupportingMarker):]
	for _, required := range []string{
		"`21_SAVE_POINT_WORKSPACE_SEMANTICS.md`",
		overviewSupportingMarker,
	} {
		requireReleaseReadinessText(t, "overview supporting non-release-facing docs", overviewSupporting, required)
	}

	policySection := markdownSectionByHeading(t,
		"docs/12_RELEASE_POLICY.md",
		readRepoFile(t, "docs/12_RELEASE_POLICY.md"),
		"## Documentation Gates",
	)
	for _, required := range []string{
		"`docs/21_SAVE_POINT_WORKSPACE_SEMANTICS.md`",
		"supporting non-release-facing reference",
	} {
		requireReleaseReadinessText(t, "release policy documentation gates", policySection, required)
	}

	ledgerEntry := releaseEvidenceEntry(t,
		readRepoFile(t, "docs/RELEASE_EVIDENCE.md"),
		"## v0.4.2 - 2026-04-28",
	)
	for _, required := range []string{
		"`docs/21_SAVE_POINT_WORKSPACE_SEMANTICS.md`",
		"Supporting non-release-facing reference",
	} {
		requireReleaseReadinessText(t, "release evidence GA docs evidence", ledgerEntry, required)
	}

	matrixPromise := markdownSectionByHeading(t,
		"docs/14_TRACEABILITY_MATRIX.md",
		readRepoFile(t, "docs/14_TRACEABILITY_MATRIX.md"),
		"## Promise 1: Real Folder Save Points",
	)
	normativeDocs := textBetweenMarkers(t, matrixPromise, "Normative docs:", "Evidence:")
	if strings.Contains(normativeDocs, "`docs/21_SAVE_POINT_WORKSPACE_SEMANTICS.md`") {
		t.Fatalf("docs/21_SAVE_POINT_WORKSPACE_SEMANTICS.md is non-release-facing and must not be listed as a normative release-facing doc")
	}
	for _, required := range []string{
		"`docs/21_SAVE_POINT_WORKSPACE_SEMANTICS.md`",
		"Supporting non-release-facing reference",
	} {
		requireReleaseReadinessText(t, "traceability promise 1", matrixPromise, required)
	}
}

func TestDocs_ReleaseEvidenceDoesNotMixCandidateAndFinalSemantics(t *testing.T) {
	latestHeading := latestChangelogHeading(t)
	entry := releaseEvidenceEntry(t, readRepoFile(t, "docs/RELEASE_EVIDENCE.md"), latestHeading)
	if releaseEvidenceClaimsFinalTaggedRelease(entry) {
		for _, forbidden := range []string{"pending final", "not published"} {
			if strings.Contains(strings.ToLower(entry), forbidden) {
				t.Fatalf("final tagged release evidence %q must not include unresolved %q language", latestHeading, forbidden)
			}
		}
		if strings.Contains(strings.ToLower(entry), "candidate") {
			for _, required := range []string{
				"source archive",
				"tag snapshot",
				"publication final evidence",
				"GitHub Release page",
				"post-release main ledger",
				"tag was not moved",
			} {
				requireReleaseReadinessText(t, "final tagged release source archive boundary "+latestHeading, entry, required)
			}
		}
		if strings.Contains(strings.ToLower(entry), "candidate") &&
			!strings.Contains(entry, "Tag source archive evidence class: `GA candidate readiness`") {
			t.Fatalf("final tagged release evidence %q must not include candidate/pending language", latestHeading)
		}
		return
	}

	for _, forbidden := range []struct {
		name    string
		pattern *regexp.Regexp
	}{
		{name: "PASS status", pattern: releaseEvidenceStatusPassPattern},
		{name: "release-gate PASS row", pattern: releaseEvidenceGatePassPattern},
		{name: "published artifact count", pattern: releaseEvidencePublishedArtifactCountPattern},
		{name: "final tag line", pattern: releaseEvidenceTagPattern},
		{name: "final tagged commit", pattern: releaseEvidenceCommitPattern},
	} {
		if forbidden.pattern.MatchString(entry) {
			t.Fatalf("candidate release evidence %q must not claim %s before the final tagged release", latestHeading, forbidden.name)
		}
	}
}

func TestDocs_ReleaseEvidenceLedgerContainsPolicyRequiredArtifacts(t *testing.T) {
	policy := readRepoFile(t, "docs/12_RELEASE_POLICY.md")
	policySection := markdownSectionByHeading(t, "docs/12_RELEASE_POLICY.md", policy, "## Required Release Artifacts")
	for _, required := range []string{
		"release evidence ledger",
		"make release-gate",
		"coverage",
		"representative repo",
		"artifact",
		"signing",
		"runbook",
	} {
		requireReleaseReadinessText(t, "release policy required artifacts", policySection, required)
	}

	latestHeading := latestChangelogHeading(t)
	entry := releaseEvidenceEntry(t, readRepoFile(t, "docs/RELEASE_EVIDENCE.md"), latestHeading)
	for _, required := range []string{
		"Release identity",
		"make release-gate",
		"docs-contract",
		"ci-contract",
		"test-race",
		"test-cover",
		"lint",
		"build",
		"conformance",
		"library",
		"regression",
		"fuzz-tests",
		"fuzz",
		"Coverage total",
		"Coverage threshold",
		"jvs doctor --strict",
		"integrity",
		"GA docs",
		"artifact",
		"signing",
		"runbook",
	} {
		requireReleaseReadinessText(t, "release evidence entry", entry, required)
	}
}

func TestDocs_ReleaseEvidenceLatestChangelogLinksLedger(t *testing.T) {
	entry := firstChangelogEntry(readRepoFile(t, "docs/99_CHANGELOG.md"))
	if !strings.Contains(entry, "### Release evidence") {
		t.Fatalf("latest changelog entry must include release evidence section")
	}
	if !strings.Contains(entry, "RELEASE_EVIDENCE.md") {
		t.Fatalf("latest changelog release evidence section must link docs/RELEASE_EVIDENCE.md")
	}
}

func TestDocs_ReleaseReadinessSectionsConsistentWithPolicy(t *testing.T) {
	changelogEntry := firstChangelogEntry(readRepoFile(t, "docs/99_CHANGELOG.md"))
	if !releaseReadinessHeadingPattern.MatchString(changelogEntry) {
		t.Fatalf("latest changelog entry must be date/tag shaped like v0.x.y - YYYY-MM-DD")
	}

	for _, heading := range []string{
		"### Highlights",
		"### Breaking changes",
		"### Known limitations",
		"### Risk labels",
		"### Migration notes",
		"### Release evidence",
		"### Release artifacts",
	} {
		if !strings.Contains(changelogEntry, heading) {
			t.Fatalf("latest changelog entry must include %q", heading)
		}
	}

	workflowNotes := normalizeMarkdownEscapes(readRepoFile(t, ".github/workflows/ci.yml"))
	for _, required := range []string{
		"## Build Artifacts",
		"### Binaries",
		"### Verification",
		"## Breaking changes",
		"## Known limitations",
		"## Risk labels",
		"## Migration notes",
		"SHA256SUMS",
		".bundle",
		"SHA256SUMS.bundle",
		"cosign verify-blob",
	} {
		if !strings.Contains(workflowNotes, required) {
			t.Fatalf("generated release notes must include %q", required)
		}
	}

	for _, required := range []string{
		"remote push/pull",
		"in-JVS signing commands",
		"compression contracts",
		"merge/rebase",
		"complex retention policy flags",
		"integrity",
		"migration",
		"jvs doctor --strict",
		"jvs doctor --strict --repair-runtime",
	} {
		requireReleaseReadinessText(t, "latest changelog entry", changelogEntry, required)
		requireReleaseReadinessText(t, "generated release notes", workflowNotes, required)
	}
	requireReleaseReadinessAnyText(t, "latest changelog entry", changelogEntry,
		"partial-save contracts",
		"partial save contracts",
	)
	requireReleaseReadinessAnyText(t, "generated release notes", workflowNotes,
		"partial-save contracts",
		"partial save contracts",
	)
	requireReleaseReadinessAbsentText(t, "latest changelog entry", changelogEntry, "partial checkpoint contracts")
	requireReleaseReadinessAbsentText(t, "generated release notes", workflowNotes, "partial checkpoint contracts")
	requireReleaseReadinessAbsentText(t, "latest changelog entry", changelogEntry, "v0 does not include signing commands")
	requireReleaseReadinessAbsentText(t, "generated release notes", workflowNotes, "v0 does not include signing commands")
	requireReleaseReadinessAbsentText(t, "generated release notes", workflowNotes, "SHA256SUMS.sig")
	requireReleaseReadinessAbsentText(t, "generated release notes", workflowNotes, "jvs-linux-amd64.sig")
	requireReleaseReadinessAbsentText(t, "generated release notes", workflowNotes, "jvs-linux-amd64.pem")
	requireReleaseReadinessAbsentText(t, "generated release notes", workflowNotes, ".sig and .pem")
	requireReleaseReadinessAbsentText(t, "generated release notes", workflowNotes, "--signature")

	for _, required := range runtimeStateBoundaryTerms() {
		requireReleaseReadinessText(t, "latest changelog entry runtime-state boundary", changelogEntry, required)
		requireReleaseReadinessText(t, "generated release notes runtime-state boundary", workflowNotes, required)
	}
}

func TestDocs_RiskLabelsMatchThreatModelAndReleaseNotes(t *testing.T) {
	labels := riskLabelsFromThreatModel(t)
	if len(labels) == 0 {
		threatModel := readRepoFile(t, "docs/10_THREAT_MODEL.md")
		for _, required := range []string{"## Threats", "## Controls", "## Residual Risks"} {
			if !strings.Contains(threatModel, required) {
				t.Fatalf("docs/10_THREAT_MODEL.md must include current threat model section %q", required)
			}
		}
		return
	}

	changelogEntry := firstChangelogEntry(readRepoFile(t, "docs/99_CHANGELOG.md"))
	workflowNotes := normalizeMarkdownEscapes(readRepoFile(t, ".github/workflows/ci.yml"))
	for _, label := range labels {
		requireReleaseReadinessText(t, "latest changelog entry", changelogEntry, label)
		requireReleaseReadinessText(t, "generated release notes", workflowNotes, label)
	}
}

func TestDocs_PerformanceClaimPatternMatchesO1(t *testing.T) {
	tests := []struct {
		name string
		line string
		want bool
	}{
		{
			name: "matches O(1) followed by punctuation",
			line: "The `juicefs-clone` engine provides O(1), metadata-clone save points on supported JuiceFS.",
			want: true,
		},
		{
			name: "matches O(1) followed by space",
			line: "Save point speed is O(1) with supported JuiceFS.",
			want: true,
		},
		{
			name: "matches constant-time",
			line: "Restore uses constant-time metadata clone with supported `juicefs-clone`.",
			want: true,
		},
		{
			name: "ignores O(n)",
			line: "The copy fallback remains O(n) in payload size and file count.",
			want: false,
		},
		{
			name: "ignores words containing instant",
			line: "The instantiation code path is unrelated.",
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := releaseFacingPerformanceClaimPattern.MatchString(tt.line)
			if got != tt.want {
				t.Fatalf("releaseFacingPerformanceClaimPattern.MatchString(%q) = %v, want %v", tt.line, got, tt.want)
			}
		})
	}
}

func TestDocs_PerformanceClaimsRequireRealEngineScope(t *testing.T) {
	tests := []struct {
		name string
		line string
		want bool
	}{
		{
			name: "juicefs clone engine scope",
			line: "Save point speed is O(1) with the `juicefs-clone` engine on supported JuiceFS.",
			want: true,
		},
		{
			name: "juicefs storage scope",
			line: "O(1) operations are available on supported JuiceFS mounts.",
			want: true,
		},
		{
			name: "juicefs clone supported storage scope",
			line: "`juicefs-clone` on supported JuiceFS provides O(1) whole-tree save point restore.",
			want: true,
		},
		{
			name: "juicefs without support qualifier is not scope",
			line: "Save points are O(1) with JuiceFS.",
			want: false,
		},
		{
			name: "reflink copy is not whole tree constant scope",
			line: "`reflink-copy` provides O(1) restore.",
			want: false,
		},
		{
			name: "copy on write is not whole tree constant scope",
			line: "copy-on-write provides O(1) whole-tree save point restore.",
			want: false,
		},
		{
			name: "cow is not whole tree constant scope",
			line: "CoW save points are constant-time for whole trees.",
			want: false,
		},
		{
			name: "reflink negative O1 disclaimer is allowed",
			line: "`reflink-copy` still walks the tree and is not an O(1) whole-tree restore promise.",
			want: true,
		},
		{
			name: "with alone is not scope",
			line: "Save points are O(1) with healthy storage.",
			want: false,
		},
		{
			name: "on alone is not scope",
			line: "Save points are O(1) on production systems.",
			want: false,
		},
		{
			name: "engine alone is not scope",
			line: "Engine-scoped O(1) save points are available.",
			want: false,
		},
		{
			name: "benchmark alone is not scope",
			line: "Benchmark result: O(1) save point creation.",
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := scopedPerformanceClaim(tt.line)
			if got != tt.want {
				t.Fatalf("scopedPerformanceClaim(%q) = %v, want %v", tt.line, got, tt.want)
			}
		})
	}
}

func TestDocs_SavePointReferenceClaimsRequireEngineScope(t *testing.T) {
	tests := []struct {
		name      string
		line      string
		wantClaim bool
		wantScope bool
	}{
		{
			name:      "juicefs clone reference claim",
			line:      "With `juicefs-clone`, save points are metadata references, not data copies.",
			wantClaim: true,
			wantScope: true,
		},
		{
			name:      "supported juicefs reference claim",
			line:      "On supported JuiceFS, save points are references, not copies.",
			wantClaim: true,
			wantScope: true,
		},
		{
			name:      "generic references not copies claim",
			line:      "Your actual workspace data is stored once - save points are references, not copies.",
			wantClaim: true,
			wantScope: false,
		},
		{
			name:      "generic references not copies claim with negative O1 disclaimer",
			line:      "Save points are references, not copies; not O(1).",
			wantClaim: true,
			wantScope: false,
		},
		{
			name:      "generic small reference files claim",
			line:      "- **Save points:** Small reference files",
			wantClaim: true,
			wantScope: false,
		},
		{
			name:      "generic references claim",
			line:      "| Repository size | Blobs grow endlessly | Save points are references |",
			wantClaim: true,
			wantScope: false,
		},
		{
			name:      "non reference claim",
			line:      "Save points are immutable.",
			wantClaim: false,
			wantScope: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotClaim := savePointReferenceClaimPattern.MatchString(tt.line)
			if gotClaim != tt.wantClaim {
				t.Fatalf("savePointReferenceClaimPattern.MatchString(%q) = %v, want %v", tt.line, gotClaim, tt.wantClaim)
			}
			if !gotClaim {
				return
			}
			got := scopedSavePointReferenceClaim(tt.line)
			if got != tt.wantScope {
				t.Fatalf("scopedSavePointReferenceClaim(%q) = %v, want %v", tt.line, got, tt.wantScope)
			}
		})
	}
}

func TestDocs_SavePointReferenceClaimsAreEngineScopedAcrossReleaseFacingDocs(t *testing.T) {
	for _, doc := range activePublicContractDocs() {
		t.Run(doc, func(t *testing.T) {
			scanPublicDocLines(t, doc, func(lineNo int, line string) {
				if !savePointReferenceClaimPattern.MatchString(line) {
					return
				}
				if scopedSavePointReferenceClaim(line) {
					return
				}
				t.Fatalf("%s:%d has an unscoped save point reference/not-copy storage claim:\n%s", doc, lineNo, line)
			})
		})
	}
}

func TestDocs_PerformanceClaimsAreEngineScopedAcrossReleaseFacingDocs(t *testing.T) {
	for _, doc := range activePublicContractDocs() {
		t.Run(doc, func(t *testing.T) {
			body := releaseFacingClaimBody(t, doc)
			previousLine := ""
			for lineNo, line := range strings.Split(body, "\n") {
				if !releaseFacingPerformanceClaimPattern.MatchString(line) {
					previousLine = line
					continue
				}
				if scopedPerformanceClaim(line) || scopedPerformanceClaim(previousLine) {
					previousLine = line
					continue
				}
				t.Fatalf("%s:%d has an unscoped constant-time/O(1) claim:\n%s", doc, lineNo+1, line)
			}
		})
	}
}

func TestDocs_PerformanceGuideAvoidsPortableLatencyPromises(t *testing.T) {
	scanPublicDocLines(t, "docs/PERFORMANCE.md", func(lineNo int, line string) {
		if portableLatencyPromisePattern.MatchString(line) {
			t.Fatalf("docs/PERFORMANCE.md:%d publishes fixed size-to-latency numbers as portable guidance; use regression-baseline wording without fixed portable latency promises:\n%s", lineNo, line)
		}
	})
}

func TestDocs_PerformanceResultsCoverRequiredGAEngines(t *testing.T) {
	results := readRepoFile(t, "docs/PERFORMANCE_RESULTS.md")
	for _, required := range []string{
		"## GA Result Boundaries",
		"Required engine coverage",
		"Required benchmark package commands",
		"## Public Interpretation",
		"`juicefs-clone`",
		"`reflink-copy`",
		"`copy`",
		"./internal/snapshot",
		"./internal/restore",
		"./internal/gc",
		"./internal/worktree",
	} {
		if !strings.Contains(results, required) {
			t.Fatalf("docs/PERFORMANCE_RESULTS.md must include current GA boundary/command %q", required)
		}
	}
	for _, forbidden := range []string{
		"v7.2",
		"v8.0",
		"v7.3",
		"make benchmark",
		"scripts/benchmark.sh",
		"scripts/collect_results.sh",
		"scripts/compare_benchmarks.sh",
		"N/A",
		"Targets for",
	} {
		if strings.Contains(results, forbidden) {
			t.Fatalf("docs/PERFORMANCE_RESULTS.md contains stale or unsupported benchmark language %q", forbidden)
		}
	}
}

func markdownDocsUnder(t *testing.T, dir string) []string {
	t.Helper()
	var docs []string
	root := repoFile(t, dir)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			return nil
		}
		rel, err := filepath.Rel(repoFile(t), path)
		if err != nil {
			return err
		}
		docs = append(docs, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	return docs
}

func userFacingPlanPlaceholderDocs(t *testing.T) []string {
	t.Helper()
	docs := []string{"README.md"}
	for _, doc := range markdownDocsUnder(t, "docs/user") {
		docs = appendUniqueString(docs, doc)
	}
	return docs
}

func userWorkflowPlaceholderDocs() []string {
	return []string{
		"docs/user/quickstart.md",
		"docs/user/best-practices.md",
		"docs/user/examples.md",
		"docs/user/tutorials.md",
	}
}

func userDocCanonicalWorkflowPlaceholderAlias(placeholder string) (string, bool) {
	switch placeholder {
	case "<printed-view-path>":
		return "<view-path>", true
	default:
		return "", false
	}
}

func userDocTypedPlaceholders(body string) []string {
	var placeholders []string
	for _, placeholder := range userDocTypedPlaceholderPattern.FindAllString(body, -1) {
		placeholders = appendUniqueString(placeholders, placeholder)
	}
	return placeholders
}

func userDocExplainsPlaceholder(body, placeholder string) bool {
	for _, line := range markdownNonFencedCodeLines(body) {
		if userDocPlaceholderExplanationLine(line, placeholder) {
			return true
		}
	}
	return false
}

func userDocLinksPlaceholderExplanation(t *testing.T, sourceDoc, body, placeholder string) bool {
	t.Helper()
	for _, match := range markdownDocLinkPattern.FindAllStringSubmatch(body, -1) {
		target := markdownDocLinkTarget(t, sourceDoc, match[1])
		if target == "" || !strings.HasPrefix(target, "docs/user/") {
			continue
		}
		if userDocExplainsPlaceholder(readRepoFile(t, target), placeholder) {
			return true
		}
	}
	return false
}

func markdownNonFencedCodeLines(body string) []string {
	var lines []string
	inFence := false
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

func userDocPlaceholderExplanationLine(line, placeholder string) bool {
	trimmed := strings.TrimSpace(line)
	if !strings.Contains(trimmed, placeholder) {
		return false
	}
	if strings.HasPrefix(trimmed, "|") && strings.Count(trimmed, "|") >= 3 {
		return true
	}
	lower := strings.ToLower(trimmed)
	for _, marker := range []string{
		"means",
		"use ",
		"replace",
		"printed by",
		"printed from",
		"folder path",
		"original folder",
		"view path",
		"view id",
		"plan id",
		"save point id",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func allDocsContractMarkdownDocs(t *testing.T) []string {
	t.Helper()
	docs := []string{
		"README.md",
		"CONTRIBUTING.md",
		"SECURITY.md",
	}
	for _, doc := range markdownDocsUnder(t, "docs") {
		docs = appendUniqueString(docs, doc)
	}
	return docs
}

func normalizeMarkdownEscapes(body string) string {
	return strings.ReplaceAll(body, "\\`", "`")
}

func requireReleaseReadinessText(t *testing.T, name, body, required string) {
	t.Helper()
	if !strings.Contains(strings.ToLower(body), strings.ToLower(required)) {
		t.Fatalf("%s must include %q", name, required)
	}
}

func assertNoPathShapedRuntimeMechanisms(t *testing.T, name, body string) {
	t.Helper()
	for lineNo, line := range strings.Split(body, "\n") {
		assertNoPathShapedRuntimeMechanismLine(t, name, lineNo+1, line)
	}
}

func assertNoPathShapedRuntimeMechanismLine(t *testing.T, name string, lineNo int, line string) {
	t.Helper()
	if pathShapedRuntimeMechanismPattern.MatchString(line) {
		t.Fatalf("%s:%d exposes path-shaped internal runtime mechanism; use generic runtime-state wording instead:\n%s", name, lineNo, line)
	}
}

func requireReleaseReadinessAnyText(t *testing.T, name, body string, requiredAny ...string) {
	t.Helper()
	lowerBody := strings.ToLower(body)
	for _, required := range requiredAny {
		if strings.Contains(lowerBody, strings.ToLower(required)) {
			return
		}
	}
	t.Fatalf("%s must include one of %q", name, requiredAny)
}

func requireReleaseReadinessAbsentText(t *testing.T, name, body, forbidden string) {
	t.Helper()
	if strings.Contains(strings.ToLower(body), strings.ToLower(forbidden)) {
		t.Fatalf("%s must not include legacy readiness text %q", name, forbidden)
	}
}

func riskLabelsFromThreatModel(t *testing.T) []string {
	t.Helper()
	threatModel := readRepoFile(t, "docs/10_THREAT_MODEL.md")
	if !strings.Contains(threatModel, "## Risk labeling") {
		return nil
	}
	section := markdownSectionByHeading(t, "docs/10_THREAT_MODEL.md", threatModel, "## Risk labeling")
	var labels []string
	for _, line := range strings.Split(section, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "- ") {
			continue
		}
		match := backtickedFieldPattern.FindStringSubmatch(line)
		if match != nil {
			labels = append(labels, match[1])
		}
	}
	return labels
}

func releaseFacingClaimBody(t *testing.T, doc string) string {
	t.Helper()
	body := readRepoFile(t, doc)
	if doc == "docs/99_CHANGELOG.md" {
		return firstChangelogEntry(body)
	}
	return body
}

func scopedPerformanceClaim(line string) bool {
	lower := strings.ToLower(line)
	if strings.Contains(lower, "not ") || strings.Contains(lower, "forbid") {
		return true
	}
	return releaseFacingStorageScopePattern.MatchString(line) ||
		regexp.MustCompile(`(?i)\bnot\b.*(?:o\(1\)|instant(?:ly)?|constant-time|constant overhead)(?:[^A-Za-z0-9_]|$)`).MatchString(line) ||
		negatedReleaseFacingPerformanceClaimPattern.MatchString(line)
}

func scopedSavePointReferenceClaim(line string) bool {
	return releaseFacingStorageScopePattern.MatchString(line)
}

func engineConstantsFromModel(t *testing.T) []string {
	t.Helper()
	fset := token.NewFileSet()
	path := repoFile(t, "pkg/model/types.go")
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse pkg/model/types.go: %v", err)
	}

	var constants []string
	ast.Inspect(file, func(node ast.Node) bool {
		valueSpec, ok := node.(*ast.ValueSpec)
		if !ok || len(valueSpec.Names) == 0 {
			return true
		}
		typeIdent, ok := valueSpec.Type.(*ast.Ident)
		if !ok || typeIdent.Name != "EngineType" {
			return true
		}
		for i, name := range valueSpec.Names {
			if i >= len(valueSpec.Values) {
				continue
			}
			lit, ok := valueSpec.Values[i].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				continue
			}
			value, err := strconv.Unquote(lit.Value)
			if err != nil {
				pos := fset.Position(lit.Pos())
				t.Fatalf("%s:%d unquote engine constant %s: %v", path, pos.Line, name.Name, err)
			}
			constants = append(constants, name.Name+"="+value)
		}
		return true
	})
	if len(constants) == 0 {
		t.Fatalf("pkg/model/types.go defines no EngineType constants")
	}
	return constants
}

func documentedEngineConstants(section string) []string {
	var constants []string
	for _, match := range documentedEngineConstantPattern.FindAllStringSubmatch(section, -1) {
		constants = append(constants, match[1]+"="+match[2])
	}
	return constants
}

func cleanupProtectionReasonConstantsFromFacade(t *testing.T) []string {
	t.Helper()
	fset := token.NewFileSet()
	path := repoFile(t, "pkg/jvs/client.go")
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse pkg/jvs/client.go: %v", err)
	}

	var constants []string
	ast.Inspect(file, func(node ast.Node) bool {
		valueSpec, ok := node.(*ast.ValueSpec)
		if !ok || len(valueSpec.Names) == 0 {
			return true
		}
		typeIdent, ok := valueSpec.Type.(*ast.Ident)
		if !ok || typeIdent.Name != "CleanupProtectionReason" {
			return true
		}
		for i, name := range valueSpec.Names {
			if i >= len(valueSpec.Values) {
				continue
			}
			lit, ok := valueSpec.Values[i].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				continue
			}
			value, err := strconv.Unquote(lit.Value)
			if err != nil {
				pos := fset.Position(lit.Pos())
				t.Fatalf("%s:%d unquote cleanup protection reason constant %s: %v", path, pos.Line, name.Name, err)
			}
			constants = append(constants, name.Name+"="+value)
		}
		return true
	})
	if len(constants) == 0 {
		t.Fatalf("pkg/jvs/client.go defines no CleanupProtectionReason constants")
	}
	return constants
}

func documentedCleanupProtectionReasonConstants(section string) []string {
	var constants []string
	for _, match := range documentedCleanupProtectionReasonPattern.FindAllStringSubmatch(section, -1) {
		constants = append(constants, match[1]+"="+match[2])
	}
	return constants
}

func cleanupProtectionReasonTokensFromConstants(constants []string) []string {
	var tokens []string
	for _, constant := range constants {
		_, token, ok := strings.Cut(constant, "=")
		if ok {
			tokens = append(tokens, token)
		}
	}
	return tokens
}

func stableCleanupProtectionReasonTokens() []string {
	return []string{
		"history",
		"open_view",
		"active_recovery",
		"active_operation",
		"imported_clone_history",
	}
}

func stableCleanupProtectionReasonSet() map[string]bool {
	reasons := make(map[string]bool)
	for _, reason := range stableCleanupProtectionReasonTokens() {
		reasons[reason] = true
	}
	return reasons
}

func publicRuntimeRepairActionIDs() []string {
	return []string{
		"clean_locks",
		"rebind_workspace_paths",
		"clean_runtime_tmp",
		"clean_runtime_operations",
		"clean_runtime_cleanup_plans",
	}
}

func releaseFacingRuntimeRepairActionDocs() []string {
	return []string{
		"docs/02_CLI_SPEC.md",
		"docs/09_SECURITY_MODEL.md",
		"docs/11_CONFORMANCE_TEST_PLAN.md",
		"docs/13_OPERATION_RUNBOOK.md",
		"docs/ARCHITECTURE.md",
		"docs/PRODUCT_PLAN.md",
	}
}

func runtimeRepairActionIDsMentioned(body string) []string {
	var ids []string
	for _, match := range runtimeRepairActionIDPattern.FindAllStringSubmatch(body, -1) {
		ids = appendUniqueString(ids, match[1])
	}
	return ids
}

func unsupportedDoctorRepairActionIDs() []string {
	return []string{
		"clean_tmp",
		"advance_head",
		"rebuild_index",
		"audit_repair",
		"clean_intents",
	}
}

func unsupportedTraceabilityNormativeFragments() []string {
	return []string{
		"jvs worktree",
		"retention is controlled by gc policy",
		"unless pinned",
	}
}

func lineContainsRepairActionID(line, id string) bool {
	pattern := regexp.MustCompile(`(^|[^A-Za-z0-9_])` + regexp.QuoteMeta(id) + `([^A-Za-z0-9_]|$)`)
	return pattern.MatchString(line)
}

func stablePublicDocs() []string {
	return []string{
		"README.md",
		"SECURITY.md",
		"docs/README.md",
		"docs/user/README.md",
		"docs/user/quickstart.md",
		"docs/user/best-practices.md",
		"docs/user/concepts.md",
		"docs/user/commands.md",
		"docs/user/examples.md",
		"docs/user/tutorials.md",
		"docs/user/faq.md",
		"docs/user/troubleshooting.md",
		"docs/user/safety.md",
		"docs/user/recovery.md",
		"docs/00_OVERVIEW.md",
		"docs/01_REPO_LAYOUT_SPEC.md",
		"docs/QUICKSTART.md",
		"docs/02_CLI_SPEC.md",
		"docs/06_RESTORE_SPEC.md",
		"docs/08_GC_SPEC.md",
		"docs/11_CONFORMANCE_TEST_PLAN.md",
		"docs/12_RELEASE_POLICY.md",
		"docs/13_OPERATION_RUNBOOK.md",
		"docs/14_TRACEABILITY_MATRIX.md",
		"docs/18_MIGRATION_AND_BACKUP.md",
		"docs/RELEASE_EVIDENCE.md",
		"docs/API_DOCUMENTATION.md",
		"docs/EXAMPLES.md",
		"docs/FAQ.md",
		"docs/09_SECURITY_MODEL.md",
		"docs/10_THREAT_MODEL.md",
		"docs/PERFORMANCE.md",
		"docs/PERFORMANCE_RESULTS.md",
		"docs/BENCHMARKS.md",
		"docs/TROUBLESHOOTING.md",
		"docs/99_CHANGELOG.md",
		"docs/20_USER_SCENARIOS.md",
	}
}

func domainQuickstartDocs() []string {
	return []string{
		"docs/agent_sandbox_quickstart.md",
		"docs/etl_pipeline_quickstart.md",
		"docs/game_dev_quickstart.md",
	}
}

func runtimeStateBoundaryTerms() []string {
	return []string{
		"jvs doctor --strict --repair-runtime",
	}
}

func releaseFacingRuntimeStateBoundaryDocs() []string {
	return []string{
		"SECURITY.md",
		"docs/01_REPO_LAYOUT_SPEC.md",
		"docs/10_THREAT_MODEL.md",
		"docs/13_OPERATION_RUNBOOK.md",
		"docs/18_MIGRATION_AND_BACKUP.md",
		"docs/RELEASE_EVIDENCE.md",
		"docs/99_CHANGELOG.md",
	}
}

func releaseFacingMigrationRuntimeFlowDocs() []string {
	return []string{
		"docs/08_GC_SPEC.md",
		"docs/11_CONFORMANCE_TEST_PLAN.md",
		"docs/13_OPERATION_RUNBOOK.md",
		"docs/14_TRACEABILITY_MATRIX.md",
		"docs/18_MIGRATION_AND_BACKUP.md",
		"docs/PRODUCT_PLAN.md",
	}
}

func forbiddenCleanupProtectionVocabulary() []string {
	return []string{
		"active source protection",
		"active source reads",
		"source reads",
		"view/source reads",
		"views/source reads",
		"active sources",
		"live and active sources",
		"live workspace needs",
		"live workspaces",
	}
}

func stableCleanupProtectionNaturalTerms() []string {
	return []string{
		"workspace history",
		"open views",
		"active recovery plans",
		"active operations",
		"imported clone history",
	}
}

func cleanupProtectionBoundaryClaim(block string) bool {
	if strings.HasSuffix(strings.TrimSpace(block), ":") {
		return false
	}
	lower := strings.ToLower(block)
	return (strings.Contains(lower, "cleanup") && strings.Contains(lower, "must protect")) ||
		strings.Contains(lower, "cleanup protects")
}

func cleanupProtectionVocabularyContext(line string) bool {
	lower := strings.ToLower(line)
	return strings.Contains(lower, "cleanup") ||
		strings.Contains(lower, "protect") ||
		strings.Contains(lower, "protection")
}

func runtimeInternalFileVocabularyFragments() []string {
	return []string{
		"runtime lock files",
		"cleanup runtime plan files",
		"internal cleanup runtime plans",
		"operation lock files",
		"repository mutation locks",
		"operation intents",
	}
}

func releaseBlockingContractDocs() []string {
	docs := append([]string{}, stablePublicDocs()...)
	docs = append(docs,
		"CONTRIBUTING.md",
		"docs/PRODUCT_PLAN.md",
		"docs/ARCHITECTURE.md",
		"docs/03_WORKTREE_SPEC.md",
		"docs/04_SNAPSHOT_SCOPE_AND_LINEAGE_SPEC.md",
		"docs/05_SNAPSHOT_ENGINE_SPEC.md",
	)
	for _, doc := range activeReleaseFacingContractDocs() {
		docs = appendUniqueString(docs, doc)
	}
	return docs
}

func activeReleaseFacingContractDocs() []string {
	return []string{
		"docs/PRODUCT_PLAN.md",
		"docs/ARCHITECTURE.md",
		"docs/CONSTITUTION.md",
		"docs/SIGNING.md",
	}
}

func archivedNonReleaseFacingDocs() []string {
	return []string{
		"docs/archive/README.md",
		"docs/JVS_SYNC.md",
		"docs/SOURCES.md",
		"docs/TEMPLATES.md",
		"docs/plans/2026-02-20-jvs-go-implementation-design.md",
		"docs/plans/2026-02-20-jvs-implementation-plan.md",
	}
}

func activeNonReleaseFacingDesignDocs() []string {
	return []string{
		"docs/21_SAVE_POINT_WORKSPACE_SEMANTICS.md",
		"docs/22_WORKSPACE_EXPLICIT_PATH_BEHAVIOR.md",
		"docs/23_FILESYSTEM_AWARE_TRANSFER_PLANNING.md",
		"docs/24_REPO_CLONE_PRODUCT_PLAN.md",
		"docs/25_REPO_WORKSPACE_LIFECYCLE_PRODUCT_PLAN.md",
	}
}

func activeNonReleaseFacingReferenceDocs() []string {
	return []string{
		"docs/design/README.md",
		"docs/ops/README.md",
		"docs/release/README.md",
	}
}

func activeNonReleaseFacingResearchDocs() []string {
	return []string{
		"docs/PRODUCT_GAPS_FOR_NEXT_PLAN.md",
		"docs/TARGET_USERS.md",
	}
}

func activeNonReleaseFacingExampleDocs() []string {
	return domainQuickstartDocs()
}

func nonReleaseFacingDocs() []string {
	docs := append([]string{}, archivedNonReleaseFacingDocs()...)
	for _, doc := range activeNonReleaseFacingDesignDocs() {
		docs = appendUniqueString(docs, doc)
	}
	for _, doc := range activeNonReleaseFacingReferenceDocs() {
		docs = appendUniqueString(docs, doc)
	}
	for _, doc := range activeNonReleaseFacingResearchDocs() {
		docs = appendUniqueString(docs, doc)
	}
	for _, doc := range activeNonReleaseFacingExampleDocs() {
		docs = appendUniqueString(docs, doc)
	}
	return docs
}

func activePublicContractDocs() []string {
	var docs []string
	for _, doc := range releaseBlockingContractDocs() {
		if staticStringSliceContains(nonReleaseFacingDocs(), doc) {
			continue
		}
		docs = appendUniqueString(docs, doc)
	}
	return docs
}

func publicCommandDocs() []string {
	return releaseBlockingContractDocs()
}

func historyTagPublicSurfaceDocs() []string {
	return []string{
		"docs/02_CLI_SPEC.md",
		"docs/03_WORKTREE_SPEC.md",
		"docs/08_GC_SPEC.md",
		"docs/20_USER_SCENARIOS.md",
		"docs/PRODUCT_PLAN.md",
		"docs/user/commands.md",
		"docs/user/concepts.md",
		"docs/user/safety.md",
		"docs/user/faq.md",
		"docs/user/quickstart.md",
		"docs/user/examples.md",
		"docs/user/troubleshooting.md",
	}
}

func cleanupPublicSurfaceDocs() []string {
	return []string{
		"docs/02_CLI_SPEC.md",
		"docs/08_GC_SPEC.md",
		"docs/20_USER_SCENARIOS.md",
		"docs/PRODUCT_PLAN.md",
		"docs/user/commands.md",
	}
}

func cleanupLineNegatesPromise(lowerLine string) bool {
	for _, marker := range []string{
		"does not",
		"do not",
		"must not",
		"is not",
		"not ",
		"separate",
	} {
		if strings.Contains(lowerLine, marker) {
			return true
		}
	}
	return false
}

func fuzzCommandDocs() []string {
	return []string{
		"CONTRIBUTING.md",
		"test/fuzz/FUZZING.md",
	}
}

func unsupportedPublicCLIExampleRules() []unsupportedPublicCLIExampleRule {
	return []unsupportedPublicCLIExampleRule{
		{
			name:        "`jvs checkpoint --quiet`",
			pattern:     unsupportedPublicCommandFlagPattern("checkpoint", "--quiet"),
			replacement: "use `jvs --no-progress save -m <message>` for scripts",
		},
		{
			name:        "`jvs verify --recompute`",
			pattern:     unsupportedPublicCommandFlagPattern("verify", "--recompute"),
			replacement: "use `jvs doctor --strict` for public integrity checks",
		},
		{
			name:        "`jvs verify --no-payload`",
			pattern:     unsupportedPublicCommandFlagPattern("verify", "--no-payload"),
			replacement: "document `jvs doctor --strict`; payload verification flags are not public v0 surface",
		},
		{
			name:        "`jvs --version`",
			pattern:     regexp.MustCompile(`(?i)(^|[^A-Za-z0-9_-])jvs\s+--version([^A-Za-z0-9_-]|$)`),
			replacement: "use `jvs --help` for install smoke checks or `jvs status` inside a repository",
		},
	}
}

func unsupportedPublicCommandFlagPattern(command, flag string) *regexp.Regexp {
	return regexp.MustCompile(`(?i)(^|[^A-Za-z0-9_-])jvs\s+` +
		regexp.QuoteMeta(command) + `\b[^` + "`" + `|#\n]*` +
		regexp.QuoteMeta(flag) + `([^A-Za-z0-9_-]|$)`)
}

func unsupportedPublicDocCommandFragments() []string {
	return []string{
		"jvs checkpoint",
		"jvs fork",
		"jvs gc",
		"jvs verify",
		"jvs capability",
		"jvs config",
		"jvs conformance",
		"jvs import",
		"jvs clone",
		"jvs info",
		"jvs diff",
		"jvs snapshot",
		"jvs worktree",
		"jvs restore HEAD",
		"restore HEAD",
		"jvs restore --latest-tag",
		"restore --latest-tag",
		"jvs history --tag",
		"history --tag",
		"workspace fork",
		"worktree fork",
		"jvs checkpoint list --all",
		"jvs verify --no-payload",
		"jvs verify --since",
		"jvs checkpoint list --tag",
		"jvs checkpoint list --grep",
		"jvs inspect",
		"jvs ref ",
		"jvs lock ",
		"jvs remote ",
		"jvs merge ",
		"jvs rebase ",
		"jvs push",
		"jvs pull",
	}
}

func legacyPublicContractFragments() []string {
	return []string{
		"jvs checkpoint",
		"jvs fork",
		"jvs gc",
		"jvs verify",
		"jvs capability",
		"jvs config",
		"jvs conformance",
		"jvs worktree",
		"jvs snapshot",
		"current differs from latest",
		"current/latest",
		"current checkpoint",
		"latest checkpoint",
		"`current`",
		"`latest`",
		"`dirty`",
		"dirty state",
		"dirty changes",
		"dirty workspace",
		"worktree fork",
	}
}

func allMarkdownLegacyPublicDesignFragments() []string {
	fragments := []string{
		"jvs checkpoint",
		"jvs fork",
		"jvs gc",
		"jvs verify",
		"jvs capability",
		"jvs config",
		"jvs conformance",
		"jvs import",
		"jvs clone",
		"jvs info",
		"jvs diff",
		"jvs snapshot",
		"jvs worktree",
		"restore HEAD",
		"restore --latest-tag",
		"history --tag",
		"current/latest",
		"current differs from latest",
		"current checkpoint",
		"latest checkpoint",
		"worktree directories",
		"worktree directory",
		"worktrees/*",
		"snapshotted",
	}
	return fragments
}

func allowedAllMarkdownLegacyDesignLine(line string) bool {
	lower := strings.ToLower(line)
	for _, marker := range []string{
		"internal/",
		"pkg/",
		".jvs/snapshots",
		".jvs/worktrees",
		"repo/worktrees",
		"snapshot_id",
		"worktree_name",
		"head_snapshot_id",
		"latest_snapshot_id",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	for _, marker := range []string{
		"do not",
		"not part of",
		"not public",
		"not a public",
		"must not",
		"forbid",
		"forbidden",
		"unsupported",
		"old draft",
		"historical name",
		"historical implementation",
		"replacement",
		"replace",
		"replaced",
		"不要",
		"不得",
		"不能",
		"不是",
		"旧草案",
		"历史实现",
		"需要替换",
		"替换",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func lineContainsUnsupportedPublicDocCommandFragment(line, fragment string) bool {
	pattern := regexp.MustCompile(`(?i)(^|[^A-Za-z0-9_-])` + regexp.QuoteMeta(fragment) + `([^A-Za-z0-9_-]|$)`)
	return pattern.MatchString(line)
}

func publicDocCommandFields(line string) []string {
	trimmed := strings.TrimSpace(line)
	trimmed = strings.TrimPrefix(trimmed, "$ ")
	trimmed = strings.TrimPrefix(trimmed, "> ")
	if strings.HasPrefix(trimmed, "`jvs ") {
		if end := strings.Index(trimmed[1:], "`"); end >= 0 {
			trimmed = trimmed[1 : end+1]
		}
	}
	trimmed = strings.Trim(trimmed, "`|")
	trimmed = strings.TrimSpace(trimmed)
	if strings.HasPrefix(trimmed, "#") || !strings.HasPrefix(trimmed, "jvs ") {
		return nil
	}
	if idx := strings.Index(trimmed, "#"); idx >= 0 {
		trimmed = strings.TrimSpace(trimmed[:idx])
	}
	if idx := strings.Index(trimmed, "|"); idx >= 0 {
		trimmed = strings.TrimSpace(trimmed[:idx])
	}
	if idx := strings.Index(trimmed, "&&"); idx >= 0 {
		trimmed = strings.TrimSpace(trimmed[:idx])
	}
	return strings.Fields(trimmed)
}

func publicDocCommandFieldSets(line string) [][]string {
	var out [][]string
	seen := make(map[string]bool)
	add := func(candidate string) {
		fields := publicDocCommandFields(candidate)
		if len(fields) == 0 {
			return
		}
		key := strings.Join(fields, "\x00")
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, fields)
	}

	add(line)
	for _, span := range markdownInlineCodeSpans(line) {
		add(span)
		for _, part := range strings.Split(span, "&&") {
			add(part)
		}
	}
	return out
}

func markdownInlineCodeSpans(line string) []string {
	var spans []string
	remaining := line
	for {
		start := strings.Index(remaining, "`")
		if start < 0 {
			return spans
		}
		remaining = remaining[start+1:]
		end := strings.Index(remaining, "`")
		if end < 0 {
			return spans
		}
		spans = append(spans, remaining[:end])
		remaining = remaining[end+1:]
	}
}

func savePointUserGuideDocs() []string {
	return []string{
		"docs/user/README.md",
		"docs/user/quickstart.md",
		"docs/user/best-practices.md",
		"docs/user/concepts.md",
		"docs/user/commands.md",
		"docs/user/examples.md",
		"docs/user/faq.md",
		"docs/user/troubleshooting.md",
		"docs/user/safety.md",
		"docs/user/recovery.md",
		"docs/20_USER_SCENARIOS.md",
		"SECURITY.md",
	}
}

func publicRootHelpCommandNames(t *testing.T) map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, repoFile(t, "internal/cli/root.go"), nil, 0)
	if err != nil {
		t.Fatalf("parse internal/cli/root.go: %v", err)
	}

	commands := make(map[string]bool)
	ast.Inspect(file, func(node ast.Node) bool {
		valueSpec, ok := node.(*ast.ValueSpec)
		if !ok {
			return true
		}
		for i, name := range valueSpec.Names {
			if name.Name != "publicRootCommandNames" || i >= len(valueSpec.Values) {
				continue
			}
			lit, ok := valueSpec.Values[i].(*ast.CompositeLit)
			if !ok {
				t.Fatalf("publicRootCommandNames must remain a map literal")
			}
			for _, elt := range lit.Elts {
				kv, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				key, ok := kv.Key.(*ast.BasicLit)
				if !ok || key.Kind != token.STRING {
					continue
				}
				value, err := strconv.Unquote(key.Value)
				if err != nil {
					pos := fset.Position(key.Pos())
					t.Fatalf("%s:%d unquote public root command name: %v", repoFile(t, "internal/cli/root.go"), pos.Line, err)
				}
				commands[value] = true
			}
			return false
		}
		return true
	})
	if len(commands) == 0 {
		t.Fatalf("internal/cli/root.go must define publicRootCommandNames")
	}
	return commands
}

func stringBoolMapKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

func publicDocCommandPath(fields []string) []string {
	if len(fields) == 0 || fields[0] != "jvs" {
		return nil
	}
	var path []string
	for i := 1; i < len(fields); i++ {
		candidate := strings.Trim(fields[i], "`.,;:)")
		candidate = strings.Trim(candidate, "[]")
		if publicDocCommandFlagConsumesValue(candidate) {
			i++
			continue
		}
		if strings.HasPrefix(candidate, "-") ||
			(strings.HasPrefix(candidate, "<") && strings.HasSuffix(candidate, ">")) ||
			candidate == "..." {
			continue
		}
		path = append(path, candidate)
		if len(path) == 2 {
			return path
		}
	}
	return path
}

func publicDocCommandFlagConsumesValue(flag string) bool {
	if strings.Contains(flag, "=") {
		return false
	}
	switch flag {
	case "-m", "--message", "--workspace", "--repo", "--from", "--path":
		return true
	default:
		return false
	}
}

func stablePublicCommandPath(commandPath []string, publicRootCommands map[string]bool) bool {
	if len(commandPath) == 0 || !publicRootCommands[commandPath[0]] {
		return false
	}
	if len(commandPath) == 1 {
		return true
	}
	switch commandPath[0] {
	case "workspace", "view", "recovery", "cleanup", "repo":
	default:
		return true
	}
	if commandPath[0] == "view" && commandPath[1] != "close" && !publicDocKnownUnsupportedSubcommand(commandPath[0], commandPath[1]) {
		return true
	}
	switch strings.Join(commandPath[:2], " ") {
	case "workspace new",
		"workspace list",
		"workspace path",
		"workspace rename",
		"workspace move",
		"workspace delete",
		"cleanup preview",
		"cleanup run",
		"repo clone",
		"repo move",
		"repo rename",
		"repo detach",
		"view close",
		"recovery status",
		"recovery resume",
		"recovery rollback":
		return true
	default:
		return false
	}
}

func publicDocKnownUnsupportedSubcommand(root, value string) bool {
	switch root {
	case "view":
		switch value {
		case "delete", "remove", "list", "new", "fork", "gc", "verify", "capability", "checkpoint":
			return true
		default:
			return false
		}
	default:
		switch value {
		case "delete", "remove", "list", "path", "new", "fork", "gc", "verify", "capability", "checkpoint":
			return true
		default:
			return false
		}
	}
}

func publicDocForbiddenTerms() []string {
	terms := []string{
		"jvs checkpoint",
		"jvs fork",
		"jvs gc",
		"jvs verify",
		"jvs capability",
		"jvs snapshot",
		"jvs worktree",
		"restore HEAD",
		"snapshots",
		"worktrees",
		"detached state",
		"worktree fork",
		"snapshot_id",
		"worktree_name",
		"source_worktree",
		"head_snapshot",
		"latest_snapshot",
		"from_snapshot",
		"to_snapshot",
		"--paths",
		"--compress",
		"--format json",
		"--engine",
		"--keep-daily",
		"--keep-tagged",
		"--allow-protected",
		"jvs checkpoint list --tag",
		"jvs checkpoint list --grep",
		"jvs inspect",
	}
	terms = append(terms, legacyPublicContractFragments()...)
	return terms
}

func markdownHeadingLevel(line string) (int, bool) {
	trimmed := strings.TrimLeft(line, " ")
	if !strings.HasPrefix(trimmed, "#") {
		return 0, false
	}
	level := 0
	for _, r := range trimmed {
		if r != '#' {
			break
		}
		level++
	}
	if level == 0 || level > 6 || len(trimmed) <= level || trimmed[level] != ' ' {
		return 0, false
	}
	return level, true
}

func allowedPublicDocCompatibilityLine(doc, line string) bool {
	lower := strings.ToLower(line)
	for _, marker := range []string{
		".jvs/snapshots",
		".jvs/worktrees",
		"repo/worktrees",
		"snapshot_id",
		"worktree_name",
		"head_snapshot_id",
		"latest_snapshot_id",
		"snapshots/         # internal",
		"worktrees/         # internal",
		"gc/                #",
		"`worktrees/` directory",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	if strings.Contains(lower, "worktrees/") && strings.Contains(lower, "internal") {
		return true
	}
	if doc == "docs/BENCHMARKS.md" {
		for _, marker := range []string{"`benchmark", "internal/snapshot", "internal/restore", "internal/gc", "benchmarkworktree"} {
			if strings.Contains(lower, marker) {
				return true
			}
		}
	}
	// This is the JuiceFS mount flag, not the old JVS checkpoint compression flag.
	if strings.Contains(line, "juicefs mount") && strings.Contains(line, "--compress") {
		return true
	}
	if strings.Contains(line, "HEAD") {
		for _, marker := range []string{"Git", "GitHub", "CI", "github.sha"} {
			if strings.Contains(line, marker) {
				return true
			}
		}
	}
	return false
}

func traceabilityNormativeDocs(t *testing.T) []string {
	t.Helper()
	matrix := readRepoFile(t, "docs/14_TRACEABILITY_MATRIX.md")
	var docs []string
	inNormativeBlock := false
	for _, line := range strings.Split(matrix, "\n") {
		trimmed := strings.TrimSpace(line)
		if label, ok := traceabilityBlockLabel(trimmed); ok {
			inNormativeBlock = strings.HasPrefix(label, "normative")
			continue
		}
		if !inNormativeBlock {
			continue
		}
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(trimmed, "- ") {
			inNormativeBlock = false
			continue
		}
		for _, match := range traceabilityDocRefPattern.FindAllStringSubmatch(line, -1) {
			docs = appendUniqueString(docs, match[1])
		}
	}
	return docs
}

func traceabilityBlockLabel(trimmedLine string) (string, bool) {
	if strings.HasPrefix(trimmedLine, "- ") {
		trimmedLine = strings.TrimSpace(strings.TrimPrefix(trimmedLine, "- "))
	}
	if !strings.HasSuffix(trimmedLine, ":") {
		return "", false
	}
	label := strings.TrimSpace(strings.TrimSuffix(trimmedLine, ":"))
	if label == "" || strings.Contains(label, "`") {
		return "", false
	}
	return strings.ToLower(label), true
}

type markdownDocLink struct {
	source string
	target string
}

func activePublicMarkdownDocLinks(t *testing.T) []markdownDocLink {
	t.Helper()
	var links []markdownDocLink
	seen := make(map[markdownDocLink]bool)
	for _, doc := range activePublicContractDocs() {
		body := readRepoFile(t, doc)
		for _, match := range markdownDocLinkPattern.FindAllStringSubmatch(body, -1) {
			target := markdownDocLinkTarget(t, doc, match[1])
			if target == "" {
				continue
			}
			link := markdownDocLink{
				source: doc,
				target: target,
			}
			if seen[link] {
				continue
			}
			seen[link] = true
			links = append(links, link)
		}
	}
	return links
}

func markdownDocLinkTarget(t *testing.T, sourceDoc, rawTarget string) string {
	t.Helper()
	if strings.Contains(rawTarget, "://") || filepath.IsAbs(rawTarget) {
		return ""
	}
	target := strings.TrimSpace(rawTarget)
	if target == "" {
		return ""
	}
	resolved := filepath.ToSlash(filepath.Clean(filepath.Join(filepath.Dir(sourceDoc), target)))
	if strings.HasPrefix(resolved, "../") {
		return ""
	}
	if _, err := os.Stat(repoFile(t, resolved)); err != nil {
		if os.IsNotExist(err) {
			t.Fatalf("active public doc %s links missing markdown doc %s resolved as %s", sourceDoc, rawTarget, resolved)
		}
		t.Fatalf("stat linked markdown doc %s from %s: %v", resolved, sourceDoc, err)
	}
	return resolved
}

func markdownFencedCodeBlocks(body string) []markdownCodeBlock {
	var blocks []markdownCodeBlock
	var block strings.Builder
	inBlock := false
	startLine := 0
	for i, line := range strings.Split(body, "\n") {
		lineNo := i + 1
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			if inBlock {
				blocks = append(blocks, markdownCodeBlock{
					startLine: startLine,
					text:      block.String(),
				})
				inBlock = false
				block.Reset()
			} else {
				inBlock = true
				startLine = lineNo + 1
			}
			continue
		}
		if inBlock {
			block.WriteString(line)
			block.WriteByte('\n')
		}
	}
	return blocks
}

func migrationWholeFolderCopyCodeBlocks(body string) []markdownCodeBlock {
	var blocks []markdownCodeBlock
	for _, block := range markdownFencedCodeBlocks(body) {
		if len(migrationCopyCommands(meaningfulShellLines(block.text))) > 0 {
			blocks = append(blocks, block)
		}
	}
	return blocks
}

func migrationCopyBlockFailsClosed(block string) bool {
	lines := meaningfulShellLines(block)
	steps := shellCommandSteps(lines)
	commands := migrationCopyCommandsFromSteps(steps)
	if len(commands) == 0 {
		return false
	}
	for _, command := range commands {
		if !migrationCopyCommandFailsClosed(steps, command) {
			return false
		}
	}
	return true
}

func migrationCopyCommandFailsClosed(steps []shellCommandStep, command shellCopyCommand) bool {
	if migrationCopyCommandUsesOverlayOperands(command) {
		return false
	}
	if shellBlockCreatesCopyDestinationBeforeCopy(steps, command.stepIndex, command.destination) {
		return false
	}

	if shellBlockHasExplicitDestinationExitBeforeCopy(steps, command.stepIndex, command.destination) {
		return true
	}

	testStep := shellDestinationNonexistenceCheckStep(steps, command.stepIndex, command.destination)
	if testStep < 0 {
		return false
	}
	if shellNonexistenceCheckHasOrExitBeforeCopy(steps, testStep, command.stepIndex) {
		return true
	}
	if shellBlockHasActiveErrexitAt(steps, testStep) && shellStepIsStandaloneErrexitGuard(steps, testStep) {
		return true
	}
	return shellBlockHasChainedAndBetween(steps, testStep, command.stepIndex)
}

func migrationCopyCommands(lines []string) []shellCopyCommand {
	return migrationCopyCommandsFromSteps(shellCommandSteps(lines))
}

func migrationCopyCommandsFromSteps(steps []shellCommandStep) []shellCopyCommand {
	var commands []shellCopyCommand
	for _, step := range steps {
		if command, ok := migrationCopyCommand(step.fields); ok {
			command.stepIndex = step.index
			command.lineIndex = step.lineIndex
			commands = append(commands, command)
		}
	}
	return commands
}

func migrationCopyCommand(fields []string) (shellCopyCommand, bool) {
	fields = shellCommandInvocationFields(fields)
	if len(fields) == 0 {
		return shellCopyCommand{}, false
	}
	switch fields[0] {
	case "cp":
		return migrationCpCommand(fields)
	case "rsync":
		return migrationRsyncCommand(fields)
	default:
		return shellCopyCommand{}, false
	}
}

func migrationCpCommand(fields []string) (shellCopyCommand, bool) {
	operands := shellCommandOperands(fields[1:])
	if len(operands) < 2 {
		return shellCopyCommand{}, false
	}
	if !copyFieldsIncludeWholeFolderCopyFlag(fields[1:]) {
		return shellCopyCommand{}, false
	}
	return shellCopyCommand{
		command:     fields[0],
		source:      operands[len(operands)-2],
		destination: operands[len(operands)-1],
	}, true
}

func migrationRsyncCommand(fields []string) (shellCopyCommand, bool) {
	operands := shellCommandOperands(fields[1:])
	if len(operands) < 2 {
		return shellCopyCommand{}, false
	}
	if !copyFieldsIncludeWholeFolderCopyFlag(fields[1:]) {
		return shellCopyCommand{}, false
	}
	return shellCopyCommand{
		command:     fields[0],
		source:      operands[len(operands)-2],
		destination: operands[len(operands)-1],
	}, true
}

func shellCommandInvocationFields(fields []string) []string {
	for len(fields) > 0 {
		switch fields[0] {
		case "command", "builtin":
			fields = fields[1:]
		case "env":
			fields = fields[1:]
			for len(fields) > 0 && (strings.Contains(fields[0], "=") || strings.HasPrefix(fields[0], "-")) {
				fields = fields[1:]
			}
		case "sudo":
			fields = trimSudoCommandPrefix(fields)
		default:
			return fields
		}
	}
	return fields
}

func trimSudoCommandPrefix(fields []string) []string {
	fields = fields[1:]
	for len(fields) > 0 {
		field := fields[0]
		if field == "--" {
			return fields[1:]
		}
		if !strings.HasPrefix(field, "-") {
			return fields
		}
		fields = fields[1:]
		if sudoOptionTakesValue(field) && len(fields) > 0 {
			fields = fields[1:]
		}
	}
	return fields
}

func sudoOptionTakesValue(field string) bool {
	return field == "-u" || field == "--user" ||
		field == "-g" || field == "--group" ||
		field == "-h" || field == "--host" ||
		field == "-p" || field == "--prompt"
}

func copyFieldsIncludeWholeFolderCopyFlag(fields []string) bool {
	hasWholeFolderCopyFlag := false
	for _, field := range fields {
		if field == "--" {
			break
		}
		if field == "--archive" || field == "--recursive" {
			hasWholeFolderCopyFlag = true
			continue
		}
		if strings.HasPrefix(field, "--") {
			continue
		}
		if strings.HasPrefix(field, "-") &&
			(strings.Contains(field, "a") || strings.Contains(field, "R") || strings.Contains(field, "r")) {
			hasWholeFolderCopyFlag = true
		}
	}
	return hasWholeFolderCopyFlag
}

func shellCommandOperands(fields []string) []string {
	var operands []string
	for _, field := range fields {
		cleaned := cleanShellField(field)
		if cleaned == "" || cleaned == "&&" || cleaned == ";" {
			continue
		}
		if cleaned == "--" {
			continue
		}
		if strings.HasPrefix(cleaned, "-") {
			continue
		}
		operands = append(operands, cleaned)
	}
	return operands
}

func migrationCopyCommandUsesOverlayOperands(command shellCopyCommand) bool {
	source := normalizeShellPathToken(command.source)
	destination := normalizeShellPathToken(command.destination)
	return copyCommandSourceCopiesDirectoryContents(command.command, source) || strings.HasSuffix(destination, "/")
}

func copyCommandSourceCopiesDirectoryContents(command, source string) bool {
	switch command {
	case "rsync":
		return strings.HasSuffix(source, "/") ||
			strings.HasSuffix(strings.TrimRight(source, "/"), "/.")
	case "cp":
		return strings.HasSuffix(strings.TrimRight(source, "/"), "/.")
	default:
		return false
	}
}

func shellBlockCreatesCopyDestinationBeforeCopy(steps []shellCommandStep, copyStep int, destination string) bool {
	for i := 0; i < copyStep; i++ {
		if shellStepCreatesDirectory(steps[i], destination) {
			return true
		}
	}
	return false
}

func shellStepCreatesDirectory(step shellCommandStep, destination string) bool {
	fields := shellCommandInvocationFields(shellTrimReservedCommandPrefixes(step.fields))
	if len(fields) == 0 {
		return false
	}
	var operands []string
	switch fields[0] {
	case "mkdir":
		operands = shellCommandOperands(fields[1:])
	case "install":
		operands = shellInstallDirectoryOperands(fields[1:])
	default:
		return false
	}
	for _, operand := range operands {
		if shellPathTokenCreatesDestinationOrDescendant(operand, destination) {
			return true
		}
	}
	return false
}

func shellInstallDirectoryOperands(fields []string) []string {
	parsed := shellParseInstallFields(fields)
	if !parsed.explicitDirectories && !parsed.leadingDirectories {
		return nil
	}
	if parsed.explicitDirectories {
		operands := append([]string{}, parsed.operands...)
		return append(operands, parsed.targetDirectories...)
	}
	if parsed.leadingDirectories {
		if len(parsed.targetDirectories) > 0 {
			return append([]string{}, parsed.targetDirectories...)
		}
		if len(parsed.operands) == 0 {
			return nil
		}
		return []string{parsed.operands[len(parsed.operands)-1]}
	}
	return nil
}

func shellInstallOperands(fields []string) []string {
	return shellParseInstallFields(fields).operands
}

type shellInstallFields struct {
	operands            []string
	targetDirectories   []string
	explicitDirectories bool
	leadingDirectories  bool
}

func shellParseInstallFields(fields []string) shellInstallFields {
	var parsed shellInstallFields
	for i := 0; i < len(fields); i++ {
		field := cleanShellField(fields[i])
		if field == "" {
			continue
		}
		if field == "--" {
			for _, operand := range fields[i+1:] {
				cleaned := cleanShellField(operand)
				if cleaned != "" {
					parsed.operands = append(parsed.operands, cleaned)
				}
			}
			break
		}
		if strings.HasPrefix(field, "--") {
			shellParseInstallLongOption(&parsed, field, fields, &i)
			continue
		}
		if strings.HasPrefix(field, "-") && field != "-" {
			shellParseInstallShortOptions(&parsed, field, fields, &i)
			continue
		}
		parsed.operands = append(parsed.operands, field)
	}
	return parsed
}

func shellParseInstallLongOption(parsed *shellInstallFields, field string, fields []string, index *int) {
	switch {
	case field == "--directory":
		parsed.explicitDirectories = true
	case field == "--target-directory":
		if *index+1 < len(fields) {
			shellAddInstallTargetDirectory(parsed, fields[*index+1])
			*index = *index + 1
		}
	case strings.HasPrefix(field, "--target-directory="):
		shellAddInstallTargetDirectory(parsed, strings.TrimPrefix(field, "--target-directory="))
	case shellInstallLongOptionTakesValue(field):
		if !strings.Contains(field, "=") && *index+1 < len(fields) {
			*index = *index + 1
		}
	}
}

func shellParseInstallShortOptions(parsed *shellInstallFields, field string, fields []string, index *int) {
	cluster := strings.TrimPrefix(field, "-")
	for pos := 0; pos < len(cluster); pos++ {
		switch cluster[pos] {
		case 'd':
			parsed.explicitDirectories = true
		case 'D':
			parsed.leadingDirectories = true
		case 't':
			if pos+1 < len(cluster) {
				shellAddInstallTargetDirectory(parsed, cluster[pos+1:])
			} else if *index+1 < len(fields) {
				shellAddInstallTargetDirectory(parsed, fields[*index+1])
				*index = *index + 1
			}
			return
		case 'g', 'm', 'o':
			if pos+1 >= len(cluster) && *index+1 < len(fields) {
				*index = *index + 1
			}
			return
		}
	}
}

func shellAddInstallTargetDirectory(parsed *shellInstallFields, value string) {
	if cleaned := cleanShellField(value); cleaned != "" {
		parsed.targetDirectories = append(parsed.targetDirectories, cleaned)
	}
}

func shellInstallLongOptionTakesValue(field string) bool {
	if option, _, ok := strings.Cut(field, "="); ok {
		field = option
	}
	switch field {
	case "--group", "--mode", "--owner":
		return true
	default:
		return false
	}
}

func meaningfulShellLines(block string) []string {
	var lines []string
	for _, line := range strings.Split(block, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		lines = append(lines, trimmed)
	}
	return lines
}

func shellCommandSteps(lines []string) []shellCommandStep {
	var steps []shellCommandStep
	for lineIndex, line := range lines {
		var fields []string
		for _, token := range shellLineTokens(line) {
			if shellControlOperator(token) {
				if len(fields) > 0 {
					steps = append(steps, shellCommandStep{
						index:     len(steps),
						lineIndex: lineIndex,
						fields:    fields,
						opAfter:   token,
					})
					fields = nil
					continue
				}
				if len(steps) > 0 {
					steps[len(steps)-1].opAfter = token
				}
				continue
			}
			fields = append(fields, token)
		}
		if len(fields) > 0 {
			steps = append(steps, shellCommandStep{
				index:     len(steps),
				lineIndex: lineIndex,
				fields:    fields,
			})
		}
	}
	return steps
}

func shellBlockHasActiveErrexitAt(steps []shellCommandStep, limit int) bool {
	active := false
	for i := 0; i <= limit && i < len(steps); i++ {
		state, ok := shellStepErrexitStateChange(steps[i])
		if !ok {
			continue
		}
		if state {
			if shellStepCanProveParentShellStateChange(steps, i) {
				active = true
			}
			continue
		}
		if shellStepMayAffectParentShellState(steps, i) {
			active = state
		}
	}
	return active
}

func shellStepErrexitStateChange(step shellCommandStep) (bool, bool) {
	fields := shellBuiltinInvocationFields(step.fields)
	if len(fields) == 0 || fields[0] != "set" {
		return false, false
	}
	active := false
	changed := false
	for i := 1; i < len(fields); i++ {
		field := strings.Trim(fields[i], `"'`)
		if field == "--" {
			break
		}
		if field == "-o" || field == "+o" {
			if i+1 < len(fields) && strings.Trim(fields[i+1], `"'`) == "errexit" {
				active = field == "-o"
				changed = true
			}
			i++
			continue
		}
		if state, ok := shellSetFlagErrexitState(field); ok {
			active = state
			changed = true
		}
	}
	return active, changed
}

func shellSetFlagErrexitState(field string) (bool, bool) {
	if len(field) < 2 || strings.HasPrefix(field, "--") {
		return false, false
	}
	prefix := field[0]
	if prefix != '-' && prefix != '+' {
		return false, false
	}
	for _, flag := range field[1:] {
		if flag == 'e' {
			return prefix == '-', true
		}
	}
	return false, false
}

func shellBlockHasExplicitDestinationExitBeforeCopy(steps []shellCommandStep, copyStep int, destination string) bool {
	for i := 0; i < copyStep; i++ {
		if !shellStepCanStartExplicitDestinationGuard(steps, i, copyStep) {
			continue
		}
		if !shellFieldsHaveDestinationExistsCheck(steps[i].fields, destination) {
			continue
		}
		if shellDestinationExistsCheckHasAndExitBeforeCopy(steps, i, copyStep) {
			return true
		}
		if shellDestinationExistsIfTrueBranchExitsBeforeCopy(steps, i, copyStep) {
			return true
		}
	}
	return false
}

func shellDestinationExistsCheckHasAndExitBeforeCopy(steps []shellCommandStep, start, copyStep int) bool {
	return start+1 < copyStep &&
		steps[start].opAfter == "&&" &&
		shellStepExitsParentShell(steps, start+1)
}

func shellDestinationExistsIfTrueBranchExitsBeforeCopy(steps []shellCommandStep, start, copyStep int) bool {
	thenStep, ok := shellIfDestinationExistsConditionDirectlyEntersThen(steps, start, copyStep)
	if !ok {
		return false
	}
	nestedCompoundDepth := 0
	trueBranchExits := false
	for i := thenStep; i < copyStep; i++ {
		if nestedCompoundDepth > 0 {
			if shellStepStartsConditionalCompound(steps[i]) {
				nestedCompoundDepth++
			}
			if shellStepEndsConditionalCompound(steps[i]) {
				nestedCompoundDepth--
			}
			continue
		}
		switch {
		case shellStepStartsConditionalCompound(steps[i]):
			nestedCompoundDepth++
		case shellStepIsFi(steps[i]):
			return trueBranchExits
		case shellStepStartsElseBranch(steps[i]):
			return false
		case shellStepHasStandaloneExplicitExit(steps, i):
			trueBranchExits = true
		}
	}
	return false
}

func shellIfDestinationExistsConditionDirectlyEntersThen(steps []shellCommandStep, start, copyStep int) (int, bool) {
	if !shellStepStartsIf(steps[start]) {
		return 0, false
	}
	if !shellStepEndsDirectIfConditionCheck(steps[start]) {
		return 0, false
	}
	for i := start + 1; i < copyStep; i++ {
		if shellStepStartsThenBranch(steps[i]) {
			return i, true
		}
		return 0, false
	}
	return 0, false
}

func shellStepEndsDirectIfConditionCheck(step shellCommandStep) bool {
	return step.opAfter == "" || step.opAfter == ";"
}

func shellDestinationNonexistenceCheckStep(steps []shellCommandStep, limit int, destination string) int {
	for i := 0; i < limit; i++ {
		if !shellStepCanStartSimpleDestinationGuard(steps, i, limit) {
			continue
		}
		if shellStepIsDestinationNonexistenceCheck(steps[i], destination) {
			return i
		}
	}
	return -1
}

func shellLineHasDestinationNonexistenceCheck(line, destination string) bool {
	return shellFieldsHaveDestinationNonexistenceCheck(shellCommandFields(line), destination)
}

func shellStepIsDestinationNonexistenceCheck(step shellCommandStep, destination string) bool {
	return shellFieldsHaveDestinationNonexistenceCheck(step.fields, destination)
}

func shellFieldsHaveDestinationNonexistenceCheck(fields []string, destination string) bool {
	fields = shellConditionCommandFields(fields)
	return shellFieldsAreSimpleDestinationTest(fields, "!", destination)
}

func shellLineHasDestinationExistsCheck(line, destination string) bool {
	return shellFieldsHaveDestinationExistsCheck(shellCommandFields(line), destination)
}

func shellFieldsHaveDestinationExistsCheck(fields []string, destination string) bool {
	fields = shellConditionCommandFields(fields)
	return shellFieldsAreSimpleDestinationTest(fields, "", destination)
}

func shellFieldsAreSimpleDestinationTest(fields []string, negation, destination string) bool {
	switch {
	case len(fields) > 0 && fields[0] == "test":
		return shellTestFieldsAreSimpleDestinationTest(fields, negation, destination)
	case len(fields) > 0 && (fields[0] == "[" || fields[0] == "[["):
		return shellBracketFieldsAreSimpleDestinationTest(fields, negation, destination)
	default:
		return false
	}
}

func shellTestFieldsAreSimpleDestinationTest(fields []string, negation, destination string) bool {
	if negation == "!" {
		return len(fields) == 4 &&
			fields[1] == "!" &&
			fields[2] == "-e" &&
			shellPathTokenEqual(fields[3], destination)
	}
	return len(fields) == 3 &&
		fields[1] == "-e" &&
		shellPathTokenEqual(fields[2], destination)
}

func shellBracketFieldsAreSimpleDestinationTest(fields []string, negation, destination string) bool {
	close := "]"
	if fields[0] == "[[" {
		close = "]]"
	}
	if negation == "!" {
		return len(fields) == 5 &&
			fields[1] == "!" &&
			fields[2] == "-e" &&
			shellPathTokenEqual(fields[3], destination) &&
			fields[4] == close
	}
	return len(fields) == 4 &&
		fields[1] == "-e" &&
		shellPathTokenEqual(fields[2], destination) &&
		fields[3] == close
}

func shellLineHasExplicitExit(line string) bool {
	steps := shellCommandSteps([]string{line})
	for i := range steps {
		if shellStepExitsParentShell(steps, i) {
			return true
		}
	}
	return false
}

func shellStepHasExplicitExit(step shellCommandStep) bool {
	return shellFieldsHaveExplicitExit(step.fields)
}

func shellStepHasStandaloneExplicitExit(steps []shellCommandStep, index int) bool {
	if !shellStepExitsParentShell(steps, index) {
		return false
	}
	return index == 0 || (steps[index-1].opAfter != "&&" && steps[index-1].opAfter != "||")
}

func shellStepExitsParentShell(steps []shellCommandStep, index int) bool {
	if index < 0 || index >= len(steps) {
		return false
	}
	if shellScopedBodyDepthAtStep(steps, index) != 0 {
		return false
	}
	if shellStepParticipatesInPipeline(steps, index) {
		return false
	}
	if shellStepRunsInBackground(steps, index) {
		return false
	}
	return shellStepHasExplicitExit(steps[index])
}

func shellStepParticipatesInPipeline(steps []shellCommandStep, index int) bool {
	return index >= 0 && index < len(steps) &&
		(steps[index].opAfter == "|" || (index > 0 && steps[index-1].opAfter == "|"))
}

func shellStepRunsInBackground(steps []shellCommandStep, index int) bool {
	return index >= 0 && index < len(steps) && steps[index].opAfter == "&"
}

func shellStepCanProveParentShellStateChange(steps []shellCommandStep, index int) bool {
	if index < 0 || index >= len(steps) {
		return false
	}
	if !shellStepMayAffectParentShellState(steps, index) {
		return false
	}
	if shellStepRunsInErrexitExemptCondition(steps, index) {
		return false
	}
	if shellStepIsShortCircuitControlled(steps, index) {
		return false
	}
	return shellStepCanLinearlyDominateCopy(steps, index, index+1)
}

func shellStepMayAffectParentShellState(steps []shellCommandStep, index int) bool {
	if index < 0 || index >= len(steps) {
		return false
	}
	if shellScopedBodyDepthAtStep(steps, index) != 0 {
		return false
	}
	return !shellStepParticipatesInPipeline(steps, index) &&
		!shellStepRunsInBackground(steps, index)
}

func shellFieldsHaveExplicitExit(fields []string) bool {
	fields = shellBuiltinInvocationFields(fields)
	if len(fields) < 2 || fields[0] != "exit" {
		return false
	}
	status, err := strconv.Atoi(strings.Trim(fields[1], `"'`))
	return err == nil && status != 0
}

func shellNonexistenceCheckHasOrExitBeforeCopy(steps []shellCommandStep, start, end int) bool {
	return start+1 < end && steps[start].opAfter == "||" && shellStepExitsParentShell(steps, start+1)
}

func shellStepIsStandaloneErrexitGuard(steps []shellCommandStep, index int) bool {
	if index < 0 || index >= len(steps) {
		return false
	}
	if shellStepParticipatesInPipeline(steps, index) {
		return false
	}
	if shellStepRunsInBackground(steps, index) {
		return false
	}
	if shellStepRunsInErrexitExemptCondition(steps, index) {
		return false
	}
	if steps[index].opAfter == "&&" || steps[index].opAfter == "||" {
		return false
	}
	return index == 0 || (steps[index-1].opAfter != "&&" && steps[index-1].opAfter != "||")
}

func shellStepRunsInErrexitExemptCondition(steps []shellCommandStep, index int) bool {
	if index < 0 || index >= len(steps) {
		return false
	}
	var terminators []string
	for i := 0; i <= index; i++ {
		if len(steps[i].fields) == 0 {
			continue
		}
		keyword := steps[i].fields[0]
		if shellConditionListTerminator(keyword) {
			terminators = shellPopConditionListTerminator(terminators, keyword)
			if i == index {
				return false
			}
			continue
		}
		if len(terminators) > 0 && i == index {
			return true
		}
		if terminator, ok := shellConditionListStartTerminator(keyword); ok {
			terminators = append(terminators, terminator)
			if i == index {
				return true
			}
		}
	}
	return false
}

func shellConditionListStartTerminator(keyword string) (string, bool) {
	switch keyword {
	case "if", "elif":
		return "then", true
	case "while", "until":
		return "do", true
	default:
		return "", false
	}
}

func shellConditionListTerminator(keyword string) bool {
	return keyword == "then" || keyword == "do"
}

func shellPopConditionListTerminator(terminators []string, keyword string) []string {
	if len(terminators) == 0 || terminators[len(terminators)-1] != keyword {
		return terminators
	}
	return terminators[:len(terminators)-1]
}

func shellStepCanStartExplicitDestinationGuard(steps []shellCommandStep, index, copyStep int) bool {
	if !shellStepCanStartTopLevelDestinationGuard(steps, index, copyStep) {
		return false
	}
	if shellStepStartsIf(steps[index]) {
		return true
	}
	return !shellStepRunsInErrexitExemptCondition(steps, index)
}

func shellStepCanStartSimpleDestinationGuard(steps []shellCommandStep, index, copyStep int) bool {
	return shellStepCanStartTopLevelDestinationGuard(steps, index, copyStep) &&
		!shellStepRunsInErrexitExemptCondition(steps, index)
}

func shellStepCanStartTopLevelDestinationGuard(steps []shellCommandStep, index, copyStep int) bool {
	return shellStepCanLinearlyDominateCopy(steps, index, copyStep) &&
		!shellStepParticipatesInPipeline(steps, index) &&
		!shellStepRunsInBackground(steps, index) &&
		!shellStepIsShortCircuitControlled(steps, index)
}

func shellStepIsShortCircuitControlled(steps []shellCommandStep, index int) bool {
	if index <= 0 || index >= len(steps) {
		return false
	}
	return steps[index-1].opAfter == "&&" || steps[index-1].opAfter == "||"
}

func shellStepCanLinearlyDominateCopy(steps []shellCommandStep, index, copyStep int) bool {
	if index < 0 || index >= len(steps) || copyStep < 0 || copyStep > len(steps) || index >= copyStep {
		return false
	}
	if shellBranchBodyDepthAtStep(steps, index) != 0 {
		return false
	}
	if shellScopedBodyDepthAtStep(steps, index) != 0 {
		return false
	}
	if copyStep < len(steps) && shellBranchBodyDepthAtStep(steps, copyStep) != 0 {
		return false
	}
	if copyStep < len(steps) && shellScopedBodyDepthAtStep(steps, copyStep) != 0 {
		return false
	}
	return true
}

func shellBranchBodyDepthAtStep(steps []shellCommandStep, index int) int {
	depth := shellBranchBodyDepthBeforeStep(steps, index)
	if index < 0 || index >= len(steps) || len(steps[index].fields) == 0 {
		return depth
	}
	switch steps[index].fields[0] {
	case "then", "do":
		return depth + 1
	case "esac":
		if depth > 0 {
			return depth - 1
		}
	case "else", "elif":
		if depth == 0 {
			return 1
		}
	}
	return depth
}

func shellBranchBodyDepthBeforeStep(steps []shellCommandStep, index int) int {
	depth := 0
	for i := 0; i < index && i < len(steps); i++ {
		if len(steps[i].fields) == 0 {
			continue
		}
		switch steps[i].fields[0] {
		case "case", "then", "do":
			depth++
		case "esac", "fi", "done":
			if depth > 0 {
				depth--
			}
		}
	}
	return depth
}

func shellScopedBodyDepthAtStep(steps []shellCommandStep, index int) int {
	depth := shellScopedBodyDepthBeforeStep(steps, index)
	if index < 0 || index >= len(steps) {
		return depth
	}
	if shellStepEndsScopedBody(steps[index]) && depth > 0 {
		return depth - 1
	}
	return depth
}

func shellScopedBodyDepthBeforeStep(steps []shellCommandStep, index int) int {
	depth := 0
	for i := 0; i < index && i < len(steps); i++ {
		if shellStepEndsScopedBody(steps[i]) {
			if depth > 0 {
				depth--
			}
			continue
		}
		if shellStepStartsScopedBodyAt(steps, i) {
			depth++
		}
	}
	return depth
}

func shellStepStartsScopedBodyAt(steps []shellCommandStep, index int) bool {
	if index < 0 || index >= len(steps) {
		return false
	}
	return shellStepStartsScopedBody(steps[index]) || shellStepOpensPendingFunctionBody(steps, index)
}

func shellStepStartsScopedBody(step shellCommandStep) bool {
	return shellStepStartsSubshellBody(step) || shellStepStartsFunctionBody(step)
}

func shellStepOpensPendingFunctionBody(steps []shellCommandStep, index int) bool {
	if index <= 0 || index >= len(steps) || steps[index-1].opAfter != "" {
		return false
	}
	fields := shellTrimReservedCommandPrefixes(steps[index].fields)
	return len(fields) == 1 && fields[0] == "{" && shellStepIsFunctionHeader(steps[index-1])
}

func shellStepEndsScopedBody(step shellCommandStep) bool {
	fields := shellTrimReservedCommandPrefixes(step.fields)
	return len(fields) == 1 && (fields[0] == ")" || fields[0] == "}")
}

func shellStepStartsSubshellBody(step shellCommandStep) bool {
	fields := shellTrimReservedCommandPrefixes(step.fields)
	return len(fields) == 1 && fields[0] == "("
}

func shellStepStartsFunctionBody(step shellCommandStep) bool {
	fields := shellTrimReservedCommandPrefixes(step.fields)
	switch {
	case len(fields) >= 1 && shellCompactFunctionNameWithOpeningBrace(fields[0]):
		return true
	case len(fields) >= 2 && fields[1] == "{" && shellFunctionNameWithParens(fields[0]):
		return true
	case len(fields) >= 3 && fields[1] == "()" && fields[2] == "{" && shellIdentifierName(fields[0]):
		return true
	case len(fields) >= 2 && fields[0] == "function" && shellCompactFunctionNameWithOpeningBrace(fields[1]):
		return true
	case len(fields) >= 3 && fields[0] == "function" && shellFunctionKeywordName(fields[1]) && fields[2] == "{":
		return true
	case len(fields) >= 4 && fields[0] == "function" && shellIdentifierName(fields[1]) && fields[2] == "()" && fields[3] == "{":
		return true
	default:
		return false
	}
}

func shellStepIsFunctionHeader(step shellCommandStep) bool {
	fields := shellTrimReservedCommandPrefixes(step.fields)
	switch {
	case len(fields) == 1 && shellFunctionNameWithParens(fields[0]):
		return true
	case len(fields) == 2 && fields[1] == "()" && shellIdentifierName(fields[0]):
		return true
	case len(fields) == 2 && fields[0] == "function" && shellFunctionKeywordName(fields[1]):
		return true
	case len(fields) == 3 && fields[0] == "function" && shellIdentifierName(fields[1]) && fields[2] == "()":
		return true
	default:
		return false
	}
}

func shellFunctionNameWithParens(field string) bool {
	name := strings.TrimSuffix(field, "()")
	return name != field && shellIdentifierName(name)
}

func shellFunctionKeywordName(field string) bool {
	return shellIdentifierName(field) || shellFunctionNameWithParens(field)
}

func shellCompactFunctionNameWithOpeningBrace(field string) bool {
	header := strings.TrimSuffix(field, "{")
	return header != field && shellFunctionNameWithParens(header)
}

func shellBlockHasChainedAndBetween(steps []shellCommandStep, start, end int) bool {
	for i := start; i < end; i++ {
		if steps[i].opAfter != "&&" {
			return false
		}
	}
	return true
}

func shellStepIsFi(step shellCommandStep) bool {
	return len(step.fields) == 1 && step.fields[0] == "fi"
}

func shellStepStartsThenBranch(step shellCommandStep) bool {
	return len(step.fields) > 0 && step.fields[0] == "then"
}

func shellStepStartsIf(step shellCommandStep) bool {
	return len(step.fields) > 0 && step.fields[0] == "if"
}

func shellStepStartsConditionalCompound(step shellCommandStep) bool {
	fields := shellTrimReservedCommandPrefixes(step.fields)
	if len(fields) == 0 {
		return false
	}
	switch fields[0] {
	case "if", "while", "until", "for", "case":
		return true
	default:
		return false
	}
}

func shellStepEndsConditionalCompound(step shellCommandStep) bool {
	if len(step.fields) == 0 {
		return false
	}
	switch step.fields[0] {
	case "fi", "done", "esac":
		return true
	default:
		return false
	}
}

func shellStepStartsElseBranch(step shellCommandStep) bool {
	return len(step.fields) > 0 && (step.fields[0] == "else" || step.fields[0] == "elif")
}

func shellCommandFields(line string) []string {
	tokens := shellLineTokens(line)
	fields := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if shellControlOperator(token) {
			continue
		}
		fields = append(fields, token)
	}
	return fields
}

func shellLineTokens(line string) []string {
	var tokens []string
	for _, field := range strings.Fields(line) {
		for _, token := range splitShellControlOperators(field) {
			if shellControlOperator(token) {
				tokens = append(tokens, token)
				continue
			}
			cleaned := cleanShellField(token)
			if cleaned == "" {
				continue
			}
			tokens = append(tokens, cleaned)
		}
	}
	return tokens
}

func splitShellControlOperators(field string) []string {
	var tokens []string
	for field != "" {
		index, operator := firstShellControlOperator(field)
		switch {
		case index < 0:
			tokens = append(tokens, field)
			field = ""
		case index == 0:
			tokens = append(tokens, operator)
			field = field[len(operator):]
		default:
			tokens = append(tokens, field[:index])
			field = field[index:]
		}
	}
	return tokens
}

func firstShellControlOperator(field string) (int, string) {
	firstIndex := -1
	firstOperator := ""
	for _, operator := range []string{"&&", "||", ";", "|", "&"} {
		searchFrom := 0
		for {
			index := strings.Index(field[searchFrom:], operator)
			if index < 0 {
				break
			}
			index += searchFrom
			if operator == "&" && shellAmpersandIsRedirection(field, index) {
				searchFrom = index + len(operator)
				continue
			}
			if firstIndex < 0 || index < firstIndex {
				firstIndex = index
				firstOperator = operator
			}
			break
		}
	}
	return firstIndex, firstOperator
}

func shellAmpersandIsRedirection(field string, index int) bool {
	if index > 0 && (field[index-1] == '>' || field[index-1] == '<') {
		return true
	}
	if index+1 < len(field) && field[index+1] == '>' {
		return true
	}
	return false
}

func shellControlOperator(token string) bool {
	return token == "&&" || token == "||" || token == ";" || token == "|" || token == "&"
}

func cleanShellField(field string) string {
	field = strings.TrimSpace(field)
	field = strings.TrimRight(field, ";")
	field = strings.TrimSuffix(field, "&&")
	field = strings.TrimSuffix(field, "||")
	return strings.TrimSpace(field)
}

func shellPathTokenEqual(left, right string) bool {
	return normalizeShellPathToken(left) == normalizeShellPathToken(right)
}

func shellPathTokenCreatesDestinationOrDescendant(path, destination string) bool {
	path = normalizeShellDirectoryToken(path)
	destination = normalizeShellDirectoryToken(destination)
	if path == "" || destination == "" {
		return false
	}
	if shellPathTokenHasDirectoryPrefix(path, destination) {
		return true
	}
	if name, ok := shellPathTokenVariableName(destination); ok {
		return shellPathTokenHasDirectoryPrefix(path, "$"+name) ||
			shellPathTokenHasDirectoryPrefix(path, "${"+name+"}")
	}
	return false
}

func shellPathTokenHasDirectoryPrefix(path, prefix string) bool {
	return path == prefix || strings.HasPrefix(path, prefix+"/")
}

func shellPathTokenVariableName(token string) (string, bool) {
	token = normalizeShellDirectoryToken(token)
	if strings.HasPrefix(token, "${") && strings.HasSuffix(token, "}") {
		name := token[2 : len(token)-1]
		return name, shellIdentifierName(name)
	}
	if strings.HasPrefix(token, "$") {
		name := token[1:]
		return name, shellIdentifierName(name)
	}
	return "", false
}

func shellIdentifierName(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		ch := name[i]
		if ch == '_' || ('a' <= ch && ch <= 'z') || ('A' <= ch && ch <= 'Z') {
			continue
		}
		if i > 0 && '0' <= ch && ch <= '9' {
			continue
		}
		return false
	}
	return true
}

func normalizeShellPathToken(token string) string {
	token = cleanShellField(token)
	token = normalizeShellQuotedPathPrefixJoin(token)
	token = normalizeShellQuotedPathSegments(token)
	token = strings.Trim(token, `"'`)
	token = normalizeShellQuotedVariablePathJoin(token)
	token = strings.TrimSuffix(token, "]]")
	token = strings.TrimSuffix(token, "]")
	token = strings.TrimSuffix(token, ";")
	token = strings.TrimSuffix(token, "&&")
	return strings.TrimSpace(token)
}

func normalizeShellQuotedPathSegments(token string) string {
	var builder strings.Builder
	quote := byte(0)
	changed := false
	for i := 0; i < len(token); i++ {
		ch := token[i]
		if quote != 0 {
			if ch == quote {
				quote = 0
				changed = true
				continue
			}
			builder.WriteByte(ch)
			continue
		}
		if ch == '"' || ch == '\'' {
			quote = ch
			changed = true
			continue
		}
		builder.WriteByte(ch)
	}
	if quote != 0 || !changed {
		return token
	}
	return builder.String()
}

func normalizeShellQuotedPathPrefixJoin(token string) string {
	if len(token) < 3 {
		return token
	}
	quote := token[0]
	if quote != '"' && quote != '\'' {
		return token
	}
	end := strings.IndexByte(token[1:], quote)
	if end < 0 {
		return token
	}
	end++
	if end+1 >= len(token) || token[end+1] != '/' {
		return token
	}
	return token[1:end] + token[end+1:]
}

func normalizeShellQuotedVariablePathJoin(token string) string {
	end, ok := shellLeadingVariableTokenEnd(token)
	if !ok || end+1 >= len(token) {
		return token
	}
	if (token[end] == '"' || token[end] == '\'') && token[end+1] == '/' {
		return token[:end] + token[end+1:]
	}
	return token
}

func shellLeadingVariableTokenEnd(token string) (int, bool) {
	if strings.HasPrefix(token, "${") {
		end := strings.IndexByte(token, '}')
		if end < 0 {
			return 0, false
		}
		return end + 1, shellIdentifierName(token[2:end])
	}
	if !strings.HasPrefix(token, "$") {
		return 0, false
	}
	end := 1
	for end < len(token) {
		ch := token[end]
		if ch == '_' || ('a' <= ch && ch <= 'z') || ('A' <= ch && ch <= 'Z') || (end > 1 && '0' <= ch && ch <= '9') {
			end++
			continue
		}
		break
	}
	return end, shellIdentifierName(token[1:end])
}

func normalizeShellDirectoryToken(token string) string {
	token = normalizeShellPathToken(token)
	token = normalizeShellLiteralDirectoryPath(token)
	if token == "/" {
		return token
	}
	return strings.TrimRight(token, "/")
}

func normalizeShellLiteralDirectoryPath(token string) string {
	if !shellPathTokenIsLiteralPath(token) || !strings.Contains(token, "/") {
		return token
	}
	absolute := strings.HasPrefix(token, "/")
	segments := strings.Split(token, "/")
	cleaned := make([]string, 0, len(segments))
	for _, segment := range segments {
		if segment == "" || segment == "." {
			continue
		}
		if segment == ".." {
			if len(cleaned) > 0 && cleaned[len(cleaned)-1] != ".." {
				cleaned = cleaned[:len(cleaned)-1]
				continue
			}
			if absolute {
				continue
			}
		}
		cleaned = append(cleaned, segment)
	}
	if absolute {
		if len(cleaned) == 0 {
			return "/"
		}
		return "/" + strings.Join(cleaned, "/")
	}
	return strings.Join(cleaned, "/")
}

func shellPathTokenIsLiteralPath(token string) bool {
	if token == "" {
		return false
	}
	return !strings.ContainsAny(token, "$`*?[]{}")
}

func shellTrimReservedCommandPrefixes(fields []string) []string {
	for len(fields) > 0 {
		switch fields[0] {
		case "then", "do":
			fields = fields[1:]
		default:
			return fields
		}
	}
	return fields
}

func shellBuiltinInvocationFields(fields []string) []string {
	fields = shellTrimReservedCommandPrefixes(fields)
	for len(fields) > 0 {
		switch fields[0] {
		case "command", "builtin":
			fields = fields[1:]
		default:
			return fields
		}
	}
	return fields
}

func shellConditionCommandFields(fields []string) []string {
	fields = shellTrimReservedCommandPrefixes(fields)
	if len(fields) > 0 && fields[0] == "if" {
		fields = fields[1:]
	}
	return shellCommandInvocationFields(fields)
}

func releaseFacingSyncGuidanceBlocks(body string) []markdownCodeBlock {
	blocks := markdownFencedCodeBlocks(body)
	for _, block := range markdownNonFencedTextBlocks(body) {
		if !nonFencedSyncGuidanceCopiesWholeJVSMetadata(block.text) {
			continue
		}
		blocks = append(blocks, block)
	}
	return blocks
}

func markdownNonFencedTextBlocks(body string) []markdownCodeBlock {
	var blocks []markdownCodeBlock
	var block strings.Builder
	inFence := false
	startLine := 0
	flush := func() {
		if startLine == 0 {
			return
		}
		blocks = append(blocks, markdownCodeBlock{
			startLine: startLine,
			text:      block.String(),
		})
		startLine = 0
		block.Reset()
	}
	for i, line := range strings.Split(body, "\n") {
		lineNo := i + 1
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			flush()
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if strings.TrimSpace(line) == "" {
			flush()
			continue
		}
		if startLine == 0 {
			startLine = lineNo
		}
		block.WriteString(line)
		block.WriteByte('\n')
	}
	flush()
	return blocks
}

func nonFencedSyncGuidanceCopiesWholeJVSMetadata(line string) bool {
	if !wholeJVSMetadataPathPattern.MatchString(line) {
		return false
	}
	lower := strings.ToLower(line)
	for _, marker := range []string{
		"juicefs sync",
		"rsync ",
		"copy repository",
		"copying repository",
		"sync repository",
		"sync metadata",
		"external sync",
		"external copy",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func syncExampleCopiesJVSMetadata(block string) bool {
	lower := strings.ToLower(block)
	if !strings.Contains(lower, "juicefs sync") && !strings.Contains(lower, "rsync ") {
		return nonFencedSyncGuidanceCopiesWholeJVSMetadata(block)
	}
	if strings.Contains(block, ".jvs/") {
		return true
	}
	for _, marker := range []string{"jvs", "repo", "repository", "checkpoint", "workspace"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func conformanceContractAreaNames(t *testing.T) []string {
	t.Helper()
	plan := readRepoFile(t, "docs/11_CONFORMANCE_TEST_PLAN.md")
	inMandatoryAreas := false
	var areas []string
	for _, line := range strings.Split(plan, "\n") {
		switch {
		case strings.HasPrefix(line, "## Mandatory Contract Areas"):
			inMandatoryAreas = true
		case inMandatoryAreas && strings.HasPrefix(line, "## "):
			inMandatoryAreas = false
		case inMandatoryAreas && strings.HasPrefix(line, "### "):
			areas = append(areas, strings.TrimSpace(strings.TrimPrefix(line, "### ")))
		}
	}
	if len(areas) == 0 {
		t.Fatalf("docs/11_CONFORMANCE_TEST_PLAN.md must define mandatory contract areas")
	}
	return areas
}

func lineReferencesKnownContractArea(line string, areas []string) bool {
	for _, area := range areas {
		if strings.Contains(line, area) {
			return true
		}
	}
	return false
}

func publicDocsWithoutHistoricalReleaseMentions() []string {
	var docs []string
	for _, doc := range activePublicContractDocs() {
		switch doc {
		case "docs/99_CHANGELOG.md", "docs/PERFORMANCE_RESULTS.md":
			continue
		default:
			docs = append(docs, doc)
		}
	}
	return docs
}

func assertTestFileUsesStableCommands(t *testing.T, path string) {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, repoFile(t, path), nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	hiddenCommandReplacements := map[string]string{
		"checkpoint": "save or history",
		"fork":       "workspace new",
		"gc":         "cleanup preview/run",
		"verify":     "doctor --strict",
		"snapshot":   "save point",
		"worktree":   "workspace",
	}
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || !isRunJVSCall(call.Fun) {
			return true
		}
		for _, arg := range call.Args {
			lit, ok := arg.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				continue
			}
			value, err := strconv.Unquote(lit.Value)
			if err != nil {
				continue
			}
			if replacement, ok := hiddenCommandReplacements[value]; ok {
				pos := fset.Position(lit.Pos())
				t.Fatalf("%s:%d public-profile test invokes hidden/non-public command %q; use %s", path, pos.Line, value, replacement)
			}
		}
		return true
	})
}

func isRunJVSCall(expr ast.Expr) bool {
	ident, ok := expr.(*ast.Ident)
	if !ok {
		return false
	}
	switch ident.Name {
	case "runJVS", "runJVSInRepo", "runJVSInWorktree":
		return true
	default:
		return false
	}
}

func jsonFieldsForStruct(t *testing.T, path, structName string) []string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, repoFile(t, path), nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	var fields []string
	found := false
	ast.Inspect(file, func(node ast.Node) bool {
		typeSpec, ok := node.(*ast.TypeSpec)
		if !ok || typeSpec.Name.Name != structName {
			return true
		}
		found = true
		structType, ok := typeSpec.Type.(*ast.StructType)
		if !ok {
			t.Fatalf("%s must remain a struct", structName)
		}
		for _, field := range structType.Fields.List {
			if field.Tag == nil {
				continue
			}
			tag, err := strconv.Unquote(field.Tag.Value)
			if err != nil {
				t.Fatalf("unquote %s tag %q: %v", structName, field.Tag.Value, err)
			}
			jsonField := jsonTagName(tag)
			if jsonField == "" || jsonField == "-" {
				continue
			}
			fields = append(fields, jsonField)
		}
		return false
	})
	if !found {
		t.Fatalf("%s not found in %s", structName, path)
	}
	return fields
}

func jsonTagName(tag string) string {
	for _, part := range strings.Split(tag, " ") {
		if !strings.HasPrefix(part, `json:"`) {
			continue
		}
		value := strings.TrimPrefix(part, `json:"`)
		value = strings.TrimSuffix(value, `"`)
		if idx := strings.Index(value, ","); idx >= 0 {
			value = value[:idx]
		}
		return value
	}
	return ""
}

func assertSameStringSet(t *testing.T, name string, got, want []string) {
	t.Helper()
	gotSet := make(map[string]int, len(got))
	for _, value := range got {
		gotSet[value]++
	}
	wantSet := make(map[string]int, len(want))
	for _, value := range want {
		wantSet[value]++
	}
	for value, count := range wantSet {
		if gotSet[value] != count {
			t.Fatalf("%s mismatch: got %v, want %v", name, got, want)
		}
	}
	for value, count := range gotSet {
		if wantSet[value] != count {
			t.Fatalf("%s mismatch: got %v, want %v", name, got, want)
		}
	}
}

func scanPublicDocLines(t *testing.T, doc string, visit func(lineNo int, line string)) {
	t.Helper()
	path := repoFile(t, doc)
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", doc, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNo := 0
	codePackageSectionLevel := 0
	internalStorageSectionLevel := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		if level, ok := markdownHeadingLevel(line); ok {
			if codePackageSectionLevel > 0 && level <= codePackageSectionLevel {
				codePackageSectionLevel = 0
			}
			if internalStorageSectionLevel > 0 && level <= internalStorageSectionLevel {
				internalStorageSectionLevel = 0
			}
			if markdownHeadingNamesCodePackage(line) {
				codePackageSectionLevel = level
			}
			if markdownHeadingNamesInternalStorage(line) {
				internalStorageSectionLevel = level
			}
		}
		if codePackageSectionLevel > 0 {
			continue
		}
		if internalStorageSectionLevel > 0 &&
			lineNamesInternalStoragePath(line) &&
			!pathShapedRuntimeMechanismPattern.MatchString(line) {
			continue
		}
		if allowedPublicDocCompatibilityLine(doc, line) {
			continue
		}
		visit(lineNo, line)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan %s: %v", doc, err)
	}
}

func markdownHeadingNamesCodePackage(line string) bool {
	lower := strings.ToLower(line)
	return strings.Contains(lower, "pkg/") || strings.Contains(lower, "internal/")
}

func markdownHeadingNamesInternalStorage(line string) bool {
	lower := strings.ToLower(line)
	return strings.Contains(lower, "internal storage") || strings.Contains(lower, "storage layout")
}

func lineNamesInternalStoragePath(line string) bool {
	lower := strings.ToLower(line)
	for _, marker := range []string{
		".jvs/",
		"descriptors/",
		"snapshots/",
		"worktrees/",
		"audit/",
		"gc/",
		"intents/",
		"recovery-plans/",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func readRepoFile(t *testing.T, parts ...string) string {
	t.Helper()
	data, err := os.ReadFile(repoFile(t, parts...))
	if err != nil {
		t.Fatalf("read %s: %v", filepath.Join(parts...), err)
	}
	return string(data)
}

func latestChangelogHeading(t *testing.T) string {
	t.Helper()
	entry := firstChangelogEntry(readRepoFile(t, "docs/99_CHANGELOG.md"))
	match := releaseEvidenceHeadingPattern.FindStringSubmatch(entry)
	if match == nil {
		t.Fatalf("latest changelog entry must start with a v0 release heading")
	}
	return "## " + match[1] + " - " + match[2]
}

func releaseEvidenceClaimsFinalTaggedRelease(entry string) bool {
	return releaseEvidenceStatusPassPattern.MatchString(entry) ||
		releaseEvidenceGatePassPattern.MatchString(entry) ||
		releaseEvidencePublishedArtifactCountPattern.MatchString(entry) ||
		releaseEvidenceTagPattern.MatchString(entry) ||
		strings.Contains(strings.ToLower(entry), "evidence class: final release evidence") ||
		strings.Contains(strings.ToLower(entry), "evidence class: final tagged release")
}

func requireCandidateReleaseEvidence(t *testing.T, heading, entry string) {
	t.Helper()
	for _, required := range []string{
		"Evidence class: GA candidate readiness",
		"not final",
		"not tagged",
		"not published",
		"pending final tag",
		"Candidate target tag",
	} {
		requireReleaseReadinessText(t, "candidate release evidence "+heading, entry, required)
	}
}

func requireFinalTaggedReleaseEvidence(t *testing.T, heading, entry string) {
	t.Helper()
	requireReleaseReadinessText(t, "final tagged release evidence "+heading, entry, "Evidence class: Final release evidence")
	tagMatch := releaseEvidenceTagPattern.FindStringSubmatch(entry)
	if tagMatch == nil {
		t.Fatalf("final tagged release evidence %q must record a final Tag line", heading)
	}
	commitMatch := releaseEvidenceCommitPattern.FindStringSubmatch(entry)
	if commitMatch == nil {
		t.Fatalf("final tagged release evidence %q must record Final tagged commit with a 40-character SHA", heading)
	}
	if !releaseEvidenceStatusPassPattern.MatchString(entry) {
		t.Fatalf("final tagged release evidence %q must record Status: PASS", heading)
	}
	if !releaseEvidenceGatePassPattern.MatchString(entry) {
		t.Fatalf("final tagged release evidence %q must record release-gate PASS", heading)
	}
	if !strings.Contains(strings.ToLower(entry), "published artifacts") {
		t.Fatalf("final tagged release evidence %q must record published artifact evidence", heading)
	}

	tag := tagMatch[1]
	tagCommit, ok := gitTagCommit(t, tag)
	if !ok {
		t.Fatalf("final tagged release evidence %q claims tag %s, but refs/tags/%s is not available in this checkout; CI jobs that run tag-aware release validation must fetch full history and tags (for example actions/checkout fetch-depth: 0), otherwise this failure can be missing local tag metadata rather than incorrect release evidence", heading, tag, tag)
	}
	if tagCommit != commitMatch[1] {
		t.Fatalf("final tagged release evidence %q commit %s does not match tag %s commit %s", heading, commitMatch[1], tag, tagCommit)
	}
}

func gitTagCommit(t *testing.T, tag string) (string, bool) {
	t.Helper()
	cmd := exec.Command("git", "rev-list", "-n", "1", "refs/tags/"+tag)
	cmd.Dir = repoFile(t)
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	commit := strings.TrimSpace(string(out))
	if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(commit) {
		t.Fatalf("git rev-list returned non-commit output for tag %s: %q", tag, commit)
	}
	return commit, true
}

func gitShowFileAtRef(t *testing.T, ref, path string) string {
	t.Helper()
	cmd := exec.Command("git", "show", ref+":"+path)
	cmd.Dir = repoFile(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git show %s:%s failed: %v\noutput:\n%s\nTag-aware release validation needs local tag objects and history; if this is a missing tag or shallow checkout, fetch tags/full history (for example actions/checkout fetch-depth: 0).", ref, path, err, strings.TrimSpace(string(out)))
	}
	return string(out)
}

type releaseEvidenceDisclosureTarget struct {
	name string
	body string
}

func candidateTagSourceArchiveDisclosureMissing(tag, class string, targets ...releaseEvidenceDisclosureTarget) []string {
	var missing []string
	exactRequirements := []string{
		"Tag source archive evidence class: `" + class + "`",
		"Final evidence location: GitHub Release page and post-release main ledger",
		"Tag movement: `" + tag + "` was not moved",
		canonicalReleaseURLForTag(tag),
	}
	caseInsensitiveRequirements := []string{
		"source archive boundary",
		"immutable tag snapshot",
		"publication final evidence",
		"GitHub Release page",
		"post-release main ledger",
	}

	for _, target := range targets {
		for _, required := range exactRequirements {
			if !strings.Contains(target.body, required) {
				missing = append(missing, target.name+" missing "+required)
			}
		}

		lowerBody := strings.ToLower(target.body)
		for _, required := range caseInsensitiveRequirements {
			if !strings.Contains(lowerBody, strings.ToLower(required)) {
				missing = append(missing, target.name+" missing "+required)
			}
		}
	}
	return missing
}

func canonicalReleaseURLForTag(tag string) string {
	return "https://github.com/agentsmith-project/jvs/releases/tag/" + tag
}

func releaseEvidenceEntry(t *testing.T, ledger, heading string) string {
	t.Helper()
	return markdownSectionByHeading(t, "docs/RELEASE_EVIDENCE.md", ledger, heading)
}

func changelogEntry(t *testing.T, changelog, heading string) string {
	t.Helper()
	return markdownSectionByHeading(t, "docs/99_CHANGELOG.md", changelog, heading)
}

func releaseEvidenceEntryForVersion(t *testing.T, ledger, version string) string {
	t.Helper()
	for _, match := range releaseEvidenceHeadingPattern.FindAllStringSubmatch(ledger, -1) {
		if match[1] == version {
			return releaseEvidenceEntry(t, ledger, "## "+match[1]+" - "+match[2])
		}
	}
	t.Fatalf("docs/RELEASE_EVIDENCE.md missing release evidence entry for version %s", version)
	return ""
}

func releaseEvidenceClass(t *testing.T, context, entry string) string {
	t.Helper()
	match := releaseEvidenceClassPattern.FindStringSubmatch(entry)
	if match == nil {
		t.Fatalf("%s must record an Evidence class line", context)
	}
	return strings.TrimSpace(match[1])
}

func releaseEvidenceClassIsCandidateReadiness(class string) bool {
	lower := strings.ToLower(class)
	return strings.Contains(lower, "candidate") || strings.Contains(lower, "readiness")
}

func textBetweenMarkers(t *testing.T, body, startMarker, endMarker string) string {
	t.Helper()
	start := strings.Index(body, startMarker)
	if start < 0 {
		t.Fatalf("body must contain start marker %q", startMarker)
	}
	rest := body[start+len(startMarker):]
	end := strings.Index(rest, endMarker)
	if end < 0 {
		t.Fatalf("body block starting at %q must end before %q", startMarker, endMarker)
	}
	return rest[:end]
}

func firstChangelogEntry(changelog string) string {
	lines := strings.Split(changelog, "\n")
	start := -1
	for i, line := range lines {
		if strings.HasPrefix(line, "## ") {
			start = i
			break
		}
	}
	if start == -1 {
		return ""
	}
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "## ") {
			end = i
			break
		}
	}
	return strings.Join(lines[start:end], "\n")
}

func staticStringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func appendUniqueString(values []string, value string) []string {
	if staticStringSliceContains(values, value) {
		return values
	}
	return append(values, value)
}

func repoFile(t *testing.T, parts ...string) string {
	t.Helper()
	segments := append([]string{"..", ".."}, parts...)
	return filepath.Join(segments...)
}
