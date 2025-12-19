package fwdhost

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/txn2/txeh"
)

// createTempHostsFile creates a temporary hosts file for testing
func createTempHostsFile(t *testing.T, content string) (*txeh.Hosts, string, func()) {
	t.Helper()

	// Create a temporary directory
	tempDir, err := os.MkdirTemp("", "fwdhost-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	hostsPath := filepath.Join(tempDir, "hosts")

	// Create initial hosts file
	if err := os.WriteFile(hostsPath, []byte(content), 0644); err != nil {
		if removeErr := os.RemoveAll(tempDir); removeErr != nil {
			t.Logf("Warning: failed to cleanup temp dir: %v", removeErr)
		}
		t.Fatalf("Failed to create temp hosts file: %v", err)
	}

	// Create Hosts instance with temp file
	hosts, err := txeh.NewHosts(&txeh.HostsConfig{
		ReadFilePath:  hostsPath,
		WriteFilePath: hostsPath,
	})
	if err != nil {
		if removeErr := os.RemoveAll(tempDir); removeErr != nil {
			t.Logf("Warning: failed to cleanup temp dir: %v", removeErr)
		}
		t.Fatalf("Failed to create txeh.Hosts: %v", err)
	}

	cleanup := func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Logf("Warning: failed to cleanup temp dir: %v", err)
		}
	}

	return hosts, hostsPath, cleanup
}

// TestBackupHostFile_CreateBackup tests creating a new backup
//
//goland:noinspection DuplicatedCode
func TestBackupHostFile_CreateBackup(t *testing.T) {
	// Create temporary hosts file
	content := "127.0.0.1 localhost\n192.168.1.1 example.com\n"
	hosts, _, cleanup := createTempHostsFile(t, content)
	defer cleanup()

	// Get original home dir and set up temp home
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

	// Override HOME for this test
	if err := os.Setenv("HOME", tempHome); err != nil {
		t.Fatalf("Failed to set HOME env var: %v", err)
	}
	defer func() {
		if err := os.Setenv("HOME", originalHome); err != nil {
			t.Errorf("Failed to restore HOME env var: %v", err)
		}
	}()

	// Create backup
	msg, err := BackupHostFile(hosts)
	if err != nil {
		t.Fatalf("BackupHostFile failed: %v", err)
	}

	// Verify message indicates backup was created
	if !strings.Contains(msg, "Backing up your original hosts file") {
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
	hosts, _, cleanup := createTempHostsFile(t, content)
	defer cleanup()

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
	msg1, err := BackupHostFile(hosts)
	if err != nil {
		t.Fatalf("First backup failed: %v", err)
	}

	if !strings.Contains(msg1, "Backing up your original hosts file") {
		t.Errorf("Expected first backup message, got: %s", msg1)
	}

	// Modify the original hosts file (to verify backup doesn't change)
	newContent := "127.0.0.1 localhost\n192.168.1.1 newhost\n"
	if err := os.WriteFile(hosts.WriteFilePath, []byte(newContent), 0644); err != nil {
		t.Fatalf("Failed to modify hosts file: %v", err)
	}

	// Attempt second backup
	msg2, err := BackupHostFile(hosts)
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
	hosts, hostsPath, cleanup := createTempHostsFile(t, content)
	defer cleanup()

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
	_, err = BackupHostFile(hosts)
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
	hosts, _, cleanup := createTempHostsFile(t, content)
	defer cleanup()

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
	if err := os.Chmod(tempHome, 0555); err != nil {
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
		if err := os.Chmod(tempHome, 0755); err != nil {
			t.Logf("Warning: failed to restore permissions: %v", err)
		}
	}()

	// Attempt backup - should fail due to permissions
	_, err = BackupHostFile(hosts)

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
	hosts, _, cleanup := createTempHostsFile(t, "")
	defer cleanup()

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

	msg, err := BackupHostFile(hosts)
	if err != nil {
		t.Fatalf("BackupHostFile failed for empty file: %v", err)
	}

	if !strings.Contains(msg, "Backing up your original hosts file") {
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

	hosts, _, cleanup := createTempHostsFile(t, content.String())
	defer cleanup()

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

	msg, err := BackupHostFile(hosts)
	if err != nil {
		t.Fatalf("BackupHostFile failed for large file: %v", err)
	}

	if !strings.Contains(msg, "Backing up your original hosts file") {
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

	hosts, _, cleanup := createTempHostsFile(t, content)
	defer cleanup()

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

	msg, err := BackupHostFile(hosts)
	if err != nil {
		t.Fatalf("BackupHostFile failed: %v", err)
	}

	if !strings.Contains(msg, "Backing up your original hosts file") {
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
		go func(n int) {
			defer wg.Done()

			// Each goroutine creates its own hosts file
			hosts, _, cleanup := createTempHostsFile(t, content)
			defer cleanup()

			_, err := BackupHostFile(hosts)
			results <- err
		}(i)
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

	// First backup - should get creation message
	hosts1, _, cleanup1 := createTempHostsFile(t, content)
	defer cleanup1()

	msg1, err := BackupHostFile(hosts1)
	if err != nil {
		t.Fatalf("First backup failed: %v", err)
	}

	expectedSubstring1 := "Backing up your original hosts file"
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
	hosts2, _, cleanup2 := createTempHostsFile(t, content)
	defer cleanup2()

	msg2, err := BackupHostFile(hosts2)
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

	// Create first backup
	hosts1, hostsPath1, cleanup1 := createTempHostsFile(t, originalContent)
	defer cleanup1()

	_, err = BackupHostFile(hosts1)
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
		if err := os.WriteFile(hostsPath1, []byte(modifiedContent), 0644); err != nil {
			t.Fatalf("Failed to modify hosts file: %v", err)
		}

		_, err := BackupHostFile(hosts1)
		if err != nil {
			t.Fatalf("Backup %d failed: %v", i+1, err)
		}
	}

	// Verify backup still has original content
	finalBackupContent, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("Failed to read final backup: %v", err)
	}

	if string(finalBackupContent) != string(initialBackupContent) {
		t.Error("Backup was modified, should be idempotent")
	}

	if string(finalBackupContent) != originalContent {
		t.Error("Backup doesn't match original content")
	}

	t.Log("Idempotency verified - backup preserved after multiple attempts")
}
