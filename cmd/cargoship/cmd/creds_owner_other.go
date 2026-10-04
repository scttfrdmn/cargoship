//go:build !unix

package cmd

import "os"

// fileOwnerUID reports no owner on platforms without POSIX file ownership. The caller
// degrades to reporting just the mode, which is still more than the AWS SDK's EC2 IMDS
// timeout said.
func fileOwnerUID(_ os.FileInfo) (uint32, bool) { return 0, false }
