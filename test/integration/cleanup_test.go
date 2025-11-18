//go:build integration
// +build integration

package integration

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestGracefulShutdown tests that kubefwd shuts down cleanly when receiving SIGTERM/SIGINT
func TestGracefulShutdown(t *testing.T) {
	requiresSudo(t)
	requiresKindCluster(t)

	// Read original /etc/hosts before starting kubefwd
	originalHosts, err := os.ReadFile("/etc/hosts")
	if err != nil {
		t.Fatalf("Failed to read /etc/hosts before test: %v", err)
	}

	// Start kubefwd
	cmd := startKubefwd(t, "svc", "-n", "test-simple", "-v")

	// Wait for forwarding to stabilize
	t.Log("Waiting for forwarding to stabilize...")
	time.Sleep(10 * time.Second)

	// Verify service is forwarded
	if !verifyHostsEntry(t, "nginx") {
		t.Error("Service 'nginx' not found in /etc/hosts before shutdown")
	} else {
		t.Log("✓ Service forwarding confirmed before shutdown")
	}

	// Send graceful shutdown signal (SIGINT)
	t.Log("Sending SIGINT for graceful shutdown...")
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatalf("Failed to send interrupt signal: %v", err)
	}

	// Wait for process to exit with timeout
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Logf("kubefwd exited with: %v (may be normal for interrupt)", err)
		} else {
			t.Log("✓ kubefwd exited cleanly")
		}
	case <-time.After(30 * time.Second):
		t.Error("kubefwd did not exit within 30 seconds")
		cmd.Process.Kill()
		<-done
		t.FailNow()
	}

	// Give system time to complete cleanup
	time.Sleep(3 * time.Second)

	// Verify no kubefwd processes remain
	psCmd := exec.Command("pgrep", "-f", "kubefwd")
	if err := psCmd.Run(); err == nil {
		// pgrep succeeded, meaning process was found
		output, _ := exec.Command("pgrep", "-af", "kubefwd").CombinedOutput()
		t.Errorf("kubefwd processes still running after shutdown:\n%s", output)
	} else {
		t.Log("✓ No kubefwd processes remaining")
	}

	// Read /etc/hosts after shutdown
	modifiedHosts, err := os.ReadFile("/etc/hosts")
	if err != nil {
		t.Fatalf("Failed to read /etc/hosts after shutdown: %v", err)
	}

	// Check if /etc/hosts was restored (should be close to original)
	// Allow for some differences due to system changes, but service entries should be gone
	if verifyHostsEntry(t, "nginx") {
		t.Error("/etc/hosts still contains 'nginx' entry after shutdown")
	} else {
		t.Log("✓ Service entries removed from /etc/hosts")
	}

	// Log summary of hosts file changes
	originalLines := len(strings.Split(string(originalHosts), "\n"))
	modifiedLines := len(strings.Split(string(modifiedHosts), "\n"))
	t.Logf("/etc/hosts lines: before=%d, after=%d", originalLines, modifiedLines)

	if abs(originalLines-modifiedLines) > 10 {
		t.Logf("WARNING: /etc/hosts line count differs by %d", abs(originalLines-modifiedLines))
	}

	t.Log("✓ Graceful shutdown test passed")
}

// TestHostsFileRestore tests that /etc/hosts is properly restored after kubefwd exits
func TestHostsFileRestore(t *testing.T) {
	requiresSudo(t)
	requiresKindCluster(t)

	// Read original /etc/hosts
	originalHosts, err := os.ReadFile("/etc/hosts")
	if err != nil {
		t.Fatalf("Failed to read original /etc/hosts: %v", err)
	}

	// Count non-comment lines in original
	originalNonCommentLines := countNonCommentLines(string(originalHosts))
	t.Logf("Original /etc/hosts has %d non-comment lines", originalNonCommentLines)

	// Start kubefwd with multiple services
	cmd := startKubefwd(t, "svc", "-n", "kf-a", "-v")

	// Wait for forwarding
	time.Sleep(10 * time.Second)

	// Read modified /etc/hosts
	modifiedHosts, err := os.ReadFile("/etc/hosts")
	if err != nil {
		t.Fatalf("Failed to read modified /etc/hosts: %v", err)
	}

	modifiedNonCommentLines := countNonCommentLines(string(modifiedHosts))
	t.Logf("Modified /etc/hosts has %d non-comment lines", modifiedNonCommentLines)

	// Should have more lines due to service entries
	if modifiedNonCommentLines <= originalNonCommentLines {
		t.Error("/etc/hosts was not modified (expected service entries to be added)")
	} else {
		added := modifiedNonCommentLines - originalNonCommentLines
		t.Logf("✓ %d lines added to /etc/hosts", added)
	}

	// Stop kubefwd gracefully
	stopKubefwd(t, cmd)

	// Read restored /etc/hosts
	restoredHosts, err := os.ReadFile("/etc/hosts")
	if err != nil {
		t.Fatalf("Failed to read restored /etc/hosts: %v", err)
	}

	restoredNonCommentLines := countNonCommentLines(string(restoredHosts))
	t.Logf("Restored /etc/hosts has %d non-comment lines", restoredNonCommentLines)

	// Should be back to original count (or very close)
	lineDiff := abs(restoredNonCommentLines - originalNonCommentLines)
	if lineDiff > 2 {
		t.Errorf("/etc/hosts not properly restored: diff=%d lines", lineDiff)
		t.Errorf("Original: %d, Restored: %d", originalNonCommentLines, restoredNonCommentLines)
	} else {
		t.Log("✓ /etc/hosts properly restored (within 2 lines of original)")
	}

	// Verify service entries are gone
	serviceNames := []string{"ok", "ok-ss", "ok-headless"}
	entriesRemaining := 0
	for _, svc := range serviceNames {
		if verifyHostsEntry(t, svc) {
			t.Logf("WARNING: Service '%s' still in /etc/hosts", svc)
			entriesRemaining++
		}
	}

	if entriesRemaining > 0 {
		t.Errorf("%d service entries remain in /etc/hosts after cleanup", entriesRemaining)
	} else {
		t.Log("✓ All service entries removed from /etc/hosts")
	}
}

// TestNetworkAliasRemoval tests that loopback IP aliases (127.x.x.x) are removed
func TestNetworkAliasRemoval(t *testing.T) {
	requiresSudo(t)
	requiresKindCluster(t)

	// Get initial loopback aliases
	initialAliases := getLoopbackAliases(t)
	t.Logf("Initial 127.1.x.x aliases: %d", len(initialAliases))

	// Start kubefwd
	cmd := startKubefwd(t, "svc", "-n", "test-simple", "-v")

	// Wait for forwarding
	time.Sleep(10 * time.Second)

	// Get loopback aliases after kubefwd starts
	activeAliases := getLoopbackAliases(t)
	t.Logf("Active 127.1.x.x aliases (during forwarding): %d", len(activeAliases))

	// Should have more aliases now
	if len(activeAliases) <= len(initialAliases) {
		t.Error("No new loopback aliases detected (expected aliases for forwarded services)")
	} else {
		added := len(activeAliases) - len(initialAliases)
		t.Logf("✓ %d loopback aliases added for port forwarding", added)
	}

	// Stop kubefwd
	stopKubefwd(t, cmd)

	// Get loopback aliases after cleanup
	finalAliases := getLoopbackAliases(t)
	t.Logf("Final 127.1.x.x aliases: %d", len(finalAliases))

	// Should be back to initial count
	aliasDiff := abs(len(finalAliases) - len(initialAliases))
	if aliasDiff > 0 {
		t.Errorf("Loopback aliases not fully removed: %d aliases remain", aliasDiff)
		if len(finalAliases) > 0 {
			t.Logf("Remaining aliases: %v", finalAliases)
		}
	} else {
		t.Log("✓ All loopback aliases removed")
	}
}

// TestNoLeakedProcesses tests that no kubefwd processes remain after shutdown
func TestNoLeakedProcesses(t *testing.T) {
	requiresSudo(t)
	requiresKindCluster(t)

	// Verify no kubefwd processes before test
	if hasKubefwdProcesses(t) {
		t.Fatal("kubefwd processes detected before test started - please clean up first")
	}

	// Start and stop kubefwd multiple times
	iterations := 3
	for i := 0; i < iterations; i++ {
		t.Logf("Iteration %d/%d", i+1, iterations)

		// Start kubefwd
		cmd := startKubefwd(t, "svc", "-n", "test-simple", "-v")

		// Brief operation time
		time.Sleep(5 * time.Second)

		// Stop kubefwd
		stopKubefwd(t, cmd)

		// Check for leaked processes
		time.Sleep(2 * time.Second)
		if hasKubefwdProcesses(t) {
			t.Errorf("Iteration %d: kubefwd processes leaked after shutdown", i+1)
			output, _ := exec.Command("pgrep", "-af", "kubefwd").CombinedOutput()
			t.Logf("Leaked processes:\n%s", output)
		} else {
			t.Logf("✓ Iteration %d: No process leaks", i+1)
		}
	}

	// Final verification
	if hasKubefwdProcesses(t) {
		t.Error("kubefwd processes remain after all iterations")
	} else {
		t.Log("✓ No kubefwd process leaks detected after all iterations")
	}
}

// Helper functions

func countNonCommentLines(content string) int {
	lines := strings.Split(content, "\n")
	count := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			count++
		}
	}
	return count
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func getLoopbackAliases(t *testing.T) []string {
	t.Helper()

	var cmd *exec.Cmd

	// Platform-specific commands
	if runtime.GOOS == "darwin" {
		// macOS
		cmd = exec.Command("ifconfig", "lo0")
	} else if runtime.GOOS == "linux" {
		// Linux
		cmd = exec.Command("ip", "addr", "show", "lo")
	} else {
		t.Skip("Unsupported platform for loopback alias check")
		return nil
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("Failed to get loopback aliases: %v", err)
		return []string{}
	}

	// Parse output for 127.1.x.x addresses
	lines := strings.Split(string(output), "\n")
	aliases := []string{}

	for _, line := range lines {
		// Look for inet 127.1.x.x
		if strings.Contains(line, "inet 127.1.") || strings.Contains(line, "inet addr:127.1.") {
			// Extract IP address
			fields := strings.Fields(line)
			for _, field := range fields {
				if strings.HasPrefix(field, "127.1.") {
					// Remove /netmask if present
					ip := strings.Split(field, "/")[0]
					aliases = append(aliases, ip)
					break
				}
			}
		}
	}

	return aliases
}

func hasKubefwdProcesses(t *testing.T) bool {
	t.Helper()

	cmd := exec.Command("pgrep", "-f", "kubefwd")
	err := cmd.Run()
	// pgrep returns 0 if processes found, 1 if none found
	return err == nil
}
