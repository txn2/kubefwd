package fwdapi

import (
	"fmt"
	"sync"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/txn2/kubefwd/pkg/fwdapi/types"
)

func TestBoundedSize(t *testing.T) {
	tests := []struct {
		name     string
		size     int
		limit    int
		expected int
	}{
		{"zero size", 0, 100, 0},
		{"negative size", -10, 100, 0},
		{"within limit", 50, 100, 50},
		{"at limit", 100, 100, 100},
		{"exceeds limit", 150, 100, 100},
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

func TestNewLogBuffer(t *testing.T) {
	buf := NewLogBuffer(100)
	if buf == nil {
		t.Fatal("NewLogBuffer returned nil")
	}
	if buf.size != 100 {
		t.Errorf("Expected size 100, got %d", buf.size)
	}
	if buf.Count() != 0 {
		t.Errorf("Expected count 0, got %d", buf.Count())
	}
}

func TestLogBufferAdd(t *testing.T) {
	buf := NewLogBuffer(5)

	// Add entries
	for i := 0; i < 3; i++ {
		buf.Add(types.LogBufferEntry{
			Timestamp: time.Now(),
			Level:     "info",
			Message:   "test message",
		})
	}

	if buf.Count() != 3 {
		t.Errorf("Expected count 3, got %d", buf.Count())
	}
}

func TestLogBufferAddWrap(t *testing.T) {
	buf := NewLogBuffer(3)

	// Add more entries than buffer size to test wrapping
	for i := 0; i < 5; i++ {
		buf.Add(types.LogBufferEntry{
			Timestamp: time.Now(),
			Level:     "info",
			Message:   fmt.Sprintf("message %d", i),
		})
	}

	// Count should be at max buffer size
	if buf.Count() != 3 {
		t.Errorf("Expected count 3 (buffer full), got %d", buf.Count())
	}
}

func TestLogBufferGetLast(t *testing.T) {
	buf := NewLogBuffer(10)

	// Add 5 entries
	for i := 0; i < 5; i++ {
		buf.Add(types.LogBufferEntry{
			Timestamp: time.Now(),
			Level:     "info",
			Message:   "message",
		})
	}

	// Get last 3
	entries := buf.GetLast(3)
	if len(entries) != 3 {
		t.Errorf("Expected 3 entries, got %d", len(entries))
	}

	// Get more than available
	entries = buf.GetLast(10)
	if len(entries) != 5 {
		t.Errorf("Expected 5 entries (all available), got %d", len(entries))
	}

	// Get zero
	entries = buf.GetLast(0)
	if entries != nil {
		t.Errorf("Expected nil for GetLast(0), got %v", entries)
	}

	// Get negative
	entries = buf.GetLast(-1)
	if entries != nil {
		t.Errorf("Expected nil for GetLast(-1), got %v", entries)
	}
}

func TestLogBufferGetLastEmpty(t *testing.T) {
	buf := NewLogBuffer(10)

	entries := buf.GetLast(5)
	if entries != nil {
		t.Errorf("Expected nil for empty buffer, got %v", entries)
	}
}

func TestLogBufferGetAll(t *testing.T) {
	buf := NewLogBuffer(10)

	// Add 5 entries
	for i := 0; i < 5; i++ {
		buf.Add(types.LogBufferEntry{
			Timestamp: time.Now(),
			Level:     "info",
			Message:   "message",
		})
	}

	entries := buf.GetAll()
	if len(entries) != 5 {
		t.Errorf("Expected 5 entries, got %d", len(entries))
	}
}

func TestLogBufferClear(t *testing.T) {
	buf := NewLogBuffer(10)

	// Add entries
	for i := 0; i < 5; i++ {
		buf.Add(types.LogBufferEntry{
			Timestamp: time.Now(),
			Level:     "info",
			Message:   "message",
		})
	}

	if buf.Count() != 5 {
		t.Errorf("Expected count 5 before clear, got %d", buf.Count())
	}

	buf.Clear()

	if buf.Count() != 0 {
		t.Errorf("Expected count 0 after clear, got %d", buf.Count())
	}

	entries := buf.GetAll()
	if entries != nil {
		t.Errorf("Expected nil entries after clear, got %v", entries)
	}
}

func TestLogBufferHookLevels(t *testing.T) {
	buf := NewLogBuffer(10)

	// Test with nil levels (should default to AllLevels)
	hook := NewLogBufferHook(buf, nil)
	levels := hook.Levels()
	if len(levels) != len(log.AllLevels) {
		t.Errorf("Expected %d levels (AllLevels), got %d", len(log.AllLevels), len(levels))
	}

	// Test with specific levels
	specificLevels := []log.Level{log.ErrorLevel, log.WarnLevel}
	hook2 := NewLogBufferHook(buf, specificLevels)
	levels2 := hook2.Levels()
	if len(levels2) != 2 {
		t.Errorf("Expected 2 levels, got %d", len(levels2))
	}
}

func TestLogBufferHookFire(t *testing.T) {
	buf := NewLogBuffer(10)
	hook := NewLogBufferHook(buf, nil)

	// Create a log entry
	entry := &log.Entry{
		Time:    time.Now(),
		Level:   log.InfoLevel,
		Message: "test message",
		Data: log.Fields{
			"key1": "value1",
			"key2": 123, // non-string value
		},
	}

	err := hook.Fire(entry)
	if err != nil {
		t.Errorf("Fire returned error: %v", err)
	}

	if buf.Count() != 1 {
		t.Errorf("Expected 1 entry after Fire, got %d", buf.Count())
	}

	entries := buf.GetLast(1)
	if len(entries) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(entries))
	}

	if entries[0].Message != "test message" {
		t.Errorf("Expected message 'test message', got '%s'", entries[0].Message)
	}

	if entries[0].Level != "info" {
		t.Errorf("Expected level 'info', got '%s'", entries[0].Level)
	}

	// Check fields
	if entries[0].Fields["key1"] != "value1" {
		t.Errorf("Expected field key1='value1', got '%s'", entries[0].Fields["key1"])
	}

	// Non-string values should be converted to empty string
	orig := entry.Data["key2"]
	if _, ok := orig.(string); ok {
		t.Fatalf("Test setup error: entry.Data['key2'] should be non-string, got string with value %q", orig)
	}
	if entries[0].Fields["key2"] != "" {
		t.Errorf("Expected field key2='' (from non-string %T(%v)), got %q", orig, orig, entries[0].Fields["key2"])
	}
}

func TestGetLogBuffer(t *testing.T) {
	// Preserve and reset the global state for testing
	logBufferMu.Lock()
	origBuf := globalLogBuffer
	origInit := logBufferInitialized
	globalLogBuffer = nil
	logBufferInitialized = false
	logBufferMu.Unlock()

	defer func() {
		logBufferMu.Lock()
		globalLogBuffer = origBuf
		logBufferInitialized = origInit
		logBufferMu.Unlock()
	}()

	buf1 := GetLogBuffer()
	if buf1 == nil {
		t.Fatal("GetLogBuffer returned nil")
	}

	buf2 := GetLogBuffer()
	if buf1 != buf2 {
		t.Error("GetLogBuffer should return the same instance")
	}
}

func TestGetLogBufferProvider(t *testing.T) {
	// Preserve and reset the global state for testing
	logBufferMu.Lock()
	origBuf := globalLogBuffer
	origInit := logBufferInitialized
	globalLogBuffer = nil
	logBufferInitialized = false
	logBufferMu.Unlock()

	defer func() {
		logBufferMu.Lock()
		globalLogBuffer = origBuf
		logBufferInitialized = origInit
		logBufferMu.Unlock()
	}()

	provider := GetLogBufferProvider()
	if provider == nil {
		t.Fatal("GetLogBufferProvider returned nil")
	}

	// Verify we can call methods on the provider (type already enforced by return type)
	_ = provider.GetLast(10)
}

func TestInitLogBuffer(t *testing.T) {
	// Preserve and reset the global state for testing
	logBufferMu.Lock()
	origBuf := globalLogBuffer
	origInit := logBufferInitialized
	globalLogBuffer = nil
	logBufferInitialized = false
	logBufferMu.Unlock()

	defer func() {
		logBufferMu.Lock()
		globalLogBuffer = origBuf
		logBufferInitialized = origInit
		logBufferMu.Unlock()
	}()

	// This will add a hook to logrus, but we can't easily verify it.
	// Just ensure it doesn't panic when called.
	InitLogBuffer()
}

func TestLogBufferConcurrency(t *testing.T) {
	buf := NewLogBuffer(100)

	var wg sync.WaitGroup
	// Spawn multiple goroutines to add entries
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				buf.Add(types.LogBufferEntry{
					Timestamp: time.Now(),
					Level:     "info",
					Message:   "concurrent message",
				})
			}
		}(i)
	}

	// Spawn readers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				_ = buf.GetLast(10)
				_ = buf.Count()
			}
		}()
	}

	wg.Wait()

	// Just verify we didn't deadlock or panic
	count := buf.Count()
	if count == 0 || count > 100 {
		t.Errorf("Unexpected count after concurrent operations: %d", count)
	}
}
