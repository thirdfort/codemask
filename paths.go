package codemask

import (
	"path"
	"regexp"
	"slices"
	"strings"
)

// Tags are generic hints about what a path is. They are worked out from the
// path alone and say nothing about the contents.
const (
	TagTest      = "test"
	TagData      = "data"
	TagManifest  = "manifest"
	TagLockfile  = "lockfile"
	TagGenerated = "generated"
	TagMigration = "migration"
	TagCI        = "ci"
	TagIaC       = "iac"
	TagDocs      = "docs"
	TagStyles    = "styles"
	TagUI        = "ui"
)

var lockfiles = []string{
	"go.sum", "package-lock.json", "yarn.lock", "pnpm-lock.yaml", "bun.lock", "bun.lockb",
	"uv.lock", "poetry.lock", "Pipfile.lock", "Cargo.lock", "Gemfile.lock", "composer.lock",
	".terraform.lock.hcl",
}

var manifests = []string{
	"go.mod", "go.work", "package.json", "pyproject.toml", "Pipfile", "Cargo.toml",
	"Gemfile", "composer.json", "Chart.yaml",
}

var dataDirs = []string{"testdata", "fixtures", "__fixtures__", "seed", "seeds", "__snapshots__", "golden"}

var dataExts = []string{".csv", ".tsv", ".snap", ".golden", ".ndjson", ".jsonl", ".parquet", ".xlsx", ".xls"}

var proseExts = []string{".md", ".mdx", ".markdown", ".rst", ".txt", ".adoc"}

var generatedMarkers = []string{
	".pb.go", ".pb.gw.go", "_pb2.py", "_pb2_grpc.py", ".gen.go", "_gen.go", "zz_generated",
	".generated.", "/generated/", "__generated__/", ".min.js", ".min.css",
}

// pathTags lists the tags that apply to a path, in a fixed order.
func pathTags(p string) []string {
	base, ext := path.Base(p), strings.ToLower(path.Ext(p))
	dirs := strings.Split(path.Dir(p), "/")
	has := func(names ...string) bool {
		return slices.ContainsFunc(dirs, func(d string) bool { return slices.Contains(names, d) })
	}
	anyIn := func(s string, subs []string) bool {
		return slices.ContainsFunc(subs, func(sub string) bool { return strings.Contains(s, sub) })
	}

	var tags []string
	add := func(ok bool, tag string) {
		if ok {
			tags = append(tags, tag)
		}
	}
	add(strings.HasSuffix(base, "_test.go") || strings.Contains(base, ".test.") ||
		strings.Contains(base, ".spec.") || strings.HasPrefix(base, "test_") ||
		strings.HasSuffix(base, "_test.py") || base == "conftest.py" ||
		has("__tests__", "test", "tests", "e2e"), TagTest)
	add(has(dataDirs...) || slices.Contains(dataExts, ext), TagData)
	add(slices.Contains(manifests, base) || strings.HasPrefix(base, "requirements") && ext == ".txt", TagManifest)
	add(slices.Contains(lockfiles, base), TagLockfile)
	add(anyIn("/"+p, generatedMarkers), TagGenerated)
	add(has("migrations", "migration", "migrate"), TagMigration)
	add(strings.HasPrefix(p, ".github/workflows/") || base == ".gitlab-ci.yml" || has(".circleci", ".buildkite") ||
		base == "Jenkinsfile" || base == "cloudbuild.yaml", TagCI)
	add(ext == ".tf" || ext == ".tfvars" || ext == ".hcl" && base != ".terraform.lock.hcl" ||
		base == "Chart.yaml" || base == "kustomization.yaml" || isDockerfile(base) ||
		strings.HasPrefix(base, "docker-compose") || has("helm", "charts", "k8s", "kustomize", "deploy", "infra"), TagIaC)
	add(slices.Contains(proseExts, ext) || has("docs") || base == "LICENSE" || base == "CHANGELOG", TagDocs)
	add(slices.Contains([]string{".css", ".scss", ".sass", ".less"}, ext), TagStyles)
	add(slices.Contains([]string{".tsx", ".jsx", ".vue", ".svelte"}, ext), TagUI)
	return tags
}

// countsReason says why a file must be reduced to counts before any lexing,
// or "" if it may be masked.
func countsReason(p string, tags []string) string {
	switch {
	case slices.Contains(tags, TagGenerated):
		return ReasonGenerated
	case slices.Contains(tags, TagLockfile):
		return ReasonLockfile
	case slices.Contains(tags, TagData):
		return ReasonData
	case slices.Contains(proseExts, strings.ToLower(path.Ext(p))):
		return ReasonProse
	default:
		return ""
	}
}

func isDockerfile(base string) bool {
	return base == "Dockerfile" || strings.HasPrefix(base, "Dockerfile.") || strings.HasSuffix(base, ".dockerfile")
}

// piiPattern is the last check before anything leaves: email addresses, UK and
// international phone numbers, UK postcodes, dates, UK National Insurance
// numbers, card numbers and runs of 8 or more digits. Masking should already
// have removed all of these. A match means it did not, and the file drops to
// counts.
var piiPattern = regexp.MustCompile(strings.Join([]string{
	`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`,
	`\+?44\s?7\d{3}\s?\d{6}`,
	`\b07\d{9}\b`,
	`\+\d{10,14}`,
	`\b[A-Z]{1,2}\d[A-Z\d]?\s?\d[A-Z]{2}\b`,
	`\b(19|20)\d{2}-[01]\d-[0-3]\d\b`,
	`\b[0-3]?\d/[01]?\d/(19|20)\d{2}\b`,
	`\b[A-CEGHJ-PR-TW-Z]{2}\d{6}[A-D]\b`,
	`\b(?:\d{4}[ -]){3}\d{4}\b`,
	`\b\d{8,}\b`,
}, "|"))

// redactPath replaces any path segment that looks like PII. Paths are sent as
// they are, so testdata/jane@example.com.json must not be.
func redactPath(p string) string {
	if !piiPattern.MatchString(p) {
		return p
	}
	segs := strings.Split(p, "/")
	for i, s := range segs {
		if !piiPattern.MatchString(s) {
			continue
		}
		ext := ""
		if i == len(segs)-1 {
			ext = path.Ext(s)
			if piiPattern.MatchString(ext) {
				ext = ""
			}
		}
		segs[i] = "<redacted>" + ext
	}
	return strings.Join(segs, "/")
}
