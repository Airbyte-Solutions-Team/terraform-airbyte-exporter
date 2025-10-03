package cmd

import (
	"api-to-terraform/internal/api"
	"api-to-terraform/internal/converter"
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

	viper.BindPFlag("airbyte.output-dir", airbyteExportCmd.Flags().Lookup("output-dir"))
	viper.BindPFlag("airbyte.split", airbyteExportCmd.Flags().Lookup("split"))
}

func runAirbyteExport(cmd *cobra.Command, args []string) error {
	baseURL := viper.GetString("api.url")
	clientID := viper.GetString("api.client_id")
	clientSecret := viper.GetString("api.client_secret")
	outputDir := viper.GetString("airbyte.output-dir")
	splitFiles := viper.GetBool("airbyte.split")

	if baseURL == "" {
		baseURL = "https://api.airbyte.com"
	}

	// Create API client
	client := api.NewClient(baseURL, clientID, clientSecret)
	conv := converter.NewTerraformConverter()

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
		fmt.Fprintf(os.Stderr, "Fetching %s from %s%s...\n", resource.name, baseURL, resource.endpoint)

		data, err := client.Get(resource.endpoint)
		if err != nil {
			return fmt.Errorf("failed to fetch %s: %w", resource.name, err)
		}

		// Convert to Terraform
		terraform, err := conv.Convert(data)
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

	// Get all variables HCL
	variablesHCL := conv.GetVariablesHCL()

	if splitFiles {
		// Write variables file first if there are any
		if variablesHCL != "" {
			variablesPath := fmt.Sprintf("%s/variables.tf", outputDir)
			err := os.WriteFile(variablesPath, []byte(variablesHCL), 0644)
			if err != nil {
				return fmt.Errorf("failed to write variables.tf: %w", err)
			}
			fmt.Fprintf(os.Stderr, "Wrote variables to %s\n", variablesPath)
		}

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
	} else {
		// Write combined file with variables at the top
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
		fmt.Fprintf(os.Stderr, "Wrote all resources to %s\n", filepath)
	}

	return nil
}
