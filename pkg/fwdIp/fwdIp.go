package fwdIp

import (
	"fmt"
	"net"
	"sync"
)

// Registry is a structure to create and hold all of the
// IP address assignments
type Registry struct {
	mutex *sync.Mutex
	inc   map[int]map[int]int
	reg   map[string]net.IP
}

var ipRegistry *Registry

// Init
func init() {
	ipRegistry = &Registry{
		mutex: &sync.Mutex{},
		// counter for the service cluster and namespace
		inc: map[int]map[int]int{0: {0: 0}},
		reg: make(map[string]net.IP),
	}
}

func GetIp(svcName string, podName string, clusterN int, NamespaceN int) (net.IP, error) {
	ipRegistry.mutex.Lock()
	defer ipRegistry.mutex.Unlock()

	regKey := fmt.Sprintf("%d-%d-%s-%s", clusterN, NamespaceN, svcName, podName)

	if ip, ok := ipRegistry.reg[regKey]; ok {
		return ip, nil
	}

	if ipRegistry.inc[clusterN] == nil {
		ipRegistry.inc[clusterN] = map[int]int{0: 0}
	}

	// @TODO check ranges
	ip := net.IP{127, 1, 27, 1}.To4()
	ip[1] += byte(clusterN)
	ip[2] += byte(NamespaceN)
	ip[3] += byte(ipRegistry.inc[clusterN][NamespaceN])

	ipRegistry.inc[clusterN][NamespaceN]++
	ipRegistry.reg[regKey] = ip

	return ip, nil
}
