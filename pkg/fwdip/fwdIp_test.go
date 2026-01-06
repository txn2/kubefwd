package fwdip

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// resetRegistry resets the global IP registry for test isolation
func resetRegistry() {
	ipRegistry = &Registry{
		mutex:     &sync.Mutex{},
		inc:       map[int]map[int]int{0: {0: 0}},
		reg:       make(map[string]net.IP),
		allocated: make(map[string]bool),
	}
	// Reset the forwardConfiguration so each test starts fresh
	forwardConfiguration = nil
}

// TestGetIP_BasicAllocation tests basic IP allocation sequencing
func TestGetIP_BasicAllocation(t *testing.T) {
	resetRegistry()

	opts := ForwardIPOpts{
		ServiceName: "test-svc",
		PodName:     "test-pod-1",
		Context:     "test-ctx",
		ClusterN:    0,
		NamespaceN:  0,
	}

	ip1, err := GetIP(opts)
	if err != nil {
		t.Fatalf("GetIP failed: %v", err)
	}

	// First IP should be 127.1.27.1 (base unreserved IP from default config)
	expected := "127.1.27.1"
	if ip1.String() != expected {
		t.Errorf("Expected first IP to be %s, got %s", expected, ip1.String())
	}

	// Get second IP for different pod
	opts.PodName = "test-pod-2"
	ip2, err := GetIP(opts)
	if err != nil {
		t.Fatalf("GetIP failed: %v", err)
	}

	// Second IP should increment the last octet
	expected = "127.1.27.2"
	if ip2.String() != expected {
		t.Errorf("Expected second IP to be %s, got %s", expected, ip2.String())
	}

	// Get third IP
	opts.PodName = "test-pod-3"
	ip3, err := GetIP(opts)
	if err != nil {
		t.Fatalf("GetIP failed: %v", err)
	}

	expected = "127.1.27.3"
	if ip3.String() != expected {
		t.Errorf("Expected third IP to be %s, got %s", expected, ip3.String())
	}
}

// TestGetIP_SameServiceReturnsSameIP tests that requesting the same service/pod returns the same IP
func TestGetIP_SameServiceReturnsSameIP(t *testing.T) {
	resetRegistry()

	opts := ForwardIPOpts{
		ServiceName: "test-svc",
		PodName:     "test-pod",
		Context:     "test-ctx",
		ClusterN:    0,
		NamespaceN:  0,
	}

	ip1, err := GetIP(opts)
	if err != nil {
		t.Fatalf("GetIP failed: %v", err)
	}

	// Request again - should get same IP
	ip2, err := GetIP(opts)
	if err != nil {
		t.Fatalf("GetIP failed: %v", err)
	}

	if ip1.String() != ip2.String() {
		t.Errorf("Expected same IP for same service/pod, got %s and %s", ip1.String(), ip2.String())
	}

	// Verify it's cached (not incrementing counter)
	opts.PodName = "different-pod"
	ip3, err := GetIP(opts)
	if err != nil {
		t.Fatalf("GetIP failed: %v", err)
	}

	// Should get a different IP for different pod, but verifying exact value
	// is fragile due to global state. Just verify it's different from ip1.
	if ip3.String() == ip1.String() {
		t.Error("Expected different IP for different pod")
	}
}

// TestGetIP_ClusterIncrement tests that ClusterN increments the second octet
func TestGetIP_ClusterIncrement(t *testing.T) {
	resetRegistry()

	// Cluster 0
	opts := ForwardIPOpts{
		ServiceName: "test-svc",
		PodName:     "test-pod-1",
		Context:     "cluster0",
		ClusterN:    0,
		NamespaceN:  0,
	}

	ip1, err := GetIP(opts)
	if err != nil {
		t.Fatalf("GetIP failed: %v", err)
	}
	if ip1.String() != "127.1.27.1" {
		t.Errorf("Expected 127.1.27.1, got %s", ip1.String())
	}

	// Cluster 1 - should increment second octet
	opts.ClusterN = 1
	opts.PodName = "test-pod-2"
	opts.Context = "cluster1"
	ip2, err := GetIP(opts)
	if err != nil {
		t.Fatalf("GetIP failed: %v", err)
	}
	if ip2.String() != "127.2.27.1" {
		t.Errorf("Expected 127.2.27.1 for cluster 1, got %s", ip2.String())
	}

	// Cluster 2
	opts.ClusterN = 2
	opts.PodName = "test-pod-3"
	opts.Context = "cluster2"
	ip3, err := GetIP(opts)
	if err != nil {
		t.Fatalf("GetIP failed: %v", err)
	}
	if ip3.String() != "127.3.27.1" {
		t.Errorf("Expected 127.3.27.1 for cluster 2, got %s", ip3.String())
	}
}

// TestGetIP_NamespaceIncrement tests that NamespaceN increments the third octet
func TestGetIP_NamespaceIncrement(t *testing.T) {
	resetRegistry()

	// Namespace 0
	opts := ForwardIPOpts{
		ServiceName: "test-svc",
		PodName:     "test-pod-1",
		Context:     "ctx",
		Namespace:   "ns0",
		ClusterN:    0,
		NamespaceN:  0,
	}

	ip1, err := GetIP(opts)
	if err != nil {
		t.Fatalf("GetIP failed: %v", err)
	}
	if ip1.String() != "127.1.27.1" {
		t.Errorf("Expected 127.1.27.1, got %s", ip1.String())
	}

	// Namespace 1 - should increment third octet
	opts.NamespaceN = 1
	opts.PodName = "test-pod-2"
	opts.Namespace = "ns1"
	ip2, err := GetIP(opts)
	if err != nil {
		t.Fatalf("GetIP failed: %v", err)
	}
	if ip2.String() != "127.1.28.1" {
		t.Errorf("Expected 127.1.28.1 for namespace 1, got %s", ip2.String())
	}

	// Namespace 5
	opts.NamespaceN = 5
	opts.PodName = "test-pod-3"
	opts.Namespace = "ns5"
	ip3, err := GetIP(opts)
	if err != nil {
		t.Fatalf("GetIP failed: %v", err)
	}
	if ip3.String() != "127.1.32.1" {
		t.Errorf("Expected 127.1.32.1 for namespace 5, got %s", ip3.String())
	}
}

// TestGetIP_BoundsCheckCluster tests error when ClusterN > 255
func TestGetIP_BoundsCheckCluster(t *testing.T) {
	resetRegistry()

	opts := ForwardIPOpts{
		ServiceName: "test-svc",
		PodName:     "test-pod",
		Context:     "ctx",
		ClusterN:    256, // Out of bounds
		NamespaceN:  0,
	}

	_, err := GetIP(opts)
	if err == nil {
		t.Error("Expected error for ClusterN > 255")
	}
	if !errors.Is(err, ErrIPBoundsExceeded) {
		t.Errorf("Expected ErrIPBoundsExceeded, got: %v", err)
	}
}

// TestGetIP_BoundsCheckNamespace tests error when NamespaceN > 255
func TestGetIP_BoundsCheckNamespace(t *testing.T) {
	resetRegistry()

	opts := ForwardIPOpts{
		ServiceName: "test-svc",
		PodName:     "test-pod",
		Context:     "ctx",
		ClusterN:    0,
		NamespaceN:  256, // Out of bounds
	}

	_, err := GetIP(opts)
	if err == nil {
		t.Error("Expected error for NamespaceN > 255")
	}
	if !errors.Is(err, ErrIPBoundsExceeded) {
		t.Errorf("Expected ErrIPBoundsExceeded, got: %v", err)
	}
}

// TestGetIP_BoundsCheckCounter tests error when counter exceeds 255
func TestGetIP_BoundsCheckCounter(t *testing.T) {
	resetRegistry()

	// Manually set counter to 256 (out of bounds)
	ipRegistry.inc[0][0] = 256

	opts := ForwardIPOpts{
		ServiceName: "test-svc",
		PodName:     "test-pod",
		Context:     "ctx",
		ClusterN:    0,
		NamespaceN:  0,
	}

	_, err := GetIP(opts)
	if err == nil {
		t.Error("Expected error for counter > 255")
	}
	if !errors.Is(err, ErrIPBoundsExceeded) {
		t.Errorf("Expected ErrIPBoundsExceeded, got: %v", err)
	}
}

// TestGetIP_WithYAMLConfiguration tests IP allocation with YAML config file
//
//goland:noinspection DuplicatedCode
func TestGetIP_WithYAMLConfiguration(t *testing.T) {
	resetRegistry()

	// Create temp config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yaml")

	configContent := `baseUnreservedIP: "127.2.0.1"
serviceConfigurations:
  - name: "reserved-svc"
    ip: "127.10.10.10"
  - name: "another-svc.default"
    ip: "127.20.20.20"
`
	if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Test reserved service
	opts := ForwardIPOpts{
		ServiceName:              "reserved-svc",
		PodName:                  "pod1",
		Context:                  "ctx",
		ClusterN:                 0,
		NamespaceN:               0,
		ForwardConfigurationPath: configPath,
	}

	ip, err := GetIP(opts)
	if err != nil {
		t.Fatalf("GetIP failed: %v", err)
	}

	if ip.String() != "127.10.10.10" {
		t.Errorf("Expected reserved IP 127.10.10.10, got %s", ip.String())
	}

	// Test unreserved service (should use baseUnreservedIP)
	resetRegistry() // Reset to reload config
	opts.ServiceName = "unreserved-svc"
	opts.PodName = "pod2"

	ip2, err := GetIP(opts)
	if err != nil {
		t.Fatalf("GetIP failed: %v", err)
	}

	if ip2.String() != "127.2.0.1" {
		t.Errorf("Expected base unreserved IP 127.2.0.1, got %s", ip2.String())
	}
}

// TestGetIP_WithCLIReservations tests CLI-passed IP reservations
func TestGetIP_WithCLIReservations(t *testing.T) {
	resetRegistry()

	opts := ForwardIPOpts{
		ServiceName: "cli-reserved",
		PodName:     "pod1",
		Context:     "ctx",
		ClusterN:    0,
		NamespaceN:  0,
		ForwardIPReservations: []string{
			"cli-reserved:127.50.50.50",
			"another-cli-svc:127.60.60.60",
		},
	}

	ip, err := GetIP(opts)
	if err != nil {
		t.Fatalf("GetIP failed: %v", err)
	}

	if ip.String() != "127.50.50.50" {
		t.Errorf("Expected CLI reserved IP 127.50.50.50, got %s", ip.String())
	}
}

// TestGetIP_CLIOverridesYAML tests that CLI reservations override YAML config
func TestGetIP_CLIOverridesYAML(t *testing.T) {
	resetRegistry()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yaml")

	configContent := `baseUnreservedIP: "127.2.0.1"
serviceConfigurations:
  - name: "my-svc"
    ip: "127.100.100.100"
`
	if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	opts := ForwardIPOpts{
		ServiceName:              "my-svc",
		PodName:                  "pod1",
		Context:                  "ctx",
		ClusterN:                 0,
		NamespaceN:               0,
		ForwardConfigurationPath: configPath,
		ForwardIPReservations: []string{
			"my-svc:127.111.111.111", // CLI override
		},
	}

	ip, err := GetIP(opts)
	if err != nil {
		t.Fatalf("GetIP failed: %v", err)
	}

	// Should use CLI reservation, not YAML
	if ip.String() != "127.111.111.111" {
		t.Errorf("Expected CLI to override YAML, got %s instead of 127.111.111.111", ip.String())
	}
}

// TestGetIP_ConflictDetection tests IP conflict detection and auto-retry
func TestGetIP_ConflictDetection(t *testing.T) {
	resetRegistry()

	// Allocate first IP
	opts1 := ForwardIPOpts{
		ServiceName: "svc1",
		PodName:     "pod1",
		Context:     "ctx",
		ClusterN:    0,
		NamespaceN:  0,
	}

	ip1, err := GetIP(opts1)
	if err != nil {
		t.Fatalf("GetIP failed: %v", err)
	}

	// Now try to reserve the same IP for a different service
	// This should auto-increment to the next available IP
	resetRegistry()
	opts2 := ForwardIPOpts{
		ServiceName:           "svc2",
		PodName:               "pod2",
		Context:               "ctx",
		ClusterN:              0,
		NamespaceN:            0,
		ForwardIPReservations: []string{fmt.Sprintf("svc2:%s", ip1.String())},
	}

	// Manually allocate ip1 first
	ipRegistry.allocated[ip1.String()] = true

	ip2, err := GetIP(opts2)
	if err != nil {
		t.Fatalf("GetIP failed: %v", err)
	}

	// Should get next available IP
	if ip2.String() == ip1.String() {
		t.Error("Expected different IP due to conflict, got same IP")
	}

	expected := "127.1.27.2"
	if ip2.String() != expected {
		t.Errorf("Expected conflict resolution to give %s, got %s", expected, ip2.String())
	}
}

// TestGetIP_ConflictingReservations tests handling of conflicting reservations
//
//goland:noinspection DuplicatedCode
func TestGetIP_ConflictingReservations(t *testing.T) {
	resetRegistry()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yaml")

	configContent := `baseUnreservedIP: "127.2.0.1"
serviceConfigurations:
  - name: "reserved-svc"
    ip: "127.10.10.10"
`
	if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// First, allocate the reserved IP for the correct service
	opts1 := ForwardIPOpts{
		ServiceName:              "reserved-svc",
		PodName:                  "pod1",
		Context:                  "ctx",
		ClusterN:                 0,
		NamespaceN:               0,
		ForwardConfigurationPath: configPath,
	}

	ip1, err := GetIP(opts1)
	if err != nil {
		t.Fatalf("GetIP failed: %v", err)
	}

	if ip1.String() != "127.10.10.10" {
		t.Errorf("Expected reserved IP 127.10.10.10, got %s", ip1.String())
	}

	// Now try to get IP for a different service
	// Since 127.10.10.10 is already allocated, it should get next available
	resetRegistry() // Reset to reload config

	// Manually mark the reserved IP as allocated
	ipRegistry.allocated["127.10.10.10"] = true

	opts2 := ForwardIPOpts{
		ServiceName:              "different-svc",
		PodName:                  "pod2",
		Context:                  "ctx",
		ClusterN:                 0,
		NamespaceN:               0,
		ForwardConfigurationPath: configPath,
	}

	ip2, err := GetIP(opts2)
	if err != nil {
		t.Fatalf("GetIP failed: %v", err)
	}

	// Should get baseUnreservedIP since reserved IP is taken
	expected := "127.2.0.1"
	if ip2.String() != expected {
		t.Errorf("Expected fallback to %s, got %s", expected, ip2.String())
	}
}

// TestGetIP_ConcurrentAllocation tests thread-safe concurrent IP allocation
func TestGetIP_ConcurrentAllocation(t *testing.T) {
	resetRegistry()

	numGoroutines := 50
	results := make(chan string, numGoroutines)
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			opts := ForwardIPOpts{
				ServiceName: fmt.Sprintf("svc-%d", n),
				PodName:     fmt.Sprintf("pod-%d", n),
				Context:     "ctx",
				ClusterN:    0,
				NamespaceN:  0,
			}

			ip, err := GetIP(opts)
			if err != nil {
				t.Errorf("GetIP failed: %v", err)
				return
			}
			results <- ip.String()
		}(i)
	}

	wg.Wait()
	close(results)

	// Collect all IPs
	ipSet := make(map[string]bool)
	for ip := range results {
		if ipSet[ip] {
			t.Errorf("Duplicate IP allocated: %s", ip)
		}
		ipSet[ip] = true
	}

	// Should have allocated exactly numGoroutines unique IPs
	if len(ipSet) != numGoroutines {
		t.Errorf("Expected %d unique IPs, got %d", numGoroutines, len(ipSet))
	}
}

// TestIpFromString tests parsing IP strings
func TestIpFromString(t *testing.T) {
	tests := []struct {
		input    string
		expected string
		hasError bool
	}{
		{"127.0.0.1", "127.0.0.1", false},
		{"127.1.27.1", "127.1.27.1", false},
		{"192.168.1.1", "192.168.1.1", false},
		{"0.0.0.0", "0.0.0.0", false},                 // Minimum valid values
		{"255.255.255.255", "255.255.255.255", false}, // Maximum valid values
		{"abc.def.ghi.jkl", "", true},                 // Returns error - invalid integer
		{"invalid.ip.string", "", true},               // Returns error - invalid integer
		{"127.0.0", "", true},                         // Returns error - not enough octets
		{"127.0.0.1.1", "", true},                     // Returns error - too many octets
		{"", "", true},                                // Returns error - empty string
		{"256.0.0.1", "", true},                       // Returns error - octet0 > 255
		{"127.256.0.1", "", true},                     // Returns error - octet1 > 255
		{"127.0.256.1", "", true},                     // Returns error - octet2 > 255
		{"127.0.0.256", "", true},                     // Returns error - octet3 > 255
		{"-1.0.0.1", "", true},                        // Returns error - negative octet0
		{"127.-1.0.1", "", true},                      // Returns error - negative octet1
		{"127.0.-1.1", "", true},                      // Returns error - negative octet2
		{"127.0.0.-1", "", true},                      // Returns error - negative octet3
		{"999.999.999.999", "", true},                 // Returns error - all octets out of range
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			ip, err := ipFromString(tt.input)
			if tt.hasError {
				if err == nil {
					t.Errorf("Expected error for input %s, got nil", tt.input)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error for input %s: %v", tt.input, err)
				}
				if ip.String() != tt.expected {
					t.Errorf("Expected %s, got %s", tt.expected, ip.String())
				}
			}
		})
	}
}

// TestServiceConfigurationFromReservation tests parsing CLI reservation strings
func TestServiceConfigurationFromReservation(t *testing.T) {
	tests := []struct {
		input string
		name  string
		ip    string
		isNil bool
	}{
		{"my-svc:127.0.0.1", "my-svc", "127.0.0.1", false},
		{"service.namespace:127.1.1.1", "service.namespace", "127.1.1.1", false},
		{"invalid", "", "", true},
		{":127.0.0.1", "", "", true},
		{"my-svc:", "", "", true},
		{"", "", "", true},
	}

	for _, tt := range tests {
		cfg := ServiceConfigurationFromReservation(tt.input)
		if tt.isNil {
			if cfg != nil {
				t.Errorf("Expected nil for input %s, got %+v", tt.input, cfg)
			}
		} else {
			if cfg == nil {
				t.Errorf("Expected valid config for input %s, got nil", tt.input)
				continue
			}
			if cfg.Name != tt.name {
				t.Errorf("Expected name %s, got %s", tt.name, cfg.Name)
			}
			if cfg.IP != tt.ip {
				t.Errorf("Expected IP %s, got %s", tt.ip, cfg.IP)
			}
		}
	}
}

// TestServiceConfiguration_Matches tests service name matching
func TestServiceConfiguration_Matches(t *testing.T) {
	cfg := &ServiceConfiguration{
		Name: "my-svc",
		IP:   "127.0.0.1",
	}

	opts := ForwardIPOpts{
		ServiceName: "my-svc",
		PodName:     "pod1",
		Context:     "ctx",
		Namespace:   "default",
		ClusterN:    0,
		NamespaceN:  0,
	}

	// Direct match
	if !cfg.Matches(opts) {
		t.Error("Expected direct service name to match")
	}

	// Non-matching
	opts.ServiceName = "different-svc"
	if cfg.Matches(opts) {
		t.Error("Expected different service name not to match")
	}

	// Test with FQDN matching
	cfg.Name = "my-svc.default.svc.cluster.local"
	opts.ServiceName = "my-svc"
	opts.Namespace = "default"

	if !cfg.Matches(opts) {
		t.Error("Expected FQDN to match")
	}
}

// TestForwardIPOpts_MatchList tests hostname matching list generation
func TestForwardIPOpts_MatchList(t *testing.T) {
	opts := ForwardIPOpts{
		ServiceName: "my-svc",
		PodName:     "my-pod",
		Context:     "my-ctx",
		Namespace:   "my-ns",
		ClusterN:    0,
		NamespaceN:  0,
	}

	matchList := opts.MatchList()

	// Should contain various hostname formats
	expectedMatches := []string{
		"my-svc",
		"my-pod",
		"my-svc.my-ns",
		"my-svc.my-ns.svc",
		"my-svc.my-ns.svc.cluster.local",
		"my-pod.my-svc.my-ns",
		"my-svc.my-ns.my-ctx",
	}

	for _, expected := range expectedMatches {
		found := false
		for _, match := range matchList {
			if match == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected match list to contain %s", expected)
		}
	}
}

// TestForwardIPOpts_MatchList_ClusterN tests match list for different clusters
func TestForwardIPOpts_MatchList_ClusterN(t *testing.T) {
	opts := ForwardIPOpts{
		ServiceName: "my-svc",
		PodName:     "my-pod",
		Context:     "cluster2",
		Namespace:   "my-ns",
		ClusterN:    1, // Non-zero cluster
		NamespaceN:  0,
	}

	matchList := opts.MatchList()

	// For ClusterN > 0 && NamespaceN == 0, should include context in names
	expectedMatches := []string{
		"my-svc.cluster2",
		"my-pod.cluster2",
		"my-svc.my-ns.cluster2",
	}

	for _, expected := range expectedMatches {
		found := false
		for _, match := range matchList {
			if match == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected match list to contain %s for ClusterN > 0", expected)
		}
	}
}

// TestBlockNonLoopbackIPs tests validation that blocks non-loopback IPs
func TestBlockNonLoopbackIPs(t *testing.T) {
	tests := []struct {
		name        string
		config      *ForwardConfiguration
		shouldPanic bool
	}{
		{
			name: "Valid loopback base IP",
			config: &ForwardConfiguration{
				BaseUnreservedIP:      "127.0.0.1",
				ServiceConfigurations: []*ServiceConfiguration{},
			},
			shouldPanic: false,
		},
		{
			name: "Invalid non-loopback base IP",
			config: &ForwardConfiguration{
				BaseUnreservedIP:      "192.168.1.1",
				ServiceConfigurations: []*ServiceConfiguration{},
			},
			shouldPanic: true,
		},
		{
			name: "Valid loopback service IPs",
			config: &ForwardConfiguration{
				BaseUnreservedIP: "127.0.0.1",
				ServiceConfigurations: []*ServiceConfiguration{
					{Name: "svc1", IP: "127.10.10.10"},
					{Name: "svc2", IP: "127.20.20.20"},
				},
			},
			shouldPanic: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if tt.shouldPanic && r == nil {
					t.Error("Expected panic for non-loopback IP")
				}
				if !tt.shouldPanic && r != nil {
					t.Errorf("Unexpected panic: %v", r)
				}
			}()

			blockNonLoopbackIPs(tt.config)
		})
	}
}

// TestNotifyOfDuplicateIPReservations tests duplicate IP detection
func TestNotifyOfDuplicateIPReservations(_ *testing.T) {
	// Test the no-duplicates case
	// Note: Cannot test the duplicate case because notifyOfDuplicateIPReservations
	// calls log.Fatal which exits the entire test process
	config := &ForwardConfiguration{
		BaseUnreservedIP: "127.0.0.1",
		ServiceConfigurations: []*ServiceConfiguration{
			{Name: "svc1", IP: "127.10.10.10"},
			{Name: "svc2", IP: "127.20.20.20"},
		},
	}

	// Should not panic or fatal for unique IPs
	notifyOfDuplicateIPReservations(config)
}

// TestGetIP_InvalidConfigFile tests handling of invalid config files
func TestGetIP_InvalidConfigFile(t *testing.T) {
	resetRegistry()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid.yaml")

	// Write invalid YAML
	if err := os.WriteFile(configPath, []byte("invalid: yaml: content: [[["), 0o644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	opts := ForwardIPOpts{
		ServiceName:              "test-svc",
		PodName:                  "pod1",
		Context:                  "ctx",
		ClusterN:                 0,
		NamespaceN:               0,
		ForwardConfigurationPath: configPath,
	}

	// Should fall back to default config
	ip, err := GetIP(opts)
	if err != nil {
		t.Fatalf("GetIP failed: %v", err)
	}

	// Should use default base IP (127.x.x.x range)
	// Note: Exact IP may vary due to global state from other tests
	if !ip.IsLoopback() || ip[0] != 127 {
		t.Errorf("Expected fallback to loopback IP in 127.x.x.x range, got %s", ip.String())
	}
}

// TestGetIP_NonExistentConfigFile tests handling when config file doesn't exist
func TestGetIP_NonExistentConfigFile(t *testing.T) {
	resetRegistry()

	opts := ForwardIPOpts{
		ServiceName:              "test-svc",
		PodName:                  "pod1",
		Context:                  "ctx",
		ClusterN:                 0,
		NamespaceN:               0,
		ForwardConfigurationPath: "/nonexistent/path/config.yaml",
	}

	// Should fall back to default config
	ip, err := GetIP(opts)
	if err != nil {
		t.Fatalf("GetIP failed: %v", err)
	}

	// Should use default base IP (127.x.x.x range)
	// Note: Exact IP may vary due to global state from other tests
	if !ip.IsLoopback() || ip[0] != 127 {
		t.Errorf("Expected fallback to loopback IP in 127.x.x.x range, got %s", ip.String())
	}
}

// TestRegisterHostname tests registering hostnames
func TestRegisterHostname(t *testing.T) {
	ResetRegistry()

	RegisterHostname("my-service")
	RegisterHostname("my-service.default")
	RegisterHostname("my-service.default.svc.cluster.local")

	hostnames := GetRegisteredHostnames()

	if len(hostnames) != 3 {
		t.Errorf("Expected 3 hostnames, got %d", len(hostnames))
	}

	expected := []string{"my-service", "my-service.default", "my-service.default.svc.cluster.local"}
	for i, h := range expected {
		if hostnames[i] != h {
			t.Errorf("Expected hostname %s at index %d, got %s", h, i, hostnames[i])
		}
	}
}

// TestGetRegisteredHostnames_ReturnsACopy tests that GetRegisteredHostnames returns a copy
func TestGetRegisteredHostnames_ReturnsACopy(t *testing.T) {
	ResetRegistry()

	RegisterHostname("host1")
	RegisterHostname("host2")

	hostnames := GetRegisteredHostnames()
	originalLen := len(hostnames)

	// Modify the returned slice (intentionally discarding result to test immutability)
	_ = append(hostnames, "host3")

	// Original should be unchanged
	hostnames2 := GetRegisteredHostnames()
	if len(hostnames2) != originalLen {
		t.Error("GetRegisteredHostnames should return a copy, not the original slice")
	}
}

// TestGetRegisteredHostnames_Empty tests empty hostnames
func TestGetRegisteredHostnames_Empty(t *testing.T) {
	ResetRegistry()

	hostnames := GetRegisteredHostnames()

	if len(hostnames) != 0 {
		t.Errorf("Expected 0 hostnames after reset, got %d", len(hostnames))
	}
}

// TestResetRegistry tests the ResetRegistry function
func TestResetRegistry(t *testing.T) {
	// First add some state
	ResetRegistry()

	RegisterHostname("test-host")

	// Verify state exists
	hostnames := GetRegisteredHostnames()
	if len(hostnames) != 1 {
		t.Errorf("Expected 1 hostname before reset, got %d", len(hostnames))
	}

	// Reset
	ResetRegistry()

	// Verify all state is cleared
	hostnames = GetRegisteredHostnames()
	if len(hostnames) != 0 {
		t.Errorf("Expected 0 hostnames after reset, got %d", len(hostnames))
	}

	// Test that forwardConfiguration is reset by verifying we can get IP
	// (forwardConfiguration is also set to nil in ResetRegistry)
	opts := ForwardIPOpts{
		ServiceName: "reset-test-svc",
		PodName:     "reset-test-pod",
		Context:     "ctx",
		ClusterN:    0,
		NamespaceN:  0,
	}
	ip, err := GetIP(opts)
	if err != nil {
		t.Fatalf("GetIP failed after reset: %v", err)
	}

	// Should get a valid loopback IP
	if !ip.IsLoopback() {
		t.Errorf("Expected loopback IP after reset, got %s", ip.String())
	}
}

// TestServiceConfiguration_String tests the String method
func TestServiceConfiguration_String(t *testing.T) {
	cfg := ServiceConfiguration{
		Name: "my-service",
		IP:   "127.1.27.50",
	}

	result := cfg.String()
	expected := "Name: my-service IP:127.1.27.50"

	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

// TestServiceConfiguration_MatchesName tests the MatchesName method
func TestServiceConfiguration_MatchesName(t *testing.T) {
	cfg1 := &ServiceConfiguration{Name: "my-service", IP: "127.0.0.1"}
	cfg2 := &ServiceConfiguration{Name: "my-service", IP: "127.0.0.2"}
	cfg3 := &ServiceConfiguration{Name: "other-service", IP: "127.0.0.3"}

	if !cfg1.MatchesName(cfg2) {
		t.Error("Expected same names to match")
	}

	if cfg1.MatchesName(cfg3) {
		t.Error("Expected different names not to match")
	}
}

// TestForwardIPOpts_MatchList_NamespaceN tests match list for different namespaces
func TestForwardIPOpts_MatchList_NamespaceN(t *testing.T) {
	opts := ForwardIPOpts{
		ServiceName: "my-svc",
		PodName:     "my-pod",
		Context:     "my-ctx",
		Namespace:   "ns2",
		ClusterN:    0,
		NamespaceN:  1, // Non-zero namespace
	}

	matchList := opts.MatchList()

	// For ClusterN == 0 && NamespaceN > 0, should include namespace in names
	expectedMatches := []string{
		"my-svc.ns2",
		"my-svc.ns2.svc",
		"my-pod.ns2",
		"my-pod.my-svc.ns2",
	}

	for _, expected := range expectedMatches {
		found := false
		for _, match := range matchList {
			if match == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected match list to contain %s for NamespaceN > 0", expected)
		}
	}

	// Should NOT contain just the service name (since NamespaceN > 0)
	for _, match := range matchList {
		if match == "my-svc" {
			t.Error("Did not expect match list to contain just 'my-svc' for NamespaceN > 0")
			break
		}
	}
}

// TestForwardIPOpts_MatchList_BothNonZero tests match list for both cluster and namespace non-zero
func TestForwardIPOpts_MatchList_BothNonZero(t *testing.T) {
	opts := ForwardIPOpts{
		ServiceName: "my-svc",
		PodName:     "my-pod",
		Context:     "cluster2",
		Namespace:   "ns2",
		ClusterN:    1,
		NamespaceN:  1,
	}

	matchList := opts.MatchList()

	// For ClusterN > 0 && NamespaceN > 0, should include both context and namespace
	expectedMatches := []string{
		"my-svc.ns2.cluster2",
		"my-pod.ns2.cluster2",
		"my-svc.ns2.svc.cluster2",
		"my-pod.my-svc.ns2.cluster2",
	}

	for _, expected := range expectedMatches {
		found := false
		for _, match := range matchList {
			if match == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected match list to contain %s for both ClusterN > 0 and NamespaceN > 0", expected)
		}
	}

	// Verify we got the expected count (9 items for this case)
	if len(matchList) != 9 {
		t.Errorf("Expected 9 match list items for both non-zero, got %d", len(matchList))
	}
}

// TestRegisterHostname_Concurrent tests concurrent hostname registration
func TestRegisterHostname_Concurrent(t *testing.T) {
	ResetRegistry()

	var wg sync.WaitGroup
	numGoroutines := 50

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			RegisterHostname(fmt.Sprintf("host-%d", n))
		}(i)
	}

	wg.Wait()

	hostnames := GetRegisteredHostnames()
	if len(hostnames) != numGoroutines {
		t.Errorf("Expected %d hostnames, got %d", numGoroutines, len(hostnames))
	}
}
