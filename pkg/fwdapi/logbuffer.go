package fwdapi

import (
	"sync"

	log "github.com/sirupsen/logrus"
	"github.com/txn2/kubefwd/pkg/fwdapi/types"
)

// Maximum allocation limit for log buffer (CodeQL CWE-770 compliance)
const maxLogBufferSize = 10000

// boundedSize returns size bounded to limit for memory safety
func boundedSize(size, limit int) int {
	if size <= 0 {
		return 0
	}
	if size > limit {
		return limit
	}
	return size
}

// LogBuffer is a ring buffer that stores log entries
// It implements types.LogBufferProvider
type LogBuffer struct {
	entries []types.LogBufferEntry
	size    int
	head    int
	count   int
	mu      sync.RWMutex
}

// NewLogBuffer creates a new log buffer with the specified size
func NewLogBuffer(size int) *LogBuffer {
	return &LogBuffer{
		entries: make([]types.LogBufferEntry, size),
		size:    size,
	}
}

// Add adds a log entry to the buffer
func (b *LogBuffer) Add(entry types.LogBufferEntry) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.entries[b.head] = entry
	b.head = (b.head + 1) % b.size
	if b.count < b.size {
		b.count++
	}
}

// GetLast returns the last n entries (most recent first)
func (b *LogBuffer) GetLast(n int) []types.LogBufferEntry {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if n <= 0 || b.count == 0 {
		return nil
	}
	if n > b.count {
		n = b.count
	}

	// Explicit upper bound for memory safety (CodeQL CWE-770)
	allocSize := boundedSize(n, maxLogBufferSize)
	if allocSize == 0 {
		return nil
	}

	result := make([]types.LogBufferEntry, allocSize)
	for i := 0; i < allocSize; i++ {
		// Start from most recent (head-1) and go backwards
		idx := (b.head - 1 - i + b.size) % b.size
		result[i] = b.entries[idx]
	}
	return result
}

// GetAll returns all entries (most recent first)
func (b *LogBuffer) GetAll() []types.LogBufferEntry {
	return b.GetLast(b.count)
}

// Count returns the number of entries in the buffer
func (b *LogBuffer) Count() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.count
}

// Clear clears all entries from the buffer
func (b *LogBuffer) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.head = 0
	b.count = 0
}

// Verify LogBuffer implements LogBufferProvider
var _ types.LogBufferProvider = (*LogBuffer)(nil)

// LogBufferHook is a logrus hook that writes to a LogBuffer
type LogBufferHook struct {
	buffer *LogBuffer
	levels []log.Level
}

// NewLogBufferHook creates a new LogBufferHook
func NewLogBufferHook(buffer *LogBuffer, levels []log.Level) *LogBufferHook {
	if levels == nil {
		levels = log.AllLevels
	}
	return &LogBufferHook{
		buffer: buffer,
		levels: levels,
	}
}

// Levels returns the log levels this hook handles
func (h *LogBufferHook) Levels() []log.Level {
	return h.levels
}

// Fire is called when a log entry is made
func (h *LogBufferHook) Fire(entry *log.Entry) error {
	fields := make(map[string]string)
	for k, v := range entry.Data {
		if s, ok := v.(string); ok {
			fields[k] = s
		} else {
			fields[k] = ""
		}
	}

	h.buffer.Add(types.LogBufferEntry{
		Timestamp: entry.Time,
		Level:     entry.Level.String(),
		Message:   entry.Message,
		Fields:    fields,
	})
	return nil
}

// Global log buffer instance
var (
	globalLogBuffer      *LogBuffer
	logBufferMu          sync.Mutex
	logBufferInitialized bool
)

// GetLogBuffer returns the global log buffer, creating it if necessary
func GetLogBuffer() *LogBuffer {
	logBufferMu.Lock()
	defer logBufferMu.Unlock()

	if logBufferInitialized {
		return globalLogBuffer
	}

	globalLogBuffer = NewLogBuffer(1000)
	logBufferInitialized = true
	return globalLogBuffer
}

// GetLogBufferProvider returns the global log buffer as a LogBufferProvider interface
// This avoids import cycles by allowing handlers to receive the provider
func GetLogBufferProvider() types.LogBufferProvider {
	return GetLogBuffer()
}

// InitLogBuffer initializes the log buffer and attaches it to logrus
func InitLogBuffer() {
	buffer := GetLogBuffer()
	hook := NewLogBufferHook(buffer, log.AllLevels)
	log.AddHook(hook)
}
