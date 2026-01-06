// Package fwdns provides dynamic namespace watcher management for kubefwd.
// It enables runtime addition and removal of namespace watchers, supporting
// dynamic control of which namespaces are being forwarded.
package fwdns

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/bep/debounce"
	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"

	"github.com/txn2/kubefwd/pkg/fwdcfg"
	"github.com/txn2/kubefwd/pkg/fwdport"
	"github.com/txn2/kubefwd/pkg/fwdservice"
	"github.com/txn2/kubefwd/pkg/fwdsvcregistry"
	"github.com/txn2/kubefwd/pkg/fwdtui"
	"github.com/txn2/kubefwd/pkg/fwdtui/events"
)

// WatcherKey creates a unique key for a namespace/context combination
func WatcherKey(namespace, k8sContext string) string {
	return namespace + "." + k8sContext
}

// NamespaceManager manages the lifecycle of namespace watchers.
// It supports dynamic addition and removal of namespace watchers at runtime.
type NamespaceManager struct {
	mu       sync.RWMutex
	watchers map[string]*NamespaceWatcher // key: "namespace.context"

	// Shared resources
	hostFile   *fwdport.HostFileWithLock
	configPath string

	// Configuration for new watchers
	domain          string
	portMapping     []string
	timeout         int
	fwdConfigPath   string
	fwdReservations []string
	resyncInterval  time.Duration
	retryInterval   time.Duration
	autoReconnect   bool
	labelSelector   string
	fieldSelector   string

	// Kubernetes config
	configGetter *fwdcfg.ConfigGetter
	clientSets   map[string]kubernetes.Interface   // context -> clientSet
	restClients  map[string]*restclient.RESTClient // context -> restClient
	restConfigs  map[string]*restclient.Config     // context -> restConfig

	// Counters for IP allocation
	contextIndex   map[string]int // context -> cluster index
	namespaceIndex map[string]int // namespace.context -> namespace index

	// Global stop channel (from main)
	globalStopCh <-chan struct{}

	// Done channel signals all watchers stopped
	doneCh chan struct{}

	// Ad-hoc IP locks for services added without namespace watchers
	adHocIPLocks   map[string]*sync.Mutex // key: namespace.context -> lock
	adHocIPLocksMu sync.RWMutex
}

// NamespaceInfo provides information about a watched namespace
type NamespaceInfo struct {
	Key           string    `json:"key"`
	Namespace     string    `json:"namespace"`
	Context       string    `json:"context"`
	ServiceCount  int       `json:"serviceCount"`
	ActiveCount   int       `json:"activeCount"`
	ErrorCount    int       `json:"errorCount"`
	StartedAt     time.Time `json:"startedAt"`
	Running       bool      `json:"running"`
	LabelSelector string    `json:"labelSelector,omitempty"`
	FieldSelector string    `json:"fieldSelector,omitempty"`
}

// WatcherOpts configures a namespace watcher
type WatcherOpts struct {
	LabelSelector string
	FieldSelector string
}

// ManagerConfig configures the NamespaceManager
type ManagerConfig struct {
	HostFile        *fwdport.HostFileWithLock
	ConfigPath      string
	Domain          string
	PortMapping     []string
	Timeout         int
	FwdConfigPath   string
	FwdReservations []string
	ResyncInterval  time.Duration
	RetryInterval   time.Duration
	AutoReconnect   bool
	LabelSelector   string
	FieldSelector   string
	GlobalStopCh    <-chan struct{}
}

// NewManager creates a new NamespaceManager
func NewManager(cfg ManagerConfig) *NamespaceManager {
	return &NamespaceManager{
		watchers:        make(map[string]*NamespaceWatcher),
		hostFile:        cfg.HostFile,
		configPath:      cfg.ConfigPath,
		domain:          cfg.Domain,
		portMapping:     cfg.PortMapping,
		timeout:         cfg.Timeout,
		fwdConfigPath:   cfg.FwdConfigPath,
		fwdReservations: cfg.FwdReservations,
		resyncInterval:  cfg.ResyncInterval,
		retryInterval:   cfg.RetryInterval,
		autoReconnect:   cfg.AutoReconnect,
		labelSelector:   cfg.LabelSelector,
		fieldSelector:   cfg.FieldSelector,
		configGetter:    fwdcfg.NewConfigGetter(),
		clientSets:      make(map[string]kubernetes.Interface),
		restClients:     make(map[string]*restclient.RESTClient),
		restConfigs:     make(map[string]*restclient.Config),
		contextIndex:    make(map[string]int),
		namespaceIndex:  make(map[string]int),
		globalStopCh:    cfg.GlobalStopCh,
		doneCh:          make(chan struct{}),
		adHocIPLocks:    make(map[string]*sync.Mutex),
	}
}

// StartWatcher starts a namespace watcher for the given context and namespace
func (m *NamespaceManager) StartWatcher(ctx, namespace string, opts WatcherOpts) (*NamespaceInfo, error) {
	// If no context specified, use current context
	if ctx == "" {
		currentCtx, err := m.configGetter.GetCurrentContext(m.configPath)
		if err != nil {
			return nil, fmt.Errorf("failed to get current context: %w", err)
		}
		ctx = currentCtx
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	key := WatcherKey(namespace, ctx)

	// Unblock this namespace in case it was previously removed
	// This allows AddForward to work again for this namespace
	if store := fwdtui.GetStore(); store != nil {
		store.UnblockNamespace(namespace, ctx)
	}

	// Check if already watching
	if existing, ok := m.watchers[key]; ok && existing.Running() {
		return existing.Info(), fmt.Errorf("already watching namespace %s in context %s", namespace, ctx)
	}

	// Get or create clientSet for context
	clientSet, restConfig, restClient, err := m.getOrCreateClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client for context %s: %w", ctx, err)
	}

	// Allocate indices for IP addressing
	clusterN := m.getContextIndex(ctx)
	namespaceN := m.getNamespaceIndex(key)

	// Merge selectors (watcher opts override defaults)
	labelSelector := m.labelSelector
	if opts.LabelSelector != "" {
		labelSelector = opts.LabelSelector
	}
	fieldSelector := m.fieldSelector
	if opts.FieldSelector != "" {
		fieldSelector = opts.FieldSelector
	}

	// Create watcher
	watcher := &NamespaceWatcher{
		manager:       m,
		key:           key,
		namespace:     namespace,
		context:       ctx,
		clientSet:     clientSet,
		restConfig:    restConfig,
		restClient:    restClient,
		clusterN:      clusterN,
		namespaceN:    namespaceN,
		labelSelector: labelSelector,
		fieldSelector: fieldSelector,
		stopCh:        make(chan struct{}),
		doneCh:        make(chan struct{}),
		ipLock:        &sync.Mutex{},
		startedAt:     time.Now(),
	}

	// Set running with mutex for proper memory visibility
	watcher.runningMu.Lock()
	watcher.running = true
	watcher.runningMu.Unlock()

	m.watchers[key] = watcher

	// Start watching in background
	go watcher.Run()

	return watcher.Info(), nil
}

// StopWatcher stops the namespace watcher for the given context and namespace
func (m *NamespaceManager) StopWatcher(ctx, namespace string) error {
	// If no context specified, use current context
	if ctx == "" {
		currentCtx, err := m.configGetter.GetCurrentContext(m.configPath)
		if err != nil {
			return fmt.Errorf("failed to get current context: %w", err)
		}
		ctx = currentCtx
	}

	m.mu.Lock()
	key := WatcherKey(namespace, ctx)
	watcher, ok := m.watchers[key]
	m.mu.Unlock()

	if !ok {
		return fmt.Errorf("no watcher found for namespace %s in context %s", namespace, ctx)
	}

	// IMPORTANT: Block this namespace in the state store FIRST, before stopping.
	// This prevents race condition where in-flight port forward events re-add entries.
	// Must be done synchronously (not via event) to ensure block is set before any events.
	if store := fwdtui.GetStore(); store != nil {
		store.RemoveByNamespace(namespace, ctx)
	}

	// Stop the watcher
	watcher.Stop()

	// Wait for it to finish
	<-watcher.Done()

	// Remove all services for this namespace from the registry
	m.removeNamespaceServices(namespace, ctx)

	// Also emit event for any other listeners (TUI update, etc.)
	if fwdtui.EventsEnabled() {
		fwdtui.Emit(events.NewNamespaceRemovedEvent(namespace, ctx))
	}

	// Remove from map
	m.mu.Lock()
	delete(m.watchers, key)
	m.mu.Unlock()

	return nil
}

// ListWatchers returns information about all active watchers
func (m *NamespaceManager) ListWatchers() []NamespaceInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]NamespaceInfo, 0, len(m.watchers))
	for _, w := range m.watchers {
		result = append(result, *w.Info())
	}
	return result
}

// GetWatcher returns the watcher for a given key
func (m *NamespaceManager) GetWatcher(ctx, namespace string) *NamespaceWatcher {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.watchers[WatcherKey(namespace, ctx)]
}

// GetWatcherByKey returns the watcher for a given key
func (m *NamespaceManager) GetWatcherByKey(key string) *NamespaceWatcher {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.watchers[key]
}

// Done returns a channel that closes when all watchers have stopped
func (m *NamespaceManager) Done() <-chan struct{} {
	return m.doneCh
}

// StopAll stops all watchers
func (m *NamespaceManager) StopAll() {
	m.mu.RLock()
	watcherList := make([]*NamespaceWatcher, 0, len(m.watchers))
	for _, w := range m.watchers {
		watcherList = append(watcherList, w)
	}
	m.mu.RUnlock()

	// Stop all watchers in parallel
	var wg sync.WaitGroup
	for _, w := range watcherList {
		wg.Add(1)
		go func(watcher *NamespaceWatcher) {
			defer wg.Done()
			watcher.Stop()
			<-watcher.Done()
		}(w)
	}
	wg.Wait()

	close(m.doneCh)
}

// GetClientSet returns the clientSet for a context
func (m *NamespaceManager) GetClientSet(ctx string) kubernetes.Interface {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.clientSets[ctx]
}

// SetClientSet sets a clientSet for a context (for testing)
func (m *NamespaceManager) SetClientSet(ctx string, clientSet kubernetes.Interface) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.clientSets[ctx] = clientSet
}

// getOrCreateClient gets or creates kubernetes clients for a context
func (m *NamespaceManager) getOrCreateClient(ctx string) (kubernetes.Interface, *restclient.Config, *restclient.RESTClient, error) {
	// Check cache first (already holding lock)
	if cs, ok := m.clientSets[ctx]; ok {
		return cs, m.restConfigs[ctx], m.restClients[ctx], nil
	}

	// Create REST config
	restConfig, err := m.configGetter.GetRestConfig(m.configPath, ctx)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get REST config: %w", err)
	}

	// Create clientSet
	clientSet, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create clientSet: %w", err)
	}

	// Create REST client
	restClient, err := m.configGetter.GetRESTClient()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create REST client: %w", err)
	}

	// Cache
	m.clientSets[ctx] = clientSet
	m.restConfigs[ctx] = restConfig
	m.restClients[ctx] = restClient

	return clientSet, restConfig, restClient, nil
}

// getContextIndex returns a unique index for a context (for IP allocation)
func (m *NamespaceManager) getContextIndex(ctx string) int {
	if idx, ok := m.contextIndex[ctx]; ok {
		return idx
	}
	idx := len(m.contextIndex)
	m.contextIndex[ctx] = idx
	return idx
}

// getNamespaceIndex returns a unique index for a namespace.context (for IP allocation)
func (m *NamespaceManager) getNamespaceIndex(key string) int {
	if idx, ok := m.namespaceIndex[key]; ok {
		return idx
	}
	idx := len(m.namespaceIndex)
	m.namespaceIndex[key] = idx
	return idx
}

// removeNamespaceServices removes all services for a namespace from the registry
func (m *NamespaceManager) removeNamespaceServices(namespace, ctx string) {
	// Get all service keys for this namespace
	services := fwdsvcregistry.GetAll()
	removedCount := 0
	log.Debugf("removeNamespaceServices: looking for services in namespace=%s, context=%s (total services: %d)",
		namespace, ctx, len(services))

	for _, svc := range services {
		if svc.Namespace == namespace && svc.Context == ctx {
			key := svc.Svc.Name + "." + svc.Namespace + "." + svc.Context
			log.Debugf("removeNamespaceServices: removing service %s", key)
			fwdsvcregistry.RemoveByName(key)
			removedCount++
		}
	}

	log.Debugf("removeNamespaceServices: removed %d services for %s.%s", removedCount, namespace, ctx)
}

// GetOrCreateClient gets or creates kubernetes clients for a context (public wrapper)
func (m *NamespaceManager) GetOrCreateClient(ctx string) (kubernetes.Interface, *restclient.Config, *restclient.RESTClient, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.getOrCreateClient(ctx)
}

// GetContextIndex returns a unique index for a context (for IP allocation)
func (m *NamespaceManager) GetContextIndex(ctx string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.getContextIndex(ctx)
}

// GetNamespaceIndex returns a unique index for a namespace.context key (for IP allocation)
func (m *NamespaceManager) GetNamespaceIndex(key string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.getNamespaceIndex(key)
}

// GetOrCreateIPLock returns an IP lock for the given namespace/context.
// If a watcher exists for this namespace, returns its lock.
// Otherwise, creates/returns an ad-hoc lock for services added without watchers.
func (m *NamespaceManager) GetOrCreateIPLock(ctx, namespace string) *sync.Mutex {
	key := WatcherKey(namespace, ctx)

	// Check if watcher exists
	m.mu.RLock()
	if watcher, ok := m.watchers[key]; ok {
		m.mu.RUnlock()
		return watcher.ipLock
	}
	m.mu.RUnlock()

	// Create/get ad-hoc lock
	m.adHocIPLocksMu.Lock()
	defer m.adHocIPLocksMu.Unlock()

	if lock, ok := m.adHocIPLocks[key]; ok {
		return lock
	}

	lock := &sync.Mutex{}
	m.adHocIPLocks[key] = lock
	return lock
}

// GetCurrentContext returns the current kubernetes context
func (m *NamespaceManager) GetCurrentContext() (string, error) {
	return m.configGetter.GetCurrentContext(m.configPath)
}

// CreateServiceFWD creates a ServiceFWD for a specific service without a namespace watcher.
// The caller must add it to fwdsvcregistry.Add() to start forwarding.
func (m *NamespaceManager) CreateServiceFWD(ctx, namespace string, svc *v1.Service) (*fwdservice.ServiceFWD, error) {
	// Validate selector
	selector := labels.Set(svc.Spec.Selector).AsSelector().String()
	if selector == "" {
		return nil, fmt.Errorf("service %s.%s has no pod selector", svc.Name, namespace)
	}

	// If no context specified, use current context
	if ctx == "" {
		currentCtx, err := m.configGetter.GetCurrentContext(m.configPath)
		if err != nil {
			return nil, fmt.Errorf("failed to get current context: %w", err)
		}
		ctx = currentCtx
	}

	// Get or create K8s clients for context
	clientSet, restConfig, restClient, err := m.GetOrCreateClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client for context %s: %w", ctx, err)
	}

	// Get indices for IP allocation
	key := WatcherKey(namespace, ctx)
	clusterN := m.GetContextIndex(ctx)
	namespaceN := m.GetNamespaceIndex(key)

	// Get or create IP lock for this namespace
	ipLock := m.GetOrCreateIPLock(ctx, namespace)

	// Create ServiceFWD
	svcfwd := &fwdservice.ServiceFWD{
		ClientSet:                clientSet,
		Context:                  ctx,
		Namespace:                namespace,
		Timeout:                  m.timeout,
		Hostfile:                 m.hostFile,
		ClientConfig:             *restConfig,
		RESTClient:               restClient,
		NamespaceN:               namespaceN,
		ClusterN:                 clusterN,
		Domain:                   m.domain,
		PodLabelSelector:         selector,
		NamespaceServiceLock:     ipLock,
		Svc:                      svc,
		Headless:                 svc.Spec.ClusterIP == "None",
		PortForwards:             make(map[string]*fwdport.PortForwardOpts),
		SyncDebouncer:            debounce.New(5 * time.Second),
		DoneChannel:              make(chan struct{}),
		PortMap:                  parsePortMapPublic(m.portMapping),
		ForwardConfigurationPath: m.fwdConfigPath,
		ForwardIPReservations:    m.fwdReservations,
		ResyncInterval:           m.resyncInterval,
		RetryInterval:            m.retryInterval,
		AutoReconnect:            m.autoReconnect,
	}

	return svcfwd, nil
}

// parsePortMapPublic converts string mappings to PortMap slice (package-level helper)
func parsePortMapPublic(mappings []string) *[]fwdservice.PortMap {
	if len(mappings) == 0 {
		return nil
	}
	var portList []fwdservice.PortMap
	for _, s := range mappings {
		parts := splitPortMapping(s)
		if len(parts) == 2 {
			portList = append(portList, fwdservice.PortMap{
				SourcePort: parts[0],
				TargetPort: parts[1],
			})
		}
	}
	return &portList
}

// NamespaceWatcher watches a single namespace for service events
type NamespaceWatcher struct {
	manager    *NamespaceManager
	key        string
	namespace  string
	context    string
	clientSet  kubernetes.Interface
	restConfig *restclient.Config
	restClient *restclient.RESTClient
	clusterN   int
	namespaceN int

	labelSelector string
	fieldSelector string

	controller cache.Controller
	stopCh     chan struct{}
	doneCh     chan struct{}
	ipLock     *sync.Mutex

	startedAt time.Time
	running   bool
	runningMu sync.RWMutex
}

// Run starts the namespace watcher (blocking)
// Note: running is set to true in StartWatcher() before this goroutine starts
func (w *NamespaceWatcher) Run() {
	defer close(w.doneCh)

	defer func() {
		w.runningMu.Lock()
		w.running = false
		w.runningMu.Unlock()
	}()

	log.Infof("Starting watcher for namespace %s in context %s", w.namespace, w.context)

	// Apply filtering
	optionsModifier := func(options *metav1.ListOptions) {
		options.FieldSelector = w.fieldSelector
		options.LabelSelector = w.labelSelector
	}

	// Create informer
	_, controller := cache.NewInformerWithOptions(cache.InformerOptions{
		ListerWatcher: &cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				optionsModifier(&options)
				return w.clientSet.CoreV1().Services(w.namespace).List(context.TODO(), options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				options.Watch = true
				optionsModifier(&options)
				return w.clientSet.CoreV1().Services(w.namespace).Watch(context.TODO(), options)
			},
		},
		ObjectType:   &v1.Service{},
		ResyncPeriod: 0,
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc:    w.addServiceHandler,
			DeleteFunc: w.deleteServiceHandler,
			UpdateFunc: w.updateServiceHandler,
		},
	})

	w.controller = controller

	// Listen for global stop or local stop
	stopCh := make(chan struct{})
	go func() {
		select {
		case <-w.stopCh:
			close(stopCh)
		case <-w.manager.globalStopCh:
			close(stopCh)
		}
	}()

	// Run the controller (blocking)
	controller.Run(stopCh)

	log.Infof("Stopped watcher for namespace %s in context %s", w.namespace, w.context)
}

// Stop signals the watcher to stop
func (w *NamespaceWatcher) Stop() {
	select {
	case <-w.stopCh:
		// Already stopped
	default:
		close(w.stopCh)
	}
}

// Done returns a channel that closes when the watcher stops
func (w *NamespaceWatcher) Done() <-chan struct{} {
	return w.doneCh
}

// Running returns true if the watcher is currently running
func (w *NamespaceWatcher) Running() bool {
	w.runningMu.RLock()
	defer w.runningMu.RUnlock()
	return w.running
}

// Info returns information about this watcher
func (w *NamespaceWatcher) Info() *NamespaceInfo {
	// Count services for this namespace
	var serviceCount, activeCount, errorCount int
	services := fwdsvcregistry.GetAll()
	for _, svc := range services {
		if svc.Namespace == w.namespace && svc.Context == w.context {
			serviceCount++
			// Count active/error status (would need to check port forwards)
			// For now, count all as active if they exist
			activeCount++
		}
	}

	return &NamespaceInfo{
		Key:           w.key,
		Namespace:     w.namespace,
		Context:       w.context,
		ServiceCount:  serviceCount,
		ActiveCount:   activeCount,
		ErrorCount:    errorCount,
		StartedAt:     w.startedAt,
		Running:       w.Running(),
		LabelSelector: w.labelSelector,
		FieldSelector: w.fieldSelector,
	}
}

// addServiceHandler handles new service events
func (w *NamespaceWatcher) addServiceHandler(obj interface{}) {
	svc, ok := obj.(*v1.Service)
	if !ok {
		return
	}

	// Check if service has a valid config to do forwarding
	selector := labels.Set(svc.Spec.Selector).AsSelector().String()
	if selector == "" {
		log.Warnf("WARNING: No Pod selector for service %s.%s, skipping", svc.Name, svc.Namespace)
		return
	}

	// Define a service to forward
	svcfwd := &fwdservice.ServiceFWD{
		ClientSet:                w.clientSet,
		Context:                  w.context,
		Namespace:                w.namespace,
		Timeout:                  w.manager.timeout,
		Hostfile:                 w.manager.hostFile,
		ClientConfig:             *w.restConfig,
		RESTClient:               w.restClient,
		NamespaceN:               w.namespaceN,
		ClusterN:                 w.clusterN,
		Domain:                   w.manager.domain,
		PodLabelSelector:         selector,
		NamespaceServiceLock:     w.ipLock,
		Svc:                      svc,
		Headless:                 svc.Spec.ClusterIP == "None",
		PortForwards:             make(map[string]*fwdport.PortForwardOpts),
		SyncDebouncer:            debounce.New(5 * time.Second),
		DoneChannel:              make(chan struct{}),
		PortMap:                  w.parsePortMap(w.manager.portMapping),
		ForwardConfigurationPath: w.manager.fwdConfigPath,
		ForwardIPReservations:    w.manager.fwdReservations,
		ResyncInterval:           w.manager.resyncInterval,
		RetryInterval:            w.manager.retryInterval,
		AutoReconnect:            w.manager.autoReconnect,
	}

	// Add to registry
	fwdsvcregistry.Add(svcfwd)
}

// deleteServiceHandler handles service deletion events
func (w *NamespaceWatcher) deleteServiceHandler(obj interface{}) {
	svc, ok := obj.(*v1.Service)
	if !ok {
		return
	}

	// Remove from registry
	fwdsvcregistry.RemoveByName(svc.Name + "." + svc.Namespace + "." + w.context)
}

// updateServiceHandler handles service update events
func (w *NamespaceWatcher) updateServiceHandler(oldObj interface{}, newObj interface{}) {
	oldSvc, oldOk := oldObj.(*v1.Service)
	newSvc, newOk := newObj.(*v1.Service)

	if !oldOk || !newOk {
		return
	}

	// Check if selector or ports changed
	oldSelector := labels.Set(oldSvc.Spec.Selector).AsSelector().String()
	newSelector := labels.Set(newSvc.Spec.Selector).AsSelector().String()
	selectorChanged := oldSelector != newSelector
	portsChanged := len(oldSvc.Spec.Ports) != len(newSvc.Spec.Ports)

	if selectorChanged || portsChanged {
		key := newSvc.Name + "." + newSvc.Namespace + "." + w.context
		log.Infof("Service %s updated (selector=%v, ports=%v), triggering resync",
			key, selectorChanged, portsChanged)

		// Find and resync the service
		if svcfwd := fwdsvcregistry.Get(key); svcfwd != nil {
			svcfwd.SyncPodForwards(true) // force=true
		}
	}
}

// parsePortMap converts string mappings to PortMap slice
func (w *NamespaceWatcher) parsePortMap(mappings []string) *[]fwdservice.PortMap {
	if len(mappings) == 0 {
		return nil
	}
	var portList []fwdservice.PortMap
	for _, s := range mappings {
		parts := splitPortMapping(s)
		if len(parts) == 2 {
			portList = append(portList, fwdservice.PortMap{
				SourcePort: parts[0],
				TargetPort: parts[1],
			})
		}
	}
	return &portList
}

// splitPortMapping splits a port mapping string "source:target"
func splitPortMapping(s string) []string {
	for i := 0; i < len(s); i++ {
		if s[i] == ':' {
			return []string{s[:i], s[i+1:]}
		}
	}
	return nil
}
