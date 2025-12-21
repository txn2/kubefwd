/*
Copyright 2018-2024 Craig Johnston <cjimti@gmail.com>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package views

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/txn2/kubefwd/pkg/fwdtui/events"
	"github.com/txn2/kubefwd/pkg/fwdtui/state"
)

// Column indices
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

// ServicesView displays the services table
type ServicesView struct {
	Table       *tview.Table
	store       *state.Store
	eventBus    *events.Bus
	app         *tview.Application
	selectedRow int
	filterInput *tview.InputField
	filtering   bool
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

	// Initialize with headers
	v.renderHeaders()

	return v
}

// Column max widths for compact display
const (
	maxHostnameWidth  = 35
	maxPodWidth       = 45 // Pod gets more space (favored)
	maxNamespaceWidth = 10
	maxContextWidth   = 10
)

// renderHeaders renders the table header row
func (v *ServicesView) renderHeaders() {
	for i, h := range columnHeaders {
		cell := tview.NewTableCell(h).
			SetTextColor(tcell.ColorYellow).
			SetSelectable(false)

		// Match expansion with data cells
		switch i {
		case colHostname:
			cell.SetExpansion(1)
		case colPod:
			cell.SetExpansion(2) // Pod gets most space
		}
		v.Table.SetCell(0, i, cell)
	}
}

// Refresh updates the table with current data
func (v *ServicesView) Refresh() {
	forwards := v.store.GetFiltered()

	// Capture current selection before clearing
	currentRow, _ := v.Table.GetSelection()
	if currentRow > 0 {
		v.selectedRow = currentRow
	}

	// Clear existing data rows (keep header)
	rowCount := v.Table.GetRowCount()
	for row := rowCount - 1; row > 0; row-- {
		v.Table.RemoveRow(row)
	}

	// Render data rows
	for i, fwd := range forwards {
		row := i + 1 // Skip header row

		// Hostname - truncated, expands to fill space
		v.Table.SetCell(row, colHostname, tview.NewTableCell(truncate(fwd.PrimaryHostname(), maxHostnameWidth)).
			SetExpansion(1))

		// Local Address - fixed width
		v.Table.SetCell(row, colLocalAddr, tview.NewTableCell(fwd.LocalAddress()))

		// Pod - truncated, gets most expansion (favored)
		v.Table.SetCell(row, colPod, tview.NewTableCell(truncate(fwd.PodName, maxPodWidth)).
			SetExpansion(2))

		// Namespace - fixed width
		v.Table.SetCell(row, colNamespace, tview.NewTableCell(truncate(fwd.Namespace, maxNamespaceWidth)))

		// Context - fixed width
		v.Table.SetCell(row, colContext, tview.NewTableCell(truncate(fwd.Context, maxContextWidth)))

		// Status - fixed width
		statusCell := tview.NewTableCell(fwd.Status.String())
		switch fwd.Status {
		case state.StatusActive:
			statusCell.SetTextColor(tcell.ColorGreen)
		case state.StatusError:
			statusCell.SetTextColor(tcell.ColorRed)
		case state.StatusConnecting:
			statusCell.SetTextColor(tcell.ColorYellow)
		case state.StatusStopping:
			statusCell.SetTextColor(tcell.ColorOrange)
		}
		v.Table.SetCell(row, colStatus, statusCell)

		// Bytes In
		v.Table.SetCell(row, colBytesIn, tview.NewTableCell(humanBytes(fwd.BytesIn)).
			SetAlign(tview.AlignRight))

		// Bytes Out
		v.Table.SetCell(row, colBytesOut, tview.NewTableCell(humanBytes(fwd.BytesOut)).
			SetAlign(tview.AlignRight))

		// Rate In
		v.Table.SetCell(row, colRateIn, tview.NewTableCell(humanRate(fwd.RateIn)).
			SetAlign(tview.AlignRight))

		// Rate Out
		v.Table.SetCell(row, colRateOut, tview.NewTableCell(humanRate(fwd.RateOut)).
			SetAlign(tview.AlignRight))
	}

	// Restore selection if possible
	if v.selectedRow > 0 && v.selectedRow < v.Table.GetRowCount() {
		v.Table.Select(v.selectedRow, 0)
	} else if v.Table.GetRowCount() > 1 {
		v.Table.Select(1, 0)
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

// truncate truncates a string to max length with ellipsis
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}
