package hostess

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
)

// ErrInvalidVersionArg is raised when a function expects IPv 4 or 6 but is
// passed a value not 4 or 6.
var ErrInvalidVersionArg = errors.New("Version argument must be 4 or 6")

// Hostlist is a sortable set of Hostnames. When in a Hostlist, Hostnames must
// follow some rules:
//
// 	- Hostlist may contain IPv4 AND IPv6 ("IP version" or "IPv") Hostnames.
// 	- Names are only allowed to overlap if IP version is different.
// 	- Adding a Hostname for an existing name will replace the old one.
//
// The Hostlist uses a deterministic Sort order designed to make a hostfile
// output look a particular way. Generally you don't need to worry about this
// as Sort will be called automatically before Format. However, the Hostlist
// may or may not be sorted at any particular time during runtime.
//
// See the docs and implementation in Sort and Add for more details.
type Hostlist []*Hostname

// NewHostlist initializes a new Hostlist
func NewHostlist() *Hostlist {
	return &Hostlist{}
}

// Len returns the number of Hostnames in the list, part of sort.Interface
func (h Hostlist) Len() int {
	return len(h)
}

// MakeSurrogateIP takes an IP like 127.0.0.1 and munges it to 0.0.0.1 so we can
// sort it more easily. Note that we don't actually want to change the value,
// so we use value copies here (not pointers).
func MakeSurrogateIP(IP net.IP) net.IP {
	if len(IP.String()) > 3 && IP.String()[0:3] == "127" {
		return net.ParseIP("0" + IP.String()[3:])
	}
	return IP
}

// Less determines the sort order of two Hostnames, part of sort.Interface
func (h Hostlist) Less(A, B int) bool {
	// Sort IPv4 before IPv6
	// A is IPv4 and B is IPv6. A wins!
	if !h[A].IPv6 && h[B].IPv6 {
		return true
	}
	// A is IPv6 but B is IPv4. A loses!
	if h[A].IPv6 && !h[B].IPv6 {
		return false
	}

	// Sort "localhost" at the top
	if h[A].Domain == "localhost" {
		return true
	}
	if h[B].Domain == "localhost" {
		return false
	}

	// Compare the the IP addresses (byte array)
	// We want to push 127. to the top so we're going to mark it zero.
	surrogateA := MakeSurrogateIP(h[A].IP)
	surrogateB := MakeSurrogateIP(h[B].IP)
	if !surrogateA.Equal(surrogateB) {
		for charIndex := range surrogateA {
			// A and B's IPs differ at this index, and A is less. A wins!
			if surrogateA[charIndex] < surrogateB[charIndex] {
				return true
			}
			// A and B's IPs differ at this index, and B is less. A loses!
			if surrogateA[charIndex] > surrogateB[charIndex] {
				return false
			}
		}
		// If we got here then the IPs are the same and we want to continue on
		// to the domain sorting section.
	}

	// Prep for sorting by domain name
	aLength := len(h[A].Domain)
	bLength := len(h[B].Domain)
	max := aLength
	if bLength > max {
		max = bLength
	}

	// Sort domains alphabetically
	// TODO: This works best if domains are lowercased. However, we do not
	// enforce lowercase because of UTF-8 domain names, which may be broken by
	// case folding. There is a way to do this correctly but it's complicated
	// so I'm not going to do it right now.
	for charIndex := 0; charIndex < max; charIndex++ {
		// This index is longer than A, so A is shorter. A wins!
		if charIndex >= aLength {
			return true
		}
		// This index is longer than B, so B is shorter. A loses!
		if charIndex >= bLength {
			return false
		}
		// A and B differ at this index and A is less. A wins!
		if h[A].Domain[charIndex] < h[B].Domain[charIndex] {
			return true
		}
		// A and B differ at this index and B is less. A loses!
		if h[A].Domain[charIndex] > h[B].Domain[charIndex] {
			return false
		}
	}

	// If we got here then A and B are the same -- by definition A is not Less
	// than B so we return false. Technically we shouldn't get here since Add
	// should not allow duplicates, but we'll guard anyway.
	return false
}

// Swap changes the position of two Hostnames, part of sort.Interface
func (h Hostlist) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

// Sort this list of Hostnames, according to Hostlist sorting rules:
//
// 	1. localhost comes before other domains
// 	2. IPv4 comes before IPv6
// 	3. IPs are sorted in numerical order
// 	4. domains are sorted in alphabetical
func (h *Hostlist) Sort() {
	sort.Sort(*h)
}

// Contains returns true if this Hostlist has the specified Hostname
func (h *Hostlist) Contains(b *Hostname) bool {
	for _, a := range *h {
		if a.Equal(b) {
			return true
		}
	}
	return false
}

// ContainsDomain returns true if a Hostname in this Hostlist matches domain
func (h *Hostlist) ContainsDomain(domain string) bool {
	for _, hostname := range *h {
		if hostname.Domain == domain {
			return true
		}
	}
	return false
}

// ContainsIP returns true if a Hostname in this Hostlist matches IP
func (h *Hostlist) ContainsIP(IP net.IP) bool {
	for _, hostname := range *h {
		if hostname.EqualIP(IP) {
			return true
		}
	}
	return false
}

// Add a new Hostname to this hostlist. Add uses some merging logic in the
// event it finds duplicated hostnames. In the case of a conflict (incompatible
// entries) the last write wins. In the case of duplicates, duplicates will be
// removed and the remaining entry will be enabled if any of the duplicates was
// enabled.
//
// Both duplicate and conflicts return errors so you are aware of them, but you
// don't necessarily need to do anything about the error.
func (h *Hostlist) Add(hostnamev *Hostname) error {
	hostname, err := NewHostname(hostnamev.Domain, hostnamev.IP.String(), hostnamev.Enabled)
	if err != nil {
		return err
	}
	for index, found := range *h {
		if found.Equal(hostname) {
			// If either hostname is enabled we will set the existing one to
			// enabled state. That way if we add a hostname from the end of a
			// hosts file it will take over, and if we later add a disabled one
			// the original one will stick. We still error in this case so the
			// user can see that there is a duplicate.
			(*h)[index].Enabled = found.Enabled || hostname.Enabled
			return fmt.Errorf("Duplicate hostname entry for %s -> %s",
				hostname.Domain, hostname.IP)
		} else if found.Domain == hostname.Domain && found.IPv6 == hostname.IPv6 {
			(*h)[index] = hostname
			return fmt.Errorf("Conflicting hostname entries for %s -> %s and -> %s",
				hostname.Domain, hostname.IP, found.IP)
		}
	}
	*h = append(*h, hostname)
	return nil
}

// IndexOf will indicate the index of a Hostname in Hostlist, or -1 if it is
// not found.
func (h *Hostlist) IndexOf(host *Hostname) int {
	for index, found := range *h {
		if found.Equal(host) {
			return index
		}
	}
	return -1
}

// IndexOfDomainV will indicate the index of a Hostname in Hostlist that has
// the same domain and IP version, or -1 if it is not found.
//
// This function will panic if IP version is not 4 or 6.
func (h *Hostlist) IndexOfDomainV(domain string, version int) int {
	if version != 4 && version != 6 {
		panic(ErrInvalidVersionArg)
	}
	for index, hostname := range *h {
		if hostname.Domain == domain && hostname.IPv6 == (version == 6) {
			return index
		}
	}
	return -1
}

// Remove will delete the Hostname at the specified index. If index is out of
// bounds (i.e. -1), Remove silently no-ops. Remove returns the number of items
// removed (0 or 1).
func (h *Hostlist) Remove(index int) int {
	if index > -1 && index < len(*h) {
		*h = append((*h)[:index], (*h)[index+1:]...)
		return 1
	}
	return 0
}

// RemoveDomain removes both IPv4 and IPv6 Hostname entries matching domain.
// Returns the number of entries removed.
func (h *Hostlist) RemoveDomain(domain string) int {
	return h.RemoveDomainV(domain, 4) + h.RemoveDomainV(domain, 6)
}

// RemoveDomainV removes a Hostname entry matching the domain and IP version.
func (h *Hostlist) RemoveDomainV(domain string, version int) int {
	return h.Remove(h.IndexOfDomainV(domain, version))
}

// Enable will change any Hostnames matching domain to be enabled.
func (h *Hostlist) Enable(domain string) bool {
	found := false
	for _, hostname := range *h {
		if hostname.Domain == domain {
			hostname.Enabled = true
			found = true
		}
	}
	return found
}

// EnableV will change a Hostname matching domain and IP version to be enabled.
//
// This function will panic if IP version is not 4 or 6.
func (h *Hostlist) EnableV(domain string, version int) bool {
	found := false
	if version != 4 && version != 6 {
		panic(ErrInvalidVersionArg)
	}
	for _, hostname := range *h {
		if hostname.Domain == domain && hostname.IPv6 == (version == 6) {
			hostname.Enabled = true
			found = true
		}
	}
	return found
}

// Disable will change any Hostnames matching domain to be disabled.
func (h *Hostlist) Disable(domain string) bool {
	found := false
	for _, hostname := range *h {
		if hostname.Domain == domain {
			hostname.Enabled = false
			found = true
		}
	}
	return found
}

// DisableV will change any Hostnames matching domain and IP version to be disabled.
//
// This function will panic if IP version is not 4 or 6.
func (h *Hostlist) DisableV(domain string, version int) bool {
	found := false
	if version != 4 && version != 6 {
		panic(ErrInvalidVersionArg)
	}
	for _, hostname := range *h {
		if hostname.Domain == domain && hostname.IPv6 == (version == 6) {
			hostname.Enabled = false
			found = true
		}
	}
	return found
}

// FilterByIP filters the list of hostnames by IP address.
func (h *Hostlist) FilterByIP(IP net.IP) (hostnames []*Hostname) {
	for _, hostname := range *h {
		if hostname.IP.Equal(IP) {
			hostnames = append(hostnames, hostname)
		}
	}
	return
}

// FilterByDomain filters the list of hostnames by Domain.
func (h *Hostlist) FilterByDomain(domain string) (hostnames []*Hostname) {
	for _, hostname := range *h {
		if hostname.Domain == domain {
			hostnames = append(hostnames, hostname)
		}
	}
	return
}

// FilterByDomainV filters the list of hostnames by domain and IPv4 or IPv6.
// This should never contain more than one item, but returns a list for
// consistency with other filter functions.
//
// This function will panic if IP version is not 4 or 6.
func (h *Hostlist) FilterByDomainV(domain string, version int) (hostnames []*Hostname) {
	if version != 4 && version != 6 {
		panic(ErrInvalidVersionArg)
	}
	for _, hostname := range *h {
		if hostname.Domain == domain && hostname.IPv6 == (version == 6) {
			hostnames = append(hostnames, hostname)
		}
	}
	return
}

// GetUniqueIPs extracts an ordered list of unique IPs from the Hostlist.
// This calls Sort() internally.
func (h *Hostlist) GetUniqueIPs() []net.IP {
	h.Sort()
	// A map doesn't preserve order so we're going to use the map to check
	// whether we've seen something and use the list to keep track of the
	// order.
	seen := make(map[string]bool)
	inOrder := []net.IP{}

	for _, hostname := range *h {
		key := (*hostname).IP.String()
		if !seen[key] {
			seen[key] = true
			inOrder = append(inOrder, (*hostname).IP)
		}
	}
	return inOrder
}

// Format takes the current list of Hostnames in this Hostfile and turns it
// into a string suitable for use as an /etc/hosts file.
// Sorting uses the following logic:
//
// 1. List is sorted by IP address
// 2. Commented items are sorted displayed
// 3. 127.* appears at the top of the list (so boot resolvers don't break)
// 4. When present, "localhost" will always appear first in the domain list
func (h *Hostlist) Format() []byte {
	h.Sort()
	out := []byte{}

	// We want to output one line of hostnames per IP, so first we get that
	// list of IPs and iterate.
	for _, IP := range h.GetUniqueIPs() {
		// Technically if an IP has some disabled hostnames we'll show two
		// lines, one starting with a comment (#).
		enabledIPs := []string{}
		disabledIPs := []string{}

		// For this IP, get all hostnames that match and iterate over them.
		for _, hostname := range h.FilterByIP(IP) {
			// If it's enabled, put it in the enabled bucket (likewise for
			// disabled hostnames)
			if hostname.Enabled {
				enabledIPs = append(enabledIPs, hostname.Domain)
			} else {
				disabledIPs = append(disabledIPs, hostname.Domain)
			}
		}

		// Finally, if the bucket contains anything, concatenate it all
		// together and append it to the output. Also add a newline.
		if len(enabledIPs) > 0 {
			concat := fmt.Sprintf("%s %s", IP.String(), strings.Join(enabledIPs, " "))
			out = append(out, []byte(concat)...)
			out = append(out, []byte("\n")...)
		}

		if len(disabledIPs) > 0 {
			concat := fmt.Sprintf("# %s %s", IP.String(), strings.Join(disabledIPs, " "))
			out = append(out, []byte(concat)...)
			out = append(out, []byte("\n")...)
		}
	}

	return out
}

// Dump exports all entries in the Hostlist as JSON
func (h *Hostlist) Dump() ([]byte, error) {
	return json.MarshalIndent(h, "", "  ")
}

// Apply imports all entries from the JSON input to this Hostlist
func (h *Hostlist) Apply(jsonbytes []byte) error {
	var hostnames Hostlist
	err := json.Unmarshal(jsonbytes, &hostnames)
	if err != nil {
		return err
	}

	for _, hostname := range hostnames {
		h.Add(hostname)
	}

	return nil
}
