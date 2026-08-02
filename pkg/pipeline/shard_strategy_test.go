package pipeline

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scttfrdmn/cargoship/pkg/chunking"
	"github.com/scttfrdmn/cargoship/pkg/config"
	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

// TestShardStrategiesMatchConfig pins the two lists together. pkg/config can't
// import pkg/pipeline (the dependency runs the other way), so the accepted set
// is duplicated there; this is what keeps the copy honest. Before #316 the
// config copy omitted "round-robin" — the flag's real default — so a config file
// naming it was rejected by `cargoship config --validate`.
func TestShardStrategiesMatchConfig(t *testing.T) {
	assert.Equal(t, ShardStrategies(), config.ValidShardStrategies(),
		"pkg/config's shard-strategy list has drifted from pkg/pipeline's")

	for _, s := range config.ValidShardStrategies() {
		assert.NoError(t, ValidateShardStrategy(s),
			"config accepts %q but the pipeline rejects it", s)
	}
}

// These tests are deliberately behavioral: each one distinguishes strategies by
// the assignments they produce, so they fail against the pre-#316 code where
// every strategy resolved to chunkID % shardCount. Presence/default assertions
// (the shape of the old cmd tests) passed while the flags did nothing, which is
// how this shipped in the first place.

func chunk(id int, totalSize int64, paths ...string) *chunking.Chunk {
	files := make([]chunking.File, 0, len(paths))
	// Spread totalSize evenly so per-file sizes stay consistent with TotalSize.
	var per int64
	if len(paths) > 0 {
		per = totalSize / int64(len(paths))
	}
	for _, p := range paths {
		files = append(files, chunking.File{Path: p, Size: per})
	}
	return &chunking.Chunk{ID: id, Files: files, TotalSize: totalSize, FileCount: len(files)}
}

func TestNewShardAssigner(t *testing.T) {
	t.Run("empty strategy defaults to round-robin", func(t *testing.T) {
		a, err := newShardAssigner("", 4)
		require.NoError(t, err)
		assert.Equal(t, ShardStrategyRoundRobin, a.strategy)
	})

	t.Run("rejects unknown strategy", func(t *testing.T) {
		_, err := newShardAssigner("magic", 4)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown shard strategy")
	})

	t.Run("rejects non-positive shard count", func(t *testing.T) {
		_, err := newShardAssigner(ShardStrategyHash, 0)
		require.Error(t, err)
	})

	t.Run("accepts every advertised strategy", func(t *testing.T) {
		for _, s := range ShardStrategies() {
			_, err := newShardAssigner(s, 4)
			assert.NoError(t, err, "strategy %q is advertised in --help but rejected", s)
		}
	})
}

func TestValidateShardStrategy(t *testing.T) {
	assert.NoError(t, ValidateShardStrategy(""), "empty means default")
	for _, s := range ShardStrategies() {
		assert.NoError(t, ValidateShardStrategy(s))
	}
	err := ValidateShardStrategy("nope")
	require.Error(t, err)
	// The message must list the alternatives; a bare "invalid" is what sent
	// users to the docs to find a strategy list that was itself wrong.
	assert.Contains(t, err.Error(), ShardStrategyRoundRobin)
	assert.Contains(t, err.Error(), ShardStrategyDirectory)
}

// TestShardAssignerRoundRobin pins the historical behavior so a future strategy
// change can't silently redistribute existing users' uploads.
func TestShardAssignerRoundRobin(t *testing.T) {
	a, err := newShardAssigner(ShardStrategyRoundRobin, 4)
	require.NoError(t, err)

	for id := 0; id < 12; id++ {
		assert.Equal(t, id%4, a.assign(chunk(id, 100, "a.txt")),
			"round-robin must remain chunkID %% shardCount")
	}
}

// TestShardAssignerHashDiffersFromRoundRobin is the core regression test: the
// flag's advertised default was "hash" while the implementation was round-robin.
// If hash ever collapses back to chunkID %% shardCount, this fails.
func TestShardAssignerHashDiffersFromRoundRobin(t *testing.T) {
	const shards = 8
	hash, err := newShardAssigner(ShardStrategyHash, shards)
	require.NoError(t, err)
	rr, err := newShardAssigner(ShardStrategyRoundRobin, shards)
	require.NoError(t, err)

	// Same chunk IDs, distinct contents.
	differences := 0
	for id := 0; id < 32; id++ {
		c := chunk(id, 1000, fmt.Sprintf("dir/file-%d.txt", id))
		if hash.assign(c) != rr.assign(c) {
			differences++
		}
	}
	assert.Greater(t, differences, 0,
		"hash must not be an alias for round-robin — that mismatch is #316")
}

func TestShardAssignerHashIsContentStable(t *testing.T) {
	const shards = 8

	t.Run("same content, different chunk ID, same shard", func(t *testing.T) {
		a, err := newShardAssigner(ShardStrategyHash, shards)
		require.NoError(t, err)
		first := a.assign(chunk(1, 500, "x/a.txt", "x/b.txt"))
		second := a.assign(chunk(99, 500, "x/a.txt", "x/b.txt"))
		assert.Equal(t, first, second, "hash must depend on content, not chunk ID")
	})

	t.Run("file order does not change the shard", func(t *testing.T) {
		a, err := newShardAssigner(ShardStrategyHash, shards)
		require.NoError(t, err)
		forward := a.assign(chunk(1, 600, "a.txt", "b.txt", "c.txt"))
		reverse := a.assign(chunk(1, 600, "c.txt", "b.txt", "a.txt"))
		assert.Equal(t, forward, reverse,
			"scan order must not affect assignment, or two runs of the same upload diverge")
	})

	t.Run("empty chunk is handled", func(t *testing.T) {
		a, err := newShardAssigner(ShardStrategyHash, shards)
		require.NoError(t, err)
		assert.Equal(t, 0, a.assign(chunk(0, 0)))
	})
}

// TestShardAssignerSizeBalances uses a deliberately uneven workload: one large
// chunk followed by many small ones. Round-robin spreads by count and leaves the
// large chunk's shard heaviest; size-based must even out the bytes.
func TestShardAssignerSizeBalances(t *testing.T) {
	const shards = 4
	sizes := []int64{1000, 10, 10, 10, 10, 10, 10, 10}

	size, err := newShardAssigner(ShardStrategySize, shards)
	require.NoError(t, err)
	rr, err := newShardAssigner(ShardStrategyRoundRobin, shards)
	require.NoError(t, err)

	sizeLoad := make([]int64, shards)
	rrLoad := make([]int64, shards)
	for id, s := range sizes {
		c := chunk(id, s, fmt.Sprintf("f%d.bin", id))
		sizeLoad[size.assign(c)] += s
		rrLoad[rr.assign(c)] += s
	}

	assert.Equal(t, sizeLoad, size.shardBytes, "internal accounting must match observed load")

	spread := func(load []int64) int64 {
		min, max := load[0], load[0]
		for _, v := range load[1:] {
			if v < min {
				min = v
			}
			if v > max {
				max = v
			}
		}
		return max - min
	}
	// Size-based can't beat a single dominant chunk, but it must put every
	// small chunk on the other shards rather than continuing to round-robin
	// onto the heavy one.
	assert.Less(t, spread(sizeLoad), spread(rrLoad),
		"least-loaded must produce a tighter byte spread than round-robin")
	assert.Equal(t, int64(0), sizeLoad[0]-1000,
		"the 1000-byte chunk lands alone on shard 0, and nothing else joins it")
}

func TestShardAssignerSizeTiesGoLowest(t *testing.T) {
	a, err := newShardAssigner(ShardStrategySize, 3)
	require.NoError(t, err)
	// All shards start empty, so equal chunks behave like round-robin.
	assert.Equal(t, 0, a.assign(chunk(0, 10, "a")))
	assert.Equal(t, 1, a.assign(chunk(1, 10, "b")))
	assert.Equal(t, 2, a.assign(chunk(2, 10, "c")))
	assert.Equal(t, 0, a.assign(chunk(3, 10, "d")))
}

// TestShardAssignerTypeGroups checks that chunks sharing a predominant content
// type co-locate, and that a different type can land elsewhere.
func TestShardAssignerTypeGroups(t *testing.T) {
	a, err := newShardAssigner(ShardStrategyType, 8)
	require.NoError(t, err)

	goA := a.assign(chunk(0, 100, "src/one.go"))
	goB := a.assign(chunk(7, 100, "other/two.go"))
	assert.Equal(t, goA, goB, "same predominant content type must share a shard")

	// Predominance is by bytes, so a chunk whose bulk is Go code groups with
	// Go even when it contains a stray file of another type.
	mixed := &chunking.Chunk{
		ID: 3,
		Files: []chunking.File{
			{Path: "src/big.go", Size: 10_000},
			{Path: "img/tiny.jpg", Size: 10},
		},
		TotalSize: 10_010,
	}
	assert.Equal(t, goA, a.assign(mixed), "predominance must be weighted by size")
}

func TestShardAssignerDirectoryGroups(t *testing.T) {
	a, err := newShardAssigner(ShardStrategyDirectory, 8)
	require.NoError(t, err)

	first := a.assign(chunk(0, 100, "/data/project-a/x.bin", "/data/project-a/y.bin"))
	second := a.assign(chunk(5, 100, "/data/project-a/z.bin"))
	assert.Equal(t, first, second, "chunks under one directory must share a shard")
}

func TestCommonDirPrefix(t *testing.T) {
	tests := []struct {
		name  string
		paths []string
		want  string
	}{
		{"single file", []string{"/a/b/c.txt"}, "/a/b"},
		{"same dir", []string{"/a/b/c.txt", "/a/b/d.txt"}, "/a/b"},
		{"diverging subdirs", []string{"/a/b/c.txt", "/a/e/f.txt"}, "/a"},
		{
			// Segment-wise comparison: "bc" and "bd" share no segment, so the
			// answer is "/a" and not the string prefix "/a/b".
			"similar sibling names are not a shared prefix",
			[]string{"/a/bc/x.txt", "/a/bd/y.txt"},
			"/a",
		},
		{"nested under shared root", []string{"/a/b/c.txt", "/a/b/d/e.txt"}, "/a/b"},
		{"no shared root", []string{"/x/1.txt", "/y/2.txt"}, ""},
		{"empty", nil, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := make([]chunking.File, 0, len(tt.paths))
			for _, p := range tt.paths {
				files = append(files, chunking.File{Path: p, Size: 1})
			}
			assert.Equal(t, tt.want, commonDirPrefix(files))
		})
	}
}

// TestShardAssignerStaysInRange guards the modulo arithmetic for every strategy,
// including a negative chunk ID (which would panic an unguarded array index).
func TestShardAssignerStaysInRange(t *testing.T) {
	for _, strategy := range ShardStrategies() {
		for _, shards := range []int{1, 3, 8, 32} {
			a, err := newShardAssigner(strategy, shards)
			require.NoError(t, err)

			for _, id := range []int{-7, -1, 0, 1, 5, 1 << 20} {
				got := a.assign(chunk(id, 128, fmt.Sprintf("p/%d.dat", id)))
				assert.GreaterOrEqual(t, got, 0, "%s/%d shards, id %d", strategy, shards, id)
				assert.Less(t, got, shards, "%s/%d shards, id %d", strategy, shards, id)
			}
		}
	}
}

// TestShardStrategyDoesNotAffectRestore pins the invariant that makes changing
// the shard strategy safe for existing archives: readers take each chunk's shard
// and key from the manifest, and nothing recomputes assignment from the chunk ID.
//
// This matters because the strategy is an upload-time choice that is NOT recorded
// in the manifest. If any reader recomputed `chunk.ID % shard_count`, it would
// address the wrong S3 object for every archive written with a non-default
// strategy — turning a tuning flag into a data-loss bug.
func TestShardStrategyDoesNotAffectRestore(t *testing.T) {
	const shards = 8
	files := []chunking.File{{Path: "/data/a.bin", Size: 100}}

	// Two strategies that disagree about where chunk 3 belongs.
	rr, err := newShardAssigner(ShardStrategyRoundRobin, shards)
	require.NoError(t, err)
	dir, err := newShardAssigner(ShardStrategyDirectory, shards)
	require.NoError(t, err)

	c := &chunking.Chunk{ID: 3, Files: files, TotalSize: 100}
	rrShard := rr.assign(c)
	dirShard := dir.assign(c)

	// Whatever each strategy chose, the recorded shard is what a reader uses.
	// Simulate the two manifests a reader could be handed.
	for name, shard := range map[string]int{"round-robin": rrShard, "directory": dirShard} {
		t.Run(name, func(t *testing.T) {
			recorded := manifest.ChunkEntry{
				ID:      3,
				ShardID: shard,
				S3Key:   fmt.Sprintf("uploads/u1/shard-%d/chunk-3.tar.zst", shard),
			}
			// A reader must resolve to the recorded key, not one derived from the
			// chunk ID. With round-robin, chunk 3 → shard 3; if the recorded shard
			// differs, deriving it would produce a key for an object that does not
			// exist.
			assert.Equal(t,
				fmt.Sprintf("uploads/u1/shard-%d/chunk-3.tar.zst", recorded.ShardID),
				manifest.ResolveObjectKey("", "bucket", recorded.S3Key),
				"restore must follow the manifest's recorded shard")
			assert.Contains(t, recorded.S3Key, fmt.Sprintf("shard-%d/", recorded.ShardID),
				"the recorded key and recorded ShardID must agree")
		})
	}

	// And the guard that gives the above its teeth: the strategies really do
	// disagree here, so a reader that recomputed would be wrong for one of them.
	assert.NotEqual(t, rrShard, dirShard,
		"fixture must be one where the strategies disagree, or this test proves nothing")
}
