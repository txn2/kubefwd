package fwdmcp

import (
	"testing"
	"time"

	"github.com/txn2/kubefwd/pkg/fwdmcp/testutil"
	"github.com/txn2/kubefwd/pkg/fwdtui/events"
	"github.com/txn2/kubefwd/pkg/fwdtui/state"
)

// === P1: State Lifecycle Tests ===
// These tests verify the state store correctly handles namespace/service lifecycle operations.
// They would have caught the namespace removal bug where headless services weren't cleaned up.

// TestIntegration_NamespaceRemoval_CleansAllForwards tests that removing a namespace
// cleans up ALL forwards, including headless service pod-specific entries.
func TestIntegration_NamespaceRemoval_CleansAllForwards(t *testing.T) {
	store := state.NewStore(100)
	bus := events.NewBus(100)

	// Subscribe store to events
	bus.Subscribe(events.NamespaceRemoved, func(e events.Event) {
		store.RemoveByNamespace(e.Namespace, e.Context)
	})

	bus.Start()
	defer bus.Stop()

	// Populate with kft1 and kft2 data
	testutil.PopulateStoreKFT1(store, testutil.ContextFixture)
	testutil.PopulateStoreKFT2(store, testutil.ContextFixture)

	initialCount := store.Count()
	t.Logf("Initial forward count: %d", initialCount)

	kft1Count := testutil.CountForwardsForNamespace(store, "kft1", testutil.ContextFixture)
	kft2Count := testutil.CountForwardsForNamespace(store, "kft2", testutil.ContextFixture)
	t.Logf("kft1 forwards: %d, kft2 forwards: %d", kft1Count, kft2Count)

	// Remove kft2 namespace via event
	bus.Publish(events.NewNamespaceRemovedEvent("kft2", testutil.ContextFixture))

	// Wait for event processing
	time.Sleep(100 * time.Millisecond)

	// Verify ALL kft2 forwards are removed
	testutil.AssertNoForwardsForNamespace(t, store, "kft2", testutil.ContextFixture)
	testutil.AssertNoServicesForNamespace(t, store, "kft2", testutil.ContextFixture)

	// Verify kft1 is unaffected
	testutil.AssertForwardsExistForNamespace(t, store, "kft1", testutil.ContextFixture, kft1Count)

	finalCount := store.Count()
	expectedCount := kft1Count // Only kft1 should remain
	if finalCount != expectedCount {
		t.Errorf("Expected %d forwards after removal, got %d", expectedCount, finalCount)
	}

	t.Logf("Final forward count: %d (removed %d)", finalCount, initialCount-finalCount)
}

// TestIntegration_HeadlessServiceCleanup tests that headless service pod-specific entries
// are properly cleaned up when namespace is removed.
func TestIntegration_HeadlessServiceCleanup(t *testing.T) {
	store := state.NewStore(100)

	// Add headless service with multiple pods
	baseService := "mydb-headless.staging." + testutil.ContextFixture
	pods := []string{"pod-0", "pod-1", "pod-2"}
	ports := []string{"5432", "5433"}

	for _, pod := range pods {
		for _, port := range ports {
			key := baseService + "." + pod + "." + port
			store.AddForward(state.ForwardSnapshot{
				Key:         key,
				ServiceKey:  baseService,
				ServiceName: pod + ".mydb-headless",
				Namespace:   "staging",
				Context:     testutil.ContextFixture,
				PodName:     pod,
				LocalPort:   port,
				PodPort:     port,
				Status:      state.StatusActive,
				Headless:    true,
			})
		}
	}

	// Also add a regular (non-headless) service for comparison
	store.AddForward(state.ForwardSnapshot{
		Key:         "regular-svc.staging." + testutil.ContextFixture + ".pod-x.80",
		ServiceKey:  "regular-svc.staging." + testutil.ContextFixture,
		ServiceName: "regular-svc",
		Namespace:   "staging",
		Context:     testutil.ContextFixture,
		PodName:     "pod-x",
		LocalPort:   "80",
		PodPort:     "80",
		Status:      state.StatusActive,
		Headless:    false,
	})

	expectedHeadless := len(pods) * len(ports) // 3 pods * 2 ports = 6
	expectedRegular := 1
	expectedTotal := expectedHeadless + expectedRegular

	if store.Count() != expectedTotal {
		t.Errorf("Expected %d forwards, got %d", expectedTotal, store.Count())
	}
	t.Logf("Created %d forwards (%d headless, %d regular)", store.Count(), expectedHeadless, expectedRegular)

	// Remove namespace
	removed := store.RemoveByNamespace("staging", testutil.ContextFixture)
	t.Logf("Removed %d forwards", removed)

	// ALL forwards should be removed
	if store.Count() != 0 {
		t.Errorf("Expected 0 forwards after namespace removal, got %d", store.Count())
		forwards := store.GetFiltered()
		for _, fwd := range forwards {
			t.Errorf("  Orphaned: %s (headless=%v)", fwd.Key, fwd.Headless)
		}
	}

	// Verify the remove count matches
	if removed != expectedTotal {
		t.Errorf("RemoveByNamespace returned %d, expected %d", removed, expectedTotal)
	}
}

// TestIntegration_PartialServiceRemoval tests removing individual services doesn't affect others.
func TestIntegration_PartialServiceRemoval(t *testing.T) {
	store := state.NewStore(100)

	// Add two services
	svc1Key := "svc1.ns1." + testutil.ContextFixture
	svc2Key := "svc2.ns1." + testutil.ContextFixture

	store.AddForward(state.ForwardSnapshot{
		Key:         svc1Key + ".pod-a.80",
		ServiceKey:  svc1Key,
		ServiceName: "svc1",
		Namespace:   "ns1",
		Context:     testutil.ContextFixture,
		PodName:     "pod-a",
		LocalPort:   "80",
		Status:      state.StatusActive,
	})

	store.AddForward(state.ForwardSnapshot{
		Key:         svc2Key + ".pod-b.80",
		ServiceKey:  svc2Key,
		ServiceName: "svc2",
		Namespace:   "ns1",
		Context:     testutil.ContextFixture,
		PodName:     "pod-b",
		LocalPort:   "80",
		Status:      state.StatusActive,
	})

	if store.Count() != 2 {
		t.Fatalf("Expected 2 forwards, got %d", store.Count())
	}

	// Remove only svc1
	store.RemoveForward(svc1Key + ".pod-a.80")

	// svc2 should still exist
	if store.Count() != 1 {
		t.Errorf("Expected 1 forward after partial removal, got %d", store.Count())
	}

	testutil.AssertForwardNotExists(t, store, svc1Key+".pod-a.80")
	testutil.AssertForwardExists(t, store, svc2Key+".pod-b.80")
}

// TestIntegration_ServiceStatusUpdates tests status updates propagate correctly.
func TestIntegration_ServiceStatusUpdates(t *testing.T) {
	store := state.NewStore(100)

	key := "svc.ns." + testutil.ContextFixture + ".pod.80"
	serviceKey := "svc.ns." + testutil.ContextFixture

	store.AddForward(state.ForwardSnapshot{
		Key:         key,
		ServiceKey:  serviceKey,
		ServiceName: "svc",
		Namespace:   "ns",
		Context:     testutil.ContextFixture,
		PodName:     "pod",
		LocalPort:   "80",
		Status:      state.StatusActive,
	})

	// Verify initial status
	fwd := store.GetForward(key)
	if fwd.Status != state.StatusActive {
		t.Errorf("Expected status %d, got %d", state.StatusActive, fwd.Status)
	}

	// Update to error status
	store.UpdateStatus(key, state.StatusError, "connection refused")

	fwd = store.GetForward(key)
	if fwd.Status != state.StatusError {
		t.Errorf("Expected status %d after update, got %d", state.StatusError, fwd.Status)
	}
	if fwd.Error != "connection refused" {
		t.Errorf("Expected error message 'connection refused', got '%s'", fwd.Error)
	}

	// Verify service aggregate reflects error
	svc := store.GetService(serviceKey)
	if svc.ErrorCount != 1 {
		t.Errorf("Expected service error count 1, got %d", svc.ErrorCount)
	}
}

// TestIntegration_MetricsAggregation tests that metrics are properly aggregated.
func TestIntegration_MetricsAggregation(t *testing.T) {
	store := state.NewStore(100)

	serviceKey := "svc.ns." + testutil.ContextFixture

	// Add two forwards for the same service
	store.AddForward(state.ForwardSnapshot{
		Key:         serviceKey + ".pod1.80",
		ServiceKey:  serviceKey,
		ServiceName: "svc",
		Namespace:   "ns",
		Context:     testutil.ContextFixture,
		PodName:     "pod1",
		LocalPort:   "80",
		Status:      state.StatusActive,
	})

	store.AddForward(state.ForwardSnapshot{
		Key:         serviceKey + ".pod2.80",
		ServiceKey:  serviceKey,
		ServiceName: "svc",
		Namespace:   "ns",
		Context:     testutil.ContextFixture,
		PodName:     "pod2",
		LocalPort:   "80",
		Status:      state.StatusActive,
	})

	// Update metrics for both
	store.UpdateMetrics(serviceKey+".pod1.80", 1000, 500, 100, 50, 80, 40)
	store.UpdateMetrics(serviceKey+".pod2.80", 2000, 1000, 200, 100, 160, 80)

	// Verify aggregation
	svc := store.GetService(serviceKey)
	if svc == nil {
		t.Fatal("Service not found")
	}

	expectedBytesIn := uint64(3000)  // 1000 + 2000
	expectedBytesOut := uint64(1500) // 500 + 1000

	if svc.TotalBytesIn != expectedBytesIn {
		t.Errorf("Expected TotalBytesIn %d, got %d", expectedBytesIn, svc.TotalBytesIn)
	}
	if svc.TotalBytesOut != expectedBytesOut {
		t.Errorf("Expected TotalBytesOut %d, got %d", expectedBytesOut, svc.TotalBytesOut)
	}

	// Verify summary
	summary := store.GetSummary()
	if summary.TotalBytesIn != expectedBytesIn {
		t.Errorf("Summary TotalBytesIn expected %d, got %d", expectedBytesIn, summary.TotalBytesIn)
	}
}

// TestIntegration_FilterAndSort tests filtering and sorting functionality.
func TestIntegration_FilterAndSort(t *testing.T) {
	store := state.NewStore(100)

	// Add services from different namespaces
	testutil.PopulateStore(store, "production", testutil.ContextFixture,
		[]testutil.ServiceFixture{{Name: "api", Ports: []int{80}}}, false)
	testutil.PopulateStore(store, "staging", testutil.ContextFixture,
		[]testutil.ServiceFixture{{Name: "api", Ports: []int{80}}}, false)
	testutil.PopulateStore(store, "development", testutil.ContextFixture,
		[]testutil.ServiceFixture{{Name: "api", Ports: []int{80}}}, false)

	// Test filter
	store.SetFilter("production")
	filtered := store.GetFiltered()

	for _, fwd := range filtered {
		if fwd.Namespace != "production" {
			t.Errorf("Filter 'production' returned non-production service: %s", fwd.Namespace)
		}
	}

	// Clear filter
	store.SetFilter("")
	all := store.GetFiltered()
	if len(all) != 3 {
		t.Errorf("Expected 3 forwards without filter, got %d", len(all))
	}
}

// TestIntegration_ConcurrentAccess tests thread-safety of store operations.
func TestIntegration_ConcurrentAccess(t *testing.T) {
	store := state.NewStore(100)

	// Populate initial data
	testutil.PopulateStoreKFT1(store, testutil.ContextFixture)

	done := make(chan bool)

	// Concurrent readers
	for i := 0; i < 5; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_ = store.GetFiltered()
				_ = store.GetServices()
				_ = store.GetSummary()
			}
			done <- true
		}()
	}

	// Concurrent writers
	for i := 0; i < 3; i++ {
		go func(id int) {
			for j := 0; j < 50; j++ {
				key := "concurrent-" + testutil.ContextFixture + "." + itoa(id) + "." + itoa(j)
				store.AddForward(state.ForwardSnapshot{
					Key:         key,
					ServiceKey:  "concurrent." + testutil.ContextFixture,
					ServiceName: "concurrent",
					Namespace:   "concurrent",
					Context:     testutil.ContextFixture,
					PodName:     "pod",
					LocalPort:   itoa(j),
					Status:      state.StatusActive,
				})
				store.UpdateMetrics(key, uint64(j), uint64(j), 0, 0, 0, 0)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 8; i++ {
		<-done
	}

	// Verify store is still consistent
	if store.Count() == 0 {
		t.Error("Store should not be empty after concurrent access")
	}
}

// itoa converts int to string.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	result := ""
	for i > 0 {
		result = string(rune('0'+i%10)) + result
		i /= 10
	}
	return result
}

// TestIntegration_LogBuffer tests log buffer functionality.
func TestIntegration_LogBuffer(t *testing.T) {
	store := state.NewStore(10) // Small buffer size

	// Add more logs than buffer size
	for i := 0; i < 20; i++ {
		store.AddLog(state.LogEntry{
			Level:   "info",
			Message: "Log message " + itoa(i),
		})
	}

	// Should only have last 10
	logs := store.GetLogs(100)
	if len(logs) > 10 {
		t.Errorf("Expected max 10 logs, got %d", len(logs))
	}

	// Last log should be message 19
	if len(logs) > 0 {
		lastMsg := logs[len(logs)-1].Message
		if lastMsg != "Log message 19" {
			t.Errorf("Expected last message 'Log message 19', got '%s'", lastMsg)
		}
	}
}
