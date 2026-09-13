// Package s3 provides AWS S3 transport implementation for CargoShip
package s3

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager"
	tmtypes "github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	awsconfig "github.com/scttfrdmn/cargoship/pkg/aws/config"
	"github.com/scttfrdmn/cargoship/pkg/observability/tracing"
	"go.opentelemetry.io/otel/trace"
)

// Transporter implements S3-based transport for CargoShip
type Transporter struct {
	client   *s3.Client
	uploader *transfermanager.Client
	config   awsconfig.S3Config
	tracer   *tracing.S3Tracer // Optional: S3 operation tracer (Issue #155)
}

// Archive represents a CargoShip archive for upload
type Archive struct {
	Key             string                 // S3 object key
	Reader          io.Reader              // Archive content
	Size            int64                  // Archive size in bytes
	StorageClass    awsconfig.StorageClass // Target storage class
	Metadata        map[string]string      // Custom metadata
	OriginalSize    int64                  // Original uncompressed size
	CompressionType string                 // Compression algorithm used
	AccessPattern   string                 // Expected access pattern
	RetentionDays   int                    // Expected retention period

	// ContentEncoding, when non-empty, is set as the object's HTTP
	// `Content-Encoding` header (e.g. "zstd", "gzip") so a standards-conforming
	// reader — aws s3 cp, boto3, a browser fetch — knows to decode the body
	// (#353). It is an EXPLICIT signal a caller sets when it hands CargoShip a
	// body it has already content-encoded; it is deliberately NOT derived from
	// CompressionType. CargoShip's own chunk objects leave this empty on purpose:
	// a .tar.zst chunk is addressed by its key extension and read as raw bytes
	// (and, for format-2.1 random access, via byte-range GETs), so stamping
	// Content-Encoding on it would make HTTP clients auto-decompress the whole
	// object and break ranged reads. CompressionType remains a private
	// x-amz-meta-* annotation, unchanged.
	ContentEncoding string
}

// UploadResult contains the result of an S3 upload
type UploadResult struct {
	Location     string             // S3 URL
	Key          string             // S3 object key
	ETag         string             // S3 ETag
	UploadID     string             // Multipart upload ID (if used)
	Duration     time.Duration      // Upload duration
	Throughput   float64            // Upload throughput in MB/s
	StorageClass types.StorageClass // Actual storage class used
}

// NewTransporter creates a new S3 transporter
func NewTransporter(client *s3.Client, config awsconfig.S3Config) *Transporter {
	// #384: migrated off the deprecated feature/s3/manager Uploader to
	// feature/s3/transfermanager. transfermanager aborts a failed multipart upload
	// by default (no LeavePartsOnError knob) and manages its own part buffers (no
	// BufferProvider); part size and concurrency map directly.
	uploader := transfermanager.New(client, func(o *transfermanager.Options) {
		o.PartSizeBytes = config.MultipartChunkSize
		o.Concurrency = config.Concurrency
		// #522: transfermanager defaults to calculating a CRC32 on every upload,
		// which is ~5% of upload CPU and redundant with CargoShip's own per-archive
		// SHA-256 (#271) + verify-on-restore. Only compute a checksum when the API
		// requires it. TLS still protects bytes on the wire.
		o.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
	})

	return &Transporter{
		client:   client,
		uploader: uploader,
		config:   config,
	}
}

// SetTracer sets the S3 tracer for distributed tracing (Issue #155)
func (t *Transporter) SetTracer(tracer *tracing.S3Tracer) {
	t.tracer = tracer
}

// putObjectInput builds the S3 PutObjectInput for an archive. Extracted from
// Upload so the header/metadata mapping — notably the #353 Content-Encoding
// rule — is unit-testable without a live S3 client.
func (t *Transporter) putObjectInput(archive Archive, storageClass types.StorageClass) *transfermanager.UploadObjectInput {
	input := &transfermanager.UploadObjectInput{
		Bucket: aws.String(t.config.Bucket),
		Key:    aws.String(archive.Key),
		Body:   archive.Reader,
		// #384: transfermanager has its own StorageClass enum with identical string
		// values (STANDARD, GLACIER, …), so a string cast preserves the class.
		StorageClass: tmtypes.StorageClass(string(storageClass)),
		Metadata:     t.buildMetadata(archive),
	}

	// #353: set the HTTP Content-Encoding header when the caller pre-encoded the
	// body, so standards-conforming readers decode it. Empty for CargoShip's own
	// chunks (see Archive.ContentEncoding) — CompressionType is NOT used here.
	if archive.ContentEncoding != "" {
		input.ContentEncoding = aws.String(archive.ContentEncoding)
	}

	// Add KMS encryption if configured
	if t.config.KMSKeyID != "" {
		input.ServerSideEncryption = tmtypes.ServerSideEncryptionAwsKms
		input.SSEKMSKeyID = aws.String(t.config.KMSKeyID)
	}

	return input
}

// Upload uploads an archive to S3 with intelligent storage class selection
func (t *Transporter) Upload(ctx context.Context, archive Archive) (*UploadResult, error) {
	startTime := time.Now()

	// Optimize storage class based on archive characteristics
	storageClass := t.optimizeStorageClass(archive)

	// Create S3 operation span if tracing enabled (Issue #155)
	var span trace.Span
	if t.tracer != nil {
		ctx, span = t.tracer.StartUploadSpan(ctx, t.config.Bucket, archive.Key, archive.Size)
		defer span.End()

		// Add transporter info and storage class
		t.tracer.AddTransporterInfo(span, "basic", t.config.Concurrency)
		t.tracer.AddStorageClass(span, string(storageClass))
	}

	// Prepare upload input (extracted for unit testing — see putObjectInput).
	input := t.putObjectInput(archive, storageClass)

	// Perform upload
	result, err := t.uploader.UploadObject(ctx, input)
	if err != nil {
		// Record error in span
		if span != nil && t.tracer != nil {
			t.tracer.RecordError(span, err)
		}
		return nil, fmt.Errorf("failed to upload archive to s3://%s/%s (size: %d bytes, storage class: %s): %w",
			t.config.Bucket, archive.Key, archive.Size, storageClass, err)
	}

	duration := time.Since(startTime)
	throughput := float64(archive.Size) / duration.Seconds() / (1024 * 1024) // MB/s

	// Record success and metrics in span
	if span != nil && t.tracer != nil {
		t.tracer.RecordSuccess(span)
		t.tracer.AddUploadMetrics(span, archive.Size, duration.Milliseconds(), throughput)
	}

	return &UploadResult{
		// #384: transfermanager returns *string for these (manager returned string).
		Location:     aws.ToString(result.Location),
		Key:          archive.Key,
		ETag:         aws.ToString(result.ETag),
		UploadID:     aws.ToString(result.UploadID),
		Duration:     duration,
		Throughput:   throughput,
		StorageClass: storageClass,
	}, nil
}

// optimizeStorageClass selects the optimal storage class based on archive characteristics
func (t *Transporter) optimizeStorageClass(archive Archive) types.StorageClass {
	// An explicitly requested class is a decision the caller already made, so it
	// wins over both the configured default and the heuristics below (which are
	// for archives that express an intent via AccessPattern/RetentionDays rather
	// than naming a class). Without this, a per-object StorageClass is silently
	// ignored on this path while OptimizedTransporter honours it — see #352.
	if archive.StorageClass != "" {
		return types.StorageClass(archive.StorageClass)
	}

	// Use configured default if no optimization criteria
	if archive.AccessPattern == "" && archive.RetentionDays == 0 {
		return types.StorageClass(t.config.StorageClass)
	}

	// Deep Archive for long-term archival with no expected access
	if archive.AccessPattern == "archive" && archive.RetentionDays > 365 {
		return types.StorageClassDeepArchive
	}

	// Glacier for long-term storage with rare access
	if archive.RetentionDays > 90 || archive.AccessPattern == "rare" {
		return types.StorageClassGlacier
	}

	// Standard-IA for infrequent access
	if archive.AccessPattern == "infrequent" {
		return types.StorageClassStandardIa
	}

	// Intelligent Tiering for unknown access patterns
	if archive.AccessPattern == "unknown" || archive.AccessPattern == "" {
		return types.StorageClassIntelligentTiering
	}

	// Default to Standard for frequent access
	return types.StorageClassStandard
}

// buildMetadata creates S3 metadata from archive information
func (t *Transporter) buildMetadata(archive Archive) map[string]string {
	metadata := make(map[string]string)

	// Copy custom metadata
	for k, v := range archive.Metadata {
		metadata[k] = v
	}

	// Add CargoShip-specific metadata
	metadata["cargoship-original-size"] = strconv.FormatInt(archive.OriginalSize, 10)
	metadata["cargoship-compression-type"] = archive.CompressionType
	metadata["cargoship-created-by"] = "cargoship"
	metadata["cargoship-upload-time"] = time.Now().UTC().Format(time.RFC3339)

	if archive.AccessPattern != "" {
		metadata["cargoship-access-pattern"] = archive.AccessPattern
	}

	if archive.RetentionDays > 0 {
		metadata["cargoship-retention-days"] = strconv.Itoa(archive.RetentionDays)
	}

	return metadata
}

// Exists checks if an object exists in S3
func (t *Transporter) Exists(ctx context.Context, key string) (bool, error) {
	_, err := t.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(t.config.Bucket),
		Key:    aws.String(key),
	})

	if err != nil {
		// Check if it's a "not found" error
		var notFound *types.NotFound
		if errors.As(err, &notFound) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check if object exists at s3://%s/%s: %w", t.config.Bucket, key, err)
	}

	return true, nil
}

// GetObjectInfo retrieves metadata about an object
func (t *Transporter) GetObjectInfo(ctx context.Context, key string) (*s3.HeadObjectOutput, error) {
	output, err := t.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(t.config.Bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get object info for s3://%s/%s: %w", t.config.Bucket, key, err)
	}
	return output, nil
}

// GetConfig returns the current transport configuration
func (t *Transporter) GetConfig() awsconfig.S3Config {
	return t.config
}

// Compile-time check that Transporter implements BasicTransporter interface
var _ BasicTransporter = (*Transporter)(nil)
