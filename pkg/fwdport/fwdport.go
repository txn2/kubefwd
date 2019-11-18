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
	"github.com/txn2/kubefwd/pkg/fwdpub"
	"github.com/txn2/txeh"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

type HostFileWithLock struct {
	Hosts *txeh.Hosts
	sync.Mutex
}

type HostsParams struct {
	localServiceName string
	nsServiceName    string
	fullServiceName  string
}

type PortForwardOpts struct {
	Out               *fwdpub.Publisher
	Config            *restclient.Config
	ClientSet         *kubernetes.Clientset
	RESTClient        *restclient.RESTClient
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

	defer signal.Stop(signals)

	localNamedEndPoint := fmt.Sprintf("%s:%s", pfo.Service, pfo.LocalPort)

	pfo.BuildTheHostsParams()
	pfo.AddHosts()

	go func() {
		select {
		case <-signals:
		case <-pfo.ManualStopChan:
		}
		if pfStopChannel != nil {
			pfo.removeHosts()
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

	err = pfo.WaitForPodRunning(signals)
	if err != nil {
		pfo.Stop()
		return err
	}

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
	if pfo.Remote {
		fullServiceName = fmt.Sprintf("%s.%s.svc.cluster.%s", pfo.Service, pfo.Namespace, pfo.Context)
	}
	pfo.HostsParams.localServiceName = localServiceName
	pfo.HostsParams.nsServiceName = nsServiceName
	pfo.HostsParams.fullServiceName = fullServiceName
	return
}

// this method to add hosts obj in /etc/hosts
func (pfo *PortForwardOpts) AddHosts() {

	pfo.Hostfile.Lock()
	if pfo.Remote {

		pfo.Hostfile.Hosts.RemoveHost(pfo.HostsParams.fullServiceName)
		if pfo.Domain != "" {
			pfo.Hostfile.Hosts.AddHost(pfo.LocalIp.String(), pfo.Service+"."+pfo.Domain)
		}
		pfo.Hostfile.Hosts.AddHost(pfo.LocalIp.String(), pfo.Service)

	} else {

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
		if pfo.Domain != "" {
			pfo.Hostfile.Hosts.RemoveHost(pfo.HostsParams.nsServiceName + "." + pfo.Domain)
			pfo.Hostfile.Hosts.AddHost(pfo.LocalIp.String(), pfo.HostsParams.nsServiceName+"."+pfo.Domain)
		}
		pfo.Hostfile.Hosts.RemoveHost(pfo.HostsParams.nsServiceName)
		pfo.Hostfile.Hosts.AddHost(pfo.LocalIp.String(), pfo.HostsParams.nsServiceName)

	}
	err := pfo.Hostfile.Hosts.Save()
	if err != nil {
		log.Error("Error saving hosts file", err)
	}
	pfo.Hostfile.Unlock()
	return
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

	if pfo.Remote == false {
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
	pfo.Hostfile.Hosts.RemoveHost(pfo.HostsParams.fullServiceName)

	// fmt.Printf("Delete Host And Save !\r\n")
	err = pfo.Hostfile.Hosts.Save()
	if err != nil {
		log.Errorf("Error saving /etc/hosts: %s\n", err.Error())
	}
	pfo.Hostfile.Unlock()
}

// Waiting for the pod running
func (pfo *PortForwardOpts) WaitForPodRunning(signals chan os.Signal) error {
	pod, err := pfo.ClientSet.CoreV1().Pods(pfo.Namespace).Get(pfo.PodName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	if pod.Status.Phase == v1.PodRunning {
		return nil
	}

	watcher, err := pfo.ClientSet.CoreV1().Pods(pfo.Namespace).Watch(metav1.SingleObject(pod.ObjectMeta))
	if err != nil {
		return err
	}
	RunningChannel := make(chan struct{})

	defer close(RunningChannel)

	// if the os.signal (we enter the Ctrl+C)
	// or RunningChannel channel (the watch for pod runnings is done)
	// or timeout after 300s
	// we'll stop the watcher
	// TODO: change the 300s timeout to custom settings.
	go func() {
		defer watcher.Stop()
		select {
		case <-signals:
		case <-RunningChannel:
		case <-time.After(time.Second * 300):
		}
	}()

	// watcher until the pod status is running
	for {
		event := <-watcher.ResultChan()
		// switch event.Type {
		// case watch.Deleted:
		// 	return fmt.Errorf("%s/%s is deleted!", pfo.Namespace, pfo.PodName)
		// }
		if event.Object != nil {
			changedPod := event.Object.(*v1.Pod)
			if changedPod.Status.Phase == v1.PodRunning {
				return nil
			}
		}
		time.Sleep(time.Second * 3)
	}
}

// this method to stop PortForward for the pfo
func (pfo *PortForwardOpts) Stop() {
	close(pfo.ManualStopChan)
}
