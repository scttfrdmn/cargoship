package porter

import (
	"os"
	"path"
	"testing"

	"github.com/scttfrdmn/cargoship/pkg/config"
	"github.com/scttfrdmn/cargoship/pkg/inventory"
	"github.com/stretchr/testify/require"
)

func TestHashInner(t *testing.T) {
	fn := path.Join(t.TempDir(), "test.txt")
	require.NoError(t, os.WriteFile(fn, []byte("Testing"), 0o600))
	require.NoError(t, hashInner(fn, inventory.MD5Hash, []config.HashSet{}))
}
