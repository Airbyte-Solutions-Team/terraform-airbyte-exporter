package cmd

import (
	"fmt"
	"os"

	"github.com/Airbyte-Solutions-Team/terraform-airbyte-exporter/internal/api"
	"github.com/Airbyte-Solutions-Team/terraform-airbyte-exporter/internal/state"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var stateCmd = &cobra.Command{
	Use:   "state",
	Short: "Manage Airbyte connection states for migration",
	Long:  "Export and map connection states for migration between Airbyte instances",
}

var stateExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export connection states from an Airbyte instance",
	Long: `Export connection states from an Airbyte instance to prepare for migration.

States are exported to a JSON file that includes:
- Old connection IDs
- Original connection names (for reference)
- State data for each connection

Example usage:
  abtfexport state export --workspace ws_123 --output connection_states.json`,
	RunE: runStateExport,
}

var stateMapCmd = &cobra.Command{
	Use:   "map",
	Short: "Generate mapping between old and new connection IDs",
	Long: `Generate a mapping file by matching connections in the new instance.

Requires:
- State file from old instance (via 'state export')
- Access to new Airbyte instance (use --api-url, --client-id, --client-secret)
- Connections in new instance with names set to old connection IDs

Example usage:
  abtfexport state map --states connection_states.json --workspace ws_456 --output mapping.json`,
	RunE: runStateMap,
}

func init() {
	// State export command flags
	stateExportCmd.Flags().String("connection-id", "", "Export state for specific connection only")
	stateExportCmd.Flags().String("workspace", "", "Workspace ID to export states from (required for all connections)")
	stateExportCmd.Flags().StringP("output", "o", "connection_states.json", "Output file for states")

	viper.BindPFlag("state.export.connection-id", stateExportCmd.Flags().Lookup("connection-id"))
	viper.BindPFlag("state.export.workspace", stateExportCmd.Flags().Lookup("workspace"))
	viper.BindPFlag("state.export.output", stateExportCmd.Flags().Lookup("output"))

	// State map command flags
	stateMapCmd.Flags().String("states", "", "Path to state file from old instance (required)")
	stateMapCmd.Flags().StringP("output", "o", "connection_mapping.json", "Output file for mapping")
	stateMapCmd.Flags().String("workspace", "", "Workspace ID in the new instance (required)")

	stateMapCmd.MarkFlagRequired("states")
	stateMapCmd.MarkFlagRequired("workspace")

	viper.BindPFlag("state.map.states", stateMapCmd.Flags().Lookup("states"))
	viper.BindPFlag("state.map.output", stateMapCmd.Flags().Lookup("output"))
	viper.BindPFlag("state.map.workspace", stateMapCmd.Flags().Lookup("workspace"))

	stateCmd.AddCommand(stateExportCmd)
	stateCmd.AddCommand(stateMapCmd)
	rootCmd.AddCommand(stateCmd)
}

func runStateExport(cmd *cobra.Command, args []string) error {
	// Get configuration
	baseURL := viper.GetString("api.url")
	clientID := viper.GetString("api.client_id")
	clientSecret := viper.GetString("api.client_secret")

	connectionID := viper.GetString("state.export.connection-id")
	workspaceID := viper.GetString("state.export.workspace")
	outputPath := viper.GetString("state.export.output")

	if baseURL == "" {
		baseURL = "https://api.airbyte.com"
	}

	// Validate: need either connection-id or workspace
	if connectionID == "" && workspaceID == "" {
		return fmt.Errorf("either --connection-id or --workspace must be specified")
	}

	// Create API client
	client := api.NewClient(baseURL, clientID, clientSecret)

	// Create exporter
	exporter := state.NewExporter(client, baseURL)

	// Export state
	if connectionID != "" {
		fmt.Fprintf(os.Stderr, "Exporting state for connection %s...\n", connectionID)
		return exporter.ExportSingleConnectionState(connectionID, outputPath)
	} else {
		fmt.Fprintf(os.Stderr, "Exporting states for all connections in workspace %s...\n", workspaceID)
		return exporter.ExportConnectionStates(workspaceID, outputPath)
	}
}

func runStateMap(cmd *cobra.Command, args []string) error {
	// Get configuration (uses same flags as other commands)
	baseURL := viper.GetString("api.url")
	clientID := viper.GetString("api.client_id")
	clientSecret := viper.GetString("api.client_secret")

	statesPath := viper.GetString("state.map.states")
	workspaceID := viper.GetString("state.map.workspace")
	outputPath := viper.GetString("state.map.output")

	if baseURL == "" {
		baseURL = "https://api.airbyte.com"
	}

	// Create API client
	client := api.NewClient(baseURL, clientID, clientSecret)

	// Create mapper
	mapper := state.NewMapper(client)

	// Generate mapping
	fmt.Fprintf(os.Stderr, "Generating mapping from %s...\n", statesPath)
	fmt.Fprintf(os.Stderr, "Connecting to %s with workspace %s...\n", baseURL, workspaceID)
	return mapper.GenerateMapping(statesPath, workspaceID, outputPath)
}
