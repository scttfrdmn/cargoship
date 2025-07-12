package launch

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FileWatcher monitors directories for files that need to be archived
type FileWatcher struct {
	watchPaths  []WatchPath
	logger      *slog.Logger
	
	// Detection patterns
	detectors   []DatasetDetector
	
	// State tracking
	lastScan    time.Time
	candidates  map[string]*ArchiveCandidate
}

// ArchiveCandidate represents a file or directory that may need archiving
type ArchiveCandidate struct {
	Path            string            `json:"path"`
	Type            CandidateType     `json:"type"`
	Size            int64             `json:"size"`
	ModTime         time.Time         `json:"mod_time"`
	DetectedBy      string            `json:"detected_by"`
	Confidence      float64           `json:"confidence"`
	Metadata        map[string]string `json:"metadata"`
	StorageClass    string            `json:"storage_class"`
	Priority        int               `json:"priority"`
	ReadyForArchive bool              `json:"ready_for_archive"`
	Reason          string            `json:"reason,omitempty"`
}

// CandidateType represents the type of archive candidate
type CandidateType string

const (
	CandidateTypeFile      CandidateType = "file"
	CandidateTypeDirectory CandidateType = "directory"
	CandidateTypeDataset   CandidateType = "dataset"
)

// DatasetDetector interface for different types of research data detection
type DatasetDetector interface {
	Name() string
	Detect(path string, info os.FileInfo) (*ArchiveCandidate, error)
	GetConfidence(candidate *ArchiveCandidate) float64
}

// NewFileWatcher creates a new file watcher
func NewFileWatcher(watchPaths []WatchPath, logger *slog.Logger) (*FileWatcher, error) {
	fw := &FileWatcher{
		watchPaths: watchPaths,
		logger:     logger.With("component", "file-watcher"),
		candidates: make(map[string]*ArchiveCandidate),
	}
	
	// Initialize dataset detectors
	fw.detectors = []DatasetDetector{
		&GenomicsDetector{logger: logger},
		&ImagingDetector{logger: logger},
		&ComputationalDetector{logger: logger},
		&GeneralDatasetDetector{logger: logger},
	}
	
	fw.logger.Info("File watcher initialized", "watch_paths", len(watchPaths), "detectors", len(fw.detectors))
	
	return fw, nil
}

// Scan performs a scan of all watched paths for archive candidates
func (fw *FileWatcher) Scan() ([]*ArchiveCandidate, error) {
	fw.logger.Info("Starting file system scan")
	fw.lastScan = time.Now()
	
	var allCandidates []*ArchiveCandidate
	
	for _, watchPath := range fw.watchPaths {
		fw.logger.Debug("Scanning watch path", "path", watchPath.Path)
		
		candidates, err := fw.scanPath(watchPath)
		if err != nil {
			fw.logger.Error("Failed to scan path", "path", watchPath.Path, "error", err)
			continue
		}
		
		allCandidates = append(allCandidates, candidates...)
	}
	
	fw.logger.Info("File system scan completed", 
		"candidates_found", len(allCandidates),
		"scan_duration", time.Since(fw.lastScan))
	
	return allCandidates, nil
}

// scanPath scans a specific watch path for archive candidates
func (fw *FileWatcher) scanPath(watchPath WatchPath) ([]*ArchiveCandidate, error) {
	var candidates []*ArchiveCandidate
	
	// Check if path exists
	if _, err := os.Stat(watchPath.Path); os.IsNotExist(err) {
		fw.logger.Warn("Watch path does not exist", "path", watchPath.Path)
		return candidates, nil
	}
	
	// Walk the directory tree
	err := filepath.Walk(watchPath.Path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			fw.logger.Warn("Error accessing path", "path", path, "error", err)
			return nil // Continue walking
		}
		
		// Skip if not recursive and in subdirectory
		if !watchPath.Recursive {
			relPath, _ := filepath.Rel(watchPath.Path, path)
			if strings.Contains(relPath, string(filepath.Separator)) {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}
		
		// Check file age requirements
		if time.Since(info.ModTime()) < watchPath.MinAge {
			return nil
		}
		
		// Apply include/exclude patterns
		if !fw.matchesPatterns(path, watchPath.IncludePatterns, watchPath.ExcludePatterns) {
			return nil
		}
		
		// Run detection algorithms
		for _, detector := range fw.detectors {
			candidate, err := detector.Detect(path, info)
			if err != nil {
				fw.logger.Debug("Detector error", "detector", detector.Name(), "path", path, "error", err)
				continue
			}
			
			if candidate != nil {
				// Set storage class from watch path if not specified
				if candidate.StorageClass == "" {
					candidate.StorageClass = watchPath.StorageClass
				}
				
				// Calculate confidence
				candidate.Confidence = detector.GetConfidence(candidate)
				
				// Determine if ready for archive
				candidate.ReadyForArchive = fw.isReadyForArchive(candidate, watchPath)
				
				candidates = append(candidates, candidate)
				fw.logger.Debug("Archive candidate detected",
					"path", candidate.Path,
					"type", candidate.Type,
					"detector", candidate.DetectedBy,
					"confidence", candidate.Confidence,
					"ready", candidate.ReadyForArchive)
			}
		}
		
		return nil
	})
	
	return candidates, err
}

// matchesPatterns checks if a path matches include/exclude patterns
func (fw *FileWatcher) matchesPatterns(path string, includePatterns, excludePatterns []string) bool {
	// Check exclude patterns first
	for _, pattern := range excludePatterns {
		if matched, _ := filepath.Match(pattern, filepath.Base(path)); matched {
			return false
		}
	}
	
	// If no include patterns, include by default
	if len(includePatterns) == 0 {
		return true
	}
	
	// Check include patterns
	for _, pattern := range includePatterns {
		if matched, _ := filepath.Match(pattern, filepath.Base(path)); matched {
			return true
		}
	}
	
	return false
}

// isReadyForArchive determines if a candidate is ready for archival
func (fw *FileWatcher) isReadyForArchive(candidate *ArchiveCandidate, watchPath WatchPath) bool {
	// Check minimum age requirement
	if time.Since(candidate.ModTime) < watchPath.MinAge {
		candidate.Reason = fmt.Sprintf("File too new (modified %v ago, minimum %v required)", 
			time.Since(candidate.ModTime), watchPath.MinAge)
		return false
	}
	
	// Check confidence threshold
	if candidate.Confidence < 0.7 {
		candidate.Reason = fmt.Sprintf("Low confidence score: %.2f", candidate.Confidence)
		return false
	}
	
	// Check for completion markers if it's a dataset
	if candidate.Type == CandidateTypeDataset {
		if !fw.hasCompletionMarkers(candidate.Path) {
			candidate.Reason = "Dataset completion markers not found"
			return false
		}
	}
	
	// Check if file is still being written to
	if fw.isBeingModified(candidate.Path) {
		candidate.Reason = "File is still being modified"
		return false
	}
	
	return true
}

// hasCompletionMarkers checks for dataset completion markers
func (fw *FileWatcher) hasCompletionMarkers(path string) bool {
	completionMarkers := []string{
		"ANALYSIS_COMPLETE",
		"processing_finished.flag",
		"analysis_complete.txt",
		"job_done.txt",
		"FINISHED",
		".complete",
	}
	
	dir := path
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		dir = filepath.Dir(path)
	}
	
	for _, marker := range completionMarkers {
		markerPath := filepath.Join(dir, marker)
		if _, err := os.Stat(markerPath); err == nil {
			return true
		}
	}
	
	return false
}

// isBeingModified checks if a file is currently being written to
func (fw *FileWatcher) isBeingModified(path string) bool {
	// Get current modification time
	info1, err := os.Stat(path)
	if err != nil {
		return false
	}
	
	// Wait a short time and check again
	time.Sleep(100 * time.Millisecond)
	
	info2, err := os.Stat(path)
	if err != nil {
		return false
	}
	
	// If modification time changed, file is being written to
	return !info1.ModTime().Equal(info2.ModTime())
}

// GenomicsDetector detects genomics research data
type GenomicsDetector struct {
	logger *slog.Logger
}

func (gd *GenomicsDetector) Name() string {
	return "genomics"
}

func (gd *GenomicsDetector) Detect(path string, info os.FileInfo) (*ArchiveCandidate, error) {
	filename := strings.ToLower(filepath.Base(path))
	
	// Genomics file extensions
	genomicsExtensions := []string{
		".fastq", ".fastq.gz", ".fq", ".fq.gz",
		".bam", ".sam", ".cram",
		".vcf", ".vcf.gz", ".bcf",
		".bed", ".gff", ".gtf",
		".fa", ".fasta", ".fna",
	}
	
	for _, ext := range genomicsExtensions {
		if strings.HasSuffix(filename, ext) {
			candidate := &ArchiveCandidate{
				Path:       path,
				Type:       CandidateTypeFile,
				Size:       info.Size(),
				ModTime:    info.ModTime(),
				DetectedBy: gd.Name(),
				Metadata: map[string]string{
					"data_type": "genomics",
					"file_type": strings.TrimPrefix(ext, "."),
				},
				StorageClass: "deep-archive", // Genomics data is rarely accessed
				Priority:     getGenomicsPriority(ext),
			}
			
			// Determine storage class based on file type
			if strings.Contains(ext, "vcf") || strings.Contains(ext, "bed") {
				candidate.StorageClass = "glacier" // Analysis results, may be accessed
			}
			
			return candidate, nil
		}
	}
	
	return nil, nil
}

func (gd *GenomicsDetector) GetConfidence(candidate *ArchiveCandidate) float64 {
	// High confidence for known genomics file types
	confidence := 0.9
	
	// Boost confidence for large files (likely real data)
	if candidate.Size > 100*1024*1024 { // > 100MB
		confidence += 0.05
	}
	
	// Check for genomics-related directory names
	dirName := strings.ToLower(filepath.Dir(candidate.Path))
	genomicsKeywords := []string{"sequencing", "genomics", "fastq", "alignment", "variants"}
	for _, keyword := range genomicsKeywords {
		if strings.Contains(dirName, keyword) {
			confidence += 0.03
			break
		}
	}
	
	if confidence > 1.0 {
		confidence = 1.0
	}
	
	return confidence
}

// ImagingDetector detects microscopy and imaging data
type ImagingDetector struct {
	logger *slog.Logger
}

func (id *ImagingDetector) Name() string {
	return "imaging"
}

func (id *ImagingDetector) Detect(path string, info os.FileInfo) (*ArchiveCandidate, error) {
	filename := strings.ToLower(filepath.Base(path))
	
	// Imaging file extensions
	imagingExtensions := []string{
		".tiff", ".tif", ".czi", ".lsm", ".nd2",
		".ome.tiff", ".ome.tif",
		".dv", ".stk", ".lif",
		".zvi", ".oib", ".oif",
	}
	
	for _, ext := range imagingExtensions {
		if strings.HasSuffix(filename, ext) {
			candidate := &ArchiveCandidate{
				Path:       path,
				Type:       CandidateTypeFile,
				Size:       info.Size(),
				ModTime:    info.ModTime(),
				DetectedBy: id.Name(),
				Metadata: map[string]string{
					"data_type": "imaging",
					"file_type": strings.TrimPrefix(ext, "."),
				},
				StorageClass: "glacier", // Imaging data may be accessed for analysis
				Priority:     getImagingPriority(ext),
			}
			
			return candidate, nil
		}
	}
	
	return nil, nil
}

func (id *ImagingDetector) GetConfidence(candidate *ArchiveCandidate) float64 {
	confidence := 0.85
	
	// Boost confidence for large files (high-resolution images)
	if candidate.Size > 50*1024*1024 { // > 50MB
		confidence += 0.1
	}
	
	// Check for imaging-related directory names
	dirName := strings.ToLower(filepath.Dir(candidate.Path))
	imagingKeywords := []string{"microscopy", "imaging", "confocal", "fluorescence", "brightfield"}
	for _, keyword := range imagingKeywords {
		if strings.Contains(dirName, keyword) {
			confidence += 0.05
			break
		}
	}
	
	if confidence > 1.0 {
		confidence = 1.0
	}
	
	return confidence
}

// ComputationalDetector detects computational research output
type ComputationalDetector struct {
	logger *slog.Logger
}

func (cd *ComputationalDetector) Name() string {
	return "computational"
}

func (cd *ComputationalDetector) Detect(path string, info os.FileInfo) (*ArchiveCandidate, error) {
	filename := strings.ToLower(filepath.Base(path))
	
	// Computational output extensions
	computationalExtensions := []string{
		".out", ".log", ".dat", ".csv", ".tsv",
		".h5", ".hdf5", ".nc", ".mat",
		".pkl", ".pickle", ".rds", ".rda",
		".json", ".xml", ".yaml", ".yml",
	}
	
	for _, ext := range computationalExtensions {
		if strings.HasSuffix(filename, ext) {
			candidate := &ArchiveCandidate{
				Path:       path,
				Type:       CandidateTypeFile,
				Size:       info.Size(),
				ModTime:    info.ModTime(),
				DetectedBy: cd.Name(),
				Metadata: map[string]string{
					"data_type": "computational",
					"file_type": strings.TrimPrefix(ext, "."),
				},
				StorageClass: "standard-ia", // May need occasional access
				Priority:     getComputationalPriority(ext),
			}
			
			// Adjust storage class based on file type
			if strings.Contains(ext, "log") || strings.Contains(ext, "out") {
				candidate.StorageClass = "deep-archive" // Logs rarely accessed
			}
			
			return candidate, nil
		}
	}
	
	return nil, nil
}

func (cd *ComputationalDetector) GetConfidence(candidate *ArchiveCandidate) float64 {
	confidence := 0.7
	
	// Check for computational directory names
	dirName := strings.ToLower(filepath.Dir(candidate.Path))
	computationalKeywords := []string{"analysis", "results", "output", "simulation", "computation"}
	for _, keyword := range computationalKeywords {
		if strings.Contains(dirName, keyword) {
			confidence += 0.1
			break
		}
	}
	
	if confidence > 1.0 {
		confidence = 1.0
	}
	
	return confidence
}

// GeneralDatasetDetector detects general research datasets
type GeneralDatasetDetector struct {
	logger *slog.Logger
}

func (gdd *GeneralDatasetDetector) Name() string {
	return "general"
}

func (gdd *GeneralDatasetDetector) Detect(path string, info os.FileInfo) (*ArchiveCandidate, error) {
	// Look for dataset directories
	if info.IsDir() {
		dirName := strings.ToLower(filepath.Base(path))
		
		// Dataset directory patterns
		datasetPatterns := []string{
			"data", "dataset", "experiment", "study",
			"analysis", "results", "output", "processed",
			"raw", "finished", "completed", "archive",
		}
		
		for _, pattern := range datasetPatterns {
			if strings.Contains(dirName, pattern) {
				candidate := &ArchiveCandidate{
					Path:       path,
					Type:       CandidateTypeDataset,
					Size:       gdd.calculateDirSize(path),
					ModTime:    info.ModTime(),
					DetectedBy: gdd.Name(),
					Metadata: map[string]string{
						"data_type": "dataset",
						"pattern":   pattern,
					},
					StorageClass: "glacier",
					Priority:     5,
				}
				
				return candidate, nil
			}
		}
	}
	
	return nil, nil
}

func (gdd *GeneralDatasetDetector) GetConfidence(candidate *ArchiveCandidate) float64 {
	return 0.6 // Lower confidence for general detection
}

func (gdd *GeneralDatasetDetector) calculateDirSize(path string) int64 {
	var size int64
	
	_ = filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	
	return size
}

// Priority calculation helpers
func getGenomicsPriority(ext string) int {
	switch {
	case strings.Contains(ext, "fastq"):
		return 1 // Highest priority - raw data
	case strings.Contains(ext, "bam") || strings.Contains(ext, "sam"):
		return 2 // Alignment files
	case strings.Contains(ext, "vcf"):
		return 3 // Variant calls
	default:
		return 4
	}
}

func getImagingPriority(ext string) int {
	switch {
	case strings.Contains(ext, "czi") || strings.Contains(ext, "lsm"):
		return 1 // Raw microscopy files
	case strings.Contains(ext, "tiff") || strings.Contains(ext, "tif"):
		return 2 // Processed images
	default:
		return 3
	}
}

func getComputationalPriority(ext string) int {
	switch {
	case strings.Contains(ext, "csv") || strings.Contains(ext, "tsv"):
		return 2 // Results files
	case strings.Contains(ext, "log") || strings.Contains(ext, "out"):
		return 5 // Log files - lowest priority
	default:
		return 3
	}
}