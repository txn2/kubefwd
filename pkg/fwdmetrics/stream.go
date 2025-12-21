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

	"k8s.io/apimachinery/pkg/util/httpstream"
)

// MetricsStream wraps httpstream.Stream to track bytes transferred.
// IMPORTANT: We do NOT embed httpstream.Stream because if the underlying
// stream implements io.ReaderFrom or io.WriterTo, io.Copy would use those
// directly and bypass our Read/Write methods. By storing as a field and
// explicitly implementing only the required interface methods, we force
// io.Copy to use our instrumented Read/Write.
type MetricsStream struct {
	stream  httpstream.Stream // NOT embedded - stored as field
	metrics *PortForwardMetrics
}

// Verify MetricsStream implements httpstream.Stream
var _ httpstream.Stream = (*MetricsStream)(nil)

// NewMetricsStream wraps a stream with metrics tracking
func NewMetricsStream(stream httpstream.Stream, metrics *PortForwardMetrics) *MetricsStream {
	return &MetricsStream{
		stream:  stream,
		metrics: metrics,
	}
}

// Read reads data from the stream and tracks bytes received
func (ms *MetricsStream) Read(p []byte) (n int, err error) {
	n, err = ms.stream.Read(p)
	if n > 0 && ms.metrics != nil {
		ms.metrics.AddBytesIn(uint64(n))
	}
	return n, err
}

// Write writes data to the stream and tracks bytes sent
func (ms *MetricsStream) Write(p []byte) (n int, err error) {
	n, err = ms.stream.Write(p)
	if n > 0 && ms.metrics != nil {
		ms.metrics.AddBytesOut(uint64(n))
	}
	return n, err
}

// Close closes the stream
func (ms *MetricsStream) Close() error {
	return ms.stream.Close()
}

// Reset closes both directions of the stream
func (ms *MetricsStream) Reset() error {
	return ms.stream.Reset()
}

// Headers returns the headers used to create the stream
func (ms *MetricsStream) Headers() http.Header {
	return ms.stream.Headers()
}

// Identifier returns the stream's ID
func (ms *MetricsStream) Identifier() uint32 {
	return ms.stream.Identifier()
}
