package services

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/txn2/kubefwd/pkg/fwdapi"
	"github.com/txn2/kubefwd/pkg/fwdapi/types"
	"github.com/txn2/kubefwd/pkg/fwdcfg"
	"github.com/txn2/kubefwd/pkg/fwdhost"
	"github.com/txn2/kubefwd/pkg/fwdmcp"
	"github.com/txn2/kubefwd/pkg/fwdmetrics"
	"github.com/txn2/kubefwd/pkg/fwdns"
	"github.com/txn2/kubefwd/pkg/fwdport"
	"github.com/txn2/kubefwd/pkg/fwdsvcregistry"
	"github.com/txn2/kubefwd/pkg/fwdtui"
	"github.com/txn2/kubefwd/pkg/fwdtui/events"
	"github.com/txn2/kubefwd/pkg/fwdtui/state"
	"github.com/txn2/kubefwd/pkg/utils"
	"github.com/txn2/txeh"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	authorizationv1 "k8s.io/api/authorization/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilRuntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
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
var timeout int
var hostsPath string
var refreshHostsBackup bool
var purgeStaleIps bool
var resyncInterval time.Duration
var retryInterval time.Duration
var tuiMode bool
var apiMode bool
var mcpMode bool
var autoReconnect bool

// Version is set by the main package
var Version string

func init() {
	// override error output from k8s.io/apimachinery/pkg/util/runtime
	utilRuntime.ErrorHandlers[0] = func(_ context.Context, err error, _ string, _ ...interface{}) {
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
	Cmd.Flags().StringVarP(&fwdConfigurationPath, "fwd-conf", "z", "", "Define an IP reservation configuration")
	Cmd.Flags().IntVarP(&timeout, "timeout", "t", 300, "Specify a timeout seconds for the port forwarding.")
	Cmd.Flags().StringVar(&hostsPath, "hosts-path", "/etc/hosts", "Hosts Path default /etc/hosts.")
	Cmd.Flags().BoolVarP(&refreshHostsBackup, "refresh-backup", "b", false, "Create a fresh hosts backup, replacing any existing backup.")
	Cmd.Flags().BoolVarP(&purgeStaleIps, "purge-stale-ips", "p", false, "Remove stale kubefwd host entries (IPs in 127.1.27.1 - 127.255.255.255 range) before starting.")
	Cmd.Flags().DurationVar(&resyncInterval, "resync-interval", 5*time.Minute, "Interval for forced service resync (e.g., 1m, 5m, 30s)")
	Cmd.Flags().DurationVar(&retryInterval, "retry-interval", 10*time.Second, "Retry interval when no pods found for a service (e.g., 5s, 10s, 30s)")
	Cmd.Flags().BoolVar(&tuiMode, "tui", false, "Enable terminal user interface mode for interactive service monitoring")
	Cmd.Flags().BoolVar(&apiMode, "api", false, "Enable REST API server on http://kubefwd.internal/api for automation and monitoring")
	Cmd.Flags().BoolVar(&mcpMode, "mcp", false, "Enable MCP (Model Context Protocol) server on stdio for AI assistant integration")
	Cmd.Flags().BoolVarP(&autoReconnect, "auto-reconnect", "a", false, "Automatically reconnect when port forwards are lost (exponential backoff: 1s to 5min). Defaults to true in TUI/API mode.")
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
		"  kubefwd svc --all-namespaces\n" +
		"  kubefwd svc --hosts-path /etc/hosts",
	Run: runCmd,
}

// setAllNamespace Form V1Core get all namespace
func setAllNamespace(clientSet kubernetes.Interface, options metav1.ListOptions, namespaces *[]string) {
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
func checkConnection(clientSet kubernetes.Interface, namespaces []string) error {
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

	_, err = os.Stat(hostsPath)
	if err != nil {
		log.Fatalf("Hosts path does not exist: %s", hostsPath)
	}

	// Initialize TUI mode if enabled
	if tuiMode {
		fwdtui.Version = Version
		fwdtui.Enable()
		// Start metrics registry sampling EARLY, before port forwards begin
		// This ensures rate calculation has samples from the start of data transfer
		fwdmetrics.GetRegistry().Start()

		// In TUI mode, enable auto-reconnect by default unless user explicitly disabled it
		if !cmd.Flags().Changed("auto-reconnect") {
			autoReconnect = true
		}
	}

	// Initialize API mode if enabled
	if apiMode {
		fwdapi.Enable()

		// In API mode, enable auto-reconnect by default unless user explicitly disabled it
		if !cmd.Flags().Changed("auto-reconnect") {
			autoReconnect = true
		}
	}

	// Initialize MCP mode if enabled
	if mcpMode {
		fwdmcp.Enable()

		// In MCP mode, enable auto-reconnect by default unless user explicitly disabled it
		if !cmd.Flags().Changed("auto-reconnect") {
			autoReconnect = true
		}
	}

	// Only show instructions in non-TUI mode (and non-API-only mode)
	if !tuiMode && !apiMode {
		log.Println("Press [Ctrl-C] to stop forwarding.")
		log.Println("'cat " + hostsPath + "' to see all host entries.")
	}

	hostFile, err := txeh.NewHosts(&txeh.HostsConfig{
		ReadFilePath:    hostsPath,
		WriteFilePath:   hostsPath,
		MaxHostsPerLine: 0, // Auto: 9 on Windows, unlimited elsewhere
	})
	if err != nil {
		log.Fatalf("HostFile error: %s", err.Error())
	}

	log.Printf("Loaded hosts file %s\n", hostFile.ReadFilePath)

	msg, err := fwdhost.BackupHostFile(hostFile, refreshHostsBackup)
	if err != nil {
		log.Fatalf("Error backing up hostfile: %s\n", err.Error())
	}

	log.Printf("HostFile management: %s", msg)

	if purgeStaleIps {
		count, err := fwdhost.PurgeStaleIps(hostFile)
		if err != nil {
			log.Fatalf("Error purging stale IPs: %s\n", err.Error())
		}
		if count > 0 {
			log.Printf("Purged %d stale host entries from previous kubefwd sessions\n", count)
		}
	} else {
		staleCount := fwdhost.CountStaleEntries(hostFile)
		if staleCount > 0 {
			log.Infof("HOUSEKEEPING: Found %d existing host entries in kubefwd IP range. These will not affect operation and will be preserved. Use -p to purge on next run if you no longer need them.", staleCount)
		}
	}

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

	// Initialize TUI manager if in TUI mode
	var tuiManager *fwdtui.Manager
	var apiManager *fwdapi.Manager
	var mcpServer *fwdmcp.Server
	var stopOnce sync.Once
	triggerShutdown := func() {
		stopOnce.Do(func() {
			close(stopListenCh)
		})
	}
	// Map of context -> clientSet for pod log streaming
	clientSets := make(map[string]*kubernetes.Clientset)
	var clientSetsMu sync.RWMutex

	if fwdtui.IsEnabled() {
		tuiManager = fwdtui.Init(stopListenCh, triggerShutdown)

		// Set up pod logs streamer
		tuiManager.SetPodLogsStreamer(func(ctx context.Context, namespace, podName, containerName, k8sContext string, tailLines int64) (io.ReadCloser, error) {
			clientSetsMu.RLock()
			clientSet, ok := clientSets[k8sContext]
			clientSetsMu.RUnlock()

			if !ok {
				return nil, fmt.Errorf("no clientset for context: %s", k8sContext)
			}

			opts := &v1.PodLogOptions{
				Follow:    true,
				TailLines: &tailLines,
			}
			// Set container name if specified (required for multi-container pods)
			if containerName != "" {
				opts.Container = containerName
			}

			return clientSet.CoreV1().Pods(namespace).GetLogs(podName, opts).Stream(ctx)
		})

		// Set up errored services reconnector
		tuiManager.SetErroredServicesReconnector(func() int {
			store := fwdtui.GetStore()
			if store == nil {
				return 0
			}

			forwards := store.GetFiltered()

			// Collect unique registry keys with errors
			// Use RegistryKey (not ServiceKey) for proper registry lookup
			// ServiceKey may include pod name for headless services, but registry uses service.namespace.context
			erroredServices := make(map[string]bool)
			for _, fwd := range forwards {
				if fwd.Status == state.StatusError {
					// Prefer RegistryKey if available, fall back to ServiceKey for backwards compatibility
					key := fwd.RegistryKey
					if key == "" {
						key = fwd.ServiceKey
					}
					erroredServices[key] = true
				}
			}

			// Trigger reconnection for each errored service
			count := 0
			for registryKey := range erroredServices {
				if svcfwd := fwdsvcregistry.Get(registryKey); svcfwd != nil {
					// ForceReconnect resets backoff state and triggers immediate reconnection
					// This bypasses any pending backoff timers from auto-reconnect
					go svcfwd.ForceReconnect()
					count++
				}
			}

			return count
		})
	}

	// Initialize API manager if in API mode
	if fwdapi.IsEnabled() {
		// Initialize event infrastructure for API mode (allows events without TUI)
		fwdtui.InitEventInfrastructure()
		fwdmetrics.GetRegistry().Start()

		apiManager = fwdapi.Init(stopListenCh, triggerShutdown, Version)

		// Set up adapters for API data access
		stateReader, metricsProvider, serviceController, eventStreamer := fwdapi.CreateAPIAdapters()
		apiManager.SetStateReader(stateReader)
		apiManager.SetMetricsProvider(metricsProvider)
		apiManager.SetServiceController(serviceController)
		apiManager.SetEventStreamer(eventStreamer)
		apiManager.SetNamespaces(namespaces)
		apiManager.SetContexts(contexts)
		apiManager.SetTUIEnabled(tuiMode)

		// Set up diagnostics adapter
		diagnosticsProvider := fwdapi.CreateDiagnosticsAdapter(func() types.ManagerInfo {
			if mgr := fwdapi.GetManager(); mgr != nil {
				return mgr
			}
			return nil
		})
		apiManager.SetDiagnosticsProvider(diagnosticsProvider)
	}

	// Initialize MCP server if in MCP mode
	if fwdmcp.IsEnabled() {
		// Initialize event infrastructure for MCP mode (allows events without TUI)
		fwdtui.InitEventInfrastructure()
		fwdmetrics.GetRegistry().Start()

		mcpServer = fwdmcp.Init(Version)

		// Set up adapters for MCP data access (reuse API adapters)
		stateReader, metricsProvider, serviceController, _ := fwdapi.CreateAPIAdapters()
		mcpServer.SetStateReader(stateReader)
		mcpServer.SetMetricsProvider(metricsProvider)
		mcpServer.SetServiceController(serviceController)

		// Set up diagnostics adapter
		diagnosticsProvider := fwdapi.CreateDiagnosticsAdapter(func() types.ManagerInfo {
			if mgr := fwdapi.GetManager(); mgr != nil {
				return mgr
			}
			return nil
		})
		mcpServer.SetDiagnosticsProvider(diagnosticsProvider)

		// Set up manager info
		mcpServer.SetManagerInfo(func() types.ManagerInfo {
			if mgr := fwdapi.GetManager(); mgr != nil {
				return mgr
			}
			return nil
		})
	}

	// Listen for shutdown signal from user
	go func() {
		sigChan := make(chan os.Signal, 2)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		defer signal.Stop(sigChan)

		// First signal: graceful shutdown
		<-sigChan
		if fwdtui.IsEnabled() {
			fwdtui.Emit(events.Event{Type: events.ShutdownStarted})
		} else {
			log.Infof("Shutting down... (press Ctrl+C again to force)")
		}
		triggerShutdown()

		// Second signal: force exit
		<-sigChan
		log.Warnf("Forced shutdown - cleaning up hosts file")
		if err := fwdhost.RemoveAllocatedHosts(); err != nil {
			log.Errorf("Failed to clean hosts file: %s", err)
		}
		os.Exit(1)
	}()

	// if no context override
	if len(contexts) < 1 {
		contexts = append(contexts, rawConfig.CurrentContext)
	}

	fwdsvcregistry.Init(stopListenCh)

	hostFileWithLock := &fwdport.HostFileWithLock{Hosts: hostFile}

	// Set up API network (loopback IP and hosts entry) if API mode enabled
	if fwdapi.IsEnabled() {
		if err := fwdapi.SetupAPINetwork(hostFileWithLock); err != nil {
			log.Fatalf("Failed to setup API network: %s", err)
		}
	}

	// Create the namespace manager for dynamic watcher management
	nsManager := fwdns.NewManager(fwdns.ManagerConfig{
		HostFile:        hostFileWithLock,
		ConfigPath:      cfgFilePath,
		Domain:          domain,
		PortMapping:     mappings,
		Timeout:         timeout,
		FwdConfigPath:   fwdConfigurationPath,
		FwdReservations: fwdReservations,
		ResyncInterval:  resyncInterval,
		RetryInterval:   retryInterval,
		AutoReconnect:   autoReconnect,
		LabelSelector:   listOptions.LabelSelector,
		FieldSelector:   listOptions.FieldSelector,
		GlobalStopCh:    stopListenCh,
	})

	// Register namespace manager with API if enabled
	if apiManager != nil {
		apiManager.SetNamespaceManager(nsManager)

		// Set up Kubernetes discovery adapter
		k8sDiscovery := fwdapi.NewKubernetesDiscoveryAdapter(
			apiManager.GetNamespaceManager,
			cfgFilePath,
		)
		apiManager.SetKubernetesDiscovery(k8sDiscovery)
	}

	// Start watchers for each context/namespace combination
	for _, ctx := range contexts {
		// Create clientSet for this context (for connectivity check and --all-namespaces)
		restConfig, err := configGetter.GetRestConfig(cfgFilePath, ctx)
		if err != nil {
			log.Fatalf("Error generating REST configuration: %s\n", err.Error())
		}

		clientSet, err := kubernetes.NewForConfig(restConfig)
		if err != nil {
			log.Fatalf("Error creating k8s clientSet: %s\n", err.Error())
		}

		// Store clientSet for pod log streaming
		clientSetsMu.Lock()
		clientSets[ctx] = clientSet
		clientSetsMu.Unlock()

		// if use --all-namespace, from v1 api get all ns.
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

		// Start a watcher for each namespace
		for _, namespace := range namespaces {
			_, err := nsManager.StartWatcher(ctx, namespace, fwdns.WatcherOpts{
				LabelSelector: listOptions.LabelSelector,
				FieldSelector: listOptions.FieldSelector,
			})
			if err != nil {
				log.Errorf("Failed to start watcher for %s.%s: %v", namespace, ctx, err)
			}
		}
	}

	// Start API server in background if enabled
	if apiManager != nil {
		go func() {
			if err := apiManager.Run(); err != nil {
				log.Errorf("API server error: %s", err)
			}
		}()
	}

	// Start MCP server in background if enabled
	if mcpServer != nil {
		go func() {
			if err := mcpServer.Run(context.Background()); err != nil {
				log.Errorf("MCP server error: %s", err)
			}
		}()
	}

	// If TUI mode, run the TUI (blocks until user quits)
	// Otherwise, block until shutdown signal is received
	if tuiManager != nil {
		if err := tuiManager.Run(); err != nil {
			log.Errorf("TUI error: %s", err)
		}
	} else if mcpServer != nil && !tuiMode && apiManager == nil {
		// MCP-only mode (no TUI, no API): block silently on stdio
		<-stopListenCh
	} else if apiManager != nil && !tuiMode {
		// API-only mode (no TUI): show info and block
		log.Infof("API server running at http://%s/ (http://%s/)", fwdapi.APIIP+":"+fwdapi.APIPort, fwdapi.Hostname)
		log.Println("Press [Ctrl-C] to stop forwarding.")
		<-stopListenCh
	} else {
		// Standard mode: block until shutdown signal (Ctrl+C)
		<-stopListenCh
	}

	// Stop namespace manager and wait for watchers
	go nsManager.StopAll()
	select {
	case <-nsManager.Done():
		log.Debugf("All namespace watchers are done")
	case <-time.After(3 * time.Second):
		log.Debugf("Timeout waiting for namespace watchers, forcing exit")
	}

	// Shutdown all active services with timeout
	select {
	case <-fwdsvcregistry.Done():
		log.Debugf("Service registry shutdown complete")
	case <-time.After(3 * time.Second):
		log.Debugf("Timeout waiting for service registry, forcing exit")
	}

	// Wait for TUI cleanup if enabled
	if tuiManager != nil {
		select {
		case <-tuiManager.Done():
			log.Debugf("TUI cleanup complete")
		case <-time.After(1 * time.Second):
			log.Debugf("Timeout waiting for TUI cleanup")
		}
	}

	// Wait for API cleanup if enabled
	if apiManager != nil {
		apiManager.Stop()
		select {
		case <-apiManager.Done():
			log.Debugf("API server cleanup complete")
		case <-time.After(1 * time.Second):
			log.Debugf("Timeout waiting for API cleanup")
		}
		// Clean up API network configuration
		fwdapi.CleanupAPINetwork(hostFile)
	}

	// Wait for MCP cleanup if enabled
	if mcpServer != nil {
		mcpServer.Stop()
		select {
		case <-mcpServer.Done():
			log.Debugf("MCP server cleanup complete")
		case <-time.After(1 * time.Second):
			log.Debugf("Timeout waiting for MCP cleanup")
		}
	}

	// Final safety net: ensure all hosts are cleaned up
	if err := fwdhost.RemoveAllocatedHosts(); err != nil {
		log.Errorf("Failed to clean hosts file: %s", err)
	}

	log.Infof("Clean exit")
}
