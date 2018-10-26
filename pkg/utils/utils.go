/*
Copyright 2018 Craig Johnston <cjimti@gmail.com>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package utils

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"

	"errors"

	"os/exec"

	"log"

	"strconv"

	"github.com/cbednarski/hostess"
	"github.com/spf13/cobra"
	"github.com/txn2/kubefwd/pkg/portforward"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/transport/spdy"
)

type Publisher struct {
	Output        bool
	PublisherName string
	ProducerName  string
}

func (p *Publisher) MakeProducer(producer string) Publisher {
	p.ProducerName = producer
	return *p
}

func (p *Publisher) Write(b []byte) (int, error) {
	readLine := string(b)
	strings.TrimSuffix(readLine, "\n")

	if p.Output {
		fmt.Printf("%s, %s, %s", p.PublisherName, p.ProducerName, readLine)
	}
	return 0, nil
}

func K8sConfig(cmd *cobra.Command) *restclient.Config {
	// use the current context in kubeconfig
	cfgFilePath := cmd.Flag("kubeconfig").Value.String()
	namespace := cmd.Flag("namespace").Value.String()

	if namespace == "" {
		namespace = "default"
	}

	if cfgFilePath == "" {
		fmt.Println("No config found. Use --kubeconfig to specify one")
		os.Exit(1)
	}

	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", cfgFilePath)
	if err != nil {
		panic(err.Error())
	}

	return config
}

type PortForwardOpts struct {
	Out       *Publisher
	Config    *restclient.Config
	ClientSet *kubernetes.Clientset
	Namespace string
	Service   string
	PodName   string
	PodPort   string
	LocalIp   net.IP
	LocalPort string
	Hostfile  *hostess.Hostfile
}

func PortForward(wg *sync.WaitGroup, pfo *PortForwardOpts) {

	defer wg.Done()

	transport, upgrader, err := spdy.RoundTripperFor(pfo.Config)
	if err != nil {
		fmt.Printf("Error: %s\n", err.Error())
		os.Exit(1)
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
	nsLocalHost := pfo.Service + "." + pfo.Namespace + ".svc.cluster.local"

	hostname := hostess.MustHostname(localHost, pfo.LocalIp.String(), true)
	pfo.Hostfile.Hosts.RemoveDomain(hostname.Domain)
	pfo.Hostfile.Hosts.Add(hostname)

	nsHostname := hostess.MustHostname(nsLocalHost, pfo.LocalIp.String(), true)
	pfo.Hostfile.Hosts.RemoveDomain(nsHostname.Domain)
	pfo.Hostfile.Hosts.Add(nsHostname)

	err = pfo.Hostfile.Save()
	if err != nil {
		fmt.Println("Cannot save hostfile.")
		os.Exit(1)
	}

	go func() {
		<-signals
		if stopChannel != nil {
			fmt.Printf("Stoped forwarding %s and removing %s from hosts.\n", localIpEndPoint, localHost)
			pfo.Hostfile.Hosts.RemoveDomain(localHost)
			pfo.Hostfile.Hosts.RemoveDomain(nsLocalHost)

			close(stopChannel)
		}
	}()

	p := pfo.Out.MakeProducer(localNamedEndPoint)
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, "POST", &u)

	fw, err := portforward.New(dialer, fwdPorts, stopChannel, readyChannel, &p, &p)
	if err != nil {
		fmt.Printf("portforward.New Error: %s\n", err.Error())
		os.Exit(1)
	}

	fw.LocalIp(pfo.LocalIp)

	err = fw.ForwardPorts()
	if err != nil {
		fmt.Printf("fw.ForwardPorts Error: %s\n", err.Error())
		os.Exit(1)
	}
}

func ReadyInterface(a byte, b byte, c byte, d int, port string) (net.IP, int, error) {

	ip := net.IPv4(a, b, c, byte(d))

	// lo means we are probably on linux and not mac
	_, err := net.InterfaceByName("lo")
	if err == nil {
		// if no error then check to see if the ip:port are in use
		_, err := net.Dial("tcp", ip.String()+":"+port)
		if err != nil {
			return ip, d + 1, nil
		}

		return ip, d + 1, errors.New("ip and port are in use")
	}

	for i := d; i < 255; i++ {

		ip = net.IPv4(a, b, c, byte(i))

		iface, err := net.InterfaceByName("lo0")
		if err != nil {
			return net.IP{}, i, err
		}

		addrs, err := iface.Addrs()
		if err != nil {
			return net.IP{}, i, err
		}

		// check the addresses already assigned to the interface
		for _, addr := range addrs {

			// found a match
			if addr.String() == ip.String()+"/8" {
				// found ip, now check for unused port
				conn, err := net.Dial("tcp", ip.String()+":"+port)
				if err != nil {
					return net.IPv4(a, b, c, byte(i)), i + 1, nil
				}
				conn.Close()
			}
		}

		// ip is not in the list of addrs for iface
		cmd := "ifconfig"
		args := []string{"lo0", "alias", ip.String(), "up"}
		if err := exec.Command(cmd, args...).Run(); err != nil {
			fmt.Println("Cannot ifconfig lo0 alias " + ip.String() + " up")
			os.Exit(1)
		}

		conn, err := net.Dial("tcp", ip.String()+":"+port)
		if err != nil {
			return net.IPv4(a, b, c, byte(i)), i + 1, nil
		}
		conn.Close()

	}

	return net.IP{}, d, errors.New("unable to find an available IP/Port")
}

func CheckRoot() bool {
	cmd := exec.Command("id", "-u")

	output, err := cmd.Output()
	if err != nil {
		log.Fatal(err)
		return false
	}

	i, err := strconv.Atoi(string(output[:len(output)-1]))
	if err != nil {
		log.Fatal(err)
		return false
	}

	if i == 0 {
		return true
	}

	return false
}
