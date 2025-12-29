//go:build live

// Package mcp provides live integration tests for kubefwd MCP functionality.
//
// These tests require:
//   - demo-microservices.yaml deployed to the cluster
//   - kubefwd running with --api flag, forwarding kft1 namespace
//
// Run with:
//
//	go test ./test/mcp/... -v -tags=live
package mcp

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// TestAPIURL is the kubefwd API endpoint for testing.
var TestAPIURL = getAPIURL()

func getAPIURL() string {
	if url := os.Getenv("KUBEFWD_API_URL"); url != "" {
		return url
	}
	return "http://localhost:8080"
}

// === P0: Critical Namespace Lifecycle Tests ===

// TestLive_AddRemoveNamespace_kft2 tests the full namespace lifecycle.
// This test would have caught the headless service cleanup bug.
func TestLive_AddRemoveNamespace_kft2(t *testing.T) {
	client := NewClient(TestAPIURL)

	// Verify API is reachable
	health := client.GetHealth(t)
	if health.Status != "healthy" && health.Status != "degraded" {
		t.Skipf("kubefwd not healthy: %s", health.Status)
	}

	// Step 1: Verify kft2 is NOT currently forwarded
	initialServices := client.ListServices(t, "kft2")
	if len(initialServices) > 0 {
		// Clean up first
		client.RemoveNamespace(t, "kft2", "")
		time.Sleep(2 * time.Second)
	}

	// Step 2: Add kft2 namespace
	t.Log("Adding kft2 namespace...")
	result := client.AddNamespace(t, "kft2", "")
	if !result.Success {
		t.Fatalf("Failed to add kft2 namespace: %s", result.Error)
	}
	t.Logf("Added kft2 namespace, context=%s", result.Context)

	// Step 3: Wait for services to be discovered (30 expected: 15 regular + 15 headless)
	t.Log("Waiting for services to be discovered...")
	services := WaitForServices(t, client, "kft2", 20, 30*time.Second)
	t.Logf("Discovered %d services in kft2", len(services))
	AssertServiceCountAtLeast(t, services, 20)

	// Step 4: Verify HTTP connectivity to a kft2 service
	t.Log("Testing HTTP connectivity to dashboard-ui.kft2...")
	WaitForHTTPReady(t, "http://dashboard-ui.kft2/", 200, 20*time.Second)

	// Step 5: Remove kft2 namespace
	t.Log("Removing kft2 namespace...")
	removeResult := client.RemoveNamespace(t, "kft2", "")
	if !removeResult.Success {
		t.Fatalf("Failed to remove kft2 namespace: %s", removeResult.Error)
	}

	// Step 6: Wait for all kft2 services to be removed
	t.Log("Waiting for kft2 services to be removed...")
	time.Sleep(2 * time.Second)

	// Step 7: CRITICAL - Verify ALL kft2 services removed (including headless pod entries)
	finalServices := client.ListServices(t, "kft2")
	if len(finalServices) > 0 {
		t.Errorf("Expected 0 kft2 services after removal, got %d:", len(finalServices))
		for _, svc := range finalServices {
			t.Errorf("  - Orphaned service: %s (status=%s)", svc.Key, svc.Status)
		}
		t.Fatal("Namespace removal left orphaned services - this is the headless service cleanup bug!")
	}
	t.Log("All kft2 services properly removed")

	// Step 8: Verify kft1 is unaffected
	kft1Services := client.ListServices(t, "kft1")
	AssertServiceCountAtLeast(t, kft1Services, 20)
	t.Logf("kft1 still has %d services (unaffected)", len(kft1Services))
}

// TestLive_HTTPConnectivity tests HTTP connectivity to forwarded services.
func TestLive_HTTPConnectivity(t *testing.T) {
	client := NewClient(TestAPIURL)

	// Test api-gateway (nginx)
	t.Run("api-gateway", func(t *testing.T) {
		AssertHTTPReachableWithRetry(t, "http://api-gateway.kft1/", 200, 3, time.Second)
	})

	// Test frontend-web (nginx)
	t.Run("frontend-web", func(t *testing.T) {
		AssertHTTPReachableWithRetry(t, "http://frontend-web.kft1/", 200, 3, time.Second)
	})

	// Test auth-service (traefik/whoami - should return request details)
	t.Run("auth-service-whoami", func(t *testing.T) {
		AssertHTTPReachableWithRetry(t, "http://auth-service.kft1/", 200, 3, time.Second)
	})

	// Verify hostnames are registered
	hostnames := client.ListHostnames(t)
	AssertHostnameExists(t, hostnames, "api-gateway.kft1")
}

// TestLive_MultiPortService tests services with multiple ports.
func TestLive_MultiPortService(t *testing.T) {
	client := NewClient(TestAPIURL)

	// product-catalog has: HTTP:80, PostgreSQL:5432, Redis:6379
	info := client.GetConnectionInfo(t, "product-catalog", "kft1", "")
	if info == nil {
		t.Skip("product-catalog service not found")
	}

	AssertConnectionInfoValid(t, info)
	t.Logf("product-catalog LocalIP: %s", info.LocalIP)
	t.Logf("product-catalog Hostnames: %v", info.Hostnames)
	t.Logf("product-catalog Ports: %+v", info.Ports)

	// Verify HTTP port accessible
	AssertHTTPReachableWithRetry(t, fmt.Sprintf("http://%s:80/", info.LocalIP), 200, 3, time.Second)

	// Verify PostgreSQL port accessible (TCP connect)
	AssertTCPReachableWithRetry(t, fmt.Sprintf("%s:5432", info.LocalIP), 3, time.Second)

	// Verify Redis port accessible (TCP connect)
	AssertTCPReachableWithRetry(t, fmt.Sprintf("%s:6379", info.LocalIP), 3, time.Second)
}

// TestLive_HeadlessPodDiscovery tests that headless services have pod-specific entries.
func TestLive_HeadlessPodDiscovery(t *testing.T) {
	client := NewClient(TestAPIURL)

	services := client.ListServices(t, "kft1")

	// Look for headless service entries with pod names
	headlessFound := false
	var headlessEntries []string

	for _, svc := range services {
		// Headless entries have pattern: pod-name.service-name-headless
		if strings.Contains(svc.Key, "-headless") {
			headlessEntries = append(headlessEntries, svc.Key)
			// Check if this looks like a pod-specific entry (has . in ServiceName)
			if strings.Contains(svc.ServiceName, ".") {
				headlessFound = true
			}
		}
	}

	t.Logf("Found %d headless-related entries", len(headlessEntries))
	for _, entry := range headlessEntries {
		t.Logf("  - %s", entry)
	}

	if !headlessFound && len(headlessEntries) > 0 {
		t.Log("Note: No pod-specific headless entries found (only service-level entries)")
	}
}

// TestLive_GetConnectionInfo tests the connection info endpoint.
func TestLive_GetConnectionInfo(t *testing.T) {
	client := NewClient(TestAPIURL)

	// Test auth-service connection info
	info := client.GetConnectionInfo(t, "auth-service", "kft1", "")
	if info == nil {
		t.Skip("auth-service not found")
	}

	AssertConnectionInfoValid(t, info)

	t.Logf("auth-service connection info:")
	t.Logf("  Service: %s", info.Service)
	t.Logf("  Namespace: %s", info.Namespace)
	t.Logf("  LocalIP: %s", info.LocalIP)
	t.Logf("  Hostnames: %v", info.Hostnames)
	t.Logf("  Ports: %+v", info.Ports)
	t.Logf("  Status: %s", info.Status)

	if len(info.EnvVars) > 0 {
		t.Log("  EnvVars:")
		for k, v := range info.EnvVars {
			t.Logf("    %s=%s", k, v)
		}
	}
}

// TestLive_CrossNamespaceIsolation tests that namespace operations don't affect other namespaces.
func TestLive_CrossNamespaceIsolation(t *testing.T) {
	client := NewClient(TestAPIURL)

	// Count initial kft1 services
	initialKft1 := client.ListServices(t, "kft1")
	initialCount := len(initialKft1)
	t.Logf("Initial kft1 service count: %d", initialCount)

	// Add kft2
	result := client.AddNamespace(t, "kft2", "")
	if !result.Success {
		t.Skipf("Could not add kft2: %s", result.Error)
	}

	// Wait for kft2 services
	WaitForServices(t, client, "kft2", 10, 20*time.Second)

	// Verify kft1 is unchanged
	afterAddKft1 := client.ListServices(t, "kft1")
	if len(afterAddKft1) != initialCount {
		t.Errorf("kft1 service count changed after adding kft2: %d -> %d", initialCount, len(afterAddKft1))
	}

	// Remove kft2
	client.RemoveNamespace(t, "kft2", "")
	time.Sleep(2 * time.Second)

	// Verify kft1 is still unchanged
	afterRemoveKft1 := client.ListServices(t, "kft1")
	if len(afterRemoveKft1) != initialCount {
		t.Errorf("kft1 service count changed after removing kft2: %d -> %d", initialCount, len(afterRemoveKft1))
	}

	t.Logf("kft1 service count remained stable at %d throughout kft2 lifecycle", initialCount)
}

// TestLive_ServiceFind tests the service find/search functionality.
func TestLive_ServiceFind(t *testing.T) {
	client := NewClient(TestAPIURL)

	// Search for services containing "gateway"
	t.Run("query=gateway", func(t *testing.T) {
		services := client.FindServices(t, "gateway", 0, "")
		if len(services) == 0 {
			t.Error("Expected to find services matching 'gateway'")
		}
		for _, svc := range services {
			t.Logf("Found: %s", svc.Key)
			if !strings.Contains(strings.ToLower(svc.ServiceName), "gateway") &&
				!strings.Contains(strings.ToLower(svc.Key), "gateway") {
				t.Errorf("Service %s does not match 'gateway' query", svc.Key)
			}
		}
	})

	// Search for services on port 80
	t.Run("port=80", func(t *testing.T) {
		services := client.FindServices(t, "", 80, "")
		if len(services) == 0 {
			t.Error("Expected to find services on port 80")
		}
		t.Logf("Found %d services on port 80", len(services))
	})

	// Search in specific namespace
	t.Run("namespace=kft1", func(t *testing.T) {
		services := client.FindServices(t, "", 0, "kft1")
		if len(services) == 0 {
			t.Error("Expected to find services in kft1")
		}
		for _, svc := range services {
			if svc.Namespace != "kft1" {
				t.Errorf("Service %s is not in kft1 namespace", svc.Key)
			}
		}
	})
}

// TestLive_Hostnames tests hostname listing and verification.
func TestLive_Hostnames(t *testing.T) {
	client := NewClient(TestAPIURL)

	hostnames := client.ListHostnames(t)
	t.Logf("Found %d hostnames", len(hostnames))

	// Verify some expected hostnames exist
	kft1Hostnames := 0
	for _, h := range hostnames {
		if h.Namespace == "kft1" {
			kft1Hostnames++
		}
	}

	if kft1Hostnames == 0 {
		t.Error("Expected to find hostnames for kft1 namespace")
	}
	t.Logf("Found %d hostnames for kft1", kft1Hostnames)

	// Check specific hostnames
	AssertHostnameExists(t, hostnames, "api-gateway.kft1")
}

// TestLive_HealthAndStatus tests health and status endpoints.
func TestLive_HealthAndStatus(t *testing.T) {
	client := NewClient(TestAPIURL)

	t.Run("health", func(t *testing.T) {
		health := client.GetHealth(t)
		t.Logf("Health: status=%s, version=%s, uptime=%s", health.Status, health.Version, health.Uptime)

		if health.Status != "healthy" && health.Status != "degraded" {
			t.Errorf("Unexpected health status: %s", health.Status)
		}
	})

	t.Run("quickStatus", func(t *testing.T) {
		status := client.GetQuickStatus(t)
		t.Logf("QuickStatus: status=%s, message=%s", status.Status, status.Message)
	})

	t.Run("metrics", func(t *testing.T) {
		metrics := client.GetMetrics(t)
		t.Logf("Metrics: services=%d, forwards=%d, bytesIn=%d, bytesOut=%d",
			metrics.TotalServices, metrics.TotalForwards, metrics.TotalBytesIn, metrics.TotalBytesOut)
	})
}

// TestLive_KubernetesDiscovery tests Kubernetes discovery endpoints.
func TestLive_KubernetesDiscovery(t *testing.T) {
	client := NewClient(TestAPIURL)

	t.Run("contexts", func(t *testing.T) {
		contexts := client.ListContexts(t)
		t.Logf("Current context: %s", contexts.CurrentContext)
		t.Logf("Available contexts: %d", len(contexts.Contexts))
		for _, ctx := range contexts.Contexts {
			t.Logf("  - %s (cluster=%s, active=%v)", ctx.Name, ctx.Cluster, ctx.Active)
		}
	})

	t.Run("namespaces", func(t *testing.T) {
		namespaces := client.ListK8sNamespaces(t, "")
		t.Logf("Found %d namespaces", len(namespaces))

		// Verify kft1 and kft2 exist
		foundKft1, foundKft2 := false, false
		for _, ns := range namespaces {
			if ns.Name == "kft1" {
				foundKft1 = true
			}
			if ns.Name == "kft2" {
				foundKft2 = true
			}
		}
		if !foundKft1 {
			t.Error("Expected to find kft1 namespace")
		}
		if !foundKft2 {
			t.Error("Expected to find kft2 namespace")
		}
	})

	t.Run("services", func(t *testing.T) {
		services := client.ListK8sServices(t, "kft1", "")
		t.Logf("Found %d K8s services in kft1", len(services))

		// Verify some expected services
		foundAPIGateway := false
		for _, svc := range services {
			if svc.Name == "api-gateway" {
				foundAPIGateway = true
				t.Logf("api-gateway: type=%s, clusterIP=%s, forwarded=%v",
					svc.Type, svc.ClusterIP, svc.Forwarded)
			}
		}
		if !foundAPIGateway {
			t.Error("Expected to find api-gateway service in kft1")
		}
	})
}

// TestLive_Logs tests the logs endpoint.
func TestLive_Logs(t *testing.T) {
	client := NewClient(TestAPIURL)

	logs := client.GetLogs(t, 10, "", "")
	t.Logf("Retrieved %d log entries", len(logs))

	for i, log := range logs {
		t.Logf("  [%d] %s %s: %s", i, log.Timestamp.Format("15:04:05"), log.Level, log.Message)
	}
}

// TestLive_ServiceControl tests service control operations.
func TestLive_ServiceControl(t *testing.T) {
	client := NewClient(TestAPIURL)

	// Get a service to test with
	services := client.ListServices(t, "kft1")
	if len(services) == 0 {
		t.Skip("No services to test with")
	}

	testService := services[0]
	t.Logf("Testing with service: %s", testService.Key)

	t.Run("sync", func(t *testing.T) {
		// Sync without force (should use debounce)
		ok := client.SyncService(t, testService.Key, false)
		if !ok {
			t.Error("Sync service failed")
		}
	})

	t.Run("reconnectAllErrors", func(t *testing.T) {
		// This should return 0 if no services are in error state
		triggered := client.ReconnectAllErrors(t)
		t.Logf("Triggered %d reconnections", triggered)
	})
}
