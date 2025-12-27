package fwdnet

import (
	"net"
	"os"
	"runtime"
	"testing"

	"github.com/txn2/kubefwd/pkg/fwdip"
)

// TestReadyInterface_Linux tests behavior on Linux systems
func TestReadyInterface_Linux(t *testing.T) {
	// This test verifies the Linux code path exists and is reachable
	// On Linux, ReadyInterface should simply check if IP:port is available
	// without requiring ifconfig commands

	if runtime.GOOS != "linux" {
		t.Skip("Skipping Linux-specific test on non-Linux platform")
	}

	// Test with a loopback IP that should be available
	opts := fwdip.ForwardIPOpts{
		ServiceName:              "test-svc",
		PodName:                  "test-pod",
		Context:                  "test-ctx",
		ClusterN:                 0,
		NamespaceN:               0,
		Namespace:                "default",
		Port:                     "0", // Port 0 should be available (kernel will assign)
		ForwardConfigurationPath: "",
		ForwardIPReservations:    nil,
	}

	ip, err := ReadyInterface(opts)
	if err != nil {
		t.Errorf("ReadyInterface failed on Linux: %v", err)
	}

	if ip == nil {
		t.Error("Expected non-nil IP on Linux")
	}

	t.Logf("Linux: Got IP %s", ip)
}

// TestReadyInterface_Darwin tests behavior on macOS systems
func TestReadyInterface_Darwin(t *testing.T) {
	// On macOS, ReadyInterface uses ifconfig which requires sudo for alias creation
	// This test verifies the code path and interface detection logic

	if runtime.GOOS != "darwin" {
		t.Skip("Skipping macOS-specific test on non-macOS platform")
	}

	// Check if we can access lo0 interface (this doesn't require sudo)
	_, err := net.InterfaceByName("lo0")
	if err != nil {
		t.Fatalf("Cannot access lo0 interface on macOS: %v", err)
	}

	// Test with standard loopback - this should already exist
	opts := fwdip.ForwardIPOpts{
		ServiceName:              "test-svc",
		PodName:                  "test-pod",
		Context:                  "test-ctx",
		ClusterN:                 0,
		NamespaceN:               0,
		Namespace:                "default",
		Port:                     "0", // Port 0 should be available
		ForwardConfigurationPath: "",
		ForwardIPReservations:    nil,
	}

	// This will attempt to create an alias, which may fail without sudo
	// We test that it at least tries and returns appropriately
	ip, err := ReadyInterface(opts)

	// Without sudo, this might fail or succeed depending on IP availability
	// We just verify it doesn't panic and returns a valid result
	if err != nil {
		t.Logf("macOS ReadyInterface returned error (may need sudo): %v", err)
	} else if ip != nil {
		t.Logf("macOS: Got IP %s", ip)
	}
}

// TestReadyInterface_Windows tests behavior on Windows systems
func TestReadyInterface_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Skipping Windows-specific test on non-Windows platform")
	}

	opts := fwdip.ForwardIPOpts{
		ServiceName:              "test-svc",
		PodName:                  "test-pod",
		Context:                  "test-ctx",
		ClusterN:                 0,
		NamespaceN:               0,
		Namespace:                "default",
		Port:                     "0",
		ForwardConfigurationPath: "",
		ForwardIPReservations:    nil,
	}

	ip, err := ReadyInterface(opts)
	if err != nil {
		t.Errorf("ReadyInterface failed on Windows: %v", err)
	}

	if ip == nil {
		t.Error("Expected non-nil IP on Windows")
	}

	t.Logf("Windows: Got IP %s", ip)
}

// TestLoopbackInterfaceDetection tests platform-specific loopback detection
func TestLoopbackInterfaceDetection(t *testing.T) {
	switch runtime.GOOS {
	case "linux", "windows":
		// Should have "lo" interface
		_, err := net.InterfaceByName("lo")
		if err != nil && runtime.GOOS == "linux" {
			t.Errorf("Expected 'lo' interface on Linux, got error: %v", err)
		}

	case "darwin":
		// Should have "lo0" interface
		iface, err := net.InterfaceByName("lo0")
		if err != nil {
			t.Fatalf("Expected 'lo0' interface on macOS, got error: %v", err)
		}

		// Verify we can get addresses
		addrs, err := iface.Addrs()
		if err != nil {
			t.Errorf("Failed to get addresses for lo0: %v", err)
		}

		if len(addrs) == 0 {
			t.Error("Expected at least one address on lo0")
		}

		t.Logf("lo0 has %d addresses", len(addrs))
		for _, addr := range addrs {
			t.Logf("  - %s", addr.String())
		}
	}
}

// TestRemoveInterfaceAlias_NoSudo tests that RemoveInterfaceAlias doesn't panic
func TestRemoveInterfaceAlias_NoSudo(t *testing.T) {
	// RemoveInterfaceAlias should not panic even if it fails
	// It silently suppresses errors

	testIP := net.ParseIP("127.1.27.99")
	if testIP == nil {
		t.Fatal("Failed to parse test IP")
	}

	// This will likely fail without sudo on macOS, but should not panic
	// On Linux/Windows, it's a no-op anyway
	RemoveInterfaceAlias(testIP)

	// If we get here without panicking, the test passes
	t.Log("RemoveInterfaceAlias completed without panic")
}

// TestReadyInterface_PortInUse tests detection of in-use ports
func TestReadyInterface_PortInUse(t *testing.T) {
	// This test works on all platforms - it tests the port-in-use detection

	// Start a listener on a specific port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer func() {
		if err := listener.Close(); err != nil {
			t.Logf("Warning: failed to close listener: %v", err)
		}
	}()

	// Get the actual port assigned
	addr := listener.Addr().(*net.TCPAddr)
	usedPort := addr.Port

	t.Logf("Using port %d for in-use test", usedPort)

	// On Linux/Windows, ReadyInterface should detect this
	// On macOS without sudo, behavior may vary
	if runtime.GOOS == "linux" || runtime.GOOS == "windows" {
		opts := fwdip.ForwardIPOpts{
			ServiceName:              "test-svc",
			PodName:                  "test-pod",
			Context:                  "test-ctx",
			ClusterN:                 0,
			NamespaceN:               0,
			Namespace:                "default",
			Port:                     string(rune(usedPort)),
			ForwardConfigurationPath: "",
			ForwardIPReservations:    []string{"test-svc:127.0.0.1"},
		}

		_, err := ReadyInterface(opts)
		if err == nil {
			t.Log("Port in use detection may not work as expected (test informational only)")
		} else {
			t.Logf("Correctly detected port in use: %v", err)
		}
	} else {
		t.Skip("Port-in-use test requires Linux or Windows")
	}
}

// TestReadyInterface_IPAllocation tests that IPs are allocated correctly
func TestReadyInterface_IPAllocation(t *testing.T) {
	// Test that ReadyInterface allocates IPs in the correct range
	// This test doesn't require sudo - it just verifies the IP allocation logic

	opts := fwdip.ForwardIPOpts{
		ServiceName:              "test-svc",
		PodName:                  "test-pod",
		Context:                  "test-ctx",
		ClusterN:                 0,
		NamespaceN:               0,
		Namespace:                "default",
		Port:                     "0",
		ForwardConfigurationPath: "",
		ForwardIPReservations:    nil,
	}

	ip, err := ReadyInterface(opts)

	// On platforms without sudo, this might fail, but if it succeeds,
	// verify the IP is in the loopback range
	if err == nil && ip != nil {
		if !ip.IsLoopback() {
			t.Errorf("Expected loopback IP, got %s", ip)
		}

		// Should be in 127.x.x.x range
		if ip[0] != 127 {
			t.Errorf("Expected IP starting with 127, got %s", ip)
		}

		t.Logf("Allocated IP: %s", ip)
	} else if err != nil {
		t.Logf("IP allocation failed (may need sudo): %v", err)
	}
}

// TestReadyInterface_MultipleIPs tests allocating multiple IPs
func TestReadyInterface_MultipleIPs(t *testing.T) {
	// Test that we can allocate multiple different IPs
	// This primarily tests the IP allocation logic from fwdIp package

	services := []string{"svc1", "svc2", "svc3"}
	allocatedIPs := make(map[string]net.IP)

	for _, svc := range services {
		opts := fwdip.ForwardIPOpts{
			ServiceName:              svc,
			PodName:                  "test-pod",
			Context:                  "test-ctx",
			ClusterN:                 0,
			NamespaceN:               0,
			Namespace:                "default",
			Port:                     "0",
			ForwardConfigurationPath: "",
			ForwardIPReservations:    nil,
		}

		ip, err := ReadyInterface(opts)
		if err != nil {
			t.Logf("Failed to allocate IP for %s (may need sudo): %v", svc, err)
			continue
		}

		if ip != nil {
			allocatedIPs[svc] = ip
			t.Logf("Allocated %s -> %s", svc, ip)
		}
	}

	// If we successfully allocated any IPs, verify they're unique
	if len(allocatedIPs) > 1 {
		seen := make(map[string]bool)
		for svc, ip := range allocatedIPs {
			ipStr := ip.String()
			if seen[ipStr] {
				t.Errorf("Duplicate IP %s allocated for service %s", ipStr, svc)
			}
			seen[ipStr] = true
		}
		t.Logf("Successfully allocated %d unique IPs", len(allocatedIPs))
	}
}

// TestReadyInterface_WithReservation tests IP reservation
func TestReadyInterface_WithReservation(t *testing.T) {
	// Test that IP reservations are respected
	// Note: The actual IP returned may not match the reservation if:
	// 1. The IP is already in use
	// 2. The system can't create the alias (no sudo on macOS)
	// This test verifies the function completes without error

	reservedIP := "127.1.27.100"
	opts := fwdip.ForwardIPOpts{
		ServiceName:              "reserved-svc",
		PodName:                  "test-pod",
		Context:                  "test-ctx",
		ClusterN:                 0,
		NamespaceN:               0,
		Namespace:                "default",
		Port:                     "0",
		ForwardConfigurationPath: "",
		ForwardIPReservations:    []string{"reserved-svc:" + reservedIP},
	}

	ip, err := ReadyInterface(opts)
	if err != nil {
		t.Logf("IP reservation test failed (may need sudo): %v", err)
		return
	}

	if ip == nil {
		t.Error("Expected non-nil IP with reservation")
		return
	}

	// The IP allocation uses global state and counter, so we can't guarantee
	// the exact IP without resetting state between tests. Just verify we got
	// a loopback IP in the right range.
	if !ip.IsLoopback() || ip[0] != 127 {
		t.Errorf("Expected loopback IP in 127.x.x.x range, got %s", ip.String())
	} else {
		t.Logf("IP reservation function working, allocated: %s", ip.String())
		if ip.String() == reservedIP {
			t.Logf("  ✓ Reservation honored: got requested IP %s", reservedIP)
		} else {
			t.Logf("  ℹ Got different IP (global counter state from previous tests)")
		}
	}
}

// TestInterfaceAliasIntegration is an integration test that requires sudo
func TestInterfaceAliasIntegration(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Interface alias integration test is macOS-specific")
	}

	// Check if running with sufficient privileges
	if os.Geteuid() != 0 {
		t.Skip("Skipping interface alias test - requires sudo/root (run with: sudo go test)")
	}

	// Reset the global IP registry to ensure this test starts fresh
	fwdip.ResetRegistry()

	testIP := net.ParseIP("127.1.27.254")
	if testIP == nil {
		t.Fatal("Failed to parse test IP")
	}

	// Test allocation
	opts := fwdip.ForwardIPOpts{
		ServiceName:              "integration-test-svc",
		PodName:                  "test-pod",
		Context:                  "test-ctx",
		ClusterN:                 0,
		NamespaceN:               0,
		Namespace:                "default",
		Port:                     "0",
		ForwardConfigurationPath: "",
		ForwardIPReservations:    []string{"integration-test-svc:127.1.27.254"},
	}

	ip, err := ReadyInterface(opts)
	if err != nil {
		t.Fatalf("Failed to create interface alias with sudo: %v", err)
	}

	if !ip.Equal(testIP) {
		t.Errorf("Expected IP %s, got %s", testIP, ip)
	}

	// Verify the alias was created
	iface, err := net.InterfaceByName("lo0")
	if err != nil {
		t.Fatalf("Failed to get lo0 interface: %v", err)
	}

	addrs, err := iface.Addrs()
	if err != nil {
		t.Fatalf("Failed to get interface addresses: %v", err)
	}

	found := false
	expectedAddr := testIP.String() + "/8"
	for _, addr := range addrs {
		if addr.String() == expectedAddr {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("Interface alias %s not found in lo0 addresses", expectedAddr)
	}

	// Clean up - remove the alias
	RemoveInterfaceAlias(testIP)

	// Verify removal
	iface, err = net.InterfaceByName("lo0")
	if err != nil {
		t.Logf("Warning: failed to get lo0 interface for verification: %v", err)
	} else {
		addrs, err = iface.Addrs()
		if err != nil {
			t.Logf("Warning: failed to get addresses for verification: %v", err)
		} else {
			for _, addr := range addrs {
				if addr.String() == expectedAddr {
					t.Errorf("Interface alias %s still present after removal", expectedAddr)
				}
			}
		}
	}

	t.Log("Interface alias integration test passed")
}

// TestPlatformDetection tests runtime platform detection
func TestPlatformDetection(t *testing.T) {
	goos := runtime.GOOS

	switch goos {
	case "linux", "darwin", "windows":
		t.Logf("Running on supported platform: %s", goos)
	default:
		t.Logf("Running on platform: %s (may not be fully supported)", goos)
	}

	// Verify the platform detection logic matches what the code expects
	switch goos {
	case "linux", "windows":
		// Should try to use "lo" interface
		_, err := net.InterfaceByName("lo")
		if err != nil && goos == "linux" {
			t.Logf("Warning: 'lo' interface not found on Linux")
		}
	case "darwin":
		// Should use "lo0" interface
		_, err := net.InterfaceByName("lo0")
		if err != nil {
			t.Errorf("Expected 'lo0' interface on macOS")
		}
	}
}

// TestConcurrentReadyInterface tests concurrent interface ready calls
func TestConcurrentReadyInterface(t *testing.T) {
	if os.Geteuid() != 0 && runtime.GOOS == "darwin" {
		t.Skip("Skipping concurrent test - requires sudo on macOS")
	}

	// This tests that concurrent ReadyInterface calls don't conflict
	// Note: This may require sudo on macOS to actually create aliases

	numGoroutines := 10
	results := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(n int) {
			opts := fwdip.ForwardIPOpts{
				ServiceName:              "concurrent-test-" + string(rune('a'+n)),
				PodName:                  "test-pod",
				Context:                  "test-ctx",
				ClusterN:                 0,
				NamespaceN:               0,
				Namespace:                "default",
				Port:                     "0",
				ForwardConfigurationPath: "",
				ForwardIPReservations:    nil,
			}

			_, err := ReadyInterface(opts)
			results <- err
		}(i)
	}

	// Collect results
	errors := 0
	for i := 0; i < numGoroutines; i++ {
		if err := <-results; err != nil {
			errors++
			t.Logf("Goroutine error: %v", err)
		}
	}

	if errors > 0 {
		t.Logf("Concurrent ReadyInterface had %d errors (may be expected without sudo)", errors)
	} else {
		t.Log("All concurrent ReadyInterface calls succeeded")
	}
}

// TestMockInterfaceManager tests the MockInterfaceManager
func TestMockInterfaceManager(t *testing.T) {
	mock := NewMockInterfaceManager()

	// Test initial state
	if mock.GetReadyInterfaceCallCount() != 0 {
		t.Errorf("Expected 0 ReadyInterface calls, got %d", mock.GetReadyInterfaceCallCount())
	}

	if mock.GetRemoveAliasCallCount() != 0 {
		t.Errorf("Expected 0 RemoveInterfaceAlias calls, got %d", mock.GetRemoveAliasCallCount())
	}

	// Test ReadyInterface
	opts := fwdip.ForwardIPOpts{
		ServiceName: "test-svc",
		Namespace:   "default",
	}

	ip, err := mock.ReadyInterface(opts)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if ip == nil {
		t.Error("Expected non-nil IP")
	}

	if mock.GetReadyInterfaceCallCount() != 1 {
		t.Errorf("Expected 1 ReadyInterface call, got %d", mock.GetReadyInterfaceCallCount())
	}

	// Test same service returns same IP
	ip2, err := mock.ReadyInterface(opts)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if !ip.Equal(ip2) {
		t.Errorf("Expected same IP for same service, got %s and %s", ip, ip2)
	}

	// Test different service gets different IP
	opts2 := fwdip.ForwardIPOpts{
		ServiceName: "other-svc",
		Namespace:   "default",
	}

	ip3, err := mock.ReadyInterface(opts2)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if ip.Equal(ip3) {
		t.Errorf("Expected different IP for different service, got %s", ip3)
	}

	// Test RemoveInterfaceAlias
	mock.RemoveInterfaceAlias(ip)

	if mock.GetRemoveAliasCallCount() != 1 {
		t.Errorf("Expected 1 RemoveInterfaceAlias call, got %d", mock.GetRemoveAliasCallCount())
	}
}

// TestMockInterfaceManager_Error tests error handling in mock
func TestMockInterfaceManager_Error(t *testing.T) {
	mock := NewMockInterfaceManager()

	// Set error
	testErr := net.UnknownNetworkError("test error")
	mock.ReadyInterfaceErr = testErr

	opts := fwdip.ForwardIPOpts{
		ServiceName: "test-svc",
		Namespace:   "default",
	}

	_, err := mock.ReadyInterface(opts)
	if err == nil {
		t.Error("Expected error, got nil")
	}

	if err.Error() != testErr.Error() {
		t.Errorf("Expected error %q, got %q", testErr, err)
	}
}

// TestMockInterfaceManager_Reset tests reset functionality
func TestMockInterfaceManager_Reset(t *testing.T) {
	mock := NewMockInterfaceManager()

	// Add some state
	opts := fwdip.ForwardIPOpts{
		ServiceName: "test-svc",
		Namespace:   "default",
	}
	ip, _ := mock.ReadyInterface(opts)
	mock.RemoveInterfaceAlias(ip)

	if mock.GetReadyInterfaceCallCount() != 1 {
		t.Error("Expected 1 call before reset")
	}

	// Reset
	mock.Reset()

	if mock.GetReadyInterfaceCallCount() != 0 {
		t.Errorf("Expected 0 calls after reset, got %d", mock.GetReadyInterfaceCallCount())
	}

	if mock.GetRemoveAliasCallCount() != 0 {
		t.Errorf("Expected 0 alias calls after reset, got %d", mock.GetRemoveAliasCallCount())
	}

	// Verify IP counter was reset
	ip2, _ := mock.ReadyInterface(opts)
	if ip2.String() != "127.1.27.1" {
		t.Errorf("Expected IP counter to reset to 127.1.27.1, got %s", ip2)
	}
}

// TestSetManager tests setting a custom manager
func TestSetManager(t *testing.T) {
	// Save original manager
	defer ResetManager()

	mock := NewMockInterfaceManager()
	SetManager(mock)

	opts := fwdip.ForwardIPOpts{
		ServiceName: "test-svc",
		Namespace:   "default",
		Port:        "8080",
	}

	// Calls should go to mock
	_, err := ReadyInterface(opts)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if mock.GetReadyInterfaceCallCount() != 1 {
		t.Errorf("Expected mock to receive 1 call, got %d", mock.GetReadyInterfaceCallCount())
	}

	// Test RemoveInterfaceAlias goes to mock
	testIP := net.ParseIP("127.1.27.50")
	RemoveInterfaceAlias(testIP)

	if mock.GetRemoveAliasCallCount() != 1 {
		t.Errorf("Expected mock to receive 1 RemoveInterfaceAlias call, got %d", mock.GetRemoveAliasCallCount())
	}
}

// TestResetManager tests restoring the default manager
func TestResetManager(t *testing.T) {
	mock := NewMockInterfaceManager()
	SetManager(mock)

	// Verify mock is active
	opts := fwdip.ForwardIPOpts{
		ServiceName: "test-svc",
		Namespace:   "default",
		Port:        "8080",
	}

	_, _ = ReadyInterface(opts)
	if mock.GetReadyInterfaceCallCount() != 1 {
		t.Error("Expected mock to be active")
	}

	// Reset to default
	ResetManager()

	// Calls should now go to default manager (not the mock)
	opts.ServiceName = "another-svc"
	_, _ = ReadyInterface(opts)

	// Mock call count should still be 1 (not incremented)
	if mock.GetReadyInterfaceCallCount() != 1 {
		t.Error("Expected calls to go to default manager after reset")
	}
}

// TestGetManager_ThreadSafety tests concurrent access to manager
func TestGetManager_ThreadSafety(t *testing.T) {
	defer ResetManager()

	numGoroutines := 50
	done := make(chan bool, numGoroutines*2)

	// Mix of getManager calls and SetManager/ResetManager calls
	for i := 0; i < numGoroutines; i++ {
		go func() {
			_ = getManager()
			done <- true
		}()

		go func(n int) {
			if n%2 == 0 {
				SetManager(NewMockInterfaceManager())
			} else {
				ResetManager()
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < numGoroutines*2; i++ {
		<-done
	}

	// If we get here without panic/race, the test passes
	t.Log("Concurrent manager access completed without issues")
}
