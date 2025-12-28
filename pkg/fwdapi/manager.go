package fwdapi

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
	"github.com/txn2/kubefwd/pkg/fwdapi/types"
)

// Re-export interface types for external use
type (
	StateReader       = types.StateReader
	MetricsProvider   = types.MetricsProvider
	ServiceController = types.ServiceController
	EventStreamer     = types.EventStreamer
)

const (
	// APIIP is the dedicated loopback IP for the API server
	// Uses 127.2.27.1 to avoid conflicts with forwarded services (127.1.x.x)
	APIIP = "127.2.27.1"

	// APIPort is the port the API server listens on
	APIPort = "80"

	// APIHostname is the hostname added to /etc/hosts
	APIHostname = "api.kubefwd.local"
)

var (
	apiEnabled bool
	apiManager *Manager
	once       sync.Once
	mu         sync.RWMutex
)

// Manager manages the API server lifecycle
type Manager struct {
	server    *http.Server
	router    *gin.Engine
	stopChan  chan struct{}
	doneChan  chan struct{}
	startTime time.Time

	// Dependencies (interfaces for testability)
	stateReader         StateReader
	metricsProvider     MetricsProvider
	serviceController   ServiceController
	eventStreamer       EventStreamer
	diagnosticsProvider types.DiagnosticsProvider

	// CRUD controllers
	namespaceController types.NamespaceController
	serviceCRUD         types.ServiceCRUD
	k8sDiscovery        types.KubernetesDiscovery

	// Callbacks
	triggerShutdown func()

	// Configuration
	version    string
	namespaces []string
	contexts   []string
	tuiEnabled bool
}

// Enable marks API mode as enabled
func Enable() {
	mu.Lock()
	defer mu.Unlock()
	apiEnabled = true
}

// IsEnabled returns whether API mode is enabled
func IsEnabled() bool {
	mu.RLock()
	defer mu.RUnlock()
	return apiEnabled
}

// GetManager returns the global API manager (nil if not initialized)
func GetManager() *Manager {
	mu.RLock()
	defer mu.RUnlock()
	return apiManager
}

// Init initializes the API manager
func Init(shutdownChan <-chan struct{}, triggerShutdown func(), version string) *Manager {
	once.Do(func() {
		apiManager = &Manager{
			stopChan:        make(chan struct{}),
			doneChan:        make(chan struct{}),
			startTime:       time.Now(),
			triggerShutdown: triggerShutdown,
			version:         version,
		}

		// Listen for external shutdown signal
		go func() {
			<-shutdownChan
			apiManager.Stop()
		}()
	})
	return apiManager
}

// SetStateReader sets the state reader dependency
func (m *Manager) SetStateReader(reader StateReader) {
	m.stateReader = reader
}

// SetMetricsProvider sets the metrics provider dependency
func (m *Manager) SetMetricsProvider(provider MetricsProvider) {
	m.metricsProvider = provider
}

// SetServiceController sets the service controller dependency
func (m *Manager) SetServiceController(controller ServiceController) {
	m.serviceController = controller
}

// SetEventStreamer sets the event streamer dependency
func (m *Manager) SetEventStreamer(streamer EventStreamer) {
	m.eventStreamer = streamer
}

// SetDiagnosticsProvider sets the diagnostics provider dependency
func (m *Manager) SetDiagnosticsProvider(provider types.DiagnosticsProvider) {
	m.diagnosticsProvider = provider
}

// SetNamespaceController sets the namespace controller for CRUD operations
func (m *Manager) SetNamespaceController(controller types.NamespaceController) {
	m.namespaceController = controller
}

// SetServiceCRUD sets the service CRUD controller for add/remove operations
func (m *Manager) SetServiceCRUD(crud types.ServiceCRUD) {
	m.serviceCRUD = crud
}

// SetKubernetesDiscovery sets the Kubernetes discovery provider
func (m *Manager) SetKubernetesDiscovery(discovery types.KubernetesDiscovery) {
	m.k8sDiscovery = discovery
}

// SetNamespaces sets the namespaces being forwarded (for info endpoint)
func (m *Manager) SetNamespaces(namespaces []string) {
	m.namespaces = namespaces
}

// SetContexts sets the contexts being used (for info endpoint)
func (m *Manager) SetContexts(contexts []string) {
	m.contexts = contexts
}

// SetTUIEnabled sets whether TUI is also enabled (for info endpoint)
func (m *Manager) SetTUIEnabled(enabled bool) {
	m.tuiEnabled = enabled
}

// Run starts the API server (blocks until stopped)
func (m *Manager) Run() error {
	if m.stateReader == nil {
		return fmt.Errorf("state reader not configured")
	}

	// Set Gin to release mode for cleaner logs
	gin.SetMode(gin.ReleaseMode)

	// Create router with all routes
	m.router = m.setupRouter()

	// Create HTTP server
	addr := net.JoinHostPort(APIIP, APIPort)
	m.server = &http.Server{
		Addr:         addr,
		Handler:      m.router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // Disable for SSE streaming
		IdleTimeout:  120 * time.Second,
	}

	log.Infof("API server listening on http://%s (http://%s/)", addr, APIHostname)

	// Start server in goroutine
	errCh := make(chan error, 1)
	go func() {
		if err := m.server.ListenAndServe(); err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	// Wait for stop signal or error
	select {
	case <-m.stopChan:
		// Graceful shutdown
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := m.server.Shutdown(ctx); err != nil {
			log.Errorf("API server shutdown error: %v", err)
		}
	case err := <-errCh:
		return err
	}

	close(m.doneChan)
	return nil
}

// Stop stops the API server
func (m *Manager) Stop() {
	select {
	case <-m.stopChan:
		return // Already stopped
	default:
		close(m.stopChan)
	}
}

// Done returns a channel that closes when the API server is stopped
func (m *Manager) Done() <-chan struct{} {
	return m.doneChan
}

// Uptime returns the server uptime
func (m *Manager) Uptime() time.Duration {
	return time.Since(m.startTime)
}

// StartTime returns when the server started
func (m *Manager) StartTime() time.Time {
	return m.startTime
}

// Version returns the configured version
func (m *Manager) Version() string {
	return m.version
}

// Namespaces returns the namespaces being forwarded
func (m *Manager) Namespaces() []string {
	return m.namespaces
}

// Contexts returns the contexts being used
func (m *Manager) Contexts() []string {
	return m.contexts
}

// TUIEnabled returns whether TUI is also enabled
func (m *Manager) TUIEnabled() bool {
	return m.tuiEnabled
}
