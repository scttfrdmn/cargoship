package cmd

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"

	cargoconfig "github.com/scttfrdmn/cargoship/pkg/aws/config"
)

// TestParseSize tests the parseSize helper function
func TestParseSize(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int64
		wantErr bool
	}{
		// Basic units
		{
			name:  "bytes",
			input: "1024B",
			want:  1024,
		},
		{
			name:  "kilobytes",
			input: "1KB",
			want:  1024,
		},
		{
			name:  "megabytes",
			input: "1MB",
			want:  1024 * 1024,
		},
		{
			name:  "gigabytes",
			input: "1GB",
			want:  1024 * 1024 * 1024,
		},
		{
			name:  "terabytes",
			input: "1TB",
			want:  1024 * 1024 * 1024 * 1024,
		},

		// Decimal values
		{
			name:  "decimal gigabytes",
			input: "1.5GB",
			want:  int64(1.5 * 1024 * 1024 * 1024),
		},
		{
			name:  "decimal megabytes",
			input: "100.5MB",
			want:  int64(100.5 * 1024 * 1024),
		},
		{
			name:  "small decimal",
			input: "0.5GB",
			want:  int64(0.5 * 1024 * 1024 * 1024),
		},

		// Large values
		{
			name:  "large gigabytes",
			input: "500GB",
			want:  500 * 1024 * 1024 * 1024,
		},
		{
			name:  "multiple terabytes",
			input: "10TB",
			want:  10 * 1024 * 1024 * 1024 * 1024,
		},

		// Case insensitivity
		{
			name:  "lowercase gb",
			input: "100gb",
			want:  100 * 1024 * 1024 * 1024,
		},
		{
			name:  "mixed case MB",
			input: "50Mb",
			want:  50 * 1024 * 1024,
		},

		// Whitespace handling
		{
			name:  "leading whitespace",
			input: "  100GB",
			want:  100 * 1024 * 1024 * 1024,
		},
		{
			name:  "trailing whitespace",
			input: "100GB  ",
			want:  100 * 1024 * 1024 * 1024,
		},
		{
			name:  "both whitespace",
			input: "  100GB  ",
			want:  100 * 1024 * 1024 * 1024,
		},

		// No unit (bytes assumed)
		{
			name:  "no unit",
			input: "1024",
			want:  1024,
		},

		// Negative values (currently accepted by parseFloat)
		{
			name:  "negative gigabytes",
			input: "-100GB",
			want:  -100 * 1024 * 1024 * 1024,
		},

		// Error cases
		{
			name:    "invalid number",
			input:   "notanumberGB",
			wantErr: true,
		},
		{
			name:    "invalid unit",
			input:   "100XB",
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSize(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseSize(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("parseSize(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

// TestParseStorageClass tests the parseStorageClass helper function
func TestParseStorageClass(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    cargoconfig.StorageClass
		wantErr bool
	}{
		// Valid storage classes
		{
			name:  "STANDARD",
			input: "STANDARD",
			want:  cargoconfig.StorageClassStandard,
		},
		{
			name:  "STANDARD_IA",
			input: "STANDARD_IA",
			want:  cargoconfig.StorageClassStandardIA,
		},
		{
			name:  "ONEZONE_IA",
			input: "ONEZONE_IA",
			want:  cargoconfig.StorageClassOneZoneIA,
		},
		{
			name:  "INTELLIGENT_TIERING",
			input: "INTELLIGENT_TIERING",
			want:  cargoconfig.StorageClassIntelligentTiering,
		},
		{
			name:  "GLACIER",
			input: "GLACIER",
			want:  cargoconfig.StorageClassGlacier,
		},
		{
			name:  "DEEP_ARCHIVE",
			input: "DEEP_ARCHIVE",
			want:  cargoconfig.StorageClassDeepArchive,
		},

		// Case insensitivity
		{
			name:  "lowercase standard",
			input: "standard",
			want:  cargoconfig.StorageClassStandard,
		},
		{
			name:  "mixed case standard_ia",
			input: "Standard_IA",
			want:  cargoconfig.StorageClassStandardIA,
		},
		{
			name:  "lowercase glacier",
			input: "glacier",
			want:  cargoconfig.StorageClassGlacier,
		},

		// Error cases
		{
			name:    "invalid class",
			input:   "INVALID_CLASS",
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "random text",
			input:   "not_a_storage_class",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseStorageClass(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseStorageClass(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("parseStorageClass(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestMakeHeader tests the makeHeader helper function
func TestMakeHeader(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "simple header",
			input: "Cost Report",
			want:  "\033[1mCost Report\033[0m",
		},
		{
			name:  "header with spaces",
			input: "Budget Status Report",
			want:  "\033[1mBudget Status Report\033[0m",
		},
		{
			name:  "header with numbers",
			input: "30-Day Forecast",
			want:  "\033[1m30-Day Forecast\033[0m",
		},
		{
			name:  "header with special chars",
			input: "Cost Breakdown ($)",
			want:  "\033[1mCost Breakdown ($)\033[0m",
		},
		{
			name:  "empty string",
			input: "",
			want:  "\033[1m\033[0m",
		},
		{
			name:  "unicode header",
			input: "💰 Budget",
			want:  "\033[1m💰 Budget\033[0m",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := makeHeader(tt.input)
			if got != tt.want {
				t.Errorf("makeHeader(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestMakeHeader_ContainsBoldFormatting verifies that makeHeader produces bold ANSI codes
func TestMakeHeader_ContainsBoldFormatting(t *testing.T) {
	result := makeHeader("Test")

	// Should contain ANSI bold start code
	if !strings.Contains(result, "\033[1m") {
		t.Error("makeHeader() should contain ANSI bold start code \\033[1m")
	}

	// Should contain ANSI reset code
	if !strings.Contains(result, "\033[0m") {
		t.Error("makeHeader() should contain ANSI reset code \\033[0m")
	}

	// Should contain the input text
	if !strings.Contains(result, "Test") {
		t.Error("makeHeader() should contain the input text")
	}
}

// TestNewCostCmd_Structure tests command construction and hierarchy
func TestNewCostCmd_Structure(t *testing.T) {
	cmd := NewCostCmd()

	// Verify command name
	if cmd.Use != "cost" {
		t.Errorf("expected command name 'cost', got %q", cmd.Use)
	}

	// Verify command has short description
	if cmd.Short == "" {
		t.Error("command should have a short description")
	}

	// Verify command has long description
	if cmd.Long == "" {
		t.Error("command should have a long description")
	}

	// Verify expected subcommands exist
	expectedSubcommands := []string{
		"estimate",
		"upload",
		"budget",
		"pricing",
		"report",
		"projects",
		"project",
		"forecast",
		"burnrate",
		"exhaustion",
		"benchmark-compare",
		"summary", // Issue #186: DVC stage / git-commit cost aggregation
	}

	commands := cmd.Commands()
	commandNames := make(map[string]bool)
	for _, c := range commands {
		commandNames[c.Name()] = true
	}

	for _, expected := range expectedSubcommands {
		if !commandNames[expected] {
			t.Errorf("expected subcommand %q not found", expected)
		}
	}

	// Verify we have the expected number of subcommands
	if len(commands) != len(expectedSubcommands) {
		t.Errorf("expected %d subcommands, got %d", len(expectedSubcommands), len(commands))
	}
}

// TestNewCostCmd_PersistentFlags tests global flags
func TestNewCostCmd_PersistentFlags(t *testing.T) {
	cmd := NewCostCmd()

	// Test region flag
	regionFlag := cmd.PersistentFlags().Lookup("region")
	if regionFlag == nil {
		t.Fatal("expected 'region' flag to exist")
	}
	if regionFlag.Shorthand != "r" {
		t.Errorf("expected region shorthand 'r', got %q", regionFlag.Shorthand)
	}
	if regionFlag.DefValue != "us-west-2" {
		t.Errorf("expected region default 'us-west-2', got %q", regionFlag.DefValue)
	}

	// Test json flag
	jsonFlag := cmd.PersistentFlags().Lookup("json")
	if jsonFlag == nil {
		t.Fatal("expected 'json' flag to exist")
	}
	if jsonFlag.DefValue != "false" {
		t.Errorf("expected json default 'false', got %q", jsonFlag.DefValue)
	}
}

// TestEstimateCmd_Flags tests estimate subcommand flags
func TestEstimateCmd_Flags(t *testing.T) {
	cmd := NewCostCmd()

	// Find estimate subcommand
	var estimateCmd *cobra.Command
	for _, c := range cmd.Commands() {
		if c.Name() == "estimate" {
			estimateCmd = c
			break
		}
	}

	if estimateCmd == nil {
		t.Fatal("estimate subcommand not found")
	}

	// Verify flags exist
	flags := []struct {
		name     string
		required bool
	}{
		{"size", true},
		{"storage-class", false},
		{"operation", false},
	}

	for _, flag := range flags {
		f := estimateCmd.Flags().Lookup(flag.name)
		if f == nil {
			t.Errorf("expected flag %q to exist", flag.name)
			continue
		}

		// Note: We can't directly test required flags without executing the command
		// but we can verify they exist
	}

	// Verify help text
	if estimateCmd.Short == "" {
		t.Error("estimate command should have short description")
	}
	if estimateCmd.Long == "" {
		t.Error("estimate command should have long description")
	}
}

// TestForecastCmd_Structure tests forecast subcommand structure (Issue #147 Phase 6)
func TestForecastCmd_Structure(t *testing.T) {
	cmd := NewCostCmd()

	// Find forecast subcommand
	var forecastCmd *cobra.Command
	for _, c := range cmd.Commands() {
		if c.Name() == "forecast" {
			forecastCmd = c
			break
		}
	}

	if forecastCmd == nil {
		t.Fatal("forecast subcommand not found")
	}

	// Verify command accepts optional project ID argument
	if forecastCmd.Args == nil {
		t.Error("forecast command should have args validation")
	}

	// Verify flags
	modelFlag := forecastCmd.Flags().Lookup("model")
	if modelFlag == nil {
		t.Fatal("expected 'model' flag to exist")
	}
	if modelFlag.DefValue != "linear" {
		t.Errorf("expected model default 'linear', got %q", modelFlag.DefValue)
	}

	daysFlag := forecastCmd.Flags().Lookup("days")
	if daysFlag == nil {
		t.Fatal("expected 'days' flag to exist")
	}
	if daysFlag.DefValue != "90" {
		t.Errorf("expected days default '90', got %q", daysFlag.DefValue)
	}

	// Verify help text
	if forecastCmd.Short == "" {
		t.Error("forecast command should have short description")
	}
	if !strings.Contains(forecastCmd.Short, "ML-based") {
		t.Error("forecast command short description should mention ML-based")
	}
	if !strings.Contains(forecastCmd.Long, "forecast") {
		t.Error("forecast command long description should mention forecasting")
	}
}

// TestBurnrateCmd_Structure tests burnrate subcommand structure (Issue #147 Phase 6)
func TestBurnrateCmd_Structure(t *testing.T) {
	cmd := NewCostCmd()

	// Find burnrate subcommand
	var burnrateCmd *cobra.Command
	for _, c := range cmd.Commands() {
		if c.Name() == "burnrate" {
			burnrateCmd = c
			break
		}
	}

	if burnrateCmd == nil {
		t.Fatal("burnrate subcommand not found")
	}

	// Verify command accepts optional project ID argument
	if burnrateCmd.Args == nil {
		t.Error("burnrate command should have args validation")
	}

	// Verify flags
	daysFlag := burnrateCmd.Flags().Lookup("days")
	if daysFlag == nil {
		t.Fatal("expected 'days' flag to exist")
	}
	if daysFlag.DefValue != "90" {
		t.Errorf("expected days default '90', got %q", daysFlag.DefValue)
	}

	// Verify help text
	if burnrateCmd.Short == "" {
		t.Error("burnrate command should have short description")
	}
	if !strings.Contains(burnrateCmd.Long, "burn rate") || !strings.Contains(burnrateCmd.Long, "trend") {
		t.Error("burnrate command long description should mention burn rate and trend")
	}
}

// TestExhaustionCmd_Structure tests exhaustion subcommand structure (Issue #147 Phase 6)
func TestExhaustionCmd_Structure(t *testing.T) {
	cmd := NewCostCmd()

	// Find exhaustion subcommand
	var exhaustionCmd *cobra.Command
	for _, c := range cmd.Commands() {
		if c.Name() == "exhaustion" {
			exhaustionCmd = c
			break
		}
	}

	if exhaustionCmd == nil {
		t.Fatal("exhaustion subcommand not found")
	}

	// Verify command accepts optional project ID argument
	if exhaustionCmd.Args == nil {
		t.Error("exhaustion command should have args validation")
	}

	// Verify flags
	budgetFlag := exhaustionCmd.Flags().Lookup("budget")
	if budgetFlag == nil {
		t.Fatal("expected 'budget' flag to exist")
	}

	spentFlag := exhaustionCmd.Flags().Lookup("spent")
	if spentFlag == nil {
		t.Fatal("expected 'spent' flag to exist")
	}
	if spentFlag.DefValue != "0" {
		t.Errorf("expected spent default '0', got %q", spentFlag.DefValue)
	}

	// Verify help text
	if exhaustionCmd.Short == "" {
		t.Error("exhaustion command should have short description")
	}
	if !strings.Contains(exhaustionCmd.Long, "budget") || !strings.Contains(exhaustionCmd.Long, "exhausted") {
		t.Error("exhaustion command long description should mention budget exhaustion")
	}
}
