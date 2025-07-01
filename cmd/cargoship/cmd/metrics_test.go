package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMetricsCmd(t *testing.T) {
	cmd := NewMetricsCmd()
	
	require.NotNil(t, cmd)
	assert.Equal(t, "metrics", cmd.Use)
	assert.Equal(t, "Test CloudWatch metrics integration", cmd.Short)
	assert.NotEmpty(t, cmd.Long)
	assert.NotNil(t, cmd.RunE)
	
	// Test flags are defined
	flags := cmd.Flags()
	
	namespaceFlag := flags.Lookup("namespace")
	require.NotNil(t, namespaceFlag)
	assert.Equal(t, "CargoShip/Test", namespaceFlag.DefValue)
	
	regionFlag := flags.Lookup("region")
	require.NotNil(t, regionFlag)
	assert.Equal(t, "us-west-2", regionFlag.DefValue)
	
	testFlag := flags.Lookup("test")
	require.NotNil(t, testFlag)
	assert.Equal(t, "false", testFlag.DefValue)
}

func TestRunMetricsWithoutTestFlag(t *testing.T) {
	// Test the validation error when --test flag is not provided
	cmd := NewMetricsCmd()
	
	// Execute without --test flag
	err := cmd.RunE(cmd, []string{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "use --test flag to send test metrics")
}

func TestRunMetricsWithTestFlag(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping AWS integration tests in short mode")
	}
	
	// This test requires AWS credentials and would hit CloudWatch
	// We test the validation and flag parsing instead
	cmd := NewMetricsCmd()
	
	// Set test flag
	err := cmd.Flags().Set("test", "true")
	require.NoError(t, err)
	
	// Set custom namespace and region
	err = cmd.Flags().Set("namespace", "CargoShip/TestSuite")
	require.NoError(t, err)
	
	err = cmd.Flags().Set("region", "us-east-1")
	require.NoError(t, err)
	
	// Verify flags can be retrieved
	test, err := cmd.Flags().GetBool("test")
	assert.NoError(t, err)
	assert.True(t, test)
	
	namespace, err := cmd.Flags().GetString("namespace")
	assert.NoError(t, err)
	assert.Equal(t, "CargoShip/TestSuite", namespace)
	
	region, err := cmd.Flags().GetString("region")
	assert.NoError(t, err)
	assert.Equal(t, "us-east-1", region)
	
	// The actual AWS call would fail without credentials, 
	// but we've verified the setup is correct
}

func TestMetricsCommandStructure(t *testing.T) {
	cmd := NewMetricsCmd()
	
	// Test that help text contains expected sections
	helpText := cmd.Long
	assert.Contains(t, helpText, "Test CloudWatch metrics integration")
	assert.Contains(t, helpText, "Examples:")
	assert.Contains(t, helpText, "cargoship metrics")
	assert.Contains(t, helpText, "--test")
	assert.Contains(t, helpText, "--namespace")
	assert.Contains(t, helpText, "--region")
}

func TestMetricsGlobalVariables(t *testing.T) {
	// Save original values
	originalNamespace := metricsNamespace
	originalRegion := metricsRegion
	originalTest := metricsTest
	
	defer func() {
		// Restore original values
		metricsNamespace = originalNamespace
		metricsRegion = originalRegion
		metricsTest = originalTest
	}()
	
	// Test setting values through command flags
	cmd := NewMetricsCmd()
	
	// Test namespace flag
	err := cmd.Flags().Set("namespace", "CargoShip/Production")
	require.NoError(t, err)
	
	err = cmd.Flags().Set("region", "eu-west-1")
	require.NoError(t, err)
	
	err = cmd.Flags().Set("test", "true")
	require.NoError(t, err)
	
	// Verify flags can be retrieved
	namespace, err := cmd.Flags().GetString("namespace")
	assert.NoError(t, err)
	assert.Equal(t, "CargoShip/Production", namespace)
	
	region, err := cmd.Flags().GetString("region")
	assert.NoError(t, err)
	assert.Equal(t, "eu-west-1", region)
	
	test, err := cmd.Flags().GetBool("test")
	assert.NoError(t, err)
	assert.True(t, test)
}

func TestMetricsCommandExamples(t *testing.T) {
	cmd := NewMetricsCmd()
	
	// Test that long description contains useful examples
	longDesc := cmd.Long
	assert.Contains(t, longDesc, "cargoship metrics --test")
	assert.Contains(t, longDesc, "CargoShip/Prod")
	assert.Contains(t, longDesc, "us-east-1")
}

func TestMetricsFlagTypes(t *testing.T) {
	cmd := NewMetricsCmd()
	flags := cmd.Flags()
	
	// Test namespace flag (string)
	namespaceFlag := flags.Lookup("namespace")
	require.NotNil(t, namespaceFlag)
	assert.Equal(t, "string", namespaceFlag.Value.Type())
	
	// Test region flag (string)
	regionFlag := flags.Lookup("region")
	require.NotNil(t, regionFlag)
	assert.Equal(t, "string", regionFlag.Value.Type())
	
	// Test test flag (bool)
	testFlag := flags.Lookup("test")
	require.NotNil(t, testFlag)
	assert.Equal(t, "bool", testFlag.Value.Type())
}

func TestMetricsInit(t *testing.T) {
	// Test that the metrics command can be created without panicking
	// The init function in metrics.go is empty, so we test command creation instead
	assert.NotPanics(t, func() {
		cmd := NewMetricsCmd()
		assert.NotNil(t, cmd)
	})
}

func TestMetricsFlagValidation(t *testing.T) {
	cmd := NewMetricsCmd()
	
	// Test invalid bool flag value
	err := cmd.Flags().Set("test", "invalid")
	assert.Error(t, err)
	
	// Test valid bool flag values
	err = cmd.Flags().Set("test", "true")
	assert.NoError(t, err)
	
	err = cmd.Flags().Set("test", "false")
	assert.NoError(t, err)
	
	// Test string flag values
	err = cmd.Flags().Set("namespace", "")
	assert.NoError(t, err) // Empty string is valid
	
	err = cmd.Flags().Set("region", "us-gov-west-1")
	assert.NoError(t, err)
}

func TestMetricsCommandValidation(t *testing.T) {
	// Test various flag combinations and validation
	testCases := []struct {
		name        string
		flags       map[string]string
		shouldError bool
		errorMsg    string
	}{
		{
			name:        "no test flag",
			flags:       map[string]string{},
			shouldError: true,
			errorMsg:    "use --test flag",
		},
		{
			name:        "test flag false",
			flags:       map[string]string{"test": "false"},
			shouldError: true,
			errorMsg:    "use --test flag",
		},
		{
			name:        "test flag true",
			flags:       map[string]string{"test": "true"},
			shouldError: false, // Would error on AWS call but validation passes
		},
		{
			name: "test with custom settings",
			flags: map[string]string{
				"test":      "true",
				"namespace": "Custom/Namespace",
				"region":    "ap-southeast-1",
			},
			shouldError: false, // Would error on AWS call but validation passes
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := NewMetricsCmd()
			
			// Set flags
			for flag, value := range tc.flags {
				err := cmd.Flags().Set(flag, value)
				require.NoError(t, err)
			}
			
			// For validation-only tests
			if tc.shouldError && tc.errorMsg == "use --test flag" {
				err := cmd.RunE(cmd, []string{})
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.errorMsg)
			}
			// AWS integration tests would require credentials
		})
	}
}

func TestMetricsRegions(t *testing.T) {
	// Test common AWS regions
	validRegions := []string{
		"us-east-1",
		"us-east-2", 
		"us-west-1",
		"us-west-2",
		"eu-west-1",
		"eu-central-1",
		"ap-southeast-1",
		"ap-northeast-1",
	}
	
	cmd := NewMetricsCmd()
	
	for _, region := range validRegions {
		err := cmd.Flags().Set("region", region)
		assert.NoError(t, err, "Should accept valid AWS region: %s", region)
		
		actualRegion, err := cmd.Flags().GetString("region")
		assert.NoError(t, err)
		assert.Equal(t, region, actualRegion)
	}
}

func TestMetricsNamespaces(t *testing.T) {
	// Test various CloudWatch namespace formats
	validNamespaces := []string{
		"CargoShip/Test",
		"CargoShip/Production",
		"MyCompany/CargoShip",
		"AWS/Custom",
		"Test123",
	}
	
	cmd := NewMetricsCmd()
	
	for _, namespace := range validNamespaces {
		err := cmd.Flags().Set("namespace", namespace)
		assert.NoError(t, err, "Should accept valid namespace: %s", namespace)
		
		actualNamespace, err := cmd.Flags().GetString("namespace")
		assert.NoError(t, err)
		assert.Equal(t, namespace, actualNamespace)
	}
}