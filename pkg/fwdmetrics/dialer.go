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
