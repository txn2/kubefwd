package fwdport

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

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

// PodSyncer interface is used to represent a fwdservice.ServiceFWD reference, which cannot be used directly due to circular imports.
// It's a reference from a pod to it's parent service.
type ServiceFWD interface {
	String() string
	SyncPodForwards(bool)
}

type HostFileWithLock struct {
	Hosts *txeh.Hosts
	sync.Mutex
}

type HostsParams struct {
	localServiceName string
	nsServiceName    string
	fullServiceName  string
	svcServiceName   string
}

type PortForwardOpts struct {
	Out            *fwdpub.Publisher
	Config         *restclient.Config
	ClientSet      *kubernetes.Clientset
	RESTClient     *restclient.RESTClient
	Context        string
	Namespace      string
	Service        string
	ServiceFwd     ServiceFWD
	PodName        string
	PodPort        string
	LocalIp        net.IP
	LocalPort      string
	Hostfile       *HostFileWithLock
	ShortName      bool
	Domain         string
	HostsParams    *HostsParams
	ManualStopChan chan struct{} // Send a signal on this to stop the portforwarding
	DoneChan       chan struct{} // Listen on this channel for when the shutdown is completed.
}

// PortForward does the portforward for a single pod.
// It is a blocking call and will return when an error occured of after a cancellation signal has been received.
func (pfo *PortForwardOpts) PortForward() error {
	defer close(pfo.DoneChan)

	transport, upgrader, err := spdy.RoundTripperFor(pfo.Config)
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
	downstreamStopChannel := make(chan struct{}) // TODO: can this be the same as pfStopChannel?

	localNamedEndPoint := fmt.Sprintf("%s:%s", pfo.Service, pfo.LocalPort)

	pfo.BuildTheHostsParams()
	pfo.AddHosts()

	// Wait until the stop signal is received from above
	go func() {
		<-pfo.ManualStopChan
		close(downstreamStopChannel)
		pfo.removeHosts()
		pfo.removeInterfaceAlias()
		close(pfStopChannel)

	}()

	// Waiting until the pod is runnning
	pod, err := pfo.WaitUntilPodRunning(downstreamStopChannel)
	if err != nil {
		pfo.Stop()
		return err
	} else if pod == nil {
		// if err is not nil but pod is nil
		// mean service deleted but pod is not runnning.
		// No error, just return
		pfo.Stop()
		return nil
	}

	// Listen for pod is deleted
	go pfo.ListenUntilPodDeleted(downstreamStopChannel, pod)

	p := pfo.Out.MakeProducer(localNamedEndPoint)

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, req.URL())

	var address []string
	if pfo.LocalIp != nil {
		address = []string{pfo.LocalIp.To4().String(), pfo.LocalIp.To16().String()}
	} else {
		address = []string{"localhost"}
	}

	fw, err := portforward.NewOnAddresses(dialer, address, fwdPorts, pfStopChannel, make(chan struct{}), &p, &p)
	if err != nil {
		pfo.Stop()
		return err
	}

	// Blocking call
	if err = fw.ForwardPorts(); err != nil {
		pfo.Stop()
		return err
	}

	return nil
}

// this method to build the HostsParams
func (pfo *PortForwardOpts) BuildTheHostsParams() {
	pfo.HostsParams = &HostsParams{}
	localServiceName := pfo.Service
	nsServiceName := pfo.Service + "." + pfo.Namespace
	fullServiceName := fmt.Sprintf("%s.%s.svc.cluster.%s", pfo.Service, pfo.Namespace, pfo.Context)
	svcServiceName := fmt.Sprintf("%s.%s.svc", pfo.Service, pfo.Namespace)
	pfo.HostsParams.localServiceName = localServiceName
	pfo.HostsParams.nsServiceName = nsServiceName
	pfo.HostsParams.fullServiceName = fullServiceName
	pfo.HostsParams.svcServiceName = svcServiceName
}

// this method to add hosts obj in /etc/hosts
func (pfo *PortForwardOpts) AddHosts() {

	pfo.Hostfile.Lock()
	if pfo.ShortName {
		if pfo.Domain != "" {
			pfo.Hostfile.Hosts.RemoveHost(pfo.HostsParams.localServiceName + "." + pfo.Domain)
			pfo.Hostfile.Hosts.AddHost(pfo.LocalIp.String(), pfo.HostsParams.localServiceName+"."+pfo.Domain)
		}
		pfo.Hostfile.Hosts.RemoveHost(pfo.HostsParams.localServiceName)
		pfo.Hostfile.Hosts.AddHost(pfo.LocalIp.String(), pfo.HostsParams.localServiceName)
	}

	pfo.Hostfile.Hosts.RemoveHost(pfo.HostsParams.fullServiceName)
	pfo.Hostfile.Hosts.AddHost(pfo.LocalIp.String(), pfo.HostsParams.fullServiceName)

	pfo.Hostfile.Hosts.RemoveHost(pfo.HostsParams.svcServiceName)
	pfo.Hostfile.Hosts.AddHost(pfo.LocalIp.String(), pfo.HostsParams.svcServiceName)

	if pfo.Domain != "" {
		pfo.Hostfile.Hosts.RemoveHost(pfo.HostsParams.nsServiceName + "." + pfo.Domain)
		pfo.Hostfile.Hosts.AddHost(pfo.LocalIp.String(), pfo.HostsParams.nsServiceName+"."+pfo.Domain)
	}
	pfo.Hostfile.Hosts.RemoveHost(pfo.HostsParams.nsServiceName)
	pfo.Hostfile.Hosts.AddHost(pfo.LocalIp.String(), pfo.HostsParams.nsServiceName)
	err := pfo.Hostfile.Hosts.Save()
	if err != nil {
		log.Error("Error saving hosts file", err)
	}
	pfo.Hostfile.Unlock()
}

// this method to remove hosts obj in /etc/hosts
func (pfo *PortForwardOpts) removeHosts() {
	// we should lock the pfo.Hostfile here
	// because sometimes other goroutine write the *txeh.Hosts
	pfo.Hostfile.Lock()
	// other applications or process may have written to /etc/hosts
	// since it was originally updated.
	err := pfo.Hostfile.Hosts.Reload()
	if err != nil {
		log.Error("Unable to reload /etc/hosts: " + err.Error())
		return
	}

	if pfo.Domain != "" {
		// fmt.Printf("removeHost: %s\r\n", (pfo.HostsParams.localServiceName + "." + pfo.Domain))
		// fmt.Printf("removeHost: %s\r\n", (pfo.HostsParams.nsServiceName + "." + pfo.Domain))
		pfo.Hostfile.Hosts.RemoveHost(pfo.HostsParams.localServiceName + "." + pfo.Domain)
		pfo.Hostfile.Hosts.RemoveHost(pfo.HostsParams.nsServiceName + "." + pfo.Domain)
	}
	// fmt.Printf("removeHost: %s\r\n", pfo.HostsParams.localServiceName)
	// fmt.Printf("removeHost: %s\r\n", pfo.HostsParams.nsServiceName)
	pfo.Hostfile.Hosts.RemoveHost(pfo.HostsParams.localServiceName)
	pfo.Hostfile.Hosts.RemoveHost(pfo.HostsParams.nsServiceName)
	// fmt.Printf("removeHost: %s\r\n", pfo.HostsParams.fullServiceName)
	pfo.Hostfile.Hosts.RemoveHost(pfo.HostsParams.fullServiceName)
	pfo.Hostfile.Hosts.RemoveHost(pfo.HostsParams.svcServiceName)

	// fmt.Printf("Delete Host And Save !\r\n")
	err = pfo.Hostfile.Hosts.Save()
	if err != nil {
		log.Errorf("Error saving /etc/hosts: %s\n", err.Error())
	}
	pfo.Hostfile.Unlock()
}

func (pfo *PortForwardOpts) removeInterfaceAlias() {
	fwdnet.RemoveInterfaceAlias(pfo.LocalIp)
}

// Waiting for the pod running
func (pfo *PortForwardOpts) WaitUntilPodRunning(stopChannel <-chan struct{}) (*v1.Pod, error) {
	pod, err := pfo.ClientSet.CoreV1().Pods(pfo.Namespace).Get(pfo.PodName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	if pod.Status.Phase == v1.PodRunning {
		return pod, nil
	}

	watcher, err := pfo.ClientSet.CoreV1().Pods(pfo.Namespace).Watch(metav1.SingleObject(pod.ObjectMeta))
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

// listen for pod is deleted
func (pfo *PortForwardOpts) ListenUntilPodDeleted(stopChannel <-chan struct{}, pod *v1.Pod) {

	watcher, err := pfo.ClientSet.CoreV1().Pods(pfo.Namespace).Watch(metav1.SingleObject(pod.ObjectMeta))
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
			log.Warnf("Pod %s deleted, resyncing the %s service pods.", pod.ObjectMeta.Name, pfo.ServiceFwd)
			pfo.ServiceFwd.SyncPodForwards(false)
			return
		}
	}
}

// Stop sends the shutdown signal to the portforwarding process.
// In case the shutdownsignal was already given before, this is a no-op.
func (pfo *PortForwardOpts) Stop() {
	select {
	case <-pfo.DoneChan:
		return
	case <-pfo.ManualStopChan:
		return
	default:
	}
	close(pfo.ManualStopChan)
}
