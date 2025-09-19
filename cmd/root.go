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
	Use:   "api-to-terraform",
	Short: "A CLI tool to fetch data from REST APIs and convert to Terraform",
	Long: `This tool fetches data from REST APIs and converts the responses
into Terraform configuration files. It supports various API endpoints
and can generate HCL formatted output.`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig)

	// Persistent flags available to all commands
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.api-to-terraform.yaml)")
	rootCmd.PersistentFlags().String("api-url", "", "Base URL of the API")
	rootCmd.PersistentFlags().String("api-key", "", "API key for authentication")

	// Bind flags to viper
	viper.BindPFlag("api.url", rootCmd.PersistentFlags().Lookup("api-url"))
	viper.BindPFlag("api.key", rootCmd.PersistentFlags().Lookup("api-key"))
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

		// Search config in home directory with name ".api-to-terraform" (without extension).
		viper.AddConfigPath(home)
		viper.SetConfigType("yaml")
		viper.SetConfigName(".api-to-terraform")
	}

	viper.AutomaticEnv()          // read in environment variables that match
	viper.SetEnvPrefix("AIRBYTE") // prefix for env vars

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
	}
}
