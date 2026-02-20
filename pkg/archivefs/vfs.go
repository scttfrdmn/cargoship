// Package archivefs provides a virtual filesystem view over a CargoShip manifest,
// enabling navigation and inspection of archived files without extraction.
package archivefs

import (
	"path"
	"sort"
	"strings"

	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

// Entry represents a file or directory node in the virtual filesystem.
type Entry struct {
	// Name is the base name of this entry (no path separators).
	Name string
	// IsDir is true when this entry represents a directory.
	IsDir bool
	// File is the underlying manifest entry; nil when IsDir is true.
	File *manifest.FileEntry
}

// VirtualFS provides directory-style navigation over a CargoShip manifest's
// file list. It builds an in-memory tree of virtual directories derived from
// file paths so that ls/cd/stat operations work without any S3 access.
type VirtualFS struct {
	m        *manifest.Manifest
	query    *manifest.ManifestQuery
	children map[string][]*Entry // normalized dir path → sorted children
	dirSet   map[string]struct{} // set of all known virtual directory paths
}

// New builds a VirtualFS from m, indexing all file paths into a virtual tree.
func New(m *manifest.Manifest) *VirtualFS {
	vfs := &VirtualFS{
		m:        m,
		query:    manifest.NewManifestQuery(m),
		children: make(map[string][]*Entry),
		dirSet:   make(map[string]struct{}),
	}
	// Root directory always exists.
	vfs.dirSet[""] = struct{}{}

	// addedDirs prevents duplicate directory entries under the same parent.
	addedDirs := make(map[string]map[string]bool)

	ensureDir := func(parent, child string) {
		if _, ok := addedDirs[parent]; !ok {
			addedDirs[parent] = make(map[string]bool)
		}
		if !addedDirs[parent][child] {
			addedDirs[parent][child] = true
			vfs.children[parent] = append(vfs.children[parent], &Entry{
				Name:  child,
				IsDir: true,
			})
		}
	}

	for i := range m.Files {
		fe := &m.Files[i]
		dirPath := path.Dir(fe.Path)
		if dirPath == "." {
			dirPath = ""
		}

		// Register all ancestor directories up to the root.
		cur := dirPath
		for {
			vfs.dirSet[cur] = struct{}{}
			if cur == "" {
				break
			}
			parent := path.Dir(cur)
			if parent == "." {
				parent = ""
			}
			ensureDir(parent, path.Base(cur))
			cur = parent
		}

		// Add the file entry to its parent directory.
		vfs.children[dirPath] = append(vfs.children[dirPath], &Entry{
			Name: path.Base(fe.Path),
			File: fe,
		})
	}

	// Sort each directory: subdirectories first (alpha), then files (alpha).
	for dir := range vfs.children {
		entries := vfs.children[dir]
		sort.Slice(entries, func(i, j int) bool {
			a, b := entries[i], entries[j]
			if a.IsDir != b.IsDir {
				return a.IsDir
			}
			return a.Name < b.Name
		})
	}

	return vfs
}

// List returns the direct children of dir. Returns nil if dir does not exist.
func (v *VirtualFS) List(dir string) []*Entry {
	if _, ok := v.dirSet[dir]; !ok {
		return nil
	}
	return v.children[dir]
}

// Stat returns the FileEntry for the given absolute path. Returns nil if path
// is a directory or does not exist in the manifest.
func (v *VirtualFS) Stat(absPath string) *manifest.FileEntry {
	return v.query.FindFile(absPath)
}

// IsDir reports whether absPath is a directory in the virtual filesystem.
func (v *VirtualFS) IsDir(absPath string) bool {
	_, ok := v.dirSet[absPath]
	return ok
}

// Resolve resolves p against cwd, returning a normalized virtual path (no
// leading slash, "" for root). Handles ".", "..", and absolute paths.
func (v *VirtualFS) Resolve(cwd, p string) string {
	p = strings.TrimSpace(p)
	if p == "" || p == "." {
		return cwd
	}
	if strings.HasPrefix(p, "/") {
		p = strings.TrimPrefix(p, "/")
		if p == "" {
			return ""
		}
		return cleanPath(p)
	}
	var combined string
	if cwd == "" {
		combined = p
	} else {
		combined = cwd + "/" + p
	}
	return cleanPath(combined)
}

// FindGlob returns all FileEntries matching pattern. The pattern is tested
// against both the full path and the basename using path.Match semantics.
func (v *VirtualFS) FindGlob(pattern string) []*manifest.FileEntry {
	var results []*manifest.FileEntry
	for i := range v.m.Files {
		fe := &v.m.Files[i]
		if ok, _ := path.Match(pattern, fe.Path); ok {
			results = append(results, fe)
			continue
		}
		if ok, _ := path.Match(pattern, path.Base(fe.Path)); ok {
			results = append(results, fe)
		}
	}
	return results
}

// Stages returns a map of DVC stage name → file count for all stages present
// in the manifest.
func (v *VirtualFS) Stages() map[string]int {
	stages := make(map[string]int)
	for i := range v.m.Files {
		fe := &v.m.Files[i]
		if fe.DVCMetadata != nil && fe.DVCMetadata.Stage != "" {
			stages[fe.DVCMetadata.Stage]++
		}
	}
	return stages
}

// FilesForStage returns all FileEntries tagged with the given DVC stage.
func (v *VirtualFS) FilesForStage(stage string) []*manifest.FileEntry {
	return v.query.FindFilesByDVCStage(stage)
}

// Manifest returns the underlying manifest.
func (v *VirtualFS) Manifest() *manifest.Manifest {
	return v.m
}

// cleanPath applies path.Clean and maps "." back to the root string "".
func cleanPath(p string) string {
	c := path.Clean(p)
	if c == "." {
		return ""
	}
	return c
}
