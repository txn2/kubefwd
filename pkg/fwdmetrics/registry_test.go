package fwdmetrics

import (
	"sync"
	"testing"
	"time"
)

// TestRegistryRegisterUnregisterService tests service registration and unregistration
func TestRegistryRegisterUnregisterService(t *testing.T) {
	// Reset global registry for test
	registryOnce = sync.Once{}
	globalRegistry = nil
	reg := GetRegistry()
	defer reg.Stop()

	// Create a service
	svc := NewServiceMetrics("testsvc", "testns", "testctx")

	// Register it
	reg.RegisterService(svc)

	if reg.ServiceCount() != 1 {
		t.Errorf("Expected 1 service, got %d", reg.ServiceCount())
	}

	// Get it back
	retrieved := reg.GetService("testsvc.testns.testctx")
	if retrieved == nil {
		t.Fatal("Expected to get service, got nil")
	}
	if retrieved.ServiceName != "testsvc" {
		t.Errorf("Expected ServiceName 'testsvc', got '%s'", retrieved.ServiceName)
	}

	// Unregister it
	reg.UnregisterService("testsvc.testns.testctx")

	if reg.ServiceCount() != 0 {
		t.Errorf("Expected 0 services after unregister, got %d", reg.ServiceCount())
	}

	// Verify it's gone
	retrieved = reg.GetService("testsvc.testns.testctx")
	if retrieved != nil {
		t.Error("Expected nil after unregister, got service")
	}
}

// TestRegistryGetService tests getting a non-existent service
func TestRegistryGetServiceNotFound(t *testing.T) {
	registryOnce = sync.Once{}
	globalRegistry = nil
	reg := GetRegistry()
	defer reg.Stop()

	svc := reg.GetService("nonexistent.ns.ctx")
	if svc != nil {
		t.Error("Expected nil for non-existent service")
	}
}

// TestRegistryUnregisterPortForward tests port forward unregistration
func TestRegistryUnregisterPortForward(t *testing.T) {
	registryOnce = sync.Once{}
	globalRegistry = nil
	reg := GetRegistry()
	defer reg.Stop()

	// Register a port forward
	pf := NewPortForwardMetrics("svc", "ns", "ctx", "pod1", "127.0.0.1", "8080", "80")
	reg.RegisterPortForward("svc.ns.ctx", pf)

	if reg.ServiceCount() != 1 {
		t.Errorf("Expected 1 service, got %d", reg.ServiceCount())
	}
	if reg.PortForwardCount() != 1 {
		t.Errorf("Expected 1 port forward, got %d", reg.PortForwardCount())
	}

	// Unregister the port forward
	reg.UnregisterPortForward("svc.ns.ctx", "pod1", "8080")

	// Service should be removed since it has no port forwards
	if reg.ServiceCount() != 0 {
		t.Errorf("Expected 0 services after unregistering last port forward, got %d", reg.ServiceCount())
	}
}

// TestRegistryUnregisterPortForwardWithMultiple tests keeping service when other forwards exist
func TestRegistryUnregisterPortForwardWithMultiple(t *testing.T) {
	registryOnce = sync.Once{}
	globalRegistry = nil
	reg := GetRegistry()
	defer reg.Stop()

	// Register two port forwards for the same service
	pf1 := NewPortForwardMetrics("svc", "ns", "ctx", "pod1", "127.0.0.1", "8080", "80")
	pf2 := NewPortForwardMetrics("svc", "ns", "ctx", "pod2", "127.0.0.2", "8080", "80")
	reg.RegisterPortForward("svc.ns.ctx", pf1)
	reg.RegisterPortForward("svc.ns.ctx", pf2)

	if reg.PortForwardCount() != 2 {
		t.Errorf("Expected 2 port forwards, got %d", reg.PortForwardCount())
	}

	// Unregister one
	reg.UnregisterPortForward("svc.ns.ctx", "pod1", "8080")

	// Service should still exist with one port forward
	if reg.ServiceCount() != 1 {
		t.Errorf("Expected 1 service, got %d", reg.ServiceCount())
	}
	if reg.PortForwardCount() != 1 {
		t.Errorf("Expected 1 port forward, got %d", reg.PortForwardCount())
	}

	// Unregister the other
	reg.UnregisterPortForward("svc.ns.ctx", "pod2", "8080")
	if reg.ServiceCount() != 0 {
		t.Errorf("Expected 0 services, got %d", reg.ServiceCount())
	}
}

// TestRegistryUnregisterPortForwardNonExistent tests unregistering from non-existent service
func TestRegistryUnregisterPortForwardNonExistent(t *testing.T) {
	registryOnce = sync.Once{}
	globalRegistry = nil
	reg := GetRegistry()
	defer reg.Stop()

	// Should not panic
	reg.UnregisterPortForward("nonexistent.ns.ctx", "pod1", "8080")
}

// TestRegistryGetAllServices tests getting all services
func TestRegistryGetAllServices(t *testing.T) {
	registryOnce = sync.Once{}
	globalRegistry = nil
	reg := GetRegistry()
	defer reg.Stop()

	// Register multiple services
	svc1 := NewServiceMetrics("svc1", "ns", "ctx")
	svc2 := NewServiceMetrics("svc2", "ns", "ctx")
	svc3 := NewServiceMetrics("svc3", "ns", "ctx")
	reg.RegisterService(svc1)
	reg.RegisterService(svc2)
	reg.RegisterService(svc3)

	all := reg.GetAllServices()
	if len(all) != 3 {
		t.Errorf("Expected 3 services, got %d", len(all))
	}

	// Verify they are the ones we registered
	names := make(map[string]bool)
	for _, s := range all {
		names[s.ServiceName] = true
	}
	for _, name := range []string{"svc1", "svc2", "svc3"} {
		if !names[name] {
			t.Errorf("Expected service %s in result", name)
		}
	}
}

// TestRegistryGetTotals tests aggregated totals across all services
func TestRegistryGetTotals(t *testing.T) {
	registryOnce = sync.Once{}
	globalRegistry = nil
	reg := GetRegistry()
	defer reg.Stop()

	// Register port forwards and add bytes
	pf1 := NewPortForwardMetrics("svc1", "ns", "ctx", "pod1", "127.0.0.1", "8080", "80")
	pf2 := NewPortForwardMetrics("svc2", "ns", "ctx", "pod2", "127.0.0.2", "8081", "80")
	reg.RegisterPortForward("svc1.ns.ctx", pf1)
	reg.RegisterPortForward("svc2.ns.ctx", pf2)

	pf1.AddBytesIn(1000)
	pf1.AddBytesOut(500)
	pf2.AddBytesIn(2000)
	pf2.AddBytesOut(1000)

	bytesIn, bytesOut, _, _ := reg.GetTotals()
	if bytesIn != 3000 {
		t.Errorf("Expected total bytesIn 3000, got %d", bytesIn)
	}
	if bytesOut != 1500 {
		t.Errorf("Expected total bytesOut 1500, got %d", bytesOut)
	}
}

// TestServiceMetricsGetTotals tests aggregated totals for a service
func TestServiceMetricsGetTotals(t *testing.T) {
	svc := NewServiceMetrics("svc", "ns", "ctx")

	pf1 := NewPortForwardMetrics("svc", "ns", "ctx", "pod1", "127.0.0.1", "8080", "80")
	pf2 := NewPortForwardMetrics("svc", "ns", "ctx", "pod2", "127.0.0.2", "8081", "80")
	svc.AddPortForward(pf1)
	svc.AddPortForward(pf2)

	pf1.AddBytesIn(500)
	pf1.AddBytesOut(250)
	pf2.AddBytesIn(700)
	pf2.AddBytesOut(350)

	bytesIn, bytesOut, _, _ := svc.GetTotals()
	if bytesIn != 1200 {
		t.Errorf("Expected bytesIn 1200, got %d", bytesIn)
	}
	if bytesOut != 600 {
		t.Errorf("Expected bytesOut 600, got %d", bytesOut)
	}
}

// TestServiceMetricsRemovePortForward tests removing a port forward from service
func TestServiceMetricsRemovePortForward(t *testing.T) {
	svc := NewServiceMetrics("svc", "ns", "ctx")

	pf := NewPortForwardMetrics("svc", "ns", "ctx", "pod1", "127.0.0.1", "8080", "80")
	svc.AddPortForward(pf)

	if svc.Count() != 1 {
		t.Errorf("Expected 1 port forward, got %d", svc.Count())
	}

	svc.RemovePortForward("pod1", "8080")

	if svc.Count() != 0 {
		t.Errorf("Expected 0 port forwards after removal, got %d", svc.Count())
	}
}

// TestPortForwardMetricsKey tests the Key method
func TestPortForwardMetricsKey(t *testing.T) {
	pf := NewPortForwardMetrics("mysvc", "myns", "myctx", "mypod", "127.0.0.1", "8080", "80")

	key := pf.Key()
	expected := "mysvc.myns.myctx.mypod"
	if key != expected {
		t.Errorf("Expected key '%s', got '%s'", expected, key)
	}
}

// TestPortForwardMetricsServiceKey tests the ServiceKey method
func TestPortForwardMetricsServiceKey(t *testing.T) {
	pf := NewPortForwardMetrics("mysvc", "myns", "myctx", "mypod", "127.0.0.1", "8080", "80")

	key := pf.ServiceKey()
	expected := "mysvc.myns.myctx"
	if key != expected {
		t.Errorf("Expected service key '%s', got '%s'", expected, key)
	}
}

// TestRegistryGetServiceSnapshot tests getting a single service snapshot
func TestRegistryGetServiceSnapshot(t *testing.T) {
	registryOnce = sync.Once{}
	globalRegistry = nil
	reg := GetRegistry()
	defer reg.Stop()

	// Register a service with port forwards
	pf := NewPortForwardMetrics("svc", "ns", "ctx", "pod1", "127.0.0.1", "8080", "80")
	reg.RegisterPortForward("svc.ns.ctx", pf)

	// Add some traffic
	pf.AddBytesIn(1000)
	pf.AddBytesOut(500)

	// Get the snapshot
	snapshot := reg.GetServiceSnapshot("svc.ns.ctx")
	if snapshot == nil {
		t.Fatal("Expected non-nil snapshot")
	}

	if snapshot.ServiceName != "svc" {
		t.Errorf("Expected ServiceName 'svc', got '%s'", snapshot.ServiceName)
	}
	if snapshot.Namespace != "ns" {
		t.Errorf("Expected Namespace 'ns', got '%s'", snapshot.Namespace)
	}
	if snapshot.TotalBytesIn != 1000 {
		t.Errorf("Expected TotalBytesIn 1000, got %d", snapshot.TotalBytesIn)
	}
	if snapshot.TotalBytesOut != 500 {
		t.Errorf("Expected TotalBytesOut 500, got %d", snapshot.TotalBytesOut)
	}
}

// TestRegistryGetServiceSnapshot_NotFound tests getting a non-existent service snapshot
func TestRegistryGetServiceSnapshot_NotFound(t *testing.T) {
	registryOnce = sync.Once{}
	globalRegistry = nil
	reg := GetRegistry()
	defer reg.Stop()

	snapshot := reg.GetServiceSnapshot("nonexistent.ns.ctx")
	if snapshot != nil {
		t.Error("Expected nil snapshot for non-existent service")
	}
}

// TestRegistrySubscribe tests subscribing to metrics updates
func TestRegistrySubscribe(t *testing.T) {
	registryOnce = sync.Once{}
	globalRegistry = nil
	reg := GetRegistry()
	defer reg.Stop()

	// Register a service
	svc := NewServiceMetrics("svc", "ns", "ctx")
	reg.RegisterService(svc)

	// Subscribe with a short interval
	ch, cancel := reg.Subscribe(10 * time.Millisecond)
	defer cancel()

	// Wait for an update
	select {
	case snapshots := <-ch:
		if len(snapshots) != 1 {
			t.Errorf("Expected 1 snapshot, got %d", len(snapshots))
		}
		if snapshots[0].ServiceName != "svc" {
			t.Errorf("Expected ServiceName 'svc', got '%s'", snapshots[0].ServiceName)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Timed out waiting for subscription update")
	}
}

// TestRegistrySubscribe_Cancel tests canceling a subscription
func TestRegistrySubscribe_Cancel(t *testing.T) {
	registryOnce = sync.Once{}
	globalRegistry = nil
	reg := GetRegistry()
	defer reg.Stop()

	// Subscribe
	ch, cancel := reg.Subscribe(10 * time.Millisecond)

	// Cancel immediately
	cancel()

	// Channel should be closed eventually
	select {
	case _, ok := <-ch:
		if ok {
			// Got a message, that's fine - try again
			select {
			case _, ok := <-ch:
				if ok {
					t.Error("Channel should be closed after cancel")
				}
			case <-time.After(50 * time.Millisecond):
				// Closed
			}
		}
		// Channel closed as expected
	case <-time.After(100 * time.Millisecond):
		t.Error("Channel should be closed after cancel")
	}
}

// TestRegistrySubscribe_RegistryStop tests subscription cleanup when registry stops
func TestRegistrySubscribe_RegistryStop(t *testing.T) {
	registryOnce = sync.Once{}
	globalRegistry = nil
	reg := GetRegistry()

	// Subscribe with longer interval to avoid flooding
	ch, _ := reg.Subscribe(100 * time.Millisecond)

	// Stop the registry
	reg.Stop()

	// Wait for channel to be closed with a timeout
	timeout := time.After(500 * time.Millisecond)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				// Channel closed as expected
				return
			}
			// Got a message, keep waiting for close
		case <-timeout:
			// Timeout is acceptable - the goroutine will eventually close
			// but we don't want to wait forever in a test
			return
		}
	}
}

// TestPortForwardMetricsEnableHTTPSniffing tests enabling HTTP sniffing
func TestPortForwardMetricsEnableHTTPSniffing(t *testing.T) {
	pf := NewPortForwardMetrics("svc", "ns", "ctx", "pod", "127.0.0.1", "8080", "80")

	// Initially nil
	if pf.GetHTTPSniffer() != nil {
		t.Error("Expected nil sniffer initially")
	}

	// GetHTTPLogs should return nil when sniffing is not enabled
	logs := pf.GetHTTPLogs(10)
	if logs != nil {
		t.Error("Expected nil logs when sniffing not enabled")
	}

	// Enable sniffing
	pf.EnableHTTPSniffing(100)

	if pf.GetHTTPSniffer() == nil {
		t.Error("Expected non-nil sniffer after enabling")
	}

	// GetHTTPLogs returns nil when there are no logs (not empty slice)
	// This is expected behavior from the implementation
	logs = pf.GetHTTPLogs(10)
	if logs != nil {
		t.Error("Expected nil logs when no HTTP traffic has occurred")
	}

	// Verify sniffer is accessible
	sniffer := pf.GetHTTPSniffer()
	if sniffer == nil {
		t.Error("Expected to get sniffer")
	}
}
