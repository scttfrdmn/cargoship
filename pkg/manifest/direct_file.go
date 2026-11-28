package manifest

import "time"

// RecordDirectFile records where a direct-upload file was stored and what was
// sent: its S3 key and shard, the SHA-256 of the bytes sent (empty when
// checksums are off), and the size and modification time of those bytes. A
// file that changed after the scan is stored as it is at upload time, so its
// entry must say so: restore checks a direct object's length against the
// recorded size, and sync compares the recorded modification time (#1015).
func (b *Builder) RecordDirectFile(path string, shardID int, s3Key, checksum string, size int64, modTime time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for i := range b.manifest.Files {
		f := &b.manifest.Files[i]
		if f.Path != path {
			continue
		}
		f.S3Key, f.ShardID, f.ChunkID = s3Key, shardID, 0 // direct mode: no chunking
		if checksum != "" {
			f.Checksum = checksum
		}
		if f.Size != size {
			b.manifest.TotalBytes += size - f.Size
			f.Size = size
		}
		f.ModTime = modTime
		return
	}
}
