//go:build integration

package cost

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssdkconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/scttfrdmn/cargoship/pkg/aws/config"
	"github.com/stretchr/testify/require"
)

// These tests exercise the S3-backed budget store against a REAL S3 bucket.
// They self-skip unless CARGOSHIP_ENABLE_S3_INTEGRATION_TESTS=1 and AWS config
// loads. Bucket from CARGOSHIP_TEST_BUCKET, region from AWS_REGION; credentials
// via AWS_PROFILE (local) or the default chain (CI/OIDC). They run in the
// nightly real-AWS lane (.github/workflows/nightly-aws.yml).

func realS3StoreClient(t *testing.T) (*s3.Client, string) {
	t.Helper()
	if os.Getenv("CARGOSHIP_ENABLE_S3_INTEGRATION_TESTS") != "1" {
		t.Skip("Skipping real-AWS budget store test; set CARGOSHIP_ENABLE_S3_INTEGRATION_TESTS=1 to run")
	}
	bucket := os.Getenv("CARGOSHIP_TEST_BUCKET")
	if bucket == "" {
		t.Skip("CARGOSHIP_TEST_BUCKET not set")
	}
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-west-2"
	}
	ctx := context.Background()
	opts := []func(*awssdkconfig.LoadOptions) error{awssdkconfig.WithRegion(region)}
	if profile := os.Getenv("AWS_PROFILE"); profile != "" {
		opts = append(opts, awssdkconfig.WithSharedConfigProfile(profile))
	}
	cfg, err := awssdkconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		t.Skipf("AWS config not available: %v", err)
	}
	return s3.NewFromConfig(cfg), bucket
}

// TestS3Store_TwoConcurrentWritersConverge proves loss-free convergence: two
// independent stores each record a distinct upload for the same project
// concurrently; after both settle, a fresh Load contains BOTH records — the
// If-Match CAS + union-merge handled the 412 conflict without dropping either.
func TestS3Store_TwoConcurrentWritersConverge(t *testing.T) {
	client, bucket := realS3StoreClient(t)
	ctx := context.Background()
	// Unique prefix per run so concurrent CI runs don't collide.
	prefix := fmt.Sprintf(".cargoship-itest/%d/", time.Now().UnixNano())
	t.Cleanup(func() { cleanupPrefix(ctx, client, bucket, prefix) })

	writer := func(jobID string) error {
		s := newS3Store(ctx, client, bucket, prefix, nil)
		_, tok, err := s.Load()
		if err != nil {
			return err
		}
		state := LedgerState{
			Version:        StoreVersion,
			ProjectBudgets: map[string]config.ProjectBudget{"proj": {ProjectID: "proj", MaxBudget: 100}},
			Records: []CostRecord{{
				Timestamp: time.Now().UTC(), ProjectID: "proj",
				JobID: jobID, FileName: jobID + ".dat", Cost: 1.0, SizeGB: 1.0,
			}},
		}
		return s.Save(state, tok)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, id := range []string{"writerA", "writerB"} {
		wg.Add(1)
		go func(jobID string) {
			defer wg.Done()
			errs <- writer(jobID)
		}(id)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err, "concurrent writer should converge via CAS")
	}

	// Fresh load must contain both records.
	s := newS3Store(ctx, client, bucket, prefix, nil)
	got, _, err := s.Load()
	require.NoError(t, err)
	jobs := map[string]bool{}
	for _, r := range got.Records {
		jobs[r.JobID] = true
	}
	require.Truef(t, jobs["writerA"] && jobs["writerB"],
		"both records must survive concurrent writes; got %v", jobs)
}

func cleanupPrefix(ctx context.Context, client *s3.Client, bucket, prefix string) {
	out, err := client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket), Prefix: aws.String(prefix),
	})
	if err != nil {
		return
	}
	for _, obj := range out.Contents {
		_, _ = client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(bucket), Key: obj.Key})
	}
}
