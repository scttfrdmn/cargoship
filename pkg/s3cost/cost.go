// Package s3cost turns a measured S3 workload — request counts by operation,
// bytes stored, bytes transferred out — into an itemized dollar cost, using the
// canonical fallback price table (pkg/aws/pricingfallback).
//
// It exists so the benchmark harness can score ANY mover — CargoShip or a
// competitor (aws s3 cp / s5cmd / rclone) — with one consistent, transparent
// cost model. The two charges that actually decide "cheapest in class" are:
//
//   - S3 request charges. PUT/COPY/POST/LIST are the expensive tier and vary by
//     storage class; GET/SELECT/HEAD are cheap; DELETE is free. Uploading a
//     million small files as a million PUTs runs up a real bill that packing
//     into a handful of chunk PUTs avoids — this model makes that visible.
//   - Data movement. S3 ingress (upload) is FREE, so an upload's transfer cost
//     is $0; the billable movement is egress (download/restore) at $/GB, charged
//     on the bytes that physically leave S3 (compressed, for a client-side
//     packer like CargoShip — a real saving over a 1:1 mover).
//
// Monthly storage is reported separately (it recurs; the others are one-time).
package s3cost

import (
	"sort"
	"strings"

	"github.com/scttfrdmn/cargoship/pkg/aws/config"
	"github.com/scttfrdmn/cargoship/pkg/aws/pricingfallback"
)

// EgressUSDPerGB is the S3 → internet data-transfer-out rate. Ingress (upload)
// is free, so only egress (download/restore) is billed here. Cross-region
// replication transfer is out of scope for this model.
const EgressUSDPerGB = 0.09

const bytesPerGB = 1024 * 1024 * 1024

// Usage is a measured (not estimated) workload for one mover on one dataset.
type Usage struct {
	StorageClass config.StorageClass
	// Operations maps an S3 operation name (as the API/CloudWatch reports it —
	// PutObject, UploadPart, CompleteMultipartUpload, GetObject, HeadObject,
	// ListObjectsV2, DeleteObject, …) to how many were issued. Retries should not
	// be included; count logical operations.
	Operations map[string]int64
	// StoredBytes is what S3 holds after the upload — the COMPRESSED size for a
	// client-side packer, the raw size for a 1:1 mover. Drives monthly storage.
	StoredBytes int64
	// EgressBytes is the bytes that leave S3 on download/restore (compressed for a
	// packer). Zero if the workload is upload-only.
	EgressBytes int64
}

// LineItem is one itemized charge. Count is requests (for request tiers) or bytes
// (for transfer/storage); UnitUSD is per-1,000 requests or per-GB respectively.
type LineItem struct {
	Label   string  `json:"label"`
	Count   int64   `json:"count"`
	UnitUSD float64 `json:"unit_usd"`
	USD     float64 `json:"usd"`
}

// Breakdown is the itemized cost of a Usage.
type Breakdown struct {
	// Requests holds one line per billing tier that had operations, most-expensive
	// first. DELETE (free) is included only if any occurred, at $0.
	Requests       []LineItem `json:"requests"`
	DataTransfer   LineItem   `json:"data_transfer"`   // egress; ingress is free
	MonthlyStorage LineItem   `json:"monthly_storage"` // recurring
	// Totals.
	RequestsUSD float64 `json:"requests_usd"` // sum of Requests
	OneTimeUSD  float64 `json:"one_time_usd"` // requests + egress (cost to move + retrieve once)
	MonthlyUSD  float64 `json:"monthly_usd"`  // storage per month
}

// tier maps an S3 operation name to its billing tier ("PUT", "GET", or "DELETE").
// The tier names match pricingfallback.RequestPrice.
func tier(op string) string {
	switch strings.ToUpper(op) {
	case "PUTOBJECT", "CREATEMULTIPARTUPLOAD", "UPLOADPART", "UPLOADPARTCOPY",
		"COMPLETEMULTIPARTUPLOAD", "COPYOBJECT", "POSTOBJECT",
		"LISTOBJECTS", "LISTOBJECTSV2", "LISTMULTIPARTUPLOADS", "LISTPARTS",
		"LISTBUCKETS", "PUTOBJECTTAGGING":
		return "PUT"
	case "GETOBJECT", "GETOBJECTTAGGING", "SELECTOBJECTCONTENT",
		"HEADOBJECT", "HEADBUCKET", "GETOBJECTATTRIBUTES":
		return "GET"
	default:
		// DeleteObject, DeleteObjects, AbortMultipartUpload, and anything
		// unrecognized are treated as the free DELETE tier so an unknown op never
		// silently inflates the bill.
		return "DELETE"
	}
}

// tierLabel is the human label for a billing tier.
func tierLabel(t string) string {
	switch t {
	case "PUT":
		return "PUT/COPY/POST/LIST requests"
	case "GET":
		return "GET/SELECT/HEAD requests"
	default:
		return "DELETE/abort requests (free)"
	}
}

// Compute returns the itemized cost of a measured Usage.
func Compute(u Usage) Breakdown {
	cls := u.StorageClass
	if cls == "" {
		cls = config.StorageClassStandard
	}

	// Aggregate operation counts into billing tiers.
	byTier := map[string]int64{}
	for op, n := range u.Operations {
		byTier[tier(op)] += n
	}

	var b Breakdown
	// Emit tiers most-expensive first for readability: PUT, GET, DELETE.
	for _, t := range []string{"PUT", "GET", "DELETE"} {
		n, ok := byTier[t]
		if !ok || n == 0 {
			continue
		}
		unit := pricingfallback.RequestPrice(t, cls) // USD per 1,000 requests
		cost := (float64(n) / 1000.0) * unit
		b.Requests = append(b.Requests, LineItem{
			Label: tierLabel(t), Count: n, UnitUSD: unit, USD: cost,
		})
		b.RequestsUSD += cost
	}

	egressGB := float64(u.EgressBytes) / bytesPerGB
	b.DataTransfer = LineItem{
		Label: "data transfer out (egress; ingress free)",
		Count: u.EgressBytes, UnitUSD: EgressUSDPerGB, USD: egressGB * EgressUSDPerGB,
	}

	storedGB := float64(u.StoredBytes) / bytesPerGB
	storageUnit := pricingfallback.StoragePrice(cls)
	b.MonthlyStorage = LineItem{
		Label: "storage (per month)",
		Count: u.StoredBytes, UnitUSD: storageUnit, USD: storedGB * storageUnit,
	}

	b.OneTimeUSD = b.RequestsUSD + b.DataTransfer.USD
	b.MonthlyUSD = b.MonthlyStorage.USD
	return b
}

// TotalRequests returns the number of billable + free S3 operations in a Usage —
// the "file operations" count that dominates a many-small-files bill.
func TotalRequests(u Usage) int64 {
	ops := make([]string, 0, len(u.Operations))
	for op := range u.Operations {
		ops = append(ops, op)
	}
	sort.Strings(ops) // deterministic, though the sum is order-independent
	var n int64
	for _, op := range ops {
		n += u.Operations[op]
	}
	return n
}
