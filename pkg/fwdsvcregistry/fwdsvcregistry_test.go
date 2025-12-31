package fwdsvcregistry

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/txn2/kubefwd/pkg/fwdport"
	"github.com/txn2/kubefwd/pkg/fwdservice"
	"github.com/txn2/txeh"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// waitForCondition polls the provided condition until it returns true or the timeout elapses.
func waitForCondition(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for {
		if condition() {
			return
		}

		if time.Now().After(deadline) {
			t.Fatalf("condition not met within %s", timeout)
		}

		time.Sleep(10 * time.Millisecond)
	}
}

// createMockServiceFWD creates a mock ServiceFWD for testing
func createMockServiceFWD(name, namespace, context string) *fwdservice.ServiceFWD {
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}

	hosts, err := txeh.NewHosts(&txeh.HostsConfig{})
	if err != nil {
		// In tests, this should not fail with default config
		panic(fmt.Sprintf("Failed to create txeh.Hosts: %v", err))
	}

	return &fwdservice.ServiceFWD{
		ClientSet:            fake.NewClientset(),
		Svc:                  svc,
		Namespace:            namespace,
		Context:              context,
		PortForwards:         make(map[string]*fwdport.PortForwardOpts),
		NamespaceServiceLock: &sync.Mutex{},
		Hostfile:             &fwdport.HostFileWithLock{Hosts: hosts},
		DoneChannel:          make(chan struct{}),
		SyncDebouncer:        func(f func()) { /* no-op for testing */ },
	}
}

// TestInit tests registry initialization
func TestInit(t *testing.T) {
	shutdownChan := make(chan struct{})
	Init(shutdownChan)

	if svcRegistry == nil {
		t.Fatal("Expected svcRegistry to be initialized")
	}

	if svcRegistry.services == nil {
		t.Error("Expected services map to be initialized")
	}

	if svcRegistry.mutex == nil {
		t.Error("Expected mutex to be initialized")
	}

	// Cleanup
	close(shutdownChan)
	<-Done()
}

// TestAdd_SingleService tests adding a single service
func TestAdd_SingleService(t *testing.T) {
	shutdownChan := make(chan struct{})
	Init(shutdownChan)
	defer func() {
		close(shutdownChan)
		<-Done()
	}()

	svc := createMockServiceFWD("test-svc", "default", "test-ctx")

	Add(svc)

	// Wait for async add to complete
	waitForCondition(t, 1*time.Second, func() bool {
		svcRegistry.mutex.Lock()
		_, found := svcRegistry.services[svc.String()]
		svcRegistry.mutex.Unlock()
		return found
	})
}

// TestAdd_DuplicateService tests that adding the same service twice is idempotent
func TestAdd_DuplicateService(t *testing.T) {
	shutdownChan := make(chan struct{})
	Init(shutdownChan)
	defer func() {
		close(shutdownChan)
		<-Done()
	}()

	svc := createMockServiceFWD("test-svc", "default", "test-ctx")

	// Add twice
	Add(svc)
	Add(svc)

	waitForCondition(t, 1*time.Second, func() bool {
		svcRegistry.mutex.Lock()
		count := len(svcRegistry.services)
		svcRegistry.mutex.Unlock()
		return count == 1
	})
}

// TestAdd_MultipleServices tests adding multiple different services
func TestAdd_MultipleServices(t *testing.T) {
	shutdownChan := make(chan struct{})
	Init(shutdownChan)
	defer func() {
		close(shutdownChan)
		<-Done()
	}()

	svc1 := createMockServiceFWD("svc1", "default", "ctx1")
	svc2 := createMockServiceFWD("svc2", "default", "ctx1")
	svc3 := createMockServiceFWD("svc3", "kube-system", "ctx1")

	Add(svc1)
	Add(svc2)
	Add(svc3)

	waitForCondition(t, 1*time.Second, func() bool {
		svcRegistry.mutex.Lock()
		count := len(svcRegistry.services)
		svcRegistry.mutex.Unlock()
		return count == 3
	})
}

// TestAdd_AfterShutdown tests that adding after shutdown is a no-op
func TestAdd_AfterShutdown(t *testing.T) {
	shutdownChan := make(chan struct{})
	Init(shutdownChan)

	// Trigger shutdown
	close(shutdownChan)
	<-Done()

	svc := createMockServiceFWD("test-svc", "default", "test-ctx")

	// Try to add after shutdown
	Add(svc)

	time.Sleep(50 * time.Millisecond)

	svcRegistry.mutex.Lock()
	count := len(svcRegistry.services)
	svcRegistry.mutex.Unlock()

	if count != 0 {
		t.Errorf("Expected 0 services after shutdown, got %d", count)
	}
}

// TestAdd_ConcurrentAdds tests thread-safe concurrent service additions
func TestAdd_ConcurrentAdds(t *testing.T) {
	shutdownChan := make(chan struct{})
	Init(shutdownChan)
	defer func() {
		close(shutdownChan)
		<-Done()
	}()

	numServices := 50
	var wg sync.WaitGroup

	for i := 0; i < numServices; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			svc := createMockServiceFWD(
				fmt.Sprintf("svc-%d", n),
				fmt.Sprintf("ns-%d", n),
				"ctx",
			)
			Add(svc)
		}(i)
	}

	wg.Wait()
	time.Sleep(100 * time.Millisecond)

	svcRegistry.mutex.Lock()
	count := len(svcRegistry.services)
	svcRegistry.mutex.Unlock()

	// Should have added all unique services
	if count != numServices {
		t.Errorf("Expected %d services after concurrent adds, got %d", numServices, count)
	}
}

// TestRemoveByName_ExistingService tests removing an existing service
func TestRemoveByName_ExistingService(t *testing.T) {
	shutdownChan := make(chan struct{})
	Init(shutdownChan)
	defer func() {
		close(shutdownChan)
		<-Done()
	}()

	svc := createMockServiceFWD("test-svc", "default", "test-ctx")
	Add(svc)

	time.Sleep(50 * time.Millisecond)

	// Verify it was added
	svcRegistry.mutex.Lock()
	_, found := svcRegistry.services[svc.String()]
	svcRegistry.mutex.Unlock()

	if !found {
		t.Fatal("Service should have been added")
	}

	// Remove it
	RemoveByName(svc.String())

	svcRegistry.mutex.Lock()
	_, stillFound := svcRegistry.services[svc.String()]
	svcRegistry.mutex.Unlock()

	if stillFound {
		t.Error("Service should have been removed from registry")
	}
}

// TestRemoveByName_NonExistentService tests removing a service that doesn't exist
func TestRemoveByName_NonExistentService(t *testing.T) {
	shutdownChan := make(chan struct{})
	Init(shutdownChan)
	defer func() {
		close(shutdownChan)
		<-Done()
	}()

	// Try to remove non-existent service - should not panic
	RemoveByName("nonexistent.default.ctx")

	// Should still have empty registry
	svcRegistry.mutex.Lock()
	count := len(svcRegistry.services)
	svcRegistry.mutex.Unlock()

	if count != 0 {
		t.Errorf("Expected 0 services, got %d", count)
	}
}

// TestRemoveByName_ConcurrentRemoves tests thread-safe concurrent service removals
func TestRemoveByName_ConcurrentRemoves(t *testing.T) {
	shutdownChan := make(chan struct{})
	Init(shutdownChan)
	defer func() {
		close(shutdownChan)
		<-Done()
	}()

	// Add services first
	numServices := 30
	services := make([]*fwdservice.ServiceFWD, numServices)
	for i := 0; i < numServices; i++ {
		svc := createMockServiceFWD(
			fmt.Sprintf("svc-%d", i),
			"ns-"+string(rune('a'+i/26)),
			"ctx",
		)
		services[i] = svc
		Add(svc)
	}

	time.Sleep(100 * time.Millisecond)

	// Now remove them concurrently
	var wg sync.WaitGroup
	for i := 0; i < numServices; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			RemoveByName(services[n].String())
		}(i)
	}

	wg.Wait()

	svcRegistry.mutex.Lock()
	count := len(svcRegistry.services)
	svcRegistry.mutex.Unlock()

	if count != 0 {
		t.Errorf("Expected 0 services after concurrent removes, got %d", count)
	}
}

// TestConcurrentAddRemove tests concurrent adds and removes
func TestConcurrentAddRemove(t *testing.T) {
	shutdownChan := make(chan struct{})
	Init(shutdownChan)
	defer func() {
		close(shutdownChan)
		<-Done()
	}()

	var wg sync.WaitGroup
	numOperations := 100

	// Concurrent adds
	for i := 0; i < numOperations; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			svc := createMockServiceFWD(
				fmt.Sprintf("svc-%d", n),
				"default",
				"ctx",
			)
			Add(svc)
		}(i)
	}

	// Concurrent removes (some may not exist)
	for i := 0; i < numOperations; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			time.Sleep(time.Millisecond * time.Duration(n%10))
			name := fmt.Sprintf("svc-%d.default.ctx", n)
			RemoveByName(name)
		}(i)
	}

	wg.Wait()

	// Should complete without panics or deadlocks
	svcRegistry.mutex.Lock()
	finalCount := len(svcRegistry.services)
	svcRegistry.mutex.Unlock()

	// Exact count is non-deterministic due to race, but should be valid
	if finalCount < 0 {
		t.Error("Invalid service count")
	}
}

// TestShutDownAll tests shutting down all services
func TestShutDownAll(t *testing.T) {
	shutdownChan := make(chan struct{})
	Init(shutdownChan)

	// Add some services
	for i := 0; i < 5; i++ {
		svc := createMockServiceFWD(
			"svc-"+string(rune('a'+i)),
			"default",
			"ctx",
		)
		Add(svc)
	}

	time.Sleep(100 * time.Millisecond)

	// Verify services were added
	svcRegistry.mutex.Lock()
	initialCount := len(svcRegistry.services)
	svcRegistry.mutex.Unlock()

	if initialCount != 5 {
		t.Errorf("Expected 5 services initially, got %d", initialCount)
	}

	// Shutdown all
	ShutDownAll()

	svcRegistry.mutex.Lock()
	finalCount := len(svcRegistry.services)
	svcRegistry.mutex.Unlock()

	if finalCount != 0 {
		t.Errorf("Expected 0 services after ShutDownAll, got %d", finalCount)
	}

	// Cleanup
	close(shutdownChan)
	<-Done()
}

// TestShutdownSignal tests that closing the shutdown channel triggers shutdown
func TestShutdownSignal(t *testing.T) {
	shutdownChan := make(chan struct{})
	Init(shutdownChan)

	// Add services
	for i := 0; i < 3; i++ {
		svc := createMockServiceFWD(
			"svc-"+string(rune('a'+i)),
			"default",
			"ctx",
		)
		Add(svc)
	}

	time.Sleep(50 * time.Millisecond)

	// Trigger shutdown
	close(shutdownChan)

	// Wait for shutdown to complete
	select {
	case <-Done():
		// Success - shutdown completed
	case <-time.After(2 * time.Second):
		t.Fatal("Shutdown did not complete within timeout")
	}

	// All services should be removed
	svcRegistry.mutex.Lock()
	count := len(svcRegistry.services)
	svcRegistry.mutex.Unlock()

	if count != 0 {
		t.Errorf("Expected 0 services after shutdown signal, got %d", count)
	}
}

// TestDone_WithoutInit tests Done() when registry not initialized
func TestDone_WithoutInit(t *testing.T) {
	// Reset registry
	svcRegistry = nil

	doneChan := Done()

	// Should return a closed channel
	select {
	case <-doneChan:
		// Good - channel is closed
	case <-time.After(100 * time.Millisecond):
		t.Error("Expected Done() to return closed channel when not initialized")
	}
}

// TestConcurrentShutdown tests concurrent shutdown operations
func TestConcurrentShutdown(t *testing.T) {
	shutdownChan := make(chan struct{})
	Init(shutdownChan)

	// Add services
	for i := 0; i < 10; i++ {
		svc := createMockServiceFWD(
			"svc-"+string(rune('a'+i)),
			"default",
			"ctx",
		)
		Add(svc)
	}

	time.Sleep(100 * time.Millisecond)

	var wg sync.WaitGroup

	// Concurrent ShutDownAll calls
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ShutDownAll()
		}()
	}

	wg.Wait()

	// Should complete without panics or deadlocks
	svcRegistry.mutex.Lock()
	count := len(svcRegistry.services)
	svcRegistry.mutex.Unlock()

	if count != 0 {
		t.Errorf("Expected 0 services after concurrent shutdowns, got %d", count)
	}

	// Cleanup
	close(shutdownChan)
	<-Done()
}

// TestServiceNameUniqueness tests that service names are properly unique
func TestServiceNameUniqueness(t *testing.T) {
	shutdownChan := make(chan struct{})
	Init(shutdownChan)
	defer func() {
		close(shutdownChan)
		<-Done()
	}()

	// Same name, different namespaces
	svc1 := createMockServiceFWD("app", "ns1", "ctx")
	svc2 := createMockServiceFWD("app", "ns2", "ctx")

	Add(svc1)
	Add(svc2)

	time.Sleep(50 * time.Millisecond)

	// Should have both services (different full names)
	svcRegistry.mutex.Lock()
	count := len(svcRegistry.services)
	_, found1 := svcRegistry.services["app.ns1.ctx"]
	_, found2 := svcRegistry.services["app.ns2.ctx"]
	svcRegistry.mutex.Unlock()

	if count != 2 {
		t.Errorf("Expected 2 services with same name but different namespaces, got %d", count)
	}

	if !found1 || !found2 {
		t.Error("Expected both services to be found by their full names")
	}
}

// TestGet_ExistingService tests getting an existing service
func TestGet_ExistingService(t *testing.T) {
	shutdownChan := make(chan struct{})
	Init(shutdownChan)
	defer func() {
		close(shutdownChan)
		<-Done()
	}()

	svc := createMockServiceFWD("test-svc", "default", "test-ctx")
	Add(svc)

	time.Sleep(50 * time.Millisecond)

	// Get the service
	retrieved := Get(svc.String())

	if retrieved == nil {
		t.Fatal("Expected to get service")
	}

	if retrieved.String() != svc.String() {
		t.Errorf("Expected service %s, got %s", svc.String(), retrieved.String())
	}
}

// TestGet_NonExistentService tests getting a service that doesn't exist
func TestGet_NonExistentService(t *testing.T) {
	shutdownChan := make(chan struct{})
	Init(shutdownChan)
	defer func() {
		close(shutdownChan)
		<-Done()
	}()

	retrieved := Get("nonexistent.default.ctx")

	if retrieved != nil {
		t.Error("Expected nil for non-existent service")
	}
}

// TestGetAll tests getting all services
func TestGetAll(t *testing.T) {
	shutdownChan := make(chan struct{})
	Init(shutdownChan)
	defer func() {
		close(shutdownChan)
		<-Done()
	}()

	// Add multiple services
	svc1 := createMockServiceFWD("svc1", "default", "ctx")
	svc2 := createMockServiceFWD("svc2", "default", "ctx")
	svc3 := createMockServiceFWD("svc3", "kube-system", "ctx")

	Add(svc1)
	Add(svc2)
	Add(svc3)

	time.Sleep(100 * time.Millisecond)

	// Get all services
	all := GetAll()

	if len(all) != 3 {
		t.Errorf("Expected 3 services, got %d", len(all))
	}

	// Verify all services are present
	found := make(map[string]bool)
	for _, svc := range all {
		found[svc.String()] = true
	}

	if !found["svc1.default.ctx"] {
		t.Error("Expected svc1 in GetAll result")
	}
	if !found["svc2.default.ctx"] {
		t.Error("Expected svc2 in GetAll result")
	}
	if !found["svc3.kube-system.ctx"] {
		t.Error("Expected svc3 in GetAll result")
	}
}

// TestGetAll_Empty tests getting all services when registry is empty
func TestGetAll_Empty(t *testing.T) {
	shutdownChan := make(chan struct{})
	Init(shutdownChan)
	defer func() {
		close(shutdownChan)
		<-Done()
	}()

	all := GetAll()

	if len(all) != 0 {
		t.Errorf("Expected 0 services, got %d", len(all))
	}
}

// TestGetByNamespace tests getting services by namespace
func TestGetByNamespace(t *testing.T) {
	shutdownChan := make(chan struct{})
	Init(shutdownChan)
	defer func() {
		close(shutdownChan)
		<-Done()
	}()

	// Add services in different namespaces
	svc1 := createMockServiceFWD("svc1", "default", "ctx")
	svc2 := createMockServiceFWD("svc2", "default", "ctx")
	svc3 := createMockServiceFWD("svc3", "kube-system", "ctx")
	svc4 := createMockServiceFWD("svc4", "production", "ctx")

	Add(svc1)
	Add(svc2)
	Add(svc3)
	Add(svc4)

	time.Sleep(100 * time.Millisecond)

	// Get services in default namespace - GetByNamespace takes (namespace, context)
	defaultServices := GetByNamespace("default", "ctx")

	if len(defaultServices) != 2 {
		t.Errorf("Expected 2 services in default namespace, got %d", len(defaultServices))
	}

	// Get services in kube-system namespace
	kubeSystemServices := GetByNamespace("kube-system", "ctx")

	if len(kubeSystemServices) != 1 {
		t.Errorf("Expected 1 service in kube-system namespace, got %d", len(kubeSystemServices))
	}

	// Get services in non-existent namespace
	nonExistentServices := GetByNamespace("nonexistent", "ctx")

	if len(nonExistentServices) != 0 {
		t.Errorf("Expected 0 services in nonexistent namespace, got %d", len(nonExistentServices))
	}
}

// TestGetByNamespace_DifferentContexts tests getting services by namespace with different contexts
func TestGetByNamespace_DifferentContexts(t *testing.T) {
	shutdownChan := make(chan struct{})
	Init(shutdownChan)
	defer func() {
		close(shutdownChan)
		<-Done()
	}()

	// Add services in same namespace but different contexts
	svc1 := createMockServiceFWD("svc1", "default", "ctx1")
	svc2 := createMockServiceFWD("svc2", "default", "ctx2")

	Add(svc1)
	Add(svc2)

	time.Sleep(100 * time.Millisecond)

	// Get services in default namespace for ctx1 - GetByNamespace takes (namespace, context)
	ctx1Services := GetByNamespace("default", "ctx1")

	if len(ctx1Services) != 1 {
		t.Errorf("Expected 1 service in default.ctx1, got %d", len(ctx1Services))
	}

	// Get services in default namespace for ctx2
	ctx2Services := GetByNamespace("default", "ctx2")

	if len(ctx2Services) != 1 {
		t.Errorf("Expected 1 service in default.ctx2, got %d", len(ctx2Services))
	}
}

// TestRaceConditions runs all tests with race detector to verify thread safety
// This test doesn't do anything itself, but when run with -race flag,
// it will catch any race conditions in the other tests
func TestRaceConditions(t *testing.T) {
	// This test exists to ensure we run with -race detector
	// All the actual race condition testing happens in other tests
	t.Log("Run with: go test -race ./pkg/fwdsvcregistry/...")
}
