package cmd

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	s3pkg "github.com/scttfrdmn/cargoship/pkg/aws/s3"
)

// frozenS3 reports every key as a 1 GiB GLACIER object with no restore, and
// counts RestoreObject calls.
type frozenS3 struct{ restores int }

func (f *frozenS3) HeadObject(context.Context, *s3.HeadObjectInput, ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	return &s3.HeadObjectOutput{StorageClass: s3types.StorageClassGlacier, ContentLength: aws.Int64(1 << 30)}, nil
}

func (f *frozenS3) RestoreObject(context.Context, *s3.RestoreObjectInput, ...func(*s3.Options)) (*s3.RestoreObjectOutput, error) {
	f.restores++
	return &s3.RestoreObjectOutput{}, nil
}

// TestGlacierPreflight_RequestsNothing pins #973 at the unit level: the
// pre-flight and its cost cap never send RestoreObject; only the explicit
// request step does.
func TestGlacierPreflight_RequestsNothing(t *testing.T) {
	ctx := context.Background()
	keys := []string{"a.tar.zst", "b.tar.zst"} // 2 GiB at $0.01/GB standard = $0.02

	t.Run("under the cap", func(t *testing.T) {
		client := &frozenS3{}
		gr := s3pkg.NewGlacierRestorer(client, 0)
		report, err := glacierPreflight(ctx, gr, "b", keys, s3pkg.RestoreTierStandard, 1.0)
		require.NoError(t, err)
		assert.Zero(t, client.restores)
		assert.Len(t, report.Frozen, 2)

		require.NoError(t, requestGlacierRestores(ctx, gr, "b", report, s3pkg.RestoreTierStandard))
		assert.Equal(t, 2, client.restores)
	})

	t.Run("over the cap", func(t *testing.T) {
		client := &frozenS3{}
		gr := s3pkg.NewGlacierRestorer(client, 0)
		_, err := glacierPreflight(ctx, gr, "b", keys, s3pkg.RestoreTierStandard, 0.01)
		require.ErrorContains(t, err, "exceeds --max-restore-cost")
		assert.Zero(t, client.restores, "an aborted restore must not have requested a retrieval")
	})

	t.Run("nothing frozen requests nothing", func(t *testing.T) {
		client := &frozenS3{}
		gr := s3pkg.NewGlacierRestorer(client, 0)
		require.NoError(t, requestGlacierRestores(ctx, gr, "b", &s3pkg.AccessibilityReport{}, s3pkg.RestoreTierStandard))
		assert.Zero(t, client.restores)
	})
}
