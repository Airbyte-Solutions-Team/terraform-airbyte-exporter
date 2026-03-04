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
	Long:  "Export, map, apply, and restore connection states for migration between Airbyte instances",
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

var stateApplyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Apply saved connection states to new connections",
	Long: `Apply previously exported connection states to new connections using a mapping file.

This transfers the sync state (cursor positions, stream states) from old connections
to their corresponding new connections, eliminating the need to re-sync historical data.

Requires:
- Mapping file from 'state map' command
- State file from 'state export' command
- Access to new Airbyte instance (use --api-url, --client-id, --client-secret)

Example usage:
  abtfexport state apply --mapping connection_mapping.json --states connection_states.json

Use --dry-run to preview which states would be applied without making changes:
  abtfexport state apply --mapping connection_mapping.json --states connection_states.json --dry-run`,
	RunE: runStateApply,
}

var stateRestoreCmd = &cobra.Command{
	Use:   "restore",
	Short: "Restore original names, schedules, and status for migrated connections",
	Long: `Restore the original connection names, schedules, and status after state migration.

During migration, connections are created with old IDs as names, set to inactive status,
and configured with manual sync schedules. This command restores them to their original
configuration.

Requires:
- Mapping file from 'state map' command
- State file from 'state export' command (contains original names, schedules, status)
- Access to new Airbyte instance (use --api-url, --client-id, --client-secret)

Example usage:
  abtfexport state restore --mapping connection_mapping.json --states connection_states.json

Use --dry-run to preview which changes would be made without applying them:
  abtfexport state restore --mapping connection_mapping.json --states connection_states.json --dry-run`,
	RunE: runStateRestore,
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

	// State apply command flags
	stateApplyCmd.Flags().String("mapping", "", "Path to mapping file from 'state map' command (required)")
	stateApplyCmd.Flags().String("states", "", "Path to state file from 'state export' command (required)")
	stateApplyCmd.Flags().Bool("dry-run", false, "Preview state applications without making changes")

	stateApplyCmd.MarkFlagRequired("mapping")
	stateApplyCmd.MarkFlagRequired("states")

	viper.BindPFlag("state.apply.mapping", stateApplyCmd.Flags().Lookup("mapping"))
	viper.BindPFlag("state.apply.states", stateApplyCmd.Flags().Lookup("states"))
	viper.BindPFlag("state.apply.dry-run", stateApplyCmd.Flags().Lookup("dry-run"))

	// State restore command flags
	stateRestoreCmd.Flags().String("mapping", "", "Path to mapping file from 'state map' command (required)")
	stateRestoreCmd.Flags().String("states", "", "Path to state file from 'state export' command (required)")
	stateRestoreCmd.Flags().Bool("dry-run", false, "Preview restorations without making changes")

	stateRestoreCmd.MarkFlagRequired("mapping")
	stateRestoreCmd.MarkFlagRequired("states")

	viper.BindPFlag("state.restore.mapping", stateRestoreCmd.Flags().Lookup("mapping"))
	viper.BindPFlag("state.restore.states", stateRestoreCmd.Flags().Lookup("states"))
	viper.BindPFlag("state.restore.dry-run", stateRestoreCmd.Flags().Lookup("dry-run"))

	stateCmd.AddCommand(stateExportCmd)
	stateCmd.AddCommand(stateMapCmd)
	stateCmd.AddCommand(stateApplyCmd)
	stateCmd.AddCommand(stateRestoreCmd)
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
	// Get configuration
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

func runStateApply(cmd *cobra.Command, args []string) error {
	// Get configuration
	baseURL := viper.GetString("api.url")
	clientID := viper.GetString("api.client_id")
	clientSecret := viper.GetString("api.client_secret")

	mappingPath := viper.GetString("state.apply.mapping")
	statesPath := viper.GetString("state.apply.states")
	dryRun := viper.GetBool("state.apply.dry-run")

	if baseURL == "" {
		baseURL = "https://api.airbyte.com"
	}

	// Create API client
	client := api.NewClient(baseURL, clientID, clientSecret)

	// Create applier
	applier := state.NewApplier(client)

	// Apply states
	if dryRun {
		fmt.Fprintf(os.Stderr, "Dry run: previewing state application...\n")
	} else {
		fmt.Fprintf(os.Stderr, "Applying connection states...\n")
	}
	fmt.Fprintf(os.Stderr, "Mapping file: %s\n", mappingPath)
	fmt.Fprintf(os.Stderr, "States file: %s\n", statesPath)

	return applier.ApplyStates(mappingPath, statesPath, dryRun)
}

func runStateRestore(cmd *cobra.Command, args []string) error {
	// Get configuration
	baseURL := viper.GetString("api.url")
	clientID := viper.GetString("api.client_id")
	clientSecret := viper.GetString("api.client_secret")

	mappingPath := viper.GetString("state.restore.mapping")
	statesPath := viper.GetString("state.restore.states")
	dryRun := viper.GetBool("state.restore.dry-run")

	if baseURL == "" {
		baseURL = "https://api.airbyte.com"
	}

	// Create API client
	client := api.NewClient(baseURL, clientID, clientSecret)

	// Create applier (handles both state application and restoration)
	applier := state.NewApplier(client)

	// Restore connections
	if dryRun {
		fmt.Fprintf(os.Stderr, "Dry run: previewing connection restoration...\n")
	} else {
		fmt.Fprintf(os.Stderr, "Restoring connection configurations...\n")
	}
	fmt.Fprintf(os.Stderr, "Mapping file: %s\n", mappingPath)
	fmt.Fprintf(os.Stderr, "States file: %s\n", statesPath)

	return applier.RestoreConnections(mappingPath, statesPath, dryRun)
}
