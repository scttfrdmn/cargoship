// Package config provides AWS configuration management for CargoShip
package config

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// ClientCacheKey represents a unique key for caching S3 clients
type ClientCacheKey struct {
	Bucket  string
	Region  string
	Profile string
}

// String returns a string representation of the cache key
func (k ClientCacheKey) String() string {
	return fmt.Sprintf("%s:%s:%s", k.Bucket, k.Region, k.Profile)
}

// Hash returns a hash of the cache key for map lookups
func (k ClientCacheKey) Hash() string {
	h := sha256.New()
	h.Write([]byte(k.String()))
	return hex.EncodeToString(h.Sum(nil))
}

// S3ClientCache provides global caching of S3 clients to improve performance
// Inspired by s5cmd and MinIO mc's session caching strategies
type S3ClientCache struct {
	mu      sync.RWMutex
	clients map[string]*s3.Client // Key: ClientCacheKey.Hash()
	configs map[string]aws.Config // Key: ClientCacheKey.Hash() (for session reuse)
}

// Global S3 client cache instance
var globalS3ClientCache = &S3ClientCache{
	clients: make(map[string]*s3.Client),
	configs: make(map[string]aws.Config),
}

// GetOrCreateS3Client retrieves a cached S3 client or creates a new one
// This dramatically improves performance by reusing HTTP/2 connections and credentials
func GetOrCreateS3Client(ctx context.Context, bucket, region, profile string) (*s3.Client, error) {
	key := ClientCacheKey{
		Bucket:  bucket,
		Region:  region,
		Profile: profile,
	}

	return globalS3ClientCache.GetOrCreate(ctx, key)
}

// GetOrCreate retrieves a cached S3 client or creates a new one
func (c *S3ClientCache) GetOrCreate(ctx context.Context, key ClientCacheKey) (*s3.Client, error) {
	keyHash := key.Hash()

	// Fast path: check if client already cached
	c.mu.RLock()
	if client, exists := c.clients[keyHash]; exists {
		c.mu.RUnlock()
		return client, nil
	}
	c.mu.RUnlock()

	// Slow path: create new client (acquire write lock)
	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock (another goroutine may have created it)
	if client, exists := c.clients[keyHash]; exists {
		return client, nil
	}

	// Create new AWS config and S3 client
	cfg, err := c.loadAWSConfig(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config for %s: %w", key.String(), err)
	}

	client := s3.NewFromConfig(cfg)

	// Cache both config and client
	c.configs[keyHash] = cfg
	c.clients[keyHash] = client

	return client, nil
}

// loadAWSConfig loads AWS configuration with the specified region and profile
func (c *S3ClientCache) loadAWSConfig(ctx context.Context, key ClientCacheKey) (aws.Config, error) {
	var opts []func(*awsconfig.LoadOptions) error

	// Set region if specified
	if key.Region != "" {
		opts = append(opts, awsconfig.WithRegion(key.Region))
	}

	// Set profile if specified
	if key.Profile != "" {
		opts = append(opts, awsconfig.WithSharedConfigProfile(key.Profile))
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return aws.Config{}, fmt.Errorf("failed to load AWS config: %w", err)
	}

	return cfg, nil
}

// GetCached retrieves a cached S3 client without creating a new one
func (c *S3ClientCache) GetCached(key ClientCacheKey) (*s3.Client, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	keyHash := key.Hash()
	client, exists := c.clients[keyHash]
	return client, exists
}

// Clear removes all cached clients (useful for testing)
func (c *S3ClientCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.clients = make(map[string]*s3.Client)
	c.configs = make(map[string]aws.Config)
}

// Size returns the number of cached clients
func (c *S3ClientCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.clients)
}

// GetStats returns cache statistics
func (c *S3ClientCache) GetStats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return CacheStats{
		CachedClients: len(c.clients),
		CachedConfigs: len(c.configs),
	}
}

// CacheStats holds cache statistics
type CacheStats struct {
	CachedClients int
	CachedConfigs int
}

// Invalidate removes a specific cached client
func (c *S3ClientCache) Invalidate(key ClientCacheKey) {
	c.mu.Lock()
	defer c.mu.Unlock()

	keyHash := key.Hash()
	delete(c.clients, keyHash)
	delete(c.configs, keyHash)
}

// ClearGlobalCache clears the global S3 client cache (useful for testing)
func ClearGlobalCache() {
	globalS3ClientCache.Clear()
}

// GetGlobalCacheStats returns global cache statistics
func GetGlobalCacheStats() CacheStats {
	return globalS3ClientCache.GetStats()
}
