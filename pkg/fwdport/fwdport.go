//go:generate ${GOPATH}/bin/mockgen -source=fwdport.go -destination=mock_fwdport.go -package=fwdport
package fwdport

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/util/httpstream"

	log "github.com/sirupsen/logrus"
	"github.com/txn2/kubefwd/pkg/fwdnet"
	"github.com/txn2/kubefwd/pkg/fwdpub"
	"github.com/txn2/txeh"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

// ServiceFWD PodSyncer interface is used to represent a
// fwdservice.ServiceFWD reference, which cannot be used directly
// due to circular imports.  It's a reference from a pod to it's
// parent service.
type ServiceFWD interface {
	String() string
	SyncPodForwards(bool)
	ListServicePodNames() []string
	AddServicePod(pfo *PortForwardOpts)
	GetServicePodPortForwards(servicePodName string) []*PortForwardOpts
	RemoveServicePod(servicePodName string, stop bool)
	RemoveServicePodByPort(servicePodName string, podPort string, stop bool)
	GetHostsOperator() HostsOperator
}

type PortForwardHelper interface {
	GetPortForwardRequest(pfo *PortForwardOpts) *restclient.Request
	NewOnAddresses(dialer httpstream.Dialer, addresses []string, ports []string, stopChan <-chan struct{}, readyChan chan struct{}, out, errOut io.Writer) (*portforward.PortForwarder, error)
	ForwardPorts(forwarder *portforward.PortForwarder) error

	RoundTripperFor(config *restclient.Config) (http.RoundTripper, spdy.Upgrader, error)
	NewDialer(upgrader spdy.Upgrader, client *http.Client, method string, pfRequest *restclient.Request) httpstream.Dialer
}

type HostsOperator interface {
	AddHosts(pfo *PortForwardOpts)
	RemoveHosts(pfo *PortForwardOpts)
	RemoveInterfaceAlias(pfo *PortForwardOpts)
}

type PortForwardHelperImpl struct {}

type HostsOperatorImpl struct {
	hostsOperationsMutex     *sync.Mutex
	hostsFileContainsEntries bool
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
	ClientSet  kubernetes.Clientset
	RESTClient restclient.Interface

	Service    string
	ServiceFwd ServiceFWD
	PodName    string
	PodPort    string
	LocalIp    net.IP
	LocalPort  string
	HostFile   *HostFileWithLock

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
	ManualStopChan chan PortForwardStopOpts // Send a signal on this to stop the portforwarding
	DoneChan       chan struct{}            // Listen on this channel for when the shutdown is completed.

	StateWaiter       PodStateWaiter
	PortForwardHelper PortForwardHelper
}

type PortForwardStopOpts struct {
	RemoveHosts          bool
	RemoveFromServiceFwd bool
}

type pingingDialer struct {
	wrappedDialer     httpstream.Dialer
	pingPeriod        time.Duration
	pingStopChan      chan struct{}
	pingTargetPodName string
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
				log.Debugf("Ping process stopped for %s", p.pingTargetPodName)
				return
			}
		}
	}(streamConn)

	return streamConn, streamProtocolVersion, dialErr
}

// PortForward does the port-forward for a single pod.
// It is a blocking call and will return when an error occurred
// or after a cancellation signal has been received.
func PortForward(pfo *PortForwardOpts) error {
	defer close(pfo.DoneChan)
	var stopOpts PortForwardStopOpts
	var stopOptsReceived = false

	defer pfo.handleStop(&stopOpts)

	transport, upgrader, err := pfo.PortForwardHelper.RoundTripperFor(&pfo.Config)
	if err != nil {
		return err
	}

	// check that pod port can be strconv.ParseUint
	_, err = strconv.ParseUint(pfo.PodPort, 10, 32)
	if err != nil {
		pfo.PodPort = pfo.LocalPort
	}

	fwdPorts := []string{fmt.Sprintf("%s:%s", pfo.LocalPort, pfo.PodPort)}
	req := pfo.PortForwardHelper.GetPortForwardRequest(pfo)

	pfStopChannel := make(chan struct{}, 1)      // Signal that k8s forwarding takes as input for us to signal when to stop
	downstreamStopChannel := make(chan struct{}) // @TODO: can this be the same as pfStopChannel?
	pingStopChannel := make(chan struct{})

	localNamedEndPoint := fmt.Sprintf("%s:%s", pfo.Service, pfo.LocalPort)

	// Close created downstream channels if there are stop signal from above
	go func() {
		stopOpts, stopOptsReceived = <-pfo.ManualStopChan
		close(pfo.ManualStopChan)
		close(downstreamStopChannel)
		close(pfStopChannel)
		close(pingStopChannel)
	}()

	// Waiting until the pod is running
	pod, err := pfo.StateWaiter.WaitUntilPodRunning(downstreamStopChannel)
	if err != nil {
		log.Errorf("Error on wait until pod running. Err: %v", err)
		if !stopOptsReceived {
			stopOpts = PortForwardStopOpts{RemoveHosts: false, RemoveFromServiceFwd: true}
			pfo.Stop(stopOpts)
		}
		return err
	} else if pod == nil {
		// TODO: Review comment below (is NOT nil? really?)
		// if err is not nil but pod is nil
		// mean service deleted but pod is not runnning.
		// No error, just return
		if !stopOptsReceived {
			stopOpts = PortForwardStopOpts{RemoveHosts: false, RemoveFromServiceFwd: true}
			pfo.Stop(stopOpts)
		}
		return nil
	}

	// Listen for pod is deleted
	// @TODO need a test for this, does not seem to work as intended
	// go pfo.ListenUntilPodDeleted(downstreamStopChannel, pod)

	p := pfo.Out.MakeProducer(localNamedEndPoint)

	dialer := pfo.PortForwardHelper.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, req)
	dialerWithPing := pingingDialer{
		wrappedDialer:     dialer,
		pingPeriod:        time.Second * 30,
		pingStopChan:      pingStopChannel,
		pingTargetPodName: pfo.String(),
	}

	var address []string
	if pfo.LocalIp != nil {
		address = []string{pfo.LocalIp.To4().String(), pfo.LocalIp.To16().String()}
	} else {
		address = []string{"localhost"}
	}

	fw, err := pfo.PortForwardHelper.NewOnAddresses(dialerWithPing, address, fwdPorts, pfStopChannel, make(chan struct{}), &p, &p)
	if err != nil {
		log.Errorf("Error at creating new PortForwarder. Err: %v", err)
		if !stopOptsReceived {
			stopOpts = PortForwardStopOpts{RemoveHosts: false, RemoveFromServiceFwd: true}
			pfo.Stop(stopOpts)
		}
		return err
	}

	pfo.ServiceFwd.GetHostsOperator().AddHosts(pfo)

	// Blocking call
	if err = pfo.PortForwardHelper.ForwardPorts(fw); err != nil {
		log.Errorf("ForwardPorts error for %s: %s", pfo, err.Error())
		if !stopOptsReceived {
			stopOpts = PortForwardStopOpts{RemoveHosts: true, RemoveFromServiceFwd: true}
			pfo.Stop(stopOpts)
		}

		return err
	} else {
		if !stopOptsReceived {
			stopOpts = PortForwardStopOpts{RemoveHosts: true, RemoveFromServiceFwd: true}
			pfo.Stop(stopOpts)
		}
	}

	return nil
}

func (pfo *PortForwardOpts) handleStop(opts *PortForwardStopOpts) {
	log.Debugf("On Port-Forward stop: handle stop for %s:%s", pfo, pfo.PodPort)
	if opts.RemoveFromServiceFwd {
		log.Debugf("On Port-Forward stop: remove for %s:%s from service fwd ", pfo, pfo.PodPort)
		pfo.ServiceFwd.RemoveServicePodByPort(pfo.String(), pfo.PodPort, false)
	}
	if opts.RemoveHosts {
		log.Debugf("On Port-Forward stop: remove hosts for %s:%s", pfo, pfo.PodPort)
		pfo.ServiceFwd.GetHostsOperator().RemoveHosts(pfo)
		pfo.ServiceFwd.GetHostsOperator().RemoveInterfaceAlias(pfo)
	}
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

// WaitUntilPodRunning Waiting for the pod running
func (waiter *PodStateWaiterImpl) WaitUntilPodRunning(stopChannel <-chan struct{}) (*v1.Pod, error) {
	pod, err := waiter.ClientSet.CoreV1().Pods(waiter.Namespace).Get(context.TODO(), waiter.PodName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	if pod.Status.Phase == v1.PodRunning {
		return pod, nil
	}

	watcher, err := waiter.ClientSet.CoreV1().Pods(waiter.Namespace).Watch(context.TODO(), metav1.SingleObject(pod.ObjectMeta))
	if err != nil {
		return nil, err
	}

	// if the os.signal (we enter the Ctrl+C)
	// or ManualStop (service delete or some thing wrong)
	// or RunningChannel channel (the watch for pod runnings is done)
	// or timeout after 300s
	// we'll stop the watcher
	// TODO: change the 300s timeout to custom settings.
	go func() {
		defer watcher.Stop()
		select {
		case <-stopChannel:
		case <-time.After(time.Second * 300):
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

// ListenUntilPodDeleted listen for pod is deleted
func (waiter *PodStateWaiterImpl) ListenUntilPodDeleted(stopChannel <-chan struct{}, pod *v1.Pod) {

	watcher, err := waiter.ClientSet.CoreV1().Pods(waiter.Namespace).Watch(context.TODO(), metav1.SingleObject(pod.ObjectMeta))
	if err != nil {
		return
	}

	// Listen for stop signal from above
	go func() {
		<-stopChannel
		watcher.Stop()
	}()

	// watcher until the pod is deleted, then trigger a syncpodforwards
	for {
		event, ok := <-watcher.ResultChan()
		if !ok {
			break
		}
		switch event.Type {
		case watch.Deleted:
			log.Warnf("Pod %s deleted, resyncing the %s service pods.", pod.ObjectMeta.Name, waiter.ServiceFwd)
			waiter.ServiceFwd.SyncPodForwards(false)
			return
		}
	}
}

// Stop sends the shutdown signal to the port-forwarding process.
// In case the shutdown signal was already given before, this is a no-op.
func (pfo *PortForwardOpts) Stop(opts PortForwardStopOpts) {
	select {
	case <-pfo.DoneChan:
		return
	case <-pfo.ManualStopChan:
		return
	default:
	}
	pfo.ManualStopChan <- opts
}

func (pfo *PortForwardOpts) String() string {
	return pfo.PodName
}

type PodStateWaiter interface {
	WaitUntilPodRunning(stopChannel <-chan struct{}) (*v1.Pod, error)
	//ListenUntilPodDeleted(stopChannel <-chan struct{}, pod *v1.Pod)
}

type PodStateWaiterImpl struct {
	Namespace  string
	PodName    string
	ClientSet  kubernetes.Clientset
	ServiceFwd ServiceFWD
}

func (p PortForwardHelperImpl) GetPortForwardRequest(pfo *PortForwardOpts) *restclient.Request {
	// if need to set timeout, set it here.
	// restClient.Client.Timeout = 32
	return pfo.RESTClient.Post().
		Resource("pods").
		Namespace(pfo.Namespace).
		Name(pfo.PodName).
		SubResource("portforward")
}

func (p PortForwardHelperImpl) NewOnAddresses(dialer httpstream.Dialer, addresses []string, ports []string, stopChan <-chan struct{}, readyChan chan struct{}, out, errOut io.Writer) (*portforward.PortForwarder, error) {
	return portforward.NewOnAddresses(dialer, addresses, ports, stopChan, readyChan, out, errOut)
}

func (p PortForwardHelperImpl) RoundTripperFor(config *restclient.Config) (http.RoundTripper, spdy.Upgrader, error) {
	return spdy.RoundTripperFor(config)
}

func (p PortForwardHelperImpl) NewDialer(upgrader spdy.Upgrader, client *http.Client, method string, pfRequest *restclient.Request) httpstream.Dialer {
	return spdy.NewDialer(upgrader, client, method, pfRequest.URL())
}

func (p PortForwardHelperImpl) ForwardPorts(forwarder *portforward.PortForwarder) error {
	return forwarder.ForwardPorts()
}

// AddHosts adds hostname entries to /etc/hosts
func (operator *HostsOperatorImpl) AddHosts(pfo *PortForwardOpts) {
	operator.hostsOperationsMutex.Lock()
	defer operator.hostsOperationsMutex.Unlock()

	dealWithHostsFile := !operator.hostsFileContainsEntries

	if dealWithHostsFile {
		pfo.HostFile.Lock()
	}

	// pfo.Service holds only the service name
	// start with the smallest allowable hostname

	// bare service name
	if pfo.ClusterN == 0 && pfo.NamespaceN == 0 {
		operator.addHost(pfo, pfo.Service, dealWithHostsFile)

		if pfo.Domain != "" {
			operator.addHost(pfo, fmt.Sprintf(
				"%s.%s",
				pfo.Service,
				pfo.Domain,
			), dealWithHostsFile)
		}
	}

	// alternate cluster / first namespace
	if pfo.ClusterN > 0 && pfo.NamespaceN == 0 {
		operator.addHost(pfo, fmt.Sprintf(
			"%s.%s",
			pfo.Service,
			pfo.Context,
		), dealWithHostsFile)
	}

	// namespaced without cluster
	if pfo.ClusterN == 0 {
		operator.addHost(pfo, fmt.Sprintf(
			"%s.%s",
			pfo.Service,
			pfo.Namespace,
		), dealWithHostsFile)

		operator.addHost(pfo, fmt.Sprintf(
			"%s.%s.svc",
			pfo.Service,
			pfo.Namespace,
		), dealWithHostsFile)

		operator.addHost(pfo, fmt.Sprintf(
			"%s.%s.svc.cluster.local",
			pfo.Service,
			pfo.Namespace,
		), dealWithHostsFile)

		if pfo.Domain != "" {
			operator.addHost(pfo, fmt.Sprintf(
				"%s.%s.svc.cluster.%s",
				pfo.Service,
				pfo.Namespace,
				pfo.Domain,
			), dealWithHostsFile)
		}

	}

	operator.addHost(pfo, fmt.Sprintf(
		"%s.%s.%s",
		pfo.Service,
		pfo.Namespace,
		pfo.Context,
	), dealWithHostsFile)

	operator.addHost(pfo, fmt.Sprintf(
		"%s.%s.svc.%s",
		pfo.Service,
		pfo.Namespace,
		pfo.Context,
	), dealWithHostsFile)

	operator.addHost(pfo, fmt.Sprintf(
		"%s.%s.svc.cluster.%s",
		pfo.Service,
		pfo.Namespace,
		pfo.Context,
	), dealWithHostsFile)

	if dealWithHostsFile {
		err := pfo.HostFile.Hosts.Save()
		if err != nil {
			log.Error("Error saving hosts file", err)
		}
		pfo.HostFile.Unlock()
	}
	if dealWithHostsFile {
		operator.hostsFileContainsEntries = dealWithHostsFile
	}
}

// RemoveHosts removes hosts /etc/hosts  associated with a forwarded pod
func (operator *HostsOperatorImpl) RemoveHosts(pfo *PortForwardOpts) {
	operator.hostsOperationsMutex.Lock()
	defer operator.hostsOperationsMutex.Unlock()

	// we should lock the pfo.HostFile here
	// because sometimes other goroutine write the *txeh.Hosts
	pfo.HostFile.Lock()
	defer pfo.HostFile.Unlock()

	// We must not remove hosts entries if port-forwarding on one of the service ports is cancelled and others not
	if !operator.hostsFileContainsEntries {
		log.Debugf("Do not remove hosts entry for %s", pfo)
		return
	}

	// other applications or process may have written to /etc/hosts
	// since it was originally updated.
	err := pfo.HostFile.Hosts.Reload()
	if err != nil {
		log.Errorf("Unable to reload /etc/hosts: %s", err.Error())
		return
	}

	// remove all hosts
	for _, host := range pfo.Hosts {
		log.Debugf("Removing host %s for pod %s in namespace %s from context %s", host, pfo.PodName, pfo.Namespace, pfo.Context)
		pfo.HostFile.Hosts.RemoveHost(host)
	}

	// fmt.Printf("Delete Host And Save !\r\n")
	err = pfo.HostFile.Hosts.Save()
	if err != nil {
		log.Errorf("Error saving /etc/hosts: %s\n", err.Error())
	} else {
		operator.hostsFileContainsEntries = false
	}
}

func (operator *HostsOperatorImpl) RemoveInterfaceAlias(pfo *PortForwardOpts) {
	operator.hostsOperationsMutex.Lock()
	defer operator.hostsOperationsMutex.Unlock()
	fwdnet.RemoveInterfaceAlias(pfo.LocalIp)
}

func (operator HostsOperatorImpl) addHost(pfo *PortForwardOpts, host string, editHostFile bool) {
	// add to list of hostnames for this port-forward
	pfo.Hosts = append(pfo.Hosts, host)

	if editHostFile {
		// remove host if it already exists in /etc/hosts
		pfo.HostFile.Hosts.RemoveHost(host)

		// add host to /etc/hosts
		pfo.HostFile.Hosts.AddHost(pfo.LocalIp.String(), host)
	}
}

func NewHostsOperator() *HostsOperatorImpl {
	return &HostsOperatorImpl{
		hostsOperationsMutex:     &sync.Mutex{},
		hostsFileContainsEntries: false,
	}
}