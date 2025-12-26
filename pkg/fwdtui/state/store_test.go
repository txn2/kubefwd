package state

import (
	"sync"
	"testing"
	"time"
)

// TestNewStore tests store creation
func TestNewStore(t *testing.T) {
	store := NewStore(100)

	if store == nil {
		t.Fatal("Expected non-nil store")
	}

	if store.maxLogSize != 100 {
		t.Errorf("Expected maxLogSize 100, got %d", store.maxLogSize)
	}

	if store.Count() != 0 {
		t.Errorf("Expected 0 forwards, got %d", store.Count())
	}

	if store.ServiceCount() != 0 {
		t.Errorf("Expected 0 services, got %d", store.ServiceCount())
	}
}

// TestNewStore_DefaultMaxLogSize tests default max log size
func TestNewStore_DefaultMaxLogSize(t *testing.T) {
	store := NewStore(0) // Should default to 1000

	if store.maxLogSize != 1000 {
		t.Errorf("Expected default maxLogSize 1000, got %d", store.maxLogSize)
	}

	store2 := NewStore(-10) // Negative should also default to 1000

	if store2.maxLogSize != 1000 {
		t.Errorf("Expected default maxLogSize 1000 for negative input, got %d", store2.maxLogSize)
	}
}

// TestAddForward tests adding a forward
func TestAddForward(t *testing.T) {
	store := NewStore(100)

	snapshot := ForwardSnapshot{
		Key:         "svc.ns.ctx.pod.8080",
		ServiceKey:  "svc.ns.ctx",
		ServiceName: "my-service",
		Namespace:   "default",
		Context:     "minikube",
		PodName:     "pod-123",
		LocalPort:   "8080",
		Status:      StatusActive,
	}

	store.AddForward(snapshot)

	if store.Count() != 1 {
		t.Errorf("Expected 1 forward, got %d", store.Count())
	}

	if store.ServiceCount() != 1 {
		t.Errorf("Expected 1 service, got %d", store.ServiceCount())
	}

	// Retrieve and verify
	retrieved := store.GetForward("svc.ns.ctx.pod.8080")
	if retrieved == nil {
		t.Fatal("Expected to retrieve forward")
	}

	if retrieved.ServiceName != "my-service" {
		t.Errorf("Expected service name 'my-service', got %s", retrieved.ServiceName)
	}
}

// TestAddForward_MultiplePodsForSameService tests multiple pods for one service
func TestAddForward_MultiplePodsForSameService(t *testing.T) {
	store := NewStore(100)

	// Add first pod
	store.AddForward(ForwardSnapshot{
		Key:         "svc.ns.ctx.pod1.8080",
		ServiceKey:  "svc.ns.ctx",
		ServiceName: "my-service",
		Namespace:   "default",
		Context:     "minikube",
		PodName:     "pod-1",
		LocalPort:   "8080",
		Status:      StatusActive,
	})

	// Add second pod for same service
	store.AddForward(ForwardSnapshot{
		Key:         "svc.ns.ctx.pod2.8080",
		ServiceKey:  "svc.ns.ctx",
		ServiceName: "my-service",
		Namespace:   "default",
		Context:     "minikube",
		PodName:     "pod-2",
		LocalPort:   "8080",
		Status:      StatusActive,
	})

	if store.Count() != 2 {
		t.Errorf("Expected 2 forwards, got %d", store.Count())
	}

	// Should still be only 1 service
	if store.ServiceCount() != 1 {
		t.Errorf("Expected 1 service, got %d", store.ServiceCount())
	}

	// Service should aggregate both pods
	svc := store.GetService("svc.ns.ctx")
	if svc == nil {
		t.Fatal("Expected to retrieve service")
	}

	if len(svc.PortForwards) != 2 {
		t.Errorf("Expected 2 port forwards in service, got %d", len(svc.PortForwards))
	}

	if svc.ActiveCount != 2 {
		t.Errorf("Expected 2 active pods, got %d", svc.ActiveCount)
	}
}

// TestRemoveForward tests removing a forward
func TestRemoveForward(t *testing.T) {
	store := NewStore(100)

	store.AddForward(ForwardSnapshot{
		Key:         "svc.ns.ctx.pod.8080",
		ServiceKey:  "svc.ns.ctx",
		ServiceName: "my-service",
		Namespace:   "default",
		PodName:     "pod-123",
		Status:      StatusActive,
	})

	if store.Count() != 1 {
		t.Fatal("Expected 1 forward before removal")
	}

	store.RemoveForward("svc.ns.ctx.pod.8080")

	if store.Count() != 0 {
		t.Errorf("Expected 0 forwards after removal, got %d", store.Count())
	}

	// Service should also be removed when no forwards left
	if store.ServiceCount() != 0 {
		t.Errorf("Expected 0 services after removing only forward, got %d", store.ServiceCount())
	}
}

// TestRemoveForward_NonExistent tests removing a non-existent forward
func TestRemoveForward_NonExistent(t *testing.T) {
	store := NewStore(100)

	// Should not panic
	store.RemoveForward("nonexistent.key")

	if store.Count() != 0 {
		t.Errorf("Expected 0 forwards, got %d", store.Count())
	}
}

// TestUpdateMetrics tests updating metrics
func TestUpdateMetrics(t *testing.T) {
	store := NewStore(100)

	store.AddForward(ForwardSnapshot{
		Key:         "svc.ns.ctx.pod.8080",
		ServiceKey:  "svc.ns.ctx",
		ServiceName: "my-service",
		Namespace:   "default",
		PodName:     "pod-123",
		Status:      StatusActive,
	})

	store.UpdateMetrics("svc.ns.ctx.pod.8080", 1000, 2000, 100.5, 200.5, 50.0, 100.0)

	fwd := store.GetForward("svc.ns.ctx.pod.8080")
	if fwd == nil {
		t.Fatal("Expected to retrieve forward")
	}

	if fwd.BytesIn != 1000 {
		t.Errorf("Expected BytesIn 1000, got %d", fwd.BytesIn)
	}

	if fwd.BytesOut != 2000 {
		t.Errorf("Expected BytesOut 2000, got %d", fwd.BytesOut)
	}

	if fwd.RateIn != 100.5 {
		t.Errorf("Expected RateIn 100.5, got %f", fwd.RateIn)
	}

	if fwd.RateOut != 200.5 {
		t.Errorf("Expected RateOut 200.5, got %f", fwd.RateOut)
	}
}

// TestUpdateStatus tests updating status
func TestUpdateStatus(t *testing.T) {
	store := NewStore(100)

	store.AddForward(ForwardSnapshot{
		Key:         "svc.ns.ctx.pod.8080",
		ServiceKey:  "svc.ns.ctx",
		ServiceName: "my-service",
		Namespace:   "default",
		PodName:     "pod-123",
		Status:      StatusConnecting,
	})

	store.UpdateStatus("svc.ns.ctx.pod.8080", StatusError, "connection refused")

	fwd := store.GetForward("svc.ns.ctx.pod.8080")
	if fwd == nil {
		t.Fatal("Expected to retrieve forward")
	}

	if fwd.Status != StatusError {
		t.Errorf("Expected status StatusError, got %v", fwd.Status)
	}

	if fwd.Error != "connection refused" {
		t.Errorf("Expected error 'connection refused', got %s", fwd.Error)
	}
}

// TestSetFilter tests setting and getting filter
func TestSetFilter(t *testing.T) {
	store := NewStore(100)

	store.SetFilter("MyFilter")

	filter := store.GetFilter()
	// Filter is stored lowercase
	if filter != "myfilter" {
		t.Errorf("Expected filter 'myfilter', got %s", filter)
	}
}

// TestGetFiltered_WithFilter tests filtering forwards
func TestGetFiltered_WithFilter(t *testing.T) {
	store := NewStore(100)

	store.AddForward(ForwardSnapshot{
		Key:         "svc1.ns.ctx.pod.8080",
		ServiceKey:  "svc1.ns.ctx",
		ServiceName: "web-server",
		Namespace:   "default",
		PodName:     "pod-1",
	})

	store.AddForward(ForwardSnapshot{
		Key:         "svc2.ns.ctx.pod.8080",
		ServiceKey:  "svc2.ns.ctx",
		ServiceName: "api-server",
		Namespace:   "default",
		PodName:     "pod-2",
	})

	store.AddForward(ForwardSnapshot{
		Key:         "svc3.ns.ctx.pod.8080",
		ServiceKey:  "svc3.ns.ctx",
		ServiceName: "database",
		Namespace:   "production",
		PodName:     "pod-3",
	})

	// Filter by service name
	store.SetFilter("server")
	filtered := store.GetFiltered()

	if len(filtered) != 2 {
		t.Errorf("Expected 2 filtered results for 'server', got %d", len(filtered))
	}

	// Filter by namespace
	store.SetFilter("production")
	filtered = store.GetFiltered()

	if len(filtered) != 1 {
		t.Errorf("Expected 1 filtered result for 'production', got %d", len(filtered))
	}

	// Empty filter returns all
	store.SetFilter("")
	filtered = store.GetFiltered()

	if len(filtered) != 3 {
		t.Errorf("Expected 3 results with empty filter, got %d", len(filtered))
	}
}

// TestGetSummary tests getting summary statistics
func TestGetSummary(t *testing.T) {
	store := NewStore(100)

	store.AddForward(ForwardSnapshot{
		Key:         "svc1.ns.ctx.pod1.8080",
		ServiceKey:  "svc1.ns.ctx",
		ServiceName: "service-1",
		Namespace:   "default",
		Status:      StatusActive,
		BytesIn:     100,
		BytesOut:    200,
		RateIn:      10.0,
		RateOut:     20.0,
	})

	store.AddForward(ForwardSnapshot{
		Key:         "svc2.ns.ctx.pod2.8080",
		ServiceKey:  "svc2.ns.ctx",
		ServiceName: "service-2",
		Namespace:   "default",
		Status:      StatusError,
		BytesIn:     50,
		BytesOut:    100,
	})

	summary := store.GetSummary()

	if summary.TotalServices != 2 {
		t.Errorf("Expected 2 total services, got %d", summary.TotalServices)
	}

	if summary.TotalForwards != 2 {
		t.Errorf("Expected 2 total forwards, got %d", summary.TotalForwards)
	}

	if summary.ActiveForwards != 1 {
		t.Errorf("Expected 1 active forward, got %d", summary.ActiveForwards)
	}

	if summary.ErrorCount != 1 {
		t.Errorf("Expected 1 error, got %d", summary.ErrorCount)
	}

	if summary.TotalBytesIn != 150 {
		t.Errorf("Expected 150 total bytes in, got %d", summary.TotalBytesIn)
	}

	if summary.TotalBytesOut != 300 {
		t.Errorf("Expected 300 total bytes out, got %d", summary.TotalBytesOut)
	}
}

// TestSetSort tests setting sort options
func TestSetSort(t *testing.T) {
	store := NewStore(100)

	store.AddForward(ForwardSnapshot{
		Key:         "svc1.ns.ctx.pod.8080",
		ServiceKey:  "svc1.ns.ctx",
		ServiceName: "zebra",
		Namespace:   "default",
		Hostnames:   []string{"zebra"},
	})

	store.AddForward(ForwardSnapshot{
		Key:         "svc2.ns.ctx.pod.8080",
		ServiceKey:  "svc2.ns.ctx",
		ServiceName: "apple",
		Namespace:   "default",
		Hostnames:   []string{"apple"},
	})

	store.AddForward(ForwardSnapshot{
		Key:         "svc3.ns.ctx.pod.8080",
		ServiceKey:  "svc3.ns.ctx",
		ServiceName: "mango",
		Namespace:   "default",
		Hostnames:   []string{"mango"},
	})

	// Sort ascending by service name
	store.SetSort("service", true)
	filtered := store.GetFiltered()

	if filtered[0].ServiceName != "apple" {
		t.Errorf("Expected first service 'apple', got %s", filtered[0].ServiceName)
	}
	if filtered[2].ServiceName != "zebra" {
		t.Errorf("Expected last service 'zebra', got %s", filtered[2].ServiceName)
	}

	// Sort descending
	store.SetSort("service", false)
	filtered = store.GetFiltered()

	if filtered[0].ServiceName != "zebra" {
		t.Errorf("Expected first service 'zebra' (desc), got %s", filtered[0].ServiceName)
	}
}

// TestAddLog tests adding log entries
func TestAddLog(t *testing.T) {
	store := NewStore(5) // Small buffer for testing

	for i := 0; i < 3; i++ {
		store.AddLog(LogEntry{
			Timestamp: time.Now(),
			Level:     "info",
			Message:   "test message",
		})
	}

	logs := store.GetLogs(10)
	if len(logs) != 3 {
		t.Errorf("Expected 3 logs, got %d", len(logs))
	}
}

// TestAddLog_Overflow tests log buffer overflow
func TestAddLog_Overflow(t *testing.T) {
	store := NewStore(5) // Small buffer

	for i := 0; i < 10; i++ {
		store.AddLog(LogEntry{
			Timestamp: time.Now(),
			Level:     "info",
			Message:   "message",
		})
	}

	logs := store.GetLogs(100)
	if len(logs) != 5 {
		t.Errorf("Expected 5 logs (maxLogSize), got %d", len(logs))
	}
}

// TestGetLogs_Count tests getting limited log count
func TestGetLogs_Count(t *testing.T) {
	store := NewStore(100)

	for i := 0; i < 20; i++ {
		store.AddLog(LogEntry{
			Timestamp: time.Now(),
			Level:     "info",
			Message:   "message",
		})
	}

	logs := store.GetLogs(5)
	if len(logs) != 5 {
		t.Errorf("Expected 5 logs, got %d", len(logs))
	}

	logs = store.GetLogs(0)
	if len(logs) != 20 {
		t.Errorf("Expected 20 logs with count=0, got %d", len(logs))
	}

	logs = store.GetLogs(-1)
	if len(logs) != 20 {
		t.Errorf("Expected 20 logs with count=-1, got %d", len(logs))
	}
}

// TestUpdateHostnames tests updating hostnames
func TestUpdateHostnames(t *testing.T) {
	store := NewStore(100)

	store.AddForward(ForwardSnapshot{
		Key:         "svc.ns.ctx.pod.8080",
		ServiceKey:  "svc.ns.ctx",
		ServiceName: "my-service",
		Namespace:   "default",
		PodName:     "pod-123",
		Hostnames:   []string{},
	})

	store.UpdateHostnames("svc.ns.ctx.pod.8080", []string{"my-service", "my-service.default"})

	fwd := store.GetForward("svc.ns.ctx.pod.8080")
	if fwd == nil {
		t.Fatal("Expected to retrieve forward")
	}

	if len(fwd.Hostnames) != 2 {
		t.Errorf("Expected 2 hostnames, got %d", len(fwd.Hostnames))
	}

	if fwd.Hostnames[0] != "my-service" {
		t.Errorf("Expected first hostname 'my-service', got %s", fwd.Hostnames[0])
	}
}

// TestConcurrentAccess tests thread safety
func TestConcurrentAccess(t *testing.T) {
	store := NewStore(100)

	var wg sync.WaitGroup
	numGoroutines := 50

	// Concurrent adds
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			store.AddForward(ForwardSnapshot{
				Key:         "svc.ns.ctx.pod" + string(rune('a'+n)) + ".8080",
				ServiceKey:  "svc.ns.ctx",
				ServiceName: "my-service",
				Namespace:   "default",
				PodName:     "pod-" + string(rune('a'+n)),
			})
		}(i)
	}

	// Concurrent reads
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = store.GetFiltered()
			_ = store.GetSummary()
			_ = store.Count()
		}()
	}

	// Concurrent updates
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := "svc.ns.ctx.pod" + string(rune('a'+n)) + ".8080"
			store.UpdateMetrics(key, uint64(n*100), uint64(n*200), float64(n), float64(n*2), 0, 0)
			store.UpdateStatus(key, StatusActive, "")
		}(i)
	}

	// Concurrent logs
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			store.AddLog(LogEntry{
				Timestamp: time.Now(),
				Level:     "info",
				Message:   "concurrent log",
			})
		}()
	}

	wg.Wait()

	// Verify no panics occurred and data is consistent
	count := store.Count()
	if count == 0 {
		t.Error("Expected some forwards after concurrent adds")
	}
}

// TestGetForward_ReturnsACopy tests that GetForward returns a copy
func TestGetForward_ReturnsACopy(t *testing.T) {
	store := NewStore(100)

	store.AddForward(ForwardSnapshot{
		Key:         "svc.ns.ctx.pod.8080",
		ServiceKey:  "svc.ns.ctx",
		ServiceName: "my-service",
		Namespace:   "default",
		PodName:     "pod-123",
		BytesIn:     100,
	})

	fwd := store.GetForward("svc.ns.ctx.pod.8080")
	if fwd == nil {
		t.Fatal("Expected to retrieve forward")
	}

	// Modify the returned copy
	fwd.BytesIn = 9999

	// Original should be unchanged
	original := store.GetForward("svc.ns.ctx.pod.8080")
	if original.BytesIn == 9999 {
		t.Error("Modifying returned forward should not affect store")
	}
}

// TestGetService_ReturnsACopy tests that GetService returns a copy
func TestGetService_ReturnsACopy(t *testing.T) {
	store := NewStore(100)

	store.AddForward(ForwardSnapshot{
		Key:         "svc.ns.ctx.pod.8080",
		ServiceKey:  "svc.ns.ctx",
		ServiceName: "my-service",
		Namespace:   "default",
		PodName:     "pod-123",
	})

	svc := store.GetService("svc.ns.ctx")
	if svc == nil {
		t.Fatal("Expected to retrieve service")
	}

	// Modify the returned copy
	svc.ServiceName = "modified-name"

	// Original should be unchanged
	original := store.GetService("svc.ns.ctx")
	if original.ServiceName == "modified-name" {
		t.Error("Modifying returned service should not affect store")
	}
}

// TestMatchesFilter_Hostnames tests filter matching on hostnames
func TestMatchesFilter_Hostnames(t *testing.T) {
	store := NewStore(100)

	store.AddForward(ForwardSnapshot{
		Key:         "svc.ns.ctx.pod.8080",
		ServiceKey:  "svc.ns.ctx",
		ServiceName: "my-service",
		Namespace:   "default",
		PodName:     "pod-123",
		Hostnames:   []string{"my-service", "my-service.default.svc.cluster.local"},
	})

	// Filter by hostname
	store.SetFilter("cluster.local")
	filtered := store.GetFiltered()

	if len(filtered) != 1 {
		t.Errorf("Expected 1 match for 'cluster.local' hostname, got %d", len(filtered))
	}
}
