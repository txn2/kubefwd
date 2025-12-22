package hooks

import (
	"github.com/rivo/tview"
	"github.com/sirupsen/logrus"
	"github.com/txn2/kubefwd/pkg/fwdtui/views"
)

// TUILogHook captures logrus entries and sends them to the TUI
type TUILogHook struct {
	logsView *views.LogsView
	app      *tview.Application
	levels   []logrus.Level
}

// NewTUILogHook creates a new TUI log hook
func NewTUILogHook(logsView *views.LogsView, app *tview.Application) *TUILogHook {
	return &TUILogHook{
		logsView: logsView,
		app:      app,
		levels:   logrus.AllLevels,
	}
}

// Levels returns the log levels this hook handles
func (h *TUILogHook) Levels() []logrus.Level {
	return h.levels
}

// Fire is called when a log entry is made
func (h *TUILogHook) Fire(entry *logrus.Entry) error {
	// Thread-safe update to TextView
	h.app.QueueUpdateDraw(func() {
		h.logsView.AppendLogWithTime(entry.Time, entry.Level, entry.Message)
	})

	return nil
}

// SetLevels sets which log levels this hook should capture
func (h *TUILogHook) SetLevels(levels []logrus.Level) {
	h.levels = levels
}
