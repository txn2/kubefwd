package version

import (
	"fmt"

	"github.com/spf13/cobra"
)

var Version = "0.0.0"

var Cmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version of Kubefwd",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("Kubefwd version: %s\n", Version)
	},
}
