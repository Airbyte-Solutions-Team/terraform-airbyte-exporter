package cmd

import (
	"api_to_terraform/internal/api"
	"api_to_terraform/internal/converter"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// airbyteCmd represents the airbyte command
var airbyteCmd = &cobra.Command{
	Use:   "airbyte [command]",
	Short: "Airbyte-specific commands",
	Long: `Commands specifically designed for working with the Airbyte API.
	
This includes fetching and converting sources, destinations, and connections.`,
}

// airbyteExportCmd exports all Airbyte resources
var airbyteExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export all Airbyte resources to Terraform",
	Long: `Export all sources, destinations, and connections from Airbyte
and convert them to Terraform configuration files.`,
	RunE: runAirbyteExport,
}

func init() {
	rootCmd.AddCommand(airbyteCmd)
	airbyteCmd.AddCommand(airbyteExportCmd)

	airbyteExportCmd.Flags().StringP("output-dir", "d", ".", "Directory to write Terraform files")
	airbyteExportCmd.Flags().Bool("split", false, "Write each resource type to a separate file")
	airbyteExportCmd.Flags().String("workspace", "", "Airbyte workspace ID to export")
	airbyteExportCmd.Flags().Bool("skip-imports", true, "Skip generating import blocks (default: true)")
	airbyteExportCmd.Flags().String("provider-version", "", "Specific Airbyte provider version to use (default: fetch latest)")
	airbyteExportCmd.Flags().Bool("skip-version-check", false, "Skip fetching latest provider version and use fallback")
	airbyteExportCmd.Flags().Bool("include-variables", true, "Include variables.tf content inside airbyte.tf (single file mode only, default: true)")
	airbyteExportCmd.Flags().Bool("separate-variables", false, "Generate separate variables.tf file instead of including in airbyte.tf")

	viper.BindPFlag("airbyte.output-dir", airbyteExportCmd.Flags().Lookup("output-dir"))
	viper.BindPFlag("airbyte.split", airbyteExportCmd.Flags().Lookup("split"))
	viper.BindPFlag("api.workspace", airbyteExportCmd.Flags().Lookup("workspace"))
	viper.BindPFlag("airbyte.skip-imports", airbyteExportCmd.Flags().Lookup("skip-imports"))
	viper.BindPFlag("airbyte.provider-version", airbyteExportCmd.Flags().Lookup("provider-version"))
	viper.BindPFlag("airbyte.skip-version-check", airbyteExportCmd.Flags().Lookup("skip-version-check"))
	viper.BindPFlag("airbyte.include-variables", airbyteExportCmd.Flags().Lookup("include-variables"))
	viper.BindPFlag("airbyte.separate-variables", airbyteExportCmd.Flags().Lookup("separate-variables"))
}

func runAirbyteExport(cmd *cobra.Command, args []string) error {
	baseURL := viper.GetString("api.url")
	clientID := viper.GetString("api.client_id")
	clientSecret := viper.GetString("api.client_secret")
	outputDir := viper.GetString("airbyte.output-dir")
	splitFiles := viper.GetBool("airbyte.split")
	skipImports := viper.GetBool("airbyte.skip-imports")
	providerVersion := viper.GetString("airbyte.provider-version")
	skipVersionCheck := viper.GetBool("airbyte.skip-version-check")
	separateVariables := viper.GetBool("airbyte.separate-variables")

	if baseURL == "" {
		baseURL = "https://api.airbyte.com"
	}

	// Create API client
	client := api.NewClient(baseURL, clientID, clientSecret)
	conv := converter.NewTerraformConverter()
	conv.SetSkipImports(skipImports)

	// Get workspace ID from config
	workspaceID := viper.GetString("api.workspace")

	// If workspace ID is not provided, we need to fetch it for declarative source definitions
	var workspaceIDs []string
	if workspaceID != "" {
		workspaceIDs = []string{workspaceID}
	} else {
		// Fetch all workspaces
		fmt.Fprintf(os.Stderr, "No workspace ID provided, fetching all workspaces...\n")
		workspacesData, err := client.GetWorkspaces()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to fetch workspaces: %v\n", err)
			fmt.Fprintf(os.Stderr, "Skipping declarative source definitions export\n")
		} else {
			// Parse workspaces
			var workspaceResp struct {
				Data []struct {
					WorkspaceID string `json:"workspaceId"`
					Name        string `json:"name"`
				} `json:"data"`
			}
			if err := json.Unmarshal(workspacesData, &workspaceResp); err == nil {
				for _, ws := range workspaceResp.Data {
					workspaceIDs = append(workspaceIDs, ws.WorkspaceID)
				}
				fmt.Fprintf(os.Stderr, "Found %d workspace(s)\n", len(workspaceIDs))
			}
		}
	}

	// Resources to export
	resources := []struct {
		name     string
		endpoint string
		filename string
	}{
		{"sources", "/v1/sources", "sources.tf"},
		{"destinations", "/v1/destinations", "destinations.tf"},
		{"connections", "/v1/connections", "connections.tf"},
	}

	var allTerraform strings.Builder
	var allResources []string

	// Reset variables at the start
	conv.ResetVariables()

	if workspaceID != "" {
		fmt.Fprintf(os.Stderr, "Using workspace ID: %s\n", workspaceID)
	}
	for _, resource := range resources {
		fmt.Fprintf(os.Stderr, "Fetching %s from %s%s...\n", resource.name, baseURL, resource.endpoint)

		data, err := client.Get(resource.endpoint, &workspaceID)
		if err != nil {
			return fmt.Errorf("failed to fetch %s: %w", resource.name, err)
		}

		// Convert to Terraform
		terraform, err := conv.Convert(data, workspaceID)
		if err != nil {
			return fmt.Errorf("failed to convert %s to Terraform: %w", resource.name, err)
		}

		if splitFiles {
			// Store the terraform content for later
			allResources = append(allResources, terraform)
		} else {
			// Append to combined output
			if allTerraform.Len() > 0 {
				allTerraform.WriteString("\n\n")
			}
			allTerraform.WriteString(fmt.Sprintf("# %s\n", strings.Title(resource.name)))
			allTerraform.WriteString(terraform)
		}
	}

	// Fetch declarative source definitions if we have workspace IDs
	var declarativeDefsTerraform string
	if len(workspaceIDs) > 0 {
		fmt.Fprintf(os.Stderr, "Fetching declarative source definitions...\n")
		for _, wsID := range workspaceIDs {
			endpoint := fmt.Sprintf("/v1/workspaces/%s/definitions/declarative_sources", wsID)
			fmt.Fprintf(os.Stderr, "Fetching declarative source definitions for workspace %s from %s%s...\n", wsID, baseURL, endpoint)

			data, err := client.Get(endpoint, nil)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: Failed to fetch declarative source definitions for workspace %s: %v\n", wsID, err)
				continue
			}

			// Convert to Terraform
			terraform, err := conv.Convert(data, wsID)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: Failed to convert declarative source definitions for workspace %s: %v\n", wsID, err)
				continue
			}

			// Only append if we got actual content (not just empty result)
			if terraform != "" && strings.TrimSpace(terraform) != "" {
				if declarativeDefsTerraform != "" {
					declarativeDefsTerraform += "\n\n"
				}
				declarativeDefsTerraform += terraform
			} else {
				fmt.Fprintf(os.Stderr, "No declarative source definitions found for workspace %s\n", wsID)
			}
		}

		if declarativeDefsTerraform != "" {
			if splitFiles {
				// Store the terraform content for later
				allResources = append(allResources, declarativeDefsTerraform)
			} else {
				// Append to combined output
				if allTerraform.Len() > 0 {
					allTerraform.WriteString("\n\n")
				}
				allTerraform.WriteString("# Declarative Source Definitions\n")
				allTerraform.WriteString(declarativeDefsTerraform)
			}
		}
	}

	// Get all variables HCL and tfvars content
	variablesHCL := conv.GetVariablesHCL()
	tfvarsContent := conv.GetTfvarsContent()

	if splitFiles {
		// Write variables file (always includes basic Airbyte variables)
		variablesPath := fmt.Sprintf("%s/variables.tf", outputDir)
		err := os.WriteFile(variablesPath, []byte(variablesHCL), 0644)
		if err != nil {
			return fmt.Errorf("failed to write variables.tf: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Wrote variables to %s\n", variablesPath)

		// Write providers file
		providersHCL := conv.GetProvidersContent(providerVersion, skipVersionCheck)
		providersPath := fmt.Sprintf("%s/providers.tf", outputDir)
		err = os.WriteFile(providersPath, []byte(providersHCL), 0644)
		if err != nil {
			return fmt.Errorf("failed to write providers.tf: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Wrote providers to %s\n", providersPath)

		// Write tfvars file (always includes Airbyte API credentials)
		tfvarsPath := fmt.Sprintf("%s/terraform.tfvars.example", outputDir)
		err = os.WriteFile(tfvarsPath, []byte(tfvarsContent), 0644)
		if err != nil {
			return fmt.Errorf("failed to write terraform.tfvars.example: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Wrote variable values template to %s\n", tfvarsPath)

		// Write each resource file
		for i, resource := range resources {
			if i < len(allResources) {
				filepath := fmt.Sprintf("%s/%s", outputDir, resource.filename)
				err := os.WriteFile(filepath, []byte(allResources[i]), 0644)
				if err != nil {
					return fmt.Errorf("failed to write %s: %w", resource.filename, err)
				}
				fmt.Fprintf(os.Stderr, "Wrote %s to %s\n", resource.name, filepath)
			}
		}

		// Write declarative source definitions file if we have any
		if len(allResources) > len(resources) {
			filepath := fmt.Sprintf("%s/declarative_source_definitions.tf", outputDir)
			err := os.WriteFile(filepath, []byte(allResources[len(resources)]), 0644)
			if err != nil {
				return fmt.Errorf("failed to write declarative_source_definitions.tf: %w", err)
			}
			fmt.Fprintf(os.Stderr, "Wrote declarative source definitions to %s\n", filepath)
		}
	} else {
		// Handle variables based on separate-variables flag
		if separateVariables {
			// Generate separate variables.tf file
			variablesPath := fmt.Sprintf("%s/variables.tf", outputDir)
			err := os.WriteFile(variablesPath, []byte(variablesHCL), 0644)
			if err != nil {
				return fmt.Errorf("failed to write variables.tf: %w", err)
			}
			fmt.Fprintf(os.Stderr, "Wrote variables to %s\n", variablesPath)

			// Write combined file without variables
			filepath := fmt.Sprintf("%s/airbyte.tf", outputDir)
			err = os.WriteFile(filepath, []byte(allTerraform.String()), 0644)
			if err != nil {
				return fmt.Errorf("failed to write airbyte.tf: %w", err)
			}
			fmt.Fprintf(os.Stderr, "Wrote all resources to %s\n", filepath)
		} else {
			// Include variables inside airbyte.tf (default behavior)
			var finalOutput strings.Builder
			if variablesHCL != "" {
				finalOutput.WriteString(variablesHCL)
				finalOutput.WriteString("\n")
			}
			finalOutput.WriteString(allTerraform.String())

			filepath := fmt.Sprintf("%s/airbyte.tf", outputDir)
			err := os.WriteFile(filepath, []byte(finalOutput.String()), 0644)
			if err != nil {
				return fmt.Errorf("failed to write airbyte.tf: %w", err)
			}
			fmt.Fprintf(os.Stderr, "Wrote all resources with variables to %s\n", filepath)
		}

		// Write providers file separately
		providersHCL := conv.GetProvidersContent(providerVersion, skipVersionCheck)
		providersPath := fmt.Sprintf("%s/providers.tf", outputDir)
		err := os.WriteFile(providersPath, []byte(providersHCL), 0644)
		if err != nil {
			return fmt.Errorf("failed to write providers.tf: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Wrote providers to %s\n", providersPath)

		// Write tfvars file (always includes Airbyte API credentials)
		tfvarsPath := fmt.Sprintf("%s/terraform.tfvars.example", outputDir)
		err = os.WriteFile(tfvarsPath, []byte(tfvarsContent), 0644)
		if err != nil {
			return fmt.Errorf("failed to write terraform.tfvars.example: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Wrote variable values template to %s\n", tfvarsPath)
	}

	// Print success message
	hasDeclarativeDefs := declarativeDefsTerraform != ""
	printSuccessMessage(outputDir, splitFiles, variablesHCL != "", true, skipImports, hasDeclarativeDefs, separateVariables)

	return nil
}

func printSuccessMessage(outputDir string, splitFiles bool, hasVariables bool, hasTfvars bool, skipImports bool, hasDeclarativeDefs bool, separateVariables bool) {
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "✅ Export completed successfully!")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Generated files:")

	if splitFiles {
		fmt.Fprintf(os.Stderr, "  • %s/sources.tf\n", outputDir)
		fmt.Fprintf(os.Stderr, "  • %s/destinations.tf\n", outputDir)
		fmt.Fprintf(os.Stderr, "  • %s/connections.tf\n", outputDir)
		if hasDeclarativeDefs {
			fmt.Fprintf(os.Stderr, "  • %s/declarative_source_definitions.tf\n", outputDir)
		}
		fmt.Fprintf(os.Stderr, "  • %s/variables.tf\n", outputDir)
	} else {
		fmt.Fprintf(os.Stderr, "  • %s/airbyte.tf\n", outputDir)
		if separateVariables {
			fmt.Fprintf(os.Stderr, "  • %s/variables.tf\n", outputDir)
		}
	}

	// Always show providers.tf
	fmt.Fprintf(os.Stderr, "  • %s/providers.tf\n", outputDir)

	if hasTfvars {
		fmt.Fprintf(os.Stderr, "  • %s/terraform.tfvars.example\n", outputDir)
	}

	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "⚠️  IMPORTANT: Next steps")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "  1. Review and modify the generated Terraform files as required by your organization")
	fmt.Fprintln(os.Stderr, "  2. Update security and compliance settings to match your standards")
	fmt.Fprintln(os.Stderr, "  3. Verify all resource configurations before applying")

	if hasTfvars {
		fmt.Fprintln(os.Stderr, "  4. Copy terraform.tfvars.example to terraform.tfvars and update with your actual values:")
		fmt.Fprintln(os.Stderr, "     • Airbyte API credentials (server_url, client_id, client_secret, workspace_id)")
		fmt.Fprintln(os.Stderr, "     • Source and destination secrets")
		fmt.Fprintln(os.Stderr, "  5. Ensure terraform.tfvars is added to .gitignore")
		fmt.Fprintln(os.Stderr, "  6. Run 'terraform init' to initialize the Terraform working directory")
		fmt.Fprintln(os.Stderr, "  7. Run 'terraform plan' to review the planned changes")
		fmt.Fprintln(os.Stderr, "  8. Run 'terraform apply' to create the resources")
	} else {
		fmt.Fprintln(os.Stderr, "  4. Run 'terraform init' to initialize the Terraform working directory")
		fmt.Fprintln(os.Stderr, "  5. Run 'terraform plan' to review the planned changes")
		fmt.Fprintln(os.Stderr, "  6. Run 'terraform apply' to create the resources")
	}

	if !skipImports {
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "💡 To import existing resources into Terraform state, run:")
		fmt.Fprintf(os.Stderr, "     cd %s && terraform init && terraform plan -generate-config-out=generated.tf\n", outputDir)
	}

	fmt.Fprintln(os.Stderr, "")
}
