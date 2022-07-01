package config

import (
	"github.com/ProtonMail/go-crypto/openpgp"
)

type SuitCaseOpts struct {
	Format       string
	Destination  string
	EncryptInner bool
	EncryptTo    *openpgp.EntityList
	// Maximum size per suitecase
	MaxBytes int64
}
