package migration

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/scttfrdmn/cargoship/pkg/extraction"
	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

// maxListed caps how many paths an error names.
const maxListed = 10

// verifyUploadCovers returns an error unless the new upload holds everything the
// source archive did, so deleting the original loses nothing (#911). Every
// regular file under extracted (the extraction of the archive) must be in m
// with a stored location and its size; the archive must have had no entries the
// extractor couldn't restore and no symlinks, which the upload doesn't store.
func verifyUploadCovers(extracted string, stats *extraction.ExtractionStats, m *manifest.Manifest) error {
	if stats != nil && stats.UnsupportedEntries > 0 {
		return fmt.Errorf("the archive holds %d entr(ies) that can't be extracted or stored (%s)",
			stats.UnsupportedEntries, strings.Join(stats.UnsupportedExamples, ", "))
	}
	if stats != nil && stats.SymlinksExtracted > 0 {
		return fmt.Errorf("the archive holds %d symlink(s), which the new upload doesn't store", stats.SymlinksExtracted)
	}
	if m == nil {
		return fmt.Errorf("the new upload's manifest is unavailable, so its contents can't be checked")
	}

	stored := make(map[string]manifest.FileEntry, len(m.Files))
	for _, f := range m.Files {
		stored[filepath.ToSlash(manifest.RelativeToSource(m.SourcePath, f.Path))] = f
	}

	var missing []string
	n := 0
	err := filepath.WalkDir(extracted, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(extracted, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !info.Mode().IsRegular() {
			n++
			if len(missing) < maxListed {
				missing = append(missing, rel+" (not a regular file)")
			}
			return nil
		}
		f, ok := stored[rel]
		switch {
		case !ok:
			n++
			if len(missing) < maxListed {
				missing = append(missing, rel)
			}
		case f.S3Key == "" || f.Size != info.Size():
			n++
			if len(missing) < maxListed {
				missing = append(missing, fmt.Sprintf("%s (stored %d of %d bytes)", rel, f.Size, info.Size()))
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("check the extracted archive: %w", err)
	}
	if n > 0 {
		return fmt.Errorf("%d file(s) from the archive are not in the new upload: %s", n, strings.Join(missing, ", "))
	}
	return nil
}
