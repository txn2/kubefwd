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

// HTTPSniffer detects and parses HTTP requests/responses from stream data.
//
// LIMITATION: Request/response pairing is best-effort. For pipelined or concurrent
// HTTP requests over the same connection, responses may be incorrectly paired with
// requests. The sniffer uses a single currentReq field to track the most recent
// request, so only sequential request-response pairs are guaranteed to be accurate.
// This is acceptable for debugging/monitoring purposes but should not be relied
// upon for precise request-response correlation in high-concurrency scenarios.
type HTTPSniffer struct {
	mu         sync.Mutex
	reqBuffer  *bytes.Buffer
	resBuffer  *bytes.Buffer
	currentReq *httpRequest
	// Ring buffer: fixed-size array with index-based wrap to avoid memory leaks
	logs     []HTTPLogEntry
	logIndex int // Next write position
	logCount int // Number of valid entries (up to maxLogs)
	maxLogs  int
	enabled  bool
}

// HTTP detection patterns
var (
	httpRequestRegex  = regexp.MustCompile(`^(GET|POST|PUT|DELETE|PATCH|HEAD|OPTIONS|CONNECT|TRACE)\s+(\S+)\s+HTTP/1\.[01]`)
	httpResponseRegex = regexp.MustCompile(`^HTTP/1\.[01]\s+(\d{3})`)
	contentLengthRe   = regexp.MustCompile(`(?i)Content-Length:\s*(\d+)`)
)

// NewHTTPSniffer creates a new HTTP sniffer with the specified log capacity.
// The log buffer is pre-allocated to avoid memory growth during operation.
func NewHTTPSniffer(maxLogs int) *HTTPSniffer {
	if maxLogs <= 0 {
		maxLogs = 50
	}
	return &HTTPSniffer{
		reqBuffer: bytes.NewBuffer(make([]byte, 0, 1024)),
		resBuffer: bytes.NewBuffer(make([]byte, 0, 1024)),
		logs:      make([]HTTPLogEntry, maxLogs), // Pre-allocate fixed size
		logIndex:  0,
		logCount:  0,
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
//
//goland:noinspection DuplicatedCode
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
//
//goland:noinspection DuplicatedCode
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
			statusCode, err := strconv.Atoi(match[1])
			if err != nil {
				// Invalid status code format - shouldn't happen with regex match but handle gracefully
				statusCode = 0
			}

			// Try to get content length from headers (best-effort, size=0 if not found or invalid)
			var size int64
			if clMatch := contentLengthRe.FindSubmatch(bufData); clMatch != nil {
				if parsed, err := strconv.ParseInt(string(clMatch[1]), 10, 64); err == nil {
					size = parsed
				}
				// On parse error, size remains 0 (unknown size)
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

// addLog adds a log entry to the ring buffer (must be called with lock held).
// Uses index-based ring buffer to avoid memory leaks from slice retention.
func (s *HTTPSniffer) addLog(entry HTTPLogEntry) {
	s.logs[s.logIndex] = entry
	s.logIndex = (s.logIndex + 1) % s.maxLogs
	if s.logCount < s.maxLogs {
		s.logCount++
	}
}

// GetLogs returns the most recent log entries in chronological order
func (s *HTTPSniffer) GetLogs(count int) []HTTPLogEntry {
	s.mu.Lock()
	defer s.mu.Unlock()

	if count <= 0 || count > s.logCount {
		count = s.logCount
	}

	if count == 0 {
		return nil
	}

	result := make([]HTTPLogEntry, count)
	// Calculate start position for reading 'count' most recent entries
	// logIndex points to next write position, so most recent is at logIndex-1
	startIdx := (s.logIndex - count + s.maxLogs) % s.maxLogs
	for i := 0; i < count; i++ {
		result[i] = s.logs[(startIdx+i)%s.maxLogs]
	}
	return result
}

// GetAllLogs returns all log entries in chronological order
func (s *HTTPSniffer) GetAllLogs() []HTTPLogEntry {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.logCount == 0 {
		return nil
	}

	result := make([]HTTPLogEntry, s.logCount)
	// Start from oldest entry
	startIdx := (s.logIndex - s.logCount + s.maxLogs) % s.maxLogs
	for i := 0; i < s.logCount; i++ {
		result[i] = s.logs[(startIdx+i)%s.maxLogs]
	}
	return result
}

// ClearLogs removes all log entries
func (s *HTTPSniffer) ClearLogs() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logIndex = 0
	s.logCount = 0
	// Zero out entries to allow GC of any referenced objects
	for i := range s.logs {
		s.logs[i] = HTTPLogEntry{}
	}
}

// LogCount returns the number of log entries
func (s *HTTPSniffer) LogCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.logCount
}
