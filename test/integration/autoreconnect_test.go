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

// TestPodDeletionReconnect tests that kubefwd reconnects when a pod is deleted
func TestPodDeletionReconnect(t *testing.T) {
	requiresSudo(t)
	requiresKindCluster(t)

	// Start kubefwd for test-simple namespace
	cmd := startKubefwd(t, "svc", "-n", "test-simple", "-v")
	defer stopKubefwd(t, cmd)

	// Wait for initial forwarding
	t.Log("Waiting for initial forwarding to stabilize...")
	time.Sleep(10 * time.Second)

	// Verify initial connectivity
	t.Log("Testing initial connectivity...")
	resp, err := httpGet("http://nginx:80/", 5, 2*time.Second)
	if err != nil {
		t.Fatalf("Failed initial connection: %v", err)
	}
	resp.Body.Close()
	t.Log("✓ Initial connection successful")

	// Get current pod name
	getPod := exec.Command("kubectl", "get", "pods", "-n", "test-simple",
		"-l", "app=nginx",
		"-o", "jsonpath={.items[0].metadata.name}")

	output, err := getPod.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to get pod name: %v", err)
	}

	podName := strings.TrimSpace(string(output))
	if podName == "" {
		t.Fatal("No pod found")
	}

	t.Logf("Current pod: %s", podName)

	// Delete the pod
	t.Logf("Deleting pod %s...", podName)
	if err := deletePod(t, "test-simple", podName); err != nil {
		t.Fatalf("Failed to delete pod: %v", err)
	}

	// Wait for new pod to be created and become ready
	t.Log("Waiting for new pod to be ready...")
	time.Sleep(5 * time.Second)

	if err := waitForPod(t, "test-simple", "app=nginx", 60*time.Second); err != nil {
		t.Fatalf("New pod did not become ready: %v", err)
	}

	// Give kubefwd time to detect pod change and reconnect
	t.Log("Waiting for kubefwd to reconnect to new pod...")
	time.Sleep(15 * time.Second)

	// Test connectivity again - this should work if auto-reconnect works
	t.Log("Testing connectivity after pod deletion...")
	resp, err = httpGet("http://nginx:80/", 10, 3*time.Second)
	if err != nil {
		t.Errorf("Failed to reconnect after pod deletion: %v", err)
		t.Log("Note: Auto-reconnect may require manual intervention in current kubefwd version")
	} else {
		resp.Body.Close()
		if resp.StatusCode == 200 {
			t.Log("✓ Successfully reconnected after pod deletion (HTTP 200)")
		} else {
			t.Errorf("Reconnected but got HTTP %d (expected 200)", resp.StatusCode)
		}
	}
}

// TestDeploymentScale tests pod selection when scaling deployment
func TestDeploymentScale(t *testing.T) {
	requiresSudo(t)
	requiresKindCluster(t)

	// Start kubefwd for test-simple namespace
	cmd := startKubefwd(t, "svc", "-n", "test-simple", "-v")
	defer stopKubefwd(t, cmd)

	// Wait for initial forwarding
	t.Log("Waiting for initial forwarding to stabilize...")
	time.Sleep(10 * time.Second)

	// Test initial connectivity
	t.Log("Testing initial connectivity...")
	resp, err := httpGet("http://nginx:80/", 5, 2*time.Second)
	if err != nil {
		t.Fatalf("Failed initial connection: %v", err)
	}
	resp.Body.Close()
	t.Log("✓ Initial connection successful")

	// Scale deployment down to 1 replica
	t.Log("Scaling deployment down to 1 replica...")
	if err := scaleDeployment(t, "test-simple", "nginx", 1); err != nil {
		t.Fatalf("Failed to scale deployment: %v", err)
	}

	// Wait for scale down
	time.Sleep(10 * time.Second)

	// Test connectivity after scale down
	t.Log("Testing connectivity after scale down...")
	resp, err = httpGet("http://nginx:80/", 5, 2*time.Second)
	if err != nil {
		t.Errorf("Connection failed after scale down: %v", err)
	} else {
		resp.Body.Close()
		t.Log("✓ Connection successful after scale down")
	}

	// Scale deployment back up to 3 replicas
	t.Log("Scaling deployment up to 3 replicas...")
	if err := scaleDeployment(t, "test-simple", "nginx", 3); err != nil {
		t.Fatalf("Failed to scale deployment: %v", err)
	}

	// Wait for scale up
	time.Sleep(10 * time.Second)

	// Wait for all pods to be ready
	if err := waitForPod(t, "test-simple", "app=nginx", 60*time.Second); err != nil {
		t.Logf("Warning: Not all pods became ready: %v", err)
	}

	// Test connectivity after scale up
	t.Log("Testing connectivity after scale up...")
	resp, err = httpGet("http://nginx:80/", 5, 2*time.Second)
	if err != nil {
		t.Errorf("Connection failed after scale up: %v", err)
	} else {
		resp.Body.Close()
		t.Log("✓ Connection successful after scale up")
	}

	// Restore original replica count (2)
	t.Log("Restoring original replica count (2)...")
	scaleDeployment(t, "test-simple", "nginx", 2)
}

// TestRollingUpdate tests forwarding during a rolling update
func TestRollingUpdate(t *testing.T) {
	requiresSudo(t)
	requiresKindCluster(t)

	// Start kubefwd for kf-a namespace (ok deployment has 5 replicas)
	cmd := startKubefwd(t, "svc", "-n", "kf-a", "-v")
	defer stopKubefwd(t, cmd)

	// Wait for initial forwarding
	t.Log("Waiting for initial forwarding to stabilize...")
	time.Sleep(10 * time.Second)

	// Test initial connectivity
	t.Log("Testing initial connectivity...")
	resp, err := httpGet("http://ok:8080/", 5, 2*time.Second)
	if err != nil {
		t.Fatalf("Failed initial connection: %v", err)
	}
	resp.Body.Close()
	t.Log("✓ Initial connection successful")

	// Trigger a rolling update by changing an environment variable
	t.Log("Triggering rolling update...")
	updateCmd := exec.Command("kubectl", "set", "env", "deployment/ok",
		fmt.Sprintf("MESSAGE=updated-%d", time.Now().Unix()),
		"-n", "kf-a")

	if output, err := updateCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to trigger rolling update: %v, output: %s", err, output)
	}

	t.Log("Rolling update triggered, monitoring connectivity...")

	// Monitor connectivity during rolling update
	// Make requests every 2 seconds for 60 seconds
	successCount := 0
	totalRequests := 30
	requestInterval := 2 * time.Second

	for i := 0; i < totalRequests; i++ {
		resp, err := httpGet("http://ok:8080/", 3, 500*time.Millisecond)
		if err != nil {
			t.Logf("Request %d/%d failed: %v", i+1, totalRequests, err)
		} else {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				successCount++
			}
		}

		time.Sleep(requestInterval)
	}

	// Calculate success rate
	successRate := float64(successCount) / float64(totalRequests)
	t.Logf("Success rate during rolling update: %.1f%% (%d/%d)",
		successRate*100, successCount, totalRequests)

	// We expect some disruption during rolling update, but most requests should succeed
	// Require at least 70% success rate (allowing for some pod restart time)
	if successRate < 0.7 {
		t.Errorf("Success rate during rolling update too low: %.1f%% (expected >= 70%%)",
			successRate*100)
	} else {
		t.Logf("✓ Acceptable connectivity during rolling update: %.1f%%", successRate*100)
	}

	// Wait for rolling update to complete
	t.Log("Waiting for rolling update to complete...")
	time.Sleep(10 * time.Second)

	// Test final connectivity
	t.Log("Testing connectivity after rolling update...")
	resp, err = httpGet("http://ok:8080/", 5, 2*time.Second)
	if err != nil {
		t.Errorf("Failed to connect after rolling update: %v", err)
	} else {
		resp.Body.Close()
		if resp.StatusCode == 200 {
			t.Log("✓ Connection successful after rolling update")
		}
	}
}

// TestPodRestartReconnect tests reconnection when a pod restarts (crashes)
func TestPodRestartReconnect(t *testing.T) {
	requiresSudo(t)
	requiresKindCluster(t)

	// Start kubefwd for test-simple namespace
	cmd := startKubefwd(t, "svc", "-n", "test-simple", "-v")
	defer stopKubefwd(t, cmd)

	// Wait for initial forwarding
	t.Log("Waiting for initial forwarding to stabilize...")
	time.Sleep(10 * time.Second)

	// Test initial connectivity
	t.Log("Testing initial connectivity...")
	resp, err := httpGet("http://nginx:80/", 5, 2*time.Second)
	if err != nil {
		t.Fatalf("Failed initial connection: %v", err)
	}
	resp.Body.Close()
	t.Log("✓ Initial connection successful")

	// Get current pod name and restart count
	getPod := exec.Command("kubectl", "get", "pods", "-n", "test-simple",
		"-l", "app=nginx",
		"-o", "jsonpath={.items[0].metadata.name}")

	output, err := getPod.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to get pod name: %v", err)
	}

	podName := strings.TrimSpace(string(output))
	t.Logf("Current pod: %s", podName)

	// Force pod restart by deleting the container (kubectl delete pod)
	// This simulates a pod crash and restart
	t.Logf("Forcing pod %s to restart...", podName)
	deleteCmd := exec.Command("kubectl", "delete", "pod", podName, "-n", "test-simple", "--wait=false")
	if output, err := deleteCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to restart pod: %v, output: %s", err, output)
	}

	// Wait for pod to be recreated
	t.Log("Waiting for pod to be recreated...")
	time.Sleep(5 * time.Second)

	// Wait for new pod to be ready
	if err := waitForPod(t, "test-simple", "app=nginx", 60*time.Second); err != nil {
		t.Fatalf("Pod did not become ready after restart: %v", err)
	}

	// Give kubefwd time to detect the restart and reconnect
	t.Log("Waiting for kubefwd to reconnect...")
	time.Sleep(15 * time.Second)

	// Test connectivity after restart
	t.Log("Testing connectivity after pod restart...")
	resp, err = httpGet("http://nginx:80/", 10, 3*time.Second)
	if err != nil {
		t.Errorf("Failed to reconnect after pod restart: %v", err)
		t.Log("Note: Auto-reconnect may require manual intervention in current kubefwd version")
	} else {
		resp.Body.Close()
		if resp.StatusCode == 200 {
			t.Log("✓ Successfully reconnected after pod restart (HTTP 200)")
		} else {
			t.Errorf("Reconnected but got HTTP %d (expected 200)", resp.StatusCode)
		}
	}
}
