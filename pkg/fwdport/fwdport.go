package fwdport

import (
	"errors"
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
	Out         *fwdpub.Publisher
	Config      *restclient.Config
	ClientSet   *kubernetes.Clientset
	RESTClient  *restclient.RESTClient
	Context     string
	Namespace   string
	Service     string
	PodName     string
	PodPort     string
	LocalIp     net.IP
	LocalPort   string
	Hostfile    *HostFileWithLock
	ExitOnFail  bool
	ShortName   bool
	Remote      bool
	Domain      string
	HostsParams *HostsParams
	stopSignals chan os.Signal
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

	stopChannel := make(chan struct{}, 1)
	readyChannel := make(chan struct{})

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)
	pfo.stopSignals = signals
	defer signal.Stop(signals)

	localNamedEndPoint := fmt.Sprintf("%s:%s", pfo.Service, pfo.LocalPort)

	pfo.BuildTheHostsParams()
	pfo.AddHosts()

	go func() {
		<-signals
		if stopChannel != nil {
			pfo.removeHosts()
			close(stopChannel)
		}
	}()

	p := pfo.Out.MakeProducer(localNamedEndPoint)

	fmt.Printf("%+v\r\n", *req.URL())
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, "POST", req.URL())

	var address []string
	if pfo.LocalIp != nil {
		address = []string{pfo.LocalIp.To4().String(), pfo.LocalIp.To16().String()}
	} else {
		address = []string{"localhost"}
	}

	fw, err := portforward.NewOnAddresses(dialer, address, fwdPorts, stopChannel, readyChannel, &p, &p)
	if err != nil {
		pfo.Stop()
		return err
	}

	err = pfo.WaitForPodRunning()
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
			fmt.Printf("removeHost: %s\r\n", (pfo.HostsParams.localServiceName + "." + pfo.Domain))
			fmt.Printf("removeHost: %s\r\n", (pfo.HostsParams.nsServiceName + "." + pfo.Domain))
			pfo.Hostfile.Hosts.RemoveHost(pfo.HostsParams.localServiceName + "." + pfo.Domain)
			pfo.Hostfile.Hosts.RemoveHost(pfo.HostsParams.nsServiceName + "." + pfo.Domain)
		}
		fmt.Printf("removeHost: %s\r\n", pfo.HostsParams.localServiceName)
		fmt.Printf("removeHost: %s\r\n", pfo.HostsParams.nsServiceName)
		pfo.Hostfile.Hosts.RemoveHost(pfo.HostsParams.localServiceName)
		pfo.Hostfile.Hosts.RemoveHost(pfo.HostsParams.nsServiceName)
	}
	fmt.Printf("removeHost: %s\r\n", pfo.HostsParams.fullServiceName)
	pfo.Hostfile.Hosts.RemoveHost(pfo.HostsParams.fullServiceName)

	fmt.Printf("Delete Host And Save !\r\n")
	err = pfo.Hostfile.Hosts.Save()
	if err != nil {
		log.Errorf("Error saving /etc/hosts: %s\n", err.Error())
	}
	pfo.Hostfile.Unlock()
}

// Waiting for the pod running
func (pfo *PortForwardOpts) WaitForPodRunning() error {
	// default timetout settings is 5s * 60 == 300s
	// TODO: change it to watch for pod running
	for i := 0; i < 60; i++ {
		pod, err := pfo.ClientSet.CoreV1().Pods(pfo.Namespace).Get(pfo.PodName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if pod.Status.Phase != "running" {
			time.Sleep(5 * time.Second)
		} else {
			return nil
		}
	}
	return errors.New("Error: waiting for pod running time out!")
}

// this method to stop PortForward for the pfo
func (pfo *PortForwardOpts) Stop() {
	signal.Stop(pfo.stopSignals)
}
