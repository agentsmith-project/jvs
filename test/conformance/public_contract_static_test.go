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
)

var historyBareJQSelector = regexp.MustCompile(`jq\s+-r\s+['"]\.\[(?:\]|[0-9]+)`)
var traceabilityDocRefPattern = regexp.MustCompile("`((?:README\\.md|docs/[^`]+\\.md))`")
var markdownDocLinkPattern = regexp.MustCompile(`\]\(([^)#]+\.md)(?:#[^)]+)?\)`)
var stalePublicReleaseVocabularyPattern = regexp.MustCompile(`\bv7\.(?:[0-9]+|x)\b`)
var makefileCoverageThresholdPattern = regexp.MustCompile(`if\s*\(\$\$3\+0\s*<\s*([0-9]+)\)`)
var releaseReadinessHeadingPattern = regexp.MustCompile(`(?m)^## v0\.[0-9]+\.[0-9]+ - [0-9]{4}-[0-9]{2}-[0-9]{2}$`)
var releaseEvidenceHeadingPattern = regexp.MustCompile(`(?m)^## (v0\.[0-9]+\.[0-9]+) - ([0-9]{4}-[0-9]{2}-[0-9]{2})$`)
var releaseEvidenceCommitPattern = regexp.MustCompile("(?m)^- Final tagged commit: `([0-9a-f]{40})`$")
var releaseEvidenceTagPattern = regexp.MustCompile("(?m)^- Tag: `(v0\\.[0-9]+\\.[0-9]+)`$")
var releaseEvidenceStatusPassPattern = regexp.MustCompile(`(?mi)^- Status:\s*PASS\b`)
var releaseEvidenceGatePassPattern = regexp.MustCompile(`(?mi)\|\s*Release gate\s*\|[^|]*\|\s*PASS\s*\|`)
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
var doctorEngineSurfacePattern = regexp.MustCompile("(?i)(?:`?jvs\\s+doctor`?|`?doctor`?)[^.]*\\b(?:engine\\s+(?:visibility|probes?|summary)|capabilit(?:y|ies))\\b|\\b(?:engine\\s+(?:visibility|probes?|summary)|capabilit(?:y|ies))\\b[^.]*`?(?:jvs\\s+doctor|doctor)`?")

type unsupportedPublicCLIExampleRule struct {
	name        string
	pattern     *regexp.Regexp
	replacement string
}

type runtimeStateSyncExcludeRule struct {
	name     string
	patterns []string
}

type markdownCodeBlock struct {
	startLine int
	text      string
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

func TestDocs_ReleaseBlockingManifestIncludesPublicDocs(t *testing.T) {
	docs := releaseBlockingContractDocs()
	for _, want := range []string{
		"docs/01_REPO_LAYOUT_SPEC.md",
		"docs/QUICKSTART.md",
		"docs/PRODUCT_PLAN.md",
		"docs/CONSTITUTION.md",
		"docs/TARGET_USERS.md",
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
		if !stringSliceContains(docs, want) {
			t.Fatalf("release-blocking docs manifest must include %s", want)
		}
	}
}

func TestDocs_ReleaseBlockingManifestIncludesLinkedActiveMarkdownDocs(t *testing.T) {
	docs := releaseBlockingContractDocs()
	for _, link := range activePublicMarkdownDocLinks(t) {
		if stringSliceContains(nonReleaseFacingDocs(), link.target) {
			continue
		}
		if !stringSliceContains(docs, link.target) {
			t.Fatalf("release-blocking docs manifest must include markdown doc %s linked from active public doc %s", link.target, link.source)
		}
	}
}

func TestDocs_PublicCommandManifestCoversReleaseBlockingDocs(t *testing.T) {
	commandDocs := publicCommandDocs()
	for _, doc := range releaseBlockingContractDocs() {
		if !stringSliceContains(commandDocs, doc) {
			t.Fatalf("public command docs manifest must cover release-blocking doc %s", doc)
		}
	}
}

func TestDocs_ActivePublicManifestCoversReleaseBlockingDocs(t *testing.T) {
	activeDocs := activePublicContractDocs()
	for _, doc := range releaseBlockingContractDocs() {
		if stringSliceContains(nonReleaseFacingDocs(), doc) {
			continue
		}
		if !stringSliceContains(activeDocs, doc) {
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

func TestDocs_AllMarkdownDocsAreReleaseClassified(t *testing.T) {
	classified := append([]string{}, activePublicContractDocs()...)
	classified = append(classified, archivedNonReleaseFacingDocs()...)
	classified = append(classified, activeNonReleaseFacingDesignDocs()...)
	classified = append(classified, activeNonReleaseFacingReferenceDocs()...)
	for _, doc := range markdownDocsUnder(t, "docs") {
		if !stringSliceContains(classified, doc) {
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
		if !stringSliceContains(docs, doc) {
			t.Fatalf("release-blocking docs manifest must include traceability normative doc %s", doc)
		}
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
	for _, doc := range runtimeOperationTerminologyDocs() {
		t.Run(doc, func(t *testing.T) {
			scanPublicDocLines(t, doc, func(lineNo int, line string) {
				if staleCurrentRuntimeLockTerminologyPattern.MatchString(line) {
					t.Fatalf("%s:%d describes runtime locks as current release-facing behavior:\n%s", doc, lineNo, line)
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

func TestDocs_MigrationRuntimeStateIncludesLocksIntentsAndGCPlans(t *testing.T) {
	migration := readRepoFile(t, "docs/18_MIGRATION_AND_BACKUP.md")
	runtimePolicy := markdownSectionByHeadingAny(t, "docs/18_MIGRATION_AND_BACKUP.md", migration,
		"## Runtime-state policy (MUST)",
		"## Runtime-State Policy",
	)
	for _, required := range []string{
		".jvs/locks/",
		".jvs/intents/",
		".jvs/gc/*.json",
		"non-portable",
		"jvs doctor --strict --repair-runtime",
	} {
		requireReleaseReadinessText(t, "migration runtime-state policy", runtimePolicy, required)
	}

	migrationFlow := markdownSectionByHeadingAny(t, "docs/18_MIGRATION_AND_BACKUP.md", migration,
		"## Migration flow",
		"## Migration Flow",
	)
	for _, required := range []string{
		"--exclude '.jvs/locks/**'",
		"--exclude '.jvs/intents/**'",
		"--exclude '.jvs/gc/*.json'",
	} {
		requireReleaseReadinessText(t, "migration sync example", migrationFlow, required)
	}

	layout := readRepoFile(t, "docs/01_REPO_LAYOUT_SPEC.md")
	portability := markdownSectionByHeadingAny(t, "docs/01_REPO_LAYOUT_SPEC.md", layout,
		"## Portability classes",
		"## Portability Classes",
	)
	for _, required := range []string{
		".jvs/locks/",
		".jvs/intents/",
		".jvs/gc/*.json",
	} {
		requireReleaseReadinessText(t, "repo layout runtime-state portability class", portability, required)
	}
}

func TestDocs_ReleaseFacingRuntimeStateBoundaryDocs(t *testing.T) {
	for _, doc := range releaseFacingRuntimeStateBoundaryDocs() {
		t.Run(doc, func(t *testing.T) {
			body := readRepoFile(t, doc)
			for _, required := range runtimeStateBoundaryTerms() {
				requireReleaseReadinessText(t, "release-facing runtime-state boundary "+doc, body, required)
			}
		})
	}
}

func TestDocs_ReleaseFacingSyncExamplesExcludeRuntimeState(t *testing.T) {
	for _, doc := range activePublicContractDocs() {
		t.Run(doc, func(t *testing.T) {
			body := releaseFacingClaimBody(t, doc)
			for _, block := range releaseFacingSyncGuidanceBlocks(body) {
				if !syncExampleCopiesJVSMetadata(block.text) {
					continue
				}
				for _, rule := range runtimeStateSyncExcludeRules() {
					if codeBlockContainsAny(block.text, rule.patterns) {
						continue
					}
					t.Fatalf("%s:%d sync example copies JVS metadata without excluding %s runtime state; include one of %v:\n%s",
						doc, block.startLine, rule.name, rule.patterns, block.text)
				}
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

func publicCleanupPlanJSONFields() []string {
	return []string{
		"plan_id",
		"created_at",
		"protected_save_points",
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

func TestDocs_DomainQuickstartsAreReleaseFacingCommandDocs(t *testing.T) {
	docs := publicCommandDocs()
	for _, doc := range domainQuickstartDocs() {
		if !stringSliceContains(docs, doc) {
			t.Fatalf("%s must remain covered by release-facing public command docs contract", doc)
		}
		if stringSliceContains(nonReleaseFacingDocs(), doc) {
			t.Fatalf("%s is a release-facing domain quickstart and must not be classified non-release-facing", doc)
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
		{"workspace", "new"},
		{"workspace", "list"},
		{"workspace", "path"},
		{"workspace", "rename"},
		{"workspace", "remove"},
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

func TestDocs_PublicCommandFieldSetsFindInlineJVSCommands(t *testing.T) {
	line := "Use `jvs save -m baseline`, then `jvs cleanup preview`, then `jvs workspace new exp --from abc123`."
	fieldSets := publicDocCommandFieldSets(line)
	var paths []string
	for _, fields := range fieldSets {
		paths = append(paths, strings.Join(publicDocCommandPath(fields), " "))
	}

	for _, want := range []string{"save", "cleanup preview", "workspace new"} {
		if !stringSliceContains(paths, want) {
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
	runbook := readRepoFile(t, "docs/13_OPERATION_RUNBOOK.md")
	for _, id := range publicRuntimeRepairActionIDs() {
		if !strings.Contains(runbook, "`"+id+"`") {
			t.Fatalf("docs/13_OPERATION_RUNBOOK.md must document public runtime repair action %q", id)
		}
	}

	for _, doc := range []string{"docs/13_OPERATION_RUNBOOK.md"} {
		t.Run(doc, func(t *testing.T) {
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

func TestDocs_ReleaseEvidenceDoesNotMixCandidateAndFinalSemantics(t *testing.T) {
	latestHeading := latestChangelogHeading(t)
	entry := releaseEvidenceEntry(t, readRepoFile(t, "docs/RELEASE_EVIDENCE.md"), latestHeading)
	if releaseEvidenceClaimsFinalTaggedRelease(entry) {
		if strings.Contains(strings.ToLower(entry), "candidate") ||
			strings.Contains(strings.ToLower(entry), "pending final") ||
			strings.Contains(strings.ToLower(entry), "not published") {
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
		".sig",
		".pem",
	} {
		if !strings.Contains(workflowNotes, required) {
			t.Fatalf("generated release notes must include %q", required)
		}
	}

	for _, required := range []string{
		"remote push/pull",
		"signing commands",
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
		"partial checkpoint contracts",
	)

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

func publicRuntimeRepairActionIDs() []string {
	return []string{
		"clean_locks",
		"clean_runtime_tmp",
		"clean_runtime_operations",
	}
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
		"docs/user/concepts.md",
		"docs/user/commands.md",
		"docs/user/examples.md",
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
		"docs/agent_sandbox_quickstart.md",
		"docs/etl_pipeline_quickstart.md",
		"docs/game_dev_quickstart.md",
	}
}

func domainQuickstartDocs() []string {
	return []string{
		"docs/agent_sandbox_quickstart.md",
		"docs/etl_pipeline_quickstart.md",
		"docs/game_dev_quickstart.md",
	}
}

func runtimeOperationTerminologyDocs() []string {
	return []string{
		"docs/02_CLI_SPEC.md",
		"docs/11_CONFORMANCE_TEST_PLAN.md",
		"docs/PRODUCT_PLAN.md",
	}
}

func runtimeStateBoundaryTerms() []string {
	return []string{
		".jvs/locks/",
		".jvs/intents/",
		".jvs/gc/*.json",
	}
}

func runtimeStateSyncExcludeRules() []runtimeStateSyncExcludeRule {
	return []runtimeStateSyncExcludeRule{
		{
			name:     "mutation locks",
			patterns: []string{"--exclude '.jvs/locks/**'", `--exclude ".jvs/locks/**"`},
		},
		{
			name:     "operation records",
			patterns: []string{"--exclude '.jvs/intents/**'", `--exclude ".jvs/intents/**"`},
		},
		{
			name:     "active GC plans",
			patterns: []string{"--exclude '.jvs/gc/*.json'", `--exclude ".jvs/gc/*.json"`},
		},
	}
}

func releaseFacingRuntimeStateBoundaryDocs() []string {
	return []string{
		"SECURITY.md",
		"docs/01_REPO_LAYOUT_SPEC.md",
		"docs/10_THREAT_MODEL.md",
		"docs/13_OPERATION_RUNBOOK.md",
		"docs/18_MIGRATION_AND_BACKUP.md",
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
		"docs/TARGET_USERS.md",
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
	}
}

func activeNonReleaseFacingReferenceDocs() []string {
	return []string{
		"docs/design/README.md",
		"docs/ops/README.md",
		"docs/release/README.md",
	}
}

func nonReleaseFacingDocs() []string {
	docs := append([]string{}, archivedNonReleaseFacingDocs()...)
	for _, doc := range activeNonReleaseFacingDesignDocs() {
		docs = appendUniqueString(docs, doc)
	}
	for _, doc := range activeNonReleaseFacingReferenceDocs() {
		docs = appendUniqueString(docs, doc)
	}
	return docs
}

func activePublicContractDocs() []string {
	var docs []string
	for _, doc := range releaseBlockingContractDocs() {
		if stringSliceContains(nonReleaseFacingDocs(), doc) {
			continue
		}
		docs = appendUniqueString(docs, doc)
	}
	return docs
}

func publicCommandDocs() []string {
	return releaseBlockingContractDocs()
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
	case "workspace", "view", "recovery", "cleanup":
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
		"workspace remove",
		"cleanup preview",
		"cleanup run",
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
		"detached",
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
		if strings.HasPrefix(trimmed, "- ") && strings.HasSuffix(trimmed, ":") {
			label := strings.ToLower(strings.TrimSuffix(strings.TrimPrefix(trimmed, "- "), ":"))
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

func codeBlockContainsAny(block string, patterns []string) bool {
	for _, pattern := range patterns {
		if strings.Contains(block, pattern) {
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
		if internalStorageSectionLevel > 0 && lineNamesInternalStoragePath(line) {
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

func releaseEvidenceEntry(t *testing.T, ledger, heading string) string {
	t.Helper()
	return markdownSectionByHeading(t, "docs/RELEASE_EVIDENCE.md", ledger, heading)
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

func stringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func appendUniqueString(values []string, value string) []string {
	if stringSliceContains(values, value) {
		return values
	}
	return append(values, value)
}

func repoFile(t *testing.T, parts ...string) string {
	t.Helper()
	segments := append([]string{"..", ".."}, parts...)
	return filepath.Join(segments...)
}
