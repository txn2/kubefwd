package components

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/evertras/bubble-table/table"
	"github.com/txn2/kubefwd/pkg/fwdtui/state"
	"github.com/txn2/kubefwd/pkg/fwdtui/styles"
)

// OpenDetailMsg is sent when detail view should open (kept for compatibility)
type OpenDetailMsg struct {
	Key string
}

// RemoveForwardMsg is sent when a forward should be removed
type RemoveForwardMsg struct {
	RegistryKey string // service.namespace.context key for registry lookup
}

// Column keys
const (
	colKeyKey         = "_key"         // Hidden key column for row identification
	colKeyRegistryKey = "_registrykey" // Hidden registry key for service removal
	colKeyHostname    = "hostname"
	colKeyLocalAddr   = "localaddr"
	colKeyPod         = "pod"
	colKeyPort        = "port"
	colKeyNamespace   = "namespace"
	colKeyContext     = "context"
	colKeyStatus      = "status"
	colKeyBytesIn     = "bytesin"
	colKeyBytesOut    = "bytesout"
	colKeyRateIn      = "ratein"
	colKeyRateOut     = "rateout"
)

// ServicesModel displays the services table
type ServicesModel struct {
	table         table.Model
	store         *state.Store
	width         int
	height        int
	pageSize      int // Current page size for click-to-row calculation
	focused       bool
	showNS        bool // Show namespace column
	showCtx       bool // Show context column
	showBandwidth bool // Show bandwidth columns (Total In/Out, Rate In/Out)
	compactView   bool // Compact view: show Port instead of Local Address + Pod
	filtering     bool
	filterText    string
}

// NewServicesModel creates a new services model
func NewServicesModel(store *state.Store) ServicesModel {
	columns := buildColumns(false, false, true, true) // Default to compact view

	m := ServicesModel{
		store:         store,
		focused:       true,
		showBandwidth: true,
		compactView:   true, // Default to compact view - more useful for developers
	}

	m.table = table.New(columns).
		WithBaseStyle(lipgloss.NewStyle().Padding(0, 1)).
		BorderRounded().
		HeaderStyle(styles.TableHeaderStyle).
		HighlightStyle(styles.TableSelectedStyle).
		Focused(true).
		WithPageSize(20).
		WithFooterVisibility(false)

	return m
}

// buildColumns creates the table columns based on visibility settings
func buildColumns(showNS, showCtx, showBandwidth, compactView bool) []table.Column {
	cols := []table.Column{
		table.NewFlexColumn(colKeyHostname, "Hostname", 3),
	}

	if compactView {
		cols = append(cols, table.NewColumn(colKeyPort, "Port", 12))
	} else {
		cols = append(cols,
			table.NewColumn(colKeyLocalAddr, "Local Address", 18),
			table.NewFlexColumn(colKeyPod, "Pod", 2),
		)
	}

	if showNS {
		cols = append(cols, table.NewColumn(colKeyNamespace, "Namespace", 15))
	}
	if showCtx {
		cols = append(cols, table.NewColumn(colKeyContext, "Context", 15))
	}

	cols = append(cols, table.NewColumn(colKeyStatus, "Status", 12))

	if showBandwidth {
		cols = append(cols,
			table.NewColumn(colKeyBytesIn, "Total In", 10),
			table.NewColumn(colKeyBytesOut, "Total Out", 10),
			table.NewColumn(colKeyRateIn, "Rate In", 11),
			table.NewColumn(colKeyRateOut, "Rate Out", 11),
		)
	}

	return cols
}

// Init initializes the services model
func (m *ServicesModel) Init() tea.Cmd {
	return nil
}

// handleFilterKeyMsg handles key messages while in filtering mode
func (m *ServicesModel) handleFilterKeyMsg(msg tea.KeyMsg) (ServicesModel, tea.Cmd) {
	switch msg.String() {
	case "enter", "esc":
		m.filtering = false
		if msg.String() == "enter" {
			m.store.SetFilter(m.filterText)
		} else {
			m.filterText = m.store.GetFilter()
		}
		m.Refresh()
	case "backspace":
		if len(m.filterText) > 0 {
			m.filterText = m.filterText[:len(m.filterText)-1]
		}
	default:
		if len(msg.String()) == 1 {
			m.filterText += msg.String()
		}
	}
	return *m, nil
}

// handleNormalKeyMsg handles key messages in normal (non-filtering) mode
// Returns the updated model, command, and whether the key was fully handled
func (m *ServicesModel) handleNormalKeyMsg(msg tea.KeyMsg) (ServicesModel, tea.Cmd, bool) {
	switch msg.String() {
	case "/":
		m.filtering = true
		m.filterText = m.store.GetFilter()
		return *m, nil, true
	case "esc":
		m.store.SetFilter("")
		m.filterText = ""
		m.Refresh()
		return *m, nil, true
	case "b":
		m.showBandwidth = !m.showBandwidth
		m.rebuildColumns()
		return *m, nil, true
	case "c":
		m.compactView = !m.compactView
		m.rebuildColumns()
		return *m, nil, true
	case "d":
		registryKey := m.GetSelectedRegistryKey()
		if registryKey != "" {
			return *m, func() tea.Msg { return RemoveForwardMsg{RegistryKey: registryKey} }, true
		}
		return *m, nil, true
	case "g", "home":
		m.table = m.table.PageFirst()
	case "G", "end":
		m.table = m.table.PageLast()
	case "pgdown":
		m.table = m.table.PageDown()
	case "pgup":
		m.table = m.table.PageUp()
	}
	return *m, nil, false
}

// handleMouseMsg handles mouse messages for scrolling
func (m *ServicesModel) handleMouseMsg(msg tea.MouseMsg) {
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		for range 3 {
			m.table = m.table.WithHighlightedRow(m.table.GetHighlightedRowIndex() - 1)
		}
	case tea.MouseButtonWheelDown:
		for range 3 {
			m.table = m.table.WithHighlightedRow(m.table.GetHighlightedRowIndex() + 1)
		}
	default:
		// Other mouse buttons not handled
	}
}

// Update handles messages for the services table
func (m *ServicesModel) Update(msg tea.Msg) (ServicesModel, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.table = m.table.WithTargetWidth(m.width - 2)

	case tea.MouseMsg:
		m.handleMouseMsg(msg)

	case tea.KeyMsg:
		if m.filtering {
			return m.handleFilterKeyMsg(msg)
		}
		if result, resultCmd, handled := m.handleNormalKeyMsg(msg); handled {
			return result, resultCmd
		}
	}

	m.table, cmd = m.table.Update(msg)
	return *m, cmd
}

// rebuildColumns rebuilds the table columns based on current visibility settings
func (m *ServicesModel) rebuildColumns() {
	columns := buildColumns(m.showNS, m.showCtx, m.showBandwidth, m.compactView)
	m.table = m.table.WithColumns(columns)
	if m.width > 0 {
		m.table = m.table.WithTargetWidth(m.width - 2)
	}
}

// Refresh updates the table with current data from the store
func (m *ServicesModel) Refresh() {
	forwards := m.store.GetFiltered()

	// Check if we need namespace/context columns
	namespaces := make(map[string]struct{})
	contexts := make(map[string]struct{})
	for _, fwd := range forwards {
		namespaces[fwd.Namespace] = struct{}{}
		contexts[fwd.Context] = struct{}{}
	}

	newShowNS := len(namespaces) > 1
	newShowCtx := len(contexts) > 1

	// Rebuild columns if visibility changed
	if newShowNS != m.showNS || newShowCtx != m.showCtx {
		m.showNS = newShowNS
		m.showCtx = newShowCtx
		m.rebuildColumns()
	}

	// Build rows
	rows := make([]table.Row, 0, len(forwards))
	for _, fwd := range forwards {
		rowData := table.RowData{
			colKeyKey:         fwd.Key,         // Store the forward key for detail view
			colKeyRegistryKey: fwd.RegistryKey, // Store the registry key for removal
			colKeyHostname:    fwd.PrimaryHostname(),
			colKeyLocalAddr:   fwd.LocalAddress(),
			colKeyPod:         fwd.PodName,
			colKeyPort:        formatPort(fwd.LocalPort, fwd.PodPort),
			colKeyStatus:      table.NewStyledCell(fwd.Status.String(), statusStyle(fwd.Status)),
			colKeyBytesIn:     humanBytes(fwd.BytesIn),
			colKeyBytesOut:    humanBytes(fwd.BytesOut),
			colKeyRateIn:      humanRate(fwd.RateIn),
			colKeyRateOut:     humanRate(fwd.RateOut),
		}

		if m.showNS {
			rowData[colKeyNamespace] = fwd.Namespace
		}
		if m.showCtx {
			rowData[colKeyContext] = fwd.Context
		}

		rows = append(rows, table.NewRow(rowData))
	}

	m.table = m.table.WithRows(rows)
	if m.width > 0 {
		m.table = m.table.WithTargetWidth(m.width - 2)
	}
}

// View renders the services table (no border - parent handles that)
func (m *ServicesModel) View() string {
	var filterLine string
	if m.filtering {
		filterLine = fmt.Sprintf(" Filter: %s█", m.filterText)
	} else if m.filterText != "" {
		filterLine = fmt.Sprintf(" Filter: %s (press Esc to clear)", m.filterText)
	}

	tableView := m.table.View()

	if filterLine != "" {
		return lipgloss.JoinVertical(lipgloss.Left, filterLine, tableView)
	}
	return tableView
}

// SetFocus sets the focus state of the table
func (m *ServicesModel) SetFocus(focused bool) {
	m.focused = focused
	m.table = m.table.Focused(focused)
}

// SetSize updates the table dimensions
func (m *ServicesModel) SetSize(width, height int) {
	m.width = width
	m.height = height
	m.table = m.table.WithTargetWidth(width - 2)
	// Table chrome with BorderRounded(): top border (1) + header (1) + separator (1) + bottom border (1) = 4
	// Plus possible filter line (1) = 5 total reserved
	pageSize := height - 5
	if pageSize < 1 {
		pageSize = 1
	}
	m.pageSize = pageSize
	m.table = m.table.WithPageSize(pageSize)
}

// IsFiltering returns whether the table is in filter mode
func (m *ServicesModel) IsFiltering() bool {
	return m.filtering
}

// statusStyle returns the lipgloss style for a status
func statusStyle(status state.ForwardStatus) lipgloss.Style {
	switch status {
	case state.StatusActive:
		return styles.StatusActiveStyle
	case state.StatusConnecting:
		return styles.StatusConnectingStyle
	case state.StatusError:
		return styles.StatusErrorStyle
	case state.StatusStopping:
		return styles.StatusStoppingStyle
	case state.StatusPending:
		return styles.StatusPendingStyle
	default:
		return styles.StatusPendingStyle
	}
}

// humanBytes formats bytes to human-readable string
func humanBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	// Bounds check: cap at exabytes (index 5)
	units := "KMGTPE"
	if exp >= len(units) {
		exp = len(units) - 1
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), units[exp])
}

// humanRate formats bytes/sec to human-readable string
func humanRate(rate float64) string {
	if rate < 1 {
		return "0 B/s"
	}
	const unit = 1024.0
	if rate < unit {
		return fmt.Sprintf("%.0f B/s", rate)
	}
	div, exp := unit, 0
	for n := rate / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	// Bounds check: cap at exabytes (index 5)
	units := "KMGTPE"
	if exp >= len(units) {
		exp = len(units) - 1
	}
	return fmt.Sprintf("%.1f %cB/s", rate/div, units[exp])
}

// formatPort formats the port mapping (local→pod)
func formatPort(localPort, podPort string) string {
	if localPort == podPort {
		return localPort
	}
	return fmt.Sprintf("%s→%s", localPort, podPort)
}

// GetSelectedKey returns the key of the currently highlighted row, or empty string if none
func (m *ServicesModel) GetSelectedKey() string {
	row := m.table.HighlightedRow()
	if row.Data == nil {
		return ""
	}
	if key, ok := row.Data[colKeyKey]; ok {
		if keyStr, ok := key.(string); ok {
			return keyStr
		}
	}
	return ""
}

// GetSelectedRegistryKey returns the registry key (service.namespace.context) of the currently highlighted row
func (m *ServicesModel) GetSelectedRegistryKey() string {
	row := m.table.HighlightedRow()
	if row.Data == nil {
		return ""
	}
	if key, ok := row.Data[colKeyRegistryKey]; ok {
		if keyStr, ok := key.(string); ok {
			return keyStr
		}
	}
	return ""
}

// HasSelection returns true if a row is selected
func (m *ServicesModel) HasSelection() bool {
	return m.GetSelectedKey() != ""
}

// SelectByY selects a row based on the Y position within the services panel
func (m *ServicesModel) SelectByY(y int) {
	// Account for table chrome:
	// - Filter line (if visible): 1 line
	// - Top border: 1 line
	// - Header row: 1 line
	// - Header separator: 1 line
	// Total offset: 3 lines (or 4 if filtering)

	offset := 3
	if m.filtering || m.filterText != "" {
		offset = 4
	}

	relativeRow := y - offset
	if relativeRow < 0 || relativeRow >= m.pageSize {
		return
	}

	// Calculate absolute row index based on current page
	// Current page is determined by the currently highlighted row
	currentHighlighted := m.table.GetHighlightedRowIndex()
	currentPage := 0
	if m.pageSize > 0 {
		currentPage = currentHighlighted / m.pageSize
	}

	absoluteRowIndex := (currentPage * m.pageSize) + relativeRow

	// Use bubble-table's WithHighlightedRow API
	m.table = m.table.WithHighlightedRow(absoluteRowIndex)
}
