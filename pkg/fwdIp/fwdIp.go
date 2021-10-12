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
	ForwardIPReservations    []string
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
	BaseUnreservedIP      string                  `yaml:"baseUnreservedIP"`
	ServiceConfigurations []*ServiceConfiguration `yaml:"serviceConfigurations"`
}

type ServiceConfiguration struct {
	Name string `yaml:"name"`
	IP   string `yaml:"ip"`
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
	baseUnreservedIP := getBaseUnreservedIP(opts)

	// if a configuration exists use it
	svcConf := getConfigurationForService(opts)
	if svcConf != nil {
		if ip, err := ipFromString(svcConf.IP); err == nil {
			if err := addToRegistry(regKey, opts, ip); err == nil {
				return ip
			}
			log.Errorf("Unable to forward service %s to requested IP %s due to collision", opts.ServiceName, svcConf.IP)
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
		// this recursive call will continue to inc the ip offset until
		// an open slot is found or we go out of bounds
		return determineIP(regKey, opts)
	}
	return ip
}

func addToRegistry(regKey string, opts ForwardIPOpts, ip net.IP) error {
	allocationKey := fmt.Sprintf("%s", ip.String())
	if _, ok := ipRegistry.allocated[allocationKey]; ok {
		// ip/port pair has allready ben allocated
		return fmt.Errorf("ip %s has already been allocated", allocationKey)
	}
	ipRegistry.reg[regKey] = ip
	ipRegistry.allocated[allocationKey] = true
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

func getBaseUnreservedIP(opts ForwardIPOpts) []byte {
	fwdCfg := getForwardConfiguration(opts)
	ip, err := ipFromString(fwdCfg.BaseUnreservedIP)
	if err != nil {
		panic(err)
	}
	return ip
}

func getConfigurationForService(opts ForwardIPOpts) *ServiceConfiguration {
	fwdCfg := getForwardConfiguration(opts)

	for _, svcCfg := range fwdCfg.ServiceConfigurations {
		if svcCfg.Matches(opts) {
			return svcCfg
		}
	}
	return nil
}

func applyCLIPassedReservations(opts ForwardIPOpts, f *ForwardConfiguration) *ForwardConfiguration {
	for _, resStr := range opts.ForwardIPReservations {
		res := ServiceConfigurationFromReservation(resStr)

		overridden := false
		for _, svcCfg := range f.ServiceConfigurations {
			if svcCfg.MatchesName(res) {
				svcCfg.IP = res.IP
				overridden = true
				log.Infof("cli reservation flag overriding config for %s now %s", svcCfg.Name, svcCfg.IP)
			}
		}
		if !overridden {
			f.ServiceConfigurations = append(f.ServiceConfigurations, res)
		}
	}
	return f
}

func getForwardConfiguration(opts ForwardIPOpts) *ForwardConfiguration {
	if forwardConfiguration != nil {
		return forwardConfiguration
	}

	if opts.ForwardConfigurationPath == "" {
		forwardConfiguration = defaultConfiguration
		return applyCLIPassedReservations(opts, forwardConfiguration)
	}

	dat, err := os.ReadFile(opts.ForwardConfigurationPath)
	if err != nil {
		// fall back to existing kubefwd base
		log.Errorf("ForwardConfiguration read error %s", err)
		forwardConfiguration = defaultConfiguration
		return applyCLIPassedReservations(opts, forwardConfiguration)
	}

	conf := &ForwardConfiguration{}
	err = yaml.Unmarshal(dat, conf)
	if err != nil {
		// fall back to existing kubefwd base
		log.Errorf("ForwardConfiguration parse error %s", err)
		forwardConfiguration = defaultConfiguration
		return applyCLIPassedReservations(opts, forwardConfiguration)
	}

	forwardConfiguration = conf
	return applyCLIPassedReservations(opts, forwardConfiguration)
}

func (o ForwardIPOpts) MatchList() []string {
	if o.ClusterN == 0 && o.NamespaceN == 0 {
		return []string{
			fmt.Sprintf("%s", o.PodName),

			fmt.Sprintf("%s", o.ServiceName),

			fmt.Sprintf("%s.%s", o.PodName, o.ServiceName),

			fmt.Sprintf("%s.%s", o.PodName, o.Context),

			fmt.Sprintf("%s.%s", o.ServiceName, o.Context),

			fmt.Sprintf("%s.%s", o.PodName, o.Namespace),
			fmt.Sprintf("%s.%s.svc", o.PodName, o.Namespace),
			fmt.Sprintf("%s.%s.svc.cluster.local", o.PodName, o.Namespace),

			fmt.Sprintf("%s.%s", o.ServiceName, o.Namespace),
			fmt.Sprintf("%s.%s.svc", o.ServiceName, o.Namespace),
			fmt.Sprintf("%s.%s.svc.cluster.local", o.ServiceName, o.Namespace),

			fmt.Sprintf("%s.%s.%s", o.PodName, o.ServiceName, o.Namespace),
			fmt.Sprintf("%s.%s.%s.svc", o.PodName, o.ServiceName, o.Namespace),
			fmt.Sprintf("%s.%s.%s.svc.cluster.local", o.PodName, o.ServiceName, o.Namespace),

			fmt.Sprintf("%s.%s.%s", o.PodName, o.ServiceName, o.Context),

			fmt.Sprintf("%s.%s.%s", o.PodName, o.Namespace, o.Context),
			fmt.Sprintf("%s.%s.svc.%s", o.PodName, o.Namespace, o.Context),
			fmt.Sprintf("%s.%s.svc.cluster.%s", o.PodName, o.Namespace, o.Context),

			fmt.Sprintf("%s.%s.%s", o.ServiceName, o.Namespace, o.Context),
			fmt.Sprintf("%s.%s.svc.%s", o.ServiceName, o.Namespace, o.Context),
			fmt.Sprintf("%s.%s.svc.cluster.%s", o.ServiceName, o.Namespace, o.Context),

			fmt.Sprintf("%s.%s.%s.%s", o.PodName, o.ServiceName, o.Namespace, o.Context),
			fmt.Sprintf("%s.%s.%s.svc.%s", o.PodName, o.ServiceName, o.Namespace, o.Context),
			fmt.Sprintf("%s.%s.%s.svc.cluster.%s", o.PodName, o.ServiceName, o.Namespace, o.Context),
		}
	}

	if o.ClusterN > 0 && o.NamespaceN == 0 {
		return []string{
			fmt.Sprintf("%s.%s", o.PodName, o.Context),

			fmt.Sprintf("%s.%s.%s", o.PodName, o.ServiceName, o.Context),

			fmt.Sprintf("%s.%s", o.ServiceName, o.Context),

			fmt.Sprintf("%s.%s.%s", o.ServiceName, o.Namespace, o.Context),
			fmt.Sprintf("%s.%s.svc.%s", o.ServiceName, o.Namespace, o.Context),
			fmt.Sprintf("%s.%s.svc.cluster.%s", o.ServiceName, o.Namespace, o.Context),

			fmt.Sprintf("%s.%s.%s", o.PodName, o.Namespace, o.Context),
			fmt.Sprintf("%s.%s.svc.%s", o.PodName, o.Namespace, o.Context),
			fmt.Sprintf("%s.%s.svc.cluster.%s", o.PodName, o.Namespace, o.Context),

			fmt.Sprintf("%s.%s.%s.%s", o.PodName, o.ServiceName, o.Namespace, o.Context),
			fmt.Sprintf("%s.%s.%s.svc.%s", o.PodName, o.ServiceName, o.Namespace, o.Context),
			fmt.Sprintf("%s.%s.%s.svc.cluster.%s", o.PodName, o.ServiceName, o.Namespace, o.Context),
		}
	}

	if o.ClusterN == 0 && o.NamespaceN > 0 {
		return []string{
			fmt.Sprintf("%s.%s", o.PodName, o.Namespace),

			fmt.Sprintf("%s.%s", o.ServiceName, o.Namespace),
			fmt.Sprintf("%s.%s.svc", o.ServiceName, o.Namespace),
			fmt.Sprintf("%s.%s.svc.cluster.local", o.ServiceName, o.Namespace),

			fmt.Sprintf("%s.%s.%s", o.PodName, o.ServiceName, o.Namespace),
			fmt.Sprintf("%s.%s.%s.svc", o.PodName, o.ServiceName, o.Namespace),
			fmt.Sprintf("%s.%s.%s.svc.cluster.local", o.PodName, o.ServiceName, o.Namespace),

			fmt.Sprintf("%s.%s.%s", o.PodName, o.Namespace, o.Context),
			fmt.Sprintf("%s.%s.svc.%s", o.PodName, o.Namespace, o.Context),
			fmt.Sprintf("%s.%s.svc.cluster.%s", o.PodName, o.Namespace, o.Context),

			fmt.Sprintf("%s.%s.%s", o.ServiceName, o.Namespace, o.Context),
			fmt.Sprintf("%s.%s.svc.%s", o.ServiceName, o.Namespace, o.Context),
			fmt.Sprintf("%s.%s.svc.cluster.%s", o.ServiceName, o.Namespace, o.Context),

			fmt.Sprintf("%s.%s.%s.%s", o.PodName, o.ServiceName, o.Namespace, o.Context),
			fmt.Sprintf("%s.%s.%s.svc.%s", o.PodName, o.ServiceName, o.Namespace, o.Context),
			fmt.Sprintf("%s.%s.%s.svc.cluster.%s", o.PodName, o.ServiceName, o.Namespace, o.Context),
		}
	}

	return []string{
		fmt.Sprintf("%s.%s.%s", o.PodName, o.Namespace, o.Context),
		fmt.Sprintf("%s.%s.svc.%s", o.PodName, o.Namespace, o.Context),
		fmt.Sprintf("%s.%s.svc.cluster.%s", o.PodName, o.Namespace, o.Context),

		fmt.Sprintf("%s.%s.%s", o.ServiceName, o.Namespace, o.Context),
		fmt.Sprintf("%s.%s.svc.%s", o.ServiceName, o.Namespace, o.Context),
		fmt.Sprintf("%s.%s.svc.cluster.%s", o.ServiceName, o.Namespace, o.Context),

		fmt.Sprintf("%s.%s.%s.%s", o.PodName, o.ServiceName, o.Namespace, o.Context),
		fmt.Sprintf("%s.%s.%s.svc.%s", o.PodName, o.ServiceName, o.Namespace, o.Context),
		fmt.Sprintf("%s.%s.%s.svc.cluster.%s", o.PodName, o.ServiceName, o.Namespace, o.Context),
	}
}

func ServiceConfigurationFromReservation(reservation string) *ServiceConfiguration {
	parts := strings.Split(reservation, ":")
	if len(parts) != 2 {
		return nil
	}
	return &ServiceConfiguration{
		Name: parts[0],
		IP:   parts[1],
	}
}

func (c ServiceConfiguration) String() string {
	return fmt.Sprintf("Name: %s IP:%s", c.Name, c.IP)
}

func (c ServiceConfiguration) Matches(opts ForwardIPOpts) bool {
	matchList := opts.MatchList()
	for _, toMatch := range matchList {
		if c.Name == toMatch {
			return true
		}
	}
	return false
}

func (c ServiceConfiguration) MatchesName(otherCfg *ServiceConfiguration) bool {
	return c.Name == otherCfg.Name
}
