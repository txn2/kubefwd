package fwdmetrics

import (
	"errors"
	"net/http"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/httpstream"
)

// connTestStream implements httpstream.Stream for connection tests
type connTestStream struct {
	readErr  error
	writeErr error
	closeErr error
	resetErr error
	headers  http.Header
}

func (m *connTestStream) Read(p []byte) (n int, err error) {
	if m.readErr != nil {
		return 0, m.readErr
	}
	return len(p), nil
}

func (m *connTestStream) Write(p []byte) (n int, err error) {
	if m.writeErr != nil {
		return 0, m.writeErr
	}
	return len(p), nil
}

func (m *connTestStream) Close() error {
	return m.closeErr
}

func (m *connTestStream) Reset() error {
	return m.resetErr
}

func (m *connTestStream) Headers() http.Header {
	return m.headers
}

func (m *connTestStream) Identifier() uint32 {
	return 1
}

// mockConnection implements httpstream.Connection for testing
type mockConnection struct {
	createStreamErr error
	closeErr        error
	closeChan       chan bool
	createdStreams  []http.Header
	removedStreams  []httpstream.Stream
	idleTimeout     time.Duration
}

func newMockConnection() *mockConnection {
	return &mockConnection{
		closeChan:      make(chan bool),
		createdStreams: []http.Header{},
		removedStreams: []httpstream.Stream{},
	}
}

func (m *mockConnection) CreateStream(headers http.Header) (httpstream.Stream, error) {
	if m.createStreamErr != nil {
		return nil, m.createStreamErr
	}
	m.createdStreams = append(m.createdStreams, headers)
	return &connTestStream{headers: headers}, nil
}

func (m *mockConnection) Close() error {
	close(m.closeChan)
	return m.closeErr
}

func (m *mockConnection) CloseChan() <-chan bool {
	return m.closeChan
}

func (m *mockConnection) SetIdleTimeout(timeout time.Duration) {
	m.idleTimeout = timeout
}

func (m *mockConnection) RemoveStreams(streams ...httpstream.Stream) {
	m.removedStreams = append(m.removedStreams, streams...)
}

// Tests for MetricsConnection

func TestNewMetricsConnection(t *testing.T) {
	wrapped := newMockConnection()
	metrics := NewPortForwardMetrics("test-svc", "test-ns", "test-ctx", "test-pod", "127.0.0.1", "8080", "80")

	mc := NewMetricsConnection(wrapped, metrics)

	if mc == nil {
		t.Fatal("NewMetricsConnection returned nil")
	}
	if mc.wrapped != wrapped {
		t.Error("wrapped connection not set correctly")
	}
	if mc.metrics != metrics {
		t.Error("metrics not set correctly")
	}
}

func TestMetricsConnection_CreateStream_DataStream(t *testing.T) {
	wrapped := newMockConnection()
	metrics := NewPortForwardMetrics("test-svc", "test-ns", "test-ctx", "test-pod", "127.0.0.1", "8080", "80")
	mc := NewMetricsConnection(wrapped, metrics)

	headers := http.Header{}
	headers.Set(v1.StreamType, v1.StreamTypeData)

	stream, err := mc.CreateStream(headers)
	if err != nil {
		t.Fatalf("CreateStream failed: %v", err)
	}

	// Data streams should be wrapped with MetricsStream
	if _, ok := stream.(*MetricsStream); !ok {
		t.Error("data stream should be wrapped with MetricsStream")
	}
}

func TestMetricsConnection_CreateStream_ErrorStream(t *testing.T) {
	wrapped := newMockConnection()
	metrics := NewPortForwardMetrics("test-svc", "test-ns", "test-ctx", "test-pod", "127.0.0.1", "8080", "80")
	mc := NewMetricsConnection(wrapped, metrics)

	headers := http.Header{}
	headers.Set(v1.StreamType, v1.StreamTypeError)

	stream, err := mc.CreateStream(headers)
	if err != nil {
		t.Fatalf("CreateStream failed: %v", err)
	}

	// Error streams should NOT be wrapped
	if _, ok := stream.(*MetricsStream); ok {
		t.Error("error stream should not be wrapped with MetricsStream")
	}
}

func TestMetricsConnection_CreateStream_NilMetrics(t *testing.T) {
	wrapped := newMockConnection()
	mc := NewMetricsConnection(wrapped, nil)

	headers := http.Header{}
	headers.Set(v1.StreamType, v1.StreamTypeData)

	stream, err := mc.CreateStream(headers)
	if err != nil {
		t.Fatalf("CreateStream failed: %v", err)
	}

	// With nil metrics, stream should not be wrapped
	if _, ok := stream.(*MetricsStream); ok {
		t.Error("stream should not be wrapped when metrics is nil")
	}
}

func TestMetricsConnection_CreateStream_Error(t *testing.T) {
	wrapped := newMockConnection()
	wrapped.createStreamErr = errors.New("create stream failed")
	metrics := NewPortForwardMetrics("test-svc", "test-ns", "test-ctx", "test-pod", "127.0.0.1", "8080", "80")
	mc := NewMetricsConnection(wrapped, metrics)

	headers := http.Header{}
	_, err := mc.CreateStream(headers)
	if err == nil {
		t.Error("expected error from CreateStream")
	}
}

func TestMetricsConnection_Close(t *testing.T) {
	wrapped := newMockConnection()
	mc := NewMetricsConnection(wrapped, nil)

	err := mc.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Verify close channel is closed
	select {
	case <-wrapped.closeChan:
		// expected
	default:
		t.Error("close channel should be closed")
	}
}

func TestMetricsConnection_Close_Error(t *testing.T) {
	wrapped := newMockConnection()
	wrapped.closeErr = errors.New("close failed")
	mc := NewMetricsConnection(wrapped, nil)

	err := mc.Close()
	if err == nil {
		t.Error("expected error from Close")
	}
}

func TestMetricsConnection_CloseChan(t *testing.T) {
	wrapped := newMockConnection()
	mc := NewMetricsConnection(wrapped, nil)

	closeChan := mc.CloseChan()
	if closeChan != wrapped.closeChan {
		t.Error("CloseChan should return the wrapped connection's close channel")
	}
}

func TestMetricsConnection_SetIdleTimeout(t *testing.T) {
	wrapped := newMockConnection()
	mc := NewMetricsConnection(wrapped, nil)

	timeout := 5 * time.Minute
	mc.SetIdleTimeout(timeout)

	if wrapped.idleTimeout != timeout {
		t.Errorf("expected idle timeout %v, got %v", timeout, wrapped.idleTimeout)
	}
}

func TestMetricsConnection_RemoveStreams(t *testing.T) {
	wrapped := newMockConnection()
	metrics := NewPortForwardMetrics("test-svc", "test-ns", "test-ctx", "test-pod", "127.0.0.1", "8080", "80")
	mc := NewMetricsConnection(wrapped, metrics)

	// Create a regular stream
	regularStream := &connTestStream{}

	// Create a MetricsStream
	metricsStream := NewMetricsStream(&mockStream{}, metrics)

	mc.RemoveStreams(regularStream, metricsStream)

	if len(wrapped.removedStreams) != 2 {
		t.Fatalf("expected 2 removed streams, got %d", len(wrapped.removedStreams))
	}

	// First stream should be passed as-is
	if wrapped.removedStreams[0] != regularStream {
		t.Error("regular stream should be passed as-is")
	}

	// Second stream should be unwrapped
	if _, ok := wrapped.removedStreams[1].(*mockStream); !ok {
		t.Error("MetricsStream should be unwrapped before removal")
	}
}
