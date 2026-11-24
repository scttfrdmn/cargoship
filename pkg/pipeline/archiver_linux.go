//go:build linux

package pipeline

import (
	"archive/tar"
	"io"
	"os"

	"github.com/scttfrdmn/cargoship/pkg/ioutils"
)

// copyFileToArchive uses splice() on Linux for zero-copy file transfer
func (s *ArchiverStage) copyFileToArchive(tw *tar.Writer, f *os.File, length int64) (int64, error) {
	return ioutils.CopyOptimizedWithSplice(tw, io.LimitReader(f, length))
}
