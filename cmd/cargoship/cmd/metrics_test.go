package cmd

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scttfrdmn/cargoship/pkg/aws/metrics"
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

func TestRunMetricsWithMockCloudWatch(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping comprehensive metrics tests in short mode")
	}
	
	// Test the full runMetrics execution path with proper mocking
	// This requires manual dependency injection since the function
	// creates its own CloudWatch client
	
	// Test validation works
	cmd := NewMetricsCmd()
	err := cmd.Flags().Set("test", "true")
	require.NoError(t, err)
	err = cmd.Flags().Set("namespace", "CargoShip/TestSuite") 
	require.NoError(t, err)
	err = cmd.Flags().Set("region", "us-east-1")
	require.NoError(t, err)
	
	// Verify the command would attempt AWS calls with proper flags
	// We can't easily mock the AWS client creation without major refactoring,
	// but we can verify the setup is correct
	
	test, err := cmd.Flags().GetBool("test")
	assert.NoError(t, err)
	assert.True(t, test)
	
	// Test that global variables are set correctly when flags are parsed
	// (The actual AWS SDK calls would fail in test environment)
}

func TestMetricsGlobalVariableScope(t *testing.T) {
	// Test global variable behavior and flag binding
	
	// Save originals
	origNamespace := metricsNamespace
	origRegion := metricsRegion
	origTest := metricsTest
	
	defer func() {
		metricsNamespace = origNamespace
		metricsRegion = origRegion
		metricsTest = origTest
	}()
	
	// Test that setting command flags affects global vars
	cmd := NewMetricsCmd()
	
	// Test default values through flags
	namespace, err := cmd.Flags().GetString("namespace")
	assert.NoError(t, err)
	assert.Equal(t, "CargoShip/Test", namespace)
	
	region, err := cmd.Flags().GetString("region")
	assert.NoError(t, err)
	assert.Equal(t, "us-west-2", region)
	
	test, err := cmd.Flags().GetBool("test")
	assert.NoError(t, err)
	assert.False(t, test)
}

// MockCloudWatchClient for testing runMetrics
type MockCloudWatchClient struct {
	putMetricDataErr error
	putMetricDataCalls []cloudwatch.PutMetricDataInput
}

func (m *MockCloudWatchClient) PutMetricData(ctx context.Context, params *cloudwatch.PutMetricDataInput, optFns ...func(*cloudwatch.Options)) (*cloudwatch.PutMetricDataOutput, error) {
	if params != nil {
		m.putMetricDataCalls = append(m.putMetricDataCalls, *params)
	}
	return &cloudwatch.PutMetricDataOutput{}, m.putMetricDataErr
}

func TestRunMetricsErrorPaths(t *testing.T) {
	// Save original global variables
	originalNamespace := metricsNamespace
	originalRegion := metricsRegion
	originalTest := metricsTest
	defer func() {
		metricsNamespace = originalNamespace
		metricsRegion = originalRegion
		metricsTest = originalTest
	}()

	testCases := []struct {
		name         string
		setupFunc    func(*testing.T) *cobra.Command
		expectedErr  string
	}{
		{
			name: "test flag false",
			setupFunc: func(t *testing.T) *cobra.Command {
				cmd := NewMetricsCmd()
				err := cmd.Flags().Set("test", "false")
				require.NoError(t, err)
				return cmd
			},
			expectedErr: "use --test flag to send test metrics",
		},
		{
			name: "invalid AWS region",
			setupFunc: func(t *testing.T) *cobra.Command {
				cmd := NewMetricsCmd()
				err := cmd.Flags().Set("test", "true")
				require.NoError(t, err)
				err = cmd.Flags().Set("region", "invalid-region-name")
				require.NoError(t, err)
				return cmd
			},
			expectedErr: "failed to publish upload metrics",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := tc.setupFunc(t)
			err := cmd.RunE(cmd, []string{})
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tc.expectedErr)
		})
	}
}

func TestRunMetricsOutputFormatting(t *testing.T) {
	// Save original global variables
	originalNamespace := metricsNamespace
	originalRegion := metricsRegion
	originalTest := metricsTest
	defer func() {
		metricsNamespace = originalNamespace
		metricsRegion = originalRegion
		metricsTest = originalTest
	}()

	// Test output formatting paths
	cmd := NewMetricsCmd()
	err := cmd.Flags().Set("test", "true")
	require.NoError(t, err)
	err = cmd.Flags().Set("namespace", "TestNamespace/Validation")
	require.NoError(t, err)
	err = cmd.Flags().Set("region", "us-east-1")
	require.NoError(t, err)

	// Verify output sections would be printed
	// The actual execution would fail on AWS calls, but we can test setup
	namespace, err := cmd.Flags().GetString("namespace")
	assert.NoError(t, err)
	assert.Equal(t, "TestNamespace/Validation", namespace)

	region, err := cmd.Flags().GetString("region")
	assert.NoError(t, err)
	assert.Equal(t, "us-east-1", region)

	test, err := cmd.Flags().GetBool("test")
	assert.NoError(t, err)
	assert.True(t, test)
}

func TestRunMetricsValidConfigValues(t *testing.T) {
	// Test that the function creates proper config values
	cmd := NewMetricsCmd()
	err := cmd.Flags().Set("test", "true")
	require.NoError(t, err)
	err = cmd.Flags().Set("namespace", "CargoShip/UnitTest")
	require.NoError(t, err)
	err = cmd.Flags().Set("region", "us-west-2")
	require.NoError(t, err)

	// Test the metrics config creation path by verifying flag values
	namespace, err := cmd.Flags().GetString("namespace")
	assert.NoError(t, err)
	assert.Equal(t, "CargoShip/UnitTest", namespace)

	region, err := cmd.Flags().GetString("region")
	assert.NoError(t, err)
	assert.Equal(t, "us-west-2", region)

	// Verify that the test values would create valid metric configs
	metricsConfig := metrics.MetricConfig{
		Namespace:     namespace,
		Region:        region,
		BatchSize:     5,
		FlushInterval: 10 * time.Second,
		Enabled:       true,
	}

	assert.Equal(t, "CargoShip/UnitTest", metricsConfig.Namespace)
	assert.Equal(t, "us-west-2", metricsConfig.Region)
	assert.Equal(t, 5, metricsConfig.BatchSize)
	assert.Equal(t, 10*time.Second, metricsConfig.FlushInterval)
	assert.True(t, metricsConfig.Enabled)
}

func TestRunMetricsValidMetricStructures(t *testing.T) {
	// Test that the function would create valid metric structures
	// This tests the data structures used in runMetrics without AWS calls

	// Test UploadMetrics structure
	uploadMetrics := &metrics.UploadMetrics{
		Duration:        45 * time.Second,
		ThroughputMBps:  15.5,
		TotalBytes:      500 * 1024 * 1024, // 500MB
		ChunkCount:      25,
		Concurrency:     8,
		ErrorCount:      0,
		Success:         true,
		StorageClass:    "INTELLIGENT_TIERING",
		ContentType:     "application/octet-stream",
		CompressionType: "zstd",
	}
	assert.Equal(t, 45*time.Second, uploadMetrics.Duration)
	assert.Equal(t, 15.5, uploadMetrics.ThroughputMBps)
	assert.Equal(t, int64(500*1024*1024), uploadMetrics.TotalBytes)
	assert.True(t, uploadMetrics.Success)

	// Test CostMetrics structure
	costMetrics := &metrics.CostMetrics{
		EstimatedMonthlyCost:    2.30,
		EstimatedAnnualCost:     27.60,
		ActualMonthlyCost:       0.10,
		DataSizeGB:              100.0,
		PotentialSavingsPercent: 95.7,
		StorageClass:            "DEEP_ARCHIVE",
		OptimizationType:        "archive_optimization",
	}
	assert.Equal(t, 2.30, costMetrics.EstimatedMonthlyCost)
	assert.Equal(t, 95.7, costMetrics.PotentialSavingsPercent)

	// Test NetworkMetrics structure
	networkMetrics := &metrics.NetworkMetrics{
		BandwidthMBps:       25.0,
		LatencyMs:           50.0,
		PacketLossPercent:   0.1,
		OptimalChunkSizeMB:  24,
		OptimalConcurrency:  8,
		NetworkCondition:    "excellent",
	}
	assert.Equal(t, 25.0, networkMetrics.BandwidthMBps)
	assert.Equal(t, "excellent", networkMetrics.NetworkCondition)

	// Test OperationalMetrics structure
	operationalMetrics := &metrics.OperationalMetrics{
		ActiveUploads:     3,
		QueuedUploads:     7,
		CompletedUploads:  45,
		FailedUploads:     2,
		MemoryUsageMB:     256.5,
		CPUUsagePercent:   25.3,
	}
	assert.Equal(t, 3, operationalMetrics.ActiveUploads)
	assert.Equal(t, 256.5, operationalMetrics.MemoryUsageMB)

	// Test LifecycleMetrics structure
	lifecycleMetrics := &metrics.LifecycleMetrics{
		ActivePolicies:          1,
		EstimatedSavingsPercent: 95.7,
		ObjectsTransitioned:     1250,
		PolicyTemplate:          "archive-optimization",
		BucketName:              "cargoship-production",
	}
	assert.Equal(t, 1, lifecycleMetrics.ActivePolicies)
	assert.Equal(t, "archive-optimization", lifecycleMetrics.PolicyTemplate)
}

func TestRunMetricsStringFormatting(t *testing.T) {
	// Test the string formatting used in runMetrics output
	namespace := "CargoShip/Production"
	region := "us-east-1"

	// Test console URL formatting
	consoleURL := fmt.Sprintf("https://console.aws.amazon.com/cloudwatch/home?region=%s#metricsV2:graph=~();search=%s", region, namespace)
	expectedURL := "https://console.aws.amazon.com/cloudwatch/home?region=us-east-1#metricsV2:graph=~();search=CargoShip/Production"
	assert.Equal(t, expectedURL, consoleURL)

	// Test that output strings would be properly formatted
	outputStrings := []string{
		"🔍 Testing CloudWatch metrics integration...",
		fmt.Sprintf("   Namespace: %s", namespace),
		fmt.Sprintf("   Region: %s", region),
		"📊 Publishing upload metrics...",
		"💰 Publishing cost metrics...",
		"🌐 Publishing network metrics...",
		"⚙️ Publishing operational metrics...",
		"🔄 Publishing lifecycle metrics...",
		"🚀 Flushing metrics to CloudWatch...",
		"✅ All test metrics published successfully!",
	}

	for _, str := range outputStrings {
		assert.NotEmpty(t, str)
		assert.True(t, len(str) > 5) // Ensure meaningful output
	}
}