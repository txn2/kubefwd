package views

import (
	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
	"github.com/rivo/tview"
)

// TextSegment represents a styled segment of text
type TextSegment struct {
	Text  string
	Style tcell.Style
	Url   string // If set, makes this segment a clickable hyperlink
}

// StyledTextView is a tview primitive that renders styled text with hyperlink support
type StyledTextView struct {
	*tview.Box
	segments []TextSegment
	align    int // 0=left, 1=center, 2=right
}

// NewStyledTextView creates a new styled text view
func NewStyledTextView() *StyledTextView {
	return &StyledTextView{
		Box:      tview.NewBox(),
		segments: make([]TextSegment, 0),
		align:    1, // center by default
	}
}

// SetAlign sets text alignment (0=left, 1=center, 2=right)
func (s *StyledTextView) SetAlign(align int) *StyledTextView {
	s.align = align
	return s
}

// Clear removes all segments
func (s *StyledTextView) Clear() *StyledTextView {
	s.segments = make([]TextSegment, 0)
	return s
}

// AddText adds a plain text segment
func (s *StyledTextView) AddText(text string, style tcell.Style) *StyledTextView {
	s.segments = append(s.segments, TextSegment{
		Text:  text,
		Style: style,
	})
	return s
}

// AddLink adds a clickable hyperlink segment
func (s *StyledTextView) AddLink(text string, url string, style tcell.Style) *StyledTextView {
	s.segments = append(s.segments, TextSegment{
		Text:  text,
		Style: style,
		Url:   url,
	})
	return s
}

// Draw renders the styled text to the screen
func (s *StyledTextView) Draw(screen tcell.Screen) {
	s.Box.DrawForSubclass(screen, s)
	x, y, width, _ := s.GetInnerRect()

	// Calculate total width of all segments
	totalWidth := 0
	for _, seg := range s.segments {
		totalWidth += runewidth.StringWidth(seg.Text)
	}

	// Calculate starting X based on alignment
	startX := x
	switch s.align {
	case 1: // center
		startX = x + (width-totalWidth)/2
	case 2: // right
		startX = x + width - totalWidth
	}

	// Draw each segment
	currentX := startX
	for _, seg := range s.segments {
		style := seg.Style
		if seg.Url != "" {
			style = style.Url(seg.Url)
		}

		for _, r := range seg.Text {
			if currentX >= x+width {
				break
			}
			rw := runewidth.RuneWidth(r)
			screen.SetContent(currentX, y, r, nil, style)
			currentX += rw
		}
	}
}

// Commonly used styles
var (
	StyleDefault    = tcell.StyleDefault
	StyleBold       = tcell.StyleDefault.Bold(true)
	StyleYellowBold = tcell.StyleDefault.Foreground(tcell.ColorYellow).Bold(true)
	StyleWhite      = tcell.StyleDefault.Foreground(tcell.ColorWhite)
	StyleBlue       = tcell.StyleDefault.Foreground(tcell.ColorBlue)
	StyleBlueLink   = tcell.StyleDefault.Foreground(tcell.ColorBlue).Underline(true)
	StyleGreen      = tcell.StyleDefault.Foreground(tcell.ColorGreen)
	StyleRed        = tcell.StyleDefault.Foreground(tcell.ColorRed)
)
