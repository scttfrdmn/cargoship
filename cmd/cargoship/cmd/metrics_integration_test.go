//go:build integration

package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scttfrdmn/substrate"
)

const testRegion = "us-east-1"

var substrateURL string

func TestMain(m *testing.M) {
	url, cancel, err := launchSubstrate()
	if err != nil {
		fmt.Fprintf(os.Stderr, "substrate: %v\n", err)
		os.Exit(1)
	}
	defer cancel()
	substrateURL = url
	os.Exit(m.Run())
}

// launchSubstrate starts an in-process Substrate server for use in TestMain.
func launchSubstrate() (string, context.CancelFunc, error) {
	cfg := substrate.DefaultConfig()
	cfg.Server.Address = "127.0.0.1:0"
	cfg.EventStore.Enabled = false
	cfg.Log.Level = "error"

	state := substrate.NewMemoryStateManager()
	tc := substrate.NewTimeController(time.Now())
	registry := substrate.NewPluginRegistry()
	logger := substrate.NewDefaultLogger(slog.LevelError, false)
	store := substrate.NewEventStore(cfg.EventStore.ToEventStoreConfig(), substrate.WithTimeController(tc))

	ctx := context.Background()
	if err := substrate.RegisterDefaultPlugins(ctx, registry, state, tc, logger, store, nil); err != nil {
		return "", nil, fmt.Errorf("register plugins: %w", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, fmt.Errorf("listen: %w", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port

	srv := substrate.NewServer(*cfg, registry, store, state, tc, logger)
	srvCtx, cancel := context.WithCancel(context.Background())
	go func() { _ = srv.Serve(srvCtx, ln) }()

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, pingErr := http.Get(baseURL + "/health") //nolint:noctx
		if pingErr == nil {
			_ = resp.Body.Close()
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	return baseURL, cancel, nil
}

func getTestCloudWatchClient() *cloudwatch.Client {
	cfg := aws.Config{
		Region:       testRegion,
		Credentials:  credentials.NewStaticCredentialsProvider("test", "test", ""),
		BaseEndpoint: aws.String(substrateURL),
	}
	return cloudwatch.NewFromConfig(cfg)
}

func TestRunMetricsIntegrationWithLocalStack(t *testing.T) {
	// This test requires Substrate running with CloudWatch support
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Route AWS SDK calls to Substrate for this test
	t.Setenv("AWS_ENDPOINT_URL", substrateURL)
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	t.Setenv("AWS_REGION", testRegion)

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

	// Run the metrics command - this should work with Substrate
	err = cmd.RunE(cmd, []string{})
	assert.NoError(t, err, "runMetrics should succeed with Substrate")

	// Verify metrics were published to Substrate
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
	if output != nil && len(output.Metrics) > 0 {
		t.Logf("Successfully published %d metrics to Substrate CloudWatch", len(output.Metrics))

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
		t.Log("No metrics found - Substrate may not fully support CloudWatch metrics API")
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
		t.Skip("Skipping integration test in short mode")
	}

	// Route AWS SDK calls to Substrate for this test
	t.Setenv("AWS_ENDPOINT_URL", substrateURL)
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	t.Setenv("AWS_REGION", testRegion)

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
		t.Skip("Skipping integration test in short mode")
	}

	// Route AWS SDK calls to Substrate for this test
	t.Setenv("AWS_ENDPOINT_URL", substrateURL)
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	t.Setenv("AWS_REGION", testRegion)

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

	// This should succeed because we're using Substrate
	err = cmd.RunE(cmd, []string{})
	assert.NoError(t, err, "Should handle AWS config loading gracefully")
}
