package components

import (
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
