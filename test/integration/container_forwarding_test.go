//go:build integration
// +build integration

package integration

import (
	"testing"
)

// TestContainerForwardSingleService tests forwarding using testcontainers
// This test runs kubefwd inside a privileged container, eliminating the need for sudo on the host
func TestContainerForwardSingleService(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping container-based test in short mode")
	}

	kindClusterName := "kubefwd-test"
	RequiresKindCluster(t, kindClusterName)

	// Create test runner (starts privileged container)
	runner, err := NewContainerTestRunner(t, kindClusterName)
	if err != nil {
		t.Fatalf("Failed to create container test runner: %v", err)
	}
	defer runner.Terminate()

	// Run the actual test inside the container
	// The test runs as root in the container, no sudo needed on host!
	if err := runner.RunTest("TestForwardSingleService"); err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

// TestContainerForwardMultipleServices tests multiple service forwarding in container
func TestContainerForwardMultipleServices(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping container-based test in short mode")
	}

	kindClusterName := "kubefwd-test"
	RequiresKindCluster(t, kindClusterName)

	runner, err := NewContainerTestRunner(t, kindClusterName)
	if err != nil {
		t.Fatalf("Failed to create container test runner: %v", err)
	}
	defer runner.Terminate()

	if err := runner.RunTest("TestForwardMultipleServices"); err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

// TestContainerHeadlessService tests headless service forwarding in container
func TestContainerHeadlessService(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping container-based test in short mode")
	}

	kindClusterName := "kubefwd-test"
	RequiresKindCluster(t, kindClusterName)

	runner, err := NewContainerTestRunner(t, kindClusterName)
	if err != nil {
		t.Fatalf("Failed to create container test runner: %v", err)
	}
	defer runner.Terminate()

	if err := runner.RunTest("TestForwardHeadlessService"); err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

// TestContainerCleanup tests cleanup verification in container
func TestContainerCleanup(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping container-based test in short mode")
	}

	kindClusterName := "kubefwd-test"
	RequiresKindCluster(t, kindClusterName)

	runner, err := NewContainerTestRunner(t, kindClusterName)
	if err != nil {
		t.Fatalf("Failed to create container test runner: %v", err)
	}
	defer runner.Terminate()

	if err := runner.RunTest("TestGracefulShutdown"); err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

// TestContainerAllForwardingTests runs the entire forwarding test suite in container
func TestContainerAllForwardingTests(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping container-based test in short mode")
	}

	kindClusterName := "kubefwd-test"
	RequiresKindCluster(t, kindClusterName)

	runner, err := NewContainerTestRunner(t, kindClusterName)
	if err != nil {
		t.Fatalf("Failed to create container test runner: %v", err)
	}
	defer runner.Terminate()

	// Run all forwarding tests
	if err := runner.RunTestSuite("TestForward"); err != nil {
		t.Fatalf("Forwarding test suite failed: %v", err)
	}
}
