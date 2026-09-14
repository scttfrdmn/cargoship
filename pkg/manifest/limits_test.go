package manifest

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func TestReadAllLimited(t *testing.T) {
	// Under the limit returns all bytes.
	got, err := readAllLimited(strings.NewReader("hello"), 10, "x")
	if err != nil || string(got) != "hello" {
		t.Fatalf("under limit: got %q err %v", got, err)
	}
	// Exactly at the limit is allowed.
	if _, err := readAllLimited(strings.NewReader("hello"), 5, "x"); err != nil {
		t.Fatalf("at limit should pass: %v", err)
	}
	// One byte over the limit is rejected, not truncated.
	_, err = readAllLimited(strings.NewReader("hello!"), 5, "the payload")
	if err == nil || !strings.Contains(err.Error(), "safety limit") ||
		!strings.Contains(err.Error(), "the payload") {
		t.Fatalf("over limit must error with a safety-limit message, got %v", err)
	}
}

func TestObjectReadLimit(t *testing.T) {
	if got := objectReadLimit(0); got != MaxObjectBytes {
		t.Errorf("unknown size should fall back to MaxObjectBytes, got %d", got)
	}
	if got := objectReadLimit(-5); got != MaxObjectBytes {
		t.Errorf("negative size should fall back to MaxObjectBytes, got %d", got)
	}
	if got := objectReadLimit(1000); got != 1000+objectSizeSlack {
		t.Errorf("declared size should add slack, got %d", got)
	}
}

func TestCheckContentLength(t *testing.T) {
	cl := func(n int64) *int64 { return &n }
	if err := checkContentLength(nil, 100, "x"); err != nil {
		t.Errorf("nil ContentLength must pass (enforced later by the read): %v", err)
	}
	if err := checkContentLength(cl(100), 100, "x"); err != nil {
		t.Errorf("equal to limit must pass: %v", err)
	}
	err := checkContentLength(cl(101), 100, "manifest object k")
	if err == nil || !strings.Contains(err.Error(), "safety limit") ||
		!strings.Contains(err.Error(), "manifest object k") {
		t.Fatalf("over limit must error, got %v", err)
	}
}

// clMock is a minimal S3Downloader that returns a chosen body and (optional)
// ContentLength, for exercising the restore read bounds (CSH-SEC-003).
type clMock struct {
	body          []byte
	contentLength *int64
}

func (m *clMock) GetObject(_ context.Context, _ *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	return &s3.GetObjectOutput{
		Body:          io.NopCloser(bytes.NewReader(m.body)),
		ContentLength: m.contentLength,
	}, nil
}

// TestDownloadChunk_RejectsOversizedObject is the CSH-SEC-003 regression: a chunk
// object whose reported size exceeds the manifest's declared CompressedSize (plus
// slack) is refused before its bytes are allocated.
func TestDownloadChunk_RejectsOversizedObject(t *testing.T) {
	big := int64(5 << 20) // 5 MiB reported, well over declared 10 bytes + 1 MiB slack
	m := &Manifest{
		Version: ManifestVersion, Bucket: "b",
		Chunks: []ChunkEntry{{ID: 0, S3Key: "chunk-0.tar.zst", CompressedSize: 10}},
	}
	se := NewSelectiveExtractor(m, &clMock{body: []byte("tiny"), contentLength: &big}, 0)

	_, err := se.downloadChunk(context.Background(), "chunk-0.tar.zst")
	if err == nil || !strings.Contains(err.Error(), "safety limit") {
		t.Fatalf("oversized object must be refused, got %v", err)
	}
}

// TestDownloadChunk_AllowsDeclaredSize confirms the bound does not reject an
// object that matches its declared size (no regression to normal restores).
func TestDownloadChunk_AllowsDeclaredSize(t *testing.T) {
	payload := bytes.Repeat([]byte("z"), 512)
	n := int64(len(payload))
	m := &Manifest{
		Version: ManifestVersion, Bucket: "b",
		Chunks: []ChunkEntry{{ID: 0, S3Key: "chunk-0.tar.zst", CompressedSize: n}},
	}
	se := NewSelectiveExtractor(m, &clMock{body: payload, contentLength: &n}, 0)

	got, err := se.downloadChunk(context.Background(), "chunk-0.tar.zst")
	if err != nil {
		t.Fatalf("declared-size object must be read: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("returned %d bytes, want %d", len(got), len(payload))
	}
}

func TestValidateManifestCounts(t *testing.T) {
	if err := validateManifestCounts(1000, 10); err != nil {
		t.Errorf("normal counts must pass: %v", err)
	}
	if err := validateManifestCounts(MaxManifestFiles, MaxManifestChunks); err != nil {
		t.Errorf("at the limit must pass: %v", err)
	}
	if err := validateManifestCounts(MaxManifestFiles+1, 0); err == nil ||
		!strings.Contains(err.Error(), "files") {
		t.Errorf("over the file limit must error, got %v", err)
	}
	if err := validateManifestCounts(0, MaxManifestChunks+1); err == nil ||
		!strings.Contains(err.Error(), "chunks") {
		t.Errorf("over the chunk limit must error, got %v", err)
	}
}
