package pipeline

import (
	"sync"
	"testing"

	"github.com/scttfrdmn/cargoship/pkg/chunking"
)

// TestAssignGlobalChunkIDsUniqueAcrossConcurrentBatches guards the data-loss bug
// where concurrently-processed scan batches each numbered their chunks 0..k-1,
// so the "chunk-{id}" S3 keys collided across batches and later chunk objects
// overwrote earlier ones in S3 (silently losing files on restore). Every chunk
// ID — and therefore every S3 key — must be globally unique.
func TestAssignGlobalChunkIDsUniqueAcrossConcurrentBatches(t *testing.T) {
	s := &ScannerStage{}
	const batches, perBatch = 64, 16

	var wg sync.WaitGroup
	got := make([][]int, batches)
	for b := 0; b < batches; b++ {
		wg.Add(1)
		go func(b int) {
			defer wg.Done()
			// The chunker numbers each batch's chunks 0..perBatch-1 locally.
			chunks := make([]chunking.Chunk, perBatch)
			for i := range chunks {
				chunks[i].ID = i
			}
			s.assignGlobalChunkIDs(chunks)
			ids := make([]int, perBatch)
			for i := range chunks {
				ids[i] = chunks[i].ID
			}
			got[b] = ids
		}(b)
	}
	wg.Wait()

	seen := make(map[int]bool, batches*perBatch)
	for _, ids := range got {
		for _, id := range ids {
			if seen[id] {
				t.Fatalf("duplicate chunk ID %d across batches — S3 keys would collide", id)
			}
			seen[id] = true
		}
	}
	if len(seen) != batches*perBatch {
		t.Fatalf("expected %d unique chunk IDs, got %d", batches*perBatch, len(seen))
	}
}

// TestAssignGlobalChunkIDsEmpty is a no-op guard (no allocation, no panic).
func TestAssignGlobalChunkIDsEmpty(t *testing.T) {
	s := &ScannerStage{}
	s.assignGlobalChunkIDs(nil)
	if s.chunkIDSeq != 0 {
		t.Fatalf("empty batch must not consume IDs, got seq=%d", s.chunkIDSeq)
	}
}
