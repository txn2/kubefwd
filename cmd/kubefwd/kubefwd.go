/*
Copyright 2018 Craig Johnston <cjimti@gmail.com>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package main

import (
	"os"

	"fmt"

	"github.com/spf13/cobra"
	"github.com/txn2/kubefwd/cmd/kubefwd/services"
)

var globalUsage = ``
var Version = "0.0.0"

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "kubefwd",
		Short: "Expose Kubernetes services for local development.",
		Long:  globalUsage,
	}

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print the version of Kubefwd",
		Long:  ``,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("Kubefwd version: %s\nhttps://github.com/txn2/kubefwd\n", Version)
		},
	}

	cmd.AddCommand(versionCmd, services.Cmd)

	return cmd
}

func main() {
	cmd := newRootCmd()

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
