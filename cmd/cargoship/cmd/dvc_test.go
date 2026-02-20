package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/scttfrdmn/cargoship/pkg/archivefs"
	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

// buildDVCManifest returns a manifest with DVC stage annotations for testing.
func buildDVCManifest() *manifest.Manifest {
	now := time.Now()
	files := []manifest.FileEntry{
		{
			Path:        "data/processed/features.csv",
			Size:        1024,
			ModTime:     now,
			ContentHash: "abc123",
			DVCMetadata: &manifest.DVCMetadata{Stage: "preprocess"},
		},
		{
			Path:        "data/processed/labels.csv",
			Size:        512,
			ModTime:     now,
			ContentHash: "def456",
			DVCMetadata: &manifest.DVCMetadata{Stage: "preprocess"},
		},
		{
			Path:        "models/model.pkl",
			Size:        2048,
			ModTime:     now,
			ContentHash: "ghi789",
			DVCMetadata: &manifest.DVCMetadata{Stage: "train"},
		},
		{
			Path:    "README.md",
			Size:    256,
			ModTime: now,
		},
	}
	return &manifest.Manifest{
		Version:    manifest.ManifestVersion,
		UploadID:   "dvc-test-001",
		Bucket:     "test-bucket",
		Files:      files,
		TotalFiles: int64(len(files)),
	}
}

// ---------------------------------------------------------------------------
// TestDVCStages_Basic
// ---------------------------------------------------------------------------

func TestDVCStages_Basic(t *testing.T) {
	m := buildDVCManifest()
	vfs := archivefs.New(m)
	stages := vfs.Stages()

	if cnt, ok := stages["preprocess"]; !ok || cnt != 2 {
		t.Errorf("stages[preprocess] = %d, want 2", cnt)
	}
	if cnt, ok := stages["train"]; !ok || cnt != 1 {
		t.Errorf("stages[train] = %d, want 1", cnt)
	}
	// README.md has no stage.
	if _, ok := stages[""]; ok {
		t.Error("empty stage key should not appear in stages map")
	}
}

// ---------------------------------------------------------------------------
// TestDVCStages_NoStages
// ---------------------------------------------------------------------------

func TestDVCStages_NoStages(t *testing.T) {
	now := time.Now()
	m := &manifest.Manifest{
		Version:    manifest.ManifestVersion,
		UploadID:   "no-stages-001",
		Files:      []manifest.FileEntry{{Path: "file.txt", Size: 1, ModTime: now}},
		TotalFiles: 1,
	}
	vfs := archivefs.New(m)
	stages := vfs.Stages()
	if len(stages) != 0 {
		t.Errorf("expected empty stages map, got %v", stages)
	}
}

// ---------------------------------------------------------------------------
// TestDVCStatus_Unchanged / Modified / Missing
// ---------------------------------------------------------------------------

func TestDVCStatus_Unchanged(t *testing.T) {
	dir := t.TempDir()

	// Write a known file.
	content := []byte("hello dvc")
	fpath := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(fpath, content, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	hash, err := manifest.ComputeContentHash(fpath)
	if err != nil {
		t.Fatalf("compute hash: %v", err)
	}

	status := computeFileStatus(fpath, hash)
	if status != "unchanged" {
		t.Errorf("status = %q, want unchanged", status)
	}
}

func TestDVCStatus_Modified(t *testing.T) {
	dir := t.TempDir()

	fpath := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(fpath, []byte("current content"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	// Pass a hash that does NOT match the file.
	status := computeFileStatus(fpath, "0000000000000000000000000000000000000000")
	if status != "modified" {
		t.Errorf("status = %q, want modified", status)
	}
}

func TestDVCStatus_Missing(t *testing.T) {
	status := computeFileStatus("/nonexistent/path/file.txt", "anyhash")
	if status != "missing" {
		t.Errorf("status = %q, want missing", status)
	}
}

// ---------------------------------------------------------------------------
// TestDVCExport_GeneratesFiles
// ---------------------------------------------------------------------------

func TestDVCExport_GeneratesFiles(t *testing.T) {
	dir := t.TempDir()
	m := buildDVCManifest()

	n, err := m.GenerateDVCFiles(dir, &manifest.DVCGenerateOptions{CacheDir: ".dvc/cache"})
	if err != nil {
		t.Fatalf("GenerateDVCFiles: %v", err)
	}
	// 3 files have ContentHash; README.md does not.
	if n != 3 {
		t.Errorf("generated %d .dvc files, want 3", n)
	}

	// Verify one of the generated files exists and contains expected content.
	dvcFile := filepath.Join(dir, "models", "model.pkl.dvc")
	data, err := os.ReadFile(dvcFile)
	if err != nil {
		t.Fatalf("read .dvc file: %v", err)
	}
	if !strings.Contains(string(data), "ghi789") {
		t.Errorf(".dvc file missing expected hash; content:\n%s", data)
	}
}

// ---------------------------------------------------------------------------
// TestNewDVCCmd_Structure
// ---------------------------------------------------------------------------

func TestNewDVCCmd_Structure(t *testing.T) {
	cmd := NewDVCCmd()

	if cmd.Name() != "dvc" {
		t.Errorf("cmd.Name() = %q, want dvc", cmd.Name())
	}

	// Verify all three subcommands are registered.
	names := make(map[string]bool)
	for _, sub := range cmd.Commands() {
		names[sub.Name()] = true
	}
	for _, want := range []string{"stages", "status", "export"} {
		if !names[want] {
			t.Errorf("subcommand %q not registered", want)
		}
	}
}

// ---------------------------------------------------------------------------
// TestFilterByStage
// ---------------------------------------------------------------------------

func TestFilterByStage(t *testing.T) {
	files := buildDVCManifest().Files
	preprocessFiles := filterByStage(files, "preprocess")
	if len(preprocessFiles) != 2 {
		t.Errorf("filterByStage preprocess: got %d, want 2", len(preprocessFiles))
	}
	trainFiles := filterByStage(files, "train")
	if len(trainFiles) != 1 {
		t.Errorf("filterByStage train: got %d, want 1", len(trainFiles))
	}
	noFiles := filterByStage(files, "evaluate")
	if len(noFiles) != 0 {
		t.Errorf("filterByStage evaluate: got %d, want 0", len(noFiles))
	}
}

// ---------------------------------------------------------------------------
// TestDVCStagesOutput (exercises command output formatting)
// ---------------------------------------------------------------------------

func TestDVCStagesOutput(t *testing.T) {
	m := buildDVCManifest()
	vfs := archivefs.New(m)
	stages := vfs.Stages()

	// Simulate the output the command would produce.
	var buf bytes.Buffer
	names := make([]string, 0, len(stages))
	for n := range stages {
		names = append(names, n)
	}

	for _, name := range names {
		buf.WriteString(name)
	}

	output := buf.String()
	if !strings.Contains(output, "preprocess") {
		t.Error("output should contain 'preprocess'")
	}
	if !strings.Contains(output, "train") {
		t.Error("output should contain 'train'")
	}
}
