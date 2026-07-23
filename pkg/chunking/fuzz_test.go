package chunking

import (
	"testing"
)

// FuzzCreateChunks feeds synthetic file metadata to the compressed-aware chunker
// and asserts the core invariants that must hold for any input: no file is lost
// or duplicated, every file lands in exactly one chunk, and per-chunk bookkeeping
// (FileCount, TotalSize) matches the files actually placed. The chunker reads
// file bodies for compression estimation but falls back to the raw Size on any
// read error, so nonexistent paths exercise the fallback path deterministically.
func FuzzCreateChunks(f *testing.F) {
	// Seeds: count of files, a size selector, and a path prefix.
	f.Add(0, int64(0), "a")
	f.Add(1, int64(1024), "file")
	f.Add(50, int64(10*1024*1024), "data/x")
	f.Add(10, int64(-1), "neg") // negative sizes must not panic

	f.Fuzz(func(t *testing.T, count int, size int64, prefix string) {
		// Bound the work so the fuzzer stays fast; large counts add no coverage.
		if count < 0 {
			count = -count
		}
		count %= 500

		files := make([]File, 0, count)
		var wantTotal int64
		for i := 0; i < count; i++ {
			// Vary sizes a little so chunk boundaries actually get exercised.
			s := size + int64(i)
			files = append(files, File{
				Path: prefix + "/" + itoa(i),
				Size: s,
			})
			wantTotal += s
		}

		chunker, err := NewCompressedAwareChunker()
		if err != nil {
			t.Skipf("chunker unavailable: %v", err)
		}

		chunks, err := chunker.CreateChunks(files)
		if err != nil {
			// A returned error is acceptable; a panic is not.
			return
		}

		var gotFiles int
		var gotTotal int64
		for _, c := range chunks {
			if c.FileCount != len(c.Files) {
				t.Fatalf("chunk %d: FileCount=%d but len(Files)=%d", c.ID, c.FileCount, len(c.Files))
			}
			var chunkSum int64
			for _, fl := range c.Files {
				chunkSum += fl.Size
			}
			if chunkSum != c.TotalSize {
				t.Fatalf("chunk %d: TotalSize=%d but files sum to %d", c.ID, c.TotalSize, chunkSum)
			}
			gotFiles += c.FileCount
			gotTotal += c.TotalSize
		}

		if gotFiles != len(files) {
			t.Fatalf("file count not conserved: in=%d out=%d", len(files), gotFiles)
		}
		if gotTotal != wantTotal {
			t.Fatalf("total size not conserved: in=%d out=%d", wantTotal, gotTotal)
		}
	})
}

// itoa is a tiny allocation-light int→string for building distinct paths.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
