package fwdmetrics

import (
	"time"
)

// PortForwardSnapshot is an immutable snapshot for TUI rendering
type PortForwardSnapshot struct {
	ServiceName    string
	Namespace      string
	Context        string
	PodName        string
	LocalIP        string
	LocalPort      string
	PodPort        string
	BytesIn        uint64
	BytesOut       uint64
	RateIn         float64 // bytes/sec instantaneous
	RateOut        float64 // bytes/sec instantaneous
	AvgRateIn      float64 // bytes/sec 10-second average
	AvgRateOut     float64 // bytes/sec 10-second average
	ConnectedAt    time.Time
	LastActivityAt time.Time
	History        []RateSample   // for graphing
	HTTPLogs       []HTTPLogEntry // recent HTTP request/response logs
}

// ServiceSnapshot aggregates all port forwards for a service
type ServiceSnapshot struct {
	ServiceName   string
	Namespace     string
	Context       string
	TotalBytesIn  uint64
	TotalBytesOut uint64
	TotalRateIn   float64
	TotalRateOut  float64
	PortForwards  []PortForwardSnapshot
}

// GetSnapshot creates an immutable snapshot of port forward metrics
func (m *PortForwardMetrics) GetSnapshot() PortForwardSnapshot {
	rateIn, rateOut := m.GetInstantRate()
	avgRateIn, avgRateOut := m.GetAverageRate(10)

	snapshot := PortForwardSnapshot{
		ServiceName:    m.ServiceName,
		Namespace:      m.Namespace,
		Context:        m.Context,
		PodName:        m.PodName,
		LocalIP:        m.LocalIP,
		LocalPort:      m.LocalPort,
		PodPort:        m.PodPort,
		BytesIn:        m.GetBytesIn(),
		BytesOut:       m.GetBytesOut(),
		RateIn:         rateIn,
		RateOut:        rateOut,
		AvgRateIn:      avgRateIn,
		AvgRateOut:     avgRateOut,
		ConnectedAt:    m.ConnectedAt,
		LastActivityAt: m.GetLastActivity(),
		History:        m.GetHistory(30),
	}

	// Include HTTP logs if sniffing is enabled
	if m.httpSniffer != nil {
		snapshot.HTTPLogs = m.httpSniffer.GetAllLogs()
	}

	return snapshot
}

// GetSnapshot creates an immutable snapshot of service metrics
func (s *ServiceMetrics) GetSnapshot() ServiceSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	snapshot := ServiceSnapshot{
		ServiceName:  s.ServiceName,
		Namespace:    s.Namespace,
		Context:      s.Context,
		PortForwards: make([]PortForwardSnapshot, 0, len(s.PortForwards)),
	}

	for _, pf := range s.PortForwards {
		pfSnapshot := pf.GetSnapshot()
		snapshot.PortForwards = append(snapshot.PortForwards, pfSnapshot)
		snapshot.TotalBytesIn += pfSnapshot.BytesIn
		snapshot.TotalBytesOut += pfSnapshot.BytesOut
		snapshot.TotalRateIn += pfSnapshot.RateIn
		snapshot.TotalRateOut += pfSnapshot.RateOut
	}

	return snapshot
}

// GetAllSnapshots returns current state for TUI rendering
func (r *Registry) GetAllSnapshots() []ServiceSnapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()

	snapshots := make([]ServiceSnapshot, 0, len(r.services))

	for _, svc := range r.services {
		snapshots = append(snapshots, svc.GetSnapshot())
	}

	return snapshots
}

// GetServiceSnapshot returns snapshot for single service
func (r *Registry) GetServiceSnapshot(key string) *ServiceSnapshot {
	r.mu.RLock()
	svc, ok := r.services[key]
	r.mu.RUnlock()

	if !ok {
		return nil
	}

	snapshot := svc.GetSnapshot()
	return &snapshot
}

// Subscribe returns a channel that receives updates at specified interval
func (r *Registry) Subscribe(interval time.Duration) (<-chan []ServiceSnapshot, func()) {
	ch := make(chan []ServiceSnapshot, 1)
	stopCh := make(chan struct{})

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		defer close(ch)

		for {
			select {
			case <-ticker.C:
				select {
				case ch <- r.GetAllSnapshots():
				default:
					// Channel full, skip this update
				}
			case <-stopCh:
				return
			case <-r.stopCh:
				return
			}
		}
	}()

	cancel := func() { close(stopCh) }
	return ch, cancel
}
