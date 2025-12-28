package fwdmetrics

import (
	"fmt"
	"net/http"

	log "github.com/sirupsen/logrus"
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
	// Panic recovery to prevent metrics/sniffing issues from crashing the stream
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("MetricsStream Read panic recovered: %v", r)
			// If we panicked after reading, return what we read with an error
			if n == 0 {
				err = fmt.Errorf("metrics stream panic: %v", r)
			}
		}
	}()

	n, err = ms.stream.Read(p)
	if n > 0 && ms.metrics != nil {
		ms.metrics.AddBytesIn(uint64(n))
		// Sniff for HTTP responses (data from pod)
		if sniffer := ms.metrics.GetHTTPSniffer(); sniffer != nil {
			sniffer.SniffResponse(p[:n])
		}
	}
	return n, err
}

// Write writes data to the stream and tracks bytes sent
func (ms *MetricsStream) Write(p []byte) (n int, err error) {
	// Panic recovery to prevent metrics/sniffing issues from crashing the stream
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("MetricsStream Write panic recovered: %v", r)
			// If we panicked after writing, return what we wrote with an error
			if n == 0 {
				err = fmt.Errorf("metrics stream panic: %v", r)
			}
		}
	}()

	n, err = ms.stream.Write(p)
	if n > 0 && ms.metrics != nil {
		ms.metrics.AddBytesOut(uint64(n))
		// Sniff for HTTP requests (data to pod)
		if sniffer := ms.metrics.GetHTTPSniffer(); sniffer != nil {
			sniffer.SniffRequest(p[:n])
		}
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
