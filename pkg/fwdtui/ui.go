package fwdtui

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	log "github.com/sirupsen/logrus"
	"github.com/txn2/kubefwd/pkg/fwdapi/types"
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

	// Standalone event infrastructure (for API-only mode without TUI)
	standaloneEventBus *events.Bus
	standaloneStore    *state.Store
	eventOnce          sync.Once
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

// PodLogsStreamer is a function that streams pod logs from Kubernetes
// It returns an io.ReadCloser that streams log lines
type PodLogsStreamer func(ctx context.Context, namespace, podName, containerName, k8sContext string, tailLines int64) (io.ReadCloser, error)

// RootModel is the main bubbletea model
type RootModel struct {
	// Components
	header    components.HeaderModel
	services  components.ServicesModel
	logs      components.LogsModel
	statusBar components.StatusBarModel
	help      components.HelpModel
	detail    components.DetailModel
	browse    components.BrowseModel

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
	servicesStartY int // Y where services content begins (for click detection)
	logsStartY     int // Y where logs content begins (for click detection)

	// Channels for async updates
	eventCh   <-chan events.Event
	metricsCh <-chan []fwdmetrics.ServiceSnapshot
	logCh     <-chan LogEntryMsg
	stopCh    <-chan struct{}

	// Callbacks
	triggerShutdown  func()
	streamPodLogs    PodLogsStreamer
	reconnectErrored func() int             // Returns number of services reconnected
	removeForward    func(key string) error // Removes a forward by registry key

	// Pod log streaming state
	logStreamCancel context.CancelFunc
	logStreamCh     chan string
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

// EventsEnabled returns true if the event infrastructure is available.
// This is true when either TUI is enabled OR API-only mode initialized events.
func EventsEnabled() bool {
	mu.RLock()
	defer mu.RUnlock()
	return tuiManager != nil || standaloneEventBus != nil
}

// InitEventInfrastructure initializes the event bus and state store for API-only mode.
// This allows events to be published and consumed without the full TUI.
// Safe to call multiple times - only initializes once.
func InitEventInfrastructure() {
	eventOnce.Do(func() {
		mu.Lock()
		defer mu.Unlock()
		standaloneStore = state.NewStore(1000)
		standaloneEventBus = events.NewBus(1000)

		// Subscribe the store to events so it tracks state
		standaloneEventBus.SubscribeAll(func(e events.Event) {
			handleEventForStore(standaloneStore, e)
		})

		// Start the event bus
		standaloneEventBus.Start()
	})
}

// handleEventForStore processes a kubefwd event and updates the store
// This is the same logic used by the TUI but extracted for standalone use
func handleEventForStore(store *state.Store, e events.Event) {
	switch e.Type {
	case events.PodAdded:
		key := e.ServiceKey + "." + e.PodName + "." + e.LocalPort
		snapshot := state.ForwardSnapshot{
			Key:           key,
			ServiceKey:    e.ServiceKey,
			RegistryKey:   e.RegistryKey,
			ServiceName:   e.Service,
			Namespace:     e.Namespace,
			Context:       e.Context,
			PodName:       e.PodName,
			ContainerName: e.ContainerName,
			LocalIP:       e.LocalIP,
			LocalPort:     e.LocalPort,
			PodPort:       e.PodPort,
			Hostnames:     e.Hostnames,
			Status:        state.StatusConnecting,
			StartedAt:     e.Timestamp,
		}
		store.AddForward(snapshot)

	case events.PodRemoved:
		key := e.ServiceKey + "." + e.PodName + "." + e.LocalPort
		store.RemoveForward(key)

	case events.PodStatusChanged:
		handlePodStatusChanged(store, e)

	case events.ServiceRemoved:
		forwards := store.GetFiltered()
		for _, fwd := range forwards {
			if fwd.ServiceKey == e.ServiceKey {
				store.RemoveForward(fwd.Key)
			}
		}

	case events.NamespaceRemoved:
		// Remove all services and forwards for this namespace/context
		store.RemoveByNamespace(e.Namespace, e.Context)
	case events.ServiceAdded, events.ServiceUpdated, events.BandwidthUpdate,
		events.LogMessage, events.ShutdownStarted, events.ShutdownComplete:
		// These events don't require store updates
	}
}

// handlePodStatusChanged processes pod status change events and updates the store
func handlePodStatusChanged(store *state.Store, e events.Event) {
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
	store.UpdateStatus(key, status, errorMsg)

	// Update hostnames when "active" status arrives with populated hostnames
	if e.Status == "active" && len(e.Hostnames) > 0 {
		store.UpdateHostnames(key, e.Hostnames)
	}
}

// GetEventBus returns the global event bus (nil if not initialized)
// Checks TUI manager first, then standalone event bus
func GetEventBus() *events.Bus {
	mu.RLock()
	defer mu.RUnlock()
	if tuiManager != nil {
		return tuiManager.eventBus
	}
	return standaloneEventBus
}

// GetStore returns the global state store (nil if not initialized)
// Checks TUI manager first, then standalone store
func GetStore() *state.Store {
	mu.RLock()
	defer mu.RUnlock()
	if tuiManager != nil {
		return tuiManager.store
	}
	return standaloneStore
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
				// Buffer full - silently drop event
				// This typically happens during shutdown when many removal events occur
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
			browse:          components.NewBrowseModel(),
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
		log.AddHook(&tuiLogHook{logCh: logCh, stopCh: shutdownChan})
	})

	return tuiManager
}

// SetPodLogsStreamer sets the function used to stream pod logs
func (m *Manager) SetPodLogsStreamer(streamer PodLogsStreamer) {
	if m.model != nil {
		m.model.streamPodLogs = streamer
	}
}

// SetErroredServicesReconnector sets the function used to reconnect errored services
// The function should return the number of services that were triggered for reconnection
func (m *Manager) SetErroredServicesReconnector(reconnector func() int) {
	if m.model != nil {
		m.model.reconnectErrored = reconnector
	}
}

// SetBrowseDiscovery sets the Kubernetes discovery adapter for the browse modal
func (m *Manager) SetBrowseDiscovery(discovery types.KubernetesDiscovery) {
	if m.model != nil {
		m.model.browse.SetDiscovery(discovery)
	}
}

// SetBrowseNamespaceController sets the namespace controller for the browse modal
func (m *Manager) SetBrowseNamespaceController(nc types.NamespaceController) {
	if m.model != nil {
		m.model.browse.SetNamespaceController(nc)
	}
}

// SetBrowseServiceCRUD sets the service CRUD adapter for the browse modal
func (m *Manager) SetBrowseServiceCRUD(sc types.ServiceCRUD) {
	if m.model != nil {
		m.model.browse.SetServiceCRUD(sc)
	}
}

// SetHeaderContext sets the current context displayed in the header
func (m *Manager) SetHeaderContext(ctx string) {
	if m.model != nil {
		m.model.header.SetContext(ctx)
	}
}

// SetRemoveForwardCallback sets the function used to remove forwards
func (m *Manager) SetRemoveForwardCallback(remover func(key string) error) {
	if m.model != nil {
		m.model.removeForward = remover
	}
}

// Run starts the TUI application
func (m *Manager) Run() error {
	// Ensure TERM is set
	if os.Getenv("TERM") == "" {
		_ = os.Setenv("TERM", "xterm-256color")
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
func (m *RootModel) Init() tea.Cmd {
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
func (m *RootModel) Update(msg tea.Msg) (model tea.Model, cmd tea.Cmd) {
	// Panic recovery to prevent TUI crash from leaving terminal in broken state
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("TUI Update panic recovered: %v", r)
			model = m
			cmd = nil
		}
	}()

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.handleWindowSizeMsg(msg)
	case tea.KeyMsg:
		return m.handleKeyMsg(msg)
	case tea.MouseMsg:
		return m.handleMouseMsg(msg)
	case MetricsUpdateMsg:
		return m, m.handleMetricsUpdateMsg(msg)
	case KubefwdEventMsg:
		return m, m.handleKubefwdEventMsg(msg)
	case LogEntryMsg:
		return m, m.handleLogEntryMsg(msg)
	case ShutdownMsg:
		return m.handleShutdownMsg()
	default:
		// Check component messages
		if result, cmd, handled := m.handleComponentMessage(msg); handled {
			return result, cmd
		}
		// Check browse messages
		if result, cmd, handled := m.handleBrowseMessage(msg); handled {
			return result, cmd
		}
	}

	return m, nil
}

// waitForLogLine returns a command that waits for the next log line
func (m *RootModel) waitForLogLine() tea.Cmd {
	logCh := m.logStreamCh
	if logCh == nil {
		return nil
	}
	return func() tea.Msg {
		line, ok := <-logCh
		if !ok {
			// Channel closed, stream ended
			return components.PodLogsStopMsg{}
		}
		return components.PodLogLineMsg{Line: line}
	}
}

// View renders the UI
func (m *RootModel) View() string {
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

	// Overlay browse modal if visible
	if m.browse.IsVisible() {
		return m.browse.View()
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
		logsTitle,
		logsContent,
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

	// Fixed lines: section titles (2) + blank line before Services (1)
	fixedLines := 3

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

	// Calculate Y positions for click detection
	// Layout: header, blank, "Services" title, services content, "Logs" title, logs content, status
	m.servicesStartY = headerHeight + 2                    // after header + blank + title
	m.logsStartY = m.servicesStartY + m.servicesHeight + 1 // after services + title

	// Set component sizes - they handle their own rendering
	m.services.SetSize(contentWidth, servicesHeight)
	m.logs.SetSize(contentWidth, logsHeight)
	m.statusBar.SetWidth(m.width)
	m.detail.SetSize(m.width, m.height)
	m.browse.SetSize(m.width, m.height)
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
	case FocusDetail:
		// FocusDetail doesn't participate in tab cycling
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
				key := svc.ServiceName + "." + svc.Namespace + "." + svc.Context + "." + pf.PodName + "." + pf.LocalPort
				if key == detailKey && len(pf.HTTPLogs) > 0 {
					// Convert metrics HTTP logs to detail HTTP logs
					logs := make([]components.HTTPLogEntry, len(pf.HTTPLogs))
					for i, httpLog := range pf.HTTPLogs {
						logs[i] = components.HTTPLogEntry{
							Timestamp:  httpLog.Timestamp,
							Method:     httpLog.Method,
							Path:       httpLog.Path,
							StatusCode: httpLog.StatusCode,
							Duration:   httpLog.Duration,
							Size:       httpLog.Size,
						}
					}
					m.detail.SetHTTPLogs(logs)
					break
				}
			}
		}
	}
}

// handleWindowSizeMsg handles terminal resize events
func (m *RootModel) handleWindowSizeMsg(msg tea.WindowSizeMsg) {
	m.width = msg.Width
	m.height = msg.Height
	m.updateSizes()
	m.header, _ = m.header.Update(msg)
	m.statusBar, _ = m.statusBar.Update(msg)
	m.help, _ = m.help.Update(msg)
}

// handleKeyMsg handles keyboard input
func (m *RootModel) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Help modal captures all input when visible
	if m.help.IsVisible() {
		m.help, _ = m.help.Update(msg)
		return m, nil
	}

	// Detail view captures all input when visible
	if m.detail.IsVisible() {
		var cmd tea.Cmd
		m.detail, cmd = m.detail.Update(msg)
		if !m.detail.IsVisible() {
			m.focus = FocusServices
		}
		return m, cmd
	}

	// Browse modal captures all input when visible
	if m.browse.IsVisible() {
		var cmd tea.Cmd
		m.browse, cmd = m.browse.Update(msg)
		if ctx := m.browse.GetCurrentContext(); ctx != "" {
			m.header.SetContext(ctx)
		}
		return m, cmd
	}

	// Handle global keys
	if result, cmd, handled := m.handleGlobalKeys(msg); handled {
		return result, cmd
	}

	// Route to focused component
	return m.handleFocusedComponentKey(msg)
}

// handleGlobalKeys handles global keyboard shortcuts
func (m *RootModel) handleGlobalKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit, true
	case "q":
		if !m.services.IsFiltering() {
			m.quitting = true
			return m, tea.Quit, true
		}
	case "?":
		m.help.Toggle()
		return m, nil, true
	case "tab":
		m.cycleFocus()
		return m, nil, true
	case "r":
		if !m.services.IsFiltering() && m.reconnectErrored != nil {
			count := m.reconnectErrored()
			if count > 0 {
				return m, SendLog(log.InfoLevel,
					fmt.Sprintf("Reconnecting %d service(s) with errors...", count)), true
			}
			return m, SendLog(log.InfoLevel, "No services with errors to reconnect"), true
		}
	case "f":
		if !m.services.IsFiltering() {
			m.browse.SetSize(m.width, m.height)
			return m, m.browse.Show(), true
		}
	}
	return nil, nil, false
}

// handleFocusedComponentKey routes key to the focused component
func (m *RootModel) handleFocusedComponentKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.focus {
	case FocusServices:
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
	case FocusLogs:
		m.logs, cmd = m.logs.Update(msg)
	case FocusDetail:
		// Detail view handles its own keys in handleKeyMsg before reaching here
	}
	return m, cmd
}

// handleMouseMsg handles mouse input
func (m *RootModel) handleMouseMsg(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Handle mouse wheel scrolling
	if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
		var cmd tea.Cmd
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
			case FocusDetail:
				// Detail visible case handled above
			}
		}
	}

	// Handle click for focus switching
	if msg.Button == tea.MouseButtonLeft {
		return m.handleMouseClick(msg)
	}

	return m, tea.Batch(cmds...)
}

// handleMouseClick handles mouse click events
func (m *RootModel) handleMouseClick(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	y := msg.Y

	// Check if click is in Services area
	if y >= m.servicesStartY && y < m.servicesStartY+m.servicesHeight {
		m.focus = FocusServices
		m.services.SetFocus(true)
		m.logs.SetFocus(false)
		relativeY := y - m.servicesStartY
		m.services.SelectByY(relativeY)
		return m, nil
	}

	// Check if click is in Logs area
	if y >= m.logsStartY && y < m.logsStartY+m.logsHeight {
		m.focus = FocusLogs
		m.services.SetFocus(false)
		m.logs.SetFocus(true)
		return m, nil
	}

	return m, nil
}

// handlePodLogsRequest handles pod log streaming requests
func (m *RootModel) handlePodLogsRequest(msg components.PodLogsRequestMsg) (tea.Model, tea.Cmd) {
	if m.streamPodLogs == nil {
		m.detail.SetLogsError("Pod logs not available - streamer not configured")
		return m, nil
	}

	// Cancel any existing stream
	if m.logStreamCancel != nil {
		m.logStreamCancel()
	}

	// Create new context for this stream
	ctx, cancel := context.WithCancel(context.Background())
	m.logStreamCancel = cancel
	m.logStreamCh = make(chan string, 100)

	namespace := msg.Namespace
	podName := msg.PodName
	containerName := msg.ContainerName
	k8sContext := msg.Context
	tailLines := msg.TailLines
	logCh := m.logStreamCh

	go m.streamPodLogsGoroutine(ctx, logCh, namespace, podName, containerName, k8sContext, tailLines)

	m.detail.SetLogsStreaming(true)
	m.detail.SetLogsLoading(false)
	return m, m.waitForLogLine()
}

// streamPodLogsGoroutine streams pod logs in background
func (m *RootModel) streamPodLogsGoroutine(ctx context.Context, logCh chan<- string, namespace, podName, containerName, k8sContext string, tailLines int64) {
	defer close(logCh)

	stream, err := m.streamPodLogs(ctx, namespace, podName, containerName, k8sContext, tailLines)
	if err != nil {
		select {
		case <-ctx.Done():
			return
		default:
			logCh <- "\x00ERROR:" + err.Error()
			return
		}
	}
	defer func() { _ = stream.Close() }()

	scanner := bufio.NewScanner(stream)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		case logCh <- scanner.Text():
		}
	}
}

// handlePodLogLine handles individual log lines from the stream
func (m *RootModel) handlePodLogLine(msg components.PodLogLineMsg) (tea.Model, tea.Cmd) {
	line := msg.Line
	if len(line) > 7 && line[:7] == "\x00ERROR:" {
		m.detail.SetLogsError(line[7:])
		m.detail.SetLogsStreaming(false)
	} else {
		m.detail.AppendLogLine(line)
		if m.detail.IsLogsStreaming() && m.logStreamCh != nil {
			return m, m.waitForLogLine()
		}
	}
	return m, nil
}

// handleBrowseMessage handles browse modal related messages
func (m *RootModel) handleBrowseMessage(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case components.BrowseContextsLoadedMsg:
		var cmd tea.Cmd
		m.browse, cmd = m.browse.Update(msg)
		if msg.CurrentContext != "" {
			m.header.SetContext(msg.CurrentContext)
		}
		return m, cmd, true
	case components.BrowseNamespacesLoadedMsg, components.BrowseServicesLoadedMsg:
		var cmd tea.Cmd
		m.browse, cmd = m.browse.Update(msg)
		return m, cmd, true
	case components.ServiceForwardedMsg:
		var cmd tea.Cmd
		m.browse, cmd = m.browse.Update(msg)
		if msg.Error == nil {
			return m, tea.Batch(cmd, SendLog(log.InfoLevel,
				fmt.Sprintf("Forwarded service %s (%s)", msg.ServiceName, msg.LocalIP))), true
		}
		return m, cmd, true
	case components.NamespaceForwardedMsg:
		var cmd tea.Cmd
		m.browse, cmd = m.browse.Update(msg)
		if msg.Error == nil {
			return m, tea.Batch(cmd, SendLog(log.InfoLevel,
				fmt.Sprintf("Forwarded %d services from namespace %s", msg.ServiceCount, msg.Namespace))), true
		}
		return m, cmd, true
	}
	return nil, nil, false
}

// handleMetricsUpdateMsg handles metrics update messages
func (m *RootModel) handleMetricsUpdateMsg(msg MetricsUpdateMsg) tea.Cmd {
	m.handleMetricsUpdate(msg)
	m.services.Refresh()
	m.statusBar.UpdateStats(m.store.GetSummary())
	return ListenMetrics(m.metricsCh)
}

// handleKubefwdEventMsg handles kubefwd event messages
func (m *RootModel) handleKubefwdEventMsg(msg KubefwdEventMsg) tea.Cmd {
	m.handleKubefwdEvent(msg.Event)
	m.services.Refresh()
	m.statusBar.UpdateStats(m.store.GetSummary())
	return ListenEvents(m.eventCh)
}

// handleLogEntryMsg handles log entry messages
func (m *RootModel) handleLogEntryMsg(msg LogEntryMsg) tea.Cmd {
	m.logs.AppendLog(msg.Level, msg.Message, msg.Time)
	return ListenLogs(m.logCh)
}

// handleOpenDetailMsg handles opening the detail view
func (m *RootModel) handleOpenDetailMsg(msg components.OpenDetailMsg) {
	if msg.Key != "" {
		m.detail.Show(msg.Key)
		m.detail.SetSize(m.width, m.height)
		m.focus = FocusDetail
	}
}

// handleRefreshMsg handles refresh requests
func (m *RootModel) handleRefreshMsg() {
	m.services.Refresh()
	m.statusBar.UpdateStats(m.store.GetSummary())
}

// handlePodLogsErrorMsg handles pod logs error messages
func (m *RootModel) handlePodLogsErrorMsg(msg components.PodLogsErrorMsg) {
	m.detail.SetLogsError(msg.Error.Error())
	m.detail.SetLogsStreaming(false)
}

// handleShutdownMsg handles shutdown messages
func (m *RootModel) handleShutdownMsg() (tea.Model, tea.Cmd) {
	m.quitting = true
	return m, tea.Quit
}

// handleComponentMessage handles component-specific messages
func (m *RootModel) handleComponentMessage(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case components.OpenDetailMsg:
		m.handleOpenDetailMsg(msg)
		return m, nil, true
	case RefreshMsg:
		m.handleRefreshMsg()
		return m, nil, true
	case components.PodLogsRequestMsg:
		result, cmd := m.handlePodLogsRequest(msg)
		return result, cmd, true
	case components.PodLogsStopMsg:
		m.handlePodLogsStopMsg()
		return m, nil, true
	case components.PodLogLineMsg:
		result, cmd := m.handlePodLogLine(msg)
		return result, cmd, true
	case components.PodLogsErrorMsg:
		m.handlePodLogsErrorMsg(msg)
		return m, nil, true
	case components.ReconnectErroredMsg:
		result, cmd := m.handleReconnectErroredMsg()
		return result, cmd, true
	case components.RemoveForwardMsg:
		result, cmd := m.handleRemoveForward(msg)
		return result, cmd, true
	}
	return nil, nil, false
}

// handlePodLogsStopMsg handles stopping pod log streaming
func (m *RootModel) handlePodLogsStopMsg() {
	if m.logStreamCancel != nil {
		m.logStreamCancel()
		m.logStreamCancel = nil
	}
	m.logStreamCh = nil
}

// handleReconnectErroredMsg handles reconnect requests
func (m *RootModel) handleReconnectErroredMsg() (tea.Model, tea.Cmd) {
	if m.reconnectErrored != nil {
		count := m.reconnectErrored()
		if count > 0 {
			return m, SendLog(log.InfoLevel,
				fmt.Sprintf("Reconnecting %d service(s) with errors...", count))
		}
		return m, SendLog(log.InfoLevel, "No services with errors to reconnect")
	}
	return m, nil
}

// handleRemoveForward handles forward removal requests
func (m *RootModel) handleRemoveForward(msg components.RemoveForwardMsg) (tea.Model, tea.Cmd) {
	if msg.RegistryKey != "" && m.removeForward != nil {
		if err := m.removeForward(msg.RegistryKey); err != nil {
			return m, SendLog(log.ErrorLevel,
				fmt.Sprintf("Failed to remove forward %s: %v", msg.RegistryKey, err))
		}
		return m, SendLog(log.InfoLevel,
			fmt.Sprintf("Removed forward: %s", msg.RegistryKey))
	}
	return m, nil
}

// buildForwardKey creates a forward key from event data
func buildForwardKey(e events.Event) string {
	return e.ServiceKey + "." + e.PodName + "." + e.LocalPort
}

// parseStatusString converts a status string to ForwardStatus
func parseStatusString(status string) state.ForwardStatus {
	switch status {
	case "connecting":
		return state.StatusConnecting
	case "active":
		return state.StatusActive
	case "error":
		return state.StatusError
	case "stopping":
		return state.StatusStopping
	default:
		return state.StatusPending
	}
}

// handlePodAddedEvent processes a PodAdded event
func (m *RootModel) handlePodAddedEvent(e events.Event) {
	key := buildForwardKey(e)
	snapshot := state.ForwardSnapshot{
		Key:           key,
		ServiceKey:    e.ServiceKey,
		RegistryKey:   e.RegistryKey,
		ServiceName:   e.Service,
		Namespace:     e.Namespace,
		Context:       e.Context,
		PodName:       e.PodName,
		ContainerName: e.ContainerName,
		LocalIP:       e.LocalIP,
		LocalPort:     e.LocalPort,
		PodPort:       e.PodPort,
		Hostnames:     e.Hostnames,
		Status:        state.StatusConnecting,
		StartedAt:     e.Timestamp,
	}
	m.store.AddForward(snapshot)
}

// handlePodStatusChangedEvent processes a PodStatusChanged event
func (m *RootModel) handlePodStatusChangedEvent(e events.Event) {
	key := buildForwardKey(e)
	status := parseStatusString(e.Status)
	errorMsg := ""
	if e.Error != nil {
		errorMsg = e.Error.Error()
	}
	m.store.UpdateStatus(key, status, errorMsg)

	if e.Status == "active" && len(e.Hostnames) > 0 {
		m.store.UpdateHostnames(key, e.Hostnames)
	}
}

// handleServiceRemovedEvent processes a ServiceRemoved event
func (m *RootModel) handleServiceRemovedEvent(e events.Event) {
	forwards := m.store.GetFiltered()
	for _, fwd := range forwards {
		if fwd.ServiceKey == e.ServiceKey {
			m.store.RemoveForward(fwd.Key)
		}
	}
}

// handleKubefwdEvent processes kubefwd events
func (m *RootModel) handleKubefwdEvent(e events.Event) {
	switch e.Type {
	case events.PodAdded:
		m.handlePodAddedEvent(e)
	case events.PodRemoved:
		m.store.RemoveForward(buildForwardKey(e))
	case events.PodStatusChanged:
		m.handlePodStatusChangedEvent(e)
	case events.ServiceRemoved:
		m.handleServiceRemovedEvent(e)
	case events.NamespaceRemoved:
		m.store.RemoveByNamespace(e.Namespace, e.Context)
	case events.ServiceAdded, events.ServiceUpdated, events.BandwidthUpdate, events.LogMessage, events.ShutdownStarted, events.ShutdownComplete:
		// These events are handled elsewhere or ignored
	}
}

// tuiLogHook is a logrus hook that sends logs to the TUI
type tuiLogHook struct {
	logCh  chan<- LogEntryMsg
	stopCh <-chan struct{}
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
		// Buffer full - silently drop during shutdown
		select {
		case <-h.stopCh:
			// Shutdown in progress, silently drop
		default:
			// Not shutting down but buffer full - this is rare
		}
	}
	return nil
}
