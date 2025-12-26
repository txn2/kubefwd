package fwdmetrics

import (
	"bytes"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"
)

// mockStream implements httpstream.Stream for testing
type mockStream struct {
	readData  []byte
	readPos   int
	writeData bytes.Buffer
}

func (m *mockStream) Read(p []byte) (n int, err error) {
	if m.readPos >= len(m.readData) {
		return 0, io.EOF
	}
	n = copy(p, m.readData[m.readPos:])
	m.readPos += n
	return n, nil
}

func (m *mockStream) Write(p []byte) (n int, err error) {
	return m.writeData.Write(p)
}

func (m *mockStream) Close() error         { return nil }
func (m *mockStream) Reset() error         { return nil }
func (m *mockStream) Headers() http.Header { return nil }
func (m *mockStream) Identifier() uint32   { return 0 }

func TestMetricsStreamTracksBytes(t *testing.T) {
	// Create metrics
	metrics := NewPortForwardMetrics("svc", "ns", "ctx", "pod", "127.0.0.1", "8080", "80")

	// Create mock stream with test data
	mock := &mockStream{readData: []byte("hello world")}

	// Wrap with MetricsStream
	ms := NewMetricsStream(mock, metrics)

	// Read data
	buf := make([]byte, 100)
	n, _ := ms.Read(buf)

	t.Logf("Read %d bytes, BytesIn=%d", n, metrics.GetBytesIn())

	// Verify bytes were tracked
	if metrics.GetBytesIn() != uint64(n) {
		t.Errorf("Expected BytesIn=%d, got %d", n, metrics.GetBytesIn())
	}

	// Write data
	data := []byte("response data")
	n, _ = ms.Write(data)

	t.Logf("Wrote %d bytes, BytesOut=%d", n, metrics.GetBytesOut())

	// Verify bytes were tracked
	if metrics.GetBytesOut() != uint64(n) {
		t.Errorf("Expected BytesOut=%d, got %d", n, metrics.GetBytesOut())
	}
}

func TestRateCalculatorRequiresTwoSamples(t *testing.T) {
	rc := NewRateCalculator(60)

	// No samples - should return 0
	rateIn, rateOut := rc.GetInstantRate()
	if rateIn != 0 || rateOut != 0 {
		t.Errorf("Expected 0 rate with no samples, got in=%.2f out=%.2f", rateIn, rateOut)
	}

	// One sample - should still return 0
	rc.AddSample(1000, 500, time.Now())
	t.Logf("After 1 sample: SampleCount=%d", rc.SampleCount())

	rateIn, rateOut = rc.GetInstantRate()
	if rateIn != 0 || rateOut != 0 {
		t.Errorf("Expected 0 rate with 1 sample, got in=%.2f out=%.2f", rateIn, rateOut)
	}

	// Two samples - should return non-zero
	time.Sleep(100 * time.Millisecond)
	rc.AddSample(2000, 1000, time.Now())
	t.Logf("After 2 samples: SampleCount=%d", rc.SampleCount())

	rateIn, rateOut = rc.GetInstantRate()
	t.Logf("Rate with 2 samples: in=%.2f out=%.2f", rateIn, rateOut)

	if rateIn <= 0 || rateOut <= 0 {
		t.Errorf("Expected positive rate with 2 samples, got in=%.2f out=%.2f", rateIn, rateOut)
	}
}

func TestRegistrySamplesAllPortForwards(t *testing.T) {
	// Reset global registry for test
	registryOnce = sync.Once{}
	globalRegistry = nil

	reg := GetRegistry()

	// Create and register a port forward
	pf := NewPortForwardMetrics("svc", "ns", "ctx", "pod", "127.0.0.1", "8080", "80")
	serviceKey := "svc.ns.ctx"
	reg.RegisterPortForward(serviceKey, pf)

	// Verify it's registered
	if reg.ServiceCount() != 1 {
		t.Errorf("Expected 1 service, got %d", reg.ServiceCount())
	}
	if reg.PortForwardCount() != 1 {
		t.Errorf("Expected 1 port forward, got %d", reg.PortForwardCount())
	}

	// Simulate some bytes
	pf.AddBytesIn(1000)
	pf.AddBytesOut(500)

	t.Logf("Before sampling: BytesIn=%d, BytesOut=%d, SampleCount=%d",
		pf.GetBytesIn(), pf.GetBytesOut(), pf.rateCalc.SampleCount())

	// Start registry and let it sample
	reg.Start()
	time.Sleep(1500 * time.Millisecond) // Wait for at least 1 sample

	sampleCount := pf.rateCalc.SampleCount()
	t.Logf("After 1.5s: SampleCount=%d", sampleCount)

	// Check that sample was recorded
	if sampleCount < 1 {
		t.Errorf("Expected at least 1 sample, got %d", sampleCount)
	}

	// Wait for another sample
	time.Sleep(1000 * time.Millisecond)
	sampleCount = pf.rateCalc.SampleCount()
	t.Logf("After 2.5s: SampleCount=%d", sampleCount)

	if sampleCount < 2 {
		t.Errorf("Expected at least 2 samples, got %d", sampleCount)
	}

	// Now rate should be calculable
	snapshot := pf.GetSnapshot()
	t.Logf("Snapshot: BytesIn=%d, RateIn=%.2f", snapshot.BytesIn, snapshot.RateIn)

	if snapshot.BytesIn != 1000 {
		t.Errorf("Expected BytesIn=1000, got %d", snapshot.BytesIn)
	}

	reg.Stop()
}

func TestEndToEndMetricsFlow(t *testing.T) {
	// This test simulates the full flow:
	// 1. Create metrics and register with registry
	// 2. Wrap a stream and transfer data
	// 3. Let registry sample
	// 4. Verify snapshots have correct data

	registryOnce = sync.Once{}
	globalRegistry = nil
	reg := GetRegistry()
	reg.Start()
	defer reg.Stop()

	// Create metrics
	pf := NewPortForwardMetrics("testsvc", "testns", "testctx", "testpod", "127.0.0.1", "9090", "90")
	reg.RegisterPortForward("testsvc.testns.testctx", pf)

	t.Logf("Registered port forward. Services=%d, PortForwards=%d",
		reg.ServiceCount(), reg.PortForwardCount())

	// Create and use MetricsStream
	mock := &mockStream{readData: bytes.Repeat([]byte("x"), 10000)}
	ms := NewMetricsStream(mock, pf)

	// Simulate data transfer
	buf := make([]byte, 1024)
	totalRead := 0
	for {
		n, err := ms.Read(buf)
		totalRead += n
		if err != nil {
			break
		}
	}

	// Write some data too
	_, _ = ms.Write(bytes.Repeat([]byte("y"), 5000))

	t.Logf("Transferred data: Read=%d bytes, Wrote=5000 bytes", totalRead)
	t.Logf("Metrics: BytesIn=%d BytesOut=%d", pf.GetBytesIn(), pf.GetBytesOut())

	// Verify bytes are tracked immediately
	if pf.GetBytesIn() != uint64(totalRead) {
		t.Errorf("BytesIn mismatch: expected %d, got %d", totalRead, pf.GetBytesIn())
	}
	if pf.GetBytesOut() != 5000 {
		t.Errorf("BytesOut mismatch: expected 5000, got %d", pf.GetBytesOut())
	}

	// Wait for registry to take samples
	t.Log("Waiting for registry to sample...")
	time.Sleep(2500 * time.Millisecond)

	sampleCount := pf.rateCalc.SampleCount()
	t.Logf("After 2.5s: SampleCount=%d", sampleCount)

	if sampleCount < 2 {
		t.Errorf("Expected at least 2 samples, got %d", sampleCount)
	}

	// Get rate directly
	rateIn, rateOut := pf.GetInstantRate()
	t.Logf("Direct rate: RateIn=%.2f RateOut=%.2f", rateIn, rateOut)

	// Get snapshots and verify
	snapshots := reg.GetAllSnapshots()
	t.Logf("Got %d service snapshots", len(snapshots))

	if len(snapshots) != 1 {
		t.Fatalf("Expected 1 service snapshot, got %d", len(snapshots))
	}

	svc := snapshots[0]
	t.Logf("Service: Name=%s, PortForwards=%d", svc.ServiceName, len(svc.PortForwards))

	if len(svc.PortForwards) != 1 {
		t.Fatalf("Expected 1 port forward, got %d", len(svc.PortForwards))
	}

	pfSnap := svc.PortForwards[0]
	t.Logf("Snapshot: BytesIn=%d BytesOut=%d RateIn=%.2f RateOut=%.2f",
		pfSnap.BytesIn, pfSnap.BytesOut, pfSnap.RateIn, pfSnap.RateOut)

	if pfSnap.BytesIn != uint64(totalRead) {
		t.Errorf("Snapshot BytesIn wrong: expected %d, got %d", totalRead, pfSnap.BytesIn)
	}

	if pfSnap.BytesOut != 5000 {
		t.Errorf("Snapshot BytesOut wrong: expected 5000, got %d", pfSnap.BytesOut)
	}
}

// TestRateWithContinuousTraffic verifies rate is calculated correctly with ongoing traffic
func TestRateWithContinuousTraffic(t *testing.T) {
	registryOnce = sync.Once{}
	globalRegistry = nil
	reg := GetRegistry()
	reg.Start()
	defer reg.Stop()

	pf := NewPortForwardMetrics("svc", "ns", "ctx", "pod", "127.0.0.1", "8080", "80")
	reg.RegisterPortForward("svc.ns.ctx", pf)

	// Simulate continuous traffic: add bytes every 200ms
	go func() {
		for i := 0; i < 20; i++ {
			pf.AddBytesIn(1000)
			pf.AddBytesOut(500)
			time.Sleep(200 * time.Millisecond)
		}
	}()

	// Wait for traffic to accumulate and samples to be taken
	time.Sleep(3 * time.Second)

	snapshot := pf.GetSnapshot()
	t.Logf("With continuous traffic: BytesIn=%d RateIn=%.2f SampleCount=%d",
		snapshot.BytesIn, snapshot.RateIn, pf.rateCalc.SampleCount())

	// With continuous traffic, rate should be non-zero
	if snapshot.RateIn <= 0 {
		t.Errorf("Expected positive RateIn with continuous traffic, got %.2f", snapshot.RateIn)
	}
	if snapshot.RateOut <= 0 {
		t.Errorf("Expected positive RateOut with continuous traffic, got %.2f", snapshot.RateOut)
	}
}

// TestIoCopyUsesMetricsStream verifies that io.Copy correctly uses our Read/Write methods
func TestIoCopyUsesMetricsStream(t *testing.T) {
	metrics := NewPortForwardMetrics("svc", "ns", "ctx", "pod", "127.0.0.1", "8080", "80")
	mock := &mockStream{readData: bytes.Repeat([]byte("x"), 5000)}
	ms := NewMetricsStream(mock, metrics)

	// Use io.Copy to read from MetricsStream
	var dst bytes.Buffer
	n, err := io.Copy(&dst, ms)
	if err != nil {
		t.Fatalf("io.Copy failed: %v", err)
	}

	t.Logf("io.Copy read %d bytes, BytesIn=%d", n, metrics.GetBytesIn())

	// Verify our Read method was called (bytes tracked)
	if metrics.GetBytesIn() != uint64(n) {
		t.Errorf("io.Copy bypassed MetricsStream.Read! Expected BytesIn=%d, got %d", n, metrics.GetBytesIn())
	}

	// Now test Write via io.Copy
	metrics2 := NewPortForwardMetrics("svc2", "ns", "ctx", "pod", "127.0.0.1", "8081", "80")
	mock2 := &mockStream{}
	ms2 := NewMetricsStream(mock2, metrics2)

	src := bytes.NewReader(bytes.Repeat([]byte("y"), 3000))
	n, err = io.Copy(ms2, src)
	if err != nil {
		t.Fatalf("io.Copy write failed: %v", err)
	}

	t.Logf("io.Copy wrote %d bytes, BytesOut=%d", n, metrics2.GetBytesOut())

	// Verify our Write method was called
	if metrics2.GetBytesOut() != uint64(n) {
		t.Errorf("io.Copy bypassed MetricsStream.Write! Expected BytesOut=%d, got %d", n, metrics2.GetBytesOut())
	}
}
