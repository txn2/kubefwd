// Package mcp provides the MCP (Model Context Protocol) subcommand for kubefwd.
// This command starts an MCP server that connects to a running kubefwd REST API,
// allowing AI assistants like Claude to interact with kubefwd without requiring sudo.
package mcp

import (
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/txn2/kubefwd/pkg/fwdapi/types"
	"github.com/txn2/kubefwd/pkg/fwdmcp"
)

var (
	apiURL  string
	verbose bool
)

// Version is set by the main package
var Version string

func init() {
	Cmd.Flags().StringVar(&apiURL, "api-url", "http://kubefwd.internal/api", "URL of the kubefwd REST API")
	Cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Verbose output")
}

// Cmd is the MCP subcommand
var Cmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start MCP server (connects to kubefwd REST API)",
	Long: `Start an MCP (Model Context Protocol) server that connects to a running
kubefwd instance via its REST API.

Architecture:
  ┌─────────────┐    stdio     ┌─────────────┐    HTTP      ┌─────────────┐
  │  AI Client  │ ←──────────→ │ kubefwd mcp │ ←──────────→ │   kubefwd   │
  │ (Claude,etc)│   MCP proto  │  (bridge)   │ REST API     │ (with sudo) │
  └─────────────┘              └─────────────┘              └─────────────┘

Why two processes?
  - kubefwd needs sudo for /etc/hosts and network interfaces
  - MCP clients spawn MCP servers as child processes (no sudo possible)
  - So 'kubefwd mcp' runs without sudo and talks to kubefwd via REST API

This command does NOT require sudo and can be spawned by Claude Code, Cursor,
or other MCP-compatible AI assistants.

Prerequisites:
  1. Start kubefwd in a separate terminal (requires sudo):
     sudo -E kubefwd              # Idle mode with API auto-enabled
     sudo -E kubefwd -n default   # Forward namespace (API auto-enabled)

  2. Configure your MCP client (e.g., Claude Code):
     {
       "mcpServers": {
         "kubefwd": {
           "command": "kubefwd",
           "args": ["mcp"]
         }
       }
     }

The MCP server provides developer-focused tools for:
  - Adding/removing namespaces to forward dynamically
  - Adding/removing individual services to forward
  - Discovering available Kubernetes namespaces and services
  - Getting connection info (hostnames, IPs, ports, env vars)
  - Finding forwarded services by name or port
  - Listing and inspecting forwarded services
  - Viewing metrics and logs
  - Triggering reconnections and syncs
  - Diagnosing errors`,
	Example: `  # Start MCP server (connects to kubefwd API at http://kubefwd.internal/api)
  kubefwd mcp

  # Connect to a custom API URL
  kubefwd mcp --api-url http://localhost:8080/api

  # With verbose logging (logs go to stderr, not interfering with stdio MCP)
  kubefwd mcp --verbose`,
	Run: runMCP,
}

func runMCP(_ *cobra.Command, _ []string) {
	// Configure logging to stderr (stdout is used for MCP stdio transport)
	log.SetOutput(os.Stderr)
	if verbose {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.WarnLevel)
	}

	log.Infof("Starting kubefwd MCP server (version %s)", Version)
	log.Infof("Connecting to REST API at: %s", apiURL)

	// Initialize MCP server first so tools are registered for discovery
	// (allows Smithery and other registries to introspect capabilities)
	server := fwdmcp.Init(Version)

	// Check API connection - if unavailable, tools will return helpful errors
	apiAvailable := false
	if err := verifyAPIConnection(apiURL); err != nil {
		log.Warnf("Cannot connect to kubefwd API at %s: %v", apiURL, err)
		log.Warn("MCP server will start but tools require kubefwd to be running.")
		log.Warn("Start kubefwd in another terminal with: sudo -E kubefwd")
	} else {
		log.Info("API connection verified")
		apiAvailable = true
	}

	// Only set up HTTP adapters if API is available
	// If not available, providers stay nil and handlers return helpful instructions
	if apiAvailable {
		// Create HTTP-based adapters
		stateReader := fwdmcp.NewStateReaderHTTP(apiURL)
		metricsProvider := fwdmcp.NewMetricsProviderHTTP(apiURL)
		serviceController := fwdmcp.NewServiceControllerHTTP(apiURL)
		diagnosticsProvider := fwdmcp.NewDiagnosticsProviderHTTP(apiURL)
		managerInfo := fwdmcp.NewManagerInfoHTTP(apiURL)

		// Create CRUD HTTP adapters for developer-focused tools
		namespaceController := fwdmcp.NewNamespaceControllerHTTP(apiURL)
		serviceCRUD := fwdmcp.NewServiceCRUDHTTP(apiURL)
		k8sDiscovery := fwdmcp.NewKubernetesDiscoveryHTTP(apiURL)
		connectionInfo := fwdmcp.NewConnectionInfoProviderHTTP(apiURL)

		// Create enhanced HTTP adapters for AI-optimized tools
		analysisProvider := fwdmcp.NewAnalysisProviderHTTP(apiURL)
		httpTrafficProvider := fwdmcp.NewHTTPTrafficProviderHTTP(apiURL)
		historyProvider := fwdmcp.NewHistoryProviderHTTP(apiURL)

		server.SetStateReader(stateReader)
		server.SetMetricsProvider(metricsProvider)
		server.SetServiceController(serviceController)
		server.SetDiagnosticsProvider(diagnosticsProvider)
		server.SetManagerInfo(func() types.ManagerInfo {
			return managerInfo
		})

		// Set CRUD controllers for developer-focused tools
		server.SetNamespaceController(namespaceController)
		server.SetServiceCRUD(serviceCRUD)
		server.SetKubernetesDiscovery(k8sDiscovery)
		server.SetConnectionInfoProvider(connectionInfo)

		// Set enhanced providers for AI-optimized tools
		server.SetAnalysisProvider(analysisProvider)
		server.SetHTTPTrafficProvider(httpTrafficProvider)
		server.SetHistoryProvider(historyProvider)
	}

	log.Info("MCP server initialized, starting stdio transport...")

	// Run stdio server (blocks until client disconnects)
	if err := server.ServeStdio(); err != nil {
		log.Errorf("MCP server error: %v", err)
		os.Exit(1)
	}

	log.Info("MCP server stopped")
}

// verifyAPIConnection checks if the kubefwd API is reachable
func verifyAPIConnection(baseURL string) error {
	client := fwdmcp.NewHTTPClient(baseURL)

	var resp struct {
		Status string `json:"status"`
	}

	// Try to hit the health endpoint
	if err := client.Get("/health", &resp); err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}

	return nil
}
