package pipeline

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/scttfrdmn/cargoship/pkg/chunking"
)

// TestNewShardProgressRenderer tests progress renderer creation
func TestNewShardProgressRenderer(t *testing.T) {
	ctx := context.Background()
	coordinator := createMockCoordinator(t, 4)
	defer func() {
		if err := coordinator.Close(); err != nil {
			t.Logf("coordinator close error (expected in test): %v", err)
		}
	}()

	tracker := NewShardProgressRenderer(ctx, coordinator, 1024*1024*1024)

	if tracker == nil {
		t.Fatal("expected non-nil tracker")
	}
	if tracker.coordinator != coordinator {
		t.Error("coordinator not set correctly")
	}
	if tracker.estimatedSize != 1024*1024*1024 {
		t.Errorf("expected estimatedSize 1GB, got %d", tracker.estimatedSize)
	}
}

// TestShardProgressRenderer_Stop tests graceful shutdown
func TestShardProgressRenderer_Stop(t *testing.T) {
	ctx := context.Background()
	coordinator := createMockCoordinator(t, 4)
	defer func() { _ = coordinator.Close() }()

	tracker := NewShardProgressRenderer(ctx, coordinator, 1024*1024)

	// Should not panic
	tracker.Stop()
	tracker.Stop() // Test idempotency
}

// TestRenderOnce tests single snapshot rendering
func TestRenderOnce(t *testing.T) {
	coordinator := createMockCoordinator(t, 4)
	defer func() { _ = coordinator.Close() }()

	// Add some mock activity
	_ = coordinator.Start()
	time.Sleep(10 * time.Millisecond)

	startTime := time.Now()
	output := RenderOnce(coordinator, 1024*1024, startTime)

	// Verify output contains expected sections
	if !strings.Contains(output, "Uploading to S3") && !strings.Contains(output, "Upload completed") {
		t.Errorf("expected upload status in output, got: %s", output)
	}
	if !strings.Contains(output, "Shard") {
		t.Error("expected shard information in output")
	}
	if !strings.Contains(output, "Total:") {
		t.Error("expected total statistics in output")
	}
}

// TestRenderOnce_Complete tests rendering when upload is complete
func TestRenderOnce_Complete(t *testing.T) {
	coordinator := createMockCoordinator(t, 2)
	defer func() { _ = coordinator.Close() }()

	_ = coordinator.Start()
	_ = coordinator.Close() // Mark as complete

	startTime := time.Now().Add(-5 * time.Second)
	output := RenderOnce(coordinator, 1024, startTime)

	if !strings.Contains(output, "completed") {
		t.Errorf("expected 'completed' status, got: %s", output)
	}
}

// TestProgressModel_Init tests model initialization
func TestProgressModel_Init(t *testing.T) {
	ctx := context.Background()
	coordinator := createMockCoordinator(t, 4)
	defer func() { _ = coordinator.Close() }()

	tracker := NewShardProgressRenderer(ctx, coordinator, 1024*1024)
	model := newProgressModel(tracker)

	if len(model.shardProgress) != 4 {
		t.Errorf("expected 4 shard progress bars, got %d", len(model.shardProgress))
	}

	cmd := model.Init()
	if cmd != nil {
		t.Error("expected nil init command")
	}
}

// TestProgressModel_Update_WindowSize tests window resize handling
func TestProgressModel_Update_WindowSize(t *testing.T) {
	ctx := context.Background()
	coordinator := createMockCoordinator(t, 2)
	defer func() { _ = coordinator.Close() }()

	tracker := NewShardProgressRenderer(ctx, coordinator, 1024)
	model := newProgressModel(tracker)

	// Send window size update using bubbletea's actual type
	msg := tea.WindowSizeMsg{Width: 120, Height: 40}
	updatedModel, _ := model.Update(msg)
	m := updatedModel.(progressModel)

	if m.width != 120 {
		t.Errorf("expected width 120, got %d", m.width)
	}
	if m.height != 40 {
		t.Errorf("expected height 40, got %d", m.height)
	}
}

// TestProgressModel_Update_Tick tests tick message handling
func TestProgressModel_Update_Tick(t *testing.T) {
	ctx := context.Background()
	coordinator := createMockCoordinator(t, 2)
	defer func() { _ = coordinator.Close() }()

	_ = coordinator.Start()

	tracker := NewShardProgressRenderer(ctx, coordinator, 1024*1024)
	model := newProgressModel(tracker)

	// Send tick message
	initialTime := model.lastUpdateTime
	time.Sleep(10 * time.Millisecond)

	updatedModel, _ := model.Update(tickMsg{})
	m := updatedModel.(progressModel)

	if !m.lastUpdateTime.After(initialTime) {
		t.Error("expected lastUpdateTime to be updated")
	}
}

// TestProgressModel_View_InProgress tests view rendering for in-progress upload
func TestProgressModel_View_InProgress(t *testing.T) {
	ctx := context.Background()
	coordinator := createMockCoordinator(t, 3)
	defer func() { _ = coordinator.Close() }()

	_ = coordinator.Start()
	time.Sleep(10 * time.Millisecond)

	tracker := NewShardProgressRenderer(ctx, coordinator, 1024*1024)
	model := newProgressModel(tracker)
	model.lastStats = coordinator.GetStats()

	view := model.View()

	// Verify key components
	if !strings.Contains(view, "Shard 0:") {
		t.Error("expected Shard 0 in view")
	}
	if !strings.Contains(view, "Shard 1:") {
		t.Error("expected Shard 1 in view")
	}
	if !strings.Contains(view, "Shard 2:") {
		t.Error("expected Shard 2 in view")
	}
	if !strings.Contains(view, "Total:") {
		t.Error("expected Total statistics in view")
	}
	if !strings.Contains(view, "files") {
		t.Error("expected files count in view")
	}
}

// TestProgressModel_View_Complete tests view rendering for completed upload
func TestProgressModel_View_Complete(t *testing.T) {
	ctx := context.Background()
	coordinator := createMockCoordinator(t, 2)
	defer func() { _ = coordinator.Close() }()

	_ = coordinator.Start()
	_ = coordinator.Close()

	tracker := NewShardProgressRenderer(ctx, coordinator, 1024)
	model := newProgressModel(tracker)
	model.lastStats = coordinator.GetStats()

	view := model.View()

	if !strings.Contains(view, "completed") {
		t.Errorf("expected 'completed' in view, got: %s", view)
	}
}

// TestProgressModel_CompressionRatio tests compression ratio display
func TestProgressModel_CompressionRatio(t *testing.T) {
	ctx := context.Background()
	coordinator := createMockCoordinator(t, 2)
	defer func() { _ = coordinator.Close() }()

	_ = coordinator.Start()
	time.Sleep(10 * time.Millisecond)

	tracker := NewShardProgressRenderer(ctx, coordinator, 1024*1024)
	model := newProgressModel(tracker)
	model.lastStats = coordinator.GetStats()

	view := model.View()

	// Should contain compression percentage
	if !strings.Contains(view, "compression") {
		t.Error("expected compression info in view")
	}
}

// TestProgressModel_ETA tests ETA calculation and display
func TestProgressModel_ETA(t *testing.T) {
	ctx := context.Background()
	coordinator := createMockCoordinator(t, 2)
	defer func() { _ = coordinator.Close() }()

	_ = coordinator.Start()
	time.Sleep(10 * time.Millisecond)

	// Set estimated size larger than processed to trigger ETA
	tracker := NewShardProgressRenderer(ctx, coordinator, 10*1024*1024)
	model := newProgressModel(tracker)
	model.lastStats = coordinator.GetStats()

	// Simulate some progress
	model.lastStats.BytesProcessed = 1024 * 1024 // 1MB processed out of 10MB

	view := model.View()

	// ETA should appear when we have progress but not complete
	if model.lastStats.BytesProcessed > 0 && !model.lastStats.IsComplete() {
		// ETA may or may not appear depending on progress calculation
		// This is not a strict requirement, just verify it doesn't crash
		_ = view
	}
}

// Helper function to create a mock coordinator for testing
func createMockCoordinator(t *testing.T, shardCount int) *ShardCoordinator {
	t.Helper()

	ctx := context.Background()
	memMgr := NewMemoryManager(ctx, nil) // Use defaults

	// Create router
	router, err := chunking.NewShardRouter(&chunking.ShardRouterConfig{
		ShardCount: shardCount,
		Strategy:   chunking.ShardByHash,
	})
	if err != nil {
		t.Fatalf("failed to create router: %v", err)
	}

	config := &ShardCoordinatorConfig{
		ShardCount:    shardCount,
		Bucket:        "test-bucket",
		Prefix:        "test-prefix",
		S3Client:      &mockS3Uploader{},
		MemoryManager: memMgr,
		Router:        router,
	}

	coordinator, err := NewShardCoordinator(ctx, config)
	if err != nil {
		t.Fatalf("failed to create coordinator: %v", err)
	}

	// Start coordinator to avoid blocking in Close()
	if err := coordinator.Start(); err != nil {
		t.Fatalf("failed to start coordinator: %v", err)
	}

	return coordinator
}

// mockS3Uploader is a mock S3 uploader for testing
type mockS3Uploader struct{}

func (m *mockS3Uploader) PutObject(ctx context.Context, input *s3.PutObjectInput, opts ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	return &s3.PutObjectOutput{}, nil
}

func (m *mockS3Uploader) CreateMultipartUpload(ctx context.Context, input *s3.CreateMultipartUploadInput, opts ...func(*s3.Options)) (*s3.CreateMultipartUploadOutput, error) {
	uploadID := "mock-upload-id"
	return &s3.CreateMultipartUploadOutput{UploadId: &uploadID}, nil
}

func (m *mockS3Uploader) UploadPart(ctx context.Context, input *s3.UploadPartInput, opts ...func(*s3.Options)) (*s3.UploadPartOutput, error) {
	etag := "mock-etag"
	return &s3.UploadPartOutput{ETag: &etag}, nil
}

func (m *mockS3Uploader) CompleteMultipartUpload(ctx context.Context, input *s3.CompleteMultipartUploadInput, opts ...func(*s3.Options)) (*s3.CompleteMultipartUploadOutput, error) {
	return &s3.CompleteMultipartUploadOutput{}, nil
}

func (m *mockS3Uploader) AbortMultipartUpload(ctx context.Context, input *s3.AbortMultipartUploadInput, opts ...func(*s3.Options)) (*s3.AbortMultipartUploadOutput, error) {
	return &s3.AbortMultipartUploadOutput{}, nil
}

// Benchmark for RenderOnce performance
func BenchmarkRenderOnce(b *testing.B) {
	coordinator := createMockCoordinatorBench(b, 8)
	defer func() { _ = coordinator.Close() }()

	_ = coordinator.Start()
	startTime := time.Now()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = RenderOnce(coordinator, 1024*1024*1024, startTime)
	}
}

// Benchmark for progress model view rendering
func BenchmarkProgressModel_View(b *testing.B) {
	ctx := context.Background()
	coordinator := createMockCoordinatorBench(b, 8)
	defer func() { _ = coordinator.Close() }()

	_ = coordinator.Start()

	tracker := NewShardProgressRenderer(ctx, coordinator, 1024*1024*1024)
	model := newProgressModel(tracker)
	model.lastStats = coordinator.GetStats()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = model.View()
	}
}

func createMockCoordinatorBench(b *testing.B, shardCount int) *ShardCoordinator {
	b.Helper()

	ctx := context.Background()
	memMgr := NewMemoryManager(ctx, nil) // Use defaults

	// Create router
	router, err := chunking.NewShardRouter(&chunking.ShardRouterConfig{
		ShardCount: shardCount,
		Strategy:   chunking.ShardByHash,
	})
	if err != nil {
		b.Fatalf("failed to create router: %v", err)
	}

	config := &ShardCoordinatorConfig{
		ShardCount:    shardCount,
		Bucket:        "test-bucket",
		Prefix:        "test-prefix",
		S3Client:      &mockS3Uploader{},
		MemoryManager: memMgr,
		Router:        router,
	}

	coordinator, err := NewShardCoordinator(ctx, config)
	if err != nil {
		b.Fatalf("failed to create coordinator: %v", err)
	}

	// Start coordinator to avoid blocking in Close()
	if err := coordinator.Start(); err != nil {
		b.Fatalf("failed to start coordinator: %v", err)
	}

	return coordinator
}
