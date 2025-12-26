package fwdtui

import (
	"testing"
	"time"

	"github.com/txn2/kubefwd/pkg/fwdtui/events"
	"github.com/txn2/kubefwd/pkg/fwdtui/state"
)

func TestEventBus(t *testing.T) {
	bus := events.NewBus(100)
	bus.Start()
	defer bus.Stop()

	received := make(chan bool, 1)
	bus.Subscribe(events.ServiceAdded, func(e events.Event) {
		if e.Service == "test-svc" {
			received <- true
		}
	})

	bus.Publish(events.NewServiceEvent(events.ServiceAdded, "test-svc", "default", "ctx"))

	select {
	case <-received:
		// Success
	case <-time.After(time.Second):
		t.Error("Event not received within timeout")
	}
}

func TestStateStore(t *testing.T) {
	store := state.NewStore(100)

	snapshot := state.ForwardSnapshot{
		Key:         "test-key",
		ServiceName: "test-svc",
		Namespace:   "default",
		Context:     "ctx",
		PodName:     "pod-1",
		LocalIP:     "127.1.27.1",
		LocalPort:   "8080",
		Status:      state.StatusActive,
	}

	store.AddForward(snapshot)

	forwards := store.GetFiltered()
	if len(forwards) != 1 {
		t.Errorf("Expected 1 forward, got %d", len(forwards))
	}

	if forwards[0].ServiceName != "test-svc" {
		t.Errorf("Expected service name 'test-svc', got '%s'", forwards[0].ServiceName)
	}

	// Test filter
	store.SetFilter("test")
	filtered := store.GetFiltered()
	if len(filtered) != 1 {
		t.Errorf("Expected 1 filtered forward, got %d", len(filtered))
	}

	store.SetFilter("nomatch")
	filtered = store.GetFiltered()
	if len(filtered) != 0 {
		t.Errorf("Expected 0 filtered forwards, got %d", len(filtered))
	}

	// Test remove
	store.SetFilter("")
	store.RemoveForward("test-key")
	forwards = store.GetFiltered()
	if len(forwards) != 0 {
		t.Errorf("Expected 0 forwards after remove, got %d", len(forwards))
	}
}

func TestEnableDisable(t *testing.T) {
	// Reset state for test
	mu.Lock()
	tuiEnabled = false
	mu.Unlock()

	if IsEnabled() {
		t.Error("Expected TUI to be disabled initially")
	}

	Enable()

	if !IsEnabled() {
		t.Error("Expected TUI to be enabled after Enable()")
	}
}
