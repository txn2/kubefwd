package components

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sirupsen/logrus"
	"github.com/txn2/kubefwd/pkg/fwdtui/styles"
)

const maxLogLines = 1000

// LogEntry represents a single log entry
type LogEntry struct {
	Time    time.Time
	Level   logrus.Level
	Message string
}

// LogsModel displays scrollable log output
type LogsModel struct {
	viewport   viewport.Model
	logs       []LogEntry
	width      int
	height     int
	focused    bool
	ready      bool
	autoFollow bool // auto-scroll to bottom on new logs
}

// NewLogsModel creates a new logs model
func NewLogsModel() LogsModel {
	return LogsModel{
		logs:       make([]LogEntry, 0, maxLogLines),
		autoFollow: true, // auto-follow by default
	}
}

// Init initializes the logs model
func (m *LogsModel) Init() tea.Cmd {
	return nil
}

// handleWindowSizeMsg handles window resize messages
func (m *LogsModel) handleWindowSizeMsg(msg tea.WindowSizeMsg) {
	m.width = msg.Width
	m.height = msg.Height
	viewportHeight := m.height - 1
	if viewportHeight < 1 {
		viewportHeight = 1
	}
	if !m.ready {
		m.viewport = viewport.New(m.width, viewportHeight)
		m.viewport.Style = lipgloss.NewStyle()
		m.ready = true
	} else {
		m.viewport.Width = m.width
		m.viewport.Height = viewportHeight
	}
	m.updateContent()
}

// handleKeyMsg handles keyboard navigation messages
func (m *LogsModel) handleKeyMsg(msg tea.KeyMsg) {
	if !m.focused || !m.ready {
		return
	}
	switch msg.String() {
	case "j":
		m.viewport.ScrollDown(1)
	case "k":
		m.viewport.ScrollUp(1)
		m.autoFollow = false
	case "g":
		m.viewport.GotoTop()
		m.autoFollow = false
	case "G":
		m.viewport.GotoBottom()
		m.autoFollow = true
	case "up", "pgup", "home":
		m.autoFollow = false
	case "end":
		m.autoFollow = true
	}
}

// handleMouseMsg handles mouse scroll messages
func (m *LogsModel) handleMouseMsg(msg tea.MouseMsg) {
	if m.focused && m.ready && msg.Button == tea.MouseButtonWheelUp {
		m.autoFollow = false
	}
}

// Update handles messages for the logs viewport
func (m *LogsModel) Update(msg tea.Msg) (LogsModel, tea.Cmd) {
	var cmd tea.Cmd
	wasAtBottom := m.ready && m.viewport.AtBottom()

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.handleWindowSizeMsg(msg)
	case tea.KeyMsg:
		m.handleKeyMsg(msg)
	case tea.MouseMsg:
		m.handleMouseMsg(msg)
	}

	if m.ready {
		m.viewport, cmd = m.viewport.Update(msg)
		if !wasAtBottom && m.viewport.AtBottom() {
			m.autoFollow = true
		}
	}
	return *m, cmd
}

// AppendLog adds a log entry
func (m *LogsModel) AppendLog(level logrus.Level, message string, t time.Time) {
	entry := LogEntry{
		Time:    t,
		Level:   level,
		Message: message,
	}

	m.logs = append(m.logs, entry)

	// Trim if exceeding max
	if len(m.logs) > maxLogLines {
		m.logs = m.logs[len(m.logs)-maxLogLines:]
	}

	m.updateContent()

	// Auto-scroll to bottom only if following
	if m.ready && m.autoFollow {
		m.viewport.GotoBottom()
	}
}

// updateContent rebuilds the viewport content from logs
func (m *LogsModel) updateContent() {
	if !m.ready {
		return
	}

	var sb strings.Builder
	for _, entry := range m.logs {
		line := m.formatLogEntry(entry)
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	m.viewport.SetContent(sb.String())
}

// formatLogEntry formats a log entry with colors and wrapping
func (m *LogsModel) formatLogEntry(entry LogEntry) string {
	timestamp := styles.LogTimestampStyle.Render(entry.Time.Format("[15:04:05]"))

	// Trim any trailing newlines from the message
	message := strings.TrimRight(entry.Message, "\n\r")

	var levelStyle lipgloss.Style
	var levelStr string

	switch entry.Level {
	case logrus.PanicLevel:
		levelStyle = styles.LogPanicStyle
		levelStr = "PANIC"
	case logrus.FatalLevel:
		levelStyle = styles.LogFatalStyle
		levelStr = "FATAL"
	case logrus.ErrorLevel:
		levelStyle = styles.LogErrorStyle
		levelStr = "ERROR"
	case logrus.WarnLevel:
		levelStyle = styles.LogWarnStyle
		levelStr = "WARN"
	case logrus.InfoLevel:
		levelStyle = styles.LogInfoStyle
		levelStr = "INFO"
	case logrus.DebugLevel:
		levelStyle = styles.LogDebugStyle
		levelStr = "DEBUG"
	case logrus.TraceLevel:
		levelStyle = styles.LogTraceStyle
		levelStr = "TRACE"
	default:
		levelStyle = styles.LogInfoStyle
		levelStr = "INFO"
	}

	level := levelStyle.Render(fmt.Sprintf("[%s]", levelStr))

	// Calculate prefix width for wrapping: [HH:MM:SS][LEVEL] = ~18 chars
	prefixWidth := 18
	availableWidth := m.width - prefixWidth - 1
	if availableWidth < 20 {
		availableWidth = 20
	}

	// Wrap long messages
	if len(message) > availableWidth {
		message = wrapText(message, availableWidth, prefixWidth)
	}

	return fmt.Sprintf("%s%s %s", timestamp, level, message)
}

// wrapText wraps text to fit within width, indenting continuation lines
func wrapText(text string, width, indent int) string {
	if width <= 0 {
		return text
	}

	var result strings.Builder
	indentStr := strings.Repeat(" ", indent)
	remaining := text
	firstLine := true

	for len(remaining) > 0 {
		if !firstLine {
			result.WriteString("\n")
			result.WriteString(indentStr)
		}

		if len(remaining) <= width {
			result.WriteString(remaining)
			break
		}

		// Find a good break point (space)
		breakPoint := width
		for i := width; i > width/2; i-- {
			if remaining[i] == ' ' {
				breakPoint = i
				break
			}
		}

		result.WriteString(remaining[:breakPoint])
		remaining = strings.TrimLeft(remaining[breakPoint:], " ")
		firstLine = false
	}

	return result.String()
}

// View renders the logs viewport (no border - parent handles that)
func (m *LogsModel) View() string {
	if !m.ready {
		return "Loading..."
	}

	// Status line with following/paused indicator
	var statusLine string
	if m.autoFollow {
		statusLine = styles.StatusActiveStyle.Render("Following")
	} else {
		statusLine = styles.StatusConnectingStyle.Render("Paused") +
			styles.DetailLabelStyle.Render(" (scroll to bottom to resume)")
	}
	statusLine += styles.DetailLabelStyle.Render(fmt.Sprintf(" - %d lines", len(m.logs)))

	return statusLine + "\n" + m.viewport.View()
}

// SetFocus sets the focus state
func (m *LogsModel) SetFocus(focused bool) {
	m.focused = focused
}

// SetSize updates the viewport dimensions
func (m *LogsModel) SetSize(width, height int) {
	m.width = width
	m.height = height

	// Viewport height is 1 less to accommodate status line
	viewportHeight := height - 1
	if viewportHeight < 1 {
		viewportHeight = 1
	}

	if !m.ready {
		// Initialize viewport on first SetSize call
		m.viewport = viewport.New(width, viewportHeight)
		m.viewport.Style = lipgloss.NewStyle()
		m.viewport.MouseWheelEnabled = true
		m.ready = true
	} else {
		m.viewport.Width = width
		m.viewport.Height = viewportHeight
	}
	m.updateContent()
}

// Clear removes all log entries
func (m *LogsModel) Clear() {
	m.logs = make([]LogEntry, 0, maxLogLines)
	m.updateContent()
}
