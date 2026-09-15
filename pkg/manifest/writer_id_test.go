package manifest

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestSetWriterID covers the builder setter, the "writers" format marker, and the
// legacy (single-writer) omitempty behavior (#520).
func TestSetWriterID(t *testing.T) {
	b, err := NewBuilder("u1", "/src", "b", "p/writers/lab-nas-1", "r")
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}
	b.SetWriterID("lab-nas-1")
	b.SetWriterID("lab-nas-1") // idempotent: must not duplicate the feature marker
	m := b.Finalize()
	if m.WriterID != "lab-nas-1" {
		t.Errorf("WriterID=%q, want lab-nas-1", m.WriterID)
	}
	if n := countFeatureMarker(m.FormatFeatures, FormatFeatureWriters); n != 1 {
		t.Errorf("FormatFeatures %v should include %q exactly once, got %d", m.FormatFeatures, FormatFeatureWriters, n)
	}

	// Empty writerID is a no-op: single-writer manifest, no feature, and writer_id
	// omitted from JSON so a legacy upload is byte-compatible with pre-#520 readers.
	b2, _ := NewBuilder("u2", "/src", "b", "p", "r")
	b2.SetWriterID("")
	m2 := b2.Finalize()
	if m2.WriterID != "" || containsID(m2.FormatFeatures, FormatFeatureWriters) {
		t.Errorf("empty writerID should be a no-op, got WriterID=%q features=%v", m2.WriterID, m2.FormatFeatures)
	}
	data, err := json.Marshal(m2)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(data), "writer_id") {
		t.Errorf("legacy manifest JSON must omit writer_id, got: %s", data)
	}
}

func countFeatureMarker(ss []string, s string) int {
	n := 0
	for _, x := range ss {
		if x == s {
			n++
		}
	}
	return n
}
