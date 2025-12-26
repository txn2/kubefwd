package fwdnet

import (
	"net"
	"sync"

	"github.com/txn2/kubefwd/pkg/fwdip"
)

// MockInterfaceManager is a test double for InterfaceManager.
// It tracks all calls and allows configuring return values.
type MockInterfaceManager struct {
	mu sync.Mutex

	// ReadyInterfaceCalls tracks all calls to ReadyInterface
	ReadyInterfaceCalls []fwdip.ForwardIPOpts

	// RemoveInterfaceAliasCalls tracks all calls to RemoveInterfaceAlias
	RemoveInterfaceAliasCalls []net.IP

	// AllocatedIPs maps service keys (name.namespace) to allocated IPs
	// for consistent allocation across multiple calls
	AllocatedIPs map[string]net.IP

	// ReadyInterfaceErr is the error to return from ReadyInterface
	// Set this to simulate errors
	ReadyInterfaceErr error

	// ipCounter tracks the next IP octet to allocate
	ipCounter int
}

// NewMockInterfaceManager creates a new mock starting at 127.1.27.1
func NewMockInterfaceManager() *MockInterfaceManager {
	return &MockInterfaceManager{
		AllocatedIPs: make(map[string]net.IP),
		ipCounter:    1,
	}
}

// ReadyInterface implements InterfaceManager.
// It allocates unique IPs per service and tracks all calls.
func (m *MockInterfaceManager) ReadyInterface(opts fwdip.ForwardIPOpts) (net.IP, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.ReadyInterfaceCalls = append(m.ReadyInterfaceCalls, opts)

	if m.ReadyInterfaceErr != nil {
		return nil, m.ReadyInterfaceErr
	}

	// Return consistent IP for same service
	key := opts.ServiceName + "." + opts.Namespace
	if ip, exists := m.AllocatedIPs[key]; exists {
		return ip, nil
	}

	// Allocate new IP
	ip := net.IPv4(127, 1, 27, byte(m.ipCounter))
	m.ipCounter++
	m.AllocatedIPs[key] = ip

	return ip, nil
}

// RemoveInterfaceAlias implements InterfaceManager.
// It tracks all calls without performing any system operations.
func (m *MockInterfaceManager) RemoveInterfaceAlias(ip net.IP) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.RemoveInterfaceAliasCalls = append(m.RemoveInterfaceAliasCalls, ip)
}

// GetReadyInterfaceCallCount returns the number of ReadyInterface calls
func (m *MockInterfaceManager) GetReadyInterfaceCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.ReadyInterfaceCalls)
}

// GetRemoveAliasCallCount returns the number of RemoveInterfaceAlias calls
func (m *MockInterfaceManager) GetRemoveAliasCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.RemoveInterfaceAliasCalls)
}

// Reset clears all recorded calls and allocated IPs
func (m *MockInterfaceManager) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ReadyInterfaceCalls = nil
	m.RemoveInterfaceAliasCalls = nil
	m.AllocatedIPs = make(map[string]net.IP)
	m.ipCounter = 1
	m.ReadyInterfaceErr = nil
}
