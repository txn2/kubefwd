package state

import (
	"sync"
)

// ForwardHistory stores rate history samples for a single port forward
type ForwardHistory struct {
	RateIn  []float64
	RateOut []float64
	maxSize int
}

// RateHistory stores rate history for all port forwards
type RateHistory struct {
	mu       sync.RWMutex
	forwards map[string]*ForwardHistory
	maxSize  int
}

// NewRateHistory creates a new rate history store with specified max samples
func NewRateHistory(maxSize int) *RateHistory {
	if maxSize <= 0 {
		maxSize = 60 // Default to 60 seconds of history
	}
	return &RateHistory{
		forwards: make(map[string]*ForwardHistory),
		maxSize:  maxSize,
	}
}

// AddSample adds a rate sample for a forward
func (h *RateHistory) AddSample(forwardKey string, rateIn, rateOut float64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	fh, exists := h.forwards[forwardKey]
	if !exists {
		fh = &ForwardHistory{
			RateIn:  make([]float64, 0, h.maxSize),
			RateOut: make([]float64, 0, h.maxSize),
			maxSize: h.maxSize,
		}
		h.forwards[forwardKey] = fh
	}

	// Append new sample
	fh.RateIn = append(fh.RateIn, rateIn)
	fh.RateOut = append(fh.RateOut, rateOut)

	// Trim if exceeding max size
	if len(fh.RateIn) > fh.maxSize {
		fh.RateIn = fh.RateIn[len(fh.RateIn)-fh.maxSize:]
	}
	if len(fh.RateOut) > fh.maxSize {
		fh.RateOut = fh.RateOut[len(fh.RateOut)-fh.maxSize:]
	}
}

// GetHistory returns the last 'count' rate samples for a forward
// Returns copies to prevent concurrent modification issues
func (h *RateHistory) GetHistory(forwardKey string, count int) (rateIn, rateOut []float64) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	fh, exists := h.forwards[forwardKey]
	if !exists {
		return nil, nil
	}

	// Calculate start index for requested count
	startIn := 0
	if len(fh.RateIn) > count {
		startIn = len(fh.RateIn) - count
	}
	startOut := 0
	if len(fh.RateOut) > count {
		startOut = len(fh.RateOut) - count
	}

	// Create copies
	rateIn = make([]float64, len(fh.RateIn)-startIn)
	copy(rateIn, fh.RateIn[startIn:])

	rateOut = make([]float64, len(fh.RateOut)-startOut)
	copy(rateOut, fh.RateOut[startOut:])

	return rateIn, rateOut
}

// GetAllHistory returns the full history for a forward
func (h *RateHistory) GetAllHistory(forwardKey string) (rateIn, rateOut []float64) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	fh, exists := h.forwards[forwardKey]
	if !exists {
		return nil, nil
	}

	// Create copies
	rateIn = make([]float64, len(fh.RateIn))
	copy(rateIn, fh.RateIn)

	rateOut = make([]float64, len(fh.RateOut))
	copy(rateOut, fh.RateOut)

	return rateIn, rateOut
}

// Remove removes history for a forward
func (h *RateHistory) Remove(forwardKey string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.forwards, forwardKey)
}

// Clear removes all history
func (h *RateHistory) Clear() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.forwards = make(map[string]*ForwardHistory)
}

// Size returns the number of forwards being tracked
func (h *RateHistory) Size() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.forwards)
}
