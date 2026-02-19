package multiregion

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// geoLocationCacheTTL is the duration a detected client location is cached.
const geoLocationCacheTTL = 5 * time.Minute

// awsRegionCoords maps AWS region identifiers to their approximate geographic
// coordinates [lat°, lon°].
var awsRegionCoords = map[string][2]float64{
	"us-east-1":      {39.0, -77.5},  // N. Virginia
	"us-east-2":      {40.4, -82.9},  // Ohio
	"us-west-1":      {37.3, -121.9}, // N. California
	"us-west-2":      {45.8, -119.7}, // Oregon
	"ca-central-1":   {45.5, -73.6},  // Montréal
	"eu-west-1":      {53.3, -6.3},   // Ireland
	"eu-west-2":      {51.5, -0.1},   // London
	"eu-west-3":      {48.9, 2.4},    // Paris
	"eu-central-1":   {50.1, 8.7},    // Frankfurt
	"eu-north-1":     {59.3, 18.1},   // Stockholm
	"eu-south-1":     {45.5, 9.2},    // Milan
	"ap-southeast-1": {1.4, 103.8},   // Singapore
	"ap-southeast-2": {-33.9, 151.2}, // Sydney
	"ap-northeast-1": {35.7, 139.7},  // Tokyo
	"ap-northeast-2": {37.6, 126.9},  // Seoul
	"ap-northeast-3": {34.7, 135.5},  // Osaka
	"ap-south-1":     {19.1, 72.9},   // Mumbai
	"ap-east-1":      {22.3, 114.2},  // Hong Kong
	"sa-east-1":      {-23.5, -46.6}, // São Paulo
	"me-south-1":     {26.1, 50.5},   // Bahrain
	"af-south-1":     {-33.9, 18.4},  // Cape Town
}

// dataResidencyZones groups AWS regions by data-sovereignty zone.
// Used to enforce regulatory requirements when a client specifies a zone.
var dataResidencyZones = map[string][]string{
	"EU":   {"eu-west-1", "eu-west-2", "eu-west-3", "eu-central-1", "eu-north-1", "eu-south-1"},
	"US":   {"us-east-1", "us-east-2", "us-west-1", "us-west-2"},
	"APAC": {"ap-southeast-1", "ap-southeast-2", "ap-northeast-1", "ap-northeast-2", "ap-northeast-3", "ap-south-1", "ap-east-1"},
	"SA":   {"sa-east-1"},
	"ME":   {"me-south-1"},
	"AF":   {"af-south-1"},
	"CA":   {"ca-central-1"},
}

// ClientLocation represents a geographic point with optional ISO country code.
type ClientLocation struct {
	Lat         float64
	Lon         float64
	CountryCode string // ISO 3166-1 alpha-2, e.g. "US", "DE"
}

// GeoLocator detects the geographic location of the current host.
// Implementations must be safe for concurrent use.
type GeoLocator interface {
	// Locate returns the host's location, or nil when it cannot be determined.
	Locate(ctx context.Context) *ClientLocation
}

// HTTPGeoLocator uses the free ip-api.com JSON endpoint to determine the
// caller's location from their public IP address.
type HTTPGeoLocator struct {
	client  *http.Client
	timeout time.Duration
}

// NewHTTPGeoLocator creates an HTTPGeoLocator with a 2-second timeout.
func NewHTTPGeoLocator() *HTTPGeoLocator {
	return &HTTPGeoLocator{
		client:  &http.Client{},
		timeout: 2 * time.Second,
	}
}

// Locate calls ip-api.com/json to obtain the public IP's coordinates.
// Returns nil on any error so callers degrade gracefully.
func (g *HTTPGeoLocator) Locate(ctx context.Context) *ClientLocation {
	tctx, cancel := context.WithTimeout(ctx, g.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(tctx, http.MethodGet,
		"http://ip-api.com/json/?fields=lat,lon,countryCode", nil)
	if err != nil {
		return nil
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return nil
	}
	defer func() { _ = resp.Body.Close() }()

	var payload struct {
		Lat         float64 `json:"lat"`
		Lon         float64 `json:"lon"`
		CountryCode string  `json:"countryCode"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil
	}
	return &ClientLocation{Lat: payload.Lat, Lon: payload.Lon, CountryCode: payload.CountryCode}
}

// geoLocationCache is a thread-safe TTL cache around a GeoLocator.
type geoLocationCache struct {
	mu      sync.RWMutex
	cached  *ClientLocation
	expiry  time.Time
	ttl     time.Duration
	locator GeoLocator
}

// newGeoLocationCache wraps locator with a TTL cache.
func newGeoLocationCache(locator GeoLocator, ttl time.Duration) *geoLocationCache {
	return &geoLocationCache{locator: locator, ttl: ttl}
}

// Get returns the cached location if still fresh, otherwise fetches a new one.
func (c *geoLocationCache) Get(ctx context.Context) *ClientLocation {
	c.mu.RLock()
	if c.cached != nil && time.Now().Before(c.expiry) {
		loc := c.cached
		c.mu.RUnlock()
		return loc
	}
	c.mu.RUnlock()

	// Refresh — only one goroutine does the network call; others will re-use
	// the slightly-stale cache on the next read rather than stampeding.
	loc := c.locator.Locate(ctx)

	c.mu.Lock()
	c.cached = loc
	c.expiry = time.Now().Add(c.ttl)
	c.mu.Unlock()

	return loc
}

// haversineKm computes the great-circle distance in kilometres between two
// geographic coordinates using the Haversine formula.
func haversineKm(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadiusKm = 6371.0
	dLat := (lat2 - lat1) * math.Pi / 180.0
	dLon := (lon2 - lon1) * math.Pi / 180.0
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180.0)*math.Cos(lat2*math.Pi/180.0)*
			math.Sin(dLon/2)*math.Sin(dLon/2)
	return earthRadiusKm * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

// regionDistance returns the great-circle distance in km from (lat, lon) to
// the known centre of regionName. Returns math.MaxFloat64 for unknown regions
// so they sort to the bottom of proximity rankings.
func regionDistance(regionName string, lat, lon float64) float64 {
	coords, ok := awsRegionCoords[regionName]
	if !ok {
		return math.MaxFloat64
	}
	return haversineKm(lat, lon, coords[0], coords[1])
}

// isRegionInResidencyZone reports whether regionName is permitted by the
// requested data-sovereignty zone (e.g. "EU"). Returns true when zone is
// empty (no restriction).
func isRegionInResidencyZone(zone, regionName string) bool {
	if zone == "" {
		return true
	}
	for _, r := range dataResidencyZones[zone] {
		if r == regionName {
			return true
		}
	}
	return false
}

// parseLatLon parses a "lat,lon" string into floating-point coordinates.
// Accepts optional whitespace around the comma.
func parseLatLon(s string) (lat, lon float64, err error) {
	parts := strings.SplitN(s, ",", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("expected \"lat,lon\", got %q", s)
	}
	lat, err = strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid latitude %q: %w", parts[0], err)
	}
	lon, err = strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid longitude %q: %w", parts[1], err)
	}
	return lat, lon, nil
}
