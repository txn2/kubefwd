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
	"strings"
	"sync"
	"time"

	"github.com/rivo/tview"
	"github.com/sirupsen/logrus"
)

// LogsView displays log messages
type LogsView struct {
	TextView  *tview.TextView
	maxLines  int
	lineCount int
	mu        sync.Mutex
}

// NewLogsView creates a new logs view
func NewLogsView(maxLines int) *LogsView {
	if maxLines <= 0 {
		maxLines = 1000
	}

	tv := tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetWrap(true).
		SetWordWrap(true)

	tv.SetBorder(true).SetTitle(" Logs ")
	tv.SetChangedFunc(func() {
		// Auto-scroll to bottom when content changes
		tv.ScrollToEnd()
	})

	return &LogsView{
		TextView: tv,
		maxLines: maxLines,
	}
}

// AppendLog adds a log entry to the view
func (v *LogsView) AppendLog(level logrus.Level, message string) {
	v.AppendLogWithTime(time.Now(), level, message)
}

// AppendLogWithTime adds a log entry with a specific timestamp
func (v *LogsView) AppendLogWithTime(timestamp time.Time, level logrus.Level, message string) {
	v.mu.Lock()
	defer v.mu.Unlock()

	// Format the log entry with color
	color := levelColor(level)
	levelStr := levelString(level)
	// Trim trailing newlines to avoid double-spacing
	message = strings.TrimRight(message, "\n\r")
	formatted := fmt.Sprintf("[%s][%s]%s[-] %s\n",
		timestamp.Format("15:04:05"),
		color,
		levelStr,
		escapeText(message),
	)

	// Append to text view
	fmt.Fprint(v.TextView, formatted)
	v.lineCount++

	// Trim old lines if exceeding max
	if v.lineCount > v.maxLines {
		v.trimLines()
	}
}

// trimLines removes old lines to keep the buffer under maxLines
func (v *LogsView) trimLines() {
	text := v.TextView.GetText(false)
	lines := strings.Split(text, "\n")

	if len(lines) > v.maxLines {
		// Keep only the last maxLines lines
		start := len(lines) - v.maxLines
		newText := strings.Join(lines[start:], "\n")
		v.TextView.SetText(newText)
		v.lineCount = v.maxLines
	}
}

// Clear clears all log entries
func (v *LogsView) Clear() {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.TextView.Clear()
	v.lineCount = 0
}

// levelColor returns the tview color tag for a log level
func levelColor(level logrus.Level) string {
	switch level {
	case logrus.PanicLevel, logrus.FatalLevel:
		return "red::b" // bold red
	case logrus.ErrorLevel:
		return "red"
	case logrus.WarnLevel:
		return "yellow"
	case logrus.InfoLevel:
		return "green"
	case logrus.DebugLevel:
		return "blue"
	case logrus.TraceLevel:
		return "gray"
	default:
		return "white"
	}
}

// levelString returns a short string representation of the log level
func levelString(level logrus.Level) string {
	switch level {
	case logrus.PanicLevel:
		return "PANIC"
	case logrus.FatalLevel:
		return "FATAL"
	case logrus.ErrorLevel:
		return "ERROR"
	case logrus.WarnLevel:
		return "WARN "
	case logrus.InfoLevel:
		return "INFO "
	case logrus.DebugLevel:
		return "DEBUG"
	case logrus.TraceLevel:
		return "TRACE"
	default:
		return "     "
	}
}

// escapeText escapes special characters for tview
func escapeText(s string) string {
	// Escape brackets that could be interpreted as color tags
	s = strings.ReplaceAll(s, "[", "[[")
	s = strings.ReplaceAll(s, "]", "]]")
	return s
}
