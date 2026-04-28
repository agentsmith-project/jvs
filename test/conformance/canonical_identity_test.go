//go:build conformance

package conformance

import (
	"strings"
	"testing"
)

const canonicalRepositorySlug = "agentsmith-project/jvs"

func canonicalRepositoryURL() string {
	return "https://github.com/" + canonicalRepositorySlug
}

func canonicalGoModulePath() string {
	return "github.com/" + canonicalRepositorySlug
}

func legacyIdentityFragments() []string {
	legacyProject := "jvs-" + "project"
	return []string{
		"github.com/" + legacyProject + "/jvs",
		legacyProject + "/jvs",
		legacyProject + ".org",
	}
}

func TestDocs_CanonicalIdentityRequiredInReleaseFacingDocs(t *testing.T) {
	for _, tc := range []struct {
		doc      string
		required []string
	}{
		{
			doc: "README.md",
			required: []string{
				"https://img.shields.io/github/v/release/" + canonicalRepositorySlug,
				"git clone " + canonicalRepositoryURL() + ".git",
				"go install " + canonicalGoModulePath() + "/cmd/jvs@<VERSION>",
			},
		},
		{
			doc: "SECURITY.md",
			required: []string{
				canonicalRepositoryURL() + "/security/advisories",
				"GitHub Security Advisory",
				"configured security contact",
			},
		},
		{
			doc: "docs/API_DOCUMENTATION.md",
			required: []string{
				`"` + canonicalGoModulePath() + `/pkg/jvs"`,
			},
		},
		{
			doc: "docs/SIGNING.md",
			required: []string{
				canonicalRepositoryURL() + "/releases/download/vX.Y.Z/jvs-linux-amd64",
				"--certificate-identity=https://github.com/" + canonicalRepositorySlug + "/.github/workflows/ci.yml@<workflow-ref>",
			},
		},
		{
			doc: "docs/RELEASE_EVIDENCE.md",
			required: []string{
				canonicalRepositoryURL() + "/actions/runs/<run_id>",
				canonicalRepositoryURL() + "/releases/tag/v0.4.0",
				"https://github.com/" + canonicalRepositorySlug + "/.github/workflows/ci.yml@<workflow-ref>",
			},
		},
	} {
		t.Run(tc.doc, func(t *testing.T) {
			body := readRepoFile(t, tc.doc)
			for _, required := range tc.required {
				if !strings.Contains(body, required) {
					t.Fatalf("%s missing canonical identity text %q", tc.doc, required)
				}
			}
		})
	}
}

func TestDocs_ActiveChangelogUsesCanonicalIdentity(t *testing.T) {
	entry := firstChangelogEntry(readRepoFile(t, "docs/99_CHANGELOG.md"))
	for _, required := range []string{
		canonicalGoModulePath(),
		canonicalRepositoryURL() + "/releases/tag/v0.4.0",
	} {
		if !strings.Contains(entry, required) {
			t.Fatalf("latest changelog entry missing canonical identity text %q", required)
		}
	}
	for _, fragment := range legacyIdentityFragments() {
		if strings.Contains(entry, fragment) {
			t.Fatalf("latest changelog entry contains legacy release-facing identity fragment %q", fragment)
		}
	}
}

func TestDocs_ReleaseFacingDocsDoNotContainLegacyIdentity(t *testing.T) {
	for _, doc := range activePublicContractDocs() {
		t.Run(doc, func(t *testing.T) {
			body := readRepoFile(t, doc)
			if doc == "docs/99_CHANGELOG.md" {
				body = firstChangelogEntry(body)
			}
			for _, fragment := range legacyIdentityFragments() {
				if strings.Contains(body, fragment) {
					t.Fatalf("%s contains legacy release-facing identity fragment %q", doc, fragment)
				}
			}
		})
	}
}
