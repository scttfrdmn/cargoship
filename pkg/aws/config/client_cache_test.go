package config

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func TestClientCacheKey_String(t *testing.T) {
	key := ClientCacheKey{
		Bucket:  "test-bucket",
		Region:  "us-west-2",
		Profile: "default",
	}

	expected := "test-bucket:us-west-2:default"
	actual := key.String()

	if actual != expected {
		t.Errorf("Expected %s, got %s", expected, actual)
	}
}

func TestClientCacheKey_Hash(t *testing.T) {
	key1 := ClientCacheKey{
		Bucket:  "test-bucket",
		Region:  "us-west-2",
		Profile: "default",
	}

	key2 := ClientCacheKey{
		Bucket:  "test-bucket",
		Region:  "us-west-2",
		Profile: "default",
	}

	key3 := ClientCacheKey{
		Bucket:  "test-bucket",
		Region:  "us-east-1", // Different region
		Profile: "default",
	}

	// Same keys should have same hash
	if key1.Hash() != key2.Hash() {
		t.Error("Expected identical keys to have same hash")
	}

	// Different keys should have different hashes
	if key1.Hash() == key3.Hash() {
		t.Error("Expected different keys to have different hashes")
	}
}

func TestS3ClientCache_BasicCaching(t *testing.T) {
	ctx := context.Background()
	cache := &S3ClientCache{
		clients: make(map[string]*s3.Client),
		configs: make(map[string]aws.Config),
	}

	key := ClientCacheKey{
		Bucket:  "test-bucket",
		Region:  "us-west-2",
		Profile: "",
	}

	// First call should create client
	client1, err := cache.GetOrCreate(ctx, key)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	if client1 == nil {
		t.Fatal("Expected non-nil client")
	}

	// Second call should return cached client
	client2, err := cache.GetOrCreate(ctx, key)
	if err != nil {
		t.Fatalf("Failed to get cached client: %v", err)
	}

	// Should be the same instance (pointer equality)
	if client1 != client2 {
		t.Error("Expected cached client to be same instance")
	}

	// Check cache stats
	stats := cache.GetStats()
	if stats.CachedClients != 1 {
		t.Errorf("Expected 1 cached client, got %d", stats.CachedClients)
	}
}

func TestS3ClientCache_MultipleRegions(t *testing.T) {
	ctx := context.Background()
	cache := &S3ClientCache{
		clients: make(map[string]*s3.Client),
		configs: make(map[string]aws.Config),
	}

	key1 := ClientCacheKey{
		Bucket:  "test-bucket",
		Region:  "us-west-2",
		Profile: "",
	}

	key2 := ClientCacheKey{
		Bucket:  "test-bucket",
		Region:  "us-east-1",
		Profile: "",
	}

	// Create clients for different regions
	client1, err := cache.GetOrCreate(ctx, key1)
	if err != nil {
		t.Fatalf("Failed to create client for us-west-2: %v", err)
	}

	client2, err := cache.GetOrCreate(ctx, key2)
	if err != nil {
		t.Fatalf("Failed to create client for us-east-1: %v", err)
	}

	// Should be different instances
	if client1 == client2 {
		t.Error("Expected different clients for different regions")
	}

	// Check cache stats
	stats := cache.GetStats()
	if stats.CachedClients != 2 {
		t.Errorf("Expected 2 cached clients, got %d", stats.CachedClients)
	}
}

func TestS3ClientCache_ConcurrentAccess(t *testing.T) {
	ctx := context.Background()
	cache := &S3ClientCache{
		clients: make(map[string]*s3.Client),
		configs: make(map[string]aws.Config),
	}

	key := ClientCacheKey{
		Bucket:  "test-bucket",
		Region:  "us-west-2",
		Profile: "",
	}

	// Simulate concurrent access
	const numGoroutines = 10
	results := make(chan *s3.Client, numGoroutines)
	errors := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			client, err := cache.GetOrCreate(ctx, key)
			if err != nil {
				errors <- err
				return
			}
			results <- client
		}()
	}

	// Collect results
	var clients []*s3.Client
	for i := 0; i < numGoroutines; i++ {
		select {
		case client := <-results:
			clients = append(clients, client)
		case err := <-errors:
			t.Fatalf("Concurrent access failed: %v", err)
		}
	}

	// All should be the same instance
	firstClient := clients[0]
	for i, client := range clients {
		if client != firstClient {
			t.Errorf("Client %d is different instance", i)
		}
	}

	// Should only have 1 cached client despite concurrent access
	stats := cache.GetStats()
	if stats.CachedClients != 1 {
		t.Errorf("Expected 1 cached client, got %d", stats.CachedClients)
	}
}

func TestS3ClientCache_Clear(t *testing.T) {
	ctx := context.Background()
	cache := &S3ClientCache{
		clients: make(map[string]*s3.Client),
		configs: make(map[string]aws.Config),
	}

	key := ClientCacheKey{
		Bucket:  "test-bucket",
		Region:  "us-west-2",
		Profile: "",
	}

	// Create client
	_, err := cache.GetOrCreate(ctx, key)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Verify cached
	if cache.Size() != 1 {
		t.Errorf("Expected cache size 1, got %d", cache.Size())
	}

	// Clear cache
	cache.Clear()

	// Verify cleared
	if cache.Size() != 0 {
		t.Errorf("Expected cache size 0 after clear, got %d", cache.Size())
	}
}

func TestS3ClientCache_Invalidate(t *testing.T) {
	ctx := context.Background()
	cache := &S3ClientCache{
		clients: make(map[string]*s3.Client),
		configs: make(map[string]aws.Config),
	}

	key1 := ClientCacheKey{
		Bucket:  "test-bucket-1",
		Region:  "us-west-2",
		Profile: "",
	}

	key2 := ClientCacheKey{
		Bucket:  "test-bucket-2",
		Region:  "us-west-2",
		Profile: "",
	}

	// Create two clients
	_, err := cache.GetOrCreate(ctx, key1)
	if err != nil {
		t.Fatalf("Failed to create client 1: %v", err)
	}

	_, err = cache.GetOrCreate(ctx, key2)
	if err != nil {
		t.Fatalf("Failed to create client 2: %v", err)
	}

	// Verify both cached
	if cache.Size() != 2 {
		t.Errorf("Expected cache size 2, got %d", cache.Size())
	}

	// Invalidate first client
	cache.Invalidate(key1)

	// Verify only second client remains
	if cache.Size() != 1 {
		t.Errorf("Expected cache size 1 after invalidation, got %d", cache.Size())
	}

	// Verify first client not cached
	_, exists := cache.GetCached(key1)
	if exists {
		t.Error("Expected key1 to not be cached after invalidation")
	}

	// Verify second client still cached
	_, exists = cache.GetCached(key2)
	if !exists {
		t.Error("Expected key2 to still be cached")
	}
}

func TestGetOrCreateS3Client_GlobalCache(t *testing.T) {
	ctx := context.Background()

	// Clear global cache before test
	ClearGlobalCache()

	// Create client using global cache
	client1, err := GetOrCreateS3Client(ctx, "test-bucket", "us-west-2", "")
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	if client1 == nil {
		t.Fatal("Expected non-nil client")
	}

	// Get same client again
	client2, err := GetOrCreateS3Client(ctx, "test-bucket", "us-west-2", "")
	if err != nil {
		t.Fatalf("Failed to get cached client: %v", err)
	}

	// Should be same instance
	if client1 != client2 {
		t.Error("Expected same client instance from global cache")
	}

	// Check global stats
	stats := GetGlobalCacheStats()
	if stats.CachedClients != 1 {
		t.Errorf("Expected 1 cached client in global cache, got %d", stats.CachedClients)
	}
}

func TestGetOrCreateS3Client_DifferentBuckets(t *testing.T) {
	ctx := context.Background()

	// Clear global cache before test
	ClearGlobalCache()

	// Create clients for different buckets
	client1, err := GetOrCreateS3Client(ctx, "bucket-1", "us-west-2", "")
	if err != nil {
		t.Fatalf("Failed to create client 1: %v", err)
	}

	client2, err := GetOrCreateS3Client(ctx, "bucket-2", "us-west-2", "")
	if err != nil {
		t.Fatalf("Failed to create client 2: %v", err)
	}

	// Should be different instances (different buckets)
	if client1 == client2 {
		t.Error("Expected different clients for different buckets")
	}

	// Check global stats
	stats := GetGlobalCacheStats()
	if stats.CachedClients != 2 {
		t.Errorf("Expected 2 cached clients in global cache, got %d", stats.CachedClients)
	}
}

func BenchmarkS3ClientCache_GetOrCreate(b *testing.B) {
	ctx := context.Background()
	cache := &S3ClientCache{
		clients: make(map[string]*s3.Client),
		configs: make(map[string]aws.Config),
	}

	key := ClientCacheKey{
		Bucket:  "test-bucket",
		Region:  "us-west-2",
		Profile: "",
	}

	// Pre-create client for benchmark (avoid first-time creation overhead)
	_, _ = cache.GetOrCreate(ctx, key)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = cache.GetOrCreate(ctx, key)
	}
}

func BenchmarkS3ClientCache_ConcurrentGetOrCreate(b *testing.B) {
	ctx := context.Background()
	cache := &S3ClientCache{
		clients: make(map[string]*s3.Client),
		configs: make(map[string]aws.Config),
	}

	key := ClientCacheKey{
		Bucket:  "test-bucket",
		Region:  "us-west-2",
		Profile: "",
	}

	// Pre-create client
	_, _ = cache.GetOrCreate(ctx, key)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = cache.GetOrCreate(ctx, key)
		}
	})
}
