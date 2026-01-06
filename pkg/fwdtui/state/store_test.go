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
	if original == nil {
		t.Fatal("Expected original forward to exist")
	}
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
	if original == nil {
		t.Fatal("Expected original service to exist")
	}
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

// TestMatchesFilter_PodName tests filter matching on pod name
func TestMatchesFilter_PodName(t *testing.T) {
	store := NewStore(100)

	store.AddForward(ForwardSnapshot{
		Key:         "svc.ns.ctx.my-pod-abc123.8080",
		ServiceKey:  "svc.ns.ctx",
		ServiceName: "my-service",
		Namespace:   "default",
		PodName:     "my-pod-abc123",
	})

	store.AddForward(ForwardSnapshot{
		Key:         "svc.ns.ctx.other-pod-xyz.8080",
		ServiceKey:  "svc.ns.ctx",
		ServiceName: "my-service",
		Namespace:   "default",
		PodName:     "other-pod-xyz",
	})

	// Filter by pod name
	store.SetFilter("abc123")
	filtered := store.GetFiltered()

	if len(filtered) != 1 {
		t.Errorf("Expected 1 match for 'abc123' pod name, got %d", len(filtered))
	}

	if filtered[0].PodName != "my-pod-abc123" {
		t.Errorf("Expected pod 'my-pod-abc123', got %s", filtered[0].PodName)
	}
}

// TestGetServices tests getting all services
func TestGetServices(t *testing.T) {
	store := NewStore(100)

	// Add forwards for multiple services
	store.AddForward(ForwardSnapshot{
		Key:         "svc1.ns.ctx.pod.8080",
		ServiceKey:  "svc1.ns.ctx",
		ServiceName: "zebra-service",
		Namespace:   "default",
		Status:      StatusActive,
	})

	store.AddForward(ForwardSnapshot{
		Key:         "svc2.ns.ctx.pod.8080",
		ServiceKey:  "svc2.ns.ctx",
		ServiceName: "alpha-service",
		Namespace:   "default",
		Status:      StatusActive,
	})

	store.AddForward(ForwardSnapshot{
		Key:         "svc3.ns.ctx.pod.8080",
		ServiceKey:  "svc3.ns.ctx",
		ServiceName: "beta-service",
		Namespace:   "production",
		Status:      StatusError,
	})

	services := store.GetServices()

	if len(services) != 3 {
		t.Fatalf("Expected 3 services, got %d", len(services))
	}

	// Services should be sorted by name
	if services[0].ServiceName != "alpha-service" {
		t.Errorf("Expected first service 'alpha-service', got %s", services[0].ServiceName)
	}

	if services[1].ServiceName != "beta-service" {
		t.Errorf("Expected second service 'beta-service', got %s", services[1].ServiceName)
	}

	if services[2].ServiceName != "zebra-service" {
		t.Errorf("Expected third service 'zebra-service', got %s", services[2].ServiceName)
	}
}

// TestSortForwards_AllFields tests all sort field options
func TestSortForwards_AllFields(t *testing.T) {
	store := NewStore(100)

	store.AddForward(ForwardSnapshot{
		Key:         "svc1.ns.ctx.pod.8080",
		ServiceKey:  "svc1.ns.ctx",
		ServiceName: "alpha",
		Namespace:   "zebra",
		LocalPort:   "8080",
		Hostnames:   []string{"alpha"},
		Status:      StatusActive,
		RateIn:      100.0,
		RateOut:     50.0,
	})

	store.AddForward(ForwardSnapshot{
		Key:         "svc2.ns.ctx.pod.9090",
		ServiceKey:  "svc2.ns.ctx",
		ServiceName: "zebra",
		Namespace:   "alpha",
		LocalPort:   "9090",
		Hostnames:   []string{"zebra"},
		Status:      StatusError,
		RateIn:      200.0,
		RateOut:     100.0,
	})

	store.AddForward(ForwardSnapshot{
		Key:         "svc3.ns.ctx.pod.7070",
		ServiceKey:  "svc3.ns.ctx",
		ServiceName: "mango",
		Namespace:   "mango",
		LocalPort:   "7070",
		Hostnames:   []string{"mango"},
		Status:      StatusConnecting,
		RateIn:      50.0,
		RateOut:     25.0,
	})

	tests := []struct {
		sortField string
		ascending bool
		firstSvc  string
		lastSvc   string
	}{
		{"hostname", true, "alpha", "zebra"},
		{"hostname", false, "zebra", "alpha"},
		{"namespace", true, "zebra", "alpha"},     // namespace alpha < mango < zebra
		{"namespace", false, "alpha", "zebra"},    // descending
		{"status", true, "mango", "zebra"},        // StatusConnecting(1) < StatusActive(2) < StatusError(3)
		{"status", false, "zebra", "mango"},       // descending
		{"rateIn", true, "mango", "zebra"},        // 50 < 100 < 200
		{"rateIn", false, "zebra", "mango"},       // descending
		{"rateOut", true, "mango", "zebra"},       // 25 < 50 < 100
		{"rateOut", false, "zebra", "mango"},      // descending
		{"unknown_field", true, "alpha", "zebra"}, // default to hostname
	}

	for _, tt := range tests {
		t.Run(tt.sortField+"_"+boolToStr(tt.ascending), func(t *testing.T) {
			store.SetSort(tt.sortField, tt.ascending)
			filtered := store.GetFiltered()

			if filtered[0].ServiceName != tt.firstSvc {
				t.Errorf("Sort by %s (asc=%v): expected first '%s', got '%s'",
					tt.sortField, tt.ascending, tt.firstSvc, filtered[0].ServiceName)
			}

			if filtered[len(filtered)-1].ServiceName != tt.lastSvc {
				t.Errorf("Sort by %s (asc=%v): expected last '%s', got '%s'",
					tt.sortField, tt.ascending, tt.lastSvc, filtered[len(filtered)-1].ServiceName)
			}
		})
	}
}

func boolToStr(b bool) string {
	if b {
		return "asc"
	}
	return "desc"
}

// TestForwardStatus_String tests all ForwardStatus string representations
func TestForwardStatus_String(t *testing.T) {
	tests := []struct {
		status   ForwardStatus
		expected string
	}{
		{StatusPending, "Pending"},
		{StatusConnecting, "Connecting"},
		{StatusActive, "Active"},
		{StatusError, "Error"},
		{StatusStopping, "Stopping"},
		{ForwardStatus(99), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if tt.status.String() != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, tt.status.String())
			}
		})
	}
}

// TestForwardSnapshot_PrimaryHostname tests PrimaryHostname edge cases
func TestForwardSnapshot_PrimaryHostname(t *testing.T) {
	tests := []struct {
		name     string
		snapshot ForwardSnapshot
		expected string
	}{
		{
			name: "no hostnames returns service name",
			snapshot: ForwardSnapshot{
				ServiceName: "my-service",
				Hostnames:   []string{},
			},
			expected: "my-service",
		},
		{
			name: "nil hostnames returns service name",
			snapshot: ForwardSnapshot{
				ServiceName: "my-service",
				Hostnames:   nil,
			},
			expected: "my-service",
		},
		{
			name: "single hostname",
			snapshot: ForwardSnapshot{
				ServiceName: "my-service",
				Hostnames:   []string{"svc"},
			},
			expected: "svc",
		},
		{
			name: "multiple hostnames returns shortest",
			snapshot: ForwardSnapshot{
				ServiceName: "my-service",
				Hostnames:   []string{"my-service.namespace.svc.cluster.local", "my-service.namespace", "my-service"},
			},
			expected: "my-service",
		},
		{
			name: "first is already shortest",
			snapshot: ForwardSnapshot{
				ServiceName: "my-service",
				Hostnames:   []string{"a", "bb", "ccc"},
			},
			expected: "a",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.snapshot.PrimaryHostname()
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// TestForwardSnapshot_LocalAddress tests LocalAddress
func TestForwardSnapshot_LocalAddress(t *testing.T) {
	snapshot := ForwardSnapshot{
		LocalIP:   "127.1.27.1",
		LocalPort: "8080",
	}

	expected := "127.1.27.1:8080"
	if snapshot.LocalAddress() != expected {
		t.Errorf("Expected %q, got %q", expected, snapshot.LocalAddress())
	}
}

// TestNewRateHistory tests creating a new rate history
func TestNewRateHistory(t *testing.T) {
	h := NewRateHistory(30)
	if h == nil {
		t.Fatal("Expected non-nil RateHistory")
	}
	if h.maxSize != 30 {
		t.Errorf("Expected maxSize 30, got %d", h.maxSize)
	}
	if h.Size() != 0 {
		t.Errorf("Expected size 0, got %d", h.Size())
	}
}

// TestNewRateHistory_DefaultSize tests default max size
func TestNewRateHistory_DefaultSize(t *testing.T) {
	h := NewRateHistory(0)
	if h.maxSize != 60 {
		t.Errorf("Expected default maxSize 60, got %d", h.maxSize)
	}

	h2 := NewRateHistory(-10)
	if h2.maxSize != 60 {
		t.Errorf("Expected default maxSize 60 for negative, got %d", h2.maxSize)
	}
}

// TestRateHistory_AddSample tests adding samples
func TestRateHistory_AddSample(t *testing.T) {
	h := NewRateHistory(5)

	h.AddSample("fwd1", 100.0, 50.0)
	h.AddSample("fwd1", 200.0, 100.0)
	h.AddSample("fwd1", 300.0, 150.0)

	if h.Size() != 1 {
		t.Errorf("Expected 1 forward tracked, got %d", h.Size())
	}

	rateIn, rateOut := h.GetAllHistory("fwd1")
	if len(rateIn) != 3 {
		t.Errorf("Expected 3 rateIn samples, got %d", len(rateIn))
	}
	if len(rateOut) != 3 {
		t.Errorf("Expected 3 rateOut samples, got %d", len(rateOut))
	}

	if rateIn[0] != 100.0 || rateIn[2] != 300.0 {
		t.Errorf("Unexpected rateIn values: %v", rateIn)
	}
}

// TestRateHistory_AddSample_Overflow tests sample buffer overflow
func TestRateHistory_AddSample_Overflow(t *testing.T) {
	h := NewRateHistory(3)

	// Add more samples than max size
	for i := 1; i <= 5; i++ {
		h.AddSample("fwd1", float64(i*100), float64(i*50))
	}

	rateIn, rateOut := h.GetAllHistory("fwd1")

	// Should only have last 3 samples
	if len(rateIn) != 3 {
		t.Errorf("Expected 3 samples after overflow, got %d", len(rateIn))
	}

	// Should be samples 3, 4, 5 (values 300, 400, 500)
	if rateIn[0] != 300.0 || rateIn[1] != 400.0 || rateIn[2] != 500.0 {
		t.Errorf("Expected [300, 400, 500], got %v", rateIn)
	}

	if rateOut[0] != 150.0 || rateOut[1] != 200.0 || rateOut[2] != 250.0 {
		t.Errorf("Expected [150, 200, 250], got %v", rateOut)
	}
}

// TestRateHistory_GetHistory tests getting limited history
func TestRateHistory_GetHistory(t *testing.T) {
	h := NewRateHistory(10)

	for i := 1; i <= 6; i++ {
		h.AddSample("fwd1", float64(i*100), float64(i*50))
	}

	// Get last 3 samples
	rateIn, rateOut := h.GetHistory("fwd1", 3)

	if len(rateIn) != 3 {
		t.Errorf("Expected 3 samples, got %d", len(rateIn))
	}

	// Should be samples 4, 5, 6 (values 400, 500, 600)
	if rateIn[0] != 400.0 || rateIn[1] != 500.0 || rateIn[2] != 600.0 {
		t.Errorf("Expected [400, 500, 600], got %v", rateIn)
	}

	if rateOut[0] != 200.0 || rateOut[1] != 250.0 || rateOut[2] != 300.0 {
		t.Errorf("Expected [200, 250, 300], got %v", rateOut)
	}
}

// TestRateHistory_GetHistory_NonExistent tests getting history for non-existent forward
func TestRateHistory_GetHistory_NonExistent(t *testing.T) {
	h := NewRateHistory(10)

	rateIn, rateOut := h.GetHistory("nonexistent", 5)

	if rateIn != nil {
		t.Errorf("Expected nil rateIn for nonexistent key, got %v", rateIn)
	}
	if rateOut != nil {
		t.Errorf("Expected nil rateOut for nonexistent key, got %v", rateOut)
	}
}

// TestRateHistory_GetAllHistory tests getting all history
func TestRateHistory_GetAllHistory(t *testing.T) {
	h := NewRateHistory(10)

	for i := 1; i <= 5; i++ {
		h.AddSample("fwd1", float64(i), float64(i*2))
	}

	rateIn, rateOut := h.GetAllHistory("fwd1")

	if len(rateIn) != 5 {
		t.Errorf("Expected 5 samples, got %d", len(rateIn))
	}
	if len(rateOut) != 5 {
		t.Errorf("Expected 5 rateOut samples, got %d", len(rateOut))
	}

	// Verify it returns a copy (intentionally discarding result to test immutability)
	originalLen := len(rateIn)
	_ = append(rateIn, 999.0)

	rateIn2, _ := h.GetAllHistory("fwd1")
	if len(rateIn2) != originalLen {
		t.Error("GetAllHistory should return a copy, not modify original")
	}
}

// TestRateHistory_GetAllHistory_NonExistent tests getting all history for non-existent forward
func TestRateHistory_GetAllHistory_NonExistent(t *testing.T) {
	h := NewRateHistory(10)

	rateIn, rateOut := h.GetAllHistory("nonexistent")

	if rateIn != nil {
		t.Errorf("Expected nil rateIn, got %v", rateIn)
	}
	if rateOut != nil {
		t.Errorf("Expected nil rateOut, got %v", rateOut)
	}
}

// TestRateHistory_Remove tests removing a forward
func TestRateHistory_Remove(t *testing.T) {
	h := NewRateHistory(10)

	h.AddSample("fwd1", 100.0, 50.0)
	h.AddSample("fwd2", 200.0, 100.0)

	if h.Size() != 2 {
		t.Errorf("Expected 2 forwards, got %d", h.Size())
	}

	h.Remove("fwd1")

	if h.Size() != 1 {
		t.Errorf("Expected 1 forward after remove, got %d", h.Size())
	}

	rateIn, _ := h.GetAllHistory("fwd1")
	if rateIn != nil {
		t.Error("Expected nil after remove")
	}

	rateIn, _ = h.GetAllHistory("fwd2")
	if rateIn == nil {
		t.Error("Expected fwd2 to still exist")
	}
}

// TestRateHistory_Clear tests clearing all history
func TestRateHistory_Clear(t *testing.T) {
	h := NewRateHistory(10)

	h.AddSample("fwd1", 100.0, 50.0)
	h.AddSample("fwd2", 200.0, 100.0)
	h.AddSample("fwd3", 300.0, 150.0)

	if h.Size() != 3 {
		t.Errorf("Expected 3 forwards, got %d", h.Size())
	}

	h.Clear()

	if h.Size() != 0 {
		t.Errorf("Expected 0 forwards after clear, got %d", h.Size())
	}
}

// TestRateHistory_Concurrent tests thread safety
func TestRateHistory_Concurrent(t *testing.T) {
	h := NewRateHistory(100)

	var wg sync.WaitGroup
	numGoroutines := 50

	// Concurrent adds
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := "fwd" + string(rune('a'+n%26))
			h.AddSample(key, float64(n*100), float64(n*50))
		}(i)
	}

	// Concurrent reads
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := "fwd" + string(rune('a'+n%26))
			_, _ = h.GetHistory(key, 5)
			_, _ = h.GetAllHistory(key)
			_ = h.Size()
		}(i)
	}

	wg.Wait()

	// Should not panic and size should be reasonable
	if h.Size() > 26 {
		t.Errorf("Size should be <= 26, got %d", h.Size())
	}
}

// TestUpdateMetrics_NonExistent tests updating metrics for non-existent forward
func TestUpdateMetrics_NonExistent(t *testing.T) {
	store := NewStore(100)

	// Should not panic
	store.UpdateMetrics("nonexistent.key", 1000, 2000, 100.0, 200.0, 50.0, 100.0)

	if store.Count() != 0 {
		t.Error("Should not create forward when updating non-existent key")
	}
}

// TestUpdateStatus_NonExistent tests updating status for non-existent forward
func TestUpdateStatus_NonExistent(t *testing.T) {
	store := NewStore(100)

	// Should not panic
	store.UpdateStatus("nonexistent.key", StatusError, "error message")

	if store.Count() != 0 {
		t.Error("Should not create forward when updating non-existent key")
	}
}

// TestUpdateHostnames_NonExistent tests updating hostnames for non-existent forward
func TestUpdateHostnames_NonExistent(t *testing.T) {
	store := NewStore(100)

	// Should not panic
	store.UpdateHostnames("nonexistent.key", []string{"host1", "host2"})

	if store.Count() != 0 {
		t.Error("Should not create forward when updating non-existent key")
	}
}

// TestGetForward_NonExistent tests getting non-existent forward
func TestGetForward_NonExistent(t *testing.T) {
	store := NewStore(100)

	fwd := store.GetForward("nonexistent.key")
	if fwd != nil {
		t.Error("Expected nil for non-existent forward")
	}
}

// TestGetService_NonExistent tests getting non-existent service
func TestGetService_NonExistent(t *testing.T) {
	store := NewStore(100)

	svc := store.GetService("nonexistent.key")
	if svc != nil {
		t.Error("Expected nil for non-existent service")
	}
}

// TestRemoveByNamespace tests removing all services and forwards for a namespace
func TestRemoveByNamespace(t *testing.T) {
	store := NewStore(100)

	// Add forwards from different namespaces
	store.AddForward(ForwardSnapshot{
		Key:         "svc1.ns1.ctx1.pod1.8080",
		ServiceKey:  "svc1.ns1.ctx1",
		ServiceName: "svc1",
		Namespace:   "ns1",
		Context:     "ctx1",
		PodName:     "pod1",
		LocalPort:   "8080",
	})
	store.AddForward(ForwardSnapshot{
		Key:         "svc2.ns1.ctx1.pod2.8080",
		ServiceKey:  "svc2.ns1.ctx1",
		ServiceName: "svc2",
		Namespace:   "ns1",
		Context:     "ctx1",
		PodName:     "pod2",
		LocalPort:   "8080",
	})
	store.AddForward(ForwardSnapshot{
		Key:         "svc3.ns2.ctx1.pod3.8080",
		ServiceKey:  "svc3.ns2.ctx1",
		ServiceName: "svc3",
		Namespace:   "ns2",
		Context:     "ctx1",
		PodName:     "pod3",
		LocalPort:   "8080",
	})
	store.AddForward(ForwardSnapshot{
		Key:         "svc4.ns1.ctx2.pod4.8080",
		ServiceKey:  "svc4.ns1.ctx2",
		ServiceName: "svc4",
		Namespace:   "ns1",
		Context:     "ctx2",
		PodName:     "pod4",
		LocalPort:   "8080",
	})

	// Verify initial state
	if store.Count() != 4 {
		t.Fatalf("Expected 4 forwards, got %d", store.Count())
	}
	if store.ServiceCount() != 4 {
		t.Fatalf("Expected 4 services, got %d", store.ServiceCount())
	}

	// Remove all services for ns1.ctx1
	removed := store.RemoveByNamespace("ns1", "ctx1")

	// Should have removed 2 forwards
	if removed != 2 {
		t.Errorf("Expected to remove 2 forwards, removed %d", removed)
	}

	// Should have 2 forwards remaining
	if store.Count() != 2 {
		t.Errorf("Expected 2 forwards remaining, got %d", store.Count())
	}

	// Should have 2 services remaining
	if store.ServiceCount() != 2 {
		t.Errorf("Expected 2 services remaining, got %d", store.ServiceCount())
	}

	// Verify the correct services were removed
	if store.GetService("svc1.ns1.ctx1") != nil {
		t.Error("svc1.ns1.ctx1 should have been removed")
	}
	if store.GetService("svc2.ns1.ctx1") != nil {
		t.Error("svc2.ns1.ctx1 should have been removed")
	}
	if store.GetService("svc3.ns2.ctx1") == nil {
		t.Error("svc3.ns2.ctx1 should NOT have been removed")
	}
	if store.GetService("svc4.ns1.ctx2") == nil {
		t.Error("svc4.ns1.ctx2 should NOT have been removed (different context)")
	}
}

// TestRemoveByNamespace_Empty tests removing from empty store
func TestRemoveByNamespace_Empty(t *testing.T) {
	store := NewStore(100)

	removed := store.RemoveByNamespace("ns1", "ctx1")

	if removed != 0 {
		t.Errorf("Expected 0 removed from empty store, got %d", removed)
	}
}

// TestRemoveByNamespace_NoMatch tests removing when no services match
func TestRemoveByNamespace_NoMatch(t *testing.T) {
	store := NewStore(100)

	store.AddForward(ForwardSnapshot{
		Key:         "svc1.ns1.ctx1.pod1.8080",
		ServiceKey:  "svc1.ns1.ctx1",
		ServiceName: "svc1",
		Namespace:   "ns1",
		Context:     "ctx1",
		PodName:     "pod1",
		LocalPort:   "8080",
	})

	removed := store.RemoveByNamespace("ns2", "ctx1")

	if removed != 0 {
		t.Errorf("Expected 0 removed when no match, got %d", removed)
	}
	if store.Count() != 1 {
		t.Errorf("Expected 1 forward remaining, got %d", store.Count())
	}
}

// TestRemoveByNamespace_BlocksNewAdds tests that RemoveByNamespace blocks future AddForward calls
// This prevents race condition where in-flight port forward events re-add entries after removal
func TestRemoveByNamespace_BlocksNewAdds(t *testing.T) {
	store := NewStore(100)

	// Add a forward
	store.AddForward(ForwardSnapshot{
		Key:         "svc1.ns1.ctx1.pod1.8080",
		ServiceKey:  "svc1.ns1.ctx1",
		ServiceName: "svc1",
		Namespace:   "ns1",
		Context:     "ctx1",
		PodName:     "pod1",
		LocalPort:   "8080",
	})

	if store.Count() != 1 {
		t.Fatalf("Expected 1 forward, got %d", store.Count())
	}

	// Remove the namespace
	removed := store.RemoveByNamespace("ns1", "ctx1")
	if removed != 1 {
		t.Errorf("Expected 1 removed, got %d", removed)
	}

	if store.Count() != 0 {
		t.Errorf("Expected 0 forwards after removal, got %d", store.Count())
	}

	// Try to add a new forward for the removed namespace (simulating in-flight event)
	store.AddForward(ForwardSnapshot{
		Key:         "svc1.ns1.ctx1.pod2.8080",
		ServiceKey:  "svc1.ns1.ctx1",
		ServiceName: "svc1",
		Namespace:   "ns1",
		Context:     "ctx1",
		PodName:     "pod2",
		LocalPort:   "8080",
	})

	// The add should be blocked - count should still be 0
	if store.Count() != 0 {
		t.Errorf("Expected blocked namespace to prevent AddForward, but count is %d", store.Count())
	}

	// Adding to a different namespace should still work
	store.AddForward(ForwardSnapshot{
		Key:         "svc2.ns2.ctx1.pod1.8080",
		ServiceKey:  "svc2.ns2.ctx1",
		ServiceName: "svc2",
		Namespace:   "ns2",
		Context:     "ctx1",
		PodName:     "pod1",
		LocalPort:   "8080",
	})

	if store.Count() != 1 {
		t.Errorf("Expected AddForward to work for unblocked namespace, got count %d", store.Count())
	}
}

// TestUnblockNamespace tests that UnblockNamespace allows AddForward again
func TestUnblockNamespace(t *testing.T) {
	store := NewStore(100)

	// Add and remove a namespace
	store.AddForward(ForwardSnapshot{
		Key:         "svc1.ns1.ctx1.pod1.8080",
		ServiceKey:  "svc1.ns1.ctx1",
		ServiceName: "svc1",
		Namespace:   "ns1",
		Context:     "ctx1",
		PodName:     "pod1",
		LocalPort:   "8080",
	})
	store.RemoveByNamespace("ns1", "ctx1")

	// Verify blocked
	store.AddForward(ForwardSnapshot{
		Key:         "svc1.ns1.ctx1.pod2.8080",
		ServiceKey:  "svc1.ns1.ctx1",
		ServiceName: "svc1",
		Namespace:   "ns1",
		Context:     "ctx1",
		PodName:     "pod2",
		LocalPort:   "8080",
	})
	if store.Count() != 0 {
		t.Errorf("Expected namespace to be blocked, got count %d", store.Count())
	}

	// Unblock the namespace
	store.UnblockNamespace("ns1", "ctx1")

	// Now AddForward should work
	store.AddForward(ForwardSnapshot{
		Key:         "svc1.ns1.ctx1.pod3.8080",
		ServiceKey:  "svc1.ns1.ctx1",
		ServiceName: "svc1",
		Namespace:   "ns1",
		Context:     "ctx1",
		PodName:     "pod3",
		LocalPort:   "8080",
	})

	if store.Count() != 1 {
		t.Errorf("Expected AddForward to work after UnblockNamespace, got count %d", store.Count())
	}
}

// TestBoundedSize tests the boundedSize helper function
func TestBoundedSize(t *testing.T) {
	tests := []struct {
		name     string
		size     int
		limit    int
		expected int
	}{
		{"zero size", 0, 100, 0},
		{"negative size", -5, 100, 0},
		{"below limit", 50, 100, 50},
		{"at limit", 100, 100, 100},
		{"above limit", 150, 100, 100},
		{"way above limit", 10000, 100, 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := boundedSize(tt.size, tt.limit)
			if result != tt.expected {
				t.Errorf("boundedSize(%d, %d) = %d, want %d", tt.size, tt.limit, result, tt.expected)
			}
		})
	}
}
