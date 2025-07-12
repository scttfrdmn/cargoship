package controller

import (
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewController(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
		},
		{
			name: "missing listen address",
			config: &Config{
				AuthToken: "test-token",
			},
			wantErr: true,
		},
		{
			name: "missing auth token",
			config: &Config{
				ListenAddr: ":8080",
			},
			wantErr: true,
		},
		{
			name: "TLS enabled without cert files",
			config: &Config{
				ListenAddr: ":8080",
				AuthToken:  "test-token",
				TLSEnabled: true,
			},
			wantErr: true,
		},
		{
			name: "valid config without TLS",
			config: &Config{
				ListenAddr: ":8080",
				AuthToken:  "test-token",
				TLSEnabled: false,
				LogLevel:   "info",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl, err := NewController(tt.config, logger)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, ctrl)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, ctrl)
				assert.Equal(t, tt.config, ctrl.config)
				assert.NotNil(t, ctrl.registry)
				assert.NotNil(t, ctrl.wsServer)
			}
		})
	}
}

func TestControllerLifecycle(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	config := &Config{
		ListenAddr: ":0", // Use random port for testing
		AuthToken:  "test-token",
		TLSEnabled: false,
		LogLevel:   "error",
	}

	ctrl, err := NewController(config, logger)
	require.NoError(t, err)
	require.NotNil(t, ctrl)

	// Test components are accessible
	assert.NotNil(t, ctrl.GetRegistry())
	assert.NotNil(t, ctrl.GetWebSocketServer())

	// Note: We don't test Start() here because it would try to bind to a port
	// and we want unit tests to be isolated

	// Test stop (should work even if not started)
	err = ctrl.Stop()
	assert.NoError(t, err)
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "valid config without TLS",
			config: &Config{
				ListenAddr: ":8080",
				AuthToken:  "test-token",
				TLSEnabled: false,
			},
			wantErr: false,
		},
		{
			name: "valid config with TLS",
			config: &Config{
				ListenAddr:  ":8080",
				AuthToken:   "test-token",
				TLSEnabled:  true,
				TLSCertFile: "cert.pem",
				TLSKeyFile:  "key.pem",
			},
			wantErr: false,
		},
		{
			name: "missing listen address",
			config: &Config{
				AuthToken: "test-token",
			},
			wantErr: true,
		},
		{
			name: "missing auth token",
			config: &Config{
				ListenAddr: ":8080",
			},
			wantErr: true,
		},
		{
			name: "TLS enabled without cert file",
			config: &Config{
				ListenAddr: ":8080",
				AuthToken:  "test-token",
				TLSEnabled: true,
				TLSKeyFile: "key.pem",
			},
			wantErr: true,
		},
		{
			name: "TLS enabled without key file",
			config: &Config{
				ListenAddr:  ":8080",
				AuthToken:   "test-token",
				TLSEnabled:  true,
				TLSCertFile: "cert.pem",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfig(tt.config)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestBuildTLSConfig(t *testing.T) {
	tests := []struct {
		name      string
		config    *Config
		wantNil   bool
		wantError bool
	}{
		{
			name: "TLS disabled",
			config: &Config{
				TLSEnabled: false,
			},
			wantNil:   true,
			wantError: false,
		},
		{
			name: "TLS enabled with invalid files",
			config: &Config{
				TLSEnabled:  true,
				TLSCertFile: "non-existent-cert.pem",
				TLSKeyFile:  "non-existent-key.pem",
			},
			wantNil:   true,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tlsConfig, err := buildTLSConfig(tt.config)
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if tt.wantNil {
				assert.Nil(t, tlsConfig)
			} else {
				assert.NotNil(t, tlsConfig)
			}
		})
	}
}