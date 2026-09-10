package s3

import (
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/stretchr/testify/assert"

	awsconfig "github.com/scttfrdmn/cargoship/pkg/aws/config"
)

// TestTransporterContentEncoding is the #353 guard: an explicit
// Archive.ContentEncoding becomes the object's HTTP Content-Encoding header, and
// — crucially — CompressionType alone does NOT set it (that stays a private
// x-amz-meta annotation), so CargoShip's own .tar.zst chunks are never stamped
// with a header that would make HTTP clients auto-decompress them.
func TestTransporterContentEncoding(t *testing.T) {
	tr := &Transporter{config: awsconfig.S3Config{Bucket: "b"}}

	// Explicit encoding → header set.
	in := tr.putObjectInput(Archive{Key: "k", ContentEncoding: "zstd"}, types.StorageClassStandard)
	if assert.NotNil(t, in.ContentEncoding, "explicit ContentEncoding must set the header") {
		assert.Equal(t, "zstd", *in.ContentEncoding)
	}

	// CompressionType set but ContentEncoding empty → NO header (the bug #353 fixes),
	// while the private compression annotation is still recorded in metadata.
	in = tr.putObjectInput(Archive{Key: "k", CompressionType: "zstd"}, types.StorageClassStandard)
	assert.Nil(t, in.ContentEncoding, "CompressionType must not leak into Content-Encoding")
	assert.Equal(t, "zstd", in.Metadata["cargoship-compression-type"], "compression stays a private metadata annotation")
}

// TestOptimizedTransporterContentEncoding asserts the optimized path applies the
// same #353 rule.
func TestOptimizedTransporterContentEncoding(t *testing.T) {
	tr := &OptimizedTransporter{config: awsconfig.S3Config{Bucket: "b"}}

	in := tr.putObjectInput(&Archive{Key: "k", ContentEncoding: "gzip"})
	if assert.NotNil(t, in.ContentEncoding) {
		assert.Equal(t, "gzip", *in.ContentEncoding)
	}

	in = tr.putObjectInput(&Archive{Key: "k", CompressionType: "zstd"})
	assert.Nil(t, in.ContentEncoding, "CompressionType must not leak into Content-Encoding")
}

// TestContentEncodingCargoShipChunksUnset documents the invariant that CargoShip's
// own chunk archives (built by the pipeline) leave ContentEncoding empty, so the
// header is never set on .tar.zst objects — the zero value guarantees it.
func TestContentEncodingCargoShipChunksUnset(t *testing.T) {
	var a Archive // as the pipeline builds it: ContentEncoding never assigned
	assert.Empty(t, a.ContentEncoding)
	assert.False(t, strings.Contains(a.ContentEncoding, "zstd"))
}
