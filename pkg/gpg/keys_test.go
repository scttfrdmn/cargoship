package gpg

import (
	"errors"
	"testing"

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
			err: errors.New("Name is required"),
		},
		"missing email": {
			opts: &KeyOpts{
				Name:    "foo",
				KeyType: "rsa",
			},
			err: errors.New("Email is required"),
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := NewKeyPair(test.opts)
			require.Equal(t, test.err, err)
		})
	}
}
