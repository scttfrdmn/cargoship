/*
Package transporters define how transport plugins behave
*/
package transporters

import (
	"fmt"
	"os"
	"path"

	"github.com/oklog/ulid/v2"
	"github.com/scttfrdmn/cargoship-cli/pkg/rclone"
)

// Transporter describes how items that meet a Transporter behaves
type Transporter interface {
	// Configure(c Config) error
	Check() error
	Send(s, u string) error
	SendWithChannel(s, u string, c chan rclone.TransferStatus) error
}

// Config is everything a transporter needs to be configured
type Config struct {
	Destination string
	Purge       bool
}

// ToEnv sets interesting info in a Key/Value format
func (c Config) ToEnv() error {
	prefix := "SUITCASECTL_"
	env := map[string]string{
		fmt.Sprintf("%vDESTINATION", prefix): c.Destination,
	}
	for k, v := range env {
		err := os.Setenv(k, v)
		if err != nil {
			return err
		}
	}
	return nil
}

// UniquifyDest turns a string in to a unique string, by appending a ULID
func UniquifyDest(s string) string {
	u := ulid.Make()
	return path.Join(s, u.String())
}
