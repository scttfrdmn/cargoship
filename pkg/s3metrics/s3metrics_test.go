package s3metrics

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scttfrdmn/cargoship/pkg/aws/config"
	"github.com/scttfrdmn/cargoship/pkg/s3cost"
)

type fakeBucketMetrics struct {
	in *s3.PutBucketMetricsConfigurationInput
}

func (f *fakeBucketMetrics) PutBucketMetricsConfiguration(_ context.Context, in *s3.PutBucketMetricsConfigurationInput, _ ...func(*s3.Options)) (*s3.PutBucketMetricsConfigurationOutput, error) {
	f.in = in
	return &s3.PutBucketMetricsConfigurationOutput{}, nil
}

func TestConfigure_BuildsPrefixFilter(t *testing.T) {
	f := &fakeBucketMetrics{}
	require.NoError(t, Configure(context.Background(), f, "bkt", "s5cmd-run", "bench/s5cmd/"))

	require.NotNil(t, f.in)
	assert.Equal(t, "bkt", aws.ToString(f.in.Bucket))
	assert.Equal(t, "s5cmd-run", aws.ToString(f.in.Id))
	require.NotNil(t, f.in.MetricsConfiguration)
	assert.Equal(t, "s5cmd-run", aws.ToString(f.in.MetricsConfiguration.Id))
	pf, ok := f.in.MetricsConfiguration.Filter.(*s3types.MetricsFilterMemberPrefix)
	require.True(t, ok, "filter must be a prefix filter")
	assert.Equal(t, "bench/s5cmd/", pf.Value)
}

// fakeMetricData returns queued GetMetricData pages in order, so a test can
// exercise NextToken pagination and cross-period summing.
type fakeMetricData struct {
	pages []*cloudwatch.GetMetricDataOutput
	calls int
}

func (f *fakeMetricData) GetMetricData(_ context.Context, in *cloudwatch.GetMetricDataInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricDataOutput, error) {
	f.calls++
	// The caller must echo the previous page's NextToken.
	page := f.pages[0]
	f.pages = f.pages[1:]
	return page, nil
}

func result(id string, vals ...float64) cwtypes.MetricDataResult {
	return cwtypes.MetricDataResult{Id: aws.String(id), Values: vals}
}

func TestReadCounts_SumsAcrossPeriodsAndPages(t *testing.T) {
	fd := &fakeMetricData{pages: []*cloudwatch.GetMetricDataOutput{
		{ // page 1: Put across two periods, some Get; more Put on the next page
			MetricDataResults: []cwtypes.MetricDataResult{
				result("m0", 3, 2), // Put = 5 so far
				result("m1", 4),    // Get = 4
			},
			NextToken: aws.String("more"),
		},
		{ // page 2: remaining Put + a Delete
			MetricDataResults: []cwtypes.MetricDataResult{
				result("m0", 1), // Put += 1 → 6
				result("m2", 7), // Delete = 7
			},
		},
	}}

	start := time.Now().Add(-10 * time.Minute)
	c, err := ReadCounts(context.Background(), fd, "bkt", "run", start, time.Now())
	require.NoError(t, err)
	assert.Equal(t, 2, fd.calls, "must follow NextToken to the second page")
	assert.Equal(t, int64(6), c.Put, "Put must sum across periods AND pages")
	assert.Equal(t, int64(4), c.Get)
	assert.Equal(t, int64(7), c.Delete)
	assert.Zero(t, c.List, "an absent metric reads as zero")
}

// TestCounts_FeedS3Cost is the point of the package: server-side counts price
// through the same model as CargoShip's client-side counts, on the right tiers.
func TestCounts_FeedS3Cost(t *testing.T) {
	c := Counts{Put: 10, Get: 3, Delete: 4}
	b := s3cost.Compute(s3cost.Usage{
		StorageClass: config.StorageClassStandard,
		Operations:   c.Operations(),
	})
	require.Len(t, b.Requests, 3)
	assert.Equal(t, int64(10), b.Requests[0].Count, "Put → PUT tier")
	assert.Equal(t, int64(3), b.Requests[1].Count, "Get → GET tier")
	assert.Equal(t, int64(4), b.Requests[2].Count)
	assert.Zero(t, b.Requests[2].USD, "Delete is free")
}

func TestQueryIndex(t *testing.T) {
	assert.Equal(t, 0, queryIndex("m0"))
	assert.Equal(t, 12, queryIndex("m12"))
	assert.Equal(t, -1, queryIndex("bogus"))
}
