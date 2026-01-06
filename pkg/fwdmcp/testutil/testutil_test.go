package testutil

import (
	"testing"

	"github.com/txn2/kubefwd/pkg/fwdtui/state"
)

func TestKFT1Services(t *testing.T) {
	services := KFT1Services()
	if len(services) == 0 {
		t.Error("Expected at least one service in KFT1Services")
	}

	// Check first service has required fields
	svc := services[0]
	if svc.Name == "" {
		t.Error("Expected Name to be set")
	}
	if len(svc.Ports) == 0 {
		t.Error("Expected Ports to be set")
	}
}

func TestKFT2Services(t *testing.T) {
	services := KFT2Services()
	if len(services) == 0 {
		t.Error("Expected at least one service in KFT2Services")
	}

	// Check first service
	svc := services[0]
	if svc.Name == "" {
		t.Error("Expected Name to be set")
	}
}

func TestGenerateServiceSnapshots(t *testing.T) {
	services := KFT1Services()
	snapshots := GenerateServiceSnapshots("test-ns", "test-ctx", services, false)

	// Each service generates one snapshot
	if len(snapshots) != len(services) {
		t.Errorf("Expected %d snapshots, got %d", len(services), len(snapshots))
	}

	// Check first snapshot
	snap := snapshots[0]
	if snap.Namespace != "test-ns" {
		t.Errorf("Expected namespace 'test-ns', got '%s'", snap.Namespace)
	}
	if snap.Context != "test-ctx" {
		t.Errorf("Expected context 'test-ctx', got '%s'", snap.Context)
	}
}

func TestGenerateServiceSnapshots_WithHeadless(t *testing.T) {
	services := KFT1Services()
	snapshots := GenerateServiceSnapshots("test-ns", "test-ctx", services, true)

	// Each service generates two snapshots (regular + headless)
	expected := len(services) * 2
	if len(snapshots) != expected {
		t.Errorf("Expected %d snapshots with headless, got %d", expected, len(snapshots))
	}
}

func TestGenerateForwardSnapshots(t *testing.T) {
	services := []ServiceFixture{
		{Name: "test-svc", Ports: []int{80, 443}, Containers: 2},
	}
	snapshots := GenerateForwardSnapshots("test-ns", "test-ctx", services, false)

	// Each port generates one forward
	if len(snapshots) != 2 {
		t.Errorf("Expected 2 snapshots, got %d", len(snapshots))
	}

	// Check first snapshot
	snap := snapshots[0]
	if snap.ServiceName != "test-svc" {
		t.Errorf("Expected ServiceName 'test-svc', got '%s'", snap.ServiceName)
	}
	if snap.Namespace != "test-ns" {
		t.Errorf("Expected namespace 'test-ns', got '%s'", snap.Namespace)
	}
}

func TestGenerateForwardSnapshots_WithHeadless(t *testing.T) {
	services := []ServiceFixture{
		{Name: "test-svc", Ports: []int{80}, Containers: 1},
	}
	snapshots := GenerateForwardSnapshots("test-ns", "test-ctx", services, true)

	// One regular + one headless
	if len(snapshots) != 2 {
		t.Errorf("Expected 2 snapshots with headless, got %d", len(snapshots))
	}

	// First should be regular
	if snapshots[0].Headless {
		t.Error("First snapshot should not be headless")
	}
	// Second should be headless
	if !snapshots[1].Headless {
		t.Error("Second snapshot should be headless")
	}
}

func TestPopulateStore(t *testing.T) {
	store := state.NewStore(100)
	services := KFT1Services()
	PopulateStore(store, "test-ns", "test-ctx", services, false)

	// Check that forwards were added
	count := store.Count()
	if count == 0 {
		t.Error("Expected store to have forwards after PopulateStore")
	}
}

func TestPopulateStoreKFT1(t *testing.T) {
	store := state.NewStore(100)
	PopulateStoreKFT1(store, "test-ctx")

	count := store.Count()
	if count == 0 {
		t.Error("Expected store to have forwards after PopulateStoreKFT1")
	}
}

func TestPopulateStoreKFT2(t *testing.T) {
	store := state.NewStore(100)
	PopulateStoreKFT2(store, "test-ctx")

	count := store.Count()
	if count == 0 {
		t.Error("Expected store to have forwards after PopulateStoreKFT2")
	}
}

func TestFormatIP(t *testing.T) {
	// Test formatIP with a known IP value
	// 127.1.0.1 = 127*256^3 + 1*256^2 + 0*256 + 1
	ip := 127*256*256*256 + 1*256*256 + 0*256 + 1
	result := formatIP(ip)
	expected := "127.1.0.1"
	if result != expected {
		t.Errorf("Expected IP '%s', got '%s'", expected, result)
	}
}

func TestItoa(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "0"},
		{1, "1"},
		{42, "42"},
		{8080, "8080"},
		{65535, "65535"},
	}

	for _, tc := range tests {
		result := itoa(tc.input)
		if result != tc.expected {
			t.Errorf("itoa(%d) = '%s', expected '%s'", tc.input, result, tc.expected)
		}
	}
}

// Tests for assertions.go

func TestAssertStoreEmpty(t *testing.T) {
	store := state.NewStore(100)
	// Empty store should pass
	AssertStoreEmpty(t, store)
}

func TestAssertStoreForwardCount(t *testing.T) {
	store := state.NewStore(100)
	// Empty store
	AssertStoreForwardCount(t, store, 0)

	// Add some forwards
	PopulateStoreKFT1(store, "test-ctx")
	count := store.Count()
	AssertStoreForwardCount(t, store, count)
}

func TestAssertStoreServiceCount(t *testing.T) {
	store := state.NewStore(100)
	// Empty store
	AssertStoreServiceCount(t, store, 0)

	// Add some services
	PopulateStoreKFT1(store, "test-ctx")
	count := store.ServiceCount()
	AssertStoreServiceCount(t, store, count)
}

func TestAssertNoForwardsForNamespace(t *testing.T) {
	store := state.NewStore(100)
	// Empty store should pass
	AssertNoForwardsForNamespace(t, store, "nonexistent", "ctx")

	// Add kft1 data
	PopulateStoreKFT1(store, "test-ctx")
	// Check for namespace that doesn't exist in kft1
	AssertNoForwardsForNamespace(t, store, "kft2", "test-ctx")
}

func TestAssertNoServicesForNamespace(t *testing.T) {
	store := state.NewStore(100)
	// Empty store should pass
	AssertNoServicesForNamespace(t, store, "nonexistent", "ctx")

	// Add kft1 data
	PopulateStoreKFT1(store, "test-ctx")
	// Check for namespace that doesn't exist
	AssertNoServicesForNamespace(t, store, "kft2", "test-ctx")
}

func TestAssertForwardsExistForNamespace(t *testing.T) {
	store := state.NewStore(100)
	PopulateStoreKFT1(store, "test-ctx")
	// Check kft1 has forwards
	AssertForwardsExistForNamespace(t, store, "kft1", "test-ctx", 1)
}

func TestAssertForwardExists(t *testing.T) {
	store := state.NewStore(100)
	PopulateStoreKFT1(store, "test-ctx")

	// Get a forward key and check it exists
	forwards := store.GetFiltered()
	if len(forwards) > 0 {
		AssertForwardExists(t, store, forwards[0].Key)
	}
}

func TestAssertForwardNotExists(t *testing.T) {
	store := state.NewStore(100)
	AssertForwardNotExists(t, store, "nonexistent-key")
}

func TestAssertServiceExists(t *testing.T) {
	store := state.NewStore(100)
	PopulateStoreKFT1(store, "test-ctx")

	// Get a service key and check it exists
	services := store.GetServices()
	if len(services) > 0 {
		AssertServiceExists(t, store, services[0].Key)
	}
}

func TestAssertServiceNotExists(t *testing.T) {
	store := state.NewStore(100)
	AssertServiceNotExists(t, store, "nonexistent-key")
}

func TestCountForwardsForNamespace(t *testing.T) {
	store := state.NewStore(100)

	// Empty store
	count := CountForwardsForNamespace(store, "kft1", "test-ctx")
	if count != 0 {
		t.Errorf("Expected 0 forwards in empty store, got %d", count)
	}

	// With data
	PopulateStoreKFT1(store, "test-ctx")
	count = CountForwardsForNamespace(store, "kft1", "test-ctx")
	if count == 0 {
		t.Error("Expected forwards for kft1 after population")
	}
}

func TestCountServicesForNamespace(t *testing.T) {
	store := state.NewStore(100)

	// Empty store
	count := CountServicesForNamespace(store, "kft1", "test-ctx")
	if count != 0 {
		t.Errorf("Expected 0 services in empty store, got %d", count)
	}

	// With data
	PopulateStoreKFT1(store, "test-ctx")
	count = CountServicesForNamespace(store, "kft1", "test-ctx")
	if count == 0 {
		t.Error("Expected services for kft1 after population")
	}
}
