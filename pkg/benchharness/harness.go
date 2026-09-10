// Package benchharness runs a CargoShip upload+restore of a reproducible corpus
// and measures its speed and full S3 cost from EXACT, middleware-counted requests
// — the Phase-1 (CargoShip-only) benchmark. Competitor tools are a later phase.
package benchharness

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/scttfrdmn/cargoship/pkg/aws/config"
	"github.com/scttfrdmn/cargoship/pkg/aws/pricingfallback"
	"github.com/scttfrdmn/cargoship/pkg/corpus"
	"github.com/scttfrdmn/cargoship/pkg/manifest"
	"github.com/scttfrdmn/cargoship/pkg/pipeline"
	"github.com/scttfrdmn/cargoship/pkg/s3count"
)

// Options configures one benchmark run.
type Options struct {
	Profile    corpus.Profile
	Client     *awss3.Client    // MUST already be instrumented by Counter (see Run)
	Counter    *s3count.Counter // the counter wired into Client
	Bucket     string
	Prefix     string // S3 key prefix for this run (isolates it)
	Region     string
	SrcDir     string // where to plant the corpus (caller owns cleanup)
	RestoreDir string // where to restore (caller owns cleanup)
}

// Phase captures the speed + request counts of one leg (upload or restore).
type Phase struct {
	Duration time.Duration  `json:"duration_ms"`
	Bytes    int64          `json:"bytes"`
	MBPerSec float64        `json:"mb_per_sec"`
	OpCounts map[string]int `json:"op_counts"`
}

// Cost is the modeled S3 bill (USD), from measured counts + stored bytes, at the
// STANDARD storage class. Ingress is free (#451); storage is monthly.
type Cost struct {
	UploadRequestsUSD  float64 `json:"upload_requests_usd"`
	MonthlyStorageUSD  float64 `json:"monthly_storage_usd"`
	RestoreRequestsUSD float64 `json:"restore_requests_usd"`
	RestoreEgressUSD   float64 `json:"restore_egress_usd"`
}

// RunResult is the full record for one profile.
type RunResult struct {
	Profile       string `json:"profile"`
	Files         int    `json:"files"`
	SourceBytes   int64  `json:"source_bytes"`
	StoredBytes   int64  `json:"stored_bytes"` // sum of chunk CompressedSize (what S3 holds)
	Chunks        int    `json:"chunks"`
	Upload        Phase  `json:"upload"`
	Restore       Phase  `json:"restore"`
	Cost          Cost   `json:"cost"`
	ByteIdentical bool   `json:"byte_identical"`
}

// Run plants the profile, uploads it with the instrumented client, restores it,
// verifies byte-identity, and computes speed + cost. The caller builds Client
// with opts.Counter.Instrument applied before NewFromConfig, and owns bucket
// lifecycle + SrcDir/RestoreDir cleanup.
func Run(ctx context.Context, opts Options) (*RunResult, error) {
	files, err := opts.Profile.Plant(opts.SrcDir)
	if err != nil {
		return nil, fmt.Errorf("plant corpus: %w", err)
	}
	var srcBytes int64
	for _, f := range files {
		srcBytes += int64(f.Size)
	}

	uploadID := fmt.Sprintf("%d-bench", time.Now().UnixNano())
	pc := &pipeline.PipelineConfig{
		ScannerWorkers: 4, ArchiverWorkers: 4, UploaderWorkers: 4,
		S3Bucket: opts.Bucket, S3Prefix: opts.Prefix, S3Region: opts.Region,
		UseRealS3: true, S3Client: opts.Client, S3PartSize: 5 * 1024 * 1024,
		EnableManifest: true, SourcePath: opts.SrcDir, UploadID: uploadID,
		EnableMultiPrefix: true, ShardCount: 4, FileChecksums: true,
	}

	// --- Upload leg ---
	opts.Counter.Reset()
	p, err := pipeline.NewPipeline(pc)
	if err != nil {
		return nil, fmt.Errorf("new pipeline: %w", err)
	}
	upStart := time.Now()
	res, err := p.Run(ctx, opts.SrcDir)
	upDur := time.Since(upStart)
	if err != nil {
		return nil, fmt.Errorf("upload run: %w", err)
	}
	if !res.Success {
		return nil, fmt.Errorf("upload did not succeed (%d failed jobs)", len(res.FailedJobs))
	}
	uploadCounts := opts.Counter.Counts()

	// Fetch + parse the manifest (the source of truth for stored bytes/chunks).
	manifestKey := fmt.Sprintf("%s/uploads/%s/manifest.json.gz", opts.Prefix, uploadID)
	obj, err := opts.Client.GetObject(ctx, &awss3.GetObjectInput{Bucket: aws.String(opts.Bucket), Key: aws.String(manifestKey)})
	if err != nil {
		return nil, fmt.Errorf("get manifest: %w", err)
	}
	mBytes, err := io.ReadAll(obj.Body)
	_ = obj.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	m, err := manifest.FromJSONCompressed(mBytes)
	if err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	var storedBytes int64
	for _, c := range m.Chunks {
		storedBytes += c.CompressedSize
	}
	if storedBytes == 0 { // direct-upload manifests have no chunks
		storedBytes = res.TotalBytes
	}

	// --- Restore leg (whole corpus) ---
	opts.Counter.Reset()
	se := manifest.NewSelectiveExtractor(m, opts.Client, 0)
	targets := make([]string, len(files))
	for i, f := range files {
		targets[i] = f.RelPath
	}
	rStart := time.Now()
	stats, err := se.BatchRestore(ctx, targets, opts.RestoreDir)
	rDur := time.Since(rStart)
	if err != nil {
		return nil, fmt.Errorf("restore: %w", err)
	}
	restoreCounts := opts.Counter.Counts()
	if stats.Failed != 0 {
		return nil, fmt.Errorf("restore had %d failures", stats.Failed)
	}

	byteIdentical, err := verifyByteIdentical(files, opts.RestoreDir)
	if err != nil {
		return nil, err
	}

	rr := &RunResult{
		Profile:     opts.Profile.Name,
		Files:       len(files),
		SourceBytes: srcBytes,
		StoredBytes: storedBytes,
		Chunks:      len(m.Chunks),
		Upload: Phase{
			Duration: upDur, Bytes: res.TotalBytes,
			MBPerSec: mbPerSec(res.TotalBytes, upDur), OpCounts: uploadCounts,
		},
		Restore: Phase{
			Duration: rDur, Bytes: stats.Bytes,
			MBPerSec: mbPerSec(stats.Bytes, rDur), OpCounts: restoreCounts,
		},
		ByteIdentical: byteIdentical,
	}
	rr.Cost = computeCost(storedBytes, stats.Bytes, uploadCounts, restoreCounts)
	return rr, nil
}

func mbPerSec(bytes int64, d time.Duration) float64 {
	if d <= 0 {
		return 0
	}
	return (float64(bytes) / (1024 * 1024)) / d.Seconds()
}

// verifyByteIdentical confirms every planted file restored with its exact bytes
// (matched by basename, as BatchRestore flattens to the dataset-relative path).
func verifyByteIdentical(files []corpus.File, restoreDir string) (bool, error) {
	idx, err := corpus.IndexByBase(restoreDir)
	if err != nil {
		return false, err
	}
	for _, f := range files {
		path, ok := idx[f.Base]
		if !ok {
			return false, nil
		}
		got, err := os.ReadFile(path)
		if err != nil {
			return false, err
		}
		sum := sha256.Sum256(got)
		if hex.EncodeToString(sum[:]) != f.Sum {
			return false, nil
		}
	}
	return true, nil
}

// requestTier maps an S3 operation name to the fallback pricing request type.
func requestTier(op string) string {
	switch op {
	case "PutObject", "CreateMultipartUpload", "UploadPart", "CompleteMultipartUpload", "CopyObject":
		return "PUT"
	case "ListObjectsV2", "ListObjects", "ListMultipartUploads", "ListParts":
		return "LIST"
	case "GetObject", "HeadObject", "HeadBucket":
		return "GET"
	default: // DeleteObject, AbortMultipartUpload, unknown → free tier
		return "DELETE"
	}
}

// computeCost models the STANDARD-class S3 bill from measured counts + stored
// bytes. Ingress (upload data transfer) is free (#451); storage is monthly;
// restore includes GET-tier requests + egress ($0.09/GB).
func computeCost(storedBytes, restoreBytes int64, upload, restore map[string]int) Cost {
	const std = config.StorageClassStandard
	reqUSD := func(counts map[string]int) float64 {
		var usd float64
		for op, n := range counts {
			usd += (float64(n) / 1000.0) * pricingfallback.RequestPrice(requestTier(op), std)
		}
		return usd
	}
	storedGB := float64(storedBytes) / (1024 * 1024 * 1024)
	restoreGB := float64(restoreBytes) / (1024 * 1024 * 1024)
	return Cost{
		UploadRequestsUSD:  reqUSD(upload),
		MonthlyStorageUSD:  storedGB * pricingfallback.StoragePrice(std),
		RestoreRequestsUSD: reqUSD(restore),
		RestoreEgressUSD:   restoreGB * 0.09, // $0.09/GB egress
	}
}
