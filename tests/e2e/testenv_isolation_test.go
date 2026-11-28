//go:build e2e

package e2e

import "github.com/scttfrdmn/cargoship/internal/testenv"

// Isolate HOME, the XDG dirs and the AWS config before any test (or TestMain)
// runs. The cargoship binaries these tests spawn inherit os.Environ(), so they
// never read or write the developer's real ~/.cargoship* files or AWS profile
// (#1048).
func init() { testenv.Isolate() }
