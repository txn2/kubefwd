//go:build integration
// +build integration

package integration

import (
	"fmt"
	"testing"
	"time"
)

// TestForwardSingleService tests forwarding a single service from one namespace
func TestForwardSingleService(t *testing.T) {
	requiresSudo(t)
	requiresKindCluster(t)

	// Start kubefwd for test-simple namespace
	cmd := startKubefwd(t, "svc", "-n", "test-simple", "-v")
	defer stopKubefwd(t, cmd)

	// Wait for forwarding to stabilize
	t.Log("Waiting for forwarding to stabilize...")
	time.Sleep(5 * time.Second)

	// Verify hosts file entry
	if !verifyHostsEntry(t, "nginx") {
		t.Error("Service 'nginx' not found in /etc/hosts")
	} else {
		t.Log("✓ Service 'nginx' found in /etc/hosts")
	}

	// Test HTTP connectivity
	t.Log("Testing HTTP connectivity to nginx service...")
	resp, err := httpGet("http://nginx:80/", 5, 2*time.Second)
	if err != nil {
		t.Fatalf("Failed to connect to forwarded service: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("Expected HTTP 200, got %d", resp.StatusCode)
	} else {
		t.Log("✓ Successfully connected to forwarded service (HTTP 200)")
	}
}

// TestForwardMultipleServices tests forwarding multiple services from one namespace
//
//goland:noinspection DuplicatedCode,HttpUrlsUsage
func TestForwardMultipleServices(t *testing.T) {
	requiresSudo(t)
	requiresKindCluster(t)

	// Start kubefwd for kf-a namespace which has multiple services
	cmd := startKubefwd(t, "svc", "-n", "kf-a", "-v")
	defer stopKubefwd(t, cmd)

	// Wait for forwarding to stabilize
	t.Log("Waiting for forwarding to stabilize...")
	time.Sleep(10 * time.Second)

	// Services to verify from kf-a namespace
	services := []string{"ok", "ok-headless", "ok-ss", "ok-ss-headless"}

	// Verify all services are in hosts file
	for _, svc := range services {
		if !verifyHostsEntry(t, svc) {
			t.Errorf("Service '%s' not found in /etc/hosts", svc)
		} else {
			t.Logf("✓ Service '%s' found in /etc/hosts", svc)
		}
	}

	// Test HTTP connectivity to non-headless services
	testServices := []struct {
		name string
		port int
	}{
		{"ok", 8080},
		{"ok-ss", 8080},
	}

	for _, ts := range testServices {
		t.Logf("Testing HTTP connectivity to %s:%d...", ts.name, ts.port)
		resp, err := httpGet(fmt.Sprintf("http://%s:%d/", ts.name, ts.port), 5, 2*time.Second)
		if err != nil {
			t.Errorf("Failed to connect to %s: %v", ts.name, err)
			continue
		}
		resp.Body.Close()

		if resp.StatusCode != 200 {
			t.Errorf("Expected HTTP 200 from %s, got %d", ts.name, resp.StatusCode)
		} else {
			t.Logf("✓ Successfully connected to %s (HTTP 200)", ts.name)
		}
	}
}

// TestForwardWithCustomDomain tests forwarding with a custom domain suffix
//
//goland:noinspection HttpUrlsUsage
func TestForwardWithCustomDomain(t *testing.T) {
	requiresSudo(t)
	requiresKindCluster(t)

	// Start kubefwd with custom domain
	customDomain := "local.test"
	cmd := startKubefwd(t, "svc", "-n", "test-simple", "-d", customDomain, "-v")
	defer stopKubefwd(t, cmd)

	// Wait for forwarding to stabilize
	t.Log("Waiting for forwarding to stabilize...")
	time.Sleep(5 * time.Second)

	// Verify hosts file entry with custom domain
	expectedHostname := "nginx." + customDomain
	if !verifyHostsEntry(t, expectedHostname) {
		t.Errorf("Service '%s' not found in /etc/hosts", expectedHostname)
	} else {
		t.Logf("✓ Service '%s' found in /etc/hosts", expectedHostname)
	}

	// Test HTTP connectivity with custom domain
	t.Logf("Testing HTTP connectivity to %s...", expectedHostname)
	resp, err := httpGet(fmt.Sprintf("http://%s:80/", expectedHostname), 5, 2*time.Second)
	if err != nil {
		t.Fatalf("Failed to connect to forwarded service with custom domain: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("Expected HTTP 200, got %d", resp.StatusCode)
	} else {
		t.Log("✓ Successfully connected with custom domain (HTTP 200)")
	}
}

// TestForwardMultipleNamespaces tests forwarding services from multiple namespaces
//
//goland:noinspection DuplicatedCode,HttpUrlsUsage
func TestForwardMultipleNamespaces(t *testing.T) {
	requiresSudo(t)
	requiresKindCluster(t)

	// Start kubefwd for multiple namespaces
	cmd := startKubefwd(t, "svc", "-n", "kf-a,kf-b", "-v")
	defer stopKubefwd(t, cmd)

	// Wait for forwarding to stabilize
	t.Log("Waiting for forwarding to stabilize...")
	time.Sleep(15 * time.Second)

	// Verify services from both namespaces are in hosts file
	// Format should be: service.namespace
	expectedHosts := []string{
		"ok.kf-a",
		"ok.kf-b",
		"ok-ss.kf-a",
		"ok-ss.kf-b",
	}

	for _, host := range expectedHosts {
		if !verifyHostsEntry(t, host) {
			t.Errorf("Service '%s' not found in /etc/hosts", host)
		} else {
			t.Logf("✓ Service '%s' found in /etc/hosts", host)
		}
	}

	// Verify that services from different namespaces get different IPs
	ipKfA := getHostsEntryIP(t, "ok.kf-a")
	ipKfB := getHostsEntryIP(t, "ok.kf-b")

	if ipKfA == "" || ipKfB == "" {
		t.Error("Failed to get IP addresses for services")
	} else if ipKfA == ipKfB {
		t.Errorf("Services from different namespaces have same IP: %s", ipKfA)
	} else {
		t.Logf("✓ Services from different namespaces have different IPs: %s vs %s", ipKfA, ipKfB)
	}

	// Test HTTP connectivity to services from both namespaces
	testHosts := []struct {
		name string
		port int
	}{
		{"ok.kf-a", 8080},
		{"ok.kf-b", 8080},
	}

	for _, th := range testHosts {
		t.Logf("Testing HTTP connectivity to %s:%d...", th.name, th.port)
		resp, err := httpGet(fmt.Sprintf("http://%s:%d/", th.name, th.port), 5, 2*time.Second)
		if err != nil {
			t.Errorf("Failed to connect to %s: %v", th.name, err)
			continue
		}
		resp.Body.Close()

		if resp.StatusCode != 200 {
			t.Errorf("Expected HTTP 200 from %s, got %d", th.name, resp.StatusCode)
		} else {
			t.Logf("✓ Successfully connected to %s (HTTP 200)", th.name)
		}
	}
}

// TestServiceAccessibility tests that forwarded services are actually accessible via HTTP
//
//goland:noinspection HttpUrlsUsage
func TestServiceAccessibility(t *testing.T) {
	requiresSudo(t)
	requiresKindCluster(t)

	// Start kubefwd for kf-a namespace
	cmd := startKubefwd(t, "svc", "-n", "kf-a", "-v")
	defer stopKubefwd(t, cmd)

	// Wait for forwarding to stabilize
	t.Log("Waiting for forwarding to stabilize...")
	time.Sleep(10 * time.Second)

	// Test multiple HTTP requests to verify consistent accessibility
	serviceName := "ok"
	servicePort := 8080
	requestCount := 5

	t.Logf("Testing %d HTTP requests to %s:%d...", requestCount, serviceName, servicePort)

	successCount := 0
	for i := 0; i < requestCount; i++ {
		resp, err := httpGet(fmt.Sprintf("http://%s:%d/", serviceName, servicePort), 3, 1*time.Second)
		if err != nil {
			t.Logf("Request %d failed: %v", i+1, err)
			continue
		}
		resp.Body.Close()

		if resp.StatusCode == 200 {
			successCount++
			t.Logf("Request %d: HTTP %d ✓", i+1, resp.StatusCode)
		} else {
			t.Logf("Request %d: HTTP %d (expected 200)", i+1, resp.StatusCode)
		}

		// Small delay between requests
		time.Sleep(500 * time.Millisecond)
	}

	// Require at least 80% success rate
	successRate := float64(successCount) / float64(requestCount)
	t.Logf("Success rate: %.1f%% (%d/%d)", successRate*100, successCount, requestCount)

	if successRate < 0.8 {
		t.Errorf("Service accessibility below threshold: %.1f%% (expected >= 80%%)", successRate*100)
	} else {
		t.Logf("✓ Service accessibility acceptable: %.1f%%", successRate*100)
	}
}
