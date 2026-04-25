//go:build conformance

package conformance

import (
	"bufio"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var checkpointListBareJQSelector = regexp.MustCompile(`jq\s+-r\s+['"]\.\[(?:\]|[0-9]+)`)
var traceabilityDocRefPattern = regexp.MustCompile("`((?:README\\.md|docs/[^`]+\\.md))`")
var markdownDocLinkPattern = regexp.MustCompile(`\]\(([^)#]+\.md)(?:#[^)]+)?\)`)
var stalePublicReleaseVocabularyPattern = regexp.MustCompile(`\bv7\.(?:[0-9]+|x)\b`)
var numberedConformanceTestRef = regexp.MustCompile(`\b[Tt]ests?\s+\d`)
var markdownBulletFieldPattern = regexp.MustCompile("^\\s*-\\s+`([A-Za-z0-9_]+)`")
var backtickedFieldPattern = regexp.MustCompile("`([A-Za-z0-9_]+)`")
var jsonTagFieldPattern = regexp.MustCompile("json:\"([A-Za-z0-9_]+)(?:,[^\"]*)?\"")

func TestDocs_PublicTerminologyContract(t *testing.T) {
	for _, doc := range activePublicContractDocs() {
		t.Run(doc, func(t *testing.T) {
			path := repoFile(t, doc)
			file, err := os.Open(path)
			if err != nil {
				t.Fatalf("open %s: %v", doc, err)
			}
			defer file.Close()

			scanner := bufio.NewScanner(file)
			lineNo := 0
			internalSectionLevel := 0
			for scanner.Scan() {
				lineNo++
				line := scanner.Text()
				if level, ok := markdownHeadingLevel(line); ok {
					if internalSectionLevel > 0 && level <= internalSectionLevel {
						internalSectionLevel = 0
					}
					if markedInternalCompatibilityHeading(line) {
						internalSectionLevel = level
					}
				}
				if internalSectionLevel > 0 || allowedPublicDocCompatibilityLine(doc, line) {
					continue
				}
				normalizedLine := strings.ToLower(line)
				for _, forbidden := range publicDocForbiddenTerms() {
					if strings.Contains(normalizedLine, strings.ToLower(forbidden)) {
						t.Fatalf("%s:%d exposes legacy public term %q:\n%s", doc, lineNo, forbidden, line)
					}
				}
			}
			if err := scanner.Err(); err != nil {
				t.Fatalf("scan %s: %v", doc, err)
			}
		})
	}
}

func TestConformancePublicProfileUsesStableCommands(t *testing.T) {
	for _, dir := range []string{"test/conformance", "test/regression"} {
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

func TestDocs_CheckpointListJSONExamplesUseEnvelope(t *testing.T) {
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
				if !strings.Contains(line, "jvs checkpoint list --json") || !strings.Contains(line, "jq") {
					continue
				}
				if checkpointListBareJQSelector.MatchString(line) {
					t.Fatalf("%s:%d treats checkpoint list JSON as a top-level array; select from .data instead:\n%s", doc, lineNo, line)
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
		if stringSliceContains(archivedNonReleaseFacingDocs(), link.target) {
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
		if stringSliceContains(archivedNonReleaseFacingDocs(), doc) {
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

func TestDocs_GCPlanJSONFieldsMatchPublicFacade(t *testing.T) {
	fields := jsonFieldsForStruct(t, "internal/cli/public_json.go", "publicGCPlan")
	want := publicGCPlanJSONFields()
	assertSameStringSet(t, "internal/cli.publicGCPlan JSON fields", fields, want)

	libraryFields := jsonFieldsForStruct(t, "pkg/jvs/client.go", "GCPlan")
	assertSameStringSet(t, "pkg/jvs.GCPlan JSON fields", libraryFields, want)

	for _, docSpec := range []struct {
		doc     string
		fields  func(t *testing.T, doc, section string) []string
		section string
	}{
		{
			doc:     "docs/02_CLI_SPEC.md",
			section: "### `jvs gc plan [--json]`",
			fields: func(t *testing.T, doc, section string) []string {
				return markdownBulletFieldsAfterLabel(t, doc, section, "Required `data` fields:")
			},
		},
		{
			doc:     "docs/08_GC_SPEC.md",
			section: "## `jvs gc plan` (MUST)",
			fields: func(t *testing.T, doc, section string) []string {
				return markdownBulletFieldsAfterLabel(t, doc, section, "JSON `data` includes:")
			},
		},
		{
			doc:     "docs/API_DOCUMENTATION.md",
			section: "### Garbage Collection",
			fields: func(t *testing.T, doc, section string) []string {
				proseFields := backtickedFieldsAfterLabel(t, doc, section, "Public JSON fields:")
				assertSameStringSet(t, doc+" public GC JSON field prose", proseFields, publicGCPlanJSONFields())
				return jsonTagFieldsForDocumentedType(t, doc, section, "GCPlan")
			},
		},
	} {
		t.Run(docSpec.doc, func(t *testing.T) {
			body := readRepoFile(t, docSpec.doc)
			section := markdownSectionByHeading(t, docSpec.doc, body, docSpec.section)
			docFields := docSpec.fields(t, docSpec.doc, section)
			assertSameStringSet(t, docSpec.doc+" public GC JSON fields", docFields, want)
			for _, field := range forbiddenPublicGCJSONFieldNames() {
				if strings.Contains(section, "`"+field+"`") {
					t.Fatalf("%s public GC section documents non-public field %q", docSpec.doc, field)
				}
			}
		})
	}
}

func publicGCPlanJSONFields() []string {
	return []string{
		"plan_id",
		"created_at",
		"protected_checkpoints",
		"protected_by_lineage",
		"candidate_count",
		"to_delete",
		"deletable_bytes_estimate",
	}
}

func forbiddenPublicGCJSONFieldNames() []string {
	return []string{
		"delete_checkpoints",
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
	if !strings.Contains(doc, "`pkg/model`") || !strings.Contains(doc, "internal compatibility") {
		t.Fatalf("docs/API_DOCUMENTATION.md must mark pkg/model as internal compatibility, not a retention/pin public surface")
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

func TestDocs_APIPublicExamplesUseStableFacade(t *testing.T) {
	body := readRepoFile(t, "docs/API_DOCUMENTATION.md")
	for _, heading := range []string{"## Quick Example", "## Integration Example"} {
		t.Run(heading, func(t *testing.T) {
			section := markdownSectionByHeading(t, "docs/API_DOCUMENTATION.md", body, heading)
			for _, required := range []string{
				`"github.com/jvs-project/jvs/pkg/jvs"`,
				"jvs.OpenOrInit(",
				".Snapshot(ctx, jvs.SnapshotOptions{",
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
		`"github.com/jvs-project/jvs/pkg/model"`,
		`"github.com/jvs-project/jvs/pkg/fsutil"`,
		`"github.com/jvs-project/jvs/pkg/jsonutil"`,
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
		`"github.com/jvs-project/jvs/pkg/model"`,
		".jvs/descriptors",
	}
}

func TestDocs_PublicCommandExamplesUseStableCommands(t *testing.T) {
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

				fields := publicDocCommandFields(line)
				if len(fields) == 0 {
					return
				}
				command := publicDocCommandName(fields)
				if command == "" {
					return
				}
				if !stablePublicCommand(command) {
					t.Fatalf("%s:%d documents unsupported/non-v0-stable command %q:\n%s", doc, lineNo, command, line)
				}
			})
		})
	}
}

func TestDocs_PublicLibraryGCHidesV0RetentionSurface(t *testing.T) {
	fset := token.NewFileSet()
	path := repoFile(t, "pkg/jvs/client.go")
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse pkg/jvs/client.go: %v", err)
	}

	forbiddenGCOptionFields := map[string]bool{
		"KeepMinSnapshots": true,
		"KeepMinAge":       true,
	}
	ast.Inspect(file, func(node ast.Node) bool {
		typeSpec, ok := node.(*ast.TypeSpec)
		if !ok || typeSpec.Name.Name != "GCOptions" {
			return true
		}
		structType, ok := typeSpec.Type.(*ast.StructType)
		if !ok {
			t.Fatalf("pkg/jvs.GCOptions must remain a struct")
		}
		for _, field := range structType.Fields.List {
			for _, name := range field.Names {
				if forbiddenGCOptionFields[name.Name] {
					pos := fset.Position(name.Pos())
					t.Fatalf("%s:%d exposes v0 retention knob %s on public pkg/jvs.GCOptions", path, pos.Line, name.Name)
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
		t.Fatalf("%s:%d public pkg/jvs.Client.GC must not route through retention policy planning", path, pos.Line)
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

func TestDocs_VerifyAllContractIsCheckpointScoped(t *testing.T) {
	securityModel := readRepoFile(t, "docs/09_SECURITY_MODEL.md")
	if !strings.Contains(securityModel, "`jvs doctor --strict` MUST validate the audit hash chain") {
		t.Fatalf("security model must assign audit chain validation to doctor --strict")
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
					t.Fatalf("%s:%d promises audit verification through verify --all; audit chain belongs to doctor --strict:\n%s", doc, lineNo, line)
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
	} {
		if !strings.Contains(entry, heading) {
			t.Fatalf("latest changelog entry must include %q", heading)
		}
	}
}

func TestDocs_PerformanceConstantClaimsAreEngineScoped(t *testing.T) {
	for _, doc := range []string{"docs/PERFORMANCE.md", "docs/PERFORMANCE_RESULTS.md", "docs/BENCHMARKS.md"} {
		t.Run(doc, func(t *testing.T) {
			scanPublicDocLines(t, doc, func(lineNo int, line string) {
				lower := strings.ToLower(line)
				if !strings.Contains(lower, "o(1)") &&
					!strings.Contains(lower, "constant-time") &&
					!strings.Contains(lower, "constant overhead") {
					return
				}
				if strings.Contains(lower, "juicefs") ||
					strings.Contains(lower, "reflink") ||
					strings.Contains(lower, "engine") ||
					strings.Contains(lower, "filesystem") ||
					strings.Contains(lower, "metadata") ||
					strings.Contains(lower, "supported") ||
					strings.Contains(lower, "fallback") {
					return
				}
				t.Fatalf("%s:%d has an unscoped constant-time/O(1) claim:\n%s", doc, lineNo, line)
			})
		})
	}
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
		"docs/TEMPLATES.md",
	}
}

func activePublicContractDocs() []string {
	var docs []string
	for _, doc := range releaseBlockingContractDocs() {
		if stringSliceContains(archivedNonReleaseFacingDocs(), doc) {
			continue
		}
		docs = appendUniqueString(docs, doc)
	}
	return docs
}

func publicCommandDocs() []string {
	return releaseBlockingContractDocs()
}

func unsupportedPublicDocCommandFragments() []string {
	return []string{
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

func lineContainsUnsupportedPublicDocCommandFragment(line, fragment string) bool {
	pattern := regexp.MustCompile(`(?i)(^|[^A-Za-z0-9_-])` + regexp.QuoteMeta(fragment) + `([^A-Za-z0-9_-]|$)`)
	return pattern.MatchString(line)
}

func publicDocCommandFields(line string) []string {
	trimmed := strings.TrimSpace(line)
	trimmed = strings.TrimPrefix(trimmed, "$ ")
	trimmed = strings.TrimPrefix(trimmed, "> ")
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

func publicDocCommandName(fields []string) string {
	if len(fields) == 0 || fields[0] != "jvs" {
		return ""
	}
	for i := 1; i < len(fields); i++ {
		candidate := strings.Trim(fields[i], "`.,;:)")
		candidate = strings.Trim(candidate, "[]")
		if strings.HasPrefix(candidate, "-") ||
			(strings.HasPrefix(candidate, "<") && strings.HasSuffix(candidate, ">")) ||
			candidate == "..." {
			continue
		}
		return candidate
	}
	return ""
}

func stablePublicCommand(command string) bool {
	switch command {
	case "init", "import", "clone", "capability", "info", "status", "checkpoint",
		"diff", "restore", "fork", "workspace", "verify", "doctor", "gc",
		"completion", "help":
		return true
	default:
		return false
	}
}

func publicDocForbiddenTerms() []string {
	return []string{
		"jvs snapshot",
		"jvs worktree",
		"jvs history",
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

func markedInternalCompatibilityHeading(line string) bool {
	lower := strings.ToLower(line)
	for _, marker := range []string{"legacy", "internal", "on-disk", "on disk", "compatibility"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func allowedPublicDocCompatibilityLine(doc, line string) bool {
	lower := strings.ToLower(line)
	for _, marker := range []string{".jvs/snapshots", ".jvs/worktrees", "repo/worktrees"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	if doc == "docs/BENCHMARKS.md" {
		for _, marker := range []string{"`benchmark", "internal/snapshot", "internal/restore", "internal/gc"} {
			if strings.Contains(lower, marker) {
				return true
			}
		}
	}
	for _, marker := range []string{
		"docs/03_worktree_spec.md",
	} {
		if strings.Contains(lower, marker) {
			return true
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

	legacyCommands := map[string]string{
		"snapshot": "checkpoint",
		"history":  "checkpoint list",
		"worktree": "workspace or fork",
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
			if replacement, ok := legacyCommands[value]; ok {
				pos := fset.Position(lit.Pos())
				t.Fatalf("%s:%d public-profile test invokes legacy command %q; use %s", path, pos.Line, value, replacement)
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
	internalSectionLevel := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		if level, ok := markdownHeadingLevel(line); ok {
			if internalSectionLevel > 0 && level <= internalSectionLevel {
				internalSectionLevel = 0
			}
			if markedInternalCompatibilityHeading(line) {
				internalSectionLevel = level
			}
		}
		if internalSectionLevel > 0 || allowedPublicDocCompatibilityLine(doc, line) {
			continue
		}
		visit(lineNo, line)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan %s: %v", doc, err)
	}
}

func readRepoFile(t *testing.T, parts ...string) string {
	t.Helper()
	data, err := os.ReadFile(repoFile(t, parts...))
	if err != nil {
		t.Fatalf("read %s: %v", filepath.Join(parts...), err)
	}
	return string(data)
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
