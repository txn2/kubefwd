package components

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/txn2/kubefwd/pkg/fwdtui/state"
	"github.com/txn2/kubefwd/pkg/fwdtui/styles"
)

// Tab constants
const (
	TabInfo = iota
	TabHTTP
	TabLogs
)

// HTTPLogEntry represents an HTTP request/response log entry
type HTTPLogEntry struct {
	Timestamp  time.Time
	Method     string
	Path       string
	StatusCode int
	Duration   time.Duration
	Size       int64
}

// DetailModel displays detailed information for a selected forward
type DetailModel struct {
	visible     bool
	width       int
	height      int
	forwardKey  string
	snapshot    *state.ForwardSnapshot
	store       *state.Store
	rateHistory *state.RateHistory

	// Tab state
	currentTab int // 0=Info, 1=HTTP, 2=Logs

	// HTTP tab state
	httpLogs         []HTTPLogEntry
	httpScrollOffset int
	maxHTTPLogs      int

	// Logs tab state
	podLogs          []string
	logsScrollOffset int
	logsLoading      bool
	logsStreaming    bool
	logsAutoFollow   bool // auto-scroll to bottom on new logs
	logsError        string
	maxPodLogs       int

	// Sparkline width
	sparklineWidth int

	// Clipboard feedback
	copiedIndex   int  // -1 = none, 0+ = which connect string was copied
	copiedVisible bool // Show "Copied!" feedback
}

// NewDetailModel creates a new detail view model
func NewDetailModel(store *state.Store, history *state.RateHistory) DetailModel {
	return DetailModel{
		store:          store,
		rateHistory:    history,
		maxHTTPLogs:    100,
		maxPodLogs:     1000,
		sparklineWidth: 30,
		copiedIndex:    -1,
	}
}

// Show opens the detail view for the given forward key
func (m *DetailModel) Show(forwardKey string) {
	m.visible = true
	m.forwardKey = forwardKey
	m.currentTab = TabInfo
	m.httpScrollOffset = 0
	m.logsScrollOffset = 0
	m.podLogs = nil
	m.logsLoading = false
	m.logsStreaming = false
	m.logsAutoFollow = true // auto-follow by default
	m.logsError = ""
	m.copiedIndex = -1
	m.copiedVisible = false

	// Get initial snapshot
	if m.store != nil {
		m.snapshot = m.store.GetForward(forwardKey)
	}
}

// Hide closes the detail view and returns a command to stop log streaming
func (m *DetailModel) Hide() tea.Cmd {
	wasStreaming := m.logsStreaming
	m.visible = false
	m.forwardKey = ""
	m.snapshot = nil
	m.logsStreaming = false

	if wasStreaming {
		return func() tea.Msg {
			return PodLogsStopMsg{}
		}
	}
	return nil
}

// IsVisible returns whether the detail view is visible
func (m DetailModel) IsVisible() bool {
	return m.visible
}

// GetForwardKey returns the key of the currently displayed forward
func (m DetailModel) GetForwardKey() string {
	return m.forwardKey
}

// SetSize updates the dimensions
func (m *DetailModel) SetSize(width, height int) {
	m.width = width
	m.height = height
	// Adjust sparkline width based on available space
	m.sparklineWidth = (width - 40) / 2
	if m.sparklineWidth > 40 {
		m.sparklineWidth = 40
	}
	if m.sparklineWidth < 15 {
		m.sparklineWidth = 15
	}
}

// UpdateSnapshot refreshes the snapshot data
func (m *DetailModel) UpdateSnapshot() {
	if m.store != nil && m.forwardKey != "" {
		m.snapshot = m.store.GetForward(m.forwardKey)
	}
}

// AddHTTPLog adds an HTTP log entry
func (m *DetailModel) AddHTTPLog(entry HTTPLogEntry) {
	m.httpLogs = append(m.httpLogs, entry)
	if len(m.httpLogs) > m.maxHTTPLogs {
		m.httpLogs = m.httpLogs[len(m.httpLogs)-m.maxHTTPLogs:]
	}
}

// ClearHTTPLogs clears all HTTP log entries
func (m *DetailModel) ClearHTTPLogs() {
	m.httpLogs = nil
	m.httpScrollOffset = 0
}

// SetHTTPLogs sets the HTTP log entries (used when getting logs from metrics)
func (m *DetailModel) SetHTTPLogs(logs []HTTPLogEntry) {
	m.httpLogs = logs
}

// SetPodLogs sets the pod logs
func (m *DetailModel) SetPodLogs(logs []string) {
	m.podLogs = logs
	m.logsLoading = false
}

// AppendLogLine appends a single log line
func (m *DetailModel) AppendLogLine(line string) {
	m.podLogs = append(m.podLogs, line)
	// Trim if exceeding max
	if len(m.podLogs) > m.maxPodLogs {
		m.podLogs = m.podLogs[len(m.podLogs)-m.maxPodLogs:]
		// Adjust scroll offset if we trimmed lines and not auto-following
		if m.logsScrollOffset > 0 && !m.logsAutoFollow {
			m.logsScrollOffset--
			if m.logsScrollOffset < 0 {
				m.logsScrollOffset = 0
			}
		}
	}
	// Auto-scroll to bottom if following
	if m.logsAutoFollow {
		maxScroll := m.getLogsMaxScroll()
		m.logsScrollOffset = maxScroll
	}
}

// SetLogsLoading sets the loading state for logs
func (m *DetailModel) SetLogsLoading(loading bool) {
	m.logsLoading = loading
}

// SetLogsStreaming sets the streaming state for logs
func (m *DetailModel) SetLogsStreaming(streaming bool) {
	m.logsStreaming = streaming
}

// IsLogsStreaming returns whether logs are currently streaming
func (m DetailModel) IsLogsStreaming() bool {
	return m.logsStreaming
}

// SetLogsError sets an error message for the logs tab
func (m *DetailModel) SetLogsError(err string) {
	m.logsError = err
	m.logsLoading = false
}

// GetCurrentTab returns the current tab index
func (m DetailModel) GetCurrentTab() int {
	return m.currentTab
}

// GetSnapshot returns the current snapshot
func (m DetailModel) GetSnapshot() *state.ForwardSnapshot {
	return m.snapshot
}

// ClearCopiedMsg is sent to clear the "Copied!" feedback
type ClearCopiedMsg struct{}

// PodLogsRequestMsg is sent to request pod logs streaming
type PodLogsRequestMsg struct {
	Namespace string
	PodName   string
	Context   string
	TailLines int64
}

// PodLogsStopMsg is sent to stop pod logs streaming
type PodLogsStopMsg struct{}

// PodLogLineMsg contains a single log line from the stream
type PodLogLineMsg struct {
	Line string
}

// PodLogsErrorMsg indicates an error fetching logs
type PodLogsErrorMsg struct {
	Error error
}

// Init implements tea.Model
func (m DetailModel) Init() tea.Cmd {
	return nil
}

// Update handles keyboard input for the detail view
func (m DetailModel) Update(msg tea.Msg) (DetailModel, tea.Cmd) {
	switch msg := msg.(type) {
	case ClearCopiedMsg:
		m.copiedVisible = false
		m.copiedIndex = -1
		return m, nil

	case PodLogLineMsg:
		// Append log line from stream
		m.AppendLogLine(msg.Line)
		return m, nil

	case PodLogsErrorMsg:
		m.logsError = msg.Error.Error()
		m.logsLoading = false
		m.logsStreaming = false
		return m, nil

	case tea.KeyMsg:
		key := msg.String()

		// Number keys 1-9 copy specific connect strings (only on Info tab)
		if m.currentTab == TabInfo && len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
			idx := int(key[0] - '1') // Convert '1'-'9' to 0-8
			connectStrings := m.getConnectStrings()
			if idx < len(connectStrings) {
				if copyToClipboard(connectStrings[idx]) {
					m.copiedIndex = idx
					m.copiedVisible = true
					return m, clearCopiedAfterDelay()
				}
			}
			return m, nil
		}

		switch key {
		case "esc", "q":
			cmd := m.Hide()
			return m, cmd

		case "tab", "right":
			prevTab := m.currentTab
			m.currentTab = (m.currentTab + 1) % 3

			var cmds []tea.Cmd

			// Stop streaming if leaving Logs tab
			if prevTab == TabLogs && m.logsStreaming {
				m.logsStreaming = false
				cmds = append(cmds, func() tea.Msg { return PodLogsStopMsg{} })
			}

			// Start streaming if entering Logs tab
			if m.currentTab == TabLogs && !m.logsStreaming && !m.logsLoading {
				m.logsLoading = true
				m.podLogs = nil // Clear old logs
				m.logsScrollOffset = 0
				cmds = append(cmds, m.requestPodLogs())
			}

			if len(cmds) > 0 {
				return m, tea.Batch(cmds...)
			}
			return m, nil

		case "shift+tab", "left":
			prevTab := m.currentTab
			m.currentTab = (m.currentTab + 2) % 3 // +2 is same as -1 mod 3

			// Stop streaming if leaving Logs tab
			if prevTab == TabLogs && m.logsStreaming {
				m.logsStreaming = false
				return m, func() tea.Msg { return PodLogsStopMsg{} }
			}

			// Start streaming if entering Logs tab
			if m.currentTab == TabLogs && !m.logsStreaming && !m.logsLoading {
				m.logsLoading = true
				m.podLogs = nil
				m.logsScrollOffset = 0
				return m, m.requestPodLogs()
			}
			return m, nil

		case "y": // Yank/copy first connect string (only on Info tab)
			if m.currentTab == TabInfo {
				connectStrings := m.getConnectStrings()
				if len(connectStrings) > 0 {
					if copyToClipboard(connectStrings[0]) {
						m.copiedIndex = 0
						m.copiedVisible = true
						return m, clearCopiedAfterDelay()
					}
				}
			}
			return m, nil

		case "j", "down":
			// Scroll down (only on HTTP and Logs tabs)
			if m.currentTab == TabHTTP {
				maxScroll := m.getHTTPMaxScroll()
				if m.httpScrollOffset < maxScroll {
					m.httpScrollOffset++
				}
			} else if m.currentTab == TabLogs {
				maxScroll := m.getLogsMaxScroll()
				if m.logsScrollOffset < maxScroll {
					m.logsScrollOffset++
				}
				// Re-enable auto-follow if at bottom
				m.logsAutoFollow = (m.logsScrollOffset >= maxScroll)
			}
			return m, nil

		case "k", "up":
			// Scroll up (only on HTTP and Logs tabs)
			if m.currentTab == TabHTTP {
				if m.httpScrollOffset > 0 {
					m.httpScrollOffset--
				}
			} else if m.currentTab == TabLogs {
				if m.logsScrollOffset > 0 {
					m.logsScrollOffset--
					m.logsAutoFollow = false // Stop auto-follow when scrolling up
				}
			}
			return m, nil

		case "g", "home":
			// Go to top
			if m.currentTab == TabHTTP {
				m.httpScrollOffset = 0
			} else if m.currentTab == TabLogs {
				m.logsScrollOffset = 0
				m.logsAutoFollow = false
			}
			return m, nil

		case "G", "end":
			// Go to bottom
			if m.currentTab == TabHTTP {
				m.httpScrollOffset = m.getHTTPMaxScroll()
			} else if m.currentTab == TabLogs {
				m.logsScrollOffset = m.getLogsMaxScroll()
				m.logsAutoFollow = true // Re-enable at bottom
			}
			return m, nil

		case "pgdown", "ctrl+d":
			// Page down
			pageSize := m.getViewportHeight() / 2
			if pageSize < 1 {
				pageSize = 1
			}
			if m.currentTab == TabHTTP {
				m.httpScrollOffset += pageSize
				maxScroll := m.getHTTPMaxScroll()
				if m.httpScrollOffset > maxScroll {
					m.httpScrollOffset = maxScroll
				}
			} else if m.currentTab == TabLogs {
				m.logsScrollOffset += pageSize
				maxScroll := m.getLogsMaxScroll()
				if m.logsScrollOffset > maxScroll {
					m.logsScrollOffset = maxScroll
				}
				m.logsAutoFollow = (m.logsScrollOffset >= maxScroll)
			}
			return m, nil

		case "pgup", "ctrl+u":
			// Page up
			pageSize := m.getViewportHeight() / 2
			if pageSize < 1 {
				pageSize = 1
			}
			if m.currentTab == TabHTTP {
				m.httpScrollOffset -= pageSize
				if m.httpScrollOffset < 0 {
					m.httpScrollOffset = 0
				}
			} else if m.currentTab == TabLogs {
				m.logsScrollOffset -= pageSize
				if m.logsScrollOffset < 0 {
					m.logsScrollOffset = 0
				}
				m.logsAutoFollow = false
			}
			return m, nil
		}

	case tea.WindowSizeMsg:
		m.SetSize(msg.Width, msg.Height)

	case tea.MouseMsg:
		// Handle mouse wheel scrolling (only on HTTP and Logs tabs)
		if msg.Button == tea.MouseButtonWheelUp {
			if m.currentTab == TabHTTP && m.httpScrollOffset > 0 {
				m.httpScrollOffset -= 3
				if m.httpScrollOffset < 0 {
					m.httpScrollOffset = 0
				}
			} else if m.currentTab == TabLogs && m.logsScrollOffset > 0 {
				m.logsScrollOffset -= 3
				if m.logsScrollOffset < 0 {
					m.logsScrollOffset = 0
				}
				m.logsAutoFollow = false
			}
			return m, nil
		} else if msg.Button == tea.MouseButtonWheelDown {
			if m.currentTab == TabHTTP {
				maxScroll := m.getHTTPMaxScroll()
				m.httpScrollOffset += 3
				if m.httpScrollOffset > maxScroll {
					m.httpScrollOffset = maxScroll
				}
			} else if m.currentTab == TabLogs {
				maxScroll := m.getLogsMaxScroll()
				m.logsScrollOffset += 3
				if m.logsScrollOffset > maxScroll {
					m.logsScrollOffset = maxScroll
				}
				m.logsAutoFollow = (m.logsScrollOffset >= maxScroll)
			}
			return m, nil
		}
	}
	return m, nil
}

// requestPodLogs returns a command to request pod logs
func (m *DetailModel) requestPodLogs() tea.Cmd {
	if m.snapshot == nil {
		return nil
	}
	return func() tea.Msg {
		return PodLogsRequestMsg{
			Namespace: m.snapshot.Namespace,
			PodName:   m.snapshot.PodName,
			Context:   m.snapshot.Context,
			TailLines: 100,
		}
	}
}

// getViewportHeight returns the available viewport height for tab content
func (m *DetailModel) getViewportHeight() int {
	// Use same box height calculation as View()
	boxHeight := m.height - 6
	if boxHeight < 15 {
		boxHeight = 15
	}
	// Inner height minus: borders (2), padding (2), tab bar (2 lines), footer (1)
	viewportHeight := boxHeight - 7
	if viewportHeight < 5 {
		viewportHeight = 5
	}
	return viewportHeight
}

// getHTTPMaxScroll returns the maximum scroll offset for HTTP logs
func (m *DetailModel) getHTTPMaxScroll() int {
	contentLines := len(m.httpLogs)
	viewportHeight := m.getViewportHeight()
	maxScroll := contentLines - viewportHeight
	if maxScroll < 0 {
		maxScroll = 0
	}
	return maxScroll
}

// getLogsMaxScroll returns the maximum scroll offset for pod logs
func (m *DetailModel) getLogsMaxScroll() int {
	contentLines := len(m.podLogs)
	viewportHeight := m.getViewportHeight()
	// Account for streaming status line
	if m.logsStreaming {
		viewportHeight -= 2
	}
	if viewportHeight < 1 {
		viewportHeight = 1
	}
	maxScroll := contentLines - viewportHeight
	if maxScroll < 0 {
		maxScroll = 0
	}
	return maxScroll
}

// getConnectStrings returns all hostname:port combinations
func (m DetailModel) getConnectStrings() []string {
	if m.snapshot == nil {
		return nil
	}

	port := m.snapshot.LocalPort
	if port == "" {
		port = m.snapshot.PodPort
	}

	var result []string
	if len(m.snapshot.Hostnames) > 0 {
		for _, hostname := range m.snapshot.Hostnames {
			result = append(result, fmt.Sprintf("%s:%s", hostname, port))
		}
	} else {
		result = append(result, fmt.Sprintf("%s:%s", m.snapshot.ServiceName, port))
	}
	return result
}

// copyToClipboard copies text to system clipboard
func copyToClipboard(text string) bool {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "linux":
		// Try xclip first, then xsel
		if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard")
		} else if _, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.Command("xsel", "--clipboard", "--input")
		} else {
			return false
		}
	default:
		return false
	}

	pipe, err := cmd.StdinPipe()
	if err != nil {
		return false
	}

	if err := cmd.Start(); err != nil {
		return false
	}

	_, err = pipe.Write([]byte(text))
	pipe.Close()

	if err != nil {
		return false
	}

	return cmd.Wait() == nil
}

// clearCopiedAfterDelay returns a command that clears the copied feedback
func clearCopiedAfterDelay() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return ClearCopiedMsg{}
	})
}

// View renders the detail view
func (m *DetailModel) View() string {
	if !m.visible || m.snapshot == nil {
		return ""
	}

	// Calculate dimensions with padding
	contentWidth := m.width - 12
	if contentWidth < 40 {
		contentWidth = 40
	}
	// Fixed height: window height minus padding (6 lines total for top/bottom margins)
	boxHeight := m.height - 6
	if boxHeight < 15 {
		boxHeight = 15
	}

	var b strings.Builder

	// Render tab bar
	b.WriteString(m.renderTabBar())
	b.WriteString("\n\n")

	// Render content based on current tab
	var tabContent string
	switch m.currentTab {
	case TabInfo:
		tabContent = m.renderInfoTab()
	case TabHTTP:
		tabContent = m.renderHTTPTab()
	case TabLogs:
		tabContent = m.renderLogsTab()
	}
	b.WriteString(tabContent)

	// Footer with keybindings
	footer := m.renderFooter()

	// Calculate content height and pad to fill the box
	// Box inner height = boxHeight - 2 (borders) - 2 (padding from DetailBorderStyle)
	innerHeight := boxHeight - 4
	currentLines := strings.Count(b.String(), "\n") + 1 + 1 // +1 for footer line
	paddingNeeded := innerHeight - currentLines
	if paddingNeeded > 0 {
		b.WriteString(strings.Repeat("\n", paddingNeeded))
	}

	// Add footer
	b.WriteString(footer)

	// Style the box with fixed dimensions
	boxStyle := styles.DetailBorderStyle.
		Width(contentWidth).
		Height(boxHeight - 2) // -2 for top/bottom borders

	return lipgloss.Place(
		m.width,
		m.height,
		lipgloss.Center,
		lipgloss.Center,
		boxStyle.Render(b.String()),
	)
}

// renderTabBar renders the tab bar
func (m *DetailModel) renderTabBar() string {
	tabs := []string{"Info", fmt.Sprintf("HTTP (%d)", len(m.httpLogs)), "Logs"}

	var parts []string
	for i, tab := range tabs {
		if i == m.currentTab {
			// Active tab
			parts = append(parts, styles.TabActiveStyle.Render("["+tab+"]"))
		} else {
			// Inactive tab
			parts = append(parts, styles.TabInactiveStyle.Render("["+tab+"]"))
		}
	}

	return strings.Join(parts, " ")
}

// renderInfoTab renders the Info tab content
func (m *DetailModel) renderInfoTab() string {
	var b strings.Builder

	// Status
	b.WriteString(styles.DetailLabelStyle.Render("STATUS: "))
	b.WriteString(m.renderStatus(m.snapshot.Status))
	if m.snapshot.Error != "" {
		b.WriteString(" - ")
		b.WriteString(styles.StatusErrorStyle.Render(m.snapshot.Error))
	}
	b.WriteString("\n\n")

	// Connect Strings - show numbered hostname:port combinations for copying
	connectStrings := m.getConnectStrings()
	sectionTitle := "CONNECT STRINGS"
	if m.copiedVisible {
		sectionTitle += " ✓ Copied!"
	} else {
		sectionTitle += " (press 1-9 to copy)"
	}
	b.WriteString(styles.DetailSectionStyle.Render(sectionTitle))
	b.WriteString("\n")

	for i, connectStr := range connectStrings {
		if i >= 9 {
			break // Only show up to 9 (keys 1-9)
		}
		num := fmt.Sprintf("[%d] ", i+1)
		b.WriteString("  ")
		if m.copiedVisible && m.copiedIndex == i {
			// Highlight the copied item
			b.WriteString(styles.StatusActiveStyle.Render(num + connectStr + " ✓"))
		} else {
			b.WriteString(styles.DetailFooterKeyStyle.Render(num))
			b.WriteString(styles.DetailHostnameStyle.Render(connectStr))
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")

	// Pod Info
	b.WriteString(styles.DetailSectionStyle.Render("POD"))
	b.WriteString("\n")
	b.WriteString("  ")
	b.WriteString(styles.DetailValueStyle.Render(m.snapshot.PodName))
	b.WriteString("\n")
	if !m.snapshot.StartedAt.IsZero() {
		duration := time.Since(m.snapshot.StartedAt)
		b.WriteString("  ")
		b.WriteString(styles.DetailLabelStyle.Render("Connected: "))
		b.WriteString(styles.DetailValueStyle.Render(formatDuration(duration)))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	// Bandwidth
	b.WriteString(styles.DetailSectionStyle.Render("BANDWIDTH"))
	b.WriteString("\n")

	// Get rate history for sparklines
	var rateInHistory, rateOutHistory []float64
	if m.rateHistory != nil {
		rateInHistory, rateOutHistory = m.rateHistory.GetHistory(m.forwardKey, m.sparklineWidth)
	}

	// Received
	b.WriteString("  ")
	b.WriteString(styles.DetailLabelStyle.Render("Received: "))
	b.WriteString(styles.DetailValueStyle.Render(humanBytes(m.snapshot.BytesIn)))
	b.WriteString(" (")
	b.WriteString(styles.DetailValueStyle.Render(humanRate(m.snapshot.RateIn)))
	b.WriteString(") ")
	if len(rateInHistory) > 0 {
		sparkline := RenderSparkline(rateInHistory, m.sparklineWidth)
		b.WriteString(styles.SparklineInStyle.Render(sparkline))
	}
	b.WriteString("\n")

	// Sent
	b.WriteString("  ")
	b.WriteString(styles.DetailLabelStyle.Render("Sent:     "))
	b.WriteString(styles.DetailValueStyle.Render(humanBytes(m.snapshot.BytesOut)))
	b.WriteString(" (")
	b.WriteString(styles.DetailValueStyle.Render(humanRate(m.snapshot.RateOut)))
	b.WriteString(") ")
	if len(rateOutHistory) > 0 {
		sparkline := RenderSparkline(rateOutHistory, m.sparklineWidth)
		b.WriteString(styles.SparklineOutStyle.Render(sparkline))
	}
	b.WriteString("\n")

	return b.String()
}

// renderHTTPTab renders the HTTP tab content
func (m *DetailModel) renderHTTPTab() string {
	var b strings.Builder

	if len(m.httpLogs) == 0 {
		b.WriteString(styles.DetailLabelStyle.Render("No HTTP requests captured"))
		b.WriteString("\n\n")
		b.WriteString(styles.DetailLabelStyle.Render("HTTP sniffing captures requests passing through the port forward."))
		return b.String()
	}

	viewportHeight := m.getViewportHeight()

	// Show logs in chronological order (oldest first, newest at bottom)
	start := m.httpScrollOffset
	end := start + viewportHeight
	if end > len(m.httpLogs) {
		end = len(m.httpLogs)
	}

	for i := start; i < end; i++ {
		entry := m.httpLogs[i]

		// Timestamp
		ts := entry.Timestamp.Format("15:04:05")
		b.WriteString(styles.HTTPTimestampStyle.Render(ts))
		b.WriteString(" ")

		// Method with color
		method := m.renderMethod(entry.Method)
		b.WriteString(method)
		b.WriteString(" ")

		// Path (truncate if too long)
		path := entry.Path
		maxPathLen := 30
		if len(path) > maxPathLen {
			path = path[:maxPathLen-3] + "..."
		}
		b.WriteString(styles.HTTPPathStyle.Render(fmt.Sprintf("%-30s", path)))
		b.WriteString(" ")

		// Status code with color
		if entry.StatusCode > 0 {
			status := m.renderStatusCode(entry.StatusCode)
			b.WriteString(status)
			b.WriteString(" ")
		}

		// Duration
		if entry.Duration > 0 {
			dur := fmt.Sprintf("%4dms", entry.Duration.Milliseconds())
			b.WriteString(styles.HTTPDurationStyle.Render(dur))
			b.WriteString(" ")
		}

		// Size
		if entry.Size > 0 {
			b.WriteString(styles.DetailValueStyle.Render(humanBytes(uint64(entry.Size))))
		}

		b.WriteString("\n")
	}

	// Scroll indicator
	maxScroll := m.getHTTPMaxScroll()
	if maxScroll > 0 {
		b.WriteString("\n")
		scrollInfo := fmt.Sprintf("[%d/%d]", m.httpScrollOffset+1, maxScroll+1)
		if m.httpScrollOffset > 0 {
			scrollInfo = "↑ " + scrollInfo
		}
		if m.httpScrollOffset < maxScroll {
			scrollInfo = scrollInfo + " ↓"
		}
		b.WriteString(styles.DetailLabelStyle.Render(scrollInfo))
	}

	return b.String()
}

// renderLogsTab renders the Logs tab content
func (m *DetailModel) renderLogsTab() string {
	var b strings.Builder

	// Show error if present
	if m.logsError != "" {
		b.WriteString(styles.StatusErrorStyle.Render("Error: " + m.logsError))
		return b.String()
	}

	if m.logsLoading && len(m.podLogs) == 0 {
		b.WriteString(styles.DetailLabelStyle.Render("Loading pod logs..."))
		return b.String()
	}

	if len(m.podLogs) == 0 {
		b.WriteString(styles.DetailLabelStyle.Render("No pod logs available"))
		return b.String()
	}

	// Status line: streaming state and follow indicator
	if m.logsStreaming {
		if m.logsAutoFollow {
			b.WriteString(styles.StatusActiveStyle.Render("● Following"))
		} else {
			b.WriteString(styles.StatusConnectingStyle.Render("● Paused"))
			b.WriteString(styles.DetailLabelStyle.Render(" (scroll to bottom to resume)"))
		}
		b.WriteString(styles.DetailLabelStyle.Render(fmt.Sprintf(" - %d lines", len(m.podLogs))))
		b.WriteString("\n\n")
	}

	viewportHeight := m.getViewportHeight()
	if m.logsStreaming {
		viewportHeight -= 2 // Account for status line
	}
	if viewportHeight < 1 {
		viewportHeight = 1
	}

	// Apply scrolling
	start := m.logsScrollOffset
	end := start + viewportHeight
	if end > len(m.podLogs) {
		end = len(m.podLogs)
	}
	if start > len(m.podLogs) {
		start = len(m.podLogs)
	}

	for i := start; i < end; i++ {
		line := m.podLogs[i]
		// Truncate long lines
		maxLen := m.width - 20
		if maxLen < 40 {
			maxLen = 40
		}
		if len(line) > maxLen {
			line = line[:maxLen-3] + "..."
		}
		b.WriteString(styles.DetailValueStyle.Render(line))
		b.WriteString("\n")
	}

	// Scroll indicator (only when not following or there's scrollable content)
	maxScroll := m.getLogsMaxScroll()
	if maxScroll > 0 && !m.logsAutoFollow {
		scrollInfo := fmt.Sprintf("[%d/%d]", m.logsScrollOffset+1, maxScroll+1)
		if m.logsScrollOffset > 0 {
			scrollInfo = "↑ " + scrollInfo
		}
		if m.logsScrollOffset < maxScroll {
			scrollInfo = scrollInfo + " ↓"
		}
		b.WriteString(styles.DetailLabelStyle.Render(scrollInfo))
	}

	return b.String()
}

// renderStatus renders the status with appropriate styling
func (m DetailModel) renderStatus(status state.ForwardStatus) string {
	switch status {
	case state.StatusActive:
		return styles.StatusActiveStyle.Render("Active")
	case state.StatusConnecting:
		return styles.StatusConnectingStyle.Render("Connecting")
	case state.StatusError:
		return styles.StatusErrorStyle.Render("Error")
	case state.StatusStopping:
		return styles.StatusStoppingStyle.Render("Stopping")
	case state.StatusPending:
		return styles.StatusPendingStyle.Render("Pending")
	default:
		return styles.DetailValueStyle.Render("Unknown")
	}
}

// renderMethod renders an HTTP method with appropriate color
func (m DetailModel) renderMethod(method string) string {
	method = strings.ToUpper(method)
	padded := fmt.Sprintf("%-7s", method)
	switch method {
	case "GET":
		return styles.HTTPMethodGetStyle.Render(padded)
	case "POST":
		return styles.HTTPMethodPostStyle.Render(padded)
	case "PUT":
		return styles.HTTPMethodPutStyle.Render(padded)
	case "DELETE":
		return styles.HTTPMethodDeleteStyle.Render(padded)
	default:
		return styles.HTTPMethodOtherStyle.Render(padded)
	}
}

// renderStatusCode renders an HTTP status code with appropriate color
func (m DetailModel) renderStatusCode(code int) string {
	codeStr := fmt.Sprintf("%d", code)
	if code >= 200 && code < 300 {
		return styles.HTTPStatus2xxStyle.Render(codeStr)
	} else if code >= 300 && code < 400 {
		return styles.HTTPStatus3xxStyle.Render(codeStr)
	} else if code >= 400 && code < 500 {
		return styles.HTTPStatus4xxStyle.Render(codeStr)
	} else if code >= 500 {
		return styles.HTTPStatus5xxStyle.Render(codeStr)
	}
	return styles.DetailValueStyle.Render(codeStr)
}

// renderFooter renders the footer with keybindings based on current tab
func (m DetailModel) renderFooter() string {
	var parts []string

	parts = append(parts, styles.DetailFooterKeyStyle.Render("[Esc]")+" Back")
	parts = append(parts, styles.DetailFooterKeyStyle.Render("[Tab]")+" Switch")

	switch m.currentTab {
	case TabInfo:
		parts = append(parts, styles.DetailFooterKeyStyle.Render("[1-9]")+" Copy")
	case TabHTTP, TabLogs:
		parts = append(parts, styles.DetailFooterKeyStyle.Render("[j/k]")+" Scroll")
	}

	return styles.DetailFooterStyle.Render(strings.Join(parts, "  "))
}

// formatDuration formats a duration in a human-readable way
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	} else if d < time.Hour {
		return fmt.Sprintf("%dm %ds ago", int(d.Minutes()), int(d.Seconds())%60)
	} else if d < 24*time.Hour {
		return fmt.Sprintf("%dh %dm ago", int(d.Hours()), int(d.Minutes())%60)
	}
	return fmt.Sprintf("%dd %dh ago", int(d.Hours()/24), int(d.Hours())%24)
}
