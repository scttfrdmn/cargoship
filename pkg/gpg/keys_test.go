package gpg

import (
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestNewKeyPair(t *testing.T) {
	gotKp, err := NewKeyPair(&KeyOpts{
		Name:    "test",
		Email:   "user@localhost",
		KeyType: "rsa",
		Bits:    1024,
	})
	require.NoError(t, err)
	require.IsType(t, &KeyPair{}, gotKp)
}

func TestNewKeyPairErrors(t *testing.T) {
	tests := map[string]struct {
		opts *KeyOpts
		err  error
	}{
		"missing name": {
			opts: &KeyOpts{
				Email:   "foo@bar.com",
				KeyType: "rsa",
			},
			err: errors.New("name is required"),
		},
		"missing email": {
			opts: &KeyOpts{
				Name:    "foo",
				KeyType: "rsa",
			},
			err: errors.New("email is required"),
		},
	}
	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			_, err := NewKeyPair(test.opts)
			require.Equal(t, test.err, err)
		})
	}
}

func TestNewKeyFilesWithPair(t *testing.T) {
	gotKp, err := NewKeyPair(&KeyOpts{
		Name:    "test",
		Email:   "user@localhost",
		KeyType: "rsa",
		Bits:    1024,
	})
	require.NoError(t, err)
	require.IsType(t, &KeyPair{}, gotKp)
	got, err := NewKeyFilesWithPair(gotKp, "")
	require.NoError(t, err)
	require.Equal(t, 2, len(got))
}

func TestProductionKeyTypeCompletion(t *testing.T) {
	got, _ := KeyTypeCompletion(&cobra.Command{}, []string{}, "")
	require.Equal(
		t,
		[]string{"rsa", "x25519"},
		got,
	)
}
