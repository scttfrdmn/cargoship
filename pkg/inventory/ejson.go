package inventory

import (
	"errors"
	"io"

	easyjson "github.com/mailru/easyjson"
	"github.com/rs/zerolog/log"
)

/*
Inventoryer using EasyJSON to encode and decode the inventory.
*/

type EJSONer struct{}

func (r *EJSONer) Write(w io.Writer, i *DirectoryInventory) error {
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

func (r EJSONer) Read(b []byte) (*DirectoryInventory, error) {
	if b == nil {
		return nil, errors.New("bytes is nil")
	}
	var inventory DirectoryInventory
	err := easyjson.Unmarshal(b, &inventory)
	if err != nil {
		return nil, err
	}
	return &inventory, nil
}
