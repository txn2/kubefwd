package components

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/txn2/kubefwd/pkg/fwdtui/styles"
)

// HelpModel displays keyboard shortcuts
type HelpModel struct {
	visible bool
	width   int
	height  int
}

// NewHelpModel creates a new help model
func NewHelpModel() HelpModel {
	return HelpModel{}
}

// Init initializes the help model
func (m *HelpModel) Init() tea.Cmd {
	return nil
}

// Update handles messages for the help modal
func (m *HelpModel) Update(msg tea.Msg) (HelpModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		if m.visible {
			switch msg.String() {
			case "?", "q", "esc":
				m.visible = false
			}
		}
	}
	return *m, nil
}

// Toggle toggles the help visibility
func (m *HelpModel) Toggle() {
	m.visible = !m.visible
}

// IsVisible returns whether help is visible
func (m *HelpModel) IsVisible() bool {
	return m.visible
}

// Show makes help visible
func (m *HelpModel) Show() {
	m.visible = true
}

// Hide hides help
func (m *HelpModel) Hide() {
	m.visible = false
}

// View renders the help modal
func (m *HelpModel) View() string {
	if !m.visible {
		return ""
	}

	helpItems := []struct {
		key  string
		desc string
	}{
		{"Navigation", ""},
		{"j / ↓", "Move down / scroll"},
		{"k / ↑", "Move up / scroll"},
		{"g / Home", "Go to first"},
		{"G / End", "Go to last"},
		{"PgDn / PgUp", "Page down/up"},
		{"Tab", "Switch focus (services/logs)"},
		{"", ""},
		{"Actions", ""},
		{"Enter", "Open detail view"},
		{"f", "Browse namespaces/services"},
		{"d", "Remove selected forward"},
		{"r", "Reconnect errored services"},
		{"/", "Filter services"},
		{"Esc", "Clear filter / Close view"},
		{"?", "Toggle help"},
		{"q", "Quit"},
		{"", ""},
		{"Browse Modal", ""},
		{"x", "Change context"},
		{"Enter", "Select / Forward"},
		{"Esc", "Go back / Close"},
		{"", ""},
		{"Display", ""},
		{"b", "Toggle bandwidth columns"},
		{"c", "Toggle compact view"},
		{"", ""},
		{"Detail View", ""},
		{"1-9", "Copy connect string to clipboard"},
		{"j/k", "Scroll content"},
		{"h", "Toggle HTTP requests"},
		{"", ""},
		{"Mouse", "Wheel scrolls, Shift+drag selects text"},
	}

	var lines []string
	for _, item := range helpItems {
		if item.key == "" && item.desc == "" {
			lines = append(lines, "")
			continue
		}
		if item.desc == "" {
			// Section header
			lines = append(lines, styles.HelpTitleStyle.Render(item.key))
			continue
		}
		key := styles.HelpKeyStyle.Render(item.key)
		desc := styles.HelpDescStyle.Render(item.desc)
		lines = append(lines, key+desc)
	}

	content := strings.Join(lines, "\n")
	modal := styles.HelpModalStyle.Render(content)

	// Center the modal
	modalWidth := lipgloss.Width(modal)
	modalHeight := lipgloss.Height(modal)

	horizontalPadding := (m.width - modalWidth) / 2
	verticalPadding := (m.height - modalHeight) / 2

	if horizontalPadding < 0 {
		horizontalPadding = 0
	}
	if verticalPadding < 0 {
		verticalPadding = 0
	}

	// Build centered output
	var sb strings.Builder

	// Vertical padding (top)
	for range verticalPadding {
		sb.WriteString("\n")
	}

	// Horizontal padding + modal
	leftPad := lipgloss.NewStyle().Width(horizontalPadding).Render("")
	for _, line := range strings.Split(modal, "\n") {
		sb.WriteString(leftPad + line + "\n")
	}

	return sb.String()
}
