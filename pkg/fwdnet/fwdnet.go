package fwdnet

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"

	"github.com/txn2/kubefwd/pkg/fwdIp"
)

// ReadyInterface prepares a local IP address on
// the loopback interface.
func ReadyInterface(opts fwdIp.ForwardIPOpts) (net.IP, error) {

	ip, _ := fwdIp.GetIp(opts)

	// lo means we are probably on linux and not mac
	_, err := net.InterfaceByName("lo")
	if err == nil || runtime.GOOS == "windows" {
		// TODO - Ignore if this is a host-local service (already running)
		// if no error then check to see if the ip:port are in use
		_, err := net.Dial("tcp", ip.String()+":"+opts.Port)
		if err != nil {
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
			conn, err := net.Dial("tcp", ip.String()+":"+opts.Port)
			if err != nil {
				return ip, nil
			}
			_ = conn.Close()
		}
	}

	// ip is not in the list of addrs for networkInterface
	cmd := "ifconfig"
	args := []string{"lo0", "alias", ip.String(), "up"}
	if err := exec.Command(cmd, args...).Run(); err != nil {
		fmt.Println("Cannot ifconfig lo0 alias " + ip.String() + " up")
		fmt.Println("Error: " + err.Error())
		os.Exit(1)
	}

	conn, err := net.Dial("tcp", ip.String()+":"+opts.Port)
	if err != nil {
		return ip, nil
	}
	_ = conn.Close()

	return net.IP{}, errors.New("unable to find an available IP/Port")
}

// RemoveInterfaceAlias can remove the Interface alias after port forwarding.
// if -alias command get err, just print the error and continue.
func RemoveInterfaceAlias(ip net.IP) {
	cmd := "ifconfig"
	args := []string{"lo0", "-alias", ip.String()}
	if err := exec.Command(cmd, args...).Run(); err != nil {
		// suppress for now
		// @todo research alternative to ifconfig
		// @todo suggest ifconfig or alternative
		// @todo research libs for interface management
		//fmt.Println("Cannot ifconfig lo0 -alias " + ip.String() + "\r\n" + err.Error())
	}
}
