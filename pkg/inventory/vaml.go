package inventory

/*
Read and write the inventory using Victors YAML fork. This fork is a more memory
efficient writer.
*/

import (
	"bufio"
	"errors"
	"io"

	"github.com/rs/zerolog/log"
	"github.com/vjorlikowski/yaml"
)

type VAMLer struct{}

func (r *VAMLer) Write(w io.Writer, i *DirectoryInventory) error {
	if w == nil {
		return errors.New("writer is nil")
	}
	if i == nil {
		return errors.New("inventory is nil")
	}

	log.Debug().Msg("About to encode inventory in to yaml file")
	writer := bufio.NewWriterSize(w, 10240)
	defer writer.Flush()

	// Pass the buffered IO writer to the encoder
	printMemUsage()
	log.Debug().Msg("About to create a new VAML encoder")
	enc := yaml.NewEncoder(writer)
	return enc.Encode(i)
}

func (r VAMLer) Read(b []byte) (*DirectoryInventory, error) {
	var inventory DirectoryInventory
	err := yaml.Unmarshal(b, &inventory)
	if err != nil {
		return nil, err
	}
	return &inventory, nil
}
