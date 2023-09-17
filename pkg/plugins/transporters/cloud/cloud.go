/*
Package cloud defines how to transport the files out to the cloud ☁️
*/
package cloud

import (
	"errors"
	"path"

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
	return nil
}

// Send the data on up
func (t Transporter) Send(s, u string) error {
	dest := t.Config.Destination
	if u != "" {
		dest = path.Join(dest, u)
	}
	/*
		c := make(chan rclone.TransferStatus)

		go func() {
			for {
				status := <-c
				status.Name = filepath.Base(s)
				if status.Stats.Bytes != 0 {
					fmt.Fprintf(os.Stderr, "TransferStatus: %+v\n", status)
				}
				// log.Info().Interface("status", status).Msg("transfer status")
			}
		}()
	*/
	err := rclone.Copy(s, dest, nil)

	return err
	// return rclone.Clone(s, dest)
}

// Validate this meets the Transporter interface
var _ transporters.Transporter = (*Transporter)(nil)
