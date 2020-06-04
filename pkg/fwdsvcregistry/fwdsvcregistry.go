package fwdsvcregistry

import (
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/txn2/kubefwd/pkg/fwdservice"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

// ServicesRegistry is a structure to hold all of the services we need to do portforwarding for.
type ServicesRegistry struct {
	mutex          *sync.Mutex
	services       map[string]*fwdservice.ServiceFWD
	shutDownSignal <-chan struct{}
	doneSignal     chan struct{} // indicates when all services were succesfully shutdown
}

var svcRegistry *ServicesRegistry

func Init(shutDownSignal <-chan struct{}) {
	svcRegistry = &ServicesRegistry{
		mutex:          &sync.Mutex{},
		services:       make(map[string]*fwdservice.ServiceFWD),
		shutDownSignal: shutDownSignal,
		doneSignal:     make(chan struct{}),
	}

	go func() {
		<-svcRegistry.shutDownSignal
		ShutDownAll()
		close(svcRegistry.doneSignal)
	}()
}

func Done() <-chan struct{} {
	if svcRegistry != nil {
		return svcRegistry.doneSignal
	}
	// No registry initialized, return a dummy channel then and close it ourselves
	ch := make(chan struct{})
	close(ch)
	return ch
}

// Add will add this service to the registry of services configured to do forwarding (if it wasn't already configured) and start the portforwarding process.
func Add(svcfwd *fwdservice.ServiceFWD) {
	// If we are already shutting down, don't add a new service anymore.
	select {
	case <-svcRegistry.shutDownSignal:
		return
	default:
	}

	svcRegistry.mutex.Lock()
	defer svcRegistry.mutex.Unlock()

	if _, found := svcRegistry.services[svcfwd.String()]; !found {
		svcRegistry.services[svcfwd.String()] = svcfwd
		log.Debugf("Starting forwarding service %s", svcfwd)
	} else {
		return
	}

	// Start the portforwarding
	go svcfwd.SyncPodForwards(false)

	// Schedule a resync every x minutes to deal with potential connection errors.
	go func() {
		for {
			select {
			case <-time.After(10 * time.Minute):
				svcfwd.SyncPodForwards(false)
			case <-svcfwd.DoneChannel:
				return
			}
		}
	}()
}

// SyncAll does a pod sync for all known services.
func SyncAll() {
	// If we are already shutting down, don't sync services anymore.
	select {
	case <-svcRegistry.shutDownSignal:
		return
	default:
	}

	for _, svc := range svcRegistry.services {
		svc.SyncPodForwards(true)
	}
}

// ShutDownAll will shutdown all active services and remove them from the registry
func ShutDownAll() {
	for name := range svcRegistry.services {
		RemoveByName(name)
	}
	log.Debugf("All services have shut down")
}

// RemoveByName will shutdown and remove the service, identified by svcName.svcNamespace, from the inventory of services, if it was currently being configured to do forwarding.
func RemoveByName(name string) {
	// Pop the service from the registry
	svcRegistry.mutex.Lock()
	svcfwd, found := svcRegistry.services[name]
	if !found {
		svcRegistry.mutex.Unlock()
		return
	}
	delete(svcRegistry.services, name)
	svcRegistry.mutex.Unlock()

	// Synchronously stop the forwarding of all active pods in it
	activePodForwards := svcfwd.ListPodNames()
	log.Debugf("Stopping service %s with %d portforward(s)", svcfwd, len(activePodForwards))

	podsAllDone := &sync.WaitGroup{}
	podsAllDone.Add(len(activePodForwards))
	for _, podName := range activePodForwards {
		go func(podName string) {
			svcfwd.RemovePod(podName)
			podsAllDone.Done()
		}(podName)
	}
	podsAllDone.Wait()

	// Signal that the service has shut down
	close(svcfwd.DoneChannel)
}

// GetByName returns the ServiceFWD object, if it currently being forwarded.
func GetByName(name string) *fwdservice.ServiceFWD {
	svcRegistry.mutex.Lock()
	defer svcRegistry.mutex.Unlock()
	return svcRegistry.services[name]
}
