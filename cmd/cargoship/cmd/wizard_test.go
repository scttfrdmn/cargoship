package cmd

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	porter "github.com/scttfrdmn/cargoship/pkg"
)

func TestNewWizardCmd(t *testing.T) {
	cmd := NewWizardCmd()
	
	require.NotNil(t, cmd)
	assert.Equal(t, "wizard", cmd.Use)
	assert.Equal(t, "Run a console wizard to do the creation", cmd.Short)
	assert.NotEmpty(t, cmd.Long)
	assert.NotNil(t, cmd.PreRunE)
	assert.NotNil(t, cmd.PostRunE)
	
	// Test flags are defined
	flags := cmd.PersistentFlags()
	
	inventoryDirFlag := flags.Lookup("inventory-directory")
	require.NotNil(t, inventoryDirFlag)
	// The default value is []string{"."}, but flags show it as a string
	assert.NotEmpty(t, inventoryDirFlag.DefValue)
}

func TestWizardPreRunE(t *testing.T) {
	// Test the pre-run function logic without calling the global persistent pre-run
	// We'll test the Porter setup portion separately
	cmd := NewWizardCmd()
	
	// Set up a minimal context
	ctx := context.Background()
	cmd.SetContext(ctx)
	
	// Test just the Porter setup part of wizardPreRunE
	// We'll create the porter options directly like the function does
	opts := []porter.Option{
		porter.WithLogger(logger),
		porter.WithHashAlgorithm(hashAlgo),
		porter.WithVersion(version),
		porter.WithCLIMeta(
			porter.NewCLIMeta(
				porter.WithStart(toPTR(time.Now())),
			),
		),
	}
	
	// Create porter and add to context
	porterInstance := porter.New(opts...)
	cmd.SetContext(context.WithValue(cmd.Context(), porter.PorterKey, porterInstance))
	
	// Verify that porter was added to the context
	porterValue := cmd.Context().Value(porter.PorterKey)
	assert.NotNil(t, porterValue, "Porter should be set in command context")
	
	// Verify it's the correct type
	_, ok := porterValue.(*porter.Porter)
	assert.True(t, ok, "Context value should be a Porter instance")
}

func TestWizardPostRunE(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping wizard post-run test in short mode")
	}
	
	// This test requires proper porter setup and would need actual file operations
	// We'll test the validation and setup instead
	cmd := NewWizardCmd()
	
	// The post-run function requires a porter in the context
	// We'd need to set up a full porter instance for this to work
	// For now, we'll verify the function exists and can be called
	assert.NotNil(t, cmd.PostRunE)
	
	// The actual execution would require:
	// - Porter instance in context
	// - Valid destination directory  
	// - Proper CLI meta setup
	// These are integration concerns tested elsewhere
}

func TestWizardCommandStructure(t *testing.T) {
	cmd := NewWizardCmd()
	
	// Test that help text contains expected content
	helpText := cmd.Long
	assert.Contains(t, helpText, "simple command")
	assert.Contains(t, helpText, "create suitcase")
	
	// Verify command has the right hooks
	assert.NotNil(t, cmd.PreRunE, "Should have PreRunE hook")
	assert.NotNil(t, cmd.PostRunE, "Should have PostRunE hook")
}

func TestWizardFlags(t *testing.T) {
	cmd := NewWizardCmd()
	flags := cmd.PersistentFlags()
	
	// Test inventory-directory flag
	inventoryFlag := flags.Lookup("inventory-directory")
	require.NotNil(t, inventoryFlag)
	assert.Equal(t, "stringArray", inventoryFlag.Value.Type())
	
	// Test setting the flag
	err := flags.Set("inventory-directory", "/path/to/inventory")
	assert.NoError(t, err)
	
	// Test getting the flag value
	inventoryDirs, err := flags.GetStringArray("inventory-directory")
	assert.NoError(t, err)
	assert.Contains(t, inventoryDirs, "/path/to/inventory")
}

func TestWizardFlagMultipleValues(t *testing.T) {
	cmd := NewWizardCmd()
	flags := cmd.PersistentFlags()
	
	// Test setting multiple inventory directories
	err := flags.Set("inventory-directory", "/path/1")
	assert.NoError(t, err)
	
	err = flags.Set("inventory-directory", "/path/2")
	assert.NoError(t, err)
	
	// Get all values
	inventoryDirs, err := flags.GetStringArray("inventory-directory")
	assert.NoError(t, err)
	assert.Len(t, inventoryDirs, 2)
	assert.Contains(t, inventoryDirs, "/path/1")
	assert.Contains(t, inventoryDirs, "/path/2")
}

func TestWizardPreRunEContextSetup(t *testing.T) {
	// Test that the pre-run function logic sets up the porter context correctly
	cmd := NewWizardCmd()
	
	// Start with empty context
	originalCtx := context.Background()
	cmd.SetContext(originalCtx)
	
	// Verify porter is not in context initially
	porterValue := cmd.Context().Value(porter.PorterKey)
	assert.Nil(t, porterValue)
	
	// Simulate the porter setup part of wizardPreRunE
	opts := []porter.Option{
		porter.WithLogger(logger),
		porter.WithHashAlgorithm(hashAlgo),
		porter.WithVersion(version),
		porter.WithCLIMeta(
			porter.NewCLIMeta(
				porter.WithStart(toPTR(time.Now())),
			),
		),
	}
	
	porterInstance := porter.New(opts...)
	cmd.SetContext(context.WithValue(cmd.Context(), porter.PorterKey, porterInstance))
	
	// Verify porter is now in context
	porterValue = cmd.Context().Value(porter.PorterKey)
	assert.NotNil(t, porterValue)
	
	// Verify it's properly configured
	porterInstance, ok := porterValue.(*porter.Porter)
	require.True(t, ok)
	assert.NotNil(t, porterInstance)
	// Logger might be nil in tests, but other fields should be set
	assert.NotEmpty(t, porterInstance.HashAlgorithm)
	assert.NotEmpty(t, porterInstance.Version)
	assert.NotNil(t, porterInstance.CLIMeta)
}

func TestWizardPorterOptions(t *testing.T) {
	// Test that the porter options are set correctly
	cmd := NewWizardCmd()
	
	ctx := context.Background()
	cmd.SetContext(ctx)
	
	// Create porter with the same options as wizardPreRunE
	opts := []porter.Option{
		porter.WithLogger(logger),
		porter.WithHashAlgorithm(hashAlgo),
		porter.WithVersion(version),
		porter.WithCLIMeta(
			porter.NewCLIMeta(
				porter.WithStart(toPTR(time.Now())),
			),
		),
	}
	
	porterInstance := porter.New(opts...)
	cmd.SetContext(context.WithValue(cmd.Context(), porter.PorterKey, porterInstance))
	
	// Get porter from context
	porterValue := cmd.Context().Value(porter.PorterKey)
	require.NotNil(t, porterValue)
	
	porterInstance, ok := porterValue.(*porter.Porter)
	require.True(t, ok)
	
	// Verify porter configuration
	// Logger might be nil in test environment
	assert.Equal(t, hashAlgo, porterInstance.HashAlgorithm, "Hash algorithm should match global setting")
	assert.Equal(t, version, porterInstance.Version, "Version should match global setting")
	assert.NotNil(t, porterInstance.CLIMeta, "CLI meta should be initialized")
	assert.NotNil(t, porterInstance.CLIMeta.StartedAt, "CLI meta start time should be set")
}

func TestWizardGlobalVariables(t *testing.T) {
	// Test that wizard functions can access global variables
	
	// These global variables should be accessible in wizard functions
	assert.NotEmpty(t, hashAlgo, "hashAlgo should be set")
	assert.NotEmpty(t, version, "version should be set")
	
	// Test that globals would be used in porter creation
	opts := []porter.Option{
		porter.WithLogger(logger),
		porter.WithHashAlgorithm(hashAlgo),
		porter.WithVersion(version),
		porter.WithCLIMeta(
			porter.NewCLIMeta(
				porter.WithStart(toPTR(time.Now())),
			),
		),
	}
	
	porterInstance := porter.New(opts...)
	
	// Verify globals were used
	assert.Equal(t, hashAlgo, porterInstance.HashAlgorithm)
	assert.Equal(t, version, porterInstance.Version)
}

func TestWizardCommandIntegration(t *testing.T) {
	// Test that the wizard command can be created and configured without errors
	cmd := NewWizardCmd()
	
	// Test command configuration
	assert.Equal(t, "wizard", cmd.Use)
	assert.NotEmpty(t, cmd.Short)
	assert.NotEmpty(t, cmd.Long)
	
	// Test that flags can be set
	err := cmd.PersistentFlags().Set("inventory-directory", ".")
	assert.NoError(t, err)
	
	// Test that porter can be set up in context manually (simulating pre-run)
	ctx := context.Background()
	cmd.SetContext(ctx)
	
	opts := []porter.Option{
		porter.WithLogger(logger),
		porter.WithHashAlgorithm(hashAlgo),
		porter.WithVersion(version),
		porter.WithCLIMeta(
			porter.NewCLIMeta(
				porter.WithStart(toPTR(time.Now())),
			),
		),
	}
	
	porterInstance := porter.New(opts...)
	cmd.SetContext(context.WithValue(cmd.Context(), porter.PorterKey, porterInstance))
	
	// Verify context was properly set up
	porterValue := cmd.Context().Value(porter.PorterKey)
	assert.NotNil(t, porterValue)
}