package manifest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// skipIfNoGit skips the test when git is not on PATH.
func skipIfNoGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found on PATH")
	}
}

// gitEnv returns environment variables that prevent test git commands from
// reading the user's global or system config, and supply mandatory identity
// fields required by git commit.
//
// The inherited environment is FILTERED, not just appended to. Git exports
// GIT_DIR, GIT_WORK_TREE and GIT_INDEX_FILE to the processes it spawns, so when
// this suite runs from inside a hook (the pre-commit hook runs `go test`), every
// `git` call below would ignore cmd.Dir and operate on the REAL repository
// instead of the test's temp dir. That is not a fussy hygiene point: it made
// initRepo's `git commit` land a stray "initial commit" on the checked-out
// branch and flipped the real repo's core.bare, while the tests reported
// confusing assertion failures about the wrong branch and a remote URL that
// "shouldn't exist on a fresh repo".
func gitEnv() []string {
	// Drop anything git might have exported that would redirect these commands
	// at another repository. cmd.Dir is the only thing that should decide which
	// repo is touched.
	inherited := os.Environ()
	filtered := make([]string, 0, len(inherited)+7)
	for _, kv := range inherited {
		switch strings.SplitN(kv, "=", 2)[0] {
		case "GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY",
			"GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_COMMON_DIR", "GIT_PREFIX",
			"GIT_CEILING_DIRECTORIES":
			continue
		}
		filtered = append(filtered, kv)
	}

	return append(filtered,
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=CargoShip Test",
		"GIT_AUTHOR_EMAIL=test@cargoship.test",
		"GIT_COMMITTER_NAME=CargoShip Test",
		"GIT_COMMITTER_EMAIL=test@cargoship.test",
		"GIT_TERMINAL_PROMPT=0",
	)
}

// mustGit runs a git command inside dir, failing the test on error.
func mustGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = gitEnv()
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v failed: %s", args, out)
	return strings.TrimSpace(string(out))
}

// commitFile writes name with the given content inside dir and commits it,
// returning the new HEAD SHA. Used when a test needs two repos whose commits
// are guaranteed to differ.
func commitFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
	mustGit(t, dir, "add", ".")
	mustGit(t, dir, "commit", "-m", "add "+name)
	return mustGit(t, dir, "rev-parse", "HEAD")
}

// initRepo creates a new git repo in dir with one committed file and returns
// the HEAD commit SHA.
func initRepo(t *testing.T, dir string) string {
	t.Helper()
	mustGit(t, dir, "init", "-b", "main")
	mustGit(t, dir, "config", "user.email", "test@cargoship.test")
	mustGit(t, dir, "config", "user.name", "CargoShip Test")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("test"), 0o644))
	mustGit(t, dir, "add", ".")
	mustGit(t, dir, "commit", "-m", "initial commit")
	return mustGit(t, dir, "rev-parse", "HEAD")
}

// --- ExtractGitMetadata ---

func TestExtractGitMetadata_InsideRepo(t *testing.T) {
	skipIfNoGit(t)
	dir := t.TempDir()
	commit := initRepo(t, dir)

	meta, err := ExtractGitMetadata(dir)
	require.NoError(t, err)
	require.NotNil(t, meta)

	assert.Equal(t, commit, meta.Commit, "commit SHA must match HEAD")
	assert.Equal(t, "main", meta.Branch)
	assert.Empty(t, meta.Tag, "no tag on fresh repo")
	assert.Empty(t, meta.Remote, "no remote on fresh repo")
	assert.False(t, meta.Dirty, "clean working tree")
}

func TestExtractGitMetadata_OutsideRepo(t *testing.T) {
	skipIfNoGit(t)
	dir := t.TempDir() // plain directory, not a git repo

	meta, err := ExtractGitMetadata(dir)
	require.NoError(t, err, "non-repo must not produce an error")
	require.NotNil(t, meta)

	assert.Empty(t, meta.Commit)
	assert.Empty(t, meta.Branch)
	assert.Empty(t, meta.Tag)
	assert.Empty(t, meta.Remote)
	assert.False(t, meta.Dirty)
}

func TestExtractGitMetadata_NonexistentPath(t *testing.T) {
	skipIfNoGit(t)

	meta, err := ExtractGitMetadata("/nonexistent/path/that/cannot/exist")
	require.NoError(t, err)
	require.NotNil(t, meta)
	assert.Empty(t, meta.Commit)
}

func TestExtractGitMetadata_DetachedHEAD(t *testing.T) {
	skipIfNoGit(t)
	dir := t.TempDir()
	commit := initRepo(t, dir)

	// Detach HEAD by checking out the commit SHA directly.
	mustGit(t, dir, "checkout", "--detach", commit)

	meta, err := ExtractGitMetadata(dir)
	require.NoError(t, err)

	assert.Equal(t, commit, meta.Commit, "commit must still be populated in detached state")
	assert.Empty(t, meta.Branch, "branch must be empty in detached-HEAD state")
	assert.False(t, meta.Dirty)
}

func TestExtractGitMetadata_TaggedCommit(t *testing.T) {
	skipIfNoGit(t)
	dir := t.TempDir()
	initRepo(t, dir)

	// Create an annotated tag at HEAD.
	mustGit(t, dir, "tag", "-a", "v1.2.3", "-m", "release v1.2.3")

	meta, err := ExtractGitMetadata(dir)
	require.NoError(t, err)

	assert.Equal(t, "v1.2.3", meta.Tag, "annotated tag at HEAD must be captured")
	assert.Equal(t, "main", meta.Branch)
}

func TestExtractGitMetadata_LightweightTag(t *testing.T) {
	skipIfNoGit(t)
	dir := t.TempDir()
	initRepo(t, dir)

	// Lightweight tag.
	mustGit(t, dir, "tag", "v0.1.0")

	meta, err := ExtractGitMetadata(dir)
	require.NoError(t, err)

	assert.Equal(t, "v0.1.0", meta.Tag, "lightweight tag at HEAD must be captured")
}

func TestExtractGitMetadata_TagNotAtHEAD(t *testing.T) {
	skipIfNoGit(t)
	dir := t.TempDir()
	initRepo(t, dir)

	// Tag the initial commit, then make a new commit so HEAD is ahead of the tag.
	mustGit(t, dir, "tag", "v1.0.0")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "second.txt"), []byte("second"), 0o644))
	mustGit(t, dir, "add", ".")
	mustGit(t, dir, "commit", "-m", "second commit")

	meta, err := ExtractGitMetadata(dir)
	require.NoError(t, err)

	assert.Empty(t, meta.Tag, "tag not at HEAD must be empty")
}

func TestExtractGitMetadata_DirtyWorkingTree(t *testing.T) {
	skipIfNoGit(t)
	dir := t.TempDir()
	initRepo(t, dir)

	// Modify a tracked file without staging.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("modified"), 0o644))

	meta, err := ExtractGitMetadata(dir)
	require.NoError(t, err)

	assert.True(t, meta.Dirty, "modified tracked file must mark working tree dirty")
}

func TestExtractGitMetadata_DirtyUntrackedFile(t *testing.T) {
	skipIfNoGit(t)
	dir := t.TempDir()
	initRepo(t, dir)

	// Add an untracked file.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("new"), 0o644))

	meta, err := ExtractGitMetadata(dir)
	require.NoError(t, err)

	assert.True(t, meta.Dirty, "untracked file must mark working tree dirty")
}

func TestExtractGitMetadata_CleanAfterStageAndCommit(t *testing.T) {
	skipIfNoGit(t)
	dir := t.TempDir()
	initRepo(t, dir)

	// Add a file, stage it, commit it — tree should be clean.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "extra.txt"), []byte("extra"), 0o644))
	mustGit(t, dir, "add", ".")
	mustGit(t, dir, "commit", "-m", "add extra")

	meta, err := ExtractGitMetadata(dir)
	require.NoError(t, err)

	assert.False(t, meta.Dirty)
}

func TestExtractGitMetadata_WithRemote(t *testing.T) {
	skipIfNoGit(t)
	// Use two local repos: one as "origin", one as the working repo.
	originDir := t.TempDir()
	workDir := t.TempDir()

	initRepo(t, originDir)
	// Clone origin into workDir using a local file:// URL.
	mustGit(t, workDir, "clone", "file://"+originDir, ".")

	meta, err := ExtractGitMetadata(workDir)
	require.NoError(t, err)

	assert.NotEmpty(t, meta.Remote, "cloned repo must have an origin remote")
	assert.Contains(t, meta.Remote, originDir, "remote URL must reference the origin directory")
}

func TestExtractGitMetadata_SubdirectoryInsideRepo(t *testing.T) {
	skipIfNoGit(t)
	dir := t.TempDir()
	initRepo(t, dir)

	// Create a subdirectory inside the repo.
	sub := filepath.Join(dir, "subdir", "nested")
	require.NoError(t, os.MkdirAll(sub, 0o755))

	// ExtractGitMetadata should work from any path inside the repo.
	meta, err := ExtractGitMetadata(sub)
	require.NoError(t, err)

	assert.NotEmpty(t, meta.Commit, "must find commit from subdirectory")
	assert.Equal(t, "main", meta.Branch)
}

func TestExtractGitMetadata_PathTraversalSanitized(t *testing.T) {
	skipIfNoGit(t)
	dir := t.TempDir()
	initRepo(t, dir)

	// Pass a path with .. traversal components that resolves to the repo root.
	traversal := filepath.Join(dir, "subdir", "..", "subdir", "..")

	meta, err := ExtractGitMetadata(traversal)
	require.NoError(t, err)

	// After Clean+Abs the path resolves to dir, so we should get real metadata.
	assert.NotEmpty(t, meta.Commit, "path traversal must resolve to repo root and return metadata")
}

func TestExtractGitMetadata_EmptyString(t *testing.T) {
	skipIfNoGit(t)

	// Empty string resolves to the current working directory; just ensure no panic.
	meta, err := ExtractGitMetadata("")
	require.NoError(t, err)
	require.NotNil(t, meta)
}

// TestExtractGitMetadata_CommitIsFull40Chars verifies the SHA is the full form.
func TestExtractGitMetadata_CommitIsFull40Chars(t *testing.T) {
	skipIfNoGit(t)
	dir := t.TempDir()
	initRepo(t, dir)

	meta, err := ExtractGitMetadata(dir)
	require.NoError(t, err)

	if meta.Commit != "" {
		assert.Len(t, meta.Commit, 40, "commit SHA must be full 40-character form")
		// Must be all hex chars.
		for _, c := range meta.Commit {
			assert.True(t, (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f'),
				"commit SHA must be lowercase hex: got %q", string(c))
		}
	}
}

// TestExtractGitMetadata_IgnoresAmbientGitDir pins the fix for a real incident:
// GIT_DIR (which git exports to everything it spawns, including `go test` run
// from a hook) used to override `git -C dir`, so metadata was attributed to the
// exported repository instead of the directory being archived.
//
// That matters beyond test hygiene. The manifest is a trust artifact — a commit
// and branch that never contained the data is worse than no provenance at all,
// because it looks authoritative. This test asserts the archived directory wins.
func TestExtractGitMetadata_IgnoresAmbientGitDir(t *testing.T) {
	skipIfNoGit(t)

	// Two unrelated repos: one we archive, one we point GIT_DIR at. Their commits
	// must differ, so give each a distinct file — initRepo alone commits identical
	// content, which hashes to the same SHA when both land in the same second.
	target := t.TempDir()
	initRepo(t, target)
	targetCommit := commitFile(t, target, "target.txt", "archived repository")

	decoy := t.TempDir()
	initRepo(t, decoy)
	decoyCommit := commitFile(t, decoy, "decoy.txt", "repository named by GIT_DIR")

	require.NotEqual(t, targetCommit, decoyCommit, "repos must differ for this to prove anything")

	// Point the ambient environment at the decoy, exactly as a git hook would.
	t.Setenv("GIT_DIR", filepath.Join(decoy, ".git"))
	t.Setenv("GIT_WORK_TREE", decoy)
	t.Setenv("GIT_INDEX_FILE", filepath.Join(decoy, ".git", "index"))

	meta, err := ExtractGitMetadata(target)
	require.NoError(t, err)

	assert.Equal(t, targetCommit, meta.Commit,
		"metadata must describe the directory passed in, not the repo in GIT_DIR")
	assert.NotEqual(t, decoyCommit, meta.Commit,
		"ambient GIT_DIR must not redirect provenance to another repository")
}
