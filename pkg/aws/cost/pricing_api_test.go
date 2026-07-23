package cost

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/pricing"
	"github.com/scttfrdmn/cargoship/pkg/aws/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakePricingClient is a pricingAPIClient that returns canned Price List
// products (or an error) without touching AWS.
type fakePricingClient struct {
	products []string
	err      error
}

func (f *fakePricingClient) GetProducts(_ context.Context, _ *pricing.GetProductsInput, _ ...func(*pricing.Options)) (*pricing.GetProductsOutput, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &pricing.GetProductsOutput{PriceList: f.products}, nil
}

// newTestPricingManager builds a PricingManager wired to a fake pricing client
// with the API enabled.
func newTestPricingManager(t *testing.T, client pricingAPIClient) *PricingManager {
	t.Helper()
	pm, err := NewPricingManager(&config.PricingConfig{
		UseAWSPricingAPI:     true,
		Currency:             "USD",
		PricingCacheDuration: "24h",
	}, aws.Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	pm.pricingAPI = client
	return pm
}

func storageProduct(volumeType, usd string) string {
	return fmt.Sprintf(`{"product":{"attributes":{"volumeType":%q,"regionCode":"us-east-1"}},`+
		`"terms":{"OnDemand":{"K.T":{"priceDimensions":{"K.T.D":{"beginRange":"0","unit":"GB-Mo","pricePerUnit":{"USD":%q}}}}}}}`,
		volumeType, usd)
}

func requestProduct(group, usdPerRequest string) string {
	return fmt.Sprintf(`{"product":{"attributes":{"group":%q,"regionCode":"us-east-1"}},`+
		`"terms":{"OnDemand":{"K.T":{"priceDimensions":{"K.T.D":{"beginRange":"0","unit":"Requests","pricePerUnit":{"USD":%q}}}}}}}`,
		group, usdPerRequest)
}

func TestGetAWSStoragePrice_FromAPI(t *testing.T) {
	pm := newTestPricingManager(t, &fakePricingClient{
		products: []string{storageProduct("Standard", "0.0230000000")},
	})
	price, err := pm.getAWSStoragePrice(context.Background(), config.StorageClassStandard, "us-east-1")
	require.NoError(t, err)
	assert.InDelta(t, 0.023, price, 1e-9)
}

func TestGetAWSStoragePrice_UnmappedClass(t *testing.T) {
	pm := newTestPricingManager(t, &fakePricingClient{})
	_, err := pm.getAWSStoragePrice(context.Background(), config.StorageClass("BOGUS"), "us-east-1")
	assert.Error(t, err)
}

func TestGetAWSRequestPrice_ScalesToPerThousand(t *testing.T) {
	pm := newTestPricingManager(t, &fakePricingClient{
		products: []string{requestProduct("S3-API-Tier1", "0.0000050000")},
	})
	// $0.000005 per request → $0.005 per 1,000.
	price, err := pm.getAWSRequestPrice(context.Background(), "PUT", config.StorageClassStandard, "us-east-1")
	require.NoError(t, err)
	assert.InDelta(t, 0.005, price, 1e-9)
}

func TestGetAWSRequestPrice_UsesClassGroup(t *testing.T) {
	// A Glacier PUT must query the S3-API-GIR-Tier1 group, so a fake returning
	// only that product resolves; a Standard query against the same fake would
	// too (the fake ignores filters), so assert the price flows through.
	pm := newTestPricingManager(t, &fakePricingClient{
		products: []string{requestProduct("S3-API-GIR-Tier1", "0.0000200000")},
	})
	price, err := pm.getAWSRequestPrice(context.Background(), "PUT", config.StorageClassGlacier, "us-east-1")
	require.NoError(t, err)
	assert.InDelta(t, 0.02, price, 1e-9) // $0.00002/request → $0.02/1,000
}

func TestGetAWSRequestPrice_UnmappedType(t *testing.T) {
	pm := newTestPricingManager(t, &fakePricingClient{})
	_, err := pm.getAWSRequestPrice(context.Background(), "DELETE", config.StorageClassStandard, "us-east-1")
	assert.Error(t, err)
}

func TestQueryAWSPrice_NoUsablePrice(t *testing.T) {
	pm := newTestPricingManager(t, &fakePricingClient{products: []string{`{"bogus":true}`}})
	_, err := pm.queryAWSPrice(context.Background(), "us-east-1", map[string]string{"productFamily": "Storage"})
	assert.Error(t, err)
}

func TestQueryAWSPrice_APIError(t *testing.T) {
	pm := newTestPricingManager(t, &fakePricingClient{err: fmt.Errorf("boom")})
	_, err := pm.queryAWSPrice(context.Background(), "us-east-1", map[string]string{"productFamily": "Storage"})
	assert.Error(t, err)
}

// TestGetRequestPrice_FallsBackOnAPIError confirms getRequestPrice returns the
// static fallback (not an error) when the API call fails, so cost estimation
// keeps working offline.
func TestGetRequestPrice_FallsBackOnAPIError(t *testing.T) {
	pm := newTestPricingManager(t, &fakePricingClient{err: fmt.Errorf("no network")})
	price, err := pm.getRequestPrice(context.Background(), "PUT", config.StorageClassStandard, "us-east-1")
	require.NoError(t, err)
	assert.InDelta(t, 0.005, price, 1e-9) // corrected fallback
}

func TestGetRequestPrice_UsesAPIWhenAvailable(t *testing.T) {
	pm := newTestPricingManager(t, &fakePricingClient{
		products: []string{requestProduct("S3-API-Tier2", "0.0000004000")},
	})
	price, err := pm.getRequestPrice(context.Background(), "GET", config.StorageClassStandard, "eu-west-1")
	require.NoError(t, err)
	assert.InDelta(t, 0.0004, price, 1e-9)
}

func TestS3StorageVolumeType(t *testing.T) {
	cases := map[config.StorageClass]string{
		config.StorageClassStandard:           "Standard",
		config.StorageClassStandardIA:         "Standard - Infrequent Access",
		config.StorageClassOneZoneIA:          "One Zone - Infrequent Access",
		config.StorageClassIntelligentTiering: "Intelligent-Tiering Frequent Access",
		config.StorageClassGlacier:            "Glacier Instant Retrieval",
		config.StorageClassDeepArchive:        "Glacier Deep Archive",
	}
	for sc, want := range cases {
		got, ok := s3StorageVolumeType(sc)
		assert.True(t, ok, "storage class %q should map", sc)
		assert.Equal(t, want, got, "storage class %q", sc)
	}

	_, ok := s3StorageVolumeType(config.StorageClass("BOGUS"))
	assert.False(t, ok, "unknown storage class must not map")
}

func TestS3RequestGroup(t *testing.T) {
	// Standard (and unmodeled classes) use the base group; verb selects the tier.
	tier1 := []string{"PUT", "POST", "COPY", "LIST"}
	for _, rt := range tier1 {
		got, ok := s3RequestGroup(rt, config.StorageClassStandard)
		assert.True(t, ok, "%s should map", rt)
		assert.Equal(t, "S3-API-Tier1", got, "%s", rt)
	}
	tier2 := []string{"GET", "SELECT"}
	for _, rt := range tier2 {
		got, ok := s3RequestGroup(rt, config.StorageClassStandard)
		assert.True(t, ok, "%s should map", rt)
		assert.Equal(t, "S3-API-Tier2", got, "%s", rt)
	}
	_, ok := s3RequestGroup("DELETE", config.StorageClassStandard)
	assert.False(t, ok, "DELETE has no Tier group (free)")

	// Storage class selects the per-class request group (#252). Verified against
	// the live Pricing API.
	classCases := []struct {
		sc      config.StorageClass
		wantPut string
		wantGet string
	}{
		{config.StorageClassStandard, "S3-API-Tier1", "S3-API-Tier2"},
		{config.StorageClassStandardIA, "S3-API-SIA-Tier1", "S3-API-SIA-Tier2"},
		{config.StorageClassOneZoneIA, "S3-API-ZIA-Tier1", "S3-API-ZIA-Tier2"},
		{config.StorageClassGlacier, "S3-API-GIR-Tier1", "S3-API-GIR-Tier2"},
		// Intelligent-Tiering and Deep Archive have no distinct request group;
		// AWS prices them at the Standard rate.
		{config.StorageClassIntelligentTiering, "S3-API-Tier1", "S3-API-Tier2"},
		{config.StorageClassDeepArchive, "S3-API-Tier1", "S3-API-Tier2"},
	}
	for _, c := range classCases {
		put, ok := s3RequestGroup("PUT", c.sc)
		assert.True(t, ok)
		assert.Equal(t, c.wantPut, put, "PUT group for %s", c.sc)
		get, ok := s3RequestGroup("GET", c.sc)
		assert.True(t, ok)
		assert.Equal(t, c.wantGet, get, "GET group for %s", c.sc)
	}
}

// TestPriceListItem_Parse verifies the Price List product JSON is parsed the way
// the AWS Pricing API actually returns it (shape confirmed against the live API).
func TestPriceListItem_Parse(t *testing.T) {
	// Trimmed real-world Standard storage product (us-east-1), with two pricing
	// bands to confirm we can read the first-band ($0.023/GB-Mo) price.
	raw := `{
      "product": {"attributes": {"volumeType": "Standard", "regionCode": "us-east-1"}},
      "terms": {"OnDemand": {"ABC.JRTCKXETXF": {"priceDimensions": {
        "ABC.JRTCKXETXF.D1": {"beginRange": "0", "unit": "GB-Mo", "pricePerUnit": {"USD": "0.0230000000"}},
        "ABC.JRTCKXETXF.D2": {"beginRange": "512000", "unit": "GB-Mo", "pricePerUnit": {"USD": "0.0220000000"}}
      }}}}
    }`

	var item priceListItem
	require.NoError(t, json.Unmarshal([]byte(raw), &item))
	assert.Equal(t, "Standard", item.Product.Attributes["volumeType"])

	term, ok := item.Terms.OnDemand["ABC.JRTCKXETXF"]
	require.True(t, ok)
	require.Len(t, term.PriceDimensions, 2)
	assert.Equal(t, "0.0230000000", term.PriceDimensions["ABC.JRTCKXETXF.D1"].PricePerUnit.USD)
}
