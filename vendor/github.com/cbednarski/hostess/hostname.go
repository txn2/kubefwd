package hostess

import (
	"fmt"
	"net"
	"regexp"
	"strings"
)

var ipv4Pattern = regexp.MustCompile(`^(?:[0-9]{1,3}\.){3}[0-9]{1,3}$`)
var ipv6Pattern = regexp.MustCompile(`^[(a-fA-F0-9){1-4}:]+$`)

// LooksLikeIPv4 returns true if the IP looks like it's IPv4. This does not
// validate whether the string is a valid IP address.
func LooksLikeIPv4(ip string) bool {
	return ipv4Pattern.MatchString(ip)
}

// LooksLikeIPv6 returns true if the IP looks like it's IPv6. This does not
// validate whether the string is a valid IP address.
func LooksLikeIPv6(ip string) bool {
	if !strings.Contains(ip, ":") {
		return false
	}
	return ipv6Pattern.MatchString(ip)
}

// Hostname represents a hosts file entry, including a Domain, IP, whether the
// Hostname is enabled (uncommented in the hosts file), and whether the IP is
// in the IPv6 format. You should always create these with NewHostname(). Note:
// when using Hostnames in the context of a Hostlist, you should not change the
// Hostname fields except through the Hostlist's aggregate methods. Doing so
// can cause unexpected behavior. Instead, use Hostlist's Add, Remove, Enable,
// and Disable methods.
type Hostname struct {
	Domain  string `json:"domain"`
	IP      net.IP `json:"ip"`
	Enabled bool   `json:"enabled"`
	IPv6    bool   `json:"-"`
}

// NewHostname creates a new Hostname struct and automatically sets the IPv6
// field based on the IP you pass in.
func NewHostname(domain, ip string, enabled bool) (*Hostname, error) {
	if !LooksLikeIPv4(ip) && !LooksLikeIPv6(ip) {
		return nil, fmt.Errorf("Unable to parse IP address %q", ip)
	}
	IP := net.ParseIP(ip)
	return &Hostname{domain, IP, enabled, LooksLikeIPv6(ip)}, nil
}

// MustHostname calls NewHostname but panics if there is an error parsing it.
func MustHostname(domain, ip string, enabled bool) *Hostname {
	hostname, err := NewHostname(domain, ip, enabled)
	if err != nil {
		panic(err)
	}
	return hostname
}

// Equal compares two Hostnames. Note that only the Domain and IP fields are
// compared because Enabled is transient state, and IPv6 should be set
// automatically based on IP.
func (h *Hostname) Equal(n *Hostname) bool {
	return h.Domain == n.Domain && h.IP.Equal(n.IP)
}

// EqualIP compares an IP against this Hostname.
func (h *Hostname) EqualIP(ip net.IP) bool {
	return h.IP.Equal(ip)
}

// IsValid does a spot-check on the domain and IP to make sure they aren't blank
func (h *Hostname) IsValid() bool {
	return h.Domain != "" && h.IP != nil
}

// Format outputs the Hostname as you'd see it in a hosts file, with a comment
// if it is disabled. E.g.
// # 127.0.0.1 blah.example.com
func (h *Hostname) Format() string {
	r := fmt.Sprintf("%s %s", h.IP.String(), h.Domain)
	if !h.Enabled {
		r = "# " + r
	}
	return r
}

// FormatEnabled displays Hostname.Enabled as (On) or (Off)
func (h *Hostname) FormatEnabled() string {
	if h.Enabled {
		return "(On)"
	}
	return "(Off)"
}

// FormatHuman outputs the Hostname in a more human-readable format:
// blah.example.com -> 127.0.0.1 (Off)
func (h *Hostname) FormatHuman() string {
	return fmt.Sprintf("%s -> %s %s", h.Domain, h.IP, h.FormatEnabled())
}
