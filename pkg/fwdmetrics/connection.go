/*
Copyright 2018-2024 Craig Johnston <cjimti@gmail.com>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package fwdmetrics

import (
	"net/http"
	"time"

	api "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/httpstream"
)

// MetricsConnection wraps httpstream.Connection to wrap created streams with metrics
type MetricsConnection struct {
	wrapped httpstream.Connection
	metrics *PortForwardMetrics
}

// NewMetricsConnection wraps a connection with metrics tracking
func NewMetricsConnection(conn httpstream.Connection, metrics *PortForwardMetrics) *MetricsConnection {
	return &MetricsConnection{
		wrapped: conn,
		metrics: metrics,
	}
}

// CreateStream creates a new stream and wraps it with metrics tracking if it's a data stream
func (mc *MetricsConnection) CreateStream(headers http.Header) (httpstream.Stream, error) {
	stream, err := mc.wrapped.CreateStream(headers)
	if err != nil {
		return nil, err
	}

	// Only wrap data streams, not error streams
	streamType := headers.Get(api.StreamType)
	if streamType == api.StreamTypeData && mc.metrics != nil {
		return NewMetricsStream(stream, mc.metrics), nil
	}

	return stream, nil
}

// Close closes the connection
func (mc *MetricsConnection) Close() error {
	return mc.wrapped.Close()
}

// CloseChan returns a channel that is closed when the connection is closed
func (mc *MetricsConnection) CloseChan() <-chan bool {
	return mc.wrapped.CloseChan()
}

// SetIdleTimeout sets the idle timeout for the connection
func (mc *MetricsConnection) SetIdleTimeout(timeout time.Duration) {
	mc.wrapped.SetIdleTimeout(timeout)
}

// RemoveStreams removes the specified streams from the connection
func (mc *MetricsConnection) RemoveStreams(streams ...httpstream.Stream) {
	// Unwrap MetricsStreams before delegating
	unwrapped := make([]httpstream.Stream, len(streams))
	for i, s := range streams {
		if ms, ok := s.(*MetricsStream); ok {
			unwrapped[i] = ms.stream
		} else {
			unwrapped[i] = s
		}
	}
	mc.wrapped.RemoveStreams(unwrapped...)
}
