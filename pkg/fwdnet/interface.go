package fwdnet

import (
	"net"

	"github.com/txn2/kubefwd/pkg/fwdIp"
)

// InterfaceManager handles loopback interface operations.
// This interface allows mocking network operations for testing.
type InterfaceManager interface {
	// ReadyInterface prepares a local IP address on the loopback interface.
	// Returns the allocated IP or an error.
	ReadyInterface(opts fwdIp.ForwardIPOpts) (net.IP, error)

	// RemoveInterfaceAlias removes an IP alias from the loopback interface.
	RemoveInterfaceAlias(ip net.IP)
}

// defaultInterfaceManager is the production implementation.
// Methods are defined in fwdnet.go.
type defaultInterfaceManager struct{}

// Manager is the package-level InterfaceManager used by the application.
// Replace this with a mock for testing.
var Manager InterfaceManager = &defaultInterfaceManager{}

// SetManager replaces the current InterfaceManager (for testing).
func SetManager(m InterfaceManager) {
	Manager = m
}

// ResetManager restores the default InterfaceManager.
func ResetManager() {
	Manager = &defaultInterfaceManager{}
}
