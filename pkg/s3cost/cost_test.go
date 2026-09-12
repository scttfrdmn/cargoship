package s3cost

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/scttfrdmn/cargoship/pkg/aws/config"
	"github.com/scttfrdmn/cargoship/pkg/aws/pricingfallback"
)

func TestCompute_ItemizesOperationsAndTransfer(t *testing.T) {
	const std = config.StorageClassStandard
	u := Usage{
		StorageClass: std,
		Operations: map[string]int64{
			"PutObject":               1,
			"CreateMultipartUpload":   1,
			"UploadPart":              5,
			"CompleteMultipartUpload": 1, // → 8 PUT-tier
			"ListObjectsV2":           2, // → PUT-tier too (10 total PUT-tier)
			"GetObject":               3, // → GET-tier
			"DeleteObject":            4, // → DELETE (free)
		},
		StoredBytes: 2 * bytesPerGB, // 2 GiB stored
		EgressBytes: 1 * bytesPerGB, // 1 GiB egress
	}
	b := Compute(u)

	putUnit := pricingfallback.RequestPrice("PUT", std) // 0.005 / 1k
	getUnit := pricingfallback.RequestPrice("GET", std) // 0.0004 / 1k
	storeUnit := pricingfallback.StoragePrice(std)      // 0.023 / GB-mo

	// Three tiers present, most-expensive first: PUT(10), GET(3), DELETE(4, free).
	if assert.Len(t, b.Requests, 3) {
		assert.Equal(t, int64(10), b.Requests[0].Count) // PUT tier aggregates the 8 upload ops + 2 LIST
		assert.InDelta(t, 10.0/1000*putUnit, b.Requests[0].USD, 1e-12)
		assert.Equal(t, int64(3), b.Requests[1].Count) // GET
		assert.InDelta(t, 3.0/1000*getUnit, b.Requests[1].USD, 1e-12)
		assert.Equal(t, int64(4), b.Requests[2].Count) // DELETE
		assert.Zero(t, b.Requests[2].USD, "DELETE is free")
	}

	assert.InDelta(t, 10.0/1000*putUnit+3.0/1000*getUnit, b.RequestsUSD, 1e-12)
	assert.InDelta(t, 1.0*EgressUSDPerGB, b.DataTransfer.USD, 1e-12) // 1 GiB egress
	assert.InDelta(t, 2.0*storeUnit, b.MonthlyStorage.USD, 1e-12)    // 2 GiB stored
	assert.InDelta(t, b.RequestsUSD+b.DataTransfer.USD, b.OneTimeUSD, 1e-12)
	assert.InDelta(t, b.MonthlyStorage.USD, b.MonthlyUSD, 1e-12)
	assert.Equal(t, int64(17), TotalRequests(u)) // 1+1+5+1+2+3+4
}

// TestCompute_UploadOnlyHasNoTransferCost guards the "ingress is free" property.
func TestCompute_UploadOnlyHasNoTransferCost(t *testing.T) {
	b := Compute(Usage{
		StorageClass: config.StorageClassStandard,
		Operations:   map[string]int64{"PutObject": 1000},
		StoredBytes:  10 * bytesPerGB,
		// EgressBytes: 0 → upload-only
	})
	assert.Zero(t, b.DataTransfer.USD, "an upload incurs no data-transfer charge (ingress free)")
	assert.InDelta(t, pricingfallback.RequestPrice("PUT", config.StorageClassStandard), b.RequestsUSD, 1e-12) // 1000/1000 * unit
	assert.Positive(t, b.MonthlyUSD)
	assert.InDelta(t, b.RequestsUSD, b.OneTimeUSD, 1e-12)
}

// TestCompute_ManySmallFilesRunUpRequestCost is the headline: N direct PUTs cost
// far more than packing into a few chunk PUTs, at the same stored bytes.
func TestCompute_ManySmallFilesRunUpRequestCost(t *testing.T) {
	stored := int64(1 * bytesPerGB)
	direct := Compute(Usage{StorageClass: config.StorageClassStandard,
		Operations: map[string]int64{"PutObject": 100000}, StoredBytes: stored})
	packed := Compute(Usage{StorageClass: config.StorageClassStandard,
		Operations: map[string]int64{"PutObject": 16, "UploadPart": 4}, StoredBytes: stored})
	assert.Greater(t, direct.OneTimeUSD, packed.OneTimeUSD*100,
		"100k PUTs should dwarf a packed handful — the request-cost story")
}

// TestTier_UnknownOpIsFree ensures an unrecognized op never inflates the bill.
func TestTier_UnknownOpIsFree(t *testing.T) {
	assert.Equal(t, "DELETE", tier("SomeFutureOp"))
	assert.Equal(t, "PUT", tier("uploadpart")) // case-insensitive
	assert.Equal(t, "GET", tier("HeadObject"))
}
