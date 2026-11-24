package pipeline

import (
	"context"
	"fmt"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/scttfrdmn/cargoship/pkg/chunking"
	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

// headObjectAPI is the one S3 call the resume skip check needs.
type headObjectAPI interface {
	HeadObject(ctx context.Context, in *s3.HeadObjectInput, opts ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
}

// resumeIndex is a snapshot of what a resumed upload had already stored, taken
// from its partial manifest before the re-scan starts (#904).
//
// Resume used to skip a chunk whenever a stored chunk had the same ID. Chunk IDs
// come from a per-run counter shared by concurrent scan batches, so they are not
// stable between runs: the same ID can name a different set of files. Skipping
// on an ID match left files bound to objects that do not hold them. The index lets
// a chunk be skipped only when a stored chunk holds exactly the same files, each
// with the same size and modification time.
type resumeIndex struct {
	entries  map[string]manifest.FileEntry  // path#part -> stored entry
	perChunk map[string]int                 // chunk S3Key -> stored entries bound to it
	chunks   map[string]manifest.ChunkEntry // uploaded chunks by S3Key
	// nextChunkID is above every chunk ID the stored state uses, so a new chunk's
	// key (derived from its ID) can never overwrite a stored chunk object.
	nextChunkID int
}

func chunkFileKey(path string, part int) string { return path + "#" + strconv.Itoa(part) }

// newResumeIndex indexes the stored state of m, a manifest built from a partial
// manifest (whose never-stored entries NewBuilderFromExisting has already dropped).
func newResumeIndex(m *manifest.Manifest) *resumeIndex {
	ix := &resumeIndex{
		entries:  make(map[string]manifest.FileEntry, len(m.Files)),
		perChunk: make(map[string]int),
		chunks:   make(map[string]manifest.ChunkEntry, len(m.Chunks)),
	}
	maxID := -1
	for _, c := range m.Chunks {
		if c.ID > maxID {
			maxID = c.ID
		}
		if c.S3Key != "" && !c.UploadedAt.IsZero() {
			ix.chunks[c.S3Key] = c
		}
	}
	for _, f := range m.Files {
		if f.ChunkID > maxID {
			maxID = f.ChunkID
		}
		if f.S3Key == "" {
			continue
		}
		ix.entries[chunkFileKey(f.Path, f.PartIndex)] = f
		ix.perChunk[f.S3Key]++
	}
	ix.nextChunkID = maxID + 1
	return ix
}

// storedChunkFor returns the stored chunk holding exactly files, unchanged.
func (ix *resumeIndex) storedChunkFor(files []chunking.File) (manifest.ChunkEntry, bool) {
	if ix == nil || len(files) == 0 {
		return manifest.ChunkEntry{}, false
	}
	first, ok := ix.entries[chunkFileKey(files[0].Path, files[0].PartIndex)]
	if !ok {
		return manifest.ChunkEntry{}, false
	}
	chunk, ok := ix.chunks[first.S3Key]
	if !ok || ix.perChunk[chunk.S3Key] != len(files) {
		return manifest.ChunkEntry{}, false
	}
	seen := make(map[string]bool, len(files))
	for _, f := range files {
		key := chunkFileKey(f.Path, f.PartIndex)
		e, ok := ix.entries[key]
		if !ok || seen[key] || e.S3Key != chunk.S3Key || e.Size != f.Size || !e.ModTime.Equal(f.ModTime) ||
			e.Offset != f.Offset || e.Length != f.Length {
			return manifest.ChunkEntry{}, false
		}
		seen[key] = true
	}
	return chunk, true
}

// alreadyStored reports whether a resumed run may skip uploading job, because a
// stored chunk already holds exactly its files (#904). With SkipExisting the stored
// object must also still exist in S3 at its recorded size. Existence alone was
// never enough: under unstable chunk IDs, the object at a job's own key may hold
// other files.
func (p *Pipeline) alreadyStored(ctx context.Context, job *Job, client headObjectAPI, bucket, prefix string) (bool, error) {
	if p == nil || p.config == nil || !p.config.ResumeMode {
		return false, nil
	}
	chunk, ok := p.resumeIdx.storedChunkFor(job.Chunk.Files)
	if !ok {
		return false, nil
	}
	if !p.config.SkipExisting {
		return true, nil
	}
	key := objectKeyFor(prefix, chunk.S3Key)
	out, err := client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	if err != nil {
		if isNoSuchKeyErr(err) {
			return false, nil // recorded but gone: upload it again
		}
		return false, fmt.Errorf("check stored chunk %s: %w", key, err)
	}
	if chunk.CompressedSize > 0 && aws.ToInt64(out.ContentLength) != chunk.CompressedSize {
		return false, nil
	}
	return true, nil
}

// chunkFiles lists job's files in the form BindChunkFiles takes.
func chunkFiles(files []chunking.File) []manifest.ChunkFile {
	out := make([]manifest.ChunkFile, len(files))
	for i, f := range files {
		out[i] = manifest.ChunkFile{Path: f.Path, PartIndex: f.PartIndex, Size: f.Size, ModTime: f.ModTime}
	}
	return out
}
