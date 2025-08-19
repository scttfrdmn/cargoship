package s3optimization

import (
	"io"
	"sync"
	"time"
)

// PrefetchCache implements an intelligent cache for prefetched S3 objects.
type PrefetchCache struct {
	config       *PrefetchConfig
	cache        map[string]*CachedObject
	lruList      *LRUList
	sizeTracker  *SizeTracker
	mu           sync.RWMutex
	
	// Performance metrics
	hits         int64
	misses       int64
	evictions    int64
	totalSize    int64
	maxSize      int64
}

// CachedObject represents a cached S3 object.
type CachedObject struct {
	Key          string
	Data         []byte
	Size         int64
	CachedAt     time.Time
	LastAccessed time.Time
	AccessCount  int64
	TTL          time.Duration
	Metadata     map[string]string
	
	// LRU chain
	prev *CachedObject
	next *CachedObject
	
	// Priority and scoring
	Priority     float64
	HitRatio     float64
	PrefetchHit  bool
}

// LRUList maintains the LRU ordering of cached objects.
type LRUList struct {
	head *CachedObject
	tail *CachedObject
	size int
}

// SizeTracker tracks cache size and manages size-based eviction.
type SizeTracker struct {
	totalSize      int64
	maxSize        int64
	sizeByKey      map[string]int64
	evictionPolicy string
}

// NewPrefetchCache creates a new prefetch cache.
func NewPrefetchCache(config *PrefetchConfig) *PrefetchCache {
	cache := &PrefetchCache{
		config:      config,
		cache:       make(map[string]*CachedObject),
		lruList:     NewLRUList(),
		sizeTracker: NewSizeTracker(config.CacheSize, config.EvictionPolicy),
		maxSize:     config.CacheSize,
	}
	
	// Start cleanup goroutine
	go cache.periodicCleanup()
	
	return cache
}

// Get retrieves an object from the cache.
func (pc *PrefetchCache) Get(key string) (*CachedObject, bool) {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	
	obj, exists := pc.cache[key]
	if !exists {
		pc.misses++
		return nil, false
	}
	
	// Check TTL
	if pc.isExpired(obj) {
		pc.removeObjectLocked(key)
		pc.misses++
		return nil, false
	}
	
	// Update access information
	obj.LastAccessed = time.Now()
	obj.AccessCount++
	obj.HitRatio = float64(obj.AccessCount) / float64(time.Since(obj.CachedAt).Hours()+1)
	
	// Move to front of LRU list
	pc.lruList.MoveToFront(obj)
	
	pc.hits++
	return obj, true
}

// Put stores an object in the cache.
func (pc *PrefetchCache) Put(key string, data []byte, metadata map[string]string) error {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	
	// Check if already cached
	if existingObj, exists := pc.cache[key]; exists {
		// Update existing object
		existingObj.Data = data
		existingObj.Size = int64(len(data))
		existingObj.LastAccessed = time.Now()
		existingObj.Metadata = metadata
		pc.lruList.MoveToFront(existingObj)
		return nil
	}
	
	// Create new cached object
	obj := &CachedObject{
		Key:          key,
		Data:         data,
		Size:         int64(len(data)),
		CachedAt:     time.Now(),
		LastAccessed: time.Now(),
		AccessCount:  0,
		TTL:          pc.config.CacheTTL,
		Metadata:     metadata,
		Priority:     pc.calculatePriority(key, int64(len(data))),
	}
	
	// Check if we need to make space
	if pc.sizeTracker.totalSize+obj.Size > pc.maxSize {
		if err := pc.makeSpace(obj.Size); err != nil {
			return err
		}
	}
	
	// Add to cache
	pc.cache[key] = obj
	pc.lruList.AddToFront(obj)
	pc.sizeTracker.Add(key, obj.Size)
	pc.totalSize += obj.Size
	
	return nil
}

// Remove removes an object from the cache.
func (pc *PrefetchCache) Remove(key string) {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	pc.removeObjectLocked(key)
}

// Clear clears all objects from the cache.
func (pc *PrefetchCache) Clear() {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	
	pc.cache = make(map[string]*CachedObject)
	pc.lruList = NewLRUList()
	pc.sizeTracker = NewSizeTracker(pc.maxSize, pc.config.EvictionPolicy)
	pc.totalSize = 0
}

// GetStats returns cache statistics.
func (pc *PrefetchCache) GetStats() *CacheStats {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	
	hitRate := 0.0
	totalRequests := pc.hits + pc.misses
	if totalRequests > 0 {
		hitRate = float64(pc.hits) / float64(totalRequests)
	}
	
	return &CacheStats{
		Hits:        pc.hits,
		Misses:      pc.misses,
		Evictions:   pc.evictions,
		HitRate:     hitRate,
		TotalSize:   pc.totalSize,
		MaxSize:     pc.maxSize,
		ObjectCount: int64(len(pc.cache)),
		Utilization: float64(pc.totalSize) / float64(pc.maxSize),
	}
}

// AdaptSize adapts the cache size based on performance metrics.
func (pc *PrefetchCache) AdaptSize(multiplier float64) {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	
	newSize := int64(float64(pc.maxSize) * multiplier)
	
	// Set reasonable bounds
	minSize := int64(64 * 1024 * 1024)  // 64MB minimum
	maxSize := int64(8 * 1024 * 1024 * 1024) // 8GB maximum
	
	if newSize < minSize {
		newSize = minSize
	} else if newSize > maxSize {
		newSize = maxSize
	}
	
	pc.maxSize = newSize
	pc.sizeTracker.maxSize = newSize
	
	// If cache is now too large, evict objects
	if pc.totalSize > pc.maxSize {
		_ = pc.makeSpace(pc.totalSize - pc.maxSize)
	}
}

// removeObjectLocked removes an object from the cache (must hold lock).
func (pc *PrefetchCache) removeObjectLocked(key string) {
	if obj, exists := pc.cache[key]; exists {
		delete(pc.cache, key)
		pc.lruList.Remove(obj)
		pc.sizeTracker.Remove(key)
		pc.totalSize -= obj.Size
	}
}

// makeSpace makes space in the cache by evicting objects.
func (pc *PrefetchCache) makeSpace(neededSize int64) error {
	freedSize := int64(0)
	
	for freedSize < neededSize && pc.lruList.size > 0 {
		// Find object to evict based on eviction policy
		var objToEvict *CachedObject
		
		switch pc.sizeTracker.evictionPolicy {
		case "lru":
			objToEvict = pc.lruList.tail
		case "lfu":
			objToEvict = pc.findLFUObject()
		case "priority":
			objToEvict = pc.findLowestPriorityObject()
		default:
			objToEvict = pc.lruList.tail // Default to LRU
		}
		
		if objToEvict == nil {
			break
		}
		
		freedSize += objToEvict.Size
		pc.removeObjectLocked(objToEvict.Key)
		pc.evictions++
	}
	
	return nil
}

// isExpired checks if an object has expired.
func (pc *PrefetchCache) isExpired(obj *CachedObject) bool {
	return time.Since(obj.CachedAt) > obj.TTL
}

// calculatePriority calculates the priority for caching an object.
func (pc *PrefetchCache) calculatePriority(key string, size int64) float64 {
	// Base priority
	priority := 1.0
	
	// Adjust for size (smaller objects get higher priority)
	if size > 0 {
		sizeScore := 1.0 / (1.0 + float64(size)/(1024*1024)) // Normalize by MB
		priority *= sizeScore
	}
	
	// Adjust for key characteristics
	if len(key) > 0 {
		// Common file types get higher priority
		if isCommonFileType(key) {
			priority *= 1.2
		}
		
		// Recent objects (based on key naming) get higher priority
		if isRecentObject(key) {
			priority *= 1.1
		}
	}
	
	return priority
}

// findLFUObject finds the least frequently used object.
func (pc *PrefetchCache) findLFUObject() *CachedObject {
	var lfu *CachedObject
	var minAccessCount int64 = -1
	
	for _, obj := range pc.cache {
		if minAccessCount == -1 || obj.AccessCount < minAccessCount {
			minAccessCount = obj.AccessCount
			lfu = obj
		}
	}
	
	return lfu
}

// findLowestPriorityObject finds the object with the lowest priority.
func (pc *PrefetchCache) findLowestPriorityObject() *CachedObject {
	var lowest *CachedObject
	var minPriority float64 = -1
	
	for _, obj := range pc.cache {
		if minPriority == -1 || obj.Priority < minPriority {
			minPriority = obj.Priority
			lowest = obj
		}
	}
	
	return lowest
}

// periodicCleanup performs periodic cleanup of expired objects.
func (pc *PrefetchCache) periodicCleanup() {
	ticker := time.NewTicker(time.Minute * 5) // Cleanup every 5 minutes
	defer ticker.Stop()
	
	for range ticker.C {
		pc.cleanupExpired()
	}
}

// cleanupExpired removes expired objects from the cache.
func (pc *PrefetchCache) cleanupExpired() {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	
	var expiredKeys []string
	
	for key, obj := range pc.cache {
		if pc.isExpired(obj) {
			expiredKeys = append(expiredKeys, key)
		}
	}
	
	for _, key := range expiredKeys {
		pc.removeObjectLocked(key)
		pc.evictions++
	}
}

// NewLRUList creates a new LRU list.
func NewLRUList() *LRUList {
	return &LRUList{}
}

// AddToFront adds an object to the front of the LRU list.
func (lru *LRUList) AddToFront(obj *CachedObject) {
	if lru.head == nil {
		lru.head = obj
		lru.tail = obj
		obj.prev = nil
		obj.next = nil
	} else {
		obj.next = lru.head
		obj.prev = nil
		lru.head.prev = obj
		lru.head = obj
	}
	lru.size++
}

// Remove removes an object from the LRU list.
func (lru *LRUList) Remove(obj *CachedObject) {
	if obj.prev != nil {
		obj.prev.next = obj.next
	} else {
		lru.head = obj.next
	}
	
	if obj.next != nil {
		obj.next.prev = obj.prev
	} else {
		lru.tail = obj.prev
	}
	
	obj.prev = nil
	obj.next = nil
	lru.size--
}

// MoveToFront moves an object to the front of the LRU list.
func (lru *LRUList) MoveToFront(obj *CachedObject) {
	if obj == lru.head {
		return // Already at front
	}
	
	lru.Remove(obj)
	lru.AddToFront(obj)
}

// NewSizeTracker creates a new size tracker.
func NewSizeTracker(maxSize int64, evictionPolicy string) *SizeTracker {
	return &SizeTracker{
		maxSize:        maxSize,
		sizeByKey:      make(map[string]int64),
		evictionPolicy: evictionPolicy,
	}
}

// Add adds an object's size to the tracker.
func (st *SizeTracker) Add(key string, size int64) {
	st.sizeByKey[key] = size
	st.totalSize += size
}

// Remove removes an object's size from the tracker.
func (st *SizeTracker) Remove(key string) {
	if size, exists := st.sizeByKey[key]; exists {
		st.totalSize -= size
		delete(st.sizeByKey, key)
	}
}

// CacheStats contains cache performance statistics.
type CacheStats struct {
	Hits        int64   `json:"hits"`
	Misses      int64   `json:"misses"`
	Evictions   int64   `json:"evictions"`
	HitRate     float64 `json:"hit_rate"`
	TotalSize   int64   `json:"total_size"`
	MaxSize     int64   `json:"max_size"`
	ObjectCount int64   `json:"object_count"`
	Utilization float64 `json:"utilization"`
}

// ReadableObject implements io.Reader for cached objects.
type ReadableObject struct {
	data   []byte
	offset int64
}

// NewReadableObject creates a new readable object from cached data.
func NewReadableObject(data []byte) *ReadableObject {
	return &ReadableObject{
		data:   data,
		offset: 0,
	}
}

// Read implements io.Reader.
func (ro *ReadableObject) Read(p []byte) (n int, err error) {
	if ro.offset >= int64(len(ro.data)) {
		return 0, io.EOF
	}
	
	remaining := int64(len(ro.data)) - ro.offset
	toRead := int64(len(p))
	if toRead > remaining {
		toRead = remaining
	}
	
	copy(p, ro.data[ro.offset:ro.offset+toRead])
	ro.offset += toRead
	
	if ro.offset >= int64(len(ro.data)) {
		return int(toRead), io.EOF
	}
	
	return int(toRead), nil
}

// Seek implements io.Seeker.
func (ro *ReadableObject) Seek(offset int64, whence int) (int64, error) {
	var newOffset int64
	
	switch whence {
	case io.SeekStart:
		newOffset = offset
	case io.SeekCurrent:
		newOffset = ro.offset + offset
	case io.SeekEnd:
		newOffset = int64(len(ro.data)) + offset
	default:
		return 0, io.EOF
	}
	
	if newOffset < 0 {
		newOffset = 0
	} else if newOffset > int64(len(ro.data)) {
		newOffset = int64(len(ro.data))
	}
	
	ro.offset = newOffset
	return newOffset, nil
}

// Close implements io.Closer.
func (ro *ReadableObject) Close() error {
	return nil
}

// Helper functions

func isCommonFileType(key string) bool {
	if len(key) < 4 {
		return false
	}
	
	ext := key[len(key)-4:]
	commonTypes := []string{".jpg", ".png", ".pdf", ".txt", ".json", ".xml"}
	
	for _, commonType := range commonTypes {
		if ext == commonType {
			return true
		}
	}
	
	return false
}

func isRecentObject(key string) bool {
	// Simple heuristic - check if key contains recent date patterns
	now := time.Now()
	currentYear := now.Year()
	currentMonth := int(now.Month())
	
	// Look for current year in key
	yearStr := string(rune('0' + currentYear/1000))
	yearStr += string(rune('0' + (currentYear/100)%10))
	yearStr += string(rune('0' + (currentYear/10)%10))
	yearStr += string(rune('0' + currentYear%10))
	
	if containsString(key, yearStr) {
		return true
	}
	
	// Look for current month
	monthStr := string(rune('0' + currentMonth/10))
	monthStr += string(rune('0' + currentMonth%10))
	
	return containsString(key, monthStr)
}

func containsString(s, substr string) bool {
	if len(substr) > len(s) {
		return false
	}
	
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	
	return false
}