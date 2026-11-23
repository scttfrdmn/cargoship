package pipeline

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scttfrdmn/cargoship/pkg/chunking"
	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

var t0 = time.Unix(1_700_000_000, 0).UTC()

// storedState is a partial manifest's stored state: chunk 3 holds a and b,
// chunk 7 holds c.
func storedState() *manifest.Manifest {
	return &manifest.Manifest{
		Files: []manifest.FileEntry{
			{Path: "a", Size: 1, ModTime: t0, ChunkID: 3, S3Key: "uploads/u/shard-0/chunk-3.tar.zst"},
			{Path: "b", Size: 2, ModTime: t0, ChunkID: 3, S3Key: "uploads/u/shard-0/chunk-3.tar.zst"},
			{Path: "c", Size: 3, ModTime: t0, ChunkID: 7, S3Key: "uploads/u/shard-1/chunk-7.tar.zst"},
		},
		Chunks: []manifest.ChunkEntry{
			{ID: 3, S3Key: "uploads/u/shard-0/chunk-3.tar.zst", UploadedAt: t0, CompressedSize: 100},
			{ID: 7, S3Key: "uploads/u/shard-1/chunk-7.tar.zst", UploadedAt: t0, CompressedSize: 50},
		},
	}
}

func files(fs ...chunking.File) []chunking.File { return fs }

// TestResumeIndex_StoredChunkFor pins #904: a chunk is reused only when a stored
// chunk holds exactly the same files, unchanged. A matching chunk ID is irrelevant.
func TestResumeIndex_StoredChunkFor(t *testing.T) {
	ix := newResumeIndex(storedState())
	assert.Equal(t, 8, ix.nextChunkID, "new chunks must be numbered above every stored ID")

	a := chunking.File{Path: "a", Size: 1, ModTime: t0}
	b := chunking.File{Path: "b", Size: 2, ModTime: t0}
	c := chunking.File{Path: "c", Size: 3, ModTime: t0}
	tests := []struct {
		name  string
		files []chunking.File
		want  string
	}{
		{"exact set, any order", files(b, a), "uploads/u/shard-0/chunk-3.tar.zst"},
		{"single-file chunk", files(c), "uploads/u/shard-1/chunk-7.tar.zst"},
		{"subset of a stored chunk", files(a), ""},
		{"superset", files(a, b, c), ""},
		{"spans two stored chunks", files(a, c), ""},
		{"modified mtime", files(a, chunking.File{Path: "b", Size: 2, ModTime: t0.Add(time.Second)}), ""},
		{"modified size", files(a, chunking.File{Path: "b", Size: 9, ModTime: t0}), ""},
		{"unknown file", files(chunking.File{Path: "new", Size: 1, ModTime: t0}), ""},
		{"duplicate entry", files(a, a), ""},
		{"empty", nil, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			chunk, ok := ix.storedChunkFor(tc.files)
			assert.Equal(t, tc.want != "", ok)
			assert.Equal(t, tc.want, chunk.S3Key)
		})
	}

	var nilIx *resumeIndex
	_, ok := nilIx.storedChunkFor(files(a))
	assert.False(t, ok)
}

func TestResumeIndex_IgnoresUnuploadedChunksAndCountsFileIDs(t *testing.T) {
	m := storedState()
	m.Chunks[1].UploadedAt = time.Time{} // recorded but never confirmed
	m.Files = append(m.Files, manifest.FileEntry{Path: "z", ChunkID: 12, S3Key: "uploads/u/shard-2/chunk-12.tar.zst"})
	ix := newResumeIndex(m)
	_, ok := ix.storedChunkFor(files(chunking.File{Path: "c", Size: 3, ModTime: t0}))
	assert.False(t, ok, "a chunk without UploadedAt is not stored")
	assert.Equal(t, 13, ix.nextChunkID, "file entries' chunk IDs count too")
}

type fakeHead struct {
	size int64
	err  error
	keys []string
}

func (f *fakeHead) HeadObject(_ context.Context, in *s3.HeadObjectInput, _ ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	f.keys = append(f.keys, aws.ToString(in.Key))
	if f.err != nil {
		return nil, f.err
	}
	return &s3.HeadObjectOutput{ContentLength: aws.Int64(f.size)}, nil
}

func TestAlreadyStored(t *testing.T) {
	job := &Job{S3Key: "uploads/u/shard-2/chunk-9.tar.zst", Chunk: chunking.Chunk{ID: 3, Files: files(
		chunking.File{Path: "a", Size: 1, ModTime: t0}, chunking.File{Path: "b", Size: 2, ModTime: t0})}}
	other := &Job{Chunk: chunking.Chunk{ID: 3, Files: files(chunking.File{Path: "a", Size: 1, ModTime: t0})}}
	notFound := &types.NotFound{}

	tests := []struct {
		name         string
		resume, skip bool
		job          *Job
		head         *fakeHead
		want         bool
		wantErr      bool
	}{
		{name: "not resuming", job: job, head: &fakeHead{}},
		{name: "matching files", resume: true, job: job, head: &fakeHead{}, want: true},
		{name: "same chunk ID, different files", resume: true, job: other, head: &fakeHead{}},
		{name: "skip-existing, object present", resume: true, skip: true, job: job, head: &fakeHead{size: 100}, want: true},
		{name: "skip-existing, wrong size", resume: true, skip: true, job: job, head: &fakeHead{size: 7}},
		{name: "skip-existing, object gone", resume: true, skip: true, job: job, head: &fakeHead{err: notFound}},
		{name: "skip-existing, head fails", resume: true, skip: true, job: job, head: &fakeHead{err: errors.New("boom")}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := &Pipeline{config: &PipelineConfig{ResumeMode: tc.resume, SkipExisting: tc.skip}}
			if tc.resume {
				p.resumeIdx = newResumeIndex(storedState())
			}
			got, err := p.alreadyStored(context.Background(), tc.job, tc.head, "bkt", "pfx")
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
			if tc.skip && len(tc.head.keys) > 0 {
				assert.Equal(t, "pfx/uploads/u/shard-0/chunk-3.tar.zst", tc.head.keys[0],
					"the STORED chunk's object is checked, not the job's own key")
			}
		})
	}
}

func TestRefuseCompletedResume(t *testing.T) {
	ctx := context.Background()
	require.NoError(t, refuseCompletedResume(ctx, &fakeHead{err: &types.NotFound{}}, "b", "p", "u"))

	err := refuseCompletedResume(ctx, &fakeHead{}, "b", "p", "u")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already completed")
	assert.Contains(t, err.Error(), "p/uploads/u/manifest.json.gz")

	err = refuseCompletedResume(ctx, &fakeHead{err: errors.New("AccessDenied")}, "b", "p", "u")
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "s3:ListBucket"), err.Error())
}
