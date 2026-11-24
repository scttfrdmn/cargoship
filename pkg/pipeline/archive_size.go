package pipeline

import (
	"time"

	"github.com/scttfrdmn/cargoship/pkg/chunking"
	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

// archivedFile is what the archiver actually wrote for a file that changed
// between the scan and the archive.
type archivedFile struct {
	size    int64
	modTime time.Time
}

// setArchived records that the file under key was archived with size and
// modTime rather than the scanned values (#910). Safe for concurrent use.
func (j *Job) setArchived(key string, size int64, modTime time.Time) {
	j.archivedMu.Lock()
	defer j.archivedMu.Unlock()
	if j.archived == nil {
		j.archived = make(map[string]archivedFile)
	}
	j.archived[key] = archivedFile{size: size, modTime: modTime}
}

// boundFiles lists the chunk's files for the manifest, with the size and
// modification time that were archived (#910).
//
// The manifest used to keep the scan-time size of a file that changed before it
// was archived, while the tar held its new length. Frame restore slices the
// manifest size, so a shrunk file was padded with the next file's bytes (with
// --no-file-checksums nothing noticed) and a grown one was cut short. Recording
// what was archived keeps the entry and the tar in agreement; a later sync sees
// the newer modification time and uploads the file again.
func (j *Job) boundFiles() []manifest.ChunkFile {
	out := chunkFiles(j.Chunk.Files)
	j.archivedMu.Lock()
	defer j.archivedMu.Unlock()
	for i, f := range j.Chunk.Files {
		if a, ok := j.archived[fileChecksumKey(f)]; ok {
			out[i].Size = a.size
			out[i].ModTime = a.modTime
		}
	}
	return out
}

// noteArchivedSize records the archived size and modification time of a whole
// (unsplit) file when they differ from the scan.
func noteArchivedSize(job *Job, file chunking.File, size int64, modTime time.Time) {
	if job == nil || file.TotalParts > 1 {
		return
	}
	if size != file.Size || !modTime.Equal(file.ModTime) {
		job.setArchived(fileChecksumKey(file), size, modTime)
	}
}
