//go:build e2e

// End-to-end regression for #975: download, shell and dvc read the whole
// dataset of an incremental version, not just that version's delta.
//
// restore and browse resolve a version's chain through loadEffectiveManifest
// (#552); these three read the single delta manifest, so after two syncs a file
// unchanged since the first was absent from all of them.
package e2e

import (
	"context"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

var syncUploadID = regexp.MustCompile(`Upload ID: (\S+)`)

// syncTwice syncs a two-file tree, adds a file, syncs again, and returns the
// source dir and the S3 URL of the second (incremental) version.
func syncTwice(t *testing.T, bucket string) (src, headURL string) {
	t.Helper()
	if err := createBucket(substrateURL, bucket); err != nil {
		t.Fatalf("create bucket: %v", err)
	}
	src = t.TempDir()
	writeFile(t, filepath.Join(src, "base-one.txt"), "cargoship-chain-base-one\n")
	writeFile(t, filepath.Join(src, "base-two.txt"), "cargoship-chain-base-two\n")
	dest := "s3://" + bucket + "/chain"
	runCargoship(t, "sync", src, dest, "--region", "us-east-1")

	writeFile(t, filepath.Join(src, "delta.txt"), "cargoship-chain-delta\n")
	out := runCargoship(t, "sync", src, dest, "--region", "us-east-1")
	m := syncUploadID.FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("no upload ID in the second sync's output:\n%s", out)
	}
	return src, dest + "/uploads/" + m[1]
}

var chainFiles = []string{"base-one.txt", "base-two.txt", "delta.txt"}

func TestChain_DownloadSeesTheWholeDataset(t *testing.T) {
	src, head := syncTwice(t, "chain-download")

	dest := t.TempDir()
	runCargoship(t, "download", head, dest, "--region", "us-east-1")
	for _, name := range chainFiles {
		if findFileByBaseOrEmpty(dest, name) == "" {
			t.Errorf("download of the head version is missing %s, a file of the full dataset", name)
		}
	}

	// --files names a file unchanged since the base version.
	one := t.TempDir()
	runCargoship(t, "download", head, one, "--region", "us-east-1", "--files", filepath.Join(src, "base-one.txt"))
	if findFileByBaseOrEmpty(one, "base-one.txt") == "" {
		t.Error("download --files of a base-version file extracted nothing")
	}
}

// TestDownload_FilesMatchingNothingFails: --files that match nothing wrote
// nothing and exited 0 (#983).
func TestDownload_FilesMatchingNothingFails(t *testing.T) {
	_, head := syncTwice(t, "chain-download-nomatch")
	out, err := runCargoshipAllowErr(t, "download", head, t.TempDir(), "--region", "us-east-1",
		"--files", "/no/such/file.txt")
	if err == nil {
		t.Fatalf("download --files matching nothing exited 0:\n%s", out)
	}
	if !strings.Contains(out, "/no/such/file.txt") {
		t.Errorf("the failure should name the unmatched path:\n%s", out)
	}
}

func TestChain_ShellSeesTheWholeDataset(t *testing.T) {
	_, head := syncTwice(t, "chain-shell")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, cargoshipBin, "shell", head, "--region", "us-east-1")
	cmd.Stdin = strings.NewReader("find *.txt\nexit\n")
	outBytes, err := cmd.CombinedOutput()
	out := string(outBytes)
	if err != nil {
		t.Fatalf("shell failed: %v\n%s", err, out)
	}
	for _, name := range chainFiles {
		if !strings.Contains(out, name) {
			t.Errorf("shell on the head version doesn't list %s:\n%s", name, out)
		}
	}
}

func TestChain_DVCStatusSeesTheWholeDataset(t *testing.T) {
	src, head := syncTwice(t, "chain-dvc")
	out := runCargoship(t, "dvc", "status", src, head, "--region", "us-east-1", "--json")
	for _, name := range chainFiles {
		if !strings.Contains(out, name) {
			t.Errorf("dvc status against the head version doesn't list %s:\n%s", name, out)
		}
	}
}
