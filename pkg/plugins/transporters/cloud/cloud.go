/*
Package cloud defines how to transport the files out to the cloud ☁️
*/
package cloud

import (
	"errors"
	"fmt"

	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/plugins/transporters"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/rclone"
)

// Transporter is the main struct for this plugin
type Transporter struct {
	Config transporters.Config
}

// Check if the transporter is valid
func (t *Transporter) Check() error {
	if t.Config.Destination == "" {
		return errors.New("destination is not set")
	}
	if !rclone.Exists(t.Config.Destination) {
		return fmt.Errorf("destination does not exist: %v", t.Config.Destination)
	}
	return nil
}

// Send the data on up
func (t Transporter) Send(s string) error {
	return rclone.Clone(s, t.Config.Destination, "")
}

// Validate this meets the Transporter interface
var _ transporters.Transporter = (*Transporter)(nil)
