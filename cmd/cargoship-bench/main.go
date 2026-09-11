// Command cargoship-bench runs the Phase-1 (CargoShip-only) speed + cost
// benchmark: for each reproducible corpus profile it uploads and restores through
// the real pipeline, measuring throughput and the exact S3 request bill from a
// counting middleware. Point it at real S3 (default credential chain) or at an
// emulator/LocalStack via --endpoint.
//
// It writes objects under --prefix and does NOT delete them — clean the bucket
// yourself afterward (see the real-AWS torture procedure in project docs).
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/scttfrdmn/cargoship/pkg/benchharness"
	"github.com/scttfrdmn/cargoship/pkg/corpus"
	"github.com/scttfrdmn/cargoship/pkg/s3count"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "cargoship-bench:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		profileName = flag.String("profile", "all", "corpus profile to run, or 'all'")
		bucket      = flag.String("bucket", "", "S3 bucket (required)")
		region      = flag.String("region", "us-east-1", "AWS region")
		prefix      = flag.String("prefix", "", "S3 key prefix (default bench-<timestamp>)")
		endpoint    = flag.String("endpoint", "", "S3 endpoint override (emulator/LocalStack); enables path-style")
		mode        = flag.String("mode", "auto", "upload path: auto|direct|packed")
		asJSON      = flag.Bool("json", false, "emit JSON results")
	)
	flag.Parse()

	switch benchharness.Mode(*mode) {
	case benchharness.ModeAuto, benchharness.ModeDirect, benchharness.ModePacked:
	default:
		return fmt.Errorf("invalid --mode %q (want auto|direct|packed)", *mode)
	}

	if *bucket == "" {
		return fmt.Errorf("--bucket is required")
	}
	pfx := *prefix
	if pfx == "" {
		pfx = fmt.Sprintf("bench-%d", time.Now().UnixNano())
	}

	profiles := corpus.Profiles()
	if *profileName != "all" {
		p, ok := corpus.ProfileByName(*profileName)
		if !ok {
			return fmt.Errorf("unknown profile %q (have: %s)", *profileName, profileNames())
		}
		profiles = []corpus.Profile{p}
	}

	ctx := context.Background()
	results := make([]*benchharness.RunResult, 0, len(profiles))
	for _, prof := range profiles {
		client, counter, err := newInstrumentedClient(ctx, *region, *endpoint)
		if err != nil {
			return err
		}
		srcDir, err := os.MkdirTemp("", "cargoship-bench-src-")
		if err != nil {
			return err
		}
		restoreDir, err := os.MkdirTemp("", "cargoship-bench-restore-")
		if err != nil {
			return err
		}
		rr, err := benchharness.Run(ctx, benchharness.Options{
			Profile: prof, Client: client, Counter: counter,
			Bucket: *bucket, Prefix: fmt.Sprintf("%s/%s", pfx, prof.Name), Region: *region,
			SrcDir: srcDir, RestoreDir: restoreDir, Mode: benchharness.Mode(*mode),
		})
		_ = os.RemoveAll(srcDir)
		_ = os.RemoveAll(restoreDir)
		if err != nil {
			return fmt.Errorf("profile %s: %w", prof.Name, err)
		}
		results = append(results, rr)
		if !*asJSON {
			printHuman(rr)
		}
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(results)
	}
	fmt.Fprintf(os.Stderr, "\nobjects left under s3://%s/%s — remember to clean up the bucket.\n", *bucket, pfx)
	return nil
}

func newInstrumentedClient(ctx context.Context, region, endpoint string) (*awss3.Client, *s3count.Counter, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return nil, nil, fmt.Errorf("load AWS config: %w", err)
	}
	counter := s3count.New()
	counter.Instrument(&cfg)
	var opts []func(*awss3.Options)
	if endpoint != "" {
		cfg.BaseEndpoint = aws.String(endpoint)
		opts = append(opts, func(o *awss3.Options) { o.UsePathStyle = true })
	}
	return awss3.NewFromConfig(cfg, opts...), counter, nil
}

func printHuman(rr *benchharness.RunResult) {
	ok := "✅"
	if !rr.ByteIdentical {
		ok = "❌ NOT byte-identical"
	}
	fmt.Printf("── %s [%s] %s\n", rr.Profile, rr.Mode, ok)
	fmt.Printf("   files=%d  source=%s  stored=%s  chunks=%d\n",
		rr.Files, humanBytes(rr.SourceBytes), humanBytes(rr.StoredBytes), rr.Chunks)
	fmt.Printf("   upload : %6.1f MB/s  %v\n", rr.Upload.MBPerSec, rr.Upload.OpCounts)
	fmt.Printf("   restore: %6.1f MB/s  %v\n", rr.Restore.MBPerSec, rr.Restore.OpCounts)
	fmt.Printf("   cost   : upload-requests $%.6f  storage $%.6f/mo  restore(req $%.6f + egress $%.6f)\n",
		rr.Cost.UploadRequestsUSD, rr.Cost.MonthlyStorageUSD, rr.Cost.RestoreRequestsUSD, rr.Cost.RestoreEgressUSD)
}

func humanBytes(b int64) string {
	const u = 1024
	if b < u {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(u), 0
	for n := b / u; n >= u; n /= u {
		div *= u
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

func profileNames() string {
	s := ""
	for i, p := range corpus.Profiles() {
		if i > 0 {
			s += ", "
		}
		s += p.Name
	}
	return s
}
