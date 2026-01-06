package fwdmetrics

import (
	"errors"
	"testing"

	"k8s.io/apimachinery/pkg/util/httpstream"
)

// mockDialer implements httpstream.Dialer for testing
type mockDialer struct {
	conn     httpstream.Connection
	protocol string
	err      error
}

func (m *mockDialer) Dial(protocols ...string) (httpstream.Connection, string, error) {
	if m.err != nil {
		return nil, "", m.err
	}
	return m.conn, m.protocol, nil
}

// Tests for MetricsDialer

func TestNewMetricsDialer(t *testing.T) {
	wrapped := &mockDialer{}
	metrics := NewPortForwardMetrics("test-svc", "test-ns", "test-ctx", "test-pod", "127.0.0.1", "8080", "80")

	md := NewMetricsDialer(wrapped, metrics)

	if md == nil {
		t.Fatal("NewMetricsDialer returned nil")
	}
	if md.wrapped != wrapped {
		t.Error("wrapped dialer not set correctly")
	}
	if md.metrics != metrics {
		t.Error("metrics not set correctly")
	}
}

func TestMetricsDialer_Dial_Success(t *testing.T) {
	mockConn := newMockConnection()
	wrapped := &mockDialer{
		conn:     mockConn,
		protocol: "v4.channel.k8s.io",
	}
	metrics := NewPortForwardMetrics("test-svc", "test-ns", "test-ctx", "test-pod", "127.0.0.1", "8080", "80")
	md := NewMetricsDialer(wrapped, metrics)

	conn, proto, err := md.Dial("v4.channel.k8s.io")
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}

	if proto != "v4.channel.k8s.io" {
		t.Errorf("expected protocol v4.channel.k8s.io, got %s", proto)
	}

	// Connection should be wrapped with MetricsConnection
	if _, ok := conn.(*MetricsConnection); !ok {
		t.Error("connection should be wrapped with MetricsConnection")
	}
}

func TestMetricsDialer_Dial_NilMetrics(t *testing.T) {
	mockConn := newMockConnection()
	wrapped := &mockDialer{
		conn:     mockConn,
		protocol: "v4.channel.k8s.io",
	}
	md := NewMetricsDialer(wrapped, nil)

	conn, proto, err := md.Dial("v4.channel.k8s.io")
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}

	if proto != "v4.channel.k8s.io" {
		t.Errorf("expected protocol v4.channel.k8s.io, got %s", proto)
	}

	// With nil metrics, connection should not be wrapped
	if _, ok := conn.(*MetricsConnection); ok {
		t.Error("connection should not be wrapped when metrics is nil")
	}
}

func TestMetricsDialer_Dial_Error(t *testing.T) {
	wrapped := &mockDialer{
		err: errors.New("dial failed"),
	}
	metrics := NewPortForwardMetrics("test-svc", "test-ns", "test-ctx", "test-pod", "127.0.0.1", "8080", "80")
	md := NewMetricsDialer(wrapped, metrics)

	_, _, err := md.Dial("v4.channel.k8s.io")
	if err == nil {
		t.Error("expected error from Dial")
	}
}

func TestMetricsDialer_Dial_NilConnection(t *testing.T) {
	wrapped := &mockDialer{
		conn:     nil,
		protocol: "",
	}
	metrics := NewPortForwardMetrics("test-svc", "test-ns", "test-ctx", "test-pod", "127.0.0.1", "8080", "80")
	md := NewMetricsDialer(wrapped, metrics)

	conn, proto, err := md.Dial("v4.channel.k8s.io")
	if err != nil {
		t.Fatalf("Dial should not fail with nil connection: %v", err)
	}

	if conn != nil {
		t.Error("connection should be nil")
	}
	if proto != "" {
		t.Error("protocol should be empty")
	}
}

func TestMetricsDialer_Dial_MultipleProtocols(t *testing.T) {
	mockConn := newMockConnection()
	wrapped := &mockDialer{
		conn:     mockConn,
		protocol: "v4.channel.k8s.io",
	}
	metrics := NewPortForwardMetrics("test-svc", "test-ns", "test-ctx", "test-pod", "127.0.0.1", "8080", "80")
	md := NewMetricsDialer(wrapped, metrics)

	// Test with multiple protocols (common in k8s port forwarding)
	conn, proto, err := md.Dial("v4.channel.k8s.io", "v3.channel.k8s.io")
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}

	if proto != "v4.channel.k8s.io" {
		t.Errorf("expected protocol v4.channel.k8s.io, got %s", proto)
	}

	if _, ok := conn.(*MetricsConnection); !ok {
		t.Error("connection should be wrapped with MetricsConnection")
	}
}
