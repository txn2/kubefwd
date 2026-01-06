package components

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/txn2/kubefwd/pkg/fwdapi/types"
	"github.com/txn2/kubefwd/pkg/fwdtui/styles"
)

// BrowseView represents the current view level in the browse modal
type BrowseView int

const (
	NamespacesView BrowseView = iota
	ContextsView
	ServicesView
)

// BrowseModel implements the browse modal for discovering and forwarding services
type BrowseModel struct {
	visible bool
	width   int
	height  int

	// Navigation state
	currentView       BrowseView
	currentContext    string
	selectedNamespace string

	// Data (cached from API calls)
	contexts   []types.K8sContext
	namespaces []types.K8sNamespace
	services   []types.K8sService

	// UI state
	selectedIndex int
	scrollOffset  int
	loading       bool
	errorMsg      string

	// Injected dependencies (set via SetDiscovery/SetController)
	discovery           types.KubernetesDiscovery
	namespaceController types.NamespaceController
	serviceCRUD         types.ServiceCRUD
}

// Browse modal message types

// BrowseContextsLoadedMsg is sent when contexts are loaded
type BrowseContextsLoadedMsg struct {
	Contexts       []types.K8sContext
	CurrentContext string
	Error          error
}

// BrowseNamespacesLoadedMsg is sent when namespaces are loaded
type BrowseNamespacesLoadedMsg struct {
	Namespaces []types.K8sNamespace
	Error      error
}

// BrowseServicesLoadedMsg is sent when services are loaded
type BrowseServicesLoadedMsg struct {
	Services []types.K8sService
	Error    error
}

// ServiceForwardedMsg is sent when a service is successfully forwarded
type ServiceForwardedMsg struct {
	Key         string
	ServiceName string
	Namespace   string
	LocalIP     string
	Hostnames   []string
	Error       error
}

// NamespaceForwardedMsg is sent when a namespace is successfully forwarded
type NamespaceForwardedMsg struct {
	Namespace    string
	Context      string
	ServiceCount int
	Error        error
}

// NewBrowseModel creates a new browse model
func NewBrowseModel() BrowseModel {
	return BrowseModel{
		currentView: NamespacesView,
	}
}

// SetDiscovery sets the Kubernetes discovery adapter
func (m *BrowseModel) SetDiscovery(d types.KubernetesDiscovery) {
	m.discovery = d
}

// SetNamespaceController sets the namespace controller
func (m *BrowseModel) SetNamespaceController(nc types.NamespaceController) {
	m.namespaceController = nc
}

// SetServiceCRUD sets the service CRUD adapter
func (m *BrowseModel) SetServiceCRUD(sc types.ServiceCRUD) {
	m.serviceCRUD = sc
}

// IsVisible returns whether the modal is visible
func (m *BrowseModel) IsVisible() bool {
	return m.visible
}

// Show opens the browse modal and loads initial data
func (m *BrowseModel) Show() tea.Cmd {
	m.visible = true
	m.currentView = NamespacesView
	m.selectedIndex = 0
	m.scrollOffset = 0
	m.errorMsg = ""
	m.loading = true

	// Load contexts first to get current context, then load namespaces
	return m.loadContexts()
}

// Hide closes the browse modal
func (m *BrowseModel) Hide() {
	m.visible = false
	m.loading = false
	m.errorMsg = ""
}

// SetSize sets the modal dimensions
func (m *BrowseModel) SetSize(width, height int) {
	m.width = width
	m.height = height
}

// Init implements tea.Model
func (m *BrowseModel) Init() tea.Cmd {
	return nil
}

// Update handles messages for the browse modal
func (m *BrowseModel) Update(msg tea.Msg) (BrowseModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKeyMsg(msg)

	case BrowseContextsLoadedMsg:
		m.loading = false
		if msg.Error != nil {
			m.errorMsg = fmt.Sprintf("Error loading contexts: %v", msg.Error)
			return *m, nil
		}
		m.contexts = msg.Contexts
		m.currentContext = msg.CurrentContext
		// Now load namespaces for current context
		m.loading = true
		return *m, m.loadNamespaces()

	case BrowseNamespacesLoadedMsg:
		m.loading = false
		if msg.Error != nil {
			m.errorMsg = fmt.Sprintf("Error loading namespaces: %v", msg.Error)
			return *m, nil
		}
		m.namespaces = msg.Namespaces
		m.selectedIndex = 0
		m.scrollOffset = 0
		return *m, nil

	case BrowseServicesLoadedMsg:
		m.loading = false
		if msg.Error != nil {
			m.errorMsg = fmt.Sprintf("Error loading services: %v", msg.Error)
			return *m, nil
		}
		m.services = msg.Services
		m.selectedIndex = 0 // 0 is "Forward All"
		m.scrollOffset = 0
		return *m, nil

	case ServiceForwardedMsg:
		m.loading = false
		if msg.Error != nil {
			m.errorMsg = fmt.Sprintf("Error forwarding service: %v", msg.Error)
			return *m, nil
		}
		// Reload services to update forwarded status
		m.loading = true
		return *m, m.loadServices(m.selectedNamespace)

	case NamespaceForwardedMsg:
		m.loading = false
		if msg.Error != nil {
			m.errorMsg = fmt.Sprintf("Error forwarding namespace: %v", msg.Error)
			return *m, nil
		}
		// Close modal after forwarding namespace
		m.Hide()
		return *m, nil
	}

	return *m, nil
}

// handleKeyMsg handles keyboard input
func (m *BrowseModel) handleKeyMsg(msg tea.KeyMsg) (BrowseModel, tea.Cmd) {
	// Always allow 'q' to close the modal, even during loading
	if msg.String() == "q" {
		m.Hide()
		return *m, nil
	}

	// During loading, also allow 'esc' to close (abort)
	if m.loading && msg.String() == "esc" {
		m.Hide()
		return *m, nil
	}

	// Don't handle other keys while loading
	if m.loading {
		return *m, nil
	}

	// Clear error on any keypress
	if m.errorMsg != "" {
		m.errorMsg = ""
	}

	switch m.currentView {
	case NamespacesView:
		return m.handleNamespacesViewKey(msg)
	case ContextsView:
		return m.handleContextsViewKey(msg)
	case ServicesView:
		return m.handleServicesViewKey(msg)
	}

	return *m, nil
}

// handleNamespacesViewKey handles keys in namespaces view
func (m *BrowseModel) handleNamespacesViewKey(msg tea.KeyMsg) (BrowseModel, tea.Cmd) {
	switch msg.String() {
	case "q", "esc", "f":
		m.Hide()
		return *m, nil

	case "x":
		// Switch to contexts view
		m.currentView = ContextsView
		m.selectedIndex = m.findCurrentContextIndex()
		m.scrollOffset = 0
		return *m, nil

	case "j", "down":
		if len(m.namespaces) > 0 && m.selectedIndex < len(m.namespaces)-1 {
			m.selectedIndex++
			m.adjustScrollOffset()
		}
		return *m, nil

	case "k", "up":
		if m.selectedIndex > 0 {
			m.selectedIndex--
			m.adjustScrollOffset()
		}
		return *m, nil

	case "g":
		m.selectedIndex = 0
		m.scrollOffset = 0
		return *m, nil

	case "G":
		if len(m.namespaces) > 0 {
			m.selectedIndex = len(m.namespaces) - 1
			m.adjustScrollOffset()
		}
		return *m, nil

	case "enter":
		if len(m.namespaces) > 0 && m.selectedIndex < len(m.namespaces) {
			// Navigate to services view
			m.selectedNamespace = m.namespaces[m.selectedIndex].Name
			m.currentView = ServicesView
			m.loading = true
			m.selectedIndex = 0
			m.scrollOffset = 0
			return *m, m.loadServices(m.selectedNamespace)
		}
		return *m, nil
	}

	return *m, nil
}

// handleContextsViewKey handles keys in contexts view
func (m *BrowseModel) handleContextsViewKey(msg tea.KeyMsg) (BrowseModel, tea.Cmd) {
	switch msg.String() {
	case "q":
		m.Hide()
		return *m, nil

	case "esc", "left":
		// Go back to namespaces view
		m.currentView = NamespacesView
		m.selectedIndex = 0
		m.scrollOffset = 0
		return *m, nil

	case "j", "down":
		if len(m.contexts) > 0 && m.selectedIndex < len(m.contexts)-1 {
			m.selectedIndex++
			m.adjustScrollOffset()
		}
		return *m, nil

	case "k", "up":
		if m.selectedIndex > 0 {
			m.selectedIndex--
			m.adjustScrollOffset()
		}
		return *m, nil

	case "enter":
		if len(m.contexts) > 0 && m.selectedIndex < len(m.contexts) {
			// Select new context and reload namespaces
			m.currentContext = m.contexts[m.selectedIndex].Name
			m.currentView = NamespacesView
			m.loading = true
			m.selectedIndex = 0
			m.scrollOffset = 0
			return *m, m.loadNamespaces()
		}
		return *m, nil
	}

	return *m, nil
}

// handleServicesViewKey handles keys in services view
func (m *BrowseModel) handleServicesViewKey(msg tea.KeyMsg) (BrowseModel, tea.Cmd) {
	switch msg.String() {
	case "q":
		m.Hide()
		return *m, nil

	case "esc", "left":
		// Go back to namespaces view
		m.currentView = NamespacesView
		m.selectedIndex = 0
		m.scrollOffset = 0
		return *m, nil

	case "j", "down":
		// +1 for "Forward All" option at index 0
		maxIndex := len(m.services)
		if m.selectedIndex < maxIndex {
			m.selectedIndex++
			m.adjustScrollOffset()
		}
		return *m, nil

	case "k", "up":
		if m.selectedIndex > 0 {
			m.selectedIndex--
			m.adjustScrollOffset()
		}
		return *m, nil

	case "g":
		m.selectedIndex = 0
		m.scrollOffset = 0
		return *m, nil

	case "G":
		m.selectedIndex = len(m.services) // Forward All + services
		m.adjustScrollOffset()
		return *m, nil

	case "enter":
		if m.selectedIndex == 0 {
			// "Forward All" selected
			m.loading = true
			return *m, m.forwardNamespace(m.selectedNamespace)
		}
		// Individual service selected
		svcIndex := m.selectedIndex - 1
		if svcIndex >= 0 && svcIndex < len(m.services) {
			svc := m.services[svcIndex]
			if svc.Forwarded {
				// Already forwarded, show message
				m.errorMsg = fmt.Sprintf("%s is already forwarded", svc.Name)
				return *m, nil
			}
			m.loading = true
			return *m, m.forwardService(svc)
		}
		return *m, nil
	}

	return *m, nil
}

// loadContexts loads available Kubernetes contexts
func (m *BrowseModel) loadContexts() tea.Cmd {
	return func() tea.Msg {
		if m.discovery == nil {
			return BrowseContextsLoadedMsg{Error: fmt.Errorf("discovery adapter not configured")}
		}

		resp, err := m.discovery.ListContexts()
		if err != nil {
			return BrowseContextsLoadedMsg{Error: err}
		}

		return BrowseContextsLoadedMsg{
			Contexts:       resp.Contexts,
			CurrentContext: resp.CurrentContext,
		}
	}
}

// loadNamespaces loads namespaces for the current context
func (m *BrowseModel) loadNamespaces() tea.Cmd {
	ctx := m.currentContext
	return func() tea.Msg {
		if m.discovery == nil {
			return BrowseNamespacesLoadedMsg{Error: fmt.Errorf("discovery adapter not configured")}
		}

		namespaces, err := m.discovery.ListNamespaces(ctx)
		if err != nil {
			return BrowseNamespacesLoadedMsg{Error: err}
		}

		return BrowseNamespacesLoadedMsg{Namespaces: namespaces}
	}
}

// loadServices loads services in a namespace
func (m *BrowseModel) loadServices(namespace string) tea.Cmd {
	ctx := m.currentContext
	return func() tea.Msg {
		if m.discovery == nil {
			return BrowseServicesLoadedMsg{Error: fmt.Errorf("discovery adapter not configured")}
		}

		services, err := m.discovery.ListServices(ctx, namespace)
		if err != nil {
			return BrowseServicesLoadedMsg{Error: err}
		}

		return BrowseServicesLoadedMsg{Services: services}
	}
}

// forwardService forwards a single service
func (m *BrowseModel) forwardService(svc types.K8sService) tea.Cmd {
	ctx := m.currentContext
	return func() tea.Msg {
		if m.serviceCRUD == nil {
			return ServiceForwardedMsg{Error: fmt.Errorf("service CRUD adapter not configured")}
		}

		req := types.AddServiceRequest{
			Namespace:   svc.Namespace,
			ServiceName: svc.Name,
			Context:     ctx,
		}

		resp, err := m.serviceCRUD.AddService(req)
		if err != nil {
			return ServiceForwardedMsg{Error: err}
		}

		return ServiceForwardedMsg{
			Key:         resp.Key,
			ServiceName: resp.ServiceName,
			Namespace:   resp.Namespace,
			LocalIP:     resp.LocalIP,
			Hostnames:   resp.Hostnames,
		}
	}
}

// forwardNamespace forwards all services in a namespace
func (m *BrowseModel) forwardNamespace(namespace string) tea.Cmd {
	ctx := m.currentContext
	return func() tea.Msg {
		if m.namespaceController == nil {
			return NamespaceForwardedMsg{Error: fmt.Errorf("namespace controller not configured")}
		}

		resp, err := m.namespaceController.AddNamespace(ctx, namespace, types.AddNamespaceOpts{})
		if err != nil {
			return NamespaceForwardedMsg{Error: err}
		}

		return NamespaceForwardedMsg{
			Namespace:    resp.Namespace,
			Context:      resp.Context,
			ServiceCount: resp.ServiceCount,
		}
	}
}

// findCurrentContextIndex returns the index of the current context in the contexts list
func (m *BrowseModel) findCurrentContextIndex() int {
	for i, ctx := range m.contexts {
		if ctx.Name == m.currentContext {
			return i
		}
	}
	return 0
}

// adjustScrollOffset adjusts scroll offset to keep selection visible
func (m *BrowseModel) adjustScrollOffset() {
	visibleItems := m.getVisibleItemCount()
	if visibleItems <= 0 {
		return
	}

	// Scroll down if needed
	if m.selectedIndex >= m.scrollOffset+visibleItems {
		m.scrollOffset = m.selectedIndex - visibleItems + 1
	}
	// Scroll up if needed
	if m.selectedIndex < m.scrollOffset {
		m.scrollOffset = m.selectedIndex
	}
}

// getVisibleItemCount returns how many items can be visible in the list area
func (m *BrowseModel) getVisibleItemCount() int {
	// Modal height minus title, subtitle, footer, borders, padding
	availableHeight := m.height - 12
	if availableHeight < 3 {
		return 3
	}
	return availableHeight
}

// View renders the browse modal
func (m *BrowseModel) View() string {
	if !m.visible {
		return ""
	}

	var content strings.Builder

	// Title based on current view
	switch m.currentView {
	case NamespacesView:
		content.WriteString(styles.BrowseTitleStyle.Render("Browse Namespaces"))
		content.WriteString("\n")
		ctxDisplay := styles.BrowseSubtitleStyle.Render("Context: " + m.currentContext)
		hint := styles.BrowseHintStyle.Render(" [x: change]")
		content.WriteString(ctxDisplay + hint)

	case ContextsView:
		content.WriteString(styles.BrowseTitleStyle.Render("Select Context"))
		content.WriteString("\n")
		content.WriteString(styles.BrowseHintStyle.Render("Current: " + m.currentContext))

	case ServicesView:
		content.WriteString(styles.BrowseTitleStyle.Render("Services in " + m.selectedNamespace))
		content.WriteString("\n")
		content.WriteString(styles.BrowseSubtitleStyle.Render("Context: " + m.currentContext))
	}
	content.WriteString("\n\n")

	// Loading state
	if m.loading {
		content.WriteString(styles.BrowseHintStyle.Render("Loading..."))
		return m.wrapInModal(content.String())
	}

	// Error state
	if m.errorMsg != "" {
		content.WriteString(styles.StatusErrorStyle.Render(m.errorMsg))
		content.WriteString("\n\n")
	}

	// List content based on view
	switch m.currentView {
	case NamespacesView:
		content.WriteString(m.renderNamespaceList())
	case ContextsView:
		content.WriteString(m.renderContextList())
	case ServicesView:
		content.WriteString(m.renderServiceList())
	}

	return m.wrapInModalWithFooter(content.String(), m.renderFooter())
}

// renderNamespaceList renders the list of namespaces
func (m *BrowseModel) renderNamespaceList() string {
	if len(m.namespaces) == 0 {
		return styles.BrowseHintStyle.Render("No namespaces found")
	}

	var sb strings.Builder
	visibleItems := m.getVisibleItemCount()
	endIndex := m.scrollOffset + visibleItems
	if endIndex > len(m.namespaces) {
		endIndex = len(m.namespaces)
	}

	for i := m.scrollOffset; i < endIndex; i++ {
		ns := m.namespaces[i]
		line := m.renderListItem(i, ns.Name, ns.Forwarded)
		sb.WriteString(line)
		if i < endIndex-1 {
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// renderContextList renders the list of contexts
func (m *BrowseModel) renderContextList() string {
	if len(m.contexts) == 0 {
		return styles.BrowseHintStyle.Render("No contexts found")
	}

	var sb strings.Builder
	visibleItems := m.getVisibleItemCount()
	endIndex := m.scrollOffset + visibleItems
	if endIndex > len(m.contexts) {
		endIndex = len(m.contexts)
	}

	for i := m.scrollOffset; i < endIndex; i++ {
		ctx := m.contexts[i]
		name := ctx.Name
		if ctx.Active {
			name += " (current)"
		}
		line := m.renderListItem(i, name, false)
		sb.WriteString(line)
		if i < endIndex-1 {
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// renderServiceList renders the list of services with "Forward All" at top
func (m *BrowseModel) renderServiceList() string {
	var sb strings.Builder

	// "Forward All" option at index 0
	forwardAllText := "Forward All Services"
	if m.selectedIndex == 0 {
		sb.WriteString(styles.BrowseSelectedStyle.Render("> " + forwardAllText))
	} else {
		sb.WriteString(styles.BrowseForwardAllStyle.Render("  " + forwardAllText))
	}
	sb.WriteString("\n")
	sb.WriteString(styles.BrowseHintStyle.Render("  ────────────────────"))
	sb.WriteString("\n")

	if len(m.services) == 0 {
		sb.WriteString(styles.BrowseHintStyle.Render("  No services found"))
		return sb.String()
	}

	// Service list starting at index 1
	visibleItems := m.getVisibleItemCount() - 2 // Account for Forward All and separator
	if visibleItems < 1 {
		visibleItems = 1
	}

	// Adjust for services only (offset by 1 for Forward All)
	svcScrollOffset := 0
	if m.scrollOffset > 0 {
		svcScrollOffset = m.scrollOffset - 1
		if svcScrollOffset < 0 {
			svcScrollOffset = 0
		}
	}

	endIndex := svcScrollOffset + visibleItems
	if endIndex > len(m.services) {
		endIndex = len(m.services)
	}

	for i := svcScrollOffset; i < endIndex; i++ {
		svc := m.services[i]
		// selectedIndex 1 = services[0], selectedIndex 2 = services[1], etc.
		isSelected := m.selectedIndex == i+1

		// Format: name (type) [ports]
		portStr := m.formatPorts(svc.Ports)
		displayName := fmt.Sprintf("%s (%s) %s", svc.Name, svc.Type, portStr)

		var line string
		switch {
		case isSelected:
			line = styles.BrowseSelectedStyle.Render("> " + displayName)
		case svc.Forwarded:
			line = styles.BrowseForwardedStyle.Render("  " + displayName + " [forwarded]")
		default:
			line = styles.BrowseItemStyle.Render("  " + displayName)
		}
		sb.WriteString(line)
		if i < endIndex-1 {
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// renderListItem renders a single list item
func (m *BrowseModel) renderListItem(index int, text string, forwarded bool) string {
	isSelected := index == m.selectedIndex

	if isSelected {
		return styles.BrowseSelectedStyle.Render("> " + text)
	}
	if forwarded {
		return styles.BrowseForwardedStyle.Render("  " + text + " [forwarded]")
	}
	return styles.BrowseItemStyle.Render("  " + text)
}

// formatPorts formats port information for display
func (m *BrowseModel) formatPorts(ports []types.K8sServicePort) string {
	if len(ports) == 0 {
		return ""
	}

	var portStrs []string
	for _, p := range ports {
		portStrs = append(portStrs, fmt.Sprintf("%d", p.Port))
	}

	return "[" + strings.Join(portStrs, ",") + "]"
}

// renderFooter renders the footer with keybindings
func (m *BrowseModel) renderFooter() string {
	var hints []string

	switch m.currentView {
	case NamespacesView:
		hints = []string{"j/k: navigate", "enter: browse services", "x: change context", "esc: close"}
	case ContextsView:
		hints = []string{"j/k: navigate", "enter: select", "esc: back"}
	case ServicesView:
		hints = []string{"j/k: navigate", "enter: forward", "esc: back"}
	}

	return styles.BrowseHintStyle.Render(strings.Join(hints, " | "))
}

// wrapInModal wraps content in modal styling
func (m *BrowseModel) wrapInModal(content string) string {
	return m.wrapInModalWithFooter(content, "")
}

// wrapInModalWithFooter wraps content in modal styling with footer at bottom
func (m *BrowseModel) wrapInModalWithFooter(content, footer string) string {
	// Calculate modal dimensions
	modalWidth := m.width - 10
	if modalWidth < 40 {
		modalWidth = 40
	}
	if modalWidth > 80 {
		modalWidth = 80
	}

	modalHeight := m.height - 6
	if modalHeight < 10 {
		modalHeight = 10
	}

	// Inner height (modal height minus border=2 and vertical padding=2)
	innerHeight := modalHeight - 4
	if innerHeight < 3 {
		innerHeight = 3
	}

	var finalContent string
	if footer != "" {
		// Use lipgloss.Place to position footer at very bottom of inner area
		// First, place content at top
		contentArea := lipgloss.Place(
			modalWidth-6,  // inner width (minus border + horizontal padding)
			innerHeight-2, // leave 2 lines: 1 blank + 1 footer
			lipgloss.Left,
			lipgloss.Top,
			content,
		)
		// Join with footer (blank line above for balanced spacing)
		finalContent = lipgloss.JoinVertical(lipgloss.Left, contentArea, "", footer)
	} else {
		finalContent = content
	}

	// Apply modal style WITHOUT setting Height - let content determine it
	// The outer lipgloss.Place will handle centering
	styledContent := styles.BrowseModalStyle.
		Width(modalWidth).
		Render(finalContent)

	// Center the modal in the terminal
	return lipgloss.Place(
		m.width,
		m.height,
		lipgloss.Center,
		lipgloss.Center,
		styledContent,
	)
}

// GetCurrentContext returns the currently selected context
func (m *BrowseModel) GetCurrentContext() string {
	return m.currentContext
}
