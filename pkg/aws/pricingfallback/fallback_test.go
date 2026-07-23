package pricingfallback

import (
	"testing"

	"github.com/scttfrdmn/cargoship/pkg/aws/config"
)

// TestStoragePrice pins the canonical storage fallback table so a change to any
// number is deliberate and reviewed (the fallback tables previously diverged
// across three stacks — #237).
func TestStoragePrice(t *testing.T) {
	cases := map[config.StorageClass]float64{
		config.StorageClassStandard:           0.023,
		config.StorageClassStandardIA:         0.0125,
		config.StorageClassOneZoneIA:          0.01,
		config.StorageClassIntelligentTiering: 0.0225, // reconciled from 0.023 (#237)
		config.StorageClassGlacier:            0.004,
		config.StorageClassDeepArchive:        0.00099,
	}
	for sc, want := range cases {
		if got := StoragePrice(sc); got != want {
			t.Errorf("StoragePrice(%s) = %v, want %v", sc, got, want)
		}
	}
	if got := StoragePrice(config.StorageClass("BOGUS")); got != 0.023 {
		t.Errorf("StoragePrice(unknown) = %v, want 0.023 (Standard default)", got)
	}
}

// TestRequestPrice pins the request fallback table, including the key #237 fix:
// PUT prices vary by storage class (archival classes cost more), where the cost
// stack priced every PUT at the Standard rate and the pricing stack carried a
// 10x-too-low table.
func TestRequestPrice(t *testing.T) {
	type key struct {
		verb string
		sc   config.StorageClass
	}
	cases := map[key]float64{
		{"PUT", config.StorageClassStandard}:           0.005,
		{"PUT", config.StorageClassStandardIA}:         0.01,
		{"PUT", config.StorageClassOneZoneIA}:          0.01,
		{"PUT", config.StorageClassIntelligentTiering}: 0.005,
		{"PUT", config.StorageClassGlacier}:            0.03,
		{"PUT", config.StorageClassDeepArchive}:        0.05,
		{"COPY", config.StorageClassGlacier}:           0.03, // COPY/POST/LIST share the PUT tier
		{"LIST", config.StorageClassDeepArchive}:       0.05,
		{"GET", config.StorageClassStandard}:           0.0004, // GET/SELECT/DELETE don't vary by class
		{"GET", config.StorageClassGlacier}:            0.0004,
		{"SELECT", config.StorageClassDeepArchive}:     0.0004,
		{"DELETE", config.StorageClassStandard}:        0.0,
		{"DELETE", config.StorageClassGlacier}:         0.0,
	}
	for k, want := range cases {
		if got := RequestPrice(k.verb, k.sc); got != want {
			t.Errorf("RequestPrice(%s, %s) = %v, want %v", k.verb, k.sc, got, want)
		}
	}
	if got := RequestPrice("put", config.StorageClassGlacier); got != 0.03 {
		t.Errorf("lowercase verb not normalized: got %v", got)
	}
	if got := RequestPrice("WEIRD", config.StorageClassStandard); got != 0.005 {
		t.Errorf("unknown verb should default to PUT tier: got %v", got)
	}
}

// TestTables verifies the map helpers cover every storage class and agree with
// the scalar accessors — so a caller populating a PriceData map can't silently
// miss a class.
func TestTables(t *testing.T) {
	st := StoragePriceTable()
	rt := PutRequestPriceTable()
	for _, c := range allClasses() {
		if st[c] != StoragePrice(c) {
			t.Errorf("StoragePriceTable[%s]=%v != StoragePrice=%v", c, st[c], StoragePrice(c))
		}
		if rt[c] != PutRequestPrice(c) {
			t.Errorf("PutRequestPriceTable[%s]=%v != PutRequestPrice=%v", c, rt[c], PutRequestPrice(c))
		}
	}
	if len(st) != len(allClasses()) || len(rt) != len(allClasses()) {
		t.Errorf("tables incomplete: storage=%d request=%d want=%d", len(st), len(rt), len(allClasses()))
	}
}
