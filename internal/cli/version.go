package cli

import (
	"fmt"

	"github.com/andywolf/agentium/internal/version"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Long:  `Print detailed version information including commit hash and build date.`,
	Run: func(cmd *cobra.Command, args []string) {
		verbose, _ := cmd.Flags().GetBool("verbose")
		if verbose {
			fmt.Println(version.Full())
		} else {
			fmt.Println(version.Info())
		}
	},
}

func init() {
	versionCmd.Flags().BoolP("verbose", "v", false, "print verbose version information")
	rootCmd.AddCommand(versionCmd)
}
