package fwdnet

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/txn2/kubefwd/pkg/fwdip"
)

// portProbeTimeout bounds the dial portInUse makes against a loopback address.
const portProbeTimeout = 2 * time.Second

// portInUse reports whether something is already listening on ip:port. A
// failed dial means the port is free, so the dial error is the answer, not a
// failure to report.
func portInUse(ip net.IP, port string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), portProbeTimeout)
	defer cancel()
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(ip.String(), port))
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// ReadyInterface prepares a local IP address on
// the loopback interface.
func ReadyInterface(opts fwdip.ForwardIPOpts) (net.IP, error) {
	return getManager().ReadyInterface(opts)
}

// RemoveInterfaceAlias can remove the Interface alias after port forwarding.
// if -alias command get err, just print the error and continue.
func RemoveInterfaceAlias(ip net.IP) {
	getManager().RemoveInterfaceAlias(ip)
}

// ReadyInterface implements InterfaceManager for the default production implementation.
func (d *defaultInterfaceManager) ReadyInterface(opts fwdip.ForwardIPOpts) (net.IP, error) {
	ip, err := fwdip.GetIP(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to allocate IP: %w", err)
	}

	// lo means we are probably on linux and not mac
	_, err = net.InterfaceByName("lo")
	if err == nil || runtime.GOOS == "windows" {
		// if no error then check to see if the ip:port are in use
		if !portInUse(ip, opts.Port) {
			return ip, nil
		}

		return ip, errors.New("ip and port are in use")
	}

	networkInterface, err := net.InterfaceByName("lo0")
	if err != nil {
		return net.IP{}, err
	}

	addrs, err := networkInterface.Addrs()
	if err != nil {
		return net.IP{}, err
	}

	// check the addresses already assigned to the interface
	for _, addr := range addrs {
		// found a match
		if addr.String() == ip.String()+"/8" {
			// found ip, now check for unused port
			if !portInUse(ip, opts.Port) {
				return ip, nil
			}
		}
	}

	// ip is not in the list of addrs for networkInterface
	// #nosec G204 -- fixed command and flags; ip is a net.IP and no shell is involved.
	if err := exec.CommandContext(context.Background(), "ifconfig", "lo0", "alias", ip.String(), "up").Run(); err != nil {
		return nil, fmt.Errorf("cannot ifconfig lo0 alias %s up: %w", ip.String(), err)
	}

	if !portInUse(ip, opts.Port) {
		return ip, nil
	}

	return net.IP{}, errors.New("unable to find an available IP/Port")
}

// RemoveInterfaceAlias implements InterfaceManager for the default production implementation.
func (d *defaultInterfaceManager) RemoveInterfaceAlias(ip net.IP) {
	// #nosec G204 -- fixed command and flags; ip is a net.IP and no shell is involved.
	if err := exec.CommandContext(context.Background(), "ifconfig", "lo0", "-alias", ip.String()).Run(); err != nil {
		// Log at debug level - alias removal errors are expected during cleanup
		// when the alias was never created or already removed
		log.Debugf("Note: could not remove interface alias %s: %v", ip.String(), err)
	}
}
