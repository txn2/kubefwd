package fwdsvcregistry

import (
	log "github.com/sirupsen/logrus"
	"github.com/txn2/kubefwd/pkg/fwdservice"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"sync"
)

// ServicesRegistry is a structure to hold all of the kubernetes
// services to do port-forwarding for.
type ServicesRegistry struct {
	mutex          *sync.Mutex
	services       map[string]*fwdservice.ServiceFWD
	shutDownSignal <-chan struct{}
	doneSignal     chan struct{} // indicates when all services were succesfully shutdown
}

var svcRegistry *ServicesRegistry

// Init
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

// Done
func Done() <-chan struct{} {
	if svcRegistry != nil {
		return svcRegistry.doneSignal
	}
	// No registry initialized, return a dummy channel then and close it ourselves
	ch := make(chan struct{})
	close(ch)
	return ch
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

	// Start port forwarding
	go serviceFwd.SyncPodForwards(false)

	// Schedule a re sync every x minutes to deal with potential connection errors.
	// @TODO review the need for this, if we keep it make if configurable
	// @TODO this causes the services to try and bind a second time to the local ports and fails --cjimti
	//
	//go func() {
	//	for {
	//		select {
	//		case <-time.After(10 * time.Second):
	//			serviceFwd.SyncPodForwards(false)
	//		case <-serviceFwd.DoneChannel:
	//			return
	//		}
	//	}
	//}()
}

// SyncAll does a pod sync for all known services.
//func SyncAll() {
//	// If we are already shutting down, don't sync services anymore.
//	select {
//	case <-svcRegistry.shutDownSignal:
//		return
//	default:
//	}
//
//	for _, svc := range svcRegistry.services {
//		svc.SyncPodForwards(true)
//	}
//}

// ShutDownAll will shutdown all active services and remove them from the registry
func ShutDownAll() {
	for name := range svcRegistry.services {
		RemoveByName(name)
	}
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

	// Signal that the service has shut down
	close(serviceFwd.DoneChannel)
}
