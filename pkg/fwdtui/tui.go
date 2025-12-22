package fwdtui

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/rivo/tview"
	log "github.com/sirupsen/logrus"
	"github.com/txn2/kubefwd/pkg/fwdmetrics"
	"github.com/txn2/kubefwd/pkg/fwdtui/events"
	"github.com/txn2/kubefwd/pkg/fwdtui/hooks"
	"github.com/txn2/kubefwd/pkg/fwdtui/state"
	"github.com/txn2/kubefwd/pkg/fwdtui/views"
)

var (
	tuiEnabled bool
	tuiManager *Manager
	once       sync.Once
	mu         sync.RWMutex
	Version    string // Set by main package before Init
)

// Manager manages the TUI lifecycle
type Manager struct {
	app             *tview.Application
	layout          *views.Layout
	store           *state.Store
	eventBus        *events.Bus
	logHook         *hooks.TUILogHook
	metricsCancel   func()
	stopChan        chan struct{}
	doneChan        chan struct{}
	originalOut     io.Writer
	triggerShutdown func() // Callback to trigger main app shutdown
}

// Enable marks TUI mode as enabled
func Enable() {
	mu.Lock()
	defer mu.Unlock()
	tuiEnabled = true
}

// IsEnabled returns whether TUI mode is enabled
func IsEnabled() bool {
	mu.RLock()
	defer mu.RUnlock()
	return tuiEnabled
}

// GetEventBus returns the global event bus (nil if TUI not initialized)
func GetEventBus() *events.Bus {
	mu.RLock()
	defer mu.RUnlock()
	if tuiManager != nil {
		return tuiManager.eventBus
	}
	return nil
}

// Init initializes the TUI manager
// shutdownChan is used to listen for external shutdown signals
// triggerShutdown is called when the TUI wants to trigger app shutdown (e.g., user presses 'q')
func Init(shutdownChan <-chan struct{}, triggerShutdown func()) *Manager {
	once.Do(func() {
		app := tview.NewApplication()
		store := state.NewStore(1000)
		eventBus := events.NewBus(1000)

		tuiManager = &Manager{
			app:             app,
			store:           store,
			eventBus:        eventBus,
			stopChan:        make(chan struct{}),
			doneChan:        make(chan struct{}),
			triggerShutdown: triggerShutdown,
		}

		// Create layout
		tuiManager.layout = views.NewLayout(
			tuiManager.app,
			tuiManager.store,
			tuiManager.eventBus,
			Version,
		)

		// Create logrus hook
		tuiManager.logHook = hooks.NewTUILogHook(
			tuiManager.layout.GetLogsView(),
			tuiManager.app,
		)

		// Add logrus hook and suppress terminal output
		tuiManager.originalOut = log.StandardLogger().Out
		log.AddHook(tuiManager.logHook)
		log.SetOutput(io.Discard)

		// Subscribe to events
		tuiManager.subscribeToEvents()

		// Start event bus
		tuiManager.eventBus.Start()

		// Start metrics registry
		fwdmetrics.GetRegistry().Start()

		// Subscribe to metrics updates
		metricsCh, cancel := fwdmetrics.GetRegistry().Subscribe(500 * time.Millisecond)
		tuiManager.metricsCancel = cancel
		go tuiManager.handleMetricsUpdates(metricsCh)

		// Handle shutdown signal
		go func() {
			<-shutdownChan
			tuiManager.Stop()
		}()
	})

	return tuiManager
}

// Run starts the TUI application
func (m *Manager) Run() error {
	// Ensure TERM is set for proper terminal handling
	if os.Getenv("TERM") == "" {
		os.Setenv("TERM", "xterm-256color")
	}

	// Build and set the root
	root := m.layout.Build()
	if root == nil {
		log.SetOutput(m.originalOut)
		log.Error("TUI: layout.Build() returned nil")
		return fmt.Errorf("layout.Build() returned nil")
	}

	m.app.SetRoot(root, true)
	m.app.EnableMouse(true)

	// Add welcome message to logs
	m.layout.GetLogsView().AppendLog(log.InfoLevel, "kubefwd TUI started. Press ? for help, q to quit.")
	m.layout.GetLogsView().AppendLog(log.InfoLevel, "Waiting for services to be discovered...")

	// Run the application - blocks until app.Stop() is called
	err := m.app.Run()

	// Restore log output
	log.SetOutput(m.originalOut)

	if err != nil {
		log.Errorf("TUI: app.Run() error: %v", err)
	}

	// Trigger shutdown of the main application
	if m.triggerShutdown != nil {
		m.triggerShutdown()
	}

	close(m.doneChan)

	// Force exit after a short delay - the cleanup can take too long
	go func() {
		time.Sleep(2 * time.Second)
		os.Exit(0)
	}()

	return err
}

// Stop stops the TUI application
func (m *Manager) Stop() {
	select {
	case <-m.stopChan:
		// Already stopped
		return
	default:
		close(m.stopChan)
	}

	// Cancel metrics subscription
	if m.metricsCancel != nil {
		m.metricsCancel()
	}

	// Stop metrics registry
	fwdmetrics.GetRegistry().Stop()

	// Stop event bus
	m.eventBus.Stop()

	// Restore log output
	log.SetOutput(m.originalOut)

	// Stop the TUI app
	m.app.Stop()
}

// Done returns a channel that closes when the TUI is fully stopped
func (m *Manager) Done() <-chan struct{} {
	return m.doneChan
}

// subscribeToEvents sets up event handlers
func (m *Manager) subscribeToEvents() {
	// Service added
	m.eventBus.Subscribe(events.ServiceAdded, func(e events.Event) {
		m.app.QueueUpdateDraw(func() {
			// Service will be added when pods are forwarded
		})
	})

	// Service removed
	m.eventBus.Subscribe(events.ServiceRemoved, func(e events.Event) {
		m.app.QueueUpdateDraw(func() {
			// Remove all forwards for this service
			forwards := m.store.GetFiltered()
			for _, fwd := range forwards {
				if fwd.ServiceKey == e.ServiceKey {
					m.store.RemoveForward(fwd.Key)
				}
			}
			m.layout.Refresh()
		})
	})

	// Pod added (port forward started)
	m.eventBus.Subscribe(events.PodAdded, func(e events.Event) {
		m.app.QueueUpdateDraw(func() {
			snapshot := state.ForwardSnapshot{
				Key:         e.ServiceKey + "." + e.PodName,
				ServiceKey:  e.ServiceKey,
				ServiceName: e.Service,
				Namespace:   e.Namespace,
				Context:     e.Context,
				PodName:     e.PodName,
				LocalIP:     e.LocalIP,
				LocalPort:   e.LocalPort,
				PodPort:     e.PodPort,
				Hostnames:   e.Hostnames,
				Status:      state.StatusActive,
				StartedAt:   e.Timestamp,
			}
			m.store.AddForward(snapshot)
			m.layout.Refresh()
		})
	})

	// Pod removed (port forward stopped)
	m.eventBus.Subscribe(events.PodRemoved, func(e events.Event) {
		m.app.QueueUpdateDraw(func() {
			key := e.ServiceKey + "." + e.PodName
			m.store.RemoveForward(key)
			m.layout.Refresh()
		})
	})

	// Pod status changed
	m.eventBus.Subscribe(events.PodStatusChanged, func(e events.Event) {
		m.app.QueueUpdateDraw(func() {
			key := e.ServiceKey + "." + e.PodName
			var status state.ForwardStatus
			switch e.Status {
			case "connecting":
				status = state.StatusConnecting
			case "active":
				status = state.StatusActive
			case "error":
				status = state.StatusError
			case "stopping":
				status = state.StatusStopping
			default:
				status = state.StatusPending
			}
			errorMsg := ""
			if e.Error != nil {
				errorMsg = e.Error.Error()
			}
			m.store.UpdateStatus(key, status, errorMsg)
			m.layout.Refresh()
		})
	})

	// Shutdown events
	m.eventBus.Subscribe(events.ShutdownStarted, func(e events.Event) {
		m.app.QueueUpdateDraw(func() {
			// Update status bar to show shutdown in progress
			m.layout.GetStatusBar().TextView.SetText(" [yellow]Shutting down...[-]")
		})
	})
}

// podMetrics aggregates metrics for a single pod across all its ports
type podMetrics struct {
	bytesIn    uint64
	bytesOut   uint64
	rateIn     float64
	rateOut    float64
	avgRateIn  float64
	avgRateOut float64
}

// handleMetricsUpdates processes metrics updates from the registry
func (m *Manager) handleMetricsUpdates(ch <-chan []fwdmetrics.ServiceSnapshot) {
	for snapshots := range ch {
		m.app.QueueUpdateDraw(func() {
			// Aggregate metrics by pod key (a pod may have multiple ports)
			podTotals := make(map[string]*podMetrics)

			for _, svc := range snapshots {
				for _, pf := range svc.PortForwards {
					key := svc.ServiceName + "." + svc.Namespace + "." + svc.Context + "." + pf.PodName
					if pm, ok := podTotals[key]; ok {
						// Accumulate metrics from multiple ports
						pm.bytesIn += pf.BytesIn
						pm.bytesOut += pf.BytesOut
						pm.rateIn += pf.RateIn
						pm.rateOut += pf.RateOut
						pm.avgRateIn += pf.AvgRateIn
						pm.avgRateOut += pf.AvgRateOut
					} else {
						podTotals[key] = &podMetrics{
							bytesIn:    pf.BytesIn,
							bytesOut:   pf.BytesOut,
							rateIn:     pf.RateIn,
							rateOut:    pf.RateOut,
							avgRateIn:  pf.AvgRateIn,
							avgRateOut: pf.AvgRateOut,
						}
					}
				}
			}

			// Update store with aggregated metrics
			for key, pm := range podTotals {
				m.store.UpdateMetrics(
					key,
					pm.bytesIn,
					pm.bytesOut,
					pm.rateIn,
					pm.rateOut,
					pm.avgRateIn,
					pm.avgRateOut,
				)
			}

			// Refresh layout to show updated metrics (both table and status bar)
			m.layout.Refresh()
		})
	}
}

// Emit sends an event if TUI is enabled
func Emit(event events.Event) {
	if bus := GetEventBus(); bus != nil {
		bus.Publish(event)
	}
}
