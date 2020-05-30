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
	authorizationv1 "k8s.io/api/authorization/v1"
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
		"  kubefwd svc -n default -l \"app in (ws, api)\"\n" +
		"  kubefwd svc -n default -n the-project\n" +
		"  kubefwd svc -n default -d internal.example.com\n" +
		"  kubefwd svc -n the-project -x prod-cluster\n",
	Run: runCmd,
}

// checkConnection tests if you can connect to the cluster in your config,
// and if you have the necessary permissions to use kubefwd.
func checkConnection(clientSet *kubernetes.Clientset, namespaces []string) error {
	// Check simple connectivity: can you connect to the api server
	_, err := clientSet.Discovery().ServerVersion()
	if err != nil {
		return err
	}

	// Check RBAC permissions for each of the requested namespaces
	requiredPermissions := []authorizationv1.ResourceAttributes{
		{Verb: "list", Resource: "pods"}, {Verb: "get", Resource: "pods"}, {Verb: "watch", Resource: "pods"},
		{Verb: "get", Resource: "services"},
	}
	for _, namespace := range namespaces {
		for _, perm := range requiredPermissions {
			perm.Namespace = namespace
			ssar := &authorizationv1.SelfSubjectAccessReview{
				Spec: authorizationv1.SelfSubjectAccessReviewSpec{
					ResourceAttributes: &perm,
				},
			}
			ssar, err = clientSet.AuthorizationV1().SelfSubjectAccessReviews().Create(ssar)
			if err != nil {
				return err
			}
			if !ssar.Status.Allowed {
				return fmt.Errorf("Missing RBAC permission: %v", perm)
			}
		}
	}

	return nil
}

func runCmd(cmd *cobra.Command, args []string) {

	if verbose {
		log.SetLevel(log.DebugLevel)
	}

	hasRoot, err := utils.CheckRoot()

	if !hasRoot {
		log.Errorf(`
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

	if domain != "" {
		log.Printf("Adding custom domain %s to all forwarded entries\n", domain)
	}

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

		// check connectivity
		err = checkConnection(clientSet, namespaces)
		if err != nil {
			log.Fatalf("Error connecting to k8s cluster: %s\n", err.Error())
		}
		log.Infof("Succesfully connected context: %v", ctx)

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

	// for issue #105
	// increase time sleep here for no pod service
	// TODO: better way to solve the problem
	// maybe the wg.Add(1) move to AddServiceHandler and wg.Done() before return?
	time.Sleep(4 * time.Second)

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

// StartListen sets up event handlers to act on service-related events.
func (opts *FwdServiceOpts) StartListen(stopListenCh <-chan struct{}) {

	optionsModifier := func(options *metav1.ListOptions) {
		options.FieldSelector = fields.Everything().String()
		options.LabelSelector = opts.ListOptions.LabelSelector
	}
	watchlist := cache.NewFilteredListWatchFromClient(opts.RESTClient, "services", opts.Namespace, optionsModifier)
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

// AddServiceHandler is the event handler for when a new service comes in from k8s.
func (opts *FwdServiceOpts) AddServiceHandler(obj interface{}) {
	svc, ok := obj.(*v1.Service)
	if !ok {
		return
	}

	log.Debugf("Add service %s namespace %s.", svc.Name, svc.Namespace)

	opts.ForwardService(svc)
}

// DeleteServiceHandler is the event handler for when a service gets deleted in k8s.
func (opts *FwdServiceOpts) DeleteServiceHandler(obj interface{}) {
	svc, ok := obj.(*v1.Service)
	if !ok {
		return
	}

	log.Debugf("Delete service %s namespace %s.", svc.Name, svc.Namespace)

	opts.UnForwardService(svc)
}

// UpdateServiceHandler is the event handler to deal with service changes from k8s.
// It currently does not do anything.
func (opts *FwdServiceOpts) UpdateServiceHandler(old interface{}, new interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(new)
	if err == nil {
		log.Printf("update service %s.", key)
	}
}

// ForwardService selects one or all pods behind a service, and invokes the forwarding setup for that or those pod(s).
func (opts *FwdServiceOpts) ForwardService(svc *v1.Service) {
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

	// for issue #99
	// add this check for the service scale down to 0 situation.
	// TODO: a better way to do this check.
	if len(pods.Items) < 1 {
		log.Warnf("WARNING: No Running Pods returned for service %s in %s on cluster %s.\n", svc.Name, svc.Namespace, svc.ClusterName)
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
}

func (opts *FwdServiceOpts) UnForwardService(svc *v1.Service) {

	utils.Lock.Lock()
	// Search in the PortForwardOpts entries for for this service. If it is currently active,
	// stop the forwarding and threadSafe delete the PortForward obj.
	for i := 0; i < len(AllPortForwardOpts); i++ {
		pfo := AllPortForwardOpts[i]
		if pfo.NativeServiceName == svc.Name && pfo.Namespace == svc.Namespace {
			pfo.Stop()
			AllPortForwardOpts = append(AllPortForwardOpts[:i], AllPortForwardOpts[i+1:]...)
			i--
		}
	}
	utils.Lock.Unlock()
}

// LoopPodsToForward starts the portforwarding for each pod
func (opts *FwdServiceOpts) LoopPodsToForward(pods []v1.Pod, svc *v1.Service) {
	publisher := &fwdpub.Publisher{
		PublisherName: "Services",
		Output:        false,
	}

	// If multiple pods need to be forwarded, they all get their own host entry
	includePodNameInHost := len(pods) > 1

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
				if namedPodPort, ok := portSearch(podPort, pod.Spec.Containers); ok {
					podPort = namedPodPort
				}
			}

			serviceHostName := svc.Name

			if includePodNameInHost {
				serviceHostName = pod.Name + "." + serviceHostName
			}

			svcName = serviceHostName

			if !opts.ShortName {
				serviceHostName = serviceHostName + "." + pod.Namespace
			}

			if opts.Domain != "" {
				serviceHostName = serviceHostName + "." + opts.Domain
			}

			if opts.Remote {
				serviceHostName = fmt.Sprintf("%s.svc.cluster.%s", serviceHostName, opts.Context)
			}

			log.Debugf("Resolving:    %s to %s\n",
				serviceHostName,
				localIp.String(),
			)

			log.Printf("Port-Forward: %s:%d to pod %s:%s\n",
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

				log.Warnf("Stopped forwarding %s in %s.", pfo.Service, pfo.Namespace)

				opts.Wg.Done()
			}()

		}

	}
}

// ForwardFirstPodInService will set up portforwarding to a single pod of the service.
// A single entry for the service will be added to the hosts file.
func (opts *FwdServiceOpts) ForwardFirstPodInService(pods *v1.PodList, svc *v1.Service) {
	opts.LoopPodsToForward([]v1.Pod{pods.Items[0]}, svc)
}

// ForwardAllPodInService will set up portforwarding for each pod in the service. A separate entry
// for every pod will thus be added to the hosts file and individual pods can be addressed.
func (opts *FwdServiceOpts) ForwardAllPodInService(pods *v1.PodList, svc *v1.Service) {
	opts.LoopPodsToForward(pods.Items, svc)
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
