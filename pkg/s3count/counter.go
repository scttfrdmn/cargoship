// Package s3count counts the S3 API operations an aws-sdk-go-v2 client issues,
// via a smithy middleware. The benchmark harness uses it to attribute exact
// request counts (PutObject, CreateMultipartUpload, UploadPart, GetObject,
// ListObjectsV2, HeadObject, …) to an upload or restore, so the S3 request bill
// can be computed from measured — not estimated — counts.
package s3count

import (
	"context"
	"sort"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/smithy-go/middleware"
)

// Counter tallies S3 operations by name. Safe for concurrent use (the pipeline
// uploads from many goroutines).
type Counter struct {
	mu  sync.Mutex
	ops map[string]int
}

// New returns an empty Counter.
func New() *Counter {
	return &Counter{ops: make(map[string]int)}
}

func (c *Counter) inc(op string) {
	if op == "" {
		op = "Unknown"
	}
	c.mu.Lock()
	c.ops[op]++
	c.mu.Unlock()
}

// Counts returns a copy of the per-operation tallies.
func (c *Counter) Counts() map[string]int {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[string]int, len(c.ops))
	for k, v := range c.ops {
		out[k] = v
	}
	return out
}

// Total returns the sum of all operation counts.
func (c *Counter) Total() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, v := range c.ops {
		n += v
	}
	return n
}

// Names returns the operation names seen, sorted (stable output).
func (c *Counter) Names() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	names := make([]string, 0, len(c.ops))
	for k := range c.ops {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// Reset clears the tallies (to measure a distinct phase, e.g. restore after
// upload, with the same client).
func (c *Counter) Reset() {
	c.mu.Lock()
	c.ops = make(map[string]int)
	c.mu.Unlock()
}

// Instrument appends a counting middleware to cfg.APIOptions. It increments once
// per logical operation (in the Initialize step, above the retry loop), so
// multipart uploads count each CreateMultipartUpload / UploadPart /
// CompleteMultipartUpload separately, and SDK retries of a single operation are
// not double-counted. Apply before constructing the s3.Client from cfg.
func (c *Counter) Instrument(cfg *aws.Config) {
	cfg.APIOptions = append(cfg.APIOptions, func(stack *middleware.Stack) error {
		return stack.Initialize.Add(
			middleware.InitializeMiddlewareFunc("cargoship-s3count",
				func(ctx context.Context, in middleware.InitializeInput, next middleware.InitializeHandler) (middleware.InitializeOutput, middleware.Metadata, error) {
					c.inc(middleware.GetOperationName(ctx))
					return next.HandleInitialize(ctx, in)
				}),
			middleware.Before,
		)
	})
}
