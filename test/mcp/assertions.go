//go:build live

package mcp

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/txn2/kubefwd/pkg/fwdapi/types"
)

// === HTTP Connectivity Assertions ===

// AssertHTTPReachable verifies that an HTTP endpoint is reachable and returns the expected status.
func AssertHTTPReachable(t *testing.T, url string, expectedStatus int) {
	t.Helper()

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("HTTP request to %s failed: %v", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != expectedStatus {
		t.Errorf("Expected status %d from %s, got %d", expectedStatus, url, resp.StatusCode)
	}
}

// AssertHTTPReachableWithRetry verifies HTTP reachability with retries.
func AssertHTTPReachableWithRetry(t *testing.T, url string, expectedStatus int, retries int, delay time.Duration) {
	t.Helper()

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	var lastErr error
	for i := 0; i < retries; i++ {
		resp, err := client.Get(url)
		if err != nil {
			lastErr = err
			time.Sleep(delay)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode == expectedStatus {
			return // Success
		}
		lastErr = fmt.Errorf("unexpected status %d", resp.StatusCode)
		time.Sleep(delay)
	}

	t.Fatalf("HTTP request to %s failed after %d retries: %v", url, retries, lastErr)
}

// AssertTCPReachable verifies that a TCP endpoint is reachable.
func AssertTCPReachable(t *testing.T, address string) {
	t.Helper()

	conn, err := net.DialTimeout("tcp", address, 5*time.Second)
	if err != nil {
		t.Fatalf("TCP connection to %s failed: %v", address, err)
	}
	conn.Close()
}

// AssertTCPReachableWithRetry verifies TCP reachability with retries.
func AssertTCPReachableWithRetry(t *testing.T, address string, retries int, delay time.Duration) {
	t.Helper()

	var lastErr error
	for i := 0; i < retries; i++ {
		conn, err := net.DialTimeout("tcp", address, 3*time.Second)
		if err != nil {
			lastErr = err
			time.Sleep(delay)
			continue
		}
		conn.Close()
		return // Success
	}

	t.Fatalf("TCP connection to %s failed after %d retries: %v", address, retries, lastErr)
}

// AssertTCPNotReachable verifies that a TCP endpoint is NOT reachable.
func AssertTCPNotReachable(t *testing.T, address string) {
	t.Helper()

	conn, err := net.DialTimeout("tcp", address, 2*time.Second)
	if err == nil {
		conn.Close()
		t.Errorf("Expected TCP connection to %s to fail, but it succeeded", address)
	}
}

// === Service State Assertions ===

// AssertServiceCount verifies the number of services matches expected.
func AssertServiceCount(t *testing.T, services []types.ServiceResponse, expected int) {
	t.Helper()

	if len(services) != expected {
		t.Errorf("Expected %d services, got %d", expected, len(services))
	}
}

// AssertServiceCountAtLeast verifies at least n services exist.
func AssertServiceCountAtLeast(t *testing.T, services []types.ServiceResponse, minimum int) {
	t.Helper()

	if len(services) < minimum {
		t.Errorf("Expected at least %d services, got %d", minimum, len(services))
	}
}

// AssertNoServicesInNamespace verifies no services exist for a namespace.
func AssertNoServicesInNamespace(t *testing.T, services []types.ServiceResponse, namespace string) {
	t.Helper()

	for _, svc := range services {
		if svc.Namespace == namespace {
			t.Errorf("Found unexpected service %s in namespace %s", svc.Key, namespace)
		}
	}
}

// AssertServicesInNamespace verifies services exist for a namespace.
func AssertServicesInNamespace(t *testing.T, services []types.ServiceResponse, namespace string, minimum int) {
	t.Helper()

	count := 0
	for _, svc := range services {
		if svc.Namespace == namespace {
			count++
		}
	}

	if count < minimum {
		t.Errorf("Expected at least %d services in namespace %s, got %d", minimum, namespace, count)
	}
}

// AssertServiceExists verifies a service exists by key or name pattern.
func AssertServiceExists(t *testing.T, services []types.ServiceResponse, pattern string) {
	t.Helper()

	for _, svc := range services {
		if svc.Key == pattern || strings.Contains(svc.ServiceName, pattern) {
			return // Found
		}
	}

	t.Errorf("Expected to find service matching %q", pattern)
}

// AssertServiceNotExists verifies a service does NOT exist.
func AssertServiceNotExists(t *testing.T, services []types.ServiceResponse, pattern string) {
	t.Helper()

	for _, svc := range services {
		if svc.Key == pattern || strings.Contains(svc.ServiceName, pattern) {
			t.Errorf("Expected service %q to NOT exist, but found %s", pattern, svc.Key)
		}
	}
}

// AssertAllServicesActive verifies all services have active status.
func AssertAllServicesActive(t *testing.T, services []types.ServiceResponse) {
	t.Helper()

	for _, svc := range services {
		if svc.Status != "active" {
			t.Errorf("Expected service %s to be active, got %s", svc.Key, svc.Status)
		}
	}
}

// AssertNoServiceErrors verifies no services are in error state.
func AssertNoServiceErrors(t *testing.T, services []types.ServiceResponse) {
	t.Helper()

	for _, svc := range services {
		if svc.Status == "error" || svc.ErrorCount > 0 {
			t.Errorf("Service %s has errors (status=%s, errorCount=%d)", svc.Key, svc.Status, svc.ErrorCount)
		}
	}
}

// === Headless Service Assertions ===

// AssertHeadlessServiceHasPodEntries verifies a headless service has pod-specific entries.
func AssertHeadlessServiceHasPodEntries(t *testing.T, services []types.ServiceResponse, headlessServiceName string) {
	t.Helper()

	// Look for entries like "pod-name.service-name" pattern
	found := false
	for _, svc := range services {
		if strings.Contains(svc.Key, headlessServiceName) && strings.Contains(svc.ServiceName, ".") {
			// This looks like a pod-specific headless entry
			found = true
			break
		}
	}

	if !found {
		t.Errorf("Expected to find pod-specific entries for headless service %s", headlessServiceName)
	}
}

// AssertNoOrphanedHeadlessEntries verifies no orphaned headless entries remain after namespace removal.
func AssertNoOrphanedHeadlessEntries(t *testing.T, services []types.ServiceResponse, namespace string) {
	t.Helper()

	for _, svc := range services {
		if svc.Namespace == namespace {
			t.Errorf("Found orphaned service entry: %s (this may indicate headless service cleanup bug)", svc.Key)
		}
	}
}

// === Connection Info Assertions ===

// AssertConnectionInfoValid verifies connection info has required fields.
func AssertConnectionInfoValid(t *testing.T, info *types.ConnectionInfoResponse) {
	t.Helper()

	if info == nil {
		t.Fatal("Connection info is nil")
	}

	if info.LocalIP == "" {
		t.Error("Connection info missing LocalIP")
	}

	if len(info.Hostnames) == 0 {
		t.Error("Connection info has no hostnames")
	}

	if len(info.Ports) == 0 {
		t.Error("Connection info has no ports")
	}
}

// AssertConnectionInfoHasPort verifies connection info includes a specific port.
func AssertConnectionInfoHasPort(t *testing.T, info *types.ConnectionInfoResponse, port int) {
	t.Helper()

	if info == nil {
		t.Fatal("Connection info is nil")
	}

	for _, p := range info.Ports {
		if p.LocalPort == port || p.RemotePort == port {
			return // Found
		}
	}

	t.Errorf("Connection info does not include port %d", port)
}

// === Hostname Assertions ===

// AssertHostnameExists verifies a hostname is registered.
func AssertHostnameExists(t *testing.T, hostnames []types.HostnameEntry, hostname string) {
	t.Helper()

	for _, h := range hostnames {
		if h.Hostname == hostname {
			return // Found
		}
	}

	t.Errorf("Expected hostname %q to exist", hostname)
}

// AssertHostnameNotExists verifies a hostname is NOT registered.
func AssertHostnameNotExists(t *testing.T, hostnames []types.HostnameEntry, hostname string) {
	t.Helper()

	for _, h := range hostnames {
		if h.Hostname == hostname {
			t.Errorf("Expected hostname %q to NOT exist", hostname)
			return
		}
	}
}

// AssertNoHostnamesForNamespace verifies no hostnames exist for a namespace.
func AssertNoHostnamesForNamespace(t *testing.T, hostnames []types.HostnameEntry, namespace string) {
	t.Helper()

	for _, h := range hostnames {
		if h.Namespace == namespace {
			t.Errorf("Found unexpected hostname %s for namespace %s", h.Hostname, namespace)
		}
	}
}

// === Wait Helpers ===

// WaitForCondition waits for a condition to be true with timeout.
func WaitForCondition(t *testing.T, timeout time.Duration, interval time.Duration, condition func() bool, description string) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(interval)
	}

	t.Fatalf("Timeout waiting for condition: %s", description)
}

// WaitForServices waits for a minimum number of services to appear.
func WaitForServices(t *testing.T, client *Client, namespace string, minimum int, timeout time.Duration) []types.ServiceResponse {
	t.Helper()

	var services []types.ServiceResponse
	WaitForCondition(t, timeout, 500*time.Millisecond, func() bool {
		services = client.ListServices(t, namespace)
		return len(services) >= minimum
	}, fmt.Sprintf("at least %d services in namespace %s", minimum, namespace))

	return services
}

// WaitForNoServices waits for all services in a namespace to be removed.
func WaitForNoServices(t *testing.T, client *Client, namespace string, timeout time.Duration) {
	t.Helper()

	WaitForCondition(t, timeout, 500*time.Millisecond, func() bool {
		services := client.ListServices(t, namespace)
		return len(services) == 0
	}, fmt.Sprintf("no services in namespace %s", namespace))
}

// WaitForHTTPReady waits for an HTTP endpoint to be ready.
func WaitForHTTPReady(t *testing.T, url string, expectedStatus int, timeout time.Duration) {
	t.Helper()

	client := &http.Client{
		Timeout: 3 * time.Second,
	}

	WaitForCondition(t, timeout, 1*time.Second, func() bool {
		resp, err := client.Get(url)
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		return resp.StatusCode == expectedStatus
	}, fmt.Sprintf("HTTP endpoint %s to return %d", url, expectedStatus))
}
