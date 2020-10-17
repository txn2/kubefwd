package fwdservice

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"

	"github.com/txn2/kubefwd/pkg/fwdnet"
	"github.com/txn2/kubefwd/pkg/fwdport"
	"github.com/txn2/kubefwd/pkg/fwdpub"

	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	restclient "k8s.io/client-go/rest"
)

// Single service which we need to forward, with a reference to all the pods being forwarded for it
type ServiceFWD struct {
	ClientSet    *kubernetes.Clientset
	Context      string
	Namespace    string
	ListOptions  metav1.ListOptions
	Hostfile     *fwdport.HostFileWithLock
	ClientConfig *restclient.Config
	RESTClient   *restclient.RESTClient
	ShortName    bool
	Remote       bool
	IpC          byte
	IpD          *int
	Domain       string

	PodLabelSelector string                              // The label selector to query for matching pods.
	NamespaceIPLock  *sync.Mutex                         // Synchronization for IP handout for each portforward
	Svc              *v1.Service                         // Reference to the k8s service.
	Headless         bool                                // A headless service will forward all of the pods, while normally only a single pod is forwarded.
	LastSyncedAt     time.Time                           // When was the set of pods last synced
	PortForwards     map[string]*fwdport.PortForwardOpts // A mapping of all the pods currently being forwarded. key = podname
	DoneChannel      chan struct{}                       // After shutdown is complete, this channel will be closed
}

func (svcFwd *ServiceFWD) String() string {
	return svcFwd.Svc.Name + "." + svcFwd.Namespace
}

// GetPodsForService queries k8s and returns all pods backing this service
// which are eligible for portforwarding; exclude some pods which are in final/failure state.
func (svcFwd *ServiceFWD) GetPodsForService() []v1.Pod {
	listOpts := metav1.ListOptions{LabelSelector: svcFwd.PodLabelSelector}

	pods, err := svcFwd.ClientSet.CoreV1().Pods(svcFwd.Svc.Namespace).List(listOpts)

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

// SyncPodForwards selects one or all pods behind a service, and invokes the forwarding setup for that or those pod(s).
// It will remove pods in-mem that are no longer returned by k8s, should these not be correctly deleted.
func (svcFwd *ServiceFWD) SyncPodForwards(force bool) {
	// When a whole set of pods gets deleted at once, they all will trigger a SyncPodForwards() call. This would hammer k8s with load needlessly.
	// Therefore keep a timestamp from when this was last called and only allow call if the previous one was not too recent.
	if !force && time.Since(svcFwd.LastSyncedAt) < 10*time.Minute {
		log.Debugf("Skipping pods refresh for %s due to rate limiting", svcFwd)
		return
	}
	defer func() { svcFwd.LastSyncedAt = time.Now() }()

	k8sPods := svcFwd.GetPodsForService()

	// If no pods are found currently. Will try again next resync period
	if len(k8sPods) == 0 {
		log.Warnf("WARNING: No Running Pods returned for service %s", svcFwd)
		return
	}

	// Check if the pods currently being forwarded still exist in k8s and if they are not in a (pre-)running state, if not: remove them
	for _, podName := range svcFwd.ListPodNames() {
		keep := false
		for _, pod := range k8sPods {
			if podName == pod.Name && (pod.Status.Phase == v1.PodPending || pod.Status.Phase == v1.PodRunning) {
				keep = true
				break
			}
		}
		if !keep {
			svcFwd.RemovePod(podName)
		}
	}
	// Set up portforwarding for one or all of these pods
	// normal service portforward the first pod as service name. headless service not only forward first Pod as service name, but also portforward all pods.
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
		for _, podName := range svcFwd.ListPodNames() {
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

		// Stop forwarding others, should there be. In case none of the currently forwarded pods are good to keep,
		// podNameToKeep will be the empty string, and the comparison will mean we will remove all pods, which is the desired behaviour.
		for _, podName := range svcFwd.ListPodNames() {
			if podName != podNameToKeep {
				svcFwd.RemovePod(podName)
			}
		}

		// If no good pod was being forwarded already, start one
		if podNameToKeep == "" {
			svcFwd.LoopPodsToForward([]v1.Pod{k8sPods[0]}, false)
		}

	}
}

// LoopPodsToForward starts the portforwarding for each pod in the given list
func (svcFwd *ServiceFWD) LoopPodsToForward(pods []v1.Pod, includePodNameInHost bool) {
	publisher := &fwdpub.Publisher{
		PublisherName: "Services",
		Output:        false,
	}

	// Ip address handout is a critical section for synchronization, use a lock which synchronizes inside each namespace.
	svcFwd.NamespaceIPLock.Lock()
	defer svcFwd.NamespaceIPLock.Unlock()

	for _, pod := range pods {
		// If pod is already configured to be forwarded, skip it
		if _, found := svcFwd.PortForwards[pod.Name]; found {
			continue
		}

		podPort := ""
		svcName := ""

		localIp, dInc, err := fwdnet.ReadyInterface(127, 1, svcFwd.IpC, *svcFwd.IpD, podPort)
		if err != nil {
			log.Warnf("WARNING: error readying interface: %s\n", err)
		}
		*svcFwd.IpD = dInc

		for _, port := range svcFwd.Svc.Spec.Ports {

			podPort = port.TargetPort.String()
			localPort := strconv.Itoa(int(port.Port))

			if _, err := strconv.Atoi(podPort); err != nil {
				// search a pods containers for the named port
				if namedPodPort, ok := portSearch(podPort, pod.Spec.Containers); ok {
					podPort = namedPodPort
				}
			}

			serviceHostName := svcFwd.Svc.Name

			if includePodNameInHost {
				serviceHostName = pod.Name + "." + serviceHostName
			}

			if !svcFwd.ShortName {
				serviceHostName = serviceHostName + "." + pod.Namespace
			}

			if svcFwd.Domain != "" {
				serviceHostName = serviceHostName + "." + svcFwd.Domain
			}

			if svcFwd.Remote {
				serviceHostName = fmt.Sprintf("%s.svc.cluster.%s", serviceHostName, svcFwd.Context)
			}

			svcName = serviceHostName

			log.Debugf("Resolving:    %s to %s\n",
				svcName,
				localIp.String(),
			)

			log.Printf("Port-Forward: %s %s:%d to pod %s:%s\n",
				localIp.String(),
				svcName,
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
				Hostfile:   svcFwd.Hostfile,
				ShortName:  svcFwd.ShortName,
				Remote:     svcFwd.Remote,
				Domain:     svcFwd.Domain,

				ManualStopChan: make(chan struct{}),
				DoneChan:       make(chan struct{}),
			}

			// Fire and forget. The stopping is done in the service.Shutdown() method.
			go func() {
				svcFwd.AddPod(pfo)
				if err := pfo.PortForward(); err != nil {
					select {
					case <-pfo.ManualStopChan: // if shutdown was given, we don't bother with the error.
					default:
						log.Errorf("PortForward error on %s: %s", pfo.PodName, err.Error())
					}
				} else {
					select {
					case <-pfo.ManualStopChan: // if shutdown was given, don't log a warning as it's an intented stopping.
					default:
						log.Warnf("Stopped forwarding pod %s for %s", pfo.PodName, svcFwd)
					}
				}
			}()

		}

	}
}

func (svcFwd *ServiceFWD) AddPod(pfo *fwdport.PortForwardOpts) {
	svcFwd.NamespaceIPLock.Lock()
	if _, found := svcFwd.PortForwards[pfo.PodName]; !found {
		svcFwd.PortForwards[pfo.PodName] = pfo
	}
	svcFwd.NamespaceIPLock.Unlock()
}

func (svcFwd *ServiceFWD) ListPodNames() []string {
	svcFwd.NamespaceIPLock.Lock()
	currentPodNames := make([]string, 0, len(svcFwd.PortForwards))
	for podName := range svcFwd.PortForwards {
		currentPodNames = append(currentPodNames, podName)
	}
	svcFwd.NamespaceIPLock.Unlock()
	return currentPodNames
}

func (svcFwd *ServiceFWD) RemovePod(podName string) {
	if pod, found := svcFwd.PortForwards[podName]; found {
		pod.Stop()
		<-pod.DoneChan
		svcFwd.NamespaceIPLock.Lock()
		delete(svcFwd.PortForwards, podName)
		svcFwd.NamespaceIPLock.Unlock()
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
