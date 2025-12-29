package fwdapi

import (
	"context"
	"fmt"
	"sync"
	"time"

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
		if svc.ActiveCount > 0 && svc.ErrorCount == 0 {
			active++
		} else if svc.ErrorCount > 0 && svc.ActiveCount == 0 {
			errored++
		} else if svc.ErrorCount > 0 && svc.ActiveCount > 0 {
			partial++
		} else {
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
	status := "pending"
	if svc.ActiveCount > 0 && svc.ErrorCount == 0 {
		status = "active"
	} else if svc.ErrorCount > 0 && svc.ActiveCount == 0 {
		status = "error"
	} else if svc.ErrorCount > 0 && svc.ActiveCount > 0 {
		status = "partial"
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
	// Calculate connection state
	connState := "disconnected"
	switch fwd.Status {
	case state.StatusActive:
		connState = "connected"
	case state.StatusConnecting:
		connState = "connecting"
	case state.StatusError:
		connState = "error"
	}

	// Calculate uptime and idle duration
	var uptime, idleDuration string
	if !fwd.StartedAt.IsZero() {
		uptime = time.Now().Sub(fwd.StartedAt).Round(1e9).String()
	}
	if !fwd.LastActive.IsZero() {
		idleDuration = time.Now().Sub(fwd.LastActive).Round(1e9).String()
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

	// List namespaces from the cluster
	nsList, err := clientSet.CoreV1().Namespaces().List(context.Background(), metav1.ListOptions{})
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

	// List services from the cluster
	svcList, err := clientSet.CoreV1().Services(namespace).List(context.Background(), metav1.ListOptions{})
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

	svc, err := clientSet.CoreV1().Services(namespace).Get(context.Background(), name, metav1.GetOptions{})
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
