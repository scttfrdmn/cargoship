// Package s3metrics reads a mover's SERVER-SIDE S3 request counts from CloudWatch
// S3 request metrics, so the benchmark harness can score any black-box competitor
// (aws s3 cp / s5cmd / rclone / mc) with the same pkg/s3cost model as CargoShip —
// whose counts come from client-side middleware (pkg/s3count) that a competitor
// binary doesn't expose.
//
// Request metrics are a bucket feature: PutBucketMetricsConfiguration with a
// prefix filter tells S3 to publish per-request CloudWatch metrics (AllRequests,
// PutRequests, GetRequests, …) under the AWS/S3 namespace, tagged with the filter
// Id. Two real-world latencies apply: enabling a NEW configuration takes up to
// ~15 minutes to start filtering, and published metrics lag the requests by a few
// minutes. So a caller enables the config ONCE per bucket and reads counts over
// the run window after a settling delay — this package itself never sleeps.
//
// CloudWatch cannot be emulated (rpcv2cbor), so live use is real-AWS-only; the
// unit tests cover query construction and response parsing via the two client
// interfaces below.
package s3metrics

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// namespace is where S3 publishes request metrics.
const namespace = "AWS/S3"

// BucketMetricsAPI is the S3 surface needed to enable request metrics on a bucket.
type BucketMetricsAPI interface {
	PutBucketMetricsConfiguration(ctx context.Context, params *s3.PutBucketMetricsConfigurationInput, optFns ...func(*s3.Options)) (*s3.PutBucketMetricsConfigurationOutput, error)
}

// MetricDataAPI is the CloudWatch surface needed to read the published counts.
type MetricDataAPI interface {
	GetMetricData(ctx context.Context, params *cloudwatch.GetMetricDataInput, optFns ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricDataOutput, error)
}

// Counts is the server-side request tally for one prefix over one time window.
type Counts struct {
	Put    int64 `json:"put"`
	Get    int64 `json:"get"`
	Delete int64 `json:"delete"`
	Head   int64 `json:"head"`
	List   int64 `json:"list"`
	Post   int64 `json:"post"`
	All    int64 `json:"all"`

	BytesDownloaded int64 `json:"bytes_downloaded"`
	BytesUploaded   int64 `json:"bytes_uploaded"`
}

// requestMetrics are the AWS/S3 request metrics read here, each wired to the field
// it populates. Order defines the GetMetricData query id (m0, m1, …).
var requestMetrics = []struct {
	name string
	set  func(*Counts, int64)
}{
	{"PutRequests", func(c *Counts, v int64) { c.Put = v }},
	{"GetRequests", func(c *Counts, v int64) { c.Get = v }},
	{"DeleteRequests", func(c *Counts, v int64) { c.Delete = v }},
	{"HeadRequests", func(c *Counts, v int64) { c.Head = v }},
	{"ListRequests", func(c *Counts, v int64) { c.List = v }},
	{"PostRequests", func(c *Counts, v int64) { c.Post = v }},
	{"AllRequests", func(c *Counts, v int64) { c.All = v }},
	{"BytesDownloaded", func(c *Counts, v int64) { c.BytesDownloaded = v }},
	{"BytesUploaded", func(c *Counts, v int64) { c.BytesUploaded = v }},
}

// Operations maps the server-side counts onto representative S3 operation names,
// keyed so pkg/s3cost's tier classifier bills them correctly (Put/Post/List →
// PUT tier, Get/Head → GET tier, Delete → free). Feed the result straight into
// s3cost.Usage.Operations to price a competitor's run.
func (c Counts) Operations() map[string]int64 {
	return map[string]int64{
		"PutObject":     c.Put,
		"PostObject":    c.Post,
		"ListObjectsV2": c.List,
		"GetObject":     c.Get,
		"HeadObject":    c.Head,
		"DeleteObject":  c.Delete,
	}
}

// Configure enables (idempotently) a prefix-filtered request-metrics configuration
// named filterID on bucket, so S3 publishes CloudWatch metrics for objects under
// prefix. Safe to call before every run; it takes up to ~15 minutes to activate
// the FIRST time on a bucket.
func Configure(ctx context.Context, api BucketMetricsAPI, bucket, filterID, prefix string) error {
	_, err := api.PutBucketMetricsConfiguration(ctx, &s3.PutBucketMetricsConfigurationInput{
		Bucket: aws.String(bucket),
		Id:     aws.String(filterID),
		MetricsConfiguration: &s3types.MetricsConfiguration{
			Id:     aws.String(filterID),
			Filter: &s3types.MetricsFilterMemberPrefix{Value: prefix},
		},
	})
	if err != nil {
		return fmt.Errorf("put bucket metrics configuration %q: %w", filterID, err)
	}
	return nil
}

// ReadCounts sums each request metric over [start, end) for the filterID's prefix.
// Period is 60s (S3 request-metric granularity); values are summed across periods
// and across GetMetricData pages. A metric with no data reads as zero.
func ReadCounts(ctx context.Context, api MetricDataAPI, bucket, filterID string, start, end time.Time) (Counts, error) {
	dims := []cwtypes.Dimension{
		{Name: aws.String("BucketName"), Value: aws.String(bucket)},
		{Name: aws.String("FilterId"), Value: aws.String(filterID)},
	}
	queries := make([]cwtypes.MetricDataQuery, len(requestMetrics))
	for i, m := range requestMetrics {
		queries[i] = cwtypes.MetricDataQuery{
			Id: aws.String(fmt.Sprintf("m%d", i)),
			MetricStat: &cwtypes.MetricStat{
				Metric: &cwtypes.Metric{
					Namespace:  aws.String(namespace),
					MetricName: aws.String(m.name),
					Dimensions: dims,
				},
				Period: aws.Int32(60),
				Stat:   aws.String("Sum"),
			},
		}
	}

	sums := make([]float64, len(requestMetrics))
	var token *string
	for {
		out, err := api.GetMetricData(ctx, &cloudwatch.GetMetricDataInput{
			StartTime:         aws.Time(start),
			EndTime:           aws.Time(end),
			MetricDataQueries: queries,
			NextToken:         token,
		})
		if err != nil {
			return Counts{}, fmt.Errorf("get metric data: %w", err)
		}
		for _, r := range out.MetricDataResults {
			idx := queryIndex(aws.ToString(r.Id))
			if idx < 0 || idx >= len(sums) {
				continue
			}
			for _, v := range r.Values {
				sums[idx] += v
			}
		}
		if out.NextToken == nil {
			break
		}
		token = out.NextToken
	}

	var counts Counts
	for i, m := range requestMetrics {
		m.set(&counts, int64(sums[i]))
	}
	return counts, nil
}

// queryIndex parses a "m<idx>" query id back to its requestMetrics index, or -1.
func queryIndex(id string) int {
	n, err := strconv.Atoi(strings.TrimPrefix(id, "m"))
	if err != nil {
		return -1
	}
	return n
}
