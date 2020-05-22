package fwdnet

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"sync"

	log "github.com/sirupsen/logrus"
)

var addrs []net.Addr
var initAddresses sync.Once

// getLocalListenAddrs returns the listen addresses for the lo0 interface.
// It will exit if these could not be determined.
func getLocalListenAddrs() []net.Addr {
	initAddresses.Do(func() {
		if addrs == nil {
			iface, err := net.InterfaceByName("lo0")
			if err != nil {
				log.Fatalf("Could not get lo0 netInterface: %s", err)
			}

			addrs, err = iface.Addrs()
			if err != nil {
				log.Fatalf("Could not get lo0 listen addresses: %s", err)
			}
		}
	})

	return addrs
}

// ReadyInterface prepares a local IP address on
// the loopback interface.
func ReadyInterface(a byte, b byte, c byte, d int, port string) (net.IP, int, error) {

	ip := net.IPv4(a, b, c, byte(d))

	// lo means we are probably on linux and not mac
	_, err := net.InterfaceByName("lo")
	if err == nil || runtime.GOOS == "windows" {
		// if no error then check to see if the ip:port are in use
		_, err := net.Dial("tcp", ip.String()+":"+port)
		if err != nil {
			return ip, d + 1, nil
		}

		return ip, d + 1, errors.New("ip and port are in use")
	}

	for i := d; i < 255; i++ {

		ip = net.IPv4(a, b, c, byte(i))

		// check the addresses already assigned to the interface
		for _, addr := range getLocalListenAddrs() {

			// found a match
			if addr.String() == ip.String()+"/8" {
				// found ip, now check for unused port
				conn, err := net.Dial("tcp", ip.String()+":"+port)
				if err != nil {
					return net.IPv4(a, b, c, byte(i)), i + 1, nil
				}
				conn.Close()
			}
		}

		// ip is not in the list of addrs for iface
		cmd := "ifconfig"
		args := []string{"lo0", "alias", ip.String(), "up"}
		if err := exec.Command(cmd, args...).Run(); err != nil {
			fmt.Println("Cannot ifconfig lo0 alias " + ip.String() + " up")
			fmt.Println("Error: " + err.Error())
			os.Exit(1)
		}

		conn, err := net.Dial("tcp", ip.String()+":"+port)
		if err != nil {
			return net.IPv4(a, b, c, byte(i)), i + 1, nil
		}
		conn.Close()

	}

	return net.IP{}, d, errors.New("unable to find an available IP/Port")
}

// RemoveInterfaceAlias can remove the Interface alias after port forwarding.
// if -alias command get err, just print the error and continue.
func RemoveInterfaceAlias(ip net.IP) {
	cmd := "ifconfig"
	args := []string{"lo0", "-alias", ip.String()}
	if err := exec.Command(cmd, args...).Run(); err != nil {
		// suppress for now and @todo research why this would fail
		//fmt.Println("Cannot ifconfig lo0 -alias " + ip.String() + "\r\n" + err.Error())
	}
}
