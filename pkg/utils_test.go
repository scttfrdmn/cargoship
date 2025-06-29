package porter

import (
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/scttfrdmn/cargoship-cli/pkg/config"
	"github.com/scttfrdmn/cargoship-cli/pkg/inventory"
)

func TestHashInner(t *testing.T) {
	fn := path.Join(t.TempDir(), "test.txt")
	require.NoError(t, os.WriteFile(fn, []byte("Testing"), 0o600))
	require.NoError(t, hashInner(fn, inventory.MD5Hash, []config.HashSet{}))
}
