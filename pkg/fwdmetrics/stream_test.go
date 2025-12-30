package fwdmetrics

import (
	"bytes"
	"io"
	"net/http"
	"testing"
)

// testableStream implements httpstream.Stream for testing with configurable behavior
type testableStream struct {
	readData   []byte
	readPos    int
	writeData  bytes.Buffer
	closeCalls int
	resetCalls int
	headers    http.Header
	identifier uint32
}

func (m *testableStream) Read(p []byte) (n int, err error) {
	if m.readPos >= len(m.readData) {
		return 0, io.EOF
	}
	n = copy(p, m.readData[m.readPos:])
	m.readPos += n
	return n, nil
}

func (m *testableStream) Write(p []byte) (n int, err error) {
	return m.writeData.Write(p)
}

func (m *testableStream) Close() error {
	m.closeCalls++
	return nil
}

func (m *testableStream) Reset() error {
	m.resetCalls++
	return nil
}

func (m *testableStream) Headers() http.Header {
	return m.headers
}

func (m *testableStream) Identifier() uint32 {
	return m.identifier
}

// TestMetricsStreamClose tests the Close method
func TestMetricsStreamClose(t *testing.T) {
	mock := &testableStream{}
	metrics := NewPortForwardMetrics("svc", "ns", "ctx", "pod", "127.0.0.1", "8080", "80")
	ms := NewMetricsStream(mock, metrics)

	err := ms.Close()
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if mock.closeCalls != 1 {
		t.Errorf("Expected 1 Close call, got %d", mock.closeCalls)
	}
}

// TestMetricsStreamReset tests the Reset method
func TestMetricsStreamReset(t *testing.T) {
	mock := &testableStream{}
	metrics := NewPortForwardMetrics("svc", "ns", "ctx", "pod", "127.0.0.1", "8080", "80")
	ms := NewMetricsStream(mock, metrics)

	err := ms.Reset()
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if mock.resetCalls != 1 {
		t.Errorf("Expected 1 Reset call, got %d", mock.resetCalls)
	}
}

// TestMetricsStreamHeaders tests the Headers method
func TestMetricsStreamHeaders(t *testing.T) {
	expectedHeaders := http.Header{
		"Content-Type":  []string{"application/json"},
		"X-Custom-Test": []string{"value1", "value2"},
	}
	mock := &testableStream{headers: expectedHeaders}
	metrics := NewPortForwardMetrics("svc", "ns", "ctx", "pod", "127.0.0.1", "8080", "80")
	ms := NewMetricsStream(mock, metrics)

	headers := ms.Headers()
	if headers == nil {
		t.Fatal("Expected headers, got nil")
	}

	if headers.Get("Content-Type") != "application/json" {
		t.Errorf("Expected Content-Type 'application/json', got '%s'", headers.Get("Content-Type"))
	}

	customValues := headers["X-Custom-Test"]
	if len(customValues) != 2 {
		t.Errorf("Expected 2 X-Custom-Test values, got %d", len(customValues))
	}
}

// TestMetricsStreamIdentifier tests the Identifier method
func TestMetricsStreamIdentifier(t *testing.T) {
	mock := &testableStream{identifier: 12345}
	metrics := NewPortForwardMetrics("svc", "ns", "ctx", "pod", "127.0.0.1", "8080", "80")
	ms := NewMetricsStream(mock, metrics)

	id := ms.Identifier()
	if id != 12345 {
		t.Errorf("Expected identifier 12345, got %d", id)
	}
}

// TestMetricsStreamMultipleOperations tests multiple operations in sequence
func TestMetricsStreamMultipleOperations(t *testing.T) {
	mock := &testableStream{
		readData:   []byte("test data"),
		headers:    http.Header{"Test": []string{"header"}},
		identifier: 42,
	}
	metrics := NewPortForwardMetrics("svc", "ns", "ctx", "pod", "127.0.0.1", "8080", "80")
	ms := NewMetricsStream(mock, metrics)

	// Test identifier
	if ms.Identifier() != 42 {
		t.Errorf("Identifier mismatch")
	}

	// Test headers
	if ms.Headers().Get("Test") != "header" {
		t.Errorf("Headers mismatch")
	}

	// Read some data
	buf := make([]byte, 100)
	n, _ := ms.Read(buf)
	if n != 9 {
		t.Errorf("Expected 9 bytes, got %d", n)
	}

	// Write some data
	_, _ = ms.Write([]byte("response"))

	// Reset
	_ = ms.Reset()
	if mock.resetCalls != 1 {
		t.Errorf("Expected 1 reset call")
	}

	// Close
	_ = ms.Close()
	if mock.closeCalls != 1 {
		t.Errorf("Expected 1 close call")
	}
}
