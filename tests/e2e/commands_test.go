//go:build e2e

// Broadens the end-to-end coverage beyond the Quick Start (upload → info →
// verify → restore) to the other documented CLI workflows, driven through the
// real cargoship binary against the in-process Substrate S3 emulator. These are
// the executable form of the command docs: if a documented happy path breaks,
// the matching test here fails. (#238 Phase 2)
package e2e

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// runCargoshipAllowErr runs the binary and returns combined output + the error
// (nil on success) without failing the test — for commands whose exit code is
// the thing under test.
func runCargoshipAllowErr(t *testing.T, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, cargoshipBin, args...)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// uploadTree uploads a small directory to the emulator and returns the upload
// URL (…/uploads/<id>). Shared setup for the S3-backed command tests.
func uploadTree(t *testing.T, bucket string, files map[string]string) string {
	t.Helper()
	if err := createBucket(substrateURL, bucket); err != nil {
		t.Fatalf("create bucket %s: %v", bucket, err)
	}
	src := t.TempDir()
	for name, content := range files {
		p := filepath.Join(src, name)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		writeFile(t, p, content)
	}
	out := runCargoship(t, "upload", src, "s3://"+bucket+"/archives", "--region", "us-east-1")
	id := extractUploadID(t, out)
	return "s3://" + bucket + "/archives/uploads/" + id
}

// TestEstimate_Local runs `estimate` on a local tree (no S3) in both table and
// JSON form, asserting the JSON is well-formed and carries a cost estimate.
func TestEstimate_Local(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "a.txt"), "hello\n")
	writeFile(t, filepath.Join(src, "b.txt"), "world\n")

	table := runCargoship(t, "estimate", src)
	if !strings.Contains(table, "Cost Estimate") {
		t.Fatalf("estimate table output missing 'Cost Estimate':\n%s", table)
	}

	jsonOut := runCargoship(t, "estimate", src, "--format", "json")
	// The command prints a config line before the JSON; slice from the first '{'.
	brace := strings.IndexByte(jsonOut, '{')
	if brace < 0 {
		t.Fatalf("no JSON object in estimate --format json output:\n%s", jsonOut)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(jsonOut[brace:]), &parsed); err != nil {
		t.Fatalf("estimate JSON did not parse: %v\n%s", err, jsonOut[brace:])
	}
	if _, ok := parsed["cost_estimate"]; !ok {
		t.Fatalf("estimate JSON missing cost_estimate key: %v", parsed)
	}
}

// TestCostBenchmarkCompare verifies the naive-vs-CargoShip cost comparison
// (used by the small-files tutorial) emits correct PUT-request pricing.
func TestCostBenchmarkCompare(t *testing.T) {
	out := runCargoship(t, "cost", "benchmark-compare",
		"--tool", "aws-cli", "--size-gb", "1", "--files", "1000")
	brace := strings.IndexByte(out, '{')
	if brace < 0 {
		t.Fatalf("no JSON in benchmark-compare output:\n%s", out)
	}
	var r struct {
		Tool           string  `json:"tool"`
		PUTRequestCost float64 `json:"put_request_cost"`
	}
	if err := json.Unmarshal([]byte(out[brace:]), &r); err != nil {
		t.Fatalf("benchmark-compare JSON did not parse: %v\n%s", err, out[brace:])
	}
	// 1000 files ÷ 1000 × $0.005/1k = $0.005 (guards the #233 pricing fix).
	if r.PUTRequestCost < 0.0049 || r.PUTRequestCost > 0.0051 {
		t.Fatalf("PUT request cost = %v, want ~0.005 (1000 files @ $0.005/1k)", r.PUTRequestCost)
	}
}

// TestBudgetSetStatus_RoundTrip sets a project budget in one process and reads
// it back in another, asserting it persisted (#241). Uses an isolated on-disk
// store via CARGOSHIP_BUDGET_STORE so it doesn't touch the developer's real
// ~/.cargoship/budgets.json.
func TestBudgetSetStatus_RoundTrip(t *testing.T) {
	store := filepath.Join(t.TempDir(), "budgets.json")
	t.Setenv("CARGOSHIP_BUDGET_STORE", store)

	// Set the budget (one process).
	out, err := runCargoshipAllowErr(t, "budget", "set", "e2e-proj",
		"--cost", "100", "--volume", "50")
	if err != nil {
		t.Fatalf("budget set failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Budget set for project") {
		t.Fatalf("budget set output missing confirmation:\n%s", out)
	}

	// Read it back (a fresh process) — it must have persisted.
	status, err := runCargoshipAllowErr(t, "budget", "status", "e2e-proj")
	if err != nil {
		t.Fatalf("budget status failed (did the budget persist?): %v\n%s", err, status)
	}
	if !strings.Contains(status, "$100.00") {
		t.Fatalf("budget status did not report the persisted $100.00 budget:\n%s", status)
	}
}

// TestList_FromManifest uploads a tree, then lists it back from the manifest.
func TestList_FromManifest(t *testing.T) {
	url := uploadTree(t, "list-e2e", map[string]string{
		"one.txt": "first\n",
		"two.txt": "second\n",
	})
	// url is s3://bucket/archives/uploads/<id>; list takes --bucket/--upload-id
	// plus the --prefix the upload was written under ("archives"). --verbose
	// prints full paths (the default view truncates long absolute paths).
	bucket, uploadID := splitUploadURL(t, url)
	out := runCargoship(t, "list", "--bucket", bucket, "--upload-id", uploadID,
		"--prefix", "archives", "--verbose", "--region", "us-east-1")
	if !strings.Contains(out, "2 files") && !strings.Contains(out, "2 total") {
		t.Fatalf("list did not report 2 files:\n%s", out)
	}
	for _, want := range []string{"one.txt", "two.txt"} {
		if !strings.Contains(out, want) {
			t.Fatalf("list output missing %q:\n%s", want, out)
		}
	}
}

// TestDownload_RoundTrip uploads a tree then downloads the whole upload back,
// asserting a known file's content survives the round trip.
func TestDownload_RoundTrip(t *testing.T) {
	const want = "downloadable content\n"
	url := uploadTree(t, "download-e2e", map[string]string{"payload.txt": want})

	dest := t.TempDir()
	runCargoship(t, "download", url, dest, "--region", "us-east-1")

	// The file may land at dest/payload.txt or under a nested path; search for it.
	var found string
	_ = filepath.Walk(dest, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && filepath.Base(p) == "payload.txt" {
			found = p
		}
		return nil
	})
	if found == "" {
		t.Fatalf("payload.txt not found under %s after download", dest)
	}
	got, err := os.ReadFile(found)
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if string(got) != want {
		t.Fatalf("downloaded content mismatch:\n got: %q\nwant: %q", got, want)
	}
}

// TestSync_Incremental uploads a tree, adds a file, and re-syncs — asserting the
// second run succeeds (incremental upload path).
func TestSync_Incremental(t *testing.T) {
	bucket := "sync-e2e"
	if err := createBucket(substrateURL, bucket); err != nil {
		t.Fatalf("create bucket: %v", err)
	}
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "keep.txt"), "unchanged\n")

	dest := "s3://" + bucket + "/synced"
	if out, err := runCargoshipAllowErr(t, "sync", src, dest, "--region", "us-east-1"); err != nil {
		t.Fatalf("initial sync failed: %v\n%s", err, out)
	}

	// Add a new file and sync again.
	writeFile(t, filepath.Join(src, "added.txt"), "new file\n")
	if out, err := runCargoshipAllowErr(t, "sync", src, dest, "--region", "us-east-1"); err != nil {
		t.Fatalf("incremental sync failed: %v\n%s", err, out)
	}
}

// TestDVC_Stages uploads a tree, then runs `dvc stages` against the manifest.
// `dvc stages` takes the upload S3_URL directly.
func TestDVC_Stages(t *testing.T) {
	url := uploadTree(t, "dvc-e2e", map[string]string{
		"data/train.bin": "train\n",
		"data/test.bin":  "test\n",
	})
	// These files have no DVC metadata, so the output is a valid "no stages"
	// result — the command must run without error.
	if out, err := runCargoshipAllowErr(t, "dvc", "stages", url, "--region", "us-east-1"); err != nil {
		t.Fatalf("dvc stages failed: %v\n%s", err, out)
	}
}

// splitUploadURL turns s3://bucket/prefix/uploads/<id> into (bucket, id).
func splitUploadURL(t *testing.T, url string) (bucket, uploadID string) {
	t.Helper()
	trimmed := strings.TrimPrefix(url, "s3://")
	parts := strings.SplitN(trimmed, "/", 2)
	if len(parts) != 2 {
		t.Fatalf("malformed upload URL: %s", url)
	}
	bucket = parts[0]
	idx := strings.LastIndex(url, "/uploads/")
	if idx < 0 {
		t.Fatalf("no /uploads/ in URL: %s", url)
	}
	uploadID = url[idx+len("/uploads/"):]
	return bucket, uploadID
}

// TestRestore_RefusesSymlinkedDestination is the binary-level form of #341: the
// destination directory already contains a symlink pointing outside itself, as
// could be left by any process with write access there. The restore must refuse
// and exit non-zero, and nothing may appear at the link target.
//
// The unit tests in pkg/manifest pin the write helpers; this pins the contract a
// user actually experiences, including the exit code — a refusal reported only in
// stats but exiting 0 would look like a successful restore.
func TestRestore_RefusesSymlinkedDestination(t *testing.T) {
	url := uploadTree(t, "symlink-dest-e2e", map[string]string{
		"cache/config.txt": "original payload\n",
	})

	base := t.TempDir()
	dest := filepath.Join(base, "dest")
	outside := filepath.Join(base, "outside")
	for _, d := range []string{dest, outside} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	if err := os.Symlink(outside, filepath.Join(dest, "cache")); err != nil {
		t.Skipf("symlinks unavailable on this platform: %v", err)
	}

	out, err := runCargoshipAllowErr(t, "restore", url, dest, "--file", "config.txt", "--region", "us-east-1")

	if escaped := filepath.Join(outside, "config.txt"); !fileMissing(t, escaped) {
		t.Fatalf("restore escaped the destination: wrote through a symlink to %s\n%s", escaped, out)
	}
	if err == nil {
		t.Fatalf("restore into a symlinked destination exited 0; it must fail\n%s", out)
	}
}

// TestRestore_CleanDestinationStillWorks is the companion control: the same
// archive into an untouched destination must still round-trip byte-identically,
// confirming the containment work did not break the ordinary path.
func TestRestore_CleanDestinationStillWorks(t *testing.T) {
	want := "original payload\n"
	url := uploadTree(t, "clean-dest-e2e", map[string]string{
		"cache/config.txt": want,
	})

	dest := t.TempDir()
	runCargoship(t, "restore", url, dest, "--file", "config.txt", "--region", "us-east-1")

	got, err := os.ReadFile(findFileByBase(t, dest, "config.txt"))
	if err != nil {
		t.Fatalf("read restored file: %v", err)
	}
	if string(got) != want {
		t.Fatalf("restored content mismatch:\n got: %q\nwant: %q", got, want)
	}
}

// fileMissing reports whether path does not exist, failing on any other error so
// a permission problem is not silently read as "nothing escaped".
func fileMissing(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Lstat(path)
	if err == nil {
		return false
	}
	if !os.IsNotExist(err) {
		t.Fatalf("lstat %s: %v", path, err)
	}
	return true
}
