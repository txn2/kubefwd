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

	// CRUD controllers for developer-focused tools
	namespaceController types.NamespaceController
	serviceCRUD         types.ServiceCRUD
	k8sDiscovery        types.KubernetesDiscovery
	connectionInfo      types.ConnectionInfoProvider

	// Additional providers for enhanced MCP tools
	analysisProvider    *AnalysisProviderHTTP
	httpTrafficProvider *HTTPTrafficProviderHTTP
	historyProvider     *HistoryProviderHTTP

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

// SetNamespaceController sets the namespace controller for CRUD operations
func (s *Server) SetNamespaceController(controller types.NamespaceController) {
	s.mu.Lock()
	s.namespaceController = controller
	s.mu.Unlock()
}

// SetServiceCRUD sets the service CRUD controller for add/remove operations
func (s *Server) SetServiceCRUD(crud types.ServiceCRUD) {
	s.mu.Lock()
	s.serviceCRUD = crud
	s.mu.Unlock()
}

// SetKubernetesDiscovery sets the Kubernetes discovery provider
func (s *Server) SetKubernetesDiscovery(discovery types.KubernetesDiscovery) {
	s.mu.Lock()
	s.k8sDiscovery = discovery
	s.mu.Unlock()
}

// SetConnectionInfoProvider sets the connection info provider
func (s *Server) SetConnectionInfoProvider(provider types.ConnectionInfoProvider) {
	s.mu.Lock()
	s.connectionInfo = provider
	s.mu.Unlock()
}

// SetAnalysisProvider sets the analysis provider for AI-optimized endpoints
func (s *Server) SetAnalysisProvider(provider *AnalysisProviderHTTP) {
	s.mu.Lock()
	s.analysisProvider = provider
	s.mu.Unlock()
}

// SetHTTPTrafficProvider sets the HTTP traffic provider for traffic inspection
func (s *Server) SetHTTPTrafficProvider(provider *HTTPTrafficProviderHTTP) {
	s.mu.Lock()
	s.httpTrafficProvider = provider
	s.mu.Unlock()
}

// SetHistoryProvider sets the history provider for event/error/reconnection history
func (s *Server) SetHistoryProvider(provider *HistoryProviderHTTP) {
	s.mu.Lock()
	s.historyProvider = provider
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

// getNamespaceController safely gets the namespace controller
func (s *Server) getNamespaceController() types.NamespaceController {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.namespaceController
}

// getServiceCRUD safely gets the service CRUD controller
func (s *Server) getServiceCRUD() types.ServiceCRUD {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.serviceCRUD
}

// getK8sDiscovery safely gets the Kubernetes discovery provider
func (s *Server) getK8sDiscovery() types.KubernetesDiscovery {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.k8sDiscovery
}

// getCurrentContext returns the current kubeconfig context, or empty string if unavailable
func (s *Server) getCurrentContext() string {
	k8s := s.getK8sDiscovery()
	if k8s == nil {
		return ""
	}
	resp, err := k8s.ListContexts()
	if err != nil || resp == nil {
		return ""
	}
	return resp.CurrentContext
}

// getConnectionInfo safely gets the connection info provider
func (s *Server) getConnectionInfo() types.ConnectionInfoProvider {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.connectionInfo
}

// getAnalysisProvider safely gets the analysis provider
func (s *Server) getAnalysisProvider() *AnalysisProviderHTTP {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.analysisProvider
}

// getHTTPTrafficProvider safely gets the HTTP traffic provider
func (s *Server) getHTTPTrafficProvider() *HTTPTrafficProviderHTTP {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.httpTrafficProvider
}

// getHistoryProvider safely gets the history provider
func (s *Server) getHistoryProvider() *HistoryProviderHTTP {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.historyProvider
}

// ServeStdio starts the MCP server on stdio transport and blocks until done.
// This is a convenience wrapper around Run for use with HTTP client mode.
func (s *Server) ServeStdio() error {
	return s.Run(context.Background())
}
