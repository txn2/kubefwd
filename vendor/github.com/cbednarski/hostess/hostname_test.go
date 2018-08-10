package hostess_test

import (
	"net"
	"testing"

	"github.com/cbednarski/hostess"
)

func TestHostname(t *testing.T) {
	h := hostess.MustHostname(domain, ip, enabled)

	if h.Domain != domain {
		t.Errorf("Domain should be %s", domain)
	}
	if !h.IP.Equal(net.ParseIP(ip)) {
		t.Errorf("IP should be %s", ip)
	}
	if h.Enabled != enabled {
		t.Errorf("Enabled should be %t", enabled)
	}
}

func TestEqual(t *testing.T) {
	a := hostess.MustHostname("localhost", "127.0.0.1", true)
	b := hostess.MustHostname("localhost", "127.0.0.1", false)
	c := hostess.MustHostname("localhost", "127.0.1.1", false)

	if !a.Equal(b) {
		t.Errorf("%+v and %+v should be equal", a, b)
	}
	if a.Equal(c) {
		t.Errorf("%+v and %+v should not be equal", a, c)
	}
}

func TestEqualIP(t *testing.T) {
	a := hostess.MustHostname("localhost", "127.0.0.1", true)
	c := hostess.MustHostname("localhost", "127.0.1.1", false)
	ip := net.ParseIP("127.0.0.1")

	if !a.EqualIP(ip) {
		t.Errorf("%s and %s should be equal", a.IP, ip)
	}
	if a.EqualIP(c.IP) {
		t.Errorf("%s and %s should not be equal", a.IP, c.IP)
	}
}

func TestIsValid(t *testing.T) {
	hostname := &hostess.Hostname{
		Domain:  "localhost",
		IP:      net.ParseIP("127.0.0.1"),
		Enabled: true,
		IPv6:    true,
	}
	if !hostname.IsValid() {
		t.Fatalf("%+v should be a valid hostname", hostname)
	}
}

func TestIsValidBlank(t *testing.T) {
	hostname := &hostess.Hostname{
		Domain:  "",
		IP:      net.ParseIP("127.0.0.1"),
		Enabled: true,
		IPv6:    true,
	}
	if hostname.IsValid() {
		t.Errorf("%+v should be invalid because the name is blank", hostname)
	}
}
func TestIsValidBadIP(t *testing.T) {
	hostname := &hostess.Hostname{
		Domain:  "localhost",
		IP:      net.ParseIP("localhost"),
		Enabled: true,
		IPv6:    true,
	}
	if hostname.IsValid() {
		t.Errorf("%+v should be invalid because the ip is malformed", hostname)
	}
}

func TestFormatHostname(t *testing.T) {
	hostname := hostess.MustHostname(domain, ip, enabled)

	const exp_enabled = "127.0.0.1 localhost"
	if hostname.Format() != exp_enabled {
		t.Errorf("Hostname format doesn't match desired output: %s", Diff(hostname.Format(), exp_enabled))
	}

	hostname.Enabled = false
	const exp_disabled = "# 127.0.0.1 localhost"
	if hostname.Format() != exp_disabled {
		t.Errorf("Hostname format doesn't match desired output: %s", Diff(hostname.Format(), exp_disabled))
	}
}

func TestFormatEnabled(t *testing.T) {
	hostname := hostess.MustHostname(domain, ip, enabled)
	const expectedOn = "(On)"
	if hostname.FormatEnabled() != expectedOn {
		t.Errorf("Expected hostname to be turned %s", expectedOn)
	}
	const expectedHumanOn = "localhost -> 127.0.0.1 (On)"
	if hostname.FormatHuman() != expectedHumanOn {
		t.Errorf("Unexpected output%s", Diff(expectedHumanOn, hostname.FormatHuman()))
	}

	hostname.Enabled = false
	if hostname.FormatEnabled() != "(Off)" {
		t.Error("Expected hostname to be turned (Off)")
	}
}
