// Package pricingfallback holds the single canonical table of S3 fallback
// prices used when the live AWS Price List API is unavailable or disabled.
//
// It exists to resolve #237: CargoShip previously had three independent fallback
// tables (in pkg/aws/cost, pkg/aws/costs, and pkg/aws/pricing) that computed the
// same prices from separate code and drifted — including two separate instances
// of the 10x PUT-price error (#233). All pricing stacks now resolve fallback
// prices through this leaf package, so each number is defined exactly once.
//
// This package depends only on pkg/aws/config (a leaf), so every pricing stack
// can import it without creating a cycle. Numbers are approximate AWS us-east-1
// list prices; the live Price List API overrides them when enabled.
package pricingfallback

import (
	"strings"

	"github.com/scttfrdmn/cargoship/pkg/aws/config"
)

// StoragePrice returns the storage price in USD per GB-month for a storage class.
func StoragePrice(storageClass config.StorageClass) float64 {
	switch storageClass {
	case config.StorageClassStandard:
		return 0.023
	case config.StorageClassStandardIA:
		return 0.0125
	case config.StorageClassOneZoneIA:
		return 0.01
	case config.StorageClassIntelligentTiering:
		// Intelligent-Tiering frequent-access tier is priced at Standard; the
		// small monitoring/automation fee is not modeled here. (Reconciled to
		// 0.0225 across the former stacks — #237.)
		return 0.0225
	case config.StorageClassGlacier:
		return 0.004
	case config.StorageClassDeepArchive:
		return 0.00099
	default:
		return 0.023
	}
}

// RequestPrice returns the request price in USD per 1,000 requests for a verb
// and storage class.
//
// PUT/COPY/POST/LIST prices vary by storage class — writing to archival classes
// costs far more per request than Standard. GET/SELECT and DELETE do not vary by
// class (DELETE is free). This models real S3 pricing; pricing a PUT to Glacier
// at the Standard rate (the old pkg/aws/cost behavior) undercounted archival
// upload cost, and pkg/aws/pricing carried a 10x-too-low copy of this table
// entirely (#237).
func RequestPrice(requestType string, storageClass config.StorageClass) float64 {
	switch strings.ToUpper(requestType) {
	case "PUT", "POST", "COPY", "LIST":
		return PutRequestPrice(storageClass)
	case "GET", "SELECT":
		return 0.0004 // $0.0004 per 1,000 GET/SELECT requests (all classes)
	case "DELETE":
		return 0.0 // DELETE requests are free
	default:
		return PutRequestPrice(storageClass)
	}
}

// PutRequestPrice returns the per-1,000 PUT-class request price for a storage
// class (the tier shared by PUT/COPY/POST/LIST).
func PutRequestPrice(storageClass config.StorageClass) float64 {
	switch storageClass {
	case config.StorageClassStandard:
		return 0.005 // $0.005 per 1,000 (the #233 fix)
	case config.StorageClassStandardIA:
		return 0.01
	case config.StorageClassOneZoneIA:
		return 0.01
	case config.StorageClassIntelligentTiering:
		return 0.005
	case config.StorageClassGlacier:
		return 0.03
	case config.StorageClassDeepArchive:
		return 0.05
	default:
		return 0.005
	}
}

// StoragePriceTable returns the full storage fallback table as a fresh map, for
// callers (e.g. the live-pricing service's cached PriceData) that populate a map
// keyed by storage class.
func StoragePriceTable() map[config.StorageClass]float64 {
	classes := allClasses()
	m := make(map[config.StorageClass]float64, len(classes))
	for _, c := range classes {
		m[c] = StoragePrice(c)
	}
	return m
}

// PutRequestPriceTable returns the full PUT-class request fallback table as a
// fresh map keyed by storage class.
func PutRequestPriceTable() map[config.StorageClass]float64 {
	classes := allClasses()
	m := make(map[config.StorageClass]float64, len(classes))
	for _, c := range classes {
		m[c] = PutRequestPrice(c)
	}
	return m
}

func allClasses() []config.StorageClass {
	return []config.StorageClass{
		config.StorageClassStandard,
		config.StorageClassStandardIA,
		config.StorageClassOneZoneIA,
		config.StorageClassIntelligentTiering,
		config.StorageClassGlacier,
		config.StorageClassDeepArchive,
	}
}
