package fwdport

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"

	"github.com/prometheus/common/log"

	"github.com/txn2/txeh"

	"github.com/txn2/kubefwd/pkg/fwdpub"

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
	Hostfile   *txeh.Hosts
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

	localNamedEndPoint := fmt.Sprintf("%s:%s", pfo.Service, pfo.LocalPort)
	localHost := pfo.Service
	fullLocalHost := pfo.Service + "." + pfo.Namespace + ".svc.cluster.local"
	nsLocalHost := pfo.Service + "." + pfo.Namespace

	if pfo.ShortName {
		pfo.Hostfile.AddHost(pfo.LocalIp.String(), pfo.Service)
	}

	pfo.Hostfile.AddHost(pfo.LocalIp.String(), fullLocalHost)
	pfo.Hostfile.AddHost(pfo.LocalIp.String(), nsLocalHost)

	err = pfo.Hostfile.Save()
	if err != nil {
		return err
	}

	go func() {
		<-signals
		if stopChannel != nil {
			pfo.Hostfile.RemoveHost(localHost)
			pfo.Hostfile.RemoveHost(nsLocalHost)
			pfo.Hostfile.RemoveHost(fullLocalHost)
			err = pfo.Hostfile.Save()
			if err != nil {
				log.Error("Error saving hosts file", err)
			}

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
