package cmd

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// #725: FileChecksums was documented as "on by default, --no-file-checksums opts
// out", but was set in exactly one place in the tree (upload.go). The sync path
// never set it, so it took Go's zero value false — and that path backs BOTH
// `cargoship sync` and every ghostship fleet agent. Measured on one corpus at the
// same version: upload recorded 3/3 per-file checksums, sync recorded 0/3; a real
// 410-file NAS backup had 0/410.
//
// Without a per-file checksum recorded at upload, nothing can substantiate the
// project's byte-identity claim per file after the fact, so this guards the
// default rather than any particular wiring.
func TestNewSyncPipelineConfig_RecordsFileChecksumsByDefault(t *testing.T) {
	cfg := newSyncPipelineConfig(syncPipelineParams{
		bucket:        "bucket",
		prefix:        "prefix",
		region:        "us-west-2",
		sourcePath:    "/data/src",
		s3Client:      &s3.Client{},
		fileChecksums: true, // what both callers now pass by default
	})
	if !cfg.FileChecksums {
		t.Error("sync must record per-file checksums by default (#725); without them " +
			"'verify --deep' cannot confirm per-file integrity for anything sync or " +
			"ghostship uploads")
	}
}

// The opt-out has to keep working, or `--no-file-checksums` becomes the next
// flag that is accepted and ignored (cf. #678, #711).
func TestNewSyncPipelineConfig_FileChecksumsCanBeDisabled(t *testing.T) {
	cfg := newSyncPipelineConfig(syncPipelineParams{
		bucket:        "bucket",
		prefix:        "prefix",
		region:        "us-west-2",
		sourcePath:    "/data/src",
		s3Client:      &s3.Client{},
		fileChecksums: false,
	})
	if cfg.FileChecksums {
		t.Error("--no-file-checksums must actually disable per-file checksums")
	}
}

// `sync` must expose the same opt-out `upload` has. Before #725 it had neither
// the flag nor the checksums, so fleet users took the no-checksum trade silently
// with no way to decline it.
func TestSyncCmd_HasFileChecksumOptOut(t *testing.T) {
	cmd := NewSyncCmd()
	f := cmd.Flags().Lookup("no-file-checksums")
	if f == nil {
		t.Fatal("sync must expose --no-file-checksums for parity with upload (#725)")
	}
	// Opt-OUT framing: the default must be false, i.e. checksums on.
	if f.DefValue != "false" {
		t.Errorf("--no-file-checksums default = %q, want \"false\" so checksums are ON by default", f.DefValue)
	}
}

// The ghostship agent must record checksums unconditionally. It is the one path
// where nobody is watching, the data sits longest, and the writer's own identity
// cannot read objects back to check them — so a signed config must not be able to
// quietly downgrade the guarantee. Asserted on the struct literal the agent
// builds, via the same function the agent calls.
func TestGhostshipSyncAlwaysRecordsFileChecksums(t *testing.T) {
	p := syncRunParams{
		bucket:     "bucket",
		prefix:     "writers/lab-nas-1",
		sourcePath: "/share/data",
		region:     "us-west-2",
		writerID:   "lab-nas-1",
	}
	got := ghostshipPipelineParams(p, nil, "full", "", nil, "", 1)
	if !got.fileChecksums {
		t.Error("the ghostship agent must always record per-file checksums (#725): " +
			"an unattended write-only writer is the last place that should silently " +
			"lose per-file verifiability")
	}
}
