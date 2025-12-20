/*
Copyright 2018-2024 Craig Johnston <cjimti@gmail.com>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
	fmt.Fprintln(os.Stderr, "DEBUG: TUI Init() called")
	once.Do(func() {
		fmt.Fprintln(os.Stderr, "DEBUG: TUI once.Do executing")

		fmt.Fprintln(os.Stderr, "DEBUG: Creating tview.Application...")
		app := tview.NewApplication()
		fmt.Fprintln(os.Stderr, "DEBUG: Creating state.Store...")
		store := state.NewStore(1000)
		fmt.Fprintln(os.Stderr, "DEBUG: Creating events.Bus...")
		eventBus := events.NewBus(1000)

		tuiManager = &Manager{
			app:             app,
			store:           store,
			eventBus:        eventBus,
			stopChan:        make(chan struct{}),
			doneChan:        make(chan struct{}),
			triggerShutdown: triggerShutdown,
		}
		fmt.Fprintln(os.Stderr, "DEBUG: Manager created")

		// Create layout
		fmt.Fprintln(os.Stderr, "DEBUG: Creating layout...")
		tuiManager.layout = views.NewLayout(
			tuiManager.app,
			tuiManager.store,
			tuiManager.eventBus,
		)
		fmt.Fprintln(os.Stderr, "DEBUG: Layout created")

		// Create logrus hook
		fmt.Fprintln(os.Stderr, "DEBUG: Creating logrus hook...")
		tuiManager.logHook = hooks.NewTUILogHook(
			tuiManager.layout.GetLogsView(),
			tuiManager.app,
		)
		fmt.Fprintln(os.Stderr, "DEBUG: Logrus hook created")

		// Add logrus hook and suppress terminal output
		tuiManager.originalOut = log.StandardLogger().Out
		log.AddHook(tuiManager.logHook)
		log.SetOutput(io.Discard)
		fmt.Fprintln(os.Stderr, "DEBUG: Log output redirected")

		// Subscribe to events
		fmt.Fprintln(os.Stderr, "DEBUG: Subscribing to events...")
		tuiManager.subscribeToEvents()
		fmt.Fprintln(os.Stderr, "DEBUG: Events subscribed")

		// Start event bus
		fmt.Fprintln(os.Stderr, "DEBUG: Starting event bus...")
		tuiManager.eventBus.Start()
		fmt.Fprintln(os.Stderr, "DEBUG: Event bus started")

		// Start metrics registry
		fmt.Fprintln(os.Stderr, "DEBUG: Starting metrics registry...")
		fwdmetrics.GetRegistry().Start()
		fmt.Fprintln(os.Stderr, "DEBUG: Metrics registry started")

		// Subscribe to metrics updates
		fmt.Fprintln(os.Stderr, "DEBUG: Subscribing to metrics updates...")
		metricsCh, cancel := fwdmetrics.GetRegistry().Subscribe(500 * time.Millisecond)
		tuiManager.metricsCancel = cancel
		go tuiManager.handleMetricsUpdates(metricsCh)
		fmt.Fprintln(os.Stderr, "DEBUG: Metrics subscription done")

		// Handle shutdown signal
		go func() {
			<-shutdownChan
			tuiManager.Stop()
		}()

		fmt.Fprintln(os.Stderr, "DEBUG: Init complete")
	})

	return tuiManager
}

// Run starts the TUI application
func (m *Manager) Run() error {
	fmt.Fprintln(os.Stderr, "DEBUG: TUI Run() called")

	// Check if we have a proper terminal
	// Check stdin is a TTY
	stdinFi, err := os.Stdin.Stat()
	if err != nil {
		fmt.Fprintf(os.Stderr, "DEBUG: Cannot stat stdin: %v\n", err)
	} else {
		mode := stdinFi.Mode()
		fmt.Fprintf(os.Stderr, "DEBUG: stdin mode: %v, isCharDevice: %v\n", mode, mode&os.ModeCharDevice != 0)
	}

	// Check stdout is a TTY
	stdoutFi, err := os.Stdout.Stat()
	if err != nil {
		fmt.Fprintf(os.Stderr, "DEBUG: Cannot stat stdout: %v\n", err)
	} else {
		mode := stdoutFi.Mode()
		fmt.Fprintf(os.Stderr, "DEBUG: stdout mode: %v, isCharDevice: %v\n", mode, mode&os.ModeCharDevice != 0)
	}

	// Check TERM environment variable
	term := os.Getenv("TERM")
	if term == "" {
		fmt.Fprintln(os.Stderr, "DEBUG: WARNING: TERM environment variable not set, setting to xterm-256color")
		os.Setenv("TERM", "xterm-256color")
	} else {
		fmt.Fprintf(os.Stderr, "DEBUG: TERM=%s\n", term)
	}

	// Build and set the root
	root := m.layout.Build()
	if root == nil {
		// Restore log output for error message
		log.SetOutput(m.originalOut)
		log.Error("TUI: layout.Build() returned nil")
		return fmt.Errorf("layout.Build() returned nil")
	}

	fmt.Fprintln(os.Stderr, "DEBUG: TUI layout built, setting root")
	m.app.SetRoot(root, true)

	// Enable mouse support
	m.app.EnableMouse(true)

	// Add welcome message to logs
	m.layout.GetLogsView().AppendLog(log.InfoLevel, "kubefwd TUI started. Press ? for help, q to quit.")
	m.layout.GetLogsView().AppendLog(log.InfoLevel, "Waiting for services to be discovered...")

	fmt.Fprintln(os.Stderr, "DEBUG: TUI calling app.Run() - this should take over the terminal")

	// Run the application - this should block until app.Stop() is called
	err = m.app.Run()

	fmt.Fprintf(os.Stderr, "DEBUG: TUI app.Run() returned, err=%v\n", err)

	// Restore log output
	log.SetOutput(m.originalOut)

	// If there was an error, log it
	if err != nil {
		log.Errorf("TUI: app.Run() error: %v", err)
	}

	// Trigger shutdown of the main application
	if m.triggerShutdown != nil {
		fmt.Fprintln(os.Stderr, "DEBUG: Triggering main app shutdown...")
		m.triggerShutdown()
	}

	close(m.doneChan)

	// Force exit after a short delay - the cleanup can take too long
	go func() {
		time.Sleep(2 * time.Second)
		fmt.Fprintln(os.Stderr, "DEBUG: Force exiting after timeout")
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

// handleMetricsUpdates processes metrics updates from the registry
func (m *Manager) handleMetricsUpdates(ch <-chan []fwdmetrics.ServiceSnapshot) {
	for snapshots := range ch {
		m.app.QueueUpdateDraw(func() {
			for _, svc := range snapshots {
				for _, pf := range svc.PortForwards {
					key := svc.ServiceName + "." + svc.Namespace + "." + svc.Context + "." + pf.PodName
					m.store.UpdateMetrics(
						key,
						pf.BytesIn,
						pf.BytesOut,
						pf.RateIn,
						pf.RateOut,
						pf.AvgRateIn,
						pf.AvgRateOut,
					)
				}
			}
			// Update status bar with totals
			m.layout.GetStatusBar().Update(m.store.GetSummary())
		})
	}
}

// Emit sends an event if TUI is enabled
func Emit(event events.Event) {
	if bus := GetEventBus(); bus != nil {
		bus.Publish(event)
	}
}
