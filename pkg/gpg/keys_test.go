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

func TestKeyType_String(t *testing.T) {
	tests := []struct {
		keyType  KeyType
		expected string
	}{
		{RSAKeyType, "rsa"},
		{X25519Type, "x25519"},
		{NullKeyType, ""},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			require.Equal(t, tt.expected, tt.keyType.String())
		})
	}
}

func TestKeyType_Type(t *testing.T) {
	var keyType KeyType
	require.Equal(t, "KeyType", keyType.Type())
}

func TestKeyType_Set(t *testing.T) {
	tests := []struct {
		name        string
		value       string
		expectedKey KeyType
		expectError bool
	}{
		{
			name:        "valid RSA",
			value:       "rsa",
			expectedKey: RSAKeyType,
			expectError: false,
		},
		{
			name:        "valid X25519",
			value:       "x25519",
			expectedKey: X25519Type,
			expectError: false,
		},
		{
			name:        "invalid key type",
			value:       "invalid",
			expectError: true,
		},
		{
			name:        "empty string",
			value:       "",
			expectedKey: NullKeyType,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var keyType KeyType
			err := keyType.Set(tt.value)
			
			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expectedKey, keyType)
			}
		})
	}
}

func TestKeyType_MarshalJSON(t *testing.T) {
	tests := []struct {
		keyType  KeyType
		expected string
	}{
		{RSAKeyType, `"rsa"`},
		{X25519Type, `"x25519"`},
	}

	for _, tt := range tests {
		t.Run(tt.keyType.String(), func(t *testing.T) {
			jsonBytes, err := tt.keyType.MarshalJSON()
			require.NoError(t, err)
			require.Equal(t, tt.expected, string(jsonBytes))
		})
	}
}

func TestKeyType_MarshalJSON_NullKey(t *testing.T) {
	// Test with NullKeyType - should return empty string
	keyType := NullKeyType
	jsonBytes, err := keyType.MarshalJSON()
	require.NoError(t, err)
	require.Equal(t, `""`, string(jsonBytes))
}
