package fwdnet

import (
	"net"
	"sync"

	"github.com/txn2/kubefwd/pkg/fwdip"
)

// InterfaceManager handles loopback interface operations.
// This interface allows mocking network operations for testing.
type InterfaceManager interface {
	// ReadyInterface prepares a local IP address on the loopback interface.
	// Returns the allocated IP or an error.
	ReadyInterface(opts fwdip.ForwardIPOpts) (net.IP, error)

	// RemoveInterfaceAlias removes an IP alias from the loopback interface.
	RemoveInterfaceAlias(ip net.IP)
}

// defaultInterfaceManager is the production implementation.
// Methods are defined in fwdnet.go.
type defaultInterfaceManager struct{}

// managerMu protects access to Manager.
var managerMu sync.RWMutex

// manager is the package-level InterfaceManager used by the application.
// Access via getManager() to ensure thread-safety.
var manager InterfaceManager = &defaultInterfaceManager{}

// getManager returns the current InterfaceManager in a thread-safe manner.
func getManager() InterfaceManager {
	managerMu.RLock()
	defer managerMu.RUnlock()
	return manager
}

// SetManager replaces the current InterfaceManager (for testing).
func SetManager(m InterfaceManager) {
	managerMu.Lock()
	defer managerMu.Unlock()
	manager = m
}

// ResetManager restores the default InterfaceManager.
func ResetManager() {
	managerMu.Lock()
	defer managerMu.Unlock()
	manager = &defaultInterfaceManager{}
}
