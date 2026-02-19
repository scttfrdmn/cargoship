package multiregion

import (
	"context"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// haversineKm
// ---------------------------------------------------------------------------

func TestHaversineKm_SamePoint(t *testing.T) {
	dist := haversineKm(51.5, -0.1, 51.5, -0.1)
	assert.InDelta(t, 0.0, dist, 0.01)
}

func TestHaversineKm_LondonToNewYork(t *testing.T) {
	// London (51.5, -0.1) → New York (40.7, -74.0) ≈ 5,570 km
	dist := haversineKm(51.5, -0.1, 40.7, -74.0)
	assert.InDelta(t, 5570, dist, 100)
}

func TestHaversineKm_Antipodal(t *testing.T) {
	// Antipodal points ≈ half Earth circumference ≈ 20,015 km
	dist := haversineKm(0, 0, 0, 180)
	assert.InDelta(t, 20015, dist, 100)
}

// ---------------------------------------------------------------------------
// regionDistance
// ---------------------------------------------------------------------------

func TestRegionDistance_KnownRegion(t *testing.T) {
	// Distance from NYC (40.7, -74.0) to us-east-1 (39.0, -77.5) ≈ 350 km
	dist := regionDistance("us-east-1", 40.7, -74.0)
	assert.Less(t, dist, 500.0)
	assert.Greater(t, dist, 0.0)
}

func TestRegionDistance_UnknownRegion(t *testing.T) {
	dist := regionDistance("xx-unknown-99", 0, 0)
	assert.Equal(t, math.MaxFloat64, dist)
}

func TestRegionDistance_ClosestToTokyo(t *testing.T) {
	// From Tokyo (35.7, 139.7) the closest AWS region should be ap-northeast-1
	tokyoLat, tokyoLon := 35.7, 139.7
	candidates := []string{"us-east-1", "eu-west-1", "ap-northeast-1", "ap-southeast-1"}
	closest := candidates[0]
	minDist := math.MaxFloat64
	for _, r := range candidates {
		d := regionDistance(r, tokyoLat, tokyoLon)
		if d < minDist {
			minDist = d
			closest = r
		}
	}
	assert.Equal(t, "ap-northeast-1", closest)
}

// ---------------------------------------------------------------------------
// isRegionInResidencyZone
// ---------------------------------------------------------------------------

func TestIsRegionInResidencyZone_EmptyZone(t *testing.T) {
	assert.True(t, isRegionInResidencyZone("", "us-east-1"))
	assert.True(t, isRegionInResidencyZone("", "ap-southeast-1"))
}

func TestIsRegionInResidencyZone_EU(t *testing.T) {
	assert.True(t, isRegionInResidencyZone("EU", "eu-west-1"))
	assert.True(t, isRegionInResidencyZone("EU", "eu-central-1"))
	assert.False(t, isRegionInResidencyZone("EU", "us-east-1"))
	assert.False(t, isRegionInResidencyZone("EU", "ap-northeast-1"))
}

func TestIsRegionInResidencyZone_US(t *testing.T) {
	assert.True(t, isRegionInResidencyZone("US", "us-west-2"))
	assert.False(t, isRegionInResidencyZone("US", "eu-west-1"))
}

func TestIsRegionInResidencyZone_UnknownZone(t *testing.T) {
	assert.False(t, isRegionInResidencyZone("MARS", "us-east-1"))
}

// ---------------------------------------------------------------------------
// parseLatLon
// ---------------------------------------------------------------------------

func TestParseLatLon_Valid(t *testing.T) {
	lat, lon, err := parseLatLon("37.7749,-122.4194")
	require.NoError(t, err)
	assert.InDelta(t, 37.7749, lat, 0.0001)
	assert.InDelta(t, -122.4194, lon, 0.0001)
}

func TestParseLatLon_WithSpaces(t *testing.T) {
	lat, lon, err := parseLatLon("  51.5 , -0.1 ")
	require.NoError(t, err)
	assert.InDelta(t, 51.5, lat, 0.001)
	assert.InDelta(t, -0.1, lon, 0.001)
}

func TestParseLatLon_Invalid(t *testing.T) {
	_, _, err := parseLatLon("not-a-location")
	assert.Error(t, err)
}

func TestParseLatLon_MissingPart(t *testing.T) {
	_, _, err := parseLatLon("37.7749")
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// geoLocationCache
// ---------------------------------------------------------------------------

// staticGeoLocator returns a fixed location for testing.
type staticGeoLocator struct {
	loc   *ClientLocation
	calls int
	mu    sync.Mutex
}

func (s *staticGeoLocator) Locate(_ context.Context) *ClientLocation {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	return s.loc
}

func TestGeoLocationCache_ReturnsCachedResult(t *testing.T) {
	loc := &ClientLocation{Lat: 37.7, Lon: -122.4, CountryCode: "US"}
	locator := &staticGeoLocator{loc: loc}
	cache := newGeoLocationCache(locator, 1*time.Minute)

	got1 := cache.Get(context.Background())
	got2 := cache.Get(context.Background())

	require.NotNil(t, got1)
	assert.Equal(t, loc, got1)
	assert.Equal(t, got1, got2)
	// Second call should be served from cache — locator called only once.
	locator.mu.Lock()
	assert.Equal(t, 1, locator.calls)
	locator.mu.Unlock()
}

func TestGeoLocationCache_RefreshesAfterExpiry(t *testing.T) {
	loc := &ClientLocation{Lat: 1.0, Lon: 2.0}
	locator := &staticGeoLocator{loc: loc}
	cache := newGeoLocationCache(locator, 1*time.Millisecond)

	cache.Get(context.Background())
	time.Sleep(5 * time.Millisecond) // expire the cache
	cache.Get(context.Background())

	locator.mu.Lock()
	assert.Equal(t, 2, locator.calls, "should have fetched twice after expiry")
	locator.mu.Unlock()
}

func TestGeoLocationCache_NilLocatorResult(t *testing.T) {
	locator := &staticGeoLocator{loc: nil} // simulates geolocation failure
	cache := newGeoLocationCache(locator, 1*time.Minute)

	got := cache.Get(context.Background())
	assert.Nil(t, got)
}

func TestGeoLocationCache_ConcurrentAccess(t *testing.T) {
	loc := &ClientLocation{Lat: 48.9, Lon: 2.4, CountryCode: "FR"}
	locator := &staticGeoLocator{loc: loc}
	cache := newGeoLocationCache(locator, 1*time.Minute)

	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got := cache.Get(context.Background())
			assert.Equal(t, loc, got)
		}()
	}
	wg.Wait()
}

// ---------------------------------------------------------------------------
// selectByGeography — integration with DefaultRegionSelector
// ---------------------------------------------------------------------------

// buildGeoConfig creates a MultiRegionConfig with regions in EU and US zones.
func buildGeoConfig() *MultiRegionConfig {
	return &MultiRegionConfig{
		Enabled: true,
		Regions: []Region{
			{Name: "us-east-1", Status: RegionStatusHealthy, Priority: 1, Weight: 50},
			{Name: "us-west-2", Status: RegionStatusHealthy, Priority: 2, Weight: 50},
			{Name: "eu-west-1", Status: RegionStatusHealthy, Priority: 3, Weight: 50},
			{Name: "eu-central-1", Status: RegionStatusHealthy, Priority: 4, Weight: 50},
			{Name: "ap-northeast-1", Status: RegionStatusHealthy, Priority: 5, Weight: 50},
		},
		LoadBalancing: LoadBalancingConfig{Strategy: LoadBalancingGeographic},
	}
}

func TestSelectByGeography_ExplicitHint_PicksClosest(t *testing.T) {
	cfg := buildGeoConfig()
	// Use a nil locator — explicit hint should be used without hitting it.
	sel := NewRegionSelectorWithGeoLocator(cfg, log.New(nil), &staticGeoLocator{loc: nil})

	// Client in Tokyo → ap-northeast-1 should win.
	req := &UploadRequest{
		Context: context.Background(),
		Metadata: map[string]string{
			"client_location": "35.7,139.7", // Tokyo
		},
	}
	region, err := sel.SelectRegion(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "ap-northeast-1", region.Name)
}

func TestSelectByGeography_ExplicitHint_USCoast(t *testing.T) {
	cfg := buildGeoConfig()
	sel := NewRegionSelectorWithGeoLocator(cfg, log.New(nil), &staticGeoLocator{loc: nil})

	// Client in San Francisco → us-west-2 (Oregon) should be closest.
	req := &UploadRequest{
		Context: context.Background(),
		Metadata: map[string]string{
			"client_location": "37.7749,-122.4194", // San Francisco
		},
	}
	region, err := sel.SelectRegion(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "us-west-2", region.Name)
}

func TestSelectByGeography_IPLocator_UsedWhenNoHint(t *testing.T) {
	cfg := buildGeoConfig()
	// Locator returns Frankfurt coordinates → eu-central-1 should win.
	frankLoc := &ClientLocation{Lat: 50.1, Lon: 8.7, CountryCode: "DE"}
	sel := NewRegionSelectorWithGeoLocator(cfg, log.New(nil), &staticGeoLocator{loc: frankLoc})

	req := &UploadRequest{Context: context.Background()}
	region, err := sel.SelectRegion(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "eu-central-1", region.Name)
}

func TestSelectByGeography_DataResidency_EU(t *testing.T) {
	cfg := buildGeoConfig()
	// Client in NYC, but EU residency required.
	sel := NewRegionSelectorWithGeoLocator(cfg, log.New(nil), &staticGeoLocator{loc: nil})

	req := &UploadRequest{
		Context: context.Background(),
		Metadata: map[string]string{
			"client_location": "40.7,-74.0", // NYC
			"data_residency":  "EU",
		},
	}
	region, err := sel.SelectRegion(context.Background(), req)
	require.NoError(t, err)
	// Must be an EU region even though NYC is closer to US regions.
	assert.Contains(t, []string{"eu-west-1", "eu-central-1"}, region.Name)
}

func TestSelectByGeography_DataResidency_NoMatchFallsBackToAll(t *testing.T) {
	cfg := buildGeoConfig()
	// Remove all EU regions so residency requirement can't be met.
	cfg.Regions = []Region{
		{Name: "us-east-1", Status: RegionStatusHealthy, Priority: 1},
	}
	sel := NewRegionSelectorWithGeoLocator(cfg, log.New(nil), &staticGeoLocator{loc: nil})

	req := &UploadRequest{
		Context: context.Background(),
		Metadata: map[string]string{
			"client_location": "40.7,-74.0",
			"data_residency":  "EU", // no EU regions available
		},
	}
	region, err := sel.SelectRegion(context.Background(), req)
	require.NoError(t, err)
	// Residency restriction ignored — only region available returned.
	assert.Equal(t, "us-east-1", region.Name)
}

func TestSelectByGeography_NilLocatorAndNoHint_FallsBackToPriority(t *testing.T) {
	cfg := buildGeoConfig()
	// Locator always returns nil — no hint — should fall back to priority.
	sel := NewRegionSelectorWithGeoLocator(cfg, log.New(nil), &staticGeoLocator{loc: nil})

	req := &UploadRequest{Context: context.Background()}
	region, err := sel.SelectRegion(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "us-east-1", region.Name) // Priority 1
}

func TestSelectByGeography_InvalidHint_FallsBackToIPLocator(t *testing.T) {
	cfg := buildGeoConfig()
	frankLoc := &ClientLocation{Lat: 50.1, Lon: 8.7, CountryCode: "DE"}
	sel := NewRegionSelectorWithGeoLocator(cfg, log.New(nil), &staticGeoLocator{loc: frankLoc})

	req := &UploadRequest{
		Context: context.Background(),
		Metadata: map[string]string{
			"client_location": "not-a-location", // invalid hint
		},
	}
	region, err := sel.SelectRegion(context.Background(), req)
	require.NoError(t, err)
	// Falls back to IP locator (Frankfurt) → eu-central-1.
	assert.Equal(t, "eu-central-1", region.Name)
}

func TestSelectByGeography_NilRequestContext_UsesBackground(t *testing.T) {
	cfg := buildGeoConfig()
	tokyoLoc := &ClientLocation{Lat: 35.7, Lon: 139.7, CountryCode: "JP"}
	sel := NewRegionSelectorWithGeoLocator(cfg, log.New(nil), &staticGeoLocator{loc: tokyoLoc})

	// Context field intentionally nil — should not panic.
	req := &UploadRequest{Context: nil}
	region, err := sel.SelectRegion(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "ap-northeast-1", region.Name)
}

func TestSelectByGeography_EmptyRegions_ReturnsNil(t *testing.T) {
	cfg := buildGeoConfig()
	sel := NewRegionSelectorWithGeoLocator(cfg, log.New(nil), &staticGeoLocator{loc: nil}).(*DefaultRegionSelector)

	result := sel.selectByGeography(&UploadRequest{Context: context.Background()}, []*Region{})
	assert.Nil(t, result)
}
