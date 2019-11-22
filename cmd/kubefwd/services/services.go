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
	"strconv"
	"sync"
	"time"

	"github.com/txn2/kubefwd/pkg/fwdcfg"
	"github.com/txn2/kubefwd/pkg/fwdhost"
	"github.com/txn2/kubefwd/pkg/fwdnet"
	"github.com/txn2/kubefwd/pkg/fwdport"
	"github.com/txn2/kubefwd/pkg/fwdpub"
	"github.com/txn2/kubefwd/pkg/utils"
	"github.com/txn2/txeh"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

var namespaces []string
var contexts []string
var exitOnFail bool
var verbose bool
var domain string
var AllPortForwardOpts []*fwdport.PortForwardOpts

func init() {

	// override error output from k8s.io/apimachinery/pkg/util/runtime
	runtime.ErrorHandlers[0] = func(err error) {
		log.Errorf("Runtime error: %s", err.Error())
	}

	Cmd.Flags().StringP("kubeconfig", "c", "", "absolute path to a kubectl config file")
	Cmd.Flags().StringSliceVarP(&contexts, "context", "x", []string{}, "specify a context to override the current context")
	Cmd.Flags().StringSliceVarP(&namespaces, "namespace", "n", []string{}, "Specify a namespace. Specify multiple namespaces by duplicating this argument.")
	Cmd.Flags().StringP("selector", "l", "", "Selector (label query) to filter on; supports '=', '==', and '!=' (e.g. -l key1=value1,key2=value2).")
	Cmd.Flags().BoolVarP(&exitOnFail, "exitonfailure", "", false, "Exit(1) on failure. Useful for forcing a container restart.")
	Cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Verbose output.")
	Cmd.Flags().StringVarP(&domain, "domain", "d", "", "Append a pseudo domain name to generated host names.")

}

var Cmd = &cobra.Command{
	Use:     "services",
	Aliases: []string{"svcs", "svc"},
	Short:   "Forward services",
	Long:    `Forward multiple Kubernetes services from one or more namespaces. Filter services with selector.`,
	Example: "  kubefwd svc -n the-project\n" +
		"  kubefwd svc -n the-project -l app=wx,component=api\n" +
		"  kubefwd svc -n default -n the-project\n" +
		"  kubefwd svc -n default -d internal.example.com\n" +
		"  kubefwd svc -n the-project -x prod-cluster\n",
	Run: runCmd,
}

func runCmd(cmd *cobra.Command, args []string) {

	hasRoot, err := utils.CheckRoot()

	if !hasRoot {
		fmt.Printf(`
This program requires superuser privileges to run. These
privileges are required to add IP address aliases to your
loopback interface. Superuser privileges are also needed
to listen on low port numbers for these IP addresses.

Try:
 - sudo -E kubefwd services (Unix)
 - Running a shell with administrator rights (Windows)

`)
		if err != nil {
			log.Fatalf("Root check failure: %s", err.Error())
		}
		return
	}

	log.Println("Press [Ctrl-C] to stop forwarding.")
	log.Println("'cat /etc/hosts' to see all host entries.")

	hostFile, err := txeh.NewHostsDefault()
	if err != nil {
		log.Fatalf("Hostfile error: %s", err.Error())
	}

	log.Printf("Loaded hosts file %s\n", hostFile.ReadFilePath)

	msg, err := fwdhost.BackupHostFile(hostFile)
	if err != nil {
		log.Fatalf("Error backing up hostfile: %s\n", err.Error())
	}

	log.Printf("Hostfile management: %s", msg)

	// if sudo -E is used and the KUBECONFIG environment variable is set
	// it's easy to merge with kubeconfig files in env automatic.
	// if KUBECONFIG is blank, ToRawKubeConfigLoader() will use the
	// default kubeconfig file in $HOME/.kube/config
	cfgFilePath := ""

	// if we set the option --kubeconfig, It will have a higher priority
	// than KUBECONFIG environment. so it will override the KubeConfig options.
	flagCfgFilePath := cmd.Flag("kubeconfig").Value.String()
	if flagCfgFilePath != "" {
		cfgFilePath = flagCfgFilePath
	}

	// create a ConfigGetter
	configGetter := fwdcfg.NewConfigGetter()
	// build the ClientConfig
	rawConfig, err := configGetter.GetClientConfig(cfgFilePath)
	if err != nil {
		log.Fatalf("Error in get rawConfig: %s\n", err.Error())
	}

	// labels selector to filter services
	// see: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/
	selector := cmd.Flag("selector").Value.String()
	listOptions := metav1.ListOptions{}
	if selector != "" {
		listOptions.LabelSelector = selector
	}

	// if no namespaces were specified, check config then
	// explicitly set one to "default"
	if len(namespaces) < 1 {
		namespaces = []string{"default"}
		x := rawConfig.CurrentContext
		// use the first context if specified
		if len(contexts) > 0 {
			x = contexts[0]
		}

		for ctxName, ctxConfig := range rawConfig.Contexts {
			if ctxName == x {
				if ctxConfig.Namespace != "" {
					log.Printf("Using namespace %s from current context %s.", ctxConfig.Namespace, ctxName)
					namespaces = []string{ctxConfig.Namespace}
					break
				}
			}
		}
	}

	// ipC is the class C for the local IP address
	// increment this for each cluster
	// ipD is the class D for the local IP address
	// increment this for each service in each cluster
	ipC := 27
	ipD := 1

	wg := &sync.WaitGroup{}

	stopListenCh := make(chan struct{})
	defer close(stopListenCh)

	// if no context override
	if len(contexts) < 1 {
		contexts = append(contexts, rawConfig.CurrentContext)
	}

	for i, ctx := range contexts {
		// k8s REST config
		restConfig, err := configGetter.GetRestConfig(cfgFilePath, ctx)
		if err != nil {
			log.Fatalf("Error generating REST configuration: %s\n", err.Error())
		}

		// create the k8s clientSet
		clientSet, err := kubernetes.NewForConfig(restConfig)
		if err != nil {
			log.Fatalf("Error creating k8s clientSet: %s\n", err.Error())
		}

		// create the k8s RESTclient
		restClient, err := configGetter.GetRESTClient()
		if err != nil {
			log.Fatalf("Error creating k8s RestClient: %s\n", err.Error())
		}

		for ii, namespace := range namespaces {
			// ShortName field only use short name for the first namespace and context
			fwdServiceOpts := FwdServiceOpts{
				Wg:           wg,
				ClientSet:    clientSet,
				Context:      ctx,
				Namespace:    namespace,
				ListOptions:  listOptions,
				Hostfile:     &fwdport.HostFileWithLock{Hosts: hostFile},
				ClientConfig: restConfig,
				RESTClient:   restClient,
				ShortName:    i < 1 && ii < 1,
				Remote:       i > 0,
				IpC:          byte(ipC),
				IpD:          ipD,
				ExitOnFail:   exitOnFail,
				Domain:       domain,
			}
			go fwdServiceOpts.StartListen(stopListenCh)

			ipC = ipC + 1
		}
	}

	time.Sleep(2 * time.Second)

	wg.Wait()

	log.Printf("Done...\n")
}

type FwdServiceOpts struct {
	Wg           *sync.WaitGroup
	ClientSet    *kubernetes.Clientset
	Context      string
	Namespace    string
	ListOptions  metav1.ListOptions
	Hostfile     *fwdport.HostFileWithLock
	ClientConfig *restclient.Config
	RESTClient   *restclient.RESTClient
	ShortName    bool
	Remote       bool
	IpC          byte
	IpD          int
	ExitOnFail   bool
	Domain       string
}

func (opts *FwdServiceOpts) StartListen(stopListenCh <-chan struct{}) {

	optionsModifier := func(options *metav1.ListOptions) {
		options.FieldSelector = fields.Everything().String()
		options.LabelSelector = opts.ListOptions.LabelSelector
	}
	watchlist := cache.NewFilteredListWatchFromClient(opts.RESTClient, "services", v1.NamespaceDefault, optionsModifier)
	_, controller := cache.NewInformer(watchlist, &v1.Service{}, 0, cache.ResourceEventHandlerFuncs{
		AddFunc:    opts.AddServiceHandler,
		DeleteFunc: opts.DeleteServiceHandler,
		UpdateFunc: opts.UpdateServiceHandler,
	},
	)

	stop := make(chan struct{})
	go controller.Run(stop)
	defer close(stop)

	<-stopListenCh
}

func (opts *FwdServiceOpts) AddServiceHandler(obj interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		return
	}
	svcNamespace, svcName, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return
	}

	if verbose {
		fmt.Printf("Add service %s namespace %s !\r\n", svcName, svcNamespace)
	}

	opts.ForwardService(svcName, svcNamespace)
}

func (opts *FwdServiceOpts) DeleteServiceHandler(obj interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		return
	}
	svcNamespace, svcName, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return
	}

	if verbose {
		fmt.Printf("Delete service %s namespace %s !\r\n", svcName, svcNamespace)
	}

	opts.UnForwardService(svcName, svcNamespace)
}

func (opts *FwdServiceOpts) UpdateServiceHandler(old interface{}, new interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(new)
	if err == nil {
		fmt.Printf("update service %s !\r\n", key)
	}
}

func (opts *FwdServiceOpts) ForwardService(svcName string, svcNamespace string) {
	svc, err := opts.ClientSet.CoreV1().Services(opts.Namespace).Get(svcName, metav1.GetOptions{})
	if err != nil {
		return
	}

	set := labels.Set(svc.Spec.Selector)
	selector := set.AsSelector().String()

	if selector == "" {
		log.Warnf("WARNING: No Pod selector for service %s in %s on cluster %s.\n", svc.Name, svc.Namespace, svc.ClusterName)
		return
	}

	listOpts := metav1.ListOptions{LabelSelector: selector}

	pods, err := opts.ClientSet.CoreV1().Pods(svc.Namespace).List(listOpts)

	if err != nil {
		if errors.IsNotFound(err) {
			log.Warnf("WARNING: No Pods found for %s: %s\n", selector, err.Error())
		} else {
			log.Warnf("WARNING: Error in List pods for %s: %s\n", selector, err.Error())
		}
		return
	}

	// normal service portforward the first pod as service name.
	// headless service not only forward first Pod as service name, but also portforward all pods.
	if svc.Spec.ClusterIP == "None" {
		opts.ForwardFirstPodInService(pods, svc)
		opts.ForwardAllPodInService(pods, svc)
	} else {
		opts.ForwardFirstPodInService(pods, svc)
	}

	return
}

func (opts *FwdServiceOpts) UnForwardService(svcName string, svcNamespace string) {

	utils.Lock.Lock()
	// search for the PortForwardOpts if the svc should be unForward.
	// stop the PortForward and threadSafe delete the PortForward obj.
	for i := 0; i < len(AllPortForwardOpts); i++ {
		pfo := AllPortForwardOpts[i]
		if pfo.NativeServiceName == svcName && pfo.Namespace == svcNamespace {
			pfo.Stop()
			AllPortForwardOpts = append(AllPortForwardOpts[:i], AllPortForwardOpts[i+1:]...)
			i--
		}
	}
	utils.Lock.Unlock()
	return
}

func (opts *FwdServiceOpts) LoopPodToForward(pods []v1.Pod, podName bool, svc *v1.Service) {
	publisher := &fwdpub.Publisher{
		PublisherName: "Services",
		Output:        false,
	}
	for _, pod := range pods {

		podPort := ""
		svcName := ""

		localIp, dInc, err := fwdnet.ReadyInterface(127, 1, opts.IpC, opts.IpD, podPort)
		if err != nil {
			log.Warnf("WARNING: error readying interface: %s\n", err)
		}
		opts.IpD = dInc

		for _, port := range svc.Spec.Ports {

			podPort = port.TargetPort.String()
			localPort := strconv.Itoa(int(port.Port))

			if _, err := strconv.Atoi(podPort); err != nil {
				// search a pods containers for the named port
				if namedPodPort, ok := portSearch(podPort, pod.Spec.Containers); ok == true {
					podPort = namedPodPort
				}
			}

			serviceHostName := svc.Name

			if podName {
				serviceHostName = pod.Name + "." + serviceHostName
			}

			svcName = serviceHostName

			if opts.ShortName != true {
				serviceHostName = serviceHostName + "." + pod.Namespace
			}

			if opts.Domain != "" {
				if verbose {
					log.Printf("Using domain %s in generated hostnames", opts.Domain)
				}
				serviceHostName = serviceHostName + "." + opts.Domain
			}

			if opts.Remote {
				serviceHostName = fmt.Sprintf("%s.svc.cluster.%s", serviceHostName, opts.Context)
			}

			if verbose {
				log.Printf("Resolving:  %s%s to %s\n",
					svc.Name,
					serviceHostName,
					localIp.String(),
				)
			}

			log.Printf("Forwarding: %s:%d to pod %s:%s\n",
				serviceHostName,
				port.Port,
				pod.Name,
				podPort,
			)

			pfo := &fwdport.PortForwardOpts{
				Out:               publisher,
				Config:            opts.ClientConfig,
				ClientSet:         opts.ClientSet,
				RESTClient:        opts.RESTClient,
				ServiceOperator:   opts,
				Context:           opts.Context,
				Namespace:         pod.Namespace,
				Service:           svcName,
				NativeServiceName: svc.Name,
				PodName:           pod.Name,
				PodPort:           podPort,
				LocalIp:           localIp,
				LocalPort:         localPort,
				Hostfile:          opts.Hostfile,
				ShortName:         opts.ShortName,
				Remote:            opts.Remote,
				ExitOnFail:        exitOnFail,
				Domain:            domain,
			}
			AllPortForwardOpts = utils.ThreadSafeAppend(AllPortForwardOpts, pfo)

			opts.Wg.Add(1)
			go func() {
				err := pfo.PortForward()
				if err != nil {
					log.Printf("ERROR: %s", err.Error())
				}

				log.Printf("Stopped forwarding %s in %s.", pfo.Service, pfo.Namespace)

				opts.Wg.Done()
			}()

		}
	}
}

func (opts *FwdServiceOpts) ForwardFirstPodInService(pods *v1.PodList, svc *v1.Service) {
	opts.LoopPodToForward([]v1.Pod{pods.Items[0]}, false, svc)
}

func (opts *FwdServiceOpts) ForwardAllPodInService(pods *v1.PodList, svc *v1.Service) {
	opts.LoopPodToForward(pods.Items, true, svc)
}

func portSearch(portName string, containers []v1.Container) (string, bool) {

	for _, container := range containers {
		for _, cp := range container.Ports {
			if cp.Name == portName {
				return fmt.Sprint(cp.ContainerPort), true
			}
		}
	}

	return "", false
}
