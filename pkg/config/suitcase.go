package config

import (
	"github.com/ProtonMail/go-crypto/openpgp"
)

type SuitCaseOpts struct {
	Format       string
	Destination  string
	EncryptInner bool // Encrypt all files in the archive
	EncryptOuter bool // Encrypt the archive itself
	EncryptTo    *openpgp.EntityList
	// MaxBytes     uint64 // Maximum size per suitecase
}
