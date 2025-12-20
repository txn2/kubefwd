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
