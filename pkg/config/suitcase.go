/*
Package config holds configuration options for suitcases
*/
package config

import (
	"strings"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/spf13/cobra"
	"gitlab.oit.duke.edu/devil-ops/suitcasectl/pkg/gpg"
)

// SuitCaseOpts is options for a given suitcase
type SuitCaseOpts struct {
	Format            string
	Destination       string
	EncryptInner      bool // Encrypt all files in the archive
	EncryptOuter      bool // Encrypt the archive itself
	HashInner         bool // Hash files inside the archive
	HashOuter         bool // Hash the archive itself
	EncryptTo         *openpgp.EntityList
	PostProcessScript string
	PostProcessEnv    map[string]string
	// MaxBytes     uint64 // Maximum size per suitecase
}

// EncryptToCobra fills in the EncryptTo option using cobra.Command options
func (s *SuitCaseOpts) EncryptToCobra(cmd *cobra.Command) error {
	// Gather EncryptTo if we need it
	if strings.HasSuffix(s.Format, ".gpg") || s.EncryptInner {
		var err error
		s.EncryptTo, err = gpg.EncryptToWithCmd(cmd)
		if err != nil {
			return err
		}
	}
	return nil
}

// HashSet is a combination Filename and Hash
type HashSet struct {
	Filename string
	Hash     string
}
