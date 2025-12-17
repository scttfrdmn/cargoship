package pipeline

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/scttfrdmn/cargoship/pkg/ioutils"
)

// TestMmapLRUCache_BasicOperations tests basic Get/Put/Release operations
func TestMmapLRUCache_BasicOperations(t *testing.T) {
	cache := newMmapLRUCache(3)

	// Create temporary test files
	tmpDir := t.TempDir()
	files := make([]*os.File, 3)
	for i := 0; i < 3; i++ {
		path := filepath.Join(tmpDir, "test"+string(rune('0'+i))+".dat")
		f, err := os.Create(path)
		if err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
		// Write enough data for mmap (must be >= 128MB threshold)
		if _, err := f.Write(make([]byte, 150*1024*1024)); err != nil {
			t.Fatalf("Failed to write test data: %v", err)
		}
		_ = f.Close()

		// Reopen for reading
		f, err = os.Open(path)
		if err != nil {
			t.Fatalf("Failed to open test file: %v", err)
		}
		files[i] = f
	}
	defer func() {
		for _, f := range files {
			if f != nil {
				_ = f.Close()
			}
		}
	}()

	// Create mmap readers
	readers := make([]*ioutils.MmapReader, 3)
	for i := 0; i < 3; i++ {
		reader, err := ioutils.NewMmapReader(files[i])
		if err != nil {
			t.Fatalf("Failed to create mmap reader: %v", err)
		}
		readers[i] = reader
	}

	// Test Put and Get
	path1 := filepath.Join(tmpDir, "test0.dat")
	cache.Put(path1, readers[0], files[0])

	if cache.Len() != 1 {
		t.Errorf("Expected cache length 1, got %d", cache.Len())
	}

	reader, file, ok := cache.Get(path1)
	if !ok {
		t.Error("Expected to find entry in cache")
	}
	if reader != readers[0] {
		t.Error("Got wrong reader from cache")
	}
	if file != files[0] {
		t.Error("Got wrong file from cache")
	}

	// Test Release
	cache.Release(path1)
	cache.Release(path1) // Double release (ref count should go to 0)

	// Cache should still contain the entry
	if cache.Len() != 1 {
		t.Errorf("Expected cache length 1 after release, got %d", cache.Len())
	}
}

// TestMmapLRUCache_LRUEviction tests that LRU eviction works correctly
func TestMmapLRUCache_LRUEviction(t *testing.T) {
	cache := newMmapLRUCache(2) // Small capacity for testing

	tmpDir := t.TempDir()

	// Helper to create test file and mmap
	createEntry := func(name string) (string, *ioutils.MmapReader, *os.File) {
		path := filepath.Join(tmpDir, name)
		f, err := os.Create(path)
		if err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
		if _, err := f.Write(make([]byte, 150*1024*1024)); err != nil{
			t.Fatalf("Failed to write test data: %v", err)
		}
		_ = f.Close()

		f, err = os.Open(path)
		if err != nil {
			t.Fatalf("Failed to open test file: %v", err)
		}

		reader, err := ioutils.NewMmapReader(f)
		if err != nil {
			t.Fatalf("Failed to create mmap reader: %v", err)
		}

		return path, reader, f
	}

	// Add 3 entries (should evict the first one)
	path1, reader1, file1 := createEntry("test1.dat")
	cache.Put(path1, reader1, file1)
	cache.Release(path1) // Release so it can be evicted

	path2, reader2, file2 := createEntry("test2.dat")
	cache.Put(path2, reader2, file2)
	cache.Release(path2)

	path3, reader3, file3 := createEntry("test3.dat")
	cache.Put(path3, reader3, file3)
	cache.Release(path3)

	// Cache should have at most 2 entries
	if cache.Len() > 2 {
		t.Errorf("Expected cache length <= 2, got %d", cache.Len())
	}

	// First entry should be evicted
	_, _, ok := cache.Get(path1)
	if ok {
		t.Error("Expected first entry to be evicted, but it was found")
	}

	// Second and third entries should exist
	if _, _, ok := cache.Get(path2); !ok {
		t.Error("Expected second entry to exist")
	}
	if _, _, ok := cache.Get(path3); !ok {
		t.Error("Expected third entry to exist")
	}

	// Check eviction count
	if cache.Evictions() < 1 {
		t.Errorf("Expected at least 1 eviction, got %d", cache.Evictions())
	}
}

// TestMmapLRUCache_RefCountPreventsEviction tests that entries with ref count > 0 are not evicted
func TestMmapLRUCache_RefCountPreventsEviction(t *testing.T) {
	cache := newMmapLRUCache(2)

	tmpDir := t.TempDir()

	createEntry := func(name string) (string, *ioutils.MmapReader, *os.File) {
		path := filepath.Join(tmpDir, name)
		f, err := os.Create(path)
		if err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
		if _, err := f.Write(make([]byte, 150*1024*1024)); err != nil{
			t.Fatalf("Failed to write test data: %v", err)
		}
		_ = f.Close()

		f, err = os.Open(path)
		if err != nil {
			t.Fatalf("Failed to open test file: %v", err)
		}

		reader, err := ioutils.NewMmapReader(f)
		if err != nil {
			t.Fatalf("Failed to create mmap reader: %v", err)
		}

		return path, reader, f
	}

	// Add entry 1 and DON'T release (ref count = 1)
	path1, reader1, file1 := createEntry("test1.dat")
	cache.Put(path1, reader1, file1)

	// Add entry 2 and release
	path2, reader2, file2 := createEntry("test2.dat")
	cache.Put(path2, reader2, file2)
	cache.Release(path2)

	// Add entry 3 (should try to evict, but entry1 has ref count > 0)
	path3, reader3, file3 := createEntry("test3.dat")
	cache.Put(path3, reader3, file3)
	cache.Release(path3)

	// Entry 1 should still exist (protected by ref count)
	if _, _, ok := cache.Get(path1); !ok {
		t.Error("Expected entry 1 to exist (protected by ref count)")
	}

	// Cache can exceed capacity if entries are in use
	if cache.Len() < 2 {
		t.Errorf("Expected cache length >= 2, got %d", cache.Len())
	}
}

// TestMmapLRUCache_Concurrent tests concurrent access to the cache
func TestMmapLRUCache_Concurrent(t *testing.T) {
	cache := newMmapLRUCache(10)

	tmpDir := t.TempDir()

	// Create test files
	paths := make([]string, 5)
	for i := 0; i < 5; i++ {
		path := filepath.Join(tmpDir, "test"+string(rune('0'+i))+".dat")
		f, err := os.Create(path)
		if err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
		if _, err := f.Write(make([]byte, 150*1024*1024)); err != nil{
			t.Fatalf("Failed to write test data: %v", err)
		}
		_ = f.Close()
		paths[i] = path
	}

	var wg sync.WaitGroup
	numGoroutines := 10

	// Concurrent Put/Get/Release operations
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			for j := 0; j < 100; j++ {
				path := paths[j%len(paths)]

				// Try to get from cache
				if _, _, ok := cache.Get(path); ok {
					cache.Release(path)
					continue
				}

				// Not in cache, create and add
				f, err := os.Open(path)
				if err != nil {
					continue
				}
				reader, err := ioutils.NewMmapReader(f)
				if err != nil {
					_ = f.Close()
					continue
				}
				cache.Put(path, reader, f)
				cache.Release(path)
			}
		}(i)
	}

	wg.Wait()

	// Cache should have some entries
	if cache.Len() == 0 {
		t.Error("Expected cache to have entries after concurrent operations")
	}
}

// TestMmapLRUCache_Clear tests that Clear() properly cleans up all entries
func TestMmapLRUCache_Clear(t *testing.T) {
	cache := newMmapLRUCache(5)

	tmpDir := t.TempDir()

	// Add some entries
	for i := 0; i < 3; i++ {
		path := filepath.Join(tmpDir, "test"+string(rune('0'+i))+".dat")
		f, err := os.Create(path)
		if err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
		if _, err := f.Write(make([]byte, 150*1024*1024)); err != nil{
			t.Fatalf("Failed to write test data: %v", err)
		}
		_ = f.Close()

		f, err = os.Open(path)
		if err != nil {
			t.Fatalf("Failed to open test file: %v", err)
		}

		reader, err := ioutils.NewMmapReader(f)
		if err != nil {
			t.Fatalf("Failed to create mmap reader: %v", err)
		}

		cache.Put(path, reader, f)
		cache.Release(path)
	}

	if cache.Len() != 3 {
		t.Errorf("Expected cache length 3, got %d", cache.Len())
	}

	// Clear the cache
	cache.Clear()

	if cache.Len() != 0 {
		t.Errorf("Expected cache length 0 after clear, got %d", cache.Len())
	}
}
