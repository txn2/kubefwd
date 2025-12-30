package fwdmetrics

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestHTTPSnifferBasicRequest(t *testing.T) {
	sniffer := NewHTTPSniffer(10)

	// Simulate a basic HTTP request
	request := "GET /api/users HTTP/1.1\r\nHost: example.com\r\n\r\n"
	sniffer.SniffRequest([]byte(request))

	// Simulate a response
	response := "HTTP/1.1 200 OK\r\nContent-Length: 42\r\n\r\n"
	sniffer.SniffResponse([]byte(response))

	logs := sniffer.GetAllLogs()
	if len(logs) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(logs))
	}

	entry := logs[0]
	if entry.Method != "GET" {
		t.Errorf("expected method GET, got %s", entry.Method)
	}
	if entry.Path != "/api/users" {
		t.Errorf("expected path /api/users, got %s", entry.Path)
	}
	if entry.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", entry.StatusCode)
	}
	if entry.Size != 42 {
		t.Errorf("expected size 42, got %d", entry.Size)
	}
	if entry.Duration <= 0 {
		t.Errorf("expected positive duration, got %v", entry.Duration)
	}
}

func TestHTTPSnifferMultipleMethods(t *testing.T) {
	methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			sniffer := NewHTTPSniffer(10)

			request := method + " /test HTTP/1.1\r\n\r\n"
			sniffer.SniffRequest([]byte(request))

			response := "HTTP/1.1 200 OK\r\n\r\n"
			sniffer.SniffResponse([]byte(response))

			logs := sniffer.GetAllLogs()
			if len(logs) != 1 {
				t.Fatalf("expected 1 log entry, got %d", len(logs))
			}
			if logs[0].Method != method {
				t.Errorf("expected method %s, got %s", method, logs[0].Method)
			}
		})
	}
}

func TestHTTPSnifferRingBufferWrap(t *testing.T) {
	maxLogs := 5
	sniffer := NewHTTPSniffer(maxLogs)

	// Add more entries than the buffer can hold
	for i := 0; i < 10; i++ {
		request := fmt.Sprintf("GET /page%d HTTP/1.1\r\n\r\n", i)
		sniffer.SniffRequest([]byte(request))

		response := "HTTP/1.1 200 OK\r\n\r\n"
		sniffer.SniffResponse([]byte(response))
	}

	logs := sniffer.GetAllLogs()
	if len(logs) != maxLogs {
		t.Fatalf("expected %d log entries, got %d", maxLogs, len(logs))
	}

	// Should have entries 5-9 (the most recent 5)
	for i, log := range logs {
		expectedPath := "/page" + string(rune('0'+5+i))
		if log.Path != expectedPath {
			t.Errorf("entry %d: expected path %s, got %s", i, expectedPath, log.Path)
		}
	}
}

func TestHTTPSnifferRingBufferMemoryStability(t *testing.T) {
	maxLogs := 5
	sniffer := NewHTTPSniffer(maxLogs)

	// Verify the underlying slice capacity stays fixed
	initialCap := cap(sniffer.logs)

	// Add many entries
	for i := 0; i < 100; i++ {
		request := "GET /test HTTP/1.1\r\n\r\n"
		sniffer.SniffRequest([]byte(request))

		response := "HTTP/1.1 200 OK\r\n\r\n"
		sniffer.SniffResponse([]byte(response))
	}

	// Capacity should not have grown
	if cap(sniffer.logs) != initialCap {
		t.Errorf("ring buffer capacity grew from %d to %d (memory leak)", initialCap, cap(sniffer.logs))
	}

	if sniffer.LogCount() != maxLogs {
		t.Errorf("expected %d entries, got %d", maxLogs, sniffer.LogCount())
	}
}

func TestHTTPSnifferDisabled(t *testing.T) {
	sniffer := NewHTTPSniffer(10)
	sniffer.SetEnabled(false)

	request := "GET /test HTTP/1.1\r\n\r\n"
	sniffer.SniffRequest([]byte(request))

	response := "HTTP/1.1 200 OK\r\n\r\n"
	sniffer.SniffResponse([]byte(response))

	if sniffer.LogCount() != 0 {
		t.Errorf("expected 0 logs when disabled, got %d", sniffer.LogCount())
	}
}

func TestHTTPSnifferResponseWithoutRequest(t *testing.T) {
	sniffer := NewHTTPSniffer(10)

	// Send response without a preceding request
	response := "HTTP/1.1 404 Not Found\r\nContent-Length: 0\r\n\r\n"
	sniffer.SniffResponse([]byte(response))

	logs := sniffer.GetAllLogs()
	if len(logs) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(logs))
	}

	// Should still log the response, but without request details
	entry := logs[0]
	if entry.Method != "" {
		t.Errorf("expected empty method, got %s", entry.Method)
	}
	if entry.Path != "" {
		t.Errorf("expected empty path, got %s", entry.Path)
	}
	if entry.StatusCode != 404 {
		t.Errorf("expected status 404, got %d", entry.StatusCode)
	}
	if entry.Duration != 0 {
		t.Errorf("expected zero duration, got %v", entry.Duration)
	}
}

func TestHTTPSnifferNonHTTPData(t *testing.T) {
	sniffer := NewHTTPSniffer(10)

	// Send non-HTTP data
	sniffer.SniffRequest([]byte("This is not HTTP data\r\n"))
	sniffer.SniffResponse([]byte("Neither is this\r\n"))

	if sniffer.LogCount() != 0 {
		t.Errorf("expected 0 logs for non-HTTP data, got %d", sniffer.LogCount())
	}
}

func TestHTTPSnifferFragmentedRequest(t *testing.T) {
	sniffer := NewHTTPSniffer(10)

	// Send request in fragments
	sniffer.SniffRequest([]byte("GET /fra"))
	sniffer.SniffRequest([]byte("gmented HTTP/1.1\r\n\r\n"))

	response := "HTTP/1.1 200 OK\r\n\r\n"
	sniffer.SniffResponse([]byte(response))

	logs := sniffer.GetAllLogs()
	if len(logs) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(logs))
	}

	if logs[0].Path != "/fragmented" {
		t.Errorf("expected path /fragmented, got %s", logs[0].Path)
	}
}

func TestHTTPSnifferClearLogs(t *testing.T) {
	sniffer := NewHTTPSniffer(10)

	// Add some logs
	for i := 0; i < 5; i++ {
		sniffer.SniffRequest([]byte("GET /test HTTP/1.1\r\n\r\n"))
		sniffer.SniffResponse([]byte("HTTP/1.1 200 OK\r\n\r\n"))
	}

	if sniffer.LogCount() != 5 {
		t.Fatalf("expected 5 logs, got %d", sniffer.LogCount())
	}

	sniffer.ClearLogs()

	if sniffer.LogCount() != 0 {
		t.Errorf("expected 0 logs after clear, got %d", sniffer.LogCount())
	}

	// Verify we can still add logs after clearing
	sniffer.SniffRequest([]byte("GET /new HTTP/1.1\r\n\r\n"))
	sniffer.SniffResponse([]byte("HTTP/1.1 200 OK\r\n\r\n"))

	if sniffer.LogCount() != 1 {
		t.Errorf("expected 1 log after adding post-clear, got %d", sniffer.LogCount())
	}
}

func TestHTTPSnifferGetLogs(t *testing.T) {
	sniffer := NewHTTPSniffer(10)

	// Add 5 logs
	for i := 0; i < 5; i++ {
		sniffer.SniffRequest([]byte(fmt.Sprintf("GET /page%d HTTP/1.1\r\n\r\n", i)))
		sniffer.SniffResponse([]byte("HTTP/1.1 200 OK\r\n\r\n"))
	}

	// Get last 3
	logs := sniffer.GetLogs(3)
	if len(logs) != 3 {
		t.Fatalf("expected 3 logs, got %d", len(logs))
	}

	// Should be the most recent 3 in order
	expectedPaths := []string{"/page2", "/page3", "/page4"}
	for i, log := range logs {
		if log.Path != expectedPaths[i] {
			t.Errorf("log %d: expected path %s, got %s", i, expectedPaths[i], log.Path)
		}
	}
}

func TestHTTPSnifferGetLogsZero(t *testing.T) {
	sniffer := NewHTTPSniffer(10)

	logs := sniffer.GetLogs(5)
	if logs != nil {
		t.Errorf("expected nil for empty sniffer, got %v", logs)
	}
}

func TestHTTPSnifferVariousStatusCodes(t *testing.T) {
	testCases := []struct {
		statusLine string
		expected   int
	}{
		{"HTTP/1.1 200 OK\r\n\r\n", 200},
		{"HTTP/1.1 201 Created\r\n\r\n", 201},
		{"HTTP/1.1 301 Moved Permanently\r\n\r\n", 301},
		{"HTTP/1.1 400 Bad Request\r\n\r\n", 400},
		{"HTTP/1.1 404 Not Found\r\n\r\n", 404},
		{"HTTP/1.1 500 Internal Server Error\r\n\r\n", 500},
		{"HTTP/1.0 200 OK\r\n\r\n", 200}, // HTTP/1.0
	}

	for _, tc := range testCases {
		t.Run(tc.statusLine, func(t *testing.T) {
			sniffer := NewHTTPSniffer(10)

			sniffer.SniffRequest([]byte("GET /test HTTP/1.1\r\n\r\n"))
			sniffer.SniffResponse([]byte(tc.statusLine))

			logs := sniffer.GetAllLogs()
			if len(logs) != 1 {
				t.Fatalf("expected 1 log, got %d", len(logs))
			}
			if logs[0].StatusCode != tc.expected {
				t.Errorf("expected status %d, got %d", tc.expected, logs[0].StatusCode)
			}
		})
	}
}

func TestHTTPSnifferContentLengthVariations(t *testing.T) {
	testCases := []struct {
		name     string
		response string
		expected int64
	}{
		{"standard", "HTTP/1.1 200 OK\r\nContent-Length: 1234\r\n\r\n", 1234},
		{"lowercase", "HTTP/1.1 200 OK\r\ncontent-length: 5678\r\n\r\n", 5678},
		{"mixed case", "HTTP/1.1 200 OK\r\nContent-length: 999\r\n\r\n", 999},
		{"no content-length", "HTTP/1.1 200 OK\r\n\r\n", 0},
		{"zero", "HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n", 0},
		{"large", "HTTP/1.1 200 OK\r\nContent-Length: 9999999999\r\n\r\n", 9999999999},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sniffer := NewHTTPSniffer(10)

			sniffer.SniffRequest([]byte("GET /test HTTP/1.1\r\n\r\n"))
			sniffer.SniffResponse([]byte(tc.response))

			logs := sniffer.GetAllLogs()
			if len(logs) != 1 {
				t.Fatalf("expected 1 log, got %d", len(logs))
			}
			if logs[0].Size != tc.expected {
				t.Errorf("expected size %d, got %d", tc.expected, logs[0].Size)
			}
		})
	}
}

func TestHTTPSnifferInvalidContentLength(t *testing.T) {
	sniffer := NewHTTPSniffer(10)

	// Invalid content-length should result in size=0
	response := "HTTP/1.1 200 OK\r\nContent-Length: not-a-number\r\n\r\n"
	sniffer.SniffRequest([]byte("GET /test HTTP/1.1\r\n\r\n"))
	sniffer.SniffResponse([]byte(response))

	logs := sniffer.GetAllLogs()
	if len(logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(logs))
	}
	if logs[0].Size != 0 {
		t.Errorf("expected size 0 for invalid content-length, got %d", logs[0].Size)
	}
}

func TestHTTPSnifferDurationTracking(t *testing.T) {
	sniffer := NewHTTPSniffer(10)

	sniffer.SniffRequest([]byte("GET /test HTTP/1.1\r\n\r\n"))

	// Wait a bit to get measurable duration
	time.Sleep(10 * time.Millisecond)

	sniffer.SniffResponse([]byte("HTTP/1.1 200 OK\r\n\r\n"))

	logs := sniffer.GetAllLogs()
	if len(logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(logs))
	}

	// Duration should be at least 10ms
	if logs[0].Duration < 10*time.Millisecond {
		t.Errorf("expected duration >= 10ms, got %v", logs[0].Duration)
	}
}

func TestHTTPSnifferLongPath(t *testing.T) {
	sniffer := NewHTTPSniffer(10)

	// Test with a long path (but still within buffer limits)
	longPath := "/" + strings.Repeat("a", 200)
	request := "GET " + longPath + " HTTP/1.1\r\n\r\n"
	sniffer.SniffRequest([]byte(request))
	sniffer.SniffResponse([]byte("HTTP/1.1 200 OK\r\n\r\n"))

	logs := sniffer.GetAllLogs()
	if len(logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(logs))
	}
	if logs[0].Path != longPath {
		t.Errorf("path mismatch for long path")
	}
}

func TestHTTPSnifferIsEnabled(t *testing.T) {
	sniffer := NewHTTPSniffer(10)

	if !sniffer.IsEnabled() {
		t.Error("expected sniffer to be enabled by default")
	}

	sniffer.SetEnabled(false)
	if sniffer.IsEnabled() {
		t.Error("expected sniffer to be disabled")
	}

	sniffer.SetEnabled(true)
	if !sniffer.IsEnabled() {
		t.Error("expected sniffer to be re-enabled")
	}
}

func TestHTTPSnifferChronologicalOrder(t *testing.T) {
	sniffer := NewHTTPSniffer(10)

	// Add logs with identifiable paths
	for i := 0; i < 5; i++ {
		path := fmt.Sprintf("/order%d", i)
		sniffer.SniffRequest([]byte("GET " + path + " HTTP/1.1\r\n\r\n"))
		sniffer.SniffResponse([]byte("HTTP/1.1 200 OK\r\n\r\n"))
	}

	logs := sniffer.GetAllLogs()

	// Verify chronological order (oldest first)
	for i, log := range logs {
		expectedPath := fmt.Sprintf("/order%d", i)
		if log.Path != expectedPath {
			t.Errorf("log %d: expected %s, got %s (order wrong)", i, expectedPath, log.Path)
		}
	}
}
