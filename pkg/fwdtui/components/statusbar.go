package components

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/txn2/kubefwd/pkg/fwdtui/state"
	"github.com/txn2/kubefwd/pkg/fwdtui/styles"
)

// StatusBarModel displays summary statistics
type StatusBarModel struct {
	stats state.SummaryStats
	width int
}

// NewStatusBarModel creates a new status bar model
func NewStatusBarModel() StatusBarModel {
	return StatusBarModel{}
}

// Init initializes the status bar model
func (m *StatusBarModel) Init() tea.Cmd {
	return nil
}

// Update handles messages for the status bar
func (m *StatusBarModel) Update(msg tea.Msg) (StatusBarModel, tea.Cmd) {
	if wsm, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = wsm.Width
	}
	return *m, nil
}

// UpdateStats updates the displayed statistics
func (m *StatusBarModel) UpdateStats(stats state.SummaryStats) {
	m.stats = stats
}

// View renders the status bar
func (m *StatusBarModel) View() string {
	s := m.stats

	// Forwards count
	forwards := fmt.Sprintf("Forwards: %d/%d", s.ActiveForwards, s.TotalForwards)

	// Errors (red if > 0)
	var errors string
	if s.ErrorCount > 0 {
		errors = styles.StatusBarErrorStyle.Render(fmt.Sprintf("Errors: %d", s.ErrorCount))
	} else {
		errors = styles.StatusActiveStyle.Render("Errors: 0")
	}

	// Bandwidth
	rateIn := "In: " + humanRate(s.TotalRateIn)
	rateOut := "Out: " + humanRate(s.TotalRateOut)

	// Help hint
	help := styles.StatusBarHelpStyle.Render("Press ? for help")

	// Compose the status bar
	left := fmt.Sprintf(" %s | %s | %s | %s",
		forwards, errors, rateIn, rateOut)

	// Calculate padding to right-align help
	leftWidth := lipgloss.Width(left)
	rightWidth := lipgloss.Width(help)
	padding := m.width - leftWidth - rightWidth - 2 // 2 for margins

	if padding < 1 {
		padding = 1
	}

	spacer := lipgloss.NewStyle().Width(padding).Render("")

	return styles.StatusBarStyle.Render(left + spacer + help + " ")
}

// SetWidth updates the status bar width
func (m *StatusBarModel) SetWidth(width int) {
	m.width = width
}
