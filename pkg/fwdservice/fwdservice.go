package fwdservice

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/txn2/kubefwd/pkg/fwdIp"
	"github.com/txn2/kubefwd/pkg/fwdnet"
	"github.com/txn2/kubefwd/pkg/fwdport"
	"github.com/txn2/kubefwd/pkg/fwdpub"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	restclient "k8s.io/client-go/rest"
)

// ServiceFWD Single service to forward, with a reference to
// all the pods being forwarded for it
type ServiceFWD struct {
	ClientSet    kubernetes.Clientset
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
	HostLocalServices        []string // cli passed locally hosted services
}

/*
*
add port map
@url https://github.com/txn2/kubefwd/issues/121
*/
type PortMap struct {
	SourcePort string
	TargetPort string
}

// String representation of a ServiceFWD returns a unique name
// in the form SERVICE_NAME.NAMESPACE.CONTEXT
func (svcFwd *ServiceFWD) String() string {
	return svcFwd.Svc.Name + "." + svcFwd.Namespace + "." + svcFwd.Context
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
		if pod.Status.Phase == v1.PodPending || pod.Status.Phase == v1.PodRunning {
			podsEligible = append(podsEligible, pod)
		}
	}

	return podsEligible
}

// SyncPodForwards selects one or all pods behind a service, and invokes
// the forwarding setup for that or those pod(s). It will remove pods in-mem
// that are no longer returned by k8s, should these not be correctly deleted.
func (svcFwd *ServiceFWD) SyncPodForwards(force bool) {
	sync := func() {

		defer func() { svcFwd.LastSyncedAt = time.Now() }()

		k8sPods := svcFwd.GetPodsForService()

		// If no pods are found currently. Will try again next re-sync period.
		if len(k8sPods) == 0 {
			log.Warnf("WARNING: No Running Pods returned for service %s", svcFwd)
			return
		}

		// Check if the pods currently being forwarded still exist in k8s and if
		// they are not in a (pre-)running state, if not: remove them
		for _, podName := range svcFwd.ListServicePodNames() {
			keep := false
			for _, pod := range k8sPods {
				if podName == pod.Name && (pod.Status.Phase == v1.PodPending || pod.Status.Phase == v1.PodRunning) {
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
					if podName == pod.Name && (pod.Status.Phase == v1.PodPending || pod.Status.Phase == v1.PodRunning) {
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
	// will still reload this information at least once every 5 minutes.
	if force || time.Since(svcFwd.LastSyncedAt) > 5*time.Minute {
		// Replace current debounced function with no-op
		svcFwd.SyncDebouncer(func() {})

		// Do the syncing work
		sync()
	} else {
		// Queue sync
		svcFwd.SyncDebouncer(sync)
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
		// If pod is already configured to be forwarded, skip it
		if _, found := svcFwd.PortForwards[pod.Name]; found {
			continue
		}

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
			HostLocalServices:        svcFwd.HostLocalServices,
		}
		var localIp net.IP
		if svcFwd.isHostLocalService(svcName) {
			localIp = []byte{127, 0, 0, 1}
			log.Infof("service %s IP forced to %s as a host-local service", svcName, localIp)
		} else {
			localIpRdy, err := fwdnet.ReadyInterface(opts)
			if err != nil {
				log.Warnf("WARNING: error readying interface: %s\n", err)
			}
			localIp = localIpRdy
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
			if _, err := strconv.Atoi(podPort); err != nil {
				// search a pods containers for the named port
				if namedPodPort, ok := portSearch(podPort, pod.Spec.Containers); ok {
					podPort = namedPodPort
				}
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
				Out:        publisher,
				Config:     svcFwd.ClientConfig,
				ClientSet:  svcFwd.ClientSet,
				RESTClient: svcFwd.RESTClient,
				Context:    svcFwd.Context,
				Namespace:  pod.Namespace,
				Service:    svcName,
				ServiceFwd: svcFwd,
				PodName:    pod.Name,
				PodPort:    podPort,
				LocalIp:    localIp,
				LocalPort:  localPort,
				HostFile:   svcFwd.Hostfile,
				ClusterN:   svcFwd.ClusterN,
				NamespaceN: svcFwd.NamespaceN,
				Domain:     svcFwd.Domain,

				ManualStopChan: make(chan struct{}),
				DoneChan:       make(chan struct{}),
				HostLocal:      svcFwd.isHostLocalService(svcName),
			}

			// Fire and forget. The stopping is done in the service.Shutdown() method.
			go func() {
				svcFwd.AddServicePod(pfo)
				if err := pfo.PortForward(); err != nil {
					select {
					case <-pfo.ManualStopChan: // if shutdown was given, we don't bother with the error.
					default:
						log.Errorf("PortForward error on %s: %s", pfo.PodName, err.Error())
					}
				} else {
					select {
					case <-pfo.ManualStopChan: // if shutdown was given, don't log a warning as it's an intended stopping.
					default:
						log.Warnf("Stopped forwarding pod %s for %s", pfo.PodName, svcFwd)
					}
				}
			}()

		}

	}
}

// AddServicePod
func (svcFwd *ServiceFWD) AddServicePod(pfo *fwdport.PortForwardOpts) {
	svcFwd.NamespaceServiceLock.Lock()
	ServicePod := pfo.Service + "." + pfo.PodName
	if _, found := svcFwd.PortForwards[ServicePod]; !found {
		svcFwd.PortForwards[ServicePod] = pfo
	}
	svcFwd.NamespaceServiceLock.Unlock()
}

// ListServicePodNames
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
	if pod, found := svcFwd.PortForwards[servicePodName]; found {
		pod.Stop()
		<-pod.DoneChan
		svcFwd.NamespaceServiceLock.Lock()
		delete(svcFwd.PortForwards, servicePodName)
		svcFwd.NamespaceServiceLock.Unlock()
	}
}

func portSearch(portName string, containers []v1.Container) (string, bool) {
	for _, container := range containers {
		for _, cp := range container.Ports {
			if cp.Name == portName {
				return fmt.Sprint(cp.ContainerPort), true
			}
		}
	}

	return "", false
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

func (svcFwd *ServiceFWD) isHostLocalService(svcName string) bool {
	for _, hostLocalName := range svcFwd.HostLocalServices {
		// TODO - Should we be a case-insensitive compare?
		if strings.Compare(hostLocalName, svcName) == 0 {
			return true
		}
	}
	return false
}
