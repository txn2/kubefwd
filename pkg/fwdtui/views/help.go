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
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// HelpModal displays keyboard shortcuts
type HelpModal struct {
	Modal *tview.Modal
	Flex  *tview.Flex // The actual display widget
}

// NewHelpModal creates a new help modal using a styled Frame instead of Modal
func NewHelpModal() *HelpModal {
	// Use a TextView for better formatting control
	textView := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft)

	text := `[yellow::b]kubefwd TUI - Keyboard Shortcuts[-::-]

[green::b]Navigation:[-::-]
  [white]j / Down[-]      Move selection down
  [white]k / Up[-]        Move selection up
  [white]g / Home[-]      Go to first row
  [white]G / End[-]       Go to last row

[green::b]Actions:[-::-]
  [white]/[-]             Filter/search services
  [white]Esc[-]           Clear filter
  [white]Tab[-]           Toggle focus (table/logs)
  [white]?[-]             Show this help
  [white]q[-]             Quit application

[green::b]In Filter Mode:[-::-]
  [white]Enter[-]         Apply filter
  [white]Esc[-]           Cancel filter

[dim]Press Esc or ? to close[-]`

	textView.SetText(text)
	textView.SetBackgroundColor(tcell.ColorDefault)

	// Wrap in a frame for the border and title
	frame := tview.NewFrame(textView).
		SetBorders(1, 1, 1, 1, 2, 2)
	frame.SetBorder(true).
		SetTitle(" Help ").
		SetTitleAlign(tview.AlignCenter).
		SetBackgroundColor(tcell.ColorDefault)

	// Create a centered flex layout
	flex := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(frame, 22, 1, true).
			AddItem(nil, 0, 1, false), 60, 1, true).
		AddItem(nil, 0, 1, false)

	// Use a dummy modal just for the interface, but we'll use the flex
	modal := tview.NewModal()

	return &HelpModal{Modal: modal, Flex: flex}
}
