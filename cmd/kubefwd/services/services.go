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
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/bep/debounce"
	"github.com/txn2/kubefwd/pkg/fwdcfg"
	"github.com/txn2/kubefwd/pkg/fwdhost"
	"github.com/txn2/kubefwd/pkg/fwdport"
	"github.com/txn2/kubefwd/pkg/fwdservice"
	"github.com/txn2/kubefwd/pkg/fwdsvcregistry"
	"github.com/txn2/kubefwd/pkg/utils"
	"github.com/txn2/txeh"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	authorizationv1 "k8s.io/api/authorization/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	utilRuntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

// cmdline arguments
var namespaces []string
var contexts []string
var verbose bool
var domain string
var mappings []string
var isAllNs bool
var fwdConfigurationPath string
var fwdReservations []string
var hostLocals []string

func init() {
	// override error output from k8s.io/apimachinery/pkg/util/runtime
	utilRuntime.ErrorHandlers[0] = func(err error) {
		// "broken pipe" see: https://github.com/kubernetes/kubernetes/issues/74551
		log.Errorf("Runtime: %s", err.Error())
	}

	Cmd.Flags().StringP("kubeconfig", "c", "", "absolute path to a kubectl config file")
	Cmd.Flags().StringSliceVarP(&contexts, "context", "x", []string{}, "specify a context to override the current context")
	Cmd.Flags().StringSliceVarP(&namespaces, "namespace", "n", []string{}, "Specify a namespace. Specify multiple namespaces by duplicating this argument.")
	Cmd.Flags().StringP("selector", "l", "", "Selector (label query) to filter on; supports '=', '==', and '!=' (e.g. -l key1=value1,key2=value2).")
	Cmd.Flags().StringP("field-selector", "f", "", "Field selector to filter on; supports '=', '==', and '!=' (e.g. -f metadata.name=service-name).")
	Cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Verbose output.")
	Cmd.Flags().StringVarP(&domain, "domain", "d", "", "Append a pseudo domain name to generated host names.")
	Cmd.Flags().StringSliceVarP(&mappings, "mapping", "m", []string{}, "Specify a port mapping. Specify multiple mapping by duplicating this argument.")
	Cmd.Flags().BoolVarP(&isAllNs, "all-namespaces", "A", false, "Enable --all-namespaces option like kubectl.")
	Cmd.Flags().StringSliceVarP(&fwdReservations, "reserve", "r", []string{}, "Specify an IP reservation. Specify multiple reservations by duplicating this argument.")
	Cmd.Flags().StringSliceVarP(&hostLocals, "host-local", "", []string{}, "Specify which services are hosted locally, and therefore don't tunnel")
	// TODO - Support digging the local information out of the ip reservation configuration yml file.
	Cmd.Flags().StringVarP(&fwdConfigurationPath, "fwd-conf", "z", "", "Define an IP reservation configuration")

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
		"  kubefwd svc -n the-project -x prod-cluster\n" +
		"  kubefwd svc -n the-project -m 80:8080 -m 443:1443\n" +
		"  kubefwd svc -n the-project -z path/to/conf.yml\n" +
		"  kubefwd svc -n the-project -r svc.ns:127.3.3.1\n" +
		"  kubefwd svc -n the-project -h svc.local\n" +
		"  kubefwd svc --all-namespaces",
	Run: runCmd,
}

// setAllNamespace Form V1Core get all namespace
func setAllNamespace(clientSet *kubernetes.Clientset, options metav1.ListOptions, namespaces *[]string) {
	nsList, err := clientSet.CoreV1().Namespaces().List(context.TODO(), options)
	if err != nil {
		log.Fatalf("Error get all namespaces by CoreV1: %s\n", err.Error())
	}
	if nsList == nil {
		log.Warn("No namespaces returned.")
		return
	}

	for _, ns := range nsList.Items {
		*namespaces = append(*namespaces, ns.Name)
	}
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
			var accessReview = &authorizationv1.SelfSubjectAccessReview{
				Spec: authorizationv1.SelfSubjectAccessReviewSpec{
					ResourceAttributes: &perm,
				},
			}
			accessReview, err = clientSet.AuthorizationV1().SelfSubjectAccessReviews().Create(context.TODO(), accessReview, metav1.CreateOptions{})
			if err != nil {
				return err
			}
			if !accessReview.Status.Allowed {
				return fmt.Errorf("missing RBAC permission: %v", perm)
			}
		}
	}

	return nil
}

func runCmd(cmd *cobra.Command, _ []string) {

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
		log.Fatalf("HostFile error: %s", err.Error())
	}

	log.Printf("Loaded hosts file %s\n", hostFile.ReadFilePath)

	msg, err := fwdhost.BackupHostFile(hostFile)
	if err != nil {
		log.Fatalf("Error backing up hostfile: %s\n", err.Error())
	}

	log.Printf("HostFile management: %s", msg)

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
	listOptions := metav1.ListOptions{}
	listOptions.LabelSelector = cmd.Flag("selector").Value.String()
	listOptions.FieldSelector = cmd.Flag("field-selector").Value.String()

	// if no namespaces were specified via the flags, check config from the k8s context
	// then explicitly set one to "default"
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

	stopListenCh := make(chan struct{})

	// Listen for shutdown signal from user
	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt, syscall.SIGTERM)
		defer func() {
			signal.Stop(sigint)
		}()
		<-sigint
		log.Infof("Received shutdown signal")
		close(stopListenCh)
	}()

	// if no context override
	if len(contexts) < 1 {
		contexts = append(contexts, rawConfig.CurrentContext)
	}

	fwdsvcregistry.Init(stopListenCh)

	nsWatchesDone := &sync.WaitGroup{} // We'll wait on this to exit the program. Done() indicates that all namespace watches have shutdown cleanly.

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

		// if use --all-namespace ,from v1 api get all ns.
		if isAllNs {
			if len(namespaces) > 1 {
				log.Fatalf("Error: cannot combine options --all-namespaces and -n.")
			}
			setAllNamespace(clientSet, listOptions, &namespaces)
		}

		// check connectivity
		err = checkConnection(clientSet, namespaces)
		if err != nil {
			log.Fatalf("Error connecting to k8s cluster: %s\n", err.Error())
		}
		log.Infof("Successfully connected context: %v", ctx)

		// create the k8s RESTclient
		restClient, err := configGetter.GetRESTClient()
		if err != nil {
			log.Fatalf("Error creating k8s RestClient: %s\n", err.Error())
		}

		for ii, namespace := range namespaces {
			nsWatchesDone.Add(1)

			nameSpaceOpts := NamespaceOpts{
				ClientSet: *clientSet,
				Context:   ctx,
				Namespace: namespace,

				// For parallelization of ip handout,
				// each cluster and namespace has its own ip range
				NamespaceIPLock:   &sync.Mutex{},
				ListOptions:       listOptions,
				HostFile:          &fwdport.HostFileWithLock{Hosts: hostFile},
				ClientConfig:      *restConfig,
				RESTClient:        *restClient,
				ClusterN:          i,
				NamespaceN:        ii,
				Domain:            domain,
				ManualStopChannel: stopListenCh,
				PortMapping:       mappings,
			}

			go func(npo NamespaceOpts) {
				nameSpaceOpts.watchServiceEvents(stopListenCh)
				nsWatchesDone.Done()
			}(nameSpaceOpts)
		}
	}

	nsWatchesDone.Wait()
	log.Debugf("All namespace watchers are done")

	// Shutdown all active services
	<-fwdsvcregistry.Done()

	log.Infof("Clean exit")
}

type NamespaceOpts struct {
	NamespaceIPLock *sync.Mutex
	ListOptions     metav1.ListOptions
	HostFile        *fwdport.HostFileWithLock

	ClientSet    kubernetes.Clientset
	ClientConfig restclient.Config
	RESTClient   restclient.RESTClient

	// Context is a unique key (string) in kubectl config representing
	// a user/cluster combination. Kubefwd uses context as the
	// cluster name when forwarding to more than one cluster.
	Context string

	// Namespace is the current Kubernetes Namespace to locate services
	// and the pods that back them for port-forwarding
	Namespace string

	// ClusterN is the ordinal index of the cluster (from configuration)
	// cluster 0 is considered local while > 0 is remote
	ClusterN int

	// NamespaceN is the ordinal index of the namespace from the
	// perspective of the user. Namespace 0 is considered local
	// while > 0 is an external namespace
	NamespaceN int

	// Domain is specified by the user and used in place of .local
	Domain string
	// meaning any source port maps to target port.
	PortMapping []string

	ManualStopChannel chan struct{}
}

// watchServiceEvents sets up event handlers to act on service-related events.
func (opts *NamespaceOpts) watchServiceEvents(stopListenCh <-chan struct{}) {
	// Apply filtering
	optionsModifier := func(options *metav1.ListOptions) {
		options.FieldSelector = opts.ListOptions.FieldSelector
		options.LabelSelector = opts.ListOptions.LabelSelector
	}

	// Construct the informer object which will query the api server,
	// and send events to our handler functions
	// https://engineering.bitnami.com/articles/kubewatch-an-example-of-kubernetes-custom-controller.html
	_, controller := cache.NewInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				optionsModifier(&options)
				return opts.ClientSet.CoreV1().Services(opts.Namespace).List(context.TODO(), options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				options.Watch = true
				optionsModifier(&options)
				return opts.ClientSet.CoreV1().Services(opts.Namespace).Watch(context.TODO(), options)
			},
		},
		&v1.Service{},
		0,
		cache.ResourceEventHandlerFuncs{
			AddFunc:    opts.AddServiceHandler,
			DeleteFunc: opts.DeleteServiceHandler,
			UpdateFunc: opts.UpdateServiceHandler,
		},
	)

	// Start the informer, blocking call until we receive a stop signal
	controller.Run(stopListenCh)
	log.Infof("Stopped watching Service events in namespace %s in %s context", opts.Namespace, opts.Context)
}

// AddServiceHandler is the event handler for when a new service comes in from k8s
// (the initial list of services will also be coming in using this event for each).
func (opts *NamespaceOpts) AddServiceHandler(obj interface{}) {
	svc, ok := obj.(*v1.Service)
	if !ok {
		return
	}

	// Check if service has a valid config to do forwarding
	selector := labels.Set(svc.Spec.Selector).AsSelector().String()
	if selector == "" {
		log.Warnf("WARNING: No Pod selector for service %s.%s, skipping\n", svc.Name, svc.Namespace)
		return
	}

	// Define a service to forward
	svcfwd := &fwdservice.ServiceFWD{
		ClientSet:                opts.ClientSet,
		Context:                  opts.Context,
		Namespace:                opts.Namespace,
		Hostfile:                 opts.HostFile,
		ClientConfig:             opts.ClientConfig,
		RESTClient:               opts.RESTClient,
		NamespaceN:               opts.NamespaceN,
		ClusterN:                 opts.ClusterN,
		Domain:                   opts.Domain,
		PodLabelSelector:         selector,
		NamespaceServiceLock:     opts.NamespaceIPLock,
		Svc:                      svc,
		Headless:                 svc.Spec.ClusterIP == "None",
		PortForwards:             make(map[string]*fwdport.PortForwardOpts),
		SyncDebouncer:            debounce.New(5 * time.Second),
		DoneChannel:              make(chan struct{}),
		PortMap:                  opts.ParsePortMap(mappings),
		ForwardConfigurationPath: fwdConfigurationPath,
		ForwardIPReservations:    fwdReservations,
		HostLocalServices:        hostLocals,
	}

	// Add the service to the catalog of services being forwarded
	fwdsvcregistry.Add(svcfwd)
}

// DeleteServiceHandler is the event handler for when a service gets deleted in k8s.
func (opts *NamespaceOpts) DeleteServiceHandler(obj interface{}) {
	svc, ok := obj.(*v1.Service)
	if !ok {
		return
	}

	// If we are currently forwarding this service, shut it down.
	fwdsvcregistry.RemoveByName(svc.Name + "." + svc.Namespace + "." + opts.Context)
}

// UpdateServiceHandler is the event handler to deal with service changes from k8s.
// It currently does not do anything.
func (opts *NamespaceOpts) UpdateServiceHandler(_ interface{}, new interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(new)
	if err == nil {
		log.Printf("update service %s.", key)
	}
}

// ParsePortMap parse string port to PortMap
func (opts *NamespaceOpts) ParsePortMap(mappings []string) *[]fwdservice.PortMap {
	var portList []fwdservice.PortMap
	if mappings == nil {
		return nil
	}
	for _, s := range mappings {
		portInfo := strings.Split(s, ":")
		portList = append(portList, fwdservice.PortMap{SourcePort: portInfo[0], TargetPort: portInfo[1]})
	}
	return &portList
}
