package cli

import (
	"fmt"
	"os"

	"github.com/andywolf/agentium/internal/version"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfgFile string

var rootCmd = &cobra.Command{
	Use:   "agentium",
	Short: "Agentium - Ephemeral AI agent sessions for code automation",
	Long: `Agentium is a CLI tool that provisions ephemeral VMs to run AI coding agents.

It supports multiple AI agents (Claude Code, Aider, custom) and cloud providers
(GCP, AWS, Azure). Each session creates an isolated, containerized environment
that automatically terminates when tasks complete or limits are reached.

Example:
  agentium run --repo github.com/org/myapp --issues 12,17,24`,
}

// Execute runs the root command
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig)

	// Set version for --version flag
	rootCmd.Version = version.Short()
	rootCmd.SetVersionTemplate("{{.Name}} {{.Version}}\n")

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is .agentium.yaml)")
	rootCmd.PersistentFlags().Bool("verbose", false, "enable verbose output")
	_ = viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose"))
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error getting working directory:", err)
			os.Exit(1)
		}

		viper.AddConfigPath(cwd)
		viper.SetConfigType("yaml")
		viper.SetConfigName(".agentium")
	}

	viper.SetEnvPrefix("AGENTIUM")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err == nil {
		if viper.GetBool("verbose") {
			fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
		}
	}
}
