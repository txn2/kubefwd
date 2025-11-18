//go:build integration
// +build integration

package integration

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// requiresSudo skips test if not running as root
func requiresSudo(t *testing.T) {
	t.Helper()
	if os.Getuid() != 0 {
		t.Skip("Test requires root/sudo privileges - run with: sudo -E go test -tags=integration")
	}
}

// requiresKindCluster skips test if KIND cluster not available
func requiresKindCluster(t *testing.T) {
	t.Helper()
	cmd := exec.Command("kubectl", "cluster-info")
	if err := cmd.Run(); err != nil {
		t.Skip("KIND cluster not available - run: ./test/scripts/setup-kind.sh")
	}
}

// startKubefwd starts kubefwd with given arguments and returns the command
// The caller is responsible for calling stopKubefwd to clean up
func startKubefwd(t *testing.T, args ...string) *exec.Cmd {
	t.Helper()

	kubefwdPath := os.Getenv("KUBEFWD_BIN")
	if kubefwdPath == "" {
		// Default to the built binary in the project root
		kubefwdPath = "../../kubefwd"
	}

	t.Logf("Starting kubefwd: %s %v", kubefwdPath, args)

	cmd := exec.Command(kubefwdPath, args...)
	cmd.Env = os.Environ()

	// Capture stdout and stderr for debugging
	cmd.Stdout = &testWriter{t: t, prefix: "[kubefwd out] "}
	cmd.Stderr = &testWriter{t: t, prefix: "[kubefwd err] "}

	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start kubefwd: %v", err)
	}

	// Give kubefwd time to start forwarding and update /etc/hosts
	// This is a generous delay to account for pod discovery and forwarding setup
	t.Log("Waiting for kubefwd to initialize...")
	time.Sleep(10 * time.Second)

	return cmd
}

// stopKubefwd gracefully stops kubefwd process
func stopKubefwd(t *testing.T, cmd *exec.Cmd) {
	t.Helper()

	if cmd == nil || cmd.Process == nil {
		t.Log("No kubefwd process to stop")
		return
	}

	t.Log("Stopping kubefwd...")

	// Send SIGINT (Ctrl+C) for graceful shutdown
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Logf("Failed to send interrupt signal: %v", err)
		// Try SIGTERM
		if err := cmd.Process.Signal(os.Kill); err != nil {
			t.Logf("Failed to send kill signal: %v", err)
		}
	}

	// Wait for shutdown with timeout
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Logf("kubefwd exited with error: %v", err)
		} else {
			t.Log("kubefwd stopped gracefully")
		}
	case <-time.After(15 * time.Second):
		t.Log("kubefwd shutdown timed out, forcefully killing process")
		cmd.Process.Kill()
		<-done // Wait for process to actually exit
	}

	// Give the system time to clean up /etc/hosts and network aliases
	time.Sleep(2 * time.Second)
}

// httpGet attempts HTTP GET with retries and exponential backoff
func httpGet(url string, retries int, initialDelay time.Duration) (*http.Response, error) {
	var resp *http.Response
	var err error

	delay := initialDelay
	for i := 0; i < retries; i++ {
		resp, err = http.Get(url)
		if err == nil {
			return resp, nil
		}

		if i < retries-1 {
			time.Sleep(delay)
			// Exponential backoff up to 10 seconds
			delay *= 2
			if delay > 10*time.Second {
				delay = 10 * time.Second
			}
		}
	}

	return nil, fmt.Errorf("failed after %d retries: %w", retries, err)
}

// httpGetWithContext attempts HTTP GET with context and retries
func httpGetWithContext(ctx context.Context, url string, retries int, delay time.Duration) (*http.Response, error) {
	var resp *http.Response
	var err error

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	for i := 0; i < retries; i++ {
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		resp, err = client.Do(req)
		if err == nil {
			return resp, nil
		}

		if i < retries-1 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
				// Continue to next retry
			}
		}
	}

	return nil, fmt.Errorf("failed after %d retries: %w", retries, err)
}

// verifyHostsEntry checks if hostname is in /etc/hosts
func verifyHostsEntry(t *testing.T, hostname string) bool {
	t.Helper()

	data, err := os.ReadFile("/etc/hosts")
	if err != nil {
		t.Fatalf("Failed to read /etc/hosts: %v", err)
	}

	// Check if hostname appears in /etc/hosts
	// It should be followed by whitespace or be at end of line
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		// Skip comments
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}

		// Split line into fields (IP and hostnames)
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		// Check if any hostname matches
		for _, field := range fields[1:] {
			if field == hostname {
				return true
			}
		}
	}

	return false
}

// getHostsEntryIP returns the IP address for a hostname in /etc/hosts
func getHostsEntryIP(t *testing.T, hostname string) string {
	t.Helper()

	data, err := os.ReadFile("/etc/hosts")
	if err != nil {
		t.Fatalf("Failed to read /etc/hosts: %v", err)
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		// Skip comments
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		// Check if any hostname matches
		for _, field := range fields[1:] {
			if field == hostname {
				return fields[0] // Return IP address
			}
		}
	}

	return ""
}

// waitForPod waits for a pod with the given label selector to be ready
func waitForPod(t *testing.T, namespace, labelSelector string, timeout time.Duration) error {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "kubectl", "wait",
		"--for=condition=ready",
		"--timeout="+timeout.String(),
		"pod",
		"-l", labelSelector,
		"-n", namespace,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to wait for pod: %v, output: %s", err, output)
	}

	return nil
}

// deletePod deletes a pod by name in the given namespace
func deletePod(t *testing.T, namespace, podName string) error {
	t.Helper()

	cmd := exec.Command("kubectl", "delete", "pod", podName, "-n", namespace, "--wait=false")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete pod: %v, output: %s", err, output)
	}

	t.Logf("Deleted pod %s in namespace %s", podName, namespace)
	return nil
}

// scaleDeploy scales a deployment to the specified number of replicas
func scaleDeployment(t *testing.T, namespace, deploymentName string, replicas int) error {
	t.Helper()

	cmd := exec.Command("kubectl", "scale", "deployment", deploymentName,
		fmt.Sprintf("--replicas=%d", replicas),
		"-n", namespace,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to scale deployment: %v, output: %s", err, output)
	}

	t.Logf("Scaled deployment %s to %d replicas in namespace %s", deploymentName, replicas, namespace)
	return nil
}

// testWriter is a helper that writes to the test log
type testWriter struct {
	t      *testing.T
	prefix string
}

func (tw *testWriter) Write(p []byte) (n int, err error) {
	// Split output into lines and log each one
	lines := strings.Split(strings.TrimSuffix(string(p), "\n"), "\n")
	for _, line := range lines {
		if line != "" {
			tw.t.Log(tw.prefix + line)
		}
	}
	return len(p), nil
}
