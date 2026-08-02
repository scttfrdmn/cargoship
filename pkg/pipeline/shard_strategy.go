package pipeline

import (
	"fmt"
	"hash/fnv"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/scttfrdmn/cargoship/pkg/chunking"
	"github.com/scttfrdmn/cargoship/pkg/compression"
)

// Shard assignment strategies (#316).
//
// These name the ways the archiver can map a chunk onto one of the S3 prefix
// shards it writes to. Before #316 the CLI accepted --shard-strategy and threw
// the value away: every upload used ShardStrategyRoundRobin regardless, while
// the flag's own default advertised "hash".
const (
	// ShardStrategyRoundRobin distributes by chunk ID. Even shard sizes when
	// chunks are uniform, and the cheapest option. This is what CargoShip has
	// always done, and remains the default.
	ShardStrategyRoundRobin = "round-robin"

	// ShardStrategyHash distributes by a hash of the chunk's file paths.
	// Assignment is a pure function of chunk content, so the same input tree
	// lands the same way across runs regardless of scan or completion order.
	ShardStrategyHash = "hash"

	// ShardStrategySize sends each chunk to whichever shard currently holds
	// the fewest bytes, evening out shards when chunk sizes vary widely.
	ShardStrategySize = "size"

	// ShardStrategyType groups chunks by predominant content type, so a shard
	// tends to hold like-compressing data.
	ShardStrategyType = "type"

	// ShardStrategyDirectory groups chunks by their common directory prefix,
	// keeping a source subtree together within a shard.
	ShardStrategyDirectory = "directory"
)

// ShardStrategies lists every valid --shard-strategy value, in help-text order.
func ShardStrategies() []string {
	return []string{
		ShardStrategyRoundRobin,
		ShardStrategyHash,
		ShardStrategySize,
		ShardStrategyType,
		ShardStrategyDirectory,
	}
}

// ValidateShardStrategy reports whether name is a known strategy. An empty
// name is valid and means the default (round-robin).
func ValidateShardStrategy(name string) error {
	if name == "" {
		return nil
	}
	for _, s := range ShardStrategies() {
		if name == s {
			return nil
		}
	}
	return fmt.Errorf("unknown shard strategy %q (valid: %s)", name, strings.Join(ShardStrategies(), ", "))
}

// shardAssigner maps chunks onto shard indices in [0, shardCount).
//
// Assignment only decides which S3 prefix a chunk is uploaded under. It does
// not affect restore: the manifest records each chunk's ShardID, and restore
// reads that rather than recomputing an assignment. Changing strategy can
// therefore never make an existing archive unreadable.
type shardAssigner struct {
	strategy   string
	shardCount int

	// mu guards shardBytes, which the size strategy both reads and writes.
	// Selection happens on every archiver worker goroutine, so least-loaded
	// needs the read and the update to be one atomic step.
	mu         sync.Mutex
	shardBytes []int64
}

// newShardAssigner builds an assigner for the given strategy. An empty
// strategy means round-robin. shardCount must be positive.
func newShardAssigner(strategy string, shardCount int) (*shardAssigner, error) {
	if shardCount <= 0 {
		return nil, fmt.Errorf("shard count must be positive, got %d", shardCount)
	}
	if strategy == "" {
		strategy = ShardStrategyRoundRobin
	}
	if err := ValidateShardStrategy(strategy); err != nil {
		return nil, err
	}
	return &shardAssigner{
		strategy:   strategy,
		shardCount: shardCount,
		shardBytes: make([]int64, shardCount),
	}, nil
}

// assign returns the shard index for a chunk.
func (a *shardAssigner) assign(chunk *chunking.Chunk) int {
	switch a.strategy {
	case ShardStrategyHash:
		return a.modulo(hashFileIdentity(chunk.Files))
	case ShardStrategySize:
		return a.leastLoaded(chunk.TotalSize)
	case ShardStrategyType:
		return a.modulo(hashString(string(predominantContentType(chunk.Files))))
	case ShardStrategyDirectory:
		return a.modulo(hashString(commonDirPrefix(chunk.Files)))
	default: // ShardStrategyRoundRobin
		// chunk.ID can be negative only if a caller hand-builds a Chunk;
		// modulo normalizes rather than panicking on a negative index.
		return a.modulo(uint64(uint32(chunk.ID)))
	}
}

// modulo reduces a hash to a shard index.
func (a *shardAssigner) modulo(h uint64) int {
	return int(h % uint64(a.shardCount))
}

// leastLoaded returns the shard holding the fewest bytes and charges size to
// it. Ties go to the lowest index, so an all-equal start behaves like
// round-robin.
//
// Unlike the hash strategies this is order-dependent: which shard a given
// chunk lands on depends on what has already been assigned. That is inherent
// to load balancing, and harmless because the manifest records the resulting
// ShardID per chunk.
func (a *shardAssigner) leastLoaded(size int64) int {
	a.mu.Lock()
	defer a.mu.Unlock()

	best := 0
	for i := 1; i < a.shardCount; i++ {
		if a.shardBytes[i] < a.shardBytes[best] {
			best = i
		}
	}
	a.shardBytes[best] += size
	return best
}

// hashFileIdentity hashes a chunk's file paths and sizes, sorted so the result
// does not depend on the order the scanner produced them in.
func hashFileIdentity(files []chunking.File) uint64 {
	if len(files) == 0 {
		return 0
	}
	keys := make([]string, 0, len(files))
	for _, f := range files {
		keys = append(keys, fmt.Sprintf("%s:%d", f.Path, f.Size))
	}
	sort.Strings(keys)

	h := fnv.New64a()
	for _, k := range keys {
		_, _ = h.Write([]byte(k))
		_, _ = h.Write([]byte{0}) // separator: "ab"+"c" must not collide with "a"+"bc"
	}
	return h.Sum64()
}

// hashString hashes a grouping key with FNV-1a.
func hashString(s string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	return h.Sum64()
}

// predominantContentType returns the content type accounting for the most
// bytes in a chunk, using Magika metadata when present (#30) and falling back
// to extension-based detection. This mirrors analyzeChunkContentTypes, which
// answers the same question for compression level.
func predominantContentType(files []chunking.File) compression.ContentType {
	sizes := make(map[compression.ContentType]int64, len(files))
	for _, f := range files {
		sizes[compression.DetectContentTypeWithMetadata(f.Path, f.Metadata)] += f.Size
	}

	var predominant compression.ContentType
	var maxSize int64 = -1
	for ct, size := range sizes {
		// Break ties by name so map iteration order can't change the result.
		if size > maxSize || (size == maxSize && ct < predominant) {
			maxSize = size
			predominant = ct
		}
	}
	return predominant
}

// commonDirPrefix returns the longest directory prefix shared by every file in
// a chunk, or "" when they share nothing. Comparison is per path segment:
// "/a/bc" and "/a/bd" share "/a", not "/a/b".
func commonDirPrefix(files []chunking.File) string {
	if len(files) == 0 {
		return ""
	}

	prefix := strings.Split(filepath.ToSlash(filepath.Dir(files[0].Path)), "/")
	for _, f := range files[1:] {
		segs := strings.Split(filepath.ToSlash(filepath.Dir(f.Path)), "/")
		if len(segs) < len(prefix) {
			prefix = prefix[:len(segs)]
		}
		for i := range prefix {
			if prefix[i] != segs[i] {
				prefix = prefix[:i]
				break
			}
		}
		if len(prefix) == 0 {
			return ""
		}
	}
	return strings.Join(prefix, "/")
}
