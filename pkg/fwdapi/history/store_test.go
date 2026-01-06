package history

import (
	"testing"
	"time"
)

func TestNewStore(t *testing.T) {
	tests := []struct {
		name   string
		config Config
		want   struct {
			maxEvents     int
			maxErrors     int
			maxReconnects int
		}
	}{
		{
			name:   "default values for zero config",
			config: Config{},
			want: struct {
				maxEvents     int
				maxErrors     int
				maxReconnects int
			}{1000, 500, 200},
		},
		{
			name: "custom values",
			config: Config{
				MaxEvents:     100,
				MaxErrors:     50,
				MaxReconnects: 20,
			},
			want: struct {
				maxEvents     int
				maxErrors     int
				maxReconnects int
			}{100, 50, 20},
		},
		{
			name: "negative values get defaults",
			config: Config{
				MaxEvents:     -1,
				MaxErrors:     -1,
				MaxReconnects: -1,
			},
			want: struct {
				maxEvents     int
				maxErrors     int
				maxReconnects int
			}{1000, 500, 200},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewStore(tt.config)
			stats := store.GetStats()

			if stats.MaxEvents != tt.want.maxEvents {
				t.Errorf("MaxEvents = %d, want %d", stats.MaxEvents, tt.want.maxEvents)
			}
			if stats.MaxErrors != tt.want.maxErrors {
				t.Errorf("MaxErrors = %d, want %d", stats.MaxErrors, tt.want.maxErrors)
			}
			if stats.MaxReconnects != tt.want.maxReconnects {
				t.Errorf("MaxReconnects = %d, want %d", stats.MaxReconnects, tt.want.maxReconnects)
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.MaxEvents != 1000 {
		t.Errorf("MaxEvents = %d, want 1000", cfg.MaxEvents)
	}
	if cfg.MaxErrors != 500 {
		t.Errorf("MaxErrors = %d, want 500", cfg.MaxErrors)
	}
	if cfg.MaxReconnects != 200 {
		t.Errorf("MaxReconnects = %d, want 200", cfg.MaxReconnects)
	}
}

func TestRecordEvent(t *testing.T) {
	store := NewStore(Config{MaxEvents: 10})

	// Record an event
	id := store.RecordEvent(EventServiceAdded, "svc.ns.ctx", "", "", "Service added", nil)

	if id != 1 {
		t.Errorf("first event ID = %d, want 1", id)
	}

	// Record another event
	id2 := store.RecordEvent(EventForwardStarted, "svc.ns.ctx", "fwd-key", "pod-1", "Forward started", map[string]interface{}{"port": 8080})

	if id2 != 2 {
		t.Errorf("second event ID = %d, want 2", id2)
	}

	// Verify events are stored
	events := store.GetEvents(10, "")
	if len(events) != 2 {
		t.Errorf("event count = %d, want 2", len(events))
	}

	// Most recent first
	if events[0].ID != 2 {
		t.Errorf("first returned event ID = %d, want 2", events[0].ID)
	}
	if events[0].Type != EventForwardStarted {
		t.Errorf("event type = %s, want %s", events[0].Type, EventForwardStarted)
	}
}

func TestRecordEventRingBuffer(t *testing.T) {
	store := NewStore(Config{MaxEvents: 3})

	// Fill the buffer
	store.RecordEvent(EventServiceAdded, "svc1", "", "", "Event 1", nil)
	store.RecordEvent(EventServiceAdded, "svc2", "", "", "Event 2", nil)
	store.RecordEvent(EventServiceAdded, "svc3", "", "", "Event 3", nil)

	// This should overwrite the first event
	store.RecordEvent(EventServiceAdded, "svc4", "", "", "Event 4", nil)

	events := store.GetEvents(10, "")
	if len(events) != 3 {
		t.Errorf("event count = %d, want 3", len(events))
	}

	// Should have events 4, 3, 2 (most recent first)
	if events[0].Message != "Event 4" {
		t.Errorf("first event message = %s, want Event 4", events[0].Message)
	}
	if events[2].Message != "Event 2" {
		t.Errorf("last event message = %s, want Event 2", events[2].Message)
	}
}

func TestGetEventsWithTypeFilter(t *testing.T) {
	store := NewStore(Config{MaxEvents: 10})

	store.RecordEvent(EventServiceAdded, "svc1", "", "", "Added", nil)
	store.RecordEvent(EventForwardError, "svc1", "fwd1", "pod1", "Error", nil)
	store.RecordEvent(EventServiceAdded, "svc2", "", "", "Added", nil)
	store.RecordEvent(EventForwardError, "svc2", "fwd2", "pod2", "Error", nil)

	// Filter by type
	errors := store.GetEvents(10, EventForwardError)
	if len(errors) != 2 {
		t.Errorf("error event count = %d, want 2", len(errors))
	}

	added := store.GetEvents(10, EventServiceAdded)
	if len(added) != 2 {
		t.Errorf("added event count = %d, want 2", len(added))
	}
}

func TestGetEventsCountLimit(t *testing.T) {
	store := NewStore(Config{MaxEvents: 10})

	for i := 0; i < 5; i++ {
		store.RecordEvent(EventServiceAdded, "svc", "", "", "Event", nil)
	}

	// Request fewer than available
	events := store.GetEvents(2, "")
	if len(events) != 2 {
		t.Errorf("event count = %d, want 2", len(events))
	}

	// Request more than available
	events = store.GetEvents(100, "")
	if len(events) != 5 {
		t.Errorf("event count = %d, want 5", len(events))
	}

	// Request zero or negative
	events = store.GetEvents(0, "")
	if events != nil {
		t.Errorf("expected nil for count=0")
	}
}

func TestRecordError(t *testing.T) {
	store := NewStore(Config{MaxErrors: 10})

	id := store.RecordError("svc.ns.ctx", "fwd-key", "pod-1", "connection_refused", "Connection refused")

	if id != 1 {
		t.Errorf("first error ID = %d, want 1", id)
	}

	errors := store.GetErrors(10)
	if len(errors) != 1 {
		t.Errorf("error count = %d, want 1", len(errors))
	}

	if errors[0].ErrorType != "connection_refused" {
		t.Errorf("error type = %s, want connection_refused", errors[0].ErrorType)
	}
	if errors[0].Resolved {
		t.Error("error should not be marked as resolved")
	}
}

func TestRecordErrorRingBuffer(t *testing.T) {
	store := NewStore(Config{MaxErrors: 2})

	store.RecordError("svc1", "", "", "type1", "Error 1")
	store.RecordError("svc2", "", "", "type2", "Error 2")
	store.RecordError("svc3", "", "", "type3", "Error 3")

	errors := store.GetErrors(10)
	if len(errors) != 2 {
		t.Errorf("error count = %d, want 2", len(errors))
	}

	// Should have errors 3, 2 (most recent first)
	if errors[0].Message != "Error 3" {
		t.Errorf("first error message = %s, want Error 3", errors[0].Message)
	}
}

func TestRecordReconnect(t *testing.T) {
	store := NewStore(Config{MaxReconnects: 10})

	id := store.RecordReconnect("svc.ns.ctx", "auto", 3, true, 500*time.Millisecond, "")

	if id != 1 {
		t.Errorf("first reconnect ID = %d, want 1", id)
	}

	reconnects := store.GetReconnects(10, "")
	if len(reconnects) != 1 {
		t.Errorf("reconnect count = %d, want 1", len(reconnects))
	}

	if reconnects[0].Trigger != "auto" {
		t.Errorf("trigger = %s, want auto", reconnects[0].Trigger)
	}
	if reconnects[0].AttemptCount != 3 {
		t.Errorf("attempt count = %d, want 3", reconnects[0].AttemptCount)
	}
	if !reconnects[0].Success {
		t.Error("reconnect should be marked as success")
	}
}

func TestGetReconnectsWithServiceFilter(t *testing.T) {
	store := NewStore(Config{MaxReconnects: 10})

	store.RecordReconnect("svc1.ns.ctx", "auto", 1, true, time.Second, "")
	store.RecordReconnect("svc2.ns.ctx", "manual", 1, false, time.Second, "timeout")
	store.RecordReconnect("svc1.ns.ctx", "auto", 2, true, time.Second, "")

	// Filter by service
	svc1Reconnects := store.GetReconnects(10, "svc1.ns.ctx")
	if len(svc1Reconnects) != 2 {
		t.Errorf("svc1 reconnect count = %d, want 2", len(svc1Reconnects))
	}

	svc2Reconnects := store.GetReconnects(10, "svc2.ns.ctx")
	if len(svc2Reconnects) != 1 {
		t.Errorf("svc2 reconnect count = %d, want 1", len(svc2Reconnects))
	}

	// No filter
	allReconnects := store.GetReconnects(10, "")
	if len(allReconnects) != 3 {
		t.Errorf("all reconnect count = %d, want 3", len(allReconnects))
	}
}

func TestGetStats(t *testing.T) {
	store := NewStore(Config{
		MaxEvents:     100,
		MaxErrors:     50,
		MaxReconnects: 20,
	})

	// Initially empty
	stats := store.GetStats()
	if stats.TotalEvents != 0 || stats.TotalErrors != 0 || stats.TotalReconnects != 0 {
		t.Error("expected all counts to be 0 initially")
	}

	// Add some records
	store.RecordEvent(EventServiceAdded, "svc", "", "", "Event", nil)
	store.RecordEvent(EventServiceAdded, "svc", "", "", "Event", nil)
	store.RecordError("svc", "", "", "type", "Error")
	store.RecordReconnect("svc", "auto", 1, true, time.Second, "")

	stats = store.GetStats()
	if stats.TotalEvents != 2 {
		t.Errorf("TotalEvents = %d, want 2", stats.TotalEvents)
	}
	if stats.TotalErrors != 1 {
		t.Errorf("TotalErrors = %d, want 1", stats.TotalErrors)
	}
	if stats.TotalReconnects != 1 {
		t.Errorf("TotalReconnects = %d, want 1", stats.TotalReconnects)
	}
	if stats.MaxEvents != 100 {
		t.Errorf("MaxEvents = %d, want 100", stats.MaxEvents)
	}
}

func TestClear(t *testing.T) {
	store := NewStore(Config{MaxEvents: 10, MaxErrors: 10, MaxReconnects: 10})

	// Add records
	store.RecordEvent(EventServiceAdded, "svc", "", "", "Event", nil)
	store.RecordError("svc", "", "", "type", "Error")
	store.RecordReconnect("svc", "auto", 1, true, time.Second, "")

	// Verify they exist
	stats := store.GetStats()
	if stats.TotalEvents == 0 || stats.TotalErrors == 0 || stats.TotalReconnects == 0 {
		t.Error("expected records to exist before clear")
	}

	// Clear
	store.Clear()

	// Verify empty
	stats = store.GetStats()
	if stats.TotalEvents != 0 || stats.TotalErrors != 0 || stats.TotalReconnects != 0 {
		t.Error("expected all counts to be 0 after clear")
	}

	events := store.GetEvents(10, "")
	if len(events) != 0 {
		t.Errorf("expected 0 events after clear, got %d", len(events))
	}
}

func TestGetStore(t *testing.T) {
	// GetStore should return the global store (singleton)
	store1 := GetStore()
	store2 := GetStore()

	if store1 != store2 {
		t.Error("GetStore should return the same instance")
	}

	if store1 == nil {
		t.Error("GetStore should not return nil")
	}
}

func TestConcurrentAccess(t *testing.T) {
	store := NewStore(Config{MaxEvents: 100, MaxErrors: 100, MaxReconnects: 100})

	done := make(chan bool)

	// Writer goroutine for events
	go func() {
		for i := 0; i < 50; i++ {
			store.RecordEvent(EventServiceAdded, "svc", "", "", "Event", nil)
		}
		done <- true
	}()

	// Writer goroutine for errors
	go func() {
		for i := 0; i < 50; i++ {
			store.RecordError("svc", "", "", "type", "Error")
		}
		done <- true
	}()

	// Reader goroutine
	go func() {
		for i := 0; i < 50; i++ {
			store.GetEvents(10, "")
			store.GetErrors(10)
			store.GetStats()
		}
		done <- true
	}()

	// Wait for all goroutines
	<-done
	<-done
	<-done

	// Verify counts
	stats := store.GetStats()
	if stats.TotalEvents != 50 {
		t.Errorf("TotalEvents = %d, want 50", stats.TotalEvents)
	}
	if stats.TotalErrors != 50 {
		t.Errorf("TotalErrors = %d, want 50", stats.TotalErrors)
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
