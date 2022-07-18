package cmdhelpers

import (
	"time"

	"github.com/spf13/cobra"
)

type CLIMeta struct {
	Username    string         `yaml:"username"`
	Hostname    string         `yaml:"hostname"`
	StartedAt   *time.Time     `yaml:"started_at"`
	CompletedAt *time.Time     `yaml:"started_at"`
	Arguments   []string       `yaml:"arguments"`
	Options     *cobra.Command `yaml:"options"`
}
