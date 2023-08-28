/*
Package shell is a shell transporter plugin
*/
package shell

import (
	"os"

	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/plugins/transporters"
)

// Transporter is the main struct for this plugin
type Transporter struct {
	checkScript string
	sendScript  string
}

// Configure shell transporter
func (t *Transporter) Configure(c transporters.Config) error {
	t.checkScript = os.Getenv("SUITCASECTL_CHECKER")
	t.sendScript = os.Getenv("SUITCASECTL_SEND")
	return nil
}

// Check shell transporter
func (t Transporter) Check() error {
	return nil
}

// Send data using shell transporter
func (t Transporter) Send() error {
	return nil
}

// Validate this meets the Transporter interface
var _ transporters.Transporter = (*Transporter)(nil)
