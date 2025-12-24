package fwdtui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sirupsen/logrus"
	"github.com/txn2/kubefwd/pkg/fwdmetrics"
	"github.com/txn2/kubefwd/pkg/fwdtui/events"
)

// ListenMetrics creates a command that listens for metrics updates
func ListenMetrics(ch <-chan []fwdmetrics.ServiceSnapshot) tea.Cmd {
	return func() tea.Msg {
		snapshots, ok := <-ch
		if !ok {
			return nil
		}
		return MetricsUpdateMsg{Snapshots: snapshots}
	}
}

// ListenEvents creates a command that listens for kubefwd events
func ListenEvents(eventCh <-chan events.Event) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-eventCh
		if !ok {
			return nil
		}
		return KubefwdEventMsg{Event: event}
	}
}

// ListenLogs creates a command that listens for log entries
func ListenLogs(logCh <-chan LogEntryMsg) tea.Cmd {
	return func() tea.Msg {
		entry, ok := <-logCh
		if !ok {
			return nil
		}
		return entry
	}
}

// ListenShutdown creates a command that listens for shutdown signal
func ListenShutdown(stopCh <-chan struct{}) tea.Cmd {
	return func() tea.Msg {
		<-stopCh
		return ShutdownMsg{}
	}
}

// SendLog creates a log entry message
func SendLog(level logrus.Level, message string) tea.Cmd {
	return func() tea.Msg {
		return LogEntryMsg{
			Level:   level,
			Message: message,
			Time:    time.Now(),
		}
	}
}
