package views

import (
	"fmt"

	"github.com/rivo/tview"
	"github.com/txn2/kubefwd/pkg/fwdtui/state"
)

// StatusBar displays summary statistics
type StatusBar struct {
	TextView *tview.TextView
}

// NewStatusBar creates a new status bar
func NewStatusBar() *StatusBar {
	tv := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft)

	// Initial text
	tv.SetText(" [yellow]Services:[-] 0 | [yellow]Active:[-] 0 | [yellow]Errors:[-] 0 | [yellow]Bandwidth:[-] 0 B/s | [dim]Press ? for help[-]")

	return &StatusBar{
		TextView: tv,
	}
}

// Update updates the status bar with new statistics
func (s *StatusBar) Update(stats state.SummaryStats) {
	errorColor := "green"
	if stats.ErrorCount > 0 {
		errorColor = "red"
	}

	text := fmt.Sprintf(" [yellow]Services:[-] %d/%d | [yellow]Forwards:[-] %d/%d | [%s]Errors:[-] %d | [yellow]In:[-] %s | [yellow]Out:[-] %s | [dim]Press ? for help[-]",
		stats.ActiveServices,
		stats.TotalServices,
		stats.ActiveForwards,
		stats.TotalForwards,
		errorColor,
		stats.ErrorCount,
		humanRate(stats.TotalRateIn),
		humanRate(stats.TotalRateOut),
	)

	s.TextView.SetText(text)
}

// SetFilter updates the status bar to show current filter
func (s *StatusBar) SetFilter(filter string, stats state.SummaryStats) {
	errorColor := "green"
	if stats.ErrorCount > 0 {
		errorColor = "red"
	}

	filterText := ""
	if filter != "" {
		filterText = fmt.Sprintf("[blue]Filter:[-] %s | ", filter)
	}

	text := fmt.Sprintf(" %s[yellow]Services:[-] %d/%d | [yellow]Forwards:[-] %d/%d | [%s]Errors:[-] %d | [yellow]In:[-] %s | [yellow]Out:[-] %s | [dim]Press ? for help[-]",
		filterText,
		stats.ActiveServices,
		stats.TotalServices,
		stats.ActiveForwards,
		stats.TotalForwards,
		errorColor,
		stats.ErrorCount,
		humanRate(stats.TotalRateIn),
		humanRate(stats.TotalRateOut),
	)

	s.TextView.SetText(text)
}
