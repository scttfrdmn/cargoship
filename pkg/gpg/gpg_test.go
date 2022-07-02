package gpg

import (
	"testing"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestReadEntity(t *testing.T) {
	got, err := ReadEntity("../testdata/fakey-public.key")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.IsType(t, &openpgp.Entity{}, got)
}

func TestEncryptToWithCmd(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().StringArray("public-key", []string{"../testdata/fakey-public.key"}, "")
	cmd.Flags().Bool("exclude-systems-pubkeys", false, "")
	_, err := EncryptToWithCmd(cmd)
	require.NoError(t, err)
}
