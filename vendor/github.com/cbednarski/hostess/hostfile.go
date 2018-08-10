package hostess

import (
	"fmt"
	"io/ioutil"
	"os"
	"runtime"
	"strings"
)

const defaultOSX = `
##
# Host Database
#
# localhost is used to configure the loopback interface
# when the system is booting.  Do not change this entry.
##

127.0.0.1       localhost
255.255.255.255 broadcasthost
::1             localhost
fe80::1%lo0     localhost
`

const defaultLinux = `
127.0.0.1   localhost
127.0.1.1   HOSTNAME

# The following lines are desirable for IPv6 capable hosts
::1     localhost ip6-localhost ip6-loopback
fe00::0 ip6-localnet
ff00::0 ip6-mcastprefix
ff02::1 ip6-allnodes
ff02::2 ip6-allrouters
ff02::3 ip6-allhosts
`

// Hostfile represents /etc/hosts (or a similar file, depending on OS), and
// includes a list of Hostnames. Hostfile includes
type Hostfile struct {
	Path  string
	Hosts Hostlist
	data  []byte
}

// NewHostfile creates a new Hostfile object from the specified file.
func NewHostfile() *Hostfile {
	return &Hostfile{GetHostsPath(), Hostlist{}, []byte{}}
}

// GetHostsPath returns the location of the hostfile; either env HOSTESS_PATH
// or /etc/hosts if HOSTESS_PATH is not set.
func GetHostsPath() string {
	path := os.Getenv("HOSTESS_PATH")
	if path == "" {
		if runtime.GOOS == "windows" {
			path = "C:\\Windows\\System32\\drivers\\etc\\hosts"
		} else {
			path = "/etc/hosts"
		}
	}
	return path
}

// TrimWS (Trim Whitespace) removes space, newline, and tabs from a string
// using strings.Trim()
func TrimWS(s string) string {
	return strings.TrimSpace(s)
}

// ParseLine parses an individual line in a hostfile, which may contain one
// (un)commented ip and one or more hostnames. For example
//
//	127.0.0.1 localhost mysite1 mysite2
func ParseLine(line string) (Hostlist, error) {
	var hostnames Hostlist

	if len(line) == 0 {
		return hostnames, fmt.Errorf("line is blank")
	}

	// Parse leading # for disabled lines
	enabled := true
	if line[0:1] == "#" {
		enabled = false
		line = TrimWS(line[1:])
	}

	// Parse other #s for actual comments
	line = strings.Split(line, "#")[0]

	// Replace tabs and multispaces with single spaces throughout
	line = strings.Replace(line, "\t", " ", -1)
	for strings.Contains(line, "  ") {
		line = strings.Replace(line, "  ", " ", -1)
	}
	line = TrimWS(line)

	// Break line into words
	words := strings.Split(line, " ")
	for idx, word := range words {
		words[idx] = TrimWS(word)
	}

	// Separate the first bit (the ip) from the other bits (the domains)
	ip := words[0]
	domains := words[1:]

	// if LooksLikeIPv4(ip) || LooksLikeIPv6(ip) {
	for _, v := range domains {
		hostname, err := NewHostname(v, ip, enabled)
		if err != nil {
			return nil, err
		}
		hostnames = append(hostnames, hostname)
	}
	// }

	return hostnames, nil
}

// MustParseLine is like ParseLine but panics instead of errors.
func MustParseLine(line string) Hostlist {
	hostlist, err := ParseLine(line)
	if err != nil {
		panic(err)
	}
	return hostlist
}

// Parse reads
func (h *Hostfile) Parse() []error {
	var errs []error
	var line = 1
	for _, v := range strings.Split(string(h.data), "\n") {
		hostnames, _ := ParseLine(v)
		// if err != nil {
		// 	log.Printf("Error parsing line %d: %s\n", line, err)
		// }
		for _, hostname := range hostnames {
			err := h.Hosts.Add(hostname)
			if err != nil {
				errs = append(errs, err)
			}
		}
		line++
	}
	return errs
}

// Read the contents of the hostfile from disk
func (h *Hostfile) Read() error {
	data, err := ioutil.ReadFile(h.Path)
	if err == nil {
		h.data = data
	}
	return err
}

// LoadHostfile creates a new Hostfile struct and tries to populate it from
// disk. Read and/or parse errors are returned as a slice.
func LoadHostfile() (hostfile *Hostfile, errs []error) {
	hostfile = NewHostfile()
	readErr := hostfile.Read()
	if readErr != nil {
		errs = []error{readErr}
		return
	}
	errs = hostfile.Parse()
	hostfile.Hosts.Sort()
	return
}

// GetData returns the internal snapshot of the hostfile we read when we loaded
// this hostfile from disk (if we ever did that). This is implemented for
// testing and you probably won't need to use it.
func (h *Hostfile) GetData() []byte {
	return h.data
}

// Format takes the current list of Hostnames in this Hostfile and turns it
// into a string suitable for use as an /etc/hosts file.
// Sorting uses the following logic:
// 1. List is sorted by IP address
// 2. Commented items are left in place
// 3. 127.* appears at the top of the list (so boot resolvers don't break)
// 4. When present, localhost will always appear first in the domain list
func (h *Hostfile) Format() []byte {
	return h.Hosts.Format()
}

// Save writes the Hostfile to disk to /etc/hosts or to the location specified
// by the HOSTESS_PATH environment variable (if set).
func (h *Hostfile) Save() error {
	file, err := os.OpenFile(h.Path, os.O_RDWR|os.O_APPEND|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}

	defer file.Close()
	_, err = file.Write(h.Format())

	return err
}
