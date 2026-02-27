package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Airbyte-Solutions-Team/terraform-airbyte-exporter/internal/airbyte"
	"github.com/Airbyte-Solutions-Team/terraform-airbyte-exporter/internal/api"
	"github.com/Airbyte-Solutions-Team/terraform-airbyte-exporter/internal/converter"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// runAirbyteExport is the main export function called by the root command
func runAirbyteExport(cmd *cobra.Command, args []string) error {
	baseURL := viper.GetString("api.url")
	clientID := viper.GetString("api.client_id")
	clientSecret := viper.GetString("api.client_secret")
	username := viper.GetString("api.username")
	password := viper.GetString("api.password")
	outputDir := viper.GetString("airbyte.output-dir")
	splitFiles := viper.GetBool("airbyte.split")
	migrate := viper.GetBool("airbyte.migrate")
	providerVersion := viper.GetString("airbyte.provider-version")
	skipVersionCheck := viper.GetBool("airbyte.skip-version-check")
	separateVariables := viper.GetBool("airbyte.separate-variables")
	skipProviders := viper.GetBool("airbyte.skip-providers")

	if baseURL == "" {
		baseURL = "https://api.airbyte.com"
	}

	// Create API client
	client, err := api.NewClient(baseURL, clientID, clientSecret, username, password)
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}
	conv := converter.NewTerraformConverter()
	conv.SetMigrate(migrate)
	conv.SetServerURL(baseURL)

	// Check if connection-id is specified for targeted export
	connectionID := viper.GetString("airbyte.connection-id")
	if connectionID != "" {
		return exportSingleConnection(client, conv, connectionID, outputDir, splitFiles, providerVersion, skipVersionCheck, separateVariables, skipProviders)
	}

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
			fmt.Fprintf(os.Stderr, "Note: Declarative source definitions require workspace IDs and will be skipped\n")
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
			} else {
				fmt.Fprintf(os.Stderr, "Warning: Failed to parse workspace response: %v\n", err)
				fmt.Fprintf(os.Stderr, "Note: Declarative source definitions will be skipped\n")
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

	for _, resource := range resources {
		fmt.Fprintf(os.Stderr, "Fetching %s...\n", resource.name)

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

	// Fetch custom source and destination definitions if we have workspace IDs
	var customDefsTerraform string
	if len(workspaceIDs) > 0 {
		fmt.Fprintf(os.Stderr, "Fetching custom source and destination definitions...\n")
		for _, wsID := range workspaceIDs {
			// Try the public API endpoint first, fall back to internal config API
			endpoint := fmt.Sprintf("/v1/workspaces/%s/definitions/declarative_sources", wsID)
			data, err := client.Get(endpoint, nil)
			if err != nil {
				// Fall back to internal config API
				data, err = client.GetCustomSourceDefinitions(wsID)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Warning: Failed to fetch custom source definitions for workspace %s: %v\n", wsID, err)
				}
			}

			if data != nil {
				terraform, err := conv.Convert(data, wsID)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Warning: Failed to convert custom source definitions for workspace %s: %v\n", wsID, err)
				} else if terraform != "" && strings.TrimSpace(terraform) != "" {
					if customDefsTerraform != "" {
						customDefsTerraform += "\n\n"
					}
					customDefsTerraform += terraform
				}
			}

			// Fetch custom destination definitions via internal config API
			destData, err := client.GetCustomDestinationDefinitions(wsID)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: Failed to fetch custom destination definitions for workspace %s: %v\n", wsID, err)
			} else {
				terraform, err := conv.Convert(destData, wsID)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Warning: Failed to convert custom destination definitions for workspace %s: %v\n", wsID, err)
				} else if terraform != "" && strings.TrimSpace(terraform) != "" {
					if customDefsTerraform != "" {
						customDefsTerraform += "\n\n"
					}
					customDefsTerraform += terraform
				}
			}
		}

		if customDefsTerraform != "" {
			if splitFiles {
				allResources = append(allResources, customDefsTerraform)
			} else {
				if allTerraform.Len() > 0 {
					allTerraform.WriteString("\n\n")
				}
				allTerraform.WriteString("# Custom Source and Destination Definitions\n")
				allTerraform.WriteString(customDefsTerraform)
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

		// Write providers file (unless skipped)
		if !skipProviders {
			providersHCL := conv.GetProvidersContent(providerVersion, skipVersionCheck)
			providersPath := fmt.Sprintf("%s/providers.tf", outputDir)
			err = os.WriteFile(providersPath, []byte(providersHCL), 0644)
			if err != nil {
				return fmt.Errorf("failed to write providers.tf: %w", err)
			}
			fmt.Fprintf(os.Stderr, "Wrote providers to %s\n", providersPath)
		}

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

		// Write custom definitions file if we have any
		if len(allResources) > len(resources) {
			filepath := fmt.Sprintf("%s/custom_definitions.tf", outputDir)
			err := os.WriteFile(filepath, []byte(allResources[len(resources)]), 0644)
			if err != nil {
				return fmt.Errorf("failed to write custom_definitions.tf: %w", err)
			}
			fmt.Fprintf(os.Stderr, "Wrote custom definitions to %s\n", filepath)
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

		// Write providers file separately (unless skipped)
		if !skipProviders {
			providersHCL := conv.GetProvidersContent(providerVersion, skipVersionCheck)
			providersPath := fmt.Sprintf("%s/providers.tf", outputDir)
			err := os.WriteFile(providersPath, []byte(providersHCL), 0644)
			if err != nil {
				return fmt.Errorf("failed to write providers.tf: %w", err)
			}
			fmt.Fprintf(os.Stderr, "Wrote providers to %s\n", providersPath)
		}

		// Write tfvars file (always includes Airbyte API credentials)
		tfvarsPath := fmt.Sprintf("%s/terraform.tfvars.example", outputDir)
		err := os.WriteFile(tfvarsPath, []byte(tfvarsContent), 0644)
		if err != nil {
			return fmt.Errorf("failed to write terraform.tfvars.example: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Wrote variable values template to %s\n", tfvarsPath)
	}

	// Print success message
	hasCustomDefs := customDefsTerraform != ""
	printSuccessMessage(outputDir, splitFiles, variablesHCL != "", true, migrate, hasCustomDefs, separateVariables, skipProviders)

	return nil
}

// exportSingleConnection exports a specific connection and its associated source and destination
func exportSingleConnection(client *api.Client, conv *converter.TerraformConverter, connectionID string, outputDir string, splitFiles bool, providerVersion string, skipVersionCheck bool, separateVariables bool, skipProviders bool) error {
	fmt.Fprintf(os.Stderr, "Exporting specific connection: %s\n", connectionID)

	// Reset variables at the start
	conv.ResetVariables()

	// Fetch the connection
	fmt.Fprintf(os.Stderr, "Fetching connection %s...\n", connectionID)
	connData, err := client.GetConnectionByID(connectionID)
	if err != nil {
		return fmt.Errorf("failed to fetch connection %s: %w", connectionID, err)
	}

	// Parse the connection to get source and destination IDs
	var conn airbyte.Connection
	if err := json.Unmarshal(connData, &conn); err != nil {
		return fmt.Errorf("failed to parse connection: %w", err)
	}

	workspaceID := conn.WorkspaceID
	fmt.Fprintf(os.Stderr, "Connection workspace ID: %s\n", workspaceID)
	fmt.Fprintf(os.Stderr, "Connection source ID: %s\n", conn.SourceID)
	fmt.Fprintf(os.Stderr, "Connection destination ID: %s\n", conn.DestinationID)

	// Fetch the source
	fmt.Fprintf(os.Stderr, "Fetching source %s...\n", conn.SourceID)
	sourceData, err := client.GetSourceByID(conn.SourceID)
	if err != nil {
		return fmt.Errorf("failed to fetch source %s: %w", conn.SourceID, err)
	}

	fmt.Printf("Source data: %v", string(sourceData))
	// Fetch the destination
	fmt.Fprintf(os.Stderr, "Fetching destination %s...\n", conn.DestinationID)
	destData, err := client.GetDestinationByID(conn.DestinationID)
	if err != nil {
		return fmt.Errorf("failed to fetch destination %s: %w", conn.DestinationID, err)
	}

	// Parse and add source
	var source airbyte.Source
	if err := json.Unmarshal(sourceData, &source); err != nil {
		return fmt.Errorf("failed to parse source: %w", err)
	}

	// Parse and add destination
	var dest airbyte.Destination
	if err := json.Unmarshal(destData, &dest); err != nil {
		return fmt.Errorf("failed to parse destination: %w", err)
	}

	// Convert source to Terraform
	fmt.Fprintf(os.Stderr, "Converting source to Terraform...\n")
	sourceResp := airbyte.SourceResponse{Sources: []airbyte.Source{source}}
	sourceJSON, _ := json.Marshal(sourceResp)
	sourceTF, err := conv.Convert(sourceJSON, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to convert source to Terraform: %w", err)
	}

	// Convert destination to Terraform
	fmt.Fprintf(os.Stderr, "Converting destination to Terraform...\n")
	destResp := airbyte.DestinationResponse{Destinations: []airbyte.Destination{dest}}
	destJSON, _ := json.Marshal(destResp)
	destTF, err := conv.Convert(destJSON, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to convert destination to Terraform: %w", err)
	}

	// Convert connection to Terraform
	fmt.Fprintf(os.Stderr, "Converting connection to Terraform...\n")
	connResp := airbyte.ConnectionResponse{Connections: []airbyte.Connection{conn}}
	connJSON, _ := json.Marshal(connResp)
	connTF, err := conv.Convert(connJSON, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to convert connection to Terraform: %w", err)
	}

	// Fetch custom source definition if the source uses one
	var customDefTF string
	if source.SourceDefinitionID != "" {
		// Try the public API endpoint first, fall back to internal config API
		endpoint := fmt.Sprintf("/v1/workspaces/%s/definitions/declarative_sources", workspaceID)
		declData, err := client.Get(endpoint, nil)
		if err != nil {
			// Fall back to internal config API
			declData, err = client.GetCustomSourceDefinitions(workspaceID)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to fetch source definitions: %v\n", err)
		} else {
			customDefTF, _ = conv.Convert(declData, workspaceID)
		}
	}

	// Get all variables HCL and tfvars content
	variablesHCL := conv.GetVariablesHCL()
	tfvarsContent := conv.GetTfvarsContent()

	// Write output files
	if splitFiles {
		// Write source file
		sourcePath := fmt.Sprintf("%s/sources.tf", outputDir)
		if err := os.WriteFile(sourcePath, []byte(sourceTF), 0644); err != nil {
			return fmt.Errorf("failed to write sources.tf: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Wrote source to %s\n", sourcePath)

		// Write destination file
		destPath := fmt.Sprintf("%s/destinations.tf", outputDir)
		if err := os.WriteFile(destPath, []byte(destTF), 0644); err != nil {
			return fmt.Errorf("failed to write destinations.tf: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Wrote destination to %s\n", destPath)

		// Write connection file
		connPath := fmt.Sprintf("%s/connections.tf", outputDir)
		if err := os.WriteFile(connPath, []byte(connTF), 0644); err != nil {
			return fmt.Errorf("failed to write connections.tf: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Wrote connection to %s\n", connPath)

		// Write custom definitions if present
		if customDefTF != "" && strings.TrimSpace(customDefTF) != "" {
			defPath := fmt.Sprintf("%s/custom_definitions.tf", outputDir)
			if err := os.WriteFile(defPath, []byte(customDefTF), 0644); err != nil {
				return fmt.Errorf("failed to write custom_definitions.tf: %w", err)
			}
			fmt.Fprintf(os.Stderr, "Wrote custom definitions to %s\n", defPath)
		}

		// Write variables file
		variablesPath := fmt.Sprintf("%s/variables.tf", outputDir)
		if err := os.WriteFile(variablesPath, []byte(variablesHCL), 0644); err != nil {
			return fmt.Errorf("failed to write variables.tf: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Wrote variables to %s\n", variablesPath)
	} else {
		// Write combined file
		var allTerraform strings.Builder
		if !separateVariables && variablesHCL != "" {
			allTerraform.WriteString(variablesHCL)
			allTerraform.WriteString("\n")
		}

		allTerraform.WriteString("# Source\n")
		allTerraform.WriteString(sourceTF)
		allTerraform.WriteString("\n\n")

		allTerraform.WriteString("# Destination\n")
		allTerraform.WriteString(destTF)
		allTerraform.WriteString("\n\n")

		allTerraform.WriteString("# Connection\n")
		allTerraform.WriteString(connTF)

		if customDefTF != "" && strings.TrimSpace(customDefTF) != "" {
			allTerraform.WriteString("\n\n")
			allTerraform.WriteString("# Custom Definitions\n")
			allTerraform.WriteString(customDefTF)
		}

		filepath := fmt.Sprintf("%s/airbyte.tf", outputDir)
		if err := os.WriteFile(filepath, []byte(allTerraform.String()), 0644); err != nil {
			return fmt.Errorf("failed to write airbyte.tf: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Wrote all resources to %s\n", filepath)

		// Write separate variables file if requested
		if separateVariables {
			variablesPath := fmt.Sprintf("%s/variables.tf", outputDir)
			if err := os.WriteFile(variablesPath, []byte(variablesHCL), 0644); err != nil {
				return fmt.Errorf("failed to write variables.tf: %w", err)
			}
			fmt.Fprintf(os.Stderr, "Wrote variables to %s\n", variablesPath)
		}
	}

	// Write providers file (unless skipped)
	if !skipProviders {
		providersHCL := conv.GetProvidersContent(providerVersion, skipVersionCheck)
		providersPath := fmt.Sprintf("%s/providers.tf", outputDir)
		if err := os.WriteFile(providersPath, []byte(providersHCL), 0644); err != nil {
			return fmt.Errorf("failed to write providers.tf: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Wrote providers to %s\n", providersPath)
	}

	// Write tfvars file
	tfvarsPath := fmt.Sprintf("%s/terraform.tfvars.example", outputDir)
	if err := os.WriteFile(tfvarsPath, []byte(tfvarsContent), 0644); err != nil {
		return fmt.Errorf("failed to write terraform.tfvars.example: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Wrote variable values template to %s\n", tfvarsPath)

	// Print success message
	hasCustomDefs := customDefTF != "" && strings.TrimSpace(customDefTF) != ""
	printSuccessMessage(outputDir, splitFiles, variablesHCL != "", true, true, hasCustomDefs, separateVariables, skipProviders)

	return nil
}

func printSuccessMessage(outputDir string, splitFiles bool, hasVariables bool, hasTfvars bool, migrate bool, hasCustomDefs bool, separateVariables bool, skipProviders bool) {
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Export completed successfully!")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Generated files:")

	if splitFiles {
		fmt.Fprintf(os.Stderr, "  • %s/sources.tf\n", outputDir)
		fmt.Fprintf(os.Stderr, "  • %s/destinations.tf\n", outputDir)
		fmt.Fprintf(os.Stderr, "  • %s/connections.tf\n", outputDir)
		if hasCustomDefs {
			fmt.Fprintf(os.Stderr, "  • %s/custom_definitions.tf\n", outputDir)
		}
		fmt.Fprintf(os.Stderr, "  • %s/variables.tf\n", outputDir)
	} else {
		fmt.Fprintf(os.Stderr, "  • %s/airbyte.tf\n", outputDir)
		if separateVariables {
			fmt.Fprintf(os.Stderr, "  • %s/variables.tf\n", outputDir)
		}
	}

	// Show providers.tf if not skipped
	if !skipProviders {
		fmt.Fprintf(os.Stderr, "  • %s/providers.tf\n", outputDir)
	}

	if hasTfvars {
		fmt.Fprintf(os.Stderr, "  • %s/terraform.tfvars.example\n", outputDir)
	}

	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "IMPORTANT: Next steps")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "  1. Review and modify the generated Terraform files")
	fmt.Fprintln(os.Stderr, "  2. Update security and compliance settings as needed")
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

	if !migrate {
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "To import existing resources into Terraform state, run:")
		fmt.Fprintf(os.Stderr, "     cd %s && terraform init && terraform plan -generate-config-out=generated.tf\n", outputDir)
	}

	fmt.Fprintln(os.Stderr, "")
}
