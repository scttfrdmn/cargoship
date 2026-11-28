package manifest

import (
	"testing"
	"time"
)

// TestRecordDirectFile pins #1015's manifest side: a direct file's entry takes
// the key, checksum, size and modification time of what was sent, and the
// upload's byte total follows the size.
func TestRecordDirectFile(t *testing.T) {
	b, err := NewBuilder("u", "/src", "b", "p", "r")
	if err != nil {
		t.Fatal(err)
	}
	scanned := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	b.AddFile(FileEntry{Path: "/src/a", Size: 10, ModTime: scanned, ChunkID: -1})
	b.AddFile(FileEntry{Path: "/src/b", Size: 5, ModTime: scanned})

	sent := scanned.Add(time.Minute)
	b.RecordDirectFile("/src/a", 0, "p/uploads/u/files/a", "abc", 12, sent)
	b.RecordDirectFile("/src/b", 0, "p/uploads/u/files/b", "", 5, scanned)
	b.RecordDirectFile("/src/missing", 0, "k", "x", 1, sent) // no entry: no-op

	m := b.Build()
	a := m.Files[0]
	if a.S3Key != "p/uploads/u/files/a" || a.Checksum != "abc" || a.Size != 12 || !a.ModTime.Equal(sent) || a.ChunkID != 0 {
		t.Fatalf("entry a = %+v", a)
	}
	if m.Files[1].Checksum != "" || m.Files[1].Size != 5 {
		t.Fatalf("entry b = %+v (an empty checksum must leave none)", m.Files[1])
	}
	if m.TotalBytes != 17 {
		t.Fatalf("TotalBytes = %d, want 17 (follows the sent size)", m.TotalBytes)
	}
}
