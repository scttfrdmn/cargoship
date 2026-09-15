package launch

import (
	"strings"
	"testing"
	"time"

	cargoshipconfig "github.com/scttfrdmn/cargoship/pkg/aws/config"
)

func validConfig() *GhostShipConfig {
	return &GhostShipConfig{
		ID:       "nas-1",
		S3Config: cargoshipconfig.S3Config{Bucket: "my-bucket", StorageClass: cargoshipconfig.StorageClassStandard},
		WatchPaths: []WatchPath{{
			Path:            "/data/docs",
			IncludePatterns: []string{"*.pdf"},
			ExcludePatterns: []string{"*.tmp"},
			Recursive:       true,
		}},
		ArchivalRules: []ArchivalRule{{
			Name:         "docs",
			FilePattern:  "*.pdf",
			StorageClass: "GLACIER_IR",
		}},
	}
}

func countSeverity(issues []ConfigIssue, sev Severity) int {
	n := 0
	for _, is := range issues {
		if is.Severity == sev {
			n++
		}
	}
	return n
}

func hasIssue(issues []ConfigIssue, sev Severity, fieldSubstr string) bool {
	for _, is := range issues {
		if is.Severity == sev && strings.Contains(is.Field, fieldSubstr) {
			return true
		}
	}
	return false
}

func TestValidateConfig(t *testing.T) {
	if got := countSeverity(ValidateConfig(validConfig()), SeverityError); got != 0 {
		t.Fatalf("valid config produced %d errors: %+v", got, ValidateConfig(validConfig()))
	}

	tests := []struct {
		name   string
		mutate func(*GhostShipConfig)
		field  string // error field substring expected
	}{
		{"missing id", func(c *GhostShipConfig) { c.ID = "" }, "id"},
		{"missing bucket", func(c *GhostShipConfig) { c.S3Config.Bucket = "" }, "s3_config.bucket"},
		{"no watch paths", func(c *GhostShipConfig) { c.WatchPaths = nil }, "watch_paths"},
		{"no rules", func(c *GhostShipConfig) { c.ArchivalRules = nil }, "archival_rules"},
		{"empty watch path", func(c *GhostShipConfig) { c.WatchPaths[0].Path = "" }, "path"},
		{"bad rule storage class", func(c *GhostShipConfig) { c.ArchivalRules[0].StorageClass = "NOPE" }, "storage_class"},
		{"invalid include glob", func(c *GhostShipConfig) { c.WatchPaths[0].IncludePatterns = []string{"["} }, "include_patterns"},
		{"min>max size", func(c *GhostShipConfig) { c.ArchivalRules[0].MinSize, c.ArchivalRules[0].MaxSize = 100, 10 }, "max_size"},
		{"min>max age", func(c *GhostShipConfig) {
			c.ArchivalRules[0].MinAge, c.ArchivalRules[0].MaxAge = 2*time.Hour, time.Hour
		}, "max_age"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := validConfig()
			tt.mutate(c)
			issues := ValidateConfig(c)
			if !hasIssue(issues, SeverityError, tt.field) {
				t.Errorf("expected an error on %q, got: %+v", tt.field, issues)
			}
		})
	}

	// Dangerous combo: delete-after-archive with a broad matcher is a warning.
	t.Run("delete-after-archive + broad matcher warns", func(t *testing.T) {
		c := validConfig()
		c.ArchivalRules[0].FilePattern = "*"
		c.ArchivalRules[0].PathPattern = ""
		c.ArchivalRules[0].DeleteAfterArchive = true
		issues := ValidateConfig(c)
		if !hasIssue(issues, SeverityWarn, "delete_after_archive") {
			t.Errorf("expected a delete_after_archive warning, got: %+v", issues)
		}
		if countSeverity(issues, SeverityError) != 0 {
			t.Errorf("broad+delete should warn, not error: %+v", issues)
		}
	})
}

func TestWatchScopeWidenings(t *testing.T) {
	base := validConfig()

	if w := WatchScopeWidenings(base, validConfig()); len(w) != 0 {
		t.Errorf("identical configs should not widen, got: %v", w)
	}

	tests := []struct {
		name   string
		mutate func(*GhostShipConfig)
		want   string // substring expected in a widening
	}{
		{"new path", func(c *GhostShipConfig) {
			c.WatchPaths = append(c.WatchPaths, WatchPath{Path: "/data/other"})
		}, "new watch path"},
		{"recursive flip", func(c *GhostShipConfig) { c.WatchPaths[0].Recursive = true }, "recursive"},
		{"added include", func(c *GhostShipConfig) {
			c.WatchPaths[0].IncludePatterns = append(c.WatchPaths[0].IncludePatterns, "*.docx")
		}, "added include"},
		{"removed exclude", func(c *GhostShipConfig) { c.WatchPaths[0].ExcludePatterns = nil }, "removed exclude"},
		{"delete-after-archive enabled", func(c *GhostShipConfig) { c.ArchivalRules[0].DeleteAfterArchive = true }, "delete_after_archive"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prev := validConfig()
			prev.WatchPaths[0].Recursive = false // baseline non-recursive for the flip case
			next := validConfig()
			next.WatchPaths[0].Recursive = false
			tt.mutate(next)
			w := WatchScopeWidenings(prev, next)
			found := false
			for _, item := range w {
				if strings.Contains(item, tt.want) {
					found = true
				}
			}
			if !found {
				t.Errorf("expected a widening containing %q, got: %v", tt.want, w)
			}
		})
	}

	// Narrowing must NOT be reported.
	t.Run("narrowing is not a widening", func(t *testing.T) {
		prev := validConfig()
		next := validConfig()
		next.WatchPaths = nil // removed a path
		next.ArchivalRules[0].DeleteAfterArchive = false
		if w := WatchScopeWidenings(prev, next); len(w) != 0 {
			t.Errorf("removing a path should not widen, got: %v", w)
		}
	})
}
