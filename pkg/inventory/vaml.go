package inventory

/*
Read and write the inventory using Victors YAML fork. This fork is a more memory
efficient writer.
*/

import (
	"bufio"
	"errors"
	"io"
	"log/slog"

	"github.com/vjorlikowski/yaml"
)

// VAMLer is the Victor YAML for operator
type VAMLer struct{}

// Write will write the inventory out to an io.Writer
func (r *VAMLer) Write(w io.Writer, i *Inventory) error {
	if w == nil {
		return errors.New("writer is nil")
	}
	if i == nil {
		return errors.New("inventory is nil")
	}

	slog.Debug("About to encode inventory in to yaml file")
	writer := bufio.NewWriterSize(w, 10240)
	defer func() {
		err := writer.Flush()
		if err != nil {
			panic(err)
		}
	}()

	// Pass the buffered IO writer to the encoder
	printMemUsage()
	slog.Debug("About to create a new VAML encoder")
	enc := yaml.NewEncoder(writer)
	return enc.Encode(i)
}

// Read will ready byte in to an inventory
func (r VAMLer) Read(b []byte) (*Inventory, error) {
	var inventory Inventory
	err := yaml.Unmarshal(b, &inventory)
	if err != nil {
		return nil, err
	}
	return &inventory, nil
}
