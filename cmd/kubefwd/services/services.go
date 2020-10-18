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
	"os/signal"
	"sync"
	"syscall"

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
	"k8s.io/apimachinery/pkg/fields"
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
var exitOnFail bool
var verbose bool
var domain string

func init() {
	// override error output from k8s.io/apimachinery/pkg/util/runtime
	utilRuntime.ErrorHandlers[0] = func(err error) {
		log.Errorf("Runtime error: %s", err.Error())
		fwdsvcregistry.SyncAll()
	}

	Cmd.Flags().StringP("kubeconfig", "c", "", "absolute path to a kubectl config file")
	Cmd.Flags().StringSliceVarP(&contexts, "context", "x", []string{}, "specify a context to override the current context")
	Cmd.Flags().StringSliceVarP(&namespaces, "namespace", "n", []string{}, "Specify a namespace. Specify multiple namespaces by duplicating this argument.")
	Cmd.Flags().StringP("selector", "l", "", "Selector (label query) to filter on; supports '=', '==', and '!=' (e.g. -l key1=value1,key2=value2).")
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
		log.Fatalf("Hostfile error: %s", err.Error())
		os.Exit(1)
	}

	log.Printf("Loaded hosts file %s\n", hostFile.ReadFilePath)

	msg, err := fwdhost.BackupHostFile(hostFile)
	if err != nil {
		log.Fatalf("Error backing up hostfile: %s\n", err.Error())
		os.Exit(1)
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
		os.Exit(1)
	}

	// labels selector to filter services
	// see: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/
	selector := cmd.Flag("selector").Value.String()
	listOptions := metav1.ListOptions{}
	if selector != "" {
		listOptions.LabelSelector = selector
	}

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

	// ipC is the class C for the local IP address
	// increment this for each cluster
	// ipD is the class D for the local IP address
	// increment this for each service in each cluster
	ipC := 27
	ipD := 1

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
			nsWatchesDone.Add(1)
			go func(ii int, namespace string) {
				// ShortName field only use short name for the first namespace and context
				nameSpaceOpts := NamespaceOpts{
					ClientSet:         clientSet,
					Context:           ctx,
					Namespace:         namespace,
					NamespaceIPLock:   &sync.Mutex{}, // For parallelization of ip handout, each namespace has its own a.b.c.* range
					ListOptions:       listOptions,
					Hostfile:          &fwdport.HostFileWithLock{Hosts: hostFile},
					ClientConfig:      restConfig,
					RESTClient:        restClient,
					ShortName:         i < 1 && ii < 1,
					Remote:            i > 0,
					IpC:               byte(ipC + ii),
					IpD:               ipD,
					Domain:            domain,
					ManualStopChannel: stopListenCh,
				}
				nameSpaceOpts.watchServiceEvents(stopListenCh)
				nsWatchesDone.Done()
			}(ii, namespace)
		}
	}

	nsWatchesDone.Wait()
	log.Debugf("All namespace watchers are done")

	// Shutdown all active services
	<-fwdsvcregistry.Done()

	log.Infof("Clean exit")
}

type NamespaceOpts struct {
	ClientSet         *kubernetes.Clientset
	Context           string
	Namespace         string
	NamespaceIPLock   *sync.Mutex
	ListOptions       metav1.ListOptions
	Hostfile          *fwdport.HostFileWithLock
	ClientConfig      *restclient.Config
	RESTClient        *restclient.RESTClient
	ShortName         bool
	Remote            bool
	IpC               byte
	IpD               int
	Domain            string
	ManualStopChannel chan struct{}
}

// watchServiceEvents sets up event handlers to act on service-related events.
func (opts *NamespaceOpts) watchServiceEvents(stopListenCh <-chan struct{}) {
	// Apply filtering
	optionsModifier := func(options *metav1.ListOptions) {
		options.FieldSelector = fields.Everything().String()
		options.LabelSelector = opts.ListOptions.LabelSelector
	}

	// Construct the informer object which will query the api server,
	// and send events to our handler functions
	// https://engineering.bitnami.com/articles/kubewatch-an-example-of-kubernetes-custom-controller.html
	_, controller := cache.NewInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				optionsModifier(&options)
				return opts.ClientSet.CoreV1().Services(opts.Namespace).List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				options.Watch = true
				optionsModifier(&options)
				return opts.ClientSet.CoreV1().Services(opts.Namespace).Watch(options)
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
	log.Infof("Stopped watching Service events in namespace %s", opts.Namespace)
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
		ClientSet:        opts.ClientSet,
		Context:          opts.Context,
		Namespace:        opts.Namespace,
		Hostfile:         opts.Hostfile,
		ClientConfig:     opts.ClientConfig,
		RESTClient:       opts.RESTClient,
		ShortName:        opts.ShortName,
		Remote:           opts.Remote,
		IpC:              opts.IpC,
		IpD:              &opts.IpD,
		Domain:           opts.Domain,
		PodLabelSelector: selector,
		NamespaceIPLock:  opts.NamespaceIPLock,
		Svc:              svc,
		Headless:         svc.Spec.ClusterIP == "None",
		PortForwards:     make(map[string]*fwdport.PortForwardOpts),
		DoneChannel:      make(chan struct{}),
	}

	// Add the service to out catalog of services being forwarded
	fwdsvcregistry.Add(svcfwd)
}

// DeleteServiceHandler is the event handler for when a service gets deleted in k8s.
func (opts *NamespaceOpts) DeleteServiceHandler(obj interface{}) {
	svc, ok := obj.(*v1.Service)
	if !ok {
		return
	}

	// If we are currently forwarding this service, shut it down.
	fwdsvcregistry.RemoveByName(svc.Name + "." + svc.Namespace)
}

// UpdateServiceHandler is the event handler to deal with service changes from k8s.
// It currently does not do anything.
func (opts *NamespaceOpts) UpdateServiceHandler(_ interface{}, new interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(new)
	if err == nil {
		log.Printf("update service %s.", key)
	}
}
