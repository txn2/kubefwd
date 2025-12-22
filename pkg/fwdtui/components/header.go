package components

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/txn2/kubefwd/pkg/fwdtui/styles"
)

// HeaderModel displays the application header with title, version, and link
type HeaderModel struct {
	version string
	width   int
}

// NewHeaderModel creates a new header model
func NewHeaderModel(version string) HeaderModel {
	return HeaderModel{
		version: version,
	}
}

// Init initializes the header model
func (m HeaderModel) Init() tea.Cmd {
	return nil
}

// Update handles messages for the header
func (m HeaderModel) Update(msg tea.Msg) (HeaderModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
	}
	return m, nil
}

// View renders the header
func (m HeaderModel) View() string {
	title := styles.HeaderTitleStyle.Render("kubefwd")
	version := styles.HeaderVersionStyle.Render(fmt.Sprintf(" v%s", m.version))
	link := styles.HeaderLinkStyle.Render("github.com/txn2/kubefwd")

	return fmt.Sprintf(" %s%s | %s", title, version, link)
}

// SetWidth updates the header width
func (m *HeaderModel) SetWidth(width int) {
	m.width = width
}
