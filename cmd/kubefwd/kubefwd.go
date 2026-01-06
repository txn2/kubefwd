package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/txn2/kubefwd/cmd/kubefwd/mcp"
	"github.com/txn2/kubefwd/cmd/kubefwd/services"
	"k8s.io/klog/v2"
)

var globalUsage = `Bulk forward Kubernetes services for local development.

Each forwarded service gets its own unique loopback IP (127.x.x.x), allowing
multiple services to use the same port simultaneously. Service names are added
to /etc/hosts for transparent access using cluster service names.

Modes:
  Idle Mode:    Run without -n/--namespace; API enabled, no namespaces forwarded
  Namespace:    Forward all services from specified namespace(s)
  All:          Forward services from all namespaces (--all-namespaces)

The REST API (http://kubefwd.internal/) is auto-enabled in idle mode and allows
adding/removing namespaces and services dynamically.

Subcommands:
  mcp           Start MCP server for AI assistant integration (no sudo needed)
  version       Show version information`
var Version = "0.0.0"

// KlogWriter captures klog output and reformats it through logrus
type KlogWriter struct{}

// throttleRegex matches k8s client-side throttling messages
var throttleRegex = regexp.MustCompile(`Waited for ([\d.]+)s due to client-side throttling`)

func (w *KlogWriter) Write(p []byte) (n int, err error) {
	msg := strings.TrimSpace(string(p))

	// Skip empty lines and trace detail lines
	if msg == "" || strings.HasPrefix(msg, "Trace[") {
		return len(p), nil
	}

	// Extract and reformat throttling messages
	if matches := throttleRegex.FindStringSubmatch(msg); len(matches) > 1 {
		log.Warnf("K8s API throttled: waited %ss", matches[1])
		return len(p), nil
	}

	// Skip trace headers and request noise
	if strings.Contains(msg, "trace.go:") || strings.Contains(msg, "request.go:") {
		return len(p), nil
	}

	// Skip generic "lost connection" messages (lack useful context)
	if strings.Contains(msg, "lost connection to pod") {
		return len(p), nil
	}

	// Log any other unexpected klog messages at debug level
	log.Debugf("k8s: %s", msg)

	return len(p), nil
}

func init() {
	// Redirect k8s client-go klog output through our formatter
	klog.InitFlags(nil)
	klog.SetOutput(&KlogWriter{})
	klog.LogToStderr(false)

	// quiet version
	args := os.Args[1:]
	if len(args) == 2 && args[0] == "version" && args[1] == "quiet" {
		fmt.Println(Version)
		os.Exit(0)
	}

	log.SetOutput(&LogOutputSplitter{})
	if len(args) > 0 && (args[0] == "completion" || args[0] == "__complete" || args[0] == "mcp") {
		log.SetOutput(io.Discard)
	}
}

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "kubefwd",
		Short: "Expose Kubernetes services for local development.",
		Example: `  sudo kubefwd                      # Idle mode (API enabled, no namespaces)
  sudo kubefwd --tui                # Idle mode with TUI
  sudo kubefwd -n myapp             # Forward services from 'myapp' namespace
  sudo kubefwd -n ns1 -n ns2        # Forward from multiple namespaces
  sudo kubefwd -A                   # Forward from all namespaces
  sudo kubefwd -n myapp -l app=web  # Filter by label selector
  kubefwd mcp                       # Start MCP server for AI integration
  kubefwd version                   # Show version`,

		Long: globalUsage,
	}

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print the version of Kubefwd",
		Example: " kubefwd version\n" +
			" kubefwd version quiet\n",
		Long: ``,
		Run: func(_ *cobra.Command, _ []string) {
			fmt.Printf("Kubefwd version: %s\nhttps://kubefwd.com\n", Version)
		},
	}

	// Pass version to services package for TUI header
	services.Version = Version

	// Pass version to mcp package
	mcp.Version = Version

	cmd.AddCommand(versionCmd, services.Cmd, mcp.Cmd)

	return cmd
}

type LogOutputSplitter struct{}

func (splitter *LogOutputSplitter) Write(p []byte) (n int, err error) {
	if bytes.Contains(p, []byte("level=error")) || bytes.Contains(p, []byte("level=warn")) {
		return os.Stderr.Write(p)
	}
	return os.Stdout.Write(p)
}

// isTUIMode checks if --tui flag is present in args
func isTUIMode() bool {
	for _, arg := range os.Args {
		if arg == "--tui" {
			return true
		}
	}
	return false
}

// isMCPMode checks if running the mcp subcommand
func isMCPMode() bool {
	for _, arg := range os.Args[1:] {
		if arg == "mcp" {
			return true
		}
		// Stop at first non-flag argument
		if !strings.HasPrefix(arg, "-") {
			break
		}
	}
	return false
}

// isKnownSubcommand checks if arg is a known subcommand
func isKnownSubcommand(arg string) bool {
	knownCommands := map[string]bool{
		"services": true, "svcs": true, "svc": true,
		"version": true, "mcp": true,
		"help": true, "completion": true, "__complete": true,
	}
	return knownCommands[arg]
}

func main() {
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp:   true,
		ForceColors:     true,
		TimestampFormat: "15:04:05",
	})

	// If no subcommand provided, default to "svc" (services command)
	// This enables `sudo -E kubefwd` to work directly as idle mode
	args := os.Args[1:]
	if len(args) == 0 || (len(args) > 0 && !isKnownSubcommand(args[0])) {
		// No args, or first arg is a flag/unknown - prepend "svc"
		os.Args = append([]string{os.Args[0], "svc"}, args...)
	}

	// Only print banner in non-TUI, non-MCP mode
	if !isTUIMode() && !isMCPMode() {
		log.Print(` _          _           __             _`)
		log.Print(`| | ___   _| |__   ___ / _|_      ____| |`)
		log.Print(`| |/ / | | | '_ \ / _ \ |_\ \ /\ / / _  |`)
		log.Print(`|   <| |_| | |_) |  __/  _|\ V  V / (_| |`)
		log.Print(`|_|\_\\__,_|_.__/ \___|_|   \_/\_/ \__,_|`)
		log.Print("")
		log.Printf("Version %s", Version)
		log.Print("https://kubefwd.com")
		log.Print("")
	}

	cmd := newRootCmd()

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
