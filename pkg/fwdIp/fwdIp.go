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

type ForwardIPOpts struct {
	ServiceName              string
	PodName                  string
	Context                  string
	ClusterN                 int
	NamespaceN               int
	Namespace                string
	Port                     string
	ForwardConfigurationPath string
}

// Registry is a structure to create and hold all of the
// IP address assignments
type Registry struct {
	mutex     *sync.Mutex
	inc       map[int]map[int]int
	reg       map[string]net.IP
	allocated map[string]bool
}

type ForwardConfiguration struct {
	BaseUnreservedIP      string                 `yaml:"baseUnreservedIP"`
	ServiceConfigurations []ServiceConfiguration `yaml:"serviceConfigurations"`
}

type ServiceConfiguration struct {
	Context     string `yaml:"context"`
	Namespace   string `yaml:"namespace"`
	ServiceName string `yaml:"serviceName"`
	IP          string `yaml:"ip"`
}

var ipRegistry *Registry
var forwardConfiguration *ForwardConfiguration
var defaultConfiguration = &ForwardConfiguration{BaseUnreservedIP: "127.1.27.1"}

// Init
func init() {
	ipRegistry = &Registry{
		mutex: &sync.Mutex{},
		// counter for the service cluster and namespace
		inc:       map[int]map[int]int{0: {0: 0}},
		reg:       make(map[string]net.IP),
		allocated: make(map[string]bool),
	}
}

func GetIp(opts ForwardIPOpts) (net.IP, error) {
	ipRegistry.mutex.Lock()
	defer ipRegistry.mutex.Unlock()

	regKey := fmt.Sprintf("%d-%d-%s-%s", opts.ClusterN, opts.NamespaceN, opts.ServiceName, opts.PodName)

	if ip, ok := ipRegistry.reg[regKey]; ok {
		return ip, nil
	}

	return determineIP(regKey, opts), nil
}

func determineIP(regKey string, opts ForwardIPOpts) net.IP {
	baseUnreservedIP := getBaseUnreservedIP(opts.ForwardConfigurationPath)

	// if a configuration exists use it
	svcConf := getConfigurationForService(opts)
	if svcConf != nil {
		if ip, err := ipFromString(svcConf.IP); err == nil {
			if svcConf.HasWildcardContext() {
				// for each cluster increment octet 1
				// if the service could exist in multiple contexts
				ip[1] = baseUnreservedIP[1] + byte(opts.ClusterN)
			}

			if svcConf.HasWildcardNamespace() {
				// if the service could exist in multiple namespaces
				ip[2] = baseUnreservedIP[2] + byte(opts.NamespaceN)
			}

			if err := addToRegistry(regKey, opts, ip); err != nil {
				panic(fmt.Sprintf("Unable to forward service. %s", err))
			}
			return ip
		} else {
			log.Errorf("Invalid service ip format %s %s", svcConf.String(), err)
		}
	}

	// fall back to previous implementation if svcConf not provided
	if ipRegistry.inc[opts.ClusterN] == nil {
		ipRegistry.inc[opts.ClusterN] = map[int]int{0: 0}
	}

	// bounds check
	if opts.ClusterN > 255 ||
		opts.NamespaceN > 255 ||
		ipRegistry.inc[opts.ClusterN][opts.NamespaceN] > 255 {
		panic("Ip address generation has run out of bounds.")
	}

	ip := baseUnreservedIP
	ip[1] += byte(opts.ClusterN)
	ip[2] += byte(opts.NamespaceN)
	ip[3] += byte(ipRegistry.inc[opts.ClusterN][opts.NamespaceN])

	ipRegistry.inc[opts.ClusterN][opts.NamespaceN]++
	if err := addToRegistry(regKey, opts, ip); err != nil {
		// failure to allocate on ip
		log.Error(err)
		return determineIP(regKey, opts)
	}
	return ip
}

func addToRegistry(regKey string, opts ForwardIPOpts, ip net.IP) error {
	allocationKey := fmt.Sprintf("%s:%s", ip.String(), opts.Port)
	if _, ok := ipRegistry.allocated[allocationKey]; ok {
		// ip/port pair has allready ben allocated
		return fmt.Errorf("ip/port pair %s has already been allocated", allocationKey)
	}
	ipRegistry.reg[regKey] = ip
	return nil
}

func ipFromString(ipStr string) (net.IP, error) {
	ipParts := strings.Split(ipStr, ".")

	octet0, err := strconv.Atoi(ipParts[0])
	if err != nil {
		return nil, fmt.Errorf("Unable to parse BaseIP octet 0")
	}
	octet1, err := strconv.Atoi(ipParts[1])
	if err != nil {
		return nil, fmt.Errorf("Unable to parse BaseIP octet 1")
	}
	octet2, err := strconv.Atoi(ipParts[2])
	if err != nil {
		return nil, fmt.Errorf("Unable to parse BaseIP octet 2")
	}
	octet3, err := strconv.Atoi(ipParts[3])
	if err != nil {
		return nil, fmt.Errorf("Unable to parse BaseIP octet 3")
	}
	return net.IP{byte(octet0), byte(octet1), byte(octet2), byte(octet3)}.To4(), nil
}

func getBaseUnreservedIP(forwardConfigurationPath string) []byte {
	fwdCfg := getForwardConfiguration(forwardConfigurationPath)
	ip, err := ipFromString(fwdCfg.BaseUnreservedIP)
	if err != nil {
		panic(err)
	}
	return ip
}

func getConfigurationForService(opts ForwardIPOpts) *ServiceConfiguration {
	fwdCfg := getForwardConfiguration(opts.ForwardConfigurationPath)
	for _, c := range fwdCfg.ServiceConfigurations {
		if c.ServiceName == opts.ServiceName &&
			(c.Namespace == "*" || c.Namespace == opts.Namespace) {
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

func (c ServiceConfiguration) HasWildcardContext() bool {
	return c.Context == "*"
}

func (c ServiceConfiguration) HasWildcardNamespace() bool {
	return c.Namespace == "*"
}

func (c ServiceConfiguration) String() string {
	return fmt.Sprintf("Ctx: %s Ns:%s Svc:%s IP:%s", c.Context, c.Namespace, c.ServiceName, c.IP)
}
