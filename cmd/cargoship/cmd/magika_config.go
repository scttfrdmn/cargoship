package cmd

import (
	"github.com/spf13/viper"

	"github.com/scttfrdmn/cargoship/pkg/config"
)

// magikaConfigFromViper reads the optional `magika:` block from the loaded
// cargoship config (see initConfig in root.go). It returns nil when no magika
// block is present — which leaves AI file-type detection disabled — and
// otherwise a config seeded from defaults and overlaid with the user's keys.
//
// #30: the upload/sync/create commands pass the result into
// PipelineConfig.MagikaConfig. Before this existed, MagikaConfig was never
// populated, so the scanner's `MagikaConfig != nil && .Enabled` gate never
// opened and Magika never ran regardless of what the user configured.
func magikaConfigFromViper() *config.MagikaConfig {
	if !viper.IsSet("magika") {
		return nil
	}
	m := config.DefaultConfig().Magika
	if err := viper.UnmarshalKey("magika", &m); err != nil {
		return nil
	}
	return &m
}
