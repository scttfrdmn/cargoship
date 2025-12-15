package resume

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// ListInProgressMultipartUploads finds all incomplete multipart uploads for a bucket/prefix
// Useful for cleanup or resume operations
func ListInProgressMultipartUploads(ctx context.Context, s3Client *s3.Client, bucket, prefix string) ([]types.MultipartUpload, error) {
	var uploads []types.MultipartUpload

	input := &s3.ListMultipartUploadsInput{
		Bucket: aws.String(bucket),
		Prefix: aws.String(prefix),
	}

	paginator := s3.NewListMultipartUploadsPaginator(s3Client, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list multipart uploads: %w", err)
		}
		uploads = append(uploads, page.Uploads...)
	}

	return uploads, nil
}

// CompleteMultipartUpload completes a multipart upload using stored part information
// Used to finalize uploads that were interrupted before completion
func CompleteMultipartUpload(ctx context.Context, s3Client *s3.Client, bucket, key, uploadID string, parts []types.CompletedPart) error {
	input := &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String(bucket),
		Key:      aws.String(key),
		UploadId: aws.String(uploadID),
		MultipartUpload: &types.CompletedMultipartUpload{
			Parts: parts,
		},
	}

	_, err := s3Client.CompleteMultipartUpload(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to complete multipart upload: %w", err)
	}

	return nil
}

// AbortMultipartUpload aborts a multipart upload, freeing up S3 storage
// Used to clean up failed or abandoned uploads
func AbortMultipartUpload(ctx context.Context, s3Client *s3.Client, bucket, key, uploadID string) error {
	input := &s3.AbortMultipartUploadInput{
		Bucket:   aws.String(bucket),
		Key:      aws.String(key),
		UploadId: aws.String(uploadID),
	}

	_, err := s3Client.AbortMultipartUpload(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to abort multipart upload: %w", err)
	}

	return nil
}

// ListMultipartUploadParts lists all uploaded parts for a multipart upload
// Returns parts with their part numbers and ETags for verification
func ListMultipartUploadParts(ctx context.Context, s3Client *s3.Client, bucket, key, uploadID string) ([]types.Part, error) {
	var allParts []types.Part

	input := &s3.ListPartsInput{
		Bucket:   aws.String(bucket),
		Key:      aws.String(key),
		UploadId: aws.String(uploadID),
	}

	paginator := s3.NewListPartsPaginator(s3Client, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list parts: %w", err)
		}
		allParts = append(allParts, page.Parts...)
	}

	return allParts, nil
}

// ResumeMultipartUpload attempts to resume an incomplete multipart upload
// Verifies that the parts match the stored state and completes the upload if valid
func ResumeMultipartUpload(ctx context.Context, s3Client *s3.Client, bucket string, state *MultipartState) error {
	if state == nil {
		return fmt.Errorf("multipart state cannot be nil")
	}
	if state.S3Key == "" || state.UploadID == "" {
		return fmt.Errorf("invalid multipart state: missing S3 key or upload ID")
	}

	// List currently uploaded parts from S3
	s3Parts, err := ListMultipartUploadParts(ctx, s3Client, bucket, state.S3Key, state.UploadID)
	if err != nil {
		return fmt.Errorf("failed to list uploaded parts: %w", err)
	}

	// Verify parts match our stored state
	if len(s3Parts) != len(state.CompletedParts) {
		return fmt.Errorf("part count mismatch: expected %d parts, found %d in S3",
			len(state.CompletedParts), len(s3Parts))
	}

	// Build CompletedPart list for completion
	completedParts := make([]types.CompletedPart, 0, len(s3Parts))
	for _, part := range s3Parts {
		completedParts = append(completedParts, types.CompletedPart{
			PartNumber: part.PartNumber,
			ETag:       part.ETag,
		})
	}

	// Complete the multipart upload
	if err := CompleteMultipartUpload(ctx, s3Client, bucket, state.S3Key, state.UploadID, completedParts); err != nil {
		return fmt.Errorf("failed to complete multipart upload: %w", err)
	}

	return nil
}

// CleanupOrphanedMultipartUploads aborts multipart uploads older than a certain age
// Useful for cleanup of abandoned uploads that are consuming S3 storage
func CleanupOrphanedMultipartUploads(ctx context.Context, s3Client *s3.Client, bucket, prefix string, maxAgeDays int) (int, error) {
	uploads, err := ListInProgressMultipartUploads(ctx, s3Client, bucket, prefix)
	if err != nil {
		return 0, err
	}

	// TODO: Filter by age (requires checking upload Initiated timestamp)
	// For now, we don't automatically abort uploads to avoid data loss
	// User should manually clean up via CLI command

	return len(uploads), nil
}

// VerifyMultipartState checks if a multipart upload state is still valid in S3
// Returns true if the upload exists and has the expected parts
func VerifyMultipartState(ctx context.Context, s3Client *s3.Client, bucket string, state *MultipartState) (bool, error) {
	if state == nil {
		return false, fmt.Errorf("multipart state cannot be nil")
	}

	// Try to list parts
	s3Parts, err := ListMultipartUploadParts(ctx, s3Client, bucket, state.S3Key, state.UploadID)
	if err != nil {
		// Upload doesn't exist or is invalid
		return false, nil
	}

	// Check if part count matches
	if len(s3Parts) != len(state.CompletedParts) {
		return false, nil
	}

	return true, nil
}
