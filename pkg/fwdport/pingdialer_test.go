package fwdport

import (
	"errors"
	"net/http"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"k8s.io/streaming/pkg/httpstream"
)

// fakePingStream is the stream handed back by fakePingConn.CreateStream.
type fakePingStream struct{}

func (fakePingStream) Read([]byte) (int, error)    { return 0, nil }
func (fakePingStream) Write(b []byte) (int, error) { return len(b), nil }
func (fakePingStream) Close() error                { return nil }
func (fakePingStream) Reset() error                { return nil }
func (fakePingStream) Headers() http.Header        { return http.Header{} }
func (fakePingStream) Identifier() uint32          { return 1 }

// fakePingConn counts CreateStream calls. When gate is set, CreateStream
// signals entered and blocks until gate is closed, like a ping in flight on
// a dead connection.
type fakePingConn struct {
	creates   atomic.Int32
	gate      chan struct{}
	entered   chan struct{}
	enterOnce sync.Once
	closeCh   chan bool
}

func newFakePingConn() *fakePingConn {
	return &fakePingConn{closeCh: make(chan bool), entered: make(chan struct{})}
}

func (c *fakePingConn) CreateStream(http.Header) (httpstream.Stream, error) {
	c.creates.Add(1)
	if c.gate != nil {
		c.enterOnce.Do(func() { close(c.entered) })
		<-c.gate
	}
	return fakePingStream{}, nil
}
func (c *fakePingConn) Close() error                       { return nil }
func (c *fakePingConn) CloseChan() <-chan bool             { return c.closeCh }
func (c *fakePingConn) SetIdleTimeout(time.Duration)       {}
func (c *fakePingConn) RemoveStreams(...httpstream.Stream) {}

// fakePingDialer returns conn, or err when set.
type fakePingDialer struct {
	conn httpstream.Connection
	err  error
}

func (d fakePingDialer) Dial(...string) (httpstream.Connection, string, error) {
	if d.err != nil {
		return nil, "", d.err
	}
	return d.conn, "v4.channel.k8s.io", nil
}

func newTestPingingDialer(wrapped httpstream.Dialer) *pingingDialer {
	return &pingingDialer{
		wrappedDialer:     wrapped,
		pingPeriod:        5 * time.Millisecond,
		pingStopChan:      make(chan struct{}),
		pingTargetPodName: "test-pod",
	}
}

// waitForGoroutines waits until the goroutine count drops back to baseline.
func waitForGoroutines(t *testing.T, baseline int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if runtime.NumGoroutine() <= baseline {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("ping goroutines leaked: %d goroutines running, baseline %d", runtime.NumGoroutine(), baseline)
}

// assertNoMorePings fails if CreateStream is still being called.
func assertNoMorePings(t *testing.T, conn *fakePingConn) {
	t.Helper()
	before := conn.creates.Load()
	time.Sleep(50 * time.Millisecond) // 10 ping periods
	if after := conn.creates.Load(); after != before {
		t.Fatalf("pinging continued after stop: CreateStream calls went from %d to %d", before, after)
	}
}

// TestPingingDialer_StopDuringInFlightPing is the regression test for #614:
// a stop that arrives while a ping is blocked in CreateStream must still end
// the goroutine once CreateStream returns.
func TestPingingDialer_StopDuringInFlightPing(t *testing.T) {
	baseline := runtime.NumGoroutine()
	conn := newFakePingConn()
	conn.gate = make(chan struct{})
	d := newTestPingingDialer(fakePingDialer{conn: conn})

	if _, _, err := d.Dial(); err != nil {
		t.Fatalf("Dial: %v", err)
	}

	select {
	case <-conn.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("ping never started")
	}

	// The old stopPing gave up after 100ms here and the goroutine kept pinging.
	stopped := make(chan struct{})
	go func() {
		d.stopPing()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("stopPing blocked while a ping was in flight")
	}

	close(conn.gate) // CreateStream returns, as it does after its 30s timeout
	waitForGoroutines(t, baseline)
	assertNoMorePings(t, conn)
}

// TestPingingDialer_StopEndsEveryDial checks that one stopPing ends the ping
// goroutine of every connection the dialer produced.
func TestPingingDialer_StopEndsEveryDial(t *testing.T) {
	baseline := runtime.NumGoroutine()
	conn := newFakePingConn()
	d := newTestPingingDialer(fakePingDialer{conn: conn})

	for range 3 {
		if _, _, err := d.Dial(); err != nil {
			t.Fatalf("Dial: %v", err)
		}
	}
	time.Sleep(20 * time.Millisecond)
	if conn.creates.Load() == 0 {
		t.Fatal("expected pings before stop")
	}

	d.stopPing()
	d.stopPing() // idempotent

	waitForGoroutines(t, baseline)
	assertNoMorePings(t, conn)
}

// TestPingingDialer_ExitsWhenConnectionCloses checks that the ping goroutine
// ends with its connection even if stopPing is never called.
func TestPingingDialer_ExitsWhenConnectionCloses(t *testing.T) {
	baseline := runtime.NumGoroutine()
	conn := newFakePingConn()
	d := newTestPingingDialer(fakePingDialer{conn: conn})

	if _, _, err := d.Dial(); err != nil {
		t.Fatalf("Dial: %v", err)
	}
	close(conn.closeCh)

	waitForGoroutines(t, baseline)
	assertNoMorePings(t, conn)
	d.stopPing()
}

// TestPingingDialer_DialAfterStop checks that a connection dialed after
// stopPing never pings.
func TestPingingDialer_DialAfterStop(t *testing.T) {
	baseline := runtime.NumGoroutine()
	conn := newFakePingConn()
	d := newTestPingingDialer(fakePingDialer{conn: conn})

	d.stopPing()
	if _, _, err := d.Dial(); err != nil {
		t.Fatalf("Dial: %v", err)
	}

	waitForGoroutines(t, baseline)
	if n := conn.creates.Load(); n != 0 {
		t.Fatalf("expected no pings after stop, got %d", n)
	}
}

// TestPingingDialer_DialError checks that a failed dial returns the error and
// starts no pinging.
func TestPingingDialer_DialError(t *testing.T) {
	baseline := runtime.NumGoroutine()
	dialErr := errors.New("dial failed")
	d := newTestPingingDialer(fakePingDialer{err: dialErr})

	conn, _, err := d.Dial()
	if !errors.Is(err, dialErr) {
		t.Fatalf("expected dial error, got %v", err)
	}
	if conn != nil {
		t.Fatal("expected nil connection on dial error")
	}

	waitForGoroutines(t, baseline)
	d.stopPing()
}
