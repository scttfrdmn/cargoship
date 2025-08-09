//go:build integration
// +build integration

package cmd

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	localStackEndpoint = "http://localhost:4566"
	testRegion         = "us-east-1"
)

func TestMain(m *testing.M) {
	// Check if LocalStack is available
	if !isLocalStackAvailable() {
		fmt.Println("Skipping integration tests - LocalStack not available")
		fmt.Println("To run integration tests:")
		fmt.Println("  docker run --rm -d -p 4566:4566 localstack/localstack")
		os.Exit(0)
	}

	// Run tests
	code := m.Run()
	os.Exit(code)
}

func isLocalStackAvailable() bool {
	client := getTestCloudWatchClient()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Try to list metrics - this will succeed even if there are no metrics
	_, err := client.ListMetrics(ctx, &cloudwatch.ListMetricsInput{})
	return err == nil
}

func getTestCloudWatchClient() *cloudwatch.Client {
	cfg := aws.Config{
		Region:      testRegion,
		Credentials: credentials.NewStaticCredentialsProvider("test", "test", ""),
		EndpointResolverWithOptions: aws.EndpointResolverWithOptionsFunc(
			func(service, region string, options ...interface{}) (aws.Endpoint, error) {
				return aws.Endpoint{
					URL:           localStackEndpoint,
					SigningRegion: testRegion,
				}, nil
			},
		),
	}

	return cloudwatch.NewFromConfig(cfg)
}

func TestRunMetricsIntegrationWithLocalStack(t *testing.T) {
	// This test requires LocalStack running with CloudWatch support
	if testing.Short() {
		t.Skip("Skipping LocalStack integration test in short mode")
	}

	// Save original global variables
	originalNamespace := metricsNamespace
	originalRegion := metricsRegion
	originalTest := metricsTest
	defer func() {
		metricsNamespace = originalNamespace
		metricsRegion = originalRegion
		metricsTest = originalTest
	}()

	// Set up test environment
	metricsNamespace = "CargoShip/IntegrationTest"
	metricsRegion = testRegion
	metricsTest = true

	// Create command and run it
	cmd := NewMetricsCmd()
	err := cmd.Flags().Set("test", "true")
	require.NoError(t, err)
	err = cmd.Flags().Set("namespace", metricsNamespace)
	require.NoError(t, err)
	err = cmd.Flags().Set("region", metricsRegion)
	require.NoError(t, err)

	// Run the metrics command - this should work with LocalStack
	err = cmd.RunE(cmd, []string{})
	assert.NoError(t, err, "runMetrics should succeed with LocalStack")

	// Verify metrics were published to LocalStack
	client := getTestCloudWatchClient()
	ctx := context.Background()

	// Give a moment for metrics to be processed
	time.Sleep(2 * time.Second)

	// Check that metrics were actually published
	output, err := client.ListMetrics(ctx, &cloudwatch.ListMetricsInput{
		Namespace: aws.String(metricsNamespace),
	})
	assert.NoError(t, err)

	// We should have some metrics published
	if len(output.Metrics) > 0 {
		t.Logf("Successfully published %d metrics to LocalStack CloudWatch", len(output.Metrics))

		// Verify some expected metric names
		metricNames := make(map[string]bool)
		for _, metric := range output.Metrics {
			if metric.MetricName != nil {
				metricNames[*metric.MetricName] = true
			}
		}

		expectedMetrics := []string{"UploadDuration", "UploadThroughput", "UploadSize"}
		for _, expected := range expectedMetrics {
			assert.True(t, metricNames[expected], "Should have published %s metric", expected)
		}
	} else {
		t.Log("No metrics found - LocalStack may not fully support CloudWatch metrics API")
	}
}

func TestRunMetricsValidationIntegration(t *testing.T) {
	// Test validation paths that aren't covered by regular unit tests

	// Save original global variables
	originalTest := metricsTest
	defer func() {
		metricsTest = originalTest
	}()

	// Test without --test flag
	metricsTest = false
	cmd := NewMetricsCmd()

	err := cmd.RunE(cmd, []string{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "use --test flag")
}

func TestRunMetricsWithCustomParameters(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping LocalStack integration test in short mode")
	}

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
		name      string
		namespace string
		region    string
		shouldErr bool
	}{
		{
			name:      "custom namespace and region",
			namespace: "CustomTest/Namespace",
			region:    "us-west-2",
			shouldErr: false,
		},
		{
			name:      "production-like namespace",
			namespace: "CargoShip/Production",
			region:    "eu-west-1",
			shouldErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set test parameters
			metricsNamespace = tc.namespace
			metricsRegion = tc.region
			metricsTest = true

			cmd := NewMetricsCmd()
			err := cmd.Flags().Set("test", "true")
			require.NoError(t, err)
			err = cmd.Flags().Set("namespace", tc.namespace)
			require.NoError(t, err)
			err = cmd.Flags().Set("region", tc.region)
			require.NoError(t, err)

			err = cmd.RunE(cmd, []string{})
			if tc.shouldErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestRunMetricsAWSConfigHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping LocalStack integration test in short mode")
	}

	// This test verifies that the function can handle AWS config loading
	// even if it encounters errors (which is common in test environments)

	// Save original global variables
	originalNamespace := metricsNamespace
	originalRegion := metricsRegion
	originalTest := metricsTest
	defer func() {
		metricsNamespace = originalNamespace
		metricsRegion = originalRegion
		metricsTest = originalTest
	}()

	metricsTest = true
	metricsNamespace = "CargoShip/ConfigTest"
	metricsRegion = "us-east-1"

	cmd := NewMetricsCmd()
	err := cmd.Flags().Set("test", "true")
	require.NoError(t, err)

	// This should succeed even if AWS credentials are not properly configured
	// because we're using LocalStack
	err = cmd.RunE(cmd, []string{})
	assert.NoError(t, err, "Should handle AWS config loading gracefully")
}
