package fwdtui

import (
	"time"

	"github.com/sirupsen/logrus"
	"github.com/txn2/kubefwd/pkg/fwdmetrics"
	"github.com/txn2/kubefwd/pkg/fwdtui/events"
)

// MetricsUpdateMsg carries metrics snapshots from the metrics registry
type MetricsUpdateMsg struct {
	Snapshots []fwdmetrics.ServiceSnapshot
}

// KubefwdEventMsg wraps kubefwd events for the TUI
type KubefwdEventMsg struct {
	Event events.Event
}

// LogEntryMsg represents a log message to display
type LogEntryMsg struct {
	Level   logrus.Level
	Message string
	Time    time.Time
}

// ShutdownMsg signals the TUI to shut down
type ShutdownMsg struct{}

// RefreshMsg triggers a UI refresh
type RefreshMsg struct{}
