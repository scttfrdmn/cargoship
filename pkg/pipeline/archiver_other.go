//go:build !linux

package pipeline

import (
	"archive/tar"
	"io"
	"os"

	"github.com/scttfrdmn/cargoship/pkg/ioutils"
)

// copyFileToArchive uses standard zero-copy optimization on non-Linux platforms
func (s *ArchiverStage) copyFileToArchive(tw *tar.Writer, f *os.File, length int64) error {
	_, err := ioutils.CopyOptimized(tw, io.LimitReader(f, length))
	return err
}
