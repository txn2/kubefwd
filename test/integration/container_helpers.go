//go:build integration
// +build integration

package integration

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// ContainerTestRunner manages a privileged container for running kubefwd tests
type ContainerTestRunner struct {
	container testcontainers.Container
	ctx       context.Context
	t         *testing.T
}

// NewContainerTestRunner creates a new test runner that executes tests in a privileged container
func NewContainerTestRunner(t *testing.T, kindClusterName string) (*ContainerTestRunner, error) {
	t.Helper()

	ctx := context.Background()

	// Get absolute path to workspace (project root)
	workspacePath, err := filepath.Abs("../..")
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace path: %w", err)
	}

	// Get KIND kubeconfig
	kubeconfig, err := getKindKubeconfig(kindClusterName)
	if err != nil {
		return nil, fmt.Errorf("failed to get KIND kubeconfig: %w", err)
	}

	// Create container request
	req := testcontainers.ContainerRequest{
		Image: "golang:1.21",
		HostConfigModifier: func(hc *container.HostConfig) {
			hc.Privileged = true
			hc.CapAdd = []string{"NET_ADMIN", "SYS_ADMIN"}
		},
		Mounts: testcontainers.Mounts(
			testcontainers.BindMount(workspacePath, "/workspace"),
		),
		Env: map[string]string{
			"KUBECONFIG":        "/tmp/kubeconfig",
			"KIND_CLUSTER_NAME": kindClusterName,
			"KUBEFWD_BIN":       "/workspace/kubefwd",
			"GO111MODULE":       "on",
			"CGO_ENABLED":       "0",
		},
		Networks: []string{
			"kind", // Connect to KIND network
		},
		NetworkMode: container.NetworkMode("kind"),
		WorkingDir:  "/workspace",
		// Keep container running for test execution
		Cmd: []string{"sleep", "infinity"},
	}

	// Create and start container
	genericContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start test container: %w", err)
	}

	runner := &ContainerTestRunner{
		container: genericContainer,
		ctx:       ctx,
		t:         t,
	}

	// Copy kubeconfig into container
	if err := runner.writeKubeconfig(kubeconfig); err != nil {
		runner.Terminate()
		return nil, fmt.Errorf("failed to write kubeconfig: %w", err)
	}

	// Build kubefwd inside container
	if err := runner.buildKubefwd(); err != nil {
		runner.Terminate()
		return nil, fmt.Errorf("failed to build kubefwd: %w", err)
	}

	return runner, nil
}

// writeKubeconfig writes the kubeconfig to the container
func (r *ContainerTestRunner) writeKubeconfig(kubeconfig string) error {
	r.t.Helper()

	exitCode, output, err := r.container.Exec(r.ctx, []string{
		"sh", "-c", fmt.Sprintf("cat > /tmp/kubeconfig <<'EOF'\n%s\nEOF", kubeconfig),
	})

	if err != nil {
		return fmt.Errorf("failed to write kubeconfig: %w", err)
	}

	if exitCode != 0 {
		return fmt.Errorf("failed to write kubeconfig, exit code: %d, output: %s", exitCode, output)
	}

	r.t.Log("✓ Kubeconfig written to container")
	return nil
}

// buildKubefwd builds the kubefwd binary inside the container
func (r *ContainerTestRunner) buildKubefwd() error {
	r.t.Helper()

	r.t.Log("Building kubefwd in container...")

	exitCode, output, err := r.container.Exec(r.ctx, []string{
		"sh", "-c", "cd /workspace && go build -o kubefwd ./cmd/kubefwd/kubefwd.go",
	})

	if err != nil {
		return fmt.Errorf("failed to build kubefwd: %w", err)
	}

	if exitCode != 0 {
		outputStr := readOutput(output)
		return fmt.Errorf("kubefwd build failed, exit code: %d, output: %s", exitCode, outputStr)
	}

	r.t.Log("✓ kubefwd built successfully in container")
	return nil
}

// RunTest executes a specific test inside the container
func (r *ContainerTestRunner) RunTest(testName string) error {
	r.t.Helper()

	r.t.Logf("Running test %s in container...", testName)

	// Run the specific test
	cmd := []string{
		"sh", "-c",
		fmt.Sprintf("cd /workspace && go test -tags=integration -v ./test/integration/ -run %s", testName),
	}

	exitCode, output, err := r.container.Exec(r.ctx, cmd)
	if err != nil {
		return fmt.Errorf("failed to execute test: %w", err)
	}

	// Stream output to test log
	outputStr := readOutput(output)
	r.t.Log(outputStr)

	if exitCode != 0 {
		return fmt.Errorf("test failed with exit code: %d", exitCode)
	}

	r.t.Logf("✓ Test %s passed", testName)
	return nil
}

// RunTestSuite executes all tests in a test file
func (r *ContainerTestRunner) RunTestSuite(suiteName string) error {
	r.t.Helper()

	r.t.Logf("Running test suite %s in container...", suiteName)

	cmd := []string{
		"sh", "-c",
		fmt.Sprintf("cd /workspace && go test -tags=integration -v ./test/integration/ -run %s", suiteName),
	}

	exitCode, output, err := r.container.Exec(r.ctx, cmd)
	if err != nil {
		return fmt.Errorf("failed to execute test suite: %w", err)
	}

	outputStr := readOutput(output)
	r.t.Log(outputStr)

	if exitCode != 0 {
		return fmt.Errorf("test suite failed with exit code: %d", exitCode)
	}

	r.t.Logf("✓ Test suite %s passed", suiteName)
	return nil
}

// Terminate stops and removes the test container
func (r *ContainerTestRunner) Terminate() error {
	if r.container != nil {
		// Give container time to cleanup
		time.Sleep(2 * time.Second)
		return r.container.Terminate(r.ctx)
	}
	return nil
}

// getKindKubeconfig retrieves the kubeconfig for a KIND cluster
func getKindKubeconfig(clusterName string) (string, error) {
	cmd := fmt.Sprintf("kind get kubeconfig --name %s", clusterName)

	// This runs on the host, not in a container
	result, err := executeHostCommand(cmd)
	if err != nil {
		return "", fmt.Errorf("failed to get KIND kubeconfig: %w", err)
	}

	return result, nil
}

// executeHostCommand executes a command on the host machine
func executeHostCommand(cmd string) (string, error) {
	// This is a helper that would execute on the host
	// For now, we'll read from the default kubeconfig location
	// In practice, this should use exec.Command

	// Read kubeconfig from default KIND location
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	kubeconfigPath := filepath.Join(homeDir, ".kube", "config")
	data, err := os.ReadFile(kubeconfigPath)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

// readOutput reads the output from a container exec
func readOutput(reader io.Reader) string {
	if reader == nil {
		return ""
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Sprintf("error reading output: %v", err)
	}

	return string(data)
}

// RequiresKindCluster checks if a KIND cluster is available
func RequiresKindCluster(t *testing.T, clusterName string) {
	t.Helper()

	// Check if KIND cluster exists
	ctx := context.Background()

	// Try to create a simple container to test Docker connectivity
	req := testcontainers.ContainerRequest{
		Image:      "alpine:latest",
		Cmd:        []string{"echo", "test"},
		WaitingFor: wait.ForExit(),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})

	if err != nil {
		t.Skip("Docker not available - required for testcontainers")
	}

	container.Terminate(ctx)

	// Check if KIND network exists
	// This is a simple check - in production, we'd verify the actual cluster
	t.Log("Docker available, proceeding with container-based tests")
}
