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
	localHost := pfo.Service

	fullLocalHost := fmt.Sprintf("%s.%s.svc.cluster.local", pfo.Service, pfo.Namespace)

	if pfo.Remote {
		fullLocalHost = fmt.Sprintf("%s.%s.svc.cluster.%s", pfo.Service, pfo.Namespace, pfo.Context)
	}

	nsLocalHost := pfo.Service + "." + pfo.Namespace

	if pfo.ShortName {
		pfo.Hostfile.RemoveHost(pfo.Service)
		pfo.Hostfile.AddHost(pfo.LocalIp.String(), pfo.Service)
	}

	pfo.Hostfile.RemoveHost(fullLocalHost)
	pfo.Hostfile.RemoveHost(nsLocalHost)

	pfo.Hostfile.AddHost(pfo.LocalIp.String(), fullLocalHost)

	if pfo.Remote == false {
		pfo.Hostfile.RemoveHost(fullLocalHost)
		pfo.Hostfile.RemoveHost(nsLocalHost)

		pfo.Hostfile.AddHost(pfo.LocalIp.String(), nsLocalHost)

	}

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

func buildPath(req *restclient.Request) string {
	splitted := strings.Split(req.URL().Path, "/namespaces")
	path := splitted[0] + "/api/v1/namespaces" + splitted[1]
	return path
}
