package s3

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	awsconfig "github.com/scttfrdmn/cargoship/pkg/aws/config"
	"github.com/scttfrdmn/cargoship/pkg/staging"
)

// StagingTransporter extends the basic S3 transporter with predictive staging capabilities.
type StagingTransporter struct {
	*Transporter
	stagingSystem *staging.PredictiveStager
	config        *StagingConfig
	logger        *slog.Logger
}

// StagingConfig configures the staging-enhanced S3 transporter.
type StagingConfig struct {
	EnableStaging       bool   `yaml:"enable_staging" json:"enable_staging"`
	EnableNetworkAdapt  bool   `yaml:"enable_network_adapt" json:"enable_network_adapt"`
	StageAheadChunks    int    `yaml:"stage_ahead_chunks" json:"stage_ahead_chunks"`
	MaxStagingMemoryMB  int    `yaml:"max_staging_memory_mb" json:"max_staging_memory_mb"`
	NetworkMonitoringHz float64 `yaml:"network_monitoring_hz" json:"network_monitoring_hz"`
}

// NewStagingTransporter creates a new staging-enhanced S3 transporter.
func NewStagingTransporter(ctx context.Context, client *s3.Client, s3Config awsconfig.S3Config, stagingConfig *StagingConfig, logger *slog.Logger) (*StagingTransporter, error) {
	// Create base transporter
	baseTransporter := NewTransporter(client, s3Config)
	
	if stagingConfig == nil {
		stagingConfig = DefaultStagingConfig()
	}
	
	if logger == nil {
		logger = slog.Default()
	}
	
	st := &StagingTransporter{
		Transporter: baseTransporter,
		config:      stagingConfig,
		logger:      logger,
	}
	
	// Initialize staging system if enabled
	if stagingConfig.EnableStaging {
		stagingSystemConfig := &staging.StagingConfig{
			MaxBufferSizeMB:         stagingConfig.MaxStagingMemoryMB,
			TargetChunkSizeMB:       int(s3Config.MultipartChunkSize / (1024 * 1024)),
			MaxConcurrentStaging:    s3Config.Concurrency,
			StagingQueueDepth:       stagingConfig.StageAheadChunks * 2,
			ContentAnalysisWindow:   16, // 16KB analysis windows
			NetworkPredictionWindow: time.Second * 30,
			ChunkBoundaryLookahead:  stagingConfig.StageAheadChunks,
			EnableAdaptiveSizing:    stagingConfig.EnableNetworkAdapt,
			EnableContentAnalysis:   true,
			EnableNetworkPrediction: stagingConfig.EnableNetworkAdapt,
			MemoryPressureThreshold: 0.8,
			GCTriggerThreshold:      0.9,
		}
		
		var err error
		st.stagingSystem, err = st.initializeStagingSystem(ctx, stagingSystemConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize staging system: %w", err)
		}
		
		logger.Info("staging-enhanced S3 transporter initialized", 
			"staging_enabled", true,
			"max_staging_memory_mb", stagingConfig.MaxStagingMemoryMB,
			"stage_ahead_chunks", stagingConfig.StageAheadChunks)
	} else {
		logger.Info("staging-enhanced S3 transporter initialized", "staging_enabled", false)
	}
	
	return st, nil
}

// DefaultStagingConfig returns default staging configuration.
func DefaultStagingConfig() *StagingConfig {
	return &StagingConfig{
		EnableStaging:       true,
		EnableNetworkAdapt:  true,
		StageAheadChunks:    3,
		MaxStagingMemoryMB:  256, // 256MB staging buffer
		NetworkMonitoringHz: 0.2, // Monitor every 5 seconds
	}
}

// UploadWithStaging uploads an archive using predictive staging for optimal performance.
func (st *StagingTransporter) UploadWithStaging(ctx context.Context, archive Archive) (*UploadResult, error) {
	if !st.config.EnableStaging || st.stagingSystem == nil {
		// Fall back to regular upload
		return st.Upload(ctx, archive)
	}
	
	st.logger.Debug("starting staged upload", 
		"key", archive.Key,
		"size", archive.Size,
		"compression_type", archive.CompressionType)
	
	startTime := time.Now()
	
	// Create staged upload context
	uploadCtx := &StagedUploadContext{
		Archive:       archive,
		StartTime:     startTime,
		TotalSize:     archive.Size,
		UploadedSize:  0,
		ChunkCount:    0,
		Errors:        make([]error, 0),
		NetworkMetrics: &NetworkMetrics{
			StartTime: startTime,
		},
	}
	
	// Perform staging-optimized upload
	result, err := st.performStagedUpload(ctx, uploadCtx)
	if err != nil {
		st.logger.Error("staged upload failed", 
			"key", archive.Key, 
			"error", err,
			"chunks_uploaded", uploadCtx.ChunkCount,
			"uploaded_size", uploadCtx.UploadedSize)
		return nil, err
	}
	
	// Update staging system with performance data
	st.updateStagingPerformance(uploadCtx, result)
	
	st.logger.Info("staged upload completed", 
		"key", archive.Key,
		"duration", result.Duration,
		"throughput_mbps", result.Throughput,
		"chunks", uploadCtx.ChunkCount,
		"staging_efficiency", st.calculateStagingEfficiency(uploadCtx))
	
	return result, nil
}

// initializeStagingSystem initializes the predictive staging system.
func (st *StagingTransporter) initializeStagingSystem(ctx context.Context, config *staging.StagingConfig) (*staging.PredictiveStager, error) {
	stager := staging.NewPredictiveStager(ctx, config)
	
	if err := stager.Start(); err != nil {
		return nil, fmt.Errorf("failed to start staging system: %w", err)
	}
	
	return stager, nil
}

// performStagedUpload performs the actual upload with staging optimization.
func (st *StagingTransporter) performStagedUpload(ctx context.Context, uploadCtx *StagedUploadContext) (*UploadResult, error) {
	// Optimize storage class
	storageClass := st.optimizeStorageClass(uploadCtx.Archive)
	
	// For small files, use regular upload
	if uploadCtx.TotalSize < int64(st.config.MaxStagingMemoryMB/4*1024*1024) {
		return st.uploadSmallFile(ctx, uploadCtx, storageClass)
	}
	
	// For large files, use multipart upload with staging
	return st.uploadLargeFileWithStaging(ctx, uploadCtx, storageClass)
}

// uploadSmallFile uploads small files without staging overhead.
func (st *StagingTransporter) uploadSmallFile(ctx context.Context, uploadCtx *StagedUploadContext, storageClass types.StorageClass) (*UploadResult, error) {
	st.logger.Debug("uploading small file without staging", "size", uploadCtx.TotalSize)
	
	// Use regular upload for small files
	return st.Upload(ctx, uploadCtx.Archive)
}

// uploadLargeFileWithStaging uploads large files using predictive staging.
func (st *StagingTransporter) uploadLargeFileWithStaging(ctx context.Context, uploadCtx *StagedUploadContext, storageClass types.StorageClass) (*UploadResult, error) {
	st.logger.Debug("uploading large file with staging", "size", uploadCtx.TotalSize)
	
	// Create multipart upload
	createResp, err := st.client.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{
		Bucket:       aws.String(st.Transporter.config.Bucket),
		Key:          aws.String(uploadCtx.Archive.Key),
		StorageClass: storageClass,
		Metadata:     st.buildMetadata(uploadCtx.Archive),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create multipart upload: %w", err)
	}
	
	uploadID := aws.ToString(createResp.UploadId)
	uploadCtx.UploadID = uploadID
	
	st.logger.Debug("created multipart upload", "upload_id", uploadID)
	
	// Upload parts with staging
	completedParts, err := st.uploadPartsWithStaging(ctx, uploadCtx)
	if err != nil {
		// Abort multipart upload on error
		if abortErr := st.abortMultipartUpload(ctx, uploadCtx.Archive.Key, uploadID); abortErr != nil {
			st.logger.Error("failed to abort multipart upload", "upload_id", uploadID, "error", abortErr)
		}
		return nil, fmt.Errorf("failed to upload parts: %w", err)
	}
	
	// Complete multipart upload
	completeResp, err := st.client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String(st.Transporter.config.Bucket),
		Key:      aws.String(uploadCtx.Archive.Key),
		UploadId: aws.String(uploadID),
		MultipartUpload: &types.CompletedMultipartUpload{
			Parts: completedParts,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to complete multipart upload: %w", err)
	}
	
	duration := time.Since(uploadCtx.StartTime)
	throughput := float64(uploadCtx.TotalSize) / duration.Seconds() / (1024 * 1024) // MB/s
	
	return &UploadResult{
		Location:     aws.ToString(completeResp.Location),
		Key:          uploadCtx.Archive.Key,
		ETag:         aws.ToString(completeResp.ETag),
		UploadID:     uploadID,
		Duration:     duration,
		Throughput:   throughput,
		StorageClass: storageClass,
	}, nil
}

// uploadPartsWithStaging uploads multipart chunks using predictive staging.
func (st *StagingTransporter) uploadPartsWithStaging(ctx context.Context, uploadCtx *StagedUploadContext) ([]types.CompletedPart, error) {
	var completedParts []types.CompletedPart
	partNumber := int32(1)
	buffer := make([]byte, st.Transporter.config.MultipartChunkSize)
	
	for {
		// Read next chunk
		n, err := uploadCtx.Archive.Reader.Read(buffer)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read chunk: %w", err)
		}
		
		chunkData := buffer[:n]
		
		// Stage and upload chunk
		stagingReq := &staging.StagingRequest{
			StreamID:         fmt.Sprintf("%s-part-%d", uploadCtx.Archive.Key, partNumber),
			Reader:           &ChunkReader{data: chunkData},
			ExpectedSize:     int64(n),
			ContentType:      st.classifyContentType(uploadCtx.Archive.CompressionType),
			NetworkCondition: st.getCurrentNetworkCondition(),
			Priority:         int(partNumber), // Earlier parts have higher priority
		}
		
		// Stage chunk predictively
		if err := st.stagingSystem.StageChunks(stagingReq); err != nil {
			st.logger.Warn("staging failed, uploading directly", "part", partNumber, "error", err)
			// Fall back to direct upload
		}
		
		// Upload chunk
		uploadResp, err := st.client.UploadPart(ctx, &s3.UploadPartInput{
			Bucket:     aws.String(st.Transporter.config.Bucket),
			Key:        aws.String(uploadCtx.Archive.Key),
			UploadId:   aws.String(uploadCtx.UploadID),
			PartNumber: aws.Int32(partNumber),
			Body:       &ChunkReader{data: chunkData},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to upload part %d: %w", partNumber, err)
		}
		
		// Track completed part
		completedParts = append(completedParts, types.CompletedPart{
			ETag:       uploadResp.ETag,
			PartNumber: aws.Int32(partNumber),
		})
		
		// Update progress
		uploadCtx.UploadedSize += int64(n)
		uploadCtx.ChunkCount++
		
		// Update network metrics
		st.updateNetworkMetrics(uploadCtx, int64(n))
		
		partNumber++
		
		// Check for cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
	}
	
	return completedParts, nil
}

// updateStagingPerformance updates the staging system with actual performance data.
func (st *StagingTransporter) updateStagingPerformance(uploadCtx *StagedUploadContext, result *UploadResult) {
	if st.stagingSystem == nil {
		return
	}
	
	// Create performance record
	record := &staging.ChunkPerformanceRecord{
		ChunkID:          fmt.Sprintf("%s-upload", uploadCtx.Archive.Key),
		Size:             uploadCtx.TotalSize,
		CompressionRatio: st.calculateCompressionRatio(uploadCtx.Archive),
		UploadTime:       result.Duration,
		ThroughputMBps:   result.Throughput,
		NetworkCondition: st.getCurrentNetworkCondition(),
		Success:          true,
		ErrorType:        "",
		Timestamp:        time.Now(),
	}
	
	// Update performance history
	st.stagingSystem.UpdatePerformance(record.ChunkID, record)
}

// calculateCompressionRatio calculates the compression ratio for an archive.
func (st *StagingTransporter) calculateCompressionRatio(archive Archive) float64 {
	if archive.OriginalSize == 0 {
		return 1.0 // No compression
	}
	
	return float64(archive.Size) / float64(archive.OriginalSize)
}

// getCurrentNetworkCondition gets current network condition for staging decisions.
func (st *StagingTransporter) getCurrentNetworkCondition() *staging.NetworkCondition {
	// In a real implementation, this would get actual network metrics
	// For now, return a default condition
	return &staging.NetworkCondition{
		Timestamp:       time.Now(),
		BandwidthMBps:   50.0,  // Assume 50 MB/s
		LatencyMs:       20.0,  // Assume 20ms latency
		PacketLoss:      0.001, // Assume 0.1% packet loss
		Jitter:          2.0,   // Assume 2ms jitter
		CongestionLevel: 0.1,   // Assume 10% congestion
		Reliability:     0.95,  // Assume 95% reliability
		PredictedTrend:  staging.TrendStable,
	}
}

// classifyContentType classifies content type for staging decisions.
func (st *StagingTransporter) classifyContentType(compressionType string) string {
	switch compressionType {
	case "zstd", "gzip", "bzip2":
		return "compressed"
	case "tar":
		return "binary"
	default:
		return "binary"
	}
}

// updateNetworkMetrics updates network performance metrics.
func (st *StagingTransporter) updateNetworkMetrics(uploadCtx *StagedUploadContext, bytesTransferred int64) {
	now := time.Now()
	duration := now.Sub(uploadCtx.NetworkMetrics.LastUpdate)
	
	if duration > 0 {
		throughput := float64(bytesTransferred) / duration.Seconds() / (1024 * 1024) // MB/s
		uploadCtx.NetworkMetrics.CurrentThroughput = throughput
		uploadCtx.NetworkMetrics.LastUpdate = now
	}
}

// calculateStagingEfficiency calculates the efficiency gain from staging.
func (st *StagingTransporter) calculateStagingEfficiency(uploadCtx *StagedUploadContext) float64 {
	if uploadCtx.NetworkMetrics.CurrentThroughput == 0 {
		return 0.0
	}
	
	// Simple efficiency calculation based on achieved vs expected throughput
	expectedThroughput := 50.0 // Baseline expectation
	return uploadCtx.NetworkMetrics.CurrentThroughput / expectedThroughput
}

// abortMultipartUpload aborts a multipart upload.
func (st *StagingTransporter) abortMultipartUpload(ctx context.Context, key, uploadID string) error {
	_, err := st.client.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{
		Bucket:   aws.String(st.Transporter.config.Bucket),
		Key:      aws.String(key),
		UploadId: aws.String(uploadID),
	})
	return err
}

// GetStagingMetrics returns current staging performance metrics.
func (st *StagingTransporter) GetStagingMetrics() *staging.StagingMetrics {
	if st.stagingSystem == nil {
		return nil
	}
	
	return st.stagingSystem.GetMetrics()
}

// Stop gracefully shuts down the staging system.
func (st *StagingTransporter) Stop() error {
	if st.stagingSystem != nil {
		return st.stagingSystem.Stop()
	}
	return nil
}

// StagedUploadContext tracks the state of a staged upload operation.
type StagedUploadContext struct {
	Archive        Archive
	UploadID       string
	StartTime      time.Time
	TotalSize      int64
	UploadedSize   int64
	ChunkCount     int
	Errors         []error
	NetworkMetrics *NetworkMetrics
}

// NetworkMetrics tracks network performance during upload.
type NetworkMetrics struct {
	StartTime         time.Time
	LastUpdate        time.Time
	CurrentThroughput float64
	AverageThroughput float64
	PeakThroughput    float64
}

// ChunkReader provides an io.Reader interface for chunk data.
type ChunkReader struct {
	data   []byte
	offset int
}

// Read implements io.Reader for chunk data.
func (cr *ChunkReader) Read(p []byte) (n int, err error) {
	if cr.offset >= len(cr.data) {
		return 0, io.EOF
	}
	
	n = copy(p, cr.data[cr.offset:])
	cr.offset += n
	return n, nil
}

// Seek implements io.Seeker for compatibility with AWS SDK.
func (cr *ChunkReader) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		cr.offset = int(offset)
	case io.SeekCurrent:
		cr.offset += int(offset)
	case io.SeekEnd:
		cr.offset = len(cr.data) + int(offset)
	}
	
	if cr.offset < 0 {
		cr.offset = 0
	}
	if cr.offset > len(cr.data) {
		cr.offset = len(cr.data)
	}
	
	return int64(cr.offset), nil
}