package cost

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/pricing"
	pricingtypes "github.com/aws/aws-sdk-go-v2/service/pricing/types"
	"github.com/scttfrdmn/cargoship/pkg/aws/config"
)

// pricingAPIClient is the subset of the AWS Pricing client used here, extracted
// as an interface so queryAWSPrice can be unit-tested with a fake. *pricing.Client
// satisfies it.
type pricingAPIClient interface {
	GetProducts(ctx context.Context, params *pricing.GetProductsInput, optFns ...func(*pricing.Options)) (*pricing.GetProductsOutput, error)
}

// priceListItem is the subset of an AWS Price List API product JSON document we
// need to extract an on-demand USD price. The GetProducts response returns each
// product as a JSON string in this shape (FormatVersion aws_v1).
type priceListItem struct {
	Product struct {
		Attributes map[string]string `json:"attributes"`
	} `json:"product"`
	Terms struct {
		OnDemand map[string]struct {
			PriceDimensions map[string]struct {
				BeginRange   string `json:"beginRange"`
				Unit         string `json:"unit"`
				PricePerUnit struct {
					USD string `json:"USD"`
				} `json:"pricePerUnit"`
			} `json:"priceDimensions"`
		} `json:"OnDemand"`
	} `json:"terms"`
}

// s3StorageVolumeType maps a CargoShip storage class to the AWS Price List
// `volumeType` attribute for the S3 Storage product family. Values verified
// against the live Pricing API (us-east-1).
func s3StorageVolumeType(storageClass config.StorageClass) (string, bool) {
	switch storageClass {
	case config.StorageClassStandard:
		return "Standard", true
	case config.StorageClassStandardIA:
		return "Standard - Infrequent Access", true
	case config.StorageClassOneZoneIA:
		return "One Zone - Infrequent Access", true
	case config.StorageClassIntelligentTiering:
		return "Intelligent-Tiering Frequent Access", true
	case config.StorageClassGlacier:
		return "Glacier Instant Retrieval", true
	case config.StorageClassDeepArchive:
		return "Glacier Deep Archive", true
	default:
		return "", false
	}
}

// s3RequestGroup maps a request type to the AWS Price List `group` attribute for
// the S3 "API Request" product family. PUT/COPY/POST/LIST are Tier1; GET/SELECT
// and others are Tier2. Verified against the live Pricing API.
func s3RequestGroup(requestType string) (string, bool) {
	switch requestType {
	case "PUT", "POST", "COPY", "LIST":
		return "S3-API-Tier1", true
	case "GET", "SELECT":
		return "S3-API-Tier2", true
	default:
		return "", false
	}
}

// queryAWSPrice runs a GetProducts query with the given S3 attribute filters and
// returns the lowest first-tier on-demand USD price found (per the product's
// unit — GB-Mo for storage, per-request for requests). It returns an error when
// the API call fails or no usable price is found, so callers fall back to the
// static price table.
func (pm *PricingManager) queryAWSPrice(ctx context.Context, region string, filters map[string]string) (float64, error) {
	apiFilters := make([]pricingtypes.Filter, 0, len(filters)+1)
	apiFilters = append(apiFilters, pricingtypes.Filter{
		Type:  pricingtypes.FilterTypeTermMatch,
		Field: aws.String("regionCode"),
		Value: aws.String(region),
	})
	// Deterministic filter order keeps the cache key and requests stable.
	keys := make([]string, 0, len(filters))
	for k := range filters {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		apiFilters = append(apiFilters, pricingtypes.Filter{
			Type:  pricingtypes.FilterTypeTermMatch,
			Field: aws.String(k),
			Value: aws.String(filters[k]),
		})
	}

	out, err := pm.pricingAPI.GetProducts(ctx, &pricing.GetProductsInput{
		ServiceCode:   aws.String("AmazonS3"),
		FormatVersion: aws.String("aws_v1"),
		Filters:       apiFilters,
		MaxResults:    aws.Int32(100),
	})
	if err != nil {
		return 0, fmt.Errorf("pricing GetProducts: %w", err)
	}

	best := -1.0
	for _, raw := range out.PriceList {
		var item priceListItem
		if err := json.Unmarshal([]byte(raw), &item); err != nil {
			continue // skip malformed entries rather than failing the whole query
		}
		for _, term := range item.Terms.OnDemand {
			// Pick the first pricing band (lowest beginRange) for this product.
			type dim struct {
				begin float64
				usd   string
			}
			var dims []dim
			for _, pd := range term.PriceDimensions {
				begin, _ := strconv.ParseFloat(pd.BeginRange, 64)
				dims = append(dims, dim{begin: begin, usd: pd.PricePerUnit.USD})
			}
			if len(dims) == 0 {
				continue
			}
			sort.Slice(dims, func(i, j int) bool { return dims[i].begin < dims[j].begin })
			price, err := strconv.ParseFloat(dims[0].usd, 64)
			if err != nil || price <= 0 {
				continue
			}
			if best < 0 || price < best {
				best = price
			}
		}
	}

	if best < 0 {
		return 0, fmt.Errorf("no usable price in %d products", len(out.PriceList))
	}
	return best, nil
}
