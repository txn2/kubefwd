package components

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/txn2/kubefwd/pkg/fwdtui/styles"
)

// HeaderModel displays the application header with title, version, and link
type HeaderModel struct {
	version        string
	width          int
	currentContext string
}

// NewHeaderModel creates a new header model
func NewHeaderModel(version string) HeaderModel {
	return HeaderModel{
		version: version,
	}
}

// Init initializes the header model
func (m *HeaderModel) Init() tea.Cmd {
	return nil
}

// Update handles messages for the header
func (m *HeaderModel) Update(msg tea.Msg) (HeaderModel, tea.Cmd) {
	if wsm, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = wsm.Width
	}
	return *m, nil
}

// View renders the header
func (m *HeaderModel) View() string {
	title := styles.HeaderTitleStyle.Render("kubefwd")
	version := styles.HeaderVersionStyle.Render(" v" + m.version)
	link := styles.HeaderLinkStyle.Render("kubefwd.com")

	leftPart := fmt.Sprintf(" %s%s | %s", title, version, link)

	// Add browse hint and context on right side
	browseHint := styles.HeaderHintStyle.Render("[f: browse]")
	rightPart := browseHint
	if m.currentContext != "" {
		rightPart = browseHint + " " + styles.HeaderContextStyle.Render("ctx: "+m.currentContext)
	}

	// Calculate spacing to push context to right
	leftWidth := lipgloss.Width(leftPart)
	rightWidth := lipgloss.Width(rightPart)
	spacing := m.width - leftWidth - rightWidth - 1
	if spacing < 1 {
		spacing = 1
	}

	return leftPart + strings.Repeat(" ", spacing) + rightPart
}

// SetWidth updates the header width
func (m *HeaderModel) SetWidth(width int) {
	m.width = width
}

// SetContext updates the displayed Kubernetes context
func (m *HeaderModel) SetContext(ctx string) {
	m.currentContext = ctx
}

// GetContext returns the current context
func (m *HeaderModel) GetContext() string {
	return m.currentContext
}
