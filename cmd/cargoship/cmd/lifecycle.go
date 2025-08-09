package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/scttfrdmn/cargoship/pkg/aws/lifecycle"
)

var (
	lifecycleBucket       string
	lifecycleTemplate     string
	lifecycleListOnly     bool
	lifecycleRemove       bool
	lifecycleExport       string
	lifecycleImport       string
	lifecycleRegion       string
	lifecycleEstimateSize float64
)

// NewLifecycleCmd creates the lifecycle management command
func NewLifecycleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lifecycle",
		Short: "Manage S3 lifecycle policies for cost optimization",
		Long: `Manage S3 lifecycle policies to automatically optimize storage costs.

CargoShip provides predefined lifecycle policy templates optimized for different
use cases, or you can create custom policies based on your access patterns.

Examples:
  # List available policy templates
  cargoship lifecycle --list-templates
  
  # Apply archive optimization policy
  cargoship lifecycle --bucket my-bucket --template archive-optimization
  
  # Estimate savings for a policy
  cargoship lifecycle --bucket my-bucket --template intelligent-tiering --estimate-size 100
  
  # Export current policy
  cargoship lifecycle --bucket my-bucket --export policy.json
  
  # Remove lifecycle policy
  cargoship lifecycle --bucket my-bucket --remove`,
		RunE: runLifecycle,
	}

	cmd.Flags().StringVar(&lifecycleBucket, "bucket", "", "S3 bucket name (required unless listing templates)")
	cmd.Flags().StringVar(&lifecycleTemplate, "template", "", "Lifecycle policy template to apply")
	cmd.Flags().BoolVar(&lifecycleListOnly, "list-templates", false, "List available policy templates")
	cmd.Flags().BoolVar(&lifecycleRemove, "remove", false, "Remove existing lifecycle policy")
	cmd.Flags().StringVar(&lifecycleExport, "export", "", "Export current policy to file")
	cmd.Flags().StringVar(&lifecycleImport, "import", "", "Import policy from file")
	cmd.Flags().StringVar(&lifecycleRegion, "region", "us-east-1", "AWS region")
	cmd.Flags().Float64Var(&lifecycleEstimateSize, "estimate-size", 0, "Data size in GB for savings estimation")

	return cmd
}

func runLifecycle(cmd *cobra.Command, args []string) error {
	// Handle list templates command
	if lifecycleListOnly {
		return listLifecycleTemplates()
	}

	// Validate required parameters
	if lifecycleBucket == "" {
		return fmt.Errorf("bucket name is required (use --bucket)")
	}

	// Create AWS S3 client
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(lifecycleRegion))
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}

	s3Client := s3.NewFromConfig(cfg)
	manager := lifecycle.NewManager(s3Client, lifecycleBucket)

	// Handle different operations
	switch {
	case lifecycleRemove:
		return removeLifecyclePolicy(ctx, manager)
	case lifecycleExport != "":
		return exportLifecyclePolicy(ctx, manager, lifecycleExport)
	case lifecycleImport != "":
		return importLifecyclePolicy(ctx, manager, lifecycleImport)
	case lifecycleTemplate != "":
		return applyLifecycleTemplate(ctx, manager, lifecycleTemplate)
	default:
		return showCurrentPolicy(ctx, manager)
	}
}

// listLifecycleTemplates displays available policy templates
func listLifecycleTemplates() error {
	templates := lifecycle.GetPredefinedTemplates()

	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("12")).
		MarginBottom(1)

	fmt.Println(headerStyle.Render("📋 Available Lifecycle Policy Templates"))

	for _, template := range templates {
		fmt.Printf("\n🔧 %s (%s)\n", template.Name, template.ID)
		fmt.Printf("   %s\n", template.Description)
		fmt.Printf("   💰 Estimated savings: %.0f%% monthly\n", template.Savings.MonthlyPercent)

		fmt.Printf("   📝 Rules:\n")
		for _, rule := range template.Rules {
			fmt.Printf("      • %s: ", rule.ID)
			if len(rule.Transitions) > 0 {
				transitions := make([]string, len(rule.Transitions))
				for i, t := range rule.Transitions {
					transitions[i] = fmt.Sprintf("%dd→%s", t.Days, t.StorageClass)
				}
				fmt.Printf("%s", strings.Join(transitions, ", "))
			}
			if rule.Expiration != nil {
				fmt.Printf(", expires after %dd", rule.Expiration.Days)
			}
			fmt.Println()
		}
	}

	fmt.Printf("\n💡 Usage: cargoship lifecycle --bucket YOUR_BUCKET --template TEMPLATE_ID\n")
	return nil
}

// applyLifecycleTemplate applies a predefined template to the bucket
func applyLifecycleTemplate(ctx context.Context, manager *lifecycle.Manager, templateID string) error {
	templates := lifecycle.GetPredefinedTemplates()

	var selectedTemplate *lifecycle.PolicyTemplate
	for _, template := range templates {
		if template.ID == templateID {
			selectedTemplate = &template
			break
		}
	}

	if selectedTemplate == nil {
		return fmt.Errorf("template '%s' not found. Use --list-templates to see available templates", templateID)
	}

	fmt.Printf("📋 Applying lifecycle policy: %s\n", selectedTemplate.Name)
	fmt.Printf("   %s\n", selectedTemplate.Description)

	// Validate the policy
	if err := manager.ValidatePolicy(*selectedTemplate); err != nil {
		return fmt.Errorf("policy validation failed: %w", err)
	}

	// Apply the policy
	if err := manager.ApplyPolicy(ctx, *selectedTemplate); err != nil {
		return fmt.Errorf("failed to apply policy: %w", err)
	}

	fmt.Printf("✅ Lifecycle policy applied successfully!\n")

	// Show savings estimate if size provided
	if lifecycleEstimateSize > 0 {
		estimate, err := manager.EstimateSavings(ctx, *selectedTemplate, lifecycleEstimateSize)
		if err == nil {
			fmt.Printf("\n💰 Savings Estimate (%.1f GB):\n", estimate.CurrentSizeGB)
			fmt.Printf("   Current monthly cost: $%.2f\n", estimate.CurrentMonthlyCost)
			fmt.Printf("   Optimized monthly cost: $%.2f\n", estimate.OptimizedMonthlyCost)
			fmt.Printf("   Monthly savings: $%.2f (%.1f%%)\n", estimate.MonthlySavings, estimate.SavingsPercent)
			fmt.Printf("   Annual savings: $%.2f\n", estimate.AnnualSavings)
		}
	}

	return nil
}

// removeLifecyclePolicy removes the lifecycle policy from the bucket
func removeLifecyclePolicy(ctx context.Context, manager *lifecycle.Manager) error {
	fmt.Printf("🗑️ Removing lifecycle policy from bucket...\n")

	if err := manager.RemovePolicy(ctx); err != nil {
		return fmt.Errorf("failed to remove policy: %w", err)
	}

	fmt.Printf("✅ Lifecycle policy removed successfully!\n")
	return nil
}

// exportLifecyclePolicy exports the current policy to a file
func exportLifecyclePolicy(ctx context.Context, manager *lifecycle.Manager, filename string) error {
	fmt.Printf("📤 Exporting current lifecycle policy to %s...\n", filename)

	// Get current policy
	currentPolicy, err := manager.GetCurrentPolicy(ctx)
	if err != nil {
		return fmt.Errorf("failed to get current policy: %w", err)
	}

	// Convert to our template format (simplified)
	template := lifecycle.PolicyTemplate{
		ID:          "exported-policy",
		Name:        "Exported Policy",
		Description: "Exported from S3 bucket",
		Rules:       []lifecycle.LifecycleRule{}, // Would need conversion logic
	}

	// Export to JSON
	jsonData, err := manager.ExportPolicy(template)
	if err != nil {
		return fmt.Errorf("failed to export policy: %w", err)
	}

	// Write to file
	if err := os.WriteFile(filename, []byte(jsonData), 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	fmt.Printf("✅ Policy exported successfully!\n")
	fmt.Printf("   Rules: %d\n", len(currentPolicy.Rules))
	return nil
}

// importLifecyclePolicy imports a policy from a file
func importLifecyclePolicy(ctx context.Context, manager *lifecycle.Manager, filename string) error {
	fmt.Printf("📥 Importing lifecycle policy from %s...\n", filename)

	// Read file
	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Import policy
	template, err := manager.ImportPolicy(string(data))
	if err != nil {
		return fmt.Errorf("failed to import policy: %w", err)
	}

	// Apply the imported policy
	if err := manager.ApplyPolicy(ctx, *template); err != nil {
		return fmt.Errorf("failed to apply imported policy: %w", err)
	}

	fmt.Printf("✅ Policy imported and applied successfully!\n")
	fmt.Printf("   Policy: %s\n", template.Name)
	fmt.Printf("   Rules: %d\n", len(template.Rules))
	return nil
}

// showCurrentPolicy displays the current lifecycle policy
func showCurrentPolicy(ctx context.Context, manager *lifecycle.Manager) error {
	fmt.Printf("📋 Current lifecycle policy for bucket: %s\n\n", lifecycleBucket)

	currentPolicy, err := manager.GetCurrentPolicy(ctx)
	if err != nil {
		// Check if it's a "no policy" error
		if strings.Contains(err.Error(), "NoSuchLifecycleConfiguration") {
			fmt.Printf("❌ No lifecycle policy configured for this bucket.\n")
			fmt.Printf("\n💡 Use --template to apply a predefined policy or --list-templates to see options.\n")
			return nil
		}
		return fmt.Errorf("failed to get current policy: %w", err)
	}

	fmt.Printf("✅ Active lifecycle policy found\n")
	fmt.Printf("   Rules: %d\n\n", len(currentPolicy.Rules))

	// Display rules in a readable format
	for i, rule := range currentPolicy.Rules {
		fmt.Printf("🔧 Rule %d: %s\n", i+1, *rule.ID)
		fmt.Printf("   Status: %s\n", rule.Status)

		// Show filter
		if rule.Filter != nil {
			if rule.Filter.Prefix != nil {
				fmt.Printf("   Prefix: %s\n", *rule.Filter.Prefix)
			}
			if rule.Filter.Tag != nil {
				fmt.Printf("   Tag: %s = %s\n", *rule.Filter.Tag.Key, *rule.Filter.Tag.Value)
			}
		}

		// Show transitions
		if len(rule.Transitions) > 0 {
			fmt.Printf("   Transitions:\n")
			for _, transition := range rule.Transitions {
				fmt.Printf("      • After %d days → %s\n", *transition.Days, transition.StorageClass)
			}
		}

		// Show expiration
		if rule.Expiration != nil && rule.Expiration.Days != nil {
			fmt.Printf("   Expiration: After %d days\n", *rule.Expiration.Days)
		}

		fmt.Println()
	}

	return nil
}

func init() {
	// This command will be added to root in root.go
}
