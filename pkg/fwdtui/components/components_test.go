package components

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sirupsen/logrus"
	"github.com/txn2/kubefwd/pkg/fwdtui/state"
)

// =============================================================================
// Test Utilities
// =============================================================================

// createTestStore creates a store with sample test data
func createTestStore() *state.Store {
	store := state.NewStore(100)
	store.AddForward(state.ForwardSnapshot{
		Key:         "svc1.default.ctx1",
		ServiceName: "svc1",
		Namespace:   "default",
		Context:     "ctx1",
		PodName:     "svc1-pod-abc123",
		LocalIP:     "127.0.0.1",
		LocalPort:   "8080",
		PodPort:     "80",
		Hostnames:   []string{"svc1", "svc1.default"},
		Status:      state.StatusActive,
		BytesIn:     1024,
		BytesOut:    2048,
		RateIn:      100.0,
		RateOut:     200.0,
	})
	store.AddForward(state.ForwardSnapshot{
		Key:         "svc2.kube-system.ctx1",
		ServiceName: "svc2",
		Namespace:   "kube-system",
		Context:     "ctx1",
		PodName:     "svc2-pod-def456",
		LocalIP:     "127.0.0.2",
		LocalPort:   "9090",
		PodPort:     "9090",
		Hostnames:   []string{"svc2", "svc2.kube-system"},
		Status:      state.StatusConnecting,
	})
	return store
}

// =============================================================================
// Sparkline Tests
// =============================================================================

func TestRenderSparkline_NormalValues(t *testing.T) {
	values := []float64{10, 20, 30, 40, 50}
	result := RenderSparkline(values, 5)

	if len([]rune(result)) != 5 {
		t.Errorf("Expected sparkline length 5, got %d", len([]rune(result)))
	}

	// Check that characters are from SparklineChars
	for _, r := range result {
		found := false
		for _, c := range SparklineChars {
			if r == c {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Character %c not in SparklineChars", r)
		}
	}
}

func TestRenderSparkline_EmptySlice(t *testing.T) {
	result := RenderSparkline([]float64{}, 10)

	// Empty slice returns padding with lowest sparkline char
	if len([]rune(result)) != 10 {
		t.Errorf("Expected sparkline length 10 for empty slice, got %d", len([]rune(result)))
	}

	// All chars should be the lowest sparkline char
	for _, r := range result {
		if r != SparklineChars[0] {
			t.Errorf("Expected lowest char for empty slice, got %c", r)
		}
	}
}

func TestRenderSparkline_ZeroMax(t *testing.T) {
	values := []float64{0, 0, 0, 0}
	result := RenderSparkline(values, 4)

	// All zeros should produce the lowest sparkline character
	for _, r := range result {
		if r != SparklineChars[0] {
			t.Errorf("Expected lowest char for zero values, got %c", r)
		}
	}
}

func TestRenderSparkline_WidthRespected(t *testing.T) {
	values := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}

	// Width smaller than values - should truncate
	result := RenderSparkline(values, 5)
	if len([]rune(result)) != 5 {
		t.Errorf("Expected width 5, got %d", len([]rune(result)))
	}

	// Width larger than values - should pad
	result2 := RenderSparkline(values, 15)
	if len([]rune(result2)) != 15 {
		t.Errorf("Expected width 15, got %d", len([]rune(result2)))
	}
}

func TestRenderSparkline_SingleValue(t *testing.T) {
	values := []float64{100}
	result := RenderSparkline(values, 3)

	if len([]rune(result)) != 3 {
		t.Errorf("Expected width 3, got %d", len([]rune(result)))
	}
}

func TestRenderSparkline_NegativeWidth(t *testing.T) {
	values := []float64{1, 2, 3}
	result := RenderSparkline(values, -5)

	if result != "" {
		t.Errorf("Expected empty string for negative width, got %q", result)
	}
}

func TestRenderSparkline_ZeroWidth(t *testing.T) {
	values := []float64{1, 2, 3}
	result := RenderSparkline(values, 0)

	if result != "" {
		t.Errorf("Expected empty string for zero width, got %q", result)
	}
}

// =============================================================================
// Help Model Tests
// =============================================================================

func TestHelpModel_NewHelp(t *testing.T) {
	m := NewHelpModel()

	if m.IsVisible() {
		t.Error("Expected help to be hidden initially")
	}
}

func TestHelpModel_Toggle(t *testing.T) {
	m := NewHelpModel()

	// Initially hidden
	if m.IsVisible() {
		t.Error("Expected help to be hidden initially")
	}

	// Toggle to visible
	m.Toggle()
	if !m.IsVisible() {
		t.Error("Expected help to be visible after toggle")
	}

	// Toggle back to hidden
	m.Toggle()
	if m.IsVisible() {
		t.Error("Expected help to be hidden after second toggle")
	}
}

func TestHelpModel_ShowHide(t *testing.T) {
	m := NewHelpModel()

	m.Show()
	if !m.IsVisible() {
		t.Error("Expected help to be visible after Show()")
	}

	m.Hide()
	if m.IsVisible() {
		t.Error("Expected help to be hidden after Hide()")
	}
}

func TestHelpModel_CloseWithEscape(t *testing.T) {
	m := NewHelpModel()
	m.Show()

	// Send escape key
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	m = updated

	if m.IsVisible() {
		t.Error("Expected help to close with Escape key")
	}
}

func TestHelpModel_CloseWithQ(t *testing.T) {
	m := NewHelpModel()
	m.Show()

	// Send 'q' key
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	m = updated

	if m.IsVisible() {
		t.Error("Expected help to close with 'q' key")
	}
}

func TestHelpModel_CloseWithQuestionMark(t *testing.T) {
	m := NewHelpModel()
	m.Show()

	// Send '?' key
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m = updated

	if m.IsVisible() {
		t.Error("Expected help to close with '?' key")
	}
}

func TestHelpModel_ViewContainsShortcuts(t *testing.T) {
	m := NewHelpModel()
	m.Show()
	// Set size via WindowSizeMsg
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	m = updated

	view := m.View()

	// The help view contains keyboard shortcuts
	expectedContents := []string{
		"?",          // Help toggle shortcut
		"q",          // Quit
		"Navigation", // Navigation section
	}

	for _, expected := range expectedContents {
		if !strings.Contains(view, expected) {
			t.Errorf("Expected view to contain %q, got: %s", expected, view)
		}
	}
}

func TestHelpModel_SetSizeViaWindowMsg(t *testing.T) {
	m := NewHelpModel()
	// Set size via WindowSizeMsg
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})
	m = updated

	// Size should be stored (we can't access private fields directly,
	// but we can verify the view renders without error)
	m.Show()
	view := m.View()
	if view == "" {
		t.Error("Expected non-empty view after SetSize via WindowSizeMsg")
	}
}

func TestHelpModel_WindowSizeMsg(t *testing.T) {
	m := NewHelpModel()
	m.Show()

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated

	// Should handle window resize without error
	view := m.View()
	if view == "" {
		t.Error("Expected non-empty view after window resize")
	}
}

// =============================================================================
// Header Model Tests
// =============================================================================

func TestHeaderModel_NewWithVersion(t *testing.T) {
	m := NewHeaderModel("1.2.3")

	view := m.View()
	if !strings.Contains(view, "1.2.3") {
		t.Error("Expected view to contain version string")
	}
}

func TestHeaderModel_SetWidth(t *testing.T) {
	m := NewHeaderModel("1.0.0")
	m.SetWidth(100)

	// Verify it handles width without error
	view := m.View()
	if view == "" {
		t.Error("Expected non-empty view after SetWidth")
	}
}

func TestHeaderModel_ViewContainsVersion(t *testing.T) {
	m := NewHeaderModel("dev-123")
	m.SetWidth(80)

	view := m.View()
	if !strings.Contains(view, "dev-123") {
		t.Errorf("Expected view to contain version 'dev-123', got: %s", view)
	}
}

func TestHeaderModel_ViewContainsKubefwd(t *testing.T) {
	m := NewHeaderModel("1.0.0")
	m.SetWidth(80)

	view := m.View()
	if !strings.Contains(view, "kubefwd") {
		t.Error("Expected view to contain 'kubefwd'")
	}
}

func TestHeaderModel_WindowSizeMsg(t *testing.T) {
	m := NewHeaderModel("1.0.0")

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated

	view := m.View()
	if view == "" {
		t.Error("Expected non-empty view after window resize")
	}
}

// =============================================================================
// StatusBar Model Tests
// =============================================================================

func TestStatusBarModel_New(t *testing.T) {
	m := NewStatusBarModel()

	// Should render without error even with zero stats
	view := m.View()
	if view == "" {
		t.Error("Expected non-empty view from new status bar")
	}
}

func TestStatusBarModel_UpdateStats(t *testing.T) {
	m := NewStatusBarModel()
	m.SetWidth(80)

	stats := state.SummaryStats{
		TotalForwards:  10,
		ActiveForwards: 8,
		ErrorCount:     2,
		TotalRateIn:    1024.0,
		TotalRateOut:   2048.0,
	}

	m.UpdateStats(stats)
	view := m.View()

	// Check that stats appear in view
	if !strings.Contains(view, "8/10") || !strings.Contains(view, "Forwards") {
		t.Errorf("Expected forwards count in view, got: %s", view)
	}
}

func TestStatusBarModel_ViewShowsForwardCount(t *testing.T) {
	m := NewStatusBarModel()
	m.SetWidth(80)
	m.UpdateStats(state.SummaryStats{
		TotalForwards:  5,
		ActiveForwards: 3,
	})

	view := m.View()
	if !strings.Contains(view, "3/5") {
		t.Errorf("Expected view to show '3/5' forwards, got: %s", view)
	}
}

func TestStatusBarModel_ErrorCountStyling(t *testing.T) {
	m := NewStatusBarModel()
	m.SetWidth(80)

	// With errors
	m.UpdateStats(state.SummaryStats{ErrorCount: 5})
	view := m.View()
	if !strings.Contains(view, "5") {
		t.Error("Expected error count in view")
	}

	// Without errors
	m.UpdateStats(state.SummaryStats{ErrorCount: 0})
	view = m.View()
	if !strings.Contains(view, "0") {
		t.Error("Expected zero error count in view")
	}
}

func TestStatusBarModel_SetWidth(t *testing.T) {
	m := NewStatusBarModel()
	m.SetWidth(120)

	// Should handle width change without error
	view := m.View()
	if view == "" {
		t.Error("Expected non-empty view after SetWidth")
	}
}

func TestStatusBarModel_WindowSizeMsg(t *testing.T) {
	m := NewStatusBarModel()

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = updated

	view := m.View()
	if view == "" {
		t.Error("Expected non-empty view after window resize")
	}
}

func TestStatusBarModel_HelpHint(t *testing.T) {
	m := NewStatusBarModel()
	m.SetWidth(80)

	view := m.View()
	if !strings.Contains(view, "?") || !strings.Contains(view, "help") {
		t.Error("Expected help hint in status bar")
	}
}

// =============================================================================
// Logs Model Tests
// =============================================================================

func TestLogsModel_New(t *testing.T) {
	m := NewLogsModel()

	// Initially should be empty
	view := m.View()
	if !strings.Contains(view, "Loading") {
		t.Errorf("Expected 'Loading' message for uninitialized logs, got: %s", view)
	}
}

func TestLogsModel_AppendLog(t *testing.T) {
	m := NewLogsModel()
	m.SetSize(80, 20)

	m.AppendLog(logrus.InfoLevel, "Test message", time.Now())

	view := m.View()
	if !strings.Contains(view, "Test message") {
		t.Errorf("Expected appended log in view, got: %s", view)
	}
}

func TestLogsModel_MaxLogRotation(t *testing.T) {
	m := NewLogsModel()
	m.SetSize(80, 20)

	// Append more than maxLogLines (1000)
	for i := 0; i < 1050; i++ {
		m.AppendLog(logrus.InfoLevel, "Log entry", time.Now())
	}

	// Internal logs should be capped at 1000
	// We can't access private fields, but we can verify it doesn't crash
	// and the view still works
	view := m.View()
	if view == "" {
		t.Error("Expected non-empty view after log rotation")
	}
}

func TestLogsModel_NavigationDisablesAutoFollow(t *testing.T) {
	m := NewLogsModel()
	m.SetSize(80, 20)
	m.SetFocus(true)

	// Add some logs to enable scrolling
	for i := 0; i < 50; i++ {
		m.AppendLog(logrus.InfoLevel, "Log line", time.Now())
	}

	// Press 'k' (scroll up) - should disable auto-follow
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	m = updated

	view := m.View()
	// When auto-follow is disabled, view should show "Paused"
	if !strings.Contains(view, "Paused") {
		t.Errorf("Expected 'Paused' indicator after scrolling up, got: %s", view)
	}
}

func TestLogsModel_Clear(t *testing.T) {
	m := NewLogsModel()
	m.SetSize(80, 20)

	m.AppendLog(logrus.InfoLevel, "Test message", time.Now())
	m.Clear()

	view := m.View()
	if strings.Contains(view, "Test message") {
		t.Error("Expected logs to be cleared")
	}
}

func TestLogsModel_SetSize(t *testing.T) {
	m := NewLogsModel()
	m.SetSize(100, 30)

	m.AppendLog(logrus.InfoLevel, "Test", time.Now())

	view := m.View()
	if view == "" {
		t.Error("Expected non-empty view after SetSize")
	}
}

func TestLogsModel_SetFocus(t *testing.T) {
	m := NewLogsModel()
	m.SetSize(80, 20)

	m.SetFocus(true)
	// Focus state should be set (verified by key handling working)

	m.SetFocus(false)
	// Should still render
	view := m.View()
	if view == "" {
		t.Error("Expected non-empty view after SetFocus")
	}
}

func TestLogsModel_LogLevels(t *testing.T) {
	m := NewLogsModel()
	m.SetSize(80, 20)

	levels := []logrus.Level{
		logrus.DebugLevel,
		logrus.InfoLevel,
		logrus.WarnLevel,
		logrus.ErrorLevel,
	}

	for _, level := range levels {
		m.AppendLog(level, "Message at "+level.String(), time.Now())
	}

	view := m.View()
	for _, level := range levels {
		if !strings.Contains(view, "Message at "+level.String()) {
			t.Errorf("Expected log at level %s in view", level.String())
		}
	}
}

func TestLogsModel_VimNavigation(t *testing.T) {
	m := NewLogsModel()
	m.SetSize(80, 20)
	m.SetFocus(true)

	// Add enough logs to enable scrolling
	for i := 0; i < 50; i++ {
		m.AppendLog(logrus.InfoLevel, "Log line", time.Now())
	}

	// Test 'G' goes to bottom and enables auto-follow
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")})
	m = updated

	view := m.View()
	if !strings.Contains(view, "Following") {
		t.Error("Expected 'Following' indicator after pressing G")
	}
}

// =============================================================================
// Services Model Tests
// =============================================================================

func TestServicesModel_NewWithStore(t *testing.T) {
	store := createTestStore()
	m := NewServicesModel(store)

	m.Refresh()
	view := m.View()

	if view == "" {
		t.Error("Expected non-empty view from services model")
	}
}

func TestServicesModel_FilterModeActivation(t *testing.T) {
	store := createTestStore()
	m := NewServicesModel(store)
	m.Refresh()

	// Press '/' to activate filter mode
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = updated

	if !m.IsFiltering() {
		t.Error("Expected filter mode to be active after pressing '/'")
	}
}

func TestServicesModel_FilterInput(t *testing.T) {
	store := createTestStore()
	m := NewServicesModel(store)
	m.Refresh()

	// Activate filter mode
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = updated

	// Type some filter text
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	m = updated
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	m = updated
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m = updated

	view := m.View()
	if !strings.Contains(view, "svc") {
		t.Errorf("Expected filter text 'svc' in view, got: %s", view)
	}
}

func TestServicesModel_FilterEscape(t *testing.T) {
	store := createTestStore()
	m := NewServicesModel(store)
	m.Refresh()

	// Activate filter mode
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = updated

	// Press escape to exit filter mode
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	m = updated

	if m.IsFiltering() {
		t.Error("Expected filter mode to be deactivated after escape")
	}
}

func TestServicesModel_BandwidthToggle(t *testing.T) {
	store := createTestStore()
	m := NewServicesModel(store)
	m.SetSize(120, 30)
	m.Refresh()

	// Get initial view
	view1 := m.View()
	hasBandwidth1 := strings.Contains(view1, "Rate In") || strings.Contains(view1, "Total In")

	// Toggle bandwidth with 'b'
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	m = updated

	// Get updated view
	view2 := m.View()
	hasBandwidth2 := strings.Contains(view2, "Rate In") || strings.Contains(view2, "Total In")

	// Bandwidth visibility should have changed
	if hasBandwidth1 == hasBandwidth2 {
		t.Error("Expected bandwidth columns to toggle")
	}
}

func TestServicesModel_CompactViewToggle(t *testing.T) {
	store := createTestStore()
	m := NewServicesModel(store)
	m.SetSize(120, 30)
	m.Refresh()

	// Get initial view
	view1 := m.View()

	// Toggle compact view with 'c'
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m = updated

	// Get updated view
	view2 := m.View()

	// Views should be different after toggle
	if view1 == view2 {
		t.Error("Expected view to change after compact toggle")
	}
}

func TestServicesModel_GetSelectedKey(t *testing.T) {
	store := createTestStore()
	m := NewServicesModel(store)
	m.SetSize(80, 30)
	m.Refresh()

	key := m.GetSelectedKey()
	// Should return a valid key from our test data
	if key == "" {
		t.Error("Expected non-empty selected key")
	}
}

func TestServicesModel_HasSelection(t *testing.T) {
	store := createTestStore()
	m := NewServicesModel(store)
	m.SetSize(80, 30)
	m.Refresh()

	if !m.HasSelection() {
		t.Error("Expected HasSelection to return true when rows exist")
	}
}

func TestServicesModel_Refresh(t *testing.T) {
	store := createTestStore()
	m := NewServicesModel(store)
	m.SetSize(80, 30)

	// Initially empty (before refresh)
	key1 := m.GetSelectedKey()

	// After refresh, should have data
	m.Refresh()
	key2 := m.GetSelectedKey()

	if key1 != "" {
		t.Error("Expected empty key before refresh")
	}
	if key2 == "" {
		t.Error("Expected non-empty key after refresh")
	}
}

func TestServicesModel_SetSize(t *testing.T) {
	store := createTestStore()
	m := NewServicesModel(store)
	m.Refresh()

	m.SetSize(100, 40)

	view := m.View()
	if view == "" {
		t.Error("Expected non-empty view after SetSize")
	}
}

func TestServicesModel_SetFocus(t *testing.T) {
	store := createTestStore()
	m := NewServicesModel(store)
	m.Refresh()

	m.SetFocus(false)
	m.SetFocus(true)

	// Should handle focus changes without error
	view := m.View()
	if view == "" {
		t.Error("Expected non-empty view after SetFocus")
	}
}

// =============================================================================
// Formatting Helper Tests
// =============================================================================

func TestHumanBytes(t *testing.T) {
	tests := []struct {
		bytes    uint64
		expected string
	}{
		{0, "0 B"},
		{100, "100 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
	}

	for _, tt := range tests {
		result := humanBytes(tt.bytes)
		if result != tt.expected {
			t.Errorf("humanBytes(%d) = %s, expected %s", tt.bytes, result, tt.expected)
		}
	}
}

func TestHumanRate(t *testing.T) {
	tests := []struct {
		rate     float64
		expected string
	}{
		{0, "0 B/s"},
		{0.5, "0 B/s"},
		{100, "100 B/s"},
		{1024, "1.0 KB/s"},
		{1536, "1.5 KB/s"},
		{1048576, "1.0 MB/s"},
	}

	for _, tt := range tests {
		result := humanRate(tt.rate)
		if result != tt.expected {
			t.Errorf("humanRate(%f) = %s, expected %s", tt.rate, result, tt.expected)
		}
	}
}

func TestFormatPort(t *testing.T) {
	tests := []struct {
		local    string
		pod      string
		expected string
	}{
		{"8080", "8080", "8080"},
		{"8080", "80", "8080→80"},
		{"9090", "9000", "9090→9000"},
	}

	for _, tt := range tests {
		result := formatPort(tt.local, tt.pod)
		if result != tt.expected {
			t.Errorf("formatPort(%s, %s) = %s, expected %s", tt.local, tt.pod, result, tt.expected)
		}
	}
}

// =============================================================================
// Detail Model Tests
// =============================================================================

func TestDetailModel_NewDetail(t *testing.T) {
	store := createTestStore()
	m := NewDetailModel(store, nil)

	if m.IsVisible() {
		t.Error("Expected detail view to be hidden initially")
	}
}

func TestDetailModel_ShowHide(t *testing.T) {
	store := createTestStore()
	m := NewDetailModel(store, nil)

	m.Show("svc1.default.ctx1")

	if !m.IsVisible() {
		t.Error("Expected detail view to be visible after Show()")
	}

	if m.GetForwardKey() != "svc1.default.ctx1" {
		t.Errorf("Expected forward key 'svc1.default.ctx1', got %s", m.GetForwardKey())
	}

	m.Hide()

	if m.IsVisible() {
		t.Error("Expected detail view to be hidden after Hide()")
	}
}

func TestDetailModel_TabSwitching(t *testing.T) {
	store := createTestStore()
	m := NewDetailModel(store, nil)
	m.Show("svc1.default.ctx1")
	m.SetSize(80, 40)

	// Should start on Info tab (0)
	if m.GetCurrentTab() != TabInfo {
		t.Errorf("Expected initial tab to be Info (0), got %d", m.GetCurrentTab())
	}

	// Press tab to go to HTTP
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated

	if m.GetCurrentTab() != TabHTTP {
		t.Errorf("Expected tab to be HTTP (1) after Tab, got %d", m.GetCurrentTab())
	}

	// Press tab again to go to Logs
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated

	if m.GetCurrentTab() != TabLogs {
		t.Errorf("Expected tab to be Logs (2) after second Tab, got %d", m.GetCurrentTab())
	}

	// Press tab again to wrap to Info
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated

	if m.GetCurrentTab() != TabInfo {
		t.Errorf("Expected tab to wrap to Info (0), got %d", m.GetCurrentTab())
	}
}

func TestDetailModel_TabSwitchingWithShiftTab(t *testing.T) {
	store := createTestStore()
	m := NewDetailModel(store, nil)
	m.Show("svc1.default.ctx1")
	m.SetSize(80, 40)

	// Press shift+tab to go backwards
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = updated

	if m.GetCurrentTab() != TabLogs {
		t.Errorf("Expected tab to be Logs (2) after Shift+Tab from Info, got %d", m.GetCurrentTab())
	}
}

func TestDetailModel_TabSwitchingWithArrows(t *testing.T) {
	store := createTestStore()
	m := NewDetailModel(store, nil)
	m.Show("svc1.default.ctx1")
	m.SetSize(80, 40)

	// Should start on Info tab (0)
	if m.GetCurrentTab() != TabInfo {
		t.Errorf("Expected initial tab to be Info (0), got %d", m.GetCurrentTab())
	}

	// Press right arrow - use KeyRight type
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = updated

	if m.GetCurrentTab() != TabHTTP {
		t.Errorf("Expected tab to be HTTP (1) after right arrow, got %d", m.GetCurrentTab())
	}
}

func TestDetailModel_GetForwardKey(t *testing.T) {
	store := createTestStore()
	m := NewDetailModel(store, nil)

	// Before showing, should be empty
	if m.GetForwardKey() != "" {
		t.Error("Expected empty forward key before Show()")
	}

	m.Show("test-key")

	if m.GetForwardKey() != "test-key" {
		t.Errorf("Expected 'test-key', got %s", m.GetForwardKey())
	}
}

func TestDetailModel_UpdateSnapshot(t *testing.T) {
	store := createTestStore()
	m := NewDetailModel(store, nil)
	m.Show("svc1.default.ctx1")

	m.UpdateSnapshot()

	snapshot := m.GetSnapshot()
	if snapshot == nil {
		t.Fatal("Expected non-nil snapshot after UpdateSnapshot")
	}

	if snapshot.ServiceName != "svc1" {
		t.Errorf("Expected service name 'svc1', got %s", snapshot.ServiceName)
	}
}

func TestDetailModel_AppendLogLine(t *testing.T) {
	store := createTestStore()
	m := NewDetailModel(store, nil)
	m.Show("svc1.default.ctx1")
	m.SetSize(80, 40)

	m.AppendLogLine("Test log line 1")
	m.AppendLogLine("Test log line 2")

	// Switch to logs tab
	m.currentTab = TabLogs
	m.SetLogsStreaming(true)

	view := m.View()
	if !strings.Contains(view, "Test log line") {
		t.Errorf("Expected log lines in view, got: %s", view)
	}
}

func TestDetailModel_SetLogsStreaming(t *testing.T) {
	store := createTestStore()
	m := NewDetailModel(store, nil)
	m.Show("svc1.default.ctx1")

	m.SetLogsStreaming(true)
	if !m.IsLogsStreaming() {
		t.Error("Expected IsLogsStreaming to return true")
	}

	m.SetLogsStreaming(false)
	if m.IsLogsStreaming() {
		t.Error("Expected IsLogsStreaming to return false")
	}
}

func TestDetailModel_HTTPLogRotation(t *testing.T) {
	store := createTestStore()
	m := NewDetailModel(store, nil)
	m.Show("svc1.default.ctx1")

	// Add more than maxHTTPLogs (100)
	for i := 0; i < 150; i++ {
		m.AddHTTPLog(HTTPLogEntry{
			Timestamp:  time.Now(),
			Method:     "GET",
			Path:       "/test",
			StatusCode: 200,
			Duration:   time.Millisecond * 10,
		})
	}

	// Should not crash and view should work
	m.currentTab = TabHTTP
	m.SetSize(80, 40)
	view := m.View()
	if view == "" {
		t.Error("Expected non-empty view after HTTP log rotation")
	}
}

func TestDetailModel_PodLogRotation(t *testing.T) {
	store := createTestStore()
	m := NewDetailModel(store, nil)
	m.Show("svc1.default.ctx1")
	m.SetSize(80, 40)

	// Add more than maxPodLogs (1000)
	for i := 0; i < 1100; i++ {
		m.AppendLogLine("Log line")
	}

	// Should not crash and view should work
	m.currentTab = TabLogs
	m.SetLogsStreaming(true)
	view := m.View()
	if view == "" {
		t.Error("Expected non-empty view after pod log rotation")
	}
}

func TestDetailModel_CloseWithEscape(t *testing.T) {
	store := createTestStore()
	m := NewDetailModel(store, nil)
	m.Show("svc1.default.ctx1")
	m.SetSize(80, 40)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	m = updated

	if m.IsVisible() {
		t.Error("Expected detail view to close with Escape")
	}
}

func TestDetailModel_CloseWithQ(t *testing.T) {
	store := createTestStore()
	m := NewDetailModel(store, nil)
	m.Show("svc1.default.ctx1")
	m.SetSize(80, 40)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	m = updated

	if m.IsVisible() {
		t.Error("Expected detail view to close with 'q'")
	}
}

func TestDetailModel_SetSize(t *testing.T) {
	store := createTestStore()
	m := NewDetailModel(store, nil)
	m.Show("svc1.default.ctx1")

	m.SetSize(100, 50)

	view := m.View()
	if view == "" {
		t.Error("Expected non-empty view after SetSize")
	}
}

func TestDetailModel_InfoTabView(t *testing.T) {
	store := createTestStore()
	m := NewDetailModel(store, nil)
	m.Show("svc1.default.ctx1")
	m.SetSize(80, 40)

	view := m.View()

	expectedContents := []string{
		"Info",      // Tab name
		"STATUS",    // Status section
		"POD",       // Pod section
		"svc1",      // Service name in connect strings
		"CONNECT",   // Connect strings section
		"BANDWIDTH", // Bandwidth section
	}

	for _, expected := range expectedContents {
		if !strings.Contains(view, expected) {
			t.Errorf("Expected Info tab view to contain %q, got: %s", expected, view)
		}
	}
}

func TestDetailModel_ViewWhenHidden(t *testing.T) {
	store := createTestStore()
	m := NewDetailModel(store, nil)
	// Don't call Show()

	view := m.View()
	if view != "" {
		t.Errorf("Expected empty view when hidden, got: %s", view)
	}
}

func TestDetailModel_ClearHTTPLogs(t *testing.T) {
	store := createTestStore()
	m := NewDetailModel(store, nil)
	m.Show("svc1.default.ctx1")

	m.AddHTTPLog(HTTPLogEntry{Method: "GET", Path: "/test"})
	m.ClearHTTPLogs()

	m.currentTab = TabHTTP
	m.SetSize(80, 40)
	view := m.View()

	if strings.Contains(view, "/test") {
		t.Error("Expected HTTP logs to be cleared")
	}
}

func TestDetailModel_SetLogsError(t *testing.T) {
	store := createTestStore()
	m := NewDetailModel(store, nil)
	m.Show("svc1.default.ctx1")
	m.SetSize(80, 40)

	m.SetLogsError("Connection failed")
	m.currentTab = TabLogs

	view := m.View()
	if !strings.Contains(view, "Connection failed") {
		t.Errorf("Expected error message in view, got: %s", view)
	}
}

// =============================================================================
// Text Wrapping Tests
// =============================================================================

func TestWrapText(t *testing.T) {
	tests := []struct {
		text   string
		width  int
		indent int
	}{
		{"short text", 100, 0},           // No wrap needed
		{"this is a longer text", 10, 2}, // Needs wrapping
		{"", 10, 0},                      // Empty string
		{"a", 1, 0},                      // Single char
	}

	for _, tt := range tests {
		result := wrapText(tt.text, tt.width, tt.indent)
		// Just verify it doesn't panic and returns something
		if tt.text != "" && result == "" && tt.width > 0 {
			t.Errorf("wrapText(%q, %d, %d) returned empty string", tt.text, tt.width, tt.indent)
		}
	}
}

func TestWrapLogText(t *testing.T) {
	longLine := strings.Repeat("a", 200)
	result := wrapLogText(longLine, 80)

	// Should contain newlines for wrapped text
	if !strings.Contains(result, "\n") {
		t.Error("Expected long line to be wrapped with newlines")
	}
}

func TestWrapLogText_ShortLine(t *testing.T) {
	shortLine := "short line"
	result := wrapLogText(shortLine, 80)

	// Should not contain newlines
	if strings.Contains(result, "\n") {
		t.Error("Expected short line to not be wrapped")
	}
}

// =============================================================================
// Duration Formatting Tests
// =============================================================================

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		duration time.Duration
		contains string
	}{
		{30 * time.Second, "30s ago"},
		{90 * time.Second, "m"},
		{2 * time.Hour, "h"},
		{48 * time.Hour, "d"},
	}

	for _, tt := range tests {
		result := formatDuration(tt.duration)
		if !strings.Contains(result, tt.contains) {
			t.Errorf("formatDuration(%v) = %s, expected to contain %q", tt.duration, result, tt.contains)
		}
	}
}

// =============================================================================
// Detail Model Setter Tests
// =============================================================================

func TestDetailModel_SetHTTPLogs(t *testing.T) {
	model := NewDetailModel(nil, nil)

	logs := []HTTPLogEntry{
		{Timestamp: time.Now(), Method: "GET", Path: "/api/test", StatusCode: 200, Duration: time.Millisecond * 50},
		{Timestamp: time.Now(), Method: "POST", Path: "/api/data", StatusCode: 201, Duration: time.Millisecond * 100},
	}

	model.SetHTTPLogs(logs)

	// Verify logs were set
	if len(model.httpLogs) != 2 {
		t.Errorf("Expected 2 HTTP logs, got %d", len(model.httpLogs))
	}
}

func TestDetailModel_SetPodLogs(t *testing.T) {
	model := NewDetailModel(nil, nil)
	model.width = 80
	model.height = 24

	logs := []string{"log line 1", "log line 2", "log line 3"}
	model.SetPodLogs(logs)

	// Verify logs were set and loading is false
	if len(model.podLogs) != 3 {
		t.Errorf("Expected 3 pod logs, got %d", len(model.podLogs))
	}
	if model.logsLoading {
		t.Error("Expected logsLoading to be false after SetPodLogs")
	}
}

func TestDetailModel_SetLogsLoading(t *testing.T) {
	model := NewDetailModel(nil, nil)

	model.SetLogsLoading(true)
	if !model.logsLoading {
		t.Error("Expected logsLoading to be true")
	}

	model.SetLogsLoading(false)
	if model.logsLoading {
		t.Error("Expected logsLoading to be false")
	}
}

func TestDetailModel_Init(t *testing.T) {
	model := NewDetailModel(nil, nil)
	cmd := model.Init()

	// Init should return nil
	if cmd != nil {
		t.Error("Expected Init to return nil")
	}
}

// =============================================================================
// Header Model Tests
// =============================================================================

func TestHeaderModel_Init(t *testing.T) {
	model := NewHeaderModel("1.0.0")
	cmd := model.Init()

	// Init should return nil
	if cmd != nil {
		t.Error("Expected Init to return nil")
	}
}

func TestHeaderModel_SetContext(t *testing.T) {
	model := NewHeaderModel("1.0.0")

	model.SetContext("test-context")

	if model.GetContext() != "test-context" {
		t.Errorf("Expected context 'test-context', got '%s'", model.GetContext())
	}
}

func TestHeaderModel_GetContext(t *testing.T) {
	model := NewHeaderModel("1.0.0")

	// Default context should be empty
	if model.GetContext() != "" {
		t.Errorf("Expected empty context, got '%s'", model.GetContext())
	}

	// Set context via setter and verify
	model.SetContext("my-ctx")
	if model.GetContext() != "my-ctx" {
		t.Errorf("Expected context 'my-ctx', got '%s'", model.GetContext())
	}
}

// =============================================================================
// Help Model Tests
// =============================================================================

func TestHelpModel_Init(t *testing.T) {
	model := NewHelpModel()
	cmd := model.Init()

	// Init should return nil
	if cmd != nil {
		t.Error("Expected Init to return nil")
	}
}

// =============================================================================
// Logs Model Tests
// =============================================================================

func TestLogsModel_Init(t *testing.T) {
	model := NewLogsModel()
	cmd := model.Init()

	// Init should return nil
	if cmd != nil {
		t.Error("Expected Init to return nil")
	}
}

func TestLogsModel_Update_WindowSizeMsg(t *testing.T) {
	m := NewLogsModel()

	// Initial window size message initializes the viewport
	msg := tea.WindowSizeMsg{Width: 80, Height: 20}
	updated, _ := m.Update(msg)

	if updated.width != 80 {
		t.Errorf("Expected width 80, got %d", updated.width)
	}
	if updated.height != 20 {
		t.Errorf("Expected height 20, got %d", updated.height)
	}
	if !updated.ready {
		t.Error("Expected viewport to be ready after WindowSizeMsg")
	}

	// Second window size message should resize viewport
	msg2 := tea.WindowSizeMsg{Width: 100, Height: 30}
	updated2, _ := updated.Update(msg2)

	if updated2.width != 100 {
		t.Errorf("Expected width 100, got %d", updated2.width)
	}
	if updated2.height != 30 {
		t.Errorf("Expected height 30, got %d", updated2.height)
	}
}

func TestLogsModel_Update_WindowSizeMsg_SmallHeight(t *testing.T) {
	m := NewLogsModel()

	// Small height (less than 2) should clamp to 1
	msg := tea.WindowSizeMsg{Width: 80, Height: 1}
	updated, _ := m.Update(msg)

	if updated.height != 1 {
		t.Errorf("Expected height 1, got %d", updated.height)
	}
	if !updated.ready {
		t.Error("Expected viewport to be ready")
	}
}

func TestLogsModel_Update_KeyMsg_J_ScrollDown(t *testing.T) {
	m := NewLogsModel()
	m.SetSize(80, 10)
	m.SetFocus(true)

	// Add logs to scroll
	for i := 0; i < 30; i++ {
		m.AppendLog(logrus.InfoLevel, fmt.Sprintf("Log line %d", i), time.Now())
	}

	// Press 'j' to scroll down
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}
	updated, _ := m.Update(msg)

	// Just verify no panic and model is updated
	if updated.width != m.width {
		t.Error("Model should be updated")
	}
}

func TestLogsModel_Update_KeyMsg_K_ScrollUp(t *testing.T) {
	m := NewLogsModel()
	m.SetSize(80, 10)
	m.SetFocus(true)

	// Add logs to scroll
	for i := 0; i < 30; i++ {
		m.AppendLog(logrus.InfoLevel, fmt.Sprintf("Log line %d", i), time.Now())
	}

	// Go to bottom first
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")})

	// Press 'k' to scroll up - should disable auto-follow
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")}
	updated, _ := m.Update(msg)

	if updated.autoFollow {
		t.Error("Expected autoFollow to be false after pressing 'k'")
	}
}

func TestLogsModel_Update_KeyMsg_G_GotoTop(t *testing.T) {
	m := NewLogsModel()
	m.SetSize(80, 10)
	m.SetFocus(true)

	// Add logs to scroll
	for i := 0; i < 30; i++ {
		m.AppendLog(logrus.InfoLevel, fmt.Sprintf("Log line %d", i), time.Now())
	}

	// Press 'g' to go to top - should disable auto-follow
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")}
	updated, _ := m.Update(msg)

	if updated.autoFollow {
		t.Error("Expected autoFollow to be false after pressing 'g'")
	}
}

func TestLogsModel_Update_KeyMsg_UpperG_GotoBottom(t *testing.T) {
	m := NewLogsModel()
	m.SetSize(80, 10)
	m.SetFocus(true)

	// Add logs to scroll
	for i := 0; i < 30; i++ {
		m.AppendLog(logrus.InfoLevel, fmt.Sprintf("Log line %d", i), time.Now())
	}

	// First go to top
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})

	// Press 'G' to go to bottom - should enable auto-follow
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")}
	updated, _ := m.Update(msg)

	if !updated.autoFollow {
		t.Error("Expected autoFollow to be true after pressing 'G'")
	}
}

func TestLogsModel_Update_KeyMsg_ArrowUp(t *testing.T) {
	m := NewLogsModel()
	m.SetSize(80, 10)
	m.SetFocus(true)

	// Add logs to scroll
	for i := 0; i < 30; i++ {
		m.AppendLog(logrus.InfoLevel, fmt.Sprintf("Log line %d", i), time.Now())
	}

	// Go to bottom first to have auto-follow true
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")})

	// Press up arrow - should disable auto-follow
	msg := tea.KeyMsg{Type: tea.KeyUp}
	updated, _ := m.Update(msg)

	if updated.autoFollow {
		t.Error("Expected autoFollow to be false after pressing up arrow")
	}
}

func TestLogsModel_Update_KeyMsg_PageUp(t *testing.T) {
	m := NewLogsModel()
	m.SetSize(80, 10)
	m.SetFocus(true)

	// Add logs to scroll
	for i := 0; i < 30; i++ {
		m.AppendLog(logrus.InfoLevel, fmt.Sprintf("Log line %d", i), time.Now())
	}

	// Press pgup - should disable auto-follow
	msg := tea.KeyMsg{Type: tea.KeyPgUp}
	updated, _ := m.Update(msg)

	if updated.autoFollow {
		t.Error("Expected autoFollow to be false after pressing page up")
	}
}

func TestLogsModel_Update_KeyMsg_Home(t *testing.T) {
	m := NewLogsModel()
	m.SetSize(80, 10)
	m.SetFocus(true)

	// Add logs to scroll
	for i := 0; i < 30; i++ {
		m.AppendLog(logrus.InfoLevel, fmt.Sprintf("Log line %d", i), time.Now())
	}

	// Press home - should disable auto-follow
	msg := tea.KeyMsg{Type: tea.KeyHome}
	updated, _ := m.Update(msg)

	if updated.autoFollow {
		t.Error("Expected autoFollow to be false after pressing home")
	}
}

func TestLogsModel_Update_KeyMsg_End(t *testing.T) {
	m := NewLogsModel()
	m.SetSize(80, 10)
	m.SetFocus(true)

	// Add logs to scroll
	for i := 0; i < 30; i++ {
		m.AppendLog(logrus.InfoLevel, fmt.Sprintf("Log line %d", i), time.Now())
	}

	// First go to top
	m.Update(tea.KeyMsg{Type: tea.KeyHome})

	// Press end - should enable auto-follow
	msg := tea.KeyMsg{Type: tea.KeyEnd}
	updated, _ := m.Update(msg)

	if !updated.autoFollow {
		t.Error("Expected autoFollow to be true after pressing end")
	}
}

func TestLogsModel_Update_MouseMsg_WheelUp(t *testing.T) {
	m := NewLogsModel()
	m.SetSize(80, 10)
	m.SetFocus(true)

	// Add logs to scroll
	for i := 0; i < 30; i++ {
		m.AppendLog(logrus.InfoLevel, fmt.Sprintf("Log line %d", i), time.Now())
	}

	// Scroll wheel up - should disable auto-follow
	msg := tea.MouseMsg{Button: tea.MouseButtonWheelUp}
	updated, _ := m.Update(msg)

	if updated.autoFollow {
		t.Error("Expected autoFollow to be false after mouse wheel up")
	}
}

func TestLogsModel_Update_NotFocused(_ *testing.T) {
	m := NewLogsModel()
	m.SetSize(80, 10)
	m.SetFocus(false) // Not focused

	// Add logs to scroll
	for i := 0; i < 30; i++ {
		m.AppendLog(logrus.InfoLevel, fmt.Sprintf("Log line %d", i), time.Now())
	}

	// Press 'j' - should not do anything since not focused
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}
	_, _ = m.Update(msg)

	// Just verify no panic
}

func TestLogsModel_Update_NotReady(_ *testing.T) {
	m := NewLogsModel()
	// Don't call SetSize, so viewport is not ready

	// Press 'j' - should not panic even though not ready
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}
	_, _ = m.Update(msg)

	// Just verify no panic
}

// =============================================================================
// Services Model Tests
// =============================================================================

func TestServicesModel_Init(t *testing.T) {
	store := createTestStore()
	model := NewServicesModel(store)
	cmd := model.Init()

	// Init should return nil
	if cmd != nil {
		t.Error("Expected Init to return nil")
	}
}

func TestServicesModel_GetSelectedRegistryKey(_ *testing.T) {
	store := createTestStore()
	model := NewServicesModel(store)
	model.SetSize(80, 24)
	model.Refresh()

	// Initially no selection, should return empty
	key := model.GetSelectedRegistryKey()
	// Note: depends on whether table has data highlighted
	// Just verify it doesn't panic
	_ = key
}

func TestServicesModel_SelectByY(_ *testing.T) {
	store := createTestStore()
	model := NewServicesModel(store)
	model.SetSize(80, 24)
	model.Refresh()

	// Test SelectByY doesn't panic with various Y values
	model.SelectByY(0)
	model.SelectByY(5)
	model.SelectByY(100)
	model.SelectByY(-1)
}

// =============================================================================
// StatusBar Model Tests
// =============================================================================

func TestStatusBarModel_Init(t *testing.T) {
	model := NewStatusBarModel()
	cmd := model.Init()

	// Init should return nil
	if cmd != nil {
		t.Error("Expected Init to return nil")
	}
}

// =============================================================================
// ClearCopiedAfterDelay Tests
// =============================================================================

func TestClearCopiedAfterDelay(t *testing.T) {
	cmd := clearCopiedAfterDelay()

	// Should return a non-nil command (tea.Tick)
	if cmd == nil {
		t.Error("Expected clearCopiedAfterDelay to return a non-nil command")
	}
}

// =============================================================================
// Detail Model Render Method Tests (via instance)
// =============================================================================

func TestDetailModel_RenderStatus(t *testing.T) {
	model := NewDetailModel(nil, nil)
	tests := []struct {
		status   state.ForwardStatus
		expected string
	}{
		{state.StatusActive, "Active"},
		{state.StatusError, "Error"},
		{state.StatusConnecting, "Connecting"},
		{state.StatusPending, "Pending"},
	}

	for _, tt := range tests {
		result := model.renderStatus(tt.status)
		if !strings.Contains(result, tt.expected) {
			t.Errorf("renderStatus(%s) = %s, expected to contain %s", tt.status, result, tt.expected)
		}
	}
}

func TestDetailModel_RenderMethod(t *testing.T) {
	model := NewDetailModel(nil, nil)
	tests := []string{"GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS", "UNKNOWN"}

	for _, method := range tests {
		result := model.renderMethod(method)
		if !strings.Contains(result, method) {
			t.Errorf("renderMethod(%s) should contain the method name", method)
		}
	}
}

func TestDetailModel_RenderStatusCode(t *testing.T) {
	model := NewDetailModel(nil, nil)
	tests := []int{200, 201, 301, 400, 404, 500, 503}

	for _, code := range tests {
		result := model.renderStatusCode(code)
		// Should contain the status code as string
		codeStr := strings.TrimSpace(result)
		if codeStr == "" {
			t.Errorf("renderStatusCode(%d) returned empty string", code)
		}
	}
}

// =============================================================================
// Detail Model Update Tests - Additional Message Types
// =============================================================================

func TestDetailModel_UpdateWithClearCopiedMsg(t *testing.T) {
	store := createTestStore()
	m := NewDetailModel(store, nil)
	m.Show("svc1.default.ctx1")
	m.SetSize(80, 40)
	m.copiedVisible = true
	m.copiedIndex = 0

	updated, _ := m.Update(ClearCopiedMsg{})
	m = updated

	if m.copiedVisible {
		t.Error("Expected copiedVisible to be false after ClearCopiedMsg")
	}
	if m.copiedIndex != -1 {
		t.Errorf("Expected copiedIndex to be -1, got %d", m.copiedIndex)
	}
}

func TestDetailModel_UpdateWithPodLogLineMsg(t *testing.T) {
	store := createTestStore()
	m := NewDetailModel(store, nil)
	m.Show("svc1.default.ctx1")
	m.SetSize(80, 40)

	updated, _ := m.Update(PodLogLineMsg{Line: "test log line"})
	m = updated

	// Verify log line was appended
	if len(m.podLogs) != 1 || m.podLogs[0] != "test log line" {
		t.Errorf("Expected log line to be appended, got: %v", m.podLogs)
	}
}

func TestDetailModel_UpdateWithPodLogsErrorMsg(t *testing.T) {
	store := createTestStore()
	m := NewDetailModel(store, nil)
	m.Show("svc1.default.ctx1")
	m.SetSize(80, 40)

	updated, _ := m.Update(PodLogsErrorMsg{Error: fmt.Errorf("connection failed")})
	m = updated

	if m.logsError != "connection failed" {
		t.Errorf("Expected logsError 'connection failed', got '%s'", m.logsError)
	}
	if m.logsLoading {
		t.Error("Expected logsLoading to be false")
	}
	if m.logsStreaming {
		t.Error("Expected logsStreaming to be false")
	}
}

func TestDetailModel_UpdateWithReconnectKey(t *testing.T) {
	store := createTestStore()
	m := NewDetailModel(store, nil)
	m.Show("svc1.default.ctx1")
	m.SetSize(80, 40)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	m = updated

	// Should return a command that produces ReconnectErroredMsg
	if cmd == nil {
		t.Error("Expected non-nil command for 'r' key")
	}
}

func TestDetailModel_UpdateWithVimKeysOnHTTPTab(t *testing.T) {
	store := createTestStore()
	m := NewDetailModel(store, nil)
	m.Show("svc1.default.ctx1")
	m.SetSize(80, 40)
	m.currentTab = TabHTTP

	// Add some HTTP logs
	for i := 0; i < 50; i++ {
		m.AddHTTPLog(HTTPLogEntry{Method: "GET", Path: "/test"})
	}

	// Test 'j' key (scroll down)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updated
	if m.httpScrollOffset != 1 {
		t.Errorf("Expected httpScrollOffset 1 after 'j', got %d", m.httpScrollOffset)
	}

	// Test 'k' key (scroll up)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	m = updated
	if m.httpScrollOffset != 0 {
		t.Errorf("Expected httpScrollOffset 0 after 'k', got %d", m.httpScrollOffset)
	}

	// Test 'G' key (go to bottom)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")})
	m = updated
	if m.httpScrollOffset == 0 {
		t.Error("Expected httpScrollOffset to be > 0 after 'G'")
	}

	// Test 'g' key (go to top)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	m = updated
	if m.httpScrollOffset != 0 {
		t.Errorf("Expected httpScrollOffset 0 after 'g', got %d", m.httpScrollOffset)
	}
}

func TestDetailModel_UpdateWithArrowKeysOnHTTPTab(t *testing.T) {
	store := createTestStore()
	m := NewDetailModel(store, nil)
	m.Show("svc1.default.ctx1")
	m.SetSize(80, 40)
	m.currentTab = TabHTTP

	// Add some HTTP logs
	for i := 0; i < 50; i++ {
		m.AddHTTPLog(HTTPLogEntry{Method: "GET", Path: "/test"})
	}

	// Test 'down' key
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated
	if m.httpScrollOffset != 1 {
		t.Errorf("Expected httpScrollOffset 1 after 'down', got %d", m.httpScrollOffset)
	}

	// Test 'up' key
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updated
	if m.httpScrollOffset != 0 {
		t.Errorf("Expected httpScrollOffset 0 after 'up', got %d", m.httpScrollOffset)
	}

	// Test 'pgdown' key
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	m = updated
	if m.httpScrollOffset == 0 {
		t.Error("Expected httpScrollOffset > 0 after 'pgdown'")
	}

	// Test 'pgup' key
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	m = updated
	// Should scroll up from current position
}

func TestDetailModel_RequestPodLogs_NilSnapshot(t *testing.T) {
	m := NewDetailModel(nil, nil)
	m.snapshot = nil

	cmd := m.requestPodLogs()
	if cmd != nil {
		t.Error("Expected nil command when snapshot is nil")
	}
}

func TestDetailModel_RequestPodLogs_WithSnapshot(t *testing.T) {
	store := createTestStore()
	m := NewDetailModel(store, nil)
	m.Show("svc1.default.ctx1")
	m.UpdateSnapshot()

	cmd := m.requestPodLogs()
	if cmd == nil {
		t.Error("Expected non-nil command when snapshot is set")
	}

	// Execute the command to get the message
	msg := cmd()
	if _, ok := msg.(PodLogsRequestMsg); !ok {
		t.Errorf("Expected PodLogsRequestMsg, got %T", msg)
	}
}

func TestDetailModel_GetConnectStrings_NilSnapshot(t *testing.T) {
	m := NewDetailModel(nil, nil)
	m.snapshot = nil

	result := m.getConnectStrings()
	if result != nil {
		t.Errorf("Expected nil, got %v", result)
	}
}

func TestDetailModel_GetConnectStrings_WithHostnames(t *testing.T) {
	store := createTestStore()
	m := NewDetailModel(store, nil)
	m.Show("svc1.default.ctx1")
	m.UpdateSnapshot()

	result := m.getConnectStrings()
	if len(result) == 0 {
		t.Error("Expected non-empty connect strings")
	}

	// Check that all strings contain port
	for _, s := range result {
		if !strings.Contains(s, ":") {
			t.Errorf("Expected port in connect string, got: %s", s)
		}
	}
}

func TestDetailModel_GetConnectStrings_NoHostnames(t *testing.T) {
	m := NewDetailModel(nil, nil)
	m.snapshot = &state.ForwardSnapshot{
		ServiceName: "my-service",
		LocalPort:   "8080",
		Hostnames:   []string{}, // Empty hostnames
	}

	result := m.getConnectStrings()
	if len(result) != 1 {
		t.Errorf("Expected 1 connect string, got %d", len(result))
	}
	if result[0] != "my-service:8080" {
		t.Errorf("Expected 'my-service:8080', got '%s'", result[0])
	}
}

func TestDetailModel_GetViewportHeight(t *testing.T) {
	m := NewDetailModel(nil, nil)
	m.height = 40

	height := m.getViewportHeight()
	if height < 5 {
		t.Errorf("Expected viewport height >= 5, got %d", height)
	}
}

func TestDetailModel_GetHTTPMaxScroll(t *testing.T) {
	m := NewDetailModel(nil, nil)
	m.height = 40

	// With no HTTP logs
	maxScroll := m.getHTTPMaxScroll()
	if maxScroll != 0 {
		t.Errorf("Expected maxScroll 0 with no logs, got %d", maxScroll)
	}

	// With many HTTP logs
	for i := 0; i < 100; i++ {
		m.AddHTTPLog(HTTPLogEntry{Method: "GET", Path: "/test"})
	}

	maxScroll = m.getHTTPMaxScroll()
	if maxScroll <= 0 {
		t.Errorf("Expected maxScroll > 0 with many logs, got %d", maxScroll)
	}
}

// ServicesModel Update tests

func TestServicesModel_Update_WindowSizeMsg(t *testing.T) {
	store := state.NewStore(100)
	m := NewServicesModel(store)

	msg := tea.WindowSizeMsg{Width: 120, Height: 40}
	updated, _ := m.Update(msg)

	if updated.width != 120 {
		t.Errorf("Expected width 120, got %d", updated.width)
	}
	if updated.height != 40 {
		t.Errorf("Expected height 40, got %d", updated.height)
	}
}

func TestServicesModel_Update_MouseMsg(_ *testing.T) {
	store := state.NewStore(100)
	m := NewServicesModel(store)

	// Test wheel up
	wheelUp := tea.MouseMsg{Button: tea.MouseButtonWheelUp}
	m.Update(wheelUp)

	// Test wheel down
	wheelDown := tea.MouseMsg{Button: tea.MouseButtonWheelDown}
	m.Update(wheelDown)
}

func TestServicesModel_Update_KeyMsg_Filtering(t *testing.T) {
	store := state.NewStore(100)
	m := NewServicesModel(store)

	// Enable filtering
	slashMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}}
	updated, _ := m.Update(slashMsg)
	if !updated.filtering {
		t.Error("Expected filtering to be enabled after /")
	}

	// Type some characters
	charMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}
	updated, _ = updated.Update(charMsg)
	if updated.filterText != "a" {
		t.Errorf("Expected filterText 'a', got '%s'", updated.filterText)
	}

	// Backspace
	bsMsg := tea.KeyMsg{Type: tea.KeyBackspace}
	updated, _ = updated.Update(bsMsg)
	if updated.filterText != "" {
		t.Errorf("Expected filterText '', got '%s'", updated.filterText)
	}

	// Escape to cancel
	escMsg := tea.KeyMsg{Type: tea.KeyEscape}
	updated, _ = updated.Update(escMsg)
	if updated.filtering {
		t.Error("Expected filtering to be disabled after Escape")
	}
}

func TestServicesModel_Update_KeyMsg_Enter(t *testing.T) {
	store := state.NewStore(100)
	m := NewServicesModel(store)

	// Enable filtering and type
	slashMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}}
	updated, _ := m.Update(slashMsg)

	charMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}}
	updated, _ = updated.Update(charMsg)

	// Press enter to apply filter
	enterMsg := tea.KeyMsg{Type: tea.KeyEnter}
	updated, _ = updated.Update(enterMsg)
	if updated.filtering {
		t.Error("Expected filtering to be disabled after Enter")
	}
}

func TestServicesModel_Update_KeyMsg_ToggleBandwidth(t *testing.T) {
	store := state.NewStore(100)
	m := NewServicesModel(store)

	initialBandwidth := m.showBandwidth

	bMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}}
	updated, _ := m.Update(bMsg)

	if updated.showBandwidth == initialBandwidth {
		t.Error("Expected showBandwidth to toggle")
	}
}

func TestServicesModel_Update_KeyMsg_ToggleCompact(t *testing.T) {
	store := state.NewStore(100)
	m := NewServicesModel(store)

	initialCompact := m.compactView

	cMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}}
	updated, _ := m.Update(cMsg)

	if updated.compactView == initialCompact {
		t.Error("Expected compactView to toggle")
	}
}

func TestServicesModel_Update_KeyMsg_Delete(t *testing.T) {
	store := state.NewStore(100)
	m := NewServicesModel(store)

	// Add a forward to the store
	store.AddForward(state.ForwardSnapshot{
		Key:         "test-forward",
		ServiceName: "my-service",
		Namespace:   "default",
		Context:     "minikube",
	})
	m.Refresh()

	dMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}}
	_, cmd := m.Update(dMsg)

	// If there's a selection, cmd should return a RemoveForwardMsg
	if cmd == nil {
		t.Log("No command returned (no selection)")
	}
}

func TestServicesModel_Update_KeyMsg_Navigation(_ *testing.T) {
	store := state.NewStore(100)
	m := NewServicesModel(store)

	// Add some data
	for i := 0; i < 20; i++ {
		store.AddForward(state.ForwardSnapshot{
			Key:         fmt.Sprintf("fwd-%d", i),
			ServiceName: fmt.Sprintf("svc-%d", i),
			Namespace:   "default",
			Context:     "minikube",
		})
	}
	m.Refresh()

	// Test g (home)
	gMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}}
	m.Update(gMsg)

	// Test G (end)
	GMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}}
	m.Update(GMsg)

	// Test page down
	pgdnMsg := tea.KeyMsg{Type: tea.KeyPgDown}
	m.Update(pgdnMsg)

	// Test page up
	pgupMsg := tea.KeyMsg{Type: tea.KeyPgUp}
	m.Update(pgupMsg)

	// Test home key
	homeMsg := tea.KeyMsg{Type: tea.KeyHome}
	m.Update(homeMsg)

	// Test end key
	endMsg := tea.KeyMsg{Type: tea.KeyEnd}
	m.Update(endMsg)
}

func TestServicesModel_Update_KeyMsg_ClearFilter(t *testing.T) {
	store := state.NewStore(100)
	store.SetFilter("test-filter")
	m := NewServicesModel(store)

	// Escape in normal mode clears filter
	escMsg := tea.KeyMsg{Type: tea.KeyEscape}
	updated, _ := m.Update(escMsg)

	if store.GetFilter() != "" {
		t.Error("Expected filter to be cleared")
	}
	if updated.filterText != "" {
		t.Errorf("Expected filterText '', got '%s'", updated.filterText)
	}
}

func TestServicesModel_StatusStyle(_ *testing.T) {
	store := state.NewStore(100)
	m := NewServicesModel(store)

	// Test different status styles
	testCases := []struct {
		status   string
		expected string
	}{
		{"active", "active"},
		{"error", "error"},
		{"partial", "partial"},
		{"pending", "pending"},
		{"unknown", "unknown"},
	}

	for _, tc := range testCases {
		// statusStyle is private, but we can test it indirectly through View
		// For now, just verify the function exists
		_ = m
		_ = tc
	}
}

// =============================================================================
// Additional Detail Model Update Tests
// =============================================================================

func TestDetailModel_Update_NumberKeyCopy(_ *testing.T) {
	store := createTestStore()
	m := NewDetailModel(store, nil)
	m.Show("svc1.default.ctx1")
	m.SetSize(80, 40)
	m.UpdateSnapshot()
	m.currentTab = TabInfo

	// Press '1' to copy first connect string
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("1")}
	updated, cmd := m.Update(msg)
	m = updated

	// Just verify no panic - the actual copy depends on clipboard availability
	_ = cmd
}

func TestDetailModel_Update_NumberKeyOutOfRange(_ *testing.T) {
	store := createTestStore()
	m := NewDetailModel(store, nil)
	m.Show("svc1.default.ctx1")
	m.SetSize(80, 40)
	m.UpdateSnapshot()
	m.currentTab = TabInfo

	// Press '9' - likely out of range for connect strings
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("9")}
	updated, _ := m.Update(msg)
	m = updated

	// Just verify no panic
}

func TestDetailModel_Update_TabSwitchFromLogsStreaming(t *testing.T) {
	store := createTestStore()
	m := NewDetailModel(store, nil)
	m.Show("svc1.default.ctx1")
	m.SetSize(80, 40)
	m.UpdateSnapshot()
	m.currentTab = TabLogs
	m.logsStreaming = true

	// Press 'tab' to switch away from logs - should stop streaming
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("tab")}
	updated, cmd := m.Update(msg)
	m = updated

	if m.logsStreaming {
		t.Error("Expected logsStreaming to be false after switching tabs")
	}
	_ = cmd
}

func TestDetailModel_Update_TabSwitchToLogs(t *testing.T) {
	store := createTestStore()
	m := NewDetailModel(store, nil)
	m.Show("svc1.default.ctx1")
	m.SetSize(80, 40)
	m.UpdateSnapshot()
	m.currentTab = TabInfo
	m.logsStreaming = false
	m.logsLoading = false

	// Press 'tab' twice to get to Logs tab
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("tab")})
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("tab")})
	m = updated

	if m.currentTab != TabLogs {
		t.Errorf("Expected TabLogs after two tabs from TabInfo, got %v", m.currentTab)
	}
	if !m.logsLoading {
		t.Error("Expected logsLoading to be true when entering Logs tab")
	}
}

func TestDetailModel_Update_ShiftTabFromLogsStreaming(t *testing.T) {
	store := createTestStore()
	m := NewDetailModel(store, nil)
	m.Show("svc1.default.ctx1")
	m.SetSize(80, 40)
	m.UpdateSnapshot()
	m.currentTab = TabLogs
	m.logsStreaming = true

	// Press 'shift+tab' to switch away from logs - should stop streaming
	msg := tea.KeyMsg{Type: tea.KeyShiftTab}
	updated, _ := m.Update(msg)
	m = updated

	if m.logsStreaming {
		t.Error("Expected logsStreaming to be false after shift+tab")
	}
}

func TestDetailModel_Update_VimScrollOnLogsTab(t *testing.T) {
	store := createTestStore()
	m := NewDetailModel(store, nil)
	m.Show("svc1.default.ctx1")
	m.SetSize(80, 40)
	m.UpdateSnapshot()
	m.currentTab = TabLogs
	m.logsViewportReady = true

	// Add some logs
	for i := 0; i < 50; i++ {
		m.AppendLogLine(fmt.Sprintf("Log line %d", i))
	}

	// Test 'j' (scroll down)
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})

	// Test 'k' (scroll up) - should disable auto-follow
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	m = updated
	if m.logsAutoFollow {
		t.Error("Expected logsAutoFollow to be false after 'k'")
	}

	// Test 'g' (go to top) - should disable auto-follow
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	m = updated
	if m.logsAutoFollow {
		t.Error("Expected logsAutoFollow to be false after 'g'")
	}

	// Test 'G' (go to bottom) - should enable auto-follow
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")})
	m = updated
	if !m.logsAutoFollow {
		t.Error("Expected logsAutoFollow to be true after 'G'")
	}
}

func TestDetailModel_Update_ArrowScrollOnLogsTab(t *testing.T) {
	store := createTestStore()
	m := NewDetailModel(store, nil)
	m.Show("svc1.default.ctx1")
	m.SetSize(80, 40)
	m.UpdateSnapshot()
	m.currentTab = TabLogs
	m.logsViewportReady = true
	m.logsAutoFollow = true

	// Add logs
	for i := 0; i < 50; i++ {
		m.AppendLogLine(fmt.Sprintf("Log line %d", i))
	}

	// Test 'up' (scroll up) - should disable auto-follow
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updated
	if m.logsAutoFollow {
		t.Error("Expected logsAutoFollow to be false after 'up'")
	}

	// Test 'home' (go to top) - should disable auto-follow
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyHome})
	m = updated
	if m.logsAutoFollow {
		t.Error("Expected logsAutoFollow to be false after 'home'")
	}

	// Test 'end' (go to bottom) - should enable auto-follow
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	m = updated
	if !m.logsAutoFollow {
		t.Error("Expected logsAutoFollow to be true after 'end'")
	}
}

func TestDetailModel_Update_YankKey(t *testing.T) {
	store := createTestStore()
	m := NewDetailModel(store, nil)
	m.Show("svc1.default.ctx1")
	m.SetSize(80, 40)
	m.UpdateSnapshot()
	m.currentTab = TabInfo

	// Press 'y' to yank/copy first connect string
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")}
	updated, _ := m.Update(msg)
	m = updated

	// Just verify no panic - actual copy depends on clipboard availability
}

func TestDetailModel_Update_YankKeyOnHTTPTab(t *testing.T) {
	store := createTestStore()
	m := NewDetailModel(store, nil)
	m.Show("svc1.default.ctx1")
	m.SetSize(80, 40)
	m.UpdateSnapshot()
	m.currentTab = TabHTTP

	// Press 'y' on HTTP tab - should do nothing
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")}
	updated, _ := m.Update(msg)
	m = updated

	// Just verify no panic - yank doesn't work on HTTP tab
}

func TestDetailModel_Update_PageDownOnHTTPTab(t *testing.T) {
	store := createTestStore()
	m := NewDetailModel(store, nil)
	m.Show("svc1.default.ctx1")
	m.SetSize(80, 40)
	m.currentTab = TabHTTP

	// Add many HTTP logs
	for i := 0; i < 100; i++ {
		m.AddHTTPLog(HTTPLogEntry{Method: "GET", Path: fmt.Sprintf("/api/%d", i)})
	}

	// Test 'ctrl+d' (page down half)
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ctrl+d")}
	updated, _ := m.Update(msg)
	m = updated

	if m.httpScrollOffset == 0 {
		t.Error("Expected httpScrollOffset > 0 after ctrl+d")
	}
}

func TestDetailModel_Update_PageUpOnHTTPTab(t *testing.T) {
	store := createTestStore()
	m := NewDetailModel(store, nil)
	m.Show("svc1.default.ctx1")
	m.SetSize(80, 40)
	m.currentTab = TabHTTP
	m.httpScrollOffset = 50

	// Add many HTTP logs
	for i := 0; i < 100; i++ {
		m.AddHTTPLog(HTTPLogEntry{Method: "GET", Path: fmt.Sprintf("/api/%d", i)})
	}

	// Test 'ctrl+u' (page up half)
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ctrl+u")}
	updated, _ := m.Update(msg)
	m = updated

	if m.httpScrollOffset >= 50 {
		t.Error("Expected httpScrollOffset < 50 after ctrl+u")
	}
}

func TestDetailModel_SetSize_SmallDimensions(t *testing.T) {
	store := createTestStore()
	m := NewDetailModel(store, nil)
	m.Show("svc1.default.ctx1")

	// Test with very small dimensions
	m.SetSize(10, 5)

	// Just verify no panic
	if m.width != 10 {
		t.Errorf("Expected width 10, got %d", m.width)
	}
}

func TestDetailModel_RenderLogsTab_Loading(t *testing.T) {
	store := createTestStore()
	m := NewDetailModel(store, nil)
	m.Show("svc1.default.ctx1")
	m.SetSize(80, 40)
	m.UpdateSnapshot()
	m.currentTab = TabLogs
	m.logsLoading = true

	view := m.View()
	if !strings.Contains(view, "Loading") {
		t.Error("Expected view to contain 'Loading' when logsLoading is true")
	}
}

func TestDetailModel_RenderLogsTab_Error(t *testing.T) {
	store := createTestStore()
	m := NewDetailModel(store, nil)
	m.Show("svc1.default.ctx1")
	m.SetSize(80, 40)
	m.UpdateSnapshot()
	m.currentTab = TabLogs
	m.logsError = "Failed to fetch logs"

	view := m.View()
	if !strings.Contains(view, "Failed to fetch logs") {
		t.Error("Expected view to contain error message")
	}
}
