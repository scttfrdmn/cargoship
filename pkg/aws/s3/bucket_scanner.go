// Package s3 provides AWS S3 bucket scanning capabilities for cost analysis
package s3

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	awsconfig "github.com/scttfrdmn/cargoship/pkg/aws/config"
)

// BucketScanner scans S3 buckets and collects object metadata for cost analysis
type BucketScanner struct {
	client      *s3.Client
	config      *BucketScanConfig
	progressFn  ProgressCallback
	statsLock   sync.Mutex
	objectStats *BucketStats
}

// BucketScanConfig configures bucket scanning behavior
type BucketScanConfig struct {
	// Bucket name to scan
	Bucket string

	// Prefix to filter objects (optional)
	Prefix string

	// Maximum number of objects to scan (0 = all)
	MaxObjects int

	// Enable sampling mode for quick estimates
	EnableSampling bool

	// Sample size for sampling mode (default: 10000)
	SampleSize int

	// Number of concurrent list operations
	Concurrency int

	// Progress update interval
	ProgressInterval time.Duration
}

// BucketStats holds aggregate statistics for a bucket
type BucketStats struct {
	// Total number of objects scanned
	ObjectCount int64

	// Total size in bytes
	TotalSize int64

	// Storage class distribution
	StorageClassCounts map[types.ObjectStorageClass]int64
	StorageClassSizes  map[types.ObjectStorageClass]int64

	// Size distribution buckets
	SmallFiles   int64 // < 1 MB
	MediumFiles  int64 // 1 MB - 100 MB
	LargeFiles   int64 // 100 MB - 1 GB
	HugeFiles    int64 // > 1 GB

	// Average object size (calculated)
	AverageSize float64

	// Timestamp of scan
	ScanTime time.Time

	// Scan duration
	ScanDuration time.Duration

	// Whether this is a sampled result
	IsSampled bool

	// Sample size if sampled
	SampleSize int

	// Estimated total if sampled
	EstimatedTotal int64
}

// ObjectInfo represents metadata about an S3 object
type ObjectInfo struct {
	Key          string
	Size         int64
	StorageClass types.StorageClass
	LastModified time.Time
	ETag         string
}

// ProgressCallback is called periodically during scanning
type ProgressCallback func(stats *BucketStats)

// NewBucketScanner creates a new bucket scanner
func NewBucketScanner(client *s3.Client, config *BucketScanConfig) *BucketScanner {
	// Set defaults
	if config.SampleSize == 0 {
		config.SampleSize = 10000
	}
	if config.Concurrency == 0 {
		config.Concurrency = 1
	}
	if config.ProgressInterval == 0 {
		config.ProgressInterval = 2 * time.Second
	}

	return &BucketScanner{
		client: client,
		config: config,
		objectStats: &BucketStats{
			StorageClassCounts: make(map[types.ObjectStorageClass]int64),
			StorageClassSizes:  make(map[types.ObjectStorageClass]int64),
			ScanTime:           time.Now(),
		},
	}
}

// SetProgressCallback sets the progress callback function
func (bs *BucketScanner) SetProgressCallback(fn ProgressCallback) {
	bs.progressFn = fn
}

// Scan scans the S3 bucket and returns statistics
func (bs *BucketScanner) Scan(ctx context.Context) (*BucketStats, error) {
	startTime := time.Now()

	// Start progress reporter
	progressCtx, cancelProgress := context.WithCancel(ctx)
	defer cancelProgress()

	if bs.progressFn != nil {
		go bs.reportProgress(progressCtx)
	}

	// Determine scan mode
	var err error
	if bs.config.EnableSampling {
		err = bs.scanSampled(ctx)
	} else {
		err = bs.scanFull(ctx)
	}

	if err != nil {
		return nil, fmt.Errorf("scan failed: %w", err)
	}

	// Calculate final statistics
	bs.statsLock.Lock()
	defer bs.statsLock.Unlock()

	if bs.objectStats.ObjectCount > 0 {
		bs.objectStats.AverageSize = float64(bs.objectStats.TotalSize) / float64(bs.objectStats.ObjectCount)
	}

	bs.objectStats.ScanDuration = time.Since(startTime)

	return bs.objectStats, nil
}

// scanFull performs a complete bucket scan
func (bs *BucketScanner) scanFull(ctx context.Context) error {
	paginator := s3.NewListObjectsV2Paginator(bs.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(bs.config.Bucket),
		Prefix: aws.String(bs.config.Prefix),
	})

	for paginator.HasMorePages() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		page, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("failed to list objects: %w", err)
		}

		for _, obj := range page.Contents {
			bs.processObject(obj)

			// Check max objects limit
			if bs.config.MaxObjects > 0 && atomic.LoadInt64(&bs.objectStats.ObjectCount) >= int64(bs.config.MaxObjects) {
				return nil
			}
		}
	}

	return nil
}

// scanSampled performs a sampled bucket scan for quick estimates
func (bs *BucketScanner) scanSampled(ctx context.Context) error {
	bs.objectStats.IsSampled = true
	bs.objectStats.SampleSize = bs.config.SampleSize

	// List first N objects as a sample
	input := &s3.ListObjectsV2Input{
		Bucket:  aws.String(bs.config.Bucket),
		Prefix:  aws.String(bs.config.Prefix),
		MaxKeys: aws.Int32(int32(bs.config.SampleSize)),
	}

	resp, err := bs.client.ListObjectsV2(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to list objects: %w", err)
	}

	for _, obj := range resp.Contents {
		bs.processObject(obj)
	}

	// If we got a full sample, estimate total count
	if len(resp.Contents) == bs.config.SampleSize && aws.ToBool(resp.IsTruncated) {
		// Get approximate total count by counting all keys (lightweight operation)
		totalCount, err := bs.estimateTotalCount(ctx)
		if err != nil {
			// If estimate fails, just use sample
			totalCount = int64(len(resp.Contents))
		}

		bs.statsLock.Lock()
		bs.objectStats.EstimatedTotal = totalCount
		bs.statsLock.Unlock()

		// Extrapolate stats
		bs.extrapolateStats(totalCount)
	}

	return nil
}

// estimateTotalCount estimates the total number of objects in the bucket
func (bs *BucketScanner) estimateTotalCount(ctx context.Context) (int64, error) {
	var count int64

	paginator := s3.NewListObjectsV2Paginator(bs.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(bs.config.Bucket),
		Prefix: aws.String(bs.config.Prefix),
	})

	for paginator.HasMorePages() {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		default:
		}

		page, err := paginator.NextPage(ctx)
		if err != nil {
			return 0, fmt.Errorf("failed to count objects: %w", err)
		}

		count += int64(len(page.Contents))

		// Stop after counting 100k objects to avoid long waits
		if count > 100000 {
			// Rough extrapolation based on continuation token
			// This is approximate but better than nothing
			break
		}
	}

	return count, nil
}

// extrapolateStats extrapolates sample statistics to estimated totals
func (bs *BucketScanner) extrapolateStats(estimatedTotal int64) {
	bs.statsLock.Lock()
	defer bs.statsLock.Unlock()

	if bs.objectStats.ObjectCount == 0 {
		return
	}

	scaleFactor := float64(estimatedTotal) / float64(bs.objectStats.ObjectCount)

	// Scale all statistics
	bs.objectStats.TotalSize = int64(float64(bs.objectStats.TotalSize) * scaleFactor)

	for class := range bs.objectStats.StorageClassCounts {
		bs.objectStats.StorageClassCounts[class] = int64(float64(bs.objectStats.StorageClassCounts[class]) * scaleFactor)
		bs.objectStats.StorageClassSizes[class] = int64(float64(bs.objectStats.StorageClassSizes[class]) * scaleFactor)
	}

	bs.objectStats.SmallFiles = int64(float64(bs.objectStats.SmallFiles) * scaleFactor)
	bs.objectStats.MediumFiles = int64(float64(bs.objectStats.MediumFiles) * scaleFactor)
	bs.objectStats.LargeFiles = int64(float64(bs.objectStats.LargeFiles) * scaleFactor)
	bs.objectStats.HugeFiles = int64(float64(bs.objectStats.HugeFiles) * scaleFactor)

	bs.objectStats.ObjectCount = estimatedTotal
}

// processObject processes a single S3 object and updates statistics
func (bs *BucketScanner) processObject(obj types.Object) {
	bs.statsLock.Lock()
	defer bs.statsLock.Unlock()

	// Update counts
	bs.objectStats.ObjectCount++
	bs.objectStats.TotalSize += aws.ToInt64(obj.Size)

	// Update storage class distribution
	storageClass := obj.StorageClass
	if storageClass == "" {
		storageClass = types.ObjectStorageClassStandard
	}

	bs.objectStats.StorageClassCounts[storageClass]++
	bs.objectStats.StorageClassSizes[storageClass] += aws.ToInt64(obj.Size)

	// Update size distribution
	size := aws.ToInt64(obj.Size)
	switch {
	case size < 1*1024*1024: // < 1 MB
		bs.objectStats.SmallFiles++
	case size < 100*1024*1024: // 1-100 MB
		bs.objectStats.MediumFiles++
	case size < 1024*1024*1024: // 100 MB - 1 GB
		bs.objectStats.LargeFiles++
	default: // > 1 GB
		bs.objectStats.HugeFiles++
	}
}

// reportProgress periodically reports scan progress
func (bs *BucketScanner) reportProgress(ctx context.Context) {
	ticker := time.NewTicker(bs.config.ProgressInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			bs.statsLock.Lock()
			statsCopy := *bs.objectStats
			bs.statsLock.Unlock()

			if bs.progressFn != nil {
				bs.progressFn(&statsCopy)
			}
		}
	}
}

// GetBucketRegion detects the region of an S3 bucket
func GetBucketRegion(ctx context.Context, client *s3.Client, bucket string) (string, error) {
	// Try to get bucket location
	resp, err := client.GetBucketLocation(ctx, &s3.GetBucketLocationInput{
		Bucket: aws.String(bucket),
	})
	if err != nil {
		return "", fmt.Errorf("failed to get bucket location: %w", err)
	}

	// us-east-1 returns empty location
	if resp.LocationConstraint == "" {
		return "us-east-1", nil
	}

	return string(resp.LocationConstraint), nil
}

// ParseS3URL parses an S3 URL (s3://bucket/prefix) into bucket and prefix
func ParseS3URL(url string) (bucket, prefix string, err error) {
	// Remove s3:// prefix
	if len(url) < 5 || url[:5] != "s3://" {
		return "", "", fmt.Errorf("invalid S3 URL: must start with s3://")
	}

	path := url[5:]

	// Split bucket and prefix
	for i, ch := range path {
		if ch == '/' {
			return path[:i], path[i+1:], nil
		}
	}

	// No prefix
	return path, "", nil
}

// ConvertToStorageClass converts AWS SDK object storage class to CargoShip config storage class
func ConvertToStorageClass(awsClass types.ObjectStorageClass) awsconfig.StorageClass {
	switch awsClass {
	case types.ObjectStorageClassStandard:
		return awsconfig.StorageClassStandard
	case types.ObjectStorageClassStandardIa:
		return awsconfig.StorageClassStandardIA
	case types.ObjectStorageClassOnezoneIa:
		return awsconfig.StorageClassOneZoneIA
	case types.ObjectStorageClassIntelligentTiering:
		return awsconfig.StorageClassIntelligentTiering
	case types.ObjectStorageClassGlacier:
		return awsconfig.StorageClassGlacier
	case types.ObjectStorageClassDeepArchive:
		return awsconfig.StorageClassDeepArchive
	case types.ObjectStorageClassGlacierIr:
		return awsconfig.StorageClassGlacier // Treat Glacier IR as Glacier
	case types.ObjectStorageClassReducedRedundancy:
		return awsconfig.StorageClassStandard // Treat RRS as Standard
	default:
		return awsconfig.StorageClassStandard
	}
}
