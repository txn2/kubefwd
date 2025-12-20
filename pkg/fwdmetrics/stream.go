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
	"k8s.io/apimachinery/pkg/util/httpstream"
)

// MetricsStream wraps httpstream.Stream to track bytes transferred
type MetricsStream struct {
	httpstream.Stream
	metrics *PortForwardMetrics
}

// NewMetricsStream wraps a stream with metrics tracking
func NewMetricsStream(stream httpstream.Stream, metrics *PortForwardMetrics) *MetricsStream {
	return &MetricsStream{
		Stream:  stream,
		metrics: metrics,
	}
}

// Read reads data from the stream and tracks bytes received
func (ms *MetricsStream) Read(p []byte) (n int, err error) {
	n, err = ms.Stream.Read(p)
	if n > 0 && ms.metrics != nil {
		ms.metrics.AddBytesIn(uint64(n))
	}
	return n, err
}

// Write writes data to the stream and tracks bytes sent
func (ms *MetricsStream) Write(p []byte) (n int, err error) {
	n, err = ms.Stream.Write(p)
	if n > 0 && ms.metrics != nil {
		ms.metrics.AddBytesOut(uint64(n))
	}
	return n, err
}
