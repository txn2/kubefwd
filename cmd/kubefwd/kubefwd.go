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
	"bytes"
	"fmt"
	"io/ioutil"
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/txn2/kubefwd/cmd/kubefwd/services"
)

var globalUsage = ``
var Version = "0.0.0"

func init() {
	// quiet version
	args := os.Args[1:]
	if len(args) == 2 && args[0] == "version" && args[1] == "quiet" {
		fmt.Println(Version)
		os.Exit(0)
	}

	log.SetOutput(&LogOutputSplitter{})
	if len(args) > 0 && (args[0] == "completion" || args[0] == "__complete") {
		log.SetOutput(ioutil.Discard)
	}
}

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "kubefwd",
		Short: "Expose Kubernetes services for local development.",
		Example: " kubefwd services --help\n" +
			"  kubefwd svc -n the-project\n" +
			"  kubefwd svc -n the-project -l env=dev,component=api\n" +
			"  kubefwd svc -n the-project -f metadata.name=service-name\n" +
			"  kubefwd svc -n default -l \"app in (ws, api)\"\n" +
			"  kubefwd svc -n default -n the-project\n" +
			"  kubefwd svc -n the-project -m 80:8080 -m 443:1443\n" +
			"  kubefwd svc -n the-project -z path/to/conf.yml\n" +
			"  kubefwd svc -n the-project -r svc.ns:127.3.3.1\n" +
			"  kubefwd svc -n the-project -h svc.local\n" +
			"  kubefwd svc --all-namespaces",

		Long: globalUsage,
	}

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print the version of Kubefwd",
		Example: " kubefwd version\n" +
			" kubefwd version quiet\n",
		Long: ``,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("Kubefwd version: %s\nhttps://github.com/txn2/kubefwd\n", Version)
		},
	}

	cmd.AddCommand(versionCmd, services.Cmd)

	return cmd
}

type LogOutputSplitter struct{}

func (splitter *LogOutputSplitter) Write(p []byte) (n int, err error) {
	if bytes.Contains(p, []byte("level=error")) || bytes.Contains(p, []byte("level=warn")) {
		return os.Stderr.Write(p)
	}
	return os.Stdout.Write(p)
}

func main() {

	log.SetFormatter(&log.TextFormatter{
		FullTimestamp:   true,
		ForceColors:     true,
		TimestampFormat: "15:04:05",
	})

	log.Print(` _          _           __             _`)
	log.Print(`| | ___   _| |__   ___ / _|_      ____| |`)
	log.Print(`| |/ / | | | '_ \ / _ \ |_\ \ /\ / / _  |`)
	log.Print(`|   <| |_| | |_) |  __/  _|\ V  V / (_| |`)
	log.Print(`|_|\_\\__,_|_.__/ \___|_|   \_/\_/ \__,_|`)
	log.Print("")
	log.Printf("Version %s", Version)
	log.Print("https://github.com/txn2/kubefwd")
	log.Print("")

	cmd := newRootCmd()

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
