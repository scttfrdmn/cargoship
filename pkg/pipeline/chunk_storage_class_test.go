package pipeline

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/stretchr/testify/assert"

	"github.com/scttfrdmn/cargoship/pkg/chunking"
)

// TestChunkStorageClass pins the class every chunk upload path writes with
// (#1000): the multi-prefix uploader (the CLI default) used the configured class
// only on its SDK path and nothing at all on its transporter path, and ignored
// --auto-tier's pre-assigned and selected tiers on both.
func TestChunkStorageClass(t *testing.T) {
	old := time.Now().Add(-400 * 24 * time.Hour).UTC().Format(time.RFC3339)
	recent := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	selector := &StorageTierSelector{
		Enabled: true, DefaultClass: types.StorageClassStandard,
		HotDays: 30, ColdDays: 90, ArchiveDays: 180, FallbackToMtime: true,
	}
	file := func(atime string) chunking.File {
		return chunking.File{Metadata: map[string]string{"atime": atime}}
	}
	tests := []struct {
		name  string
		cfg   *S3UploaderConfig
		chunk chunking.Chunk
		want  types.StorageClass
	}{
		{"configured class", &S3UploaderConfig{StorageClass: types.StorageClassStandardIa},
			chunking.Chunk{Files: []chunking.File{file(old)}}, types.StorageClassStandardIa},
		{"pre-assigned tier wins", &S3UploaderConfig{StorageClass: types.StorageClassStandard, TierSelector: selector},
			chunking.Chunk{PreAssignedTier: types.StorageClassGlacier, Files: []chunking.File{file(recent)}}, types.StorageClassGlacier},
		{"selector on old files", &S3UploaderConfig{StorageClass: types.StorageClassStandard, TierSelector: selector},
			chunking.Chunk{Files: []chunking.File{file(old)}}, types.StorageClassDeepArchive},
		{"one hot file keeps the chunk hot", &S3UploaderConfig{StorageClass: types.StorageClassStandard, TierSelector: selector},
			chunking.Chunk{Files: []chunking.File{file(old), file(recent)}}, types.StorageClassStandard},
		{"disabled selector uses the configured class", &S3UploaderConfig{StorageClass: types.StorageClassGlacierIr, TierSelector: &StorageTierSelector{}},
			chunking.Chunk{Files: []chunking.File{file(old)}}, types.StorageClassGlacierIr},
		{"empty chunk uses the configured class", &S3UploaderConfig{StorageClass: types.StorageClassStandardIa, TierSelector: selector},
			chunking.Chunk{}, types.StorageClassStandardIa},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, chunkStorageClass(tt.cfg, &tt.chunk))
		})
	}
}
