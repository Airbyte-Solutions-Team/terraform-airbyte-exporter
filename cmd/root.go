package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfgFile string

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "abtfexport",
	Short: "Export Airbyte resources to Terraform configuration",
	Long: `abtfexport fetches sources, destinations, and connections from the Airbyte API
and converts them into Terraform configuration files (HCL format).

This tool helps you migrate existing Airbyte configurations to Infrastructure as Code.`,
	RunE: runAirbyteExport,
}

// SetVersionInfo sets the version information for the CLI.
// This should be called before Execute() to populate build-time version data.
func SetVersionInfo(version, commit, date string) {
	rootCmd.Version = fmt.Sprintf("%s (commit: %s, built: %s)", version, commit, date)
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig)

	// Configuration and authentication flags
	rootCmd.Flags().StringVar(&cfgFile, "config", "", "Config file location (default is $HOME/.abtfexport.yaml)")
	rootCmd.Flags().String("api-url", "", "Base URL of the Airbyte API (default: https://api.airbyte.com)")
	rootCmd.Flags().String("client-id", "", "Airbyte API client ID for authentication")
	rootCmd.Flags().String("client-secret", "", "Airbyte API client secret for authentication")
	rootCmd.Flags().String("workspace", "", "Airbyte workspace ID to filter resources")

	// Export behavior flags
	rootCmd.Flags().StringP("output-dir", "d", ".", "Directory to write Terraform files")
	rootCmd.Flags().Bool("split", false, "Write each resource type to a separate file")
	rootCmd.Flags().String("connection-id", "", "Export only a specific connection and its associated source and destination")
	rootCmd.Flags().Bool("migrate", true, "Skip generating import blocks")
	rootCmd.Flags().String("provider-version", "", "Specific Airbyte provider version to use (default: fetch latest)")
	rootCmd.Flags().Bool("skip-version-check", false, "Skip fetching latest provider version and use fallback")
	rootCmd.Flags().Bool("include-variables", true, "Include variables.tf content inside airbyte.tf (single file mode only)")
	rootCmd.Flags().Bool("separate-variables", false, "Generate separate variables.tf file instead of including in airbyte.tf")
	rootCmd.Flags().Bool("skip-providers", false, "Skip generating providers.tf file")

	// Bind flags to viper
	viper.BindPFlag("api.url", rootCmd.Flags().Lookup("api-url"))
	viper.BindPFlag("api.client_id", rootCmd.Flags().Lookup("client-id"))
	viper.BindPFlag("api.client_secret", rootCmd.Flags().Lookup("client-secret"))
	viper.BindPFlag("api.workspace", rootCmd.Flags().Lookup("workspace"))
	viper.BindPFlag("airbyte.output-dir", rootCmd.Flags().Lookup("output-dir"))
	viper.BindPFlag("airbyte.split", rootCmd.Flags().Lookup("split"))
	viper.BindPFlag("airbyte.connection-id", rootCmd.Flags().Lookup("connection-id"))
	viper.BindPFlag("airbyte.migrate", rootCmd.Flags().Lookup("migrate"))
	viper.BindPFlag("airbyte.provider-version", rootCmd.Flags().Lookup("provider-version"))
	viper.BindPFlag("airbyte.skip-version-check", rootCmd.Flags().Lookup("skip-version-check"))
	viper.BindPFlag("airbyte.include-variables", rootCmd.Flags().Lookup("include-variables"))
	viper.BindPFlag("airbyte.separate-variables", rootCmd.Flags().Lookup("separate-variables"))
	viper.BindPFlag("airbyte.skip-providers", rootCmd.Flags().Lookup("skip-providers"))
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		// Search config in home directory with name ".abtfexport" (without extension).
		viper.AddConfigPath(home)
		viper.SetConfigType("yaml")
		viper.SetConfigName(".abtfexport")
	}

	viper.AutomaticEnv()          // read in environment variables that match
	viper.SetEnvPrefix("AIRBYTE") // prefix for env vars

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
	}
}
