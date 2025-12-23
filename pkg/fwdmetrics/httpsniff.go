package fwdmetrics

import (
	"bytes"
	"regexp"
	"strconv"
	"sync"
	"time"
)

// HTTPLogEntry represents an HTTP request/response log entry
type HTTPLogEntry struct {
	Timestamp  time.Time
	Method     string
	Path       string
	StatusCode int
	Duration   time.Duration
	Size       int64
}

// httpRequest tracks an in-flight request for timing
type httpRequest struct {
	Method    string
	Path      string
	StartTime time.Time
}

// HTTPSniffer detects and parses HTTP requests/responses from stream data
type HTTPSniffer struct {
	mu         sync.Mutex
	reqBuffer  *bytes.Buffer
	resBuffer  *bytes.Buffer
	currentReq *httpRequest
	logs       []HTTPLogEntry
	maxLogs    int
	enabled    bool
}

// HTTP detection patterns
var (
	httpRequestRegex  = regexp.MustCompile(`^(GET|POST|PUT|DELETE|PATCH|HEAD|OPTIONS|CONNECT|TRACE)\s+(\S+)\s+HTTP/1\.[01]`)
	httpResponseRegex = regexp.MustCompile(`^HTTP/1\.[01]\s+(\d{3})`)
	contentLengthRe   = regexp.MustCompile(`(?i)Content-Length:\s*(\d+)`)
)

// NewHTTPSniffer creates a new HTTP sniffer with the specified log capacity
func NewHTTPSniffer(maxLogs int) *HTTPSniffer {
	if maxLogs <= 0 {
		maxLogs = 50
	}
	return &HTTPSniffer{
		reqBuffer: bytes.NewBuffer(make([]byte, 0, 1024)),
		resBuffer: bytes.NewBuffer(make([]byte, 0, 1024)),
		logs:      make([]HTTPLogEntry, 0, maxLogs),
		maxLogs:   maxLogs,
		enabled:   true,
	}
}

// SetEnabled enables or disables HTTP sniffing
func (s *HTTPSniffer) SetEnabled(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enabled = enabled
	if !enabled {
		s.reqBuffer.Reset()
		s.resBuffer.Reset()
		s.currentReq = nil
	}
}

// IsEnabled returns whether sniffing is enabled
func (s *HTTPSniffer) IsEnabled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.enabled
}

// SniffRequest analyzes data being sent TO the pod (outgoing request)
func (s *HTTPSniffer) SniffRequest(data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.enabled || len(data) == 0 {
		return
	}

	// Append to request buffer
	s.reqBuffer.Write(data)

	// Only check first 512 bytes for the request line
	if s.reqBuffer.Len() > 512 {
		s.reqBuffer.Truncate(512)
	}

	bufData := s.reqBuffer.Bytes()

	// Look for end of first line
	idx := bytes.Index(bufData, []byte("\r\n"))
	if idx < 0 {
		idx = bytes.Index(bufData, []byte("\n"))
	}

	if idx > 0 {
		line := string(bufData[:idx])

		if match := httpRequestRegex.FindStringSubmatch(line); match != nil {
			// Found an HTTP request, track it
			s.currentReq = &httpRequest{
				Method:    match[1],
				Path:      match[2],
				StartTime: time.Now(),
			}
			s.reqBuffer.Reset()
		} else if s.reqBuffer.Len() > 256 {
			// Not looking like HTTP, reset
			s.reqBuffer.Reset()
		}
	}
}

// SniffResponse analyzes data being received FROM the pod (incoming response)
func (s *HTTPSniffer) SniffResponse(data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.enabled || len(data) == 0 {
		return
	}

	// Append to response buffer
	s.resBuffer.Write(data)

	// Only check first 512 bytes for the status line
	if s.resBuffer.Len() > 512 {
		s.resBuffer.Truncate(512)
	}

	bufData := s.resBuffer.Bytes()

	// Look for end of first line
	idx := bytes.Index(bufData, []byte("\r\n"))
	if idx < 0 {
		idx = bytes.Index(bufData, []byte("\n"))
	}

	if idx > 0 {
		line := string(bufData[:idx])

		if match := httpResponseRegex.FindStringSubmatch(line); match != nil {
			statusCode, _ := strconv.Atoi(match[1])

			// Try to get content length from headers
			var size int64
			if clMatch := contentLengthRe.FindSubmatch(bufData); clMatch != nil {
				size, _ = strconv.ParseInt(string(clMatch[1]), 10, 64)
			}

			// Create log entry
			entry := HTTPLogEntry{
				Timestamp:  time.Now(),
				StatusCode: statusCode,
				Size:       size,
			}

			// If we have a pending request, pair them
			if s.currentReq != nil {
				entry.Method = s.currentReq.Method
				entry.Path = s.currentReq.Path
				entry.Duration = time.Since(s.currentReq.StartTime)
				s.currentReq = nil
			}

			s.addLog(entry)
			s.resBuffer.Reset()
		} else if s.resBuffer.Len() > 256 {
			// Not looking like HTTP, reset
			s.resBuffer.Reset()
		}
	}
}

// addLog adds a log entry to the ring buffer (must be called with lock held)
func (s *HTTPSniffer) addLog(entry HTTPLogEntry) {
	s.logs = append(s.logs, entry)
	if len(s.logs) > s.maxLogs {
		s.logs = s.logs[len(s.logs)-s.maxLogs:]
	}
}

// GetLogs returns the most recent log entries
func (s *HTTPSniffer) GetLogs(count int) []HTTPLogEntry {
	s.mu.Lock()
	defer s.mu.Unlock()

	if count <= 0 || count > len(s.logs) {
		count = len(s.logs)
	}

	if count == 0 {
		return nil
	}

	start := len(s.logs) - count
	result := make([]HTTPLogEntry, count)
	copy(result, s.logs[start:])
	return result
}

// GetAllLogs returns all log entries
func (s *HTTPSniffer) GetAllLogs() []HTTPLogEntry {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.logs) == 0 {
		return nil
	}

	result := make([]HTTPLogEntry, len(s.logs))
	copy(result, s.logs)
	return result
}

// ClearLogs removes all log entries
func (s *HTTPSniffer) ClearLogs() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logs = s.logs[:0]
}

// LogCount returns the number of log entries
func (s *HTTPSniffer) LogCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.logs)
}
