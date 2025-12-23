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

	// Content scrolling
	scrollOffset   int
	contentHeight  int
	viewportHeight int

	// HTTP log state
	httpLogs      []HTTPLogEntry
	httpVisible   bool
	httpScrollPos int
	maxHTTPLogs   int

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
		httpVisible:    true,
		maxHTTPLogs:    50,
		sparklineWidth: 30,
		copiedIndex:    -1,
	}
}

// Show opens the detail view for the given forward key
func (m *DetailModel) Show(forwardKey string) {
	m.visible = true
	m.forwardKey = forwardKey
	m.httpScrollPos = 0
	m.scrollOffset = 0
	m.copiedIndex = -1
	m.copiedVisible = false

	// Get initial snapshot
	if m.store != nil {
		m.snapshot = m.store.GetForward(forwardKey)
	}
}

// Hide closes the detail view
func (m *DetailModel) Hide() {
	m.visible = false
	m.forwardKey = ""
	m.snapshot = nil
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
	m.httpScrollPos = 0
}

// SetHTTPLogs sets the HTTP log entries (used when getting logs from metrics)
func (m *DetailModel) SetHTTPLogs(logs []HTTPLogEntry) {
	m.httpLogs = logs
}

// ClearCopiedMsg is sent to clear the "Copied!" feedback
type ClearCopiedMsg struct{}

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

	case tea.KeyMsg:
		key := msg.String()

		// Number keys 1-9 copy specific connect strings
		if len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
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
			m.Hide()
			return m, nil
		case "h":
			m.httpVisible = !m.httpVisible
			return m, nil
		case "y": // Yank/copy first connect string
			connectStrings := m.getConnectStrings()
			if len(connectStrings) > 0 {
				if copyToClipboard(connectStrings[0]) {
					m.copiedIndex = 0
					m.copiedVisible = true
					return m, clearCopiedAfterDelay()
				}
			}
			return m, nil
		case "j", "down":
			// Scroll content down
			maxScroll := m.contentHeight - m.viewportHeight
			if maxScroll < 0 {
				maxScroll = 0
			}
			if m.scrollOffset < maxScroll {
				m.scrollOffset++
			}
			return m, nil
		case "k", "up":
			// Scroll content up
			if m.scrollOffset > 0 {
				m.scrollOffset--
			}
			return m, nil
		case "g", "home":
			m.scrollOffset = 0
			return m, nil
		case "G", "end":
			maxScroll := m.contentHeight - m.viewportHeight
			if maxScroll < 0 {
				maxScroll = 0
			}
			m.scrollOffset = maxScroll
			return m, nil
		case "pgdown", "ctrl+d":
			// Page down
			pageSize := m.viewportHeight / 2
			if pageSize < 1 {
				pageSize = 1
			}
			maxScroll := m.contentHeight - m.viewportHeight
			if maxScroll < 0 {
				maxScroll = 0
			}
			m.scrollOffset += pageSize
			if m.scrollOffset > maxScroll {
				m.scrollOffset = maxScroll
			}
			return m, nil
		case "pgup", "ctrl+u":
			// Page up
			pageSize := m.viewportHeight / 2
			if pageSize < 1 {
				pageSize = 1
			}
			m.scrollOffset -= pageSize
			if m.scrollOffset < 0 {
				m.scrollOffset = 0
			}
			return m, nil
		}
	case tea.WindowSizeMsg:
		m.SetSize(msg.Width, msg.Height)

	case tea.MouseMsg:
		// Handle mouse wheel scrolling
		if msg.Button == tea.MouseButtonWheelUp {
			if m.scrollOffset > 0 {
				m.scrollOffset -= 3
				if m.scrollOffset < 0 {
					m.scrollOffset = 0
				}
			}
			return m, nil
		} else if msg.Button == tea.MouseButtonWheelDown {
			maxScroll := m.contentHeight - m.viewportHeight
			if maxScroll < 0 {
				maxScroll = 0
			}
			m.scrollOffset += 3
			if m.scrollOffset > maxScroll {
				m.scrollOffset = maxScroll
			}
			return m, nil
		}
	}
	return m, nil
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

	var b strings.Builder

	// Title
	title := fmt.Sprintf("Service: %s", m.snapshot.ServiceName)
	if m.snapshot.Namespace != "" {
		title += fmt.Sprintf(" (namespace: %s)", m.snapshot.Namespace)
	}
	b.WriteString(styles.DetailTitleStyle.Render(title))
	b.WriteString("\n\n")

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

	// Local Endpoint
	b.WriteString(styles.DetailSectionStyle.Render("LOCAL ENDPOINT"))
	b.WriteString("\n")
	endpoint := fmt.Sprintf("  %s:%s -> :%s",
		m.snapshot.LocalIP, m.snapshot.LocalPort, m.snapshot.PodPort)
	b.WriteString(styles.DetailValueStyle.Render(endpoint))
	b.WriteString("\n\n")

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
	b.WriteString("\n\n")

	// HTTP Requests (if visible and has entries)
	if m.httpVisible {
		b.WriteString(styles.DetailSectionStyle.Render(fmt.Sprintf("HTTP REQUESTS (%d)", len(m.httpLogs))))
		b.WriteString("\n")
		if len(m.httpLogs) > 0 {
			b.WriteString(m.renderHTTPLogs())
		} else {
			b.WriteString("  ")
			b.WriteString(styles.DetailLabelStyle.Render("No HTTP requests captured"))
			b.WriteString("\n")
		}
	}

	// Build scrollable content
	content := b.String()
	lines := strings.Split(content, "\n")
	m.contentHeight = len(lines)

	// Calculate available space for box content
	// Box adds: border (2) + padding (4) = 6 extra chars
	// Leave margin on each side
	contentWidth := m.width - 12
	if contentWidth < 40 {
		contentWidth = 40
	}

	// Calculate viewport height (content area inside box)
	// Box has: top border (1) + content + footer (2 lines) + bottom border (1)
	m.viewportHeight = m.height - 6
	if m.viewportHeight < 5 {
		m.viewportHeight = 5
	}

	// Clamp scroll offset
	maxScroll := m.contentHeight - m.viewportHeight
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.scrollOffset > maxScroll {
		m.scrollOffset = maxScroll
	}
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}

	// Apply scroll offset to get visible lines
	visibleLines := lines
	if m.scrollOffset > 0 && m.scrollOffset < len(lines) {
		visibleLines = lines[m.scrollOffset:]
	}
	if len(visibleLines) > m.viewportHeight {
		visibleLines = visibleLines[:m.viewportHeight]
	}

	// Build scroll indicator
	scrollInfo := ""
	if m.contentHeight > m.viewportHeight {
		scrollInfo = fmt.Sprintf(" [%d/%d]", m.scrollOffset+1, maxScroll+1)
		if m.scrollOffset > 0 {
			scrollInfo = "↑" + scrollInfo
		}
		if m.scrollOffset < maxScroll {
			scrollInfo = scrollInfo + "↓"
		}
	}

	scrolledContent := strings.Join(visibleLines, "\n")

	// Footer with keybindings and scroll info
	footer := m.renderFooter()
	if scrollInfo != "" {
		footer = styles.DetailLabelStyle.Render(scrollInfo) + "  " + footer
	}

	// Build final content with footer
	boxContent := scrolledContent + "\n\n" + footer

	// Style the box
	boxStyle := styles.DetailBorderStyle.
		Width(contentWidth)

	return lipgloss.Place(
		m.width,
		m.height,
		lipgloss.Center,
		lipgloss.Center,
		boxStyle.Render(boxContent),
	)
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

// renderHTTPLogs renders the HTTP log entries
func (m DetailModel) renderHTTPLogs() string {
	var b strings.Builder

	// Determine how many logs to show (based on available space)
	maxVisible := 10 // Show up to 10 entries
	start := m.httpScrollPos
	end := start + maxVisible
	if end > len(m.httpLogs) {
		end = len(m.httpLogs)
	}

	// Show logs in reverse order (newest first)
	for i := len(m.httpLogs) - 1 - start; i >= len(m.httpLogs)-end && i >= 0; i-- {
		entry := m.httpLogs[i]
		b.WriteString("  ")

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

	return b.String()
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

// renderFooter renders the footer with keybindings
func (m DetailModel) renderFooter() string {
	var parts []string

	parts = append(parts, styles.DetailFooterKeyStyle.Render("[Esc]")+" Back")
	parts = append(parts, styles.DetailFooterKeyStyle.Render("[1-9]")+" Copy")
	parts = append(parts, styles.DetailFooterKeyStyle.Render("[j/k]")+" Scroll")
	parts = append(parts, styles.DetailFooterKeyStyle.Render("[h]")+" HTTP")

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
