package fwdtui

import (
	"io"
	"os"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	log "github.com/sirupsen/logrus"
	"github.com/txn2/kubefwd/pkg/fwdmetrics"
	"github.com/txn2/kubefwd/pkg/fwdtui/components"
	"github.com/txn2/kubefwd/pkg/fwdtui/events"
	"github.com/txn2/kubefwd/pkg/fwdtui/state"
	"github.com/txn2/kubefwd/pkg/fwdtui/styles"
)

var (
	tuiEnabled bool
	tuiManager *Manager
	once       sync.Once
	mu         sync.RWMutex
	Version    string // Set by main package before Init
)

// Focus tracks which component has focus
type Focus int

const (
	FocusServices Focus = iota
	FocusLogs
	FocusDetail
)

// Manager manages the TUI lifecycle
type Manager struct {
	program         *tea.Program
	model           *RootModel
	store           *state.Store
	eventBus        *events.Bus
	eventCh         chan events.Event
	logCh           chan LogEntryMsg
	metricsCancel   func()
	stopChan        chan struct{}
	doneChan        chan struct{}
	originalOut     io.Writer
	triggerShutdown func()
}

// RootModel is the main bubbletea model
type RootModel struct {
	// Components
	header    components.HeaderModel
	services  components.ServicesModel
	logs      components.LogsModel
	statusBar components.StatusBarModel
	help      components.HelpModel
	detail    components.DetailModel

	// State
	store       *state.Store
	eventBus    *events.Bus
	rateHistory *state.RateHistory
	focus       Focus
	quitting    bool

	// Dimensions
	width          int
	height         int
	servicesHeight int
	logsHeight     int

	// Channels for async updates
	eventCh   <-chan events.Event
	metricsCh <-chan []fwdmetrics.ServiceSnapshot
	logCh     <-chan LogEntryMsg
	stopCh    <-chan struct{}

	// Callback
	triggerShutdown func()
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
func Init(shutdownChan <-chan struct{}, triggerShutdown func()) *Manager {
	once.Do(func() {
		store := state.NewStore(1000)
		eventBus := events.NewBus(1000)
		eventCh := make(chan events.Event, 100)
		logCh := make(chan LogEntryMsg, 100)

		tuiManager = &Manager{
			store:           store,
			eventBus:        eventBus,
			eventCh:         eventCh,
			logCh:           logCh,
			stopChan:        make(chan struct{}),
			doneChan:        make(chan struct{}),
			triggerShutdown: triggerShutdown,
		}

		// Subscribe event bus to forward to our channel
		eventBus.SubscribeAll(func(e events.Event) {
			select {
			case eventCh <- e:
			default:
				// Drop if buffer full
			}
		})

		// Start event bus
		eventBus.Start()

		// Start metrics registry
		fwdmetrics.GetRegistry().Start()

		// Subscribe to metrics updates
		metricsCh, cancel := fwdmetrics.GetRegistry().Subscribe(500 * time.Millisecond)
		tuiManager.metricsCancel = cancel

		// Create rate history for sparklines
		rateHistory := state.NewRateHistory(60)

		// Create the model
		tuiManager.model = &RootModel{
			header:          components.NewHeaderModel(Version),
			services:        components.NewServicesModel(store),
			logs:            components.NewLogsModel(),
			statusBar:       components.NewStatusBarModel(),
			help:            components.NewHelpModel(),
			detail:          components.NewDetailModel(store, rateHistory),
			store:           store,
			eventBus:        eventBus,
			rateHistory:     rateHistory,
			focus:           FocusServices,
			eventCh:         eventCh,
			metricsCh:       metricsCh,
			logCh:           logCh,
			stopCh:          shutdownChan,
			triggerShutdown: triggerShutdown,
		}

		// Suppress terminal output and capture logs
		tuiManager.originalOut = log.StandardLogger().Out
		log.SetOutput(io.Discard)

		// Add log hook
		log.AddHook(&tuiLogHook{logCh: logCh})
	})

	return tuiManager
}

// Run starts the TUI application
func (m *Manager) Run() error {
	// Ensure TERM is set
	if os.Getenv("TERM") == "" {
		os.Setenv("TERM", "xterm-256color")
	}

	// Create the program with mouse support for scrolling
	// Hold Shift to select text (standard terminal behavior with mouse capture)
	m.program = tea.NewProgram(
		m.model,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	// Run the program - blocks until quit
	_, err := m.program.Run()

	// Restore log output
	log.SetOutput(m.originalOut)

	// Trigger shutdown of main application
	if m.triggerShutdown != nil {
		m.triggerShutdown()
	}

	close(m.doneChan)

	// Force exit after delay
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
		return
	default:
		close(m.stopChan)
	}

	if m.metricsCancel != nil {
		m.metricsCancel()
	}

	fwdmetrics.GetRegistry().Stop()
	m.eventBus.Stop()

	log.SetOutput(m.originalOut)

	if m.program != nil {
		m.program.Quit()
	}
}

// Done returns a channel that closes when TUI is stopped
func (m *Manager) Done() <-chan struct{} {
	return m.doneChan
}

// Emit sends an event if TUI is enabled
func Emit(event events.Event) {
	if bus := GetEventBus(); bus != nil {
		bus.Publish(event)
	}
}

// RootModel methods

// Init initializes the model
func (m RootModel) Init() tea.Cmd {
	return tea.Batch(
		ListenEvents(m.eventCh),
		ListenMetrics(m.metricsCh),
		ListenLogs(m.logCh),
		ListenShutdown(m.stopCh),
		SendLog(log.InfoLevel, "kubefwd TUI started. Press ? for help, q to quit."),
		SendLog(log.InfoLevel, "Waiting for services to be discovered..."),
		// No mouse capture - allows text selection everywhere
		// Use keyboard: j/k to navigate, Enter to open detail, Tab to switch focus
	)
}

// Update handles messages
func (m RootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updateSizes()

		// Only forward to components that need it (not services/logs - we use SetSize)
		m.header, _ = m.header.Update(msg)
		m.statusBar, _ = m.statusBar.Update(msg)
		m.help, _ = m.help.Update(msg)

	case tea.KeyMsg:
		// Help modal captures all input when visible
		if m.help.IsVisible() {
			m.help, _ = m.help.Update(msg)
			return m, nil
		}

		// Detail view captures all input when visible
		if m.detail.IsVisible() {
			m.detail, _ = m.detail.Update(msg)
			// If detail closed, return focus to services
			if !m.detail.IsVisible() {
				m.focus = FocusServices
			}
			return m, nil
		}

		// Global keys
		switch msg.String() {
		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "q":
			if !m.services.IsFiltering() {
				m.quitting = true
				return m, tea.Quit
			}
		case "?":
			m.help.Toggle()
			return m, nil
		case "tab":
			m.cycleFocus()
			return m, nil
		}

		// Route to focused component
		var cmd tea.Cmd
		switch m.focus {
		case FocusServices:
			// Handle Enter to open detail view (but not when filtering)
			if msg.String() == "enter" && m.services.HasSelection() && !m.services.IsFiltering() {
				key := m.services.GetSelectedKey()
				if key != "" {
					m.detail.Show(key)
					m.detail.SetSize(m.width, m.height)
					m.focus = FocusDetail
					return m, nil
				}
			}
			m.services, cmd = m.services.Update(msg)
			cmds = append(cmds, cmd)
		case FocusLogs:
			m.logs, cmd = m.logs.Update(msg)
			cmds = append(cmds, cmd)
		}

	case tea.MouseMsg:
		// Handle mouse wheel scrolling only - clicks are ignored for text selection
		// Hold Shift to select text (standard terminal behavior)
		if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
			var cmd tea.Cmd
			// Detail view gets priority if visible
			if m.detail.IsVisible() {
				m.detail, cmd = m.detail.Update(msg)
				cmds = append(cmds, cmd)
			} else {
				switch m.focus {
				case FocusServices:
					m.services, cmd = m.services.Update(msg)
					cmds = append(cmds, cmd)
				case FocusLogs:
					m.logs, cmd = m.logs.Update(msg)
					cmds = append(cmds, cmd)
				}
			}
		}
		// Click events are intentionally not handled to minimize interference

	case MetricsUpdateMsg:
		m.handleMetricsUpdate(msg)
		m.services.Refresh()
		m.statusBar.UpdateStats(m.store.GetSummary())
		// Re-subscribe
		cmds = append(cmds, ListenMetrics(m.metricsCh))

	case KubefwdEventMsg:
		m.handleKubefwdEvent(msg.Event)
		m.services.Refresh()
		m.statusBar.UpdateStats(m.store.GetSummary())
		// Re-subscribe
		cmds = append(cmds, ListenEvents(m.eventCh))

	case LogEntryMsg:
		m.logs.AppendLog(msg.Level, msg.Message, msg.Time)
		// Re-subscribe
		cmds = append(cmds, ListenLogs(m.logCh))

	case ShutdownMsg:
		m.quitting = true
		return m, tea.Quit

	case components.OpenDetailMsg:
		// Open detail view (from double-click when mouse is enabled)
		if msg.Key != "" {
			m.detail.Show(msg.Key)
			m.detail.SetSize(m.width, m.height)
			m.focus = FocusDetail
		}

	case RefreshMsg:
		m.services.Refresh()
		m.statusBar.UpdateStats(m.store.GetSummary())
	}

	return m, tea.Batch(cmds...)
}

// View renders the UI
func (m RootModel) View() string {
	if m.quitting {
		return "Shutting down...\n"
	}

	// Overlay help if visible
	if m.help.IsVisible() {
		return m.help.View()
	}

	// Overlay detail view if visible
	if m.detail.IsVisible() {
		return m.detail.View()
	}

	// Render header
	header := m.header.View()

	// Services section with focus accent
	servicesFocusAccent := " "
	if m.focus == FocusServices {
		servicesFocusAccent = styles.FocusAccentStyle.Render("▌")
	}
	servicesTitle := servicesFocusAccent + styles.SectionTitleStyle.Render("Services")
	// Height() ensures section fills allocated space (content at top, padding below)
	servicesContent := lipgloss.NewStyle().
		Height(m.servicesHeight).
		Render(m.services.View())

	// Logs section with focus accent
	logsFocusAccent := " "
	if m.focus == FocusLogs {
		logsFocusAccent = styles.FocusAccentStyle.Render("▌")
	}
	logsTitle := logsFocusAccent + styles.SectionTitleStyle.Render("Logs")
	// Height() ensures section fills allocated space (content at top, padding below)
	logsContent := lipgloss.NewStyle().
		Height(m.logsHeight).
		Render(m.logs.View())

	// Render status bar
	status := m.statusBar.View()

	// Compose layout with consistent spacing
	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		"", // blank line before Services
		servicesTitle,
		servicesContent,
		"", // blank line before Logs
		logsTitle,
		logsContent,
		"", // blank line before footer
		status,
	)
}

// updateSizes recalculates component sizes
func (m *RootModel) updateSizes() {
	// Measure fixed elements
	headerHeight := lipgloss.Height(m.header.View())
	statusHeight := lipgloss.Height(m.statusBar.View())
	if headerHeight < 1 {
		headerHeight = 1
	}
	if statusHeight < 1 {
		statusHeight = 1
	}

	// Fixed lines: section titles (2) + blank lines (3)
	fixedLines := 5

	// Full terminal width
	contentWidth := m.width
	if contentWidth < 20 {
		contentWidth = 20
	}

	// Available height for services + logs
	availableHeight := m.height - headerHeight - statusHeight - fixedLines
	if availableHeight < 10 {
		availableHeight = 10
	}

	// Split 75/25 between services and logs (table is more important)
	logsHeight := availableHeight / 4
	servicesHeight := availableHeight - logsHeight

	if servicesHeight < 6 {
		servicesHeight = 6
	}
	if logsHeight < 3 {
		logsHeight = 3
	}

	// Store heights
	m.servicesHeight = servicesHeight
	m.logsHeight = logsHeight

	// Set component sizes - they handle their own rendering
	m.services.SetSize(contentWidth, servicesHeight)
	m.logs.SetSize(contentWidth, logsHeight)
	m.statusBar.SetWidth(m.width)
	m.detail.SetSize(m.width, m.height)
}

// cycleFocus switches focus between components
func (m *RootModel) cycleFocus() {
	switch m.focus {
	case FocusServices:
		m.focus = FocusLogs
		m.services.SetFocus(false)
		m.logs.SetFocus(true)
	case FocusLogs:
		m.focus = FocusServices
		m.services.SetFocus(true)
		m.logs.SetFocus(false)
	}
}

// handleMetricsUpdate processes metrics updates
func (m *RootModel) handleMetricsUpdate(msg MetricsUpdateMsg) {
	// Aggregate metrics by pod key
	type podMetrics struct {
		bytesIn, bytesOut     uint64
		rateIn, rateOut       float64
		avgRateIn, avgRateOut float64
	}

	podTotals := make(map[string]*podMetrics)

	for _, svc := range msg.Snapshots {
		for _, pf := range svc.PortForwards {
			key := svc.ServiceName + "." + svc.Namespace + "." + svc.Context + "." + pf.PodName + "." + pf.LocalPort
			if pm, ok := podTotals[key]; ok {
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

			// Store rate history for sparklines
			if m.rateHistory != nil {
				m.rateHistory.AddSample(key, pf.RateIn, pf.RateOut)
			}
		}
	}

	for key, pm := range podTotals {
		m.store.UpdateMetrics(key, pm.bytesIn, pm.bytesOut, pm.rateIn, pm.rateOut, pm.avgRateIn, pm.avgRateOut)
	}

	// Update detail view if visible
	if m.detail.IsVisible() {
		m.detail.UpdateSnapshot()

		// Find matching port forward and update HTTP logs
		detailKey := m.detail.GetForwardKey()
		for _, svc := range msg.Snapshots {
			for _, pf := range svc.PortForwards {
				key := svc.ServiceName + "." + svc.Namespace + "." + svc.Context + "." + pf.PodName
				if key == detailKey && len(pf.HTTPLogs) > 0 {
					// Convert metrics HTTP logs to detail HTTP logs
					logs := make([]components.HTTPLogEntry, len(pf.HTTPLogs))
					for i, log := range pf.HTTPLogs {
						logs[i] = components.HTTPLogEntry{
							Timestamp:  log.Timestamp,
							Method:     log.Method,
							Path:       log.Path,
							StatusCode: log.StatusCode,
							Duration:   log.Duration,
							Size:       log.Size,
						}
					}
					m.detail.SetHTTPLogs(logs)
					break
				}
			}
		}
	}
}

// handleKubefwdEvent processes kubefwd events
func (m *RootModel) handleKubefwdEvent(e events.Event) {
	switch e.Type {
	case events.PodAdded:
		snapshot := state.ForwardSnapshot{
			Key:         e.ServiceKey + "." + e.PodName + "." + e.LocalPort,
			ServiceKey:  e.ServiceKey,
			ServiceName: e.Service,
			Namespace:   e.Namespace,
			Context:     e.Context,
			PodName:     e.PodName,
			LocalIP:     e.LocalIP,
			LocalPort:   e.LocalPort,
			PodPort:     e.PodPort,
			Hostnames:   e.Hostnames,
			Status:      state.StatusConnecting,
			StartedAt:   e.Timestamp,
		}
		m.store.AddForward(snapshot)

	case events.PodRemoved:
		key := e.ServiceKey + "." + e.PodName + "." + e.LocalPort
		m.store.RemoveForward(key)

	case events.PodStatusChanged:
		key := e.ServiceKey + "." + e.PodName + "." + e.LocalPort
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

		// Update hostnames when "active" status arrives with populated hostnames
		// (hostnames are only available after AddHosts() runs in PortForward)
		if e.Status == "active" && len(e.Hostnames) > 0 {
			m.store.UpdateHostnames(key, e.Hostnames)
		}

	case events.ServiceRemoved:
		forwards := m.store.GetFiltered()
		for _, fwd := range forwards {
			if fwd.ServiceKey == e.ServiceKey {
				m.store.RemoveForward(fwd.Key)
			}
		}
	}
}

// tuiLogHook is a logrus hook that sends logs to the TUI
type tuiLogHook struct {
	logCh chan<- LogEntryMsg
}

func (h *tuiLogHook) Levels() []log.Level {
	return log.AllLevels
}

func (h *tuiLogHook) Fire(entry *log.Entry) error {
	select {
	case h.logCh <- LogEntryMsg{
		Level:   entry.Level,
		Message: entry.Message,
		Time:    entry.Time,
	}:
	default:
		// Drop if buffer full
	}
	return nil
}
