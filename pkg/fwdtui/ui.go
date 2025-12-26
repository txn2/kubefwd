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
	reconnectErrored func() int // Returns number of services reconnected

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

// GetEventBus returns the global event bus (nil if TUI not initialized)
func GetEventBus() *events.Bus {
	mu.RLock()
	defer mu.RUnlock()
	if tuiManager != nil {
		return tuiManager.eventBus
	}
	return nil
}

// GetStore returns the global state store (nil if TUI not initialized)
func GetStore() *state.Store {
	mu.RLock()
	defer mu.RUnlock()
	if tuiManager != nil {
		return tuiManager.store
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
			var cmd tea.Cmd
			m.detail, cmd = m.detail.Update(msg)
			// If detail closed, return focus to services
			if !m.detail.IsVisible() {
				m.focus = FocusServices
			}
			return m, cmd
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
		case "r":
			if !m.services.IsFiltering() {
				if m.reconnectErrored != nil {
					count := m.reconnectErrored()
					if count > 0 {
						return m, SendLog(log.InfoLevel,
							fmt.Sprintf("Reconnecting %d service(s) with errors...", count))
					}
					return m, SendLog(log.InfoLevel, "No services with errors to reconnect")
				}
			}
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
		default:
			// FocusDetail handled separately
		}

	case tea.MouseMsg:
		// Handle mouse wheel scrolling
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
				default:
					// FocusDetail handled above
				}
			}
		}

		// Handle click for focus switching (and row selection in Services)
		if msg.Button == tea.MouseButtonLeft {
			y := msg.Y

			// Check if click is in Services area
			if y >= m.servicesStartY && y < m.servicesStartY+m.servicesHeight {
				m.focus = FocusServices
				m.services.SetFocus(true)
				m.logs.SetFocus(false)

				// Calculate relative Y within services panel and select row
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
		}

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

	case components.PodLogsRequestMsg:
		// Request to stream pod logs
		if m.streamPodLogs != nil {
			// Cancel any existing stream
			if m.logStreamCancel != nil {
				m.logStreamCancel()
			}

			// Create new context for this stream
			ctx, cancel := context.WithCancel(context.Background())
			m.logStreamCancel = cancel

			// Create channel for log lines
			m.logStreamCh = make(chan string, 100)

			namespace := msg.Namespace
			podName := msg.PodName
			containerName := msg.ContainerName
			k8sContext := msg.Context
			tailLines := msg.TailLines
			logCh := m.logStreamCh

			// Start streaming goroutine
			go func() {
				defer close(logCh)

				stream, err := m.streamPodLogs(ctx, namespace, podName, containerName, k8sContext, tailLines)
				if err != nil {
					select {
					case <-ctx.Done():
						return
					default:
						// Send error as a special message
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
			}()

			// Set streaming state and return command to listen for lines
			m.detail.SetLogsStreaming(true)
			m.detail.SetLogsLoading(false)
			return m, m.waitForLogLine()
		}

		// No streamer configured
		m.detail.SetLogsError("Pod logs not available - streamer not configured")

	case components.PodLogsStopMsg:
		// Stop the current log stream
		if m.logStreamCancel != nil {
			m.logStreamCancel()
			m.logStreamCancel = nil
		}
		m.logStreamCh = nil

	case components.PodLogLineMsg:
		// Received a log line from the stream
		line := msg.Line
		// Check for error marker
		if len(line) > 7 && line[:7] == "\x00ERROR:" {
			m.detail.SetLogsError(line[7:])
			m.detail.SetLogsStreaming(false)
		} else {
			// Update the detail model
			m.detail.AppendLogLine(line)
			// Continue listening for more lines if still streaming
			if m.detail.IsLogsStreaming() && m.logStreamCh != nil {
				return m, m.waitForLogLine()
			}
		}

	case components.PodLogsErrorMsg:
		m.detail.SetLogsError(msg.Error.Error())
		m.detail.SetLogsStreaming(false)

	case components.ReconnectErroredMsg:
		// Handle reconnect request from detail view
		if m.reconnectErrored != nil {
			count := m.reconnectErrored()
			if count > 0 {
				return m, SendLog(log.InfoLevel,
					fmt.Sprintf("Reconnecting %d service(s) with errors...", count))
			}
			return m, SendLog(log.InfoLevel, "No services with errors to reconnect")
		}
	}

	return m, tea.Batch(cmds...)
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
	default:
		// FocusDetail doesn't cycle
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

// handleKubefwdEvent processes kubefwd events
func (m *RootModel) handleKubefwdEvent(e events.Event) {
	switch e.Type {
	case events.PodAdded:
		snapshot := state.ForwardSnapshot{
			Key:           e.ServiceKey + "." + e.PodName + "." + e.LocalPort,
			ServiceKey:    e.ServiceKey,
			RegistryKey:   e.RegistryKey, // for reconnection lookup
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
	default:
		// Other event types (ServiceAdded, ServiceUpdated, BandwidthUpdate, LogMessage, etc.)
		// are handled elsewhere or not needed for TUI state
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
		// Buffer full - drop log but write to stderr as fallback
		// This is rare and indicates very high log volume
		_, _ = fmt.Fprintf(os.Stderr, "TUI: dropped log (buffer full): %s\n", entry.Message)
	}
	return nil
}
