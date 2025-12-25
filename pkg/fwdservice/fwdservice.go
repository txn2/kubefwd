package fwdservice

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/txn2/kubefwd/pkg/fwdIp"
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
	RESTClient   restclient.RESTClient

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

// resetReconnectBackoff resets the backoff to initial value after successful connection.
func (svcFwd *ServiceFWD) resetReconnectBackoff() {
	svcFwd.reconnectMu.Lock()
	defer svcFwd.reconnectMu.Unlock()
	if svcFwd.reconnectBackoff != 0 {
		log.Debugf("Resetting reconnect backoff for service %s", svcFwd)
		svcFwd.reconnectBackoff = 0
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
		// Only include pods that are running/pending AND not marked for deletion
		if (pod.Status.Phase == v1.PodPending || pod.Status.Phase == v1.PodRunning) && pod.DeletionTimestamp == nil {
			podsEligible = append(podsEligible, pod)
		}
	}

	return podsEligible
}

// SyncPodForwards selects one or all pods behind a service, and invokes
// the forwarding setup for that or those pod(s). It will remove pods in-mem
// that are no longer returned by k8s, should these not be correctly deleted.
func (svcFwd *ServiceFWD) SyncPodForwards(force bool) {
	doSync := func() {
		log.Infof("SyncPodForwards starting for service %s (force=%v, currentForwards=%d)", svcFwd, force, len(svcFwd.PortForwards))
		k8sPods := svcFwd.GetPodsForService()
		log.Infof("SyncPodForwards: Found %d eligible pods for service %s", len(k8sPods), svcFwd)

		// If no pods are found currently, schedule a retry if configured.
		if len(k8sPods) == 0 {
			log.Warnf("WARNING: No Running Pods returned for service %s", svcFwd)
			// Schedule retry - don't update LastSyncedAt so we retry sooner
			if svcFwd.RetryInterval > 0 {
				go func() {
					select {
					case <-svcFwd.DoneChannel:
						return // Service is shutting down
					case <-time.After(svcFwd.RetryInterval):
						svcFwd.SyncPodForwards(false)
					}
				}()
			}
			return
		}

		// Only update LastSyncedAt after successful pod discovery
		defer func() { svcFwd.LastSyncedAt = time.Now() }()

		// Reset reconnect backoff on successful pod discovery
		svcFwd.resetReconnectBackoff()

		// Check if the pods currently being forwarded still exist in k8s and if
		// they are not in a (pre-)running state, if not: remove them
		for _, podName := range svcFwd.ListServicePodNames() {
			keep := false
			for _, pod := range k8sPods {
				if podName == pod.Name && (pod.Status.Phase == v1.PodPending || pod.Status.Phase == v1.PodRunning) && pod.DeletionTimestamp == nil {
					keep = true
					break
				}
			}
			if !keep {
				svcFwd.RemoveServicePod(podName)
			}
		}

		// Set up port-forwarding for one or all of these pods normal service
		// port-forward the first pod as service name. headless service not only
		// forward first Pod as service name, but also port-forward all pods.
		if len(k8sPods) != 0 {

			// if this is a headless service forward the first pod from the
			// service name, then subsequent pods from their pod name
			if svcFwd.Headless {
				svcFwd.LoopPodsToForward([]v1.Pod{k8sPods[0]}, false)
				svcFwd.LoopPodsToForward(k8sPods, true)
				return
			}

			// Check if currently we are forwarding a pod which is good to keep using
			podNameToKeep := ""
			for _, podName := range svcFwd.ListServicePodNames() {
				if podNameToKeep != "" {
					break
				}
				for _, pod := range k8sPods {
					if podName == pod.Name && (pod.Status.Phase == v1.PodPending || pod.Status.Phase == v1.PodRunning) && pod.DeletionTimestamp == nil {
						podNameToKeep = pod.Name
						break
					}
				}
			}

			// Stop forwarding others, should there be. In case none of the currently
			// forwarded pods are good to keep, podNameToKeep will be the empty string,
			// and the comparison will mean we will remove all pods, which is the desired behaviour.
			for _, podName := range svcFwd.ListServicePodNames() {
				if podName != podNameToKeep {
					svcFwd.RemoveServicePod(podName)
				}
			}

			// If no good pod was being forwarded already, start one
			if podNameToKeep == "" {
				svcFwd.LoopPodsToForward([]v1.Pod{k8sPods[0]}, false)
			}
		}
	}
	// When a whole set of pods gets deleted at once, they all will trigger a SyncPodForwards() call.
	// This would hammer k8s with load needlessly.  We therefore use a debouncer to only update pods
	// if things have been stable for at least a few seconds.  However, if things never stabilize we
	// will still reload this information at least once every resyncInterval (default 5 minutes).
	resyncInterval := svcFwd.ResyncInterval
	if resyncInterval == 0 {
		resyncInterval = 5 * time.Minute // default fallback
	}
	if force || time.Since(svcFwd.LastSyncedAt) > resyncInterval {
		// Replace current debounced function with no-op
		svcFwd.SyncDebouncer(func() {})

		// Do the syncing work
		doSync()
	} else {
		// Queue sync
		svcFwd.SyncDebouncer(doSync)
	}
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

		serviceHostName := svcFwd.Svc.Name
		svcName := svcFwd.Svc.Name

		if includePodNameInHost {
			serviceHostName = pod.Name + "." + svcFwd.Svc.Name
			svcName = pod.Name + "." + svcFwd.Svc.Name
		}

		opts := fwdIp.ForwardIPOpts{
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
		localIp, err := fwdnet.ReadyInterface(opts)
		if err != nil {
			log.Warnf("WARNING: error readying interface: %s\n", err)
		}

		// if this is not the first namespace on the
		// first cluster then append the namespace
		if svcFwd.NamespaceN > 0 {
			serviceHostName = serviceHostName + "." + pod.Namespace
		}

		// if this is not the first cluster append the full
		// host name
		if svcFwd.ClusterN > 0 {
			serviceHostName = serviceHostName + "." + svcFwd.Context
		}

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
				log.Fatal(err)
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
				localIp.String(),
				svcName,
			)

			log.Printf("Port-Forward: %16s %s:%d to pod %s:%s\n",
				localIp.String(),
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
				LocalIp:       localIp,
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
				// Check if shutdown was requested BEFORE we start
				// (ManualStopChan may also close during PortForward due to error cleanup,
				// but we should only skip reconnection if shutdown was requested externally)
				wasShuttingDown := false
				select {
				case <-pfo.ManualStopChan:
					wasShuttingDown = true
				default:
				}

				svcFwd.AddServicePod(pfo)
				err := pfo.PortForward()

				// Remove the forward from the map since it's no longer active
				// This allows SyncPodForwards to create a fresh forward on reconnection
				svcFwd.NamespaceServiceLock.Lock()
				servicePodKey := pfo.Service + "." + pfo.PodName + "." + pfo.LocalPort
				delete(svcFwd.PortForwards, servicePodKey)
				svcFwd.NamespaceServiceLock.Unlock()

				// If shutdown was already in progress before we started, exit cleanly
				if wasShuttingDown {
					return
				}

				// Log the error or unexpected stop
				if err != nil {
					log.Errorf("PortForward error on %s/%s: %s", pfo.Namespace, pfo.PodName, err.Error())
				} else {
					log.Warnf("Stopped forwarding pod %s for %s", pfo.PodName, svcFwd)
				}

				// Attempt auto-reconnection if enabled
				svcFwd.scheduleReconnect()
			}()

		}

	}
}

func (svcFwd *ServiceFWD) AddServicePod(pfo *fwdport.PortForwardOpts) {
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
	if isNew && fwdtui.IsEnabled() {
		event := events.NewPodEvent(
			events.PodAdded,
			pfo.Service,
			pfo.Namespace,
			pfo.Context,
			pfo.PodName,
			svcFwd.String(), // registryKey for reconnection lookup
		)
		event.LocalIP = pfo.LocalIp.String()
		event.LocalPort = pfo.LocalPort
		event.PodPort = pfo.PodPort
		event.ContainerName = pfo.ContainerName
		event.Hostnames = pfo.Hosts
		fwdtui.Emit(event)
	}
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
		// Remove from global informer if UID is set
		if pod.PodUID != "" {
			globalInformer := fwdport.GetGlobalPodInformer(svcFwd.ClientSet, svcFwd.Namespace)
			globalInformer.RemovePodByUID(pod.PodUID)
		}
		pod.Stop()
		<-pod.DoneChan
		svcFwd.NamespaceServiceLock.Lock()
		delete(svcFwd.PortForwards, servicePodName)
		svcFwd.NamespaceServiceLock.Unlock()

		// Emit event for TUI
		// Use pod (PortForwardOpts) values to match metrics key construction exactly
		// Pass svcFwd.String() as registryKey for proper registry lookup
		if fwdtui.IsEnabled() {
			fwdtui.Emit(events.NewPodEvent(
				events.PodRemoved,
				pod.Service,
				pod.Namespace,
				pod.Context,
				pod.PodName,
				svcFwd.String(), // registryKey for reconnection lookup
			))
		}
	}
}

func portSearch(portName string, containers []v1.Container) (port string, containerName string, found bool) {
	for _, container := range containers {
		for _, cp := range container.Ports {
			if cp.Name == portName {
				return fmt.Sprint(cp.ContainerPort), container.Name, true
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
				//use map port
				return portMapInfo.TargetPort
			}
		}
	}
	return p
}
