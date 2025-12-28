// Package fwdmcp provides an MCP (Model Context Protocol) server for kubefwd.
// This enables AI assistants like Claude to interact directly with kubefwd
// for debugging, monitoring, and managing Kubernetes port forwarding.
package fwdmcp

import (
	"context"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/txn2/kubefwd/pkg/fwdapi/types"
)

var (
	globalServer *Server
	serverOnce   sync.Once
	enabled      bool
	enabledMu    sync.RWMutex
)

// Server manages the MCP server lifecycle
type Server struct {
	mcpServer *mcp.Server
	version   string

	// Dependencies (same adapters as REST API)
	stateReader    types.StateReader
	metrics        types.MetricsProvider
	controller     types.ServiceController
	diagnostics    types.DiagnosticsProvider
	getManagerInfo func() types.ManagerInfo

	stopCh chan struct{}
	doneCh chan struct{}
	mu     sync.RWMutex
}

// Enable marks MCP mode as enabled
func Enable() {
	enabledMu.Lock()
	enabled = true
	enabledMu.Unlock()
}

// IsEnabled returns whether MCP mode is enabled
func IsEnabled() bool {
	enabledMu.RLock()
	defer enabledMu.RUnlock()
	return enabled
}

// Init initializes the global MCP server
func Init(version string) *Server {
	serverOnce.Do(func() {
		globalServer = &Server{
			version: version,
			stopCh:  make(chan struct{}),
			doneCh:  make(chan struct{}),
		}
		globalServer.setupServer()
	})
	return globalServer
}

// GetServer returns the global MCP server instance
func GetServer() *Server {
	return globalServer
}

// SetStateReader sets the state reader adapter
func (s *Server) SetStateReader(sr types.StateReader) {
	s.mu.Lock()
	s.stateReader = sr
	s.mu.Unlock()
}

// SetMetricsProvider sets the metrics provider adapter
func (s *Server) SetMetricsProvider(mp types.MetricsProvider) {
	s.mu.Lock()
	s.metrics = mp
	s.mu.Unlock()
}

// SetServiceController sets the service controller adapter
func (s *Server) SetServiceController(sc types.ServiceController) {
	s.mu.Lock()
	s.controller = sc
	s.mu.Unlock()
}

// SetDiagnosticsProvider sets the diagnostics provider adapter
func (s *Server) SetDiagnosticsProvider(dp types.DiagnosticsProvider) {
	s.mu.Lock()
	s.diagnostics = dp
	s.mu.Unlock()
}

// SetManagerInfo sets the manager info getter
func (s *Server) SetManagerInfo(getter func() types.ManagerInfo) {
	s.mu.Lock()
	s.getManagerInfo = getter
	s.mu.Unlock()
}

// setupServer creates the MCP server and registers tools/resources
func (s *Server) setupServer() {
	s.mcpServer = mcp.NewServer(&mcp.Implementation{
		Name:    "kubefwd",
		Version: s.version,
	}, nil)

	// Register tools
	s.registerTools()

	// Register resources
	s.registerResources()

	// Register prompts
	s.registerPrompts()
}

// Run starts the MCP server on stdio transport (blocking)
func (s *Server) Run(ctx context.Context) error {
	defer close(s.doneCh)

	// Create a context that cancels when stop is called
	runCtx, cancel := context.WithCancel(ctx)
	go func() {
		select {
		case <-s.stopCh:
			cancel()
		case <-runCtx.Done():
		}
	}()

	// Run on stdio transport
	return s.mcpServer.Run(runCtx, &mcp.StdioTransport{})
}

// Stop signals the server to stop
func (s *Server) Stop() {
	close(s.stopCh)
}

// Done returns a channel that closes when the server stops
func (s *Server) Done() <-chan struct{} {
	return s.doneCh
}

// getState safely gets the state reader
func (s *Server) getState() types.StateReader {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stateReader
}

// getMetrics safely gets the metrics provider
func (s *Server) getMetrics() types.MetricsProvider {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.metrics
}

// getController safely gets the service controller
func (s *Server) getController() types.ServiceController {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.controller
}

// getDiagnostics safely gets the diagnostics provider
func (s *Server) getDiagnostics() types.DiagnosticsProvider {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.diagnostics
}

// getManager safely gets the manager info
func (s *Server) getManager() types.ManagerInfo {
	s.mu.RLock()
	getter := s.getManagerInfo
	s.mu.RUnlock()
	if getter != nil {
		return getter()
	}
	return nil
}
