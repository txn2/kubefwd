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
	Cmd.Flags().StringVar(&apiURL, "api-url", "http://localhost:8080", "URL of the kubefwd REST API")
	Cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Verbose output")
}

// Cmd is the MCP subcommand
var Cmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start MCP server (connects to kubefwd REST API)",
	Long: `Start an MCP (Model Context Protocol) server that connects to a running
kubefwd instance via its REST API.

This command does NOT require sudo and can be spawned by Claude Code or other
MCP-compatible AI assistants.

Prerequisites:
  1. Start kubefwd with the --api flag in a separate terminal:
     sudo -E kubefwd svc -n default --api

  2. Configure Claude Code to use this MCP server:
     Add to your Claude Code MCP settings:
     {
       "mcpServers": {
         "kubefwd": {
           "command": "kubefwd",
           "args": ["mcp", "--api-url", "http://localhost:8080"]
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
	Example: `  # Start MCP server connecting to default API URL
  kubefwd mcp

  # Connect to a custom API URL
  kubefwd mcp --api-url http://localhost:9000

  # With verbose logging (logs go to stderr, not interfering with stdio MCP)
  kubefwd mcp --verbose`,
	Run: runMCP,
}

func runMCP(cmd *cobra.Command, args []string) {
	// Configure logging to stderr (stdout is used for MCP stdio transport)
	log.SetOutput(os.Stderr)
	if verbose {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.WarnLevel)
	}

	log.Infof("Starting kubefwd MCP server (version %s)", Version)
	log.Infof("Connecting to REST API at: %s", apiURL)

	// Verify API is reachable
	if err := verifyAPIConnection(apiURL); err != nil {
		log.Errorf("Cannot connect to kubefwd API at %s: %v", apiURL, err)
		log.Error("Make sure kubefwd is running with --api flag:")
		log.Error("  sudo -E kubefwd svc -n <namespace> --api")
		os.Exit(1)
	}

	log.Info("API connection verified")

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

	// Initialize MCP server
	server := fwdmcp.Init(Version)
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
