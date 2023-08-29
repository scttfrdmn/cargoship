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
)

// Transporter is the main struct for this plugin
type Transporter struct {
	checkScript string
	sendScript  string
}

// Configure shell transporter
func (t *Transporter) Configure(c transporters.Config) error {
	return nil
}

// Check shell transporter
func (t *Transporter) Check() error {
	t.checkScript = os.Getenv("SUITCASECTL_CHECK")
	t.sendScript = os.Getenv("SUITCASECTL_SEND")
	if t.checkScript == "" {
		return errors.New("must set SUITCASECTL_CHECK to the shell script to do a sanity check")
	}
	if t.sendScript == "" {
		return errors.New("must set SUITCASECTL_SEND to the shell script to send the file off")
	}

	rcmd := exec.Command(t.checkScript) // nolint
	var stdBuffer bytes.Buffer
	mw := io.MultiWriter(os.Stdout, &stdBuffer)

	rcmd.Stdout = mw
	rcmd.Stderr = mw

	// Execute the command
	log.Info().Msg("running shell transporter")
	if err := rcmd.Run(); err != nil {
		return err
	}
	// retCode := rcmd.ProcessState.ExitCode()
	// fmt.Fprintf(os.Stderr, "EXIT: %v\n", retCode)

	return nil
}

// Send data using shell transporter
func (t Transporter) Send(c transporters.Config) error {
	log.Info().Str("cmd", t.sendScript).Msg("running send command")
	rcmd := exec.Command(t.sendScript) // nolint
	var stdBuffer bytes.Buffer
	mw := io.MultiWriter(os.Stdout, &stdBuffer)

	rcmd.Stdout = mw
	rcmd.Stderr = mw

	// Execute the command
	log.Info().Msg("running shell transporter")
	if err := rcmd.Run(); err != nil {
		return err
	}
	// retCode := rcmd.ProcessState.ExitCode()
	// fmt.Fprintf(os.Stderr, "EXIT: %v\n", retCode)

	return nil
}

// Validate this meets the Transporter interface
var _ transporters.Transporter = (*Transporter)(nil)
