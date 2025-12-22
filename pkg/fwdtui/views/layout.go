package views

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/txn2/kubefwd/pkg/fwdtui/events"
	"github.com/txn2/kubefwd/pkg/fwdtui/state"
)

// Layout manages the TUI layout and component composition
type Layout struct {
	app          *tview.Application
	flex         *tview.Flex
	pages        *tview.Pages
	header       *tview.Box
	servicesView *ServicesView
	logsView     *LogsView
	helpModal    *HelpModal
	statusBar    *StatusBar
	store        *state.Store
	eventBus     *events.Bus
	focusIndex   int // 0 = services, 1 = logs
}

// NewLayout creates a new TUI layout
func NewLayout(app *tview.Application, store *state.Store, eventBus *events.Bus, version string) *Layout {
	l := &Layout{
		app:      app,
		store:    store,
		eventBus: eventBus,
	}

	// Create header with clickable hyperlink using SetDrawFunc for direct tcell access
	l.header = tview.NewBox()
	l.header.SetDrawFunc(func(screen tcell.Screen, x, y, width, height int) (int, int, int, int) {
		// Build header segments
		titleText := "kubefwd"
		versionText := fmt.Sprintf(" v%s | ", version)
		linkText := "github.com/txn2/kubefwd"
		linkURL := "https://github.com/txn2/kubefwd"

		totalWidth := len(titleText) + len(versionText) + len(linkText)
		startX := x + (width-totalWidth)/2

		// Draw title in yellow bold
		titleStyle := tcell.StyleDefault.Foreground(tcell.ColorYellow).Bold(true)
		for _, r := range titleText {
			screen.SetContent(startX, y, r, nil, titleStyle)
			startX++
		}

		// Draw version in white
		whiteStyle := tcell.StyleDefault.Foreground(tcell.ColorWhite)
		for _, r := range versionText {
			screen.SetContent(startX, y, r, nil, whiteStyle)
			startX++
		}

		// Draw link in blue with underline and URL
		linkStyle := tcell.StyleDefault.Foreground(tcell.ColorBlue).Underline(true).Url(linkURL)
		for _, r := range linkText {
			screen.SetContent(startX, y, r, nil, linkStyle)
			startX++
		}

		return x, y, width, height
	})

	// Create views
	l.servicesView = NewServicesView(store, eventBus, app)
	l.logsView = NewLogsView(1000)
	l.helpModal = NewHelpModal()
	l.statusBar = NewStatusBar()

	return l
}

// Build constructs and returns the root primitive
func (l *Layout) Build() tview.Primitive {
	// Main layout:
	//  +------------------------------------------+
	//  |           Header (1 row)                 |
	//  +------------------------------------------+
	//  |           Services Table (70%)           |
	//  +------------------------------------------+
	//  |           Log Panel (30%)                |
	//  +------------------------------------------+
	//  |           Status Bar (1 row)             |
	//  +------------------------------------------+

	l.flex = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(l.header, 1, 0, false).
		AddItem(l.servicesView.Table, 0, 7, true).
		AddItem(l.logsView.TextView, 0, 3, false).
		AddItem(l.statusBar.TextView, 1, 0, false)

	l.pages = tview.NewPages().
		AddPage("main", l.flex, true, true).
		AddPage("help", l.helpModal.Flex, true, false)

	// Set up global input capture
	l.app.SetInputCapture(l.handleInput)

	return l.pages
}

// handleInput processes global keyboard events
func (l *Layout) handleInput(event *tcell.EventKey) *tcell.EventKey {
	// Check if help modal is visible
	if l.pages.HasPage("help") {
		name, _ := l.pages.GetFrontPage()
		if name == "help" {
			if event.Key() == tcell.KeyEscape || event.Rune() == 'q' || event.Rune() == '?' {
				l.HideHelp()
				return nil
			}
			return event
		}
	}

	switch event.Rune() {
	case 'q':
		l.app.Stop()
		return nil
	case '?':
		l.ShowHelp()
		return nil
	}

	if event.Key() == tcell.KeyTab {
		l.ToggleFocus()
		return nil
	}

	return event
}

// ShowHelp displays the help modal
func (l *Layout) ShowHelp() {
	l.pages.ShowPage("help")
	l.app.SetFocus(l.helpModal.Flex)
}

// HideHelp hides the help modal
func (l *Layout) HideHelp() {
	l.pages.HidePage("help")
	l.restoreFocus()
}

// ToggleFocus toggles focus between services table and logs
func (l *Layout) ToggleFocus() {
	l.focusIndex = (l.focusIndex + 1) % 2
	l.restoreFocus()
}

// restoreFocus sets focus based on focusIndex
func (l *Layout) restoreFocus() {
	switch l.focusIndex {
	case 0:
		l.app.SetFocus(l.servicesView.Table)
	case 1:
		l.app.SetFocus(l.logsView.TextView)
	}
}

// GetServicesView returns the services view
func (l *Layout) GetServicesView() *ServicesView {
	return l.servicesView
}

// GetLogsView returns the logs view
func (l *Layout) GetLogsView() *LogsView {
	return l.logsView
}

// GetStatusBar returns the status bar
func (l *Layout) GetStatusBar() *StatusBar {
	return l.statusBar
}

// Refresh refreshes all views
func (l *Layout) Refresh() {
	l.servicesView.Refresh()
	l.statusBar.Update(l.store.GetSummary())
}
