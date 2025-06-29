/*
Package cloud defines how to transport the files out to the cloud ☁️
*/
package cloud

import (
	"errors"
	"log/slog"
	"strings"

	"github.com/scttfrdmn/cargoship-cli/pkg/plugins/transporters"
	"github.com/scttfrdmn/cargoship-cli/pkg/rclone"
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
	return nil
}

// Send sends the data up
func (t Transporter) Send(s, u string) error {
	c := make(chan rclone.TransferStatus)
	go func() {
		for {
			<-c
		}
	}()
	return t.SendWithChannel(s, u, c)
}

// SendWithChannel the data on up with an optional channel
func (t Transporter) SendWithChannel(s, u string, c chan rclone.TransferStatus) error {
	dest := t.Config.Destination
	if u != "" {
		dest = strings.TrimSuffix(dest, "/") + "/" + strings.TrimPrefix(u, "/")
	}
	slog.Debug("sending to rclone.Copy", "source", s, "destination", dest)
	err := rclone.Copy(s, dest, c)

	return err
}

// Validate this meets the Transporter interface
var _ transporters.Transporter = (*Transporter)(nil)
