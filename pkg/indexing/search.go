// Package indexing provides enhanced search capabilities for CargoShip archives
package indexing

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// SearchEngine provides advanced search capabilities across archive indexes
type SearchEngine struct {
	indexer *Indexer
	logger  *slog.Logger
}

// NewSearchEngine creates a new search engine
func NewSearchEngine(indexer *Indexer, logger *slog.Logger) *SearchEngine {
	if logger == nil {
		logger = slog.Default()
	}
	
	return &SearchEngine{
		indexer: indexer,
		logger:  logger.With("component", "search_engine"),
	}
}

// Search performs a search across all loaded indexes or a specific location
func (se *SearchEngine) Search(ctx context.Context, filter SearchFilter, locations ...string) (*SearchResult, error) {
	startTime := time.Now()
	
	se.logger.Debug("starting search", "filter", filter, "locations", locations)
	
	// If no locations specified, search all cached indexes
	if len(locations) == 0 {
		cached := se.indexer.GetCacheStats()
		if cachedCount, ok := cached["cached_indexes"].(int); ok && cachedCount == 0 {
			return &SearchResult{
				Files:        []*EnhancedFile{},
				TotalMatches: 0,
				SearchTime:   time.Since(startTime),
				IndexUsed:    "none",
				Truncated:    false,
			}, nil
		}
		
		// Search all cached indexes
		for location := range se.indexer.indexes {
			locations = append(locations, location)
		}
	}
	
	var allMatches []*EnhancedFile
	indexesUsed := []string{}
	
	// Search each specified location
	for _, location := range locations {
		index, err := se.indexer.LoadIndex(ctx, location)
		if err != nil {
			se.logger.Warn("failed to load index for search", "location", location, "error", err)
			continue
		}
		
		matches := se.searchInIndex(index, filter)
		allMatches = append(allMatches, matches...)
		indexesUsed = append(indexesUsed, location)
	}
	
	// Sort results
	se.sortResults(allMatches, filter)
	
	// Apply result limit
	truncated := false
	if filter.MaxResults > 0 && len(allMatches) > filter.MaxResults {
		allMatches = allMatches[:filter.MaxResults]
		truncated = true
	}
	
	searchTime := time.Since(startTime)
	
	result := &SearchResult{
		Files:        allMatches,
		TotalMatches: len(allMatches),
		SearchTime:   searchTime,
		IndexUsed:    strings.Join(indexesUsed, ", "),
		Truncated:    truncated,
	}
	
	se.logger.Info("search completed", 
		"matches", len(allMatches), 
		"search_time", searchTime,
		"indexes_searched", len(indexesUsed))
	
	return result, nil
}

// Browse provides directory-style browsing of archive contents
func (se *SearchEngine) Browse(ctx context.Context, location string, path string, options BrowseOptions) (*BrowseResult, error) {
	startTime := time.Now()
	
	se.logger.Debug("starting browse", "location", location, "path", path, "options", options)
	
	// Load the index for this location
	index, err := se.indexer.LoadIndex(ctx, location)
	if err != nil {
		return nil, fmt.Errorf("failed to load index for browsing: %w", err)
	}
	
	// Normalize path
	if path == "" || path == "/" {
		path = ""
	} else {
		path = strings.Trim(path, "/") + "/"
	}
	
	var files []*EnhancedFile
	directories := make(map[string]*DirectoryInfo)
	
	for _, file := range index.Files {
		// Normalize file destination path
		filePath := strings.TrimPrefix(file.Destination, "/")
		
		// Check if file is in current path or subdirectory
		if path == "" || strings.HasPrefix(filePath, path) {
			// Get relative path from current browse path
			relativePath := strings.TrimPrefix(filePath, path)
			
			// If this is a direct file in current directory
			if !strings.Contains(relativePath, "/") {
				// Apply filter if provided
				if options.Filter != nil && !se.fileMatchesFilter(file, *options.Filter) {
					continue
				}
				
				// Skip hidden files unless requested
				if !options.ShowHidden && strings.HasPrefix(filepath.Base(relativePath), ".") {
					continue
				}
				
				files = append(files, file)
			} else if options.Recursive {
				// For recursive browsing, include all matching files
				if options.Filter == nil || se.fileMatchesFilter(file, *options.Filter) {
					if options.ShowHidden || !strings.HasPrefix(filepath.Base(relativePath), ".") {
						files = append(files, file)
					}
				}
			} else {
				// Extract immediate subdirectory name
				parts := strings.Split(relativePath, "/")
				if len(parts) > 0 && parts[0] != "" {
					dirName := parts[0]
					dirPath := path + dirName
					
					if _, exists := directories[dirName]; !exists {
						directories[dirName] = &DirectoryInfo{
							Name:         dirName,
							Path:         dirPath,
							FileCount:    0,
							TotalSize:    0,
							LastModified: file.ModifiedAt,
							IsArchive:    se.isArchiveFile(file.Name),
						}
					}
					
					dir := directories[dirName]
					dir.FileCount++
					dir.TotalSize += file.Size
					if file.ModifiedAt.After(dir.LastModified) {
						dir.LastModified = file.ModifiedAt
					}
				}
			}
		}
	}
	
	// Sort files
	se.sortFilesByOptions(files, options)
	
	// Convert directories map to slice and sort
	dirList := make([]DirectoryInfo, 0, len(directories))
	for _, dir := range directories {
		dirList = append(dirList, *dir)
	}
	se.sortDirectoriesByOptions(dirList, options)
	
	// Calculate totals
	totalSize := int64(0)
	for _, file := range files {
		totalSize += file.Size
	}
	
	// Apply pagination
	hasMore := false
	if options.PageSize > 0 {
		start := options.PageOffset
		end := start + options.PageSize
		
		if start < len(files) {
			if end > len(files) {
				end = len(files)
			} else {
				hasMore = true
			}
			files = files[start:end]
		} else {
			files = []*EnhancedFile{}
		}
	}
	
	browseTime := time.Since(startTime)
	
	result := &BrowseResult{
		Path:        path,
		Files:       files,
		Directories: dirList,
		TotalFiles:  len(files),
		TotalSize:   totalSize,
		BrowseTime:  browseTime,
		HasMore:     hasMore,
	}
	
	se.logger.Info("browse completed",
		"location", location,
		"path", path,
		"files", len(files),
		"directories", len(dirList),
		"browse_time", browseTime)
	
	return result, nil
}

// FindInArchive searches for files within archive contents (ArchiveTOC)
func (se *SearchEngine) FindInArchive(ctx context.Context, location string, archivePattern string, filePattern string) (*SearchResult, error) {
	startTime := time.Now()
	
	index, err := se.indexer.LoadIndex(ctx, location)
	if err != nil {
		return nil, fmt.Errorf("failed to load index: %w", err)
	}
	
	var matches []*EnhancedFile
	
	archiveRegex, err := compilePattern(archivePattern)
	if err != nil {
		return nil, fmt.Errorf("invalid archive pattern: %w", err)
	}
	
	fileRegex, err := compilePattern(filePattern)
	if err != nil {
		return nil, fmt.Errorf("invalid file pattern: %w", err)
	}
	
	for _, file := range index.Files {
		// Check if the archive file name matches
		if archiveRegex != nil && !archiveRegex.MatchString(file.Name) {
			continue
		}
		
		// Search within archive TOC
		if len(file.ArchiveTOC) > 0 {
			tocMatches := false
			for _, tocEntry := range file.ArchiveTOC {
				if fileRegex == nil || fileRegex.MatchString(tocEntry) {
					tocMatches = true
					break
				}
			}
			if tocMatches {
				matches = append(matches, file)
			}
		}
	}
	
	searchTime := time.Since(startTime)
	
	return &SearchResult{
		Files:        matches,
		TotalMatches: len(matches),
		SearchTime:   searchTime,
		IndexUsed:    location,
		Truncated:    false,
	}, nil
}

// GetFilesBySize returns files grouped by size categories
func (se *SearchEngine) GetFilesBySize(ctx context.Context, location string) (map[string][]*EnhancedFile, error) {
	index, err := se.indexer.LoadIndex(ctx, location)
	if err != nil {
		return nil, fmt.Errorf("failed to load index: %w", err)
	}
	
	sizeGroups := make(map[string][]*EnhancedFile)
	
	for _, file := range index.Files {
		category := getSizeCategory(file.Size)
		sizeGroups[category] = append(sizeGroups[category], file)
	}
	
	return sizeGroups, nil
}

// GetFilesByType returns files grouped by file type
func (se *SearchEngine) GetFilesByType(ctx context.Context, location string) (map[string][]*EnhancedFile, error) {
	index, err := se.indexer.LoadIndex(ctx, location)
	if err != nil {
		return nil, fmt.Errorf("failed to load index: %w", err)
	}
	
	typeGroups := make(map[string][]*EnhancedFile)
	
	for _, file := range index.Files {
		fileType := getFileType(file.Name)
		typeGroups[fileType] = append(typeGroups[fileType], file)
	}
	
	return typeGroups, nil
}

// searchInIndex performs search within a single index
func (se *SearchEngine) searchInIndex(index *ArchiveIndex, filter SearchFilter) []*EnhancedFile {
	var matches []*EnhancedFile
	
	for _, file := range index.Files {
		if se.fileMatchesFilter(file, filter) {
			matches = append(matches, file)
		}
	}
	
	return matches
}

// fileMatchesFilter checks if a file matches the given search filter
func (se *SearchEngine) fileMatchesFilter(file *EnhancedFile, filter SearchFilter) bool {
	// Name pattern matching
	if filter.NamePattern != "" {
		if matched, err := filepath.Match(filter.NamePattern, file.Name); err != nil || !matched {
			if !filter.CaseSensitive {
				if matched, err := filepath.Match(strings.ToLower(filter.NamePattern), strings.ToLower(file.Name)); err != nil || !matched {
					return false
				}
			} else {
				return false
			}
		}
	}
	
	// Extension filtering
	if len(filter.Extensions) > 0 {
		ext := strings.ToLower(filepath.Ext(file.Name))
		found := false
		for _, filterExt := range filter.Extensions {
			if strings.ToLower(filterExt) == ext {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	
	// Size filtering
	if filter.MinSize > 0 && file.Size < filter.MinSize {
		return false
	}
	if filter.MaxSize > 0 && file.Size > filter.MaxSize {
		return false
	}
	
	// Date filtering
	if filter.ModifiedAfter != nil && file.ModifiedAt.Before(*filter.ModifiedAfter) {
		return false
	}
	if filter.ModifiedBefore != nil && file.ModifiedAt.After(*filter.ModifiedBefore) {
		return false
	}
	if filter.ArchivedAfter != nil && (file.ArchivedAt == nil || file.ArchivedAt.Before(*filter.ArchivedAfter)) {
		return false
	}
	if filter.ArchivedBefore != nil && (file.ArchivedAt == nil || file.ArchivedAt.After(*filter.ArchivedBefore)) {
		return false
	}
	
	// Content type filtering
	if filter.ContentType != "" {
		if matched, err := filepath.Match(filter.ContentType, file.ContentType); err != nil || !matched {
			return false
		}
	}
	
	// Tag filtering
	if len(filter.Tags) > 0 {
		for key, value := range filter.Tags {
			if !file.HasTag(key, value) {
				return false
			}
		}
	}
	
	// Storage class filtering
	if filter.StorageClass != "" && file.StorageClass != filter.StorageClass {
		return false
	}
	
	// Suitcase pattern filtering
	if filter.SuitcasePattern != "" {
		if matched, err := filepath.Match(filter.SuitcasePattern, file.SuitcaseName); err != nil || !matched {
			return false
		}
	}
	
	// Path pattern filtering
	if filter.PathPattern != "" {
		if matched, err := filepath.Match(filter.PathPattern, file.Path); err != nil || !matched {
			return false
		}
	}
	
	// Archive TOC filtering
	if filter.HasArchiveTOC && len(file.ArchiveTOC) == 0 {
		return false
	}
	
	// Compression filtering
	if filter.CompressionType != "" && file.CompressionInfo.Algorithm != filter.CompressionType {
		return false
	}
	if filter.MinCompressionRatio > 0 && file.CompressionInfo.CompressionRatio < filter.MinCompressionRatio {
		return false
	}
	
	return true
}

// sortResults sorts search results based on various criteria
func (se *SearchEngine) sortResults(files []*EnhancedFile, filter SearchFilter) {
	sort.Slice(files, func(i, j int) bool {
		// Primary sort by relevance score (files matching name pattern first)
		iMatches := filter.NamePattern != "" && se.matchesPattern(files[i].Name, filter.NamePattern)
		jMatches := filter.NamePattern != "" && se.matchesPattern(files[j].Name, filter.NamePattern)
		
		if iMatches && !jMatches {
			return true
		}
		if !iMatches && jMatches {
			return false
		}
		
		// Secondary sort by size (larger files first)
		if files[i].Size != files[j].Size {
			return files[i].Size > files[j].Size
		}
		
		// Tertiary sort by name
		return files[i].Name < files[j].Name
	})
}

// sortFilesByOptions sorts files based on browse options
func (se *SearchEngine) sortFilesByOptions(files []*EnhancedFile, options BrowseOptions) {
	sort.Slice(files, func(i, j int) bool {
		switch options.SortBy {
		case "size":
			if options.SortOrder == "desc" {
				return files[i].Size > files[j].Size
			}
			return files[i].Size < files[j].Size
		case "date", "modified":
			if options.SortOrder == "desc" {
				return files[i].ModifiedAt.After(files[j].ModifiedAt)
			}
			return files[i].ModifiedAt.Before(files[j].ModifiedAt)
		case "type":
			iType := getFileType(files[i].Name)
			jType := getFileType(files[j].Name)
			if iType != jType {
				if options.SortOrder == "desc" {
					return iType > jType
				}
				return iType < jType
			}
			fallthrough
		case "name":
			fallthrough
		default:
			if options.SortOrder == "desc" {
				return files[i].Name > files[j].Name
			}
			return files[i].Name < files[j].Name
		}
	})
}

// sortDirectoriesByOptions sorts directories based on browse options
func (se *SearchEngine) sortDirectoriesByOptions(dirs []DirectoryInfo, options BrowseOptions) {
	sort.Slice(dirs, func(i, j int) bool {
		switch options.SortBy {
		case "size":
			if options.SortOrder == "desc" {
				return dirs[i].TotalSize > dirs[j].TotalSize
			}
			return dirs[i].TotalSize < dirs[j].TotalSize
		case "date", "modified":
			if options.SortOrder == "desc" {
				return dirs[i].LastModified.After(dirs[j].LastModified)
			}
			return dirs[i].LastModified.Before(dirs[j].LastModified)
		case "name":
			fallthrough
		default:
			if options.SortOrder == "desc" {
				return dirs[i].Name > dirs[j].Name
			}
			return dirs[i].Name < dirs[j].Name
		}
	})
}

// matchesPattern checks if a string matches a glob pattern
func (se *SearchEngine) matchesPattern(text, pattern string) bool {
	matched, err := filepath.Match(pattern, text)
	if err != nil {
		return false
	}
	return matched
}

// isArchiveFile checks if a file appears to be an archive
func (se *SearchEngine) isArchiveFile(filename string) bool {
	return getFileType(filename) == "archive"
}

// compilePattern compiles a glob pattern to regex for more efficient matching
func compilePattern(pattern string) (*regexp.Regexp, error) {
	if pattern == "" {
		return nil, nil
	}
	
	// Convert glob pattern to regex
	regexPattern := strings.ReplaceAll(pattern, "*", ".*")
	regexPattern = strings.ReplaceAll(regexPattern, "?", ".")
	regexPattern = "^" + regexPattern + "$"
	
	return regexp.Compile(regexPattern)
}