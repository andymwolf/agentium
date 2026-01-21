package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize project configuration",
	Long: `Initialize Agentium configuration for the current project.

This creates a .agentium.yaml file with sensible defaults that you can customize.

Example:
  agentium init
  agentium init --provider gcp --repo github.com/org/myapp`,
	RunE: initProject,
}

func init() {
	rootCmd.AddCommand(initCmd)

	initCmd.Flags().String("name", "", "Project name")
	initCmd.Flags().String("repo", "", "GitHub repository")
	initCmd.Flags().String("provider", "gcp", "Cloud provider (gcp, aws, azure)")
	initCmd.Flags().String("region", "us-central1", "Cloud region")
	initCmd.Flags().Int64("app-id", 0, "GitHub App ID")
	initCmd.Flags().Int64("installation-id", 0, "GitHub App Installation ID")
	initCmd.Flags().Bool("force", false, "Overwrite existing config")
}

type projectConfig struct {
	Project struct {
		Name       string `yaml:"name"`
		Repository string `yaml:"repository"`
	} `yaml:"project"`
	GitHub struct {
		AppID            int64  `yaml:"app_id,omitempty"`
		InstallationID   int64  `yaml:"installation_id,omitempty"`
		PrivateKeySecret string `yaml:"private_key_secret"`
	} `yaml:"github"`
	Cloud struct {
		Provider string `yaml:"provider"`
		Region   string `yaml:"region"`
	} `yaml:"cloud"`
	Defaults struct {
		Agent         string `yaml:"agent"`
		MaxIterations int    `yaml:"max_iterations"`
		MaxDuration   string `yaml:"max_duration"`
	} `yaml:"defaults"`
}

func initProject(cmd *cobra.Command, args []string) error {
	configPath := filepath.Join(".", ".agentium.yaml")

	force, _ := cmd.Flags().GetBool("force")
	if _, err := os.Stat(configPath); err == nil && !force {
		return fmt.Errorf("config file already exists at %s (use --force to overwrite)", configPath)
	}

	cfg := projectConfig{}

	// Get values from flags or defaults
	cfg.Project.Name, _ = cmd.Flags().GetString("name")
	cfg.Project.Repository, _ = cmd.Flags().GetString("repo")
	cfg.Cloud.Provider, _ = cmd.Flags().GetString("provider")
	cfg.Cloud.Region, _ = cmd.Flags().GetString("region")
	cfg.GitHub.AppID, _ = cmd.Flags().GetInt64("app-id")
	cfg.GitHub.InstallationID, _ = cmd.Flags().GetInt64("installation-id")

	// Set default values
	if cfg.Project.Name == "" {
		cwd, _ := os.Getwd()
		cfg.Project.Name = filepath.Base(cwd)
	}

	cfg.GitHub.PrivateKeySecret = fmt.Sprintf("projects/YOUR_PROJECT/secrets/%s-github-key", cfg.Project.Name)
	cfg.Defaults.Agent = "claude-code"
	cfg.Defaults.MaxIterations = 30
	cfg.Defaults.MaxDuration = "2h"

	// Write config file
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	header := `# Agentium Configuration
# See https://github.com/andywolf/agentium for documentation

`

	if err := os.WriteFile(configPath, append([]byte(header), data...), 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	fmt.Printf("Created %s\n\n", configPath)
	fmt.Println("Next steps:")
	fmt.Println("  1. Update the repository URL")
	fmt.Println("  2. Set your GitHub App credentials")
	fmt.Println("  3. Configure your cloud provider credentials")
	fmt.Println("  4. Run 'agentium run --issues 1,2,3' to start a session")

	return nil
}
