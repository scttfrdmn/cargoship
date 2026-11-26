package s3metrics

import "github.com/scttfrdmn/cargoship/internal/testenv"

// Isolate HOME, the XDG dirs and the AWS config before any test (or TestMain)
// runs, so the suite never writes the developer's real ~/.cargoship* files or
// reaches a real AWS profile (#1048).
func init() { testenv.Isolate() }
