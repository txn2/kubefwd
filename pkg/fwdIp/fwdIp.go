package fwdIp

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"

	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

// Registry is a structure to create and hold all of the
// IP address assignments
type Registry struct {
	mutex *sync.Mutex
	inc   map[int]map[int]int
	reg   map[string]net.IP
}

type ForwardConfiguration struct {
	BaseIP                string                 `yaml:"baseIP"`
	ServiceConfigurations []ServiceConfiguration `yaml:"serviceConfigurations"`
}

type ServiceConfiguration struct {
	ServiceName   string `yaml:"serviceName"`
	FinalOctet    int    `yaml:"finalOctet"`
	FailOnOverlap bool   `yaml:"failOnOverlap"`
}

var ipRegistry *Registry
var forwardConfiguration *ForwardConfiguration
var defaultConfiguration = &ForwardConfiguration{BaseIP: "127.1.27.1"}

// Init
func init() {
	ipRegistry = &Registry{
		mutex: &sync.Mutex{},
		// counter for the service cluster and namespace
		inc: map[int]map[int]int{0: {0: 0}},
		reg: make(map[string]net.IP),
	}
}

func GetIp(svcName string, podName string, clusterN int, NamespaceN int, forwardConfigurationPath string) (net.IP, error) {
	ipRegistry.mutex.Lock()
	defer ipRegistry.mutex.Unlock()

	regKey := fmt.Sprintf("%d-%d-%s-%s", clusterN, NamespaceN, svcName, podName)

	if ip, ok := ipRegistry.reg[regKey]; ok {
		return ip, nil
	}

	return determineIP(regKey, svcName, podName, clusterN, NamespaceN, forwardConfigurationPath), nil
}

func determineIP(regKey string, svcName string, podName string, clusterN int, NamespaceN int, forwardConfigurationPath string) net.IP {
	baseIP := getBaseIP(forwardConfigurationPath)

	// if a configuration exists use it
	svcConf := getConfigurationForService(svcName, forwardConfigurationPath)
	if svcConf != nil {
		ip := net.IP{baseIP[0], baseIP[1], baseIP[2], byte(svcConf.FinalOctet)}.To4()
		ipRegistry.reg[regKey] = ip
		return ip
	}

	// fall back to previous implementation
	if ipRegistry.inc[clusterN] == nil {
		ipRegistry.inc[clusterN] = map[int]int{0: 0}
	}

	// @TODO check ranges
	ip := net.IP{baseIP[0], baseIP[1], baseIP[2], baseIP[3]}.To4()
	ip[1] += byte(clusterN)
	ip[2] += byte(NamespaceN)
	ip[3] += byte(ipRegistry.inc[clusterN][NamespaceN])

	ipRegistry.inc[clusterN][NamespaceN]++
	ipRegistry.reg[regKey] = ip

	return ip
}

func getBaseIP(forwardConfigurationPath string) []byte {
	fwdCfg := getForwardConfiguration(forwardConfigurationPath)
	ipParts := strings.Split(fwdCfg.BaseIP, ".")

	octet0, err := strconv.Atoi(ipParts[0])
	if err != nil {
		panic("Unable to parse BaseIP octet 0")
	}
	octet1, err := strconv.Atoi(ipParts[1])
	if err != nil {
		panic("Unable to parse BaseIP octet 1")
	}
	octet2, err := strconv.Atoi(ipParts[2])
	if err != nil {
		panic("Unable to parse BaseIP octet 2")
	}
	octet3, err := strconv.Atoi(ipParts[3])
	if err != nil {
		panic("Unable to parse BaseIP octet 3")
	}
	return []byte{byte(octet0), byte(octet1), byte(octet2), byte(octet3)}
}

func getConfigurationForService(serviceName string, forwardConfigurationPath string) *ServiceConfiguration {
	fwdCfg := getForwardConfiguration(forwardConfigurationPath)
	for _, c := range fwdCfg.ServiceConfigurations {
		if c.ServiceName == serviceName {
			return &c
		}
	}
	return nil
}

func getForwardConfiguration(forwardConfigurationPath string) *ForwardConfiguration {
	if forwardConfiguration != nil {
		return forwardConfiguration
	}

	if forwardConfigurationPath == "" {
		forwardConfiguration = defaultConfiguration
		return forwardConfiguration
	}

	dat, err := os.ReadFile(forwardConfigurationPath)
	if err != nil {
		// fall back to existing kubefwd base
		log.Error(fmt.Sprintf("ForwardConfiguration read error %s", err))
		forwardConfiguration = defaultConfiguration
		return forwardConfiguration
	}

	conf := &ForwardConfiguration{}
	err = yaml.Unmarshal(dat, conf)
	if err != nil {
		// fall back to existing kubefwd base
		log.Error(fmt.Sprintf("ForwardConfiguration parse error %s", err))
		forwardConfiguration = defaultConfiguration
		return forwardConfiguration
	}

	forwardConfiguration = conf
	return forwardConfiguration
}
