package fwdservice

import (
	"context"
	"strconv"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/txn2/kubefwd/pkg/fwdip"
	"github.com/txn2/kubefwd/pkg/fwdnet"
	"github.com/txn2/kubefwd/pkg/fwdport"
	"github.com/txn2/kubefwd/pkg/fwdpub"
	"github.com/txn2/kubefwd/pkg/fwdtui"
	"github.com/txn2/kubefwd/pkg/fwdtui/events"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	restclient "k8s.io/client-go/rest"
)

const (
	// Auto-reconnect backoff settings
	initialReconnectBackoff = 1 * time.Second
	maxReconnectBackoff     = 5 * time.Minute
)

// ServiceFWD Single service to forward, with a reference to
// all the pods being forwarded for it
type ServiceFWD struct {
	ClientSet    kubernetes.Interface
	ListOptions  metav1.ListOptions
	Hostfile     *fwdport.HostFileWithLock
	ClientConfig restclient.Config
	RESTClient   *restclient.RESTClient

	// Context is a unique key (string) in kubectl config representing
	// a user/cluster combination. Kubefwd uses context as the
	// cluster name when forwarding to more than one cluster.
	Context string

	// Namespace is the current Kubernetes Namespace to locate services
	// and the pods that back them for port-forwarding
	Namespace string

	// Timeout is specify a timeout seconds for the port forwarding.
	Timeout int

	// ClusterN is the ordinal index of the cluster (from configuration)
	// cluster 0 is considered local while > 0 is remote
	ClusterN int

	// NamespaceN is the ordinal index of the namespace from the
	// perspective of the user. Namespace 0 is considered local
	// while > 0 is an external namespace
	NamespaceN int

	// FwdInc the forward increment for ip
	FwdInc *int

	// Domain is specified by the user and used in place of .local
	Domain string

	PodLabelSelector     string      // The label selector to query for matching pods.
	NamespaceServiceLock *sync.Mutex //
	Svc                  *v1.Service // Reference to the k8s service.

	// Headless service will forward all of the pods,
	// while normally only a single pod is forwarded.
	Headless bool

	LastSyncedAt time.Time  // When was the set of pods last synced
	PortMap      *[]PortMap // port map array.

	// Use debouncer for listing pods so we don't hammer the k8s when a bunch of changes happen at once
	SyncDebouncer func(f func())

	// A mapping of all the pods currently being forwarded.
	// key = podName
	PortForwards map[string]*fwdport.PortForwardOpts
	DoneChannel  chan struct{} // After shutdown is complete, this channel will be closed

	ForwardConfigurationPath string   // file path to IP reservation configuration
	ForwardIPReservations    []string // cli passed IP reservations

	// ResyncInterval is the max time between forced resyncs
	ResyncInterval time.Duration

	// RetryInterval is how often to retry when no pods found
	RetryInterval time.Duration

	// AutoReconnect enables automatic reconnection with exponential backoff
	AutoReconnect bool

	// reconnectBackoff tracks the current reconnect wait time (internal state)
	reconnectBackoff time.Duration

	// reconnectMu protects reconnect state
	reconnectMu sync.Mutex

	// reconnecting prevents multiple simultaneous reconnect attempts
	reconnecting bool

	// noPodsLogged tracks if we've already logged "no pods" warning for this service
	// to avoid spamming logs with repeated warnings
	noPodsLogged bool
}

type PortMap struct {
	SourcePort string
	TargetPort string
}

// String representation of a ServiceFWD returns a unique name
// in the form SERVICE_NAME.NAMESPACE.CONTEXT
func (svcFwd *ServiceFWD) String() string {
	return svcFwd.Svc.Name + "." + svcFwd.Namespace + "." + svcFwd.Context
}

// scheduleReconnect schedules a reconnection attempt with exponential backoff.
// This is triggered by --auto-reconnect flag or user pressing 'r' in TUI when connections fail.
// Note: This complements (not replaces) the informer-based pod lifecycle handling from PR #296,
// which automatically detects pod deletion/recreation. This function handles application-level
// connection failures with backoff retry logic.
// Returns true if reconnect was scheduled, false if already reconnecting or shutting down.
func (svcFwd *ServiceFWD) scheduleReconnect() bool {
	if !svcFwd.AutoReconnect {
		return false
	}

	// Check if service is shutting down
	select {
	case <-svcFwd.DoneChannel:
		return false
	default:
	}

	svcFwd.reconnectMu.Lock()
	if svcFwd.reconnecting {
		svcFwd.reconnectMu.Unlock()
		log.Debugf("Already reconnecting for service %s, skipping", svcFwd)
		return false
	}
	svcFwd.reconnecting = true

	backoff := svcFwd.reconnectBackoff
	if backoff == 0 {
		backoff = initialReconnectBackoff
	}
	svcFwd.reconnectBackoff = backoff * 2
	if svcFwd.reconnectBackoff > maxReconnectBackoff {
		svcFwd.reconnectBackoff = maxReconnectBackoff
	}
	svcFwd.reconnectMu.Unlock()

	log.Infof("Scheduling reconnection for %s in %v", svcFwd, backoff)

	go func() {
		defer func() {
			svcFwd.reconnectMu.Lock()
			svcFwd.reconnecting = false
			svcFwd.reconnectMu.Unlock()
		}()

		select {
		case <-svcFwd.DoneChannel:
			return
		case <-time.After(backoff):
		}

		log.Infof("Attempting reconnection for service %s", svcFwd)
		// Close idle HTTP connections to force fresh TCP connections
		// This helps when previous connections timed out or are in a bad state
		svcFwd.CloseIdleHTTPConnections()
		svcFwd.SyncPodForwards(true) // force=true bypasses debouncing
	}()

	return true
}

// ResetReconnectBackoff resets the backoff to initial value after successful connection.
// This is called from PortForward() when the port forward is successfully established,
// not when pods are merely discovered. This prevents the backoff from resetting during
// pod transitions when connections immediately fail.
func (svcFwd *ServiceFWD) ResetReconnectBackoff() {
	svcFwd.reconnectMu.Lock()
	defer svcFwd.reconnectMu.Unlock()
	if svcFwd.reconnectBackoff != 0 {
		log.Debugf("Resetting reconnect backoff for service %s", svcFwd)
		svcFwd.reconnectBackoff = 0
	}
}

// ForceReconnect resets all reconnect state and triggers immediate reconnection.
// This is called when user presses 'r' in TUI to manually retry errored connections.
// Unlike scheduleReconnect(), this bypasses any pending backoff timers.
func (svcFwd *ServiceFWD) ForceReconnect() {
	svcFwd.reconnectMu.Lock()
	svcFwd.reconnectBackoff = 0
	svcFwd.reconnecting = false
	svcFwd.reconnectMu.Unlock()

	log.Infof("Force reconnecting service %s", svcFwd)

	// CRITICAL: Stop all existing port forwards and clear the map
	// After computer sleep/wake, port forwards may be stuck on dead TCP connections.
	// If we don't clear them, LoopPodsToForward will skip creating new ones
	// because it sees entries already exist in the PortForwards map.
	svcFwd.StopAllPortForwards()

	svcFwd.CloseIdleHTTPConnections()
	svcFwd.SyncPodForwards(true)
}

// StopAllPortForwards stops all active port forwards and clears the map.
// This does NOT wait for the stopped forwards to finish - they will clean up asynchronously.
// This is necessary for force reconnect because stuck forwards may never return.
func (svcFwd *ServiceFWD) StopAllPortForwards() {
	svcFwd.NamespaceServiceLock.Lock()
	// Get all forwards and clear the map immediately
	forwards := make([]*fwdport.PortForwardOpts, 0, len(svcFwd.PortForwards))
	for _, pfo := range svcFwd.PortForwards {
		forwards = append(forwards, pfo)
	}
	// Clear the map so LoopPodsToForward won't skip
	svcFwd.PortForwards = make(map[string]*fwdport.PortForwardOpts)
	svcFwd.NamespaceServiceLock.Unlock()

	log.Debugf("Stopping %d existing port forwards for %s", len(forwards), svcFwd)

	// Stop them asynchronously - don't wait for stuck goroutines
	// Emit PodRemoved events so TUI can clean up entries
	for _, pfo := range forwards {
		pfo.Stop()
		if fwdtui.EventsEnabled() {
			event := events.NewPodEvent(
				events.PodRemoved,
				pfo.Service,
				pfo.Namespace,
				pfo.Context,
				pfo.PodName,
				svcFwd.String(),
			)
			event.LocalPort = pfo.LocalPort
			fwdtui.Emit(event)
		}
	}
}

// CloseIdleHTTPConnections attempts to close idle HTTP connections in the k8s client transport.
// This helps when reconnecting after connection errors by forcing fresh TCP connections.
// It's a best-effort operation - if the transport doesn't support CloseIdleConnections, it's a no-op.
func (svcFwd *ServiceFWD) CloseIdleHTTPConnections() {
	// Try to get the HTTP transport from the ClientConfig
	// The WrapTransport field contains the transport wrapper if set
	if svcFwd.ClientConfig.Transport != nil {
		// If the transport implements CloseIdleConnections, call it
		type idleCloser interface {
			CloseIdleConnections()
		}
		if closer, ok := svcFwd.ClientConfig.Transport.(idleCloser); ok {
			closer.CloseIdleConnections()
			log.Debugf("Closed idle HTTP connections for service %s", svcFwd)
		}
	}
}

// isPodReady checks if a pod has at least one container in Ready state.
// A pod can be in Running phase but have containers that are not yet ready,
// which would cause port forwarding to fail.
func isPodReady(pod *v1.Pod) bool {
	// For Pending pods, we can't check readiness yet - let them through
	// so we can wait for them to become Running
	if pod.Status.Phase == v1.PodPending {
		return true
	}

	// For Running pods, check if at least one container is Ready
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.Ready {
			return true
		}
	}
	return false
}

// GetPodsForService queries k8s and returns all pods backing this service
// which are eligible for port-forwarding; exclude some pods which are in final/failure state.
func (svcFwd *ServiceFWD) GetPodsForService() []v1.Pod {
	listOpts := metav1.ListOptions{LabelSelector: svcFwd.PodLabelSelector}

	pods, err := svcFwd.ClientSet.CoreV1().Pods(svcFwd.Svc.Namespace).List(context.TODO(), listOpts)

	if err != nil {
		if errors.IsNotFound(err) {
			log.Warnf("WARNING: No Pods found for service %s: %s\n", svcFwd, err.Error())
		} else {
			log.Warnf("WARNING: Error in List pods for %s: %s\n", svcFwd, err.Error())
		}
		return nil
	}

	podsEligible := make([]v1.Pod, 0, len(pods.Items))

	for _, pod := range pods.Items {
		// Only include pods that are:
		// 1. Running or Pending phase
		// 2. Not marked for deletion
		// 3. Have at least one Ready container (for Running pods)
		if (pod.Status.Phase == v1.PodPending || pod.Status.Phase == v1.PodRunning) &&
			pod.DeletionTimestamp == nil &&
			isPodReady(&pod) {
			podsEligible = append(podsEligible, pod)
		}
	}

	return podsEligible
}

// forwardInfo holds forward key and pod name for sync operations
type forwardInfo struct {
	key     string
	podName string
}

// isPodEligible checks if a pod is eligible for forwarding
func isPodEligible(pod v1.Pod) bool {
	return (pod.Status.Phase == v1.PodPending || pod.Status.Phase == v1.PodRunning) && pod.DeletionTimestamp == nil
}

// scheduleRetry schedules a retry for pod sync if no pods were found
func (svcFwd *ServiceFWD) scheduleRetry() {
	if svcFwd.RetryInterval > 0 {
		go func() {
			select {
			case <-svcFwd.DoneChannel:
				return
			case <-time.After(svcFwd.RetryInterval):
				svcFwd.SyncPodForwards(false)
			}
		}()
	}
}

// getForwardInfos returns a snapshot of current forwards
func (svcFwd *ServiceFWD) getForwardInfos() []forwardInfo {
	svcFwd.NamespaceServiceLock.Lock()
	defer svcFwd.NamespaceServiceLock.Unlock()
	forwards := make([]forwardInfo, 0, len(svcFwd.PortForwards))
	for key, pfo := range svcFwd.PortForwards {
		forwards = append(forwards, forwardInfo{key: key, podName: pfo.PodName})
	}
	return forwards
}

// removeStaleForwards removes forwards for pods no longer in the eligible list
func (svcFwd *ServiceFWD) removeStaleForwards(k8sPods []v1.Pod) {
	forwards := svcFwd.getForwardInfos()
	for _, fwd := range forwards {
		if !svcFwd.podStillEligible(fwd.podName, k8sPods) {
			svcFwd.RemoveServicePod(fwd.key)
		}
	}
}

// podStillEligible checks if a pod name is in the eligible pods list
func (svcFwd *ServiceFWD) podStillEligible(podName string, k8sPods []v1.Pod) bool {
	for _, pod := range k8sPods {
		if podName == pod.Name && isPodEligible(pod) {
			return true
		}
	}
	return false
}

// findKeyToKeep finds a forward key for a pod that is still eligible
func (svcFwd *ServiceFWD) findKeyToKeep(k8sPods []v1.Pod) string {
	forwards := svcFwd.getForwardInfos()
	for _, fwd := range forwards {
		if svcFwd.podStillEligible(fwd.podName, k8sPods) {
			return fwd.key
		}
	}
	return ""
}

// syncHeadlessService syncs forwards for a headless service
func (svcFwd *ServiceFWD) syncHeadlessService(k8sPods []v1.Pod) {
	svcFwd.LoopPodsToForward([]v1.Pod{k8sPods[0]}, false)
	svcFwd.LoopPodsToForward(k8sPods, true)
}

// syncNormalService syncs forwards for a normal (non-headless) service
func (svcFwd *ServiceFWD) syncNormalService(k8sPods []v1.Pod) {
	keyToKeep := svcFwd.findKeyToKeep(k8sPods)

	// Remove forwards for pods we're not keeping
	forwards := svcFwd.getForwardInfos()
	for _, fwd := range forwards {
		if fwd.key != keyToKeep {
			svcFwd.RemoveServicePod(fwd.key)
		}
	}

	// Start new forward if needed
	if keyToKeep == "" {
		svcFwd.LoopPodsToForward([]v1.Pod{k8sPods[0]}, false)
	}
}

// doSyncPods performs the actual pod sync logic
func (svcFwd *ServiceFWD) doSyncPods(force bool) {
	if !svcFwd.noPodsLogged {
		log.Infof("SyncPodForwards starting for service %s (force=%v, currentForwards=%d)", svcFwd, force, len(svcFwd.PortForwards))
	}
	k8sPods := svcFwd.GetPodsForService()
	if !svcFwd.noPodsLogged || len(k8sPods) > 0 {
		log.Infof("SyncPodForwards: Found %d eligible pods for service %s", len(k8sPods), svcFwd)
	}

	if len(k8sPods) == 0 {
		if !svcFwd.noPodsLogged {
			log.Warnf("WARNING: No Running Pods returned for service %s", svcFwd)
			svcFwd.noPodsLogged = true
		}
		svcFwd.scheduleRetry()
		return
	}

	if svcFwd.noPodsLogged {
		log.Infof("Pods now available for service %s", svcFwd)
		svcFwd.noPodsLogged = false
	}

	defer func() { svcFwd.LastSyncedAt = time.Now() }()

	svcFwd.removeStaleForwards(k8sPods)

	if svcFwd.Headless {
		svcFwd.syncHeadlessService(k8sPods)
	} else {
		svcFwd.syncNormalService(k8sPods)
	}
}

// SyncPodForwards selects one or all pods behind a service, and invokes
// the forwarding setup for that or those pod(s). It will remove pods in-mem
// that are no longer returned by k8s, should these not be correctly deleted.
func (svcFwd *ServiceFWD) SyncPodForwards(force bool) {
	// When a whole set of pods gets deleted at once, they all will trigger a SyncPodForwards() call.
	// This would hammer k8s with load needlessly.  We therefore use a debouncer to only update pods
	// if things have been stable for at least a few seconds.  However, if things never stabilize we
	// will still reload this information at least once every resyncInterval (default 5 minutes).
	resyncInterval := svcFwd.ResyncInterval
	if resyncInterval == 0 {
		resyncInterval = 5 * time.Minute // default fallback
	}
	if force || time.Since(svcFwd.LastSyncedAt) > resyncInterval {
		svcFwd.SyncDebouncer(func() {})
		svcFwd.doSyncPods(force)
	} else {
		svcFwd.SyncDebouncer(func() { svcFwd.doSyncPods(force) })
	}
}

// buildServiceHostname constructs the service hostname based on cluster/namespace indices
func buildServiceHostname(baseName, namespace, ctx string, namespaceN, clusterN int) string {
	hostname := baseName
	if namespaceN > 0 {
		hostname = hostname + "." + namespace
	}
	if clusterN > 0 {
		hostname = hostname + "." + ctx
	}
	return hostname
}

// LoopPodsToForward starts the port-forwarding for each
// pod in the given list
func (svcFwd *ServiceFWD) LoopPodsToForward(pods []v1.Pod, includePodNameInHost bool) {
	publisher := &fwdpub.Publisher{
		PublisherName: "Services",
		Output:        false,
	}

	// Ip address handout is a critical section for synchronization,
	// use a lock which synchronizes inside each namespace.
	svcFwd.NamespaceServiceLock.Lock()
	defer svcFwd.NamespaceServiceLock.Unlock()

	for _, pod := range pods {
		podPort := ""

		svcName := svcFwd.Svc.Name
		if includePodNameInHost {
			svcName = pod.Name + "." + svcFwd.Svc.Name
		}

		opts := fwdip.ForwardIPOpts{
			ServiceName:              svcName,
			PodName:                  pod.Name,
			Context:                  svcFwd.Context,
			ClusterN:                 svcFwd.ClusterN,
			NamespaceN:               svcFwd.NamespaceN,
			Namespace:                svcFwd.Namespace,
			Port:                     podPort,
			ForwardConfigurationPath: svcFwd.ForwardConfigurationPath,
			ForwardIPReservations:    svcFwd.ForwardIPReservations,
		}
		localIP, err := fwdnet.ReadyInterface(opts)
		if err != nil {
			log.Warnf("WARNING: error readying interface: %s\n", err)
		}

		serviceHostName := buildServiceHostname(svcName, pod.Namespace, svcFwd.Context, svcFwd.NamespaceN, svcFwd.ClusterN)

		for _, port := range svcFwd.Svc.Spec.Ports {

			// Skip if pod port protocol is UDP - not supported in k8s port forwarding yet, see https://github.com/kubernetes/kubernetes/issues/47862
			if port.Protocol == v1.ProtocolUDP {
				log.Warnf("WARNING: Skipped Port-Forward for %s:%d to pod %s:%s - k8s port-forwarding doesn't support UDP protocol\n",
					serviceHostName,
					port.Port,
					pod.Name,
					port.TargetPort.String(),
				)
				continue
			}

			podPort = port.TargetPort.String()
			localPort := svcFwd.getPortMap(port.Port)
			p, err := strconv.ParseInt(localPort, 10, 32)
			if err != nil {
				log.Errorf("Failed to parse local port %q for service %s: %v", localPort, svcFwd.Svc.Name, err)
				continue
			}
			port.Port = int32(p)

			// Check if this specific pod+port combination is already being forwarded
			// Key format: "service.podname.localport"
			forwardKey := svcName + "." + pod.Name + "." + localPort
			if _, exists := svcFwd.PortForwards[forwardKey]; exists {
				log.Debugf("LoopPodsToForward: Forward already exists for %s, skipping", forwardKey)
				continue
			}

			// Determine which container owns this port (for log streaming)
			var containerName string
			if _, err := strconv.Atoi(podPort); err != nil {
				// Named port - search for container that has this named port
				if namedPodPort, container, ok := portSearch(podPort, pod.Spec.Containers); ok {
					podPort = namedPodPort
					containerName = container
				}
			} else {
				// Numeric port - find container that has this port
				podPortNum, _ := strconv.ParseInt(podPort, 10, 32)
				containerName = findContainerForPort(int32(podPortNum), pod.Spec.Containers)
			}

			log.Debugf("Resolving: %s to %s (%s)\n",
				serviceHostName,
				localIP.String(),
				svcName,
			)

			log.Printf("Port-Forward: %16s %s:%d to pod %s:%s\n",
				localIP.String(),
				serviceHostName,
				port.Port,
				pod.Name,
				podPort,
			)

			pfo := &fwdport.PortForwardOpts{
				Out:           publisher,
				Config:        svcFwd.ClientConfig,
				ClientSet:     svcFwd.ClientSet,
				RESTClient:    svcFwd.RESTClient,
				Context:       svcFwd.Context,
				Namespace:     pod.Namespace,
				Service:       svcName,
				ServiceFwd:    svcFwd,
				PodName:       pod.Name,
				PodPort:       podPort,
				ContainerName: containerName,
				LocalIP:       localIP,
				LocalPort:     localPort,
				HostFile:      svcFwd.Hostfile,
				ClusterN:      svcFwd.ClusterN,
				NamespaceN:    svcFwd.NamespaceN,
				Domain:        svcFwd.Domain,

				ManualStopChan: make(chan struct{}),
				DoneChan:       make(chan struct{}),
			}

			// Fire and forget. The stopping is done in the service.Shutdown() method.
			go func() {

				// AddServicePod returns false if this forward was already registered
				// (race condition with another goroutine). Exit early to prevent duplicates.
				if !svcFwd.AddServicePod(pfo) {
					log.Debugf("Forward already registered for %s.%s.%s, skipping duplicate", pfo.Service, pfo.PodName, pfo.LocalPort)
					return
				}

				err := pfo.PortForward()

				// Normal cleanup - remove from map and emit TUI event
				svcFwd.NamespaceServiceLock.Lock()
				servicePodKey := pfo.Service + "." + pfo.PodName + "." + pfo.LocalPort
				delete(svcFwd.PortForwards, servicePodKey)
				svcFwd.NamespaceServiceLock.Unlock()

				// Emit PodRemoved event so TUI can clean up the entry
				if fwdtui.EventsEnabled() {
					event := events.NewPodEvent(
						events.PodRemoved,
						pfo.Service,
						pfo.Namespace,
						pfo.Context,
						pfo.PodName,
						svcFwd.String(),
					)
					event.LocalPort = pfo.LocalPort
					fwdtui.Emit(event)
				}

				// If there was an error, we should try to reconnect
				// Note: PortForward() calls pfo.Stop() on error, so ManualStopChan
				// will be closed - we can't use it to distinguish manual stop from error.
				if err != nil {
					log.Errorf("PortForward error on %s/%s: %s", pfo.Namespace, pfo.PodName, err.Error())
					// Attempt auto-reconnection if enabled
					svcFwd.scheduleReconnect()
					return
				}

				// No error means it was a clean stop (manual stop or shutdown)
				// Check if service is shutting down
				select {
				case <-svcFwd.DoneChannel:
					// Service is shutting down, don't reconnect
					return
				default:
				}

				// Stopped without error but service not shutting down - unexpected
				log.Warnf("Stopped forwarding pod %s for %s", pfo.PodName, svcFwd)
				svcFwd.scheduleReconnect()
			}()

		}

	}
}

// AddServicePod registers a port forward in the service's map and emits a TUI event.
// Returns true if this is a new forward, false if it was already registered (duplicate).
// Callers should check the return value and avoid starting duplicate port forwards.
func (svcFwd *ServiceFWD) AddServicePod(pfo *fwdport.PortForwardOpts) bool {
	svcFwd.NamespaceServiceLock.Lock()
	ServicePod := pfo.Service + "." + pfo.PodName + "." + pfo.LocalPort
	isNew := false
	if _, found := svcFwd.PortForwards[ServicePod]; !found {
		svcFwd.PortForwards[ServicePod] = pfo
		isNew = true
	}
	svcFwd.NamespaceServiceLock.Unlock()

	// Emit event for TUI if this is a new pod
	// Use pfo values to match metrics key construction exactly
	// Pass svcFwd.String() as registryKey for proper registry lookup
	if isNew && fwdtui.EventsEnabled() {
		event := events.NewPodEvent(
			events.PodAdded,
			pfo.Service,
			pfo.Namespace,
			pfo.Context,
			pfo.PodName,
			svcFwd.String(), // registryKey for reconnection lookup
		)
		event.LocalIP = pfo.LocalIP.String()
		event.LocalPort = pfo.LocalPort
		event.PodPort = pfo.PodPort
		event.ContainerName = pfo.ContainerName
		event.Hostnames = pfo.Hosts
		fwdtui.Emit(event)
	}

	return isNew
}

func (svcFwd *ServiceFWD) ListServicePodNames() []string {
	svcFwd.NamespaceServiceLock.Lock()
	currentPodNames := make([]string, 0, len(svcFwd.PortForwards))
	for podName := range svcFwd.PortForwards {
		currentPodNames = append(currentPodNames, podName)
	}
	svcFwd.NamespaceServiceLock.Unlock()
	return currentPodNames
}

func (svcFwd *ServiceFWD) RemoveServicePod(servicePodName string) {
	svcFwd.NamespaceServiceLock.Lock()
	pod, found := svcFwd.PortForwards[servicePodName]
	svcFwd.NamespaceServiceLock.Unlock()

	if found {
		// Remove this specific pfo from global informer (not all pfos for this pod UID,
		// as other services may still be forwarding to the same pod)
		if pod.PodUID != "" {
			globalInformer := fwdport.GetGlobalPodInformer(svcFwd.ClientSet, svcFwd.Namespace)
			globalInformer.RemovePfo(pod)
		}
		pod.Stop()
		<-pod.DoneChan
		svcFwd.NamespaceServiceLock.Lock()
		delete(svcFwd.PortForwards, servicePodName)
		svcFwd.NamespaceServiceLock.Unlock()

		// Emit event for TUI
		// Use pod (PortForwardOpts) values to match metrics key construction exactly
		// Pass svcFwd.String() as registryKey for proper registry lookup
		if fwdtui.EventsEnabled() {
			event := events.NewPodEvent(
				events.PodRemoved,
				pod.Service,
				pod.Namespace,
				pod.Context,
				pod.PodName,
				svcFwd.String(), // registryKey for reconnection lookup
			)
			event.LocalPort = pod.LocalPort
			fwdtui.Emit(event)
		}
	}
}

func portSearch(portName string, containers []v1.Container) (port string, containerName string, found bool) {
	for _, container := range containers {
		for _, cp := range container.Ports {
			if cp.Name == portName {
				return strconv.FormatInt(int64(cp.ContainerPort), 10), container.Name, true
			}
		}
	}

	return "", "", false
}

// findContainerForPort finds the container that owns a given port number
// Returns first container if port not found in any container spec
func findContainerForPort(port int32, containers []v1.Container) string {
	for _, container := range containers {
		for _, cp := range container.Ports {
			if cp.ContainerPort == port {
				return container.Name
			}
		}
	}
	// Default to first container
	if len(containers) > 0 {
		return containers[0].Name
	}
	return ""
}

// port exist port map return
func (svcFwd *ServiceFWD) getPortMap(port int32) string {
	p := strconv.Itoa(int(port))
	if svcFwd.PortMap != nil {
		for _, portMapInfo := range *svcFwd.PortMap {
			if p == portMapInfo.SourcePort {
				// use map port
				return portMapInfo.TargetPort
			}
		}
	}
	return p
}
