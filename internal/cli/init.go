package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/andywolf/agentium/internal/agentmd"
	"github.com/andywolf/agentium/internal/cli/wizard"
	"github.com/andywolf/agentium/internal/scanner"
	"github.com/andywolf/agentium/internal/skills"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize project configuration",
	Long: `Initialize Agentium configuration for the current project.

This creates a .agentium.yaml file and generates AGENT.md with
auto-detected project information to help AI agents understand your codebase.

If a CLAUDE.md file exists, its contents are merged into AGENT.md and
CLAUDE.md is replaced with a stub pointing to AGENT.md.

Example:
  agentium init
  agentium init --provider gcp --repo github.com/org/myapp
  agentium init --greenfield
  agentium init --non-interactive`,
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

	// New flags for AGENT.md generation
	initCmd.Flags().Bool("greenfield", false, "Skip scanning, create minimal AGENT.md for new project")
	initCmd.Flags().Bool("skip-agent-md", false, "Skip AGENT.md generation")
	initCmd.Flags().Bool("non-interactive", false, "Use detected values without prompting")
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
	Monorepo *monorepoConfig `yaml:"monorepo,omitempty"`
}

type monorepoConfig struct {
	Enabled     bool   `yaml:"enabled"`
	LabelPrefix string `yaml:"label_prefix"`
}

func initProject(cmd *cobra.Command, args []string) error {
	configPath := filepath.Join(".", ".agentium.yaml")
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	force, _ := cmd.Flags().GetBool("force")
	greenfield, _ := cmd.Flags().GetBool("greenfield")
	skipAgentMD, _ := cmd.Flags().GetBool("skip-agent-md")
	nonInteractive, _ := cmd.Flags().GetBool("non-interactive")

	// Check for existing config
	configExists := false
	if _, err := os.Stat(configPath); err == nil {
		configExists = true
		if !force {
			return fmt.Errorf("config file already exists at %s (use --force to overwrite)", configPath)
		}
	}

	// Generate .agentium.yaml
	if !configExists || force {
		if err := writeConfig(cmd, configPath, cwd); err != nil {
			return err
		}
		fmt.Printf("Created %s\n", configPath)
	}

	// Migrate CLAUDE.md to AGENT.md if present
	if err := migrateCLAUDEMD(cwd); err != nil {
		return fmt.Errorf("failed to migrate CLAUDE.md: %w", err)
	}

	// Generate AGENT.md
	if !skipAgentMD {
		if err := generateAgentMD(cwd, greenfield, nonInteractive); err != nil {
			return fmt.Errorf("failed to generate AGENT.md: %w", err)
		}
	}

	// Install Claude Code skills
	if err := skills.InstallProjectSkills(cwd, force); err != nil {
		return fmt.Errorf("failed to install skills: %w", err)
	}
	fmt.Println("Installed Claude Code skills to .claude/skills/")

	// Check CLI availability
	checkCLIAvailability()

	printNextSteps(skipAgentMD)

	return nil
}

func writeConfig(cmd *cobra.Command, configPath, cwd string) error {
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
		cfg.Project.Name = filepath.Base(cwd)
	}

	cfg.GitHub.PrivateKeySecret = fmt.Sprintf("projects/YOUR_PROJECT/secrets/%s-github-key", cfg.Project.Name)
	cfg.Defaults.Agent = "claude-code"
	cfg.Defaults.MaxIterations = 30
	cfg.Defaults.MaxDuration = "2h"

	// Detect pnpm monorepo
	if hasPnpmWorkspace(cwd) {
		cfg.Monorepo = &monorepoConfig{
			Enabled:     true,
			LabelPrefix: "pkg",
		}
		fmt.Println("Detected pnpm-workspace.yaml - monorepo support enabled")
	}

	// Write config file
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	header := `# Agentium Configuration
# See https://github.com/andywolf/agentium for documentation

`

	return os.WriteFile(configPath, append([]byte(header), data...), 0644)
}

func generateAgentMD(rootDir string, greenfield, nonInteractive bool) error {
	gen, err := agentmd.NewGenerator()
	if err != nil {
		return err
	}

	var info *scanner.ProjectInfo

	if greenfield {
		// Greenfield mode: prompt for or use minimal defaults
		if nonInteractive {
			info = &scanner.ProjectInfo{
				Name: filepath.Base(rootDir),
			}
		} else {
			info, err = wizard.PromptGreenfield()
			if err != nil {
				return err
			}
		}

		// Write greenfield AGENT.md to project root
		content := gen.GenerateGreenfield(info.Name)
		agentMDPath := filepath.Join(rootDir, agentmd.AgentMDFile)
		err = os.WriteFile(agentMDPath, []byte(content), 0644)
		if err != nil {
			return err
		}

		fmt.Printf("Created %s (greenfield template)\n", agentmd.AgentMDFile)
		fmt.Println("Run 'agentium refresh' after adding code to auto-detect project details.")
		return nil
	}

	// Scan project
	s := scanner.New(rootDir)
	info, err = s.Scan()
	if err != nil {
		return fmt.Errorf("failed to scan project: %w", err)
	}

	// Interactive confirmation
	if !nonInteractive {
		info, err = wizard.ConfirmProjectInfo(info)
		if err != nil {
			return err
		}
	}

	// Write AGENT.md
	if err := gen.WriteToProject(rootDir, info); err != nil {
		return err
	}

	fmt.Printf("Created %s\n", agentmd.AgentMDFile)
	return nil
}

func checkCLIAvailability() {
	// Check if agentium is in PATH
	_, err := exec.LookPath("agentium")
	if err == nil {
		return // Already available
	}

	fmt.Println()
	fmt.Println("Note: 'agentium' is not in your PATH.")

	// Check if it was installed via go install
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		gopath = filepath.Join(os.Getenv("HOME"), "go")
	}
	gobin := os.Getenv("GOBIN")
	if gobin == "" {
		gobin = filepath.Join(gopath, "bin")
	}

	agentiumBin := filepath.Join(gobin, "agentium")
	if _, err := os.Stat(agentiumBin); err == nil {
		// Binary exists but not in PATH
		shell := detectShell()
		shellConfig := getShellConfig(shell)

		fmt.Printf("Found agentium at: %s\n", agentiumBin)
		fmt.Println()
		fmt.Println("To add it to your PATH, add this to your shell config:")
		fmt.Printf("  echo 'export PATH=\"%s:$PATH\"' >> %s\n", gobin, shellConfig)
		fmt.Println()
		fmt.Println("Or create a symlink (may require sudo):")
		fmt.Printf("  sudo ln -sf %s /usr/local/bin/agentium\n", agentiumBin)
	} else {
		fmt.Println("Install agentium globally with:")
		fmt.Println("  go install github.com/andywolf/agentium/cmd/agentium@latest")
	}
}

func detectShell() string {
	shell := os.Getenv("SHELL")
	if strings.Contains(shell, "zsh") {
		return "zsh"
	}
	if strings.Contains(shell, "fish") {
		return "fish"
	}
	return "bash"
}

func getShellConfig(shell string) string {
	home := os.Getenv("HOME")
	switch shell {
	case "zsh":
		return filepath.Join(home, ".zshrc")
	case "fish":
		return filepath.Join(home, ".config", "fish", "config.fish")
	default:
		// Check for .bashrc vs .bash_profile
		if runtime.GOOS == "darwin" {
			return filepath.Join(home, ".bash_profile")
		}
		return filepath.Join(home, ".bashrc")
	}
}

// hasPnpmWorkspace checks if a pnpm-workspace.yaml file exists in the given directory.
func hasPnpmWorkspace(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "pnpm-workspace.yaml"))
	return err == nil
}

// migrateCLAUDEMD migrates CLAUDE.md content to AGENT.md and replaces CLAUDE.md with a stub.
// This allows projects to maintain a single source of truth for AI agent instructions
// while preserving compatibility with Claude Code which reads CLAUDE.md automatically.
func migrateCLAUDEMD(rootDir string) error {
	claudeMDPath := filepath.Join(rootDir, "CLAUDE.md")
	agentMDPath := filepath.Join(rootDir, agentmd.AgentMDFile)

	// Check if CLAUDE.md exists
	claudeContent, err := os.ReadFile(claudeMDPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No CLAUDE.md to migrate
		}
		return fmt.Errorf("failed to read CLAUDE.md: %w", err)
	}

	// Check if it's already a stub (pointing to AGENT.md)
	if strings.Contains(string(claudeContent), "See AGENT.md") ||
		strings.Contains(string(claudeContent), "see [AGENT.md]") {
		return nil // Already migrated
	}

	// Check if AGENT.md already exists
	existingAgent, err := os.ReadFile(agentMDPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read existing AGENT.md: %w", err)
	}

	var newAgentContent string
	if len(existingAgent) > 0 {
		// Merge: append CLAUDE.md content to existing AGENT.md
		newAgentContent = string(existingAgent) + "\n\n---\n\n## Migrated from CLAUDE.md\n\n" + string(claudeContent)
		fmt.Println("Merged CLAUDE.md content into existing AGENT.md")
	} else {
		// No existing AGENT.md: use CLAUDE.md content as-is
		newAgentContent = string(claudeContent)
		fmt.Println("Migrated CLAUDE.md to AGENT.md")
	}

	// Write merged content to AGENT.md
	if err := os.WriteFile(agentMDPath, []byte(newAgentContent), 0644); err != nil {
		return fmt.Errorf("failed to write AGENT.md: %w", err)
	}

	// Replace CLAUDE.md with a stub
	stub := `# See AGENT.md

This project uses [AGENT.md](AGENT.md) for AI agent instructions.
`
	if err := os.WriteFile(claudeMDPath, []byte(stub), 0644); err != nil {
		return fmt.Errorf("failed to write CLAUDE.md stub: %w", err)
	}
	fmt.Println("Replaced CLAUDE.md with stub pointing to AGENT.md")

	return nil
}

func printNextSteps(skippedAgentMD bool) {
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  1. Update the repository URL in .agentium.yaml")
	fmt.Println("  2. Set your GitHub App credentials")
	fmt.Println("  3. Configure your cloud provider credentials")
	if skippedAgentMD {
		fmt.Println("  4. Create AGENT.md with project instructions")
	} else {
		fmt.Println("  4. Review and customize AGENT.md")
	}
	fmt.Println("  5. Run 'agentium run --issues 1,2,3' to start a session")
	fmt.Println()
	fmt.Println("Installed skills:")
	fmt.Println("  - /gh-issues: Create GitHub issues instead of implementing code")
}
