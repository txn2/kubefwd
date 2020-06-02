package fwdport

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
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

type HostFileWithLock struct {
	Hosts *txeh.Hosts
	sync.Mutex
}

// HostsParams holds the DNS entries which are associated with a portforward
// The struct holds the non-fully qualified entry for the fullServiceName, this one is 'duplicated' to a fully-qualified entry in the hostsfile
type HostsParams struct {
	localServiceName string // myService
	nsServiceName    string // myService.myNamespace
	fullServiceName  string // myService.myNamespace.svc.cluster.local (if pfo.Remote: myService.myNamespace.svc.cluster.local.myContextName)
	svcServiceName   string // myService.myNamespace.svc
}

type PortForwardOpts struct {
	Out               *fwdpub.Publisher
	Config            *restclient.Config
	ClientSet         *kubernetes.Clientset
	RESTClient        *restclient.RESTClient
	ServiceOperator   OperatorInterface
	Context           string
	Namespace         string
	Service           string
	NativeServiceName string
	PodName           string
	PodPort           string
	LocalIp           net.IP
	LocalPort         string
	Hostfile          *HostFileWithLock
	ExitOnFail        bool
	ShortName         bool
	Remote            bool
	Domain            string
	HostsParams       *HostsParams
	ManualStopChan    chan struct{}
}

type OperatorInterface interface {
	ForwardService(svc *v1.Service)
	UnForwardService(svc *v1.Service)
}

func (pfo *PortForwardOpts) PortForward() error {

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

	restClient := pfo.RESTClient
	// if need to set timeout, set it here.
	// restClient.Client.Timeout = 32
	req := restClient.Post().
		Resource("pods").
		Namespace(pfo.Namespace).
		Name(pfo.PodName).
		SubResource("portforward")

	pfStopChannel := make(chan struct{}, 1)
	pfReadyChannel := make(chan struct{})

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)
	pfo.ManualStopChan = make(chan struct{})
	signalChan := make(chan struct{})

	defer signal.Stop(signals)

	localNamedEndPoint := fmt.Sprintf("%s:%s", pfo.Service, pfo.LocalPort)

	pfo.BuildTheHostsParams()
	pfo.AddHosts()

	go func() {
		select {
		case <-signals:
		case <-pfo.ManualStopChan:
		}
		close(signalChan)
		if pfStopChannel != nil {
			pfo.removeHosts()
			pfo.removeInterfaceAlias()
			close(pfStopChannel)
		}
	}()

	p := pfo.Out.MakeProducer(localNamedEndPoint)

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, "POST", req.URL())

	var address []string
	if pfo.LocalIp != nil {
		address = []string{pfo.LocalIp.To4().String(), pfo.LocalIp.To16().String()}
	} else {
		address = []string{"localhost"}
	}

	// Waiting for the pod is runnning
	pod, err := pfo.WaitForPodRunning(signalChan)
	if err != nil {
		pfo.Stop()
		return err
	}
	// if err is not nil but pod is nil
	// mean service deleted but pod is not runnning.
	// the pfo.Stop() has called yet, needn't call again
	if pod == nil {
		return nil
	}

	// Listen for pod is deleted
	go pfo.ListenUntilPodDeleted(signalChan, pod)

	fw, err := portforward.NewOnAddresses(dialer, address, fwdPorts, pfStopChannel, pfReadyChannel, &p, &p)
	if err != nil {
		pfo.Stop()
		return err
	}

	err = fw.ForwardPorts()
	if err != nil {
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
	fullServiceName := fmt.Sprintf("%s.%s.svc.cluster.local", pfo.Service, pfo.Namespace)
	svcServiceName := fmt.Sprintf("%s.%s.svc", pfo.Service, pfo.Namespace)
	if pfo.Remote {
		fullServiceName = fmt.Sprintf("%s.%s.svc.cluster.%s", pfo.Service, pfo.Namespace, pfo.Context)
	}
	pfo.HostsParams.localServiceName = localServiceName
	pfo.HostsParams.nsServiceName = nsServiceName
	pfo.HostsParams.fullServiceName = fullServiceName
	pfo.HostsParams.svcServiceName = svcServiceName
}

// this method to add hosts obj in /etc/hosts
func (pfo *PortForwardOpts) AddHosts() {
	pfo.Hostfile.Lock()
	defer pfo.Hostfile.Unlock()

	localIpAsString := pfo.LocalIp.String()

	if pfo.Remote {
		pfo.Hostfile.Hosts.RemoveHost(pfo.HostsParams.fullServiceName)
		pfo.Hostfile.Hosts.RemoveHost(pfo.HostsParams.svcServiceName)

		// FQDN fullServiceName
		pfo.Hostfile.Hosts.RemoveHost(pfo.HostsParams.fullServiceName + ".")

		if pfo.Domain != "" {
			pfo.Hostfile.Hosts.AddHost(localIpAsString, pfo.Service+"."+pfo.Domain)
		}
		pfo.Hostfile.Hosts.AddHost(localIpAsString, pfo.Service)

	} else {

		if pfo.ShortName {
			if pfo.Domain != "" {
				pfo.Hostfile.Hosts.RemoveHost(pfo.HostsParams.localServiceName + "." + pfo.Domain)
				pfo.Hostfile.Hosts.AddHost(localIpAsString, pfo.HostsParams.localServiceName+"."+pfo.Domain)
			}

			pfo.Hostfile.Hosts.RemoveHost(pfo.HostsParams.localServiceName)
			pfo.Hostfile.Hosts.AddHost(localIpAsString, pfo.HostsParams.localServiceName)
		}

		// Non-FQDN fullServiceName
		pfo.Hostfile.Hosts.RemoveHost(pfo.HostsParams.fullServiceName)
		pfo.Hostfile.Hosts.AddHost(localIpAsString, pfo.HostsParams.fullServiceName)

		// FQDN fullServiceName
		pfo.Hostfile.Hosts.RemoveHost(pfo.HostsParams.fullServiceName + ".")
		pfo.Hostfile.Hosts.AddHost(localIpAsString, pfo.HostsParams.fullServiceName+".")

		pfo.Hostfile.Hosts.RemoveHost(pfo.HostsParams.svcServiceName)
		pfo.Hostfile.Hosts.AddHost(localIpAsString, pfo.HostsParams.svcServiceName)

		if pfo.Domain != "" {
			pfo.Hostfile.Hosts.RemoveHost(pfo.HostsParams.nsServiceName + "." + pfo.Domain)
			pfo.Hostfile.Hosts.AddHost(localIpAsString, pfo.HostsParams.nsServiceName+"."+pfo.Domain)
		}

		pfo.Hostfile.Hosts.RemoveHost(pfo.HostsParams.nsServiceName)
		pfo.Hostfile.Hosts.AddHost(localIpAsString, pfo.HostsParams.nsServiceName)
	}
	err := pfo.Hostfile.Hosts.Save()
	if err != nil {
		log.Error("Error saving hosts file", err)
	}
}

// this method to remove hosts obj in /etc/hosts
func (pfo *PortForwardOpts) removeHosts() {
	// we should lock the pfo.Hostfile here
	// because sometimes other goroutine write the *txeh.Hosts
	pfo.Hostfile.Lock()
	defer pfo.Hostfile.Unlock()
	// other applications or process may have written to /etc/hosts
	// since it was originally updated.
	err := pfo.Hostfile.Hosts.Reload()
	if err != nil {
		log.Error("Unable to reload /etc/hosts: " + err.Error())
		return
	}

	if !pfo.Remote {
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
	}
	// fmt.Printf("removeHost: %s\r\n", pfo.HostsParams.fullServiceName)

	// Non-FQDN fullServiceName
	pfo.Hostfile.Hosts.RemoveHost(pfo.HostsParams.fullServiceName)

	// FQDN fullServiceName
	pfo.Hostfile.Hosts.RemoveHost(pfo.HostsParams.fullServiceName + ".")

	pfo.Hostfile.Hosts.RemoveHost(pfo.HostsParams.svcServiceName)

	// fmt.Printf("Delete Host And Save !\r\n")
	err = pfo.Hostfile.Hosts.Save()
	if err != nil {
		log.Errorf("Error saving /etc/hosts: %s\n", err.Error())
	}
}

func (pfo *PortForwardOpts) removeInterfaceAlias() {
	fwdnet.RemoveInterfaceAlias(pfo.LocalIp)
}

// Waiting for the pod running
func (pfo *PortForwardOpts) WaitForPodRunning(signalsChan chan struct{}) (*v1.Pod, error) {
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
	RunningChannel := make(chan struct{})

	defer close(RunningChannel)

	// if the os.signal (we enter the Ctrl+C)
	// or ManualStop (service delete or some thing wrong)
	// or RunningChannel channel (the watch for pod runnings is done)
	// or timeout after 300s
	// we'll stop the watcher
	// TODO: change the 300s timeout to custom settings.
	go func() {
		defer watcher.Stop()
		select {
		case <-signalsChan:
		case <-RunningChannel:
		case <-pfo.ManualStopChan:
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
func (pfo *PortForwardOpts) ListenUntilPodDeleted(signalsChan chan struct{}, pod *v1.Pod) {

	watcher, err := pfo.ClientSet.CoreV1().Pods(pfo.Namespace).Watch(metav1.SingleObject(pod.ObjectMeta))
	if err != nil {
		return
	}

	go func() {
		defer watcher.Stop()
		select {
		case <-pfo.ManualStopChan:
		case <-signalsChan:
		}
	}()

	// watcher until the pod is deleted,
	// then remove forward service and forward service again.
	// TODO:
	// now if the service is normal service it's ok, the firstPod
	// deleted event will forward service again.
	// but if the service is headless service, all pod will be forward.
	// so every pod deleted event will trigger a forward service again function.
	// we'll get a pure /etc/hosts few times after.
	// maybe we can fix here later.
	for {
		event, ok := <-watcher.ResultChan()
		if !ok {
			break
		}
		switch event.Type {
		case watch.Deleted:
			log.Warnf("%s Pod deleted, restart the %s service portforward.", pod.ObjectMeta.Name, pfo.NativeServiceName)

			svc, err := pfo.ClientSet.CoreV1().Services(pfo.Namespace).Get(pfo.NativeServiceName, metav1.GetOptions{})
			if err != nil {
				return
			}

			pfo.ServiceOperator.UnForwardService(svc)
			pfo.ServiceOperator.ForwardService(svc)
			return
		}
	}
}

// this method to stop PortForward for the pfo
func (pfo *PortForwardOpts) Stop() {
	close(pfo.ManualStopChan)
}
