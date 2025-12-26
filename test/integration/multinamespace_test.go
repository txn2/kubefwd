//go:build integration
// +build integration

package integration

import (
	"fmt"
	"testing"
	"time"
)

// TestMultiNamespaceForwarding tests forwarding services from multiple namespaces simultaneously
//
//goland:noinspection HttpUrlsUsage
func TestMultiNamespaceForwarding(t *testing.T) {
	requiresSudo(t)
	requiresKindCluster(t)

	// Start kubefwd for multiple namespaces (kf-a and kf-b)
	cmd := startKubefwd(t, "svc", "-n", "kf-a,kf-b", "-v")
	defer stopKubefwd(t, cmd)

	// Wait for forwarding to stabilize
	t.Log("Waiting for multi-namespace forwarding to stabilize...")
	time.Sleep(15 * time.Second)

	// Expected hostname patterns for multi-namespace:
	// Format: service.namespace (for namespace index > 0)
	expectedHosts := []string{
		"ok.kf-a",
		"ok.kf-b",
		"ok-ss.kf-a",
		"ok-ss.kf-b",
	}

	// Verify all expected hosts are in /etc/hosts
	for _, host := range expectedHosts {
		if !verifyHostsEntry(t, host) {
			t.Errorf("Expected hostname '%s' not found in /etc/hosts", host)
		} else {
			t.Logf("✓ Hostname '%s' found in /etc/hosts", host)
		}
	}

	// Test HTTP connectivity to services from both namespaces
	testConnections := []struct {
		name string
		port int
	}{
		{"ok.kf-a", 8080},
		{"ok.kf-b", 8080},
		{"ok-ss.kf-a", 8080},
		{"ok-ss.kf-b", 8080},
	}

	for _, tc := range testConnections {
		t.Logf("Testing connection to %s:%d...", tc.name, tc.port)
		resp, err := httpGet(fmt.Sprintf("http://%s:%d/", tc.name, tc.port), 5, 2*time.Second)
		if err != nil {
			t.Errorf("Failed to connect to %s: %v", tc.name, err)
			continue
		}
		resp.Body.Close()

		if resp.StatusCode == 200 {
			t.Logf("✓ Successfully connected to %s (HTTP 200)", tc.name)
		} else {
			t.Errorf("Unexpected HTTP status from %s: %d (expected 200)", tc.name, resp.StatusCode)
		}
	}
}

// TestNamespaceIsolation tests that services from different namespaces get different IP addresses
func TestNamespaceIsolation(t *testing.T) {
	requiresSudo(t)
	requiresKindCluster(t)

	// Start kubefwd for multiple namespaces
	cmd := startKubefwd(t, "svc", "-n", "kf-a,kf-b", "-v")
	defer stopKubefwd(t, cmd)

	// Wait for forwarding to stabilize
	t.Log("Waiting for multi-namespace forwarding to stabilize...")
	time.Sleep(15 * time.Second)

	// Get IP addresses for the same service name from different namespaces
	ipKfA := getHostsEntryIP(t, "ok.kf-a")
	ipKfB := getHostsEntryIP(t, "ok.kf-b")

	if ipKfA == "" {
		t.Error("Failed to get IP address for ok.kf-a")
	} else {
		t.Logf("IP for ok.kf-a: %s", ipKfA)
	}

	if ipKfB == "" {
		t.Error("Failed to get IP address for ok.kf-b")
	} else {
		t.Logf("IP for ok.kf-b: %s", ipKfB)
	}

	// Verify that services from different namespaces have different IPs
	if ipKfA == ipKfB {
		t.Errorf("Services from different namespaces have same IP: %s", ipKfA)
		t.Error("Namespace isolation failed - services should have unique IPs")
	} else {
		t.Log("✓ Services from different namespaces have different IPs (namespace isolation working)")
	}

	// Verify both IPs are in the 127.x.x.x range
	if ipKfA != "" && ipKfA[:4] != "127." {
		t.Errorf("IP for kf-a not in loopback range: %s", ipKfA)
	}

	if ipKfB != "" && ipKfB[:4] != "127." {
		t.Errorf("IP for kf-b not in loopback range: %s", ipKfB)
	}

	// Get IPs for StatefulSet services from both namespaces
	ipSSKfA := getHostsEntryIP(t, "ok-ss.kf-a")
	ipSSKfB := getHostsEntryIP(t, "ok-ss.kf-b")

	if ipSSKfA != "" && ipSSKfB != "" {
		t.Logf("IP for ok-ss.kf-a: %s", ipSSKfA)
		t.Logf("IP for ok-ss.kf-b: %s", ipSSKfB)

		if ipSSKfA == ipSSKfB {
			t.Error("StatefulSet services from different namespaces have same IP")
		} else {
			t.Log("✓ StatefulSet services from different namespaces have different IPs")
		}
	}
}

// TestNamespaceHostnames tests hostname patterns for multi-namespace forwarding
//
//goland:noinspection HttpUrlsUsage
func TestNamespaceHostnames(t *testing.T) {
	requiresSudo(t)
	requiresKindCluster(t)

	// Start kubefwd for multiple namespaces
	cmd := startKubefwd(t, "svc", "-n", "kf-a,kf-b", "-v")
	defer stopKubefwd(t, cmd)

	// Wait for forwarding to stabilize
	t.Log("Waiting for multi-namespace forwarding to stabilize...")
	time.Sleep(15 * time.Second)

	// Test hostname patterns
	// When forwarding multiple namespaces, kubefwd should create hostnames
	// in the format: service.namespace
	hostnameTests := []struct {
		hostname string
		desc     string
	}{
		{"ok.kf-a", "normal service in first namespace"},
		{"ok.kf-b", "normal service in second namespace"},
		{"ok-headless.kf-a", "headless service in first namespace"},
		{"ok-headless.kf-b", "headless service in second namespace"},
		{"ok-ss.kf-a", "StatefulSet service in first namespace"},
		{"ok-ss.kf-b", "StatefulSet service in second namespace"},
	}

	t.Log("Verifying hostname patterns...")
	for _, ht := range hostnameTests {
		if verifyHostsEntry(t, ht.hostname) {
			t.Logf("✓ Hostname '%s' found (%s)", ht.hostname, ht.desc)
		} else {
			t.Errorf("Hostname '%s' not found in /etc/hosts (%s)", ht.hostname, ht.desc)
		}
	}

	// Verify that simple service names (without namespace) also exist
	// kubefwd may create both formats depending on configuration
	t.Log("Checking if short names (without namespace) exist...")

	shortNames := []string{"ok", "ok-headless", "ok-ss"}
	for _, name := range shortNames {
		if verifyHostsEntry(t, name) {
			t.Logf("Short name '%s' also present in /etc/hosts", name)
		} else {
			t.Logf("Short name '%s' not found (expected for multi-namespace)", name)
		}
	}

	// Test that we can resolve and connect using namespace-qualified hostnames
	t.Log("Testing connectivity using namespace-qualified hostnames...")
	testHosts := []struct {
		hostname string
		port     int
	}{
		{"ok.kf-a", 8080},
		{"ok.kf-b", 8080},
	}

	for _, th := range testHosts {
		resp, err := httpGet(fmt.Sprintf("http://%s:%d/", th.hostname, th.port), 5, 2*time.Second)
		if err != nil {
			t.Errorf("Failed to connect to %s: %v", th.hostname, err)
			continue
		}
		resp.Body.Close()

		if resp.StatusCode == 200 {
			t.Logf("✓ Successfully connected to %s using namespace-qualified hostname", th.hostname)
		} else {
			t.Errorf("Unexpected HTTP status from %s: %d", th.hostname, resp.StatusCode)
		}
	}
}
