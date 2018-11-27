package fwdport

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"

	"github.com/txn2/kubefwd/pkg/fwdpub"

	"github.com/cbednarski/hostess"
	"github.com/txn2/kubefwd/pkg/portforward"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/transport/spdy"
)

type PortForwardOpts struct {
	Out        *fwdpub.Publisher
	Config     *restclient.Config
	ClientSet  *kubernetes.Clientset
	Namespace  string
	Service    string
	PodName    string
	PodPort    string
	LocalIp    net.IP
	LocalPort  string
	Hostfile   *hostess.Hostfile
	ExitOnFail bool
	ShortName  bool
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
		Path:     "/api/v1" + req.URL().Path,
		RawQuery: "timeout=32s",
	}

	stopChannel := make(chan struct{}, 1)
	readyChannel := make(chan struct{})

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)
	defer signal.Stop(signals)

	localIpEndPoint := fmt.Sprintf("%s:%s", pfo.LocalIp.String(), pfo.LocalPort)
	localNamedEndPoint := fmt.Sprintf("%s:%s", pfo.Service, pfo.LocalPort)
	localHost := pfo.Service
	fullLocalHost := pfo.Service + "." + pfo.Namespace + ".svc.cluster.local"
	nsLocalHost := pfo.Service + "." + pfo.Namespace

	if pfo.ShortName {
		hostname := hostess.MustHostname(localHost, pfo.LocalIp.String(), true)
		pfo.Hostfile.Hosts.RemoveDomain(hostname.Domain)
		err := pfo.Hostfile.Hosts.Add(hostname)
		if err != nil {
			return err
		}
	}

	fullHostname := hostess.MustHostname(fullLocalHost, pfo.LocalIp.String(), true)
	pfo.Hostfile.Hosts.RemoveDomain(fullHostname.Domain)
	err = pfo.Hostfile.Hosts.Add(fullHostname)
	if err != nil {
		return err
	}

	nsHostname := hostess.MustHostname(nsLocalHost, pfo.LocalIp.String(), true)
	pfo.Hostfile.Hosts.RemoveDomain(nsHostname.Domain)
	err = pfo.Hostfile.Hosts.Add(nsHostname)
	if err != nil {
		return err
	}

	err = pfo.Hostfile.Save()
	if err != nil {
		return err
	}

	go func() {
		<-signals
		if stopChannel != nil {
			fmt.Printf("Stopped forwarding %s and removing %s from hosts.\n", localIpEndPoint, localHost)
			pfo.Hostfile.Hosts.RemoveDomain(localHost)
			pfo.Hostfile.Hosts.RemoveDomain(nsLocalHost)
			pfo.Hostfile.Hosts.RemoveDomain(fullLocalHost)
			close(stopChannel)
		}
	}()

	p := pfo.Out.MakeProducer(localNamedEndPoint)
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, "POST", &u)

	fw, err := portforward.New(dialer, fwdPorts, stopChannel, readyChannel, &p, &p)
	if err != nil {
		signal.Stop(signals)
		return err
	}

	fw.LocalIp(pfo.LocalIp)

	err = fw.ForwardPorts()
	if err != nil {
		signal.Stop(signals)
		return err
	}

	return nil
}
