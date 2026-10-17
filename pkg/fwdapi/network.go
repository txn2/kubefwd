package fwdapi

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"runtime"

	log "github.com/sirupsen/logrus"
	"github.com/txn2/txeh"

	"github.com/txn2/kubefwd/pkg/fwdport"
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

// linuxIPCommand is the iproute2 binary used to manage addresses on Linux.
const linuxIPCommand = "ip"

// ipExistsExitCode is the exit status iproute2 returns when the address is
// already present (on add) or already absent (on delete). Both mean the
// loopback is in the state the caller wants.
const ipExistsExitCode = 2

// goos is the platform whose commands the alias functions run. Tests replace
// it to cover every platform's branch.
var goos = runtime.GOOS

// runCommand runs a command and returns its combined output. Tests replace it
// so the alias functions can be exercised without root.
var runCommand = func(name string, args ...string) ([]byte, error) {
	// #nosec G204 -- callers pass fixed commands and flags; the only variable is a net.IP, and no shell is involved.
	return exec.CommandContext(context.Background(), name, args...).CombinedOutput()
}

// alreadyInState reports whether err is iproute2 saying the address is
// already present or already absent.
func alreadyInState(err error) bool {
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr) && exitErr.ExitCode() == ipExistsExitCode
}

// addLoopbackAlias adds an IP alias to the loopback interface
func addLoopbackAlias(ip net.IP) error {
	switch goos {
	case "darwin":
		// macOS: ifconfig lo0 alias <ip> up
		if output, err := runCommand("ifconfig", "lo0", "alias", ip.String(), "up"); err != nil {
			return fmt.Errorf("ifconfig failed: %w (%s)", err, string(output))
		}
	case "linux":
		// Linux: ip addr add <ip>/32 dev lo
		if output, err := runCommand(linuxIPCommand, "addr", "add", ip.String()+"/32", "dev", "lo"); err != nil {
			// Ignore "RTNETLINK answers: File exists" - IP already added
			if alreadyInState(err) {
				return nil
			}
			return fmt.Errorf("ip addr add failed: %w (%s)", err, string(output))
		}
	case "windows":
		// Windows: netsh interface ip add address "Loopback" <ip> 255.255.255.255
		if output, err := runCommand("netsh", "interface", "ip", "add", "address", "Loopback Pseudo-Interface 1", ip.String(), "255.255.255.255"); err != nil {
			return fmt.Errorf("netsh failed: %w (%s)", err, string(output))
		}
	default:
		return fmt.Errorf("unsupported platform: %s", goos)
	}

	log.Debugf("Added loopback alias: %s", ip.String())
	return nil
}

// removeLoopbackAlias removes an IP alias from the loopback interface
func removeLoopbackAlias(ip net.IP) error {
	if ip == nil {
		return nil
	}

	switch goos {
	case "darwin":
		// macOS: ifconfig lo0 -alias <ip>
		if output, err := runCommand("ifconfig", "lo0", "-alias", ip.String()); err != nil {
			return fmt.Errorf("ifconfig -alias failed: %w (%s)", err, string(output))
		}
	case "linux":
		// Linux: ip addr del <ip>/32 dev lo
		if output, err := runCommand(linuxIPCommand, "addr", "del", ip.String()+"/32", "dev", "lo"); err != nil {
			// Ignore "RTNETLINK answers: Cannot assign requested address" - IP not found
			if alreadyInState(err) {
				return nil
			}
			return fmt.Errorf("ip addr del failed: %w (%s)", err, string(output))
		}
	case "windows":
		// Windows: netsh interface ip delete address "Loopback" <ip>
		if output, err := runCommand("netsh", "interface", "ip", "delete", "address", "Loopback Pseudo-Interface 1", ip.String()); err != nil {
			return fmt.Errorf("netsh delete failed: %w (%s)", err, string(output))
		}
	default:
		return fmt.Errorf("unsupported platform: %s", goos)
	}

	log.Debugf("Removed loopback alias: %s", ip.String())
	return nil
}
