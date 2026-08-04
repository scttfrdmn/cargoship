// Package audit holds repository-structure tests: invariants about the shape of
// the codebase that no single package can assert about itself.
package audit

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// tarLoopAllowlist is every non-test site that constructs a tar.Reader, with the
// reason each is safe. Adding a site without updating this list fails the test.
//
// Why an inventory rather than a behavioral test: #311 was a *duplicate*
// extraction loop. `download` grew its own tar loop that joined the untrusted
// header name straight onto the output directory — the third live copy of the
// #282 traversal. No test of the shared extractor could have caught it, because
// the vulnerable code never called the shared extractor. What made it invisible
// was that a new loop could appear without anyone re-asking the containment
// question.
//
// So the invariant is: a tar-reading site is either (a) the shared extractor
// that sanitizes, (b) a reader that never writes to disk and therefore cannot
// traverse, or (c) newly added and unreviewed — which is what this test reports.
var tarLoopAllowlist = map[string]string{
	"pkg/manifest/batch_restore.go":              "the shared SelectiveExtractor; sanitizes header names and asserts containment (see TestBatchRestore_ChunkedTraversalIsContained)",
	"pkg/extraction/extractor.go":                "checks containment in extractFile, and validates symlink targets in createSymlink",
	"pkg/manifest/deep_verify.go":                "hashes entries in memory via io.CopyN; never writes to disk",
	"pkg/pipeline/rebalance.go":                  "reads entries into memory for re-chunking; never writes to disk",
	"examples/library-usage/03-manifest/main.go": "example program, not shipped in the binary; reads entries in memory",
}

// TestTarReaderSitesAreReviewed pins the set of files that read tar archives.
//
// A new entry is not necessarily a bug — but it does mean a reviewer must decide
// whether that loop writes to disk and, if so, whether it contains its output.
// #311 shipped because that question was never asked for a fourth copy.
func TestTarReaderSitesAreReviewed(t *testing.T) {
	root := repoRoot(t)

	// git grep so the search respects .gitignore and only covers tracked source.
	cmd := exec.Command("git", "grep", "-l", "tar.NewReader", "--", "*.go")
	cmd.Dir = root
	cmd.Env = auditGitEnv() // see auditGitEnv: GIT_DIR would override cmd.Dir
	out, err := cmd.Output()
	require.NoError(t, err, "git grep for tar.NewReader failed")

	var unreviewed []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		file := strings.TrimSpace(line)
		if file == "" || strings.HasSuffix(file, "_test.go") {
			continue // tests construct tar readers to build fixtures
		}
		if _, ok := tarLoopAllowlist[file]; !ok {
			unreviewed = append(unreviewed, file)
		}
	}
	sort.Strings(unreviewed)

	require.Empty(t, unreviewed,
		"new tar.NewReader site(s) not in tarLoopAllowlist: %v\n\n"+
			"A tar loop that joins an untrusted header.Name onto an output directory is a "+
			"path traversal (#282, #311). Either route the extraction through "+
			"manifest.SelectiveExtractor, or -- if this loop reads into memory and never "+
			"writes to disk -- add it to tarLoopAllowlist with that reason.",
		unreviewed)
}

// TestTarLoopAllowlistHasNoStaleEntries keeps the allowlist honest in the other
// direction: an entry for a file that no longer reads tar (or no longer exists)
// is dead documentation that makes the guard look broader than it is.
func TestTarLoopAllowlistHasNoStaleEntries(t *testing.T) {
	root := repoRoot(t)

	for file, reason := range tarLoopAllowlist {
		require.NotEmpty(t, reason, "%s: allowlist entries must state why the site is safe", file)

		path := filepath.Join(root, file)
		data, err := os.ReadFile(path) // #nosec G304 -- path comes from the in-repo allowlist
		require.NoError(t, err, "allowlisted file %s does not exist; remove the entry", file)
		require.Contains(t, string(data), "tar.NewReader",
			"allowlisted file %s no longer reads tar archives; remove the entry", file)
	}
}

// repoRoot returns the repository root, resolved from git rather than a relative
// path so the test does not silently pass from an unexpected directory.
func repoRoot(t *testing.T) string {
	t.Helper()

	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Env = auditGitEnv()
	out, err := cmd.Output()
	require.NoError(t, err, "not in a git repository")
	return strings.TrimSpace(string(out))
}

// auditGitEnv strips git's repository-location overrides from the inherited
// environment. Git exports GIT_DIR and friends to processes it spawns, and the
// pre-commit hook runs `go test` — so without this, `rev-parse --show-toplevel`
// and the `git grep` below resolve against whatever repo git had exported rather
// than the tree under test. That produced two failures whose messages pointed
// nowhere near the cause ("git grep for tar.NewReader failed", and an
// allowlisted file reported as nonexistent at a path with internal/audit
// wrongly prefixed).
func auditGitEnv() []string {
	env := os.Environ()
	out := make([]string, 0, len(env))
	for _, kv := range env {
		switch strings.SplitN(kv, "=", 2)[0] {
		case "GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY",
			"GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_COMMON_DIR", "GIT_PREFIX",
			"GIT_CEILING_DIRECTORIES":
			continue
		}
		out = append(out, kv)
	}
	return out
}
