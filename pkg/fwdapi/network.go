package fwdapi

import (
	"fmt"
	"net"
	"os/exec"
	"runtime"

	log "github.com/sirupsen/logrus"
	"github.com/txn2/kubefwd/pkg/fwdport"
	"github.com/txn2/txeh"
)

// SetupAPINetwork configures the loopback IP alias and hosts entry for the API
func SetupAPINetwork(hostFile *fwdport.HostFileWithLock) error {
	apiIP := net.ParseIP(APIIP)
	if apiIP == nil {
		return fmt.Errorf("invalid API IP: %s", APIIP)
	}

	// Add IP alias to loopback interface
	if err := addLoopbackAlias(apiIP); err != nil {
		return fmt.Errorf("failed to add API IP alias: %w", err)
	}

	// Add hosts entry for kubefwd.internal
	if hostFile != nil {
		hostFile.Lock()
		hostFile.Hosts.AddHost(APIIP, Hostname)
		if err := hostFile.Hosts.Save(); err != nil {
			hostFile.Unlock()
			return fmt.Errorf("failed to add hosts entry: %w", err)
		}
		hostFile.Unlock()
		log.Debugf("Added hosts entry: %s -> %s", Hostname, APIIP)
	}

	return nil
}

// CleanupAPINetwork removes the API network configuration
func CleanupAPINetwork(hostFile *txeh.Hosts) error {
	// Remove hosts entry
	if hostFile != nil {
		hostFile.RemoveHost(Hostname)
		if err := hostFile.Save(); err != nil {
			log.Warnf("Failed to remove hosts entry: %v", err)
		}
	}

	// Remove IP alias from loopback interface
	if err := removeLoopbackAlias(net.ParseIP(APIIP)); err != nil {
		log.Warnf("Failed to remove API IP alias: %v", err)
	}

	return nil
}

// addLoopbackAlias adds an IP alias to the loopback interface
func addLoopbackAlias(ip net.IP) error {
	switch runtime.GOOS {
	case "darwin":
		// macOS: ifconfig lo0 alias <ip> up
		cmd := exec.Command("ifconfig", "lo0", "alias", ip.String(), "up")
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("ifconfig failed: %w (%s)", err, string(output))
		}
	case "linux":
		// Linux: ip addr add <ip>/32 dev lo
		cmd := exec.Command("ip", "addr", "add", ip.String()+"/32", "dev", "lo")
		if output, err := cmd.CombinedOutput(); err != nil {
			// Ignore "RTNETLINK answers: File exists" - IP already added
			if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 2 {
				return nil
			}
			return fmt.Errorf("ip addr add failed: %w (%s)", err, string(output))
		}
	case "windows":
		// Windows: netsh interface ip add address "Loopback" <ip> 255.255.255.255
		cmd := exec.Command("netsh", "interface", "ip", "add", "address", "Loopback Pseudo-Interface 1", ip.String(), "255.255.255.255")
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("netsh failed: %w (%s)", err, string(output))
		}
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	log.Debugf("Added loopback alias: %s", ip.String())
	return nil
}

// removeLoopbackAlias removes an IP alias from the loopback interface
func removeLoopbackAlias(ip net.IP) error {
	if ip == nil {
		return nil
	}

	switch runtime.GOOS {
	case "darwin":
		// macOS: ifconfig lo0 -alias <ip>
		cmd := exec.Command("ifconfig", "lo0", "-alias", ip.String())
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("ifconfig -alias failed: %w (%s)", err, string(output))
		}
	case "linux":
		// Linux: ip addr del <ip>/32 dev lo
		cmd := exec.Command("ip", "addr", "del", ip.String()+"/32", "dev", "lo")
		if output, err := cmd.CombinedOutput(); err != nil {
			// Ignore "RTNETLINK answers: Cannot assign requested address" - IP not found
			if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 2 {
				return nil
			}
			return fmt.Errorf("ip addr del failed: %w (%s)", err, string(output))
		}
	case "windows":
		// Windows: netsh interface ip delete address "Loopback" <ip>
		cmd := exec.Command("netsh", "interface", "ip", "delete", "address", "Loopback Pseudo-Interface 1", ip.String())
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("netsh delete failed: %w (%s)", err, string(output))
		}
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	log.Debugf("Removed loopback alias: %s", ip.String())
	return nil
}
