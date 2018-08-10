package hostess

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/codegangsta/cli"
)

// AnyBool checks whether a boolean CLI flag is set either globally or on this
// individual command. This provides more flexible flag parsing behavior.
func AnyBool(c *cli.Context, key string) bool {
	return c.Bool(key) || c.GlobalBool(key)
}

// ErrCantWriteHostFile indicates that we are unable to write to the hosts file
var ErrCantWriteHostFile = fmt.Errorf(
	"Unable to write to %s. Maybe you need to sudo?", GetHostsPath())

// MaybeErrorln will print an error message unless -s is passed
func MaybeErrorln(c *cli.Context, message string) {
	if !AnyBool(c, "s") {
		os.Stderr.WriteString(fmt.Sprintf("%s\n", message))
	}
}

// MaybeError will print an error message unless -s is passed and then exit
func MaybeError(c *cli.Context, message string) {
	MaybeErrorln(c, message)
	os.Exit(1)
}

// MaybePrintln will print a message unless -q or -s is passed
func MaybePrintln(c *cli.Context, message string) {
	if !AnyBool(c, "q") && !AnyBool(c, "s") {
		fmt.Println(message)
	}
}

// MaybeLoadHostFile will try to load, parse, and return a Hostfile. If we
// encounter errors we will terminate, unless -f is passed.
func MaybeLoadHostFile(c *cli.Context) *Hostfile {
	hostsfile, errs := LoadHostfile()
	if len(errs) > 0 && !AnyBool(c, "f") {
		for _, err := range errs {
			MaybeErrorln(c, err.Error())
		}
		MaybeError(c, "Errors while parsing hostsfile. Try hostess fix")
	}
	return hostsfile
}

// AlwaysLoadHostFile will load, parse, and return a Hostfile. If we encouter
// errors they will be printed to the terminal, but we'll try to continue.
func AlwaysLoadHostFile(c *cli.Context) *Hostfile {
	hostsfile, errs := LoadHostfile()
	if len(errs) > 0 {
		for _, err := range errs {
			MaybeErrorln(c, err.Error())
		}
	}
	return hostsfile
}

// MaybeSaveHostFile will output or write the Hostfile, or exit 1 and error.
func MaybeSaveHostFile(c *cli.Context, hostfile *Hostfile) {
	// If -n is passed, no-op and output the resultant hosts file to stdout.
	// Otherwise it's for real and we're going to write it.
	if AnyBool(c, "n") {
		fmt.Printf("%s", hostfile.Format())
	} else {
		err := hostfile.Save()
		if err != nil {
			MaybeError(c, ErrCantWriteHostFile.Error())
		}
	}
}

// StrPadRight adds spaces to the right of a string until it reaches l length.
// If the input string is already that long, do nothing.
func StrPadRight(s string, l int) string {
	r := l - len(s)
	if r < 0 {
		r = 0
	}
	return s + strings.Repeat(" ", r)
}

// Add command parses <hostname> <ip> and adds or updates a hostname in the
// hosts file. If the aff command is used the hostname will be disabled or
// added in the off state.
func Add(c *cli.Context) {
	if len(c.Args()) != 2 {
		MaybeError(c, "expected <hostname> <ip>")
	}

	hostsfile := MaybeLoadHostFile(c)

	hostname, err := NewHostname(c.Args()[0], c.Args()[1], true)
	if err != nil {
		MaybeError(c, fmt.Sprintf("Failed to parse hosts entry: %s", err))
	}
	// If the command is aff instead of add then the entry should be disabled
	if c.Command.Name == "aff" {
		hostname.Enabled = false
	}

	replace := hostsfile.Hosts.ContainsDomain(hostname.Domain)
	// Note that Add() may return an error, but they are informational only. We
	// don't actually care what the error is -- we just want to add the
	// hostname and save the file. This way the behavior is idempotent.
	hostsfile.Hosts.Add(hostname)

	// If the user passes -n then we'll Add and show the new hosts file, but
	// not save it.
	if c.Bool("n") || AnyBool(c, "n") {
		fmt.Printf("%s", hostsfile.Format())
	} else {
		MaybeSaveHostFile(c, hostsfile)
		// We'll give a little bit of information about whether we added or
		// updated, but if the user wants to know they can use has or ls to
		// show the file before they run the operation. Maybe later we can add
		// a verbose flag to show more information.
		if replace {
			MaybePrintln(c, fmt.Sprintf("Updated %s", hostname.FormatHuman()))
		} else {
			MaybePrintln(c, fmt.Sprintf("Added %s", hostname.FormatHuman()))
		}
	}
}

// Del command removes any hostname(s) matching <domain> from the hosts file
func Del(c *cli.Context) {
	if len(c.Args()) != 1 {
		MaybeError(c, "expected <hostname>")
	}
	domain := c.Args()[0]
	hostsfile := MaybeLoadHostFile(c)

	found := hostsfile.Hosts.ContainsDomain(domain)
	if found {
		hostsfile.Hosts.RemoveDomain(domain)
		if AnyBool(c, "n") {
			fmt.Printf("%s", hostsfile.Format())
		} else {
			MaybeSaveHostFile(c, hostsfile)
			MaybePrintln(c, fmt.Sprintf("Deleted %s", domain))
		}
	} else {
		MaybePrintln(c, fmt.Sprintf("%s not found in %s", domain, GetHostsPath()))
	}
}

// Has command indicates whether a hostname is present in the hosts file
func Has(c *cli.Context) {
	if len(c.Args()) != 1 {
		MaybeError(c, "expected <hostname>")
	}
	domain := c.Args()[0]
	hostsfile := MaybeLoadHostFile(c)

	found := hostsfile.Hosts.ContainsDomain(domain)
	if found {
		MaybePrintln(c, fmt.Sprintf("Found %s in %s", domain, GetHostsPath()))
	} else {
		MaybeError(c, fmt.Sprintf("%s not found in %s", domain, GetHostsPath()))
	}

}

// OnOff enables (uncomments) or disables (comments) the specified hostname in
// the hosts file. Exits code 1 if the hostname is missing.
func OnOff(c *cli.Context) {
	if len(c.Args()) != 1 {
		MaybeError(c, "expected <hostname>")
	}
	domain := c.Args()[0]
	hostsfile := MaybeLoadHostFile(c)

	// Switch on / off commands
	success := false
	if c.Command.Name == "on" {
		success = hostsfile.Hosts.Enable(domain)
	} else {
		success = hostsfile.Hosts.Disable(domain)
	}

	if success {
		MaybeSaveHostFile(c, hostsfile)
		if c.Command.Name == "on" {
			MaybePrintln(c, fmt.Sprintf("Enabled %s", domain))
		} else {
			MaybePrintln(c, fmt.Sprintf("Disabled %s", domain))
		}
	} else {
		MaybeError(c, fmt.Sprintf("%s not found in %s", domain, GetHostsPath()))
	}
}

// Ls command shows a list of hostnames in the hosts file
func Ls(c *cli.Context) {
	hostsfile := AlwaysLoadHostFile(c)
	maxdomain := 0
	maxip := 0
	for _, hostname := range hostsfile.Hosts {
		dlen := len(hostname.Domain)
		if dlen > maxdomain {
			maxdomain = dlen
		}
		ilen := len(hostname.IP)
		if ilen > maxip {
			maxip = ilen
		}
	}

	for _, hostname := range hostsfile.Hosts {
		fmt.Printf("%s -> %s %s\n",
			StrPadRight(hostname.Domain, maxdomain),
			StrPadRight(hostname.IP.String(), maxip),
			hostname.FormatEnabled())
	}
}

const fixHelp = `Programmatically rewrite your hostsfile.

Domains pointing to the same IP will be consolidated onto single lines and
sorted. Duplicates and conflicts will be removed. Extra whitespace and comments
will be removed.

   hostess fix      Rewrite the hostsfile
   hostess fix -n   Show the new hostsfile. Don't write it to disk.
`

// Fix command removes duplicates and conflicts from the hosts file
func Fix(c *cli.Context) {
	hostsfile := AlwaysLoadHostFile(c)
	if bytes.Equal(hostsfile.GetData(), hostsfile.Format()) {
		MaybePrintln(c, fmt.Sprintf("%s is already formatted and contains no dupes or conflicts; nothing to do", GetHostsPath()))
		os.Exit(0)
	}
	MaybeSaveHostFile(c, hostsfile)
}

// Fixed command removes duplicates and conflicts from the hosts file
func Fixed(c *cli.Context) {
	hostsfile := AlwaysLoadHostFile(c)
	if bytes.Equal(hostsfile.GetData(), hostsfile.Format()) {
		MaybePrintln(c, fmt.Sprintf("%s is already formatted and contains no dupes or conflicts", GetHostsPath()))
		os.Exit(0)
	} else {
		MaybePrintln(c, fmt.Sprintf("%s is not formatted. Use hostess fix to format it", GetHostsPath()))
		os.Exit(1)
	}
}

// Dump command outputs hosts file contents as JSON
func Dump(c *cli.Context) {
	hostsfile := AlwaysLoadHostFile(c)
	jsonbytes, err := hostsfile.Hosts.Dump()
	if err != nil {
		MaybeError(c, err.Error())
	}
	fmt.Println(fmt.Sprintf("%s", jsonbytes))
}

// Apply command adds hostnames to the hosts file from JSON
func Apply(c *cli.Context) {
	if len(c.Args()) != 1 {
		MaybeError(c, "Usage should be apply [filename]")
	}
	filename := c.Args()[0]

	jsonbytes, err := ioutil.ReadFile(filename)
	if err != nil {
		MaybeError(c, fmt.Sprintf("Unable to read %s: %s", filename, err))
	}

	hostfile := AlwaysLoadHostFile(c)
	err = hostfile.Hosts.Apply(jsonbytes)
	if err != nil {
		MaybeError(c, fmt.Sprintf("Error applying changes to hosts file: %s", err))
	}

	MaybeSaveHostFile(c, hostfile)
	MaybePrintln(c, fmt.Sprintf("%s applied", filename))
}
