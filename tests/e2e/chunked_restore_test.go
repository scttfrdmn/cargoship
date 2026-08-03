//go:build e2e

// End-to-end regression for #334: restoring a CHUNKED archive from a PREFIXED
// destination by the exact path the manifest records.
//
// TestQuickStart_RoundTrip already drives upload → info → verify → restore, but
// it misses this defect on two independent axes, and either one alone hides it:
//
//   - Its files are small, so the upload takes the direct-upload fast path and
//     the manifest has no chunks at all. The Glacier pre-flight check only
//     resolves chunk keys, so there is nothing for it to get wrong.
//   - It restores by BASENAME (`--file greeting.txt`). The pre-flight path used
//     exact-match lookup while the restore itself falls back to basename
//     matching (#228), so a basename target yielded zero keys, the check
//     verified nothing, and the restore passed regardless.
//
// So this test deliberately pins the combination: chunked layout, non-empty S3
// prefix, exact manifest path. Before the fix it failed with
// `glacier pre-flight check failed: HeadObject "uploads/.../chunk-0.tar.zst":
// StatusCode: 404` and exit status 3.
package e2e

import (
	"os"
	"path/filepath"
	"testing"
)

// TestChunkedRestore_PrefixedDestination_ExactPath is the #334 regression.
func TestChunkedRestore_PrefixedDestination_ExactPath(t *testing.T) {
	bucket := "chunked-prefixed-e2e"
	if err := createBucket(substrateURL, bucket); err != nil {
		t.Fatalf("create bucket: %v", err)
	}

	// Files large enough to defeat the direct-upload heuristic (average size
	// >= 5 MB), so the upload produces real chunks. Patterned rather than random
	// so it compresses to a couple of KB and the test stays fast.
	src := t.TempDir()
	big := make([]byte, 6*1024*1024)
	for i := range big {
		big[i] = byte(i % 251)
	}
	for _, name := range []string{"big-a.bin", "big-b.bin"} {
		if err := os.WriteFile(filepath.Join(src, name), big, 0o644); err != nil {
			t.Fatalf("write source file: %v", err)
		}
	}

	// A non-empty prefix is essential: manifests record chunk S3Key values
	// RELATIVE to it, so with a bare s3://bucket destination the unresolved and
	// resolved keys are identical and the bug cannot appear.
	dest := "s3://" + bucket + "/archives"
	out := runCargoship(t, "upload", src, dest, "--region", "us-east-1")
	uploadID := extractUploadID(t, out)
	uploadURL := dest + "/uploads/" + uploadID

	// Restore by the ABSOLUTE path the manifest records, not a basename — the
	// path `cargoship info` shows a user, and the one that exercises the
	// pre-flight check rather than silently skipping it.
	restoreDir := t.TempDir()
	runCargoship(t, "restore", uploadURL, restoreDir,
		"--region", "us-east-1", "--file", filepath.Join(src, "big-a.bin"))

	got, err := os.ReadFile(findFileByBase(t, restoreDir, "big-a.bin"))
	if err != nil {
		t.Fatalf("read restored file: %v", err)
	}
	if len(got) != len(big) {
		t.Fatalf("restored size %d, want %d", len(got), len(big))
	}
	for i := range got {
		if got[i] != big[i] {
			t.Fatalf("restored content differs at byte %d: got %d want %d", i, got[i], big[i])
		}
	}
}
