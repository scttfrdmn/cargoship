package manifest

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/scttfrdmn/cargoship/pkg/encryption"
)

const (
	// ManifestVersion is the current manifest format version
	ManifestVersion = "1.0"

	// ManifestFileName is the standard manifest filename
	ManifestFileName = "manifest.json"

	// ManifestFileNameGZ is the compressed manifest filename
	ManifestFileNameGZ = "manifest.json.gz"
)

// Builder helps construct a manifest incrementally during upload
// All methods are thread-safe for concurrent use (Issue #88)
type Builder struct {
	mu       sync.RWMutex
	manifest *Manifest

	// Hostname for upload source tracking
	hostname string
}

// NewBuilder creates a new manifest builder
func NewBuilder(uploadID, sourcePath, bucket, prefix, region string) (*Builder, error) {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	return &Builder{
		manifest: &Manifest{
			Version:    ManifestVersion,
			UploadID:   uploadID,
			CreatedAt:  time.Now(),
			SourcePath: sourcePath,
			Hostname:   hostname,
			Bucket:     bucket,
			Prefix:     prefix,
			Region:     region,
			Files:      make([]FileEntry, 0),
			Chunks:     make([]ChunkEntry, 0),
			Shards:     make([]ShardEntry, 0),
		},
		hostname: hostname,
	}, nil
}

// NewBuilderFromExisting creates a builder from an existing manifest for resume (Issue #157)
func NewBuilderFromExisting(existing *Manifest) (*Builder, error) {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	// Clone the existing manifest to avoid modifying the original
	cloned := &Manifest{
		Version:          existing.Version,
		UploadID:         existing.UploadID,
		CreatedAt:        existing.CreatedAt,
		CompletedAt:      existing.CompletedAt,
		SourcePath:       existing.SourcePath,
		Hostname:         hostname, // Use current hostname for resumed portion
		Bucket:           existing.Bucket,
		Prefix:           existing.Prefix,
		Region:           existing.Region,
		TotalFiles:       existing.TotalFiles,
		TotalBytes:       existing.TotalBytes,
		TotalChunks:      existing.TotalChunks,
		ShardCount:       existing.ShardCount,
		CompressionType:  existing.CompressionType,
		CompressionLevel: existing.CompressionLevel,
		CompressionRatio: existing.CompressionRatio,
		Files:            append([]FileEntry(nil), existing.Files...),
		Chunks:           append([]ChunkEntry(nil), existing.Chunks...),
		Shards:           append([]ShardEntry(nil), existing.Shards...),
	}

	return &Builder{
		manifest: cloned,
		hostname: hostname,
	}, nil
}

// AddFile adds a file to the manifest (thread-safe)
func (b *Builder) AddFile(entry FileEntry) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.manifest.Files = append(b.manifest.Files, entry)
	b.manifest.TotalFiles++
	b.manifest.TotalBytes += entry.Size
}

// AddFileBatch adds multiple files to the manifest in a single lock (Issue #34 Phase 1.4)
// More efficient than calling AddFile repeatedly when adding many files at once
func (b *Builder) AddFileBatch(entries []FileEntry) {
	if len(entries) == 0 {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	// Append all entries at once
	b.manifest.Files = append(b.manifest.Files, entries...)

	// Update totals
	for _, entry := range entries {
		b.manifest.TotalFiles++
		b.manifest.TotalBytes += entry.Size
	}
}

// AddChunk adds a chunk to the manifest (thread-safe)
func (b *Builder) AddChunk(entry ChunkEntry) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.manifest.Chunks = append(b.manifest.Chunks, entry)
	b.manifest.TotalChunks++
}

// SetCompression sets compression information (thread-safe)
func (b *Builder) SetCompression(compressionType string, level int, ratio float64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.manifest.CompressionType = compressionType
	b.manifest.CompressionLevel = level
	b.manifest.CompressionRatio = ratio
}

// SetShardCount sets the number of shards (thread-safe)
func (b *Builder) SetShardCount(count int) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.manifest.ShardCount = count
	// Initialize shard entries
	b.manifest.Shards = make([]ShardEntry, count)
	for i := 0; i < count; i++ {
		b.manifest.Shards[i] = ShardEntry{
			ID:        i,
			Prefix:    fmt.Sprintf("%s/uploads/%s/shard-%d", b.manifest.Prefix, b.manifest.UploadID, i),
			ChunkKeys: make([]string, 0),
		}
	}
}

// SetSyncInfo sets sync-related fields for incremental sync (Issue #148, thread-safe)
func (b *Builder) SetSyncInfo(syncType string, previousUploadID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.manifest.SyncType = syncType
	b.manifest.PreviousManifestID = previousUploadID
}

// SetEncryption sets encryption metadata (Issue #163, thread-safe)
func (b *Builder) SetEncryption(kmsKeyID string, manifestEncrypted bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if kmsKeyID == "" && !manifestEncrypted {
		b.manifest.Encryption = nil
		return
	}

	b.manifest.Encryption = &EncryptionMetadata{
		Enabled:           kmsKeyID != "",
		DataKMSKeyID:      kmsKeyID,
		ManifestEncrypted: manifestEncrypted,
	}
}

// SetDeduplication sets deduplication metadata (Issue #108, thread-safe)
func (b *Builder) SetDeduplication(dedup *ManifestDeduplication) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.manifest.Deduplication = dedup
}

// UpdateShardStats updates statistics for a shard (thread-safe)
func (b *Builder) UpdateShardStats(shardID int, chunkKey string, fileCount int64, uncompressed, compressed int64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if shardID < 0 || shardID >= len(b.manifest.Shards) {
		return
	}

	shard := &b.manifest.Shards[shardID]
	shard.ChunkCount++
	shard.FileCount += fileCount
	shard.UncompressedSize += uncompressed
	shard.CompressedSize += compressed
	shard.ChunkKeys = append(shard.ChunkKeys, chunkKey)
}

// UpdateFileS3Keys updates all files in a chunk with the chunk's S3 key and shard ID (thread-safe)
func (b *Builder) UpdateFileS3Keys(chunkID int, shardID int, s3Key string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for i := range b.manifest.Files {
		if b.manifest.Files[i].ChunkID == chunkID {
			b.manifest.Files[i].ShardID = shardID
			b.manifest.Files[i].S3Key = s3Key
		}
	}
}

// Finalize completes the manifest and returns it (thread-safe)
func (b *Builder) Finalize() *Manifest {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.manifest.CompletedAt = time.Now()
	return b.manifest
}

// Build returns the current manifest (without finalizing, thread-safe)
func (b *Builder) Build() *Manifest {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.manifest
}

// ToJSON serializes the manifest to JSON
func (m *Manifest) ToJSON() ([]byte, error) {
	return json.MarshalIndent(m, "", "  ")
}

// ToJSONCompressed serializes the manifest to gzip-compressed JSON
func (m *Manifest) ToJSONCompressed() ([]byte, error) {
	jsonData, err := m.ToJSON()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON: %w", err)
	}

	var buf bytes.Buffer
	gzipWriter := gzip.NewWriter(&buf)

	if _, err := gzipWriter.Write(jsonData); err != nil {
		return nil, fmt.Errorf("failed to compress JSON: %w", err)
	}

	if err := gzipWriter.Close(); err != nil {
		return nil, fmt.Errorf("failed to close gzip writer: %w", err)
	}

	return buf.Bytes(), nil
}

// FromJSON deserializes a manifest from JSON
func FromJSON(data []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}
	return &m, nil
}

// FromJSONCompressed deserializes a manifest from gzip-compressed JSON
func FromJSONCompressed(data []byte) (*Manifest, error) {
	gzipReader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer func() {
		if closeErr := gzipReader.Close(); closeErr != nil {
			// Log but don't fail on close error during read
			_ = closeErr
		}
	}()

	jsonData, err := io.ReadAll(gzipReader)
	if err != nil {
		return nil, fmt.Errorf("failed to decompress JSON: %w", err)
	}

	return FromJSON(jsonData)
}

// UploadToS3 uploads the manifest to S3
func (m *Manifest) UploadToS3(ctx context.Context, s3Client *s3.Client, compress bool) error {
	var data []byte
	var key string
	var contentType string
	var contentEncoding string
	var err error

	if compress {
		data, err = m.ToJSONCompressed()
		key = fmt.Sprintf("%s/uploads/%s/%s", m.Prefix, m.UploadID, ManifestFileNameGZ)
		contentType = "application/json"
		contentEncoding = "gzip"
	} else {
		data, err = m.ToJSON()
		key = fmt.Sprintf("%s/uploads/%s/%s", m.Prefix, m.UploadID, ManifestFileName)
		contentType = "application/json"
	}

	if err != nil {
		return fmt.Errorf("failed to serialize manifest: %w", err)
	}

	// Upload to S3
	input := &s3.PutObjectInput{
		Bucket:      aws.String(m.Bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String(contentType),
	}

	if contentEncoding != "" {
		input.ContentEncoding = aws.String(contentEncoding)
	}

	_, err = s3Client.PutObject(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to upload manifest to S3: %w", err)
	}

	return nil
}

// UploadToS3WithEncryption uploads the manifest to S3 with optional KMS encryption (Issue #163)
func (m *Manifest) UploadToS3WithEncryption(ctx context.Context, s3Client *s3.Client, kmsClient encryption.KMSClient, compress bool) error {
	// Check if manifest encryption is enabled
	if m.Encryption == nil || !m.Encryption.ManifestEncrypted || m.Encryption.ManifestKMSKeyID == "" {
		// No encryption - use regular upload
		return m.UploadToS3(ctx, s3Client, compress)
	}

	// Serialize manifest to JSON
	var manifestJSON []byte
	var err error
	if compress {
		manifestJSON, err = m.ToJSONCompressed()
	} else {
		manifestJSON, err = m.ToJSON()
	}
	if err != nil {
		return fmt.Errorf("failed to serialize manifest: %w", err)
	}

	// Encrypt the manifest using KMS envelope encryption
	encryptor := encryption.NewKMSEncryptor(kmsClient, m.Encryption.ManifestKMSKeyID)
	encrypted, err := encryptor.EncryptManifest(ctx, manifestJSON)
	if err != nil {
		return fmt.Errorf("failed to encrypt manifest: %w", err)
	}

	// Store encryption metadata in manifest
	m.Encryption.Algorithm = encrypted.Algorithm
	m.Encryption.EncryptedDEK = encrypted.EncryptedDEK

	// Serialize the encrypted manifest wrapper
	encryptedJSON, err := json.MarshalIndent(encrypted, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal encrypted manifest: %w", err)
	}

	// Determine S3 key for encrypted manifest
	key := fmt.Sprintf("%s/uploads/%s/manifest.encrypted.json", m.Prefix, m.UploadID)
	if compress {
		key = fmt.Sprintf("%s/uploads/%s/manifest.encrypted.json.gz", m.Prefix, m.UploadID)

		// Compress the encrypted JSON
		var buf bytes.Buffer
		gzipWriter := gzip.NewWriter(&buf)
		if _, err := gzipWriter.Write(encryptedJSON); err != nil {
			return fmt.Errorf("failed to compress encrypted manifest: %w", err)
		}
		if err := gzipWriter.Close(); err != nil {
			return fmt.Errorf("failed to close gzip writer: %w", err)
		}
		encryptedJSON = buf.Bytes()
	}

	// Upload encrypted manifest to S3
	input := &s3.PutObjectInput{
		Bucket:      aws.String(m.Bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(encryptedJSON),
		ContentType: aws.String("application/json"),
	}

	if compress {
		input.ContentEncoding = aws.String("gzip")
	}

	_, err = s3Client.PutObject(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to upload encrypted manifest to S3: %w", err)
	}

	return nil
}

// DownloadFromS3 downloads a manifest from S3
func DownloadFromS3(ctx context.Context, s3Client *s3.Client, bucket, prefix, uploadID string) (*Manifest, error) {
	// Try compressed version first
	key := fmt.Sprintf("%s/uploads/%s/%s", prefix, uploadID, ManifestFileNameGZ)
	data, err := downloadS3Object(ctx, s3Client, bucket, key)
	if err == nil {
		manifest, err := FromJSONCompressed(data)
		if err == nil {
			return manifest, nil
		}
	}

	// Fall back to uncompressed version
	key = fmt.Sprintf("%s/uploads/%s/%s", prefix, uploadID, ManifestFileName)
	data, err = downloadS3Object(ctx, s3Client, bucket, key)
	if err != nil {
		return nil, fmt.Errorf("failed to download manifest from S3: %w", err)
	}

	return FromJSON(data)
}

// DownloadFromS3WithDecryption downloads a manifest from S3 with optional KMS decryption (Issue #163)
func DownloadFromS3WithDecryption(ctx context.Context, s3Client *s3.Client, kmsClient encryption.KMSClient, bucket, prefix, uploadID string) (*Manifest, error) {
	// Try encrypted compressed version first
	key := fmt.Sprintf("%s/uploads/%s/manifest.encrypted.json.gz", prefix, uploadID)
	data, err := downloadS3Object(ctx, s3Client, bucket, key)
	if err == nil {
		// Decompress
		gzipReader, err := gzip.NewReader(bytes.NewReader(data))
		if err == nil {
			defer func() { _ = gzipReader.Close() }()
			decompressed, err := io.ReadAll(gzipReader)
			if err == nil {
				// Decrypt
				var encryptedManifest encryption.EncryptedManifest
				if err := json.Unmarshal(decompressed, &encryptedManifest); err == nil {
					manifestJSON, err := encryption.DecryptManifestBytes(ctx, kmsClient, &encryptedManifest)
					if err == nil {
						return FromJSON(manifestJSON)
					}
				}
			}
		}
	}

	// Try encrypted uncompressed version
	key = fmt.Sprintf("%s/uploads/%s/manifest.encrypted.json", prefix, uploadID)
	data, err = downloadS3Object(ctx, s3Client, bucket, key)
	if err == nil {
		var encryptedManifest encryption.EncryptedManifest
		if err := json.Unmarshal(data, &encryptedManifest); err == nil {
			manifestJSON, err := encryption.DecryptManifestBytes(ctx, kmsClient, &encryptedManifest)
			if err == nil {
				return FromJSON(manifestJSON)
			}
		}
	}

	// Fall back to regular (non-encrypted) manifest download
	return DownloadFromS3(ctx, s3Client, bucket, prefix, uploadID)
}

// DownloadPartialManifestFromS3 downloads a partial manifest from S3 (Issue #157: Resume capability)
func DownloadPartialManifestFromS3(ctx context.Context, s3Client *s3.Client, bucket, prefix, uploadID string) (*Manifest, error) {
	// Try partial manifest (compressed only)
	key := fmt.Sprintf("%s/uploads/%s/manifest.partial.json.gz", prefix, uploadID)
	data, err := downloadS3Object(ctx, s3Client, bucket, key)
	if err != nil {
		return nil, fmt.Errorf("failed to download partial manifest from S3: %w", err)
	}

	return FromJSONCompressed(data)
}

// downloadS3Object downloads an object from S3
func downloadS3Object(ctx context.Context, s3Client *s3.Client, bucket, key string) ([]byte, error) {
	result, err := s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := result.Body.Close(); closeErr != nil {
			// Log but don't fail on close error
			_ = closeErr
		}
	}()

	return io.ReadAll(result.Body)
}

// ParseS3URL parses an S3 URL into bucket and prefix
// Supports formats: s3://bucket/prefix, s3://bucket
func ParseS3URL(s3URL string) (bucket, prefix string, err error) {
	if len(s3URL) < 5 || s3URL[:5] != "s3://" {
		return "", "", fmt.Errorf("invalid S3 URL format: must start with s3://")
	}

	path := s3URL[5:] // Remove "s3://"

	// Find first slash to separate bucket from prefix
	slashIdx := -1
	for i, c := range path {
		if c == '/' {
			slashIdx = i
			break
		}
	}

	if slashIdx == -1 {
		// No prefix, just bucket
		return path, "", nil
	}

	bucket = path[:slashIdx]
	prefix = path[slashIdx+1:]

	return bucket, prefix, nil
}
