package inventory

import (
	"errors"
	"io"

	"github.com/mailru/easyjson"
	"github.com/rs/zerolog/log"
)

/*
Inventoryer using EasyJSON to encode and decode the inventory.
*/

// EJSONer is the easy json operator
type EJSONer struct{}

// Write writes out an inventory file
func (r *EJSONer) Write(w io.Writer, i *Inventory) error {
	if w == nil {
		return errors.New("writer is nil")
	}
	if i == nil {
		return errors.New("inventory is nil")
	}
	log.Debug().Msg("About to encode inventory in to json file")
	_, err := easyjson.MarshalToWriter(i, w)
	return err
}

// Read reads the bytes and returns an inventory
func (r EJSONer) Read(b []byte) (*Inventory, error) {
	if b == nil {
		return nil, errors.New("bytes is nil")
	}
	var inventory Inventory
	err := easyjson.Unmarshal(b, &inventory)
	if err != nil {
		return nil, err
	}
	return &inventory, nil
}
