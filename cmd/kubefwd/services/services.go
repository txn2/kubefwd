package services

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/txn2/kubefwd/pkg/fwdapi"
	"github.com/txn2/kubefwd/pkg/fwdapi/types"
	"github.com/txn2/kubefwd/pkg/fwdcfg"
	"github.com/txn2/kubefwd/pkg/fwdhost"
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
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
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
var autoReconnect bool

// Version is set by the main package
var Version string

// defaultHostsPath returns the OS-appropriate hosts file path
func defaultHostsPath() string {
	if runtime.GOOS == "windows" {
		return `C:\Windows\System32\drivers\etc\hosts`
	}
	return "/etc/hosts"
}

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
	Cmd.Flags().StringVar(&hostsPath, "hosts-path", defaultHostsPath(), "Hosts file path.")
	Cmd.Flags().BoolVarP(&refreshHostsBackup, "refresh-backup", "b", false, "Create a fresh hosts backup, replacing any existing backup.")
	Cmd.Flags().BoolVarP(&purgeStaleIps, "purge-stale-ips", "p", false, "Remove stale kubefwd host entries (IPs in 127.1.27.1 - 127.255.255.255 range) before starting.")
	Cmd.Flags().DurationVar(&resyncInterval, "resync-interval", 5*time.Minute, "Interval for forced service resync (e.g., 1m, 5m, 30s)")
	Cmd.Flags().DurationVar(&retryInterval, "retry-interval", 10*time.Second, "Retry interval when no pods found for a service (e.g., 5s, 10s, 30s)")
	Cmd.Flags().BoolVar(&tuiMode, "tui", false, "Enable terminal user interface mode for interactive service monitoring")
	Cmd.Flags().BoolVar(&apiMode, "api", false, "Enable REST API server on http://kubefwd.internal/api for automation and monitoring")
	Cmd.Flags().BoolVarP(&autoReconnect, "auto-reconnect", "a", false, "Automatically reconnect when port forwards are lost (exponential backoff: 1s to 5min). Defaults to true in TUI/API mode.")
}

var Cmd = &cobra.Command{
	Use:     "services",
	Aliases: []string{"svcs", "svc"},
	Short:   "Forward services",
	Long: `Forward multiple Kubernetes services from one or more namespaces.

Idle Mode:
  When run without specifying namespaces (-n) or --all-namespaces, kubefwd starts
  in idle mode. The REST API is automatically enabled and kubefwd waits for
  namespaces and services to be added via API calls. This is useful for:
  - Running kubefwd as a background daemon
  - AI/MCP integration where all operations are API-driven
  - Dynamic environments where namespaces are not known at startup

  In idle mode, auto-reconnect (-a) is also enabled by default.`,
	Example: "  sudo kubefwd                          # Idle mode with API\n" +
		"  sudo kubefwd --tui                    # Idle mode with TUI\n" +
		"  sudo kubefwd -n the-project           # Forward from namespace\n" +
		"  sudo kubefwd -n the-project --tui     # With TUI\n" +
		"  sudo kubefwd -n the-project -l app=api\n" +
		"  sudo kubefwd -n default -n other-ns   # Multiple namespaces\n" +
		"  sudo kubefwd --all-namespaces         # All namespaces",
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

// validateEnvironment checks root privileges and hosts file
func validateEnvironment() bool {
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
		return false
	}

	if _, err = os.Stat(hostsPath); err != nil {
		log.Fatalf("Hosts path does not exist: %s", hostsPath)
	}
	return true
}

// detectAndConfigureIdleMode detects idle mode and configures flags
func detectAndConfigureIdleMode(cmd *cobra.Command) bool {
	idleMode := len(namespaces) == 0 && !isAllNs
	if idleMode {
		if !cmd.Flags().Changed("api") {
			apiMode = true
		}
		if !cmd.Flags().Changed("auto-reconnect") {
			autoReconnect = true
		}
		log.Println("Starting in idle mode - API enabled, waiting for namespaces/services via API")
	}
	return idleMode
}

// initializeTUIMode sets up TUI mode if enabled
func initializeTUIMode(cmd *cobra.Command) {
	if !tuiMode {
		return
	}
	fwdtui.Version = Version
	fwdtui.Enable()
	fwdmetrics.GetRegistry().Start()
	if !cmd.Flags().Changed("auto-reconnect") {
		autoReconnect = true
	}
}

// initializeAPIMode sets up API mode if enabled
func initializeAPIMode(cmd *cobra.Command) {
	if !apiMode {
		return
	}
	fwdapi.Enable()
	if !cmd.Flags().Changed("auto-reconnect") {
		autoReconnect = true
	}
}

// setupHostsFile initializes and backs up the hosts file
func setupHostsFile() *txeh.Hosts {
	hostFile, err := txeh.NewHosts(&txeh.HostsConfig{
		ReadFilePath:    hostsPath,
		WriteFilePath:   hostsPath,
		MaxHostsPerLine: 0,
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

	handleStaleIPs(hostFile)
	return hostFile
}

// handleStaleIPs purges or reports stale IP entries
func handleStaleIPs(hostFile *txeh.Hosts) {
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
}

// getKubeConfigPath returns the kubeconfig path from flag or default
func getKubeConfigPath(cmd *cobra.Command) string {
	flagCfgFilePath := cmd.Flag("kubeconfig").Value.String()
	if flagCfgFilePath != "" {
		return flagCfgFilePath
	}
	return ""
}

// setupListOptions creates list options from command flags
func setupListOptions(cmd *cobra.Command) metav1.ListOptions {
	return metav1.ListOptions{
		LabelSelector: cmd.Flag("selector").Value.String(),
		FieldSelector: cmd.Flag("field-selector").Value.String(),
	}
}

// resolveNamespaces determines namespaces from config if not specified
func resolveNamespaces(rawConfig *clientcmdapi.Config, idleMode bool) {
	if len(namespaces) >= 1 || idleMode {
		return
	}
	namespaces = []string{"default"}
	x := rawConfig.CurrentContext
	if len(contexts) > 0 {
		x = contexts[0]
	}

	for ctxName, ctxConfig := range rawConfig.Contexts {
		if ctxName == x && ctxConfig.Namespace != "" {
			log.Printf("Using namespace %s from current context %s.", ctxConfig.Namespace, ctxName)
			namespaces = []string{ctxConfig.Namespace}
			break
		}
	}
}

// setupSignalHandler sets up graceful shutdown on signals
func setupSignalHandler(triggerShutdown func()) {
	go func() {
		sigChan := make(chan os.Signal, 2)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		defer signal.Stop(sigChan)

		<-sigChan
		if fwdtui.EventsEnabled() {
			fwdtui.Emit(events.Event{Type: events.ShutdownStarted})
		}
		if !fwdtui.IsEnabled() {
			log.Infof("Shutting down... (press Ctrl+C again to force)")
		}
		triggerShutdown()

		<-sigChan
		log.Warnf("Forced shutdown - cleaning up hosts file")
		if err := fwdhost.RemoveAllocatedHosts(); err != nil {
			log.Errorf("Failed to clean hosts file: %s", err)
		}
		os.Exit(1)
	}()
}

// setupTUIManager initializes TUI manager with callbacks
func setupTUIManager(
	stopListenCh chan struct{},
	triggerShutdown func(),
	clientSets map[string]*kubernetes.Clientset,
	clientSetsMu *sync.RWMutex,
	getNsManager func() *fwdns.NamespaceManager,
) *fwdtui.Manager {
	if !fwdtui.IsEnabled() {
		return nil
	}

	tuiManager := fwdtui.Init(stopListenCh, triggerShutdown)
	tuiManager.SetPodLogsStreamer(createPodLogsStreamer(clientSets, clientSetsMu, getNsManager))
	tuiManager.SetErroredServicesReconnector(createErroredServicesReconnector())
	return tuiManager
}

// createPodLogsStreamer creates the pod logs streaming function
func createPodLogsStreamer(
	clientSets map[string]*kubernetes.Clientset,
	clientSetsMu *sync.RWMutex,
	getNsManager func() *fwdns.NamespaceManager,
) func(ctx context.Context, namespace, podName, containerName, k8sContext string, tailLines int64) (io.ReadCloser, error) {
	return func(ctx context.Context, namespace, podName, containerName, k8sContext string, tailLines int64) (io.ReadCloser, error) {
		clientSetsMu.RLock()
		cs, ok := clientSets[k8sContext]
		clientSetsMu.RUnlock()

		var clientSet kubernetes.Interface
		if ok && cs != nil {
			clientSet = cs
		}

		if clientSet == nil {
			if nsManager := getNsManager(); nsManager != nil {
				clientSet = nsManager.GetClientSet(k8sContext)
			}
			if clientSet == nil {
				return nil, fmt.Errorf("no clientset for context: %s", k8sContext)
			}
		}

		opts := &v1.PodLogOptions{Follow: true, TailLines: &tailLines}
		if containerName != "" {
			opts.Container = containerName
		}
		return clientSet.CoreV1().Pods(namespace).GetLogs(podName, opts).Stream(ctx)
	}
}

// createErroredServicesReconnector creates the reconnection callback
func createErroredServicesReconnector() func() int {
	return func() int {
		store := fwdtui.GetStore()
		if store == nil {
			return 0
		}

		forwards := store.GetFiltered()
		erroredServices := make(map[string]bool)
		for _, fwd := range forwards {
			if fwd.Status == state.StatusError {
				key := fwd.RegistryKey
				if key == "" {
					key = fwd.ServiceKey
				}
				erroredServices[key] = true
			}
		}

		count := 0
		for registryKey := range erroredServices {
			if svcfwd := fwdsvcregistry.Get(registryKey); svcfwd != nil {
				go svcfwd.ForceReconnect()
				count++
			}
		}
		return count
	}
}

// setupAPIManager initializes API manager with adapters
func setupAPIManager(stopListenCh chan struct{}, triggerShutdown func()) *fwdapi.Manager {
	if !fwdapi.IsEnabled() {
		return nil
	}

	fwdtui.InitEventInfrastructure()
	fwdmetrics.GetRegistry().Start()

	apiManager := fwdapi.Init(stopListenCh, triggerShutdown, Version)

	stateReader, metricsProvider, serviceController, eventStreamer := fwdapi.CreateAPIAdapters()
	apiManager.SetStateReader(stateReader)
	apiManager.SetMetricsProvider(metricsProvider)
	apiManager.SetServiceController(serviceController)
	apiManager.SetEventStreamer(eventStreamer)
	apiManager.SetNamespaces(namespaces)
	apiManager.SetContexts(contexts)
	apiManager.SetTUIEnabled(tuiMode)

	diagnosticsProvider := fwdapi.CreateDiagnosticsAdapter(func() types.ManagerInfo {
		if mgr := fwdapi.GetManager(); mgr != nil {
			return mgr
		}
		return nil
	})
	apiManager.SetDiagnosticsProvider(diagnosticsProvider)

	return apiManager
}

// startNamespaceWatchers starts watchers for each context/namespace
func startNamespaceWatchers(
	configGetter *fwdcfg.ConfigGetter,
	cfgFilePath string,
	nsManager *fwdns.NamespaceManager,
	listOptions metav1.ListOptions,
	clientSets map[string]*kubernetes.Clientset,
	clientSetsMu *sync.RWMutex,
) {
	for _, ctx := range contexts {
		restConfig, err := configGetter.GetRestConfig(cfgFilePath, ctx)
		if err != nil {
			log.Fatalf("Error generating REST configuration: %s\n", err.Error())
		}

		clientSet, err := kubernetes.NewForConfig(restConfig)
		if err != nil {
			log.Fatalf("Error creating k8s clientSet: %s\n", err.Error())
		}

		clientSetsMu.Lock()
		clientSets[ctx] = clientSet
		clientSetsMu.Unlock()

		if isAllNs {
			if len(namespaces) > 1 {
				log.Fatalf("Error: cannot combine options --all-namespaces and -n.")
			}
			setAllNamespace(clientSet, listOptions, &namespaces)
		}

		if err = checkConnection(clientSet, namespaces); err != nil {
			log.Fatalf("Error connecting to k8s cluster: %s\n", err.Error())
		}

		for _, namespace := range namespaces {
			if _, err := nsManager.StartWatcher(ctx, namespace, fwdns.WatcherOpts{
				LabelSelector: listOptions.LabelSelector,
				FieldSelector: listOptions.FieldSelector,
			}); err != nil {
				log.Errorf("Failed to start watcher for %s.%s: %v", namespace, ctx, err)
			}
		}
	}
}

// runMainLoop runs the main blocking loop
func runMainLoop(tuiManager *fwdtui.Manager, apiManager *fwdapi.Manager, idleMode bool, stopListenCh chan struct{}) {
	switch {
	case tuiManager != nil:
		if err := tuiManager.Run(); err != nil {
			log.Errorf("TUI error: %s", err)
		}
	case apiManager != nil && !tuiMode:
		log.Infof("API server running at http://%s/ (http://%s/)", fwdapi.APIIP+":"+fwdapi.APIPort, fwdapi.Hostname)
		if idleMode {
			log.Println("Idle mode: Add namespaces via POST /api/v1/namespaces or services via POST /api/v1/services")
		}
		log.Println("Press [Ctrl-C] to stop.")
		<-stopListenCh
	default:
		<-stopListenCh
	}
}

// performShutdown handles graceful shutdown sequence
func performShutdown(nsManager *fwdns.NamespaceManager, tuiManager *fwdtui.Manager, apiManager *fwdapi.Manager, hostFile *txeh.Hosts) {
	go nsManager.StopAll()
	select {
	case <-nsManager.Done():
		log.Debugf("All namespace watchers are done")
	case <-time.After(3 * time.Second):
		log.Debugf("Timeout waiting for namespace watchers, forcing exit")
	}

	select {
	case <-fwdsvcregistry.Done():
		log.Debugf("Service registry shutdown complete")
	case <-time.After(3 * time.Second):
		log.Debugf("Timeout waiting for service registry, forcing exit")
	}

	if tuiManager != nil {
		select {
		case <-tuiManager.Done():
			log.Debugf("TUI cleanup complete")
		case <-time.After(1 * time.Second):
			log.Debugf("Timeout waiting for TUI cleanup")
		}
	}

	if apiManager != nil {
		apiManager.Stop()
		select {
		case <-apiManager.Done():
			log.Debugf("API server cleanup complete")
		case <-time.After(1 * time.Second):
			log.Debugf("Timeout waiting for API cleanup")
		}
		_ = fwdapi.CleanupAPINetwork(hostFile)
	}

	if err := fwdhost.RemoveAllocatedHosts(); err != nil {
		log.Errorf("Failed to clean hosts file: %s", err)
	}

	log.Infof("Clean exit")
}

// createNamespaceManager creates and configures the namespace manager
func createNamespaceManager(hostFileWithLock *fwdport.HostFileWithLock, cfgFilePath string, listOptions metav1.ListOptions, stopListenCh chan struct{}) *fwdns.NamespaceManager {
	return fwdns.NewManager(fwdns.ManagerConfig{
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
}

// wireUpAdapters creates adapters and wires them to managers
func wireUpAdapters(getNsManager func() *fwdns.NamespaceManager, cfgFilePath string, apiManager *fwdapi.Manager, tuiManager *fwdtui.Manager) {
	k8sDiscovery := fwdapi.NewKubernetesDiscoveryAdapter(getNsManager, cfgFilePath)
	serviceCRUD := fwdapi.NewServiceCRUDAdapter(fwdtui.GetStore, getNsManager, cfgFilePath)
	nsController := fwdapi.NewNamespaceManagerAdapter(getNsManager)

	if apiManager != nil {
		apiManager.SetKubernetesDiscovery(k8sDiscovery)
		apiManager.SetServiceCRUD(serviceCRUD)
	}

	if tuiManager != nil {
		tuiManager.SetBrowseDiscovery(k8sDiscovery)
		tuiManager.SetBrowseServiceCRUD(serviceCRUD)
		tuiManager.SetBrowseNamespaceController(nsController)
		tuiManager.SetRemoveForwardCallback(func(key string) error {
			fwdsvcregistry.RemoveByName(key)
			return nil
		})
	}
}

// startAPIServer starts the API server in background if enabled
func startAPIServer(apiManager *fwdapi.Manager) {
	if apiManager == nil {
		return
	}
	go func() {
		if err := apiManager.Run(); err != nil {
			log.Errorf("API server error: %s", err)
		}
	}()
}

func runCmd(cmd *cobra.Command, _ []string) {
	if verbose {
		log.SetLevel(log.DebugLevel)
	}

	if !validateEnvironment() {
		return
	}

	idleMode := detectAndConfigureIdleMode(cmd)
	initializeTUIMode(cmd)
	initializeAPIMode(cmd)

	if !tuiMode && !apiMode && !idleMode {
		log.Println("Press [Ctrl-C] to stop forwarding.")
		log.Println("'cat " + hostsPath + "' to see all host entries.")
	}

	hostFile := setupHostsFile()

	if domain != "" {
		log.Printf("Adding custom domain %s to all forwarded entries\n", domain)
	}

	cfgFilePath := getKubeConfigPath(cmd)
	configGetter := fwdcfg.NewConfigGetter()
	rawConfig, err := configGetter.GetClientConfig(cfgFilePath)
	if err != nil {
		log.Fatalf("Error in get rawConfig: %s\n", err.Error())
	}

	listOptions := setupListOptions(cmd)
	resolveNamespaces(rawConfig, idleMode)

	if len(contexts) < 1 {
		contexts = append(contexts, rawConfig.CurrentContext)
	}

	stopListenCh := make(chan struct{})
	var stopOnce sync.Once
	triggerShutdown := func() {
		stopOnce.Do(func() {
			close(stopListenCh)
		})
	}

	clientSets := make(map[string]*kubernetes.Clientset)
	var clientSetsMu sync.RWMutex
	var nsManager *fwdns.NamespaceManager
	getNsManager := func() *fwdns.NamespaceManager { return nsManager }

	tuiManager := setupTUIManager(stopListenCh, triggerShutdown, clientSets, &clientSetsMu, getNsManager)
	apiManager := setupAPIManager(stopListenCh, triggerShutdown)
	setupSignalHandler(triggerShutdown)

	fwdsvcregistry.Init(stopListenCh)

	hostFileWithLock := &fwdport.HostFileWithLock{Hosts: hostFile}
	if fwdapi.IsEnabled() {
		if err := fwdapi.SetupAPINetwork(hostFileWithLock); err != nil {
			log.Fatalf("Failed to setup API network: %s", err)
		}
	}

	nsManager = createNamespaceManager(hostFileWithLock, cfgFilePath, listOptions, stopListenCh)
	wireUpAdapters(getNsManager, cfgFilePath, apiManager, tuiManager)

	if apiManager != nil {
		apiManager.SetNamespaceManager(nsManager)
	}
	if tuiManager != nil && len(contexts) > 0 {
		tuiManager.SetHeaderContext(contexts[0])
	}

	if !idleMode {
		startNamespaceWatchers(configGetter, cfgFilePath, nsManager, listOptions, clientSets, &clientSetsMu)
	}

	startAPIServer(apiManager)

	runMainLoop(tuiManager, apiManager, idleMode, stopListenCh)
	performShutdown(nsManager, tuiManager, apiManager, hostFile)
}
