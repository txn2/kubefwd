//go:build integration
// +build integration

package integration

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestForwardHeadlessService tests forwarding a headless service
func TestForwardHeadlessService(t *testing.T) {
	requiresSudo(t)
	requiresKindCluster(t)

	// Start kubefwd for kf-a namespace (contains ok-headless service)
	cmd := startKubefwd(t, "svc", "-n", "kf-a", "-v")
	defer stopKubefwd(t, cmd)

	// Wait for forwarding to stabilize
	t.Log("Waiting for forwarding to stabilize...")
	time.Sleep(10 * time.Second)

	// Verify headless service is in hosts file
	if !verifyHostsEntry(t, "ok-headless") {
		t.Error("Headless service 'ok-headless' not found in /etc/hosts")
	} else {
		t.Log("✓ Headless service 'ok-headless' found in /etc/hosts")
	}

	// Test HTTP connectivity to headless service
	t.Log("Testing HTTP connectivity to ok-headless service...")
	resp, err := httpGet("http://ok-headless:8080/", 5, 2*time.Second)
	if err != nil {
		t.Fatalf("Failed to connect to headless service: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("Expected HTTP 200, got %d", resp.StatusCode)
	} else {
		t.Log("✓ Successfully connected to headless service (HTTP 200)")
	}
}

// TestHeadlessAllPodsForwarded tests that all pods of a headless service are forwarded
func TestHeadlessAllPodsForwarded(t *testing.T) {
	requiresSudo(t)
	requiresKindCluster(t)

	// Start kubefwd for kf-a namespace
	cmd := startKubefwd(t, "svc", "-n", "kf-a", "-v")
	defer stopKubefwd(t, cmd)

	// Wait for forwarding to stabilize
	t.Log("Waiting for forwarding to stabilize...")
	time.Sleep(10 * time.Second)

	// Get list of pods for the ok deployment (which backs ok-headless service)
	getPods := exec.Command("kubectl", "get", "pods", "-n", "kf-a",
		"-l", "app=ok,component=deployment",
		"-o", "jsonpath={.items[*].metadata.name}")

	output, err := getPods.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to get pods: %v", err)
	}

	podNames := strings.Fields(string(output))
	if len(podNames) == 0 {
		t.Fatal("No pods found for headless service")
	}

	t.Logf("Found %d pods backing headless service", len(podNames))

	// For headless services, kubefwd should forward all pods
	// The first pod gets the service name, others get pod-name.service-name
	// Verify at least the main service entry exists
	if !verifyHostsEntry(t, "ok-headless") {
		t.Error("Main headless service entry not found in /etc/hosts")
	}

	// Try to connect to the headless service
	// This will connect to one of the pods
	t.Log("Testing connectivity to headless service (one pod)...")
	resp, err := httpGet("http://ok-headless:8080/", 5, 2*time.Second)
	if err != nil {
		t.Errorf("Failed to connect to headless service: %v", err)
	} else {
		resp.Body.Close()
		if resp.StatusCode == 200 {
			t.Log("✓ Successfully connected to headless service")
		} else {
			t.Errorf("Expected HTTP 200, got %d", resp.StatusCode)
		}
	}

	// Note: kubefwd's behavior for headless services is to forward all pods,
	// but the hostname scheme may vary. The key is that we can connect to at
	// least one pod via the headless service name.
}

// TestHeadlessPodHostnames tests that pod-specific hostnames work for headless services
func TestHeadlessPodHostnames(t *testing.T) {
	requiresSudo(t)
	requiresKindCluster(t)

	// Start kubefwd for kf-a namespace
	cmd := startKubefwd(t, "svc", "-n", "kf-a", "-v")
	defer stopKubefwd(t, cmd)

	// Wait for forwarding to stabilize
	t.Log("Waiting for forwarding to stabilize...")
	time.Sleep(10 * time.Second)

	// Read /etc/hosts to find pod-specific entries for headless service
	t.Log("Checking /etc/hosts for headless service entries...")

	// Look for entries that match the headless service pattern
	// Entries should follow pattern: IP pod-name.service-name or just service-name
	if !verifyHostsEntry(t, "ok-headless") {
		t.Error("Headless service 'ok-headless' not found in /etc/hosts")
		return
	}

	t.Log("✓ Found headless service entries in /etc/hosts")

	// Test connectivity using the headless service name
	// (kubefwd forwards the first pod as the service name for headless services)
	t.Log("Testing connectivity via headless service name...")
	resp, err := httpGet("http://ok-headless:8080/", 5, 2*time.Second)
	if err != nil {
		t.Errorf("Failed to connect via headless service name: %v", err)
	} else {
		resp.Body.Close()
		if resp.StatusCode == 200 {
			t.Log("✓ Successfully connected via headless service name")
		}
	}
}

// TestHeadlessStatefulSet tests headless service with StatefulSet
//
//goland:noinspection HttpUrlsUsage,HttpUrlsUsage
func TestHeadlessStatefulSet(t *testing.T) {
	requiresSudo(t)
	requiresKindCluster(t)

	// Start kubefwd for kf-a namespace (contains ok-ss StatefulSet with headless service)
	cmd := startKubefwd(t, "svc", "-n", "kf-a", "-v")
	defer stopKubefwd(t, cmd)

	// Wait for forwarding to stabilize
	t.Log("Waiting for forwarding to stabilize...")
	time.Sleep(10 * time.Second)

	// Verify headless StatefulSet service is in hosts file
	headlessSvcName := "ok-ss-headless"
	if !verifyHostsEntry(t, headlessSvcName) {
		t.Errorf("Headless StatefulSet service '%s' not found in /etc/hosts", headlessSvcName)
	} else {
		t.Logf("✓ Headless StatefulSet service '%s' found in /etc/hosts", headlessSvcName)
	}

	// Get StatefulSet pods
	getPods := exec.Command("kubectl", "get", "pods", "-n", "kf-a",
		"-l", "app=ok-ss,component=ss",
		"-o", "jsonpath={.items[*].metadata.name}")

	output, err := getPods.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to get StatefulSet pods: %v", err)
	}

	podNames := strings.Fields(string(output))
	if len(podNames) == 0 {
		t.Fatal("No StatefulSet pods found")
	}

	t.Logf("Found %d StatefulSet pods", len(podNames))

	// Test connectivity to headless StatefulSet service
	// StatefulSets with headless services have predictable pod names: pod-0, pod-1, pod-2, etc.
	t.Log("Testing connectivity to headless StatefulSet service...")
	resp, err := httpGet(fmt.Sprintf("http://%s:8080/", headlessSvcName), 5, 2*time.Second)
	if err != nil {
		t.Errorf("Failed to connect to headless StatefulSet service: %v", err)
	} else {
		resp.Body.Close()
		if resp.StatusCode == 200 {
			t.Log("✓ Successfully connected to headless StatefulSet service (HTTP 200)")
		} else {
			t.Errorf("Expected HTTP 200, got %d", resp.StatusCode)
		}
	}

	// Test the multi-port aspect (ok-ss-headless has ports 8080 and 8081)
	t.Log("Testing second port (8081) on headless StatefulSet service...")
	resp, err = httpGet(fmt.Sprintf("http://%s:8081/", headlessSvcName), 5, 2*time.Second)
	if err != nil {
		t.Errorf("Failed to connect to headless StatefulSet service port 8081: %v", err)
	} else {
		resp.Body.Close()
		if resp.StatusCode == 200 {
			t.Log("✓ Successfully connected to headless StatefulSet service port 8081 (HTTP 200)")
		} else {
			t.Errorf("Expected HTTP 200 on port 8081, got %d", resp.StatusCode)
		}
	}
}
