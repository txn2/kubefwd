package fwdmetrics

import (
	"sync"
	"time"
)

// Registry is the global registry for all bandwidth metrics.
//
// Lock ordering: When acquiring multiple locks, always acquire Registry.mu BEFORE
// ServiceMetrics.mu to avoid deadlocks. The takeSample() method demonstrates this
// pattern: it holds Registry.mu (RLock) while iterating services, then acquires
// ServiceMetrics.mu (RLock) for each service.
type Registry struct {
	services map[string]*ServiceMetrics
	mu       sync.RWMutex
	ticker   *time.Ticker
	stopCh   chan struct{}
	started  bool
	wg       sync.WaitGroup // Tracks running goroutines for graceful shutdown
}

var globalRegistry *Registry
var registryOnce sync.Once

// GetRegistry returns the singleton metrics registry
func GetRegistry() *Registry {
	registryOnce.Do(func() {
		globalRegistry = &Registry{
			services: make(map[string]*ServiceMetrics),
			stopCh:   make(chan struct{}),
		}
	})
	return globalRegistry
}

// Start begins the sampling ticker for rate calculation
func (r *Registry) Start() {
	r.mu.Lock()
	if r.started {
		r.mu.Unlock()
		return
	}
	r.started = true
	r.mu.Unlock()

	r.ticker = time.NewTicker(1 * time.Second)
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		for {
			select {
			case <-r.ticker.C:
				r.takeSample()
			case <-r.stopCh:
				r.ticker.Stop()
				return
			}
		}
	}()
}

// takeSample records current counters for all port forwards
func (r *Registry) takeSample() {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, svc := range r.services {
		svc.mu.RLock()
		for _, pf := range svc.PortForwards {
			pf.RecordSample()
		}
		svc.mu.RUnlock()
	}
}

// Stop shuts down the metrics registry and waits for the sampling goroutine to exit
func (r *Registry) Stop() {
	r.mu.Lock()
	if !r.started {
		r.mu.Unlock()
		return
	}
	close(r.stopCh)
	r.started = false
	r.mu.Unlock()

	// Wait for the sampling goroutine to exit
	r.wg.Wait()
}

// RegisterService adds a service to the registry
func (r *Registry) RegisterService(svc *ServiceMetrics) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.services[svc.Key()] = svc
}

// UnregisterService removes a service from the registry
func (r *Registry) UnregisterService(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.services, key)
}

// GetService returns a service by key
func (r *Registry) GetService(key string) *ServiceMetrics {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.services[key]
}

// RegisterPortForward adds a port forward to its parent service
func (r *Registry) RegisterPortForward(serviceKey string, pf *PortForwardMetrics) {
	r.mu.Lock()
	svc, ok := r.services[serviceKey]
	if !ok {
		// Create service if it doesn't exist
		svc = NewServiceMetrics(pf.ServiceName, pf.Namespace, pf.Context)
		r.services[svc.Key()] = svc
	}
	r.mu.Unlock()

	svc.AddPortForward(pf)
}

// UnregisterPortForward removes a port forward from its parent service
func (r *Registry) UnregisterPortForward(serviceKey, podName, localPort string) {
	r.mu.Lock()
	svc, ok := r.services[serviceKey]
	if !ok {
		r.mu.Unlock()
		return
	}

	svc.RemovePortForward(podName, localPort)

	// Remove service if no port forwards left
	if svc.Count() == 0 {
		delete(r.services, serviceKey)
	}
	r.mu.Unlock()
}

// GetAllServices returns all registered services
func (r *Registry) GetAllServices() []*ServiceMetrics {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*ServiceMetrics, 0, len(r.services))
	for _, svc := range r.services {
		result = append(result, svc)
	}
	return result
}

// ServiceCount returns the number of registered services
func (r *Registry) ServiceCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.services)
}

// PortForwardCount returns the total number of port forwards across all services
func (r *Registry) PortForwardCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	for _, svc := range r.services {
		count += svc.Count()
	}
	return count
}

// GetTotals returns aggregated totals across all services
func (r *Registry) GetTotals() (bytesIn, bytesOut uint64, rateIn, rateOut float64) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, svc := range r.services {
		bi, bo, ri, ro := svc.GetTotals()
		bytesIn += bi
		bytesOut += bo
		rateIn += ri
		rateOut += ro
	}
	return
}
