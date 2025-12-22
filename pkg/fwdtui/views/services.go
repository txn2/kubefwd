package views

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
	"github.com/rivo/tview"
	"github.com/txn2/kubefwd/pkg/fwdtui/events"
	"github.com/txn2/kubefwd/pkg/fwdtui/state"
)

// Column identifiers
const (
	colHostname = iota
	colLocalAddr
	colPod
	colNamespace
	colContext
	colStatus
	colBytesIn
	colBytesOut
	colRateIn
	colRateOut
	colCount // total number of possible columns
)

var columnHeaders = []string{
	"Hostname",
	"Local Address",
	"Pod",
	"Namespace",
	"Context",
	"Status",
	"Total In",
	"Total Out",
	"Rate In",
	"Rate Out",
}

// Fixed column widths (content-based)
const (
	colWidthLocalAddr = 18 // "127.1.27.12:8080"
	colWidthStatus    = 10 // "Active"
	colWidthNamespace = 15
	colWidthContext   = 15
	colWidthBytesIn   = 10 // "  1.2 KB"
	colWidthBytesOut  = 10
	colWidthRateIn    = 11 // " 1.2 KB/s"
	colWidthRateOut   = 11
)

// ServicesView displays the services table
type ServicesView struct {
	Table         *tview.Table
	store         *state.Store
	eventBus      *events.Bus
	app           *tview.Application
	selectedRow   int
	filterInput   *tview.InputField
	filtering     bool
	visibleCols   []int // which columns are currently visible
	showNS        bool  // show namespace column
	showCtx       bool  // show context column
	hostnameWidth int   // calculated width for hostname column
	podWidth      int   // calculated width for pod column
}

// NewServicesView creates a new services table view
func NewServicesView(store *state.Store, bus *events.Bus, app *tview.Application) *ServicesView {
	v := &ServicesView{
		Table:    tview.NewTable(),
		store:    store,
		eventBus: bus,
		app:      app,
	}

	v.Table.SetBorders(false)
	v.Table.SetSelectable(true, false)
	v.Table.SetFixed(1, 0) // Header row is fixed
	v.Table.SetBorder(true).SetTitle(" Services ")

	// Create filter input
	v.filterInput = tview.NewInputField().
		SetLabel(" Filter: ").
		SetFieldWidth(30).
		SetDoneFunc(func(key tcell.Key) {
			if key == tcell.KeyEnter || key == tcell.KeyEscape {
				v.filtering = false
				v.store.SetFilter(v.filterInput.GetText())
				v.Refresh()
				app.SetFocus(v.Table)
			}
		})

	// Set up keyboard handling
	v.Table.SetInputCapture(v.handleInput)

	// Initialize visible columns (will be updated on first Refresh with data)
	v.visibleCols = []int{colHostname, colLocalAddr, colPod, colStatus, colBytesIn, colBytesOut, colRateIn, colRateOut}

	return v
}

// Refresh updates the table with current data
func (v *ServicesView) Refresh() {
	forwards := v.store.GetFiltered()

	// Capture current selection before clearing
	currentRow, _ := v.Table.GetSelection()
	if currentRow > 0 {
		v.selectedRow = currentRow
	}

	// Determine which optional columns to show
	v.updateVisibleColumns(forwards)

	// Calculate dynamic widths for flexible columns
	v.calculateWidths()

	// Clear table completely and rebuild
	v.Table.Clear()

	// Render headers for visible columns with SetMaxWidth matching data cells
	for displayCol, logicalCol := range v.visibleCols {
		cell := tview.NewTableCell(columnHeaders[logicalCol]).
			SetTextColor(tcell.ColorYellow).
			SetSelectable(false)
		// Apply same MaxWidth as data cells
		switch logicalCol {
		case colHostname:
			cell.SetMaxWidth(v.hostnameWidth).SetExpansion(1)
		case colPod:
			cell.SetMaxWidth(v.podWidth).SetExpansion(1)
		case colLocalAddr:
			cell.SetMaxWidth(colWidthLocalAddr).SetExpansion(0)
		case colNamespace:
			cell.SetMaxWidth(colWidthNamespace).SetExpansion(0)
		case colContext:
			cell.SetMaxWidth(colWidthContext).SetExpansion(0)
		case colStatus:
			cell.SetMaxWidth(colWidthStatus).SetExpansion(0)
		case colBytesIn:
			cell.SetMaxWidth(colWidthBytesIn).SetExpansion(0)
		case colBytesOut:
			cell.SetMaxWidth(colWidthBytesOut).SetExpansion(0)
		case colRateIn:
			cell.SetMaxWidth(colWidthRateIn).SetExpansion(0)
		case colRateOut:
			cell.SetMaxWidth(colWidthRateOut).SetExpansion(0)
		}
		v.Table.SetCell(0, displayCol, cell)
	}

	// Render data rows
	for i, fwd := range forwards {
		row := i + 1 // Skip header row
		for displayCol, logicalCol := range v.visibleCols {
			cell := v.createCell(logicalCol, fwd)
			v.Table.SetCell(row, displayCol, cell)
		}
	}

	// Restore selection if possible
	if v.selectedRow > 0 && v.selectedRow < v.Table.GetRowCount() {
		v.Table.Select(v.selectedRow, 0)
	} else if v.Table.GetRowCount() > 1 {
		v.Table.Select(1, 0)
	}
}

// calculateWidths calculates dynamic widths for flexible columns based on table width
func (v *ServicesView) calculateWidths() {
	_, _, width, _ := v.Table.GetInnerRect()
	if width <= 0 {
		width = 120 // reasonable default before first render
	}

	// Sum fixed column widths
	fixed := colWidthLocalAddr + colWidthStatus + colWidthBytesIn + colWidthBytesOut + colWidthRateIn + colWidthRateOut
	if v.showNS {
		fixed += colWidthNamespace
	}
	if v.showCtx {
		fixed += colWidthContext
	}

	// Account for spacing between columns and borders
	numCols := len(v.visibleCols)
	spacing := numCols + 2 // column gaps + borders

	// Remaining space for flexible columns (Hostname + Pod)
	remaining := width - fixed - spacing
	if remaining < 20 {
		remaining = 20 // minimum
	}

	// Split: Hostname gets 1/3, Pod gets 2/3
	v.hostnameWidth = remaining / 3
	if v.hostnameWidth < 10 {
		v.hostnameWidth = 10
	}
	v.podWidth = remaining - v.hostnameWidth
	if v.podWidth < 15 {
		v.podWidth = 15
	}
}

// updateVisibleColumns determines which columns to show based on data
func (v *ServicesView) updateVisibleColumns(forwards []state.ForwardSnapshot) {
	// Check for unique namespaces and contexts
	namespaces := make(map[string]struct{})
	contexts := make(map[string]struct{})
	for _, fwd := range forwards {
		namespaces[fwd.Namespace] = struct{}{}
		contexts[fwd.Context] = struct{}{}
	}

	v.showNS = len(namespaces) > 1
	v.showCtx = len(contexts) > 1

	// Build visible columns list
	v.visibleCols = []int{colHostname, colLocalAddr, colPod}
	if v.showNS {
		v.visibleCols = append(v.visibleCols, colNamespace)
	}
	if v.showCtx {
		v.visibleCols = append(v.visibleCols, colContext)
	}
	v.visibleCols = append(v.visibleCols, colStatus, colBytesIn, colBytesOut, colRateIn, colRateOut)
}

// createCell creates a table cell for the given column and forward
func (v *ServicesView) createCell(col int, fwd state.ForwardSnapshot) *tview.TableCell {
	switch col {
	case colHostname:
		return tview.NewTableCell(fwd.PrimaryHostname()).
			SetMaxWidth(v.hostnameWidth).
			SetExpansion(1)
	case colLocalAddr:
		return tview.NewTableCell(fwd.LocalAddress()).
			SetMaxWidth(colWidthLocalAddr).
			SetExpansion(0)
	case colPod:
		return tview.NewTableCell(fwd.PodName).
			SetMaxWidth(v.podWidth).
			SetExpansion(1)
	case colNamespace:
		return tview.NewTableCell(fwd.Namespace).
			SetMaxWidth(colWidthNamespace).
			SetExpansion(0)
	case colContext:
		return tview.NewTableCell(fwd.Context).
			SetMaxWidth(colWidthContext).
			SetExpansion(0)
	case colStatus:
		cell := tview.NewTableCell(fwd.Status.String()).
			SetMaxWidth(colWidthStatus).
			SetExpansion(0)
		switch fwd.Status {
		case state.StatusActive:
			cell.SetTextColor(tcell.ColorGreen)
		case state.StatusError:
			cell.SetTextColor(tcell.ColorRed)
		case state.StatusConnecting:
			cell.SetTextColor(tcell.ColorYellow)
		case state.StatusStopping:
			cell.SetTextColor(tcell.ColorOrange)
		}
		return cell
	case colBytesIn:
		return tview.NewTableCell(humanBytes(fwd.BytesIn)).
			SetMaxWidth(colWidthBytesIn).
			SetExpansion(0).
			SetAlign(tview.AlignRight)
	case colBytesOut:
		return tview.NewTableCell(humanBytes(fwd.BytesOut)).
			SetMaxWidth(colWidthBytesOut).
			SetExpansion(0).
			SetAlign(tview.AlignRight)
	case colRateIn:
		return tview.NewTableCell(humanRate(fwd.RateIn)).
			SetMaxWidth(colWidthRateIn).
			SetExpansion(0).
			SetAlign(tview.AlignRight)
	case colRateOut:
		return tview.NewTableCell(humanRate(fwd.RateOut)).
			SetMaxWidth(colWidthRateOut).
			SetExpansion(0).
			SetAlign(tview.AlignRight)
	default:
		return tview.NewTableCell("").SetExpansion(0)
	}
}

// handleInput processes keyboard events for the table
func (v *ServicesView) handleInput(event *tcell.EventKey) *tcell.EventKey {
	row, _ := v.Table.GetSelection()
	maxRow := v.Table.GetRowCount() - 1

	switch event.Rune() {
	case 'j': // vim down
		if row < maxRow {
			v.Table.Select(row+1, 0)
			v.selectedRow = row + 1
		}
		return nil
	case 'k': // vim up
		if row > 1 { // Row 0 is header
			v.Table.Select(row-1, 0)
			v.selectedRow = row - 1
		}
		return nil
	case 'g': // vim top
		if maxRow >= 1 {
			v.Table.Select(1, 0)
			v.selectedRow = 1
		}
		return nil
	case 'G': // vim bottom
		if maxRow >= 1 {
			v.Table.Select(maxRow, 0)
			v.selectedRow = maxRow
		}
		return nil
	case '/': // search/filter
		v.showFilterInput()
		return nil
	}

	switch event.Key() {
	case tcell.KeyDown:
		if row < maxRow {
			v.selectedRow = row + 1
		}
	case tcell.KeyUp:
		if row > 1 {
			v.selectedRow = row - 1
		}
	case tcell.KeyHome:
		if maxRow >= 1 {
			v.Table.Select(1, 0)
			v.selectedRow = 1
		}
		return nil
	case tcell.KeyEnd:
		if maxRow >= 1 {
			v.Table.Select(maxRow, 0)
			v.selectedRow = maxRow
		}
		return nil
	case tcell.KeyEscape:
		// Clear filter
		v.store.SetFilter("")
		v.filterInput.SetText("")
		v.Refresh()
		return nil
	}

	return event
}

// showFilterInput shows the filter input field
func (v *ServicesView) showFilterInput() {
	v.filtering = true
	v.filterInput.SetText(v.store.GetFilter())
	v.app.SetFocus(v.filterInput)
}

// GetFilterInput returns the filter input field for layout integration
func (v *ServicesView) GetFilterInput() *tview.InputField {
	return v.filterInput
}

// humanBytes formats bytes to human-readable string with fixed width
func humanBytes(b uint64) string {
	var s string
	const unit = 1024
	if b < unit {
		s = fmt.Sprintf("%d B", b)
	} else {
		div, exp := uint64(unit), 0
		for n := b / unit; n >= unit; n /= unit {
			div *= unit
			exp++
		}
		s = fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
	}
	// Pad to 9 chars for stable column width
	return fmt.Sprintf("%9s", s)
}

// humanRate formats bytes/sec to human-readable string with fixed width
func humanRate(rate float64) string {
	var s string
	if rate < 1 {
		s = "0 B/s"
	} else {
		const unit = 1024.0
		if rate < unit {
			s = fmt.Sprintf("%.0f B/s", rate)
		} else {
			div, exp := unit, 0
			for n := rate / unit; n >= unit; n /= unit {
				div *= unit
				exp++
			}
			s = fmt.Sprintf("%.1f %cB/s", rate/div, "KMGTPE"[exp])
		}
	}
	// Pad to 10 chars for stable column width
	return fmt.Sprintf("%10s", s)
}

// truncate truncates a string to max width with ellipsis (Unicode-aware)
func truncate(s string, max int) string {
	return runewidth.Truncate(s, max, "…")
}
