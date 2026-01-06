package fwdapi

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/txn2/kubefwd/pkg/fwdapi/types"
	"github.com/txn2/kubefwd/pkg/fwdcfg"
	"github.com/txn2/kubefwd/pkg/fwdmetrics"
	"github.com/txn2/kubefwd/pkg/fwdns"
	"github.com/txn2/kubefwd/pkg/fwdsvcregistry"
	"github.com/txn2/kubefwd/pkg/fwdtui"
	"github.com/txn2/kubefwd/pkg/fwdtui/events"
	"github.com/txn2/kubefwd/pkg/fwdtui/state"
)

// StateReaderAdapter adapts state.Store to the StateReader interface
type StateReaderAdapter struct {
	getStore func() *state.Store
}

// NewStateReaderAdapter creates a new StateReaderAdapter
func NewStateReaderAdapter(getStore func() *state.Store) *StateReaderAdapter {
	return &StateReaderAdapter{getStore: getStore}
}

func (a *StateReaderAdapter) GetServices() []state.ServiceSnapshot {
	if store := a.getStore(); store != nil {
		return store.GetServices()
	}
	return nil
}

func (a *StateReaderAdapter) GetService(key string) *state.ServiceSnapshot {
	if store := a.getStore(); store != nil {
		return store.GetService(key)
	}
	return nil
}

func (a *StateReaderAdapter) GetSummary() state.SummaryStats {
	if store := a.getStore(); store != nil {
		return store.GetSummary()
	}
	return state.SummaryStats{}
}

func (a *StateReaderAdapter) GetFiltered() []state.ForwardSnapshot {
	if store := a.getStore(); store != nil {
		return store.GetFiltered()
	}
	return nil
}

func (a *StateReaderAdapter) GetForward(key string) *state.ForwardSnapshot {
	if store := a.getStore(); store != nil {
		return store.GetForward(key)
	}
	return nil
}

func (a *StateReaderAdapter) GetLogs(count int) []state.LogEntry {
	if store := a.getStore(); store != nil {
		return store.GetLogs(count)
	}
	return nil
}

func (a *StateReaderAdapter) Count() int {
	if store := a.getStore(); store != nil {
		return store.Count()
	}
	return 0
}

func (a *StateReaderAdapter) ServiceCount() int {
	if store := a.getStore(); store != nil {
		return store.ServiceCount()
	}
	return 0
}

// MetricsProviderAdapter adapts fwdmetrics.Registry to the MetricsProvider interface
type MetricsProviderAdapter struct {
	registry *fwdmetrics.Registry
}

// NewMetricsProviderAdapter creates a new MetricsProviderAdapter
func NewMetricsProviderAdapter(registry *fwdmetrics.Registry) *MetricsProviderAdapter {
	return &MetricsProviderAdapter{registry: registry}
}

func (a *MetricsProviderAdapter) GetAllSnapshots() []fwdmetrics.ServiceSnapshot {
	if a.registry != nil {
		return a.registry.GetAllSnapshots()
	}
	return nil
}

func (a *MetricsProviderAdapter) GetServiceSnapshot(key string) *fwdmetrics.ServiceSnapshot {
	if a.registry != nil {
		return a.registry.GetServiceSnapshot(key)
	}
	return nil
}

func (a *MetricsProviderAdapter) GetTotals() (bytesIn, bytesOut uint64, rateIn, rateOut float64) {
	if a.registry != nil {
		return a.registry.GetTotals()
	}
	return 0, 0, 0, 0
}

func (a *MetricsProviderAdapter) ServiceCount() int {
	if a.registry != nil {
		return a.registry.ServiceCount()
	}
	return 0
}

func (a *MetricsProviderAdapter) PortForwardCount() int {
	if a.registry != nil {
		return a.registry.PortForwardCount()
	}
	return 0
}

// ServiceControllerAdapter adapts fwdsvcregistry to the ServiceController interface
type ServiceControllerAdapter struct {
	getStore func() *state.Store
}

// NewServiceControllerAdapter creates a new ServiceControllerAdapter
func NewServiceControllerAdapter(getStore func() *state.Store) *ServiceControllerAdapter {
	return &ServiceControllerAdapter{getStore: getStore}
}

func (a *ServiceControllerAdapter) Reconnect(key string) error {
	svc := fwdsvcregistry.Get(key)
	if svc == nil {
		return fmt.Errorf("service not found: %s", key)
	}
	go svc.ForceReconnect()
	return nil
}

func (a *ServiceControllerAdapter) ReconnectAll() int {
	store := a.getStore()
	if store == nil {
		return 0
	}

	forwards := store.GetFiltered()

	// Collect unique registry keys with errors
	erroredServices := make(map[string]bool)
	for _, fwd := range forwards {
		if fwd.Status == state.StatusError {
			key := fwd.RegistryKey
			if key == "" {
				key = fwd.ServiceKey
			}
			erroredServices[key] = true
		}
	}

	// Trigger reconnection for each errored service
	count := 0
	for registryKey := range erroredServices {
		if svcfwd := fwdsvcregistry.Get(registryKey); svcfwd != nil {
			go svcfwd.ForceReconnect()
			count++
		}
	}

	return count
}

func (a *ServiceControllerAdapter) Sync(key string, force bool) error {
	svc := fwdsvcregistry.Get(key)
	if svc == nil {
		return fmt.Errorf("service not found: %s", key)
	}
	go svc.SyncPodForwards(force)
	return nil
}

// EventStreamerAdapter adapts events.Bus to the EventStreamer interface
type EventStreamerAdapter struct {
	getEventBus func() *events.Bus
	subscribers map[<-chan events.Event]func()
	mu          sync.Mutex
}

// NewEventStreamerAdapter creates a new EventStreamerAdapter
func NewEventStreamerAdapter(getEventBus func() *events.Bus) *EventStreamerAdapter {
	return &EventStreamerAdapter{
		getEventBus: getEventBus,
		subscribers: make(map[<-chan events.Event]func()),
	}
}

func (a *EventStreamerAdapter) Subscribe() (<-chan events.Event, func()) {
	bus := a.getEventBus()
	if bus == nil {
		// Return a closed channel if bus is not available
		ch := make(chan events.Event)
		close(ch)
		return ch, func() {}
	}

	ch := make(chan events.Event, 100)

	// Subscribe to all events from the bus
	bus.SubscribeAll(func(e events.Event) {
		select {
		case ch <- e:
		default:
			// Buffer full, drop event
		}
	})

	a.mu.Lock()
	cancel := func() {
		a.mu.Lock()
		delete(a.subscribers, ch)
		a.mu.Unlock()
		close(ch)
	}
	a.subscribers[ch] = cancel
	a.mu.Unlock()

	return ch, cancel
}

func (a *EventStreamerAdapter) SubscribeType(eventType events.EventType) (<-chan events.Event, func()) {
	bus := a.getEventBus()
	if bus == nil {
		// Return a closed channel if bus is not available
		ch := make(chan events.Event)
		close(ch)
		return ch, func() {}
	}

	ch := make(chan events.Event, 100)

	// Subscribe to specific event type
	bus.Subscribe(eventType, func(e events.Event) {
		select {
		case ch <- e:
		default:
			// Buffer full, drop event
		}
	})

	a.mu.Lock()
	cancel := func() {
		a.mu.Lock()
		delete(a.subscribers, ch)
		a.mu.Unlock()
		close(ch)
	}
	a.subscribers[ch] = cancel
	a.mu.Unlock()

	return ch, cancel
}

// DiagnosticsProviderAdapter provides diagnostic information
type DiagnosticsProviderAdapter struct {
	getStore   func() *state.Store
	getManager func() types.ManagerInfo
}

// NewDiagnosticsProviderAdapter creates a new DiagnosticsProviderAdapter
func NewDiagnosticsProviderAdapter(getStore func() *state.Store, getManager func() types.ManagerInfo) *DiagnosticsProviderAdapter {
	return &DiagnosticsProviderAdapter{
		getStore:   getStore,
		getManager: getManager,
	}
}

func (a *DiagnosticsProviderAdapter) GetSummary() types.DiagnosticSummary {
	store := a.getStore()
	if store == nil {
		return types.DiagnosticSummary{
			Status:    "unknown",
			Timestamp: time.Now(),
		}
	}

	summary := store.GetSummary()
	services := store.GetServices()

	// Calculate service counts by status
	var active, errored, partial, pending int
	for _, svc := range services {
		switch {
		case svc.ActiveCount > 0 && svc.ErrorCount == 0:
			active++
		case svc.ErrorCount > 0 && svc.ActiveCount == 0:
			errored++
		case svc.ErrorCount > 0 && svc.ActiveCount > 0:
			partial++
		default:
			pending++
		}
	}

	// Determine overall status
	status := "healthy"
	if summary.ErrorCount > 0 {
		status = "degraded"
		if summary.ErrorCount > summary.ActiveServices {
			status = "unhealthy"
		}
	}

	uptime := ""
	version := ""
	if manager := a.getManager(); manager != nil {
		uptime = manager.Uptime().String()
		version = manager.Version()
	}

	// Collect current errors
	errors := a.GetErrors(10)

	// Generate recommendations
	var recommendations []string
	if errored > 0 {
		recommendations = append(recommendations, fmt.Sprintf("Reconnect %d services in error state", errored))
	}
	if partial > 0 {
		recommendations = append(recommendations, fmt.Sprintf("Investigate %d services with partial availability", partial))
	}

	return types.DiagnosticSummary{
		Status:    status,
		Timestamp: time.Now(),
		Uptime:    uptime,
		Version:   version,
		Services: types.ServicesSummaryDiag{
			Total:   summary.TotalServices,
			Active:  active,
			Error:   errored,
			Partial: partial,
			Pending: pending,
		},
		Network:         a.GetNetworkStatus(),
		Errors:          errors,
		Recommendations: recommendations,
	}
}

func (a *DiagnosticsProviderAdapter) GetServiceDiagnostic(key string) (*types.ServiceDiagnostic, error) {
	store := a.getStore()
	if store == nil {
		return nil, fmt.Errorf("state not available")
	}

	svc := store.GetService(key)
	if svc == nil {
		return nil, fmt.Errorf("service not found: %s", key)
	}

	// Calculate status
	var status string
	switch {
	case svc.ActiveCount > 0 && svc.ErrorCount == 0:
		status = "active"
	case svc.ErrorCount > 0 && svc.ActiveCount == 0:
		status = "error"
	case svc.ErrorCount > 0 && svc.ActiveCount > 0:
		status = "partial"
	default:
		status = "pending"
	}

	// Get reconnect state from registry
	reconnectState := types.ReconnectState{}
	syncState := types.SyncState{}
	if svcFwd := fwdsvcregistry.Get(key); svcFwd != nil {
		// Try to get reconnect state (if the method exists)
		reconnectState.AutoReconnectEnabled = true // Assume enabled if TUI is active
	}

	// Build forward diagnostics
	forwards := make([]types.ForwardDiagnostic, len(svc.PortForwards))
	for i, fwd := range svc.PortForwards {
		fwdDiag, _ := a.buildForwardDiagnostic(&fwd)
		forwards[i] = fwdDiag
	}

	// Collect error history from forwards
	var errorHistory []types.ErrorDetail
	for _, fwd := range svc.PortForwards {
		if fwd.Error != "" {
			errorHistory = append(errorHistory, types.ErrorDetail{
				Timestamp:   time.Now(),
				Component:   "connection",
				ServiceKey:  svc.Key,
				ForwardKey:  fwd.Key,
				PodName:     fwd.PodName,
				Message:     fwd.Error,
				Recoverable: true,
			})
		}
	}

	return &types.ServiceDiagnostic{
		Key:            svc.Key,
		ServiceName:    svc.ServiceName,
		Namespace:      svc.Namespace,
		Context:        svc.Context,
		Status:         status,
		Headless:       svc.Headless,
		ActiveCount:    svc.ActiveCount,
		ErrorCount:     svc.ErrorCount,
		ReconnectState: reconnectState,
		SyncState:      syncState,
		Forwards:       forwards,
		ErrorHistory:   errorHistory,
	}, nil
}

func (a *DiagnosticsProviderAdapter) GetForwardDiagnostic(key string) (*types.ForwardDiagnostic, error) {
	store := a.getStore()
	if store == nil {
		return nil, fmt.Errorf("state not available")
	}

	fwd := store.GetForward(key)
	if fwd == nil {
		return nil, fmt.Errorf("forward not found: %s", key)
	}

	diag, _ := a.buildForwardDiagnostic(fwd)
	return &diag, nil
}

func (a *DiagnosticsProviderAdapter) buildForwardDiagnostic(fwd *state.ForwardSnapshot) (types.ForwardDiagnostic, error) {

	connState := "disconnected"
	switch fwd.Status {
	case state.StatusPending:
		connState = "pending"
	case state.StatusConnecting:
		connState = "connecting"
	case state.StatusActive:
		connState = "connected"
	case state.StatusError:
		connState = "error"
	case state.StatusStopping:
		connState = "stopping"
	}

	// Calculate uptime and idle duration
	var uptime, idleDuration string
	if !fwd.StartedAt.IsZero() {
		uptime = time.Since(fwd.StartedAt).Round(1e9).String()
	}
	if !fwd.LastActive.IsZero() {
		idleDuration = time.Since(fwd.LastActive).Round(1e9).String()
	}

	return types.ForwardDiagnostic{
		Key:           fwd.Key,
		ServiceKey:    fwd.ServiceKey,
		PodName:       fwd.PodName,
		ContainerName: fwd.ContainerName,
		Status:        fwd.Status.String(),
		Error:         fwd.Error,
		LocalIP:       fwd.LocalIP,
		LocalPort:     fwd.LocalPort,
		PodPort:       fwd.PodPort,
		Hostnames:     fwd.Hostnames,
		ConnectedAt:   fwd.StartedAt,
		Uptime:        uptime,
		LastActive:    fwd.LastActive,
		IdleDuration:  idleDuration,
		Connection: types.ConnectionStatus{
			State:        connState,
			BytesIn:      fwd.BytesIn,
			BytesOut:     fwd.BytesOut,
			LastActivity: fwd.LastActive,
			IdleDuration: idleDuration,
		},
		BytesIn:  fwd.BytesIn,
		BytesOut: fwd.BytesOut,
		RateIn:   fwd.RateIn,
		RateOut:  fwd.RateOut,
	}, nil
}

func (a *DiagnosticsProviderAdapter) GetNetworkStatus() types.NetworkStatus {
	store := a.getStore()
	if store == nil {
		return types.NetworkStatus{}
	}

	forwards := store.GetFiltered()

	// Collect unique IPs and hostnames
	ips := make(map[string]bool)
	ports := make(map[string]bool)
	var hostnames []string
	hostnameSet := make(map[string]bool)

	for _, fwd := range forwards {
		if fwd.LocalIP != "" {
			ips[fwd.LocalIP] = true
		}
		if fwd.LocalPort != "" {
			ports[fwd.LocalPort] = true
		}
		for _, h := range fwd.Hostnames {
			if !hostnameSet[h] {
				hostnameSet[h] = true
				hostnames = append(hostnames, h)
			}
		}
	}

	return types.NetworkStatus{
		LoopbackInterface: "lo0", // macOS default, could detect from runtime
		IPsAllocated:      len(ips),
		IPRange:           "127.1.0.0/16",
		PortsInUse:        len(ports),
		Hostnames:         hostnames,
	}
}

func (a *DiagnosticsProviderAdapter) GetErrors(count int) []types.ErrorDetail {
	store := a.getStore()
	if store == nil {
		return nil
	}

	services := store.GetServices()
	var errors []types.ErrorDetail

	for _, svc := range services {
		if svc.ErrorCount == 0 {
			continue
		}

		for _, fwd := range svc.PortForwards {
			if fwd.Error == "" {
				continue
			}

			errors = append(errors, types.ErrorDetail{
				Timestamp:   time.Now(),
				Component:   "connection",
				ServiceKey:  svc.Key,
				ForwardKey:  fwd.Key,
				PodName:     fwd.PodName,
				Message:     fwd.Error,
				Recoverable: true,
			})

			if len(errors) >= count {
				return errors
			}
		}
	}

	return errors
}

// CreateAPIAdapters creates all the adapters needed for the API
func CreateAPIAdapters() (types.StateReader, types.MetricsProvider, types.ServiceController, types.EventStreamer) {
	stateReader := NewStateReaderAdapter(fwdtui.GetStore)
	metricsProvider := NewMetricsProviderAdapter(fwdmetrics.GetRegistry())
	serviceController := NewServiceControllerAdapter(fwdtui.GetStore)
	eventStreamer := NewEventStreamerAdapter(fwdtui.GetEventBus)

	return stateReader, metricsProvider, serviceController, eventStreamer
}

// CreateDiagnosticsAdapter creates the diagnostics adapter
func CreateDiagnosticsAdapter(getManager func() types.ManagerInfo) types.DiagnosticsProvider {
	return NewDiagnosticsProviderAdapter(fwdtui.GetStore, getManager)
}

// KubernetesDiscoveryAdapter provides Kubernetes resource discovery using the NamespaceManager
type KubernetesDiscoveryAdapter struct {
	getNsManager func() *fwdns.NamespaceManager
	configGetter *fwdcfg.ConfigGetter
	configPath   string
}

// NewKubernetesDiscoveryAdapter creates a new KubernetesDiscoveryAdapter
func NewKubernetesDiscoveryAdapter(getNsManager func() *fwdns.NamespaceManager, configPath string) *KubernetesDiscoveryAdapter {
	return &KubernetesDiscoveryAdapter{
		getNsManager: getNsManager,
		configGetter: fwdcfg.NewConfigGetter(),
		configPath:   configPath,
	}
}

// ListNamespaces returns available namespaces in the cluster
func (a *KubernetesDiscoveryAdapter) ListNamespaces(ctx string) ([]types.K8sNamespace, error) {
	// If no context specified, use current context
	if ctx == "" {
		currentCtx, err := a.configGetter.GetCurrentContext(a.configPath)
		if err != nil {
			return nil, fmt.Errorf("failed to get current context: %w", err)
		}
		ctx = currentCtx
	}

	nsManager := a.getNsManager()
	if nsManager == nil {
		return nil, fmt.Errorf("namespace manager not available")
	}

	// Get or create clientSet for this context
	clientSet := nsManager.GetClientSet(ctx)
	if clientSet == nil {
		// Try to create one
		restConfig, err := a.configGetter.GetRestConfig(a.configPath, ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get REST config for context %s: %w", ctx, err)
		}
		clientSet, err = kubernetes.NewForConfig(restConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create clientSet for context %s: %w", ctx, err)
		}
	}

	// List namespaces from the cluster with timeout
	ctx2, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	nsList, err := clientSet.CoreV1().Namespaces().List(ctx2, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces: %w", err)
	}

	// Get list of forwarded namespaces
	forwardedNs := make(map[string]bool)
	for _, w := range nsManager.ListWatchers() {
		if w.Context == ctx {
			forwardedNs[w.Namespace] = true
		}
	}

	result := make([]types.K8sNamespace, len(nsList.Items))
	for i, ns := range nsList.Items {
		result[i] = types.K8sNamespace{
			Name:      ns.Name,
			Status:    string(ns.Status.Phase),
			Forwarded: forwardedNs[ns.Name],
		}
	}

	return result, nil
}

// ListServices returns available services in a namespace
func (a *KubernetesDiscoveryAdapter) ListServices(ctx, namespace string) ([]types.K8sService, error) {
	// If no context specified, use current context
	if ctx == "" {
		currentCtx, err := a.configGetter.GetCurrentContext(a.configPath)
		if err != nil {
			return nil, fmt.Errorf("failed to get current context: %w", err)
		}
		ctx = currentCtx
	}

	nsManager := a.getNsManager()
	if nsManager == nil {
		return nil, fmt.Errorf("namespace manager not available")
	}

	// Get or create clientSet for this context
	clientSet := nsManager.GetClientSet(ctx)
	if clientSet == nil {
		restConfig, err := a.configGetter.GetRestConfig(a.configPath, ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get REST config for context %s: %w", ctx, err)
		}
		clientSet, err = kubernetes.NewForConfig(restConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create clientSet for context %s: %w", ctx, err)
		}
	}

	// List services from the cluster with timeout
	ctx2, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	svcList, err := clientSet.CoreV1().Services(namespace).List(ctx2, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}

	// Build map of forwarded services
	forwardedSvcs := make(map[string]string) // service name -> forward key
	for _, svc := range fwdsvcregistry.GetAll() {
		if svc.Namespace == namespace && svc.Context == ctx {
			key := svc.Svc.Name + "." + svc.Namespace + "." + svc.Context
			forwardedSvcs[svc.Svc.Name] = key
		}
	}

	result := make([]types.K8sService, len(svcList.Items))
	for i, svc := range svcList.Items {
		ports := make([]types.K8sServicePort, len(svc.Spec.Ports))
		for j, port := range svc.Spec.Ports {
			ports[j] = types.K8sServicePort{
				Name:       port.Name,
				Port:       port.Port,
				TargetPort: port.TargetPort.String(),
				Protocol:   string(port.Protocol),
			}
		}

		forwardKey, forwarded := forwardedSvcs[svc.Name]
		result[i] = types.K8sService{
			Name:       svc.Name,
			Namespace:  svc.Namespace,
			Type:       string(svc.Spec.Type),
			ClusterIP:  svc.Spec.ClusterIP,
			Ports:      ports,
			Selector:   svc.Spec.Selector,
			Forwarded:  forwarded,
			ForwardKey: forwardKey,
		}
	}

	return result, nil
}

// GetService returns details for a specific service
func (a *KubernetesDiscoveryAdapter) GetService(ctx, namespace, name string) (*types.K8sService, error) {
	nsManager := a.getNsManager()
	if nsManager == nil {
		return nil, fmt.Errorf("namespace manager not available")
	}

	clientSet := nsManager.GetClientSet(ctx)
	if clientSet == nil {
		restConfig, err := a.configGetter.GetRestConfig(a.configPath, ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get REST config for context %s: %w", ctx, err)
		}
		clientSet, err = kubernetes.NewForConfig(restConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create clientSet for context %s: %w", ctx, err)
		}
	}

	ctx2, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	svc, err := clientSet.CoreV1().Services(namespace).Get(ctx2, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("service not found: %w", err)
	}

	ports := make([]types.K8sServicePort, len(svc.Spec.Ports))
	for i, port := range svc.Spec.Ports {
		ports[i] = types.K8sServicePort{
			Name:       port.Name,
			Port:       port.Port,
			TargetPort: port.TargetPort.String(),
			Protocol:   string(port.Protocol),
		}
	}

	// Check if forwarded
	key := name + "." + namespace + "." + ctx
	forwarded := fwdsvcregistry.Get(key) != nil

	return &types.K8sService{
		Name:       svc.Name,
		Namespace:  svc.Namespace,
		Type:       string(svc.Spec.Type),
		ClusterIP:  svc.Spec.ClusterIP,
		Ports:      ports,
		Selector:   svc.Spec.Selector,
		Forwarded:  forwarded,
		ForwardKey: key,
	}, nil
}

// ListContexts returns available Kubernetes contexts
func (a *KubernetesDiscoveryAdapter) ListContexts() (*types.K8sContextsResponse, error) {
	rawConfig, err := a.configGetter.GetClientConfig(a.configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get kubeconfig: %w", err)
	}

	contexts := make([]types.K8sContext, 0, len(rawConfig.Contexts))
	for name, ctx := range rawConfig.Contexts {
		contexts = append(contexts, types.K8sContext{
			Name:      name,
			Cluster:   ctx.Cluster,
			User:      ctx.AuthInfo,
			Namespace: ctx.Namespace,
			Active:    name == rawConfig.CurrentContext,
		})
	}

	return &types.K8sContextsResponse{
		Contexts:       contexts,
		CurrentContext: rawConfig.CurrentContext,
	}, nil
}

// resolveContextAndClient resolves context and gets kubernetes clientset
func (a *KubernetesDiscoveryAdapter) resolveContextAndClient(ctx string) (string, kubernetes.Interface, error) {
	if ctx == "" {
		currentCtx, err := a.configGetter.GetCurrentContext(a.configPath)
		if err != nil {
			return "", nil, fmt.Errorf("failed to get current context: %w", err)
		}
		ctx = currentCtx
	}

	nsManager := a.getNsManager()
	if nsManager == nil {
		return "", nil, fmt.Errorf("namespace manager not available")
	}

	clientSet := nsManager.GetClientSet(ctx)
	if clientSet == nil {
		restConfig, err := a.configGetter.GetRestConfig(a.configPath, ctx)
		if err != nil {
			return "", nil, fmt.Errorf("failed to get REST config for context %s: %w", ctx, err)
		}
		var clientErr error
		clientSet, clientErr = kubernetes.NewForConfig(restConfig)
		if clientErr != nil {
			return "", nil, fmt.Errorf("failed to create clientSet for context %s: %w", ctx, clientErr)
		}
	}
	return ctx, clientSet, nil
}

// buildPodLogOptions creates PodLogOptions from request options
func buildPodLogOptions(opts types.PodLogsOptions) (*corev1.PodLogOptions, error) {
	tailLines := int64(opts.TailLines)
	if tailLines <= 0 {
		tailLines = 100
	}
	if tailLines > 1000 {
		tailLines = 1000
	}

	podLogOpts := &corev1.PodLogOptions{
		Container:  opts.Container,
		TailLines:  &tailLines,
		Previous:   opts.Previous,
		Timestamps: opts.Timestamps,
	}

	if opts.SinceTime != "" {
		sinceTime, err := time.Parse(time.RFC3339, opts.SinceTime)
		if err != nil {
			return nil, fmt.Errorf("invalid sinceTime format (expected RFC3339): %w", err)
		}
		metaSinceTime := metav1.NewTime(sinceTime)
		podLogOpts.SinceTime = &metaSinceTime
	}
	return podLogOpts, nil
}

// readPodLogStream reads logs from a stream with size limit
func readPodLogStream(logStream interface{ Read([]byte) (int, error) }) ([]string, bool, error) {
	const maxLogSize = 1024 * 1024 // 1MB
	buf := make([]byte, maxLogSize)
	n, err := logStream.Read(buf)
	if err != nil && err.Error() != "EOF" && n == 0 {
		return nil, false, fmt.Errorf("failed to read pod logs: %w", err)
	}
	return splitLogLines(string(buf[:n])), n >= maxLogSize, nil
}

// GetPodLogs returns logs from a pod
func (a *KubernetesDiscoveryAdapter) GetPodLogs(ctx, namespace, podName string, opts types.PodLogsOptions) (*types.PodLogsResponse, error) {
	ctx, clientSet, err := a.resolveContextAndClient(ctx)
	if err != nil {
		return nil, err
	}

	podLogOpts, err := buildPodLogOptions(opts)
	if err != nil {
		return nil, err
	}

	containerName := opts.Container
	if containerName == "" {
		pod, err := clientSet.CoreV1().Pods(namespace).Get(context.Background(), podName, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to get pod: %w", err)
		}
		if len(pod.Spec.Containers) > 0 {
			containerName = pod.Spec.Containers[0].Name
			podLogOpts.Container = containerName
		}
	}

	req := clientSet.CoreV1().Pods(namespace).GetLogs(podName, podLogOpts)
	logStream, err := req.Stream(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get pod logs: %w", err)
	}
	defer func() { _ = logStream.Close() }()

	lines, truncated, err := readPodLogStream(logStream)
	if err != nil {
		return nil, err
	}

	return &types.PodLogsResponse{
		PodName:       podName,
		Namespace:     namespace,
		Context:       ctx,
		ContainerName: containerName,
		Logs:          lines,
		LineCount:     len(lines),
		Truncated:     truncated,
	}, nil
}

// splitLogLines splits log content into lines, handling different line endings
func splitLogLines(content string) []string {
	if content == "" {
		return []string{}
	}

	var lines []string
	var currentLine []byte

	for i := 0; i < len(content); i++ {
		switch content[i] {
		case '\n':
			lines = append(lines, string(currentLine))
			currentLine = nil
		case '\r':
			// Handle \r\n
			if i+1 < len(content) && content[i+1] == '\n' {
				lines = append(lines, string(currentLine))
				currentLine = nil
				i++ // skip the \n
			}
		default:
			currentLine = append(currentLine, content[i])
		}
	}

	// Add last line if there's content remaining
	if len(currentLine) > 0 {
		lines = append(lines, string(currentLine))
	}

	return lines
}

// buildListOptions creates list options from request options
func buildListOptions(opts types.ListPodsOptions) metav1.ListOptions {
	listOpts := metav1.ListOptions{}
	if opts.LabelSelector != "" {
		listOpts.LabelSelector = opts.LabelSelector
	}
	if opts.FieldSelector != "" {
		listOpts.FieldSelector = opts.FieldSelector
	}
	return listOpts
}

// appendServiceSelector adds service selector to list options
func appendServiceSelector(listOpts *metav1.ListOptions, selector map[string]string) {
	if len(selector) == 0 {
		return
	}
	var selectors []string
	for k, v := range selector {
		selectors = append(selectors, fmt.Sprintf("%s=%s", k, v))
	}
	if listOpts.LabelSelector != "" {
		listOpts.LabelSelector += ","
	}
	listOpts.LabelSelector += strings.Join(selectors, ",")
}

// ListPods returns pods in a namespace
func (a *KubernetesDiscoveryAdapter) ListPods(ctx, namespace string, opts types.ListPodsOptions) ([]types.K8sPod, error) {
	_, clientSet, err := a.resolveContextAndClient(ctx)
	if err != nil {
		return nil, err
	}

	listOpts := buildListOptions(opts)

	if opts.ServiceName != "" {
		svc, err := clientSet.CoreV1().Services(namespace).Get(context.Background(), opts.ServiceName, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to get service: %w", err)
		}
		appendServiceSelector(&listOpts, svc.Spec.Selector)
	}

	podList, err := clientSet.CoreV1().Pods(namespace).List(context.Background(), listOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	result := make([]types.K8sPod, len(podList.Items))
	for i, pod := range podList.Items {
		result[i] = a.convertPodToK8sPod(&pod, opts.ServiceName)
	}

	return result, nil
}

// determinePodStatusString determines the display status string for a pod
func determinePodStatusString(pod *corev1.Pod) string {
	if pod.DeletionTimestamp != nil {
		return "Terminating"
	}
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.State.Waiting != nil && cs.State.Waiting.Reason != "" {
			return cs.State.Waiting.Reason
		}
		if cs.State.Terminated != nil && cs.State.Terminated.Reason != "" {
			return cs.State.Terminated.Reason
		}
	}
	return string(pod.Status.Phase)
}

// convertPodToK8sPod converts a k8s Pod to our K8sPod type
func (a *KubernetesDiscoveryAdapter) convertPodToK8sPod(pod *corev1.Pod, serviceName string) types.K8sPod {
	// Calculate ready count
	readyCount := 0
	totalCount := len(pod.Spec.Containers)
	var restarts int32
	containerNames := make([]string, 0, len(pod.Spec.Containers))

	for _, c := range pod.Spec.Containers {
		containerNames = append(containerNames, c.Name)
	}

	for _, cs := range pod.Status.ContainerStatuses {
		if cs.Ready {
			readyCount++
		}
		restarts += cs.RestartCount
	}

	// Calculate age
	age := ""
	if pod.Status.StartTime != nil {
		age = formatDuration(time.Since(pod.Status.StartTime.Time))
	}

	status := determinePodStatusString(pod)

	// Check if forwarded
	isForwarded := false
	forwardedPort := ""
	if svc := fwdsvcregistry.Get(serviceName + "." + pod.Namespace); svc != nil {
		for _, fwd := range svc.PortForwards {
			if fwd.PodName == pod.Name {
				isForwarded = true
				break
			}
		}
	}

	var startTime *time.Time
	if pod.Status.StartTime != nil {
		t := pod.Status.StartTime.Time
		startTime = &t
	}

	return types.K8sPod{
		Name:          pod.Name,
		Namespace:     pod.Namespace,
		Phase:         string(pod.Status.Phase),
		Status:        status,
		Ready:         fmt.Sprintf("%d/%d", readyCount, totalCount),
		Restarts:      restarts,
		Age:           age,
		IP:            pod.Status.PodIP,
		Node:          pod.Spec.NodeName,
		Labels:        pod.Labels,
		Containers:    containerNames,
		StartTime:     startTime,
		ServiceName:   serviceName,
		IsForwarded:   isForwarded,
		ForwardedPort: forwardedPort,
	}
}

// formatDuration formats a duration in human-readable form
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

// GetPod returns detailed information about a specific pod
func (a *KubernetesDiscoveryAdapter) GetPod(ctx, namespace, podName string) (*types.K8sPodDetail, error) {
	if ctx == "" {
		currentCtx, err := a.configGetter.GetCurrentContext(a.configPath)
		if err != nil {
			return nil, fmt.Errorf("failed to get current context: %w", err)
		}
		ctx = currentCtx
	}

	nsManager := a.getNsManager()
	if nsManager == nil {
		return nil, fmt.Errorf("namespace manager not available")
	}

	clientSet := nsManager.GetClientSet(ctx)
	if clientSet == nil {
		restConfig, err := a.configGetter.GetRestConfig(a.configPath, ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get REST config for context %s: %w", ctx, err)
		}
		clientSet, err = kubernetes.NewForConfig(restConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create clientSet for context %s: %w", ctx, err)
		}
	}

	pod, err := clientSet.CoreV1().Pods(namespace).Get(context.Background(), podName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get pod: %w", err)
	}

	return a.convertPodToK8sPodDetail(pod, ctx), nil
}

// buildContainerPorts extracts port info from a container spec
func buildContainerPorts(containerPorts []corev1.ContainerPort) []types.K8sContainerPort {
	ports := make([]types.K8sContainerPort, 0, len(containerPorts))
	for _, p := range containerPorts {
		ports = append(ports, types.K8sContainerPort{
			Name:          p.Name,
			ContainerPort: p.ContainerPort,
			Protocol:      string(p.Protocol),
		})
	}
	return ports
}

// buildContainerResources extracts resource requirements from a container spec
func buildContainerResources(resources corev1.ResourceRequirements) *types.K8sResourceRequire {
	if resources.Requests == nil && resources.Limits == nil {
		return nil
	}
	res := &types.K8sResourceRequire{}
	if cpu := resources.Requests.Cpu(); cpu != nil {
		res.CPURequest = cpu.String()
	}
	if mem := resources.Requests.Memory(); mem != nil {
		res.MemoryRequest = mem.String()
	}
	if cpu := resources.Limits.Cpu(); cpu != nil {
		res.CPULimit = cpu.String()
	}
	if mem := resources.Limits.Memory(); mem != nil {
		res.MemoryLimit = mem.String()
	}
	return res
}

// applyContainerStatus applies status info to a container info struct
func applyContainerStatus(ci *types.K8sContainerInfo, cs corev1.ContainerStatus) {
	ci.Ready = cs.Ready
	ci.Started = cs.Started != nil && *cs.Started
	ci.RestartCount = cs.RestartCount

	switch {
	case cs.State.Running != nil:
		ci.State = "Running"
	case cs.State.Waiting != nil:
		ci.State = "Waiting"
		ci.StateReason = cs.State.Waiting.Reason
		ci.StateMessage = cs.State.Waiting.Message
	case cs.State.Terminated != nil:
		ci.State = "Terminated"
		ci.StateReason = cs.State.Terminated.Reason
		ci.StateMessage = cs.State.Terminated.Message
	}

	if cs.LastTerminationState.Terminated != nil {
		ci.LastState = fmt.Sprintf("Terminated: %s", cs.LastTerminationState.Terminated.Reason)
	}
}

// buildContainerInfos builds container info list from pod spec and status
func buildContainerInfos(pod *corev1.Pod) []types.K8sContainerInfo {
	containerStatusMap := make(map[string]corev1.ContainerStatus)
	for _, cs := range pod.Status.ContainerStatuses {
		containerStatusMap[cs.Name] = cs
	}

	containers := make([]types.K8sContainerInfo, 0, len(pod.Spec.Containers))
	for _, c := range pod.Spec.Containers {
		ci := types.K8sContainerInfo{
			Name:      c.Name,
			Image:     c.Image,
			Ports:     buildContainerPorts(c.Ports),
			Resources: buildContainerResources(c.Resources),
		}
		if cs, ok := containerStatusMap[c.Name]; ok {
			applyContainerStatus(&ci, cs)
		}
		containers = append(containers, ci)
	}
	return containers
}

// buildPodConditions builds condition list from pod status
func buildPodConditions(conditions []corev1.PodCondition) []types.K8sPodCondition {
	result := make([]types.K8sPodCondition, 0, len(conditions))
	for _, c := range conditions {
		result = append(result, types.K8sPodCondition{
			Type:    string(c.Type),
			Status:  string(c.Status),
			Reason:  c.Reason,
			Message: c.Message,
		})
	}
	return result
}

// checkPodForwarded checks if a pod is being forwarded and returns the forward key
func checkPodForwarded(podName, podNamespace string) (bool, string) {
	allSvcs := fwdsvcregistry.GetAll()
	for _, svc := range allSvcs {
		if svc.Namespace == podNamespace {
			for _, fwd := range svc.PortForwards {
				if fwd.PodName == podName {
					return true, svc.Svc.Name + "." + svc.Namespace + "." + svc.Context
				}
			}
		}
	}
	return false, ""
}

// convertPodToK8sPodDetail converts a k8s Pod to detailed K8sPodDetail
func (a *KubernetesDiscoveryAdapter) convertPodToK8sPodDetail(pod *corev1.Pod, ctx string) *types.K8sPodDetail {
	status := string(pod.Status.Phase)
	if pod.DeletionTimestamp != nil {
		status = "Terminating"
	}

	var startTime *time.Time
	if pod.Status.StartTime != nil {
		t := pod.Status.StartTime.Time
		startTime = &t
	}

	volumes := make([]string, 0, len(pod.Spec.Volumes))
	for _, v := range pod.Spec.Volumes {
		volumes = append(volumes, v.Name)
	}

	isForwarded, forwardKey := checkPodForwarded(pod.Name, pod.Namespace)

	return &types.K8sPodDetail{
		Name:        pod.Name,
		Namespace:   pod.Namespace,
		Context:     ctx,
		Phase:       string(pod.Status.Phase),
		Status:      status,
		Message:     pod.Status.Message,
		Reason:      pod.Status.Reason,
		IP:          pod.Status.PodIP,
		HostIP:      pod.Status.HostIP,
		Node:        pod.Spec.NodeName,
		StartTime:   startTime,
		Labels:      pod.Labels,
		Annotations: pod.Annotations,
		Containers:  buildContainerInfos(pod),
		Conditions:  buildPodConditions(pod.Status.Conditions),
		Volumes:     volumes,
		QoSClass:    string(pod.Status.QOSClass),
		IsForwarded: isForwarded,
		ForwardKey:  forwardKey,
	}
}

// GetEvents returns Kubernetes events for a resource
func (a *KubernetesDiscoveryAdapter) GetEvents(ctx, namespace string, opts types.GetEventsOptions) ([]types.K8sEvent, error) {
	if ctx == "" {
		currentCtx, err := a.configGetter.GetCurrentContext(a.configPath)
		if err != nil {
			return nil, fmt.Errorf("failed to get current context: %w", err)
		}
		ctx = currentCtx
	}

	nsManager := a.getNsManager()
	if nsManager == nil {
		return nil, fmt.Errorf("namespace manager not available")
	}

	clientSet := nsManager.GetClientSet(ctx)
	if clientSet == nil {
		restConfig, err := a.configGetter.GetRestConfig(a.configPath, ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get REST config for context %s: %w", ctx, err)
		}
		clientSet, err = kubernetes.NewForConfig(restConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create clientSet for context %s: %w", ctx, err)
		}
	}

	listOpts := metav1.ListOptions{}

	// Build field selector for specific resource
	if opts.ResourceKind != "" && opts.ResourceName != "" {
		listOpts.FieldSelector = fmt.Sprintf("involvedObject.kind=%s,involvedObject.name=%s",
			opts.ResourceKind, opts.ResourceName)
	}

	eventList, err := clientSet.CoreV1().Events(namespace).List(context.Background(), listOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to list events: %w", err)
	}

	eventItems := sortAndLimitEvents(eventList.Items, opts.Limit)
	return convertEventsToK8sEvents(eventItems), nil
}

// sortAndLimitEvents sorts events by last timestamp (most recent first) and applies limit
func sortAndLimitEvents(eventList []corev1.Event, limit int) []corev1.Event {
	// Sort by last timestamp (most recent first) using bubble sort
	for i := 0; i < len(eventList)-1; i++ {
		for j := i + 1; j < len(eventList); j++ {
			if eventList[j].LastTimestamp.After(eventList[i].LastTimestamp.Time) {
				eventList[i], eventList[j] = eventList[j], eventList[i]
			}
		}
	}

	// Apply limit
	if limit <= 0 {
		limit = 50
	}
	if len(eventList) > limit {
		eventList = eventList[:limit]
	}
	return eventList
}

// convertEventsToK8sEvents converts corev1.Event slice to types.K8sEvent slice
func convertEventsToK8sEvents(eventList []corev1.Event) []types.K8sEvent {
	result := make([]types.K8sEvent, len(eventList))
	for i, e := range eventList {
		result[i] = types.K8sEvent{
			Type:           e.Type,
			Reason:         e.Reason,
			Message:        e.Message,
			Count:          e.Count,
			FirstTimestamp: e.FirstTimestamp.Time,
			LastTimestamp:  e.LastTimestamp.Time,
			Source:         e.Source.Component,
			ObjectKind:     e.InvolvedObject.Kind,
			ObjectName:     e.InvolvedObject.Name,
		}
	}
	return result
}

// convertEndpointAddress converts a k8s EndpointAddress to our type
func convertEndpointAddress(addr corev1.EndpointAddress) types.K8sEndpointAddress {
	result := types.K8sEndpointAddress{
		IP:       addr.IP,
		Hostname: addr.Hostname,
	}
	if addr.NodeName != nil {
		result.NodeName = *addr.NodeName
	}
	if addr.TargetRef != nil && addr.TargetRef.Kind == "Pod" {
		result.PodName = addr.TargetRef.Name
	}
	return result
}

// convertEndpointSubset converts a k8s EndpointSubset to our type
// nolint:staticcheck // EndpointSubset is deprecated in k8s v1.33+ but we support older versions
func convertEndpointSubset(subset corev1.EndpointSubset) types.K8sEndpointSubset {
	result := types.K8sEndpointSubset{
		Addresses:         make([]types.K8sEndpointAddress, len(subset.Addresses)),
		NotReadyAddresses: make([]types.K8sEndpointAddress, len(subset.NotReadyAddresses)),
		Ports:             make([]types.K8sEndpointPort, len(subset.Ports)),
	}
	for j, addr := range subset.Addresses {
		result.Addresses[j] = convertEndpointAddress(addr)
	}
	for j, addr := range subset.NotReadyAddresses {
		result.NotReadyAddresses[j] = convertEndpointAddress(addr)
	}
	for j, port := range subset.Ports {
		result.Ports[j] = types.K8sEndpointPort{
			Name:     port.Name,
			Port:     port.Port,
			Protocol: string(port.Protocol),
		}
	}
	return result
}

// GetEndpoints returns endpoints for a service
func (a *KubernetesDiscoveryAdapter) GetEndpoints(ctx, namespace, serviceName string) (*types.K8sEndpoints, error) {
	_, clientSet, err := a.resolveContextAndClient(ctx)
	if err != nil {
		return nil, err
	}

	endpoints, err := clientSet.CoreV1().Endpoints(namespace).Get(context.Background(), serviceName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get endpoints: %w", err)
	}

	result := &types.K8sEndpoints{
		Name:      endpoints.Name,
		Namespace: endpoints.Namespace,
		Subsets:   make([]types.K8sEndpointSubset, len(endpoints.Subsets)),
	}
	for i, subset := range endpoints.Subsets {
		result.Subsets[i] = convertEndpointSubset(subset)
	}

	return result, nil
}

// ServiceCRUDAdapter implements types.ServiceCRUD for adding/removing individual services
type ServiceCRUDAdapter struct {
	*ServiceControllerAdapter // Embed for Reconnect/ReconnectAll/Sync
	getNsManager              func() *fwdns.NamespaceManager
	configGetter              *fwdcfg.ConfigGetter
	configPath                string
}

// NewServiceCRUDAdapter creates a new ServiceCRUDAdapter
func NewServiceCRUDAdapter(
	getStore func() *state.Store,
	getNsManager func() *fwdns.NamespaceManager,
	configPath string,
) *ServiceCRUDAdapter {
	return &ServiceCRUDAdapter{
		ServiceControllerAdapter: NewServiceControllerAdapter(getStore),
		getNsManager:             getNsManager,
		configGetter:             fwdcfg.NewConfigGetter(),
		configPath:               configPath,
	}
}

// setupPodAddedSubscription creates a subscription for PodAdded events for a service
func setupPodAddedSubscription(key string) (chan events.Event, func()) {
	podAddedCh := make(chan events.Event, 10)
	bus := fwdtui.GetEventBus()
	if bus == nil {
		return podAddedCh, nil
	}

	busUnsubscribe := bus.Subscribe(events.PodAdded, func(e events.Event) {
		if e.ServiceKey == key {
			select {
			case podAddedCh <- e:
			default:
			}
		}
	})
	return podAddedCh, func() {
		busUnsubscribe()
		close(podAddedCh)
	}
}

// buildPortMappings creates port mappings from service spec
func buildPortMappings(svc *corev1.Service) []types.PortMapping {
	ports := make([]types.PortMapping, 0, len(svc.Spec.Ports))
	for _, port := range svc.Spec.Ports {
		if port.Protocol == "UDP" {
			continue
		}
		ports = append(ports, types.PortMapping{
			LocalPort:  fmt.Sprintf("%d", port.Port),
			RemotePort: port.TargetPort.String(),
			Protocol:   string(port.Protocol),
		})
	}
	return ports
}

// waitForPodAdded waits for a pod added event or times out, returning connection info
func waitForPodAdded(podAddedCh <-chan events.Event, unsubscribe func(), key string) (string, []string) {
	if unsubscribe == nil {
		return "", nil
	}
	defer unsubscribe()

	timeout := time.After(10 * time.Second)
	select {
	case e := <-podAddedCh:
		return e.LocalIP, e.Hostnames
	case <-timeout:
		if store := fwdtui.GetStore(); store != nil {
			if svcSnapshot := store.GetService(key); svcSnapshot != nil && len(svcSnapshot.PortForwards) > 0 {
				return svcSnapshot.PortForwards[0].LocalIP, svcSnapshot.PortForwards[0].Hostnames
			}
		}
		return "", nil
	}
}

// AddService forwards a specific service
func (a *ServiceCRUDAdapter) AddService(req types.AddServiceRequest) (*types.AddServiceResponse, error) {
	nsManager := a.getNsManager()
	if nsManager == nil {
		return nil, fmt.Errorf("namespace manager not available")
	}

	ctx := req.Context
	if ctx == "" {
		currentCtx, err := nsManager.GetCurrentContext()
		if err != nil {
			return nil, fmt.Errorf("failed to get current context: %w", err)
		}
		ctx = currentCtx
	}

	if store := fwdtui.GetStore(); store != nil {
		store.UnblockNamespace(req.Namespace, ctx)
	}

	key := req.ServiceName + "." + req.Namespace + "." + ctx
	if fwdsvcregistry.Get(key) != nil {
		return nil, fmt.Errorf("service %s is already being forwarded", key)
	}

	clientSet, _, _, err := nsManager.GetOrCreateClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get kubernetes client: %w", err)
	}

	svc, err := clientSet.CoreV1().Services(req.Namespace).Get(context.Background(), req.ServiceName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get service: %w", err)
	}

	if len(svc.Spec.Selector) == 0 {
		return nil, fmt.Errorf("service %s has no pod selector - kubefwd cannot forward services without selectors", req.ServiceName)
	}

	svcfwd, err := nsManager.CreateServiceFWD(ctx, req.Namespace, svc)
	if err != nil {
		return nil, fmt.Errorf("failed to create service forward: %w", err)
	}

	podAddedCh, unsubscribe := setupPodAddedSubscription(key)
	fwdsvcregistry.Add(svcfwd)

	localIP, hostnames := waitForPodAdded(podAddedCh, unsubscribe, key)

	return &types.AddServiceResponse{
		Key:         key,
		ServiceName: req.ServiceName,
		Namespace:   req.Namespace,
		Context:     ctx,
		LocalIP:     localIP,
		Hostnames:   hostnames,
		Ports:       buildPortMappings(svc),
	}, nil
}

// RemoveService stops forwarding a service
func (a *ServiceCRUDAdapter) RemoveService(key string) error {
	// Check if service exists
	if fwdsvcregistry.Get(key) == nil {
		return fmt.Errorf("service not found: %s", key)
	}

	// Remove from registry (this stops forwarding and cleans up)
	fwdsvcregistry.RemoveByName(key)

	return nil
}
