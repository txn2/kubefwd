package fwdnet

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
)

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

		iface, err := net.InterfaceByName("lo0")
		if err != nil {
			return net.IP{}, i, err
		}

		addrs, err := iface.Addrs()
		if err != nil {
			return net.IP{}, i, err
		}

		// check the addresses already assigned to the interface
		for _, addr := range addrs {

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
