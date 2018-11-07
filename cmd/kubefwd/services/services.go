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
package services

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/cbednarski/hostess"
	"github.com/spf13/cobra"
	"github.com/txn2/kubefwd/pkg/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	restclient "k8s.io/client-go/rest"
)

var namespaces []string

func init() {
	cfgFilePath := ""

	if home := homeDir(); home != "" {
		cfgFilePath = filepath.Join(home, ".kube", "config")
	}

	Cmd.Flags().StringP("kubeconfig", "c", cfgFilePath, "absolute path to the kubeconfig file")
	Cmd.Flags().StringSliceVarP(&namespaces, "namespace", "n", []string{}, "Specify a namespace.")
	Cmd.Flags().StringP("selector", "l", "", "Selector (label query) to filter on, supports '=', '==', and '!='.(e.g. -l key1=value1,key2=value2)")
}

var Cmd = &cobra.Command{
	Use:     "services",
	Aliases: []string{"svcs", "svc"},
	Short:   "Forward all services",
	Long:    `Forward all Kubernetes services.`,
	Run: func(cmd *cobra.Command, args []string) {

		if !utils.CheckRoot() {
			fmt.Printf(`
This program requires superuser priveleges to run. These 
priveledges are required to add IP address aliases to your 
loopback interface. Superuser priveleges are also needed 
to listen on low port numbers for these IP addresses.

Try: 
 - sudo kubefwd services (Unix)
 - Running a shell with administrator rights (Windows)

`)
			return
		}

		fmt.Println("Press [Ctrl-C] to stop forwarding.")

		// get the hostfile
		hostfile, err := utils.GetHostFile()
		if err != nil {
			log.Fatal(err)
		}

		// k8s rest config
		config := utils.K8sConfig(cmd)
		selector := cmd.Flag("selector").Value.String()

		if len(namespaces) < 1 {
			namespaces = []string{"default"}
		}

		listOptions := metav1.ListOptions{}
		if selector != "" {
			listOptions.LabelSelector = selector
		}

		// create the client set
		clientSet, err := kubernetes.NewForConfig(config)
		if err != nil {
			panic(err.Error())
		}

		wg := &sync.WaitGroup{}

		ipC := 27

		for i, namespace := range namespaces {
			err = fwdServices(FwdServiceOpts{
				Wg:           wg,
				ClientSet:    clientSet,
				Namespace:    namespace,
				ListOptions:  listOptions,
				Hostfile:     hostfile,
				ClientConfig: config,
				ShortName:    i < 1, // only use shortname for the first namespace
				IpC:          byte(ipC),
			})
			if err != nil {
				fmt.Printf("Error forwarding service: %s\n", err.Error())
			}

			ipC = ipC + 1
		}

		wg.Wait()

		fmt.Printf("\nDone...\n")
		err = hostfile.Save()
		if err != nil {
			fmt.Printf("Error saving hostfile: %s\n", err.Error())
		}
	},
}

type FwdServiceOpts struct {
	Wg           *sync.WaitGroup
	ClientSet    *kubernetes.Clientset
	Namespace    string
	ListOptions  metav1.ListOptions
	Hostfile     *hostess.Hostfile
	ClientConfig *restclient.Config
	ShortName    bool
	IpC          byte
}

func fwdServices(opts FwdServiceOpts) error {

	services, err := opts.ClientSet.CoreV1().Services(opts.Namespace).List(opts.ListOptions)
	if err != nil {
		return err
	}

	publisher := &utils.Publisher{
		PublisherName: "Services",
		Output:        false,
	}

	d := 1

	// loop through the services
	for _, svc := range services.Items {
		selector := mapToSelectorStr(svc.Spec.Selector)
		pods, err := opts.ClientSet.CoreV1().Pods(svc.Namespace).List(metav1.ListOptions{LabelSelector: selector})

		if err != nil {
			fmt.Printf("no pods found for %s: %s\n", selector, err.Error())
			continue
		}

		if len(pods.Items) < 1 {
			fmt.Printf("No pods returned for service.\n")
			continue
		}

		podName := ""
		podPort := ""
		podNamespace := ""

		localIp, ii, err := utils.ReadyInterface(127, 1, opts.IpC, d, podPort)
		d = ii

		for _, port := range svc.Spec.Ports {

			podName = pods.Items[0].Name
			podNamespace = pods.Items[0].Namespace
			podPort = port.TargetPort.String()
			localPort := strconv.Itoa(int(port.Port))

			_, err = opts.ClientSet.CoreV1().Pods(podNamespace).Get(podName, metav1.GetOptions{})
			if err != nil {
				fmt.Printf("Error getting pod: %s\n", err.Error())
				// TODO: Check for other pods?
				break // no need to check other ports if we can't ge the pod
			}

			full := ""

			if opts.ShortName != true {
				full = fmt.Sprintf(".%s.svc.cluster.local", podNamespace)
			}

			fmt.Printf("Fwd %s:%s as %s%s:%d to pod %s:%s\n",
				localIp.String(),
				localPort,
				svc.Name,
				full,
				port.Port,
				podName,
				podPort,
			)

			opts.Wg.Add(1)
			pfo := &utils.PortForwardOpts{
				Out:       publisher,
				Config:    opts.ClientConfig,
				ClientSet: opts.ClientSet,
				Namespace: podNamespace,
				Service:   svc.Name,
				PodName:   podName,
				PodPort:   podPort,
				LocalIp:   localIp,
				LocalPort: localPort,
				Hostfile:  opts.Hostfile,
				ShortName: opts.ShortName,
				SkipFail:  true,
			}

			go utils.PortForward(opts.Wg, pfo)

		}
	}

	return nil
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}

func mapToSelectorStr(msel map[string]string) string {
	selector := ""
	for k, v := range msel {
		if selector != "" {
			selector = selector + ","
		}
		selector = selector + fmt.Sprintf("%s=%s", k, v)
	}

	return selector
}
