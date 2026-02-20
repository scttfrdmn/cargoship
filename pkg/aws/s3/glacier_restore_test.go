package s3

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockGlacierClient implements GlacierS3Client for testing.
type mockGlacierClient struct {
	heads    map[string]*s3.HeadObjectOutput
	headErr  error
	restored map[string]bool
}

func (m *mockGlacierClient) HeadObject(_ context.Context, input *s3.HeadObjectInput, _ ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	if m.headErr != nil {
		return nil, m.headErr
	}
	out, ok := m.heads[*input.Key]
	if !ok {
		return &s3.HeadObjectOutput{}, nil
	}
	return out, nil
}

func (m *mockGlacierClient) RestoreObject(_ context.Context, input *s3.RestoreObjectInput, _ ...func(*s3.Options)) (*s3.RestoreObjectOutput, error) {
	if m.restored == nil {
		m.restored = make(map[string]bool)
	}
	m.restored[*input.Key] = true
	// Simulate the object transitioning to in-progress on next HeadObject.
	if out, ok := m.heads[*input.Key]; ok {
		out.Restore = aws.String(`ongoing-request="true"`)
	}
	return &s3.RestoreObjectOutput{}, nil
}

func TestCheckAndRestore_Accessible(t *testing.T) {
	client := &mockGlacierClient{
		heads: map[string]*s3.HeadObjectOutput{
			"chunk-0.tar.zst": {
				StorageClass:  s3types.StorageClassStandard,
				ContentLength: aws.Int64(1024 * 1024 * 100), // 100 MB
			},
		},
	}
	gr := NewGlacierRestorer(client, 0)
	report, err := gr.CheckAndRestore(context.Background(), "my-bucket", []string{"chunk-0.tar.zst"}, RestoreTierStandard)
	require.NoError(t, err)
	assert.True(t, report.AllAccessible())
	assert.Len(t, report.Accessible, 1)
	assert.Empty(t, report.Frozen)
	assert.Empty(t, report.JustRequested)
	assert.Equal(t, float64(0), report.EstimatedCostUSD)
}

func TestCheckAndRestore_FrozenGlacier(t *testing.T) {
	sizeBytes := int64(1024 * 1024 * 1024) // 1 GB
	client := &mockGlacierClient{
		heads: map[string]*s3.HeadObjectOutput{
			"chunk-1.tar.zst": {
				StorageClass:  s3types.StorageClassGlacier,
				ContentLength: aws.Int64(sizeBytes),
				Restore:       nil, // not yet requested
			},
		},
	}
	gr := NewGlacierRestorer(client, 7)
	report, err := gr.CheckAndRestore(context.Background(), "my-bucket", []string{"chunk-1.tar.zst"}, RestoreTierStandard)
	require.NoError(t, err)
	assert.False(t, report.AllAccessible())
	assert.Len(t, report.Frozen, 1)
	assert.Len(t, report.JustRequested, 1)
	assert.True(t, client.restored["chunk-1.tar.zst"])
	// 1 GB at Standard Glacier: $0.010/GB
	assert.InDelta(t, 0.010, report.EstimatedCostUSD, 0.001)
}

func TestCheckAndRestore_InProgress(t *testing.T) {
	client := &mockGlacierClient{
		heads: map[string]*s3.HeadObjectOutput{
			"chunk-2.tar.zst": {
				StorageClass:  s3types.StorageClassGlacier,
				ContentLength: aws.Int64(512 * 1024 * 1024),
				Restore:       aws.String(`ongoing-request="true"`),
			},
		},
	}
	gr := NewGlacierRestorer(client, 0)
	report, err := gr.CheckAndRestore(context.Background(), "my-bucket", []string{"chunk-2.tar.zst"}, RestoreTierStandard)
	require.NoError(t, err)
	assert.False(t, report.AllAccessible())
	assert.Len(t, report.InProgress, 1)
	assert.Empty(t, report.JustRequested)
	assert.False(t, client.restored["chunk-2.tar.zst"]) // no new request
}

func TestCheckAndRestore_DeepArchive(t *testing.T) {
	sizeBytes := int64(2 * 1024 * 1024 * 1024) // 2 GB
	client := &mockGlacierClient{
		heads: map[string]*s3.HeadObjectOutput{
			"chunk-3.tar.zst": {
				StorageClass:  s3types.StorageClassDeepArchive,
				ContentLength: aws.Int64(sizeBytes),
			},
		},
	}
	gr := NewGlacierRestorer(client, 0)
	report, err := gr.CheckAndRestore(context.Background(), "my-bucket", []string{"chunk-3.tar.zst"}, RestoreTierBulk)
	require.NoError(t, err)
	assert.Len(t, report.JustRequested, 1)
	// 2 GB at Bulk Deep Archive: $0.0025/GB = $0.005
	assert.InDelta(t, 0.005, report.EstimatedCostUSD, 0.001)
}

func TestCheckAndRestore_RestoredCopy(t *testing.T) {
	// Object was restored and the temporary copy is still available.
	expiry := time.Now().Add(48 * time.Hour)
	client := &mockGlacierClient{
		heads: map[string]*s3.HeadObjectOutput{
			"chunk-4.tar.zst": {
				StorageClass: s3types.StorageClassGlacier,
				Restore:      aws.String(`ongoing-request="false", expiry-date="` + expiry.UTC().Format(time.RFC1123) + `"`),
			},
		},
	}
	gr := NewGlacierRestorer(client, 0)
	report, err := gr.CheckAndRestore(context.Background(), "my-bucket", []string{"chunk-4.tar.zst"}, RestoreTierStandard)
	require.NoError(t, err)
	assert.True(t, report.AllAccessible())
	assert.Len(t, report.Accessible, 1)
	obj := report.Objects[0]
	require.NotNil(t, obj.ExpiresAt)
}

func TestCheckAndRestore_MixedStorageClasses(t *testing.T) {
	client := &mockGlacierClient{
		heads: map[string]*s3.HeadObjectOutput{
			"hot.tar.zst": {
				StorageClass:  s3types.StorageClassStandard,
				ContentLength: aws.Int64(100 * 1024 * 1024),
			},
			"cold.tar.zst": {
				StorageClass:  s3types.StorageClassGlacier,
				ContentLength: aws.Int64(100 * 1024 * 1024),
			},
		},
	}
	gr := NewGlacierRestorer(client, 0)
	report, err := gr.CheckAndRestore(context.Background(), "bucket", []string{"hot.tar.zst", "cold.tar.zst"}, RestoreTierExpedited)
	require.NoError(t, err)
	assert.Len(t, report.Accessible, 1)
	assert.Len(t, report.Frozen, 1)
	assert.Len(t, report.JustRequested, 1)
}

func TestWaitForRestore(t *testing.T) {
	callCount := 0
	// First poll: still in-progress; second poll: accessible.
	client := &mockGlacierClient{
		heads: map[string]*s3.HeadObjectOutput{
			"key": {
				StorageClass: s3types.StorageClassGlacier,
				Restore:      aws.String(`ongoing-request="true"`),
			},
		},
	}
	gr := NewGlacierRestorer(client, 0)

	// Transition to accessible after first poll.
	go func() {
		time.Sleep(15 * time.Millisecond)
		client.heads["key"].Restore = aws.String(`ongoing-request="false"`)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := gr.WaitForRestore(ctx, "bucket", []string{"key"}, 10*time.Millisecond, func(pending int) {
		callCount++
	})
	require.NoError(t, err)
	assert.Positive(t, callCount)
}

func TestParseRestoreExpiry(t *testing.T) {
	header := `ongoing-request="false", expiry-date="Fri, 21 Dec 2012 00:00:00 GMT"`
	got := parseRestoreExpiry(header)
	require.NotNil(t, got, "should parse expiry")
	assert.Equal(t, 2012, got.Year())
	assert.Equal(t, time.December, got.Month())
	assert.Equal(t, 21, got.Day())
}

func TestEstimateRetrievalCost(t *testing.T) {
	tests := []struct {
		sizeGB       float64
		storageClass string
		tier         RestoreTier
		wantCost     float64
	}{
		{1.0, string(s3types.StorageClassGlacier), RestoreTierExpedited, 0.030},
		{1.0, string(s3types.StorageClassGlacier), RestoreTierStandard, 0.010},
		{1.0, string(s3types.StorageClassGlacier), RestoreTierBulk, 0.0025},
		{1.0, string(s3types.StorageClassDeepArchive), RestoreTierBulk, 0.0025},
		{1.0, "STANDARD", RestoreTierStandard, 0.0}, // no retrieval fee
	}

	for _, tt := range tests {
		got := EstimateRetrievalCost(tt.sizeGB, tt.storageClass, tt.tier)
		assert.InDelta(t, tt.wantCost, got, 0.0001, "%s %s", tt.storageClass, tt.tier)
	}
}

func TestIsGlacierClass(t *testing.T) {
	assert.True(t, isGlacierClass(string(s3types.StorageClassGlacier)))
	assert.True(t, isGlacierClass(string(s3types.StorageClassDeepArchive)))
	assert.False(t, isGlacierClass(string(s3types.StorageClassStandard)))
	assert.False(t, isGlacierClass(string(s3types.StorageClassStandardIa)))
	assert.False(t, isGlacierClass(""))
}

func TestFormatAccessibilityReport(t *testing.T) {
	report := &AccessibilityReport{
		JustRequested:    []string{"chunk-0.tar.zst"},
		EstimatedCostUSD: 0.05,
		TotalSizeGB:      5.0,
		Objects: []GlacierObjectInfo{
			{StorageClass: string(s3types.StorageClassGlacier)},
		},
	}
	out := FormatAccessibilityReport(report, RestoreTierStandard)
	assert.Contains(t, out, "Restore requested")
	assert.Contains(t, out, "3–5 hours")
	assert.Contains(t, out, "$0.0500")
}
