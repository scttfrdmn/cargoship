package manifest

import "sort"

// FileDiff summarizes the per-path differences between two effective file sets
// (each from a MergeChain/ResolveEffective view), for `dataset diff` (#521).
type FileDiff struct {
	Added     []string // paths present in `to` but not `from`
	Removed   []string // paths present in `from` but not `to`
	Modified  []string // paths in both whose content differs
	Unchanged int      // paths in both that are identical
}

// DiffFiles compares two effective file sets by path. A path present on only one
// side is Added/Removed; a path on both is Modified when its content differs and
// Unchanged otherwise. Content is compared by Checksum when both entries carry
// one (the strong signal), else by Size. Output slices are sorted for
// deterministic reporting.
func DiffFiles(from, to []FileEntry) FileDiff {
	fromByPath := make(map[string]FileEntry, len(from))
	for _, f := range from {
		fromByPath[f.Path] = f
	}
	toByPath := make(map[string]FileEntry, len(to))
	for _, f := range to {
		toByPath[f.Path] = f
	}

	var d FileDiff
	for p, tf := range toByPath {
		ff, ok := fromByPath[p]
		if !ok {
			d.Added = append(d.Added, p)
			continue
		}
		if fileEntriesDiffer(ff, tf) {
			d.Modified = append(d.Modified, p)
		} else {
			d.Unchanged++
		}
	}
	for p := range fromByPath {
		if _, ok := toByPath[p]; !ok {
			d.Removed = append(d.Removed, p)
		}
	}
	sort.Strings(d.Added)
	sort.Strings(d.Removed)
	sort.Strings(d.Modified)
	return d
}

// fileEntriesDiffer reports whether two entries for the same path represent
// different content: by checksum when both carry one, otherwise by size.
func fileEntriesDiffer(a, b FileEntry) bool {
	if a.Checksum != "" && b.Checksum != "" {
		return a.Checksum != b.Checksum
	}
	return a.Size != b.Size
}

// HasChanges reports whether the diff found any added, removed, or modified path.
func (d FileDiff) HasChanges() bool {
	return len(d.Added) > 0 || len(d.Removed) > 0 || len(d.Modified) > 0
}
