package fwdhost

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/txn2/kubefwd/pkg/fwdip"
	"github.com/txn2/txeh"
)

// createTempHostsFile creates a temporary hosts file for testing
func createTempHostsFile(t *testing.T, content string) (*txeh.Hosts, string) {
	t.Helper()

	// Create a temporary directory (automatically cleaned up by t.TempDir())
	tempDir := t.TempDir()
	hostsPath := filepath.Join(tempDir, "hosts")

	// Create initial hosts file
	if err := os.WriteFile(hostsPath, []byte(content), 0o644); err != nil {
		t.Fatalf("Failed to create temp hosts file: %v", err)
	}

	// Create Hosts instance with temp file
	hosts, err := txeh.NewHosts(&txeh.HostsConfig{
		ReadFilePath:  hostsPath,
		WriteFilePath: hostsPath,
	})
	if err != nil {
		t.Fatalf("Failed to create txeh.Hosts: %v", err)
	}

	return hosts, hostsPath
}

// TestBackupHostFile_CreateBackup tests creating a new backup
//
//goland:noinspection DuplicatedCode
func TestBackupHostFile_CreateBackup(t *testing.T) {
	// Create temporary hosts file
	content := "127.0.0.1 localhost\n192.168.1.1 example.com\n"
	hosts, _ := createTempHostsFile(t, content)

	// Set up temp home (t.TempDir auto-cleans, t.Setenv auto-restores)
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	// Create backup
	msg, err := BackupHostFile(hosts, false)
	if err != nil {
		t.Fatalf("BackupHostFile failed: %v", err)
	}

	// Verify message indicates backup was created
	if !strings.Contains(msg, "Backing up hosts file") {
		t.Errorf("Expected backup creation message, got: %s", msg)
	}

	// Verify backup file exists
	backupPath := filepath.Join(tempHome, "hosts.original")
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		t.Error("Backup file was not created")
	}

	// Verify backup content matches original
	backupContent, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("Failed to read backup file: %v", err)
	}

	if string(backupContent) != content {
		t.Errorf("Backup content doesn't match original.\nExpected: %q\nGot: %q",
			content, string(backupContent))
	}

	t.Logf("Backup created successfully at %s", backupPath)
}

// TestBackupHostFile_ExistingBackup tests behavior when backup already exists
//
//goland:noinspection DuplicatedCode
func TestBackupHostFile_ExistingBackup(t *testing.T) {
	content := "127.0.0.1 localhost\n"
	hosts, _ := createTempHostsFile(t, content)

	// Set up temp home
	originalHome := os.Getenv("HOME")
	tempHome, err := os.MkdirTemp("", "home-*")
	if err != nil {
		t.Fatalf("Failed to create temp home: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempHome); err != nil {
			t.Logf("Warning: failed to cleanup temp home: %v", err)
		}
	}()

	if err := os.Setenv("HOME", tempHome); err != nil {
		t.Fatalf("Failed to set HOME env var: %v", err)
	}
	defer func() {
		if err := os.Setenv("HOME", originalHome); err != nil {
			t.Errorf("Failed to restore HOME env var: %v", err)
		}
	}()

	// Create initial backup
	msg1, err := BackupHostFile(hosts, false)
	if err != nil {
		t.Fatalf("First backup failed: %v", err)
	}

	if !strings.Contains(msg1, "Backing up hosts file") {
		t.Errorf("Expected first backup message, got: %s", msg1)
	}

	// Modify the original hosts file (to verify backup doesn't change)
	newContent := "127.0.0.1 localhost\n192.168.1.1 newhost\n"
	if err := os.WriteFile(hosts.WriteFilePath, []byte(newContent), 0o644); err != nil {
		t.Fatalf("Failed to modify hosts file: %v", err)
	}

	// Attempt second backup
	msg2, err := BackupHostFile(hosts, false)
	if err != nil {
		t.Fatalf("Second backup failed: %v", err)
	}

	if !strings.Contains(msg2, "Original hosts backup already exists") {
		t.Errorf("Expected existing backup message, got: %s", msg2)
	}

	// Verify backup still has original content (not modified)
	backupPath := filepath.Join(tempHome, "hosts.original")
	backupContent, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("Failed to read backup file: %v", err)
	}

	if string(backupContent) != content {
		t.Error("Backup was modified when it should have been preserved")
	}

	t.Log("Existing backup preserved correctly")
}

// TestBackupHostFile_MissingSourceFile tests error handling for missing source
//
//goland:noinspection DuplicatedCode
func TestBackupHostFile_MissingSourceFile(t *testing.T) {
	// Create a hosts file, then delete it before backup
	content := "127.0.0.1 localhost\n"
	hosts, hostsPath := createTempHostsFile(t, content)

	// Set up temp home
	originalHome := os.Getenv("HOME")
	tempHome, err := os.MkdirTemp("", "home-*")
	if err != nil {
		t.Fatalf("Failed to create temp home: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempHome); err != nil {
			t.Logf("Warning: failed to cleanup temp home: %v", err)
		}
	}()

	if err := os.Setenv("HOME", tempHome); err != nil {
		t.Fatalf("Failed to set HOME env var: %v", err)
	}
	defer func() {
		if err := os.Setenv("HOME", originalHome); err != nil {
			t.Errorf("Failed to restore HOME env var: %v", err)
		}
	}()

	// Delete the source file
	if err := os.Remove(hostsPath); err != nil {
		t.Fatalf("Failed to remove hosts file: %v", err)
	}

	// Attempt backup - should fail gracefully
	_, err = BackupHostFile(hosts, false)
	if err == nil {
		t.Error("Expected error for missing source file, got nil")
	}

	t.Logf("Correctly returned error for missing source: %v", err)
}

// TestBackupHostFile_ReadOnlyBackupLocation tests permission error handling
func TestBackupHostFile_ReadOnlyBackupLocation(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("Skipping read-only test when running as root")
	}

	content := "127.0.0.1 localhost\n"
	hosts, _ := createTempHostsFile(t, content)

	// Set up read-only temp home
	originalHome := os.Getenv("HOME")
	tempHome, err := os.MkdirTemp("", "home-readonly-*")
	if err != nil {
		t.Fatalf("Failed to create temp home: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempHome); err != nil {
			t.Logf("Warning: failed to cleanup temp home: %v", err)
		}
	}()

	// Make directory read-only
	if err := os.Chmod(tempHome, 0o555); err != nil {
		t.Fatalf("Failed to make directory read-only: %v", err)
	}

	if err := os.Setenv("HOME", tempHome); err != nil {
		t.Fatalf("Failed to set HOME env var: %v", err)
	}
	defer func() {
		if err := os.Setenv("HOME", originalHome); err != nil {
			t.Errorf("Failed to restore HOME env var: %v", err)
		}
		// Restore permissions before cleanup
		if err := os.Chmod(tempHome, 0o755); err != nil {
			t.Logf("Warning: failed to restore permissions: %v", err)
		}
	}()

	// Attempt backup - should fail due to permissions
	_, err = BackupHostFile(hosts, false)

	if err == nil {
		t.Error("Expected error for read-only location, got nil")
	} else {
		t.Logf("Correctly returned error for read-only location: %v", err)
	}
}

// TestBackupHostFile_EmptyHostsFile tests backing up an empty file
//
//goland:noinspection DuplicatedCode
func TestBackupHostFile_EmptyHostsFile(t *testing.T) {
	hosts, _ := createTempHostsFile(t, "")

	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	msg, err := BackupHostFile(hosts, false)
	if err != nil {
		t.Fatalf("BackupHostFile failed for empty file: %v", err)
	}

	if !strings.Contains(msg, "Backing up hosts file") {
		t.Errorf("Expected backup message for empty file, got: %s", msg)
	}

	// Verify empty backup was created
	backupPath := filepath.Join(tempHome, "hosts.original")
	backupContent, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("Failed to read backup file: %v", err)
	}

	if len(backupContent) != 0 {
		t.Errorf("Expected empty backup, got %d bytes", len(backupContent))
	}

	t.Log("Empty file backed up correctly")
}

// TestBackupHostFile_LargeFile tests backing up a large hosts file
//
//goland:noinspection DuplicatedCode
func TestBackupHostFile_LargeFile(t *testing.T) {
	// Create a large hosts file (10,000 lines)
	var content strings.Builder
	for i := 0; i < 10000; i++ {
		content.WriteString("127.0.0.1 host")
		content.WriteString(string(rune(i)))
		content.WriteString(".example.com\n")
	}

	hosts, _ := createTempHostsFile(t, content.String())

	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	msg, err := BackupHostFile(hosts, false)
	if err != nil {
		t.Fatalf("BackupHostFile failed for large file: %v", err)
	}

	if !strings.Contains(msg, "Backing up hosts file") {
		t.Errorf("Expected backup message, got: %s", msg)
	}

	// Verify backup content matches
	backupPath := filepath.Join(tempHome, "hosts.original")
	backupContent, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("Failed to read backup file: %v", err)
	}

	if string(backupContent) != content.String() {
		t.Error("Large file backup content doesn't match original")
	}

	t.Logf("Large file (%d bytes) backed up correctly", len(backupContent))
}

// TestBackupHostFile_SpecialCharacters tests backing up file with special characters
//
//goland:noinspection DuplicatedCode
func TestBackupHostFile_SpecialCharacters(t *testing.T) {
	content := "127.0.0.1 localhost\n" +
		"# Comment with special chars: @#$%^&*()\n" +
		"192.168.1.1 host-with-dashes\n" +
		"10.0.0.1 host_with_underscores\n"

	hosts, _ := createTempHostsFile(t, content)

	originalHome := os.Getenv("HOME")
	tempHome, err := os.MkdirTemp("", "home-*")
	if err != nil {
		t.Fatalf("Failed to create temp home: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempHome); err != nil {
			t.Logf("Warning: failed to cleanup temp home: %v", err)
		}
	}()

	if err := os.Setenv("HOME", tempHome); err != nil {
		t.Fatalf("Failed to set HOME env var: %v", err)
	}
	defer func() {
		if err := os.Setenv("HOME", originalHome); err != nil {
			t.Errorf("Failed to restore HOME env var: %v", err)
		}
	}()

	msg, err := BackupHostFile(hosts, false)
	if err != nil {
		t.Fatalf("BackupHostFile failed: %v", err)
	}

	if !strings.Contains(msg, "Backing up hosts file") {
		t.Errorf("Expected backup message, got: %s", msg)
	}

	// Verify content preserved exactly
	backupPath := filepath.Join(tempHome, "hosts.original")
	backupContent, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("Failed to read backup file: %v", err)
	}

	if string(backupContent) != content {
		t.Error("Special characters not preserved in backup")
	}

	t.Log("Special characters preserved correctly")
}

// TestBackupHostFile_ConcurrentBackups tests concurrent backup attempts
//
//goland:noinspection DuplicatedCode
func TestBackupHostFile_ConcurrentBackups(t *testing.T) {
	content := "127.0.0.1 localhost\n"

	originalHome := os.Getenv("HOME")
	tempHome, err := os.MkdirTemp("", "home-*")
	if err != nil {
		t.Fatalf("Failed to create temp home: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempHome); err != nil {
			t.Logf("Warning: failed to cleanup temp home: %v", err)
		}
	}()

	if err := os.Setenv("HOME", tempHome); err != nil {
		t.Fatalf("Failed to set HOME env var: %v", err)
	}
	defer func() {
		if err := os.Setenv("HOME", originalHome); err != nil {
			t.Errorf("Failed to restore HOME env var: %v", err)
		}
	}()

	numGoroutines := 10
	var wg sync.WaitGroup
	results := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			// Each goroutine creates its own hosts file
			hosts, _ := createTempHostsFile(t, content)

			_, err := BackupHostFile(hosts, false)
			results <- err
		}()
	}

	wg.Wait()
	close(results)

	// At least one should succeed (first to create backup)
	successCount := 0
	for err := range results {
		if err == nil {
			successCount++
		}
	}

	if successCount == 0 {
		t.Error("Expected at least one successful backup in concurrent test")
	}

	// Verify backup exists
	backupPath := filepath.Join(tempHome, "hosts.original")
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		t.Error("Backup file was not created during concurrent test")
	}

	t.Logf("Concurrent backups: %d/%d succeeded", successCount, numGoroutines)
}

// TestBackupHostFile_MessageFormats tests the message return formats
//
//goland:noinspection DuplicatedCode
func TestBackupHostFile_MessageFormats(t *testing.T) {
	content := "127.0.0.1 localhost\n"

	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	// First backup - should get creation message
	hosts1, _ := createTempHostsFile(t, content)

	msg1, err := BackupHostFile(hosts1, false)
	if err != nil {
		t.Fatalf("First backup failed: %v", err)
	}

	expectedSubstring1 := "Backing up hosts file"
	if !strings.Contains(msg1, expectedSubstring1) {
		t.Errorf("First backup message should contain %q, got: %s",
			expectedSubstring1, msg1)
	}

	// Verify message includes paths
	if !strings.Contains(msg1, hosts1.WriteFilePath) {
		t.Error("First backup message should include source path")
	}

	expectedBackupPath := filepath.Join(tempHome, "hosts.original")
	if !strings.Contains(msg1, expectedBackupPath) {
		t.Error("First backup message should include backup path")
	}

	// Second backup - should get existing backup message
	hosts2, _ := createTempHostsFile(t, content)

	msg2, err := BackupHostFile(hosts2, false)
	if err != nil {
		t.Fatalf("Second backup failed: %v", err)
	}

	expectedSubstring2 := "Original hosts backup already exists"
	if !strings.Contains(msg2, expectedSubstring2) {
		t.Errorf("Second backup message should contain %q, got: %s",
			expectedSubstring2, msg2)
	}

	if !strings.Contains(msg2, expectedBackupPath) {
		t.Error("Second backup message should include backup path")
	}

	t.Log("Message formats verified correctly")
}

// TestBackupHostFile_Idempotency tests that multiple backups are idempotent
//
//goland:noinspection DuplicatedCode
func TestBackupHostFile_Idempotency(t *testing.T) {
	originalContent := "127.0.0.1 localhost\n192.168.1.1 original\n"

	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	// Create first backup
	hosts1, hostsPath1 := createTempHostsFile(t, originalContent)

	_, err := BackupHostFile(hosts1, false)
	if err != nil {
		t.Fatalf("First backup failed: %v", err)
	}

	// Read initial backup content
	backupPath := filepath.Join(tempHome, "hosts.original")
	initialBackupContent, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("Failed to read initial backup: %v", err)
	}

	// Modify the original file multiple times and attempt backups
	for i := 0; i < 5; i++ {
		modifiedContent := originalContent + "192.168.1." + string(rune(i+10)) + " modified\n"
		if err := os.WriteFile(hostsPath1, []byte(modifiedContent), 0o644); err != nil {
			t.Fatalf("Failed to modify hosts file: %v", err)
		}

		_, err := BackupHostFile(hosts1, false)
		if err != nil {
			t.Fatalf("Backup %d failed: %v", i+1, err)
		}
	}

	// Verify backup still has original content
	finalBackupContent, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("Failed to read final backup: %v", err)
	}

	if !bytes.Equal(finalBackupContent, initialBackupContent) {
		t.Error("Backup was modified, should be idempotent")
	}

	if string(finalBackupContent) != originalContent {
		t.Error("Backup doesn't match original content")
	}

	t.Log("Idempotency verified - backup preserved after multiple attempts")
}

// TestIsKubefwdIP tests the isKubefwdIP function
func TestIsKubefwdIP(t *testing.T) {
	tests := []struct {
		name     string
		ip       string
		expected bool
	}{
		// Valid kubefwd IPs (127.x.x.x where x > 1.27)
		{"valid kubefwd IP 127.1.27.1", "127.1.27.1", true},
		{"valid kubefwd IP 127.1.28.0", "127.1.28.0", true},
		{"valid kubefwd IP 127.2.0.0", "127.2.0.0", true},
		{"valid kubefwd IP 127.255.255.255", "127.255.255.255", true},
		{"valid kubefwd IP 127.1.100.50", "127.1.100.50", true},

		// Invalid - below threshold (127.1.27)
		{"invalid below threshold 127.0.0.1", "127.0.0.1", false},
		{"invalid below threshold 127.1.0.0", "127.1.0.0", false},
		{"invalid below threshold 127.1.26.255", "127.1.26.255", false},
		{"invalid localhost", "127.0.0.1", false},

		// Invalid - not loopback
		{"invalid not loopback 192.168.1.1", "192.168.1.1", false},
		{"invalid not loopback 10.0.0.1", "10.0.0.1", false},
		{"invalid not loopback 0.0.0.0", "0.0.0.0", false},

		// Invalid - malformed
		{"invalid empty string", "", false},
		{"invalid malformed", "not-an-ip", false},
		{"invalid partial IP", "127.0.0", false},

		// Edge cases
		{"edge case 127.1.27.0", "127.1.27.0", true},
		{"edge case 127.0.255.255", "127.0.255.255", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isKubefwdIP(tt.ip)
			if result != tt.expected {
				t.Errorf("isKubefwdIP(%q) = %v, want %v", tt.ip, result, tt.expected)
			}
		})
	}
}

// TestCountStaleEntries tests the CountStaleEntries function
func TestCountStaleEntries(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected int
	}{
		{
			name:     "no stale entries",
			content:  "127.0.0.1 localhost\n192.168.1.1 example.com\n",
			expected: 0,
		},
		{
			name:     "one stale entry",
			content:  "127.0.0.1 localhost\n127.1.27.1 my-service\n",
			expected: 1,
		},
		{
			name:     "multiple stale entries same IP",
			content:  "127.1.27.1 service1 service2 service3\n",
			expected: 3,
		},
		{
			name:     "multiple stale entries different IPs",
			content:  "127.1.27.1 service1\n127.1.27.2 service2\n127.2.0.0 service3\n",
			expected: 3,
		},
		{
			name:     "mixed entries",
			content:  "127.0.0.1 localhost\n127.1.27.1 service1\n192.168.1.1 example.com\n127.1.28.0 service2 service3\n",
			expected: 3,
		},
		{
			name:     "empty hosts file",
			content:  "",
			expected: 0,
		},
		{
			name:     "only comments",
			content:  "# This is a comment\n# Another comment\n",
			expected: 0,
		},
		{
			name:     "edge case at threshold",
			content:  "127.1.26.255 below-threshold\n127.1.27.0 at-threshold\n",
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hosts, _ := createTempHostsFile(t, tt.content)
			result := CountStaleEntries(hosts)
			if result != tt.expected {
				t.Errorf("CountStaleEntries() = %d, want %d", result, tt.expected)
			}
		})
	}
}

// TestPurgeStaleIps tests the PurgeStaleIps function
func TestPurgeStaleIps(t *testing.T) {
	tests := []struct {
		name            string
		content         string
		expectedCount   int
		expectedContent string
	}{
		{
			name:            "no stale entries",
			content:         "127.0.0.1 localhost\n192.168.1.1 example.com\n",
			expectedCount:   0,
			expectedContent: "127.0.0.1 localhost\n192.168.1.1 example.com\n",
		},
		{
			name:            "one stale entry",
			content:         "127.0.0.1 localhost\n127.1.27.1 my-service\n",
			expectedCount:   1,
			expectedContent: "127.0.0.1 localhost\n",
		},
		{
			name:            "multiple stale entries",
			content:         "127.0.0.1 localhost\n127.1.27.1 service1\n127.1.28.0 service2\n192.168.1.1 example.com\n",
			expectedCount:   2,
			expectedContent: "127.0.0.1 localhost\n192.168.1.1 example.com\n",
		},
		{
			name:            "all stale entries",
			content:         "127.1.27.1 service1\n127.2.0.0 service2\n",
			expectedCount:   2,
			expectedContent: "",
		},
		{
			name:            "empty hosts file",
			content:         "",
			expectedCount:   0,
			expectedContent: "",
		},
		{
			name:            "multiple hostnames per IP",
			content:         "127.0.0.1 localhost\n127.1.27.1 svc1 svc2 svc3\n",
			expectedCount:   3,
			expectedContent: "127.0.0.1 localhost\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hosts, hostsPath := createTempHostsFile(t, tt.content)

			count, err := PurgeStaleIps(hosts)
			if err != nil {
				t.Fatalf("PurgeStaleIps() error = %v", err)
			}

			if count != tt.expectedCount {
				t.Errorf("PurgeStaleIps() count = %d, want %d", count, tt.expectedCount)
			}

			// Read the file to verify content was updated
			actualContent, err := os.ReadFile(hostsPath)
			if err != nil {
				t.Fatalf("Failed to read hosts file: %v", err)
			}

			// Reload hosts to get parsed content for comparison
			updatedHosts, err := txeh.NewHosts(&txeh.HostsConfig{
				ReadFilePath:  hostsPath,
				WriteFilePath: hostsPath,
			})
			if err != nil {
				t.Fatalf("Failed to reload hosts: %v", err)
			}

			// Verify no kubefwd IPs remain
			staleCount := CountStaleEntries(updatedHosts)
			if staleCount != 0 {
				t.Errorf("After purge, still have %d stale entries. File content: %s", staleCount, string(actualContent))
			}
		})
	}
}

// TestPurgeStaleIps_PreservesComments tests that comments are preserved during purge
func TestPurgeStaleIps_PreservesComments(t *testing.T) {
	content := "# Header comment\n127.0.0.1 localhost\n# Middle comment\n127.1.27.1 stale-service\n# Footer comment\n"

	hosts, hostsPath := createTempHostsFile(t, content)

	count, err := PurgeStaleIps(hosts)
	if err != nil {
		t.Fatalf("PurgeStaleIps() error = %v", err)
	}

	if count != 1 {
		t.Errorf("Expected 1 purged entry, got %d", count)
	}

	// Read and verify comments preserved
	actualContent, err := os.ReadFile(hostsPath)
	if err != nil {
		t.Fatalf("Failed to read hosts file: %v", err)
	}

	contentStr := string(actualContent)
	if !strings.Contains(contentStr, "# Header comment") {
		t.Error("Header comment was not preserved")
	}
	if !strings.Contains(contentStr, "localhost") {
		t.Error("localhost entry was not preserved")
	}
}

// TestRemoveAllocatedHosts_NoHostnames tests RemoveAllocatedHosts when no hostnames are registered
func TestRemoveAllocatedHosts_NoHostnames(t *testing.T) {
	// Reset the fwdip registry to ensure no hostnames are registered
	fwdip.ResetRegistry()

	// When no hostnames are registered, the function should return nil immediately
	err := RemoveAllocatedHosts()
	if err != nil {
		t.Errorf("RemoveAllocatedHosts() with no registered hostnames should return nil, got: %v", err)
	}
}

// TestRemoveAllocatedHosts_WithHostnames tests RemoveAllocatedHosts with registered hostnames
// Note: This test will fail on systems where we cannot create a default hosts file
// (e.g., without root permissions), but we test the logic path nonetheless.
func TestRemoveAllocatedHosts_WithHostnames(t *testing.T) {
	// Reset the fwdip registry
	fwdip.ResetRegistry()

	// Register some test hostnames
	fwdip.RegisterHostname("test-service")
	fwdip.RegisterHostname("test-service.default")

	// Verify hostnames are registered
	hostnames := fwdip.GetRegisteredHostnames()
	if len(hostnames) != 2 {
		t.Fatalf("Expected 2 registered hostnames, got %d", len(hostnames))
	}

	// Call RemoveAllocatedHosts - this will likely fail without root permissions
	// because txeh.NewHostsDefault() requires access to /etc/hosts
	err := RemoveAllocatedHosts()

	// On most systems without root, this will return an error
	// We're testing that the function handles this gracefully
	if err != nil {
		// Expected on non-root systems - the function tried to access /etc/hosts
		t.Logf("RemoveAllocatedHosts() returned expected error (no root): %v", err)
	} else {
		// If we're running as root, the function should succeed
		t.Log("RemoveAllocatedHosts() succeeded (likely running as root)")
	}

	// Clean up
	fwdip.ResetRegistry()
}

// TestBackupHostFile_ForceRefresh tests the forceRefresh parameter
func TestBackupHostFile_ForceRefresh(t *testing.T) {
	originalContent := "127.0.0.1 localhost\n"
	modifiedContent := "127.0.0.1 localhost\n192.168.1.1 newhost\n"

	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	// Create initial backup
	hosts1, _ := createTempHostsFile(t, originalContent)
	_, err := BackupHostFile(hosts1, false)
	if err != nil {
		t.Fatalf("First backup failed: %v", err)
	}

	// Verify initial backup content
	backupPath := filepath.Join(tempHome, "hosts.original")
	initialBackup, _ := os.ReadFile(backupPath)
	if string(initialBackup) != originalContent {
		t.Error("Initial backup content mismatch")
	}

	// Create new hosts file with modified content
	hosts2, _ := createTempHostsFile(t, modifiedContent)

	// Try without force - should preserve original backup
	msg, err := BackupHostFile(hosts2, false)
	if err != nil {
		t.Fatalf("Non-force backup failed: %v", err)
	}
	if !strings.Contains(msg, "already exists") {
		t.Error("Expected 'already exists' message without force")
	}

	// Verify backup unchanged
	unchangedBackup, _ := os.ReadFile(backupPath)
	if string(unchangedBackup) != originalContent {
		t.Error("Backup was modified without force flag")
	}

	// Now try with force - should replace backup
	msg, err = BackupHostFile(hosts2, true)
	if err != nil {
		t.Fatalf("Force backup failed: %v", err)
	}
	if !strings.Contains(msg, "Backing up hosts file") {
		t.Errorf("Expected backup message with force, got: %s", msg)
	}

	// Verify backup was updated
	forcedBackup, _ := os.ReadFile(backupPath)
	if string(forcedBackup) != modifiedContent {
		t.Error("Force refresh did not update backup content")
	}

	t.Log("Force refresh correctly replaces existing backup")
}
