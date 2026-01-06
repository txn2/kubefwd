package fwdip

import (
	"testing"
)

// FuzzIpFromString tests the IP parsing function with random inputs
func FuzzIpFromString(f *testing.F) {
	// Seed corpus with valid and edge-case inputs
	f.Add("127.0.0.1")
	f.Add("127.1.27.1")
	f.Add("255.255.255.255")
	f.Add("0.0.0.0")
	f.Add("")
	f.Add("not-an-ip")
	f.Add("127.0.0")
	f.Add("127.0.0.1.2")
	f.Add("127.0.0.-1")
	f.Add("127.0.0.256")
	f.Add("a.b.c.d")
	f.Add("127.0.0.1:8080")
	f.Add("...")
	f.Add("....")
	f.Add("1.2.3.4.5.6.7.8")

	f.Fuzz(func(_ *testing.T, ipStr string) {
		// Should not panic on any input
		_, _ = ipFromString(ipStr)
	})
}

// FuzzServiceConfigurationFromReservation tests reservation parsing
func FuzzServiceConfigurationFromReservation(f *testing.F) {
	// Seed corpus
	f.Add("myservice:127.0.0.1")
	f.Add("svc:127.1.1.1")
	f.Add("")
	f.Add(":")
	f.Add("noip:")
	f.Add(":noname")
	f.Add("a]b:c[d")
	f.Add("service-name.namespace:127.0.0.1")
	f.Add("service:not-an-ip")
	f.Add("multiple:colons:here")

	f.Fuzz(func(_ *testing.T, reservation string) {
		// Should not panic on any input
		_ = ServiceConfigurationFromReservation(reservation)
	})
}
