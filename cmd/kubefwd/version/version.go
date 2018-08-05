package version

import (
	"fmt"

	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version of Kubefwd",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Kubefwd version: 1.0.0")
	},
}
