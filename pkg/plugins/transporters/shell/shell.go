/*
Package shell is a shell transporter plugin
*/
package shell

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"

	"github.com/rs/zerolog/log"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/plugins/transporters"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/rclone"
)

// Transporter is the main struct for this plugin
type Transporter struct {
	checkScript string
	Config      transporters.Config
}

// Configure shell transporter
func (t *Transporter) Configure(c transporters.Config) error {
	return nil
}

// Check shell transporter
func (t *Transporter) Check() error {
	if t.Config.Destination == "" {
		return errors.New("must set a non empty destination")
	}

	if t.checkScript == "" {
		log.Debug().Msg("no checker script specified")
		return nil
	}

	rcmd := exec.Command(t.checkScript) // nolint
	var stdBuffer bytes.Buffer
	mw := io.MultiWriter(os.Stdout, &stdBuffer)

	rcmd.Stdout = mw
	rcmd.Stderr = mw

	// Execute the command
	log.Info().Msg("running shell transporter check")
	if err := rcmd.Run(); err != nil {
		return err
	}

	return nil
}

// Send data using shell transporter
func (t Transporter) Send(s, u string) error {
	c := make(chan rclone.TransferStatus)
	// Just drop junk
	go func() {
		for {
			<-c
		}
	}()
	return t.SendWithChannel(s, u, c)
}

// SendWithChannel sends through the given channel
func (t Transporter) SendWithChannel(s, u string, c chan rclone.TransferStatus) error {
	if err := os.Setenv("SUITCASECTL_FILE", s); err != nil {
		return err
	}
	log.Info().Str("cmd", t.Config.Destination).Msg("running send command")
	rcmd := exec.Command(t.Config.Destination) // nolint
	var stdBuffer bytes.Buffer
	mw := io.MultiWriter(os.Stdout, &stdBuffer)

	rcmd.Stdout = mw
	rcmd.Stderr = mw

	// Execute the command
	log.Info().Msg("running shell transporter")
	if err := rcmd.Run(); err != nil {
		return err
	}

	return nil
}

// Validate this meets the Transporter interface
var _ transporters.Transporter = (*Transporter)(nil)
