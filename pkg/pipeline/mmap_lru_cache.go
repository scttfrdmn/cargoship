package pipeline

import (
	"container/list"
	"os"
	"sync"
	"sync/atomic"

	"github.com/scttfrdmn/cargoship/pkg/ioutils"
)

// mmapLRUCache implements an LRU cache for memory-mapped file readers
// with a maximum size limit to prevent file descriptor exhaustion.
// Issue #34 Phase 2.1: Limits open file descriptors to prevent leaks.
type mmapLRUCache struct {
	mu       sync.Mutex
	capacity int
	cache    map[string]*list.Element // path -> list element
	lruList  *list.List               // LRU list (most recent at front)
	evictions int64                   // Total number of evictions (for monitoring)
}

// mmapLRUEntry represents an entry in the LRU cache
type mmapLRUEntry struct {
	path     string
	reader   *ioutils.MmapReader
	file     *os.File
	refCount int32 // Reference count for safe cleanup
}

// newMmapLRUCache creates a new LRU cache with the specified capacity
func newMmapLRUCache(capacity int) *mmapLRUCache {
	return &mmapLRUCache{
		capacity: capacity,
		cache:    make(map[string]*list.Element),
		lruList:  list.New(),
	}
}

// Get retrieves an mmap reader from the cache, or returns nil if not found.
// If found, increments the reference count and moves the entry to the front (most recent).
func (c *mmapLRUCache) Get(path string) (*ioutils.MmapReader, *os.File, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, ok := c.cache[path]
	if !ok {
		return nil, nil, false
	}

	// Move to front (most recently used)
	c.lruList.MoveToFront(elem)

	entry := elem.Value.(*mmapLRUEntry)
	atomic.AddInt32(&entry.refCount, 1)

	return entry.reader, entry.file, true
}

// Put adds or updates an mmap reader in the cache.
// If the cache is at capacity, evicts the least recently used entry.
// Sets initial reference count to 1.
func (c *mmapLRUCache) Put(path string, reader *ioutils.MmapReader, file *os.File) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if already exists (shouldn't happen in normal flow, but handle it)
	if elem, ok := c.cache[path]; ok {
		c.lruList.MoveToFront(elem)
		entry := elem.Value.(*mmapLRUEntry)
		atomic.AddInt32(&entry.refCount, 1)
		return
	}

	// Evict if at capacity
	if c.lruList.Len() >= c.capacity {
		c.evictLRU()
	}

	// Add new entry at front
	entry := &mmapLRUEntry{
		path:     path,
		reader:   reader,
		file:     file,
		refCount: 1,
	}
	elem := c.lruList.PushFront(entry)
	c.cache[path] = elem
}

// Release decrements the reference count for an entry.
// Does NOT remove from cache - LRU eviction handles removal.
func (c *mmapLRUCache) Release(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.cache[path]; ok {
		entry := elem.Value.(*mmapLRUEntry)
		atomic.AddInt32(&entry.refCount, -1)
	}
}

// evictLRU evicts the least recently used entry from the cache.
// Must be called with c.mu held.
func (c *mmapLRUCache) evictLRU() {
	elem := c.lruList.Back()
	if elem == nil {
		return
	}

	entry := elem.Value.(*mmapLRUEntry)

	// Only evict if reference count is zero (no active users)
	// If refCount > 0, try next entry
	for atomic.LoadInt32(&entry.refCount) > 0 {
		// Move to next candidate
		prev := elem.Prev()
		if prev == nil {
			// All entries are in use, cannot evict
			// This is okay - we'll exceed capacity temporarily
			return
		}
		elem = prev
		entry = elem.Value.(*mmapLRUEntry)
	}

	// Remove from cache and list
	delete(c.cache, entry.path)
	c.lruList.Remove(elem)

	// Close file descriptors
	_ = entry.reader.Close()
	_ = entry.file.Close()

	atomic.AddInt64(&c.evictions, 1)
}

// Clear removes all entries from the cache and closes all file descriptors.
func (c *mmapLRUCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, elem := range c.cache {
		entry := elem.Value.(*mmapLRUEntry)
		_ = entry.reader.Close()
		_ = entry.file.Close()
	}

	c.cache = make(map[string]*list.Element)
	c.lruList = list.New()
}

// Len returns the current number of entries in the cache.
func (c *mmapLRUCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lruList.Len()
}

// Evictions returns the total number of evictions that have occurred.
func (c *mmapLRUCache) Evictions() int64 {
	return atomic.LoadInt64(&c.evictions)
}
