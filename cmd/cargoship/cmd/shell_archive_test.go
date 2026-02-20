package cmd

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scttfrdmn/cargoship/pkg/archivefs"
	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

// noopS3Client satisfies manifest.S3Downloader but returns errors for all calls.
// Sufficient for tests that only exercise the VirtualFS layer (no actual downloads).
type noopS3Client struct{}

func (n *noopS3Client) GetObject(_ context.Context, _ *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	return &s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader(nil))}, fmt.Errorf("no S3 in tests")
}

// buildShellManifest returns a small manifest for shell tests.
func buildShellManifest() *manifest.Manifest {
	now := time.Now()
	files := []manifest.FileEntry{
		{Path: "data/raw/input.csv", Size: 1024, ModTime: now, S3Key: "shard-0/chunk-0.tar.zst"},
		{Path: "data/train/features.parquet", Size: 13000000, ModTime: now,
			S3Key: "shard-0/chunk-1.tar.zst", ContentHash: "d8e8fca2dc0f896fd7cb4cb0031ba249",
			DVCMetadata: &manifest.DVCMetadata{Stage: "train"}},
		{Path: "data/train/labels.csv", Size: 2100000, ModTime: now,
			S3Key:       "shard-0/chunk-1.tar.zst",
			DVCMetadata: &manifest.DVCMetadata{Stage: "train"}},
		{Path: "models/model.pkl", Size: 500000, ModTime: now, S3Key: "shard-0/chunk-2.tar.zst",
			DVCMetadata: &manifest.DVCMetadata{Stage: "train"}},
		{Path: "README.md", Size: 2048, ModTime: now, S3Key: "shard-0/chunk-0.tar.zst"},
	}
	return &manifest.Manifest{
		Version:     manifest.ManifestVersion,
		UploadID:    "shell-test-001",
		Bucket:      "test-bucket",
		Files:       files,
		TotalFiles:  int64(len(files)),
		GitMetadata: &manifest.GitMetadata{Commit: "deadbeef"},
	}
}

// newTestShell builds an archiveShell backed by a mock S3 client (no downloads).
func newTestShell(t *testing.T, input string) (*archiveShell, *bytes.Buffer) {
	t.Helper()
	m := buildShellManifest()
	vfs := archivefs.New(m)
	out := new(bytes.Buffer)
	sh := &archiveShell{
		vfs:   vfs,
		se:    manifest.NewSelectiveExtractor(m, &noopS3Client{}, 0),
		s3URL: "s3://test-bucket/uploads/shell-test-001",
		cwd:   "",
		in:    bufio.NewScanner(strings.NewReader(input)),
		out:   out,
	}
	return sh, out
}

// runCommand is a convenience helper that dispatches a single command.
func runCommand(t *testing.T, input string) string {
	t.Helper()
	sh, out := newTestShell(t, "")
	sh.dispatch(context.Background(), input)
	return out.String()
}

// ---------------------------------------------------------------------------
// pwd
// ---------------------------------------------------------------------------

func TestShell_Pwd_Root(t *testing.T) {
	assert.Contains(t, runCommand(t, "pwd"), "/")
}

func TestShell_Pwd_Subdir(t *testing.T) {
	sh, out := newTestShell(t, "")
	sh.cwd = "data/train"
	sh.dispatch(context.Background(), "pwd")
	assert.Contains(t, out.String(), "data/train")
}

// ---------------------------------------------------------------------------
// ls
// ---------------------------------------------------------------------------

func TestShell_Ls_Root(t *testing.T) {
	output := runCommand(t, "ls")
	assert.Contains(t, output, "data/")
	assert.Contains(t, output, "models/")
	assert.Contains(t, output, "README.md")
}

func TestShell_Ls_Subdir(t *testing.T) {
	output := runCommand(t, "ls data/train")
	assert.Contains(t, output, "features.parquet")
	assert.Contains(t, output, "labels.csv")
}

func TestShell_Ls_Missing(t *testing.T) {
	output := runCommand(t, "ls does/not/exist")
	assert.Contains(t, output, "no such")
}

func TestShell_Ls_File(t *testing.T) {
	output := runCommand(t, "ls README.md")
	assert.Contains(t, output, "README.md")
}

// ---------------------------------------------------------------------------
// cd
// ---------------------------------------------------------------------------

func TestShell_Cd(t *testing.T) {
	sh, _ := newTestShell(t, "")
	sh.dispatch(context.Background(), "cd data/train")
	assert.Equal(t, "data/train", sh.cwd)
}

func TestShell_Cd_DotDot(t *testing.T) {
	sh, _ := newTestShell(t, "")
	sh.cwd = "data/train"
	sh.dispatch(context.Background(), "cd ..")
	assert.Equal(t, "data", sh.cwd)
}

func TestShell_Cd_Root(t *testing.T) {
	sh, _ := newTestShell(t, "")
	sh.cwd = "data/train"
	sh.dispatch(context.Background(), "cd /")
	assert.Equal(t, "", sh.cwd)
}

func TestShell_Cd_Missing(t *testing.T) {
	sh, out := newTestShell(t, "")
	sh.dispatch(context.Background(), "cd no/such/dir")
	assert.Equal(t, "", sh.cwd) // unchanged
	assert.Contains(t, out.String(), "no such")
}

func TestShell_Cd_File(t *testing.T) {
	sh, out := newTestShell(t, "")
	sh.dispatch(context.Background(), "cd README.md")
	assert.Equal(t, "", sh.cwd) // unchanged
	assert.Contains(t, out.String(), "not a directory")
}

func TestShell_Cd_NoArgs_GoesHome(t *testing.T) {
	sh, _ := newTestShell(t, "")
	sh.cwd = "data/train"
	sh.dispatch(context.Background(), "cd")
	assert.Equal(t, "", sh.cwd)
}

// ---------------------------------------------------------------------------
// stat
// ---------------------------------------------------------------------------

func TestShell_Stat(t *testing.T) {
	output := runCommand(t, "stat data/train/features.parquet")
	assert.Contains(t, output, "data/train/features.parquet")
	assert.Contains(t, output, "d8e8fca2")
	assert.Contains(t, output, "train")
	assert.Contains(t, output, "deadbeef")
}

func TestShell_Stat_Missing(t *testing.T) {
	assert.Contains(t, runCommand(t, "stat missing.txt"), "no such file")
}

func TestShell_Stat_Dir(t *testing.T) {
	assert.Contains(t, runCommand(t, "stat data"), "is a directory")
}

func TestShell_Stat_NoArgs(t *testing.T) {
	assert.Contains(t, runCommand(t, "stat"), "usage:")
}

// ---------------------------------------------------------------------------
// find
// ---------------------------------------------------------------------------

func TestShell_Find_ByExtension(t *testing.T) {
	output := runCommand(t, "find *.csv")
	assert.Contains(t, output, "input.csv")
	assert.Contains(t, output, "labels.csv")
}

func TestShell_Find_FullPath(t *testing.T) {
	output := runCommand(t, "find data/train/*.csv")
	assert.Contains(t, output, "labels.csv")
	assert.NotContains(t, output, "input.csv")
}

func TestShell_Find_NoMatch(t *testing.T) {
	assert.Contains(t, runCommand(t, "find *.xyz"), "no files matching")
}

func TestShell_Find_NoArgs(t *testing.T) {
	assert.Contains(t, runCommand(t, "find"), "usage:")
}

// ---------------------------------------------------------------------------
// stage
// ---------------------------------------------------------------------------

func TestShell_Stage_List(t *testing.T) {
	output := runCommand(t, "stage list")
	assert.Contains(t, output, "train")
	assert.Contains(t, output, "3")
}

func TestShell_Stage_NoArgs_ListsStages(t *testing.T) {
	// `stage` with no args should behave like `stage list`.
	output := runCommand(t, "stage")
	assert.Contains(t, output, "train")
}

func TestShell_Stage_Named(t *testing.T) {
	output := runCommand(t, "stage train")
	assert.Contains(t, output, "features.parquet")
	assert.Contains(t, output, "labels.csv")
	assert.Contains(t, output, "model.pkl")
}

func TestShell_Stage_Unknown(t *testing.T) {
	assert.Contains(t, runCommand(t, "stage no-such-stage"), "no files found")
}

// ---------------------------------------------------------------------------
// help / unknown
// ---------------------------------------------------------------------------

func TestShell_Help(t *testing.T) {
	output := runCommand(t, "help")
	assert.Contains(t, output, "ls")
	assert.Contains(t, output, "cd")
	assert.Contains(t, output, "stat")
}

func TestShell_Unknown(t *testing.T) {
	assert.Contains(t, runCommand(t, "notacommand"), "unknown command")
}

// ---------------------------------------------------------------------------
// exit
// ---------------------------------------------------------------------------

func TestShell_Exit(t *testing.T) {
	sh, _ := newTestShell(t, "")
	done := sh.dispatch(context.Background(), "exit")
	assert.True(t, done)
}

func TestShell_Quit(t *testing.T) {
	sh, _ := newTestShell(t, "")
	done := sh.dispatch(context.Background(), "quit")
	assert.True(t, done)
}

// ---------------------------------------------------------------------------
// run loop
// ---------------------------------------------------------------------------

func TestShell_Run_ExitsOnEOF(t *testing.T) {
	sh, _ := newTestShell(t, "ls\ncd data\npwd\nexit\n")
	err := sh.run(context.Background())
	require.NoError(t, err)
}

func TestShell_Run_IgnoresBlankLines(t *testing.T) {
	sh, out := newTestShell(t, "\n\n\nexit\n")
	require.NoError(t, sh.run(context.Background()))
	assert.NotContains(t, out.String(), "unknown command")
}

// ---------------------------------------------------------------------------
// prompt
// ---------------------------------------------------------------------------

func TestShell_Prompt_Root(t *testing.T) {
	sh, _ := newTestShell(t, "")
	assert.Equal(t, "archive:/> ", sh.prompt())
}

func TestShell_Prompt_Subdir(t *testing.T) {
	sh, _ := newTestShell(t, "")
	sh.cwd = "data/train"
	assert.Equal(t, "archive:/data/train> ", sh.prompt())
}

// ---------------------------------------------------------------------------
// get / cat / head: error paths (no real S3 — just cover the "no such file" branches)
// ---------------------------------------------------------------------------

func TestShell_Cat_Missing(t *testing.T) {
	assert.Contains(t, runCommand(t, "cat missing.txt"), "no such file")
}

func TestShell_Cat_NoArgs(t *testing.T) {
	assert.Contains(t, runCommand(t, "cat"), "usage:")
}

func TestShell_Head_Missing(t *testing.T) {
	assert.Contains(t, runCommand(t, "head missing.txt"), "no such file")
}

func TestShell_Head_NoArgs(t *testing.T) {
	assert.Contains(t, runCommand(t, "head"), "usage:")
}

func TestShell_Get_Missing(t *testing.T) {
	assert.Contains(t, runCommand(t, "get missing.txt"), "no such file")
}

func TestShell_Get_NoArgs(t *testing.T) {
	assert.Contains(t, runCommand(t, "get"), "usage:")
}

// ---------------------------------------------------------------------------
// ls within cwd
// ---------------------------------------------------------------------------

func TestShell_Ls_UsescCwd(t *testing.T) {
	sh, out := newTestShell(t, "")
	sh.cwd = "data"
	sh.dispatch(context.Background(), "ls")
	assert.Contains(t, out.String(), "raw/")
	assert.Contains(t, out.String(), "train/")
}

// ---------------------------------------------------------------------------
// noopS3Client — verify archiveShell builds without panicking
// ---------------------------------------------------------------------------
func TestShell_MockS3_Empty(t *testing.T) {
	m := &manifest.Manifest{
		Files: []manifest.FileEntry{
			{Path: "f.txt", Size: 10, S3Key: "shard-0/chunk-0.tar.zst", ModTime: time.Now()},
		},
		TotalFiles: 1,
	}
	vfs := archivefs.New(m)
	out := new(bytes.Buffer)
	sh := &archiveShell{
		vfs:   vfs,
		se:    manifest.NewSelectiveExtractor(m, &noopS3Client{}, 0),
		s3URL: "s3://b/p",
		cwd:   "",
		in:    bufio.NewScanner(strings.NewReader("")),
		out:   out,
	}
	sh.dispatch(context.Background(), "stat f.txt")
	assert.Contains(t, out.String(), "f.txt")
}
