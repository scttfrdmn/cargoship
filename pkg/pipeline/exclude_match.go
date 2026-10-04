package pipeline

import (
	"path/filepath"
	"strings"
)

// matchesExcludePattern reports whether relPath — a path relative to the scan root — is
// excluded by any of the patterns (#710).
//
// A pattern matches when it matches the whole relative path, or ANY single path segment.
// Segment matching is what makes subtree exclusion expressible: "#recycle" excludes the
// directory and everything beneath it, because the first segment of
// "#recycle/a/invoice.pdf" matches. The previous matcher compared only
// filepath.Base(path), so a directory pattern matched the directory entry and then every
// file inside it was archived anyway — on a real Synology that left 20 GB of recycle bin,
// and .aws/.ssh credentials, impossible to keep out of the archive.
//
// Deliberately NOT using "**": per-segment matching already covers subtree exclusion,
// which is the case operators actually need, and it avoids taking a glob dependency for
// syntax that would then need its own documentation and edge cases. A pattern containing
// a separator is matched against the relative path, so "a/b" still works.
//
// Patterns use filepath.Match syntax (`*`, `?`, `[...]`), which does not cross separators.
func matchesExcludePattern(relPath string, patterns []string) bool {
	if len(patterns) == 0 || relPath == "" {
		return false
	}
	// Normalise to forward slashes so patterns written in config are portable.
	rel := filepath.ToSlash(relPath)

	for _, pattern := range patterns {
		if pattern == "" {
			continue
		}
		p := filepath.ToSlash(pattern)
		p = strings.TrimSuffix(p, "/") // "#recycle/" means the same as "#recycle"

		// Whole relative path.
		if ok, err := filepath.Match(p, rel); err == nil && ok {
			return true
		}
		// A pattern with a separator is a path pattern; also treat it as a subtree root.
		if strings.Contains(p, "/") {
			if rel == p || strings.HasPrefix(rel, p+"/") {
				return true
			}
			continue
		}
		// Otherwise match any single segment, which covers both basename globs and
		// "exclude this directory and everything under it".
		for _, seg := range strings.Split(rel, "/") {
			if ok, err := filepath.Match(p, seg); err == nil && ok {
				return true
			}
		}
	}
	return false
}
