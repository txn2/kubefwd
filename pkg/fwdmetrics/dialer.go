package fwdmetrics

import (
	"k8s.io/apimachinery/pkg/util/httpstream"
)

// MetricsDialer wraps httpstream.Dialer to inject metrics tracking
type MetricsDialer struct {
	wrapped httpstream.Dialer
	metrics *PortForwardMetrics
}

// NewMetricsDialer creates a new metrics-enabled dialer
func NewMetricsDialer(wrapped httpstream.Dialer, metrics *PortForwardMetrics) *MetricsDialer {
	return &MetricsDialer{
		wrapped: wrapped,
		metrics: metrics,
	}
}

// Dial creates a new connection and wraps it with metrics tracking
func (md *MetricsDialer) Dial(protocols ...string) (httpstream.Connection, string, error) {
	conn, proto, err := md.wrapped.Dial(protocols...)
	if err != nil || conn == nil {
		return conn, proto, err
	}

	if md.metrics != nil {
		return NewMetricsConnection(conn, md.metrics), proto, nil
	}

	return conn, proto, nil
}
