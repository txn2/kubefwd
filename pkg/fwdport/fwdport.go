package fwdport

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/txn2/kubefwd/pkg/fwdpub"
	"github.com/txn2/kubefwd/pkg/portforward"
	"github.com/txn2/txeh"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/transport/spdy"
)

type PortForwardOpts struct {
	Out        *fwdpub.Publisher
	Config     *restclient.Config
	ClientSet  *kubernetes.Clientset
	Context    string
	Namespace  string
	Service    string
	PodName    string
	PodPort    string
	LocalIp    net.IP
	LocalPort  string
	Hostfile   *txeh.Hosts
	ExitOnFail bool
	ShortName  bool
	Remote     bool
	Domain     string
}

func PortForward(pfo *PortForwardOpts) error {

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

	restClient := pfo.ClientSet.RESTClient()
	req := restClient.Post().
		Resource("pods").
		Namespace(pfo.Namespace).
		Name(pfo.PodName).
		SubResource("portforward")

	u := url.URL{
		Scheme:   req.URL().Scheme,
		Host:     req.URL().Host,
		Path:     buildPath(req),
		RawQuery: "timeout=32s",
	}

	stopChannel := make(chan struct{}, 1)
	readyChannel := make(chan struct{})

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)
	defer signal.Stop(signals)

	localNamedEndPoint := fmt.Sprintf("%s:%s", pfo.Service, pfo.LocalPort)

	localServiceName := pfo.Service
	nsServiceName := pfo.Service + "." + pfo.Namespace
	fullServiceName := fmt.Sprintf("%s.%s.svc.cluster.local", pfo.Service, pfo.Namespace)

	if pfo.Remote {
		fullServiceName = fmt.Sprintf("%s.%s.svc.cluster.%s", pfo.Service, pfo.Namespace, pfo.Context)

		pfo.Hostfile.RemoveHost(fullServiceName)
		if pfo.Domain != "" {
			pfo.Hostfile.AddHost(pfo.LocalIp.String(), pfo.Service+"."+pfo.Domain)
		}
		pfo.Hostfile.AddHost(pfo.LocalIp.String(), pfo.Service)

	} else {

		if pfo.ShortName {
			if pfo.Domain != "" {
				pfo.Hostfile.RemoveHost(localServiceName + "." + pfo.Domain)
				pfo.Hostfile.AddHost(pfo.LocalIp.String(), localServiceName+"."+pfo.Domain)
			}
			pfo.Hostfile.RemoveHost(localServiceName)
			pfo.Hostfile.AddHost(pfo.LocalIp.String(), localServiceName)
		}

		pfo.Hostfile.RemoveHost(fullServiceName)
		pfo.Hostfile.AddHost(pfo.LocalIp.String(), fullServiceName)
		if pfo.Domain != "" {
			pfo.Hostfile.RemoveHost(nsServiceName + "." + pfo.Domain)
			pfo.Hostfile.AddHost(pfo.LocalIp.String(), nsServiceName+"."+pfo.Domain)
		}
		pfo.Hostfile.RemoveHost(nsServiceName)
		pfo.Hostfile.AddHost(pfo.LocalIp.String(), nsServiceName)

	}

	cleanupHostfile := func() {
		// other applications or process may have written to /etc/hosts
		// since it was originally updated.
		err := pfo.Hostfile.Reload()
		if err != nil {
			log.Error("Unable to reload /etc/hosts: " + err.Error())
			return
		}

		if pfo.Remote == false {
			if pfo.Domain != "" {
				pfo.Hostfile.RemoveHost(localServiceName + "." + pfo.Domain)
				pfo.Hostfile.RemoveHost(nsServiceName + "." + pfo.Domain)
			}
			pfo.Hostfile.RemoveHost(localServiceName)
			pfo.Hostfile.RemoveHost(nsServiceName)
		}
		pfo.Hostfile.RemoveHost(fullServiceName)

		err = pfo.Hostfile.Save()
		if err != nil {
			log.Errorf("Error saving /etc/hosts: %s\n", err.Error())
		}
	}

	go func() {
		<-signals
		if stopChannel != nil {
			cleanupHostfile()
			close(stopChannel)
		}
	}()

	p := pfo.Out.MakeProducer(localNamedEndPoint)

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, "POST", &u)

	fw, err := portforward.New(dialer, fwdPorts, stopChannel, readyChannel, &p, &p)
	if err != nil {
		signal.Stop(signals)
		cleanupHostfile()
		return err
	}

	fw.LocalIp(pfo.LocalIp)

	err = fw.ForwardPorts()
	if err != nil {
		signal.Stop(signals)
		cleanupHostfile()
		return err
	}

	return nil
}

func buildPath(req *restclient.Request) string {
	splitted := strings.Split(req.URL().Path, "/namespaces")
	path := splitted[0] + "/api/v1/namespaces" + splitted[1]
	return path
}
