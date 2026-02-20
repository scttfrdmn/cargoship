package manifest

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// copyFixtures copies testdata/dvc.yaml and testdata/dvc.lock into dir.
func copyFixtures(t *testing.T, dir string) {
	t.Helper()
	for _, name := range []string{"dvc.yaml", "dvc.lock"} {
		src := filepath.Join("testdata", name)
		dst := filepath.Join(dir, name)
		data, err := os.ReadFile(src)
		if err != nil {
			t.Fatalf("read fixture %s: %v", src, err)
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			t.Fatalf("write fixture %s: %v", dst, err)
		}
	}
}

// ---------------------------------------------------------------------------
// TestExtractDVCPipeline — happy paths (testdata fixtures)
// ---------------------------------------------------------------------------

func TestExtractDVCPipelinePreprocessStage(t *testing.T) {
	dir := t.TempDir()
	copyFixtures(t, dir)

	got, err := ExtractDVCPipeline(dir, "preprocess")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.StageName != "preprocess" {
		t.Errorf("StageName = %q, want %q", got.StageName, "preprocess")
	}
	if got.PipelineFile != "dvc.yaml" {
		t.Errorf("PipelineFile = %q, want %q", got.PipelineFile, "dvc.yaml")
	}
	wantCmd := "python scripts/preprocess.py --input data/raw --output data/processed"
	if got.Command != wantCmd {
		t.Errorf("Command = %q, want %q", got.Command, wantCmd)
	}
}

func TestExtractDVCPipelineDepsCount(t *testing.T) {
	dir := t.TempDir()
	copyFixtures(t, dir)

	got, err := ExtractDVCPipeline(dir, "preprocess")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(got.Deps) != 2 {
		t.Errorf("len(Deps) = %d, want 2", len(got.Deps))
	}
}

func TestExtractDVCPipelineDepsHaveMD5FromLock(t *testing.T) {
	dir := t.TempDir()
	copyFixtures(t, dir)

	got, err := ExtractDVCPipeline(dir, "preprocess")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, dep := range got.Deps {
		if dep.MD5 == "" {
			t.Errorf("dep %q: MD5 is empty, expected value from dvc.lock", dep.Path)
		}
		if dep.Size == 0 {
			t.Errorf("dep %q: Size is 0, expected value from dvc.lock", dep.Path)
		}
	}
}

func TestExtractDVCPipelineOutputsCount(t *testing.T) {
	dir := t.TempDir()
	copyFixtures(t, dir)

	got, err := ExtractDVCPipeline(dir, "preprocess")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(got.Outputs) != 1 {
		t.Errorf("len(Outputs) = %d, want 1", len(got.Outputs))
	}
	if len(got.Outputs) > 0 && got.Outputs[0].Path != "data/processed" {
		t.Errorf("Outputs[0].Path = %q, want %q", got.Outputs[0].Path, "data/processed")
	}
}

func TestExtractDVCPipelineParams(t *testing.T) {
	dir := t.TempDir()
	copyFixtures(t, dir)

	got, err := ExtractDVCPipeline(dir, "preprocess")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(got.Params) == 0 {
		t.Fatal("Params is empty, expected values from dvc.lock")
	}
	if _, ok := got.Params["learning_rate"]; !ok {
		t.Error("Params missing learning_rate")
	}
	if _, ok := got.Params["batch_size"]; !ok {
		t.Error("Params missing batch_size")
	}
}

func TestExtractDVCPipelineLockHash(t *testing.T) {
	dir := t.TempDir()
	copyFixtures(t, dir)

	got, err := ExtractDVCPipeline(dir, "preprocess")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.LockHash == "" {
		t.Error("LockHash is empty")
	}
	if len(got.LockHash) != 32 {
		t.Errorf("LockHash len = %d, want 32 (MD5 hex)", len(got.LockHash))
	}
}

func TestExtractDVCPipelineExecutedAt(t *testing.T) {
	dir := t.TempDir()
	copyFixtures(t, dir)

	before := time.Now().UTC().Add(-time.Second)
	got, err := ExtractDVCPipeline(dir, "preprocess")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.ExecutedAt.IsZero() {
		t.Error("ExecutedAt is zero")
	}
	if got.ExecutedAt.Before(before.Add(-24 * time.Hour)) {
		t.Errorf("ExecutedAt %v seems too old", got.ExecutedAt)
	}
}

func TestExtractDVCPipelineTrainStage(t *testing.T) {
	dir := t.TempDir()
	copyFixtures(t, dir)

	got, err := ExtractDVCPipeline(dir, "train")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.StageName != "train" {
		t.Errorf("StageName = %q, want %q", got.StageName, "train")
	}
	// train stage has 2 outputs (model.pkl + metrics/train_metrics.json)
	if len(got.Outputs) != 2 {
		t.Errorf("len(Outputs) = %d, want 2", len(got.Outputs))
	}
}

func TestExtractDVCPipelineLockHashStable(t *testing.T) {
	dir := t.TempDir()
	copyFixtures(t, dir)

	first, err := ExtractDVCPipeline(dir, "preprocess")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	second, err := ExtractDVCPipeline(dir, "train")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Both stages use the same dvc.lock so the hash must be identical.
	if first.LockHash != second.LockHash {
		t.Errorf("LockHash differs across stages: %q vs %q", first.LockHash, second.LockHash)
	}
}

// ---------------------------------------------------------------------------
// Graceful fallbacks
// ---------------------------------------------------------------------------

func TestExtractDVCPipelineNoDVCYAML(t *testing.T) {
	dir := t.TempDir()
	// No dvc.yaml present — should return zero-value, not error.
	got, err := ExtractDVCPipeline(dir, "preprocess")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.StageName != "preprocess" {
		t.Errorf("StageName = %q, want %q", got.StageName, "preprocess")
	}
	if got.Command != "" {
		t.Errorf("Command = %q, want empty", got.Command)
	}
}

func TestExtractDVCPipelineNoDVCLock(t *testing.T) {
	dir := t.TempDir()
	// Only dvc.yaml — no dvc.lock yet (stage hasn't been run).
	src := filepath.Join("testdata", "dvc.yaml")
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "dvc.yaml"), data, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	got, err := ExtractDVCPipeline(dir, "preprocess")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Command should be populated from dvc.yaml.
	if got.Command == "" {
		t.Error("Command is empty, expected value from dvc.yaml")
	}
	// LockHash should be empty since there's no dvc.lock.
	if got.LockHash != "" {
		t.Errorf("LockHash = %q, want empty (no dvc.lock)", got.LockHash)
	}
	// MD5 on deps should be empty.
	for _, dep := range got.Deps {
		if dep.MD5 != "" {
			t.Errorf("dep %q has MD5 %q without dvc.lock", dep.Path, dep.MD5)
		}
	}
}

func TestExtractDVCPipelineUnknownStage(t *testing.T) {
	dir := t.TempDir()
	copyFixtures(t, dir)

	got, err := ExtractDVCPipeline(dir, "nonexistent_stage")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Command != "" {
		t.Errorf("Command = %q for unknown stage, want empty", got.Command)
	}
}

func TestExtractDVCPipelineNonexistentDir(t *testing.T) {
	got, err := ExtractDVCPipeline("/nonexistent/path/xyz", "preprocess")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("got nil result")
	}
	if got.StageName != "preprocess" {
		t.Errorf("StageName = %q, want %q", got.StageName, "preprocess")
	}
}

func TestExtractDVCPipelinePathTraversal(t *testing.T) {
	// Path traversal should not cause a panic or security issue —
	// the result should be a graceful fallback.
	got, err := ExtractDVCPipeline("../../../../../../etc", "passwd")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// No DVC files in /etc, so should fall back to zero-value.
	if got.Command != "" {
		t.Errorf("Command = %q for path-traversal input, want empty", got.Command)
	}
}

func TestExtractDVCPipelineEmptyLock(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join("testdata", "dvc.yaml")
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "dvc.yaml"), data, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	// Write an empty dvc.lock.
	if err := os.WriteFile(filepath.Join(dir, "dvc.lock"), []byte("schema: '2.0'\nstages: {}\n"), 0o644); err != nil {
		t.Fatalf("write empty lock: %v", err)
	}

	got, err := ExtractDVCPipeline(dir, "preprocess")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Command still populated from dvc.yaml.
	if got.Command == "" {
		t.Error("Command is empty, expected value from dvc.yaml")
	}
	// LockHash should exist (file present even if empty-stages).
	if got.LockHash == "" {
		t.Error("LockHash is empty for present dvc.lock")
	}
}

// ---------------------------------------------------------------------------
// DVCPipeline struct round-trip (ensure types.go fields match)
// ---------------------------------------------------------------------------

func TestDVCPipelineZeroValue(t *testing.T) {
	p := &DVCPipeline{}
	if p.StageName != "" {
		t.Error("zero-value StageName should be empty")
	}
	if len(p.Deps) != 0 {
		t.Error("zero-value Deps should be nil/empty")
	}
}

func TestDVCDepAndOutFields(t *testing.T) {
	dep := DVCDep{Path: "data/raw", MD5: "abc123", Size: 1024}
	if dep.Path != "data/raw" {
		t.Errorf("Path = %q", dep.Path)
	}
	out := DVCOut{Path: "data/processed", MD5: "def456", Size: 512}
	if out.Path != "data/processed" {
		t.Errorf("Path = %q", out.Path)
	}
}

// ---------------------------------------------------------------------------
// BuildFileStageIndex
// ---------------------------------------------------------------------------

func TestBuildFileStageIndex_AllStages(t *testing.T) {
	dir := t.TempDir()
	// Only dvc.yaml needed.
	src := filepath.Join("testdata", "dvc.yaml")
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "dvc.yaml"), data, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	index, err := BuildFileStageIndex(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// preprocess has 1 output: data/processed (directory → stored with trailing "/")
	if stage, ok := index["data/processed/"]; !ok || stage != "preprocess" {
		t.Errorf("index[data/processed/] = %q, want preprocess", stage)
	}
	// train has 2 outputs: models/model.pkl and metrics/train_metrics.json
	if stage, ok := index["models/model.pkl"]; !ok || stage != "train" {
		t.Errorf("index[models/model.pkl] = %q, want train", stage)
	}
	if stage, ok := index["metrics/train_metrics.json"]; !ok || stage != "train" {
		t.Errorf("index[metrics/train_metrics.json] = %q, want train", stage)
	}
}

func TestBuildFileStageIndex_NoDVCYAML(t *testing.T) {
	dir := t.TempDir()
	// No dvc.yaml present — should return empty map, not error.
	index, err := BuildFileStageIndex(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(index) != 0 {
		t.Errorf("expected empty index, got %v", index)
	}
}

// ---------------------------------------------------------------------------
// AnnotateFilesWithDVCStages
// ---------------------------------------------------------------------------

func TestAnnotateFilesWithDVCStages_Basic(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join("testdata", "dvc.yaml")
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "dvc.yaml"), data, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	files := []FileEntry{
		{Path: "data/processed/features.csv"},
		{Path: "models/model.pkl"},
	}
	AnnotateFilesWithDVCStages(files, dir)

	if files[0].DVCMetadata == nil || files[0].DVCMetadata.Stage != "preprocess" {
		t.Errorf("files[0] Stage = %v, want preprocess", files[0].DVCMetadata)
	}
	if files[1].DVCMetadata == nil || files[1].DVCMetadata.Stage != "train" {
		t.Errorf("files[1] Stage = %v, want train", files[1].DVCMetadata)
	}
}

func TestAnnotateFilesWithDVCStages_DirectoryOutput(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join("testdata", "dvc.yaml")
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "dvc.yaml"), data, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	// All three paths are under data/processed/, which is a directory output of preprocess.
	files := []FileEntry{
		{Path: "data/processed/a.csv"},
		{Path: "data/processed/sub/b.csv"},
		{Path: "data/processed"}, // exact directory path itself
	}
	AnnotateFilesWithDVCStages(files, dir)

	for i, f := range files {
		if f.DVCMetadata == nil || f.DVCMetadata.Stage != "preprocess" {
			t.Errorf("files[%d] (path=%s) Stage = %v, want preprocess", i, f.Path, f.DVCMetadata)
		}
	}
}

func TestAnnotateFilesWithDVCStages_NoMatch(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join("testdata", "dvc.yaml")
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "dvc.yaml"), data, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	files := []FileEntry{
		{Path: "src/unrelated.go"},
	}
	AnnotateFilesWithDVCStages(files, dir)

	if files[0].DVCMetadata != nil {
		t.Errorf("expected no DVCMetadata, got %v", files[0].DVCMetadata)
	}
}

func TestAnnotateFilesWithDVCStages_NoDVCYAML(t *testing.T) {
	dir := t.TempDir()
	// No dvc.yaml — should be a no-op.
	files := []FileEntry{
		{Path: "data/processed/x.csv"},
	}
	AnnotateFilesWithDVCStages(files, dir)

	if files[0].DVCMetadata != nil {
		t.Errorf("expected no DVCMetadata when dvc.yaml absent, got %v", files[0].DVCMetadata)
	}
}
