//go:build !integration

package pipeline

import (
	"testing"

	"github.com/scttfrdmn/cargoship/pkg/config"
)

// TestBuildScannerConfig_ThreadsMagika guards #30: the scanner only runs Magika
// when its ScannerConfig.MagikaConfig is set, and that must come from the
// pipeline config. Before this plumbing existed the field was never populated,
// so Magika never ran regardless of user config. This fails if the wiring
// regresses.
func TestBuildScannerConfig_ThreadsMagika(t *testing.T) {
	mc := &config.MagikaConfig{Enabled: true, BatchSize: 100}
	sc := buildScannerConfig(&PipelineConfig{
		ScannerWorkers:   3,
		IncludeOnlyFiles: []string{"a.txt"},
		MagikaConfig:     mc,
	}, "/root")

	if sc.MagikaConfig != mc {
		t.Fatal("#30: buildScannerConfig must thread MagikaConfig to the scanner, else Magika never runs")
	}
	if sc.RootPath != "/root" || sc.Workers != 3 {
		t.Errorf("scanner config fields not mapped: root=%q workers=%d", sc.RootPath, sc.Workers)
	}
	if len(sc.IncludeOnlyFiles) != 1 {
		t.Errorf("IncludeOnlyFiles not mapped: %v", sc.IncludeOnlyFiles)
	}

	// No magika config → stays nil (detection disabled).
	if got := buildScannerConfig(&PipelineConfig{}, "/x").MagikaConfig; got != nil {
		t.Errorf("nil MagikaConfig must stay nil (Magika disabled), got %+v", got)
	}
}
