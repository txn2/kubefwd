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
	"In",
	"Out",
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

// renderHeaders renders the table header row
func (v *ServicesView) renderHeaders() {
	for i, h := range columnHeaders {
		cell := tview.NewTableCell(h).
			SetTextColor(tcell.ColorYellow).
			SetSelectable(false).
			SetExpansion(1)
		if i == colHostname {
			cell.SetExpansion(2) // Hostname gets more space
		}
		v.Table.SetCell(0, i, cell)
	}
}

// Refresh updates the table with current data
func (v *ServicesView) Refresh() {
	forwards := v.store.GetFiltered()

	// Clear existing data rows (keep header)
	rowCount := v.Table.GetRowCount()
	for row := rowCount - 1; row > 0; row-- {
		v.Table.RemoveRow(row)
	}

	// Render data rows
	for i, fwd := range forwards {
		row := i + 1 // Skip header row

		// Hostname
		v.Table.SetCell(row, colHostname, tview.NewTableCell(fwd.PrimaryHostname()).
			SetExpansion(2))

		// Local Address
		v.Table.SetCell(row, colLocalAddr, tview.NewTableCell(fwd.LocalAddress()).
			SetExpansion(1))

		// Pod
		v.Table.SetCell(row, colPod, tview.NewTableCell(truncate(fwd.PodName, 25)).
			SetExpansion(1))

		// Namespace
		v.Table.SetCell(row, colNamespace, tview.NewTableCell(fwd.Namespace).
			SetExpansion(1))

		// Context
		v.Table.SetCell(row, colContext, tview.NewTableCell(truncate(fwd.Context, 15)).
			SetExpansion(1))

		// Status
		statusCell := tview.NewTableCell(fwd.Status.String()).
			SetExpansion(1)
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
			SetAlign(tview.AlignRight).
			SetExpansion(1))

		// Bytes Out
		v.Table.SetCell(row, colBytesOut, tview.NewTableCell(humanBytes(fwd.BytesOut)).
			SetAlign(tview.AlignRight).
			SetExpansion(1))

		// Rate In
		v.Table.SetCell(row, colRateIn, tview.NewTableCell(humanRate(fwd.RateIn)).
			SetAlign(tview.AlignRight).
			SetExpansion(1))

		// Rate Out
		v.Table.SetCell(row, colRateOut, tview.NewTableCell(humanRate(fwd.RateOut)).
			SetAlign(tview.AlignRight).
			SetExpansion(1))
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

// humanBytes formats bytes to human-readable string
func humanBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// humanRate formats bytes/sec to human-readable string
func humanRate(rate float64) string {
	if rate < 1 {
		return "0 B/s"
	}
	const unit = 1024.0
	if rate < unit {
		return fmt.Sprintf("%.0f B/s", rate)
	}
	div, exp := unit, 0
	for n := rate / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB/s", rate/div, "KMGTPE"[exp])
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
