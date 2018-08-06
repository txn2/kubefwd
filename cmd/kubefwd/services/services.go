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
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/cbednarski/hostess"
	"github.com/spf13/cobra"
	"github.com/txn2/kubefwd/pkg/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

var Cmd = &cobra.Command{
	Use:     "services",
	Aliases: []string{"svcs", "svc"},
	Short:   "Forward all services",
	Long:    `Forward all Kubernetes services.`,
	Run: func(cmd *cobra.Command, args []string) {

		if !utils.CheckRoot() {
			fmt.Printf(`
This program requires superuser priveledges to run. These 
priveledges are required to add IP address aliases to your 
loopback interface. Superuser priveledges are also needed 
to listen on low port numbers for these IP addresses.

Try: sudo kubefwd services

`)
			return
		}

		fmt.Println("Press [Ctrl-C] to stop forwarding.")

		// k8s rest config
		config := utils.K8sConfig(cmd)
		namespace := cmd.Flag("namespace").Value.String()
		selector := cmd.Flag("selector").Value.String()

		if namespace == "" {
			namespace = "default"
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

		services, err := clientSet.CoreV1().Services(namespace).List(listOptions)

		var wg sync.WaitGroup

		publisher := &utils.Publisher{
			PublisherName: "Services",
			Output:        false,
		}

		d := 1

		//hosts := hostess.NewHostlist()
		hostfile, errs := hostess.LoadHostfile()
		if errs != nil {
			fmt.Println("Can not load /etc/hosts")
			os.Exit(1)
		}

		for _, svc := range services.Items {
			selector := mapToSelectorStr(svc.Spec.Selector)
			pods, err := clientSet.CoreV1().Pods(namespace).List(metav1.ListOptions{LabelSelector: selector})
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

			localIp, ii, err := utils.ReadyInterface(127, 1, 27, d, podPort)
			d = ii

			for _, port := range svc.Spec.Ports {

				podName = pods.Items[0].Name
				podPort = port.TargetPort.String()
				localPort := strconv.Itoa(int(port.Port))

				_, err = clientSet.CoreV1().Pods(namespace).Get(podName, metav1.GetOptions{})
				if err != nil {
					fmt.Printf("Error getting pod: %s\n", err.Error())
					break // no need to check other ports if we can't ge the pod
				}

				if err != nil {
					fmt.Println(err.Error())
					os.Exit(1)
				}

				fmt.Printf("Forwarding local %s:%s as %s:%d to pod %s:%s\n", localIp.String(), localPort, svc.Name, port.Port, podName, podPort)

				wg.Add(1)
				pfo := &utils.PortForwardOpts{
					Out:       publisher,
					Config:    config,
					ClientSet: clientSet,
					Namespace: namespace,
					Service:   svc.Name,
					PodName:   podName,
					PodPort:   podPort,
					LocalIp:   localIp,
					LocalPort: localPort,
					Hostfile:  hostfile,
				}

				go utils.PortForward(&wg, pfo)

			}
		}

		wg.Wait()
		fmt.Printf("\nDone...\n")
		hostfile.Save()
	},
}

func init() {
	cfgFilePath := ""

	if home := homeDir(); home != "" {
		cfgFilePath = filepath.Join(home, ".kube", "config")
	}

	Cmd.Flags().StringP("kubeconfig", "c", cfgFilePath, "absolute path to the kubeconfig file")
	Cmd.Flags().StringP("namespace", "n", "", "Specify a namespace.")
	Cmd.Flags().StringP("selector", "l", "", "Selector (label query) to filter on, supports '=', '==', and '!='.(e.g. -l key1=value1,key2=value2)")
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
