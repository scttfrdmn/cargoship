package staging

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"
)

// ChunkDeduplicator provides intelligent data deduplication at chunk level.
type ChunkDeduplicator struct {
	chunkHashes      map[string]*ChunkHashInfo    // Hash -> chunk info
	sizeIndex        map[int64][]*ChunkHashInfo   // Size -> list of chunks
	contentIndex     map[string][]*ChunkHashInfo  // Content type -> list of chunks
	recentAccess     map[string]time.Time         // Hash -> last access time
	duplicateStats   *DeduplicationStats
	config          *DeduplicationConfig
	rollingHasher   *RollingHasher
	weakHashMap     map[uint64][]*ChunkHashInfo   // Weak hash -> strong hash candidates
	mu              sync.RWMutex
}

// DeduplicationConfig configures chunk deduplication behavior.
type DeduplicationConfig struct {
	// Hash algorithm settings
	EnableWeakHashing       bool          `yaml:"enable_weak_hashing" json:"enable_weak_hashing"`
	EnableContentAwareness  bool          `yaml:"enable_content_awareness" json:"enable_content_awareness"`
	ChunkSizeThreshold      int64         `yaml:"chunk_size_threshold" json:"chunk_size_threshold"`
	
	// Performance settings
	MaxHashCacheSize        int           `yaml:"max_hash_cache_size" json:"max_hash_cache_size"`
	HashCacheExpirationTime time.Duration `yaml:"hash_cache_expiration_time" json:"hash_cache_expiration_time"`
	EnableAsyncHashing      bool          `yaml:"enable_async_hashing" json:"enable_async_hashing"`
	
	// Similarity detection
	EnableSimilarityDetection bool          `yaml:"enable_similarity_detection" json:"enable_similarity_detection"`
	SimilarityThreshold      float64       `yaml:"similarity_threshold" json:"similarity_threshold"`
	RollingWindowSize        int           `yaml:"rolling_window_size" json:"rolling_window_size"`
	
	// Storage optimization
	EnableDeltaCompression   bool          `yaml:"enable_delta_compression" json:"enable_delta_compression"`
	MaxDeltaChainLength     int           `yaml:"max_delta_chain_length" json:"max_delta_chain_length"`
}

// DefaultDeduplicationConfig returns sensible defaults for chunk deduplication.
func DefaultDeduplicationConfig() *DeduplicationConfig {
	return &DeduplicationConfig{
		EnableWeakHashing:         true,
		EnableContentAwareness:    true,
		ChunkSizeThreshold:        1024,  // 1KB minimum for dedup
		MaxHashCacheSize:          10000, // 10K hash entries
		HashCacheExpirationTime:   time.Hour * 24, // 24h cache expiration
		EnableAsyncHashing:        true,
		EnableSimilarityDetection: true,
		SimilarityThreshold:       0.8,  // 80% similarity
		RollingWindowSize:         16,   // 16 byte rolling window
		EnableDeltaCompression:    true,
		MaxDeltaChainLength:       5,    // Max 5 delta levels
	}
}

// ChunkHashInfo stores hash and metadata for a chunk.
type ChunkHashInfo struct {
	StrongHash       string
	WeakHash         uint64
	Size             int64
	ContentType      string
	CreatedAt        time.Time
	LastAccessedAt   time.Time
	AccessCount      int64
	ChunkData        []byte  // Stored for delta compression
	SimilarityVector []float64  // Feature vector for similarity detection
	DeltaParent      string  // Hash of parent chunk if this is a delta
	DeltaChildren    []string // Hashes of child delta chunks
	CompressionRatio float64
	Entropy          float64
}

// DeduplicationStats tracks deduplication performance metrics.
type DeduplicationStats struct {
	TotalChunksProcessed     int64
	DuplicateChunksFound     int64
	SimilarChunksFound       int64
	BytesSaved               int64
	DeduplicationRatio       float64
	AverageHashTime          time.Duration
	CacheHitRate             float64
	WeakHashCollisions       int64
	StrongHashCollisions     int64
	SimilarityComputations   int64
	DeltaCompressions        int64
	mu                       sync.RWMutex
}

// DeduplicationResult represents the result of chunk deduplication analysis.
type DeduplicationResult struct {
	IsDuplicate       bool
	IsSemanticDuplicate bool
	ExistingHash      string
	SimilarityScore   float64
	DeltaParent       string
	DeltaSize         int64
	BytesSaved        int64
	RecommendedAction DeduplicationAction
}

// DeduplicationAction specifies what action to take for a chunk.
type DeduplicationAction int

const (
	ActionStore DeduplicationAction = iota
	ActionDuplicate
	ActionDeltaCompress
	ActionSimilarityReplace
)

// RollingHasher implements a rolling hash for similarity detection.
type RollingHasher struct {
	windowSize int
	polynomial uint64
}

// NewChunkDeduplicator creates a new chunk deduplicator.
func NewChunkDeduplicator(config *DeduplicationConfig) *ChunkDeduplicator {
	if config == nil {
		config = DefaultDeduplicationConfig()
	}

	return &ChunkDeduplicator{
		chunkHashes:    make(map[string]*ChunkHashInfo),
		sizeIndex:      make(map[int64][]*ChunkHashInfo),
		contentIndex:   make(map[string][]*ChunkHashInfo),
		recentAccess:   make(map[string]time.Time),
		duplicateStats: &DeduplicationStats{},
		config:         config,
		rollingHasher:  NewRollingHasher(config.RollingWindowSize),
		weakHashMap:    make(map[uint64][]*ChunkHashInfo),
	}
}

// NewRollingHasher creates a new rolling hasher.
func NewRollingHasher(windowSize int) *RollingHasher {
	return &RollingHasher{
		windowSize: windowSize,
		polynomial: 0x1021, // CRC-16 polynomial
	}
}

// AnalyzeChunk performs comprehensive deduplication analysis on a chunk.
func (cd *ChunkDeduplicator) AnalyzeChunk(data []byte, contentType string) *DeduplicationResult {
	start := time.Now()
	defer func() {
		cd.duplicateStats.mu.Lock()
		cd.duplicateStats.TotalChunksProcessed++
		cd.duplicateStats.AverageHashTime = time.Since(start)
		cd.duplicateStats.mu.Unlock()
	}()

	// Skip deduplication for very small chunks
	if int64(len(data)) < cd.config.ChunkSizeThreshold {
		return &DeduplicationResult{
			RecommendedAction: ActionStore,
		}
	}

	// Compute hashes
	strongHash := cd.computeStrongHash(data)
	var weakHash uint64
	if cd.config.EnableWeakHashing {
		weakHash = cd.computeWeakHash(data)
	}

	cd.mu.RLock()
	
	// Check for exact duplicate
	if existing, found := cd.chunkHashes[strongHash]; found {
		cd.mu.RUnlock()
		cd.recordDuplicate(existing)
		return &DeduplicationResult{
			IsDuplicate:       true,
			ExistingHash:      strongHash,
			BytesSaved:        int64(len(data)),
			RecommendedAction: ActionDuplicate,
		}
	}

	// Check for weak hash collisions (potential similar chunks)
	var candidates []*ChunkHashInfo
	if cd.config.EnableWeakHashing {
		candidates = cd.weakHashMap[weakHash]
	}

	// Add size-based candidates for better similarity detection
	if cd.config.EnableSimilarityDetection {
		sizeRange := int64(len(data))
		lowerBound := sizeRange - (sizeRange / 10) // ±10% size variance
		upperBound := sizeRange + (sizeRange / 10)
		
		for size := lowerBound; size <= upperBound; size++ {
			if chunks, exists := cd.sizeIndex[size]; exists {
				candidates = append(candidates, chunks...)
			}
		}
	}

	cd.mu.RUnlock()

	// Analyze similarity with candidates
	var bestCandidate *ChunkHashInfo
	var bestSimilarity float64
	
	if cd.config.EnableSimilarityDetection && len(candidates) > 0 {
		bestCandidate, bestSimilarity = cd.findMostSimilar(data, candidates, contentType)
	}

	// Determine action based on analysis
	result := &DeduplicationResult{
		IsDuplicate: false,
		SimilarityScore: bestSimilarity,
	}

	if bestSimilarity >= cd.config.SimilarityThreshold {
		result.IsSemanticDuplicate = true
		result.ExistingHash = bestCandidate.StrongHash
		
		if cd.config.EnableDeltaCompression && bestCandidate.DeltaChildren != nil &&
			len(bestCandidate.DeltaChildren) < cd.config.MaxDeltaChainLength {
			// Use delta compression
			deltaSize := cd.estimateDeltaSize(data, bestCandidate.ChunkData)
			result.DeltaParent = bestCandidate.StrongHash
			result.DeltaSize = deltaSize
			result.BytesSaved = int64(len(data)) - deltaSize
			result.RecommendedAction = ActionDeltaCompress
		} else {
			// Use similarity replacement
			result.BytesSaved = int64(float64(len(data)) * bestSimilarity)
			result.RecommendedAction = ActionSimilarityReplace
		}
		
		cd.recordSimilarityHit(bestCandidate)
	} else {
		result.RecommendedAction = ActionStore
	}

	// Store new chunk if it's unique enough
	if result.RecommendedAction == ActionStore || result.RecommendedAction == ActionDeltaCompress {
		cd.storeChunkHash(data, strongHash, weakHash, contentType, result.DeltaParent)
	}

	return result
}

// computeStrongHash computes a cryptographically strong hash of the data.
func (cd *ChunkDeduplicator) computeStrongHash(data []byte) string {
	hasher := sha256.New()
	hasher.Write(data)
	return hex.EncodeToString(hasher.Sum(nil))
}

// computeWeakHash computes a fast, weak hash for quick comparison.
func (cd *ChunkDeduplicator) computeWeakHash(data []byte) uint64 {
	return cd.rollingHasher.Hash(data)
}

// Hash computes a rolling hash for the given data.
func (rh *RollingHasher) Hash(data []byte) uint64 {
	if len(data) == 0 {
		return 0
	}
	
	var hash uint64 = 1 // Start with non-zero value
	
	// Use a simpler but more effective polynomial rolling hash
	for _, b := range data {
		hash = hash*rh.polynomial + uint64(b)
	}
	
	return hash
}

// findMostSimilar finds the most similar chunk from candidates.
func (cd *ChunkDeduplicator) findMostSimilar(data []byte, candidates []*ChunkHashInfo, contentType string) (*ChunkHashInfo, float64) {
	var bestCandidate *ChunkHashInfo
	var bestSimilarity float64

	cd.duplicateStats.mu.Lock()
	cd.duplicateStats.SimilarityComputations += int64(len(candidates))
	cd.duplicateStats.mu.Unlock()

	for _, candidate := range candidates {
		// Content type matching bonus
		if cd.config.EnableContentAwareness && candidate.ContentType == contentType {
			similarity := cd.computeSimilarity(data, candidate.ChunkData)
			similarity *= 1.1 // 10% bonus for matching content type
			
			if similarity > bestSimilarity {
				bestSimilarity = similarity
				bestCandidate = candidate
			}
		} else if !cd.config.EnableContentAwareness {
			similarity := cd.computeSimilarity(data, candidate.ChunkData)
			if similarity > bestSimilarity {
				bestSimilarity = similarity
				bestCandidate = candidate
			}
		}
	}

	return bestCandidate, bestSimilarity
}

// computeSimilarity computes similarity between two chunks using various metrics.
func (cd *ChunkDeduplicator) computeSimilarity(data1, data2 []byte) float64 {
	if len(data1) == 0 || len(data2) == 0 {
		return 0.0
	}

	// Use Jaccard similarity with rolling hash shingles
	shingles1 := cd.generateShingles(data1)
	shingles2 := cd.generateShingles(data2)
	
	return cd.jaccardSimilarity(shingles1, shingles2)
}

// generateShingles generates hash shingles for similarity comparison.
func (cd *ChunkDeduplicator) generateShingles(data []byte) map[uint64]bool {
	shingles := make(map[uint64]bool)
	windowSize := cd.config.RollingWindowSize
	
	if len(data) < windowSize {
		return shingles
	}
	
	// Use a simple polynomial rolling hash for each window
	for i := 0; i <= len(data)-windowSize; i++ {
		window := data[i : i+windowSize]
		hash := cd.simpleHash(window)
		shingles[hash] = true
	}
	
	return shingles
}

// simpleHash computes a simple polynomial hash for a byte slice.
func (cd *ChunkDeduplicator) simpleHash(data []byte) uint64 {
	var hash uint64 = 5381 // djb2 hash initial value
	
	for _, b := range data {
		hash = ((hash << 5) + hash) + uint64(b) // hash * 33 + b
	}
	
	return hash
}

// jaccardSimilarity computes Jaccard similarity between two sets of shingles.
func (cd *ChunkDeduplicator) jaccardSimilarity(set1, set2 map[uint64]bool) float64 {
	if len(set1) == 0 && len(set2) == 0 {
		return 1.0
	}
	
	intersection := 0
	union := len(set1)
	
	for hash := range set2 {
		if set1[hash] {
			intersection++
		} else {
			union++
		}
	}
	
	if union == 0 {
		return 0.0
	}
	
	return float64(intersection) / float64(union)
}

// estimateDeltaSize estimates the size of delta compression between two chunks.
func (cd *ChunkDeduplicator) estimateDeltaSize(newData, baseData []byte) int64 {
	// Simple estimation: count differing bytes
	minLen := len(newData)
	if len(baseData) < minLen {
		minLen = len(baseData)
	}
	
	differences := 0
	for i := 0; i < minLen; i++ {
		if newData[i] != baseData[i] {
			differences++
		}
	}
	
	// Add size difference
	differences += abs(len(newData) - len(baseData))
	
	// Add some overhead for delta metadata
	return int64(differences) + 64
}

// storeChunkHash stores a new chunk hash in the deduplicator.
func (cd *ChunkDeduplicator) storeChunkHash(data []byte, strongHash string, weakHash uint64, contentType, deltaParent string) {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	
	now := time.Now()
	
	chunkInfo := &ChunkHashInfo{
		StrongHash:       strongHash,
		WeakHash:         weakHash,
		Size:             int64(len(data)),
		ContentType:      contentType,
		CreatedAt:        now,
		LastAccessedAt:   now,
		AccessCount:      1,
		DeltaParent:      deltaParent,
		DeltaChildren:    make([]string, 0),
		CompressionRatio: 1.0, // Will be updated later
		Entropy:          cd.calculateEntropy(data),
	}
	
	// Store chunk data for delta compression if enabled
	if cd.config.EnableDeltaCompression {
		chunkInfo.ChunkData = make([]byte, len(data))
		copy(chunkInfo.ChunkData, data)
	}
	
	// Update parent's children list if this is a delta
	if deltaParent != "" {
		if parent, exists := cd.chunkHashes[deltaParent]; exists {
			parent.DeltaChildren = append(parent.DeltaChildren, strongHash)
		}
	}
	
	// Store in main hash map
	cd.chunkHashes[strongHash] = chunkInfo
	cd.recentAccess[strongHash] = now
	
	// Update indexes
	cd.sizeIndex[chunkInfo.Size] = append(cd.sizeIndex[chunkInfo.Size], chunkInfo)
	if cd.config.EnableWeakHashing {
		cd.weakHashMap[weakHash] = append(cd.weakHashMap[weakHash], chunkInfo)
	}
	if cd.config.EnableContentAwareness {
		cd.contentIndex[contentType] = append(cd.contentIndex[contentType], chunkInfo)
	}
	
	// Cleanup if cache is too large
	if len(cd.chunkHashes) > cd.config.MaxHashCacheSize {
		cd.cleanupOldEntries()
	}
}

// calculateEntropy calculates Shannon entropy of the data.
func (cd *ChunkDeduplicator) calculateEntropy(data []byte) float64 {
	if len(data) == 0 {
		return 0.0
	}
	
	// Count byte frequencies
	freq := make(map[byte]int)
	for _, b := range data {
		freq[b]++
	}
	
	// Calculate entropy
	entropy := 0.0
	length := float64(len(data))
	
	for _, count := range freq {
		if count > 0 {
			p := float64(count) / length
			entropy -= p * logBase2(p)
		}
	}
	
	return entropy
}

// logBase2 calculates log base 2.
func logBase2(x float64) float64 {
	return logE(x) / logE(2)
}

// logE calculates natural logarithm (approximation).
func logE(x float64) float64 {
	if x <= 0 {
		return 0
	}
	// Simple approximation for demonstration
	// In practice, use math.Log
	result := 0.0
	for x > 1 {
		result++
		x /= 2.718281828
	}
	return result
}

// recordDuplicate records statistics for a duplicate chunk.
func (cd *ChunkDeduplicator) recordDuplicate(existing *ChunkHashInfo) {
	cd.duplicateStats.mu.Lock()
	defer cd.duplicateStats.mu.Unlock()
	
	cd.duplicateStats.DuplicateChunksFound++
	cd.duplicateStats.BytesSaved += existing.Size
	cd.duplicateStats.CacheHitRate = float64(cd.duplicateStats.DuplicateChunksFound) / float64(cd.duplicateStats.TotalChunksProcessed)
	
	// Update access tracking
	existing.AccessCount++
	existing.LastAccessedAt = time.Now()
}

// recordSimilarityHit records statistics for a similarity match.
func (cd *ChunkDeduplicator) recordSimilarityHit(similar *ChunkHashInfo) {
	cd.duplicateStats.mu.Lock()
	defer cd.duplicateStats.mu.Unlock()
	
	cd.duplicateStats.SimilarChunksFound++
	cd.duplicateStats.DeltaCompressions++
	
	// Update access tracking
	similar.AccessCount++
	similar.LastAccessedAt = time.Now()
}

// cleanupOldEntries removes old entries from the cache to maintain size limits.
func (cd *ChunkDeduplicator) cleanupOldEntries() {
	// Remove least recently used entries
	cutoff := time.Now().Add(-cd.config.HashCacheExpirationTime)
	
	toRemove := make([]string, 0)
	for hash, accessTime := range cd.recentAccess {
		if accessTime.Before(cutoff) {
			toRemove = append(toRemove, hash)
		}
	}
	
	// Remove entries
	for _, hash := range toRemove {
		if chunkInfo, exists := cd.chunkHashes[hash]; exists {
			// Remove from indexes
			cd.removeFromSizeIndex(chunkInfo)
			cd.removeFromWeakHashMap(chunkInfo)
			cd.removeFromContentIndex(chunkInfo)
			
			// Remove from main maps
			delete(cd.chunkHashes, hash)
			delete(cd.recentAccess, hash)
		}
	}
}

// removeFromSizeIndex removes a chunk from the size index.
func (cd *ChunkDeduplicator) removeFromSizeIndex(chunkInfo *ChunkHashInfo) {
	if chunks, exists := cd.sizeIndex[chunkInfo.Size]; exists {
		for i, chunk := range chunks {
			if chunk.StrongHash == chunkInfo.StrongHash {
				cd.sizeIndex[chunkInfo.Size] = append(chunks[:i], chunks[i+1:]...)
				break
			}
		}
		
		// Remove the slice if empty
		if len(cd.sizeIndex[chunkInfo.Size]) == 0 {
			delete(cd.sizeIndex, chunkInfo.Size)
		}
	}
}

// removeFromWeakHashMap removes a chunk from the weak hash map.
func (cd *ChunkDeduplicator) removeFromWeakHashMap(chunkInfo *ChunkHashInfo) {
	if chunks, exists := cd.weakHashMap[chunkInfo.WeakHash]; exists {
		for i, chunk := range chunks {
			if chunk.StrongHash == chunkInfo.StrongHash {
				cd.weakHashMap[chunkInfo.WeakHash] = append(chunks[:i], chunks[i+1:]...)
				break
			}
		}
		
		// Remove the slice if empty
		if len(cd.weakHashMap[chunkInfo.WeakHash]) == 0 {
			delete(cd.weakHashMap, chunkInfo.WeakHash)
		}
	}
}

// removeFromContentIndex removes a chunk from the content index.
func (cd *ChunkDeduplicator) removeFromContentIndex(chunkInfo *ChunkHashInfo) {
	if chunks, exists := cd.contentIndex[chunkInfo.ContentType]; exists {
		for i, chunk := range chunks {
			if chunk.StrongHash == chunkInfo.StrongHash {
				cd.contentIndex[chunkInfo.ContentType] = append(chunks[:i], chunks[i+1:]...)
				break
			}
		}
		
		// Remove the slice if empty
		if len(cd.contentIndex[chunkInfo.ContentType]) == 0 {
			delete(cd.contentIndex, chunkInfo.ContentType)
		}
	}
}

// GetStats returns current deduplication statistics.
func (cd *ChunkDeduplicator) GetStats() *DeduplicationStats {
	cd.duplicateStats.mu.RLock()
	defer cd.duplicateStats.mu.RUnlock()
	
	// Calculate deduplication ratio
	stats := &DeduplicationStats{
		TotalChunksProcessed:   cd.duplicateStats.TotalChunksProcessed,
		DuplicateChunksFound:   cd.duplicateStats.DuplicateChunksFound,
		SimilarChunksFound:     cd.duplicateStats.SimilarChunksFound,
		BytesSaved:             cd.duplicateStats.BytesSaved,
		AverageHashTime:        cd.duplicateStats.AverageHashTime,
		CacheHitRate:           cd.duplicateStats.CacheHitRate,
		WeakHashCollisions:     cd.duplicateStats.WeakHashCollisions,
		StrongHashCollisions:   cd.duplicateStats.StrongHashCollisions,
		SimilarityComputations: cd.duplicateStats.SimilarityComputations,
		DeltaCompressions:      cd.duplicateStats.DeltaCompressions,
	}
	
	if stats.TotalChunksProcessed > 0 {
		duplicateRate := float64(stats.DuplicateChunksFound+stats.SimilarChunksFound) / float64(stats.TotalChunksProcessed)
		stats.DeduplicationRatio = duplicateRate
	}
	
	return stats
}

// ClearCache clears all cached hash data.
func (cd *ChunkDeduplicator) ClearCache() {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	
	cd.chunkHashes = make(map[string]*ChunkHashInfo)
	cd.sizeIndex = make(map[int64][]*ChunkHashInfo)
	cd.contentIndex = make(map[string][]*ChunkHashInfo)
	cd.recentAccess = make(map[string]time.Time)
	cd.weakHashMap = make(map[uint64][]*ChunkHashInfo)
}

// abs returns the absolute value of an integer.
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}