package fleet

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// fakePutAPI captures the last PutObject call.
type fakePutAPI struct {
	key  string
	body []byte
}

func (f *fakePutAPI) PutObject(_ context.Context, in *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	f.key = aws.ToString(in.Key)
	f.body, _ = io.ReadAll(in.Body)
	return &s3.PutObjectOutput{}, nil
}

func TestWriteStatus_KeyAndRoundTrip(t *testing.T) {
	api := &fakePutAPI{}
	st := WriterStatus{
		WriterID:   "lab-nas-1",
		Hostname:   "nas.local",
		InstanceID: "inst-1",
		UpdatedAt:  time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC),
		Sources:    []SourceStatus{{Path: "/data", OK: true, Files: 3, Bytes: 42}},
	}
	if err := WriteStatus(context.Background(), api, "bucket", "archives/writers/lab-nas-1", st); err != nil {
		t.Fatalf("WriteStatus: %v", err)
	}
	if want := "archives/writers/lab-nas-1/status.json"; api.key != want {
		t.Errorf("key = %q, want %q", api.key, want)
	}
	var got WriterStatus
	if err := json.Unmarshal(api.body, &got); err != nil {
		t.Fatalf("stored body is not valid JSON: %v", err)
	}
	if got.WriterID != st.WriterID || !got.UpdatedAt.Equal(st.UpdatedAt) || len(got.Sources) != 1 {
		t.Errorf("round-trip mismatch: got %+v", got)
	}
	if !got.Healthy() {
		t.Errorf("Healthy() = false, want true for an all-OK writer")
	}
}

// fakeReadAPI serves a fixed set of CommonPrefixes and per-key object bodies.
type fakeReadAPI struct {
	commonPrefixes []string
	objects        map[string][]byte
	gotListPrefix  string
}

func (f *fakeReadAPI) ListObjectsV2(_ context.Context, in *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	f.gotListPrefix = aws.ToString(in.Prefix)
	out := &s3.ListObjectsV2Output{}
	for _, p := range f.commonPrefixes {
		out.CommonPrefixes = append(out.CommonPrefixes, s3types.CommonPrefix{Prefix: aws.String(p)})
	}
	return out, nil
}

func (f *fakeReadAPI) GetObject(_ context.Context, in *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	body, ok := f.objects[aws.ToString(in.Key)]
	if !ok {
		return nil, &s3types.NoSuchKey{}
	}
	return &s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader(body))}, nil
}

func mustJSON(t *testing.T, st WriterStatus) []byte {
	t.Helper()
	b, err := json.Marshal(st)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func TestListWriterStatuses(t *testing.T) {
	api := &fakeReadAPI{
		commonPrefixes: []string{
			"nas/writers/a/",
			"nas/writers/b/",
			"nas/writers/bad/",
			"nas/writers/missing/",
		},
		objects: map[string][]byte{
			"nas/writers/a/status.json":   mustJSON(t, WriterStatus{WriterID: "a", Sources: []SourceStatus{{OK: true}}}),
			"nas/writers/b/status.json":   mustJSON(t, WriterStatus{WriterID: "b", Sources: []SourceStatus{{OK: false, LastError: "boom"}}}),
			"nas/writers/bad/status.json": []byte("{not json"),
			// "missing" has no status.json at all.
		},
	}
	got, err := ListWriterStatuses(context.Background(), api, "bucket", "nas")
	if err != nil {
		t.Fatalf("ListWriterStatuses: %v", err)
	}
	if api.gotListPrefix != "nas/writers/" {
		t.Errorf("list prefix = %q, want nas/writers/", api.gotListPrefix)
	}
	// bad (unparseable) and missing (no object) are both skipped.
	if len(got) != 2 {
		t.Fatalf("got %d writers, want 2: %+v", len(got), got)
	}
	byID := map[string]WriterStatus{}
	for _, s := range got {
		byID[s.WriterID] = s
	}
	if !byID["a"].Healthy() {
		t.Errorf("writer a should be healthy")
	}
	if byID["b"].Healthy() {
		t.Errorf("writer b has a failed source; should be unhealthy")
	}
}

func TestListWriterStatuses_SkipsOversizedBody(t *testing.T) {
	huge := bytes.Repeat([]byte("x"), maxStatusBytes+1024)
	api := &fakeReadAPI{
		commonPrefixes: []string{"nas/writers/big/"},
		objects:        map[string][]byte{"nas/writers/big/status.json": huge},
	}
	got, err := ListWriterStatuses(context.Background(), api, "bucket", "nas")
	if err != nil {
		t.Fatalf("ListWriterStatuses: %v", err)
	}
	// A body truncated at the size bound won't parse as JSON, so it's skipped.
	if len(got) != 0 {
		t.Errorf("oversized body should be skipped, got %d writers", len(got))
	}
}

func TestStatusKey(t *testing.T) {
	if got := StatusKey("archives/writers/x"); got != "archives/writers/x/status.json" {
		t.Errorf("StatusKey = %q", got)
	}
}

func TestHealthy_NoSources(t *testing.T) {
	if (WriterStatus{}).Healthy() {
		t.Errorf("a writer with no sources is not healthy")
	}
}
