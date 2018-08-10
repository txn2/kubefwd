package main

import (
	"os"

	"github.com/cbednarski/hostess"
	"github.com/codegangsta/cli"
)

func getCommand() string {
	return os.Args[1]
}

func getArgs() []string {
	return os.Args[2:]
}

const help = `an idempotent tool for managing /etc/hosts

 * Commands will exit 0 or 1 in a sensible way to facilitate scripting.
 * Hostess operates on /etc/hosts by default. Specify the HOSTESS_PATH
   environment variable to change this.
 * Run 'hostess fix -n' to preview changes hostess will make to your hostsfile.
 * Report bugs and feedback at https://github.com/cbednarski/hostess`

func main() {
	app := cli.NewApp()
	app.Name = "hostess"
	app.Authors = []cli.Author{{Name: "Chris Bednarski", Email: "banzaimonkey@gmail.com"}}
	app.Usage = help
	app.Version = "0.3.0"

	app.Flags = []cli.Flag{
		cli.BoolFlag{
			Name:  "f",
			Usage: "operate even if there are errors or conflicts",
		},
		cli.BoolFlag{
			Name:  "n",
			Usage: "no-op. Show changes but don't write them.",
		},
		cli.BoolFlag{
			Name:  "q",
			Usage: "quiet operation -- no notices",
		},
		cli.BoolFlag{
			Name:  "s",
			Usage: "silent operation -- no errors (implies -q)",
		},
	}

	app.Commands = []cli.Command{
		{
			Name:   "add",
			Usage:  "add or replace a hosts entry",
			Action: hostess.Add,
			Flags:  app.Flags,
		},
		{
			Name:   "aff",
			Usage:  "add or replace a hosts entry in an off state",
			Action: hostess.Add,
			Flags:  app.Flags,
		},
		{
			Name:    "del",
			Aliases: []string{"rm"},
			Usage:   "delete a hosts entry",
			Action:  hostess.Del,
			Flags:   app.Flags,
		},
		{
			Name:   "has",
			Usage:  "exit 0 if entry exists, 1 if not",
			Action: hostess.Has,
			Flags:  app.Flags,
		},
		{
			Name:   "on",
			Usage:  "enable a hosts entry (if if exists)",
			Action: hostess.OnOff,
			Flags:  app.Flags,
		},
		{
			Name:   "off",
			Usage:  "disable a hosts entry (don't delete it)",
			Action: hostess.OnOff,
			Flags:  app.Flags,
		},
		{
			Name:    "list",
			Aliases: []string{"ls"},
			Usage:   "list entries in the hosts file",
			Action:  hostess.Ls,
			Flags:   app.Flags,
		},
		{
			Name:   "fix",
			Usage:  "reformat the hosts file based on hostess' rules",
			Action: hostess.Fix,
			Flags:  app.Flags,
		},
		{
			Name:   "fixed",
			Usage:  "exit 0 if the hosts file is formatted, 1 if not",
			Action: hostess.Fixed,
			Flags:  app.Flags,
		},
		{
			Name:   "dump",
			Usage:  "dump the hosts file as JSON",
			Action: hostess.Dump,
			Flags:  app.Flags,
		},
		{
			Name:   "apply",
			Usage:  "add hostnames from a JSON file to the hosts file",
			Action: hostess.Apply,
			Flags:  app.Flags,
		},
	}

	app.Run(os.Args)
	os.Exit(0)
}
