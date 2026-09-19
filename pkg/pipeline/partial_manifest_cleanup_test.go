package pipeline

import (
	"context"
	"testing"
)

// TestDeletePartialManifest_SkippedWhenNeverWritten is the #657 guard.
//
// A partial manifest is only written when EnablePartialManifest && manifestBuilder !=
// nil && UseRealS3. The cleanup used to run unconditionally, which was invisible for
// most identities (S3 DeleteObject on a missing key succeeds) but made a delete-free
// write-only fleet agent (#613) log a 403 AccessDenied warning on every healthy cycle.
//
// The assertion is structural rather than cosmetic: with cleanup correctly skipped the
// function returns before touching S3 at all. Without the gate it reaches
// `p.config.S3Client.(*s3.Client)` and panics on a nil client — so this test fails
// loudly if the gate is ever removed.
func TestDeletePartialManifest_SkippedWhenNeverWritten(t *testing.T) {
	cases := []struct {
		name string
		cfg  *PipelineConfig
	}{
		{
			name: "partial manifests disabled (the ghostship/sync default)",
			cfg:  &PipelineConfig{EnablePartialManifest: false, UseRealS3: true, S3Prefix: "writers/w1", UploadID: "u1"},
		},
		{
			name: "enabled but not a real S3 run",
			cfg:  &PipelineConfig{EnablePartialManifest: true, UseRealS3: false, S3Prefix: "writers/w1", UploadID: "u1"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// No S3 client and no manifest builder: any attempt to talk to S3 here is
			// itself the bug.
			p := &Pipeline{config: tc.cfg}
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("cleanup must not touch S3 when no partial manifest was written, but panicked: %v", r)
				}
			}()
			p.deletePartialManifest(context.Background())
		})
	}
}

// TestDeletePartialManifest_SkippedWithoutManifestBuilder covers the third clause of
// the write gate: EnablePartialManifest + UseRealS3 but no builder means nothing was
// ever written, so there is nothing to clean up.
func TestDeletePartialManifest_SkippedWithoutManifestBuilder(t *testing.T) {
	p := &Pipeline{config: &PipelineConfig{
		EnablePartialManifest: true, UseRealS3: true, S3Prefix: "writers/w1", UploadID: "u1",
	}}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("cleanup must be a no-op without a manifest builder, but panicked: %v", r)
		}
	}()
	p.deletePartialManifest(context.Background())
}
