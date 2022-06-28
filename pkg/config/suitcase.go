package config

import (
	"github.com/ProtonMail/go-crypto/openpgp"
)

type SuitCaseOpts struct {
	Format       string
	Destination  string
	EncryptInner bool
	// Inventory    *inventory.DirectoryInventory
	EncryptTo *openpgp.EntityList
}
