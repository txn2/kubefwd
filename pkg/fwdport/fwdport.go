package fwdport

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"

	"k8s.io/apimachinery/pkg/util/httpstream"

	log "github.com/sirupsen/logrus"
	"github.com/txn2/kubefwd/pkg/fwdnet"
	"github.com/txn2/kubefwd/pkg/fwdpub"
	"github.com/txn2/txeh"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

// GlobalPodInformer manages a single informer for all pods across all services
type GlobalPodInformer struct {
	mu          sync.RWMutex
	activePods  map[types.UID]*PortForwardOpts
	clientSet   kubernetes.Interface
	informers   map[string]informers.SharedInformerFactory
	stopChannel chan struct{}
}

var globalPodInformer *GlobalPodInformer
var globalPodInformerOnce sync.Once

// GetGlobalPodInformer returns the singleton global pod informer
func GetGlobalPodInformer(clientSet kubernetes.Interface, namespace string) *GlobalPodInformer {
	globalPodInformerOnce.Do(func() {
		globalPodInformer = &GlobalPodInformer{
			activePods:  make(map[types.UID]*PortForwardOpts),
			clientSet:   clientSet,
			informers:   make(map[string]informers.SharedInformerFactory),
			stopChannel: make(chan struct{}),
		}
	})
	globalPodInformer.mu.Lock()
	if _, ok := globalPodInformer.informers[namespace]; !ok {
		synced := globalPodInformer.startInformer(namespace)
		globalPodInformer.mu.Unlock()

		if !cache.WaitForCacheSync(globalPodInformer.stopChannel, synced) {
			log.Errorf("Failed to sync global pod informer cache")
		} else {
			log.Infof("Started listening for pods being deleted in namespace %s", namespace)
		}
	} else {
		globalPodInformer.mu.Unlock()
	}

	return globalPodInformer
}

// startInformer starts the global pod informer
func (gpi *GlobalPodInformer) startInformer(namespace string) cache.InformerSynced {
	gpi.informers[namespace] = informers.NewSharedInformerFactoryWithOptions(
		gpi.clientSet,
		time.Minute,
		informers.WithNamespace(namespace),
	)

	podInformer := gpi.informers[namespace].Core().V1().Pods()

	podInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			UpdateFunc: func(oldObj, newObj interface{}) {
				oldPod, oldPodOk := oldObj.(*v1.Pod)
				newPod, newPodOk := newObj.(*v1.Pod)

				if !oldPodOk || !newPodOk {
					log.Warnf("Non-pod object was updated: %v, %v", oldObj, newObj)
					return
				}

				pfo, exists := gpi.getPod(oldPod.UID)
				if !exists {
					return
				}

				if newPod.DeletionTimestamp != nil {
					log.Warnf("Pod %s marked for deletion, resyncing the %s service pods.", oldPod.ObjectMeta.Name, pfo.ServiceFwd)
					pfo.Stop()
					pfo.ServiceFwd.SyncPodForwards(false)
					gpi.removePod(oldPod.UID)
				}
			},
			DeleteFunc: func(obj interface{}) {
				deletedPod, ok := obj.(*v1.Pod)

				if !ok {
					log.Warnf("Non-pod object was deleted: %v", obj)
					return
				}

				pfo, exists := gpi.getPod(deletedPod.UID)
				if !exists {
					return
				}

				log.Warnf("Pod %s deleted, resyncing the %s service pods.", deletedPod.ObjectMeta.Name, pfo.ServiceFwd)
				pfo.Stop()
				pfo.ServiceFwd.SyncPodForwards(false)
				gpi.removePod(deletedPod.UID)
				log.Debugf("After pod %s was deleted, the %s service pods have been resynced.", deletedPod.ObjectMeta.Name, pfo.ServiceFwd)
			},
		},
	)

	go gpi.informers[namespace].Start(gpi.stopChannel)

	return podInformer.Informer().HasSynced
}

// addPod adds a pod to the active pods map
func (gpi *GlobalPodInformer) addPod(pod *v1.Pod, pfo *PortForwardOpts) {
	gpi.mu.Lock()
	defer gpi.mu.Unlock()
	gpi.activePods[pod.UID] = pfo
	log.Debugf("Added pod %s (UID: %s) to global informer", pod.Name, pod.UID)
}

// getPod retrieves a pod from the active pods map
func (gpi *GlobalPodInformer) getPod(podUID types.UID) (*PortForwardOpts, bool) {
	gpi.mu.RLock()
	defer gpi.mu.RUnlock()
	pfo, exists := gpi.activePods[podUID]
	return pfo, exists
}

// removePod removes a pod from the active pods map
func (gpi *GlobalPodInformer) removePod(podUID types.UID) {
	gpi.mu.Lock()
	defer gpi.mu.Unlock()
	delete(gpi.activePods, podUID)
	log.Debugf("Removed pod (UID: %s) from global informer", podUID)
}

// RemovePodByUID removes a pod from the global informer by UID
func (gpi *GlobalPodInformer) RemovePodByUID(podUID types.UID) {
	gpi.removePod(podUID)
}

// Stop stops the global pod informer
func (gpi *GlobalPodInformer) Stop() {
	select {
	case <-gpi.stopChannel:
		// Already stopped, nothing to do
		return
	default:
		// Not stopped yet, close the channel to stop the informer
		close(gpi.stopChannel)
		log.Infof("Stopped listening for pods being deleted")
	}
}

// StopGlobalPodInformer stops the global pod informer
// This should be called during application shutdown
func StopGlobalPodInformer() {
	if globalPodInformer != nil {
		globalPodInformer.Stop()
	}
}

// ResetGlobalPodInformer resets the singleton for testing purposes.
// This is necessary because sync.Once will otherwise prevent re-initialization
// with new clientsets in subsequent tests.
func ResetGlobalPodInformer() {
	if globalPodInformer != nil {
		globalPodInformer.Stop()
	}
	globalPodInformer = nil
	globalPodInformerOnce = sync.Once{}
}

// ServiceFWD PodSyncer interface is used to represent a
// fwdservice.ServiceFWD reference, which cannot be used directly
// due to circular imports.  It's a reference from a pod to it's
// parent service.
type ServiceFWD interface {
	String() string
	SyncPodForwards(bool)
}

// HostFileWithLock
type HostFileWithLock struct {
	Hosts *txeh.Hosts
	sync.Mutex
}

// HostsParams
type HostsParams struct {
	localServiceName string
	nsServiceName    string
	fullServiceName  string
	svcServiceName   string
}

// PortForwardOpts
type PortForwardOpts struct {
	Out        *fwdpub.Publisher
	Config     restclient.Config
	ClientSet  kubernetes.Interface
	RESTClient restclient.RESTClient

	Service    string
	ServiceFwd ServiceFWD
	PodName    string
	PodUID     types.UID
	PodPort    string
	LocalIp    net.IP
	LocalPort  string
	// Timeout for the port-forwarding process
	Timeout  int
	HostFile *HostFileWithLock

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

	Domain         string
	HostsParams    *HostsParams
	Hosts          []string
	ManualStopChan chan struct{} // Send a signal on this to stop the portforwarding
	DoneChan       chan struct{} // Listen on this channel for when the shutdown is completed.

	stopOnce sync.Once // Ensures ManualStopChan is closed exactly once
}

type pingingDialer struct {
	wrappedDialer     httpstream.Dialer
	pingPeriod        time.Duration
	pingStopChan      chan struct{}
	pingTargetPodName string
}

func (p pingingDialer) stopPing() {
	p.pingStopChan <- struct{}{}
}

func (p pingingDialer) Dial(protocols ...string) (httpstream.Connection, string, error) {
	streamConn, streamProtocolVersion, dialErr := p.wrappedDialer.Dial(protocols...)
	if dialErr != nil {
		log.Warnf("Ping process will not be performed for %s, cannot dial", p.pingTargetPodName)
	}
	go func(streamConnection httpstream.Connection) {
		if streamConnection == nil || dialErr != nil {
			return
		}
		for {
			select {
			case <-time.After(p.pingPeriod):
				if pingStream, err := streamConnection.CreateStream(nil); err == nil {
					_ = pingStream.Reset()
				}
			case <-p.pingStopChan:
				log.Debug(fmt.Sprintf("Ping process stopped for %s", p.pingTargetPodName))
				return
			}
		}
	}(streamConn)

	return streamConn, streamProtocolVersion, dialErr
}

// PortForward does the port-forward for a single pod.
// It is a blocking call and will return when an error occurred
// or after a cancellation signal has been received.
func (pfo *PortForwardOpts) PortForward() error {
	defer close(pfo.DoneChan)

	transport, upgrader, err := spdy.RoundTripperFor(&pfo.Config)
	if err != nil {
		return err
	}

	// check that pod port can be strconv.ParseUint
	_, err = strconv.ParseUint(pfo.PodPort, 10, 32)
	if err != nil {
		pfo.PodPort = pfo.LocalPort
	}

	fwdPorts := []string{fmt.Sprintf("%s:%s", pfo.LocalPort, pfo.PodPort)}

	// if need to set timeout, set it here.
	// restClient.Client.Timeout = 32
	req := pfo.RESTClient.Post().
		Resource("pods").
		Namespace(pfo.Namespace).
		Name(pfo.PodName).
		SubResource("portforward")

	pfStopChannel := make(chan struct{}, 1)      // Signal that k8s forwarding takes as input for us to signal when to stop
	downstreamStopChannel := make(chan struct{}) // @TODO: can this be the same as pfStopChannel?
	cleanupDone := make(chan struct{})

	localNamedEndPoint := fmt.Sprintf("%s:%s", pfo.Service, pfo.LocalPort)

	if err := pfo.AddHosts(); err != nil {
		return err
	}

	// Wait until the stop signal is received from above
	go func() {
		<-pfo.ManualStopChan
		close(downstreamStopChannel)
		pfo.removeHosts()
		pfo.removeInterfaceAlias()
		close(pfStopChannel)
		close(cleanupDone)
	}()

	// Waiting until the pod is running
	pod, err := pfo.WaitUntilPodRunning(downstreamStopChannel)
	if err != nil {
		pfo.Stop()
		<-cleanupDone
		return err
	} else if pod == nil {
		// if err is not nil but pod is nil
		// mean service deleted but pod is not runnning.
		// No error, just return
		pfo.Stop()
		<-cleanupDone
		return nil
	}

	// Set the pod UID for cleanup purposes
	pfo.PodUID = pod.UID

	// Register this pod with the global informer
	globalInformer := GetGlobalPodInformer(pfo.ClientSet, pfo.Namespace)
	globalInformer.addPod(pod, pfo)

	p := pfo.Out.MakeProducer(localNamedEndPoint)

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, req.URL())
	dialerWithPing := pingingDialer{
		wrappedDialer:     dialer,
		pingPeriod:        time.Second * 30,
		pingStopChan:      make(chan struct{}),
		pingTargetPodName: pfo.PodName,
	}

	var address []string
	if pfo.LocalIp != nil {
		address = []string{pfo.LocalIp.To4().String(), pfo.LocalIp.To16().String()}
	} else {
		address = []string{"localhost"}
	}

	fw, err := portforward.NewOnAddresses(dialerWithPing, address, fwdPorts, pfStopChannel, make(chan struct{}), &p, &p)
	if err != nil {
		pfo.Stop()
		<-cleanupDone
		return err
	}

	// Blocking call
	if err = fw.ForwardPorts(); err != nil {
		log.Errorf("ForwardPorts error: %s", err.Error())
		pfo.Stop()
		dialerWithPing.stopPing()
		<-cleanupDone
		return err
	}

	// Stop the ping goroutine on successful completion
	dialerWithPing.stopPing()

	<-cleanupDone
	return nil
}

//// BuildHostsParams constructs the basic hostnames for the service
//// based on the PortForwardOpts configuration
//func (pfo *PortForwardOpts) BuildHostsParams() {
//
//	localServiceName := pfo.Service
//	nsServiceName := pfo.Service + "." + pfo.Namespace
//	fullServiceName := fmt.Sprintf("%s.%s.svc.cluster.local", pfo.Service, pfo.Namespace)
//	svcServiceName := fmt.Sprintf("%s.%s.svc", pfo.Service, pfo.Namespace)
//
//	// check if this is an additional cluster (remote from the
//	// perspective of the user / argument order)
//	if pfo.ClusterN > 0 {
//		fullServiceName = fmt.Sprintf("%s.%s.svc.cluster.%s", pfo.Service, pfo.Namespace, pfo.Context)
//	}
//	pfo.HostsParams.localServiceName = localServiceName
//	pfo.HostsParams.nsServiceName = nsServiceName
//	pfo.HostsParams.fullServiceName = fullServiceName
//	pfo.HostsParams.svcServiceName = svcServiceName
//}

// AddHost
func (pfo *PortForwardOpts) addHost(host string) {
	// add to list of hostnames for this port-forward
	pfo.Hosts = append(pfo.Hosts, host)

	// remove host if it already exists in /etc/hosts
	pfo.HostFile.Hosts.RemoveHost(host)

	// add host to /etc/hosts
	pfo.HostFile.Hosts.AddHost(pfo.LocalIp.String(), host)

	sanitizedHost := sanitizeHost(host)
	if host != sanitizedHost {
		pfo.addHost(sanitizedHost) //should recurse only once
	}
}

// make sure any non-alphanumeric characters in the context name don't make it to the generated hostname
func sanitizeHost(host string) string {
	hostnameIllegalChars := regexp.MustCompile(`[^a-zA-Z0-9\-]`)
	replacementChar := `-`
	sanitizedHost := strings.Trim(hostnameIllegalChars.ReplaceAllString(host, replacementChar), replacementChar)
	return sanitizedHost
}

// AddHosts adds hostname entries to /etc/hosts
func (pfo *PortForwardOpts) AddHosts() error {

	pfo.HostFile.Lock()
	defer pfo.HostFile.Unlock()

	// Reload with retry
	var err error
	for i := 0; i < 10; i++ {
		err = pfo.HostFile.Hosts.Reload()
		if err == nil {
			break
		}
		log.Warnf("Unable to reload /etc/hosts (attempt %d/10): %s", i+1, err.Error())
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		log.Error("Unable to reload /etc/hosts: " + err.Error())
		return err
	}

	// pfo.Service holds only the service name
	// start with the smallest allowable hostname

	// bare service name
	if pfo.ClusterN == 0 && pfo.NamespaceN == 0 {
		pfo.addHost(pfo.Service)

		if pfo.Domain != "" {
			pfo.addHost(fmt.Sprintf(
				"%s.%s",
				pfo.Service,
				pfo.Domain,
			))
		}
	}

	// alternate cluster / first namespace
	if pfo.ClusterN > 0 && pfo.NamespaceN == 0 {
		pfo.addHost(fmt.Sprintf(
			"%s.%s",
			pfo.Service,
			pfo.Context,
		))
	}

	// namespaced without cluster
	if pfo.ClusterN == 0 {
		pfo.addHost(fmt.Sprintf(
			"%s.%s",
			pfo.Service,
			pfo.Namespace,
		))

		pfo.addHost(fmt.Sprintf(
			"%s.%s.svc",
			pfo.Service,
			pfo.Namespace,
		))

		pfo.addHost(fmt.Sprintf(
			"%s.%s.svc.cluster.local",
			pfo.Service,
			pfo.Namespace,
		))

		if pfo.Domain != "" {
			pfo.addHost(fmt.Sprintf(
				"%s.%s.svc.cluster.%s",
				pfo.Service,
				pfo.Namespace,
				pfo.Domain,
			))
		}

	}

	pfo.addHost(fmt.Sprintf(
		"%s.%s.%s",
		pfo.Service,
		pfo.Namespace,
		pfo.Context,
	))

	pfo.addHost(fmt.Sprintf(
		"%s.%s.svc.%s",
		pfo.Service,
		pfo.Namespace,
		pfo.Context,
	))

	pfo.addHost(fmt.Sprintf(
		"%s.%s.svc.cluster.%s",
		pfo.Service,
		pfo.Namespace,
		pfo.Context,
	))

	for i := 0; i < 10; i++ {
		err = pfo.HostFile.Hosts.Save()
		if err == nil {
			break
		}
		log.Warnf("Error saving /etc/hosts (attempt %d/10): %s", i+1, err.Error())
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		log.Error("Error saving hosts file", err)
		return err
	}
	return nil
}

// removeHosts removes hosts /etc/hosts
// associated with a forwarded pod
func (pfo *PortForwardOpts) removeHosts() {

	// we should lock the pfo.HostFile here
	// because sometimes other goroutine write the *txeh.Hosts
	pfo.HostFile.Lock()
	defer pfo.HostFile.Unlock()

	// other applications or process may have written to /etc/hosts
	// since it was originally updated.
	var err error
	for i := 0; i < 10; i++ {
		err = pfo.HostFile.Hosts.Reload()
		if err == nil {
			break
		}
		log.Warnf("Unable to reload /etc/hosts (attempt %d/10): %s", i+1, err.Error())
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		log.Error("Unable to reload /etc/hosts: " + err.Error())
		return
	}

	// remove all hosts
	for _, host := range pfo.Hosts {
		log.Debugf("Removing host %s for pod %s in namespace %s from context %s", host, pfo.PodName, pfo.Namespace, pfo.Context)
		pfo.HostFile.Hosts.RemoveHost(host)
	}

	// fmt.Printf("Delete Host And Save !\r\n")
	for i := 0; i < 10; i++ {
		err = pfo.HostFile.Hosts.Save()
		if err == nil {
			break
		}
		log.Warnf("Error saving /etc/hosts (attempt %d/10): %s", i+1, err.Error())
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		log.Errorf("Error saving /etc/hosts: %s\n", err.Error())
	}
}

// removeInterfaceAlias called on stop signal to
func (pfo *PortForwardOpts) removeInterfaceAlias() {
	fwdnet.RemoveInterfaceAlias(pfo.LocalIp)
}

// Waiting for the pod running
func (pfo *PortForwardOpts) WaitUntilPodRunning(stopChannel <-chan struct{}) (*v1.Pod, error) {
	pod, err := pfo.ClientSet.CoreV1().Pods(pfo.Namespace).Get(context.TODO(), pfo.PodName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	if pod.Status.Phase == v1.PodRunning {
		return pod, nil
	}

	watcher, err := pfo.ClientSet.CoreV1().Pods(pfo.Namespace).Watch(context.TODO(), metav1.SingleObject(pod.ObjectMeta))
	if err != nil {
		return nil, err
	}

	// done signals that this function is returning (pod became Running or loop exited)
	done := make(chan struct{})
	defer close(done)

	// if the os.signal (we enter the Ctrl+C)
	// or ManualStop (service delete or some thing wrong)
	// or done channel (the function is returning)
	// or timeout after 300s(default)
	// we'll stop the watcher
	go func() {
		defer watcher.Stop()
		select {
		case <-stopChannel:
		case <-done:
		case <-time.After(time.Duration(pfo.Timeout) * time.Second):
		}
	}()

	// watcher until the pod status is running
	for {
		event, ok := <-watcher.ResultChan()
		if !ok || event.Type == "ERROR" {
			break
		}
		if event.Object != nil && event.Type == "MODIFIED" {
			changedPod := event.Object.(*v1.Pod)
			if changedPod.Status.Phase == v1.PodRunning {
				return changedPod, nil
			}
		}
	}
	return nil, nil
}

// Stop sends the shutdown signal to the port-forwarding process.
// In case the shutdown signal was already given before, this is a no-op.
// This method is safe to call concurrently from multiple goroutines.
func (pfo *PortForwardOpts) Stop() {
	pfo.stopOnce.Do(func() {
		select {
		case <-pfo.DoneChan:
			return
		case <-pfo.ManualStopChan:
			return
		default:
		}
		close(pfo.ManualStopChan)
	})
}
