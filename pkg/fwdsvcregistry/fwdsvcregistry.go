package fwdsvcregistry

import (
	"sync"

	log "github.com/sirupsen/logrus"
	"github.com/txn2/kubefwd/pkg/fwdport"
	"github.com/txn2/kubefwd/pkg/fwdservice"
	"github.com/txn2/kubefwd/pkg/fwdtui"
	"github.com/txn2/kubefwd/pkg/fwdtui/events"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

// ServicesRegistry is a structure to hold all of the kubernetes
// services to do port-forwarding for.
type ServicesRegistry struct {
	mutex          *sync.Mutex
	services       map[string]*fwdservice.ServiceFWD
	shutDownSignal <-chan struct{}
	doneSignal     chan struct{} // indicates when all services were successfully shutdown
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

// Get retrieves a service from the registry by its key (name.namespace.context)
func Get(key string) *fwdservice.ServiceFWD {
	svcRegistry.mutex.Lock()
	defer svcRegistry.mutex.Unlock()
	return svcRegistry.services[key]
}

// GetAll returns a slice of all services in the registry
func GetAll() []*fwdservice.ServiceFWD {
	if svcRegistry == nil {
		return nil
	}
	svcRegistry.mutex.Lock()
	defer svcRegistry.mutex.Unlock()
	result := make([]*fwdservice.ServiceFWD, 0, len(svcRegistry.services))
	for _, svc := range svcRegistry.services {
		result = append(result, svc)
	}
	return result
}

// GetByNamespace returns all services for a given namespace and context
func GetByNamespace(namespace, context string) []*fwdservice.ServiceFWD {
	svcRegistry.mutex.Lock()
	defer svcRegistry.mutex.Unlock()
	var result []*fwdservice.ServiceFWD
	for _, svc := range svcRegistry.services {
		if svc.Namespace == namespace && svc.Context == context {
			result = append(result, svc)
		}
	}
	return result
}

// Add will add this service to the registry of services configured to do forwarding
// (if it wasn't already configured) and start the port-forwarding process.
func Add(serviceFwd *fwdservice.ServiceFWD) {
	// If we are already shutting down, don't add a new service anymore.
	select {
	case <-svcRegistry.shutDownSignal:
		return
	default:
	}

	svcRegistry.mutex.Lock()
	defer svcRegistry.mutex.Unlock()

	if _, found := svcRegistry.services[serviceFwd.String()]; found {
		log.Debugf("Registry: found existing service %s", serviceFwd.String())
		return
	}

	svcRegistry.services[serviceFwd.String()] = serviceFwd
	log.Debugf("Registry: Start forwarding service %s", serviceFwd)

	// Emit event for TUI
	if fwdtui.EventsEnabled() {
		fwdtui.Emit(events.NewServiceEvent(
			events.ServiceAdded,
			serviceFwd.Svc.Name,
			serviceFwd.Namespace,
			serviceFwd.Context,
		))
	}

	// Start port forwarding
	go serviceFwd.SyncPodForwards(false)
}

// ShutDownAll will shutdown all active services and remove them from the registry
func ShutDownAll() {
	// Get a snapshot of service names while holding the lock
	svcRegistry.mutex.Lock()
	serviceNames := make([]string, 0, len(svcRegistry.services))
	for name := range svcRegistry.services {
		serviceNames = append(serviceNames, name)
	}
	svcRegistry.mutex.Unlock()

	// Now remove them (RemoveByName handles its own locking)
	for _, name := range serviceNames {
		RemoveByName(name)
	}

	// Stop all global pod informers
	fwdport.StopGlobalPodInformer()

	log.Debugf("Registry: All services have shut down")
}

// RemoveByName will shutdown and remove the service, identified by svcName.svcNamespace,
// from the inventory of services, if it was currently being configured to do forwarding.
func RemoveByName(name string) {

	log.Debugf("Registry: Removing service %s", name)

	// Pop the service from the registry
	svcRegistry.mutex.Lock()
	serviceFwd, found := svcRegistry.services[name]
	if !found {
		log.Debugf("Registry: Did not find service %s.", name)
		svcRegistry.mutex.Unlock()
		return
	}
	delete(svcRegistry.services, name)
	svcRegistry.mutex.Unlock()

	// Signal shutdown BEFORE stopping pods so goroutines know not to reconnect
	close(serviceFwd.DoneChannel)

	// Emit event for TUI
	if fwdtui.EventsEnabled() {
		fwdtui.Emit(events.NewServiceEvent(
			events.ServiceRemoved,
			serviceFwd.Svc.Name,
			serviceFwd.Namespace,
			serviceFwd.Context,
		))
	}

	// Synchronously stop the forwarding of all active pods in it
	activePodForwards := serviceFwd.ListServicePodNames()
	log.Debugf("Registry: Stopping service %s with %d port-forward(s)", serviceFwd, len(activePodForwards))

	podsAllDone := &sync.WaitGroup{}
	podsAllDone.Add(len(activePodForwards))
	for _, podName := range activePodForwards {
		go func(podName string) {
			serviceFwd.RemoveServicePod(podName)
			podsAllDone.Done()
		}(podName)
	}
	podsAllDone.Wait()
}
