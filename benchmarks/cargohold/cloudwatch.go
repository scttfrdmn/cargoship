package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	cscfg "github.com/scttfrdmn/cargoship/pkg/aws/config"
	"github.com/scttfrdmn/cargoship/pkg/s3cost"
	"github.com/scttfrdmn/cargoship/pkg/s3metrics"
)

// metricsClients bundles the AWS clients the CloudWatch request-count pass needs.
type metricsClients struct {
	s3 *s3.Client
	cw *cloudwatch.Client
}

// resultLabel is the last segment of a tool's S3 prefix (and its metrics-config
// key), matching what each runner writes under: mirror tools use "<tool>-<scenario>",
// CargoHold "cargohold-<strategy>-<scenario>", tar "tar-<scenario>".
func resultLabel(tool, strategy, scenario string) string {
	if tool == "cargohold" {
		return fmt.Sprintf("cargohold-%s-%s", strategy, scenario)
	}
	return fmt.Sprintf("%s-%s", tool, scenario)
}

// metricsFilterID derives a valid CloudWatch metrics-configuration Id (letters,
// numbers, '.', '-', '_'; ≤64 chars) from a label.
func metricsFilterID(label string) string {
	id := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			return r
		default:
			return '-'
		}
	}, label)
	if len(id) > 64 {
		id = id[:64]
	}
	return id
}

// plannedLabels lists the metrics label for every (tool, strategy) that will run.
func plannedLabels(config *BenchmarkConfig) []string {
	var out []string
	for _, tool := range config.Tools {
		if tool == "cargohold" {
			for _, s := range config.ShardStrategies {
				out = append(out, resultLabel("cargohold", s, config.Scenario))
			}
		} else {
			out = append(out, resultLabel(tool, "", config.Scenario))
		}
	}
	return out
}

// enableRequestMetrics builds the AWS clients and installs a prefix-filtered
// CloudWatch request-metrics configuration for every planned tool prefix. The
// first activation on a bucket can take ~15 minutes, so it is done before the
// runs, not lazily.
func enableRequestMetrics(ctx context.Context, config *BenchmarkConfig) (*metricsClients, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}
	mc := &metricsClients{s3: s3.NewFromConfig(cfg), cw: cloudwatch.NewFromConfig(cfg)}

	labels := plannedLabels(config)
	for _, label := range labels {
		prefix := config.Prefix + "/" + label
		if err := s3metrics.Configure(ctx, mc.s3, config.Bucket, metricsFilterID(label), prefix); err != nil {
			return nil, err
		}
	}
	log.Printf("📈 CloudWatch request metrics enabled for %d prefix(es); counts read after runs (may take ~15 min to activate the first time)", len(labels))
	return mc, nil
}

// populateRequestCounts waits for metrics to publish, then reads each result's
// server-side request counts over its run window and prices them with pkg/s3cost.
func populateRequestCounts(ctx context.Context, mc *metricsClients, config *BenchmarkConfig, results []BenchmarkResult) {
	log.Printf("⏳ waiting %s for CloudWatch S3 request metrics to settle...", config.MetricsSettle)
	time.Sleep(config.MetricsSettle)

	for i := range results {
		r := &results[i]
		if r.metricsLabel == "" {
			continue
		}
		// Pad the window: request metrics are 1-minute aggregates and lag the calls.
		start := r.windowStart.Add(-1 * time.Minute)
		end := r.windowEnd.Add(2 * time.Minute)
		counts, err := s3metrics.ReadCounts(ctx, mc.cw, config.Bucket, metricsFilterID(r.metricsLabel), start, end)
		if err != nil {
			log.Printf("   ⚠️  metrics read failed for %s: %v", r.metricsLabel, err)
			continue
		}
		bd := s3cost.Compute(s3cost.Usage{
			StorageClass: cscfg.StorageClassStandard,
			Operations:   counts.Operations(),
		})
		r.RequestCount = int(counts.All)
		r.RequestsUSD = bd.RequestsUSD
		log.Printf("   %s: %d requests (put=%d get=%d list=%d head=%d del=%d) → request cost $%.6f",
			r.metricsLabel, counts.All, counts.Put, counts.Get, counts.List, counts.Head, counts.Delete, bd.RequestsUSD)
	}
}
