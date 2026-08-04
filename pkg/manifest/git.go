package manifest

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// gitTimeout is the maximum wall-clock time allowed for all git commands
// issued during a single ExtractGitMetadata call.
const gitTimeout = 15 * time.Second

// ExtractGitMetadata returns Git repository state for the repository that
// contains repoPath.
//
// The function returns a zero-value GitMetadata (and a nil error) when:
//   - repoPath does not exist or cannot be resolved to an absolute path
//   - repoPath is not inside a Git repository
//   - the git binary is not on PATH
//
// Individual fields are left empty when the corresponding git command fails
// for a legitimate reason:
//   - Branch is empty when HEAD is in detached state
//   - Tag is empty when no annotated or lightweight tag points exactly at HEAD
//   - Remote is empty when no remote named "origin" is configured
//
// The repoPath argument is sanitized with filepath.Clean + filepath.Abs before
// being passed to git, preventing path-traversal sequences from reaching the
// subprocess (git -C <path> …).
func ExtractGitMetadata(repoPath string) (*GitMetadata, error) {
	// Sanitize: normalise and resolve to an absolute path.
	absPath, err := filepath.Abs(filepath.Clean(repoPath))
	if err != nil {
		return &GitMetadata{}, nil
	}
	if _, err := os.Stat(absPath); err != nil {
		return &GitMetadata{}, nil
	}

	// Graceful fallback when git is not installed.
	if _, err := exec.LookPath("git"); err != nil {
		return &GitMetadata{}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()

	// Confirm we are inside a git repository; return zero-value (not an error)
	// if we are not.
	if _, err := gitRun(ctx, absPath, "rev-parse", "--git-dir"); err != nil {
		return &GitMetadata{}, nil
	}

	meta := &GitMetadata{}

	// HEAD commit SHA (full 40-character hex).
	if commit, err := gitRun(ctx, absPath, "rev-parse", "HEAD"); err == nil {
		meta.Commit = commit
	}

	// Current branch name; fails (exit 128) in detached-HEAD state — leave empty.
	if branch, err := gitRun(ctx, absPath, "symbolic-ref", "--short", "HEAD"); err == nil {
		meta.Branch = branch
	}

	// Exact tag at HEAD; fails when HEAD has no tag — leave empty.
	if tag, err := gitRun(ctx, absPath, "describe", "--tags", "--exact-match", "HEAD"); err == nil {
		meta.Tag = tag
	}

	// Fetch URL of the "origin" remote; fails when no origin is configured.
	if remote, err := gitRun(ctx, absPath, "remote", "get-url", "origin"); err == nil {
		meta.Remote = remote
	}

	// Dirty working tree: any output from git status --porcelain means changes.
	if status, err := gitRun(ctx, absPath, "status", "--porcelain"); err == nil {
		meta.Dirty = status != ""
	}

	return meta, nil
}

// gitRun executes `git -C dir <args…>` with the given context and returns the
// trimmed stdout.  Stderr is discarded; the caller inspects the error.
func gitRun(ctx context.Context, dir string, args ...string) (string, error) {
	cmdArgs := make([]string, 0, 2+len(args))
	cmdArgs = append(cmdArgs, "-C", dir)
	cmdArgs = append(cmdArgs, args...)

	cmd := exec.CommandContext(ctx, "git", cmdArgs...)
	cmd.Env = gitCleanEnv()
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = nil // intentionally discard

	if err := cmd.Run(); err != nil {
		return "", err
	}
	return strings.TrimSpace(stdout.String()), nil
}

// gitEnvOverrides are the environment variables that make git ignore `-C dir`
// and operate on a different repository. Git exports them to every process it
// spawns, so any tool invoked from a hook or an alias inherits them.
var gitEnvOverrides = []string{
	"GIT_DIR",
	"GIT_WORK_TREE",
	"GIT_INDEX_FILE",
	"GIT_OBJECT_DIRECTORY",
	"GIT_ALTERNATE_OBJECT_DIRECTORIES",
	"GIT_COMMON_DIR",
	"GIT_PREFIX",
	"GIT_CEILING_DIRECTORIES",
}

// gitCleanEnv returns the current environment with git's repository-location
// overrides removed, so `-C dir` alone decides which repository is inspected.
//
// Without this, GIT_DIR wins over -C: running CargoShip from a git hook, an
// alias, or `git bisect run` would attribute the manifest's provenance to
// whatever repository git had exported rather than the directory being
// archived — recording a commit and branch that never contained the data. The
// manifest is a trust artifact, so a plausible-but-wrong value is worse than
// an absent one.
func gitCleanEnv() []string {
	env := os.Environ()
	out := make([]string, 0, len(env))
	for _, kv := range env {
		key := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			key = kv[:i]
		}
		if slices.Contains(gitEnvOverrides, key) {
			continue
		}
		out = append(out, kv)
	}
	return out
}
