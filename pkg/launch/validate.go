package launch

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// Severity classifies a ConfigIssue found by ValidateConfig.
type Severity string

const (
	// SeverityError marks a problem that makes the config invalid.
	SeverityError Severity = "error"
	// SeverityWarn marks a dangerous-but-permitted combination.
	SeverityWarn Severity = "warn"
)

// ConfigIssue is one problem found by ValidateConfig (#614).
type ConfigIssue struct {
	Severity Severity
	Field    string
	Message  string
}

// validStorageClasses is the set accepted for a ghost ship's storage-class fields
// (the pkg/aws/config classes plus GLACIER_IR).
var validStorageClasses = map[string]bool{
	"STANDARD": true, "STANDARD_IA": true, "ONEZONE_IA": true,
	"INTELLIGENT_TIERING": true, "GLACIER": true, "GLACIER_IR": true, "DEEP_ARCHIVE": true,
}

// ValidateConfig checks a ghost ship config for correctness and dangerous
// combinations WITHOUT mutating it (#614). It is the strict, non-mutating
// counterpart to validateGhostShipConfig, which only checks presence and fills
// defaults. An empty return means no issues.
func ValidateConfig(c *GhostShipConfig) []ConfigIssue {
	var issues []ConfigIssue
	add := func(sev Severity, field, msg string) {
		issues = append(issues, ConfigIssue{Severity: sev, Field: field, Message: msg})
	}

	if strings.TrimSpace(c.ID) == "" {
		add(SeverityError, "id", "id must not be empty")
	}
	if strings.TrimSpace(c.S3Config.Bucket) == "" {
		add(SeverityError, "s3_config.bucket", "s3_config.bucket must not be empty")
	}
	if sc := string(c.S3Config.StorageClass); sc != "" && !validStorageClasses[sc] {
		add(SeverityError, "s3_config.storage_class", fmt.Sprintf("unknown storage class %q", sc))
	}

	if len(c.WatchPaths) == 0 {
		add(SeverityError, "watch_paths", "at least one watch path is required")
	}
	for i, wp := range c.WatchPaths {
		field := fmt.Sprintf("watch_paths[%d]", i)
		if strings.TrimSpace(wp.Path) == "" {
			add(SeverityError, field+".path", "path must not be empty")
		}
		if wp.StorageClass != "" && !validStorageClasses[wp.StorageClass] {
			add(SeverityError, field+".storage_class", fmt.Sprintf("unknown storage class %q", wp.StorageClass))
		}
		for _, p := range wp.IncludePatterns {
			if !validGlob(p) {
				add(SeverityError, field+".include_patterns", fmt.Sprintf("invalid glob %q", p))
			}
		}
		for _, p := range wp.ExcludePatterns {
			if !validGlob(p) {
				add(SeverityError, field+".exclude_patterns", fmt.Sprintf("invalid glob %q", p))
			}
		}
	}

	if len(c.ArchivalRules) == 0 {
		add(SeverityError, "archival_rules", "at least one archival rule is required")
	}
	for i, r := range c.ArchivalRules {
		field := fmt.Sprintf("archival_rules[%d]", i)
		if strings.TrimSpace(r.Name) == "" {
			add(SeverityError, field+".name", "name must not be empty")
		}
		if r.StorageClass != "" && !validStorageClasses[r.StorageClass] {
			add(SeverityError, field+".storage_class", fmt.Sprintf("unknown storage class %q", r.StorageClass))
		}
		if !validGlob(r.PathPattern) {
			add(SeverityError, field+".path_pattern", fmt.Sprintf("invalid glob %q", r.PathPattern))
		}
		if !validGlob(r.FilePattern) {
			add(SeverityError, field+".file_pattern", fmt.Sprintf("invalid glob %q", r.FilePattern))
		}
		if r.MinSize > 0 && r.MaxSize > 0 && r.MinSize > r.MaxSize {
			add(SeverityError, field+".max_size", fmt.Sprintf("min_size (%d) exceeds max_size (%d)", r.MinSize, r.MaxSize))
		}
		if r.MinAge > 0 && r.MaxAge > 0 && r.MinAge > r.MaxAge {
			add(SeverityError, field+".max_age", fmt.Sprintf("min_age (%s) exceeds max_age (%s)", r.MinAge, r.MaxAge))
		}
		if r.DeleteAfterArchive && isBroadMatcher(r) {
			add(SeverityWarn, field+".delete_after_archive",
				"delete_after_archive is enabled with a broad matcher — this deletes source files after upload; narrow file_pattern/path_pattern or disable")
		}
	}
	return issues
}

// WatchScopeWidenings reports the ways next broadens what a ghost ship reads or
// deletes relative to prev (#614) — the primitive a config-pull loop uses to
// refuse silent scope expansion. Results are human-readable and sorted; an empty
// result means next does not widen prev. Narrowing (removing a path, adding an
// exclude) is intentionally not reported.
func WatchScopeWidenings(prev, next *GhostShipConfig) []string {
	var w []string

	prevPaths := make(map[string]WatchPath, len(prev.WatchPaths))
	for _, p := range prev.WatchPaths {
		prevPaths[p.Path] = p
	}
	for _, np := range next.WatchPaths {
		pp, existed := prevPaths[np.Path]
		if !existed {
			w = append(w, fmt.Sprintf("new watch path %q", np.Path))
			for pPath := range prevPaths {
				if pPath != np.Path && isAncestor(np.Path, pPath) {
					w = append(w, fmt.Sprintf("watch path %q is an ancestor of previous path %q (broadens coverage)", np.Path, pPath))
				}
			}
			continue
		}
		if np.Recursive && !pp.Recursive {
			w = append(w, fmt.Sprintf("watch path %q switched to recursive", np.Path))
		}
		for _, inc := range addedItems(pp.IncludePatterns, np.IncludePatterns) {
			w = append(w, fmt.Sprintf("watch path %q added include pattern %q", np.Path, inc))
		}
		for _, exc := range addedItems(np.ExcludePatterns, pp.ExcludePatterns) { // removed from next
			w = append(w, fmt.Sprintf("watch path %q removed exclude pattern %q", np.Path, exc))
		}
	}

	prevRules := make(map[string]ArchivalRule, len(prev.ArchivalRules))
	for _, r := range prev.ArchivalRules {
		prevRules[r.Name] = r
	}
	for _, nr := range next.ArchivalRules {
		pr, existed := prevRules[nr.Name]
		if nr.DeleteAfterArchive && (!existed || !pr.DeleteAfterArchive) {
			w = append(w, fmt.Sprintf("rule %q enables delete_after_archive", nr.Name))
		}
		if existed {
			if broadens(pr.FilePattern, nr.FilePattern) {
				w = append(w, fmt.Sprintf("rule %q broadened file_pattern %q -> %q", nr.Name, pr.FilePattern, nr.FilePattern))
			}
			if broadens(pr.PathPattern, nr.PathPattern) {
				w = append(w, fmt.Sprintf("rule %q broadened path_pattern %q -> %q", nr.Name, pr.PathPattern, nr.PathPattern))
			}
		}
	}

	sort.Strings(w)
	return w
}

// validGlob reports whether p is a syntactically valid filepath glob. An empty
// pattern is valid ("no constraint"). filepath.Match reports ErrBadPattern for a
// malformed pattern.
func validGlob(p string) bool {
	if p == "" {
		return true
	}
	_, err := filepath.Match(p, "")
	return err == nil
}

// isBroadMatcher reports whether a rule matches essentially everything, so that
// delete_after_archive on it is especially dangerous.
func isBroadMatcher(r ArchivalRule) bool {
	return isBroadPattern(r.FilePattern) && isBroadPattern(r.PathPattern)
}

func isBroadPattern(s string) bool {
	return s == "" || s == "*" || s == "**" || s == "**/*"
}

// broadens reports whether cur is broader than prev (prev specific, cur wildcard).
func broadens(prev, cur string) bool {
	return isBroadPattern(cur) && !isBroadPattern(prev)
}

// addedItems returns the elements of cur not present in base.
func addedItems(base, cur []string) []string {
	set := make(map[string]bool, len(base))
	for _, s := range base {
		set[s] = true
	}
	var out []string
	for _, s := range cur {
		if !set[s] {
			out = append(out, s)
		}
	}
	return out
}

// isAncestor reports whether p lies under directory anc.
func isAncestor(anc, p string) bool {
	anc = strings.TrimRight(anc, "/")
	return anc != "" && strings.HasPrefix(p, anc+"/")
}
