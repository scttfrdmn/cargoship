//go:build e2e

// End-to-end tests for the ghostship backup daemon (#604), driven through the real
// cargoship binary against the in-process Substrate S3 emulator. These prove the
// ghostship-specific orchestration that unit tests can't: the writers/<id>/ key
// layout, a full writer-scoped round-trip (upload → restore, byte-identical), the
// writer-scoped INCREMENTAL chain (a second cycle finds the prior manifest under the
// writer prefix), and config-file multi-source runs.
package e2e

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// e2eS3Client builds an emulator-pointed S3 client (mirrors createBucket).
func e2eS3Client(t *testing.T) *s3.Client {
	t.Helper()
	cfg := aws.Config{
		Region:       "us-east-1",
		BaseEndpoint: aws.String(substrateURL),
		Credentials:  credentials.NewStaticCredentialsProvider("test", "test", ""),
	}
	return s3.NewFromConfig(cfg, func(o *s3.Options) { o.UsePathStyle = true })
}

// uploadIDsUnder returns the sorted upload-id segments under <prefix>/uploads/ — used
// to assert the writer-isolated layout and count version cycles.
func uploadIDsUnder(t *testing.T, client *s3.Client, bucket, prefix string) []string {
	t.Helper()
	p := strings.TrimSuffix(prefix, "/") + "/uploads/"
	out, err := client.ListObjectsV2(context.Background(), &s3.ListObjectsV2Input{
		Bucket:    aws.String(bucket),
		Prefix:    aws.String(p),
		Delimiter: aws.String("/"),
	})
	if err != nil {
		t.Fatalf("list uploads under %s: %v", p, err)
	}
	var ids []string
	for _, cp := range out.CommonPrefixes {
		seg := strings.TrimSuffix(strings.TrimPrefix(aws.ToString(cp.Prefix), p), "/")
		if seg != "" {
			ids = append(ids, seg)
		}
	}
	sort.Strings(ids)
	return ids
}

// TestGhostshipRun_RoundTrip proves the writer-isolated layout and a full round-trip:
// `ghostship run --once --writer-id` writes under writers/<id>/uploads/, and restore
// returns the exact bytes.
func TestGhostshipRun_RoundTrip(t *testing.T) {
	bucket := "gs-roundtrip"
	if err := createBucket(substrateURL, bucket); err != nil {
		t.Fatalf("create bucket: %v", err)
	}
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "greeting.txt"), "hello ghostship")
	if err := os.MkdirAll(filepath.Join(src, "docs"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeFile(t, filepath.Join(src, "docs", "readme.md"), "# readme\nnested content")

	runCargoship(t, "ghostship", "run", src, "s3://"+bucket+"/archives",
		"--writer-id", "lab-nas-1", "--once", "--region", "us-east-1")

	client := e2eS3Client(t)
	ids := uploadIDsUnder(t, client, bucket, "archives/writers/lab-nas-1")
	if len(ids) != 1 {
		t.Fatalf("want exactly 1 upload under archives/writers/lab-nas-1/uploads/, got %d: %v", len(ids), ids)
	}
	uploadURL := "s3://" + bucket + "/archives/writers/lab-nas-1/uploads/" + ids[0]

	restoreDir := t.TempDir()
	for _, tc := range []struct{ abs, base, want string }{
		{filepath.Join(src, "greeting.txt"), "greeting.txt", "hello ghostship"},
		{filepath.Join(src, "docs", "readme.md"), "readme.md", "# readme\nnested content"},
	} {
		runCargoship(t, "restore", uploadURL, restoreDir, "--region", "us-east-1", "--file", tc.abs)
		got, err := os.ReadFile(findFileByBase(t, restoreDir, tc.base))
		if err != nil {
			t.Fatalf("read restored %s: %v", tc.base, err)
		}
		if string(got) != tc.want {
			t.Fatalf("restored %s = %q, want %q", tc.base, got, tc.want)
		}
	}
}

// TestGhostshipRun_IncrementalChain proves the writer-scoped incremental chain: a
// no-change cycle uploads nothing, and a subsequent change produces a second version
// under the same writer prefix (i.e. the prior manifest is found under writers/<id>/).
func TestGhostshipRun_IncrementalChain(t *testing.T) {
	bucket := "gs-incremental"
	if err := createBucket(substrateURL, bucket); err != nil {
		t.Fatalf("create bucket: %v", err)
	}
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "a.txt"), "one")

	runOut := func(label string) string {
		out, err := runCargoshipAllowErr(t, "ghostship", "run", src, "s3://"+bucket+"/data", "--writer-id", "inc-1", "--once", "--region", "us-east-1")
		if err != nil {
			t.Fatalf("%s: %v\n%s", label, err, out)
		}
		return out
	}

	// First cycle has no prior manifest → full sync.
	if out := runOut("first"); !strings.Contains(out, "sync_type=full") {
		t.Fatalf("first cycle should be a full sync, got:\n%s", out)
	}

	// A later cycle must find the prior manifest UNDER THE WRITER PREFIX
	// (writers/inc-1/uploads/…) and chain to it → incremental. This is the
	// writer-scoped-chain proof: FindLatestManifestForSource resolves within the
	// folded prefix, which no unit test can verify.
	writeFile(t, filepath.Join(src, "a.txt"), "one-changed")
	if out := runOut("second"); !strings.Contains(out, "sync_type=incremental") {
		t.Fatalf("second cycle should chain to the prior manifest under the writer prefix (incremental), got:\n%s", out)
	}

	// And the second upload is a distinct version under the same writer prefix.
	client := e2eS3Client(t)
	if ids := uploadIDsUnder(t, client, bucket, "data/writers/inc-1"); len(ids) < 2 {
		t.Fatalf("expected a second version under data/writers/inc-1/uploads/, got %v", ids)
	}
}

// TestGhostshipRun_ConfigMultiSource proves config-file mode: each watch_paths entry is
// synced as its own chain under one writer prefix, and archival_rules are ignored with a
// warning.
func TestGhostshipRun_ConfigMultiSource(t *testing.T) {
	bucket := "gs-config"
	if err := createBucket(substrateURL, bucket); err != nil {
		t.Fatalf("create bucket: %v", err)
	}
	src1, src2 := t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(src1, "a.txt"), "alpha")
	writeFile(t, filepath.Join(src2, "b.txt"), "bravo")

	box := filepath.Join(t.TempDir(), "box.yaml")
	writeFile(t, box, fmt.Sprintf(`id: fleet-box
writer_id: fleet-box
s3_config:
  bucket: %s
watch_paths:
  - path: %s
  - path: %s
`, bucket, src1, src2))

	runCargoship(t, "ghostship", "run", "--config", box, "--once", "--region", "us-east-1")

	client := e2eS3Client(t)
	if ids := uploadIDsUnder(t, client, bucket, "writers/fleet-box"); len(ids) != 2 {
		t.Fatalf("config multi-source: want 2 uploads under writers/fleet-box/uploads/, got %d: %v", len(ids), ids)
	}

	// archival_rules are advisory in sync mode: present → run warns but still succeeds.
	boxRules := filepath.Join(t.TempDir(), "box-rules.yaml")
	writeFile(t, boxRules, fmt.Sprintf(`id: rules-box
writer_id: rules-box
s3_config:
  bucket: %s
watch_paths:
  - path: %s
archival_rules:
  - name: legacy
    file_pattern: "*.txt"
`, bucket, src1))
	out, err := runCargoshipAllowErr(t, "ghostship", "run", "--config", boxRules, "--once", "--region", "us-east-1")
	if err != nil {
		t.Fatalf("run with archival_rules should still succeed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "archival_rules") {
		t.Fatalf("expected an archival_rules ignore-warning, got:\n%s", out)
	}
}
